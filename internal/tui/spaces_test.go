package tui

import (
	"context"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/sondt2709/inbox2agent/internal/config"
)

// The test model has the space Acme with # release (work/C1, all), and the
// account lists @bob (D1) and bob, carol (G1), which no space has.

func lines(m *model) []string { return strings.Split(screen(m), "\n") }

func TestHeaderShowsTheAreasAndSetupSections(t *testing.T) {
	m := newTestModel(t)
	m.seen = seenSet{"work": {"C1"}}
	head := lines(m)[0]
	for _, want := range []string{"1 Feed", "2 Spaces 2 new", "3 Setup", "● running", "R run now"} {
		if !strings.Contains(head, want) {
			t.Errorf("header %q has no %q", head, want)
		}
	}
	if m.tab != tabSpaces {
		t.Errorf("i2a config opens tab %d, want Spaces", m.tab)
	}
	send(m, "3")
	if sub := lines(m)[1]; m.tab != tabAccounts || !strings.Contains(sub, "Accounts · Agents · Schedules · Settings · Send log") {
		t.Fatalf("3 opens tab %d with %q", m.tab, sub)
	}
	send(m, "tab", "tab", "tab", "tab")
	if m.tab != tabActivity {
		t.Errorf("tab 4 times from Accounts: tab %d, want Send log", m.tab)
	}
	send(m, "tab")
	if m.tab != tabFeed {
		t.Errorf("tab after Send log: tab %d, want Feed", m.tab)
	}
	send(m, "3")
	if m.tab != tabActivity {
		t.Errorf("3 opens tab %d, want the last section of Setup", m.tab)
	}
	at(m, tabFeed)
	if sub := lines(m)[1]; !strings.Contains(sub, "No run yet. R reads all accounts") {
		t.Errorf("the line under the header of the Feed: %q", sub)
	}
}

func TestSpacesAreaShowsASpaceAndItsConversations(t *testing.T) {
	m := newTestModel(t)
	s := screen(m)
	for _, want := range []string{"● Acme", "Everything else", "New conversations", "Ignored", "+ New space",
		"Name       Acme", "Sends to   claude-work", "When       always", "Conversations · 1", "# release", "All messages"} {
		if !strings.Contains(s, want) {
			t.Errorf("the Spaces area has no %q:\n%s", want, s)
		}
	}
	send(m, "down") // Everything else
	s = screen(m)
	for _, want := range []string{"New and other conversations read Only for me", "Sends to   triage", "Conversations in no space · 2",
		"@ @bob", "⧉ bob, carol"} {
		if !strings.Contains(s, want) {
			t.Errorf("Everything else has no %q:\n%s", want, s)
		}
	}
}

func TestLevelRemoveAndUndoInASpace(t *testing.T) {
	m := newTestModel(t)
	send(m, "enter") // the panel of Acme, on # release
	if r, _ := m.panelSelected(); r.kind != "conv" || r.conv.ID != "C1" {
		t.Fatalf("enter selects %+v, want # release", r)
	}
	send(m, "right")
	if lv := loadFile(t, m).Level("work", "C1"); lv != config.ReadMentions {
		t.Fatalf("→ on # release: level %s, want mentions", lv)
	}
	if !strings.Contains(screen(m), "Only for me") || !strings.Contains(screen(m), "Saved · u undo") {
		t.Errorf("the row or the status does not show the change:\n%s", screen(m))
	}
	send(m, "x")
	if r := loadFile(t, m).Routes("work", "C1"); !slices.Equal(r, []string{config.UnsortedID}) {
		t.Fatalf("x on # release: routes %v, want Everything else", r)
	}
	send(m, "u")
	if r := loadFile(t, m).Routes("work", "C1"); !slices.Equal(r, []string{"acme"}) {
		t.Fatalf("u after x: routes %v, want Acme again", r)
	}
	// Another process changes the file: undo does not write it.
	send(m, "x")
	b, _ := os.ReadFile(m.u.d.Paths.ConfigFile)
	if err := os.WriteFile(m.u.d.Paths.ConfigFile, append(b, []byte("\n# edited\n")...), 0o600); err != nil {
		t.Fatal(err)
	}
	send(m, "u")
	if !strings.Contains(m.status, "another process changed config.toml") {
		t.Errorf("undo after a change of another process: %q", m.status)
	}
}

func TestSpacesDialogAddsASecondSpaceAndMoves(t *testing.T) {
	m := newTestModel(t)
	if err := m.u.commit(func(c *config.Config) error {
		c.Spaces = append(c.Spaces, config.Space{ID: "team", Name: "Team", Agent: "triage", Schedule: "always"})
		return nil
	}, nil); err != nil {
		t.Fatal(err)
	}
	send(m, "/")
	typeText(m, "release")
	send(m, "enter") // one match: its spaces dialog opens
	d, ok := m.topDialog().(*spacesDialog)
	if !ok || !d.opts[0].on || d.opts[1].on {
		t.Fatalf("the spaces dialog of # release: %+v", m.topDialog())
	}
	send(m, "down", "space", "right") // check Team, then Only for me
	if !strings.Contains(screen(m), "Also send the messages of the last lookback") {
		t.Errorf("the dialog does not offer the old messages:\n%s", screen(m))
	}
	send(m, "enter")
	c := loadFile(t, m)
	if got := c.Links("work", "C1"); len(got) != 2 || got[0].Read != config.ReadAll || got[1].Space != "team" || got[1].Read != config.ReadMentions {
		t.Fatalf("links after the dialog = %+v", got)
	}
	// Move: uncheck Acme, keep Team.
	m.openDialog(m.newSpacesDialog(m.rowsWhere(func(r convRow) bool { return r.ID == "C1" })))
	send(m, "space", "enter")
	if r := loadFile(t, m).Routes("work", "C1"); !slices.Equal(r, []string{"team"}) {
		t.Errorf("after the move: routes %v, want team", r)
	}
	// Ignore unchecks the spaces.
	ig := m.newSpacesDialog(m.rowsWhere(func(r convRow) bool { return r.ID == "C1" }))
	m.openDialog(ig)
	send(m, "down", "down", "space")
	if ig.opts[1].on || !ig.ignore || !strings.Contains(screen(m), "(x) Ignore") {
		t.Errorf("Ignore does not uncheck the spaces: %+v", ig.opts)
	}
	send(m, "enter")
	if l := loadFile(t, m).Links("work", "C1"); len(l) != 1 || l[0].Space != config.UnsortedID || l[0].Read != config.ReadOff {
		t.Errorf("after Ignore: %+v", l)
	}
}

// requeueStore records the calls of Requeue.
type requeueStore struct {
	fakeStore
	calls *[]string
}

func (s requeueStore) Requeue(_ context.Context, account, id string, since time.Time, forMe bool) (int64, error) {
	*s.calls = append(*s.calls, account+"/"+id)
	return 2, nil
}

func TestSpacesDialogSendsTheOldMessagesWhenAsked(t *testing.T) {
	m := newTestModel(t)
	var calls []string
	m.u.d.Store = requeueStore{calls: &calls}
	d := m.newSpacesDialog(m.rowsWhere(func(r convRow) bool { return r.ID == "D1" }))
	m.openDialog(d)
	send(m, "space")                         // check Acme
	send(m, "down", "down", "down", "space") // the option
	if !d.requeue {
		t.Fatalf("the option is off; cursor %d of %d", d.cursor, d.n())
	}
	send(m, "enter")
	if !slices.Equal(calls, []string{"work/D1"}) || !strings.Contains(m.status, "2 messages of the last") {
		t.Errorf("requeue calls %v, status %q", calls, m.status)
	}
	if r := loadFile(t, m).Routes("work", "D1"); !slices.Equal(r, []string{"acme"}) {
		t.Errorf("routes of D1 = %v", r)
	}
}

func TestNewConversationsKeepIgnoreAndReadAgain(t *testing.T) {
	m := newTestModel(t)
	all := m.chs["work"]
	m.gotChannels(channelsMsg{account: "work", chs: all[:1]}) // the first listing had only # release
	if n := m.newCount(); n != 0 {
		t.Fatalf("new after the first listing = %d, want 0", n)
	}
	m.gotChannels(channelsMsg{account: "work", chs: all})
	if n := m.newCount(); n != 2 || !strings.Contains(lines(m)[0], "2 new") {
		t.Fatalf("new after a second listing = %d, header %q", n, lines(m)[0])
	}
	send(m, "down", "down", "enter") // New conversations
	send(m, "i")                     // ignore the first row
	c := loadFile(t, m)
	ignored := m.ignoredRows()
	if len(ignored) != 1 || c.Level("work", ignored[0].ID) != config.ReadOff {
		t.Fatalf("after i: ignored %+v", ignored)
	}
	send(m, "k") // keep the other row
	if n := m.newCount(); n != 0 {
		t.Fatalf("new after k = %d", n)
	}
	if seen := readSeen(m.u.d.Paths.DataDir); len(seen["work"]) != 3 {
		t.Errorf("the seen file = %v, want the 3 conversations", seen)
	}
	send(m, "esc", "down", "enter", "x") // Ignored, read again
	if _, _, ok := loadFile(t, m).Conversation("work", ignored[0].ID); ok || len(m.ignoredRows()) != 0 {
		t.Errorf("x in Ignored keeps an entry")
	}
}

func TestAddConversationsWithTheSearch(t *testing.T) {
	m := newTestModel(t)
	send(m, "a") // Acme is selected
	d, ok := m.topDialog().(*searchDialog)
	if !ok || d.space != "acme" {
		t.Fatalf("a in Acme opens %v", m.topDialog())
	}
	typeText(m, "bob")
	send(m, "enter", "space") // to the list, then add the first row: @bob
	if r := loadFile(t, m).Routes("work", "D1"); !slices.Equal(r, []string{"acme"}) {
		t.Fatalf("routes of @bob = %v, want acme", r)
	}
	if !strings.Contains(screen(m), "[x]") {
		t.Errorf("the row has no check mark:\n%s", screen(m))
	}
	send(m, "space")
	if r := loadFile(t, m).Routes("work", "D1"); !slices.Equal(r, []string{config.UnsortedID}) {
		t.Errorf("space again: routes %v, want Everything else", r)
	}
}

func TestSettingRowsOfASpaceSaveAtOnce(t *testing.T) {
	m := newTestModel(t)
	send(m, "enter", "up") // the panel, then the row When
	send(m, "up", "enter") // Sends to
	pd, ok := m.topDialog().(*pickDialog)
	if !ok || len(pd.options) != 2 {
		t.Fatalf("enter on Sends to: %v", m.topDialog())
	}
	send(m, "up", "enter") // triage
	if c := loadFile(t, m); c.Spaces[0].Agent != "triage" {
		t.Errorf("agent of Acme = %s", c.Spaces[0].Agent)
	}
	send(m, "up", "enter", "ctrl+u")
	typeText(m, "Acme Corp")
	send(m, "enter")
	if c := loadFile(t, m); c.Spaces[0].Name != "Acme Corp" || c.Spaces[0].ID != "acme" {
		t.Errorf("rename: %+v", c.Spaces[0])
	}
}

func TestDeleteASpaceFromTheList(t *testing.T) {
	m := newTestModel(t)
	send(m, "x", "y")
	c := loadFile(t, m)
	if len(c.Spaces) != 0 || c.Route("work", "C1") != config.UnsortedID {
		t.Errorf("after the delete: %+v %+v", c.Spaces, c.Unsorted)
	}
}

func TestEverythingElseOnHold(t *testing.T) {
	m := newTestModel(t)
	send(m, "down", "enter", "up", "up") // the row Sends to of Everything else
	send(m, "enter", "up", "enter")      // hold is the first choice
	if c := loadFile(t, m); c.Unsorted.Agent != "" {
		t.Fatalf("agent of Unsorted = %q, want hold", c.Unsorted.Agent)
	}
	if s := screen(m); !strings.Contains(s, "hold: does not send") || !strings.Contains(s, "○ Everything else") {
		t.Errorf("the panel does not show hold:\n%s", s)
	}
}

func TestNarrowFooterKeepsTheMainKeys(t *testing.T) {
	m := newTestModel(t)
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 40})
	footer := func() string { l := lines(m); return l[len(l)-1] }
	if f := footer(); !strings.Contains(f, "R run now") || !strings.Contains(f, "enter open") {
		t.Errorf("Spaces at 80 columns: %q", f)
	}
	send(m, "enter")
	if f := footer(); !strings.Contains(f, "enter spaces") || !strings.Contains(f, "←→ level") {
		t.Errorf("the panel of a space at 80 columns: %q", f)
	}
}
