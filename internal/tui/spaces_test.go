package tui

import (
	"context"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/sondt2709/inbox2agent/internal/config"
)

// The test model has the space Acme with # release (work/C1, all), and the
// account lists @bob (D1) and bob, carol (G1), which no space has.

func lines(m *model) []string { return strings.Split(screen(m), "\n") }

func TestHeaderShowsTheAreasAndSetupSections(t *testing.T) {
	m := newTestModel(t)
	m.seen = seenSet{"work": {"C1"}}
	head := lines(m)[0]
	for _, want := range []string{"Feed", "Spaces 2 new", "Setup", "● running", "ctrl+r run now"} {
		if !strings.Contains(head, want) {
			t.Errorf("header %q has no %q", head, want)
		}
	}
	if m.tab != tabSpaces {
		t.Errorf("i2a config opens tab %d, want Spaces", m.tab)
	}
	at(m, tabAccounts) // a click on Setup
	if sub := lines(m)[1]; m.tab != tabAccounts || !strings.Contains(sub, "Accounts · Schedules · Settings · Send log") {
		t.Fatalf("3 opens tab %d with %q", m.tab, sub)
	}
	send(m, "tab", "tab", "tab")
	if m.tab != tabActivity {
		t.Errorf("tab 3 times from Accounts: tab %d, want Send log", m.tab)
	}
	send(m, "tab")
	if m.tab != tabFeed {
		t.Errorf("tab after Send log: tab %d, want Feed", m.tab)
	}
	send(m, "3")
	if m.tab != tabFeed {
		t.Errorf("3 opens tab %d; the areas have no number keys", m.tab)
	}
	at(m, tabFeed)
	if sub := lines(m)[1]; !strings.Contains(sub, "No run yet. ctrl+r reads all accounts") {
		t.Errorf("the line under the header of the Feed: %q", sub)
	}
}

func TestSpacesAreaShowsASpaceAndItsConversations(t *testing.T) {
	m := newTestModel(t)
	s := screen(m)
	for _, want := range []string{"● Acme", "Everything else", "New conversations", "Ignored", "+ New space",
		"Name       Acme", "Status     ● Sending", "Agent      orca · claude · /home/user/notes · reuse", "Session    none", "When       always", "Conversations · 1", "# release", "All messages"} {
		if !strings.Contains(s, want) {
			t.Errorf("the Spaces area has no %q:\n%s", want, s)
		}
	}
	send(m, "down") // Everything else
	s = screen(m)
	for _, want := range []string{"New and other conversations read Only for me", "Agent      dry-run", "Status     ● Sending", "Conversations in no space · 2",
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
	side := "\n[[spaces]]\nid = 'side'\nname = 'Side'\nschedule = 'always'\n"
	if err := os.WriteFile(m.u.d.Paths.ConfigFile, append(b, []byte(side)...), 0o600); err != nil {
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
		c.Spaces = append(c.Spaces, config.Space{ID: "team", Name: "Team", Agent: config.NewAgent("team"), Schedule: "always"})
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

func (s requeueStore) Requeue(_ context.Context, account, id string, since time.Time, forMe bool, skip []string) (int64, error) {
	*s.calls = append(*s.calls, account+"/"+id+" skip "+strings.Join(skip, ","))
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
	if !slices.Equal(calls, []string{"work/D1 skip "}) || !strings.Contains(m.status, "2 messages of the last") {
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
	if s := screen(m); !strings.Contains(s, "Add conversations to Acme") || !strings.Contains(s, "OK") || !strings.Contains(s, "Cancel") {
		t.Fatalf("the add dialog has no title or no buttons:\n%s", s)
	}
	routes := func() []string { return loadFile(t, m).Routes("work", "D1") }
	typeText(m, "bob")
	send(m, "enter", "space") // to the list, then mark the first row: @bob
	if r := routes(); !slices.Equal(r, []string{config.UnsortedID}) {
		t.Fatalf("space saved at once: routes of @bob = %v", r)
	}
	if s := screen(m); !strings.Contains(s, "[x]") || !strings.Contains(s, "1 change not saved") {
		t.Errorf("the row has no check mark, or the dialog does not count the change:\n%s", s)
	}
	send(m, "esc") // Cancel
	if m.topDialog() != nil || !slices.Equal(routes(), []string{config.UnsortedID}) {
		t.Fatalf("esc: dialog open %v, routes %v, want closed and no change", m.topDialog() != nil, routes())
	}

	// tab to OK, tab to Cancel, enter: no change.
	send(m, "a")
	typeText(m, "bob")
	send(m, "enter", "space", "tab", "tab", "enter")
	if m.topDialog() != nil || !slices.Equal(routes(), []string{config.UnsortedID}) {
		t.Fatalf("Cancel: dialog open %v, routes %v, want closed and no change", m.topDialog() != nil, routes())
	}

	// tab to OK and enter saves.
	send(m, "a")
	typeText(m, "bob")
	send(m, "enter", "space", "tab", "enter")
	if r := routes(); m.topDialog() != nil || !slices.Equal(r, []string{"acme"}) {
		t.Fatalf("OK: dialog open %v, routes of @bob = %v, want acme", m.topDialog() != nil, r)
	}

	// A click on OK saves: remove @bob again.
	send(m, "a")
	typeText(m, "bob")
	send(m, "enter", "space")
	d = m.topDialog().(*searchDialog)
	screen(m)
	bx, by, _, _ := m.dialogBox(d)
	m.Update(tea.MouseClickMsg{X: bx + 2 + d.btnX[0][0] + 1, Y: by + 2 + d.btnY, Button: tea.MouseLeft})
	if r := routes(); m.topDialog() != nil || !slices.Equal(r, []string{config.UnsortedID}) {
		t.Errorf("a click on OK: dialog open %v, routes %v, want Everything else", m.topDialog() != nil, r)
	}
}

func TestFooterHintIsLabelThenKey(t *testing.T) {
	s := renderHints([]hint{{"r", "reply"}})
	if got := ansi.Strip(s); got != "Reply r" {
		t.Fatalf("hint = %q, want \"Reply r\"", got)
	}
	label, key := strings.Index(s, "Reply"), strings.LastIndex(s, "r")
	if !strings.Contains(s[:label], "38;2;125;86;244") || strings.Contains(s[label:key], "38;2;125;86;244") {
		t.Errorf("the label must have the primary color #7D56F4 and the key another color: %q", s)
	}
}

func TestSettingRowsOfASpaceSaveAtOnce(t *testing.T) {
	m := newTestModel(t)
	send(m, "enter", "up", "up", "up", "up") // the panel, then the rows When, Session, Agent, Status
	send(m, "enter")
	if c := loadFile(t, m); !c.Spaces[0].Paused {
		t.Fatalf("enter on Status did not pause Acme")
	}
	if s := screen(m); !strings.Contains(s, "○ Acme") || !strings.Contains(s, "○ Paused: does not send") {
		t.Errorf("the screen does not show the pause:\n%s", s)
	}
	send(m, "enter")
	if c := loadFile(t, m); c.Spaces[0].Paused {
		t.Errorf("a second enter did not resume Acme")
	}
	send(m, "up", "enter", "ctrl+u")
	typeText(m, "Acme Corp")
	send(m, "enter")
	if c := loadFile(t, m); c.Spaces[0].Name != "Acme Corp" || c.Spaces[0].ID != "acme" {
		t.Errorf("rename: %+v", c.Spaces[0])
	}
}

// The When dialog renames the schedule of the space; its ID stays.
func TestWhenDialogRenamesTheSchedule(t *testing.T) {
	m := newTestModel(t)
	send(m, "enter", "up", "enter") // the row When
	pd, ok := m.topDialog().(*pickDialog)
	if !ok || len(pd.options) != 3 || pd.options[1].label != "Rename always…" || pd.options[2].label != "Edit the times of always in Setup" {
		t.Fatalf("the When dialog: %+v", m.topDialog())
	}
	send(m, "down", "enter", "ctrl+u")
	typeText(m, "Work hours")
	send(m, "enter")
	c := loadFile(t, m)
	if sc, _ := c.Schedule("always"); sc.Name != "Work hours" || c.Spaces[0].Schedule != "always" {
		t.Fatalf("schedule = %+v, Acme uses %q", sc, c.Spaces[0].Schedule)
	}
	if s := screen(m); !strings.Contains(s, "When       Work hours") {
		t.Errorf("the row When does not show the new name:\n%s", s)
	}
	send(m, "enter", "down", "down", "enter") // Edit the times
	if m.tab != tabSchedules || m.form == nil || !m.form.active || m.form.field("every") == nil {
		t.Errorf("Edit the times did not open the schedule form: tab %d", m.tab)
	}
}

// A new space copies the agent of the selected space, with no instruction.
func TestNewSpaceCopiesTheAgent(t *testing.T) {
	m := newTestModel(t)
	id, errText := m.newSpace("Borealis")
	if errText != "" {
		t.Fatal(errText)
	}
	c := loadFile(t, m)
	b, _ := c.Space(id)
	if b.Agent.Title() != "orca · claude" || b.Agent.Folder != "/home/user/notes" || b.Instruction != "" || b.Paused {
		t.Errorf("new space = %+v", b)
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
	send(m, "down", "enter", "up", "up", "up", "up") // the row Status of Everything else
	send(m, "enter")
	if c := loadFile(t, m); !c.Unsorted.Paused {
		t.Fatalf("Unsorted is not paused")
	}
	if s := screen(m); !strings.Contains(s, "Hold: does not send") || !strings.Contains(s, "○ Everything else") {
		t.Errorf("the panel does not show hold:\n%s", s)
	}
}

func TestNarrowFooterKeepsTheMainKeys(t *testing.T) {
	m := newTestModel(t)
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 40})
	footer := func() string { l := lines(m); return l[len(l)-1] }
	if f := footer(); !strings.Contains(f, "Run now ctrl+r") || !strings.Contains(f, "Open enter") {
		t.Errorf("Spaces at 80 columns: %q", f)
	}
	send(m, "enter")
	if f := footer(); !strings.Contains(f, "Spaces enter") || !strings.Contains(f, "Level ←→") {
		t.Errorf("the panel of a space at 80 columns: %q", f)
	}
}

// A selection whose rows are gone (another process, another tab) does not
// crash a key; the key works on the row under the cursor.
func TestAStaleSelectionUsesTheCursorRow(t *testing.T) {
	m := newTestModel(t)
	send(m, "enter")
	m.convSel[convKey("gone", "X1")] = true
	send(m, "right")
	if lv := loadFile(t, m).Level("work", "C1"); lv != config.ReadMentions {
		t.Errorf("→ with a stale selection: level %s, want mentions on # release", lv)
	}
	m.convSel[convKey("gone", "X1")] = true
	send(m, "enter")
	if d, ok := m.topDialog().(*spacesDialog); !ok || d.rows[0].ID != "C1" {
		t.Errorf("enter with a stale selection opens %v", m.topDialog())
	}
	send(m, "esc")
	m.convSel[convKey("work", "C1")] = true
	at(m, tabFeed)
	if len(m.convSel) != 0 {
		t.Errorf("a tab change keeps the selection %v", m.convSel)
	}
}

// When unsorted.read is off, "read again" writes an own level, else the
// conversation stays off.
func TestReadAgainWhenEverythingElseIsOff(t *testing.T) {
	m := newTestModel(t)
	changeFile(t, m, func(c *config.Config) error {
		c.Unsorted.Read = config.ReadOff
		return c.SetSpaces("work", "D1", "@bob", []config.Link{{Space: config.UnsortedID, Read: config.ReadOff}})
	})
	refresh(m)
	m.spc = 3 // Ignored
	send(m, "enter")
	if r, ok := m.panelSelected(); !ok || r.conv.ID == "" {
		t.Fatalf("Ignored has no row: %+v", r)
	}
	id := m.ignoredRows()[0].ID
	send(m, "x")
	if lv := loadFile(t, m).Level("work", id); lv != config.ReadMentions {
		t.Errorf("x in Ignored with unsorted.read off: level %s, want mentions", lv)
	}
}

func TestDeleteASpaceKeepsTheLabelAndLevel(t *testing.T) {
	m := newTestModel(t)
	changeFile(t, m, func(c *config.Config) error {
		c.Spaces = append(c.Spaces, config.Space{ID: "beta", Name: "Beta", Agent: config.NewAgent("beta"), Schedule: "always"})
		if err := c.SetSpaces("work", "C1", "release", []config.Link{{Space: "acme", Read: config.ReadMentions}, {Space: "beta", Read: config.ReadAll}}); err != nil {
			return err
		}
		return c.SetLabel("work", "C1", "Releases")
	})
	refresh(m)
	m.deleteSpace("acme")
	send(m, "y")
	c := loadFile(t, m)
	cv, sp, _ := c.Conversation("work", "C1")
	if sp != "beta" || cv.Label != "Releases" || cv.Read != config.ReadAll {
		t.Errorf("after the delete of Acme: %+v in %s, want Beta with the label", cv, sp)
	}
	if err := c.Validate(); err != nil {
		t.Error(err)
	}
}

func TestDialogLevelFromOffAndTheWarningOfManyRows(t *testing.T) {
	m := newTestModel(t)
	changeFile(t, m, func(c *config.Config) error {
		return c.SetSpaces("work", "C1", "release", []config.Link{{Space: "acme", Read: config.ReadOff}})
	})
	refresh(m)
	rows := m.rowsWhere(func(r convRow) bool { return r.ID == "C1" || r.ID == "D1" })
	d := m.newSpacesDialog(rows)
	m.openDialog(d)
	if !strings.Contains(screen(m), "They are in different spaces now") {
		t.Errorf("no warning for rows in different spaces:\n%s", screen(m))
	}
	send(m, "esc")
	one := m.newSpacesDialog(m.rowsWhere(func(r convRow) bool { return r.ID == "C1" }))
	m.openDialog(one)
	if one.opts[0].level != config.ReadOff {
		t.Fatalf("the dialog of C1 starts with %q, want off", one.opts[0].level)
	}
	send(m, "right")
	if one.opts[0].level != config.ReadAll {
		t.Errorf("→ on a link with off: level %q, want all", one.opts[0].level)
	}
}

func TestASetupSaveEndsTheUndo(t *testing.T) {
	m := newTestModel(t)
	send(m, "enter", "right") // a change in Spaces
	at(m, tabSettings)
	send(m, "enter", "ctrl+u", "2", "d", "ctrl+s")
	at(m, tabSpaces)
	send(m, "u")
	if !strings.Contains(m.status, "Nothing to undo") {
		t.Errorf("u after a save of Settings: %q", m.status)
	}
	if c := loadFile(t, m); c.Settings.Lookback.String() != "2d" {
		t.Errorf("the save of Settings is gone: %s", c.Settings.Lookback)
	}
}

func TestRequeueSkipsTheSpacesThatTheRowHad(t *testing.T) {
	m := newTestModel(t)
	var calls []string
	m.u.d.Store = requeueStore{calls: &calls}
	if err := m.u.commit(func(c *config.Config) error {
		c.Spaces = append(c.Spaces, config.Space{ID: "team", Name: "Team", Agent: config.NewAgent("team"), Schedule: "always"})
		return nil
	}, nil); err != nil {
		t.Fatal(err)
	}
	d := m.newSpacesDialog(m.rowsWhere(func(r convRow) bool { return r.ID == "C1" }))
	m.openDialog(d)
	send(m, "down", "space")                 // check Team
	send(m, "down", "down", "down", "space") // the option
	send(m, "enter")
	if !slices.Equal(calls, []string{"work/C1 skip acme"}) {
		t.Errorf("requeue calls = %v, want C1 with Acme skipped", calls)
	}
}

func TestNoSpaceKeepsTheLabel(t *testing.T) {
	m := newTestModel(t)
	changeFile(t, m, func(c *config.Config) error { return c.SetLabel("work", "C1", "Rel") })
	refresh(m)
	m.openDialog(m.newSpacesDialog(m.rowsWhere(func(r convRow) bool { return r.ID == "C1" })))
	send(m, "space", "enter") // uncheck Acme
	c := loadFile(t, m)
	if cv, sp, ok := c.Conversation("work", "C1"); !ok || sp != config.UnsortedID || cv.Label != "Rel" {
		t.Errorf("after uncheck of the last space: %+v in %s (entry %v), want the label in Everything else", cv, sp, ok)
	}
}

func TestRequeueOptionForAnyRowThatGetsASpace(t *testing.T) {
	m := newTestModel(t)
	changeFile(t, m, func(c *config.Config) error {
		c.Spaces = append(c.Spaces, config.Space{ID: "beta", Name: "Beta", Agent: config.NewAgent("beta"), Schedule: "always"})
		if err := c.SetSpaces("work", "D1", "@bob", []config.Link{{Space: "acme", Read: config.ReadAll}, {Space: "beta", Read: config.ReadAll}}); err != nil {
			return err
		}
		return nil
	})
	refresh(m)
	// @bob (first) is in Acme and Beta; # release is in Acme only.
	d := m.newSpacesDialog(m.rowsWhere(func(r convRow) bool { return r.ID == "D1" || r.ID == "C1" }))
	if d.rows[0].ID != "D1" {
		t.Fatalf("first row %s, want D1", d.rows[0].ID)
	}
	if !d.added() {
		t.Error("the dialog does not offer the old messages, but enter adds Beta to # release")
	}
}

func TestANewSpaceCanBeUndone(t *testing.T) {
	m := newTestModel(t)
	m.spc = len(m.railItems()) - 1 // + New space
	send(m, "enter")
	typeText(m, "Gamma")
	send(m, "enter")
	if _, ok := loadFile(t, m).Space("gamma"); !ok {
		t.Fatal("no space gamma")
	}
	send(m, "esc", "u")
	if _, ok := loadFile(t, m).Space("gamma"); ok {
		t.Errorf("u after a new space: %q, the space stays", m.status)
	}
}
