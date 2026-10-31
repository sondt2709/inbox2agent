package tui

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/sondt2709/inbox2agent/internal/config"
	"github.com/sondt2709/inbox2agent/internal/inbox"
	"github.com/sondt2709/inbox2agent/internal/source/mattermost"
	"github.com/sondt2709/inbox2agent/internal/store"
)

type mattermostChannel = mattermost.ChannelInfo

// convRow is one conversation of one account, from the configuration, from
// the list of the account, or both.
type convRow struct {
	Account config.Account
	ID      string
	Name    string   // the platform name
	Label   string   // the name that the user gave
	Kind    string   // inbox.KindDirect, KindGroup, KindChannel, or "" when the account did not list it
	Space   string   // the ID of the first space
	Spaces  []string // the IDs of all spaces that list it (Space first)
	Read    string   // the read level of the first link
	Reads   []string // the read level of each link, in the order of Spaces
	Entry   bool     // the configuration has an entry
	Listed  bool     // the account lists it
	Last    time.Time
}

// Title returns the label, the name or the ID.
func (r convRow) Title() string { return orStr(r.Label, orStr(r.Name, r.ID)) }

func convKey(account, id string) string { return account + "\x00" + id }

// kindMark is the mark of a conversation kind: the same columns for all kinds.
func kindMark(kind string) string {
	switch kind {
	case inbox.KindDirect:
		return "@"
	case inbox.KindGroup:
		return "⧉"
	case inbox.KindChannel:
		return "#"
	}
	return "?"
}

// convRows joins the conversations that the accounts list with the entries of
// the configuration, account by account in the order of the lists.
func (m *model) convRows() []convRow {
	cfg := m.u.cfg
	var out []convRow
	for _, a := range cfg.Accounts {
		seen := map[string]bool{}
		for _, ch := range m.chs[a.ID] {
			cv, space, entry := cfg.Conversation(a.ID, ch.ID)
			sp, rd := links(cfg, a.ID, ch.ID)
			out = append(out, convRow{Account: a, ID: ch.ID, Name: orStr(cv.Name, ch.Name), Label: cv.Label, Kind: ch.Kind,
				Space: space, Spaces: sp, Read: cv.Read, Reads: rd, Entry: entry, Listed: true, Last: ch.LastPost})
			if ch.Name != "" {
				out[len(out)-1].Name = ch.Name
			}
			seen[ch.ID] = true
		}
		for _, s := range cfg.AllSpaces() {
			for _, cv := range s.Conversations {
				if cv.Account != a.ID || seen[cv.ID] {
					continue
				}
				seen[cv.ID] = true // one row for a conversation in many spaces
				first, _, _ := cfg.Conversation(a.ID, cv.ID)
				sp, rd := links(cfg, a.ID, cv.ID)
				out = append(out, convRow{Account: a, ID: cv.ID, Name: cv.Name, Label: first.Label, Space: s.ID, Spaces: sp,
					Read: cv.Read, Reads: rd, Entry: true})
			}
		}
	}
	return out
}

// links returns the spaces and the read levels of the links of a conversation.
func links(cfg *config.Config, account, id string) (spaces, reads []string) {
	for _, l := range cfg.Links(account, id) {
		spaces, reads = append(spaces, l.Space), append(reads, l.Read)
	}
	return spaces, reads
}

// spacesText names the spaces of a row: "Review, Team".
func (m *model) spacesText(r convRow) string {
	names := m.spaceNames()
	var out []string
	for _, id := range r.Spaces {
		out = append(out, orStr(names[id], id))
	}
	return strings.Join(out, ", ")
}

// readsText is the read level of a row, or each level when the links differ: "all, mentions".
func readsText(r convRow) string {
	for _, l := range r.Reads {
		if l != r.Read {
			return strings.Join(r.Reads, ", ")
		}
	}
	return r.Read
}

// unsortedCount is the number of conversations in Unsorted that are not off.
func (m *model) unsortedCount() int {
	n := 0
	for _, r := range m.convRows() {
		if r.Space == config.UnsortedID && r.Read != config.ReadOff {
			n++
		}
	}
	return n
}

// waitingIn returns the number of pending messages of a conversation.
func (m *model) waitingIn(account, id string) int {
	return m.pending[store.Conv{Account: account, ID: id}]
}

func newSearch() textinput.Model {
	in := textinput.New()
	in.Prompt = "/ "
	in.Placeholder = "search name, account or space"
	in.CharLimit = 60
	return in
}

// convFilters returns the filters in the order of the f key.
func (m *model) convFilters() []string {
	out := []string{"", "unsorted"}
	for _, a := range m.u.cfg.Accounts {
		out = append(out, "account:"+a.ID)
	}
	out = append(out, "kind:"+inbox.KindDirect, "kind:"+inbox.KindGroup, "kind:"+inbox.KindChannel)
	for _, s := range m.u.cfg.Spaces {
		out = append(out, "space:"+s.ID)
	}
	return out
}

func (m *model) filterLabel(f string) string {
	k, v, _ := strings.Cut(f, ":")
	switch k {
	case "":
		return "all"
	case "unsorted":
		return "Unsorted"
	case "account":
		if a, ok := m.u.cfg.Account(v); ok {
			return "account " + a.Title()
		}
	case "kind":
		return kindMark(v) + " " + v
	case "space":
		if s, ok := m.u.cfg.Space(v); ok {
			return "space " + s.Title()
		}
	}
	return f
}

// shownConvs applies the filter and the search.
func (m *model) shownConvs() []convRow {
	q := strings.ToLower(strings.TrimSpace(m.search.Value()))
	k, v, _ := strings.Cut(m.convFilter, ":")
	var out []convRow
	for _, r := range m.convRows() {
		switch {
		case k == "unsorted" && r.Space != config.UnsortedID,
			k == "account" && r.Account.ID != v,
			k == "kind" && r.Kind != v,
			k == "space" && !slices.Contains(r.Spaces, v):
			continue
		}
		if q != "" && !strings.Contains(strings.ToLower(r.Title()+" "+r.Name+" "+r.Account.Title()+" "+m.spacesText(r)+" "+r.Kind), q) {
			continue
		}
		out = append(out, r)
	}
	return out
}

func (m *model) hintsConversations() []hint {
	if len(m.convSel) > 0 {
		return []hint{{"space", "select"}, {"m", fmt.Sprintf("move %d", len(m.convSel))}, {"l", "read level"}, {"esc", "clear"}}
	}
	return []hint{{"↑↓", "move"}, {"space", "select"}, {"a", "select all"}, {"m", "move to space"}, {"L", "one more space"}, {"l", "read level"}, {"n", "rename"}, {"/", "search"}, {"f", "filter"}}
}

// targets returns the selected conversations, or the conversation under the cursor.
func (m *model) targets() []convRow {
	rows := m.shownConvs()
	if len(m.convSel) > 0 {
		var out []convRow
		for _, r := range m.convRows() {
			if m.convSel[convKey(r.Account.ID, r.ID)] {
				out = append(out, r)
			}
		}
		return out
	}
	if m.conv < len(rows) {
		return []convRow{rows[m.conv]}
	}
	return nil
}

func (m *model) keyConversations(k string) tea.Cmd {
	rows := m.shownConvs()
	switch k {
	case "up", "k":
		m.conv = max(0, m.conv-1)
	case "down", "j":
		m.conv = min(max(0, len(rows)-1), m.conv+1)
	case "space":
		if m.conv < len(rows) {
			key := convKey(rows[m.conv].Account.ID, rows[m.conv].ID)
			if m.convSel[key] {
				delete(m.convSel, key)
			} else {
				m.convSel[key] = true
			}
			m.conv = min(len(rows)-1, m.conv+1)
		}
	case "esc":
		clear(m.convSel)
	case "a":
		// Select all shown rows, or clear them when all are selected.
		all := len(rows) > 0
		for _, r := range rows {
			all = all && m.convSel[convKey(r.Account.ID, r.ID)]
		}
		for _, r := range rows {
			if all {
				delete(m.convSel, convKey(r.Account.ID, r.ID))
			} else {
				m.convSel[convKey(r.Account.ID, r.ID)] = true
			}
		}
	case "/":
		m.searching = true
		return m.search.Focus()
	case "f":
		fs := m.convFilters()
		i := slices.Index(fs, m.convFilter)
		m.convFilter = fs[(i+1)%len(fs)]
		m.conv = 0
	case "m":
		if t := m.targets(); len(t) > 0 {
			m.moveDialog(t)
		}
	case "l":
		return m.cycleRead(m.targets())
	case "L":
		if t := m.targets(); len(t) > 0 {
			m.linkDialog(t)
		}
	case "n":
		if t := m.targets(); len(t) == 1 {
			m.renameDialog(t[0])
		}
	}
	return nil
}

// searchKey edits the search while it has the keyboard. Enter keeps the
// search; esc clears it.
func (m *model) searchKey(msg tea.KeyPressMsg) tea.Cmd {
	switch msg.String() {
	case "enter", "down":
		m.searching = false
		m.search.Blur()
		return nil
	case "esc":
		m.searching = false
		m.search.SetValue("")
		m.search.Blur()
		return nil
	}
	var cmd tea.Cmd
	m.search, cmd = m.search.Update(msg)
	m.conv = 0
	return cmd
}

// setConversations moves conversations to a space, sets their read level, or
// both, and saves. space "" keeps the space; read "" keeps the level.
func (m *model) setConversations(rows []convRow, space, read, done string) tea.Cmd {
	if err := m.u.commit(func(c *config.Config) error {
		for _, r := range rows {
			if space == "" {
				// Only the level: each link of the conversation gets it, and the links stay.
				if err := c.SetLevel(r.Account.ID, r.ID, read); err != nil {
					return err
				}
				continue
			}
			// The space and the level of the file, which another process can have changed.
			cur, curSpace, _ := c.Conversation(r.Account.ID, r.ID)
			lv := read
			if lv == "" && space == curSpace {
				lv = cur.Read
			}
			if err := c.SetConversation(r.Account.ID, r.ID, space, lv, r.Name); err != nil {
				return err
			}
		}
		return nil
	}, nil); err != nil {
		m.setStatus("Cannot save: "+err.Error(), true)
		return nil
	}
	clear(m.convSel)
	m.saved(done)
	return nil
}

// cycleRead changes the read level in the order all, mentions, off. Many
// conversations get the next level of the first one.
func (m *model) cycleRead(rows []convRow) tea.Cmd {
	if len(rows) == 0 {
		return nil
	}
	i := slices.Index(config.ReadLevels, rows[0].Read)
	level := config.ReadLevels[(i+1)%len(config.ReadLevels)]
	what := rows[0].Title()
	if len(rows) > 1 {
		what = plural(len(rows), "conversation", "conversations")
	}
	return m.setConversations(rows, "", level, fmt.Sprintf("%s: read %s. Saved.", what, level))
}

// linkDialog asks for one more space of the conversations, then for the read
// level of the new link. The other spaces keep them.
func (m *model) linkDialog(rows []convRow) {
	what := rows[0].Title()
	if len(rows) > 1 {
		what = plural(len(rows), "conversation", "conversations")
	}
	var opts []pickOption
	for _, s := range m.u.cfg.Spaces {
		all := true // each row is in the space already
		for _, r := range rows {
			all = all && slices.Contains(r.Spaces, s.ID)
		}
		if all {
			continue
		}
		note := "→ " + s.Agent + " · " + s.Schedule
		opts = append(opts, pickOption{label: s.Title(), note: note, run: func() tea.Cmd {
			m.levelDialog(rows, what, s)
			return nil
		}})
	}
	if len(opts) == 0 {
		m.setStatus("Each space lists "+what+" already. Add a space in tab 5.", true)
		return
	}
	m.openDialog(&pickDialog{name: "Add to one more space",
		text: "The spaces that list " + what + " keep it. Each space gets its own copy of each message, for its own agent.", options: opts})
}

// levelDialog asks for the read level of the new link in the space s, and adds it.
func (m *model) levelDialog(rows []convRow, what string, s config.Space) {
	var opts []pickOption
	for _, lv := range []string{config.ReadAll, config.ReadMentions} {
		note := map[string]string{config.ReadAll: "each message", config.ReadMentions: "only the messages for you"}[lv]
		opts = append(opts, pickOption{label: lv, note: note, run: func() tea.Cmd {
			if err := m.u.commit(func(c *config.Config) error {
				for _, r := range rows {
					if slices.Contains(c.Routes(r.Account.ID, r.ID), s.ID) {
						continue
					}
					if err := c.AddLink(r.Account.ID, r.ID, s.ID, lv); err != nil {
						return err
					}
				}
				return nil
			}, nil); err != nil {
				m.setStatus("Cannot save: "+err.Error(), true)
				return nil
			}
			clear(m.convSel)
			m.saved(fmt.Sprintf("%s is also in %s now, with the read level %s. Saved.", what, s.Title(), lv))
			return nil
		}})
	}
	m.openDialog(&pickDialog{name: "Read level in " + s.Title(),
		text: "The read level of " + what + " in " + s.Title() + ". The other spaces keep their level.", options: opts})
}

// moveDialog asks for the space of the conversations.
func (m *model) moveDialog(rows []convRow) {
	what := rows[0].Title()
	if len(rows) > 1 {
		what = plural(len(rows), "conversation", "conversations")
	}
	var opts []pickOption
	for _, s := range m.u.cfg.AllSpaces() {
		s := s
		note := "hold: does not send"
		if s.Agent != "" {
			note = "→ " + s.Agent + " · " + s.Schedule
		}
		opts = append(opts, pickOption{label: s.Title(), note: note, run: func() tea.Cmd {
			return m.setConversations(rows, s.ID, "", fmt.Sprintf("Moved %s to %s. Saved.", what, s.Title()))
		}})
	}
	opts = append(opts, pickOption{label: "+ New space …", note: "type a name", run: func() tea.Cmd {
		m.openDialog(newInputDialog("New space", "The name of the new space for "+what+", for example Company B. "+
			"It uses the agent "+m.u.cfg.Agents[0].Title()+" and the schedule "+m.u.cfg.Schedules[0].Title()+"; change them in tab 5.",
			"", "Company B", func(name string) (string, tea.Cmd) {
				id, errText := m.newSpace(name)
				if errText != "" {
					return errText, nil
				}
				return "", m.setConversations(rows, id, "", fmt.Sprintf("Created the space %s and moved %s to it. Saved.", name, what))
			}))
		return nil
	}})
	m.openDialog(&pickDialog{name: "Move to a space", text: "Move " + what + " to:", options: opts})
}

// errNameUsed means that a space with the same name exists.
var errNameUsed = errors.New("the name is used")

// newSpace adds a space with the first agent and the first schedule, and
// saves. It returns its ID, or an error text.
func (m *model) newSpace(name string) (string, string) {
	if name == "" {
		return "", "Type a name."
	}
	var id string
	if err := m.u.commit(func(c *config.Config) error {
		for _, s := range c.AllSpaces() {
			if strings.EqualFold(s.Title(), name) {
				return errNameUsed
			}
		}
		id = config.NewID(name, func(id string) bool { _, ok := c.Space(id); return ok })
		c.Spaces = append(c.Spaces, config.Space{ID: id, Name: name, Agent: c.Agents[0].ID, Schedule: c.Schedules[0].ID})
		return nil
	}, nil); errors.Is(err, errNameUsed) {
		return "", "The name " + name + " is used."
	} else if err != nil {
		return "", "Cannot save: " + err.Error()
	}
	m.savedAny = true
	return id, ""
}

// renameDialog sets the label of a conversation.
func (m *model) renameDialog(r convRow) {
	m.openDialog(newInputDialog("Rename "+r.Title(), "The name that i2a shows for this conversation. Empty: the name on "+
		platformName(r.Account.Platform)+" ("+orStr(r.Name, r.ID)+").", r.Label, orStr(r.Name, r.ID), func(label string) (string, tea.Cmd) {
		if err := m.u.commit(func(c *config.Config) error { return c.SetLabel(r.Account.ID, r.ID, label) }, nil); err != nil {
			return "Cannot save: " + err.Error(), nil
		}
		m.saved("Renamed. Saved.")
		return "", nil
	}))
}

func (m *model) clickConversations(y int) tea.Cmd {
	row := y - 5 + m.convOff // the panel border and title, the toolbar, an empty line and the table header
	if rows := m.shownConvs(); row >= 0 && row < len(rows) {
		m.conv = row
	}
	return nil
}

func (m *model) viewConversations(w, h int) string {
	rows := m.shownConvs()
	m.conv = min(m.conv, max(0, len(rows)-1))
	inner := w - 4
	tableH := h - 7
	m.convOff = scrollTo(m.conv, m.convOff, tableH)

	search := m.search.View()
	if !m.searching && m.search.Value() == "" {
		search = faint.Render("/ search")
	}
	bar := search + faint.Render("  ·  filter (f): ") + boldStyle.Render(m.filterLabel(m.convFilter))
	if n := len(m.convSel); n > 0 {
		bar += "  " + warnStyle.Render(fmt.Sprintf("%d selected: m move · l read level · esc clear", n))
	}
	var loading []string
	for _, a := range m.u.cfg.Accounts {
		switch {
		case m.chsBusy[a.ID]:
			loading = append(loading, a.Title()+": listing …")
		case m.chsErr[a.ID] != "":
			loading = append(loading, a.Title()+": "+m.chsErr[a.ID])
		}
	}
	status := ""
	if len(loading) > 0 {
		status = faint.Render(truncate(strings.Join(loading, " · "), inner))
	}

	cols := []int{2, 2, max(16, inner/4), 8, max(10, inner/7), max(12, inner/6), 14, 8} // READ fits "all, mentions"
	head := colHeadStyle.Render(fmt.Sprintf("%-*s%-*s%-*s %-*s %-*s %-*s %-*s %s", cols[0], "", cols[1], "", cols[2], "CONVERSATION",
		cols[3], "KIND", cols[4], "ACCOUNT", cols[5], "SPACE", cols[6], "READ", "WAITING"))
	lines := []string{bar, status, head}
	if len(rows) == 0 {
		lines = append(lines, faint.Render("No conversation matches. Press f to change the filter, or / to search."))
	}
	end := min(len(rows), m.convOff+tableH)
	for i := m.convOff; i < end; i++ {
		r := rows[i]
		mark := "  "
		if m.convSel[convKey(r.Account.ID, r.ID)] {
			mark = "■ "
		}
		title := r.Title()
		if !r.Entry && r.Space == config.UnsortedID {
			title += " (new)"
		}
		wait := "–"
		if r.Read != config.ReadOff {
			wait = fmt.Sprint(m.waitingIn(r.Account.ID, r.ID))
		}
		text := fmt.Sprintf("%-*s%-*s%-*s %-*s %-*s %-*s %-*s %s", cols[0], mark, cols[1], kindMark(r.Kind), cols[2], truncate(title, cols[2]),
			cols[3], orStr(r.Kind, "?"), cols[4], truncate(r.Account.Title(), cols[4]), cols[5], truncate(m.spacesText(r), cols[5]),
			cols[6], truncate(readsText(r), cols[6]), wait)
		switch {
		case i == m.conv:
			lines = append(lines, selectedRow.Width(inner).Render(truncate(text, inner)))
		case r.Read == config.ReadOff:
			lines = append(lines, faint.Render(truncate(text, inner)))
		case r.Space == config.UnsortedID:
			lines = append(lines, warnStyle.Render(truncate(text, inner)))
		default:
			lines = append(lines, textStyle.Render(truncate(text, inner)))
		}
	}
	title := fmt.Sprintf("Conversations · %d shown · %d unsorted · a DM, a group and a channel have the same choices", len(rows), m.unsortedCount())
	return panel(title, lipgloss.JoinVertical(lipgloss.Left, lines...), w, h)
}
