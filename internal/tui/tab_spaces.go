package tui

import (
	"fmt"
	"slices"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/sondt2709/inbox2agent/internal/config"
)

// selectedSpace returns the ID of the selected space: Unsorted is row 0. It
// returns false on "+ New space".
func (m *model) selectedSpace() (string, bool) {
	switch {
	case m.spc == 0:
		return config.UnsortedID, true
	case m.spc <= len(m.u.cfg.Spaces):
		return m.u.cfg.Spaces[m.spc-1].ID, true
	}
	return "", false
}

func (m *model) hintsSpaces() []hint {
	id, ok := m.selectedSpace()
	switch {
	case !ok:
		return []hint{{"↑↓", "move"}, {"enter", "add a space"}}
	case id == config.UnsortedID:
		return []hint{{"↑↓", "move"}, {"enter", "edit"}, {"c", "sort its conversations"}, {"R", "send now"}}
	}
	return []hint{{"↑↓", "move"}, {"enter", "edit"}, {"a", "add conversations"}, {"c", "its conversations"}, {"R", "send now"}, {"x", "delete"}}
}

func (m *model) keySpaces(k string) tea.Cmd {
	n := len(m.u.cfg.Spaces) + 2
	id, ok := m.selectedSpace()
	switch k {
	case "up", "k":
		m.spc = max(0, m.spc-1)
	case "down", "j":
		m.spc = min(n-1, m.spc+1)
	case "c":
		if ok {
			return m.showConversations(id, false)
		}
	case "a":
		if ok && id != config.UnsortedID {
			return m.showConversations(id, true)
		}
	case "x":
		if ok && id != config.UnsortedID {
			m.deleteSpace(id)
		}
	case "R":
		if !ok {
			m.setStatus("Select a space, then press R.", true)
			return nil
		}
		return m.runNow([]string{id}, m.spaceNames()[id])
	}
	return nil
}

// showConversations opens the Conversations tab with the conversations of a
// space, or, to add, with the unsorted conversations and the space as the
// target of m.
func (m *model) showConversations(id string, add bool) tea.Cmd {
	s, _ := m.u.cfg.Space(id)
	m.convFilter, m.conv = "space:"+id, 0
	if id == config.UnsortedID {
		m.convFilter = "unsorted"
	}
	if add {
		m.convFilter = "unsorted"
		m.setStatus("Select the conversations for "+s.Title()+" with space, then press m and choose "+s.Title()+". Press f to see other filters.", false)
	}
	return m.setTab(tabConversations)
}

func (m *model) deleteSpace(id string) {
	s, _ := m.u.cfg.Space(id)
	name := s.Title()
	n := len(s.Conversations)
	m.openDialog(&confirmDialog{name: "Delete space " + name,
		text: fmt.Sprintf("Delete the space %s? Its %s go to Unsorted. Its cards stay in the Feed.", name, plural(n, "conversation", "conversations")),
		choices: []choice{{"y", "Delete", func() tea.Cmd {
			if err := m.u.commit(func(c *config.Config) error {
				cur, ok := c.Space(id)
				if !ok {
					return config.MissingError{Kind: config.KindSpace, ID: id}
				}
				gone := cur.Conversations
				c.Spaces = slices.DeleteFunc(c.Spaces, func(x config.Space) bool { return x.ID == id })
				for _, cv := range gone {
					// A conversation that another space lists stays in that space only.
					if _, _, listed := c.Conversation(cv.Account, cv.ID); !listed {
						c.Unsorted.Conversations = append(c.Unsorted.Conversations, cv)
					}
				}
				return nil
			}, nil); err != nil {
				m.setStatus("Cannot delete: "+err.Error(), true)
				return nil
			}
			m.spc = min(m.spc, len(m.u.cfg.Spaces))
			m.saved("Deleted the space " + name + ". Its conversations are in Unsorted.")
			return nil
		}}}})
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

// spaceRows returns the conversations of a space; for Unsorted, also the
// listed conversations that no space has.
func (m *model) spaceRows(id string) []convRow {
	var out []convRow
	for _, r := range m.convRows() {
		if slices.Contains(r.Spaces, id) {
			out = append(out, r)
		}
	}
	return out
}

func (m *model) viewSpaces(w, h int) string {
	cfg := m.u.cfg
	lw := listWidth(w) - 6
	items := []string{listLabel("□ Unsorted", fmt.Sprintf("%d to sort", m.unsortedCount()), lw)}
	for _, s := range cfg.Spaces {
		items = append(items, listLabel("■ "+s.Title(), fmt.Sprintf("%d · %d wait", len(s.Conversations), m.spaceWaiting(s.ID)), lw))
	}
	items = append(items, "+ New space")
	m.spc = min(m.spc, len(items)-1)
	title := "New space"
	if id, ok := m.selectedSpace(); ok {
		s, _ := cfg.Space(id)
		title = s.Title() + " · space"
		if id == config.UnsortedID {
			title = "Unsorted · built-in space"
		}
	}
	return m.masterDetail("Spaces", items, m.spc, &m.spcOff, title, w, h)
}

// sentence describes what a space does, in one sentence.
func (m *model) sentence(s config.Space) string {
	cfg := m.u.cfg
	rows := m.spaceRows(s.ID)
	reading, accounts := 0, []string{}
	for _, r := range rows {
		if r.Read != config.ReadOff {
			reading++
			if !slices.Contains(accounts, r.Account.Title()) {
				accounts = append(accounts, r.Account.Title())
			}
		}
	}
	from := ""
	if len(accounts) > 0 {
		from = " from " + strings.Join(accounts, ", ")
	}
	wait := plural(m.spaceWaiting(s.ID), "message", "messages")
	if s.Agent == "" {
		return fmt.Sprintf("%s holds %s%s. It does not send to an agent: %s wait until you move their conversations to a space, or until they expire after %s.",
			s.Title(), plural(len(rows), "conversation", "conversations"), from, wait, cfg.Settings.Lookback)
	}
	agent, schedule := s.Agent, s.Schedule
	text, next := "", "never"
	if a, ok := cfg.Agent(s.Agent); ok {
		agent = a.Title()
	}
	if sc, ok := cfg.Schedule(s.Schedule); ok {
		schedule, text = sc.Title(), " ("+sc.Text()+")"
		next = m.whenText(sc.Next(m.now()))
	}
	return fmt.Sprintf("%s reads %s%s → sends to %s · %s%s. Next send: %s with %s now waiting.",
		s.Title(), plural(reading, "conversation", "conversations"), from, agent, schedule, text, next, wait)
}

// whenText is "today 14:45" or "Mon 08:00"; "never" for the zero time.
func (m *model) whenText(t time.Time) string {
	if t.IsZero() {
		return "never"
	}
	now := m.now()
	if t.YearDay() == now.YearDay() && t.Year() == now.Year() {
		return "today " + t.Format("15:04")
	}
	return t.Format("Mon 15:04")
}

func (m *model) agentChoice(current string, hold bool) *formField {
	var ids, labels []string
	if hold {
		ids, labels = append(ids, ""), append(labels, "hold")
	}
	for _, a := range m.u.cfg.Agents {
		ids, labels = append(ids, a.ID), append(labels, a.Title())
	}
	help := "The agent that gets the messages of this space. Each space has its own session."
	if hold {
		help = "hold: do not send; the messages wait until you sort their conversations, or until they expire."
	}
	return choiceField("agent", "Agent", help, ids, labels, current)
}

func (m *model) scheduleChoice(current string) *formField {
	var ids, labels []string
	for _, s := range m.u.cfg.Schedules {
		ids, labels = append(ids, s.ID), append(labels, s.Title())
	}
	return choiceField("schedule", "Schedule", "When this space sends. Change the times in tab 7.", ids, labels, current)
}

// convTable is the list of the conversations of a space, for a note field.
func (m *model) convTable(rows []convRow, limit int) string {
	if len(rows) == 0 {
		return "No conversation yet."
	}
	var lines []string
	for i, r := range rows {
		if i == limit {
			lines = append(lines, fmt.Sprintf("… and %d more: press esc, then c", len(rows)-limit))
			break
		}
		lines = append(lines, fmt.Sprintf("%-9s %s %-28s %-14s %d waiting", r.Read, kindMark(r.Kind), truncate(r.Title(), 28),
			truncate(r.Account.Title(), 14), m.waitingIn(r.Account.ID, r.ID)))
	}
	return strings.Join(lines, "\n")
}

// spaceForm edits a space, or a new space when id is "".
func (m *model) spaceForm(id string) *form {
	cfg := m.u.cfg
	s := config.Space{Agent: cfg.Agents[0].ID, Schedule: cfg.Schedules[0].ID}
	if id != "" {
		cur, _ := cfg.Space(id)
		s = *cur
	}
	fm := &form{}
	if id != "" {
		fm.fields = append(fm.fields, noteField(m.sentence(s), textStyle))
	} else {
		fm.fields = append(fm.fields, noteField("A space is your own group: a company, a project, personal. After you create it, add conversations from any account with a.", faint))
	}
	fm.fields = append(fm.fields,
		textField("name", "Name", s.Name, "Company B", "The name that i2a shows. The ID stays the same after a rename.", func(v string) error {
			if strings.TrimSpace(v) == "" {
				return fmt.Errorf("type a name")
			}
			for _, x := range cfg.AllSpaces() {
				if x.ID != id && strings.EqualFold(x.Title(), strings.TrimSpace(v)) {
					return fmt.Errorf("the name %q is used", x.Title())
				}
			}
			return nil
		}),
		m.agentChoice(s.Agent, false),
		m.scheduleChoice(s.Schedule),
	)
	if id != "" {
		fm.fields = append(fm.fields,
			noteField("Conversations · a DM, a group and a channel have the same read levels: all · mentions (messages for you) · off", boldStyle),
			noteField(m.convTable(m.spaceRows(id), 12), faint))
		fm.buttons = []button{
			{label: "Save changes", primary: true, run: func() tea.Cmd { return m.saveSpace(id) }},
			{label: "Cancel", run: func() tea.Cmd { m.buildForm(true); return nil }},
			{label: "Delete space", danger: true, run: func() tea.Cmd { m.form.deactivate(); m.deleteSpace(id); return nil }},
		}
		return fm
	}
	fm.buttons = []button{{label: "Create space", primary: true, run: func() tea.Cmd { return m.saveSpace("") }}}
	return fm
}

func (m *model) saveSpace(id string) tea.Cmd {
	fm := m.form
	if !fm.validate() {
		return nil
	}
	name := strings.TrimSpace(fm.field("name").value())
	agent, sched := fm.field("agent").value(), fm.field("schedule").value()
	if err := m.commitForm(fm, func(c *config.Config) error {
		if id == "" {
			id = config.NewID(name, func(x string) bool { _, ok := c.Space(x); return ok })
			c.Spaces = append(c.Spaces, config.Space{ID: id, Name: name})
		}
		s, ok := c.Space(id)
		if !ok {
			return config.MissingError{Kind: config.KindSpace, ID: id}
		}
		s.Name, s.Agent, s.Schedule = name, agent, sched
		return nil
	}, nil); err != nil {
		saveError(fm, err)
		return nil
	}
	for i, x := range m.u.cfg.Spaces {
		if x.ID == id {
			m.spc = i + 1
		}
	}
	m.saved("Saved the space " + name + ".")
	return nil
}

// unsortedForm edits the built-in space.
func (m *model) unsortedForm() *form {
	u := m.u.cfg.Unsorted
	s := m.u.cfg.UnsortedSpace()
	agent := m.agentChoice(u.Agent, true)
	sched := m.scheduleChoice(u.Schedule)
	sched.hidden = func() bool { return agent.value() == "" }
	fm := &form{}
	fm.fields = []*formField{
		noteField(m.sentence(s), textStyle),
		noteField("New conversations land here. Move each one to a space in tab 4 (m), or set it to off (l).", warnStyle),
		choiceField("read", "Read level", "For each conversation that no space lists: all · mentions (messages for you) · off.",
			config.ReadLevels, nil, u.Read),
		agent,
		sched,
		noteField("Conversations to sort", boldStyle),
		noteField(m.convTable(m.spaceRows(config.UnsortedID), 12), faint),
	}
	fm.buttons = []button{
		{label: "Save changes", primary: true, run: func() tea.Cmd { return m.saveUnsorted() }},
		{label: "Cancel", run: func() tea.Cmd { m.buildForm(true); return nil }},
	}
	return fm
}

func (m *model) saveUnsorted() tea.Cmd {
	fm := m.form
	read, agent, sched := fm.field("read").value(), fm.field("agent").value(), fm.field("schedule").value()
	if err := m.commitForm(fm, func(c *config.Config) error {
		c.Unsorted.Read, c.Unsorted.Agent = read, agent
		if agent != "" {
			c.Unsorted.Schedule = sched
		}
		return nil
	}, nil); err != nil {
		saveError(fm, err)
		return nil
	}
	m.saved("Saved Unsorted.")
	return nil
}
