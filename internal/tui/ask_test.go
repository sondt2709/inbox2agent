package tui

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/sondt2709/inbox2agent/internal/agent"
	"github.com/sondt2709/inbox2agent/internal/config"
	"github.com/sondt2709/inbox2agent/internal/inbox"
	"github.com/sondt2709/inbox2agent/internal/render"
)

// tuiAgent records the asks of the Feed and returns a set result.
type tuiAgent struct {
	asks   []*render.Rendered
	wait   time.Duration
	result agent.Result
}

func (a *tuiAgent) Deliver(context.Context, *render.Rendered) (agent.Result, error) {
	return agent.Result{}, nil
}

func (a *tuiAgent) Ask(_ context.Context, _ string, wait time.Duration, r *render.Rendered) (agent.Result, error) {
	a.asks = append(a.asks, r)
	a.wait = wait
	return a.result, nil
}

// newAskFeed is the Feed harness with the orca profile "triage" and a fake agent.
func newAskFeed(t *testing.T) (*feedHarness, *tuiAgent) {
	t.Helper()
	h := newFeedModel(t)
	h.m.u.cfg.Profiles[0].Driver, h.m.u.cfg.Profiles[0].Workspace = config.DriverOrca, t.TempDir()
	a := &tuiAgent{result: agent.Result{Status: agent.StatusSent, Detail: "sent to term_s, turn started", Handle: "term_s"}}
	h.m.u.d.AskDriver = func(p config.Profile, _ agent.Sessions) (agent.Driver, error) {
		if p.Name != "triage" {
			t.Errorf("driver of profile %q", p.Name)
		}
		return a, nil
	}
	return h, a
}

func (h *feedHarness) asks(t *testing.T, title string) []inbox.Ask {
	t.Helper()
	all, _ := h.st.Cards(context.Background(), true)
	for _, c := range all {
		if c.Title == title {
			by, err := h.st.Asks(context.Background(), []int64{c.ID})
			if err != nil {
				t.Fatal(err)
			}
			return by[c.ID]
		}
	}
	t.Fatalf("no card %q", title)
	return nil
}

func TestFeedAskTheAgent(t *testing.T) {
	h, a := newAskFeed(t)
	send(h.m, "t")
	s := screen(h.m)
	for _, want := range []string{"Ask the agent · card", "Agent      triage · orca", "Session    the session of the profile",
		"The agent answers in this card.", "ctrl+s send", "esc cancel"} {
		if !strings.Contains(s, want) {
			t.Errorf("the ask form does not show %q:\n%s", want, s)
		}
	}
	if st := h.state(t, "Correct your reply to Lena"); st != inbox.CardRead {
		t.Errorf("state after t = %s, want read", st)
	}
	typeText(h.m, "Check the auth code and give me the effort.")
	send(h.m, "ctrl+s")

	if len(a.asks) != 1 || a.wait != 2*time.Minute {
		t.Fatalf("asks = %d, wait %s", len(a.asks), a.wait)
	}
	p := a.asks[0].Prompt
	if !strings.Contains(p, "--- ask ---\nCheck the auth code and give me the effort.\n--- end ---") ||
		!strings.Contains(p, `"title":"Correct your reply to Lena"`) || !strings.Contains(p, `card add and the key "mm:lena"`) {
		t.Errorf("prompt = %s", p)
	}
	s = screen(h.m)
	for _, want := range []string{"Sent to the agent (triage). It answers in this card.", "↪ asked",
		"↪ Asked " + time.Now().Format("15:04") + ": Check the auth code and give me the effort. · sent"} {
		if !strings.Contains(s, want) {
			t.Errorf("the Feed does not show %q:\n%s", want, s)
		}
	}
	if strings.Contains(s, "Ask the agent · card") {
		t.Error("the form is still open after the send")
	}
	if got := h.asks(t, "Correct your reply to Lena"); len(got) != 1 || got[0].State != inbox.AskSent || got[0].Handle != "term_s" {
		t.Errorf("stored asks = %+v", got)
	}
	if st := h.state(t, "Correct your reply to Lena"); st != inbox.CardRead {
		t.Errorf("state after the ask = %s, want read", st)
	}

	// The agent answers in the same card: the card is updated and no longer waits.
	if _, err := h.st.SaveCards(context.Background(), []inbox.Card{{Category: inbox.CategoryAction, Urgency: inbox.UrgencyHigh,
		Group: "Harbor Portal", Title: "Correct your reply to Lena", Body: "The effort is 6 hours.", Key: "mm:lena"}},
		time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	send(h.m, "ctrl+r")
	h.m.Update(readFeed(context.Background(), h.st, false))
	s = screen(h.m)
	if strings.Contains(s, "↪ asked") || !strings.Contains(s, "updated") || !strings.Contains(s, "The effort is 6 hours.") {
		t.Errorf("after the answer:\n%s", s)
	}
}

func TestFeedAskBusyAgent(t *testing.T) {
	h, a := newAskFeed(t)
	a.result = agent.Result{Status: agent.StatusHeld, Detail: "agent busy in term_s"}
	send(h.m, "t")
	typeText(h.m, "more")
	send(h.m, "ctrl+s")
	s := screen(h.m)
	if !strings.Contains(s, "The agent is busy. i2a sends the ask on the next dispatch.") || !strings.Contains(s, "· queued (agent busy)") ||
		!strings.Contains(s, "↪ asked") {
		t.Errorf("busy agent:\n%s", s)
	}
	if got := h.asks(t, "Correct your reply to Lena"); len(got) != 1 || got[0].State != inbox.AskQueued {
		t.Errorf("stored asks = %+v", got)
	}
}

func TestFeedAskEmptyTextAndCancel(t *testing.T) {
	h, a := newAskFeed(t)
	send(h.m, "t", "ctrl+s")
	if s := screen(h.m); !strings.Contains(s, "the text is empty") {
		t.Errorf("no empty text error:\n%s", s)
	}
	send(h.m, "esc")
	if s := screen(h.m); strings.Contains(s, "Ask the agent · card") || !strings.Contains(s, "Ask cancelled. Nothing is sent.") {
		t.Errorf("esc does not close the form:\n%s", s)
	}
	if len(a.asks) != 0 || len(h.asks(t, "Correct your reply to Lena")) != 0 {
		t.Errorf("asks = %d, want none", len(a.asks))
	}
}

func TestFeedAskDryRun(t *testing.T) {
	h := newFeedModel(t) // the default profile is a dry run
	send(h.m, "t")
	s := screen(h.m)
	if strings.Contains(s, "Ask the agent · card") || !strings.Contains(s, "Cannot ask: The profile triage uses dry-run; no agent reads asks.") {
		t.Errorf("dry run:\n%s", s)
	}
	if len(h.asks(t, "Correct your reply to Lena")) != 0 {
		t.Error("the store has an ask")
	}
}
