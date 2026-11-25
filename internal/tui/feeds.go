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

// The feed form edits one feed of a Linear account in the detail panel of
// Setup › Accounts. It replaces the account form while it is open.

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

// feedTarget returns the key and the builder of the feed form when it is
// open for the account a.
func (m *model) feedTarget(a config.Account) (string, func() *form) {
	acc, feed, ok := strings.Cut(m.feedForm, "/")
	if !ok || acc != a.ID {
		m.feedForm = "" // the selection moved to another account
		return "", nil
	}
	if feed != "+" && !slices.ContainsFunc(a.Feeds, func(f config.Feed) bool { return f.ID == feed }) {
		m.feedForm = "" // another process removed the feed
		return "", nil
	}
	return "feed:" + m.feedForm, func() *form { return m.feedEditForm(a.ID, feed) }
}

// openFeedList shows the feeds of a Linear account and "+ New feed".
func (m *model) openFeedList(id string) tea.Cmd {
	a, ok := m.u.cfg.Account(id)
	if !ok || a.Platform != config.PlatformLinear {
		return nil
	}
	opts := make([]pickOption, 0, len(a.Feeds)+1)
	for _, f := range a.Feeds {
		feed := f.ID
		opts = append(opts, pickOption{label: f.Title(), note: feedLine(f), run: func() tea.Cmd { return m.openFeedForm(id, feed) }})
	}
	opts = append(opts, pickOption{label: "+ New feed", note: "a new conversation of this account", run: func() tea.Cmd { return m.openFeedForm(id, "+") }})
	m.openDialog(&pickDialog{name: "Feeds of " + a.Title(),
		text: "Each feed is one conversation. Choose a feed to change its filters.", options: opts})
	return nil
}

// openFeedForm shows the form of a feed ("+" for a new feed) and reads the
// names of the workspace.
func (m *model) openFeedForm(acc, feed string) tea.Cmd {
	m.feedForm = acc + "/" + feed
	m.buildForm(true)
	return tea.Batch(m.focusForm(), m.loadCatalog(acc))
}

// closeFeedForm shows the account form again.
func (m *model) closeFeedForm() {
	m.feedForm = ""
	m.buildForm(true)
}

// cancelFeedForm closes the feed form. When the user changed it, it asks first.
func (m *model) cancelFeedForm() {
	if m.form == nil || !m.form.dirty() {
		m.closeFeedForm()
		return
	}
	m.openDialog(&confirmDialog{name: "Unsaved changes", text: "Discard the changes to the feed?",
		choices: []choice{{"d", "Discard", func() tea.Cmd {
			m.closeFeedForm()
			m.setStatus("Changes discarded.", false)
			return nil
		}}}})
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

// feedEditForm edits the feed of the account; feed "+" adds a new feed.
func (m *model) feedEditForm(acc, feed string) *form {
	a, _ := m.u.cfg.Account(acc)
	f := config.Feed{Kind: config.FeedComments, From: []string{config.FromMe}}
	isNew := feed == "+"
	if !isNew {
		for _, x := range a.Feeds {
			if x.ID == feed {
				f = x
			}
		}
	}
	used := func(s string) bool {
		return slices.ContainsFunc(a.Feeds, func(x config.Feed) bool { return x.ID == s })
	}
	fm := &form{}
	what := "Feed " + f.ID + " of " + a.Title() + ". It is one conversation; its ID does not change."
	if isNew {
		what = "A new feed of " + a.Title() + ". It is a new conversation in Everything else."
	}
	fm.fields = []*formField{
		noteField(what, faint),
		noteField("An item goes to the feed when it passes each filter that is not empty. Names compare without case.", faint),
	}
	if isNew {
		fm.fields = append(fm.fields, textField("id", "Feed ID", newID("feed", used), "bridge",
			"A short ID that does not change: the ID of the conversation.", func(s string) error {
				if used(s) {
					return fmt.Errorf("the feed %q exists", s)
				}
				return validName(s)
			}))
	}
	kind := choiceField("kind", "Kind", "Comments: the comments on issues. Inbox: your Linear notifications.",
		[]string{config.FeedComments, config.FeedInbox}, []string{"Comments", "Inbox"}, f.Kind)
	list := func(key, label, placeholder, help string, values []string) *formField {
		fd := textField(key, label, strings.Join(values, ", "), placeholder,
			help+" Names separated by commas. Press enter to choose from the workspace.", nil)
		fd.pick = func() tea.Cmd { return m.openNames(acc, fm, key) }
		return fd
	}
	fm.fields = append(fm.fields,
		textField("name", "Name", f.Name, orStr(f.ID, "for example Bridge requests"), "The name that i2a shows. Empty: the ID.", nil),
		kind,
		list("from", "From", "me (comments) · anyone (inbox)",
			"The author of the comment or the person of the notification. me is you.", f.From),
		textField("prefix", "Prefix", f.Prefix, "empty: any text",
			"The text after the leading mentions starts with it, with case, for example /agent or >agent.", nil),
		list("projects", "Projects", "any project", "The project of the issue.", f.Projects),
		list("labels", "Labels", "any label", "A label of the issue.", f.Labels),
		list("teams", "Teams", "any team", "The team key of the issue, for example ENG.", f.Teams),
	)
	fm.buttons = []button{
		{label: "Save feed", primary: true, run: func() tea.Cmd { return m.saveFeed(acc, feed) }},
		{label: "Cancel", run: func() tea.Cmd { return func() tea.Msg { return formEsc{} } }},
	}
	if !isNew {
		fm.buttons = append(fm.buttons, button{label: "Remove feed", danger: true, run: func() tea.Cmd { return m.removeFeed(acc, feed) }})
	}
	return fm
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

func (m *model) saveFeed(acc, feed string) tea.Cmd {
	fm := m.form
	if !fm.validate() {
		return nil
	}
	isNew := feed == "+"
	if isNew {
		feed = fm.field("id").value()
	}
	f := config.Feed{
		ID: feed, Name: fm.field("name").value(), Kind: fm.field("kind").value(),
		From: splitNames(fm.field("from").value()), Prefix: fm.field("prefix").value(),
		Projects: splitNames(fm.field("projects").value()), Labels: splitNames(fm.field("labels").value()),
		Teams: splitNames(fm.field("teams").value()),
	}
	if err := m.commitForm(fm, func(c *config.Config) error {
		a, ok := c.Account(acc)
		if !ok {
			return config.MissingError{Kind: config.KindAccount, ID: acc}
		}
		i := slices.IndexFunc(a.Feeds, func(x config.Feed) bool { return x.ID == feed })
		switch {
		case isNew && i >= 0:
			return fmt.Errorf("the feed %q exists", feed)
		case isNew:
			a.Feeds = append(a.Feeds, f)
		case i < 0:
			return fmt.Errorf("the feed %q does not exist", feed)
		default:
			a.Feeds[i] = f
		}
		return nil
	}, nil); err != nil {
		saveError(fm, err)
		return nil
	}
	m.closeFeedForm()
	if isNew {
		m.saved("Added the feed " + f.Title() + ". It is in Everything else: add it to a space in Spaces.")
	} else {
		m.saved("Saved the feed " + f.Title() + ": " + feedLine(f) + ".")
	}
	return nil
}

// removeFeed asks, then removes the feed and its entries in all spaces.
func (m *model) removeFeed(acc, feed string) tea.Cmd {
	a, ok := m.u.cfg.Account(acc)
	if !ok {
		return nil
	}
	if len(a.Feeds) <= 1 {
		m.form.err = "A Linear account needs one feed or more. Add another feed before you remove this one."
		return nil
	}
	title := feed
	for _, f := range a.Feeds {
		if f.ID == feed {
			title = f.Title()
		}
	}
	m.openDialog(&confirmDialog{name: "Remove feed " + title,
		text: "Remove the feed " + title + " of " + a.Title() + " and its entries in all spaces? Its messages that wait are not sent.",
		choices: []choice{{"y", "Remove", func() tea.Cmd {
			if err := m.commitForm(m.form, func(c *config.Config) error {
				x, ok := c.Account(acc)
				if !ok {
					return config.MissingError{Kind: config.KindAccount, ID: acc}
				}
				x.Feeds = slices.DeleteFunc(x.Feeds, func(f config.Feed) bool { return f.ID == feed })
				drop := func(list []config.Conversation) []config.Conversation {
					return slices.DeleteFunc(list, func(cv config.Conversation) bool { return cv.Account == acc && cv.ID == feed })
				}
				c.Unsorted.Conversations = drop(c.Unsorted.Conversations)
				for i := range c.Spaces {
					c.Spaces[i].Conversations = drop(c.Spaces[i].Conversations)
				}
				return nil
			}, nil); err != nil {
				if m.form != nil {
					saveError(m.form, err)
				}
				return nil
			}
			m.closeFeedForm()
			m.saved("Removed the feed " + title + ".")
			return nil
		}}}})
	return nil
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

// openNames opens the names dialog of the list field key of the form fm.
func (m *model) openNames(acc string, fm *form, key string) tea.Cmd {
	fd := fm.field(key)
	if fd == nil {
		return nil
	}
	in := textinput.New()
	in.Prompt = "Search › "
	in.Placeholder = "type to filter or to add a name"
	in.CharLimit = 200
	values := splitNames(fd.value())
	d := &namesDialog{name: fd.label, field: fd, values: values, first: slices.Clone(values),
		items: func() ([]nameItem, string) { return m.catalogItems(acc, key) }, input: in}
	m.openDialog(d)
	return tea.Batch(d.input.Focus(), m.loadCatalog(acc))
}

// namesDialog chooses names with check marks. Typing filters the rows,
// enter switches the check mark, ctrl+s applies the names to the field.
type namesDialog struct {
	name   string
	field  *formField
	values []string // the checked names, in order
	first  []string // the checked names when the dialog opened: their rows come first
	items  func() ([]nameItem, string)
	input  textinput.Model
	cursor int
	rowY   int // the body line of the first visible row, from the last view
	off    int
	end    int
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

func (d *namesDialog) update(m *model, msg tea.Msg) tea.Cmd {
	if c, ok := msg.(tea.MouseClickMsg); ok {
		_, y := m.dialogPoint(d, c.Mouse())
		if i := d.off + y - d.rowY; c.Mouse().Button == tea.MouseLeft && y >= d.rowY && i < d.end {
			d.cursor = i
			d.toggle(i)
		}
		return nil
	}
	k, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return nil
	}
	n := len(d.rows())
	switch k.String() {
	case "esc":
		m.closeDialog()
		return nil
	case "ctrl+s":
		d.field.input.SetValue(strings.Join(d.values, ", "))
		if d.field.err != "" && d.field.validate != nil && d.field.validate(d.field.value()) == nil {
			d.field.err = ""
		}
		m.closeDialog()
		return nil
	case "up":
		d.cursor = max(0, d.cursor-1)
		return nil
	case "down":
		d.cursor = min(max(0, n-1), d.cursor+1)
		return nil
	case "enter":
		d.toggle(d.cursor)
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

func (d *namesDialog) view(w, h int) string {
	_, note := d.items()
	head := []string{faint.Width(w).Render(fmt.Sprintf("%d checked. Enter checks or unchecks a row; ctrl+s applies.", len(d.values)))}
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
			mark = "[x] "
		}
		label := mark + r.value
		if r.add {
			label = "+ Add \"" + r.value + "\""
		}
		if i == d.cursor {
			lines = append(lines, selectedRow.Width(w).Render(truncate("▌ "+ansi.Strip(label+"  "+r.note), w)))
			continue
		}
		lines = append(lines, "  "+textStyle.Render(truncate(label, w-4))+"  "+faint.Render(r.note))
	}
	if len(rows) == 0 {
		lines = append(lines, faint.Render("  No name."))
	}
	vis := max(1, h-lipgloss.Height(text))
	off := scrollTo(d.cursor, 0, vis)
	end := min(len(lines), off+vis)
	d.rowY, d.off, d.end = lipgloss.Height(text), off, min(end, len(rows))
	return text + "\n" + strings.Join(lines[off:end], "\n")
}

func (d *namesDialog) hints() []hint {
	return []hint{{"↑↓", "move"}, {"enter", "check"}, {"ctrl+s", "apply"}, {"esc", "cancel"}}
}
