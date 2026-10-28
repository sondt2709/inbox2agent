package tui

import (
	"fmt"
	"os"
	"strings"

	"charm.land/lipgloss/v2"
)

var (
	accent    = lipgloss.Color("#7D56F4")
	accentDim = lipgloss.Color("#3B2F6B")
	fgColor   = lipgloss.Color("252")
	mutedFg   = lipgloss.Color("244")
	borderFg  = lipgloss.Color("#4A4A5E")
	green     = lipgloss.Color("#3FB950")
	yellow    = lipgloss.Color("#F2C94C")
	red       = lipgloss.Color("#F85149")
	blue      = lipgloss.Color("#58A6FF")

	brandStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FFFDF5")).Background(accent).Padding(0, 1)
	headerStyle  = lipgloss.NewStyle().Background(lipgloss.Color("#1E1B2E"))
	faint        = lipgloss.NewStyle().Foreground(mutedFg)
	textStyle    = lipgloss.NewStyle().Foreground(fgColor)
	boldStyle    = lipgloss.NewStyle().Bold(true).Foreground(fgColor)
	labelStyle   = lipgloss.NewStyle().Foreground(mutedFg).Width(13)
	tabActive    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FFFDF5")).Background(accent).Padding(0, 1)
	tabInactive  = lipgloss.NewStyle().Foreground(mutedFg).Padding(0, 1)
	tabKey       = lipgloss.NewStyle().Foreground(accent).Bold(true)
	okStyle      = lipgloss.NewStyle().Foreground(green)
	warnStyle    = lipgloss.NewStyle().Foreground(yellow)
	errStyle     = lipgloss.NewStyle().Foreground(red)
	infoStyle    = lipgloss.NewStyle().Foreground(blue)
	keyStyle     = lipgloss.NewStyle().Foreground(fgColor).Bold(true)
	panelStyle   = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(borderFg).Padding(0, 1)
	panelTitle   = lipgloss.NewStyle().Bold(true).Foreground(accent)
	selectedRow  = lipgloss.NewStyle().Background(accentDim).Foreground(lipgloss.Color("#FFFDF5")).Bold(true)
	dialogStyle  = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(accent).Padding(0, 1)
	dialogTitle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FFFDF5")).Background(accent).Padding(0, 1)
	dirtyStyle   = lipgloss.NewStyle().Foreground(yellow).Bold(true)
	bigNumber    = lipgloss.NewStyle().Bold(true)
	colHeadStyle = lipgloss.NewStyle().Foreground(mutedFg).Bold(true)
)

// panel draws a box of the outer size w × h with a title on the first line.
func panel(title, body string, w, h int) string {
	inner := max(1, w-4)
	content := panelTitle.Render(truncate(title, inner)) + "\n" + body
	return panelStyle.Width(w).Height(h).MaxHeight(h).Render(clipLines(content, inner, h-2))
}

// clipLines cuts each line to w cells and keeps at most h lines.
func clipLines(s string, w, h int) string {
	lines := strings.Split(s, "\n")
	if len(lines) > h {
		lines = lines[:max(0, h)]
	}
	for i, l := range lines {
		if lipgloss.Width(l) > w {
			lines[i] = truncateStyled(l, w)
		}
	}
	return strings.Join(lines, "\n")
}

// truncateStyled cuts a styled line to w cells.
func truncateStyled(s string, w int) string {
	return lipgloss.NewStyle().MaxWidth(w).Render(s)
}

// field is one "label  value" line of a detail panel.
func field(label, value string) string {
	return labelStyle.Render(label) + value
}

// listRows renders the visible rows of a list. The selected row has a bar and a background.
func listRows(items []string, cursor, offset, w, h int) string {
	var b strings.Builder
	end := min(len(items), offset+h)
	for i := offset; i < end; i++ {
		text := truncate(items[i], w-2) // the row starts with a 2-cell mark
		if i == cursor {
			b.WriteString(selectedRow.Width(w).Render("▌ " + text))
		} else {
			b.WriteString(textStyle.Render("  " + text))
		}
		if i < end-1 {
			b.WriteString("\n")
		}
	}
	return b.String()
}

// scrollTo returns the first visible row so that the cursor is visible.
func scrollTo(cursor, offset, h int) int {
	if cursor < offset {
		return cursor
	}
	if h > 0 && cursor >= offset+h {
		return cursor - h + 1
	}
	return offset
}

// listLabel is a list row: the name, and the kind at the right when it fits in w cells.
func listLabel(name, kind string, w int) string {
	if len([]rune(name))+1+len([]rune(kind)) > w {
		return name
	}
	return name + strings.Repeat(" ", w-len([]rune(name))-len([]rune(kind))) + kind
}

// hint is one key in the footer.
type hint struct{ key, desc string }

func renderHints(hs []hint) string {
	parts := make([]string, 0, len(hs))
	for _, h := range hs {
		parts = append(parts, keyStyle.Render(h.key)+" "+faint.Render(h.desc))
	}
	return strings.Join(parts, faint.Render("  ·  "))
}

func truncate(s string, n int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	r := []rune(s)
	if n <= 1 || len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

// shortPath writes the home folder as ~ and cuts the start of the path to n characters.
func shortPath(p string, n int) string {
	if home, err := os.UserHomeDir(); err == nil && strings.HasPrefix(p, home+"/") {
		p = "~" + p[len(home):]
	}
	r := []rune(p)
	if n < 2 || len(r) <= n {
		return p
	}
	return "…" + string(r[len(r)-n+1:])
}

func plural(n int, one, many string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, one)
	}
	return fmt.Sprintf("%d %s", n, many)
}

func onOff(b bool) string {
	if b {
		return "on"
	}
	return "off"
}
