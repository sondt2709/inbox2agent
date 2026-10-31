package tui

import (
	"fmt"
	"slices"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/sondt2709/inbox2agent/internal/config"
)

func (m *model) hintsSchedules() []hint {
	if m.sch >= len(m.u.cfg.Schedules) {
		return []hint{{"↑↓", "move"}, {"enter", "add a schedule"}}
	}
	return []hint{{"↑↓", "move"}, {"enter", "edit"}, {"x", "delete"}}
}

func (m *model) keySchedules(k string) tea.Cmd {
	n := len(m.u.cfg.Schedules) + 1
	switch k {
	case "up", "k":
		m.sch = max(0, m.sch-1)
	case "down", "j":
		m.sch = min(n-1, m.sch+1)
	case "a":
		m.sch = n - 1
		m.buildForm(false)
		return m.focusForm()
	case "x":
		if m.sch < len(m.u.cfg.Schedules) {
			m.deleteWithReplacement("schedule", m.u.cfg.Schedules[m.sch].ID)
		}
	}
	return nil
}

func (m *model) viewSchedules(w, h int) string {
	cfg := m.u.cfg
	items := make([]string, 0, len(cfg.Schedules)+1)
	for _, s := range cfg.Schedules {
		items = append(items, listLabel(s.Title(), "every "+s.Every.String(), listWidth(w)-6))
	}
	items = append(items, "+ Add schedule")
	m.sch = min(m.sch, len(items)-1)
	title := "New schedule"
	if m.sch < len(cfg.Schedules) {
		s := cfg.Schedules[m.sch]
		title = s.Title() + " · ID " + s.ID
	}
	return m.masterDetail("Schedules", items, m.sch, &m.schOff, title, w, h)
}

// todayLine draws the send times of today on a line of w cells from 00:00 to
// 24:00, with ┃ for the current time.
func todayLine(s config.Schedule, now time.Time, w int) string {
	w = max(24, w)
	cells := make([]rune, w)
	for i := range cells {
		cells[i] = '·'
	}
	day := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	for t := s.Next(day.Add(-time.Nanosecond)); !t.IsZero() && t.Before(day.Add(24*time.Hour)); t = s.Next(t) {
		cells[min(w-1, int(t.Sub(day).Minutes())*w/(24*60))] = '│'
	}
	cur := min(w-1, (now.Hour()*60+now.Minute())*w/(24*60))
	line := accentText(string(cells[:cur])) + warnStyle.Render("┃") + accentText(string(cells[cur+1:]))
	scale := "00" + strings.Repeat(" ", max(1, w/4-2)) + "06" + strings.Repeat(" ", max(1, w/4-2)) + "12" +
		strings.Repeat(" ", max(1, w/4-2)) + "18" + strings.Repeat(" ", max(1, w-3*(w/4)-4)) + "24"
	return line + "\n" + faint.Render(scale)
}

func accentText(s string) string { return panelTitle.UnsetBold().Render(s) }

func validDays(s string) error {
	for _, d := range splitDays(s) {
		if !slices.Contains(config.Days, d) {
			return fmt.Errorf("%q is not a day: use mon, tue, wed, thu, fri, sat, sun", d)
		}
	}
	return nil
}

func splitDays(s string) []string {
	return strings.FieldsFunc(strings.ToLower(s), func(r rune) bool { return r == ',' || r == ' ' })
}

// scheduleForm edits the schedule id, or a new schedule when id is "".
func (m *model) scheduleForm(id string) *form {
	u := m.u
	isNew := id == ""
	s := config.NewSchedule(newID("schedule", func(x string) bool { _, ok := u.cfg.Schedule(x); return ok }))
	if !isNew {
		cur, _ := u.cfg.Schedule(id)
		s = *cur
	}
	fm := &form{}
	if isNew {
		fm.fields = append(fm.fields, textField("id", "Schedule ID", s.ID, "work-hours", "A short ID that does not change.", func(v string) error {
			if _, ok := u.cfg.Schedule(v); ok {
				return fmt.Errorf("the ID %q is used", v)
			}
			return validName(v)
		}))
	} else {
		next := m.whenText(s.Next(m.now()))
		fm.fields = append(fm.fields,
			noteField(s.Text()+". Next send: "+next+". "+m.usedBy("schedule", id), textStyle),
			noteField("Today · each │ is one send · ┃ is now", faint),
			noteField(todayLine(s, m.now(), max(24, detailWidth(m.w)-4)), textStyle),
			noteField("Outside the schedule, i2a still reads the accounts. The messages wait for the next send.", faint),
		)
	}
	fm.fields = append(fm.fields,
		textField("name", "Name", s.Name, "Work hours", "The name that i2a shows. The ID stays the same after a rename.", required),
		textField("every", "Send every", s.Every.String(), "15m", "Time between two sends, for example 15m, 1h, 2h.", minDuration(time.Minute)),
		textField("days", "Days", strings.Join(s.Days, ","), "every day", "mon,tue,wed,thu,fri,sat,sun. Empty means every day.", validDays),
		textField("hours", "Hours", s.Hours, "all day", "Local time as HH:MM-HH:MM. A range can cross midnight. Empty means all day.",
			func(v string) error { _, err := config.ParseHours(v); return err }),
	)
	label := "Save changes"
	if isNew {
		label = "Create schedule"
	}
	fm.buttons = []button{
		{label: label, primary: true, run: func() tea.Cmd { return m.saveSchedule(id) }},
		{label: "Cancel", run: func() tea.Cmd { m.buildForm(true); return nil }},
	}
	if !isNew {
		fm.buttons = append(fm.buttons, button{label: "Delete schedule", danger: true, run: func() tea.Cmd {
			m.form.deactivate()
			m.deleteWithReplacement("schedule", id)
			return nil
		}})
	}
	return fm
}

// detailWidth is the inner width of the detail panel of a master-detail tab.
func detailWidth(w int) int { dw, _ := detailSize(w, 10, false); return dw }

func (m *model) saveSchedule(id string) tea.Cmd {
	fm := m.form
	if !fm.validate() {
		return nil
	}
	s := config.Schedule{}
	if id == "" {
		s.ID = fm.field("id").value()
	} else if cur, ok := m.u.cfg.Schedule(id); ok {
		s = *cur
	}
	s.Name = fm.field("name").value()
	s.Every, _ = config.ParseDuration(fm.field("every").value())
	s.Days = splitDays(fm.field("days").value())
	s.Hours = fm.field("hours").value()
	if err := m.commitForm(fm, func(c *config.Config) error {
		if id == "" {
			c.Schedules = append(c.Schedules, s)
			return nil
		}
		cur, ok := c.Schedule(id)
		if !ok {
			return config.MissingError{Kind: config.KindSchedule, ID: id}
		}
		*cur = s
		return nil
	}, nil); err != nil {
		saveError(fm, err)
		return nil
	}
	for i, x := range m.u.cfg.Schedules {
		if x.ID == s.ID {
			m.sch = i
		}
	}
	m.saved("Saved the schedule " + s.Title() + ".")
	return nil
}
