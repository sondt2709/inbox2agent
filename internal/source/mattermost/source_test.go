package mattermost

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/sondt2709/inbox2agent/internal/inbox"
)

// replay serves the responses that testdata/capture.sh saved from a real server.
// A request that is not in the manifest fails the test. This also proves that
// the source sends no request that changes the read state.
type replay struct {
	t        *testing.T
	files    map[string]string
	mu       sync.Mutex
	requests []string
	override func(w http.ResponseWriter, r *http.Request) bool
}

func newReplay(t *testing.T) (*replay, *httptest.Server) {
	t.Helper()
	b, err := os.ReadFile("testdata/manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	rp := &replay{t: t, files: map[string]string{}}
	if err := json.Unmarshal(b, &rp.files); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.Method + " " + strings.TrimPrefix(r.URL.RequestURI(), "/api/v4")
		rp.mu.Lock()
		rp.requests = append(rp.requests, key)
		override := rp.override
		rp.mu.Unlock()
		if override != nil && override(w, r) {
			return
		}
		if r.Header.Get("Authorization") != "Bearer test-token" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"id":"api.context.session_expired.app_error","message":"Invalid or expired session, please login again.","status_code":401}`))
			return
		}
		name, ok := rp.files[key]
		if !ok {
			t.Errorf("unexpected request %q", key)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		body, err := os.ReadFile(filepath.Join("testdata", name))
		if err != nil {
			t.Error(err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)
	return rp, srv
}

func (rp *replay) count(prefix string) int {
	rp.mu.Lock()
	defer rp.mu.Unlock()
	n := 0
	for _, r := range rp.requests {
		if strings.HasPrefix(r, prefix) {
			n++
		}
	}
	return n
}

// fixtureNow is one second after the newest post in the fixtures.
func fixtureNow(t *testing.T) time.Time {
	t.Helper()
	var newest int64
	for _, n := range []string{"selected", "other", "direct"} {
		b, err := os.ReadFile("testdata/posts_" + n + "_200_p0.json")
		if err != nil {
			t.Fatal(err)
		}
		var pl PostList
		if err := json.Unmarshal(b, &pl); err != nil {
			t.Fatal(err)
		}
		for _, p := range pl.Posts {
			newest = max(newest, p.CreateAt)
		}
	}
	return time.UnixMilli(newest + 1000)
}

func channelID(t *testing.T, name string) string {
	t.Helper()
	b, _ := os.ReadFile("testdata/channels.json")
	var chs []Channel
	if err := json.Unmarshal(b, &chs); err != nil {
		t.Fatal(err)
	}
	for _, c := range chs {
		if c.Name == name {
			return c.ID
		}
	}
	t.Fatalf("no channel %q in fixtures", name)
	return ""
}

func newSource(t *testing.T, srv *httptest.Server, s Settings) *Source {
	t.Helper()
	s.ID = "work"
	s.URL = srv.URL
	if s.Lookback == 0 {
		s.Lookback = time.Hour
	}
	c := NewClient(srv.URL, "test-token", srv.Client())
	c.sleep = func(context.Context, time.Duration) error { return nil }
	return New(s, c)
}

func texts(msgs []inbox.Message) []string {
	var out []string
	for _, m := range msgs {
		out = append(out, m.Text)
	}
	return out
}

func TestCollectSelectedDirectAndMentions(t *testing.T) {
	for _, perPage := range []int{200, 2} {
		rp, srv := newReplay(t)
		src := newSource(t, srv, Settings{
			Channels: []string{channelID(t, "selected")}, AllDirect: true, Mentions: true, PerPage: perPage,
		})
		msgs, cur, err := src.Collect(context.Background(), inbox.Cursor{}, fixtureNow(t))
		if err != nil {
			t.Fatal(err)
		}
		want := []string{"selected one", "selected two", "selected three", "selected four",
			"@alice please check the report.", "direct hi alice", "reply in thread"}
		got := texts(msgs)
		if strings.Join(got, "|") != strings.Join(want, "|") {
			t.Fatalf("per_page=%d: messages =\n  %q\nwant\n  %q", perPage, got, want)
		}
		for _, m := range msgs {
			if m.Author != "bob" {
				t.Errorf("author = %q, want bob", m.Author)
			}
			if !strings.HasPrefix(m.URL, srv.URL+"/fx/pl/"+m.ExternalID) {
				t.Errorf("url = %q", m.URL)
			}
		}
		if msgs[5].ChannelKind != inbox.KindDirect || msgs[5].ChannelName != "@bob" {
			t.Errorf("direct message = %+v", msgs[5])
		}
		if msgs[4].ChannelName != "other" || msgs[4].ChannelKind != inbox.KindChannel {
			t.Errorf("mention message = %+v", msgs[4])
		}
		if len(cur) != 5 { // mentions on: all 5 member channels have new posts
			t.Errorf("cursor has %d channels, want 5: %v", len(cur), cur)
		}
		if rp.count("POST /users/ids") == 0 {
			t.Error("authors were not resolved")
		}
	}
}

func TestSecondPollSkipsQuietChannels(t *testing.T) {
	rp, srv := newReplay(t)
	src := newSource(t, srv, Settings{Channels: []string{channelID(t, "selected")}, AllDirect: true, Mentions: true})
	now := fixtureNow(t)
	_, cur, err := src.Collect(context.Background(), inbox.Cursor{}, now)
	if err != nil {
		t.Fatal(err)
	}
	before := rp.count("GET /channels/")
	msgs, _, err := src.Collect(context.Background(), cur, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 0 {
		t.Errorf("second poll returned %q, want nothing", texts(msgs))
	}
	if after := rp.count("GET /channels/"); after != before {
		t.Errorf("second poll read posts of %d channels, want 0", after-before)
	}
}

func TestCollectWithoutDirectAndMentions(t *testing.T) {
	_, srv := newReplay(t)
	src := newSource(t, srv, Settings{Channels: []string{channelID(t, "selected")}})
	msgs, _, err := src.Collect(context.Background(), inbox.Cursor{}, fixtureNow(t))
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(texts(msgs), "|"); got != "selected one|selected two|selected three|selected four" {
		t.Fatalf("messages = %q", got)
	}
}

func TestLookbackLimitsFirstPoll(t *testing.T) {
	_, srv := newReplay(t)
	src := newSource(t, srv, Settings{Channels: []string{channelID(t, "selected")}, Lookback: time.Hour})
	msgs, _, err := src.Collect(context.Background(), inbox.Cursor{}, fixtureNow(t).Add(2*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 0 {
		t.Fatalf("messages older than the lookback = %q", texts(msgs))
	}
}

func TestMentionMatcher(t *testing.T) {
	m := mentionMatcher("alice")
	yes := []string{"@alice hi", "hi @alice", "hi @alice.", "(@alice)", "@ALICE look", "@alice, ok", "line\n@alice"}
	no := []string{"x@alice.example", "@alicex", "@alice-bob", "@alice.smith", "alice", "@@alice"}
	for _, s := range yes {
		if !m.MatchString(s) {
			t.Errorf("%q: no match, want a match", s)
		}
	}
	for _, s := range no {
		if m.MatchString(s) {
			t.Errorf("%q: match, want no match", s)
		}
	}
}

func TestUnauthorized(t *testing.T) {
	_, srv := newReplay(t)
	c := NewClient(srv.URL, "wrong", srv.Client())
	src := New(Settings{ID: "work", URL: srv.URL, Lookback: time.Hour}, c)
	_, _, err := src.Collect(context.Background(), inbox.Cursor{}, fixtureNow(t))
	if !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("err = %v, want ErrUnauthorized", err)
	}
	if !strings.Contains(err.Error(), "Invalid or expired session") {
		t.Errorf("err = %q, want the server message", err)
	}
}

func TestRetryOn429(t *testing.T) {
	rp, srv := newReplay(t)
	var mu sync.Mutex
	left := 2
	rp.override = func(w http.ResponseWriter, r *http.Request) bool {
		mu.Lock()
		defer mu.Unlock()
		if r.URL.Path == "/api/v4/users/me" && left > 0 {
			left--
			w.WriteHeader(http.StatusTooManyRequests)
			return true
		}
		return false
	}
	src := newSource(t, srv, Settings{})
	var waits []time.Duration
	src.client.sleep = func(_ context.Context, d time.Duration) error { waits = append(waits, d); return nil }
	name, err := src.Check(context.Background())
	if err != nil || name != "alice" {
		t.Fatalf("Check = %q, %v", name, err)
	}
	if len(waits) != 2 || waits[0] != time.Second || waits[1] != 2*time.Second {
		t.Errorf("waits = %v, want [1s 2s]", waits)
	}
}

func TestListChannels(t *testing.T) {
	_, srv := newReplay(t)
	src := newSource(t, srv, Settings{Channels: []string{channelID(t, "other")}})
	chs, err := src.ListChannels(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, c := range chs {
		s := c.Kind + ":" + c.Name
		if c.Selected {
			s += "*"
		}
		got = append(got, s)
	}
	want := "channel:off-topic|channel:other*|channel:selected|channel:town-square|direct:@bob"
	if strings.Join(got, "|") != want {
		t.Fatalf("channels = %q\nwant %q", strings.Join(got, "|"), want)
	}
}

func directID(t *testing.T) string {
	t.Helper()
	b, _ := os.ReadFile("testdata/channels.json")
	var chs []Channel
	if err := json.Unmarshal(b, &chs); err != nil {
		t.Fatal(err)
	}
	for _, c := range chs {
		if c.Type == "D" {
			return c.ID
		}
	}
	t.Fatal("no direct channel in fixtures")
	return ""
}

// A user can watch one direct conversation without all direct messages.
func TestCollectOneSelectedDirectChannel(t *testing.T) {
	_, srv := newReplay(t)
	src := newSource(t, srv, Settings{Channels: []string{directID(t)}})
	msgs, _, err := src.Collect(context.Background(), inbox.Cursor{}, fixtureNow(t))
	if err != nil {
		t.Fatal(err)
	}
	// capture.sh posts "reply in thread" in the direct channel too.
	if got := texts(msgs); strings.Join(got, "|") != "direct hi alice|reply in thread" {
		t.Fatalf("messages = %q, want only the 2 posts of the direct channel", got)
	}
	if msgs[0].ChannelKind != inbox.KindDirect || msgs[0].ChannelName != "@bob" {
		t.Errorf("message = %+v", msgs[0])
	}
}

func TestSortChannelsConversationsByRecentPost(t *testing.T) {
	at := func(h int) time.Time { return time.Date(2026, 1, 1, h, 0, 0, 0, time.UTC) }
	chs := []ChannelInfo{
		{Name: "@old", Kind: inbox.KindDirect, LastPost: at(1)},
		{Name: "zeta", Team: "b", Kind: inbox.KindChannel},
		{Name: "carol, dave", Kind: inbox.KindGroup, LastPost: at(5)},
		{Name: "alpha", Team: "b", Kind: inbox.KindChannel},
		{Name: "@new", Kind: inbox.KindDirect, LastPost: at(9)},
		{Name: "@never", Kind: inbox.KindDirect},
		{Name: "general", Team: "a", Kind: inbox.KindChannel},
	}
	SortChannels(chs)
	var got []string
	for _, c := range chs {
		got = append(got, c.Name)
	}
	want := "general|alpha|zeta|@new|carol, dave|@old|@never"
	if strings.Join(got, "|") != want {
		t.Errorf("order = %q\nwant  %q", strings.Join(got, "|"), want)
	}
}

func TestGroupLabelWithoutTheUser(t *testing.T) {
	cases := map[string]string{
		"alice, bob, carol": "bob, carol",
		"bob, alice":        "bob",
		"bob, carol":        "bob, carol",
		"alice":             "alice",
	}
	for in, want := range cases {
		if got := groupLabel(in, "alice"); got != want {
			t.Errorf("groupLabel(%q) = %q, want %q", in, got, want)
		}
	}
}
