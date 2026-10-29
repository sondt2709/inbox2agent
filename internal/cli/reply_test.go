package cli

import (
	"context"
	"strconv"
	"strings"
	"testing"

	"github.com/sondt2709/inbox2agent/internal/config"
	"github.com/sondt2709/inbox2agent/internal/inbox"
	"github.com/sondt2709/inbox2agent/internal/notify"
	"github.com/sondt2709/inbox2agent/internal/reply"
	"github.com/sondt2709/inbox2agent/internal/secrets"
	"github.com/sondt2709/inbox2agent/internal/source"
)

type cliPlatform struct{ posts []inbox.ReplyRequest }

func (p *cliPlatform) Reply(_ context.Context, r inbox.ReplyRequest) (inbox.Posted, error) {
	p.posts = append(p.posts, r)
	return inbox.Posted{ID: "post" + strconv.Itoa(len(p.posts))}, nil
}

func (p *cliPlatform) Find(context.Context, inbox.ReplyRequest) (inbox.Posted, bool, error) {
	return inbox.Posted{}, false, nil
}

// replyEnv is cardEnv with a fake platform and recorded reply notifications.
func replyEnv(t *testing.T) (msgID int64, p *cliPlatform, notes *[]string) {
	t.Helper()
	msgID, _ = cardEnv(t)
	p = &cliPlatform{}
	oldB, oldN := replyBuilder, replyNotifier
	replyBuilder = func(*config.Config, secrets.Store) reply.Builder {
		return func(string) (source.Replier, error) { return p, nil }
	}
	var got []string
	replyNotifier = func() notify.Notifier { return func(title, text string) { got = append(got, title+" | "+text) } }
	t.Cleanup(func() { replyBuilder, replyNotifier = oldB, oldN })
	return msgID, p, &got
}

func TestReplyToAMessage(t *testing.T) {
	id, p, notes := replyEnv(t)
	r := i2a(t, "Hi Lena, PF can use the shared demo account.\n", "reply", "--to", strconv.FormatInt(id, 10))
	if r.err != nil {
		t.Fatal(r.err)
	}
	if len(p.posts) != 1 || p.posts[0].Text != "Hi Lena, PF can use the shared demo account." || p.posts[0].Thread {
		t.Fatalf("posts = %+v, want one post in the channel", p.posts)
	}
	if !strings.HasPrefix(r.out, "sent to ") || !strings.Contains(r.out, "(channel). Reply 1.") {
		t.Errorf("output = %q", r.out)
	}
	if len(*notes) != 1 || !strings.HasPrefix((*notes)[0], "i2a · the agent sent a reply | ") ||
		!strings.Contains((*notes)[0], "Hi Lena, PF can use") {
		t.Errorf("notes = %q", *notes)
	}
	r = i2a(t, "Hi Lena, PF can use the shared demo account.", "reply", "--to", strconv.FormatInt(id, 10))
	if r.err == nil || !strings.Contains(r.err.Error(), "this reply was sent at") || len(p.posts) != 1 {
		t.Errorf("same reply again: %v, posts = %d", r.err, len(p.posts))
	}
}

func TestReplyWithTheSuggestedReplyOfACard(t *testing.T) {
	id, p, _ := replyEnv(t)
	add := i2a(t, `{"category":"action","title":"Answer Lena","reply":"Use the shared demo account.","refs":[`+
		strconv.FormatInt(id, 10)+`],"key":"mm:lena"}`, "card", "add")
	if add.err != nil {
		t.Fatal(add.err)
	}
	r := i2a(t, "", "reply", "--card", "mm:lena", "--thread")
	if r.err != nil {
		t.Fatal(r.err)
	}
	if len(p.posts) != 1 || p.posts[0].Text != "Use the shared demo account." || !p.posts[0].Thread {
		t.Fatalf("posts = %+v, want the card reply in the thread", p.posts)
	}
	if !strings.Contains(r.out, "(thread of lena.wong)") || !strings.Contains(r.out, "The card is done.") {
		t.Errorf("output = %q", r.out)
	}
	if cards := list(t); len(cards) != 0 {
		t.Errorf("open cards = %+v, want none", cards)
	}
}

func TestReplyErrors(t *testing.T) {
	id, p, notes := replyEnv(t)
	to := strconv.FormatInt(id, 10)
	for name, c := range map[string]struct {
		stdin string
		args  []string
		want  string
	}{
		"no target":    {"hi", nil, "give --to <message id> or --card <id|key>"},
		"both places":  {"hi", []string{"--to", to, "--thread", "--channel"}, "not both"},
		"unknown":      {"hi", []string{"--to", "99999"}, "the store has no message 99999"},
		"unknown card": {"", []string{"--card", "mm:none"}, `no card has the ID or key "mm:none"`},
		"empty":        {" \n", []string{"--to", to}, "the text is empty"},
		"too long":     {strings.Repeat("a", 4500), []string{"--to", to}, "the maximum is 4000"},
	} {
		r := i2a(t, c.stdin, append([]string{"reply"}, c.args...)...)
		if r.err == nil || !strings.Contains(r.err.Error(), c.want) {
			t.Errorf("%s: err = %v, want %q", name, r.err, c.want)
		}
	}
	if len(p.posts) != 0 || len(*notes) != 0 {
		t.Errorf("posts = %d, notes = %d, want none", len(p.posts), len(*notes))
	}
}
