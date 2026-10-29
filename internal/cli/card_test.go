package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/sondt2709/inbox2agent/internal/app"
	"github.com/sondt2709/inbox2agent/internal/inbox"
	"github.com/sondt2709/inbox2agent/internal/notify"
	"github.com/sondt2709/inbox2agent/internal/store"
)

type result struct {
	out, errOut string
	err         error
}

// i2a runs the root command with the arguments and stdin, as a person or an agent does.
func i2a(t *testing.T, stdin string, args ...string) result {
	t.Helper()
	root := newRoot()
	var out, errOut bytes.Buffer
	root.SetIn(strings.NewReader(stdin))
	root.SetOut(&out)
	root.SetErr(&errOut)
	root.SetArgs(args)
	err := root.ExecuteContext(context.Background())
	return result{out.String(), errOut.String(), err}
}

// cardEnv makes an empty I2A_HOME with one stored message, and records the notifications.
func cardEnv(t *testing.T) (msgID int64, notes *[]string) {
	t.Helper()
	t.Setenv("I2A_HOME", t.TempDir())
	t.Setenv("I2A_SECRET_BACKEND", "file")
	paths, err := app.ResolvePaths()
	if err != nil {
		t.Fatal(err)
	}
	if err := paths.EnsureDataDir(); err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(paths.DBFile)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.SavePoll(context.Background(), "work", []inbox.Message{{SourceID: "work", SourceType: "mattermost",
		ExternalID: "p1", Author: "lena.wong", Text: "Can PF get their own login?", CreatedAt: time.Now()}}, nil, time.Now()); err != nil {
		t.Fatal(err)
	}
	p, _ := st.Pending(context.Background())
	st.Close()
	var got []string
	old := cardNotifier
	cardNotifier = func() notify.Notifier { return func(title, text string) { got = append(got, title+" | "+text) } }
	t.Cleanup(func() { cardNotifier = old })
	return p[0].ID, &got
}

func list(t *testing.T, args ...string) []cardJSON {
	t.Helper()
	r := i2a(t, "", append([]string{"card", "list", "--json"}, args...)...)
	if r.err != nil {
		t.Fatal(r.err)
	}
	var cards []cardJSON
	if err := json.Unmarshal([]byte(r.out), &cards); err != nil {
		t.Fatalf("list output is not JSON: %v\n%s", err, r.out)
	}
	return cards
}

func TestCardAddListDone(t *testing.T) {
	id, notes := cardEnv(t)
	in := `[{"category":"action","urgency":"high","group":"Harbor Portal","title":"Correct your reply to Lena",
	  "body":"admin:create exists.","reply":"I checked the code.","refs":[` + itoa(id) + `,99999],"key":"mm:lena"},
	 {"category":"question","group":"Harbor Portal","title":"Review PR #57"},
	 {"category":"info","urgency":"low","title":"Staging deployed"}]`
	r := i2a(t, in, "card", "add")
	if r.err != nil {
		t.Fatalf("card add: %v\n%s", r.err, r.errOut)
	}
	if !strings.Contains(r.out, "3 cards saved: 3 added. Notifications: 2.") {
		t.Errorf("output = %q", r.out)
	}
	if !strings.Contains(r.errOut, "warning: card 1: message 99999 is not in the store") {
		t.Errorf("stderr = %q", r.errOut)
	}
	want := "i2a · ACTION · high | Harbor Portal: Correct your reply to Lena\ni2a · QUESTION · normal | Harbor Portal: Review PR #57"
	if got := strings.Join(*notes, "\n"); got != want {
		t.Errorf("notifications:\n%s\nwant:\n%s", got, want)
	}

	cards := list(t)
	if len(cards) != 3 {
		t.Fatalf("list = %d cards", len(cards))
	}
	var lena cardJSON
	for _, c := range cards {
		if c.Key == "mm:lena" {
			lena = c
		}
	}
	if lena.State != "new" || lena.Urgency != "high" || len(lena.Refs) != 1 || lena.Refs[0] != id || lena.CreatedAt == "" {
		t.Errorf("card = %+v", lena)
	}

	if r := i2a(t, "", "card", "done", "--key", "mm:lena"); r.err != nil {
		t.Fatal(r.err)
	}
	if n := len(list(t)); n != 2 {
		t.Errorf("open cards after done = %d, want 2", n)
	}
	if n := len(list(t, "--all")); n != 3 {
		t.Errorf("all cards = %d, want 3", n)
	}
	if r := i2a(t, "", "card", "done", "--key", "missing"); r.err == nil || !strings.Contains(r.err.Error(), `no card has the key "missing"`) {
		t.Errorf("unknown key: %v", r.err)
	}
	if r := i2a(t, "", "card", "done"); r.err == nil || r.err.Error() != "give --key or --id" {
		t.Errorf("card done without --key or --id: %v", r.err)
	}
}

func TestCardAddRejectsAndSavesNothing(t *testing.T) {
	_, notes := cardEnv(t)
	r := i2a(t, `[{"category":"info","title":"ok"},{"category":"todo","title":"bad"}]`, "card", "add")
	if r.err == nil || !strings.Contains(r.err.Error(), `card 2: category "todo"`) {
		t.Fatalf("err = %v", r.err)
	}
	if n := len(list(t, "--all")); n != 0 {
		t.Errorf("cards after a rejected call = %d, want 0", n)
	}
	if len(*notes) != 0 {
		t.Errorf("notifications after a rejected call: %q", *notes)
	}
}

func TestCardAddUpdateByKeyAndNotifyOff(t *testing.T) {
	_, notes := cardEnv(t)
	if r := i2a(t, `{"category":"question","title":"Review PR #57","key":"mm:pr-57"}`, "card", "add"); r.err != nil {
		t.Fatal(r.err)
	}
	r := i2a(t, `{"category":"question","urgency":"high","title":"Review PR #57 before 18:30","key":"mm:pr-57"}`, "card", "add")
	if r.err != nil || !strings.Contains(r.out, "1 cards saved: 1 updated. Notifications: 1.") {
		t.Fatalf("update: %v %q", r.err, r.out)
	}
	if got := (*notes)[len(*notes)-1]; got != "i2a · QUESTION · high (updated) | Review PR #57 before 18:30" {
		t.Errorf("notification = %q", got)
	}
	if n := len(list(t)); n != 1 {
		t.Errorf("cards = %d, want 1 for one key", n)
	}

	paths, _ := app.ResolvePaths()
	if err := writeFile(paths.ConfigFile, "[notify]\nenabled = false\n"); err != nil {
		t.Fatal(err)
	}
	before := len(*notes)
	if r := i2a(t, `{"category":"action","urgency":"high","title":"Another"}`, "card", "add"); r.err != nil {
		t.Fatal(r.err)
	}
	if len(*notes) != before {
		t.Errorf("notified with notify.enabled = false: %q", (*notes)[before:])
	}
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }

func writeFile(path, text string) error { return os.WriteFile(path, []byte(text), 0o600) }

func TestCardAddRecordsTerminal(t *testing.T) {
	cardEnv(t)
	cardOf := func(key string) inbox.Card {
		t.Helper()
		paths, _ := app.ResolvePaths()
		st, err := store.Open(paths.DBFile)
		if err != nil {
			t.Fatal(err)
		}
		defer st.Close()
		c, err := st.Card(context.Background(), 0, key)
		if err != nil {
			t.Fatal(err)
		}
		return c
	}
	t.Setenv("ORCA_TERMINAL_HANDLE", "term_b9f22da8-13d6-49bc-a1ab-53eba56c9578")
	if r := i2a(t, `{"category":"info","title":"a","key":"k1"}`, "card", "add"); r.err != nil || r.errOut != "" {
		t.Fatalf("card add: %v %q", r.err, r.errOut)
	}
	if c := cardOf("k1"); c.Handle != "term_b9f22da8-13d6-49bc-a1ab-53eba56c9578" {
		t.Errorf("handle = %q", c.Handle)
	}

	t.Setenv("ORCA_TERMINAL_HANDLE", "term_x; rm -rf /")
	r := i2a(t, `{"category":"info","title":"b","key":"k2"}`, "card", "add")
	if r.err != nil {
		t.Fatal(r.err)
	}
	if !strings.Contains(r.errOut, "warning: ORCA_TERMINAL_HANDLE is not a terminal handle") {
		t.Errorf("stderr = %q", r.errOut)
	}
	if c := cardOf("k2"); c.Handle != "" {
		t.Errorf("handle = %q, want none", c.Handle)
	}

	t.Setenv("ORCA_TERMINAL_HANDLE", "")
	if r := i2a(t, `{"category":"info","title":"c","key":"k3"}`, "card", "add"); r.err != nil || r.errOut != "" {
		t.Fatalf("card add without Orca: %v %q", r.err, r.errOut)
	}
	if c := cardOf("k3"); c.Handle != "" {
		t.Errorf("handle = %q, want none", c.Handle)
	}
}
