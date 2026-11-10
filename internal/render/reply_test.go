package render

import (
	"strings"
	"testing"

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

	r := inline(t, answer, mine, plain)
	got := content(t, r.Prompt).Conversations[0].Messages
	if x := got[0].ReplyTo; x == nil || *x != (Reply{ID: 812, From: "alice", Me: false, Text: "deploy at 5?"}) {
		t.Errorf("reply_to of 900 = %+v", x)
	}
	if x := got[1].ReplyTo; x == nil || *x != (Reply{ID: 0, From: "", Me: true, Text: "my question"}) {
		t.Errorf("reply_to of 901 = %+v", x)
	}
	if got[2].ReplyTo != nil {
		t.Errorf("reply_to of 902 = %+v, want none", got[2].ReplyTo)
	}
	if !strings.Contains(r.Prompt, "        reply_to:\n          id: 812\n          from: alice\n          me: false\n          text: deploy at 5?\n") {
		t.Errorf("reply_to of 900 in the prompt:\n%s", r.Prompt)
	}
	if !strings.Contains(r.Prompt, ContentNote) {
		t.Error("the prompt does not explain reply_to")
	}
}

func TestReplyToIsSanitized(t *testing.T) {
	msg := m("yes")
	msg.ReplyTo = &inbox.ReplyRef{Author: "al\u202Eice\nx", Text: "```" + strings.Repeat("q", 300) + "\u200B"}
	r := Items([]inbox.Message{msg}, 100)[0].ReplyTo
	if r == nil || r.From != "alice x" || len([]rune(r.Text)) != replyMax+1 || !strings.HasSuffix(r.Text, "…") || !strings.HasPrefix(r.Text, "'''q") {
		t.Errorf("reply_to = %+v; want a clean author and %d characters and …", r, replyMax)
	}
}
