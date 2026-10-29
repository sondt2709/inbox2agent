//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/sondt2709/inbox2agent/internal/inbox"
	"github.com/sondt2709/inbox2agent/internal/store"
)

// card is one card in the output of "i2a card list --json".
type card struct {
	ID        int64   `json:"id"`
	Key       string  `json:"key"`
	Category  string  `json:"category"`
	Urgency   string  `json:"urgency"`
	Title     string  `json:"title"`
	Body      string  `json:"body"`
	Refs      []int64 `json:"refs"`
	State     string  `json:"state"`
	UpdatedAt string  `json:"updated_at"`
}

// cards runs "i2a card list --json --all" and returns the cards.
func (p *person) cards() []card {
	p.t.Helper()
	out := filepath.Join(p.home, "cards.json")
	if _, code := p.run("card list --json --all > " + out); code != 0 {
		p.t.Fatalf("card list: exit %d", code)
	}
	b, err := os.ReadFile(out)
	if err != nil {
		p.t.Fatal(err)
	}
	var cs []card
	if err := json.Unmarshal(b, &cs); err != nil {
		p.t.Fatalf("card list output: %v\n%s", err, b)
	}
	return cs
}

func byTitle(cs []card, title string) card {
	for _, c := range cs {
		if c.Title == title {
			return c
		}
	}
	return card{}
}

// addCards writes the JSON to a file and runs "i2a card add < file", as an agent does with a heredoc.
func (p *person) addCards(json string, extraEnv ...string) (string, int) {
	p.t.Helper()
	f, err := os.CreateTemp(p.home, "cards-*.json")
	if err != nil {
		p.t.Fatal(err)
	}
	if _, err := f.WriteString(json); err != nil {
		p.t.Fatal(err)
	}
	f.Close()
	return p.run("card add < "+f.Name(), extraEnv...)
}

// shims makes pbcopy, open and xdg-open commands that write what they get to
// files, so the scenario can read what the Feed copied and opened.
func shims(t *testing.T) (dir, copied, opened string) {
	t.Helper()
	dir = t.TempDir()
	copied, opened = filepath.Join(dir, "copied.txt"), filepath.Join(dir, "opened.txt")
	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	write("pbcopy", "cat > "+copied)
	write("open", `printf '%s\n' "$1" >> `+opened)
	write("xdg-open", `printf '%s\n' "$1" >> `+opened)
	return dir, copied, opened
}

// S23: the agent adds cards with "i2a card add". The user runs "i2a", reads the
// Feed, sorts, filters, copies a reply, opens the thread and closes the card.
func TestS23CardFeed(t *testing.T) {
	w := mm.NewWorld(t)
	p := newPerson(t, "S23")
	p.addSource(w, w.Selected)
	w.Post(t, w.Bob, w.Selected, "S23 can PF get their own login for the demo?")
	if s, code := p.run("run --once"); code != 0 || !strings.Contains(s, "collected 1 new messages") {
		t.Fatalf("run --once: exit %d\n%s", code, s)
	}
	st, err := store.Open(filepath.Join(p.home, "inbox.db"))
	if err != nil {
		t.Fatal(err)
	}
	msgs, err := st.Messages(context.Background(), 1)
	st.Close()
	if err != nil || len(msgs) != 1 || !strings.HasPrefix(msgs[0].URL, "http") {
		t.Fatalf("stored messages = %+v, %v", msgs, err)
	}
	msg := msgs[0]

	s, code := p.addCards(fmt.Sprintf(`[
	  {"category":"info","urgency":"low","group":"Riverside CRM","title":"Staging deployed 2.14.0"},
	  {"category":"action","urgency":"high","group":"Harbor Portal","title":"Correct your reply about the demo login",
	   "body":"admin:create exists in api/src/cli.ts:248.","reply":"I checked the code: I was wrong. I will create the login today.",
	   "refs":[%d],"key":"mm:demo-login"},
	  {"category":"question","group":"Harbor Portal","title":"Review PR 57","body":"See https://evil.example"}]`, msg.ID))
	if code != 0 || !strings.Contains(s, "3 cards saved: 3 added") {
		t.Fatalf("card add: exit %d\n%s", code, s)
	}

	dir, copied, opened := shims(t)
	tm := p.term("", "PATH", dir+":"+os.Getenv("PATH"))
	tm.WaitFor(`1 Feed`, 30*time.Second)
	s = tm.WaitFor(`Correct your reply about the demo login`, 10*time.Second)
	for _, want := range []string{"3 new · 1 action · 1 question", "sort: urgency", "Suggested reply", "S23 can PF get"} {
		if !strings.Contains(s, want) {
			t.Errorf("the Feed does not show %q:\n%s", want, s)
		}
	}
	lines := strings.Split(s, "\n")
	first := -1
	for i, l := range lines {
		if strings.Contains(l, "Cards · 3") {
			first = i + 1
		}
	}
	if first < 0 || !strings.Contains(lines[first], "Correct your reply") {
		t.Errorf("the high action card is not first:\n%s", s)
	}

	tm.Press("s", `sort: newest`, 5*time.Second)
	tm.Press("s", `sort: urgency`, 5*time.Second)
	s = tm.Press("f", `Cards · 1`, 5*time.Second)
	if !strings.Contains(s, "Correct your reply") || strings.Contains(s, "Review PR 57") {
		t.Errorf("the action filter shows other cards:\n%s", s)
	}
	tm.Press("f", `Review PR 57`, 5*time.Second)
	tm.Press("f", `Staging deployed`, 5*time.Second)
	tm.Press("f", `Cards · 3`, 5*time.Second)

	tm.Press("c", `Copied the suggested reply`, 5*time.Second)
	tm.Press("o", `Opening http`, 5*time.Second)
	waitFile(t, opened, msg.URL)
	if b, _ := os.ReadFile(copied); string(b) != "I checked the code: I was wrong. I will create the login today." {
		t.Errorf("copied = %q", b)
	}

	// The question has a link only in its body: o opens nothing.
	tm.Press("Down", `▌.*QUESTION`, 5*time.Second)
	tm.Press("o", `This card has no message link`, 5*time.Second)

	tm.Press("Up", `▌.*ACTION`, 5*time.Second)
	tm.Press("d", `Done: Correct your reply`, 5*time.Second)
	tm.WaitFor(`Cards · 2`, 5*time.Second)
	tm.Press("q", `\[exit 0\]`, 5*time.Second)

	cs := p.cards()
	if c := byTitle(cs, "Correct your reply about the demo login"); c.State != inbox.CardDone || len(c.Refs) != 1 || c.Refs[0] != msg.ID {
		t.Errorf("action card = %+v, want done with the message reference", c)
	}
	if c := byTitle(cs, "Review PR 57"); c.State != inbox.CardRead {
		t.Errorf("question card = %+v, want read (the user moved to it)", c)
	}
	if c := byTitle(cs, "Staging deployed 2.14.0"); c.State != inbox.CardNew {
		t.Errorf("info card = %+v, want new", c)
	}
	if b, _ := os.ReadFile(opened); strings.Count(string(b), "\n") != 1 {
		t.Errorf("opened = %q, want only the message URL", b)
	}
}

func waitFile(t *testing.T, path, want string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		b, _ := os.ReadFile(path)
		if strings.TrimSpace(string(b)) == want {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("%s = %q, want %q", path, b, want)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// S24: the agent updates a card by its key and closes it; a card that is not
// valid makes the call fail and saves nothing. The Feed shows the update live.
func TestS24CardUpdateDoneAndReject(t *testing.T) {
	p := newPerson(t, "S24")
	if s, code := p.addCards(`{"category":"question","title":"Review PR 57","key":"mm:pr-57"}`); code != 0 {
		t.Fatalf("first add: exit %d\n%s", code, s)
	}
	tm := p.term("")
	tm.WaitFor(`Review PR 57`, 30*time.Second)

	s, code := p.addCards(`{"category":"question","urgency":"high","title":"Review PR 57 before 18:30","key":"mm:pr-57"}`)
	if code != 0 || !strings.Contains(s, "1 cards saved: 1 updated") {
		t.Fatalf("update: exit %d\n%s", code, s)
	}
	s = tm.WaitFor(`Review PR 57 before 18:30`, 5*time.Second) // the Feed reads the store every 2 seconds
	if !strings.Contains(s, "Cards · 1") || !strings.Contains(s, "updated") {
		t.Errorf("the Feed does not show one updated card:\n%s", s)
	}

	s, code = p.addCards(`[{"category":"info","title":"ok"},{"category":"todo","title":"bad"}]`)
	if code == 0 || !regexp.MustCompile(`(?i)card 2: category "todo" is not action, question or info`).MatchString(s) {
		t.Fatalf("bad card: exit %d\n%s", code, s)
	}
	if s, code := p.run("card done --key mm:pr-57"); code != 0 {
		t.Fatalf("card done: exit %d\n%s", code, s)
	}
	tm.WaitFor(`No cards yet|Cards · 0`, 5*time.Second)
	tm.Press("q", `\[exit 0\]`, 5*time.Second)

	cs := p.cards()
	if len(cs) != 1 || cs[0].State != inbox.CardDone || cs[0].Urgency != inbox.UrgencyHigh {
		t.Fatalf("cards = %+v, want one done high card and nothing from the rejected call", cs)
	}
}
