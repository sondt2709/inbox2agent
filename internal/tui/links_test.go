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
		c.Spaces = append(c.Spaces, config.Space{ID: "beta", Name: "Beta", Agent: config.NewAgent("beta"), Schedule: "always"})
		return nil
	})
	refresh(m)
	send(m, "enter", "enter") // the panel of Acme on # release, then its spaces dialog
	d, ok := m.topDialog().(*spacesDialog)
	if !ok || len(d.opts) != 2 || !d.opts[0].on || d.opts[1].on {
		t.Fatalf("the spaces dialog of # release: %+v", m.topDialog())
	}
	send(m, "down", "space", "right", "enter") // check Beta with Only for me
	c := loadFile(t, m)
	if got := c.Links("work", "C1"); len(got) != 2 || got[0].Read != config.ReadAll || got[1].Space != "beta" || got[1].Read != config.ReadMentions {
		t.Fatalf("links = %+v, want acme all and beta mentions", got)
	}
	if s := screen(m); !strings.Contains(s, "also in Beta") || !strings.Contains(s, "All messages") {
		t.Errorf("the row of Acme does not show the other space:\n%s", s)
	}
	send(m, "right") // the level in Acme only
	if got := loadFile(t, m).Links("work", "C1"); got[0].Read != config.ReadMentions || got[1].Read != config.ReadMentions || len(got) != 2 {
		t.Errorf("links after → in Acme = %+v", got)
	}
	send(m, "x") // remove from Acme: the conversation stays in Beta
	if got := loadFile(t, m).Routes("work", "C1"); !slices.Equal(got, []string{"beta"}) {
		t.Errorf("routes after x in Acme = %v, want only beta", got)
	}
}

// A card from the terminal of the session of a space shows that space, also
// when it has no message.
func TestFeedShowsTheSpaceOfTheTerminalOfACard(t *testing.T) {
	h := newFeedModel(t)
	ctx := context.Background()
	if err := h.st.SetSession(ctx, inbox.Session{Space: config.UnsortedID, Handle: "term_unsorted", CreatedAt: h.m.now()}); err != nil {
		t.Fatal(err)
	}
	if _, err := h.st.SaveCards(ctx, []inbox.Card{{Category: inbox.CategoryAction, Urgency: inbox.UrgencyHigh,
		Title: "From the terminal", Handle: "term_unsorted"}}, h.m.now()); err != nil {
		t.Fatal(err)
	}
	refresh(h.m)
	pickSpace(t, h.m, "Everything else")
	if !strings.Contains(titles(h.m), "From the terminal") {
		t.Errorf("the card of the terminal of Unsorted is not in Unsorted: %q", titles(h.m))
	}
}

func TestDeleteASpaceThatSharesAConversation(t *testing.T) {
	m := newTestModel(t)
	changeFile(t, m, func(c *config.Config) error {
		c.Spaces = append(c.Spaces, config.Space{ID: "beta", Name: "Beta", Agent: config.NewAgent("beta"), Schedule: "always"})
		return c.AddLink("work", "C1", "beta", config.ReadAll)
	})
	refresh(m)
	at(m, tabSpaces)
	m.deleteSpace("acme")
	send(m, "y")
	c := loadFile(t, m)
	if _, ok := c.Space("acme"); ok {
		t.Fatalf("the space is not deleted: %s", m.status)
	}
	if got := c.Routes("work", "C1"); !slices.Equal(got, []string{"beta"}) {
		t.Errorf("routes of C1 = %v, want only beta (not Unsorted)", got)
	}
}
