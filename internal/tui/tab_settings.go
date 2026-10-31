package tui

import (
	"context"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/sondt2709/inbox2agent/internal/config"
)

// keySettings installs and uninstalls the service when the form does not have the keyboard.
func (m *model) keySettings(k string) tea.Cmd {
	switch k {
	case "i":
		if m.u.d.InstallService == nil {
			return nil
		}
		return m.async("Installing the service ...", func(context.Context) func() tea.Cmd {
			path, err := m.u.d.InstallService()
			return func() tea.Cmd {
				if err != nil {
					m.openDialog(&msgDialog{name: "Cannot install the service", text: err.Error(), isErr: true})
					return nil
				}
				m.setStatus("Installed "+shortPath(path, 60)+".", false)
				return m.load()
			}
		})
	case "u":
		if !m.svc.Loaded {
			m.setStatus("The service is not installed.", true)
			return nil
		}
		m.openDialog(&confirmDialog{name: "Uninstall the service", text: "Stop the background service and remove its LaunchAgent file?",
			choices: []choice{{"y", "Uninstall", func() tea.Cmd {
				path, err := m.u.d.UninstallService()
				if err != nil {
					m.openDialog(&msgDialog{name: "Cannot uninstall", text: err.Error(), isErr: true})
					return nil
				}
				m.setStatus("Removed "+shortPath(path, 60)+".", false)
				return m.load()
			}}}})
	}
	return nil
}

// settingsForm edits the message limits and the notifications.
func (m *model) settingsForm() *form {
	s := m.u.cfg.Settings
	fm := &form{}
	fm.fields = []*formField{
		noteField("Messages", boldStyle),
		textField("lookback", "Lookback", s.Lookback.String(), "1d",
			"On the first read, i2a collects the messages of this time. Older waiting messages expire.", minDuration(time.Minute)),
		textField("retention", "Retention", s.Retention.String(), "7d",
			"i2a deletes sent and expired messages and digest files after this time.", minDuration(time.Hour)),
		noteField("Notifications", boldStyle),
		switchField("notify", "Notifications", "A macOS notification for new cards from the agent, and after each failed send.",
			m.u.cfg.Notify.Enabled),
		choiceField("urgency", "Notify from", "The lowest card urgency that gives a notification. Cards with a lower urgency show only in the Feed.",
			[]string{"high", "normal", "low"}, []string{"High", "Normal", "Low"}, m.u.cfg.Notify.MinUrgency),
		noteField("Service", boldStyle),
		noteField(m.serviceText(), faint),
		noteField("When a space sends is in its schedule (tab 7). How often i2a reads an account is in the account (tab 3).", faint),
	}
	fm.buttons = []button{
		{label: "Save changes", primary: true, run: func() tea.Cmd { return m.saveSettings() }},
		{label: "Cancel", run: func() tea.Cmd { m.buildForm(true); return nil }},
	}
	return fm
}

func (m *model) serviceText() string {
	switch {
	case m.svcErr != nil:
		return "Not supported on this platform: run i2a daemon in a terminal."
	case m.svc.State == "running":
		return "Running, pid " + m.svc.PID + ". It reads and sends in the background. Press esc, then i to restart it or u to uninstall it."
	case m.svc.Loaded:
		return "Loaded (" + orStr(m.svc.State, "not running") + "). Press esc, then i to restart it."
	}
	return "Not installed. Press esc, then i to run i2a in the background."
}

func (m *model) saveSettings() tea.Cmd {
	fm := m.form
	if !fm.validate() {
		return nil
	}
	next := clone(m.u.cfg)
	next.Settings.Lookback, _ = config.ParseDuration(fm.field("lookback").value())
	next.Settings.Retention, _ = config.ParseDuration(fm.field("retention").value())
	next.Notify.Enabled = fm.field("notify").value() == "on"
	next.Notify.MinUrgency = fm.field("urgency").value()
	if err := m.u.commit(next, nil); err != nil {
		fm.err = "Cannot save: " + err.Error()
		return nil
	}
	m.saved("Saved the settings.")
	return nil
}

func (m *model) viewSettings(w, h int) string {
	dw, dh := detailSize(w, h, true)
	body := ""
	if m.form != nil {
		body = m.form.render(dw, dh)
	}
	return detailPanel("Settings", body, w, h, m.form != nil && m.form.active)
}
