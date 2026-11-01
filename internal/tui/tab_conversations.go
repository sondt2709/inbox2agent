package tui

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

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
	if m.rowCache != nil {
		return m.rowCache // one frame: render computes the rows one time
	}
	return m.listRows()
}

func (m *model) listRows() []convRow {
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

// spacesText names the spaces of a row: "Review, Team"; "Everything else" for
// a row in no space.
func (m *model) spacesText(r convRow) string {
	names := m.spaceNames()
	var out []string
	for _, id := range r.Spaces {
		out = append(out, orStr(names[id], id))
	}
	return strings.Join(out, ", ")
}

// levelText is the read level in the words of the dashboard.
func levelText(read string) string {
	switch read {
	case config.ReadAll:
		return "All messages"
	case config.ReadMentions:
		return "Only for me"
	case config.ReadOff:
		return "Ignored"
	}
	return read
}

// levelIn returns the read level of the row in a space, and false when the
// space does not list the row.
func levelIn(r convRow, space string) (string, bool) {
	for i, id := range r.Spaces {
		if id == space {
			return r.Reads[i], true
		}
	}
	return "", false
}

// inNoSpace is true for a row that only Unsorted (Everything else) reads or ignores.
func inNoSpace(r convRow) bool { return r.Space == config.UnsortedID }

// isNew is true for a listed row that the configuration does not have and
// that the dashboard did not show before.
func (m *model) isNew(r convRow) bool {
	if !r.Listed || r.Entry || !inNoSpace(r) {
		return false
	}
	if _, ok := m.seen[r.Account.ID]; !ok {
		return false // the first listing of the account is not loaded yet
	}
	return !m.seen.has(r.Account.ID, r.ID)
}

// newestFirst sorts rows by the time of the last message, newest first, then
// by name. A row with no time is last.
func newestFirst(rows []convRow) []convRow {
	slices.SortStableFunc(rows, func(a, b convRow) int {
		switch {
		case a.Last.Equal(b.Last):
			return strings.Compare(strings.ToLower(a.Title()), strings.ToLower(b.Title()))
		case a.Last.After(b.Last):
			return -1
		}
		return 1
	})
	return rows
}

// rowsWhere returns the rows that keep returns true for, newest first.
func (m *model) rowsWhere(keep func(convRow) bool) []convRow {
	var out []convRow
	for _, r := range m.convRows() {
		if keep(r) {
			out = append(out, r)
		}
	}
	return newestFirst(out)
}

func (m *model) elseRows() []convRow {
	return m.rowsWhere(func(r convRow) bool { return inNoSpace(r) && r.Read != config.ReadOff })
}

func (m *model) newRows() []convRow { return m.rowsWhere(m.isNew) }

func (m *model) ignoredRows() []convRow {
	return m.rowsWhere(func(r convRow) bool { return inNoSpace(r) && r.Read == config.ReadOff })
}

func (m *model) rowsOfSpace(id string) []convRow {
	return m.rowsWhere(func(r convRow) bool { _, ok := levelIn(r, id); return ok && !inNoSpace(r) })
}

// newCount is the number of new conversations.
func (m *model) newCount() int { return len(m.newRows()) }

// waitingIn returns the number of pending messages of a conversation.
func (m *model) waitingIn(account, id string) int {
	return m.pending[store.Conv{Account: account, ID: id}]
}

// lastText is the age of the last message: "2h ago", "3d ago", or "".
func (m *model) lastText(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	d := m.now().Sub(t)
	switch {
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", max(0, int(d.Minutes())))
	case d < 48*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	case d < 60*24*time.Hour:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
	return t.Local().Format("Jan 2006")
}

// realLinks returns the links of a conversation to spaces, without Unsorted.
func realLinks(c *config.Config, account, id string) []config.Link {
	var out []config.Link
	for _, l := range c.Links(account, id) {
		if l.Space != config.UnsortedID {
			out = append(out, config.Link{Space: l.Space, Read: l.Read})
		}
	}
	return out
}

// editLinks changes the links of each row with edit, which gets the links of
// the file now, and saves. It returns false and shows the error when the save
// fails.
func (m *model) editLinks(rows []convRow, edit func(c *config.Config, r convRow) []config.Link, done string) bool {
	if err := m.u.commitUndoable(func(c *config.Config) error {
		for _, r := range rows {
			if err := c.SetSpaces(r.Account.ID, r.ID, r.Name, edit(c, r)); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		m.setStatus("Cannot save: "+err.Error(), true)
		return false
	}
	m.markSeen(rows...)
	clear(m.convSel)
	m.saved(done + " Saved · u undo")
	return true
}

// markSeen writes the rows to the file of shown conversations.
func (m *model) markSeen(rows ...convRow) {
	by := map[string][]string{}
	for _, r := range rows {
		by[r.Account.ID] = append(by[r.Account.ID], r.ID)
	}
	for acc, ids := range by {
		s, err := addSeen(m.u.d.Paths.DataDir, acc, ids, false)
		if err != nil {
			m.setStatus("Cannot write "+SeenFile+": "+err.Error(), true)
			continue
		}
		m.seen = s
	}
}

// what names the rows in a status text: the name of one row, or "3 conversations".
func what(rows []convRow) string {
	if len(rows) == 1 {
		return rows[0].Title()
	}
	return plural(len(rows), "conversation", "conversations")
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
	if err := m.u.commitUndoable(func(c *config.Config) error {
		for _, s := range c.AllSpaces() {
			if strings.EqualFold(s.Title(), name) {
				return errNameUsed
			}
		}
		id = config.NewID(name, func(id string) bool { _, ok := c.Space(id); return ok })
		c.Spaces = append(c.Spaces, config.Space{ID: id, Name: name, Agent: c.Agents[0].ID, Schedule: c.Schedules[0].ID})
		return nil
	}); errors.Is(err, errNameUsed) {
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
