package tui

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/sondt2709/inbox2agent/internal/app"
	"github.com/sondt2709/inbox2agent/internal/config"
	"github.com/sondt2709/inbox2agent/internal/inbox"
	"github.com/sondt2709/inbox2agent/internal/notify"
	"github.com/sondt2709/inbox2agent/internal/secrets"
	"github.com/sondt2709/inbox2agent/internal/service"
	"github.com/sondt2709/inbox2agent/internal/store"
)

type feedHarness struct {
	m      *model
	st     *store.Store
	copied []string
	opened []string
}

// newFeedModel returns a dashboard on the Feed tab with a real store that has
// 2 source messages and 3 cards: a high action, a normal question and a low info card.
func newFeedModel(t *testing.T) *feedHarness {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("I2A_HOME", dir)
	t.Setenv("I2A_SECRET_BACKEND", "file")
	paths, err := app.ResolvePaths()
	if err != nil {
		t.Fatal(err)
	}
	sec, err := secrets.Open(paths.SecretsFile)
	if err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(paths.DBFile)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	ctx := context.Background()
	t0 := time.Now().Add(-time.Hour)
	msgs := []inbox.Message{
		{SourceID: "work", SourceType: "mattermost", ExternalID: "p1", ChannelName: "@lena.wong", ChannelKind: inbox.KindDirect,
			Author: "lena.wong", Text: "Can PF get their own login?", URL: "https://chat.example.com/harbor/pl/p1", CreatedAt: t0},
		{SourceID: "work", SourceType: "mattermost", ExternalID: "p2", ChannelName: "pr-reviews", ChannelKind: inbox.KindChannel,
			Author: "minh.tran", Text: "PR #57", URL: "javascript:alert(1)", CreatedAt: t0},
	}
	if _, err := st.SavePoll(ctx, "work", msgs, nil, t0); err != nil {
		t.Fatal(err)
	}
	p, _ := st.Pending(ctx)
	cards := []struct {
		c  inbox.Card
		at time.Time
	}{
		{inbox.Card{Category: inbox.CategoryInfo, Urgency: inbox.UrgencyLow, Group: "Riverside CRM", Title: "Staging deployed 2.14.0"}, t0.Add(30 * time.Minute)},
		{inbox.Card{Category: inbox.CategoryAction, Urgency: inbox.UrgencyHigh, Group: "Harbor Portal", Title: "Correct your reply to Lena",
			Body: "admin:create exists.", Reply: "I checked the code: I was wrong.", Refs: []int64{p[0].ID}, Key: "mm:lena"}, t0.Add(10 * time.Minute)},
		{inbox.Card{Category: inbox.CategoryQuestion, Urgency: inbox.UrgencyNormal, Group: "Harbor Portal", Title: "Review PR #57",
			Body: "See https://evil.example", Refs: []int64{p[1].ID}}, t0.Add(20 * time.Minute)},
	}
	for _, c := range cards {
		if _, err := st.SaveCards(ctx, []inbox.Card{c.c}, c.at); err != nil {
			t.Fatal(err)
		}
	}
	h := &feedHarness{st: st}
	u := newUI(Deps{Paths: paths, Config: config.Default(), Secrets: sec, Store: st, Tab: TabFeed,
		Copy: func(s string) error { h.copied = append(h.copied, s); return nil },
		Open: func(s string) error { h.opened = append(h.opened, s); return nil },
		ServiceStatus: func() (service.State, error) {
			return service.State{}, nil
		}})
	h.m = newModel(ctx, u)
	h.m.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	h.m.Update(h.m.load()())
	return h
}

func (h *feedHarness) state(t *testing.T, title string) string {
	t.Helper()
	all, err := h.st.Cards(context.Background(), true)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range all {
		if c.Title == title {
			return c.State
		}
	}
	t.Fatalf("no card %q", title)
	return ""
}

func titles(m *model) string {
	var out []string
	for _, c := range m.visibleCards() {
		out = append(out, c.Title)
	}
	return strings.Join(out, " | ")
}

func TestFeedShowsCardsByUrgency(t *testing.T) {
	h := newFeedModel(t)
	s := screen(h.m)
	for _, want := range []string{"Feed 3", "Cards · 3 · urgency", "Correct your reply to Lena",
		"Suggested reply", "I checked the code: I was wrong.", "Can PF get their own login?"} {
		if !strings.Contains(s, want) {
			t.Errorf("the Feed does not show %q", want)
		}
	}
	if got := titles(h.m); got != "Correct your reply to Lena | Review PR #57 | Staging deployed 2.14.0" {
		t.Errorf("urgency order = %s", got)
	}
	// The card that is selected when the tab opens stays new.
	if st := h.state(t, "Correct your reply to Lena"); st != inbox.CardNew {
		t.Errorf("state at open = %s, want new", st)
	}
}

func TestFeedSortAndFilter(t *testing.T) {
	h := newFeedModel(t)
	send(h.m, "s")
	if got := titles(h.m); got != "Staging deployed 2.14.0 | Review PR #57 | Correct your reply to Lena" {
		t.Errorf("newest order = %s", got)
	}
	if !strings.Contains(screen(h.m), "Cards · 3 · newest") {
		t.Error("the Feed does not show the newest sort")
	}
	send(h.m, "f") // no category filter any more
	if got := titles(h.m); strings.Count(got, "|") != 2 {
		t.Errorf("f changed the cards: %s", got)
	}
}

func TestFeedReadRuleCopyOpenDone(t *testing.T) {
	h := newFeedModel(t)
	send(h.m, "c")
	if len(h.copied) != 1 || h.copied[0] != "I checked the code: I was wrong." {
		t.Errorf("copied = %q", h.copied)
	}
	if st := h.state(t, "Correct your reply to Lena"); st != inbox.CardRead {
		t.Errorf("state after c = %s, want read", st)
	}
	send(h.m, "o")
	if len(h.opened) != 1 || h.opened[0] != "https://chat.example.com/harbor/pl/p1" {
		t.Errorf("opened = %q", h.opened)
	}

	// Moving to a card makes it read. The question refers to a javascript: URL
	// and its body has a link: o opens nothing.
	send(h.m, "down")
	if st := h.state(t, "Review PR #57"); st != inbox.CardRead {
		t.Errorf("state after down = %s, want read", st)
	}
	send(h.m, "o")
	if len(h.opened) != 1 {
		t.Errorf("o opened a link that is not a stored http(s) message URL: %q", h.opened)
	}
	if !strings.Contains(screen(h.m), "This card has no message link.") {
		t.Error("the status does not say that the card has no link")
	}

	send(h.m, "u")
	if st := h.state(t, "Review PR #57"); st != inbox.CardNew {
		t.Errorf("state after u = %s, want new", st)
	}
	send(h.m, "d")
	if st := h.state(t, "Review PR #57"); st != inbox.CardDone {
		t.Errorf("state after d = %s, want done", st)
	}
	if strings.Contains(titles(h.m), "Review PR #57") {
		t.Error("the done card is still in the list")
	}
	send(h.m, "a")
	if !strings.Contains(titles(h.m), "Review PR #57") {
		t.Error("a does not show the done card")
	}
}

func TestFeedShowsDeletedMessageAndNewCards(t *testing.T) {
	h := newFeedModel(t)
	ctx := context.Background()
	if _, err := h.st.SaveCards(ctx, []inbox.Card{{Category: inbox.CategoryAction, Urgency: inbox.UrgencyHigh, Title: "Answer the auditor",
		Refs: []int64{999}}}, time.Now()); err != nil {
		t.Fatal(err)
	}
	h.m.Update(h.m.load()()) // the next tick
	if got := titles(h.m); !strings.HasPrefix(got, "Answer the auditor") {
		t.Fatalf("the new card is not first: %s", got)
	}
	// The selection stays on the card that the user selected.
	if c, _ := h.m.selectedCard(); c.Title != "Correct your reply to Lena" {
		t.Errorf("selection moved to %q", c.Title)
	}
	send(h.m, "up")
	if s := screen(h.m); !strings.Contains(s, "#999 message deleted") || !strings.Contains(s, "Feed 3") {
		t.Errorf("screen:\n%s", s)
	}
}

func TestFeedWithoutStore(t *testing.T) {
	m := newTestModel(t)
	m.u.d.Store = nil
	m.Update(m.load()())
	at(m, tabFeed)
	if !strings.Contains(screen(m), "No cards yet.") {
		t.Error("the Feed without a store does not say No cards yet")
	}
}

// A click on the second line of the third card selects that card and makes it read.
func TestFeedMouseClickSelectsCard(t *testing.T) {
	h := newFeedModel(t)
	lines := strings.Split(screen(h.m), "\n")
	y := -1
	for i, l := range lines {
		if strings.Contains(l, "Staging deployed") {
			y = i + 1 // the second line of the card
		}
	}
	if y < 0 {
		t.Fatal("the screen has no third card")
	}
	_, cmd := h.m.Update(tea.MouseClickMsg{X: feedRailWidth(140) + 10, Y: y, Button: tea.MouseLeft})
	run(h.m, cmd, 0)
	if c, _ := h.m.selectedCard(); c.Title != "Staging deployed 2.14.0" {
		t.Fatalf("click at y %d selected %q:\n%s", y, c.Title, screen(h.m))
	}
	if st := h.state(t, "Staging deployed 2.14.0"); st != inbox.CardRead {
		t.Errorf("state after the click = %s, want read", st)
	}
}

// The cards of the harness refer to messages that no dispatch sent: their
// space is the space of their conversation, Unsorted. F filters by space.
func TestFeedShowsAndFiltersTheSpace(t *testing.T) {
	h := newFeedModel(t)
	if s := screen(h.m); !strings.Contains(s, "space Everything else") || !strings.Contains(s, "Everything else · Harbor Portal") {
		t.Fatalf("the Feed does not show the space:\n%s", s)
	}
	pickSpace(t, h.m, "Everything else")
	if got := titles(h.m); got != "Correct your reply to Lena | Review PR #57" {
		t.Errorf("cards of Unsorted = %q; the info card has no message", got)
	}
	if !strings.Contains(screen(h.m), "▌Everything else") {
		t.Error("the space list does not mark the space filter")
	}
	pickSpace(t, h.m, "All spaces")
	if got := titles(h.m); !strings.Contains(got, "Staging deployed 2.14.0") {
		t.Errorf("All spaces did not show all cards: %q", got)
	}
}

// The rows, the Card panel and the status line show the title with no Markdown.
func TestFeedShowsPlainTitle(t *testing.T) {
	h := newFeedModel(t)
	md := inbox.Card{Category: inbox.CategoryAction, Urgency: inbox.UrgencyHigh, Title: "**Check** the `notes` on [prod](https://example.com)", Key: "md"}
	if _, err := h.st.SaveCards(context.Background(), []inbox.Card{md}, time.Now()); err != nil {
		t.Fatal(err)
	}
	h.m.Update(h.m.load()())
	for range 4 {
		if c, ok := h.m.selectedCard(); ok && c.Key == "md" {
			break
		}
		send(h.m, "up")
	}
	s := screen(h.m)
	if n := strings.Count(s, "Check the notes on prod"); n < 2 {
		t.Errorf("the row and the Card panel show the plain title %d times, want 2:\n%s", n, s)
	}
	for _, raw := range []string{"**Check**", "`notes`", "](https://example.com)"} {
		if strings.Contains(s, raw) {
			t.Errorf("the Feed shows the Markdown %q", raw)
		}
	}
	send(h.m, "d")
	if s := screen(h.m); !strings.Contains(s, "Done: Check the notes on prod") {
		t.Errorf("the status line does not show the plain title:\n%s", s)
	}
}

// rawTexts runs a command and returns the texts of its tea.RawMsg values.
func rawTexts(cmd tea.Cmd) []string {
	if cmd == nil {
		return nil
	}
	var out []string
	var walk func(msg tea.Msg)
	walk = func(msg tea.Msg) {
		switch v := msg.(type) {
		case tea.RawMsg:
			out = append(out, v.Msg.(string))
		case tea.BatchMsg:
			for _, c := range v {
				if c != nil {
					walk(c())
				}
			}
		default:
			// tea.Sequence returns an unexported message: a slice of commands.
			rv := reflect.ValueOf(msg)
			if rv.Kind() == reflect.Slice {
				for i := range rv.Len() {
					if c, ok := rv.Index(i).Interface().(tea.Cmd); ok && c != nil {
						walk(c())
					}
				}
			}
		}
	}
	walk(cmd())
	return out
}

// In iTerm2 the Feed shows a note for a new card, and none for the first load
// or for done cards that the done filter shows.
func TestFeedShowsCardNotesInTheTerminal(t *testing.T) {
	h := newFeedModel(t)
	h.m.u.d.FeedNotes = func(n notify.Note) string { return notify.OSC9(notify.OSC9Text(n), false) }
	reload := func() []string {
		_, cmd := h.m.Update(h.m.load()())
		return rawTexts(cmd)
	}
	if got := reload(); len(got) != 0 {
		t.Fatalf("a load with no new card gave notes: %q", got)
	}
	md := inbox.Card{Category: inbox.CategoryAction, Urgency: inbox.UrgencyHigh, Group: "release", Title: "**Fix** the `deploy`", Key: "md"}
	if _, err := h.st.SaveCards(context.Background(), []inbox.Card{md}, time.Now()); err != nil {
		t.Fatal(err)
	}
	if got, want := reload(), "\x1b]9;ACTION · high · release: Fix the deploy\x07"; len(got) != 1 || got[0] != want {
		t.Errorf("notes = %q, want %q", got, want)
	}
	if got := reload(); len(got) != 0 {
		t.Errorf("the same cards gave notes again: %q", got)
	}
	send(h.m, "a") // show the done cards
	if got := reload(); len(got) != 0 {
		t.Errorf("the done filter gave notes: %q", got)
	}
}

func TestCardPanelShowsTheTerminal(t *testing.T) {
	h := newFeedModel(t)
	const handle = "term_5e141e6e-a22f-461e-ad39-d8d53b147ad1"
	c := inbox.Card{Category: inbox.CategoryAction, Urgency: inbox.UrgencyHigh, Title: "From an agent", Key: "term",
		Handle: handle, AgentSession: "04ebe19c-543e-42ff-8f84-ea13cea7a3ec"}
	if _, err := h.st.SaveCards(context.Background(), []inbox.Card{c}, time.Now()); err != nil {
		t.Fatal(err)
	}
	h.m.Update(h.m.load()())
	for range 4 {
		if c, ok := h.m.selectedCard(); ok && c.Key == "term" {
			break
		}
		send(h.m, "up")
	}
	// The Card panel is 66 columns wide at 140: the Claude session goes to
	// the next line, and the handle stays whole.
	if s := screen(h.m); !strings.Contains(s, "terminal "+handle) || !strings.Contains(s, "Claude 04ebe19c") {
		t.Errorf("the Card panel has no terminal line:\n%s", s)
	}
	h.m.Update(tea.WindowSizeMsg{Width: 200, Height: 40})
	if s := screen(h.m); !strings.Contains(s, "terminal "+handle+" · Claude 04ebe19c") {
		t.Errorf("a wide Card panel does not show the terminal and the session on one line:\n%s", s)
	}
	h.m.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	// A card with no terminal has no line.
	send(h.m, "down")
	if s := screen(h.m); strings.Contains(s, "terminal term_") {
		t.Errorf("a card with no terminal shows a terminal line:\n%s", s)
	}
}
