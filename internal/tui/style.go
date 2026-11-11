package tui

import (
	"fmt"
	"os"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

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
	tabActive    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FFFDF5")).Background(accent).Padding(0, 1)
	tabInactive  = lipgloss.NewStyle().Foreground(mutedFg).Padding(0, 1)
	tabKey       = lipgloss.NewStyle().Foreground(accent).Bold(true)
	okStyle      = lipgloss.NewStyle().Foreground(green)
	warnStyle    = lipgloss.NewStyle().Foreground(yellow)
	errStyle     = lipgloss.NewStyle().Foreground(red)
	infoStyle    = lipgloss.NewStyle().Foreground(blue)
	panelStyle   = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(borderFg).Padding(0, 1)
	panelTitle   = lipgloss.NewStyle().Bold(true).Foreground(accent)
	selectedRow  = lipgloss.NewStyle().Background(accentDim).Foreground(lipgloss.Color("#FFFDF5")).Bold(true)
	dialogStyle  = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(accent).Padding(0, 1)
	dialogTitle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FFFDF5")).Background(accent).Padding(0, 1)
	dirtyStyle   = lipgloss.NewStyle().Foreground(yellow).Bold(true)
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

// hint is one action in the footer: its key and its label.
type hint struct{ key, desc string }

var (
	hintLabel = lipgloss.NewStyle().Foreground(accent).Bold(true) // the primary color
	hintKey   = lipgloss.NewStyle().Foreground(mutedFg)           // the secondary color
)

// chipBg is the background of a button in the header and the footer.
var chipBg = lipgloss.Color("#26232F")

var (
	chipPad   = lipgloss.NewStyle().Background(chipBg)
	chipLabel = hintLabel.Background(chipBg)
	chipKey   = hintKey.Background(chipBg)
)

// moreKey is the key of the More… button: it opens the list of all the
// actions of the screen. No keyboard key has this name.
const moreKey = "more…"

// barZone is a button of the header or the footer: a click sends its key.
type barZone struct {
	x0, x1, y int
	key       string
}

// keyName returns the key name that keyPress knows for the key of a hint.
// A hint with two keys or a word ("↑↓", "pgup/pgdn", "type") has none, and
// stays text.
func keyName(k string) (string, bool) {
	switch k {
	case "←":
		return "left", true
	case "→":
		return "right", true
	case "↑":
		return "up", true
	case "↓":
		return "down", true
	case "enter", "esc", "space", "tab", "shift+tab", "pgup", "pgdown", moreKey:
		return k, true
	}
	if len(k) == len("ctrl+x") && strings.HasPrefix(k, "ctrl+") {
		return k, true
	}
	if utf8.RuneCountInString(k) == 1 && k != " " {
		return k, true
	}
	return "", false
}

// renderHint writes one hint as the label, then the key: "Reply r". A button
// is a chip with a dim background.
func renderHint(h hint) string {
	if h.key == moreKey {
		return chipPad.Render(" ") + chipLabel.Render(h.desc) + chipPad.Render(" ")
	}
	if _, ok := keyName(h.key); ok {
		return chipPad.Render(" ") + chipLabel.Render(upperFirst(h.desc)) + chipPad.Render(" ") + chipKey.Render(h.key) + chipPad.Render(" ")
	}
	return hintLabel.Render(upperFirst(h.desc)) + " " + hintKey.Render(h.key)
}

// layoutHints writes the hints from column x of row y. It returns the text
// and the zone of each button, so the drawing and the clicks cannot differ.
func layoutHints(hs []hint, x, y int) (string, []barZone) {
	var b strings.Builder
	var zs []barZone
	for i, h := range hs {
		if i > 0 {
			b.WriteString("  ")
			x += 2
		}
		s := renderHint(h)
		w := lipgloss.Width(s)
		if k, ok := keyName(h.key); ok {
			zs = append(zs, barZone{x0: x, x1: x + w, y: y, key: k})
		}
		b.WriteString(s)
		x += w
	}
	return b.String(), zs
}

// renderHints writes the hints with no click zones.
func renderHints(hs []hint) string {
	s, _ := layoutHints(hs, 0, 0)
	return s
}

// upperFirst writes the first letter of s as a capital letter.
func upperFirst(s string) string {
	r := []rune(s)
	if len(r) == 0 {
		return s
	}
	r[0] = unicode.ToUpper(r[0])
	return string(r)
}

// fitHints returns the hints that fit in w columns, in their order. It drops
// whole hints from the end and never cuts a hint. When it drops a button, the
// last hint is More…, which lists all of them.
func fitHints(hs []hint, w int) []hint {
	if lipgloss.Width(renderHints(hs)) <= w {
		return hs
	}
	for n := len(hs) - 1; n >= 0; n-- {
		out := slices.Clone(hs[:n])
		if slices.ContainsFunc(hs[n:], func(h hint) bool { _, ok := keyName(h.key); return ok }) {
			out = append(out, hint{moreKey, "More…"})
		}
		if lipgloss.Width(renderHints(out)) <= w {
			return out
		}
	}
	return nil
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
