package tui

import (
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
	"charm.land/lipgloss/v2"
)

// pickItem is one channel in the picker.
type pickItem struct {
	label    string
	id       string
	selected bool
}

// picker is a Huh field that selects channels from a long list. The search box
// always has the focus: the user types to search, moves with the arrow keys,
// and selects with space. Enter goes to the next field.
type picker struct {
	title   string
	items   []pickItem
	query   string
	shown   []int // indexes in items that match the query
	cursor  int   // index in shown
	offset  int   // first visible row of shown
	rows    int
	width   int
	focused bool
	value   *[]string
	theme   huh.Theme
	// hasDarkBg selects the theme variant. It starts false and changes only
	// when the terminal reports its background, as in the Huh fields, so the
	// picker has the same colors as the fields around it.
	hasDarkBg bool
}

func newPicker(title string, items []pickItem, rows int, value *[]string) *picker {
	// Selected channels first, so the user sees the current choice.
	slices.SortStableFunc(items, func(a, b pickItem) int {
		switch {
		case a.selected == b.selected:
			return 0
		case a.selected:
			return -1
		}
		return 1
	})
	p := &picker{title: title, items: items, rows: max(3, rows), value: value, theme: huh.ThemeFunc(huh.ThemeCharm)}
	p.filter()
	return p
}

// filter shows the items whose label contains the query, ignoring case.
func (p *picker) filter() {
	q := strings.ToLower(p.query)
	p.shown = p.shown[:0]
	for i, it := range p.items {
		if strings.Contains(strings.ToLower(it.label), q) {
			p.shown = append(p.shown, i)
		}
	}
	p.cursor, p.offset = 0, 0
}

func (p *picker) selectedIDs() []string {
	var ids []string
	for _, it := range p.items {
		if it.selected {
			ids = append(ids, it.id)
		}
	}
	return ids
}

func (p *picker) store() { *p.value = p.selectedIDs() }

func (p *picker) move(d int) {
	if len(p.shown) == 0 {
		return
	}
	p.cursor = min(max(p.cursor+d, 0), len(p.shown)-1)
	if p.cursor < p.offset {
		p.offset = p.cursor
	}
	if p.cursor >= p.offset+p.rows {
		p.offset = p.cursor - p.rows + 1
	}
}

// GetFiltering reports whether a search is active. Then Esc clears the
// search and does not leave the screen.
func (p *picker) GetFiltering() bool { return p.query != "" }

func (p *picker) Init() tea.Cmd { return nil }

func (p *picker) Update(msg tea.Msg) (huh.Model, tea.Cmd) {
	if bg, ok := msg.(tea.BackgroundColorMsg); ok {
		p.hasDarkBg = bg.IsDark()
		return p, nil
	}
	km, ok := msg.(tea.KeyPressMsg)
	if !ok || !p.focused {
		return p, nil
	}
	switch km.String() {
	case "enter":
		p.store()
		return p, huh.NextField
	case "shift+tab":
		p.store()
		return p, huh.PrevField
	case "up", "ctrl+p":
		p.move(-1)
	case "down", "ctrl+n":
		p.move(1)
	case "pgup":
		p.move(-p.rows)
	case "pgdown":
		p.move(p.rows)
	case "space":
		if len(p.shown) > 0 {
			it := &p.items[p.shown[p.cursor]]
			it.selected = !it.selected
			p.store()
		}
	case "ctrl+a":
		// Select all shown items, or clear them when all are selected.
		all := true
		for _, i := range p.shown {
			all = all && p.items[i].selected
		}
		for _, i := range p.shown {
			p.items[i].selected = !all
		}
		p.store()
	case "esc", "ctrl+u":
		p.query = ""
		p.filter()
	case "backspace":
		if r := []rune(p.query); len(r) > 0 {
			p.query = string(r[:len(r)-1])
			p.filter()
		}
	default:
		if km.Text != "" && !strings.ContainsAny(km.Text, " \t\r\n") {
			p.query += km.Text
			p.filter()
		}
	}
	return p, nil
}

func (p *picker) View() string {
	st := p.theme.Theme(p.hasDarkBg)
	fs := st.Blurred
	if p.focused {
		fs = st.Focused
	}
	var b strings.Builder
	if p.title != "" {
		b.WriteString(fs.Title.Render(p.title) + "\n")
	}
	cursor := "▏"
	if !p.focused {
		cursor = ""
	}
	count := fmt.Sprintf("%d of %d · %d selected", len(p.shown), len(p.items), len(p.selectedIDs()))
	b.WriteString(fs.Description.Render("Search: "+p.query+cursor+"   "+count) + "\n")
	if len(p.shown) == 0 {
		b.WriteString(fs.Description.Render("  No channel matches. Press Esc to clear the search.") + "\n")
	}
	end := min(p.offset+p.rows, len(p.shown))
	if p.offset > 0 {
		b.WriteString(fs.Description.Render(fmt.Sprintf("  ↑ %d more", p.offset)) + "\n")
	}
	for row := p.offset; row < end; row++ {
		it := p.items[p.shown[row]]
		// The theme styles hold the marks ("> ", "✓ ", "• ") as their text.
		sel := fs.MultiSelectSelector.String()
		line := strings.Repeat(" ", lipgloss.Width(sel))
		if row == p.cursor && p.focused {
			line = sel
		}
		if it.selected {
			line += fs.SelectedPrefix.String() + fs.SelectedOption.Render(it.label)
		} else {
			line += fs.UnselectedPrefix.String() + fs.UnselectedOption.Render(it.label)
		}
		b.WriteString(line + "\n")
	}
	if rest := len(p.shown) - end; rest > 0 {
		b.WriteString(fs.Description.Render(fmt.Sprintf("  ↓ %d more", rest)) + "\n")
	}
	return fs.Base.Width(p.width).Render(strings.TrimSuffix(b.String(), "\n"))
}

func (p *picker) Focus() tea.Cmd { p.focused = true; return nil }

func (p *picker) Blur() tea.Cmd { p.focused = false; p.store(); return nil }

func (p *picker) Error() error { return nil }

func (p *picker) Run() error { return errors.New("the channel picker runs only inside a form") }

func (p *picker) RunAccessible(io.Writer, io.Reader) error {
	return errors.New("the channel picker has no accessible mode; use i2a source add --channels")
}

func (p *picker) Skip() bool { return false }

func (p *picker) Zoom() bool { return false }

func (p *picker) KeyBinds() []key.Binding {
	return []key.Binding{
		key.NewBinding(key.WithKeys("a"), key.WithHelp("type", "search")),
		key.NewBinding(key.WithKeys("space"), key.WithHelp("space", "select")),
		key.NewBinding(key.WithKeys("up", "down"), key.WithHelp("↑/↓", "move")),
		key.NewBinding(key.WithKeys("ctrl+a"), key.WithHelp("ctrl+a", "select all shown")),
		key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "next")),
	}
}

func (p *picker) WithTheme(t huh.Theme) huh.Field {
	if t != nil {
		p.theme = t
	}
	return p
}

func (p *picker) WithKeyMap(*huh.KeyMap) huh.Field { return p }

func (p *picker) WithWidth(w int) huh.Field { p.width = w; return p }

func (p *picker) WithHeight(int) huh.Field { return p }

func (p *picker) WithPosition(huh.FieldPosition) huh.Field { return p }

func (p *picker) GetKey() string { return "channels" }

func (p *picker) GetValue() any { return p.selectedIDs() }
