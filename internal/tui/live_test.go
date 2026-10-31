package tui

import (
	"os"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/sondt2709/inbox2agent/internal/config"
)

// appendSpace adds a space to config.toml as another process does.
func appendSpace(t *testing.T, m *model, id, name string) {
	t.Helper()
	f, err := os.OpenFile(m.u.d.Paths.ConfigFile, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.WriteString("\n[[spaces]]\nid = '" + id + "'\nname = '" + name + "'\nagent = 'claude-work'\nschedule = 'always'\n"); err != nil {
		t.Fatal(err)
	}
}

func TestSaveKeepsTheSpaceOfAnotherProcess(t *testing.T) {
	m := newTestModel(t)
	appendSpace(t, m, "side-project", "Side project")
	send(m, "8", "enter", "ctrl+u", "2", "d", "ctrl+s")
	text := fileText(t, m)
	if !strings.Contains(text, "lookback = '2d'") {
		t.Fatalf("the save did not write the lookback:\n%s", text)
	}
	if !strings.Contains(text, "side-project") {
		t.Fatalf("the save removed the space of another process:\n%s", text)
	}
}

// changeFile changes config.toml with the locked update, as another process does.
func changeFile(t *testing.T, m *model, change func(*config.Config) error) {
	t.Helper()
	if _, _, err := config.Update(m.u.d.Paths.ConfigFile, change); err != nil {
		t.Fatal(err)
	}
}

func loadFile(t *testing.T, m *model) *config.Config {
	t.Helper()
	c, err := config.Load(m.u.d.Paths.ConfigFile)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// refresh runs one refresh of the dashboard, as the 2-second tick does.
func refresh(m *model) { run(m, func() tea.Msg { return m.load()() }, 0) }

func TestEachTabSaveKeepsTheChangeOfAnotherProcess(t *testing.T) {
	for _, tc := range []struct {
		name  string
		edit  func(m *model)
		check func(c *config.Config) bool
	}{
		{"accounts", func(m *model) {
			send(m, "3", "enter")
			m.form.field("name").input.SetValue("Company A")
			send(m, "ctrl+s")
		}, func(c *config.Config) bool { a, _ := c.Account("work"); return a.Name == "Company A" }},
		{"agents", func(m *model) {
			send(m, "6", "enter")
			m.form.field("name").input.SetValue("Triage bot")
			send(m, "ctrl+s")
		}, func(c *config.Config) bool { a, _ := c.Agent("triage"); return a.Name == "Triage bot" }},
		{"schedules", func(m *model) {
			send(m, "7", "enter")
			m.form.field("every").input.SetValue("30m")
			send(m, "ctrl+s")
		}, func(c *config.Config) bool { s, _ := c.Schedule("always"); return s.Every.String() == "30m" }},
		{"spaces", func(m *model) {
			send(m, "5", "down", "enter")
			m.form.field("name").input.SetValue("Acme Corp")
			send(m, "ctrl+s")
		}, func(c *config.Config) bool { s, _ := c.Space("acme"); return s.Name == "Acme Corp" }},
		{"conversations", func(m *model) {
			send(m, "4", "l") // the read level of the first row
		}, func(c *config.Config) bool { return c.Level("work", "C1") != config.ReadAll }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := newTestModel(t)
			appendSpace(t, m, "side-project", "Side project")
			tc.edit(m)
			c := loadFile(t, m)
			if !tc.check(c) {
				t.Fatalf("the save did not write the change:\n%s\n%s", fileText(t, m), screen(m))
			}
			if _, ok := c.Space("side-project"); !ok {
				t.Fatalf("the save removed the space of another process:\n%s", fileText(t, m))
			}
		})
	}
}

func TestSaveOfARemovedItemFails(t *testing.T) {
	m := newTestModel(t)
	send(m, "5", "down", "enter")
	m.form.field("name").input.SetValue("Acme Corp")
	changeFile(t, m, func(c *config.Config) error {
		c.Unsorted.Conversations = append(c.Unsorted.Conversations, c.Spaces[0].Conversations...)
		c.Spaces = nil
		return nil
	})
	send(m, "ctrl+s")
	if !strings.Contains(m.form.err, "the space acme does not exist any more") {
		t.Fatalf("form error %q, want the missing space", m.form.err)
	}
	if _, ok := loadFile(t, m).Space("acme"); ok {
		t.Error("the save wrote the removed space again")
	}
}

// conflict opens the form of the schedule always, types 30m, and lets another
// process set 5m before the save.
func conflict(t *testing.T) *model {
	t.Helper()
	m := newTestModel(t)
	send(m, "7", "enter")
	m.form.field("every").input.SetValue("30m")
	changeFile(t, m, func(c *config.Config) error {
		s, _ := c.Schedule("always")
		s.Every = config.Duration(5 * time.Minute)
		return nil
	})
	send(m, "ctrl+s")
	if d := m.topDialog(); d == nil || d.title() != "Changed in another process" {
		t.Fatalf("dialog %v, want the conflict dialog", d)
	}
	if m.form.err != "" {
		t.Errorf("the form shows an error under the dialog: %q", m.form.err)
	}
	return m
}

func TestConflictReload(t *testing.T) {
	m := conflict(t)
	send(m, "enter") // Reload is the default
	if s, _ := loadFile(t, m).Schedule("always"); s.Every.String() != "5m" {
		t.Errorf("file every = %s, want 5m", s.Every)
	}
	if v := m.form.field("every").value(); v != "5m" {
		t.Errorf("form every = %q, want 5m", v)
	}
}

func TestConflictOverwrite(t *testing.T) {
	m := conflict(t)
	send(m, "o")
	if s, _ := loadFile(t, m).Schedule("always"); s.Every.String() != "30m" {
		t.Errorf("file every = %s, want 30m", s.Every)
	}
}

func TestRefreshShowsASpaceOfAnotherProcess(t *testing.T) {
	m := newTestModel(t)
	appendSpace(t, m, "side-project", "Side project")
	refresh(m)
	if s := screen(m); !strings.Contains(s, "Side project") || !strings.Contains(s, "changed in another process") {
		t.Fatalf("the Spaces tab does not show the new space:\n%s", s)
	}
}

func TestRefreshKeepsTheConfigurationOfAnInvalidFile(t *testing.T) {
	m := newTestModel(t)
	b := strings.Replace(fileText(t, m), "every = '15m'", "every = 'ten minutes'", 1)
	if err := os.WriteFile(m.u.d.Paths.ConfigFile, []byte(b), 0o600); err != nil {
		t.Fatal(err)
	}
	refresh(m)
	s := screen(m)
	if !strings.Contains(s, "Acme") || !strings.Contains(s, "config.toml has an error") || !strings.Contains(s, "every") {
		t.Fatalf("an invalid file: the status or the spaces are wrong:\n%s", s)
	}
	if _, ok := m.u.cfg.Space("acme"); !ok {
		t.Error("the dashboard lost the space acme")
	}
}

func TestRefreshKeepsADirtyForm(t *testing.T) {
	m := newTestModel(t)
	send(m, "5", "down", "enter")
	m.form.field("name").input.SetValue("Acme Corp")
	if !m.form.dirty() {
		t.Fatal("the form is not dirty")
	}
	changeFile(t, m, func(c *config.Config) error {
		c.Schedules = append(c.Schedules, config.NewSchedule("night"))
		return nil
	})
	refresh(m)
	if !m.form.dirty() || m.form.field("name").value() != "Acme Corp" {
		t.Fatalf("the reload changed the form: dirty %v, name %q", m.form.dirty(), m.form.field("name").value())
	}
	if _, ok := m.u.cfg.Schedule("night"); !ok {
		t.Error("the dashboard did not load the new schedule")
	}
	if !strings.Contains(screen(m), "Your unsaved edits stay") {
		t.Errorf("the status line does not explain the reload:\n%s", screen(m))
	}
}

func TestTheErrorOfAnInvalidFileStays(t *testing.T) {
	m := newTestModel(t)
	now := time.Now()
	m.now = func() time.Time { return now }
	b := strings.Replace(fileText(t, m), "every = '15m'", "every = 'ten minutes'", 1)
	if err := os.WriteFile(m.u.d.Paths.ConfigFile, []byte(b), 0o600); err != nil {
		t.Fatal(err)
	}
	refresh(m)
	now = now.Add(10 * time.Second) // longer than the status line shows a message
	refresh(m)
	if !strings.Contains(screen(m), "config.toml has an error") {
		t.Fatalf("the error went away while the file is still invalid:\n%s", screen(m))
	}
}
