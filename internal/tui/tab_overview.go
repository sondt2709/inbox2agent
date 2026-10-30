package tui

import (
	"context"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/sondt2709/inbox2agent/internal/config"
	"github.com/sondt2709/inbox2agent/internal/inbox"
	"github.com/sondt2709/inbox2agent/internal/store"
)

func (m *model) keyOverview(k string) tea.Cmd {
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

func card(title, body string, w int) string {
	lines := strings.Count(body, "\n") + 1
	return panel(title, body, w, lines+3)
}

func (m *model) viewOverview(w, h int) string {
	cfg := m.u.cfg
	colW := w / 2
	if w < 100 {
		colW = w
	}

	// Service
	var svc []string
	switch {
	case m.svcErr != nil:
		svc = append(svc, faint.Render("not supported on this platform"), faint.Render("run `i2a daemon` in a terminal"))
	case m.svc.State == "running":
		svc = append(svc, okStyle.Render("● running")+faint.Render("  pid "+m.svc.PID), faint.Render("polls and dispatches in the background"))
	case m.svc.Loaded:
		svc = append(svc, warnStyle.Render("● "+orStr(m.svc.State, "loaded")), faint.Render("press i to restart it"))
	default:
		svc = append(svc, faint.Render("○ not installed"), faint.Render("press i to run i2a in the background"))
	}
	serviceCard := card("Service", strings.Join(svc, "\n"), colW)

	// Messages
	c := m.counts
	num := func(n int, st lipgloss.Style, label string) string {
		return st.Inherit(bigNumber).Render(fmt.Sprintf("%5d", n)) + "  " + faint.Render(label)
	}
	msgBody := strings.Join([]string{
		num(c[inbox.StatePending], infoStyle, "pending: wait for the next dispatch"),
		num(c[inbox.StatePreviewed], warnStyle, "previewed by a dry run"),
		num(c[inbox.StateSent], okStyle, "sent to an agent"),
		num(c[inbox.StateExpired], faint, "expired (older than the lookback)"),
	}, "\n")
	if m.u.d.Store == nil {
		msgBody = faint.Render("no store yet: run i2a run --once")
	}
	messagesCard := card("Messages", msgBody, colW)

	// Sources
	var src []string
	if len(cfg.Sources) == 0 {
		src = append(src, warnStyle.Render("No source yet."), faint.Render("Press 3, then a, to add Mattermost or WhatsApp."))
	}
	for _, s := range cfg.Sources {
		kind := s.Type
		if st := m.waStates[s.ID]; st != "" {
			kind += " · " + st
		}
		src = append(src, boldStyle.Render(s.ID)+faint.Render("  "+kind+" · to "+orStr(s.Profile, cfg.Profiles[0].Name)),
			faint.Render("  "+watchSummary(s)))
	}
	sourcesCard := card("Sources", strings.Join(src, "\n"), colW)

	// Profiles
	var prof []string
	for _, p := range cfg.Profiles {
		line := boldStyle.Render(p.Name) + faint.Render("  "+p.Driver)
		if p.Driver == config.DriverOrca {
			line += faint.Render(" · " + p.Agent + " · " + p.Session + " session · " + p.Payload)
		}
		prof = append(prof, line)
	}
	profilesCard := card("Profiles", strings.Join(prof, "\n"), colW)

	// Schedule
	t := cfg.Timing
	active := okStyle.Render("active now")
	if ah, err := config.ParseActiveHours(t.ActiveHours); err == nil && ah.Set && !ah.Contains(m.now()) {
		active = warnStyle.Render("outside the active hours now")
	}
	schedBody := textStyle.Render("every "+t.Interval.String()+" · "+orStr(t.ActiveHours, "all day")) + "  " + active + "\n" +
		faint.Render("look back "+t.Lookback.String()+" · keep "+t.Retention.String()+" · notifications "+onOff(cfg.Notify.Enabled))
	scheduleCard := card("Schedule", schedBody, colW)

	// Files
	p := m.u.d.Paths
	filesBody := field("Config", textStyle.Render(shortPath(p.ConfigFile, colW-20))) + "\n" +
		field("Digests", textStyle.Render(shortPath(p.DigestDir, colW-20))) + "\n" +
		field("Log", textStyle.Render(shortPath(p.LogFile, colW-20)))
	filesCard := card("Files", filesBody, colW)

	var top string
	if w < 100 {
		top = lipgloss.JoinVertical(lipgloss.Left, sourcesCard, profilesCard, messagesCard, serviceCard, scheduleCard)
	} else {
		left := lipgloss.JoinVertical(lipgloss.Left, sourcesCard, profilesCard, scheduleCard)
		right := lipgloss.JoinVertical(lipgloss.Left, messagesCard, serviceCard, filesCard)
		top = lipgloss.JoinHorizontal(lipgloss.Top, left, lipgloss.NewStyle().Width(w-colW).Render(right))
	}
	rest := h - lipgloss.Height(top)
	if rest >= 5 {
		top = lipgloss.JoinVertical(lipgloss.Left, top, panel("Last dispatches", dispatchTable(m.dispatches, -1, 0, w-4, rest-3), w, rest))
	}
	return top
}

// watchSummary is a short text of what a source watches.
func watchSummary(s config.Source) string {
	var parts []string
	if len(s.Channels) > 0 {
		parts = append(parts, plural(len(s.Channels), "selected", "selected"))
	}
	if s.AllDirect {
		parts = append(parts, "all direct")
	}
	if s.Mentions {
		parts = append(parts, "mentions")
	}
	if len(parts) == 0 {
		return "nothing selected"
	}
	return strings.Join(parts, " · ")
}

// dispatchTable renders the dispatches as a table. cursor -1 selects no row.
func dispatchTable(ds []store.Dispatch, cursor, offset, w, h int) string {
	if len(ds) == 0 {
		return faint.Render("No dispatch yet. Run i2a run --once, or wait for the daemon.")
	}
	head := colHeadStyle.Render(fmt.Sprintf("%-12s %-10s %-8s %-8s %5s  %s", "TIME", "PROFILE", "DRIVER", "STATUS", "MSGS", "DETAIL"))
	rows := []string{head}
	end := min(len(ds), offset+h-1)
	for i := offset; i < end; i++ {
		d := ds[i]
		st := faint
		switch d.Status {
		case store.DispatchSent:
			st = okStyle
		case store.DispatchFailed:
			st = errStyle
		case store.DispatchHeld:
			st = warnStyle
		}
		prefix := fmt.Sprintf("%-12s %-10s %-8s ", d.CreatedAt.Local().Format("01-02 15:04"), truncate(d.Profile, 10), truncate(d.Driver, 8))
		rest := fmt.Sprintf(" %5d  %s", d.MessageCount, d.Detail)
		if i == cursor {
			rows = append(rows, selectedRow.Width(w).Render(truncate(prefix+fmt.Sprintf("%-8s", d.Status)+rest, w)))
			continue
		}
		line := textStyle.Render(prefix) + st.Render(fmt.Sprintf("%-8s", d.Status)) + faint.Render(rest)
		rows = append(rows, truncateStyled(line, w))
	}
	return strings.Join(rows, "\n")
}
