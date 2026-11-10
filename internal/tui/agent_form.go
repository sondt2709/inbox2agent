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

// sessionLine names the stored session of a space, from the store only: the
// 2 second refresh runs no orca command.
func (m *model) sessionLine(space string) string {
	ss := m.storedSessions()[space]
	switch {
	case ss.Handle == "":
		return "none"
	case ss.AgentSession == "":
		return ss.Handle
	}
	return ss.Handle + " · Claude " + shortID(ss.AgentSession)
}

// agentText describes the agent of a space on one line: the driver and the
// CLI, the folder and the session mode.
func agentText(a config.Agent) string {
	if a.Driver != config.DriverOrca {
		return a.Title()
	}
	return a.Title() + " · " + orStr(a.Folder, "no folder") + " · " + a.Session
}

// agentForm edits the agent and the instruction of the space id (Unsorted too).
func (m *model) agentForm(id string) *form {
	s, ok := m.u.cfg.Space(id)
	if !ok {
		return nil
	}
	p := s.Agent
	fm := &form{}
	driver := choiceField("driver", "Driver", "dry-run writes the digest file only. orca sends the batch to an agent in Orca.",
		[]string{config.DriverDryRun, config.DriverOrca}, nil, p.Driver)
	notOrca := func() bool { return driver.value() != config.DriverOrca }
	orca := func(f *formField) *formField { f.hidden = notOrca; return f }
	warn := noteField("⚠ "+OrcaWarning, warnStyle)
	warn.hidden = notOrca
	fm.fields = []*formField{
		noteField("The agent of "+s.Title()+". Only this space uses it. Session: "+m.sessionLine(id)+".", faint),
		driver,
		warn,
		orca(textField("cli", "CLI", p.CLI, "claude", "The agent that Orca starts, for example claude, codex or opencode.", required)),
		orca(textField("folder", "Folder", p.Folder, "~/notes",
			"The agent runs in this folder. Use a git folder, or add the folder in Orca first.", validWorkspace)),
		orca(choiceField("session", "Session",
			"reuse: one long session for the space · new: a new session for each batch · worktree: a new Orca worktree for each batch",
			[]string{config.SessionReuse, config.SessionNew, config.SessionWorktree}, nil, p.Session)),
		orca(textField("model", "Model", p.Model, "optional", "The model of the agent. Empty means the default of the agent.", nil)),
		choiceField("payload", "Payload", "pointer: the prompt names the digest file. inline: the prompt holds the messages.",
			[]string{config.PayloadPointer, config.PayloadInline}, nil, p.Payload),
		textField("inline", "Inline limit", strconv.Itoa(p.InlineMaxBytes), "8000", "Bytes of YAML content. A larger batch is sent as a pointer.",
			intRange(config.MinInlineBytes, config.MaxInlineBytes)),
		textField("msgmax", "Message limit", strconv.Itoa(p.MessageMaxChars), "2000", "Characters. i2a truncates longer messages.",
			intRange(100, 20000)),
		areaField("instruction", "Instruction", orStr(s.Instruction, render.DefaultInstruction),
			"sent with each batch and each ask, before the messages · plain text", 8, validInstruction),
	}
	fm.buttons = []button{
		{label: "Save changes", primary: true, run: func() tea.Cmd { return m.saveAgent(id, p) }},
		{label: "Cancel", run: func() tea.Cmd { return func() tea.Msg { return formEsc{} } }},
	}
	return fm
}

func (m *model) saveAgent(id string, p config.Agent) tea.Cmd {
	fm := m.form
	if !fm.validate() {
		return nil
	}
	v := func(k string) string { return fm.field(k).value() }
	p.Driver, p.Payload = v("driver"), v("payload")
	if p.Driver == config.DriverOrca {
		p.CLI, p.Folder, p.Session, p.Model = v("cli"), v("folder"), v("session"), v("model")
	}
	p.InlineMaxBytes, _ = strconv.Atoi(v("inline"))
	p.MessageMaxChars, _ = strconv.Atoi(v("msgmax"))
	instruction := keepCustom(v("instruction"), render.DefaultInstruction)
	if p.Driver == config.DriverOrca && p.OrcaBin == "" {
		bin, err := agent.ResolveBin("")
		if err != nil {
			fm.err = err.Error()
			return nil
		}
		p.OrcaBin = bin
	}
	title := m.spaceTitle(id)
	if err := m.commitForm(fm, func(c *config.Config) error {
		if id == config.UnsortedID {
			c.Unsorted.Agent, c.Unsorted.Instruction = p, instruction
			return nil
		}
		cur, ok := c.Space(id)
		if !ok {
			return config.MissingError{Kind: config.KindSpace, ID: id}
		}
		cur.Agent, cur.Instruction = p, instruction
		return nil
	}, nil); err != nil {
		saveError(fm, err)
		return nil
	}
	m.closeSpaceForm()
	m.savedAny = true
	m.setStatus("Saved the agent of "+title+": "+agentText(p)+".", false)
	return nil
}

// openSpaceForm shows the agent form of the space in the panel and gives it the keyboard.
func (m *model) openSpaceForm(id string) tea.Cmd {
	m.spaceForm = id
	m.buildForm(true)
	return m.focusForm()
}

// closeSpaceForm shows the rows of the space again.
func (m *model) closeSpaceForm() {
	m.spaceForm, m.form, m.formFor = "", nil, ""
}

// cancelSpaceForm closes the agent form. When the user changed it, it asks first.
func (m *model) cancelSpaceForm() {
	if m.form == nil || !m.form.dirty() {
		m.closeSpaceForm()
		return
	}
	m.openDialog(&confirmDialog{name: "Unsaved changes", text: "Discard the changes to the agent?",
		choices: []choice{{"d", "Discard", func() tea.Cmd {
			m.closeSpaceForm()
			m.setStatus("Changes discarded.", false)
			return nil
		}}}})
}

// validInstruction refuses three backticks: the instruction goes into a
// fenced block of the prompt.
func validInstruction(s string) error {
	if strings.Contains(s, "```") {
		return render.ErrFenceInInstruction
	}
	return nil
}

// keepCustom returns "" when the text is a default text, so the file stays short.
func keepCustom(s string, defaults ...string) string {
	for _, d := range defaults {
		if strings.TrimSpace(s) == strings.TrimSpace(d) {
			return ""
		}
	}
	return s
}

// usedBy names the spaces that use a schedule.
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

// deleteWithReplacement deletes a schedule. When spaces use it, the user
// chooses the schedule that the spaces use instead. kind is "schedule".
func (m *model) deleteWithReplacement(kind, id string) {
	cfg := m.u.cfg
	s, _ := cfg.Schedule(id)
	title := s.Title()
	var others []pickOption
	for _, o := range cfg.Schedules {
		if o.ID != id {
			others = append(others, pickOption{label: o.Title(), note: o.Text(), run: m.deleteItem(kind, id, o.ID)})
		}
	}
	if len(others) == 0 {
		m.setStatus("Cannot delete the last "+kind+": one is necessary.", true)
		return
	}
	users := cfg.SpacesUsing(kind, id)
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
			c.Schedules = slices.DeleteFunc(c.Schedules, func(s config.Schedule) bool { return s.ID == id })
			return nil
		}, nil); err != nil {
			m.setStatus("Cannot delete: "+err.Error(), true)
			return nil
		}
		m.sch = min(m.sch, len(m.u.cfg.Schedules))
		text := "Deleted the " + kind + " " + id + "."
		if replacement != "" {
			text += " Its spaces use " + replacement + "."
		}
		m.saved(text)
		return nil
	}
}
