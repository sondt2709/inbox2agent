package tui

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/sondt2709/inbox2agent/internal/config"
	"github.com/sondt2709/inbox2agent/internal/inbox"
	"github.com/sondt2709/inbox2agent/internal/source"
)

type notFound struct{}

func (notFound) Error() string  { return "mattermost: HTTP 404: Unable to get the post" }
func (notFound) Answered() bool { return true }

// tuiPlatform is a fake source that records the replies.
type tuiPlatform struct {
	posts []inbox.ReplyRequest
	fail  error
}

func (p *tuiPlatform) Reply(_ context.Context, r inbox.ReplyRequest) (inbox.Posted, error) {
	if p.fail != nil {
		return inbox.Posted{}, p.fail
	}
	p.posts = append(p.posts, r)
	return inbox.Posted{ID: "newpost"}, nil
}

func (p *tuiPlatform) Find(context.Context, inbox.ReplyRequest) (inbox.Posted, bool, error) {
	return inbox.Posted{}, false, nil
}

// newReplyFeed is the Feed harness with the account "work" in the configuration
// and a fake platform.
func newReplyFeed(t *testing.T) (*feedHarness, *tuiPlatform) {
	t.Helper()
	h := newFeedModel(t)
	h.m.u.cfg.Accounts = []config.Account{{ID: "work", Platform: config.PlatformMattermost, URL: "https://chat.example.com"}}
	p := &tuiPlatform{}
	h.m.u.d.Replier = func(_ *config.Config, id string) (source.Replier, error) {
		if id != "work" {
			t.Errorf("replier of account %q", id)
		}
		return p, nil
	}
	return h, p
}

func typeText(m *model, s string) {
	for _, r := range s {
		send(m, string(r))
	}
}

func TestFeedReplyEditAndSend(t *testing.T) {
	h, p := newReplyFeed(t)
	send(h.m, "r")
	s := screen(h.m)
	for _, want := range []string{"Reply · card", "Send to    @lena.wong · mattermost · work", "Reply to   #1 lena.wong",
		"Chat", "A thread reply keeps the read state. A chat post marks it read.", "I checked the code: I was wrong.",
		"ctrl+s send", "esc cancel"} {
		if !strings.Contains(s, want) {
			t.Errorf("the reply form does not show %q", want)
		}
	}
	if st := h.state(t, "Correct your reply to Lena"); st != inbox.CardRead {
		t.Errorf("state after r = %s, want read", st)
	}
	typeText(h.m, " Sorry.")
	send(h.m, "shift+tab", "space") // Where: Thread
	send(h.m, "ctrl+s")

	if len(p.posts) != 1 {
		t.Fatalf("posts = %d, want 1", len(p.posts))
	}
	got := p.posts[0]
	if got.Text != "I checked the code: I was wrong. Sorry." || !got.Thread || got.Target.ExternalID != "p1" {
		t.Errorf("post = %+v, want the edited text in the thread of p1", got)
	}
	if st := h.state(t, "Correct your reply to Lena"); st != inbox.CardDone {
		t.Errorf("state after the send = %s, want done", st)
	}
	s = screen(h.m)
	if !strings.Contains(s, "Sent to @lena.wong (thread). The card is done.") || strings.Contains(s, "Reply · card") {
		t.Errorf("screen after the send:\n%s", s)
	}
	send(h.m, "a") // show the done card
	for range 3 {
		if c, _ := h.m.selectedCard(); c.Title == "Correct your reply to Lena" {
			break
		}
		send(h.m, "up")
	}
	if s := screen(h.m); !strings.Contains(s, "✓ Replied") || !strings.Contains(s, "by you → @lena.wong (thread)") {
		t.Errorf("the done card does not show the reply:\n%s", s)
	}
}

func TestFeedReplyErrorKeepsTheForm(t *testing.T) {
	h, p := newReplyFeed(t)
	p.fail = notFound{}
	send(h.m, "r", "ctrl+s")
	s := screen(h.m)
	if !strings.Contains(s, "Reply · card") || !strings.Contains(s, "HTTP 404") || !strings.Contains(s, "I checked the code") {
		t.Errorf("the form does not stay open with the text and the error:\n%s", s)
	}
	if h.m.form == nil || !strings.Contains(h.m.form.field("text").err, "HTTP 404") {
		t.Error("the error is not under the Text field")
	}
	if st := h.state(t, "Correct your reply to Lena"); st == inbox.CardDone {
		t.Error("the card is done after a failed send")
	}
	p.fail = nil
	send(h.m, "ctrl+s")
	if len(p.posts) != 1 || h.state(t, "Correct your reply to Lena") != inbox.CardDone {
		t.Errorf("retry: posts = %d", len(p.posts))
	}
}

func TestFeedReplyEmptyTextAndCancel(t *testing.T) {
	h, p := newReplyFeed(t)
	send(h.m, "down", "r") // Review PR #57: no suggested reply, a channel message
	s := screen(h.m)
	if !strings.Contains(s, "Thread") || !strings.Contains(s, "Channel") {
		t.Errorf("the form of a channel message:\n%s", s)
	}
	send(h.m, "ctrl+s")
	if s := screen(h.m); !strings.Contains(s, "the text is empty") {
		t.Errorf("no empty text error:\n%s", s)
	}
	send(h.m, "esc")
	if s := screen(h.m); strings.Contains(s, "Reply · card") || !strings.Contains(s, "Reply cancelled. Nothing is sent.") {
		t.Errorf("esc does not close the form:\n%s", s)
	}
	if len(p.posts) != 0 {
		t.Errorf("posts = %d, want 0", len(p.posts))
	}
}

func TestFeedReplyRefusals(t *testing.T) {
	h, p := newReplyFeed(t)
	send(h.m, "down", "down", "r") // the info card has no message
	if s := screen(h.m); !strings.Contains(s, "The card has no message to reply to.") || strings.Contains(s, "Reply · card") {
		t.Errorf("card without a message:\n%s", s)
	}
	h.m.u.cfg.Accounts = nil
	send(h.m, "up", "up", "r")
	if s := screen(h.m); !strings.Contains(s, `account "work" is not in the configuration`) || strings.Contains(s, "Reply · card") {
		t.Errorf("removed source:\n%s", s)
	}
	if len(p.posts) != 0 {
		t.Errorf("posts = %d, want 0", len(p.posts))
	}
}

// The question card has "See https://evil.example" in its body and refers to
// message p2. The reply goes to p2, never to the link.
func TestFeedReplyTargetIsTheStoredMessage(t *testing.T) {
	h, p := newReplyFeed(t)
	send(h.m, "down", "r")
	typeText(h.m, "Thanks, I will review it.")
	send(h.m, "ctrl+s")
	if len(p.posts) != 1 || p.posts[0].Target.ExternalID != "p2" || !p.posts[0].Thread {
		t.Fatalf("posts = %+v, want one reply in the thread of the stored message p2", p.posts)
	}
	if strings.Contains(p.posts[0].Target.URL, "evil") {
		t.Errorf("target URL = %q", p.posts[0].Target.URL)
	}
}

// A WhatsApp group message: the form shows Quote and Chat, quotes by default,
// and does not promise a read state.
func TestFeedReplyWhatsAppGroup(t *testing.T) {
	h, _ := newReplyFeed(t)
	h.m.u.cfg.Accounts = append(h.m.u.cfg.Accounts, config.Account{ID: "phone", Platform: config.PlatformWhatsApp})
	p := &tuiPlatform{}
	h.m.u.d.Replier = func(_ *config.Config, id string) (source.Replier, error) {
		if id != "phone" {
			t.Errorf("replier of account %q", id)
		}
		return p, nil
	}
	ctx := context.Background()
	at := time.Now()
	if _, err := h.st.SavePoll(ctx, "phone", []inbox.Message{{SourceID: "phone", SourceType: "whatsapp", ExternalID: "3EB0AA",
		ChannelID: "120363000000000001@g.us", ChannelName: "Harbor team", ChannelKind: inbox.KindGroup, Author: "Lan",
		Text: "Can you join at 3?", CreatedAt: at}}, nil, at); err != nil {
		t.Fatal(err)
	}
	var id int64
	pending, _ := h.st.Pending(ctx)
	for _, m := range pending {
		if m.SourceID == "phone" {
			id = m.ID
		}
	}
	if _, err := h.st.SaveCards(ctx, []inbox.Card{{Category: inbox.CategoryQuestion, Urgency: inbox.UrgencyHigh, Group: "Harbor",
		Title: "Lan asks about 3pm", Reply: "Yes, I join at 3.", Refs: []int64{id}}}, at); err != nil {
		t.Fatal(err)
	}
	h.m.Update(h.m.load()())
	for range 4 {
		if c, _ := h.m.selectedCard(); c.Title == "Lan asks about 3pm" {
			break
		}
		send(h.m, "up")
	}

	send(h.m, "r")
	s := screen(h.m)
	for _, want := range []string{"Send to    Harbor team · whatsapp · phone", "Quote", "Chat", "i2a sends no read receipt and no typing state."} {
		if !strings.Contains(s, want) {
			t.Errorf("the reply form does not show %q:\n%s", want, s)
		}
	}
	if strings.Contains(s, "Thread") || strings.Contains(s, "keeps the read state") {
		t.Errorf("the WhatsApp form shows the Mattermost words:\n%s", s)
	}
	if h.m.form.field("where").value() != inbox.PlaceThread {
		t.Errorf("default place = %s, want the quote (thread)", h.m.form.field("where").value())
	}
	send(h.m, "ctrl+s")
	if len(p.posts) != 1 || !p.posts[0].Thread {
		t.Fatalf("posts = %+v, want one quote", p.posts)
	}
	if s := screen(h.m); !strings.Contains(s, "Sent to Harbor team (quote). The card is done.") {
		t.Errorf("status after the send:\n%s", s)
	}
}
