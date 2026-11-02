package tui

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/sondt2709/inbox2agent/internal/inbox"
)

// longCard adds a card with 80 lines of body; it is the newest high card, so
// it is first in the list.
func longCard(t *testing.T, h *feedHarness) {
	t.Helper()
	var b strings.Builder
	for i := 1; i <= 80; i++ {
		fmt.Fprintf(&b, "line %d of the body\n\n", i)
	}
	// Two long cards: "Long notes" first, "Long notes 2" second.
	if _, err := h.st.SaveCards(context.Background(), []inbox.Card{{Category: inbox.CategoryAction, Urgency: inbox.UrgencyHigh,
		Group: "release", Title: "Long notes 2", Body: b.String(), Key: "long2"}}, time.Now().Add(-time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, err := h.st.SaveCards(context.Background(), []inbox.Card{{Category: inbox.CategoryAction, Urgency: inbox.UrgencyHigh,
		Group: "release", Title: "Long notes", Body: b.String(), Key: "long1"}}, time.Now()); err != nil {
		t.Fatal(err)
	}
	h.m.Update(readFeed(context.Background(), h.st, false))
	h.m.feedSel = 0 // select the long card
	h.m.feedSelID = h.m.visibleCards()[0].ID
}

// accentBorders reports, for each panel from the left, whether its top left
// corner has the accent color.
func accentBorders(m *model) []bool {
	top := strings.Split(m.render(), "\n")[contentTop]
	var out []bool
	color := ""
	for i := 0; i < len(top); {
		if strings.HasPrefix(top[i:], "\x1b[") {
			end := strings.IndexByte(top[i:], 'm')
			color = top[i : i+end+1]
			i += end + 1
			continue
		}
		if strings.HasPrefix(top[i:], "╭") {
			out = append(out, strings.Contains(color, "125;86;244"))
		}
		i++
	}
	return out
}

// firstBorderIsAccent reports whether the left panel (the space list) has
// the accent border: the first color of the top border row is the accent.
func firstBorderIsAccent(m *model) bool {
	top := strings.Split(m.render(), "\n")[contentTop]
	at := strings.Index(top, "\x1b[38;2;")
	return at >= 0 && strings.HasPrefix(top[at:], "\x1b[38;2;125;86;244")
}

func TestFeedPanelsTakeTheKeyboardWithTheArrows(t *testing.T) {
	h := newFeedModel(t)
	if h.m.feedFocus != focusCards {
		t.Fatalf("the Feed starts with focus %d, want Cards", h.m.feedFocus)
	}
	send(h.m, "right")
	if h.m.feedFocus != focusCard || !strings.Contains(lineWith(screen(h.m), "Card 2"), "Card 2") {
		t.Errorf("→: focus %d, want Card", h.m.feedFocus)
	}
	send(h.m, "right")
	if h.m.feedFocus != focusCard {
		t.Errorf("→ on the last panel: focus %d", h.m.feedFocus)
	}
	send(h.m, "left", "left", "left")
	if h.m.feedFocus != focusSpaces {
		t.Errorf("← three times: focus %d, want Spaces", h.m.feedFocus)
	}
	// The focused panel has the accent border.
	if !firstBorderIsAccent(h.m) {
		t.Error("the space list has the keyboard but not the accent border")
	}
	send(h.m, "right")
	if got := accentBorders(h.m); len(got) != 3 || got[0] || !got[1] || got[2] {
		t.Errorf("accent borders with Cards focused = %v, want only Cards", got)
	}
	send(h.m, "right")
	if got := accentBorders(h.m); len(got) != 3 || got[0] || got[1] || !got[2] {
		t.Errorf("accent borders with Card focused = %v, want only Card", got)
	}
	h.m.Update(tea.WindowSizeMsg{Width: 90, Height: 40})
	send(h.m, "left")
	if h.m.feedFocus != focusCards {
		t.Errorf("below 100 columns ← goes to the hidden space list: focus %d", h.m.feedFocus)
	}
}

func TestCardPanelScrollsAndStartsAtTheTopForAnotherCard(t *testing.T) {
	h := newFeedModel(t)
	longCard(t, h)
	if c, _ := h.m.selectedCard(); c.Title != "Long notes" {
		t.Fatalf("selected %q", c.Title)
	}
	if !strings.Contains(screen(h.m), "· ↓ more") || !strings.Contains(screen(h.m), "line 1 of the body") {
		t.Fatalf("the long card shows no ↓ more:\n%s", screen(h.m))
	}
	send(h.m, "right", "down", "down", "down", "down", "down")
	s := screen(h.m)
	if h.m.cardOff != 5 || !strings.Contains(s, "· ↑ · ↓ more") || strings.Contains(s, "Long notes\n") {
		t.Errorf("↓ five times in Card: offset %d:\n%s", h.m.cardOff, s)
	}
	if c, _ := h.m.selectedCard(); c.Title != "Long notes" {
		t.Errorf("↓ in Card moved the selection to %q", c.Title)
	}
	send(h.m, "pgdown")
	if h.m.cardOff <= 5 {
		t.Errorf("pgdown: offset %d", h.m.cardOff)
	}
	send(h.m, "left", "down") // the next card, also long
	if c, _ := h.m.selectedCard(); c.Title != "Long notes 2" {
		t.Fatalf("the next card is %q", c.Title)
	}
	screen(h.m)
	if h.m.cardOff != 0 {
		t.Errorf("the next card starts at line %d, want 0", h.m.cardOff)
	}
}

func TestWheelActsOnThePanelUnderThePointer(t *testing.T) {
	h := newFeedModel(t)
	longCard(t, h)
	screen(h.m)
	x := feedRailWidth(140) + feedListWidth(140-feedRailWidth(140)) + 5 // over the Card panel
	h.m.Update(tea.MouseWheelMsg{X: x, Y: 10, Button: tea.MouseWheelDown})
	if h.m.feedFocus != focusCard || h.m.cardOff != 1 {
		t.Errorf("wheel over Card: focus %d, offset %d", h.m.feedFocus, h.m.cardOff)
	}
	if c, _ := h.m.selectedCard(); c.Title != "Long notes" {
		t.Errorf("the wheel over Card moved the selection to %q", c.Title)
	}
	h.m.Update(tea.MouseWheelMsg{X: feedRailWidth(140) + 5, Y: 10, Button: tea.MouseWheelDown})
	if c, _ := h.m.selectedCard(); h.m.feedFocus != focusCards || c.Title == "Long notes" {
		t.Errorf("wheel over Cards: focus %d, selected %q", h.m.feedFocus, c.Title)
	}
	h.m.Update(tea.MouseWheelMsg{X: 3, Y: 10, Button: tea.MouseWheelDown})
	if h.m.feedFocus != focusSpaces || h.m.activeSpace() == "" {
		t.Errorf("wheel over Spaces: focus %d, space %q", h.m.feedFocus, h.m.activeSpace())
	}
	// The Spaces area: the wheel over the panel moves its rows.
	m := newTestModel(t)
	m.Update(tea.MouseWheelMsg{X: listWidth(120) + 10, Y: 10, Button: tea.MouseWheelUp})
	if !m.spcFocus {
		t.Error("the wheel over the panel of a space does not give it the keyboard")
	}
	m.Update(tea.MouseWheelMsg{X: 3, Y: 10, Button: tea.MouseWheelDown})
	if m.spcFocus || m.spc != 1 {
		t.Errorf("the wheel over the list: focus %v, row %d", m.spcFocus, m.spc)
	}
}

func TestCardRowsStartAtTheLeftEdge(t *testing.T) {
	h := newFeedModel(t)
	send(h.m, "down") // select the second card; the first row is not selected
	var row1, row2 string
	ls := strings.Split(screen(h.m), "\n")
	for i, l := range ls {
		if strings.Contains(l, "Correct your reply to Lena") && strings.Contains(l, "ACTION") {
			row1, row2 = l, ls[i+1]
		}
	}
	list := strings.Index(row1, "││") + len("││")
	if list < 2 || !strings.HasPrefix(row1[list:], " ACTION   ▰▰▰ ") {
		t.Errorf("line 1 does not start with the category at the left edge: %q", row1)
	}
	if !strings.Contains(row1, "▰▰▰ ● Correct your reply") {
		t.Errorf("a new card has no ● before its title: %q", row1)
	}
	if l2 := strings.Index(row2, "││") + len("││"); !strings.HasPrefix(row2[l2:], " Everything else · Harbor Portal · ") {
		t.Errorf("line 2 is not space · group: %q", row2)
	}
	// The group is bold, the category has no badge color.
	raw := h.m.cardRow(h.m.visibleCards()[0], false, 60)
	if !strings.Contains(raw[1], boldStyle.Render("Harbor Portal")) || strings.Contains(raw[0], catBadge(inbox.CategoryAction)) {
		t.Errorf("styles of the row: %q", raw)
	}
}

func TestCtrlRRunsAndRDoesNot(t *testing.T) {
	h := newFeedModel(t)
	f := &fakeRun{}
	h.m.u.d.RunNow = f.run
	send(h.m, "R")
	if len(f.calls) != 0 {
		t.Fatalf("R started a run: %v", f.calls)
	}
	send(h.m, "ctrl+r")
	if len(f.calls) != 1 {
		t.Errorf("ctrl+r: runs = %v", f.calls)
	}
	if !strings.Contains(lines(h.m)[0], "ctrl+r run now") {
		t.Errorf("the header has no ctrl+r: %q", lines(h.m)[0])
	}
}

func TestAVeryShortWindowDoesNotCrash(t *testing.T) {
	h := newFeedModel(t)
	for _, rows := range []int{1, 2, 3, 4, 5, 6} {
		h.m.Update(tea.WindowSizeMsg{Width: 120, Height: rows})
		screen(h.m) // no panic
	}
}

func TestAnOpenFormKeepsTheSpaceAndTheFocus(t *testing.T) {
	h := newFeedModel(t)
	withAcme(t, h)
	send(h.m, "r")
	if h.m.replyCard == 0 || !h.m.form.active {
		t.Fatal("r did not open the reply form")
	}
	if got := accentBorders(h.m); len(got) != 3 || got[0] || got[1] || !got[2] {
		t.Errorf("accent borders with the reply form = %v, want only Card", got)
	}
	// The form gives back the keyboard (a click on the list), then ← ↓ in Spaces.
	h.m.form.deactivate()
	h.m.feedFocus = focusSpaces
	before := h.m.activeSpace()
	send(h.m, "down")
	if h.m.activeSpace() != before || h.m.replyCard == 0 || !strings.Contains(h.m.status, "Send or cancel the open form first") {
		t.Errorf("↓ in Spaces with an open reply: space %q→%q, reply %d, status %q", before, h.m.activeSpace(), h.m.replyCard, h.m.status)
	}
	// → to the Card panel gives the keyboard back to the form.
	send(h.m, "right", "right")
	if !h.m.form.active || h.m.feedFocus != focusCard {
		t.Errorf("→ to Card with an open form: active %v, focus %d", h.m.form.active, h.m.feedFocus)
	}
}

func TestTheWheelBuildsTheFormOfTheNewCard(t *testing.T) {
	h := newFeedModel(t)
	withAcme(t, h)
	send(h.m, "r")
	if h.m.form == nil {
		t.Fatalf("r did not open the reply form: %q", h.m.status)
	}
	h.m.form.deactivate()
	h.m.feedFocus = focusCards
	screen(h.m)
	h.m.Update(tea.MouseWheelMsg{X: feedRailWidth(140) + 5, Y: 10, Button: tea.MouseWheelDown})
	if c, _ := h.m.selectedCard(); c.ID == h.m.replyCard || (h.m.form != nil && h.m.replyCard != 0) {
		t.Errorf("after the wheel the reply form of the old card stays: selected %d, reply %d, form %v", c.ID, h.m.replyCard, h.m.form != nil)
	}
	h.m.Update(tea.MouseWheelMsg{X: feedRailWidth(140) + 5, Y: 0, Button: tea.MouseWheelDown})
	h.m.Update(tea.MouseWheelMsg{X: feedRailWidth(140) + 5, Y: 39, Button: tea.MouseWheelDown})
	if h.m.feedSel != 1 {
		t.Errorf("the wheel over the header or the footer moved the selection to %d", h.m.feedSel)
	}
}

func TestASpaceChangeSelectsItsFirstCardAndStays(t *testing.T) {
	h := newFeedModel(t)
	withAcme(t, h)
	send(h.m, "left", "down") // Acme
	h.m.Update(readFeed(context.Background(), h.st, false))
	if c, _ := h.m.selectedCard(); c.Title != "Acme deploy" {
		t.Errorf("after a refresh the selection is %q, want the first card of Acme", c.Title)
	}
	send(h.m, "up") // All: its first card, also after a refresh
	first := h.m.visibleCards()[0].Title
	h.m.Update(readFeed(context.Background(), h.st, false))
	if c, _ := h.m.selectedCard(); c.Title != first {
		t.Errorf("after a refresh in All the selection is %q, want the first card %q", c.Title, first)
	}
	// A click on a row of the space list, also after the list scrolled.
	h.m.railOff = 1
	screen(h.m)
	h.m.railOff = 1
	h.m.clickFeed(3, 2) // the first visible row: the second space row (Acme)
	if got := h.m.activeSpace(); got != h.m.feedSpaces()[h.m.railOff] {
		t.Errorf("a click on the first visible row picked %q", got)
	}
}

func TestAClosedFormGivesTheKeyboardBackToTheCards(t *testing.T) {
	h := newFeedModel(t)
	withAcme(t, h)
	send(h.m, "r")
	if h.m.feedFocus != focusCard {
		t.Fatalf("r: focus %d, want Card", h.m.feedFocus)
	}
	send(h.m, "esc")
	if h.m.replyCard != 0 || h.m.feedFocus != focusCards {
		t.Errorf("esc on the reply: reply %d, focus %d, want Cards", h.m.replyCard, h.m.feedFocus)
	}
}
