package tui

import (
	"fmt"
	"os"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/sondt2709/inbox2agent/internal/config"
)

// scheduleForm edits the timing and the notifications.
func (m *model) scheduleForm() *form {
	t := m.u.cfg.Timing
	fm := &form{}
	fm.fields = []*formField{
		textField("interval", "Interval", t.Interval.String(), "15m",
			"Time between two dispatches. Each dispatch sends one batch for each profile that has messages.", minDuration(5*time.Second)),
		textField("lookback", "Lookback", t.Lookback.String(), "1d",
			"On the first poll, i2a collects the messages of this time. Older pending messages expire.", minDuration(time.Minute)),
		textField("hours", "Active hours", t.ActiveHours, "all day",
			"Local time as HH:MM-HH:MM. Outside these hours i2a collects but does not dispatch. Empty means all day.",
			func(s string) error { _, err := config.ParseActiveHours(s); return err }),
		textField("retention", "Retention", t.Retention.String(), "7d",
			"i2a deletes sent and expired messages and digest files after this time.", minDuration(time.Hour)),
		switchField("notify", "Notifications", "A macOS notification for new cards from the agent, and after each failed dispatch.",
			m.u.cfg.Notify.Enabled),
		choiceField("urgency", "Notify from", "The lowest card urgency that gives a notification. Cards with a lower urgency show only in the Feed.",
			[]string{"high", "normal", "low"}, []string{"High", "Normal", "Low"}, m.u.cfg.Notify.MinUrgency),
	}
	fm.buttons = []button{
		{label: "Save changes", primary: true, run: func() tea.Cmd { return m.saveSchedule() }},
		{label: "Cancel", run: func() tea.Cmd { m.buildForm(true); return nil }},
	}
	return fm
}

func (m *model) saveSchedule() tea.Cmd {
	fm := m.form
	if !fm.validate() {
		return nil
	}
	next := clone(m.u.cfg)
	t := &next.Timing
	t.Interval, _ = config.ParseDuration(fm.field("interval").value())
	t.Lookback, _ = config.ParseDuration(fm.field("lookback").value())
	t.Retention, _ = config.ParseDuration(fm.field("retention").value())
	t.ActiveHours = fm.field("hours").value()
	next.Notify.Enabled = fm.field("notify").value() == "on"
	next.Notify.MinUrgency = fm.field("urgency").value()
	if err := m.u.commit(next, nil); err != nil {
		fm.err = "Cannot save: " + err.Error()
		return nil
	}
	m.saved("Saved the schedule.")
	return nil
}

func (m *model) viewSchedule(w, h int) string {
	now := m.now()
	state := okStyle.Render("● dispatching now")
	if ah, err := config.ParseActiveHours(m.u.cfg.Timing.ActiveHours); err == nil && ah.Set && !ah.Contains(now) {
		state = warnStyle.Render("● outside the active hours: collecting only")
	}
	title := "Schedule · local time " + now.Format("15:04")
	dw, dh := detailSize(w, h, true)
	body := state + "\n\n"
	if m.form != nil {
		body += m.form.render(dw, dh-3)
	}
	return detailPanel(title, body, w, h, m.form != nil && m.form.active)
}

func (m *model) keyActivity(k string) tea.Cmd {
	switch k {
	case "up", "k":
		m.act = max(0, m.act-1)
	case "down", "j":
		m.act = min(max(0, len(m.dispatches)-1), m.act+1)
	case "enter":
		if m.act >= len(m.dispatches) {
			return nil
		}
		d := m.dispatches[m.act]
		b, err := os.ReadFile(d.DigestPath)
		if err != nil {
			m.openDialog(&msgDialog{name: "Digest", text: "Cannot read " + d.DigestPath + ". Retention can delete old digest files.\n\n" + err.Error(), isErr: true})
			return nil
		}
		m.openDialog(newViewer(shortPath(d.DigestPath, 70), string(b)))
	}
	return nil
}

func (m *model) viewActivity(w, h int) string {
	rows := h - 4
	m.act = min(m.act, max(0, len(m.dispatches)-1))
	m.actOffset = scrollTo(m.act, m.actOffset, rows)
	title := fmt.Sprintf("Activity · last %d dispatches · refreshes every %s", len(m.dispatches), refreshEvery)
	return panel(title, dispatchTable(m.dispatches, m.act, m.actOffset, w-4, rows+1), w, h)
}
