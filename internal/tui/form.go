package tui

import (
	"strings"

	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// The form engine draws a whole form at one time, as a web form does: a label
// column, input boxes, segmented controls, text areas and
// a row of buttons. The keyboard moves between the fields; the mouse clicks them.

type fieldKind int

const (
	kindText fieldKind = iota
	kindSecret
	kindChoice // a segmented control; an on/off switch is a choice of "on" and "off"
	kindArea
	kindNote // text only; it does not take the focus
)

const labelWidth = 16

var (
	inputBg     = lipgloss.Color("#2A2740")
	focusBg     = lipgloss.Color("#3A3360")
	inputStyle  = lipgloss.NewStyle().Background(inputBg).Foreground(fgColor).Padding(0, 1)
	focusInput  = lipgloss.NewStyle().Background(focusBg).Foreground(lipgloss.Color("#FFFDF5")).Padding(0, 1)
	segOn       = lipgloss.NewStyle().Background(accent).Foreground(lipgloss.Color("#FFFDF5")).Bold(true).Padding(0, 1)
	segOff      = lipgloss.NewStyle().Background(inputBg).Foreground(mutedFg).Padding(0, 1)
	btnPrimary  = lipgloss.NewStyle().Background(accent).Foreground(lipgloss.Color("#FFFDF5")).Bold(true).Padding(0, 2)
	btnNormal   = lipgloss.NewStyle().Background(lipgloss.Color("#3A3A4E")).Foreground(fgColor).Padding(0, 2)
	btnDanger   = lipgloss.NewStyle().Background(lipgloss.Color("#5A1E26")).Foreground(lipgloss.Color("#FFD7DC")).Padding(0, 2)
	focusMark   = lipgloss.NewStyle().Foreground(accent).Bold(true)
	labelFocus  = lipgloss.NewStyle().Foreground(lipgloss.Color("#B9A8FF")).Bold(true)
	labelNormal = lipgloss.NewStyle().Foreground(mutedFg)
)

type formField struct {
	key, label, help string
	kind             fieldKind
	input            textinput.Model
	area             textarea.Model
	opts, labels     []string
	sel              int
	note             string
	noteStyle        lipgloss.Style
	hidden           func() bool
	validate         func(string) error
	err              string
	initial          string
}

func textField(key, label, value, placeholder, help string, validate func(string) error) *formField {
	in := textinput.New()
	in.Prompt = ""
	// The text has the background of the box, so the whole box is filled.
	st := textinput.DefaultStyles(true)
	st.Focused.Text = lipgloss.NewStyle().Background(focusBg).Foreground(lipgloss.Color("#FFFDF5"))
	st.Focused.Placeholder = lipgloss.NewStyle().Background(focusBg).Foreground(mutedFg)
	st.Blurred.Text = lipgloss.NewStyle().Background(inputBg).Foreground(fgColor)
	st.Blurred.Placeholder = lipgloss.NewStyle().Background(inputBg).Foreground(mutedFg)
	in.SetStyles(st)
	in.Placeholder = placeholder
	in.SetValue(value)
	in.CharLimit = 500
	return &formField{key: key, label: label, help: help, kind: kindText, input: in, validate: validate, initial: value}
}

func secretField(key, label, placeholder, help string, validate func(string) error) *formField {
	f := textField(key, label, "", placeholder, help, validate)
	f.kind = kindSecret
	f.input.EchoMode = textinput.EchoPassword
	f.input.EchoCharacter = '•'
	return f
}

func choiceField(key, label, help string, opts, labels []string, value string) *formField {
	sel := 0
	for i, o := range opts {
		if o == value {
			sel = i
		}
	}
	if labels == nil {
		labels = opts
	}
	return &formField{key: key, label: label, help: help, kind: kindChoice, opts: opts, labels: labels, sel: sel, initial: opts[sel]}
}

func switchField(key, label, help string, on bool) *formField {
	return choiceField(key, label, help, []string{"on", "off"}, []string{"On", "Off"}, onOff(on))
}

func areaField(key, label, value, help string, lines int, validate func(string) error) *formField {
	a := textarea.New()
	a.ShowLineNumbers = false
	a.Prompt = ""
	a.CharLimit = 8000
	a.SetHeight(lines)
	a.SetValue(value)
	return &formField{key: key, label: label, help: help, kind: kindArea, area: a, validate: validate, initial: value}
}

func noteField(text string, st lipgloss.Style) *formField {
	return &formField{kind: kindNote, note: text, noteStyle: st}
}

func (f *formField) value() string {
	switch f.kind {
	case kindText, kindSecret:
		return strings.TrimSpace(f.input.Value())
	case kindArea:
		return f.area.Value()
	case kindChoice:
		return f.opts[f.sel]
	}
	return ""
}

func (f *formField) visible() bool { return f.hidden == nil || !f.hidden() }

func (f *formField) focusable() bool { return f.kind != kindNote && f.visible() }

// button is one button of a form. The first button is the primary action: ctrl+s runs it.
type button struct {
	label   string
	primary bool
	danger  bool
	run     func() tea.Cmd
}

// formZone is a clickable area of a rendered form, relative to the form origin.
type formZone struct {
	y, x0, x1 int
	item      int // index in items()
	opt       int // option of a choice, or -1
}

type form struct {
	fields  []*formField
	buttons []button
	focus   int  // index in items()
	active  bool // the form has the keyboard
	err     string
	busy    string // an action runs: the text replaces the buttons
	scroll  int
	zones   []formZone
}

// item is one focusable part of a form: a field or a button.
type item struct {
	field  *formField
	button int
}

func (fm *form) items() []item {
	var out []item
	for _, f := range fm.fields {
		if f.focusable() {
			out = append(out, item{field: f, button: -1})
		}
	}
	for i := range fm.buttons {
		out = append(out, item{button: i})
	}
	return out
}

func (fm *form) field(key string) *formField {
	for _, f := range fm.fields {
		if f.key == key {
			return f
		}
	}
	return nil
}

func (fm *form) current() item {
	its := fm.items()
	fm.focus = min(max(0, fm.focus), len(its)-1)
	return its[fm.focus]
}

// dirty reports whether a field differs from its first value.
func (fm *form) dirty() bool {
	for _, f := range fm.fields {
		if f.kind != kindNote && f.value() != f.initial {
			return true
		}
	}
	return false
}

// setFocus moves the focus to item i and gives the keyboard to its input.
func (fm *form) setFocus(i int) tea.Cmd {
	its := fm.items()
	if len(its) == 0 {
		return nil
	}
	for _, it := range its {
		if it.field != nil {
			it.field.input.Blur()
			it.field.area.Blur()
		}
	}
	fm.focus = min(max(0, i), len(its)-1)
	if !fm.active {
		return nil
	}
	f := its[fm.focus].field
	if f == nil {
		return nil
	}
	switch f.kind {
	case kindText, kindSecret:
		f.input.CursorEnd()
		return f.input.Focus()
	case kindArea:
		return f.area.Focus()
	}
	return nil
}

func (fm *form) activate() tea.Cmd {
	fm.active = true
	return fm.setFocus(fm.focus)
}

func (fm *form) deactivate() {
	fm.active = false
	fm.setFocus(fm.focus)
}

func (fm *form) next() tea.Cmd { return fm.setFocus((fm.focus + 1) % len(fm.items())) }

func (fm *form) prev() tea.Cmd {
	n := len(fm.items())
	return fm.setFocus((fm.focus + n - 1) % n)
}

// validate checks all visible fields. It moves the focus to the first field
// with an error and reports whether the form is valid.
func (fm *form) validate() bool {
	first := -1
	for i, it := range fm.items() {
		f := it.field
		if f == nil {
			continue
		}
		f.err = ""
		if f.validate != nil {
			if err := f.validate(f.value()); err != nil {
				f.err = err.Error()
				if first < 0 {
					first = i
				}
			}
		}
	}
	if first >= 0 {
		fm.setFocus(first)
		fm.err = "Correct the fields in red."
		return false
	}
	fm.err = ""
	return true
}

// submit runs the primary button.
func (fm *form) submit() tea.Cmd {
	for _, b := range fm.buttons {
		if b.primary {
			return b.run()
		}
	}
	return nil
}

// formEsc is returned when esc leaves the form.
type formEsc struct{}

// key handles a key while the form has the keyboard.
func (fm *form) key(msg tea.KeyPressMsg) tea.Cmd {
	if fm.busy != "" {
		return nil
	}
	it := fm.current()
	f := it.field
	k := msg.String()
	switch k {
	case "ctrl+s":
		return fm.submit()
	case "tab":
		return fm.next()
	case "shift+tab":
		return fm.prev()
	case "esc":
		return func() tea.Msg { return formEsc{} }
	}
	if f == nil { // a button
		switch k {
		case "enter", "space":
			return fm.buttons[it.button].run()
		case "left", "up":
			return fm.prev()
		case "right", "down":
			return fm.next()
		}
		return nil
	}
	switch f.kind {
	case kindText, kindSecret:
		switch k {
		case "enter":
			// Enter in the last field before the buttons submits, as in a web form.
			if nx := fm.items()[(fm.focus+1)%len(fm.items())]; nx.field == nil {
				return fm.submit()
			}
			return fm.next()
		case "down":
			return fm.next()
		case "up":
			return fm.prev()
		}
		var cmd tea.Cmd
		f.input, cmd = f.input.Update(msg)
		if f.err != "" && f.validate != nil && f.validate(f.value()) == nil {
			f.err = ""
		}
		return cmd
	case kindChoice:
		switch k {
		case "left", "h":
			f.sel = (f.sel + len(f.opts) - 1) % len(f.opts)
		case "right", "l", "space":
			f.sel = (f.sel + 1) % len(f.opts)
		case "y":
			if f.opts[0] == "on" {
				f.sel = 0
			}
		case "n":
			if f.opts[0] == "on" {
				f.sel = 1
			}
		case "enter", "down":
			return fm.next()
		case "up":
			return fm.prev()
		}
		return nil
	case kindArea:
		var cmd tea.Cmd
		f.area, cmd = f.area.Update(msg)
		return cmd
	}
	return nil
}

// click handles a mouse click at x, y relative to the form origin.
func (fm *form) click(x, y int) tea.Cmd {
	for _, z := range fm.zones {
		if y != z.y || x < z.x0 || x >= z.x1 {
			continue
		}
		cmd := fm.setFocus(z.item)
		it := fm.current()
		switch {
		case it.field == nil:
			return tea.Batch(cmd, fm.buttons[it.button].run())
		case z.opt >= 0:
			it.field.sel = z.opt
		}
		return cmd
	}
	return nil
}

// render draws the form in w columns and at most h rows. It keeps the focused
// item visible and records the click zones.
func (fm *form) render(w, h int) string {
	lines := []string{""}
	fm.zones = fm.zones[:0]
	var focusTop, focusBottom int
	its := fm.items()
	cur := its[min(fm.focus, len(its)-1)]
	ctlW := max(20, min(56, w-labelWidth-2))
	idx := 0 // index of the next focusable item

	for _, f := range fm.fields {
		if !f.visible() {
			continue
		}
		if f.kind == kindNote {
			lines = append(lines, strings.Split(f.noteStyle.Width(w).Render(f.note), "\n")...)
			lines = append(lines, "")
			continue
		}
		focused := fm.active && cur.field == f
		top := len(lines)
		mark, lab := "  ", labelNormal
		if focused {
			mark, lab = focusMark.Render("▌ "), labelFocus
		}
		label := mark + lab.Width(labelWidth-2).Render(f.label)
		switch f.kind {
		case kindText, kindSecret:
			st := inputStyle
			if focused {
				st = focusInput
			}
			// A field without the focus is one styled text, so the whole box has
			// its background. The focused field shows the text input with its cursor.
			view := f.input.View()
			if !focused {
				v := f.input.Value()
				if f.kind == kindSecret {
					v = strings.Repeat("•", len([]rune(v)))
				}
				view = truncate(v, ctlW-2)
				if v == "" {
					view = lipgloss.NewStyle().Foreground(mutedFg).Render(truncate(f.input.Placeholder, ctlW-2))
				}
			}
			// The box pads the text with its background. The input pads without a
			// background, so it gets a width only when the text needs to scroll.
			if len([]rune(f.input.Value())) >= ctlW-3 {
				f.input.SetWidth(ctlW - 3)
			} else {
				f.input.SetWidth(0)
			}
			lines = append(lines, label+st.Width(ctlW).Render(view))
			fm.zones = append(fm.zones, formZone{y: top, x0: 0, x1: labelWidth + ctlW, item: idx, opt: -1})
		case kindChoice:
			x := labelWidth
			var segs []string
			for i, l := range f.labels {
				st := segOff
				if i == f.sel {
					st = segOn
				}
				s := st.Render(l)
				fm.zones = append(fm.zones, formZone{y: top, x0: x, x1: x + lipgloss.Width(s), item: idx, opt: i})
				x += lipgloss.Width(s)
				segs = append(segs, s)
			}
			line := label + strings.Join(segs, "")
			if focused {
				line += faint.Render("  ←/→")
			}
			lines = append(lines, line)
		case kindArea:
			lines = append(lines, label+faint.Render(f.help))
			f.area.SetWidth(max(20, w-4))
			for _, l := range strings.Split(f.area.View(), "\n") {
				bar := faint.Render("  │ ")
				if focused {
					bar = focusMark.Render("  │ ")
				}
				lines = append(lines, bar+l)
			}
			for y := top; y < len(lines); y++ {
				fm.zones = append(fm.zones, formZone{y: y, x0: 0, x1: w, item: idx, opt: -1})
			}
		}
		switch {
		case f.err != "":
			lines = append(lines, strings.Repeat(" ", labelWidth)+errStyle.Render("✗ "+f.err))
		case focused && f.help != "" && f.kind != kindArea:
			for _, l := range strings.Split(faint.Width(max(10, w-labelWidth)).Render(f.help), "\n") {
				lines = append(lines, strings.Repeat(" ", labelWidth)+l)
			}
		}
		if focused {
			focusTop, focusBottom = top, len(lines)
		}
		lines = append(lines, "")
		idx++
	}

	// Buttons
	top := len(lines)
	if fm.busy != "" {
		lines = append(lines, strings.Repeat(" ", labelWidth)+infoStyle.Render(fm.busy))
	} else {
		x := labelWidth
		var bs []string
		for i, b := range fm.buttons {
			st := btnNormal
			switch {
			case b.primary:
				st = btnPrimary
			case b.danger:
				st = btnDanger
			}
			text := b.label
			if fm.active && cur.field == nil && cur.button == i {
				text = "▸ " + text
				st = st.Underline(true)
			}
			s := st.Render(text)
			fm.zones = append(fm.zones, formZone{y: top, x0: x, x1: x + lipgloss.Width(s), item: idx + i, opt: -1})
			bs = append(bs, s)
			x += lipgloss.Width(s) + 2
		}
		lines = append(lines, strings.Repeat(" ", labelWidth)+strings.Join(bs, "  "))
	}
	if fm.active && cur.field == nil {
		focusTop, focusBottom = top, top+1
	}
	if fm.err != "" {
		for _, l := range strings.Split(errStyle.Width(max(10, w-labelWidth)).Render(fm.err), "\n") {
			lines = append(lines, strings.Repeat(" ", labelWidth)+l)
		}
	}

	// Scroll so that the focused item is visible.
	if len(lines) <= h {
		fm.scroll = 0
	} else {
		if focusBottom > fm.scroll+h {
			fm.scroll = focusBottom - h
		}
		if focusTop < fm.scroll {
			fm.scroll = focusTop
		}
		fm.scroll = min(fm.scroll, len(lines)-h)
		lines = lines[fm.scroll : fm.scroll+h]
		for i := range fm.zones {
			fm.zones[i].y -= fm.scroll
		}
	}
	return strings.Join(lines, "\n")
}
