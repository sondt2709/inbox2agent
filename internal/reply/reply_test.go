package reply

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sondt2709/inbox2agent/internal/inbox"
	"github.com/sondt2709/inbox2agent/internal/source"
	"github.com/sondt2709/inbox2agent/internal/store"
)

type apiErr struct{}

func (apiErr) Error() string  { return "HTTP 404: Unable to get the post" }
func (apiErr) Answered() bool { return true }

// fake is a platform. It keeps the posts and can fail the next send.
type fake struct {
	posts   []inbox.ReplyRequest
	fail    error // returned by the next Reply
	keep    bool  // with fail: the post is created, but the answer is lost
	replies int
	finds   int
}

func (f *fake) Reply(_ context.Context, r inbox.ReplyRequest) (inbox.Posted, error) {
	f.replies++
	if err := f.fail; err != nil {
		f.fail = nil
		if f.keep {
			f.posts = append(f.posts, r)
		}
		return inbox.Posted{}, err
	}
	f.posts = append(f.posts, r)
	return inbox.Posted{ID: "post" + r.PendingID, URL: "https://chat.example/fx/pl/new"}, nil
}

func (f *fake) Find(_ context.Context, r inbox.ReplyRequest) (inbox.Posted, bool, error) {
	f.finds++
	for _, p := range f.posts {
		if p.Text == r.Text && p.Thread == r.Thread && p.Target.ID == r.Target.ID {
			return inbox.Posted{ID: "found"}, true, nil
		}
	}
	return inbox.Posted{}, false, nil
}

type env struct {
	st   *store.Store
	f    *fake
	card int64
	now  time.Time
}

func setup(t *testing.T) *env {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "inbox.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	ctx := context.Background()
	now := time.Date(2026, 10, 29, 14, 52, 0, 0, time.UTC)
	msgs := []inbox.Message{{SourceID: "work", SourceType: "mattermost", ExternalID: "p1", ChannelID: "c1",
		ChannelName: "pr-reviews", ChannelKind: inbox.KindChannel, Author: "minh.tran", Text: "Can you review PR 57?", CreatedAt: now}}
	if _, err := st.SavePoll(ctx, "work", msgs, nil, now); err != nil {
		t.Fatal(err)
	}
	saved, err := st.SaveCards(ctx, []inbox.Card{{Category: inbox.CategoryQuestion, Urgency: inbox.UrgencyNormal, Title: "Review PR 57",
		Reply: "Thanks Minh", Refs: []int64{1}}}, now)
	if err != nil {
		t.Fatal(err)
	}
	return &env{st: st, f: &fake{}, card: saved[0].ID, now: now}
}

func (e *env) send(t *testing.T, text string) (inbox.Reply, error) {
	t.Helper()
	e.now = e.now.Add(time.Second)
	r, _, err := Send(context.Background(), e.st, func(string) (source.Replier, error) { return e.f, nil },
		Request{MessageID: 1, CardID: e.card, Text: text, Place: inbox.PlaceThread, By: inbox.ByUser}, func() time.Time { return e.now })
	return r, err
}

func (e *env) cardState(t *testing.T) string {
	t.Helper()
	c, err := e.st.Card(context.Background(), e.card, "")
	if err != nil {
		t.Fatal(err)
	}
	return c.State
}

func TestSendRecordsTheReplyAndClosesTheCard(t *testing.T) {
	e := setup(t)
	r, err := e.send(t, "  Thanks Minh, I will review PR 57 today.\u200b  ")
	if err != nil {
		t.Fatal(err)
	}
	if len(e.f.posts) != 1 || e.f.posts[0].Text != "Thanks Minh, I will review PR 57 today." || !e.f.posts[0].Thread {
		t.Fatalf("posts = %+v, want one clean text in the thread", e.f.posts)
	}
	if r.State != inbox.ReplySent || r.ExternalID == "" || r.SentAt.IsZero() {
		t.Errorf("reply = %+v, want sent", r)
	}
	if got := e.cardState(t); got != inbox.CardDone {
		t.Errorf("card = %s, want done", got)
	}
	byCard, _ := e.st.SentReplies(context.Background(), []int64{e.card})
	if len(byCard[e.card]) != 1 || byCard[e.card][0].By != inbox.ByUser {
		t.Errorf("sent replies = %+v", byCard)
	}
}

func TestSameReplyAgainIsRefused(t *testing.T) {
	e := setup(t)
	if _, err := e.send(t, "Thanks Minh"); err != nil {
		t.Fatal(err)
	}
	_, err := e.send(t, "Thanks Minh")
	var already *AlreadySentError
	if !errors.As(err, &already) || !strings.Contains(err.Error(), "this reply was sent at") {
		t.Fatalf("err = %v, want AlreadySentError", err)
	}
	if len(e.f.posts) != 1 {
		t.Errorf("posts = %d, want 1", len(e.f.posts))
	}
}

func TestTimeOutThenRetryFindsThePost(t *testing.T) {
	e := setup(t)
	e.f.fail, e.f.keep = context.DeadlineExceeded, true // the server created the post, the answer is lost
	_, err := e.send(t, "Thanks Minh")
	var unknown *UnknownError
	if !errors.As(err, &unknown) {
		t.Fatalf("err = %v, want UnknownError", err)
	}
	if got := e.cardState(t); got == inbox.CardDone {
		t.Error("card is done after an unknown result")
	}
	r, err := e.send(t, "Thanks Minh")
	if err != nil {
		t.Fatal(err)
	}
	if e.f.replies != 1 || len(e.f.posts) != 1 || e.f.finds != 1 {
		t.Errorf("replies=%d posts=%d finds=%d, want 1 1 1", e.f.replies, len(e.f.posts), e.f.finds)
	}
	if r.State != inbox.ReplySent || r.ExternalID != "found" {
		t.Errorf("reply = %+v, want sent as the found post", r)
	}
	if got := e.cardState(t); got != inbox.CardDone {
		t.Errorf("card = %s, want done", got)
	}
}

func TestTimeOutWithoutAPostSendsAgainWithTheSamePendingID(t *testing.T) {
	e := setup(t)
	e.f.fail = errors.New("connection reset by peer")
	if _, err := e.send(t, "Thanks Minh"); err == nil {
		t.Fatal("no error")
	}
	if _, err := e.send(t, "Thanks Minh"); err != nil {
		t.Fatal(err)
	}
	if e.f.replies != 2 || e.f.finds != 1 || len(e.f.posts) != 1 {
		t.Errorf("replies=%d finds=%d posts=%d, want 2 1 1", e.f.replies, e.f.finds, len(e.f.posts))
	}
	if e.f.posts[0].PendingID != "1793285521000" {
		t.Errorf("pending ID = %q, want the time of the first try", e.f.posts[0].PendingID)
	}
}

func TestAnsweredErrorFailsAndKeepsTheCard(t *testing.T) {
	e := setup(t)
	e.f.fail = apiErr{}
	r, err := e.send(t, "Thanks Minh")
	if err == nil || !strings.Contains(err.Error(), "HTTP 404") {
		t.Fatalf("err = %v, want the server error", err)
	}
	if r.State != inbox.ReplyFailed {
		t.Errorf("state = %s, want failed", r.State)
	}
	if got := e.cardState(t); got == inbox.CardDone {
		t.Error("card is done after a failed send")
	}
	if _, err := e.send(t, "Thanks Minh"); err != nil {
		t.Fatal(err)
	}
	if e.f.finds != 0 || len(e.f.posts) != 1 {
		t.Errorf("finds=%d posts=%d, want no find after a failed send and 1 post", e.f.finds, len(e.f.posts))
	}
}

func TestCheckText(t *testing.T) {
	if _, err := CheckText(" \u200b\n "); !errors.Is(err, ErrEmpty) {
		t.Errorf("blank: %v, want ErrEmpty", err)
	}
	if _, err := CheckText(strings.Repeat("a", 4001)); err == nil || !strings.Contains(err.Error(), "the maximum is 4000") {
		t.Errorf("4001: %v", err)
	}
	if v, err := CheckText(strings.Repeat("é", 4000)); err != nil || len([]rune(v)) != 4000 {
		t.Errorf("4000: %d, %v", len([]rune(v)), err)
	}
	if v, _ := CheckText("@channel **ship it**"); v != "@channel **ship it**" {
		t.Errorf("mentions and Markdown changed: %q", v)
	}
}

func TestSendUnknownMessageAndSource(t *testing.T) {
	e := setup(t)
	_, _, err := Send(context.Background(), e.st, func(string) (source.Replier, error) { return e.f, nil },
		Request{MessageID: 99999, Text: "hi", Place: inbox.PlaceThread}, time.Now)
	if err == nil || !strings.Contains(err.Error(), "the store has no message 99999") {
		t.Errorf("unknown message: %v", err)
	}
	_, _, err = Send(context.Background(), e.st, func(id string) (source.Replier, error) {
		return nil, errors.New(`source "` + id + `" is not in the configuration`)
	}, Request{MessageID: 1, Text: "hi", Place: inbox.PlaceThread}, time.Now)
	if err == nil || !strings.Contains(err.Error(), `source "work"`) {
		t.Errorf("removed source: %v", err)
	}
}

func TestDefaults(t *testing.T) {
	t0 := time.Now()
	msgs := []inbox.Message{{ID: 1, CreatedAt: t0}, {ID: 2, CreatedAt: t0.Add(time.Minute)}, {ID: 3, CreatedAt: t0}}
	if m, ok := DefaultTarget(msgs); !ok || m.ID != 2 {
		t.Errorf("target = %d, want the newest 2", m.ID)
	}
	if _, ok := DefaultTarget(nil); ok {
		t.Error("target of no message")
	}
	for _, c := range []struct {
		m    inbox.Message
		want string
	}{
		{inbox.Message{ChannelKind: inbox.KindChannel}, inbox.PlaceThread},
		{inbox.Message{ChannelKind: inbox.KindDirect}, inbox.PlaceChannel},
		{inbox.Message{ChannelKind: inbox.KindGroup}, inbox.PlaceChannel},
		{inbox.Message{ChannelKind: inbox.KindDirect, ThreadID: "root"}, inbox.PlaceThread},
	} {
		if got := DefaultPlace(c.m); got != c.want {
			t.Errorf("place of %+v = %s, want %s", c.m, got, c.want)
		}
	}
}
