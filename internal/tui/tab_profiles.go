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

func (m *model) hintsProfiles() []hint {
	if m.prof >= len(m.u.cfg.Profiles) {
		return []hint{{"↑↓", "move"}, {"enter", "add a profile"}}
	}
	return []hint{{"↑↓", "move"}, {"enter", "edit"}, {"x", "delete"}}
}

func (m *model) keyProfiles(k string) tea.Cmd {
	cfg := m.u.cfg
	n := len(cfg.Profiles) + 1
	switch k {
	case "up", "k":
		m.prof = max(0, m.prof-1)
	case "down", "j":
		m.prof = min(n-1, m.prof+1)
	case "a":
		m.prof = n - 1
		m.buildForm(false)
		return m.focusForm()
	case "x":
		if m.prof >= len(cfg.Profiles) {
			return nil
		}
		return m.deleteProfile(cfg.Profiles[m.prof].Name)
	}
	return nil
}

func (m *model) deleteProfile(name string) tea.Cmd {
	if len(m.u.cfg.Profiles) == 1 {
		m.setStatus("Cannot delete the last profile: one is necessary.", true)
		return nil
	}
	m.openDialog(&confirmDialog{name: "Delete profile " + name, text: "Delete the profile " + name + "? Its sources then send to the first profile.",
		choices: []choice{{"y", "Delete", func() tea.Cmd {
			next := clone(m.u.cfg)
			next.Profiles = slices.DeleteFunc(next.Profiles, func(p config.Profile) bool { return p.Name == name })
			for i := range next.Sources {
				if next.Sources[i].Profile == name {
					next.Sources[i].Profile = ""
				}
			}
			if err := m.u.commit(next, nil); err != nil {
				m.setStatus("Cannot delete: "+err.Error(), true)
				return nil
			}
			m.prof = min(m.prof, len(next.Profiles))
			m.saved("Deleted profile " + name + ".")
			return nil
		}}}})
	return nil
}

func (m *model) viewProfiles(w, h int) string {
	cfg := m.u.cfg
	items := make([]string, 0, len(cfg.Profiles)+1)
	for _, p := range cfg.Profiles {
		items = append(items, listLabel(p.Name, p.Driver, listWidth(w)-6))
	}
	items = append(items, "+ Add profile")
	m.prof = min(m.prof, len(items)-1)
	title := "New profile"
	if m.prof < len(cfg.Profiles) {
		p := cfg.Profiles[m.prof]
		used := 0
		for _, s := range cfg.Sources {
			if orStr(s.Profile, cfg.Profiles[0].Name) == p.Name {
				used++
			}
		}
		title = p.Name + " · " + p.Driver + " · used by " + plural(used, "source", "sources")
	}
	return m.masterDetail("Profiles", items, m.prof, &m.profOff, title, w, h)
}

// profileForm edits the profile name, or a new profile when name is "".
func (m *model) profileForm(name string) *form {
	u := m.u
	isNew := name == ""
	p := config.NewProfile(nextProfileName(u.cfg))
	if !isNew {
		cur, _ := u.cfg.Profile(name)
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
	fm.fields = []*formField{
		textField("name", "Name", p.Name, "triage", "A short name, for example triage.", func(s string) error {
			if s != name {
				if _, ok := u.cfg.Profile(s); ok {
					return fmt.Errorf("the name %q is used", s)
				}
			}
			return validName(s)
		}),
		driver,
		warn,
		orca(textField("agent", "Agent", p.Agent, "claude", "The agent that Orca starts, for example claude, codex or opencode.", required)),
		orca(textField("workspace", "Workspace", p.Workspace, "~/notes",
			"The agent runs in this folder. Use a git folder, or add the folder in Orca first.", validWorkspace)),
		orca(choiceField("session", "Session",
			"reuse: one long session · new: a new session for each batch · worktree: a new Orca worktree for each batch",
			[]string{config.SessionReuse, config.SessionNew, config.SessionWorktree}, nil, p.Session)),
		orca(textField("model", "Model", p.Model, "optional", "The model of the agent. Empty means the default of the agent.", nil)),
		choiceField("payload", "Payload", "pointer: the prompt names the digest file. inline: the prompt holds the messages.",
			[]string{config.PayloadPointer, config.PayloadInline}, nil, p.Payload),
		textField("inline", "Inline limit", strconv.Itoa(p.InlineMaxBytes), "8000", "Bytes. A larger inline prompt is sent as a pointer.",
			intRange(config.MinInlineBytes, config.MaxInlineBytes)),
		textField("msgmax", "Message limit", strconv.Itoa(p.MessageMaxChars), "2000", "Characters. i2a truncates longer messages.",
			intRange(100, 20000)),
		areaField("primer", "Primer", orStr(p.Primer, render.DefaultPrimer),
			"standing instructions, sent at the start of a session · fields: .BatchID .Count .Sources .DigestPath .Messages", 8,
			func(s string) error { return render.CheckTemplate("primer", s) }),
		areaField("batch", "Batch prompt", batch, "sent with each batch", 4,
			func(s string) error { return render.CheckTemplate("batch", s) }),
	}
	label := "Save changes"
	if isNew {
		label = "Create profile"
	}
	fm.buttons = []button{
		{label: label, primary: true, run: func() tea.Cmd { return m.saveProfile(name, p) }},
		{label: "Cancel", run: func() tea.Cmd { m.buildForm(true); return nil }},
	}
	if !isNew {
		fm.buttons = append(fm.buttons, button{label: "Delete profile", danger: true, run: func() tea.Cmd {
			m.form.deactivate()
			return m.deleteProfile(name)
		}})
	}
	return fm
}

func (m *model) saveProfile(oldName string, p config.Profile) tea.Cmd {
	fm := m.form
	if !fm.validate() {
		return nil
	}
	v := func(k string) string { return fm.field(k).value() }
	p.Name, p.Driver, p.Payload = v("name"), v("driver"), v("payload")
	if p.Driver == config.DriverOrca {
		p.Agent, p.Workspace, p.Session, p.Model = v("agent"), v("workspace"), v("session"), v("model")
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
	next := clone(m.u.cfg)
	if oldName == "" {
		next.Profiles = append(next.Profiles, p)
	} else {
		for i := range next.Profiles {
			if next.Profiles[i].Name == oldName {
				next.Profiles[i] = p
			}
		}
		for i := range next.Sources {
			if next.Sources[i].Profile == oldName {
				next.Sources[i].Profile = p.Name
			}
		}
	}
	if err := m.u.commit(next, nil); err != nil {
		fm.err = "Cannot save: " + err.Error()
		return nil
	}
	for i, x := range next.Profiles {
		if x.Name == p.Name {
			m.prof = i
		}
	}
	m.saved("Saved profile " + p.Name + ".")
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
