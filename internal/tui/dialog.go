package tui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// dialog is a box over the dashboard. The top dialog gets all messages.
type dialog interface {
	title() string
	// size returns the outer size of the box for a window of W × H.
	size(W, H int) (int, int)
	// update handles a message. It can close the dialog with m.closeDialog.
	update(m *model, msg tea.Msg) tea.Cmd
	view(w, h int) string
	hints() []hint
}

// msgDialog shows a text. Enter or esc closes it.
type msgDialog struct {
	name, text string
	isErr      bool
}

func (d *msgDialog) title() string { return d.name }

func (d *msgDialog) size(W, H int) (int, int) {
	w := min(W-4, 76)
	lines := lipgloss.Height(lipgloss.NewStyle().Width(w - 4).Render(d.text))
	return w, min(H-4, lines+6)
}

func (d *msgDialog) update(m *model, msg tea.Msg) tea.Cmd {
	if k, ok := msg.(tea.KeyPressMsg); ok {
		switch k.String() {
		case "enter", "esc", "q", "space":
			m.closeDialog()
		}
	}
	return nil
}

func (d *msgDialog) view(w, _ int) string {
	st := textStyle
	if d.isErr {
		st = errStyle
	}
	return "\n" + st.Width(w).Render(d.text) + "\n\n" + faint.Render("Press enter to close.")
}

func (d *msgDialog) hints() []hint { return []hint{{"enter", "close"}} }

// choice is one key of a confirmation.
type choice struct {
	key, label string
	run        func() tea.Cmd
}

// confirmDialog asks a question and acts on one key. Esc cancels.
type confirmDialog struct {
	name, text string
	choices    []choice
}

func (d *confirmDialog) title() string { return d.name }

func (d *confirmDialog) size(W, H int) (int, int) { return min(W-4, 64), min(H-4, 10) }

func (d *confirmDialog) update(m *model, msg tea.Msg) tea.Cmd {
	k, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return nil
	}
	if k.String() == "esc" {
		m.closeDialog()
		return nil
	}
	for _, c := range d.choices {
		if k.String() == c.key {
			m.closeDialog()
			return c.run()
		}
	}
	return nil
}

func (d *confirmDialog) view(w, _ int) string {
	var keys []string
	for _, c := range d.choices {
		keys = append(keys, tabKey.Render("["+c.key+"]")+" "+textStyle.Render(c.label))
	}
	keys = append(keys, tabKey.Render("[esc]")+" "+textStyle.Render("Cancel"))
	return "\n" + textStyle.Width(w).Render(d.text) + "\n\n" + strings.Join(keys, "   ")
}

func (d *confirmDialog) hints() []hint {
	var hs []hint
	for _, c := range d.choices {
		hs = append(hs, hint{c.key, strings.ToLower(c.label)})
	}
	return append(hs, hint{"esc", "cancel"})
}

// busyDialog shows a spinner while an action runs. Esc cancels the action.
type busyDialog struct{ label string }

func (d *busyDialog) title() string { return "Please wait" }

func (d *busyDialog) size(W, H int) (int, int) { return min(W-4, 60), min(H-4, 7) }

func (d *busyDialog) update(m *model, msg tea.Msg) tea.Cmd {
	if k, ok := msg.(tea.KeyPressMsg); ok && (k.String() == "esc" || k.String() == "ctrl+c") {
		m.cancelBusy()
	}
	return nil
}

func (d *busyDialog) view(int, int) string { return "" } // the model draws the spinner

func (d *busyDialog) hints() []hint { return []hint{{"esc", "cancel"}} }

// viewerDialog shows a long text with scrolling.
type viewerDialog struct {
	name string
	vp   viewport.Model
	text string
}

func newViewer(name, text string) *viewerDialog {
	return &viewerDialog{name: name, text: text, vp: viewport.New()}
}

func (d *viewerDialog) title() string { return d.name }

func (d *viewerDialog) size(W, H int) (int, int) { return min(W-4, 120), H - 4 }

func (d *viewerDialog) update(m *model, msg tea.Msg) tea.Cmd {
	if k, ok := msg.(tea.KeyPressMsg); ok {
		switch k.String() {
		case "esc", "q", "enter":
			m.closeDialog()
			return nil
		case "j":
			d.vp.ScrollDown(1)
			return nil
		case "k":
			d.vp.ScrollUp(1)
			return nil
		}
	}
	var cmd tea.Cmd
	d.vp, cmd = d.vp.Update(msg)
	return cmd
}

func (d *viewerDialog) view(w, h int) string {
	if d.vp.Width() != w || d.vp.Height() != h-1 {
		d.vp.SetWidth(w)
		d.vp.SetHeight(h - 1)
		d.vp.SetContent(lipgloss.NewStyle().Width(w).Render(d.text))
	}
	pct := faint.Render(fmt.Sprintf("%*d%%", w-1, int(d.vp.ScrollPercent()*100)))
	return d.vp.View() + "\n" + pct
}

func (d *viewerDialog) hints() []hint {
	return []hint{{"↑↓", "scroll"}, {"pgup/pgdn", "page"}, {"esc", "close"}}
}
