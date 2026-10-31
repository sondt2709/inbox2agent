//go:build e2e

package e2e

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sondt2709/inbox2agent/internal/config"
	"github.com/sondt2709/inbox2agent/internal/inbox"
	"github.com/sondt2709/inbox2agent/internal/store"
	"github.com/sondt2709/inbox2agent/test/e2e/harness"
)

// storedID returns the store ID of the message with the external ID.
func (p *person) storedID(externalID string) int64 {
	p.t.Helper()
	st, err := store.Open(filepath.Join(p.home, "inbox.db"))
	if err != nil {
		p.t.Fatal(err)
	}
	defer st.Close()
	for id := int64(1); id < 100; id++ {
		msgs, err := st.MessagesByID(context.Background(), []int64{id})
		if err != nil {
			p.t.Fatal(err)
		}
		if len(msgs) == 1 && msgs[0].ExternalID == externalID {
			return id
		}
	}
	p.t.Fatalf("no stored message %s", externalID)
	return 0
}

// replies reads the replies of the store.
func (p *person) replies(messageID int64, place, text string) inbox.Reply {
	p.t.Helper()
	st, err := store.Open(filepath.Join(p.home, "inbox.db"))
	if err != nil {
		p.t.Fatal(err)
	}
	defer st.Close()
	r, _, err := st.FindReply(context.Background(), messageID, place, text)
	if err != nil {
		p.t.Fatal(err)
	}
	return r
}

// stdinFile writes the text to a file for "i2a reply < file", as an agent does with a heredoc.
func (p *person) stdinFile(text string) string {
	p.t.Helper()
	f, err := os.CreateTemp(p.home, "reply-*.txt")
	if err != nil {
		p.t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.WriteString(text); err != nil {
		p.t.Fatal(err)
	}
	return f.Name()
}

// S26: bob writes a post and a reply in its thread. The user opens the Feed,
// presses r, edits the suggested reply and presses ctrl+s. The post goes to
// the thread root, the channel stays unread and the card is done.
func TestS26ReplyFromTheFeed(t *testing.T) {
	w := mm.NewWorld(t)
	p := newPerson(t, "S26")
	p.addAccount(w, w.Selected)
	root := w.Post(t, w.Bob, w.Selected, "S26 PR 57 adds the two-stage image check.")
	child := w.PostReply(t, w.Bob, w.Selected, root, "S26 can you review it today?")
	if s, code := p.run("run --once"); code != 0 || !strings.Contains(s, "collected 2 new messages") {
		t.Fatalf("run --once: exit %d\n%s", code, s)
	}
	rootID, childID := p.storedID(root), p.storedID(child)
	if s, code := p.addCards(fmt.Sprintf(`{"category":"question","title":"Review PR 57 today",
	  "reply":"Thanks, I will review PR 57 this afternoon.","refs":[%d,%d],"key":"mm:pr-57"}`, rootID, childID)); code != 0 {
		t.Fatalf("card add: exit %d\n%s", code, s)
	}
	viewed := w.LastViewedAt(t, w.Alice, w.Selected)

	tm := p.term("")
	tm.WaitFor(`Review PR 57 today`, 30*time.Second)
	s := tm.Press("r", `Reply · card`, 5*time.Second)
	for _, want := range []string{"Send to    selected · mattermost · work", "Reply to", "Thread", "A thread reply keeps the read state.",
		"Thanks, I will review PR 57 this afternoon.", "ctrl+s send"} {
		if !strings.Contains(s, want) {
			t.Errorf("the reply form does not show %q:\n%s", want, s)
		}
	}
	tm.Type(" Can you confirm the old API has no change?")
	tm.WaitFor(`no change\?`, 5*time.Second)
	tm.Press("C-s", `Sent to selected \(thread\)\. The card is done\.`, 30*time.Second)
	tm.Press("q", `\[exit 0\]`, 5*time.Second)

	posts := w.PostsBy(t, w.Alice, w.Selected)
	want := "Thanks, I will review PR 57 this afternoon. Can you confirm the old API has no change?"
	if len(posts) != 1 || posts[0].Message != want || posts[0].RootID != root {
		t.Fatalf("posts of alice = %+v, want one post %q in the thread %s", posts, want, root)
	}
	if after := w.LastViewedAt(t, w.Alice, w.Selected); after != viewed {
		t.Errorf("last_viewed_at changed from %d to %d; the reply marked the channel read", viewed, after)
	}
	t.Logf("S26 post %s root %s, last_viewed_at %d before and after", posts[0].ID, posts[0].RootID, viewed)
	if c := byTitle(p.cards(), "Review PR 57 today"); c.State != inbox.CardDone {
		t.Errorf("card = %+v, want done", c)
	}
	if r := p.replies(childID, inbox.PlaceThread, want); r.State != inbox.ReplySent || r.ExternalID != posts[0].ID || r.By != inbox.ByUser {
		t.Errorf("reply row = %+v", r)
	}
}

// S27: the agent replies with "i2a reply". The same reply again is refused. A
// reply with --card uses the suggested reply and closes the card. A top-level
// post in the direct channel marks it read, as Mattermost does for any post of
// the user; i2a sends no other request.
func TestS27ReplyCommand(t *testing.T) {
	w := mm.NewWorld(t)
	p := newPerson(t, "S27")
	p.addAccount(w, w.Direct)
	dm := w.Post(t, w.Bob, w.Direct, "S27 what is the staging URL for the demo?")
	if s, code := p.run("run --once"); code != 0 || !strings.Contains(s, "collected 1 new messages") {
		t.Fatalf("run --once: exit %d\n%s", code, s)
	}
	id := p.storedID(dm)
	viewed := w.LastViewedAt(t, w.Alice, w.Direct)

	text := "It is https://staging.example.com/demo. The login is on the team page."
	in := p.stdinFile(text + "\n")
	s, code := p.run(fmt.Sprintf("reply --to %d < %s", id, in))
	if code != 0 || !regexp.MustCompile(`sent to @bob-\w+ \(channel\)\. Reply \d+\.`).MatchString(s) {
		t.Fatalf("reply: exit %d\n%s", code, s)
	}
	s, code = p.run(fmt.Sprintf("reply --to %d < %s", id, in))
	if code == 0 || !strings.Contains(strings.ToLower(s), "this reply was sent at") {
		t.Errorf("same reply again: exit %d\n%s", code, s)
	}
	if s, code := p.run("reply --to 99999 < " + in); code == 0 || !strings.Contains(strings.ToLower(s), "the store has no message 99999") {
		t.Errorf("unknown message: exit %d\n%s", code, s)
	}

	if s, code := p.addCards(fmt.Sprintf(`{"category":"action","title":"Send Bob the demo login",
	  "reply":"The demo login is demo-bob. I sent the password in a separate message.","refs":[%d],"key":"mm:s27"}`, id)); code != 0 {
		t.Fatalf("card add: exit %d\n%s", code, s)
	}
	s, code = p.run("reply --card mm:s27")
	if code != 0 || !strings.Contains(s, "The card is done.") {
		t.Fatalf("reply --card: exit %d\n%s", code, s)
	}
	posts := w.PostsBy(t, w.Alice, w.Direct)
	if len(posts) != 2 || posts[1].Message != text || posts[0].Message != "The demo login is demo-bob. I sent the password in a separate message." ||
		posts[0].RootID != "" {
		t.Fatalf("posts of alice = %+v, want the 2 replies in the chat", posts)
	}
	if after := w.LastViewedAt(t, w.Alice, w.Direct); after <= viewed {
		t.Errorf("last_viewed_at = %d, want after %d: Mattermost marks the chat read for a top-level post", after, viewed)
	}
	if c := byTitle(p.cards(), "Send Bob the demo login"); c.State != inbox.CardDone {
		t.Errorf("card = %+v, want done", c)
	}
	if r := p.replies(id, inbox.PlaceChannel, text); r.By != inbox.ByAgent || r.State != inbox.ReplySent {
		t.Errorf("reply row = %+v, want sent by the agent", r)
	}
}

// dropProxy forwards all requests to the server. It forwards the first
// POST /api/v4/posts too, but it closes the connection before the answer, as a
// network that drops the response does. The server creates the post.
func dropProxy(t *testing.T, target string) string {
	t.Helper()
	u, _ := url.Parse(target)
	rp := httputil.NewSingleHostReverseProxy(u)
	var dropped atomic.Bool
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/v4/posts" && !dropped.Swap(true) {
			body, _ := io.ReadAll(r.Body)
			req, _ := http.NewRequest(r.Method, target+r.URL.RequestURI(), bytes.NewReader(body))
			req.Header = r.Header.Clone()
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Errorf("proxy: %v", err)
			} else {
				resp.Body.Close()
			}
			conn, _, _ := w.(http.Hijacker).Hijack()
			conn.Close()
			return
		}
		rp.ServeHTTP(w, r)
	})}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { srv.Close() })
	return "http://" + ln.Addr().String()
}

// S28: the answer of the first send is lost, but the server created the post.
// A second send after 35 seconds finds the post and does not post it again.
func TestS28RetryAfterALostAnswer(t *testing.T) {
	w := mm.NewWorld(t)
	p := newPerson(t, "S28")
	pw := *w
	pw.MM = &harness.Mattermost{URL: dropProxy(t, w.MM.URL)}
	p.addAccount(&pw, w.Selected)
	post := w.Post(t, w.Bob, w.Selected, "S28 is the release at 18:30?")
	if s, code := p.run("run --once"); code != 0 || !strings.Contains(s, "collected 1 new messages") {
		t.Fatalf("run --once: exit %d\n%s", code, s)
	}
	id := p.storedID(post)
	in := p.stdinFile("Yes, the release is at 18:30.")

	s, code := p.run(fmt.Sprintf("reply --to %d < %s", id, in))
	if code == 0 || !strings.Contains(strings.ToLower(s), "the result is not known") {
		t.Fatalf("first send: exit %d, want an unknown result\n%s", code, s)
	}
	if posts := w.PostsBy(t, w.Alice, w.Selected); len(posts) != 1 {
		t.Fatalf("after the lost answer: %d posts, want 1 (the server created it)", len(posts))
	}
	// Mattermost returns the first post for a known pending_post_id for about
	// 30 seconds only (25 s: same post, 35 s: a new post). Wait longer, so the
	// retry depends on the check of i2a and not on that cache.
	time.Sleep(35 * time.Second)
	s, code = p.run(fmt.Sprintf("reply --to %d < %s", id, in))
	if code != 0 || !strings.Contains(s, "sent to selected (thread") {
		t.Fatalf("retry: exit %d\n%s", code, s)
	}
	posts := w.PostsBy(t, w.Alice, w.Selected)
	if len(posts) != 1 || posts[0].RootID != post {
		t.Fatalf("posts of alice after the retry = %+v, want 1 post in the thread", posts)
	}
	if r := p.replies(id, inbox.PlaceThread, "Yes, the release is at 18:30."); r.State != inbox.ReplySent || r.ExternalID != posts[0].ID {
		t.Errorf("reply row = %+v, want sent as post %s", r, posts[0].ID)
	}
}

// S29: the user allows the agent to send. bob posts a short status; the agent
// thanks bob with "i2a reply".
func TestS29AgentSendsAReply(t *testing.T) {
	orcaBin := needOrca(t)
	w := mm.NewWorld(t)
	p := newPerson(t, "S29")
	p.addAccount(w, w.Selected)
	ws := orcaWorkspace(t)
	allowCommands(t, ws, p.home, "card", "reply")
	p.editConfig(orcaAgent(orcaBin, ws, config.SessionReuse, config.PayloadInline))
	p.editConfig(func(c *config.Config) {
		c.Agents[0].Primer = "You are my inbox assistant. Read the messages and report each topic as a card.\n" +
			"When a message only reports news and needs no decision from me, thank the sender with one short sentence:\n" +
			"  I2A_HOME=" + p.home + " I2A_SECRET_BACKEND=file " + bin + " reply --to <id> <<'EOF'\n  <text>\n  EOF\n" +
			"The id is the \"id\" of the message. Do not send any other reply."
	})
	post := w.Post(t, w.Bob, w.Selected, "S29: I pushed the fix for the cancel API. No action is necessary.")
	s, code := p.run("run --once")
	if code != 0 || !strings.Contains(s, "started session term_") {
		t.Fatalf("batch: exit %d, screen:\n%s", code, s)
	}
	h := handleRe.FindString(s)
	t.Cleanup(func() { closeTerminal(orcaBin, h) })

	deadline := time.Now().Add(4 * time.Minute)
	for {
		posts := w.PostsBy(t, w.Alice, w.Selected)
		if len(posts) > 0 {
			readAgent(t, orcaBin, h, "S29", "S29/orca-terminal.txt")
			t.Logf("S29 agent reply: %+v", posts)
			if len(posts) != 1 || posts[0].RootID != post {
				t.Errorf("posts = %+v, want 1 post in the thread of %s", posts, post)
			}
			if r := p.replies(1, inbox.PlaceThread, posts[0].Message); r.By != inbox.ByAgent || r.State != inbox.ReplySent {
				t.Errorf("reply row = %+v", r)
			}
			return
		}
		if time.Now().After(deadline) {
			readAgent(t, orcaBin, h, "S29", "S29/orca-terminal.txt")
			t.Fatal("no reply from the agent after 4m")
		}
		time.Sleep(5 * time.Second)
	}
}
