package render

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/sondt2709/inbox2agent/internal/config"
	"github.com/sondt2709/inbox2agent/internal/inbox"
)

func TestItemsReplyTo(t *testing.T) {
	answer := m("yes")
	answer.ID = 900
	answer.ReplyTo = &inbox.ReplyRef{Ext: "abc", ID: 812, Author: "alice", Text: "deploy at 5?"}
	mine := m("ok")
	mine.ID = 901
	mine.ReplyTo = &inbox.ReplyRef{Ext: "def", Me: true, Text: "my\nquestion"}
	plain := m("hello")
	plain.ID = 902

	var lines []string
	for _, it := range Items([]inbox.Message{answer, mine, plain}, 100) {
		b, err := json.Marshal(it)
		if err != nil {
			t.Fatal(err)
		}
		lines = append(lines, string(b))
	}
	for _, want := range []struct{ line, has string }{
		{lines[0], `"reply_to":{"id":812,"author":"alice","me":false,"text":"deploy at 5?"}`},
		{lines[1], `"reply_to":{"id":0,"author":"","me":true,"text":"my question"}`},
	} {
		if !strings.Contains(want.line, want.has) {
			t.Errorf("line %s\nwant %s", want.line, want.has)
		}
	}
	if strings.Contains(lines[2], "reply_to") {
		t.Errorf("line %s has reply_to, want none", lines[2])
	}
	for _, l := range lines {
		if !strings.HasPrefix(l, `{"id":`) {
			t.Errorf("line %s does not start with {\"id\":", l)
		}
	}
}

func TestReplyToIsSanitized(t *testing.T) {
	msg := m("yes")
	msg.ReplyTo = &inbox.ReplyRef{Author: "al\u202Eice\nx", Text: strings.Repeat("q", 300) + "\u200B"}
	r := Items([]inbox.Message{msg}, 100)[0].ReplyTo
	if r == nil || r.Author != "alice x" || len([]rune(r.Text)) != replyMax+1 || !strings.HasSuffix(r.Text, "…") {
		t.Errorf("reply_to = %+v; want a clean author and %d characters and …", r, replyMax)
	}
}

// basePrimerHash is the primer hash of the default agent with the card
// section of "i2a" before the change add-quote-context (13d7f62).
const basePrimerHash = "a248f227d8600d71"

func TestReplyNoteDoesNotChangeThePrimer(t *testing.T) {
	msg := m("yes")
	msg.ReplyTo = &inbox.ReplyRef{Ext: "abc", ID: 812, Author: "alice", Text: "deploy at 5?"}
	for _, payload := range []string{config.PayloadPointer, config.PayloadInline} {
		p := config.NewAgent("triage")
		p.Payload = payload
		r, err := Render(p, "Acme", "b1", []inbox.Message{msg}, t.TempDir(), "i2a")
		if err != nil {
			t.Fatal(err)
		}
		if r.PrimerHash != basePrimerHash {
			t.Errorf("%s: primer hash = %s, want %s", payload, r.PrimerHash, basePrimerHash)
		}
		b, _ := os.ReadFile(r.DigestPath)
		if !strings.Contains(string(b), ReplyNote) {
			t.Errorf("%s: digest has no reply note:\n%s", payload, b)
		}
		if payload == config.PayloadInline && !strings.Contains(r.Prompt, ReplyNote) {
			t.Errorf("inline prompt has no reply note:\n%s", r.Prompt)
		}
	}
	a, err := Ask(config.NewAgent("triage"), "Acme", "a1", AskInput{ID: 1, Text: "why?", Card: inbox.Card{ID: 2},
		Messages: []inbox.Message{msg}}, "i2a")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(a.Prompt, ReplyNote) || !strings.Contains(a.Prompt, `"reply_to":{"id":812`) {
		t.Errorf("ask prompt has no reply note or reply_to:\n%s", a.Prompt)
	}
	if a.PrimerHash != basePrimerHash {
		t.Errorf("ask primer hash = %s, want %s", a.PrimerHash, basePrimerHash)
	}
}
