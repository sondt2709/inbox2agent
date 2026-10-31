//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/sondt2709/inbox2agent/internal/config"
	"github.com/sondt2709/inbox2agent/internal/inbox"
	"github.com/sondt2709/inbox2agent/internal/store"
)

// fixtureHandle is the terminal in the fixture show_live.json.
const fixtureHandle = "term_b9f22da8-13d6-49bc-a1ab-53eba56c9578"

// fakeOrca makes an orca executable that prints the real orca 1.4.240 outputs
// of internal/agent/testdata. It writes each call to calls.log and the text of
// "terminal send" to prompt-<n>.txt.
func fakeOrca(t *testing.T) (bin, dir string) {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	data := filepath.Join(filepath.Dir(file), "..", "..", "internal", "agent", "testdata")
	dir = t.TempDir()
	bin = filepath.Join(dir, "orca")
	script := `#!/bin/sh
dir=` + shellQuote(dir) + `
data=` + shellQuote(data) + `
cmd="$1 $2"
echo "$*" | head -c 300 >> "$dir/calls.log"; echo >> "$dir/calls.log"
prev=""
for a in "$@"; do
  if [ "$prev" = "--text" ]; then
    n=$(ls "$dir" | grep -c '^prompt-')
    printf '%s' "$a" > "$dir/prompt-$((n+1)).txt"
  fi
  prev="$a"
done
case "$cmd" in
  "terminal show") cat "$data/show_live.json" ;;
  "terminal wait") cat "$data/wait_idle.json" ;;
  "terminal send") cat "$data/send_ok.json" ;;
  *) echo "fake orca: no fixture for $cmd" >&2; exit 1 ;;
esac
`
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return bin, dir
}

func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

// asksOf reads the asks of the card with the key from the store.
func (p *person) asksOf(key string) (inbox.Card, []inbox.Ask) {
	p.t.Helper()
	st, err := store.Open(filepath.Join(p.home, "inbox.db"))
	if err != nil {
		p.t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	c, err := st.Card(ctx, 0, key)
	if err != nil {
		p.t.Fatal(err)
	}
	by, err := st.Asks(ctx, []int64{c.ID})
	if err != nil {
		p.t.Fatal(err)
	}
	return c, by[c.ID]
}

// askSetup collects one post of bob with a dry-run batch, and adds a card
// about it from the Orca terminal of the fixtures.
func askSetup(t *testing.T, id, key string) *person {
	t.Helper()
	w := mm.NewWorld(t)
	p := newPerson(t, id)
	p.addAccount(w, w.Selected)
	post := w.Post(t, w.Bob, w.Selected, id+" can PF get their own login for the demo?")
	if s, code := p.run("run --once"); code != 0 || !strings.Contains(s, "collected 1 new messages") {
		t.Fatalf("run --once: exit %d\n%s", code, s)
	}
	if s, code := p.addCards(fmt.Sprintf(`{"category":"action","urgency":"high","title":"Lena needs an answer about the PF login",
	  "body":"I did not check the auth code.","refs":[%d],"key":%q}`, p.storedID(post), key), "ORCA_TERMINAL_HANDLE", fixtureHandle); code != 0 {
		t.Fatalf("card add: exit %d\n%s", code, s)
	}
	return p
}

// S30: the agent of Unsorted is a dry run. The Feed and "i2a ask" refuse the ask.
func TestS30AskDryRun(t *testing.T) {
	p := askSetup(t, "S30", "mm:s30")
	tm := p.term("")
	tm.WaitFor(`Lena needs an answer`, 30*time.Second)
	s := tm.Press("t", `Cannot ask: The space Unsorted has no agent that reads asks\.`, 5*time.Second)
	if strings.Contains(s, "Ask the agent · card") {
		t.Errorf("the ask form opened for a dry run:\n%s", s)
	}
	tm.Press("q", `\[exit 0\]`, 5*time.Second)
	s, code := p.run("ask --card mm:s30 < " + p.stdinFile("S30 research more."))
	if code != 1 || !strings.Contains(s, "The space Unsorted has no agent that reads asks.") {
		t.Errorf("i2a ask: exit %d\n%s", code, s)
	}
	if _, asks := p.asksOf("mm:s30"); len(asks) != 0 {
		t.Errorf("asks = %+v, want none", asks)
	}
}

// S31: the card came from an Orca terminal. The user asks the agent from the
// Feed with t, then with "i2a ask". Both asks go to that terminal, with the
// text of the user as the instruction and the card in one data block.
func TestS31AskTheAgent(t *testing.T) {
	p := askSetup(t, "S31", "mm:s31")
	orcaBin, calls := fakeOrca(t)
	p.editConfig(func(c *config.Config) {
		pr := &c.Agents[0]
		pr.Driver, pr.Workspace, pr.Session, pr.OrcaBin = config.DriverOrca, t.TempDir(), config.SessionReuse, orcaBin
	})

	tm := p.term("")
	tm.WaitFor(`Lena needs an answer`, 30*time.Second)
	s := tm.Press("t", `Ask the agent · card`, 5*time.Second)
	for _, want := range []string{"Agent      triage · orca · space Unsorted", "Session    terminal term_b9f22da8… (wrote the card)",
		"The agent answers in this card.", "ctrl+s send"} {
		if !strings.Contains(s, want) {
			t.Errorf("the ask form does not show %q:\n%s", want, s)
		}
	}
	tm.Type("S31 check the auth code and give me the effort in hours.")
	tm.WaitFor(`effort in hours\.`, 5*time.Second)
	tm.Press("C-s", `Sent to the agent of Unsorted\. It answers in this card\.`, 30*time.Second)
	s = tm.WaitFor(`↪ asked`, 5*time.Second)
	if !regexp.MustCompile(`↪ Asked \d\d:\d\d: S31 check the auth code .* · sent`).MatchString(s) { // the panel cuts the text
		t.Errorf("the detail panel does not show the ask:\n%s", s)
	}
	tm.Shot("S31-feed-after-ask")
	tm.Press("q", `\[exit 0\]`, 5*time.Second)

	prompt, err := os.ReadFile(filepath.Join(calls, "prompt-1.txt"))
	if err != nil {
		t.Fatal(err)
	}
	ps := string(prompt)
	for _, want := range []string{`about card 1 (key "mm:s31")`, "--- ask ---\nS31 check the auth code and give me the effort in hours.\n--- end ---",
		`card add and the key "mm:s31"`, `S31 can PF get their own login for the demo?`} {
		if !strings.Contains(ps, want) {
			t.Errorf("the prompt does not contain %q:\n%s", want, ps)
		}
	}
	if n := strings.Count(ps, "```i2a-data-"); n != 1 || strings.Count(ps, "I did not check the auth code.") != 1 {
		t.Errorf("the prompt has %d data fences:\n%s", n, ps)
	}
	if i, j := strings.Index(ps, "```i2a-data-"), strings.Index(ps, "I did not check the auth code."); j < i {
		t.Errorf("the card body is outside the data block:\n%s", ps)
	}
	log, _ := os.ReadFile(filepath.Join(calls, "calls.log"))
	if !strings.Contains(string(log), "terminal show --terminal "+fixtureHandle) || !strings.Contains(string(log), "terminal send --terminal "+fixtureHandle) {
		t.Errorf("orca calls:\n%s", log)
	}

	s, code := p.run("ask --card mm:s31 < " + p.stdinFile("S31 and list the risks."))
	if code != 0 || !strings.Contains(s, "sent to the agent of the space Unsorted ("+fixtureHandle+"). Ask 2.") {
		t.Errorf("i2a ask: exit %d\n%s", code, s)
	}
	c, asks := p.asksOf("mm:s31")
	if len(asks) != 2 || asks[0].State != inbox.AskSent || asks[1].State != inbox.AskSent || asks[0].Handle != fixtureHandle ||
		asks[1].Text != "S31 and list the risks." {
		t.Errorf("asks = %+v", asks)
	}
	if c.State != inbox.CardRead || c.Handle != fixtureHandle {
		t.Errorf("card = %+v, want read with the fixture terminal", c)
	}
	t.Logf("S31 asks: %+v", asks)
}
