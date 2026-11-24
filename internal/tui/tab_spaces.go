package tui

import (
	"fmt"
	"slices"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/sondt2709/inbox2agent/internal/config"
)

// The items of the list at the left of the Spaces area, after the spaces.
const (
	railElse    = "else"
	railNew     = "new"
	railIgnored = "ignored"
	railAdd     = "add"
)

// railItem is one row of the list at the left: a space, or one of the rail* kinds.
type railItem struct {
	kind  string // "space" or a rail* kind
	space string // the space ID; UnsortedID for railElse
}

func (m *model) railItems() []railItem {
	var out []railItem
	for _, s := range m.u.cfg.Spaces {
		out = append(out, railItem{kind: "space", space: s.ID})
	}
	return append(out, railItem{kind: railElse, space: config.UnsortedID}, railItem{kind: railNew},
		railItem{kind: railIgnored}, railItem{kind: railAdd})
}

func (m *model) selectedRail() railItem {
	items := m.railItems()
	m.spc = min(max(0, m.spc), len(items)-1)
	return items[m.spc]
}

// selectSpace moves the cursor of the list to a space.
func (m *model) selectSpace(id string) {
	for i, it := range m.railItems() {
		if it.space == id {
			m.spc = i
		}
	}
}

// pRow is one row of the panel at the right. A row with an empty kind is text.
type pRow struct {
	kind string // "set" (a setting), "conv", "add" (the button + Add conversations), or "" (text)
	key  string // the setting: "name", "agent", "schedule", "read"
	conv convRow
	text string // the rendered text of a text row
}

func (r pRow) selectable() bool { return r.kind != "" }

// panelRows returns the rows of the panel of the selected item.
func (m *model) panelRows(w int) []pRow {
	it := m.selectedRail()
	text := func(s string) pRow { return pRow{text: s} }
	var out []pRow
	switch it.kind {
	case "space", railElse:
		if it.kind == "space" {
			out = append(out, pRow{kind: "set", key: "name"})
		} else {
			out = append(out, pRow{kind: "set", key: "read"})
		}
		out = append(out, pRow{kind: "set", key: "status"}, pRow{kind: "set", key: "agent"}, pRow{kind: "set", key: "session"},
			pRow{kind: "set", key: "schedule"})
		out = append(out, text(m.settingLine("Last send", m.lastSendText(it.space), w)), text(""))
		rows := m.rowsOfSpace(it.space)
		head := fmt.Sprintf("Conversations · %d", len(rows))
		empty := "No conversation yet. Add conversations from any account."
		if it.kind == railElse {
			rows = m.elseRows()
			head = fmt.Sprintf("Conversations in no space · %d · newest first", len(rows))
			empty = "Each conversation is in a space or ignored."
		}
		out = append(out, text(boldStyle.Render(head)))
		if it.kind == "space" {
			out = append(out, pRow{kind: "add"})
		}
		if len(rows) == 0 {
			out = append(out, text(faint.Render(empty)))
		}
		for _, r := range rows {
			out = append(out, pRow{kind: "conv", conv: r})
		}
		if sends := m.recentSends(it.space, 5); sends != "" {
			out = append(out, text(""), text(boldStyle.Render("Recent sends")))
			for l := range strings.SplitSeq(sends, "\n") {
				out = append(out, text(l))
			}
		}
	case railNew:
		out = append(out, text(textStyle.Render("The accounts found these conversations after the dashboard showed them last.")),
			text(textStyle.Render("Until you decide, Everything else reads them.")), text(""))
		rows := m.newRows()
		if len(rows) == 0 {
			out = append(out, text(faint.Render("No new conversation.")))
		}
		for _, r := range rows {
			out = append(out, pRow{kind: "conv", conv: r})
		}
	case railIgnored:
		out = append(out, text(textStyle.Render("i2a does not read these conversations.")), text(""))
		rows := m.ignoredRows()
		if len(rows) == 0 {
			out = append(out, text(faint.Render("No ignored conversation.")))
		}
		for _, r := range rows {
			out = append(out, pRow{kind: "conv", conv: r})
		}
	case railAdd:
		out = append(out, text(textStyle.Render("A space is your own group of conversations from any account:")),
			text(textStyle.Render("a client, a project, personal. It sends them to one agent on one schedule.")),
			text(""), text(faint.Render("Press enter to add a space.")))
	}
	return out
}

// settingLine is a label and a value in two columns.
func (m *model) settingLine(label, value string, w int) string {
	return faint.Render(fmt.Sprintf("%-11s", label)) + truncateStyled(textStyle.Render(value), max(5, w-11))
}

// settingText renders a setting row of the selected item.
func (m *model) settingText(key string, w int) string {
	it := m.selectedRail()
	s, _ := m.u.cfg.Space(it.space)
	switch key {
	case "name":
		return m.settingLine("Name", s.Title(), w)
	case "read":
		return m.settingLine("New and other conversations read", "", 0) + " " + textStyle.Render(levelText(m.u.cfg.Unsorted.Read))
	case "status":
		switch {
		case !s.Paused:
			return m.settingLine("Status", okStyle.Render("● Sending"), w)
		case s.ID == config.UnsortedID:
			return m.settingLine("Status", "○ Hold: does not send · messages wait until they expire after "+m.u.cfg.Settings.Lookback.String(), w)
		}
		return m.settingLine("Status", "○ Paused: does not send · messages wait until they expire after "+m.u.cfg.Settings.Lookback.String(), w)
	case "agent":
		v := agentText(s.Agent)
		if s.Instruction != "" {
			v += faint.Render(" · instruction: " + strings.Join(strings.Fields(s.Instruction), " "))
		}
		return m.settingLine("Agent", v, w)
	case "session":
		return m.settingLine("Session", m.sessionLine(s.ID), w)
	case "schedule":
		v := s.Schedule
		if sc, ok := m.u.cfg.Schedule(s.Schedule); ok {
			v = sc.Title() + faint.Render(" · "+sc.Text()+" · next "+m.whenText(sc.Next(m.now())))
		}
		return m.settingLine("When", v, w)
	}
	return ""
}

// convLine renders a conversation row of the panel.
func (m *model) convLine(r convRow, w int) string {
	it := m.selectedRail()
	mark := "  "
	if m.convSel[convKey(r.Account.ID, r.ID)] {
		mark = "■ "
	}
	level := levelText(r.Read)
	if lv, ok := levelIn(r, it.space); ok && it.kind == "space" {
		level = levelText(lv)
	}
	var notes []string
	if it.kind == "space" && len(r.Spaces) > 1 {
		var others []string
		for _, id := range r.Spaces {
			if id != it.space {
				others = append(others, m.spaceNames()[id])
			}
		}
		notes = append(notes, "also in "+strings.Join(others, ", "))
	}
	if it.kind == railElse && m.isNew(r) {
		notes = append(notes, "new")
	}
	if n := m.waitingIn(r.Account.ID, r.ID); n > 0 && r.Read != config.ReadOff {
		notes = append(notes, fmt.Sprintf("%d waiting", n))
	}
	if t := m.lastText(r.Last); t != "" {
		notes = append(notes, t)
	}
	nameW := max(12, w/3)
	accW := max(8, w/7)
	return fmt.Sprintf("%s%s %-*s %-*s %-13s %s", mark, kindMark(r.Kind), nameW, truncate(r.Title(), nameW),
		accW, truncate(r.Account.Title(), accW), level, strings.Join(notes, " · "))
}

// lastSendText describes the newest send of a space.
func (m *model) lastSendText(space string) string {
	for _, d := range m.dispatches {
		if d.Space == space {
			return fmt.Sprintf("%s · %s %d · %s", d.CreatedAt.Local().Format("01-02 15:04"), d.Status, d.MessageCount,
				plural(m.spaceWaiting(space), "message waits", "messages wait"))
		}
	}
	return "none yet · " + plural(m.spaceWaiting(space), "message waits", "messages wait")
}

// recentSends lists the newest sends of a space, one line each.
func (m *model) recentSends(space string, n int) string {
	var lines []string
	for _, d := range m.dispatches {
		if d.Space != space {
			continue
		}
		lines = append(lines, faint.Render(fmt.Sprintf("%s  %-8s %3d  %s", d.CreatedAt.Local().Format("01-02 15:04"), d.Status,
			d.MessageCount, d.Detail)))
		if len(lines) == n {
			break
		}
	}
	return strings.Join(lines, "\n")
}

// spaceWaiting returns the number of pending messages that route to a space.
func (m *model) spaceWaiting(id string) int {
	n := 0
	for c, k := range m.pending {
		if slices.Contains(m.u.cfg.Routes(c.Account, c.ID), id) {
			n += k
		}
	}
	return n
}

// ---- keys ----

func (m *model) hintsSpaces() []hint {
	it := m.selectedRail()
	if !m.spcFocus {
		switch it.kind {
		case railAdd:
			return []hint{{"enter", "add a space"}, {"↑↓", "move"}}
		case railNew, railIgnored:
			return []hint{{"enter", "open"}, {"ctrl+r", "run now"}, {"↑↓", "move"}, {"u", "undo"}}
		case railElse:
			return []hint{{"enter", "open"}, {"ctrl+r", "run now"}, {"↑↓", "move"}, {"u", "undo"}}
		}
		return []hint{{"enter", "open"}, {"ctrl+r", "run now"}, {"a", "add conversations"}, {"x", "delete"}, {"↑↓", "move"}, {"u", "undo"}}
	}
	if r, ok := m.panelSelected(); ok && r.kind == "add" {
		return []hint{{"enter", "add conversations"}, {"esc", "back"}, {"ctrl+r", "run now"}, {"u", "undo"}}
	}
	if r, ok := m.panelSelected(); ok && r.kind == "set" {
		if r.key == "read" {
			return []hint{{"←→", "change"}, {"enter", "choose"}, {"esc", "back"}, {"ctrl+r", "run now"}, {"u", "undo"}}
		}
		if it.kind != "space" {
			return []hint{{"enter", "change"}, {"esc", "back"}, {"ctrl+r", "run now"}, {"u", "undo"}}
		}
		return []hint{{"enter", "change"}, {"esc", "back"}, {"ctrl+r", "run now"}, {"a", "add conversations"}, {"u", "undo"}}
	}
	sel := ""
	if n := len(m.convSel); n > 0 {
		sel = fmt.Sprintf(" %d", n)
	}
	switch it.kind {
	case railNew:
		return []hint{{"enter", "spaces" + sel}, {"k", "keep"}, {"i", "ignore"}, {"K", "keep all"}, {"space", "select"}, {"esc", "back"}, {"ctrl+r", "run now"}, {"u", "undo"}}
	case railIgnored:
		return []hint{{"enter", "spaces" + sel}, {"x", "read again"}, {"space", "select"}, {"esc", "back"}, {"ctrl+r", "run now"}, {"u", "undo"}}
	case railElse:
		return []hint{{"enter", "spaces" + sel}, {"←→", "level"}, {"i", "ignore"}, {"space", "select"}, {"n", "rename"}, {"esc", "back"}, {"ctrl+r", "run now"}, {"u", "undo"}}
	}
	return []hint{{"enter", "spaces" + sel}, {"←→", "level"}, {"x", "remove"}, {"a", "add"}, {"space", "select"}, {"n", "rename"}, {"esc", "back"}, {"ctrl+r", "run now"}, {"u", "undo"}}
}

// panelSelected returns the row under the cursor of the panel.
func (m *model) panelSelected() (pRow, bool) {
	rows := m.panelRows(m.panelWidth())
	sel := selectableIndexes(rows)
	if len(sel) == 0 {
		return pRow{}, false
	}
	m.row = min(max(0, m.row), len(sel)-1)
	return rows[sel[m.row]], true
}

func selectableIndexes(rows []pRow) []int {
	var out []int
	for i, r := range rows {
		if r.selectable() {
			out = append(out, i)
		}
	}
	return out
}

func (m *model) panelWidth() int { return max(20, m.w-listWidth(m.w)-4) }

// targetsOf returns the selected rows of the panel, or the row r when no
// selected row exists any more.
func (m *model) targetsOf(r convRow) []convRow {
	var out []convRow
	for _, x := range m.convRows() {
		if m.convSel[convKey(x.Account.ID, x.ID)] {
			out = append(out, x)
		}
	}
	if len(out) == 0 {
		clear(m.convSel)
		return []convRow{r}
	}
	return out
}

func (m *model) keySpaces(k string) tea.Cmd {
	it := m.selectedRail()
	switch k {
	case "ctrl+r":
		if it.space != "" {
			return m.runNow([]string{it.space}, m.spaceNames()[it.space])
		}
		return m.runNow(nil, "all spaces")
	case "u":
		m.undo()
		return nil
	case "a":
		if it.kind == "space" {
			m.openDialog(m.newSearchDialog(it.space))
		}
		return nil
	}
	if !m.spcFocus {
		return m.keyRail(k, it)
	}
	return m.keyPanel(k, it)
}

func (m *model) keyRail(k string, it railItem) tea.Cmd {
	switch k {
	case "up", "k":
		m.spc = max(0, m.spc-1)
		m.row, m.rowOff = 0, 0
		clear(m.convSel)
	case "down", "j":
		m.spc = min(len(m.railItems())-1, m.spc+1)
		m.row, m.rowOff = 0, 0
		clear(m.convSel)
	case "enter", "right", "l":
		if it.kind == railAdd {
			m.addSpaceDialog(nil)
			return nil
		}
		m.spcFocus, m.row, m.rowOff = true, 0, 0
		if it.kind != "space" && it.kind != railElse {
			return nil
		}
		// Start on the first conversation when the space has one, else on
		// the button + Add conversations.
		rows := m.panelRows(m.panelWidth())
		for i, idx := range selectableIndexes(rows) {
			if rows[idx].kind == "conv" {
				m.row = i
				break
			}
			if rows[idx].kind == "add" {
				m.row = i
			}
		}
	case "x":
		if it.kind == "space" {
			m.deleteSpace(it.space)
		}
	}
	return nil
}

func (m *model) keyPanel(k string, it railItem) tea.Cmd {
	rows := m.panelRows(m.panelWidth())
	sel := selectableIndexes(rows)
	r, ok := m.panelSelected()
	if it.kind == railNew && (k == "k" || k == "K") {
		k = "keep:" + k // in New conversations k keeps a row; ↑ moves
	}
	switch k {
	case "up", "k":
		m.row = max(0, m.row-1)
		return nil
	case "down", "j":
		m.row = min(max(0, len(sel)-1), m.row+1)
		return nil
	case "esc":
		if len(m.convSel) > 0 {
			clear(m.convSel)
			return nil
		}
		m.spcFocus = false
		return nil
	}
	if !ok {
		if k == "left" || k == "h" {
			m.spcFocus = false
		}
		return nil
	}
	if r.kind == "add" {
		switch k {
		case "enter", "right", "l":
			m.openDialog(m.newSearchDialog(it.space))
		case "left", "h":
			m.spcFocus = false
		}
		return nil
	}
	if r.kind == "set" {
		switch k {
		case "left", "h":
			if r.key == "read" {
				return m.setElseRead(-1)
			}
			m.spcFocus = false
		case "right", "l":
			if r.key == "read" {
				return m.setElseRead(1)
			}
		case "enter":
			return m.settingDialog(it, r.key)
		case "s":
			if r.key == "session" {
				return m.settingDialog(it, r.key)
			}
		}
		return nil
	}
	c := r.conv
	switch k {
	case "space":
		key := convKey(c.Account.ID, c.ID)
		if m.convSel[key] {
			delete(m.convSel, key)
		} else {
			m.convSel[key] = true
		}
		m.row = min(len(sel)-1, m.row+1)
	case "enter":
		m.openDialog(m.newSpacesDialog(m.targetsOf(c)))
	case "left", "right", "h", "l":
		switch it.kind {
		case "space":
			m.toggleLevel(m.targetsOf(c), it.space)
		case railElse:
			m.toggleElseLevel(m.targetsOf(c))
		}
	case "x":
		switch it.kind {
		case "space":
			s, _ := m.u.cfg.Space(it.space)
			t := m.targetsOf(c)
			m.editLinks(t, func(cf *config.Config, x convRow) []config.Link {
				return orNoSpace(cf, x, slices.DeleteFunc(realLinks(cf, x.Account.ID, x.ID), func(l config.Link) bool { return l.Space == it.space }))
			}, fmt.Sprintf("Removed %s from %s.", what(t), s.Title()))
		case railIgnored:
			t := m.targetsOf(c)
			m.editLinks(t, noSpace, what(t)+": Everything else reads it again.")
		}
	case "i":
		if it.kind == railElse || it.kind == railNew {
			t := m.targetsOf(c)
			m.editLinks(t, func(*config.Config, convRow) []config.Link {
				return []config.Link{{Space: config.UnsortedID, Read: config.ReadOff}}
			}, "Ignored "+what(t)+".")
		}
	case "keep:k":
		if it.kind == railNew {
			t := m.targetsOf(c)
			m.markSeen(t...)
			clear(m.convSel)
			m.setStatus("Kept "+what(t)+" in Everything else.", false)
		}
	case "keep:K":
		if it.kind == railNew {
			t := m.newRows()
			m.markSeen(t...)
			clear(m.convSel)
			m.setStatus("Kept "+what(t)+" in Everything else.", false)
		}
	case "n":
		m.renameDialog(c)
	}
	return nil
}

// toggleLevel switches the level of the rows in a space between all and mentions.
func (m *model) toggleLevel(rows []convRow, space string) {
	cur, _ := levelIn(rows[0], space)
	next := map[string]string{config.ReadAll: config.ReadMentions}[cur]
	if next == "" {
		next = config.ReadAll
	}
	s, _ := m.u.cfg.Space(space)
	m.editLinks(rows, func(c *config.Config, r convRow) []config.Link {
		ls := realLinks(c, r.Account.ID, r.ID)
		for i := range ls {
			if ls[i].Space == space {
				ls[i].Read = next
			}
		}
		return ls
	}, fmt.Sprintf("%s in %s: %s.", what(rows), s.Title(), levelText(next)))
}

// toggleElseLevel switches the level of rows in Everything else between all and mentions.
func (m *model) toggleElseLevel(rows []convRow) {
	next := config.ReadAll
	if rows[0].Read == config.ReadAll {
		next = config.ReadMentions
	}
	m.editLinks(rows, func(*config.Config, convRow) []config.Link {
		return []config.Link{{Space: config.UnsortedID, Read: next}}
	}, fmt.Sprintf("%s in Everything else: %s.", what(rows), levelText(next)))
}

// setElseRead changes the read level of the conversations that no entry lists.
func (m *model) setElseRead(step int) tea.Cmd {
	lv := config.ReadLevels
	i := slices.Index(lv, m.u.cfg.Unsorted.Read)
	next := lv[(i+step+len(lv))%len(lv)]
	m.commitSpaces(func(c *config.Config) error { c.Unsorted.Read = next; return nil },
		"New and other conversations: "+levelText(next)+".")
	return nil
}

// commitSpaces saves a change of the Spaces area and shows the result.
func (m *model) commitSpaces(change func(c *config.Config) error, done string) bool {
	if err := m.u.commitUndoable(change); err != nil {
		m.setStatus("Cannot save: "+err.Error(), true)
		return false
	}
	m.saved(done + " Saved · u undo")
	return true
}

// settingDialog acts on a setting row: a dialog, the agent form, or the
// pause of the space.
func (m *model) settingDialog(it railItem, key string) tea.Cmd {
	s, _ := m.u.cfg.Space(it.space)
	id := it.space
	switch key {
	case "name":
		m.openDialog(newInputDialog("Rename "+s.Title(), "The name that i2a shows. The ID "+id+" stays.", s.Name, "Company B",
			func(name string) (string, tea.Cmd) {
				if name == "" {
					return "Type a name.", nil
				}
				for _, x := range m.u.cfg.AllSpaces() {
					if x.ID != id && strings.EqualFold(x.Title(), name) {
						return "The name " + name + " is used.", nil
					}
				}
				m.commitSpaces(func(c *config.Config) error {
					cur, ok := c.Space(id)
					if !ok {
						return config.MissingError{Kind: config.KindSpace, ID: id}
					}
					cur.Name = name
					return nil
				}, "Renamed the space to "+name+".")
				return "", nil
			}))
	case "read":
		var opts []pickOption
		for _, lv := range config.ReadLevels {
			opts = append(opts, pickOption{label: levelText(lv), note: lv, run: func() tea.Cmd {
				m.commitSpaces(func(c *config.Config) error { c.Unsorted.Read = lv; return nil },
					"New and other conversations: "+levelText(lv)+".")
				return nil
			}})
		}
		m.openDialog(&pickDialog{name: "New and other conversations read",
			text: "The read level of each conversation that no space lists and that you did not set.", options: opts,
			cursor: slices.Index(config.ReadLevels, m.u.cfg.Unsorted.Read)})
	case "status":
		paused := !s.Paused
		text := s.Title() + " sends again."
		if paused {
			text = s.Title() + " is paused: its messages wait."
		}
		m.commitSpaces(func(c *config.Config) error {
			if id == config.UnsortedID {
				c.Unsorted.Paused = paused
				return nil
			}
			cur, ok := c.Space(id)
			if !ok {
				return config.MissingError{Kind: config.KindSpace, ID: id}
			}
			cur.Paused = paused
			return nil
		}, text)
	case "agent":
		return m.openSpaceForm(id)
	case "session":
		return m.openSessions(*s)
	case "schedule":
		var opts []pickOption
		cursor := 0
		for i, sc := range m.u.cfg.Schedules {
			if sc.ID == s.Schedule {
				cursor = i
			}
			opts = append(opts, pickOption{label: "Use " + sc.Title(), note: sc.Text(), run: func() tea.Cmd {
				m.commitSpaces(func(c *config.Config) error {
					if id == config.UnsortedID {
						c.Unsorted.Schedule = sc.ID
						return nil
					}
					cur, ok := c.Space(id)
					if !ok {
						return config.MissingError{Kind: config.KindSpace, ID: id}
					}
					cur.Schedule = sc.ID
					return nil
				}, s.Title()+" sends "+sc.Text()+".")
				return nil
			}})
		}
		cur, ok := m.u.cfg.Schedule(s.Schedule)
		if ok {
			sc := *cur
			opts = append(opts,
				pickOption{label: "Rename " + sc.Title() + "…", note: "the ID " + sc.ID + " stays", run: func() tea.Cmd {
					m.renameScheduleDialog(sc)
					return nil
				}},
				pickOption{label: "Edit the times of " + sc.Title() + " in Setup", note: "every, days, hours", run: func() tea.Cmd {
					return m.editSchedule(sc.ID)
				}})
		}
		m.openDialog(&pickDialog{name: "Schedule of " + s.Title(), text: "When this space sends. " + m.usedBy("schedule", s.Schedule),
			options: opts, cursor: cursor})
	}
	return nil
}

// renameScheduleDialog changes the name of a schedule; its ID stays.
func (m *model) renameScheduleDialog(sc config.Schedule) {
	id := sc.ID
	m.openDialog(newInputDialog("Rename "+sc.Title(), "The name that i2a shows. The ID "+id+" stays. "+m.usedBy("schedule", id), sc.Name, "Work hours",
		func(name string) (string, tea.Cmd) {
			if name == "" {
				return "Type a name.", nil
			}
			m.commitSpaces(func(c *config.Config) error {
				cur, ok := c.Schedule(id)
				if !ok {
					return config.MissingError{Kind: config.KindSchedule, ID: id}
				}
				cur.Name = name
				return nil
			}, "Renamed the schedule "+id+" to "+name+".")
			return "", nil
		}))
}

// editSchedule opens Setup › Schedules with the schedule and gives its form the keyboard.
func (m *model) editSchedule(id string) tea.Cmd {
	for i, sc := range m.u.cfg.Schedules {
		if sc.ID == id {
			m.sch = i
		}
	}
	cmd := m.setTab(tabSchedules)
	return tea.Batch(cmd, m.focusForm())
}

// addSpaceDialog asks for the name of a new space. then gets its ID.
func (m *model) addSpaceDialog(then func(id string)) {
	m.openDialog(newInputDialog("New space", "The name of the new space, for example Company B. It gets a copy of the agent of "+
		m.spaceTitle(m.agentSource())+" and the schedule "+m.u.cfg.Schedules[0].Title()+"; change them in its panel.",
		"", "Company B", func(name string) (string, tea.Cmd) {
			id, errText := m.newSpace(name)
			if errText != "" {
				return errText, nil
			}
			if then != nil {
				then(id)
				return "", nil
			}
			m.selectSpace(id)
			m.spcFocus, m.row = true, 0
			m.saved("Created the space " + name + ". Click + Add conversations or press a. Saved · u undo")
			return "", nil
		}))
}

func (m *model) deleteSpace(id string) {
	s, _ := m.u.cfg.Space(id)
	name := s.Title()
	n := len(s.Conversations)
	m.openDialog(&confirmDialog{name: "Delete space " + name,
		text: fmt.Sprintf("Delete the space %s? Its %s go to Everything else, except the ones that another space also lists. Its cards stay in the Feed.",
			name, plural(n, "conversation", "conversations")),
		choices: []choice{{"y", "Delete", func() tea.Cmd {
			if m.commitSpaces(func(c *config.Config) error {
				cur, ok := c.Space(id)
				if !ok {
					return config.MissingError{Kind: config.KindSpace, ID: id}
				}
				gone := slices.Clone(cur.Conversations)
				// SetSpaces keeps the label: a conversation that another space
				// lists stays there, the others keep their level in Everything else.
				for _, cv := range gone {
					rest := slices.DeleteFunc(realLinks(c, cv.Account, cv.ID), func(l config.Link) bool { return l.Space == id })
					if len(rest) == 0 {
						rest = []config.Link{{Space: config.UnsortedID, Read: cv.Read}}
					}
					if err := c.SetSpaces(cv.Account, cv.ID, cv.Name, rest); err != nil {
						return err
					}
				}
				c.Spaces = slices.DeleteFunc(c.Spaces, func(x config.Space) bool { return x.ID == id })
				return nil
			}, "Deleted the space "+name+". Its conversations are in Everything else.") {
				m.spc = min(m.spc, len(m.railItems())-1)
				m.spcFocus = false
			}
			return nil
		}}}})
}

// undo writes back the configuration from before the last save.
func (m *model) undo() {
	switch err := m.u.undo(); {
	case err == errNoUndo:
		m.setStatus("Nothing to undo.", true)
	case err != nil:
		m.setStatus("Cannot undo: "+err.Error(), true)
	default:
		m.saved("Undone. Saved · u again to redo")
	}
}

// ---- view ----

func (m *model) viewSpaces(w, h int) string {
	cfg := m.u.cfg
	lw := listWidth(w)
	inner := lw - 8 // the panel border and padding, the bar, the dot and its space
	var items []string
	newN, ignN := m.newCount(), len(m.ignoredRows())
	for _, it := range m.railItems() {
		switch it.kind {
		case "space":
			s, _ := cfg.Space(it.space)
			dot := okStyle.Render("●")
			if s.Paused {
				dot = faint.Render("○")
			}
			items = append(items, dot+" "+listLabel(s.Title(), countText(len(m.rowsOfSpace(s.ID)), m.spaceWaiting(s.ID)), inner))
		case railElse:
			dot := okStyle.Render("●")
			if cfg.Unsorted.Paused {
				dot = faint.Render("○")
			}
			items = append(items, dot+" "+listLabel("Everything else", countText(len(m.elseRows()), m.spaceWaiting(config.UnsortedID)), inner))
		case railNew:
			label := listLabel("New conversations", fmt.Sprint(newN), inner)
			if newN > 0 {
				label = warnStyle.Render(label)
			}
			items = append(items, "  "+label)
		case railIgnored:
			items = append(items, "  "+listLabel("Ignored", fmt.Sprint(ignN), inner))
		case railAdd:
			items = append(items, "  + New space")
		}
	}
	rows := h - 3
	m.spcOff = scrollTo(m.spc, m.spcOff, rows)
	list := railRows(items, m.spc, m.spcOff, lw-4, rows, !m.spcFocus)
	left := panel("Spaces", list, lw, h)

	pw := w - lw
	if m.spaceForm != "" && m.form != nil {
		body := m.form.render(pw-4, h-4)
		return lipgloss.JoinHorizontal(lipgloss.Top, left, detailPanel("Agent of "+m.spaceTitle(m.spaceForm), body, pw, h, m.form.active))
	}
	body, title := m.renderPanel(pw-4, h-3)
	var right string
	if m.spcFocus {
		right = detailPanel(title, body, pw, h, true)
	} else {
		right = panel(title, body, pw, h)
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, left, right)
}

// countText is the number of conversations, and the waiting messages when
// some wait: "5", "5 · 3⧗".
func countText(convs, waiting int) string {
	if waiting > 0 {
		return fmt.Sprintf("%d · %d⧗", convs, waiting)
	}
	return fmt.Sprint(convs)
}

// railRows draws the list at the left. The selected row has the bar; it is
// grey when the panel has the keyboard.
func railRows(items []string, cursor, offset, w, h int, focus bool) string {
	var b strings.Builder
	end := min(len(items), offset+h)
	for i := offset; i < end; i++ {
		text := truncateStyled(items[i], w-1)
		switch {
		case i == cursor && focus:
			b.WriteString(selectedRow.Width(w).Render("▌" + ansi.Strip(text)))
		case i == cursor:
			b.WriteString(boldStyle.Render("▌" + ansi.Strip(text)))
		default:
			b.WriteString(" " + text)
		}
		if i < end-1 {
			b.WriteString("\n")
		}
	}
	return b.String()
}

// renderPanel renders the rows of the panel, scrolled to the cursor.
func (m *model) renderPanel(w, h int) (string, string) {
	it := m.selectedRail()
	title := map[string]string{railElse: "Everything else", railNew: "New conversations", railIgnored: "Ignored", railAdd: "New space"}[it.kind]
	if it.kind == "space" {
		s, _ := m.u.cfg.Space(it.space)
		title = s.Title()
	}
	if it.kind == railElse {
		title = "Everything else · conversations in no space"
	}
	rows := m.panelRows(w)
	sel := selectableIndexes(rows)
	cur := -1
	if len(sel) > 0 {
		m.row = min(max(0, m.row), len(sel)-1)
		cur = sel[m.row]
	}
	lines := make([]string, len(rows))
	for i, r := range rows {
		var s string
		switch r.kind {
		case "set":
			s = m.settingText(r.key, w-2)
		case "add":
			s = tabKey.Render("+ Add conversations")
		case "conv":
			s = m.convLine(r.conv, w-2)
			if r.conv.Read == config.ReadOff && it.kind != railIgnored {
				s = faint.Render(s)
			}
		default:
			s = r.text
		}
		if i == cur && m.spcFocus {
			s = selectedRow.Width(w).Render(truncate("▌"+ansi.Strip(s), w))
		} else {
			s = " " + s
		}
		lines[i] = s
	}
	if cur >= 0 {
		m.rowOff = scrollTo(cur, m.rowOff, h)
	} else {
		m.rowOff = 0
	}
	end := min(len(lines), m.rowOff+h)
	return strings.Join(lines[min(m.rowOff, end):end], "\n"), title
}

// whenText is "today 14:45" or "Mon 08:00"; "never" for the zero time.
func (m *model) whenText(t time.Time) string { return dayTime(t, m.now()) }

// dayTime is "today 14:45" or "Mon 08:00" in the time zone of t; "never"
// for the zero time.
func dayTime(t, now time.Time) string {
	if t.IsZero() {
		return "never"
	}
	if t.YearDay() == now.YearDay() && t.Year() == now.Year() {
		return "today " + t.Format("15:04")
	}
	return t.Format("Mon 15:04")
}

// nextSend returns the next send time of all spaces that are not paused.
func (m *model) nextSend() time.Time {
	var next time.Time
	for _, s := range m.u.cfg.AllSpaces() {
		if s.Paused {
			continue
		}
		if sc, ok := m.u.cfg.Schedule(s.Schedule); ok {
			if t := sc.Next(m.now()); !t.IsZero() && (next.IsZero() || t.Before(next)) {
				next = t
			}
		}
	}
	return next
}

// cardsOf counts the shown cards of a space ("" for all).
func (m *model) cardsOf(space string) int {
	n := 0
	for _, c := range m.cards {
		if m.cardShown(c) && (space == "" || m.cardSpace(c) == space) {
			n++
		}
	}
	return n
}

// wheel moves the panel under the pointer, as ↑ and ↓ do there, and gives
// that panel the keyboard.
func (m *model) wheel(x int, k string) tea.Cmd {
	switch m.tab {
	case tabFeed:
		rw := feedRailWidth(m.w)
		switch {
		case x < rw:
			m.feedFocus = focusSpaces
		case x < rw+feedListWidth(m.w-rw):
			m.feedFocus = focusCards
		default:
			m.feedFocus = focusCard
		}
	case tabSpaces:
		// The list, or the panel when the item has rows.
		m.spcFocus = x >= listWidth(m.w) && m.selectedRail().kind != railAdd
	}
	return m.tabKey(k)
}

// clickSpaces selects the clicked row of the list or of the panel. A click
// on the selected row opens it.
func (m *model) clickSpaces(x, y int) tea.Cmd {
	line := y - 2 // the panel border and the panel title
	if line < 0 {
		return nil
	}
	if x < listWidth(m.w) {
		i := line + m.spcOff
		if i >= len(m.railItems()) {
			return nil
		}
		if i == m.spc && !m.spcFocus {
			return m.keyRail("enter", m.selectedRail())
		}
		m.spc, m.spcFocus, m.row, m.rowOff = i, false, 0, 0
		clear(m.convSel)
		return nil
	}
	idx := line + m.rowOff
	rows := m.panelRows(m.panelWidth())
	for i, at := range selectableIndexes(rows) {
		if at == idx {
			// The button + Add conversations opens with one click; another
			// row opens with a click on the selected row.
			if (i == m.row && m.spcFocus) || rows[at].kind == "add" {
				m.row, m.spcFocus = i, true
				return m.keyPanel("enter", m.selectedRail())
			}
			m.row, m.spcFocus = i, true
		}
	}
	return nil
}
