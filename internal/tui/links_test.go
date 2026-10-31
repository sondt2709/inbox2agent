package tui

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/sondt2709/inbox2agent/internal/config"
	"github.com/sondt2709/inbox2agent/internal/inbox"
)

func TestOneConversationInTwoSpaces(t *testing.T) {
	m := newTestModel(t)
	changeFile(t, m, func(c *config.Config) error {
		c.Spaces = append(c.Spaces, config.Space{ID: "beta", Name: "Beta", Agent: "claude-work", Schedule: "always"})
		return nil
	})
	refresh(m)
	send(m, "4") // the first row is release (work/C1) in Acme
	pick := func(name string) {
		t.Helper()
		d, ok := m.topDialog().(*pickDialog)
		if !ok {
			t.Fatalf("dialog %v, want a pick dialog", m.topDialog())
		}
		i := slices.IndexFunc(d.options, func(o pickOption) bool { return o.label == name })
		if i < 0 {
			t.Fatalf("the dialog has no row %q: %+v", name, d.options)
		}
		d.cursor = i
		send(m, "enter")
	}
	send(m, "L")
	if d, _ := m.topDialog().(*pickDialog); d == nil || slices.ContainsFunc(d.options, func(o pickOption) bool { return o.label == "Acme" }) {
		t.Fatalf("the link dialog offers the space that lists the conversation: %+v", d)
	}
	pick("Beta")
	if got := loadFile(t, m).Routes("work", "C1"); !slices.Equal(got, []string{"acme", "beta"}) {
		t.Fatalf("routes = %v, want acme and beta", got)
	}
	if s := screen(m); !strings.Contains(s, "Acme, Beta") || !strings.Contains(s, "is also in Beta") {
		t.Errorf("the table does not show both spaces:\n%s", s)
	}

	send(m, "l") // all → mentions on each link
	for _, l := range loadFile(t, m).Links("work", "C1") {
		if l.Read != config.ReadMentions {
			t.Errorf("link %+v after l, want mentions on each link", l)
		}
	}
	if got := loadFile(t, m).Routes("work", "C1"); len(got) != 2 {
		t.Errorf("l removed a link: %v", got)
	}

	send(m, "m")
	pick("Beta")
	if got := loadFile(t, m).Routes("work", "C1"); !slices.Equal(got, []string{"beta"}) {
		t.Errorf("routes after the move = %v, want only beta", got)
	}
}

// A card from the terminal of the session of a space shows that space, also
// when it has no message.
func TestFeedShowsTheSpaceOfTheTerminalOfACard(t *testing.T) {
	h := newFeedModel(t)
	ctx := context.Background()
	if err := h.st.SetSession(ctx, config.UnsortedID, "term_unsorted", "h", h.m.now()); err != nil {
		t.Fatal(err)
	}
	if _, err := h.st.SaveCards(ctx, []inbox.Card{{Category: inbox.CategoryAction, Urgency: inbox.UrgencyHigh,
		Title: "From the terminal", Handle: "term_unsorted"}}, h.m.now()); err != nil {
		t.Fatal(err)
	}
	refresh(h.m)
	pickSpace(t, h.m, "Unsorted")
	if !strings.Contains(titles(h.m), "From the terminal") {
		t.Errorf("the card of the terminal of Unsorted is not in Unsorted: %q", titles(h.m))
	}
}
