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

// newLinearModel adds the Linear account lin with the feeds agent and inbox
// and selects it in Setup › Accounts; the space acme lists the feed inbox.
// catErr makes the name query fail.
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
	m.Update(tea.WindowSizeMsg{Width: 180, Height: 120})
	at(m, tabAccounts)
	m.acc = 1
	m.buildForm(true)
	return m
}

// focusField gives the keyboard to the field key of the form.
func focusField(t *testing.T, m *model, key string) {
	t.Helper()
	m.form.active = true
	for i, it := range m.form.items() {
		if it.field != nil && it.field.key == key {
			m.form.setFocus(i)
			return
		}
	}
	t.Fatalf("the form has no visible field %q", key)
}

// press runs the button field key, as enter on it does.
func press(t *testing.T, m *model, key string) {
	t.Helper()
	focusField(t, m, key)
	send(m, "enter")
}

func linFeeds(t *testing.T, m *model) []config.Feed {
	t.Helper()
	a, ok := m.u.cfg.Account("lin")
	if !ok {
		t.Fatal("no account lin")
	}
	return a.Feeds
}

func TestFeedSectionsInPanel(t *testing.T) {
	m := newLinearModel(t, nil)
	s := screen(m)
	for _, want := range []string{"Feed agent · its conversation is in Everything else", "Feed inbox · its conversation is in Acme",
		"Remove feed agent", "Remove feed inbox", "+ Add feed", "Choose ▾", "Prefix", "/agent", "Save changes"} {
		if !strings.Contains(s, want) {
			t.Errorf("the account panel has no %q:\n%s", want, s)
		}
	}
	if strings.Contains(s, "Edit feeds") || m.topDialog() != nil {
		t.Errorf("the feeds are behind a button or a dialog:\n%s", s)
	}
	if m.form.dirty() {
		t.Error("a new account form is dirty")
	}
}

func TestFeedLabelFromList(t *testing.T) {
	m := newLinearModel(t, nil)
	focusField(t, m, "f0.labels")
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
	if got := m.form.field("f0.labels").value(); got != "Infra" {
		t.Fatalf("labels field = %q", got)
	}
	send(m, "ctrl+s") // Save changes
	if f := linFeeds(t, m)[0]; !slices.Equal(f.Labels, []string{"Infra"}) || f.Prefix != "/agent" || !slices.Equal(f.From, []string{"me"}) {
		t.Fatalf("saved feed = %+v; form error %q", f, m.form.err)
	}
	if !strings.Contains(fileText(t, m), "labels = ['Infra']") || !strings.Contains(m.status, "Saved account Linear.") {
		t.Errorf("status %q, config.toml:\n%s", m.status, fileText(t, m))
	}
	// The checked names come first when the list opens again.
	focusField(t, m, "f0.labels")
	send(m, "enter")
	if rows := m.topDialog().(*namesDialog).rows(); len(rows) != 2 || rows[0].value != "Infra" || rows[1].value != "Bug" {
		t.Errorf("rows = %v, want Infra first", rows)
	}
}

func TestFeedChooseButton(t *testing.T) {
	m := newLinearModel(t, nil)
	m.form.field("f0.projects").input.SetValue("Bridge, Mobile, bridge")
	screen(m) // draw the form, so it has its click zones
	var zone *formZone
	for i, z := range m.form.zones {
		if it := m.form.items()[z.item]; z.opt == optChoose && it.field != nil && it.field.key == "f0.teams" {
			zone = &m.form.zones[i]
		}
	}
	if zone == nil {
		t.Fatal("the Teams field has no Choose button")
	}
	run(m, m.focusForm(), 0) // a click in the panel gives the keyboard to the form, as model.click does
	run(m, m.form.click(zone.x0+1, zone.y), 0)
	if s := screen(m); !strings.Contains(s, "[ ] ENG  Engineering") {
		t.Fatalf("Choose did not open the team list with the key and the name:\n%s", s)
	}
	send(m, "enter", "ctrl+s", "ctrl+s")
	f := linFeeds(t, m)[0]
	if !slices.Equal(f.Projects, []string{"Bridge", "Mobile"}) || !slices.Equal(f.Teams, []string{"ENG"}) {
		t.Fatalf("saved feed = %+v; form error %q", f, m.form.err)
	}
}

func TestAddFeedInPanel(t *testing.T) {
	m := newLinearModel(t, nil)
	press(t, m, "addfeed")
	if !m.form.dirty() || !strings.Contains(screen(m), "New feed · a new conversation in Everything else") {
		t.Fatalf("+ Add feed did not add a section:\n%s", screen(m))
	}
	if it := m.form.current(); it.field == nil || it.field.key != "f2.id" || it.field.value() != "" {
		key, value := "", ""
		if it.field != nil {
			key, value = it.field.key, it.field.value()
		}
		t.Fatalf("the keyboard is not in an empty new Feed ID: field %q, value %q", key, value)
	}
	send(m, "ctrl+s")
	if f := m.form.field("f2.id"); !strings.Contains(f.err, "type a feed ID") || len(linFeeds(t, m)) != 2 {
		t.Fatalf("an empty feed ID was saved: %q", f.err)
	}
	typeText(m, "bri")
	if got := m.form.field("f2.id").value(); got != "bri" {
		t.Fatalf("typing in the new Feed ID gives %q", got)
	}
	m.form.field("f2.id").input.SetValue("agent")
	send(m, "ctrl+s")
	if f := m.form.field("f2.id"); f.err == "" || len(linFeeds(t, m)) != 2 {
		t.Fatalf("a used feed ID was saved: %q", f.err)
	}
	m.form.field("f2.id").input.SetValue("bridge")
	m.form.field("f2.prefix").input.SetValue(">agent")
	send(m, "ctrl+s")
	feeds := linFeeds(t, m)
	if len(feeds) != 3 || feeds[2].ID != "bridge" || feeds[2].Kind != config.FeedComments || feeds[2].Prefix != ">agent" ||
		!slices.Equal(feeds[2].From, []string{"me"}) {
		t.Fatalf("feeds = %+v; form error %q", feeds, m.form.err)
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
	if !strings.Contains(screen(m), "Feed bridge · its conversation is in Everything else") {
		t.Errorf("the saved feed has no section:\n%s", screen(m))
	}
}

func TestRemoveFeedInPanel(t *testing.T) {
	m := newLinearModel(t, nil)
	press(t, m, "f1.remove")
	s := screen(m)
	if !m.form.dirty() || !strings.Contains(s, "Keep feed inbox") || !strings.Contains(s, "Feed inbox · removed when you save") ||
		m.form.field("f1.prefix").visible() {
		t.Fatalf("Remove feed inbox did not mark the section:\n%s", s)
	}
	send(m, "ctrl+s")
	if feeds := linFeeds(t, m); len(feeds) != 1 || feeds[0].ID != "agent" {
		t.Fatalf("feeds = %+v; form error %q", feeds, m.form.err)
	}
	sp, _ := m.u.cfg.Space("acme")
	if slices.ContainsFunc(sp.Conversations, func(cv config.Conversation) bool { return cv.Account == "lin" }) {
		t.Errorf("acme still lists the removed feed: %+v", sp.Conversations)
	}

	// The last feed stays.
	press(t, m, "f0.remove")
	send(m, "ctrl+s")
	if !strings.Contains(m.form.err, "A Linear account needs one feed or more") || len(linFeeds(t, m)) != 1 {
		t.Fatalf("the last feed: form error %q, feeds %+v", m.form.err, linFeeds(t, m))
	}
	// Keep feed undoes the mark.
	press(t, m, "f0.remove")
	if !m.form.field("f0.prefix").visible() || !strings.Contains(screen(m), "Remove feed agent") {
		t.Error("Keep feed agent did not show the section again")
	}
}

func TestFeedNamesCannotBeRead(t *testing.T) {
	m := newLinearModel(t, errors.New("linear: the API key is rejected"))
	focusField(t, m, "f0.labels")
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
	if got := m.form.field("f0.labels").value(); got != "infra" {
		t.Fatalf("labels field = %q", got)
	}
}

func TestFeedFormConflict(t *testing.T) {
	m := newLinearModel(t, nil)
	// Another process changes a feed after the form opens.
	if _, _, err := config.Update(m.u.d.Paths.ConfigFile, func(c *config.Config) error {
		a, _ := c.Account("lin")
		a.Feeds[1].Labels = []string{"Bug"}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	m.form.field("f0.prefix").input.SetValue(">agent")
	focusField(t, m, "f0.prefix")
	send(m, "ctrl+s")
	if d, ok := m.topDialog().(*confirmDialog); !ok || d.name != "Changed in another process" {
		t.Fatalf("no conflict dialog: %#v", m.topDialog())
	}
	if !strings.Contains(fileText(t, m), "prefix = '/agent'") {
		t.Error("the save wrote over the change of the other process")
	}
}
