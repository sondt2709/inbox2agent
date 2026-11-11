package tui

import (
	"fmt"
	"slices"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
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
	rowAt   map[int]int // the row of each body line, from the last view
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

// added is true when a checked space is not a space of one of the rows.
func (d *spacesDialog) added() bool {
	for _, o := range d.opts {
		if !o.on {
			continue
		}
		for _, r := range d.rows {
			if _, in := levelIn(r, o.id); !in || inNoSpace(r) {
				return true
			}
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
	if c, ok := msg.(tea.MouseClickMsg); ok {
		// A click on a row selects it and checks it, as space does.
		_, y := m.dialogPoint(d, c.Mouse())
		if i, ok := d.rowAt[y]; ok && c.Mouse().Button == tea.MouseLeft {
			d.cursor = i
			k, _ := keyPress("space")
			return d.update(m, k)
		}
		return nil
	}
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
	cur, sp, had := c.Conversation(r.Account.ID, r.ID)
	if had && sp == config.UnsortedID && cur.Read != config.ReadOff {
		return []config.Link{{Space: config.UnsortedID, Read: cur.Read}}
	}
	if c.Unsorted.Read == config.ReadOff {
		return []config.Link{{Space: config.UnsortedID, Read: config.ReadMentions}}
	}
	if had && cur.Label != "" {
		// An entry keeps the label; it has the level of unsorted.read.
		return []config.Link{{Space: config.UnsortedID, Read: c.Unsorted.Read}}
	}
	return nil
}

// orNoSpace returns links, or the links of noSpace when links is empty.
func orNoSpace(c *config.Config, r convRow, links []config.Link) []config.Link {
	if len(links) == 0 {
		return noSpace(c, r)
	}
	return links
}

// sameSpaces is true when each row has the spaces of the first row.
func sameSpaces(rows []convRow) bool {
	for _, r := range rows[1:] {
		if !slices.Equal(r.Spaces, rows[0].Spaces) || !slices.Equal(r.Reads, rows[0].Reads) || inNoSpace(r) != inNoSpace(rows[0]) {
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
	d.rowAt = map[int]int{}
	// row draws row i. Each call is the next line, so it records the line for a click.
	row := func(i int, s string) string {
		d.rowAt[len(lines)] = i
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
		lines = append(lines, "")
		lines = append(lines, row(len(d.opts)+2, box+" Also send the messages of the last lookback to the new spaces"))
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

// searchDialog finds a conversation of any account. Without a space, enter
// opens the spaces dialog of the row. With a space it is the add dialog: each
// row shows if the space has it, space adds or removes it, and ←→ change its
// level there. The changes wait in pending until OK saves them in one save;
// Cancel and esc forget them.
type searchDialog struct {
	m       *model
	space   string // "" to find a conversation
	input   textinput.Model
	typing  bool
	cursor  int
	off     int
	end     int                  // the index after the last visible row, from the last render
	pending map[string]spacePick // by convKey
	button  int                  // the button that has the keyboard: btnNone, btnOK or btnCancel
	btnY    int                  // the body line of the buttons
	btnX    [2][2]int            // the columns of OK and Cancel on that line
	err     string
}

// spacePick is a row in the space of the add dialog, and its level there.
type spacePick struct {
	in    bool
	level string
}

const (
	btnNone = iota
	btnOK
	btnCancel
)

func (m *model) newSearchDialog(space string) *searchDialog {
	in := textinput.New()
	in.Prompt = "/ "
	in.Placeholder = "name, account or space"
	in.CharLimit = 60
	in.Focus()
	return &searchDialog{m: m, space: space, input: in, typing: true, pending: map[string]spacePick{}, btnY: -1}
}

func (d *searchDialog) title() string {
	if d.space == "" {
		return "Find a conversation"
	}
	s, _ := d.m.u.cfg.Space(d.space)
	return "Add conversations to " + s.Title()
}

func (d *searchDialog) size(W, H int) (int, int) { return min(W-4, 96), H - 4 }

func (d *searchDialog) rows(m *model) []convRow {
	q := strings.ToLower(strings.TrimSpace(d.input.Value()))
	return m.rowsWhere(func(r convRow) bool {
		return q == "" || strings.Contains(strings.ToLower(r.Title()+" "+r.Name+" "+r.Account.Title()+" "+m.spacesText(r)), q)
	})
}

// saved returns whether the file has the row in the space of the dialog, and its level there.
func (d *searchDialog) saved(r convRow) spacePick {
	lv, in := levelIn(r, d.space)
	if !in || inNoSpace(r) {
		return spacePick{level: config.ReadAll}
	}
	return spacePick{in: true, level: lv}
}

// pick returns the row in the space of the dialog with the changes that wait for OK.
func (d *searchDialog) pick(r convRow) spacePick {
	if p, ok := d.pending[convKey(r.Account.ID, r.ID)]; ok {
		return p
	}
	return d.saved(r)
}

// set keeps p as the change of the row, or forgets the change when p is the saved state.
func (d *searchDialog) set(r convRow, p spacePick) {
	k := convKey(r.Account.ID, r.ID)
	if p == d.saved(r) {
		delete(d.pending, k)
		return
	}
	d.pending[k] = p
}

func (d *searchDialog) update(m *model, msg tea.Msg) tea.Cmd {
	if c, ok := msg.(tea.MouseClickMsg); ok {
		return d.click(m, c.Mouse())
	}
	k, ok := msg.(tea.KeyPressMsg)
	if !ok {
		if d.typing {
			var cmd tea.Cmd
			d.input, cmd = d.input.Update(msg)
			return cmd
		}
		return nil
	}
	if d.space != "" && k.String() == "ctrl+s" {
		return d.save(m)
	}
	if d.button != btnNone {
		return d.keyButton(m, k.String())
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
	case "tab":
		if d.space != "" {
			d.button = btnOK
		}
	case "up", "k":
		if d.cursor == 0 {
			d.typing = true
			return d.input.Focus()
		}
		d.cursor--
	case "down", "j":
		d.cursor = min(max(0, len(rows)-1), d.cursor+1)
	case "enter":
		if d.space != "" {
			return d.save(m)
		}
		if d.cursor < len(rows) {
			m.closeDialog()
			m.openDialog(m.newSpacesDialog(rows[d.cursor : d.cursor+1]))
		}
	case "space", "x":
		if d.space != "" && d.cursor < len(rows) {
			p := d.pick(rows[d.cursor])
			p.in = !p.in
			d.set(rows[d.cursor], p)
		}
	case "left", "right", "h", "l":
		if d.space != "" && d.cursor < len(rows) {
			if p := d.pick(rows[d.cursor]); p.in {
				p.level = map[string]string{config.ReadAll: config.ReadMentions}[p.level]
				if p.level == "" {
					p.level = config.ReadAll // also from off, which a file can have
				}
				d.set(rows[d.cursor], p)
			}
		}
	}
	return nil
}

// keyButton handles a key while OK or Cancel has the keyboard.
func (d *searchDialog) keyButton(m *model, k string) tea.Cmd {
	switch k {
	case "esc":
		m.closeDialog()
	case "enter", "space":
		if d.button == btnOK {
			return d.save(m)
		}
		m.closeDialog()
	case "left", "right", "h", "l":
		d.button = btnOK + btnCancel - d.button
	case "tab":
		if d.button == btnOK {
			d.button = btnCancel
			return nil
		}
		d.button, d.typing = btnNone, true
		return d.input.Focus()
	case "shift+tab":
		if d.button == btnCancel {
			d.button = btnOK
			return nil
		}
		d.button = btnNone
	case "up", "k":
		d.button = btnNone
	}
	return nil
}

// click runs the button under the mouse.
func (d *searchDialog) click(m *model, ms tea.Mouse) tea.Cmd {
	if ms.Button != tea.MouseLeft {
		return nil
	}
	x, y := m.dialogPoint(d, ms)
	if d.space != "" && y == d.btnY {
		switch {
		case x >= d.btnX[0][0] && x < d.btnX[0][1]:
			return d.save(m)
		case x >= d.btnX[1][0] && x < d.btnX[1][1]:
			m.closeDialog()
		}
		return nil
	}
	// The rows start under the search line and a blank line.
	rows := d.rows(m)
	i := d.off + y - 2
	if y < 2 || i >= d.end || i >= len(rows) {
		return nil
	}
	again := i == d.cursor && !d.typing && d.button == btnNone
	d.typing, d.button, d.cursor = false, btnNone, i
	d.input.Blur()
	if d.space != "" {
		// A click adds or removes the row, as space does.
		p := d.pick(rows[i])
		p.in = !p.in
		d.set(rows[i], p)
		return nil
	}
	if again {
		// A click on the selected row opens its spaces, as enter does.
		m.closeDialog()
		m.openDialog(m.newSpacesDialog(rows[i : i+1]))
	}
	return nil
}

// save writes the changes of the add dialog in one save and closes it.
func (d *searchDialog) save(m *model) tea.Cmd {
	if len(d.pending) == 0 {
		m.closeDialog()
		return nil
	}
	var rows []convRow
	var added, removed, level int
	for _, r := range m.convRows() {
		p, ok := d.pending[convKey(r.Account.ID, r.ID)]
		if !ok {
			continue
		}
		rows = append(rows, r)
		switch was := d.saved(r); {
		case p.in && !was.in:
			added++
		case !p.in && was.in:
			removed++
		default:
			level++
		}
	}
	var parts []string
	if added > 0 {
		parts = append(parts, "added "+plural(added, "conversation", "conversations"))
	}
	if removed > 0 {
		parts = append(parts, "removed "+plural(removed, "conversation", "conversations"))
	}
	if level > 0 {
		parts = append(parts, "changed the read level of "+plural(level, "conversation", "conversations"))
	}
	s, _ := m.u.cfg.Space(d.space)
	done := s.Title() + ": " + strings.Join(parts, ", ") + "."
	pending, space := d.pending, d.space
	if !m.editLinks(rows, func(c *config.Config, x convRow) []config.Link {
		p := pending[convKey(x.Account.ID, x.ID)]
		links := realLinks(c, x.Account.ID, x.ID)
		i := slices.IndexFunc(links, func(l config.Link) bool { return l.Space == space })
		switch {
		case p.in && i >= 0:
			links[i].Read = p.level
		case p.in:
			links = append(links, config.Link{Space: space, Read: p.level})
		case i >= 0:
			links = slices.Delete(links, i, i+1)
		}
		return orNoSpace(c, x, links)
	}, done) {
		d.err = m.status
		return nil
	}
	m.closeDialog()
	return nil
}

func (d *searchDialog) view(w, h int) string { return d.render(d.m, w, h) }

func (d *searchDialog) render(m *model, w, h int) string {
	rows := d.rows(m)
	lines := []string{d.input.View(), ""}
	listH := max(1, h-4)
	if d.space != "" {
		listH = max(1, h-6) // a blank line and the buttons
		if d.err != "" {
			listH = max(1, listH-2)
		}
	}
	d.cursor = min(d.cursor, max(0, len(rows)-1))
	d.off = scrollTo(d.cursor, d.off, listH)
	end := min(len(rows), d.off+listH)
	d.end = end
	nameW := max(14, w/3)
	for i := d.off; i < end; i++ {
		r := rows[i]
		box := ""
		where := m.spacesText(r)
		if inNoSpace(r) {
			where = "Everything else"
			if r.Read == config.ReadOff {
				where = "ignored"
			}
		}
		if d.space != "" {
			p := d.pick(r)
			box = "[ ] "
			if p.in {
				box = okStyle.Render("[x]") + " "
				where = levelText(p.level) + faint.Render(" · ") + where
			}
			if _, changed := d.pending[convKey(r.Account.ID, r.ID)]; changed {
				box = dirtyStyle.Render("*") + box
			} else {
				box = " " + box
			}
		}
		text := fmt.Sprintf("%s%s %-*s %-12s %s  %s", box, kindMark(r.Kind), nameW, truncate(r.Title(), nameW),
			truncate(r.Account.Title(), 12), where, faint.Render(m.lastText(r.Last)))
		if i == d.cursor && !d.typing && d.button == btnNone {
			lines = append(lines, selectedRow.Width(w).Render(truncate("▌ "+ansi.Strip(text), w)))
		} else {
			lines = append(lines, "  "+text)
		}
	}
	if len(rows) == 0 {
		lines = append(lines, faint.Render("  No conversation matches."))
	}
	lines = append(lines, "", faint.Render(fmt.Sprintf("newest message first · %d of %d", len(rows), len(m.convRows()))))
	if d.space == "" {
		return strings.Join(lines, "\n")
	}
	if d.err != "" {
		lines = append(lines, "", errStyle.Render(truncate(d.err, w)))
	}
	lines = append(lines, "")
	d.btnY = len(lines)
	var bs []string
	x := 0
	for i, b := range []struct {
		label string
		st    lipgloss.Style
	}{{"OK", btnPrimary}, {"Cancel", btnNormal}} {
		text, st := b.label, b.st
		if d.button == btnOK+i {
			text, st = "▸ "+text, st.Underline(true)
		}
		r := st.Render(text)
		d.btnX[i] = [2]int{x, x + lipgloss.Width(r)}
		bs = append(bs, r)
		x += lipgloss.Width(r) + 2
	}
	line := strings.Join(bs, "  ")
	if n := len(d.pending); n > 0 {
		line += "  " + dirtyStyle.Render(plural(n, "change", "changes")+" not saved")
	}
	lines = append(lines, line)
	return strings.Join(lines, "\n")
}

func (d *searchDialog) hints() []hint {
	switch {
	case d.button != btnNone:
		return []hint{{"enter", "choose"}, {"←→", "OK or Cancel"}, {"esc", "cancel"}}
	case d.typing && d.space != "":
		return []hint{{"type", "search"}, {"enter ↓", "to the list"}, {"ctrl+s", "OK"}, {"esc", "clear or cancel"}}
	case d.typing:
		return []hint{{"type", "search"}, {"enter ↓", "to the list"}, {"esc", "clear or close"}}
	case d.space != "":
		return []hint{{"space", "add or remove"}, {"←→", "read level"}, {"enter", "OK"}, {"esc", "cancel"}, {"tab", "buttons"}, {"/", "search"}}
	}
	return []hint{{"enter", "set its spaces"}, {"/", "search"}, {"esc", "close"}}
}
