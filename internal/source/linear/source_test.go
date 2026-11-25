package linear

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/sondt2709/inbox2agent/internal/config"
	"github.com/sondt2709/inbox2agent/internal/inbox"
)

// The fixtures are real responses of the Linear API with the personal data
// replaced (testdata/capture). They were captured at about this time.
var captured = time.Date(2026, 11, 24, 19, 0, 0, 0, time.UTC)

type fixture struct {
	Status int             `json:"status"`
	Body   json.RawMessage `json:"body"`
}

func readFixture(t *testing.T, name string) fixture {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	var f fixture
	if err := json.Unmarshal(b, &f); err != nil {
		t.Fatal(err)
	}
	return f
}

// fake is a GraphQL endpoint that answers each request with the next fixture
// and records the request bodies.
type fake struct {
	mu   sync.Mutex
	srv  *httptest.Server
	seq  []fixture
	reqs []request
	keys []string
}

type request struct {
	Query     string         `json:"query"`
	Variables map[string]any `json:"variables"`
}

func newFake(t *testing.T, names ...string) *fake {
	t.Helper()
	f := &fake{}
	for _, n := range names {
		f.seq = append(f.seq, readFixture(t, n))
	}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		var req request
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("request body: %v", err)
		}
		f.reqs = append(f.reqs, req)
		f.keys = append(f.keys, r.Header.Get("Authorization"))
		if len(f.seq) == 0 {
			t.Errorf("request %d has no fixture", len(f.reqs))
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		fx := f.seq[0]
		f.seq = f.seq[1:]
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(fx.Status)
		_, _ = w.Write(fx.Body)
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func newSource(f *fake, s Settings) *Source {
	if s.ID == "" {
		s.ID = "lin"
	}
	if s.Lookback == 0 {
		s.Lookback = 7 * 24 * time.Hour
	}
	if s.Default == "" {
		s.Default = config.ReadMentions
	}
	c := NewClient(f.srv.URL, "lin_api_test", nil)
	c.sleep = func(context.Context, time.Duration) error { return nil }
	return New(s, c)
}

func defaultFeeds() []config.Feed { return config.NewLinearAccount("lin").Feeds }

func byID(msgs []inbox.Message) map[string]inbox.Message {
	out := map[string]inbox.Message{}
	for _, m := range msgs {
		out[m.ExternalID] = m
	}
	return out
}

func TestCollectDefaultFeeds(t *testing.T) {
	f := newFake(t, "poll.json", "poll2.json")
	src := newSource(f, Settings{Feeds: defaultFeeds(), MaxPages: 2})
	msgs, next, err := src.Collect(context.Background(), nil, captured)
	if err != nil {
		t.Fatal(err)
	}

	// One request for the two feeds, then one for the next page of the inbox only.
	if len(f.reqs) != 2 {
		t.Fatalf("requests = %d, want 2", len(f.reqs))
	}
	if q := f.reqs[0].Query; !strings.Contains(q, "f0: comments(") || !strings.Contains(q, "f1: notifications(") {
		t.Errorf("first request has not the two feeds:\n%s", q)
	}
	if q := f.reqs[1].Query; strings.Contains(q, "comments(") || !strings.Contains(q, "f0: notifications(") || f.reqs[1].Variables["a0"] == nil {
		t.Errorf("second request is not the next page of the inbox:\n%s\n%v", q, f.reqs[1].Variables)
	}
	if f.keys[0] != "lin_api_test" {
		t.Errorf("Authorization = %q, want the key with no scheme", f.keys[0])
	}

	var agent, notes []inbox.Message
	for _, m := range msgs {
		switch m.ChannelID {
		case "agent":
			agent = append(agent, m)
		case "inbox":
			notes = append(notes, m)
		}
	}
	// Six comments contain /agent; "please ask /agent later" does not start with it.
	var bodies []string
	for _, m := range agent {
		_, body, _ := strings.Cut(m.Text, "\n\n")
		bodies = append(bodies, body)
	}
	want := []string{
		"/agent summarize this issue",
		"@me.user /agent please check this",
		"https://linear.app/acme/profiles/me.user /agent please check the mention link",
		"/agent check this note",
		"/agent and also list the risks",
	}
	if !slices.Equal(bodies, want) {
		t.Errorf("agent bodies =\n%q\nwant\n%q", bodies, want)
	}
	// The level mentions keeps the inbox items for the user: 12 on page 1 and 23 on page 2.
	if len(notes) != 35 {
		t.Errorf("inbox messages = %d, want 35", len(notes))
	}
	for _, m := range msgs {
		if !m.ForMe {
			t.Errorf("message %s is not for the user", m.ExternalID)
		}
	}
	if next["agent"] != "2026-11-24T18:57:02.285Z" {
		t.Errorf("agent cursor = %q, want the newest updatedAt", next["agent"])
	}
	if next["inbox"] != "2026-11-24T17:07:14.643Z" {
		t.Errorf("inbox cursor = %q, want the newest createdAt", next["inbox"])
	}
}

func TestCommentMessage(t *testing.T) {
	f := newFake(t, "poll.json")
	feeds := defaultFeeds()[:1]
	src := newSource(f, Settings{Feeds: feeds})
	msgs, _, err := src.Collect(context.Background(), nil, captured)
	if err != nil {
		t.Fatal(err)
	}
	m, ok := byID(msgs)["00000000-0000-4000-8000-000000000003"]
	if !ok {
		t.Fatalf("no message for the edited comment in %d messages", len(msgs))
	}
	wantText := "ENG-101 i2a test: Linear source fixtures\n\n/agent check this note"
	if m.SourceType != "linear" || m.ChannelName != "Agent requests" || m.ChannelKind != inbox.KindChannel || m.ThreadID != "ENG-101" ||
		m.Author != "me.user" || m.Text != wantText || m.URL != "https://linear.app/acme/issue/ENG-101/issue#comment-00000001" ||
		!m.CreatedAt.Equal(time.Date(2026, 11, 24, 18, 56, 59, 196e6, time.UTC)) {
		t.Errorf("message = %+v", m)
	}
	r := byID(msgs)["00000000-0000-4000-8000-000000000008"]
	if r.ReplyTo == nil || !r.ReplyTo.Me || r.ReplyTo.Author != "" || r.ReplyTo.Ext != "00000000-0000-4000-8000-000000000005" || r.ReplyTo.Text != "/agent summarize this issue" {
		t.Errorf("reply reference = %+v", r.ReplyTo)
	}
}

func TestReplyToAnotherPerson(t *testing.T) {
	f := newFake(t, "mine.json")
	src := newSource(f, Settings{Feeds: []config.Feed{{ID: "mine", Kind: config.FeedComments}}})
	msgs, _, err := src.Collect(context.Background(), nil, captured)
	if err != nil {
		t.Fatal(err)
	}
	m := byID(msgs)["00000000-0000-4000-8000-000000000088"]
	if m.ReplyTo == nil || m.ReplyTo.Me || m.ReplyTo.Author != "user30" || m.ReplyTo.Text != "Text of comment 23." {
		t.Errorf("reply reference = %+v", m.ReplyTo)
	}
	if len(msgs) != 34 {
		t.Errorf("messages = %d, want each of the 34 comments of the user", len(msgs))
	}
}

func TestPrefixAfterMarkdown(t *testing.T) {
	f := newFake(t, "mine.json")
	src := newSource(f, Settings{Feeds: []config.Feed{{ID: "gt", Kind: config.FeedComments, Prefix: ">agent"}}})
	msgs, _, err := src.Collect(context.Background(), nil, captured)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 1 || msgs[0].ExternalID != "00000000-0000-4000-8000-000000000013" {
		t.Errorf("messages = %+v, want the >agent comment only", msgs)
	}
	if v := f.reqs[0].Variables["p0"]; v != ">agent" {
		t.Errorf("body filter = %v, want >agent", v)
	}
}

func TestInboxMessages(t *testing.T) {
	f := newFake(t, "poll.json")
	feeds := defaultFeeds()
	src := newSource(f, Settings{Feeds: feeds, MaxPages: 1, Levels: map[string]string{"inbox": config.ReadAll}})
	msgs, _, err := src.Collect(context.Background(), nil, captured)
	if err != nil {
		t.Fatal(err)
	}
	all := byID(msgs)
	n := 0
	for _, m := range msgs {
		if m.ChannelID == "inbox" {
			n++
		}
	}
	if n != 50 {
		t.Errorf("inbox messages with the level all = %d, want 50", n)
	}
	a := all["00000000-0000-4000-8000-000000000234"]
	if !a.ForMe || a.Author != "user55" || a.ThreadID != "ENG-170" || a.URL != "https://linear.app/acme/issue/ENG-170/issue" ||
		!strings.HasSuffix(a.Text, "\n\nissue assigned to you") {
		t.Errorf("assignment = %+v", a)
	}
	mn := all["00000000-0000-4000-8000-000000000268"]
	if !mn.ForMe || mn.URL != "https://linear.app/acme/issue/ENG-147/issue#comment-0000003d" {
		t.Errorf("comment mention = %+v", mn)
	}
	pr := all["00000000-0000-4000-8000-000000000171"]
	if !pr.ForMe || pr.ThreadID != "" || pr.URL != "" || pr.Text != "pull request review requested" {
		t.Errorf("review request = %+v", pr)
	}
	if c := all["00000000-0000-4000-8000-000000000003"]; c.ExternalID == "" {
		t.Error("the agent feed has no message")
	}
	for _, m := range msgs {
		if m.ChannelID == "inbox" && strings.Contains(m.Text, "issue new comment") {
			t.Errorf("a new comment notification has no comment body: %+v", m)
		}
	}
}

func TestFeedFilters(t *testing.T) {
	cases := []struct {
		name    string
		feed    config.Feed
		fixture string
		want    int
	}{
		{"project", config.Feed{ID: "x", Kind: config.FeedComments, Projects: []string{"project 20"}}, "mine.json", 6},
		{"other project", config.Feed{ID: "x", Kind: config.FeedComments, Projects: []string{"Bridge"}}, "mine.json", 0},
		{"team", config.Feed{ID: "x", Kind: config.FeedComments, Teams: []string{"eng"}}, "mine.json", 34},
		{"other team", config.Feed{ID: "x", Kind: config.FeedComments, Teams: []string{"OPS"}}, "mine.json", 0},
		{"label without case", config.Feed{ID: "x", Kind: config.FeedInbox, Labels: []string{"LABEL 1"}}, "poll.json", 22},
		{"other person", config.Feed{ID: "x", Kind: config.FeedComments, From: []string{"user30"}}, "mine.json", 0},
		{"actor", config.Feed{ID: "x", Kind: config.FeedInbox, From: []string{"USER55@example.com"}}, "poll.json", 14},
		{"inbox prefix", config.Feed{ID: "x", Kind: config.FeedInbox, Prefix: "/agent"}, "poll.json", 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var body json.RawMessage
			fx := readFixture(t, tc.fixture)
			var r struct {
				Data map[string]Page `json:"data"`
			}
			if err := json.Unmarshal(fx.Body, &r); err != nil {
				t.Fatal(err)
			}
			body = r.Data["f0"].Nodes
			if tc.feed.Kind == config.FeedInbox {
				body = r.Data["f1"].Nodes
			}
			src := New(Settings{ID: "lin"}, nil)
			got, err := src.messages(&polled{feed: tc.feed, all: true}, body)
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != tc.want {
				t.Errorf("matches = %d, want %d", len(got), tc.want)
			}
		})
	}
}

func TestHasPrefix(t *testing.T) {
	cases := []struct {
		body, prefix string
		want         bool
	}{
		{"/agent summarize this issue", "/agent", true},
		{"@me.user /agent please check this", "/agent", true},
		{"@me.user, @user30: /agent check", "/agent", true},
		{"  \n/agent check", "/agent", true},
		{"https://linear.app/acme/profiles/me.user /agent please check", "/agent", true},
		{`\>agent check`, ">agent", true},
		{"please ask /agent later", "/agent", false},
		{"/Agent check", "/agent", false},
		{"me.user /agent", "/agent", false},
	}
	for _, tc := range cases {
		if got := hasPrefix(tc.body, tc.prefix); got != tc.want {
			t.Errorf("hasPrefix(%q, %q) = %v, want %v", tc.body, tc.prefix, got, tc.want)
		}
	}
}

func TestCursorAndLevels(t *testing.T) {
	f := newFake(t, "poll.json")
	src := newSource(f, Settings{Feeds: defaultFeeds(), MaxPages: 1, Levels: map[string]string{"inbox": config.ReadOff}})
	cur := inbox.Cursor{"agent": "2026-11-24T18:00:00Z", "inbox": "2026-11-24T18:00:00Z"}
	if _, _, err := src.Collect(context.Background(), cur, captured); err != nil {
		t.Fatal(err)
	}
	q, vars := f.reqs[0].Query, f.reqs[0].Variables
	if strings.Contains(q, "notifications(") {
		t.Errorf("the feed with the level off is in the request:\n%s", q)
	}
	// An edit changes updatedAt, so a comment that gets the prefix later is found.
	if !strings.Contains(q, "updatedAt: { gt: $t0 }") || vars["t0"] != "2026-11-24T17:58:00Z" {
		t.Errorf("comments filter: %s %v, want updatedAt after the cursor minus 2 minutes", q, vars["t0"])
	}

	// A read changes updatedAt of a notification, so the inbox reads createdAt.
	f2 := newFake(t, "poll.json")
	src = newSource(f2, Settings{Feeds: defaultFeeds()[1:], MaxPages: 1})
	if _, _, err := src.Collect(context.Background(), nil, captured); err != nil {
		t.Fatal(err)
	}
	if q := f2.reqs[0].Query; !strings.Contains(q, "filter: { createdAt: { gt: $t0 } }") || f2.reqs[0].Variables["t0"] != "2026-11-17T18:58:00Z" {
		t.Errorf("inbox filter: %s %v, want createdAt after now minus lookback minus 2 minutes", q, f2.reqs[0].Variables["t0"])
	}
}

func TestPages(t *testing.T) {
	f := newFake(t, "page1.json", "page2.json")
	src := newSource(f, Settings{Feeds: []config.Feed{{ID: "mine", Kind: config.FeedComments}}, PageSize: 3, MaxPages: 2})
	msgs, _, err := src.Collect(context.Background(), nil, captured)
	if err != nil {
		t.Fatal(err)
	}
	if len(f.reqs) != 2 || len(msgs) != 6 {
		t.Fatalf("requests = %d, messages = %d, want 2 and 6", len(f.reqs), len(msgs))
	}
	if !strings.Contains(f.reqs[0].Query, "comments(first: 3,") || f.reqs[1].Variables["a0"] != "00000000-0000-4000-8000-000000000011" {
		t.Errorf("second request = %v, want after the end cursor of page 1", f.reqs[1].Variables)
	}
}

func TestOnlyQueries(t *testing.T) {
	f := newFake(t, "viewer.json", "poll.json", "poll2.json")
	src := newSource(f, Settings{Feeds: defaultFeeds(), MaxPages: 2})
	if _, err := src.Check(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, _, err := src.Collect(context.Background(), nil, captured); err != nil {
		t.Fatal(err)
	}
	for i, r := range f.reqs {
		if !strings.HasPrefix(r.Query, "query ") || strings.Contains(r.Query, "mutation") {
			t.Errorf("request %d is not only a query:\n%s", i, r.Query)
		}
	}
	err := src.client.Query(context.Background(), "mutation { notificationMarkReadAll { success } }", nil, nil)
	if err == nil || len(f.reqs) != 3 {
		t.Errorf("Query sent a mutation: err = %v, requests = %d", err, len(f.reqs))
	}
}

func TestCheck(t *testing.T) {
	f := newFake(t, "viewer.json")
	v, err := newSource(f, Settings{}).Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if v.User() != "me.user@acme" || v.Organization.Name != "Acme" {
		t.Errorf("viewer = %+v", v)
	}
}

func TestAuthError(t *testing.T) {
	f := newFake(t, "auth_error.json")
	_, _, err := newSource(f, Settings{Feeds: defaultFeeds()}).Collect(context.Background(), nil, captured)
	if !errors.Is(err, ErrUnauthorized) || !strings.Contains(err.Error(), "linear account lin") || !strings.Contains(err.Error(), "AUTHENTICATION_ERROR") {
		t.Errorf("error = %v, want an authentication error that names the account", err)
	}
}

func TestRateLimitRetry(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(readFixture(t, "viewer.json").Body)
	}))
	defer srv.Close()
	c := NewClient(srv.URL, "k", nil)
	var waits []time.Duration
	c.sleep = func(_ context.Context, d time.Duration) error { waits = append(waits, d); return nil }
	v, err := New(Settings{ID: "lin"}, c).Check(context.Background())
	if err != nil || v.DisplayName != "me.user" || calls != 2 || len(waits) != 1 {
		t.Errorf("err = %v, viewer = %+v, calls = %d, waits = %v", err, v, calls, waits)
	}
}
