package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/sondt2709/inbox2agent/internal/daemon"
)

// R reads all accounts and sends now: with the daemon when it runs, else in
// this process (Deps.RunNow). The status line shows the result and the events
// of the daemon (Deps.Events).

// runDoneMsg is the result of a run of the R key.
type runDoneMsg struct {
	res daemon.RunResult
	err error
}

// eventsMsg is the result of a try to read the events of the daemon. ch is
// nil when no daemon answers.
type eventsMsg struct{ ch <-chan daemon.Event }

// eventMsg is one event of the daemon; ok is false when the stream ended.
type eventMsg struct {
	ev daemon.Event
	ok bool
}

// runNow starts a run of the spaces (nil: all spaces). what names them in the status line.
func (m *model) runNow(spaces []string, what string) tea.Cmd {
	if m.u.d.RunNow == nil {
		m.setStatus("Read and send now is not available here.", true)
		return nil
	}
	if m.running {
		m.setStatus("A run is busy. Wait for its result.", true)
		return nil
	}
	m.running = true
	m.setStatus("Reading the accounts and sending "+what+" now…", false)
	run, ctx := m.u.d.RunNow, m.ctx
	return func() tea.Msg {
		res, err := run(ctx, spaces)
		return runDoneMsg{res: res, err: err}
	}
}

// runDone shows the result of a run: the new messages and each batch.
func (m *model) runDone(msg runDoneMsg) {
	m.running = false
	if msg.err != nil {
		m.setStatus("The run failed: "+msg.err.Error(), true)
		return
	}
	n := 0
	for _, r := range msg.res.Reads {
		n += r.New
	}
	parts := []string{fmt.Sprintf("Read %d new messages", n)}
	if len(msg.res.Batches) == 0 {
		parts = append(parts, "no batch to send")
	}
	names := m.spaceNames()
	for _, b := range msg.res.Batches {
		st := b.Status
		if b.Error != "" {
			st = "failed: " + b.Error
		}
		parts = append(parts, fmt.Sprintf("%s: %s (%d)", orStr(names[b.Space], b.Space), st, b.Count))
	}
	m.setStatus(strings.Join(parts, " · ")+".", msg.res.Failed)
}

// subscribe tries to read the events of the daemon, outside the UI loop.
func (m *model) subscribe() tea.Cmd {
	if m.u.d.Events == nil || m.events != nil || m.subscribing {
		return nil
	}
	m.subscribing = true
	events, ctx := m.u.d.Events, m.ctx
	return func() tea.Msg {
		ch, err := events(ctx)
		if err != nil {
			return eventsMsg{}
		}
		return eventsMsg{ch: ch}
	}
}

func (m *model) nextEvent() tea.Cmd {
	ch := m.events
	return func() tea.Msg {
		ev, ok := <-ch
		return eventMsg{ev: ev, ok: ok}
	}
}

// gotEvent shows the events that the user wants to see now: sends, reloads
// and errors. A failed read shows in the Accounts tab, not on each read in
// the status line. The Feed and Activity show the rest on the next refresh.
func (m *model) gotEvent(msg eventMsg) tea.Cmd {
	if !msg.ok {
		m.events = nil // the daemon stopped; the next refresh tries again
		return nil
	}
	ev, names := msg.ev, m.spaceNames()
	space := orStr(names[ev.Space], ev.Space)
	switch {
	case ev.Kind == daemon.EventSend && ev.Status == "failed":
		m.setStatus(fmt.Sprintf("Daemon: the batch of %s to %s failed: %s", space, ev.Agent, ev.Detail), true)
	case ev.Kind == daemon.EventSend:
		m.setStatus(fmt.Sprintf("Daemon: batch of %s to %s, %d messages: %s.", space, ev.Agent, ev.Count, ev.Status), false)
	case ev.Kind == daemon.EventReload:
		m.setStatus("Daemon: loaded the new config.toml.", false)
	case ev.Kind == daemon.EventError:
		m.setStatus("Daemon: "+ev.Detail, true)
	}
	return m.nextEvent()
}
