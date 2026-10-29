package mattermost

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sondt2709/inbox2agent/internal/inbox"
)

// The reply_*.json fixtures are the responses of the send requests in
// testdata/capture.sh. They are not in the manifest, so each test serves them
// with an override.

func fixturePost(t *testing.T, name string) Post {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name+".json"))
	if err != nil {
		t.Fatal(err)
	}
	var p Post
	if err := json.Unmarshal(b, &p); err != nil {
		t.Fatal(err)
	}
	return p
}

func serveFile(t *testing.T, w http.ResponseWriter, status int, name string) {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name+".json"))
	if err != nil {
		t.Fatal(err)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(b)
}

// sent records the create requests of a test.
type sent struct {
	path string
	body map[string]any
}

func captureCreate(t *testing.T, rp *replay, reply string) *[]sent {
	t.Helper()
	var out []sent
	rp.override = func(w http.ResponseWriter, r *http.Request) bool {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v4/posts" {
			return false
		}
		b, _ := io.ReadAll(r.Body)
		var body map[string]any
		if err := json.Unmarshal(b, &body); err != nil {
			t.Error(err)
		}
		out = append(out, sent{path: r.URL.RequestURI(), body: body})
		serveFile(t, w, http.StatusCreated, reply)
		return true
	}
	return &out
}

func target(srv string, p Post, kind string) inbox.Message {
	return inbox.Message{ExternalID: p.ID, ChannelID: p.ChannelID, ChannelName: "@bob", ChannelKind: kind,
		ThreadID: p.RootID, URL: srv + "/fx/pl/" + p.ID}
}

func TestReplyToAReplyUsesTheThreadRoot(t *testing.T) {
	rp, srv := newReplay(t)
	reqs := captureCreate(t, rp, "reply_create_thread")
	src := newSource(t, srv, Settings{})
	me := fixturePost(t, "reply_create_thread").UserID
	replyPost, root := fixturePost(t, "post_reply"), fixturePost(t, "post_root")

	got, err := src.Reply(context.Background(), inbox.ReplyRequest{
		Target: target(srv.URL, replyPost, inbox.KindDirect), Text: "Thanks Bob, noted.", Thread: true, PendingID: "1700000000000",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(*reqs) != 1 {
		t.Fatalf("create requests = %d, want 1", len(*reqs))
	}
	r := (*reqs)[0]
	if r.path != "/api/v4/posts?set_online=false" {
		t.Errorf("path = %q, want set_online=false", r.path)
	}
	want := map[string]any{"channel_id": replyPost.ChannelID, "message": "Thanks Bob, noted.", "root_id": root.ID,
		"pending_post_id": me + ":1700000000000"}
	for k, v := range want {
		if r.body[k] != v {
			t.Errorf("body %s = %v, want %v", k, r.body[k], v)
		}
	}
	created := fixturePost(t, "reply_create_thread")
	if got.ID != created.ID || got.URL != srv.URL+"/fx/pl/"+created.ID {
		t.Errorf("posted = %+v, want %s", got, created.ID)
	}
	if rp.count("GET /posts/"+replyPost.ID) != 1 {
		t.Error("the target post was not read before the send")
	}
}

func TestReplyInTheChannel(t *testing.T) {
	rp, srv := newReplay(t)
	reqs := captureCreate(t, rp, "reply_create_channel")
	src := newSource(t, srv, Settings{})
	root := fixturePost(t, "post_root")

	if _, err := src.Reply(context.Background(), inbox.ReplyRequest{
		Target: target(srv.URL, root, inbox.KindDirect), Text: "Noted, thanks.", PendingID: "1",
	}); err != nil {
		t.Fatal(err)
	}
	r := (*reqs)[0]
	if r.path != "/api/v4/posts?set_online=false" {
		t.Errorf("path = %q, want set_online=false", r.path)
	}
	if _, ok := r.body["root_id"]; ok {
		t.Errorf("body has root_id %v, want none for a reply in the channel", r.body["root_id"])
	}
}

func TestReplyToADeletedPostSendsNothing(t *testing.T) {
	rp, srv := newReplay(t)
	reqs := captureCreate(t, rp, "reply_create_thread")
	create := rp.override
	rp.override = func(w http.ResponseWriter, r *http.Request) bool {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v4/posts/deletedpost" {
			serveFile(t, w, http.StatusNotFound, "reply_get_deleted")
			return true
		}
		return create(w, r)
	}
	src := newSource(t, srv, Settings{})
	_, err := src.Reply(context.Background(), inbox.ReplyRequest{
		Target: inbox.Message{ExternalID: "deletedpost", URL: srv.URL + "/fx/pl/deletedpost"}, Text: "hi", Thread: true, PendingID: "1",
	})
	var answered interface{ Answered() bool }
	if !errors.As(err, &answered) || !answered.Answered() {
		t.Fatalf("err = %v, want an answered API error", err)
	}
	if !strings.Contains(err.Error(), "404") {
		t.Errorf("err = %v, want HTTP 404", err)
	}
	if len(*reqs) != 0 {
		t.Errorf("create requests = %d, want 0", len(*reqs))
	}
}

func TestFindTheReplyAfterATimeOut(t *testing.T) {
	rp, srv := newReplay(t)
	replyPost := fixturePost(t, "post_reply")
	created := fixturePost(t, "reply_create_thread")
	rp.override = func(w http.ResponseWriter, r *http.Request) bool {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v4/channels/"+replyPost.ChannelID+"/posts" {
			serveFile(t, w, http.StatusOK, "reply_posts_direct_after")
			return true
		}
		return false
	}
	src := newSource(t, srv, Settings{})
	base := inbox.ReplyRequest{Target: target(srv.URL, replyPost, inbox.KindDirect), Text: "Thanks Bob, noted.", Thread: true,
		Since: time.UnixMilli(created.CreateAt - 5000)}

	got, ok, err := src.Find(context.Background(), base)
	if err != nil || !ok || got.ID != created.ID {
		t.Fatalf("find = %+v %v %v, want post %s", got, ok, err, created.ID)
	}
	for name, r := range map[string]inbox.ReplyRequest{
		"other text":       {Target: base.Target, Text: "Thanks Bob", Thread: true, Since: base.Since},
		"in the channel":   {Target: base.Target, Text: base.Text, Thread: false, Since: base.Since},
		"before first try": {Target: base.Target, Text: base.Text, Thread: true, Since: time.UnixMilli(created.CreateAt + 1)},
	} {
		if _, ok, err := src.Find(context.Background(), r); err != nil || ok {
			t.Errorf("%s: found = %v, %v, want not found", name, ok, err)
		}
	}
	if rp.count("POST /posts") != 0 {
		t.Error("Find sent a post")
	}
}
