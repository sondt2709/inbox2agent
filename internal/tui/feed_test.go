package tui

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/sondt2709/inbox2agent/internal/app"
	"github.com/sondt2709/inbox2agent/internal/config"
	"github.com/sondt2709/inbox2agent/internal/inbox"
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
	for _, want := range []string{"1 Feed", "3 new · 1 action · 1 question", "sort: urgency", "Correct your reply to Lena",
		"Suggested reply", "I checked the code: I was wrong.", "Can PF get their own login?", "All 3", "Action 1"} {
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
	if !strings.Contains(screen(h.m), "sort: newest") {
		t.Error("the Feed does not show the newest sort")
	}
	send(h.m, "s", "f", "f")
	if got := titles(h.m); got != "Review PR #57" {
		t.Errorf("question filter = %s", got)
	}
	send(h.m, "f", "f")
	if got := titles(h.m); strings.Count(got, "|") != 2 {
		t.Errorf("all filter = %s", got)
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
	if s := screen(h.m); !strings.Contains(s, "#999 message deleted") || !strings.Contains(s, "3 new") {
		t.Errorf("screen:\n%s", s)
	}
}

func TestFeedWithoutStore(t *testing.T) {
	m := newTestModel(t)
	m.u.d.Store = nil
	m.Update(m.load()())
	send(m, "1")
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
		if strings.Contains(l, "Staging deployed 2.14.0") {
			y = i + 1 // the second line of the card
		}
	}
	if y < 0 {
		t.Fatal("the screen has no third card")
	}
	_, cmd := h.m.Update(tea.MouseClickMsg{X: 10, Y: y, Button: tea.MouseLeft})
	run(h.m, cmd, 0)
	if c, _ := h.m.selectedCard(); c.Title != "Staging deployed 2.14.0" {
		t.Fatalf("click selected %q", c.Title)
	}
	if st := h.state(t, "Staging deployed 2.14.0"); st != inbox.CardRead {
		t.Errorf("state after the click = %s, want read", st)
	}
}
