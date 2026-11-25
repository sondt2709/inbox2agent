package tui

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/sondt2709/inbox2agent/internal/config"
	"github.com/sondt2709/inbox2agent/internal/source/linear"
)

// newLinearModel adds the Linear account lin with the feeds agent and inbox;
// the space acme lists the feed inbox. catErr makes the name query fail.
func newLinearModel(t *testing.T, catErr error) *model {
	t.Helper()
	m := newTestModel(t)
	m.u.d.Catalog = func(_ context.Context, a config.Account, key string) (linear.Catalog, error) {
		if a.ID != "lin" || key != "lin_api_test" {
			t.Errorf("catalog of %s with the key %q", a.ID, key)
		}
		if catErr != nil {
			return linear.Catalog{}, catErr
		}
		return linear.Catalog{Teams: []linear.Team{{Key: "ENG", Name: "Engineering"}},
			Projects: []string{"Bridge", "Mobile App"}, Labels: []string{"Bug", "Infra"}, Users: []string{"alice"}}, nil
	}
	a := config.NewLinearAccount("lin")
	a.Name, a.User = "Linear", "me.user@acme"
	if err := m.u.commit(func(c *config.Config) error {
		c.Accounts = append(c.Accounts, a)
		s, _ := c.Space("acme")
		s.Conversations = append(s.Conversations, config.Conversation{Account: "lin", ID: "inbox", Read: config.ReadMentions})
		return nil
	}, map[string]string{"lin": "lin_api_test"}); err != nil {
		t.Fatal(err)
	}
	m.chs["lin"] = feedInfos(a)
	m.Update(tea.WindowSizeMsg{Width: 180, Height: 50})
	at(m, tabAccounts)
	m.acc = 1
	m.buildForm(true)
	return m
}

// focusField gives the keyboard to the field key of the open form.
func focusField(t *testing.T, m *model, key string) {
	t.Helper()
	m.form.active = true
	for i, it := range m.form.items() {
		if it.field != nil && it.field.key == key {
			m.form.setFocus(i)
			return
		}
	}
	t.Fatalf("the form has no field %q", key)
}

func pressButton(t *testing.T, m *model, label string) {
	t.Helper()
	for _, b := range m.form.buttons {
		if b.label == label {
			run(m, b.run(), 0)
			return
		}
	}
	t.Fatalf("the form has no button %q", label)
}

func linFeeds(t *testing.T, m *model) []config.Feed {
	t.Helper()
	a, ok := m.u.cfg.Account("lin")
	if !ok {
		t.Fatal("no account lin")
	}
	return a.Feeds
}

func TestFeedLabelFromList(t *testing.T) {
	m := newLinearModel(t, nil)
	s := screen(m)
	for _, want := range []string{"Edit feeds", "Edit feeds (key f) adds a feed or changes its filters"} {
		if !strings.Contains(s, want) {
			t.Errorf("the Linear account panel has no %q:\n%s", want, s)
		}
	}
	send(m, "f")
	d, ok := m.topDialog().(*pickDialog)
	if !ok || len(d.options) != 3 || d.options[2].label != "+ New feed" {
		t.Fatalf("f did not open the feed list: %#v", m.topDialog())
	}
	send(m, "enter") // the feed agent
	if m.feedForm != "lin/agent" || m.form == nil || !strings.Contains(screen(m), "Linear · feed agent") {
		t.Fatalf("enter did not open the feed form of agent: %q\n%s", m.feedForm, screen(m))
	}
	focusField(t, m, "labels")
	send(m, "enter")
	nd, ok := m.topDialog().(*namesDialog)
	if !ok {
		t.Fatalf("enter in Labels did not open the names: %#v", m.topDialog())
	}
	if s := screen(m); !strings.Contains(s, "[ ] Bug") || !strings.Contains(s, "[ ] Infra") {
		t.Fatalf("the names dialog does not show the labels:\n%s", s)
	}
	typeText(m, "inf")
	if rows := nd.rows(); len(rows) != 1 || rows[0].value != "Infra" {
		t.Fatalf("the search inf shows %v", rows)
	}
	send(m, "enter", "ctrl+s")
	if got := m.form.field("labels").value(); got != "Infra" {
		t.Fatalf("labels field = %q", got)
	}
	send(m, "ctrl+s") // Save feed
	if f := linFeeds(t, m)[0]; !slices.Equal(f.Labels, []string{"Infra"}) || f.Prefix != "/agent" || !slices.Equal(f.From, []string{"me"}) {
		t.Fatalf("saved feed = %+v; form error %q", f, m.form.err)
	}
	// The checked names come first when the list opens again.
	run(m, m.openFeedForm("lin", "agent"), 0)
	focusField(t, m, "labels")
	send(m, "enter")
	if rows := m.topDialog().(*namesDialog).rows(); len(rows) != 2 || rows[0].value != "Infra" || rows[1].value != "Bug" {
		t.Errorf("rows = %v, want Infra first", rows)
	}
	send(m, "esc", "esc")
	if !strings.Contains(fileText(t, m), "labels = ['Infra']") {
		t.Errorf("config.toml has no labels:\n%s", fileText(t, m))
	}
	if m.feedForm != "" || !strings.Contains(screen(m), "Feed        agent · comments · from me · prefix /agent · labels Infra") {
		t.Errorf("after the save, the account panel does not show the label:\n%s", screen(m))
	}
}

func TestFeedTypedProjectsAndTeam(t *testing.T) {
	m := newLinearModel(t, nil)
	run(m, m.openFeedForm("lin", "agent"), 0)
	m.form.field("projects").input.SetValue("Bridge, Mobile, bridge")
	focusField(t, m, "teams")
	send(m, "enter")
	if s := screen(m); !strings.Contains(s, "[ ] ENG  Engineering") {
		t.Fatalf("the team list does not show the key and the name:\n%s", s)
	}
	send(m, "enter", "ctrl+s", "ctrl+s")
	f := linFeeds(t, m)[0]
	if !slices.Equal(f.Projects, []string{"Bridge", "Mobile"}) || !slices.Equal(f.Teams, []string{"ENG"}) {
		t.Fatalf("saved feed = %+v; form error %q", f, m.form.err)
	}
}

func TestNewFeed(t *testing.T) {
	m := newLinearModel(t, nil)
	send(m, "f", "down", "down", "enter")
	if m.feedForm != "lin/+" || !strings.Contains(screen(m), "Linear · new feed") {
		t.Fatalf("+ New feed did not open the form: %q\n%s", m.feedForm, screen(m))
	}
	if strings.Contains(screen(m), "Remove feed") {
		t.Error("the form of a new feed has Remove feed")
	}
	m.form.field("id").input.SetValue("agent")
	send(m, "ctrl+s")
	if f := m.form.field("id"); f.err == "" || len(linFeeds(t, m)) != 2 {
		t.Fatalf("a used feed ID was saved: %q", f.err)
	}
	m.form.field("id").input.SetValue("bridge")
	m.form.field("prefix").input.SetValue(">agent")
	send(m, "ctrl+s")
	feeds := linFeeds(t, m)
	if len(feeds) != 3 || feeds[2].ID != "bridge" || feeds[2].Kind != config.FeedComments || feeds[2].Prefix != ">agent" ||
		!slices.Equal(feeds[2].From, []string{"me"}) {
		t.Fatalf("feeds = %+v; form error %q", feeds, m.form.err)
	}
	if !strings.Contains(m.status, "Added the feed bridge. It is in Everything else") {
		t.Errorf("status = %q", m.status)
	}
	found := false
	for _, r := range m.convRows() {
		if r.Account.ID == "lin" && r.ID == "bridge" {
			found = r.Space == config.UnsortedID
		}
	}
	if !found {
		t.Error("the feed bridge is not a conversation in Everything else")
	}
}

func TestRemoveFeed(t *testing.T) {
	m := newLinearModel(t, nil)
	run(m, m.openFeedForm("lin", "inbox"), 0)
	pressButton(t, m, "Remove feed")
	if _, ok := m.topDialog().(*confirmDialog); !ok {
		t.Fatal("Remove feed did not ask")
	}
	send(m, "y")
	if feeds := linFeeds(t, m); len(feeds) != 1 || feeds[0].ID != "agent" {
		t.Fatalf("feeds = %+v", feeds)
	}
	s, _ := m.u.cfg.Space("acme")
	if slices.ContainsFunc(s.Conversations, func(cv config.Conversation) bool { return cv.Account == "lin" }) {
		t.Errorf("acme still lists the removed feed: %+v", s.Conversations)
	}
	if m.feedForm != "" || !strings.Contains(m.status, "Removed the feed Inbox.") {
		t.Errorf("feed form %q, status %q", m.feedForm, m.status)
	}

	// The last feed stays.
	run(m, m.openFeedForm("lin", "agent"), 0)
	pressButton(t, m, "Remove feed")
	if !strings.Contains(m.form.err, "A Linear account needs one feed or more") || len(linFeeds(t, m)) != 1 {
		t.Fatalf("the last feed: form error %q, feeds %+v", m.form.err, linFeeds(t, m))
	}
	if _, ok := m.topDialog().(*confirmDialog); ok {
		t.Error("the last feed opened the question")
	}
}

func TestFeedNamesCannotBeRead(t *testing.T) {
	m := newLinearModel(t, errors.New("linear: the API key is rejected"))
	run(m, m.openFeedForm("lin", "agent"), 0)
	focusField(t, m, "labels")
	send(m, "enter")
	if s := screen(m); !strings.Contains(s, "Cannot read the names: linear: the API key is rejected") {
		t.Fatalf("the names dialog does not show the error:\n%s", s)
	}
	typeText(m, "infra")
	send(m, "enter")
	if s := screen(m); !strings.Contains(s, "[x] infra") {
		t.Fatalf("enter on + Add did not check infra:\n%s", s)
	}
	send(m, "ctrl+s")
	if got := m.form.field("labels").value(); got != "infra" {
		t.Fatalf("labels field = %q", got)
	}
}

func TestFeedFormEscAsks(t *testing.T) {
	m := newLinearModel(t, nil)
	run(m, m.openFeedForm("lin", "agent"), 0)
	send(m, "esc")
	if m.feedForm != "" || m.formFor != "acc:lin" {
		t.Fatalf("esc on a clean feed form: feed form %q, form %q", m.feedForm, m.formFor)
	}
	run(m, m.openFeedForm("lin", "agent"), 0)
	m.form.field("prefix").input.SetValue(">agent")
	send(m, "esc")
	if _, ok := m.topDialog().(*confirmDialog); !ok || m.feedForm != "lin/agent" {
		t.Fatalf("esc on a changed feed form did not ask: %q", m.feedForm)
	}
	send(m, "d")
	if m.feedForm != "" || linFeeds(t, m)[0].Prefix != "/agent" {
		t.Fatalf("discard: feed form %q, prefix %q", m.feedForm, linFeeds(t, m)[0].Prefix)
	}
}

func TestFeedFormConflict(t *testing.T) {
	m := newLinearModel(t, nil)
	run(m, m.openFeedForm("lin", "agent"), 0)
	// Another process changes the account after the form opens.
	if _, _, err := config.Update(m.u.d.Paths.ConfigFile, func(c *config.Config) error {
		a, _ := c.Account("lin")
		a.Feeds[1].Labels = []string{"Bug"}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	m.form.field("prefix").input.SetValue(">agent")
	send(m, "ctrl+s")
	if d, ok := m.topDialog().(*confirmDialog); !ok || d.name != "Changed in another process" {
		t.Fatalf("no conflict dialog: %#v", m.topDialog())
	}
	if !strings.Contains(fileText(t, m), "prefix = '/agent'") {
		t.Error("the save wrote over the change of the other process")
	}
}
