package tui

import (
	"context"
	"regexp"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/sondt2709/inbox2agent/internal/config"
	"github.com/sondt2709/inbox2agent/internal/inbox"
)

func lineWith(s, text string) string {
	for l := range strings.SplitSeq(s, "\n") {
		if strings.Contains(l, text) {
			return l
		}
	}
	return ""
}

func TestFeedShowsUrgencyBars(t *testing.T) {
	h := newFeedModel(t)
	s := screen(h.m)
	for title, bars := range map[string]string{"Correct your reply": "▰▰▰", "Review PR #57": "▰▰▱", "Staging deployed": "▰▱▱"} {
		if l := lineWith(s, title); !strings.Contains(l, bars) {
			t.Errorf("the row of %q does not show %s:\n%s", title, bars, l)
		}
	}
	if !strings.Contains(s, "▰▰▰ high · space") {
		t.Errorf("the detail panel does not show the bars and the word:\n%s", s)
	}
}

// withAcme adds the space Acme with the conversation work/C9, and a card with
// a message of C9, as another process writes config.toml.
func withAcme(t *testing.T, h *feedHarness) {
	t.Helper()
	ctx := context.Background()
	msg := inbox.Message{SourceID: "work", SourceType: "mattermost", ExternalID: "p9", ChannelID: "C9", ChannelName: "acme-ops",
		ChannelKind: inbox.KindChannel, Author: "omar", Text: "deploy", CreatedAt: h.m.now()}
	if _, err := h.st.SavePoll(ctx, "work", []inbox.Message{msg}, nil, h.m.now()); err != nil {
		t.Fatal(err)
	}
	p, _ := h.st.Pending(ctx)
	if _, err := h.st.SaveCards(ctx, []inbox.Card{{Category: inbox.CategoryAction, Urgency: inbox.UrgencyNormal,
		Title: "Acme deploy", Refs: []int64{p[len(p)-1].ID}}}, h.m.now()); err != nil {
		t.Fatal(err)
	}
	cfg := config.Default()
	cfg.Accounts = []config.Account{config.NewMattermostAccount("work", "https://chat.example.com")}
	cfg.Spaces = []config.Space{{ID: "acme", Name: "Acme", Agent: "triage", Schedule: "always",
		Conversations: []config.Conversation{{Account: "work", ID: "C9", Read: config.ReadAll}}}}
	if err := config.Save(h.m.u.d.Paths.ConfigFile, cfg); err != nil {
		t.Fatal(err)
	}
	refresh(h.m)
}

// pickSpace opens the space dialog and chooses the row that starts with name.
func pickSpace(t *testing.T, m *model, name string) {
	t.Helper()
	send(m, "F")
	d, ok := m.topDialog().(*pickDialog)
	if !ok {
		t.Fatalf("F opened %v, want the space dialog", m.topDialog())
	}
	i := slices.IndexFunc(d.options, func(o pickOption) bool { return o.label == name })
	if i < 0 {
		t.Fatalf("the space dialog has no row %q", name)
	}
	d.cursor = i
	send(m, "enter")
}

func TestSpaceDialogShowsTheCardCounts(t *testing.T) {
	h := newFeedModel(t)
	withAcme(t, h)
	send(h.m, "F")
	s := screen(h.m)
	for _, want := range []*regexp.Regexp{
		regexp.MustCompile(`All spaces\s+4 cards`), regexp.MustCompile(`Unsorted\s+2 cards`), regexp.MustCompile(`Acme\s+1 card\b`),
	} {
		if !want.MatchString(s) {
			t.Errorf("the space dialog does not match %s:\n%s", want, s)
		}
	}
}

func TestSpaceFilterKeepsTheIDAndResets(t *testing.T) {
	h := newFeedModel(t)
	withAcme(t, h)
	pickSpace(t, h.m, "Acme")
	if got := titles(h.m); got != "Acme deploy" {
		t.Fatalf("cards of Acme = %q", got)
	}
	if !strings.Contains(screen(h.m), "space: Acme") {
		t.Error("the filter bar does not show the space")
	}
	// Another process adds a space before Acme: the filter stays on Acme.
	cfg := loadFile(t, h.m)
	cfg.Spaces = append([]config.Space{{ID: "aaa", Name: "Aaa", Agent: "triage", Schedule: "always"}}, cfg.Spaces...)
	if err := config.Save(h.m.u.d.Paths.ConfigFile, cfg); err != nil {
		t.Fatal(err)
	}
	refresh(h.m)
	if got := titles(h.m); got != "Acme deploy" {
		t.Fatalf("after a new space, cards = %q, want the cards of Acme", got)
	}
	// Another process removes Acme: the filter is all spaces.
	cfg.Unsorted.Conversations = append(cfg.Unsorted.Conversations, cfg.Spaces[1].Conversations...)
	cfg.Spaces = cfg.Spaces[:1]
	if err := config.Save(h.m.u.d.Paths.ConfigFile, cfg); err != nil {
		t.Fatal(err)
	}
	refresh(h.m)
	if got := titles(h.m); !strings.Contains(got, "Staging deployed") || !strings.Contains(got, "Acme deploy") {
		t.Errorf("after the space was removed, cards = %q, want all", got)
	}
}

func TestActivityUsesTheSpaceFilter(t *testing.T) {
	m := newTestModel(t)
	send(m, "2")
	if s := screen(m); !strings.Contains(s, "unsorted preview") {
		t.Fatalf("Activity does not show the send of Unsorted:\n%s", s)
	}
	send(m, "F")
	if s := screen(m); !regexp.MustCompile(`Acme\s+1 send\b`).MatchString(s) {
		t.Errorf("the space dialog of Activity has no count of sends:\n%s", s)
	}
	send(m, "esc")
	pickSpace(t, m, "Acme")
	s := screen(m)
	if strings.Contains(s, "unsorted preview") || !strings.Contains(s, "new session term_1") || !strings.Contains(s, "space Acme") {
		t.Errorf("Activity with the filter Acme:\n%s", s)
	}
	// The same filter in the Feed.
	send(m, "1")
	if !strings.Contains(screen(m), "space: Acme") {
		t.Errorf("the Feed does not use the filter of Activity:\n%s", screen(m))
	}
}

func TestFeedFooterHasNoCutHint(t *testing.T) {
	h := newFeedModel(t)
	h.m.Update(tea.WindowSizeMsg{Width: 140, Height: 34})
	lines := strings.Split(screen(h.m), "\n")
	footer := strings.TrimSpace(lines[len(lines)-1])
	var known []string
	for _, x := range append(feedHints(), hint{"tab", "next tab"}, hint{"?", "keys"}, hint{"q", "quit"}) {
		known = append(known, x.key+" "+x.desc)
	}
	for _, part := range regexp.MustCompile(`\s+·\s+|\s{3,}`).Split(footer, -1) {
		if !slices.Contains(known, part) {
			t.Errorf("the footer has a cut hint %q:\n%s", part, footer)
		}
	}
	if !strings.Contains(footer, "? keys") {
		t.Errorf("the footer has no ? keys:\n%s", footer)
	}
}

func TestKeyHelpDoesNotWrapItsLines(t *testing.T) {
	h := newFeedModel(t)
	h.m.Update(tea.WindowSizeMsg{Width: 140, Height: 34})
	send(h.m, "?")
	s := screen(h.m)
	for l := range strings.SplitSeq(helpText, "\n") {
		if l = strings.TrimRight(l, " "); l != "" && !strings.Contains(s, l) {
			t.Errorf("the key help does not show the line %q as one line:\n%s", l, s)
		}
	}
	if l := lineWith(s, "r reply"); !strings.Contains(ansi.Strip(l), "Feed") {
		t.Errorf("the line of the Feed keys does not start with Feed: %q", l)
	}
}

func TestCardDetailWrapsTheMetaLine(t *testing.T) {
	h := newFeedModel(t)
	h.m.Update(tea.WindowSizeMsg{Width: 100, Height: 34})
	if s := screen(h.m); !strings.Contains(s, "mm:lena") {
		t.Errorf("the detail panel cuts the key of the card:\n%s", s)
	}
}

func TestActivityWithASpaceThatHasNoSend(t *testing.T) {
	m := newTestModel(t)
	cfg := loadFile(t, m)
	cfg.Spaces = append(cfg.Spaces, config.Space{ID: "quiet", Name: "Quiet", Agent: "triage", Schedule: "always"})
	if err := config.Save(m.u.d.Paths.ConfigFile, cfg); err != nil {
		t.Fatal(err)
	}
	refresh(m)
	send(m, "2")
	pickSpace(t, m, "Quiet")
	if s := screen(m); !strings.Contains(s, "This space has no send") || strings.Contains(s, "No send yet") {
		t.Errorf("Activity of a space with no send:\n%s", s)
	}
}

func TestKeyHelpFitsEightyColumns(t *testing.T) {
	h := newFeedModel(t)
	h.m.Update(tea.WindowSizeMsg{Width: 80, Height: 40})
	send(h.m, "?")
	s := screen(h.m)
	for l := range strings.SplitSeq(helpText, "\n") {
		if l = strings.TrimRight(l, " "); l != "" && !strings.Contains(s, l) {
			t.Errorf("at 80 columns the key help wraps the line %q:\n%s", l, s)
		}
	}
}

func TestRemovedSpaceDoesNotComeBackInTheFilter(t *testing.T) {
	h := newFeedModel(t)
	withAcme(t, h)
	pickSpace(t, h.m, "Acme")
	cfg := loadFile(t, h.m)
	spaces := cfg.Spaces
	cfg.Unsorted.Conversations = append(cfg.Unsorted.Conversations, cfg.Spaces[0].Conversations...)
	cfg.Spaces = nil
	if err := config.Save(h.m.u.d.Paths.ConfigFile, cfg); err != nil {
		t.Fatal(err)
	}
	refresh(h.m)
	cfg.Spaces = spaces // another process adds a space with the same ID
	cfg.Unsorted.Conversations = nil
	if err := config.Save(h.m.u.d.Paths.ConfigFile, cfg); err != nil {
		t.Fatal(err)
	}
	refresh(h.m)
	if got := titles(h.m); !strings.Contains(got, "Staging deployed") {
		t.Errorf("the old filter came back: cards = %q", got)
	}
}
