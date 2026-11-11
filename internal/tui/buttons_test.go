package tui

import (
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/sondt2709/inbox2agent/internal/config"
)

// clickOn clicks the middle of the first text on the screen, from the top or
// from the bottom, and runs the commands that come back. It fails the test
// when the screen does not show the text.
func clickOn(t *testing.T, m *model, text string, fromBottom bool) {
	t.Helper()
	lines := strings.Split(screen(m), "\n")
	order := make([]int, len(lines))
	for i := range order {
		order[i] = i
	}
	if fromBottom {
		slices.Reverse(order)
	}
	for _, y := range order {
		if i := strings.Index(lines[y], text); i >= 0 {
			x := ansi.StringWidth(lines[y][:i]) + ansi.StringWidth(text)/2
			_, cmd := m.Update(tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft})
			run(m, cmd, 0)
			return
		}
	}
	t.Fatalf("the screen has no %q:\n%s", text, strings.Join(lines, "\n"))
}

func TestFooterZonesMatchTheText(t *testing.T) {
	m := newTestModel(t)
	for _, tab := range []tabID{tabFeed, tabSpaces, tabAccounts, tabSchedules, tabSettings, tabActivity} {
		at(m, tab)
		lines := strings.Split(screen(m), "\n")
		footer := []rune(lines[len(lines)-1])
		n := 0
		for _, z := range m.barZones {
			if z.y != m.h-1 {
				continue
			}
			n++
			got := strings.TrimSpace(string(footer[z.x0:z.x1]))
			ok := strings.HasSuffix(got, " "+z.key) || got == "More…" ||
				(z.key == "left" && strings.HasSuffix(got, "←")) || (z.key == "right" && strings.HasSuffix(got, "→"))
			if !ok {
				t.Errorf("tab %d: the zone of %q covers %q", tab, z.key, got)
			}
		}
		if n < 4 { // at least one hint, then Find, Keys, Quit
			t.Errorf("tab %d: %d footer buttons:\n%s", tab, n, string(footer))
		}
	}
}

func TestClickFooterButtonRunsItsKey(t *testing.T) {
	m := newTestModel(t) // the Spaces area, Acme selected
	clickOn(t, m, "Add conversations a", true)
	if d, ok := m.topDialog().(*searchDialog); !ok || d.space != "acme" {
		t.Fatalf("a click on Add conversations a opens %v, want the add dialog of Acme", m.topDialog())
	}
	// The footer of the dialog: Cancel esc closes it.
	clickOn(t, m, "Clear or cancel esc", true)
	if m.topDialog() != nil {
		t.Fatalf("a click on the esc button of the dialog left %v open", m.topDialog())
	}
	// Find / opens the search of all conversations.
	clickOn(t, m, "Find /", true)
	if d, ok := m.topDialog().(*searchDialog); !ok || d.space != "" {
		t.Fatalf("a click on Find / opens %v, want the search", m.topDialog())
	}
}

func TestClickCancelOfAConfirmation(t *testing.T) {
	m := newTestModel(t)
	send(m, "x") // delete Acme?
	if _, ok := m.topDialog().(*confirmDialog); !ok {
		t.Fatalf("x opens %v, want a confirmation", m.topDialog())
	}
	clickOn(t, m, "Cancel esc", true)
	if m.topDialog() != nil || len(loadFile(t, m).Spaces) != 1 {
		t.Fatalf("a click on Cancel esc: dialog %v, spaces %d, want closed and Acme kept", m.topDialog(), len(loadFile(t, m).Spaces))
	}
}

func TestHeaderRunNowButton(t *testing.T) {
	h := newFeedModel(t)
	f := &fakeRun{}
	h.m.u.d.RunNow = f.run
	clickOn(t, h.m, "Run now ctrl+r", false)
	if len(f.calls) != 1 {
		t.Fatalf("a click on Run now ctrl+r: runs = %v, want 1", f.calls)
	}
}

func TestMoreListsTheActionsThatDoNotFit(t *testing.T) {
	h := newFeedModel(t)
	h.m.Update(tea.WindowSizeMsg{Width: 130, Height: 34})
	lines := strings.Split(screen(h.m), "\n")
	if footer := lines[len(lines)-1]; !strings.Contains(footer, "More…") || strings.Contains(footer, "Sort s") {
		t.Fatalf("at 130 columns the footer has no More… or still has Sort s:\n%s", footer)
	}
	clickOn(t, h.m, "More…", true)
	d, ok := h.m.topDialog().(*pickDialog)
	if !ok || d.name != "Actions" {
		t.Fatalf("More… opens %v, want the Actions list", h.m.topDialog())
	}
	var labels []string
	for _, o := range d.options {
		labels = append(labels, o.label)
	}
	for _, want := range []string{"Reply", "Sort", "Done cards", "Space list"} {
		if !slices.Contains(labels, want) {
			t.Errorf("the Actions list has no %q: %v", want, labels)
		}
	}
	sort := h.m.feedSort
	clickOn(t, h.m, "Sort", false)
	if h.m.topDialog() != nil || h.m.feedSort == sort {
		t.Fatalf("a click on Sort: dialog %v, sort %q, want closed and another sort than %q", h.m.topDialog(), h.m.feedSort, sort)
	}
}

func TestAddConversationsRow(t *testing.T) {
	m := newTestModel(t)
	if s := screen(m); !strings.Contains(s, "+ Add conversations") {
		t.Fatalf("the panel of Acme has no + Add conversations:\n%s", s)
	}
	clickOn(t, m, "+ Add conversations", false)
	if d, ok := m.topDialog().(*searchDialog); !ok || d.space != "acme" {
		t.Fatalf("one click on + Add conversations opens %v, want the add dialog of Acme", m.topDialog())
	}
	send(m, "esc", "esc", "esc")
	// The keyboard: the panel starts on # release; ↑ is the row, enter opens it.
	send(m, "enter", "up", "enter")
	if d, ok := m.topDialog().(*searchDialog); !ok || d.space != "acme" {
		t.Fatalf("enter on + Add conversations opens %v, want the add dialog of Acme", m.topDialog())
	}
}

func TestAddWithTheMouseOnly(t *testing.T) {
	m := newTestModel(t)
	routes := func() []string { return loadFile(t, m).Routes("work", "D1") }
	clickOn(t, m, "+ Add conversations", false)
	typeText(m, "bob")
	clickOn(t, m, "@ @bob", false)
	if r := routes(); !slices.Equal(r, []string{config.UnsortedID}) {
		t.Fatalf("a click on a row saved at once: %v", r)
	}
	if s := screen(m); !strings.Contains(s, "[x]") {
		t.Fatalf("a click on the row did not mark it:\n%s", s)
	}
	clickOn(t, m, "OK", false)
	if r := routes(); m.topDialog() != nil || !slices.Equal(r, []string{"acme"}) {
		t.Fatalf("OK: dialog %v, routes %v, want acme", m.topDialog(), r)
	}
}

func TestSearchAndSpacesDialogRowClicks(t *testing.T) {
	m := newTestModel(t)
	send(m, "/")
	typeText(m, "bob")
	clickOn(t, m, "@ @bob", false)
	if _, ok := m.topDialog().(*searchDialog); !ok {
		t.Fatalf("the first click on a row of the search opens %v, want the search", m.topDialog())
	}
	clickOn(t, m, "@ @bob", false)
	if _, ok := m.topDialog().(*spacesDialog); !ok {
		t.Fatalf("a click on the selected row opens %v, want the spaces dialog", m.topDialog())
	}
	clickOn(t, m, "[ ] Acme", false) // the row Acme of the spaces dialog
	send(m, "enter")
	if r := loadFile(t, m).Routes("work", "D1"); !slices.Equal(r, []string{"acme"}) {
		t.Fatalf("a click on Acme, then enter: routes %v, want acme", r)
	}
}

func TestHeaderHasNoRunButtonWhileARunIsBusy(t *testing.T) {
	h := newFeedModel(t)
	h.m.running = true
	head := strings.Split(screen(h.m), "\n")[0]
	if strings.Contains(head, "Run now ctrl+r") || !strings.Contains(head, "running") {
		t.Fatalf("a busy run: header %q, want the spinner and no button", head)
	}
	for _, z := range h.m.barZones {
		if z.y == 0 {
			t.Fatalf("a busy run: the header has the button zone %+v", z)
		}
	}
}
