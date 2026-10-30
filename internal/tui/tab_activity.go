package tui

import (
	"fmt"
	"os"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/sondt2709/inbox2agent/internal/inbox"
	"github.com/sondt2709/inbox2agent/internal/store"
)

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
	c := m.counts
	counts := faint.Render("no store yet: run i2a run --once")
	if m.u.d.Store != nil {
		counts = infoStyle.Render(fmt.Sprintf("%d waiting", c[inbox.StatePending])) + faint.Render(" · ") +
			warnStyle.Render(fmt.Sprintf("%d previewed by a dry run", c[inbox.StatePreviewed])) + faint.Render(" · ") +
			okStyle.Render(fmt.Sprintf("%d sent", c[inbox.StateSent])) + faint.Render(" · ") +
			faint.Render(fmt.Sprintf("%d expired (older than the lookback)", c[inbox.StateExpired]))
	}
	rows := h - 6
	m.act = min(m.act, max(0, len(m.dispatches)-1))
	m.actOffset = scrollTo(m.act, m.actOffset, rows)
	title := fmt.Sprintf("Activity · last %d sends · refreshes every %s", len(m.dispatches), refreshEvery)
	body := counts + "\n\n" + dispatchTable(m.dispatches, m.spaceNames(), m.act, m.actOffset, w-4, rows+1)
	return panel(title, body, w, h)
}

// spaceNames returns the name of each space by ID, Unsorted too.
func (m *model) spaceNames() map[string]string {
	out := map[string]string{}
	for _, s := range m.u.cfg.AllSpaces() {
		out[s.ID] = s.Title()
	}
	return out
}

// dispatchTable renders the dispatches as a table. cursor -1 selects no row.
func dispatchTable(ds []store.Dispatch, names map[string]string, cursor, offset, w, h int) string {
	if len(ds) == 0 {
		return faint.Render("No send yet. Run i2a run --once, or wait for the schedule of a space.")
	}
	head := colHeadStyle.Render(fmt.Sprintf("%-12s %-16s %-14s %-8s %-8s %5s  %s", "TIME", "SPACE", "AGENT", "DRIVER", "STATUS", "MSGS", "DETAIL"))
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
		space := orStr(names[d.Space], d.Space)
		prefix := fmt.Sprintf("%-12s %-16s %-14s %-8s ", d.CreatedAt.Local().Format("01-02 15:04"), truncate(space, 16),
			truncate(d.Agent, 14), truncate(d.Driver, 8))
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
