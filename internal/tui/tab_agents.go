package tui

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/sondt2709/inbox2agent/internal/agent"
	"github.com/sondt2709/inbox2agent/internal/config"
	"github.com/sondt2709/inbox2agent/internal/render"
)

func (m *model) hintsAgents() []hint {
	if m.agt >= len(m.u.cfg.Agents) {
		return []hint{{"↑↓", "move"}, {"enter", "add an agent"}}
	}
	return []hint{{"↑↓", "move"}, {"enter", "edit"}, {"x", "delete"}}
}

func (m *model) keyAgents(k string) tea.Cmd {
	n := len(m.u.cfg.Agents) + 1
	switch k {
	case "up", "k":
		m.agt = max(0, m.agt-1)
	case "down", "j":
		m.agt = min(n-1, m.agt+1)
	case "a":
		m.agt = n - 1
		m.buildForm(false)
		return m.focusForm()
	case "x":
		if m.agt < len(m.u.cfg.Agents) {
			m.deleteWithReplacement("agent", m.u.cfg.Agents[m.agt].ID)
		}
	}
	return nil
}

// usedBy names the spaces that use an agent or a schedule.
func (m *model) usedBy(kind, id string) string {
	var names []string
	for _, s := range m.u.cfg.SpacesUsing(kind, id) {
		names = append(names, s.Title())
	}
	if len(names) == 0 {
		return "No space uses it."
	}
	return "Used by " + strings.Join(names, ", ") + "."
}

// deleteWithReplacement deletes an agent or a schedule. When spaces use it,
// the user chooses the item that the spaces use instead. kind is "agent" or "schedule".
func (m *model) deleteWithReplacement(kind, id string) {
	cfg := m.u.cfg
	var others []pickOption
	var title string
	if kind == "agent" {
		a, _ := cfg.Agent(id)
		title = a.Title()
		for _, o := range cfg.Agents {
			if o.ID != id {
				others = append(others, pickOption{label: o.Title(), note: o.Driver, run: m.deleteItem(kind, id, o.ID)})
			}
		}
	} else {
		s, _ := cfg.Schedule(id)
		title = s.Title()
		for _, o := range cfg.Schedules {
			if o.ID != id {
				others = append(others, pickOption{label: o.Title(), note: o.Text(), run: m.deleteItem(kind, id, o.ID)})
			}
		}
	}
	if len(others) == 0 {
		m.setStatus("Cannot delete the last "+kind+": one is necessary.", true)
		return
	}
	users := cfg.SpacesUsing(kind, id)
	if kind == "agent" && cfg.Unsorted.Agent == id && len(users) == 0 {
		users = []config.Space{cfg.UnsortedSpace()}
	}
	if len(users) == 0 {
		m.openDialog(&confirmDialog{name: "Delete " + kind + " " + title, text: "Delete the " + kind + " " + title + "? No space uses it.",
			choices: []choice{{"y", "Delete", m.deleteItem(kind, id, "")}}})
		return
	}
	var names []string
	for _, s := range users {
		names = append(names, s.Title())
	}
	m.openDialog(&pickDialog{name: "Delete " + kind + " " + title,
		text:    fmt.Sprintf("%s %s use the %s %s. Choose the %s that they use instead:", plural(len(users), "space", "spaces"), strings.Join(names, ", "), kind, title, kind),
		options: others})
}

func (m *model) deleteItem(kind, id, replacement string) func() tea.Cmd {
	return func() tea.Cmd {
		if err := m.u.commit(func(c *config.Config) error {
			if replacement != "" {
				c.Replace(kind, id, replacement)
			}
			if kind == "agent" {
				c.Agents = slices.DeleteFunc(c.Agents, func(a config.Agent) bool { return a.ID == id })
			} else {
				c.Schedules = slices.DeleteFunc(c.Schedules, func(s config.Schedule) bool { return s.ID == id })
			}
			return nil
		}, nil); err != nil {
			m.setStatus("Cannot delete: "+err.Error(), true)
			return nil
		}
		m.agt, m.sch = min(m.agt, len(m.u.cfg.Agents)), min(m.sch, len(m.u.cfg.Schedules))
		text := "Deleted the " + kind + " " + id + "."
		if replacement != "" {
			text += " Its spaces use " + replacement + "."
		}
		m.saved(text)
		return nil
	}
}

func (m *model) viewAgents(w, h int) string {
	cfg := m.u.cfg
	items := make([]string, 0, len(cfg.Agents)+1)
	for _, a := range cfg.Agents {
		items = append(items, listLabel(a.Title(), a.Driver, listWidth(w)-6))
	}
	items = append(items, "+ Add agent")
	m.agt = min(m.agt, len(items)-1)
	title := "New agent"
	if m.agt < len(cfg.Agents) {
		a := cfg.Agents[m.agt]
		title = a.Title() + " · " + a.Driver + " · ID " + a.ID
	}
	return m.masterDetail("Agents", items, m.agt, &m.agtOff, title, w, h)
}

// agentForm edits the agent id, or a new agent when id is "".
func (m *model) agentForm(id string) *form {
	u := m.u
	isNew := id == ""
	p := config.NewAgent(newID("agent", func(s string) bool { _, ok := u.cfg.Agent(s); return ok }))
	if !isNew {
		cur, _ := u.cfg.Agent(id)
		p = *cur
	}
	batch := p.Batch
	if batch == "" {
		batch = render.DefaultPointerBatch
		if p.Payload == config.PayloadInline {
			batch = render.DefaultInlineBatch
		}
	}
	fm := &form{}
	driver := choiceField("driver", "Driver", "dry-run writes the digest file only. orca sends the batch to an agent in Orca.",
		[]string{config.DriverDryRun, config.DriverOrca}, nil, p.Driver)
	notOrca := func() bool { return driver.value() != config.DriverOrca }
	orca := func(f *formField) *formField { f.hidden = notOrca; return f }
	warn := noteField("⚠ "+OrcaWarning, warnStyle)
	warn.hidden = notOrca
	idField := textField("id", "Agent ID", p.ID, "claude-work", "A short ID that does not change, for example claude-work.", func(s string) error {
		if _, ok := u.cfg.Agent(s); ok {
			return fmt.Errorf("the ID %q is used", s)
		}
		return validName(s)
	})
	if !isNew {
		idField = noteField(m.usedBy("agent", id)+" Each space has its own session: the data of one space never goes to the session of another.", faint)
	}
	fm.fields = []*formField{
		idField,
		textField("name", "Name", p.Name, "Claude for work", "The name that i2a shows. The ID stays the same after a rename.", required),
		driver,
		warn,
		orca(textField("cli", "CLI", p.CLI, "claude", "The agent that Orca starts, for example claude, codex or opencode.", required)),
		orca(textField("workspace", "Workspace", p.Workspace, "~/notes",
			"The agent runs in this folder. Use a git folder, or add the folder in Orca first.", validWorkspace)),
		orca(choiceField("session", "Session",
			"reuse: one long session for each space · new: a new session for each batch · worktree: a new Orca worktree for each batch",
			[]string{config.SessionReuse, config.SessionNew, config.SessionWorktree}, nil, p.Session)),
		orca(textField("model", "Model", p.Model, "optional", "The model of the agent. Empty means the default of the agent.", nil)),
		choiceField("payload", "Payload", "pointer: the prompt names the digest file. inline: the prompt holds the messages.",
			[]string{config.PayloadPointer, config.PayloadInline}, nil, p.Payload),
		textField("inline", "Inline limit", strconv.Itoa(p.InlineMaxBytes), "8000", "Bytes. A larger inline prompt is sent as a pointer.",
			intRange(config.MinInlineBytes, config.MaxInlineBytes)),
		textField("msgmax", "Message limit", strconv.Itoa(p.MessageMaxChars), "2000", "Characters. i2a truncates longer messages.",
			intRange(100, 20000)),
		areaField("primer", "Primer", orStr(p.Primer, render.DefaultPrimer),
			"standing instructions, sent at the start of a session · fields: .Space .Agent .BatchID .Count .Accounts .DigestPath .Messages", 8,
			func(s string) error { return render.CheckTemplate("primer", s) }),
		areaField("batch", "Batch prompt", batch, "sent with each batch", 4,
			func(s string) error { return render.CheckTemplate("batch", s) }),
	}
	label := "Save changes"
	if isNew {
		label = "Create agent"
	}
	fm.buttons = []button{
		{label: label, primary: true, run: func() tea.Cmd { return m.saveAgent(id, p) }},
		{label: "Cancel", run: func() tea.Cmd { m.buildForm(true); return nil }},
	}
	if !isNew {
		fm.buttons = append(fm.buttons, button{label: "Delete agent", danger: true, run: func() tea.Cmd {
			m.form.deactivate()
			m.deleteWithReplacement("agent", id)
			return nil
		}})
	}
	return fm
}

func (m *model) saveAgent(id string, p config.Agent) tea.Cmd {
	fm := m.form
	if !fm.validate() {
		return nil
	}
	v := func(k string) string { return fm.field(k).value() }
	if id == "" {
		p.ID = v("id")
	}
	p.Name, p.Driver, p.Payload = v("name"), v("driver"), v("payload")
	if p.Driver == config.DriverOrca {
		p.CLI, p.Workspace, p.Session, p.Model = v("cli"), v("workspace"), v("session"), v("model")
	}
	p.InlineMaxBytes, _ = strconv.Atoi(v("inline"))
	p.MessageMaxChars, _ = strconv.Atoi(v("msgmax"))
	p.Primer = keepCustom(v("primer"), render.DefaultPrimer)
	p.Batch = keepCustom(v("batch"), render.DefaultPointerBatch, render.DefaultInlineBatch)
	if p.Driver == config.DriverOrca && p.OrcaBin == "" {
		bin, err := agent.ResolveBin("")
		if err != nil {
			fm.err = err.Error()
			return nil
		}
		p.OrcaBin = bin
	}
	if err := m.commitForm(fm, func(c *config.Config) error {
		if id == "" {
			c.Agents = append(c.Agents, p)
			return nil
		}
		a, ok := c.Agent(id)
		if !ok {
			return config.MissingError{Kind: config.KindAgent, ID: id}
		}
		*a = p
		return nil
	}, nil); err != nil {
		saveError(fm, err)
		return nil
	}
	for i, x := range m.u.cfg.Agents {
		if x.ID == p.ID {
			m.agt = i
		}
	}
	m.saved("Saved the agent " + p.Title() + ".")
	return nil
}

// keepCustom returns "" when the text is a default template, so the file stays short.
func keepCustom(s string, defaults ...string) string {
	for _, d := range defaults {
		if strings.TrimSpace(s) == strings.TrimSpace(d) {
			return ""
		}
	}
	return s
}
