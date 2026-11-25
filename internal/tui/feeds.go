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
	"github.com/sondt2709/inbox2agent/internal/source/linear"
)

// The account form of a Linear account shows each feed as a section under
// the account fields: a header, the fields of the feed and the button
// "Remove feed <id>". "+ Add feed" adds a section. "Save changes" saves the
// account and all its feeds.

// catalogState holds the names of one Linear workspace.
type catalogState struct {
	cat  linear.Catalog
	err  string
	busy bool
}

// catalogMsg brings the names of a workspace.
type catalogMsg struct {
	account string
	cat     linear.Catalog
	err     error
}

// feedSection is the part of the account form for one feed.
type feedSection struct {
	id      string // the saved feed ID; "" for a new feed (the field "id" gives it)
	pre     string // the key prefix of its fields, for example "f0."
	removed bool
}

func (sec *feedSection) isNew() bool { return sec.id == "" }

func (sec *feedSection) field(fm *form, key string) *formField { return fm.field(sec.pre + key) }

// feedPlace names the spaces that list the feed, else Everything else.
func (m *model) feedPlace(acc, feed string) string {
	var names []string
	for _, s := range m.u.cfg.Spaces {
		if slices.ContainsFunc(s.Conversations, func(cv config.Conversation) bool { return cv.Account == acc && cv.ID == feed }) {
			names = append(names, s.Title())
		}
	}
	if len(names) == 0 {
		return "Everything else"
	}
	return strings.Join(names, ", ")
}

// feedFields returns the fields of the feed sections of a Linear account and
// the button "+ Add feed". The sections go to fm.feeds.
func (m *model) feedFields(fm *form, a config.Account) []*formField {
	out := []*formField{
		noteField("Feeds. Each feed is one conversation. An item goes to a feed when it passes each filter that is not empty; names compare without case.", faint),
	}
	for _, f := range a.Feeds {
		out = append(out, m.feedSection(fm, a, f, false)...)
	}
	var add *formField
	add = buttonField("addfeed", "+ Add feed", false, func() tea.Cmd {
		f := config.Feed{Kind: config.FeedComments, From: []string{config.FromMe}}
		fields := m.feedSection(fm, a, f, true)
		i := slices.Index(fm.fields, add)
		fm.fields = slices.Insert(fm.fields, i, fields...)
		fm.changed = true
		// The keyboard goes to the Feed ID of the new section.
		for j, it := range fm.items() {
			if it.field == fields[1] {
				return fm.setFocus(j)
			}
		}
		return nil
	})
	return append(out, add)
}

// feedSection returns the fields of one feed. A new feed has the field Feed ID.
func (m *model) feedSection(fm *form, a config.Account, f config.Feed, isNew bool) []*formField {
	sec := &feedSection{id: f.ID, pre: fmt.Sprintf("f%d.", len(fm.feeds))}
	if isNew {
		sec.id = ""
	}
	fm.feeds = append(fm.feeds, sec)
	hidden := func() bool { return sec.removed }
	key := func(k string) string { return sec.pre + k }
	head := "Feed " + f.ID + " · its conversation is in " + m.feedPlace(a.ID, f.ID)
	if isNew {
		head = "New feed · a new conversation in Everything else"
	}
	header := noteField(head, panelTitle)
	var out []*formField
	out = append(out, header)
	if isNew {
		id := textField(key("id"), "Feed ID", "", "for example bridge", "A short ID that does not change: the ID of the conversation.", func(s string) error {
			if s == "" {
				return fmt.Errorf("type a feed ID, for example bridge")
			}
			for _, o := range fm.feeds {
				if o != sec && !o.removed && (o.id == s || o.isNew() && o.field(fm, "id").value() == s) {
					return fmt.Errorf("the feed %q exists", s)
				}
			}
			return validName(s)
		})
		out = append(out, id)
	}
	namePlaceholder := "empty: " + f.ID
	if isNew {
		namePlaceholder = "empty: the Feed ID"
	}
	list := func(k, label, placeholder, help string, values []string) *formField {
		fd := textField(key(k), label, strings.Join(values, ", "), placeholder,
			help+" Names separated by commas, or Choose for the names of the workspace.", nil)
		fd.pick = func() tea.Cmd { return m.openNames(a.ID, fd, k) }
		return fd
	}
	out = append(out,
		textField(key("name"), "Name", f.Name, namePlaceholder, "The name that i2a shows. Empty: the ID.", nil),
		choiceField(key("kind"), "Kind", "Comments: the comments on issues. Inbox: your Linear notifications.",
			[]string{config.FeedComments, config.FeedInbox}, []string{"Comments", "Inbox"}, f.Kind),
		list("from", "From", "me (comments) · anyone (inbox)", "The author of the comment or the person of the notification. me is you.", f.From),
		textField(key("prefix"), "Prefix", f.Prefix, "empty: any text",
			"The text after the leading mentions starts with it, with case, for example /agent or >agent.", nil),
		list("projects", "Projects", "any project", "The project of the issue.", f.Projects),
		list("labels", "Labels", "any label", "A label of the issue.", f.Labels),
		list("teams", "Teams", "any team", "The team key of the issue, for example ENG.", f.Teams),
	)
	for _, fd := range out {
		fd.hidden = hidden
	}
	var remove *formField
	label := "Remove feed " + f.ID
	if isNew {
		label = "Remove this new feed"
	}
	remove = buttonField(key("remove"), label, true, func() tea.Cmd {
		sec.removed = !sec.removed
		fm.changed = true
		if isNew {
			remove.hidden = hidden // a new section goes away
			return fm.setFocus(fm.focus)
		}
		remove.label, remove.danger = "Remove feed "+f.ID, true
		header.note, header.noteStyle = head, panelTitle
		if sec.removed {
			remove.label, remove.danger = "Keep feed "+f.ID, false
			header.note, header.noteStyle = "Feed "+f.ID+" · removed when you save, with its entries in all spaces", warnStyle
		}
		return nil
	})
	header.hidden = nil
	if isNew {
		header.hidden = hidden
	}
	return append(out, remove)
}

// formFeeds reads the feeds of the sections that are not removed, and the IDs
// of the removed feeds.
func formFeeds(fm *form) (feeds []config.Feed, removed []string) {
	for _, sec := range fm.feeds {
		if sec.removed {
			if !sec.isNew() {
				removed = append(removed, sec.id)
			}
			continue
		}
		id := sec.id
		if sec.isNew() {
			id = sec.field(fm, "id").value()
		}
		feeds = append(feeds, config.Feed{
			ID: id, Name: sec.field(fm, "name").value(), Kind: sec.field(fm, "kind").value(),
			From: splitNames(sec.field(fm, "from").value()), Prefix: sec.field(fm, "prefix").value(),
			Projects: splitNames(sec.field(fm, "projects").value()), Labels: splitNames(sec.field(fm, "labels").value()),
			Teams: splitNames(sec.field(fm, "teams").value()),
		})
	}
	return feeds, removed
}

// dropConversations removes the entries of the conversations of the account
// in all spaces.
func dropConversations(c *config.Config, acc string, ids []string) {
	drop := func(list []config.Conversation) []config.Conversation {
		return slices.DeleteFunc(list, func(cv config.Conversation) bool { return cv.Account == acc && slices.Contains(ids, cv.ID) })
	}
	c.Unsorted.Conversations = drop(c.Unsorted.Conversations)
	for i := range c.Spaces {
		c.Spaces[i].Conversations = drop(c.Spaces[i].Conversations)
	}
}

// loadCatalog reads the names of the workspace of the account one time.
func (m *model) loadCatalog(id string) tea.Cmd {
	if st := m.catalogs[id]; st != nil && (st.busy || st.err == "") {
		return nil
	}
	a, ok := m.u.cfg.Account(id)
	if !ok {
		return nil
	}
	if m.u.d.Catalog == nil {
		m.catalogs[id] = &catalogState{err: "the dashboard cannot read the names of the workspace"}
		return nil
	}
	token, err := m.u.d.Secrets.Get(secretsKey(id))
	if err != nil {
		m.catalogs[id] = &catalogState{err: "cannot read the API key: " + err.Error()}
		return nil
	}
	m.catalogs[id] = &catalogState{busy: true}
	acc, read := *a, m.u.d.Catalog
	ctx := m.ctx
	return func() tea.Msg {
		cat, err := read(ctx, acc, token)
		return catalogMsg{account: id, cat: cat, err: err}
	}
}

func (m *model) gotCatalog(msg catalogMsg) {
	st := &catalogState{cat: msg.cat}
	if msg.err != nil {
		st = &catalogState{err: msg.err.Error()}
	}
	m.catalogs[msg.account] = st
}

// splitNames reads a list field: names separated by commas, each one time.
func splitNames(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		p = strings.TrimSpace(p)
		if p != "" && !slices.ContainsFunc(out, func(x string) bool { return strings.EqualFold(x, p) }) {
			out = append(out, p)
		}
	}
	return out
}

// ---- the list of names ----

// nameItem is one row of the names dialog.
type nameItem struct{ value, note string }

// catalogItems returns the rows of a list field and a text about the names:
// "" when they are ready, else that they are loading or the error.
func (m *model) catalogItems(acc, key string) ([]nameItem, string) {
	st := m.catalogs[acc]
	switch {
	case st == nil || st.busy:
		return nil, "Reading the names from Linear …"
	case st.err != "":
		return nil, "Cannot read the names: " + st.err + "\nType a name and press enter to add it."
	}
	var out []nameItem
	add := func(names []string) {
		for _, n := range names {
			out = append(out, nameItem{value: n})
		}
	}
	switch key {
	case "from":
		out = append(out, nameItem{value: config.FromMe, note: "you"})
		add(st.cat.Users)
	case "projects":
		add(st.cat.Projects)
	case "labels":
		add(st.cat.Labels)
	case "teams":
		for _, t := range st.cat.Teams {
			out = append(out, nameItem{value: t.Key, note: t.Name})
		}
	}
	if st.cat.More {
		return out, "The workspace has more names than this list. Type a name to add it."
	}
	return out, ""
}

// openNames opens the names dialog of the list field fd (key: from,
// projects, labels or teams).
func (m *model) openNames(acc string, fd *formField, key string) tea.Cmd {
	in := textinput.New()
	in.Prompt = "Search › "
	in.Placeholder = "type to filter or to add a name"
	in.CharLimit = 200
	values := splitNames(fd.value())
	d := &namesDialog{name: fd.label, field: fd, values: values, first: slices.Clone(values), typing: true, btnY: -1,
		items: func() ([]nameItem, string) { return m.catalogItems(acc, key) }, input: in}
	m.openDialog(d)
	return tea.Batch(d.input.Focus(), m.loadCatalog(acc))
}

// namesDialog chooses names with check marks. It has the keys and the
// buttons of the add dialog (searchDialog): the search line has the keyboard
// first; enter, ↓ or tab go to the list; space or a click checks a row; enter
// or OK applies the names to the field; esc or Cancel closes it.
type namesDialog struct {
	name   string
	field  *formField
	values []string // the checked names, in order
	first  []string // the checked names when the dialog opened: their rows come first
	items  func() ([]nameItem, string)
	input  textinput.Model
	typing bool // the search line has the keyboard
	button int  // the button that has the keyboard: btnNone, btnOK or btnCancel
	cursor int
	rowY   int // the body line of the first visible row, from the last view
	off    int
	end    int
	btnY   int       // the body line of the buttons
	btnX   [2][2]int // the columns of OK and Cancel on that line
}

// row is one visible row: a name, or the row that adds the typed text.
type nameRow struct {
	nameItem
	add bool
}

func (d *namesDialog) checked(v string) bool {
	return slices.ContainsFunc(d.values, func(x string) bool { return strings.EqualFold(x, v) })
}

// rows returns the visible rows, filtered by the search: the names that were
// checked when the dialog opened, the names that the user added, then the
// other names of the list. The order does not change while the dialog is
// open. When no row matches the search, the one row adds the text.
func (d *namesDialog) rows() []nameRow {
	items, _ := d.items()
	all := make([]nameItem, 0, len(items)+len(d.values))
	in := func(list []string, v string) bool {
		return slices.ContainsFunc(list, func(x string) bool { return strings.EqualFold(x, v) })
	}
	for _, v := range d.first {
		i := slices.IndexFunc(items, func(it nameItem) bool { return strings.EqualFold(it.value, v) })
		if i >= 0 {
			all = append(all, items[i])
		} else {
			all = append(all, nameItem{value: v})
		}
	}
	for _, v := range d.values {
		if !in(d.first, v) && !slices.ContainsFunc(items, func(it nameItem) bool { return strings.EqualFold(it.value, v) }) {
			all = append(all, nameItem{value: v})
		}
	}
	for _, it := range items {
		if !in(d.first, it.value) {
			all = append(all, it)
		}
	}
	q := strings.TrimSpace(d.input.Value())
	var out []nameRow
	for _, it := range all {
		if q == "" || containsFold(it.value, q) || containsFold(it.note, q) {
			out = append(out, nameRow{nameItem: it})
		}
	}
	if q != "" && len(out) == 0 {
		out = append(out, nameRow{nameItem: nameItem{value: q}, add: true})
	}
	return out
}

func containsFold(s, sub string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(sub))
}

func (d *namesDialog) title() string { return d.name }

func (d *namesDialog) size(W, H int) (int, int) { return min(W-4, 72), min(H-4, 24) }

// toggle switches the check mark of row i.
func (d *namesDialog) toggle(i int) {
	rows := d.rows()
	if i < 0 || i >= len(rows) {
		return
	}
	r := rows[i]
	if d.checked(r.value) {
		d.values = slices.DeleteFunc(d.values, func(x string) bool { return strings.EqualFold(x, r.value) })
		return
	}
	d.values = append(d.values, r.value)
	if r.add {
		d.input.SetValue("")
		d.cursor = 0
	}
}

// apply writes the checked names to the field and closes the dialog.
func (d *namesDialog) apply(m *model) tea.Cmd {
	d.field.input.SetValue(strings.Join(d.values, ", "))
	if d.field.err != "" && d.field.validate != nil && d.field.validate(d.field.value()) == nil {
		d.field.err = ""
	}
	m.closeDialog()
	return nil
}

func (d *namesDialog) update(m *model, msg tea.Msg) tea.Cmd {
	if c, ok := msg.(tea.MouseClickMsg); ok {
		return d.click(m, c.Mouse())
	}
	k, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return nil
	}
	if k.String() == "ctrl+s" {
		return d.apply(m)
	}
	if d.button != btnNone {
		return d.keyButton(m, k.String())
	}
	n := len(d.rows())
	if d.typing {
		switch k.String() {
		case "esc":
			if d.input.Value() != "" {
				d.input.SetValue("")
				d.cursor = 0
				return nil
			}
			m.closeDialog()
			return nil
		case "enter", "down", "tab":
			d.typing = false
			d.input.Blur()
			d.cursor = 0
			return nil
		}
		before := d.input.Value()
		var cmd tea.Cmd
		d.input, cmd = d.input.Update(msg)
		if d.input.Value() != before {
			d.cursor = 0
		}
		return cmd
	}
	switch k.String() {
	case "esc":
		m.closeDialog()
	case "/":
		d.typing = true
		return d.input.Focus()
	case "tab":
		d.button = btnOK
	case "up", "k":
		if d.cursor == 0 {
			d.typing = true
			return d.input.Focus()
		}
		d.cursor--
	case "down", "j":
		d.cursor = min(max(0, n-1), d.cursor+1)
	case "enter":
		return d.apply(m)
	case "space", "x":
		d.toggle(d.cursor)
	}
	return nil
}

// keyButton handles a key while OK or Cancel has the keyboard, as in the add dialog.
func (d *namesDialog) keyButton(m *model, k string) tea.Cmd {
	switch k {
	case "esc":
		m.closeDialog()
	case "enter", "space":
		if d.button == btnOK {
			return d.apply(m)
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

// click runs the button under the mouse, or checks the row, as space does.
func (d *namesDialog) click(m *model, ms tea.Mouse) tea.Cmd {
	if ms.Button != tea.MouseLeft {
		return nil
	}
	x, y := m.dialogPoint(d, ms)
	if y == d.btnY {
		switch {
		case x >= d.btnX[0][0] && x < d.btnX[0][1]:
			return d.apply(m)
		case x >= d.btnX[1][0] && x < d.btnX[1][1]:
			m.closeDialog()
		}
		return nil
	}
	if i := d.off + y - d.rowY; y >= d.rowY && i < d.end {
		d.typing, d.button, d.cursor = false, btnNone, i
		d.input.Blur()
		d.toggle(i)
	}
	return nil
}

func (d *namesDialog) view(w, h int) string {
	_, note := d.items()
	head := []string{faint.Width(w).Render(plural(len(d.values), "name", "names") + " checked.")}
	if note != "" {
		st := faint
		if strings.HasPrefix(note, "Cannot") {
			st = errStyle
		}
		head = append(head, st.Width(w).Render(note))
	}
	d.input.SetWidth(max(10, w-12))
	head = append(head, "", d.input.View(), "")
	text := strings.Join(head, "\n")
	rows := d.rows()
	d.cursor = min(d.cursor, max(0, len(rows)-1))
	lines := make([]string, 0, len(rows))
	for i, r := range rows {
		mark := "[ ] "
		if d.checked(r.value) {
			mark = okStyle.Render("[x]") + " "
		}
		label := mark + r.value
		if r.add {
			label = "+ Add \"" + r.value + "\""
		}
		if i == d.cursor && !d.typing && d.button == btnNone {
			lines = append(lines, selectedRow.Width(w).Render(truncate("▌ "+ansi.Strip(label+"  "+r.note), w)))
			continue
		}
		lines = append(lines, "  "+textStyle.Render(truncate(label, w-4))+"  "+faint.Render(r.note))
	}
	if len(rows) == 0 {
		lines = append(lines, faint.Render("  No name."))
	}
	vis := max(1, h-lipgloss.Height(text)-2) // a blank line and the buttons
	off := scrollTo(d.cursor, 0, vis)
	end := min(len(lines), off+vis)
	d.rowY, d.off, d.end = lipgloss.Height(text), off, min(end, len(rows))
	body := text + "\n" + strings.Join(lines[off:end], "\n") + "\n"
	d.btnY = lipgloss.Height(body)
	var bs []string
	x := 0
	for i, b := range []struct {
		label string
		st    lipgloss.Style
	}{{"OK", btnPrimary}, {"Cancel", btnNormal}} {
		t, st := b.label, b.st
		if d.button == btnOK+i {
			t, st = "▸ "+t, st.Underline(true)
		}
		r := st.Render(t)
		d.btnX[i] = [2]int{x, x + lipgloss.Width(r)}
		bs = append(bs, r)
		x += lipgloss.Width(r) + 2
	}
	return body + "\n" + strings.Join(bs, "  ")
}

func (d *namesDialog) hints() []hint {
	switch {
	case d.button != btnNone:
		return []hint{{"enter", "choose"}, {"←→", "OK or Cancel"}, {"esc", "cancel"}}
	case d.typing:
		return []hint{{"type", "search"}, {"enter ↓", "to the list"}, {"ctrl+s", "OK"}, {"esc", "clear or cancel"}}
	}
	return []hint{{"space", "check"}, {"enter", "OK"}, {"esc", "cancel"}, {"tab", "buttons"}, {"/", "search"}}
}
