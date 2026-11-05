package mattermost

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/sondt2709/inbox2agent/internal/config"
	"github.com/sondt2709/inbox2agent/internal/inbox"
)

// postID returns the ID of the post with the text in a fixture file.
func postID(t *testing.T, file, text string) string {
	t.Helper()
	b, err := os.ReadFile("testdata/" + file)
	if err != nil {
		t.Fatal(err)
	}
	var pl PostList
	if err := json.Unmarshal(b, &pl); err != nil {
		t.Fatal(err)
	}
	for id, p := range pl.Posts {
		if p.Message == text {
			return id
		}
	}
	t.Fatalf("no post %q in %s", text, file)
	return ""
}

// withoutPageRoots removes from each page of the channel threads the posts
// that are not in its order, so that the source must read the roots with GET.
func withoutPageRoots(t *testing.T, rp *replay) func(w http.ResponseWriter, r *http.Request) bool {
	prefix := "/api/v4/channels/" + channelID(t, "threads") + "/posts?"
	return func(w http.ResponseWriter, r *http.Request) bool {
		if !strings.HasPrefix(r.URL.RequestURI(), prefix) {
			return false
		}
		key := r.Method + " " + strings.TrimPrefix(r.URL.RequestURI(), "/api/v4")
		b, err := os.ReadFile("testdata/" + rp.files[key])
		if err != nil {
			t.Error(err)
			return false
		}
		var pl PostList
		if err := json.Unmarshal(b, &pl); err != nil {
			t.Error(err)
			return false
		}
		kept := map[string]Post{}
		for _, id := range pl.Order {
			kept[id] = pl.Posts[id]
		}
		pl.Posts = kept
		_ = json.NewEncoder(w).Encode(pl)
		return true
	}
}

// afterRoots is a cursor of the channel threads after the two roots, so that
// the source does not read the pages of the roots.
func afterRoots(t *testing.T) inbox.Cursor {
	t.Helper()
	b, err := os.ReadFile("testdata/posts_threads_200_p0.json")
	if err != nil {
		t.Fatal(err)
	}
	var pl PostList
	if err := json.Unmarshal(b, &pl); err != nil {
		t.Fatal(err)
	}
	p := pl.Posts[postID(t, "posts_threads_200_p0.json", "threads noise one")]
	return inbox.Cursor{channelID(t, "threads"): strconv.FormatInt(p.CreateAt, 10)}
}

func threadsSource(t *testing.T, level string) (*replay, *Source) {
	t.Helper()
	rp, srv := newReplay(t)
	src := newSource(t, srv, Settings{Levels: map[string]string{channelID(t, "threads"): level}, Default: config.ReadOff, PerPage: 2})
	return rp, src
}

func byText(msgs []inbox.Message) map[string]inbox.Message {
	out := map[string]inbox.Message{}
	for _, m := range msgs {
		out[m.Text] = m
	}
	return out
}

func TestThreadRootFromThePage(t *testing.T) {
	rp, src := threadsSource(t, config.ReadAll)
	msgs, _, err := src.Collect(context.Background(), inbox.Cursor{}, fixtureNow(t))
	if err != nil {
		t.Fatal(err)
	}
	by := byText(msgs)
	alice := postID(t, "posts_threads_200_p0.json", "question from alice")
	bob := postID(t, "posts_threads_200_p0.json", "root by bob")
	if r := by["answer to alice"].ReplyTo; r == nil || *r != (inbox.ReplyRef{Ext: alice, Me: true, Text: "question from alice"}) {
		t.Errorf("reply under the root of the user = %+v", r)
	}
	if !by["answer to alice"].ForMe || by["more on the root of bob"].ForMe {
		t.Errorf("ForMe = %v and %v, want true for the reply under the root of the user only",
			by["answer to alice"].ForMe, by["more on the root of bob"].ForMe)
	}
	if r := by["more on the root of bob"].ReplyTo; r == nil || *r != (inbox.ReplyRef{Ext: bob, Author: "bob", Text: "root by bob"}) {
		t.Errorf("reply under the root of bob = %+v", r)
	}
	if r := by["threads noise one"].ReplyTo; r != nil {
		t.Errorf("top-level post has the reply reference %+v", r)
	}
	if n := rp.count("GET /posts/"); n != 0 {
		t.Errorf("GET /posts requests = %d, want 0", n)
	}
}

func TestThreadRootWithGet(t *testing.T) {
	rp, src := threadsSource(t, config.ReadMentions)
	rp.override = withoutPageRoots(t, rp)
	msgs, _, err := src.Collect(context.Background(), afterRoots(t), fixtureNow(t))
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(texts(msgs), "|"); got != "answer to alice" {
		t.Fatalf("messages = %q, want only the reply under the root of the user", got)
	}
	alice := postID(t, "posts_threads_200_p0.json", "question from alice")
	if r := msgs[0].ReplyTo; r == nil || *r != (inbox.ReplyRef{Ext: alice, Me: true, Text: "question from alice"}) || !msgs[0].ForMe {
		t.Errorf("reply reference = %+v, ForMe %v", r, msgs[0].ForMe)
	}
	if n := rp.count("GET /posts/"); n != 2 {
		t.Errorf("GET /posts requests = %d, want 2 (one for each root)", n)
	}
	// The second poll from the same cursor uses the cache.
	if _, _, err := src.Collect(context.Background(), afterRoots(t), fixtureNow(t)); err != nil {
		t.Fatal(err)
	}
	if n := rp.count("GET /posts/"); n != 2 {
		t.Errorf("GET /posts requests after the second poll = %d, want 2", n)
	}
}

func TestThreadRootNotFound(t *testing.T) {
	rp, src := threadsSource(t, config.ReadAll)
	pages := withoutPageRoots(t, rp)
	notFound, err := os.ReadFile("testdata/reply_get_deleted.json") // a real HTTP 404 response
	if err != nil {
		t.Fatal(err)
	}
	rp.override = func(w http.ResponseWriter, r *http.Request) bool {
		if strings.HasPrefix(r.URL.Path, "/api/v4/posts/") {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write(notFound)
			return true
		}
		return pages(w, r)
	}
	msgs, _, err := src.Collect(context.Background(), afterRoots(t), fixtureNow(t))
	if err != nil {
		t.Fatal(err)
	}
	by := byText(msgs)
	alice := postID(t, "posts_threads_200_p0.json", "question from alice")
	if r := by["answer to alice"].ReplyTo; r == nil || *r != (inbox.ReplyRef{Ext: alice}) || by["answer to alice"].ForMe {
		t.Errorf("reply under an unknown root = %+v, ForMe %v; want the root ID only", r, by["answer to alice"].ForMe)
	}
}

func TestThreadRootErrorKeepsTheCursor(t *testing.T) {
	rp, src := threadsSource(t, config.ReadAll)
	pages := withoutPageRoots(t, rp)
	rp.override = func(w http.ResponseWriter, r *http.Request) bool {
		if strings.HasPrefix(r.URL.Path, "/api/v4/posts/") {
			w.WriteHeader(http.StatusInternalServerError)
			return true
		}
		return pages(w, r)
	}
	msgs, cur, err := src.Collect(context.Background(), afterRoots(t), fixtureNow(t))
	if err == nil || !strings.Contains(err.Error(), "threads") {
		t.Fatalf("err = %v, want an error of the channel threads", err)
	}
	if msgs != nil || cur != nil {
		t.Errorf("messages = %v, cursor = %v; want none", msgs, cur)
	}
}
