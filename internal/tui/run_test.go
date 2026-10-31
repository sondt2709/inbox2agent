package tui

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/sondt2709/inbox2agent/internal/daemon"
)

// fakeRun records the spaces of each run. release lets a run end.
type fakeRun struct {
	mu      sync.Mutex
	calls   [][]string
	release chan struct{}
}

func (f *fakeRun) run(_ context.Context, spaces []string) (daemon.RunResult, error) {
	f.mu.Lock()
	f.calls = append(f.calls, spaces)
	f.mu.Unlock()
	if f.release != nil {
		<-f.release
	}
	return daemon.RunResult{Reads: []daemon.AccountRead{{Account: "work", New: 2}},
		Batches: []daemon.Batch{{Space: "acme", Agent: "claude-work", Status: "sent", Count: 2}}}, nil
}

func TestRInTheFeedRunsAllSpaces(t *testing.T) {
	h := newFeedModel(t)
	f := &fakeRun{}
	h.m.u.d.RunNow = f.run
	send(h.m, "R")
	if len(f.calls) != 1 || f.calls[0] != nil {
		t.Fatalf("runs = %v, want one run of all spaces", f.calls)
	}
	if s := screen(h.m); !strings.Contains(s, "Read 2 new messages · acme: sent (2).") {
		t.Errorf("the status line does not show the result:\n%s", s)
	}
}

func TestRInTheSpacesTabRunsTheSelectedSpace(t *testing.T) {
	m := newTestModel(t)
	f := &fakeRun{}
	m.u.d.RunNow = f.run
	send(m, "down", "R") // Acme
	if len(f.calls) != 1 || len(f.calls[0]) != 1 || f.calls[0][0] != "acme" {
		t.Fatalf("runs = %v, want one run of acme", f.calls)
	}
	if s := screen(m); !strings.Contains(s, "Acme: sent (2)") {
		t.Errorf("the status line does not name the space:\n%s", s)
	}
}

func TestASecondRWhileARunIsBusy(t *testing.T) {
	h := newFeedModel(t)
	f := &fakeRun{release: make(chan struct{})}
	h.m.u.d.RunNow = f.run
	cmd := h.m.keyFeed("R") // the run waits for release
	h.m.keyFeed("R")
	if !strings.Contains(screen(h.m), "A run is busy") {
		t.Errorf("a second R did not say that a run is busy:\n%s", screen(h.m))
	}
	close(f.release)
	run(h.m, cmd, 0)
	if len(f.calls) != 1 {
		t.Errorf("runs = %d, want 1", len(f.calls))
	}
	if h.m.running {
		t.Error("the run did not end")
	}
}

func TestDaemonEventsShowInTheStatusLine(t *testing.T) {
	m := newTestModel(t)
	ch := make(chan daemon.Event, 1)
	m.u.d.Events = func(context.Context) (<-chan daemon.Event, error) { return ch, nil }
	ch <- daemon.Event{Kind: daemon.EventSend, Space: "acme", Agent: "claude-work", Count: 3, Status: "sent"}
	cmd := m.subscribe()
	msg := runWithTimeout(cmd) // the subscription
	cmd = m.update(msg)
	m.update(runWithTimeout(cmd)) // the first event
	if s := screen(m); !strings.Contains(s, "Daemon: batch of Acme to claude-work, 3 messages: sent.") {
		t.Errorf("the status line does not show the event:\n%s", s)
	}
	close(ch)
	m.update(runWithTimeout(m.nextEvent()))
	if m.events != nil {
		t.Error("the dashboard keeps a closed stream")
	}
}
