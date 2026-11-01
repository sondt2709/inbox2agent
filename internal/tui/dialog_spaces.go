package tui

import (
	"fmt"
	"slices"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/sondt2709/inbox2agent/internal/config"
)

// spaceOpt is one space in the spaces dialog.
type spaceOpt struct {
	id, title string
	on        bool   // checked
	level     string // all or mentions
	was       bool   // the first conversation was in this space when the dialog opened
}

// spacesDialog sets the spaces of one or many conversations: a check mark and
// a read level for each space, or Ignore. Enter saves.
type spacesDialog struct {
	rows    []convRow
	opts    []spaceOpt
	ignore  bool
	requeue bool
	cursor  int
	err     string
}

func (m *model) newSpacesDialog(rows []convRow) *spacesDialog {
	d := &spacesDialog{rows: rows}
	first := rows[0]
	for _, s := range m.u.cfg.Spaces {
		lv, ok := levelIn(first, s.ID)
		if !ok || inNoSpace(first) {
			lv, ok = config.ReadAll, false
		}
		d.opts = append(d.opts, spaceOpt{id: s.ID, title: s.Title(), on: ok, level: lv, was: ok})
	}
	d.ignore = inNoSpace(first) && first.Read == config.ReadOff
	return d
}

func (d *spacesDialog) title() string {
	if len(d.rows) == 1 {
		r := d.rows[0]
		return kindMark(r.Kind) + " " + r.Title() + " · " + r.Account.Title()
	}
	return fmt.Sprintf("Spaces of %d conversations", len(d.rows))
}

// added is true when a checked space was not a space of the conversation.
func (d *spacesDialog) added() bool {
	for _, o := range d.opts {
		if o.on && !o.was {
			return true
		}
	}
	return false
}

// The rows after the spaces: Ignore, + New space and, when a space is added,
// the requeue option.
func (d *spacesDialog) n() int {
	n := len(d.opts) + 2
	if d.added() {
		n++
	}
	return n
}

func (d *spacesDialog) size(W, H int) (int, int) { return min(W-4, 76), min(H-4, d.n()+10) }

func (d *spacesDialog) update(m *model, msg tea.Msg) tea.Cmd {
	k, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return nil
	}
	ignoreRow, newRow, requeueRow := len(d.opts), len(d.opts)+1, len(d.opts)+2
	switch k.String() {
	case "esc":
		m.closeDialog()
	case "up", "k":
		d.cursor = max(0, d.cursor-1)
	case "down", "j":
		d.cursor = min(d.n()-1, d.cursor+1)
	case "space", "x":
		switch {
		case d.cursor < len(d.opts):
			d.opts[d.cursor].on = !d.opts[d.cursor].on
			if d.opts[d.cursor].on {
				d.ignore = false
			}
		case d.cursor == ignoreRow:
			d.ignore = !d.ignore
			if d.ignore {
				for i := range d.opts {
					d.opts[i].on = false
				}
			}
		case d.cursor == newRow:
			return d.newSpace(m)
		case d.cursor == requeueRow:
			d.requeue = !d.requeue
		}
		d.cursor = min(d.cursor, d.n()-1)
	case "left", "right", "h", "l":
		if d.cursor < len(d.opts) {
			o := &d.opts[d.cursor]
			if o.level == config.ReadAll {
				o.level = config.ReadMentions
			} else {
				o.level = config.ReadAll // also from off, which a file can have
			}
			o.on, d.ignore = true, false
		}
	case "enter":
		if d.cursor == newRow {
			return d.newSpace(m)
		}
		return d.save(m)
	}
	return nil
}

func (d *spacesDialog) newSpace(m *model) tea.Cmd {
	m.addSpaceDialog(func(id string) {
		s, _ := m.u.cfg.Space(id)
		d.opts = append(d.opts, spaceOpt{id: id, title: s.Title(), on: true, level: config.ReadAll})
		d.ignore = false
		d.cursor = len(d.opts) - 1
	})
	return nil
}

func (d *spacesDialog) save(m *model) tea.Cmd {
	var links []config.Link
	var names []string
	for _, o := range d.opts {
		if o.on {
			links = append(links, config.Link{Space: o.id, Read: o.level})
			names = append(names, o.title)
		}
	}
	done := what(d.rows) + " is in " + strings.Join(names, ", ") + "."
	switch {
	case d.ignore:
		links = []config.Link{{Space: config.UnsortedID, Read: config.ReadOff}}
		done = "Ignored " + what(d.rows) + "."
	case len(links) == 0:
		done = what(d.rows) + " is in Everything else."
	}
	ok := m.editLinks(d.rows, func(c *config.Config, r convRow) []config.Link {
		if len(links) == 0 {
			return noSpace(c, r)
		}
		return links
	}, done)
	if !ok {
		return nil
	}
	m.closeDialog()
	if d.requeue && d.added() {
		return m.requeue(d.rows)
	}
	return nil
}

// noSpace returns the links of a row that is in no space and not ignored: its
// own level of Everything else, or none, so it follows unsorted.read. When
// unsorted.read is off, no entry would ignore it, so it gets "Only for me".
func noSpace(c *config.Config, r convRow) []config.Link {
	if cur, sp, had := c.Conversation(r.Account.ID, r.ID); had && sp == config.UnsortedID && cur.Read != config.ReadOff {
		return []config.Link{{Space: config.UnsortedID, Read: cur.Read}}
	}
	if c.Unsorted.Read == config.ReadOff {
		return []config.Link{{Space: config.UnsortedID, Read: config.ReadMentions}}
	}
	return nil
}

// sameSpaces is true when each row has the spaces of the first row.
func sameSpaces(rows []convRow) bool {
	for _, r := range rows[1:] {
		if !slices.Equal(r.Spaces, rows[0].Spaces) || inNoSpace(r) != inNoSpace(rows[0]) {
			return false
		}
	}
	return true
}

func (d *spacesDialog) view(w, h int) string {
	var lines []string
	if len(d.rows) > 1 {
		lines = append(lines, faint.Render("The same spaces for each of the "+fmt.Sprint(len(d.rows))+" conversations."))
		if !sameSpaces(d.rows) {
			lines = append(lines, warnStyle.Render("They are in different spaces now. The marks show the first one; enter gives all of them these spaces."))
		}
		lines = append(lines, "")
	}
	lines = append(lines, boldStyle.Render("Spaces"))
	row := func(i int, s string) string {
		if i == d.cursor {
			return selectedRow.Width(w).Render(truncate("▌ "+ansi.Strip(s), w))
		}
		return "  " + s
	}
	for i, o := range d.opts {
		box := "[ ]"
		if o.on {
			box = okStyle.Render("[x]")
		}
		lv := faint.Render(levelText(o.level))
		if o.on {
			lv = textStyle.Render(levelText(o.level))
		}
		lines = append(lines, row(i, fmt.Sprintf("%s %-24s %s", box, truncate(o.title, 24), lv)))
	}
	if len(d.opts) == 0 {
		lines = append(lines, faint.Render("  No space yet."))
	}
	none := true
	for _, o := range d.opts {
		none = none && !o.on
	}
	if none && !d.ignore {
		lines = append(lines, faint.Render("  In no space: Everything else reads it."))
	}
	lines = append(lines, "")
	box := "( )"
	if d.ignore {
		box = warnStyle.Render("(x)")
	}
	lines = append(lines, row(len(d.opts), box+" Ignore: never read this conversation"))
	lines = append(lines, row(len(d.opts)+1, "+ New space …"))
	if d.added() {
		box := "[ ]"
		if d.requeue {
			box = okStyle.Render("[x]")
		}
		lines = append(lines, "", row(len(d.opts)+2, box+" Also send the messages of the last lookback to the new spaces"))
	}
	if d.err != "" {
		lines = append(lines, "", errStyle.Render(d.err))
	}
	return strings.Join(lines, "\n")
}

func (d *spacesDialog) hints() []hint {
	return []hint{{"space", "check"}, {"←→", "read level"}, {"enter", "save"}, {"esc", "cancel"}}
}

// requeue sends the messages of the last lookback of the rows again, to the
// spaces that each row got in this save only. The spaces that a row had
// before get a skipped delivery, so they do not get the messages.
func (m *model) requeue(rows []convRow) tea.Cmd {
	st := m.u.d.Store
	if st == nil {
		m.setStatus("No store yet: there are no old messages to send.", false)
		return nil
	}
	type job struct {
		account, id string
		forMe       bool
		skip        []string
	}
	var jobs []job
	for _, r := range rows {
		j := job{account: r.Account.ID, id: r.ID, forMe: true}
		added := false
		for _, l := range realLinks(m.u.cfg, r.Account.ID, r.ID) {
			if _, was := levelIn(r, l.Space); was && !inNoSpace(r) {
				j.skip = append(j.skip, l.Space)
				continue
			}
			added = true
			j.forMe = j.forMe && l.Read == config.ReadMentions
		}
		if added {
			jobs = append(jobs, j)
		}
	}
	lookback := m.u.cfg.Settings.Lookback
	since := m.now().Add(-lookback.D())
	ctx := m.ctx
	return func() tea.Msg {
		var n int64
		for _, j := range jobs {
			k, err := st.Requeue(ctx, j.account, j.id, since, j.forMe, j.skip)
			if err != nil {
				return statusMsg{text: "Cannot send the old messages again: " + err.Error(), isErr: true}
			}
			n += k
		}
		return statusMsg{text: fmt.Sprintf("Saved. %s of the last %s wait for the new spaces.", plural(int(n), "message", "messages"), lookback)}
	}
}

// ---- search ----

// searchDialog finds a conversation of any account. With a space, each row
// shows if the space has it, space adds or removes it, and ←→ change its
// level there. Without a space, enter opens the spaces dialog of the row.
type searchDialog struct {
	m      *model
	space  string // "" to find a conversation
	input  textinput.Model
	typing bool
	cursor int
	off    int
}

func (m *model) newSearchDialog(space string) *searchDialog {
	in := textinput.New()
	in.Prompt = "/ "
	in.Placeholder = "name, account or space"
	in.CharLimit = 60
	in.Focus()
	return &searchDialog{m: m, space: space, input: in, typing: true}
}

func (d *searchDialog) title() string { return "Find a conversation" }

func (d *searchDialog) size(W, H int) (int, int) { return min(W-4, 96), H - 4 }

func (d *searchDialog) rows(m *model) []convRow {
	q := strings.ToLower(strings.TrimSpace(d.input.Value()))
	return m.rowsWhere(func(r convRow) bool {
		return q == "" || strings.Contains(strings.ToLower(r.Title()+" "+r.Name+" "+r.Account.Title()+" "+m.spacesText(r)), q)
	})
}

func (d *searchDialog) update(m *model, msg tea.Msg) tea.Cmd {
	k, ok := msg.(tea.KeyPressMsg)
	if !ok {
		if d.typing {
			var cmd tea.Cmd
			d.input, cmd = d.input.Update(msg)
			return cmd
		}
		return nil
	}
	rows := d.rows(m)
	if d.typing {
		switch k.String() {
		case "esc":
			if d.input.Value() != "" {
				d.input.SetValue("")
				return nil
			}
			m.closeDialog()
			return nil
		case "enter", "down", "tab":
			d.typing = false
			d.input.Blur()
			d.cursor = 0
			if k.String() == "enter" && d.space == "" && len(rows) == 1 {
				m.closeDialog()
				m.openDialog(m.newSpacesDialog(rows[:1]))
			}
			return nil
		}
		var cmd tea.Cmd
		d.input, cmd = d.input.Update(msg)
		d.cursor = 0
		return cmd
	}
	switch k.String() {
	case "esc":
		m.closeDialog()
	case "/":
		d.typing = true
		return d.input.Focus()
	case "up", "k":
		if d.cursor == 0 {
			d.typing = true
			return d.input.Focus()
		}
		d.cursor--
	case "down", "j":
		d.cursor = min(max(0, len(rows)-1), d.cursor+1)
	case "enter":
		if d.cursor >= len(rows) {
			return nil
		}
		if d.space == "" {
			m.closeDialog()
			m.openDialog(m.newSpacesDialog(rows[d.cursor : d.cursor+1]))
			return nil
		}
		m.closeDialog()
	case "space", "x":
		if d.space != "" && d.cursor < len(rows) {
			d.toggle(m, rows[d.cursor])
		}
	case "left", "right", "h", "l":
		if d.space != "" && d.cursor < len(rows) {
			if _, in := levelIn(rows[d.cursor], d.space); in && !inNoSpace(rows[d.cursor]) {
				m.toggleLevel(rows[d.cursor:d.cursor+1], d.space)
			}
		}
	}
	return nil
}

// toggle adds the row to the space of the dialog, or removes it.
func (d *searchDialog) toggle(m *model, r convRow) {
	s, _ := m.u.cfg.Space(d.space)
	_, in := levelIn(r, d.space)
	in = in && !inNoSpace(r)
	if in {
		m.editLinks([]convRow{r}, func(c *config.Config, x convRow) []config.Link {
			return slices.DeleteFunc(realLinks(c, x.Account.ID, x.ID), func(l config.Link) bool { return l.Space == d.space })
		}, fmt.Sprintf("Removed %s from %s.", r.Title(), s.Title()))
		return
	}
	m.editLinks([]convRow{r}, func(c *config.Config, x convRow) []config.Link {
		return append(realLinks(c, x.Account.ID, x.ID), config.Link{Space: d.space, Read: config.ReadAll})
	}, fmt.Sprintf("Added %s to %s.", r.Title(), s.Title()))
}

func (d *searchDialog) view(w, h int) string { return d.render(d.m, w, h) }

func (d *searchDialog) render(m *model, w, h int) string {
	rows := d.rows(m)
	head := d.input.View()
	if d.space != "" {
		s, _ := m.u.cfg.Space(d.space)
		head = faint.Render("Add to "+s.Title()+" · ") + head
	}
	lines := []string{head, ""}
	listH := max(1, h-4)
	d.cursor = min(d.cursor, max(0, len(rows)-1))
	d.off = scrollTo(d.cursor, d.off, listH)
	end := min(len(rows), d.off+listH)
	nameW := max(14, w/3)
	for i := d.off; i < end; i++ {
		r := rows[i]
		box := ""
		if d.space != "" {
			box = "[ ] "
			if _, in := levelIn(r, d.space); in && !inNoSpace(r) {
				box = okStyle.Render("[x]") + " "
			}
		}
		where := m.spacesText(r)
		if inNoSpace(r) {
			where = "Everything else"
			if r.Read == config.ReadOff {
				where = "ignored"
			}
		}
		if lv, in := levelIn(r, d.space); d.space != "" && in && !inNoSpace(r) {
			where = levelText(lv) + faint.Render(" · ") + where
		}
		text := fmt.Sprintf("%s%s %-*s %-12s %s  %s", box, kindMark(r.Kind), nameW, truncate(r.Title(), nameW),
			truncate(r.Account.Title(), 12), where, faint.Render(m.lastText(r.Last)))
		if i == d.cursor && !d.typing {
			lines = append(lines, selectedRow.Width(w).Render(truncate("▌ "+ansi.Strip(text), w)))
		} else {
			lines = append(lines, "  "+text)
		}
	}
	if len(rows) == 0 {
		lines = append(lines, faint.Render("  No conversation matches."))
	}
	lines = append(lines, "", faint.Render(fmt.Sprintf("newest message first · %d of %d", len(rows), len(m.convRows()))))
	return strings.Join(lines, "\n")
}

func (d *searchDialog) hints() []hint {
	if d.typing {
		return []hint{{"type", "search"}, {"enter ↓", "to the list"}, {"esc", "clear or close"}}
	}
	if d.space != "" {
		return []hint{{"space", "add or remove"}, {"←→", "read level"}, {"/", "search"}, {"esc", "close"}}
	}
	return []hint{{"enter", "set its spaces"}, {"/", "search"}, {"esc", "close"}}
}
