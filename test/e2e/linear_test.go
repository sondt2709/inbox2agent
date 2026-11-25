//go:build e2e

package e2e

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/sondt2709/inbox2agent/internal/config"
	"github.com/sondt2709/inbox2agent/internal/inbox"
	"github.com/sondt2709/inbox2agent/test/e2e/harness"
)

// linearServer is a fake Linear GraphQL endpoint. It answers with the real
// responses that internal/source/linear/testdata/capture saved: viewer.json
// for the key check, catalog.json for the names of the workspace and
// poll.json for a poll. It records each query.
type linearServer struct {
	*httptest.Server
	mu      sync.Mutex
	queries []string
}

func newLinearServer(t *testing.T) *linearServer {
	t.Helper()
	read := func(name string) (int, []byte) {
		b, err := os.ReadFile(filepath.Join("..", "..", "internal", "source", "linear", "testdata", name))
		if err != nil {
			t.Fatal(err)
		}
		var fx struct {
			Status int             `json:"status"`
			Body   json.RawMessage `json:"body"`
		}
		if err := json.Unmarshal(b, &fx); err != nil {
			t.Fatal(err)
		}
		return fx.Status, fx.Body
	}
	s := &linearServer{}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Query string `json:"query"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		s.mu.Lock()
		s.queries = append(s.queries, req.Query)
		s.mu.Unlock()
		name := "poll.json"
		switch {
		case strings.HasPrefix(req.Query, "query Viewer"):
			name = "viewer.json"
		case strings.HasPrefix(req.Query, "query Catalog"):
			name = "catalog.json"
		}
		st, body := read(name)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(st)
		_, _ = w.Write(body)
	}))
	t.Cleanup(s.Close)
	return s
}

// TestS49LinearAgentComments: a Linear account with the default feeds. The
// feed agent sends the comments of the user that start with /agent, also
// after a mention of the user. i2a sends only queries.
func TestS49LinearAgentComments(t *testing.T) {
	srv := newLinearServer(t)
	p := newPerson(t, "S49")
	tm := harness.Start(t, filepath.Join(outRoot, p.id), `printf %s "$KEY" | `+bin+" account add linear --id lin --url "+srv.URL+" --token-stdin",
		merge(p.env, map[string]string{"KEY": "lin_api_e2e"}))
	tm.WaitFor(`Connected as me\.user@acme`, 30*time.Second)
	tm.WaitFor(`\[exit 0\]`, 10*time.Second)

	s, code := p.run("conversations --account lin")
	if code != 0 || !regexp.MustCompile(`lin\s+agent\s+channel\s+Agent requests`).MatchString(s) || !strings.Contains(s, "Inbox") {
		t.Fatalf("conversations: exit %d:\n%s", code, s)
	}
	// The fixtures are older than the default lookback; the inbox feed is off.
	p.editConfig(func(c *config.Config) {
		c.Settings.Lookback = config.Duration(10 * 365 * 24 * time.Hour)
		if err := c.SetConversation("lin", "inbox", config.UnsortedID, config.ReadOff, ""); err != nil {
			t.Fatal(err)
		}
	})

	s, code = p.run("run --once")
	if code != 0 || !strings.Contains(s, "collected 5 new messages") || !strings.Contains(s, "batch of Unsorted (dry-run, 5 messages): sent") {
		t.Fatalf("run --once: exit %d:\n%s", code, s)
	}
	ds := p.digests()
	if len(ds) != 1 {
		t.Fatalf("digests = %v, want 1", ds)
	}
	items := readDigest(t, ds[0])
	got := texts(items)
	head := "ENG-101 i2a test: Linear source fixtures\n\n"
	want := []string{
		head + "/agent and also list the risks",
		head + "/agent check this note",
		head + "/agent summarize this issue",
		head + "@me.user /agent please check this",
		head + "https://linear.app/acme/profiles/me.user /agent please check the mention link",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("digest texts =\n%q\nwant\n%q", got, want)
	}
	for _, it := range items {
		if it.Channel != "Agent requests" || it.Author != "me.user" {
			t.Errorf("item %+v, want the conversation Agent requests and the author me.user", it)
		}
		if strings.HasSuffix(it.Text, "risks") && (it.ReplyTo == nil || !it.ReplyTo.Me || it.ReplyTo.Text != "/agent summarize this issue") {
			t.Errorf("the reply has no reply_to of the user: %+v", it.ReplyTo)
		}
	}
	if c := p.counts(); c[inbox.StatePreviewed] != 5 {
		t.Errorf("store counts = %v, want 5 previewed", c)
	}

	// The next poll reads the same items again (the overlap); the store drops them.
	s, code = p.run("run --once")
	if code != 0 || !strings.Contains(s, "collected 0 new messages") {
		t.Fatalf("second run: exit %d:\n%s", code, s)
	}

	srv.mu.Lock()
	defer srv.mu.Unlock()
	if len(srv.queries) < 3 {
		t.Fatalf("queries = %d, want the check and two polls", len(srv.queries))
	}
	for i, q := range srv.queries {
		if !strings.HasPrefix(q, "query ") || strings.Contains(q, "mutation") {
			t.Errorf("request %d is not only a query:\n%s", i, q)
		}
		if strings.Contains(q, "notifications(") {
			t.Errorf("request %d reads the inbox feed with the level off", i)
		}
	}
}

// TestS50LinearFeedForm: the dashboard changes the filters of a Linear feed
// and adds a feed. The label comes from the names of the workspace.
func TestS50LinearFeedForm(t *testing.T) {
	srv := newLinearServer(t)
	p := newPerson(t, "S50")
	add := harness.Start(t, filepath.Join(outRoot, p.id), `printf %s "$KEY" | `+bin+" account add linear --id lin --url "+srv.URL+" --token-stdin",
		merge(p.env, map[string]string{"KEY": "lin_api_e2e"}))
	add.WaitFor(`\[exit 0\]`, 30*time.Second)

	tm := p.term("config")
	tm.WaitFor(`\+ New space`, 30*time.Second)
	tm.Press("Tab", `Edit feeds \(key f\)`, 5*time.Second) // Spaces → Accounts; the account lin is selected
	tm.Press("f", `Feeds of lin`, 5*time.Second)
	tm.Press("Enter", `lin · feed agent`, 5*time.Second)
	for _, f := range []string{"Kind", "From", "Prefix", "Projects", "Labels"} {
		tm.Press("Tab", `▌ `+f, 5*time.Second)
	}
	tm.Press("Enter", `Search ›`, 5*time.Second)
	tm.WaitFor(`\[ \] Label 1\b`, 10*time.Second) // the names came from the catalog query
	tm.Type("Label 17")
	tm.WaitFor(`▌ \[ \] Label 17`, 5*time.Second)
	tm.Press("Enter", `\[x\] Label 17`, 5*time.Second)
	tm.Press("C-s", `▌ Labels\s+Label 17`, 5*time.Second)
	tm.Press("C-s", `Saved the feed Agent requests`, 10*time.Second)
	if s := tm.WaitFor(`Feed\s+agent · comments · from me · prefix /agent · labels Label 17`, 5*time.Second); strings.Contains(s, "Search ›") {
		t.Fatalf("the names dialog is still open:\n%s", s)
	}

	// A new feed with a typed project.
	tm.Press("f", `Feeds of lin`, 5*time.Second)
	tm.Keys("Down", "Down")
	tm.Press("Enter", `lin · new feed`, 5*time.Second)
	tm.Replace("bridge")
	for _, f := range []string{"Name", "Kind", "From", "Prefix"} {
		tm.Press("Tab", `▌ `+f, 5*time.Second)
	}
	tm.Replace(">agent")
	tm.Press("Tab", `▌ Projects`, 5*time.Second)
	tm.Type("Project 3, Project 9")
	tm.Press("C-s", `Added the feed bridge`, 10*time.Second)
	tm.Keys("q")

	c := p.config()
	a, ok := c.Account("lin")
	if !ok || len(a.Feeds) != 3 {
		t.Fatalf("account lin = %+v", a)
	}
	if f := a.Feeds[0]; !slices.Equal(f.Labels, []string{"Label 17"}) || f.Prefix != "/agent" {
		t.Errorf("feed agent = %+v", f)
	}
	if f := a.Feeds[2]; f.ID != "bridge" || f.Kind != config.FeedComments || f.Prefix != ">agent" ||
		!slices.Equal(f.Projects, []string{"Project 3", "Project 9"}) || !slices.Equal(f.From, []string{"me"}) {
		t.Errorf("feed bridge = %+v", f)
	}
	s, code := p.run("conversations --account lin")
	if code != 0 || !regexp.MustCompile(`lin\s+bridge\s+channel\s+bridge`).MatchString(s) {
		t.Errorf("conversations: exit %d:\n%s", code, s)
	}
	srv.mu.Lock()
	defer srv.mu.Unlock()
	for i, q := range srv.queries {
		if !strings.HasPrefix(q, "query ") || strings.Contains(q, "mutation") {
			t.Errorf("request %d is not only a query:\n%s", i, q)
		}
	}
}
