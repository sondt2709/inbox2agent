package tui

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/sondt2709/inbox2agent/internal/app"
	"github.com/sondt2709/inbox2agent/internal/config"
	"github.com/sondt2709/inbox2agent/internal/inbox"
	"github.com/sondt2709/inbox2agent/internal/secrets"
	"github.com/sondt2709/inbox2agent/internal/service"
	"github.com/sondt2709/inbox2agent/internal/store"
)

type fakeStore struct{}

func (fakeStore) Counts(context.Context) (map[string]int, error) {
	return map[string]int{"pending": 4, "sent": 12}, nil
}

func (fakeStore) Dispatches(context.Context, int) ([]store.Dispatch, error) {
	return []store.Dispatch{{ID: 1, Profile: "triage", Driver: "orca", Status: store.DispatchSent, MessageCount: 3,
		Detail: "new session term_1, turn started", CreatedAt: time.Now()}}, nil
}

func (fakeStore) Cards(context.Context, bool) ([]inbox.Card, error) { return nil, nil }
func (fakeStore) SetCardState(context.Context, int64, string, string, time.Time) error {
	return nil
}
func (fakeStore) MessagesByID(context.Context, []int64) ([]inbox.Message, error) { return nil, nil }
func (fakeStore) FindReply(context.Context, int64, string, string) (inbox.Reply, bool, error) {
	return inbox.Reply{}, false, nil
}
func (fakeStore) StartReply(context.Context, inbox.Reply) (int64, error) { return 0, nil }
func (fakeStore) FinishReply(context.Context, int64, string, string, string, string, time.Time) error {
	return nil
}
func (fakeStore) SentReplies(context.Context, []int64) (map[int64][]inbox.Reply, error) {
	return nil, nil
}
func (fakeStore) Session(context.Context, string) (string, string, error) { return "", "", nil }
func (fakeStore) SetSession(context.Context, string, string, string, time.Time) error {
	return nil
}
func (fakeStore) DeleteSession(context.Context, string) error { return nil }
func (fakeStore) Card(context.Context, int64, string) (inbox.Card, error) {
	return inbox.Card{}, store.ErrNoCard
}
func (fakeStore) StartAsk(context.Context, inbox.Ask) (int64, error) { return 0, nil }
func (fakeStore) FinishAsk(context.Context, int64, string, string, string, time.Time) error {
	return nil
}
func (fakeStore) Asks(context.Context, []int64) (map[int64][]inbox.Ask, error) { return nil, nil }

// newTestModel returns a dashboard of 120 × 40 with one source and its data loaded.
func newTestModel(t *testing.T) *model {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("I2A_HOME", dir)
	t.Setenv("I2A_SECRET_BACKEND", "file")
	paths, err := app.ResolvePaths()
	if err != nil {
		t.Fatal(err)
	}
	sec, err := secrets.Open(paths.SecretsFile)
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.Default()
	cfg.Sources = append(cfg.Sources, config.NewMattermostSource("work", "https://chat.example.com"))
	if err := config.Save(paths.ConfigFile, cfg); err != nil {
		t.Fatal(err)
	}
	u := newUI(Deps{Paths: paths, Config: cfg, Secrets: sec, Store: fakeStore{}, Tab: TabOverview,
		ServiceStatus: func() (service.State, error) { return service.State{Loaded: true, State: "running", PID: "42"}, nil }})
	m := newModel(context.Background(), u)
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m.Update(m.load()())
	return m
}

// screen returns the dashboard text without color codes, as a person reads it.
func screen(m *model) string { return ansi.Strip(m.render()) }

func keyMsg(k string) tea.KeyPressMsg {
	switch k {
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	case "esc":
		return tea.KeyPressMsg{Code: tea.KeyEscape}
	case "tab":
		return tea.KeyPressMsg{Code: tea.KeyTab}
	case "shift+tab":
		return tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift}
	case "down":
		return tea.KeyPressMsg{Code: tea.KeyDown}
	case "ctrl+s":
		return tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl}
	case "ctrl+c":
		return tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}
	case "space":
		return tea.KeyPressMsg{Code: tea.KeySpace, Text: " "}
	case "ctrl+u":
		return tea.KeyPressMsg{Code: 'u', Mod: tea.ModCtrl}
	}
	r := []rune(k)[0]
	return tea.KeyPressMsg{Code: r, Text: k}
}

// send presses the keys and runs the commands that come back, as Bubble Tea
// does, except tea.Quit. It returns true when a command asked to quit.
func send(m *model, keys ...string) (quit bool) {
	for _, k := range keys {
		_, cmd := m.Update(keyMsg(k))
		quit = run(m, cmd, 0) || quit
	}
	return quit
}

func run(m *model, cmd tea.Cmd, depth int) bool {
	if cmd == nil || depth > 5 {
		return false
	}
	msg := runWithTimeout(cmd)
	switch msg := msg.(type) {
	case nil:
		return false
	case tea.QuitMsg:
		return true
	case tea.BatchMsg:
		quit := false
		for _, c := range msg {
			quit = run(m, c, depth+1) || quit
		}
		return quit
	case refreshMsg, tea.WindowSizeMsg:
		return false
	default:
		_, next := m.Update(msg)
		return run(m, next, depth+1)
	}
}

// runWithTimeout runs a command and drops it when it waits (a timer or a cursor blink).
func runWithTimeout(cmd tea.Cmd) tea.Msg {
	ch := make(chan tea.Msg, 1)
	go func() { ch <- cmd() }()
	select {
	case msg := <-ch:
		return msg
	case <-time.After(200 * time.Millisecond):
		return nil
	}
}

func TestDashboardShowsTabsAndOverview(t *testing.T) {
	m := newTestModel(t)
	s := screen(m)
	for _, want := range []string{"1 Feed", "2 Overview", "3 Sources", "6 Activity", "service running · pid 42", "work", "triage",
		"pending", "turn started"} {
		if !strings.Contains(s, want) {
			t.Errorf("the dashboard does not show %q", want)
		}
	}
	if n := strings.Count(s, "\n") + 1; n != 40 {
		t.Errorf("the dashboard has %d lines, want the window height 40", n)
	}
}

func TestTabKeys(t *testing.T) {
	m := newTestModel(t)
	send(m, "5")
	if m.tab != tabSchedule || !strings.Contains(screen(m), "Interval") {
		t.Fatalf("key 5: tab %d", m.tab)
	}
	send(m, "tab")
	if m.tab != tabActivity {
		t.Errorf("tab: tab %d, want Activity", m.tab)
	}
	send(m, "tab")
	if m.tab != tabFeed {
		t.Errorf("tab after the last tab: tab %d, want Feed", m.tab)
	}
}

func TestMouseClickOnTab(t *testing.T) {
	m := newTestModel(t)
	// Click where a person sees the tab: the text "4 Profiles" on the tab bar (row 1).
	tabBar := strings.Split(screen(m), "\n")[1]
	x := len([]rune(tabBar[:strings.Index(tabBar, "4 Profiles")]))
	m.Update(tea.MouseClickMsg{X: x + 2, Y: 1, Button: tea.MouseLeft})
	if m.tab != tabProfiles {
		t.Fatalf("click on the Profiles tab: tab %d", m.tab)
	}
	// Click the second row of the list ("+ Add profile"), then click it again to edit it.
	m.render()
	m.Update(tea.MouseClickMsg{X: 4, Y: contentTop + 3, Button: tea.MouseLeft})
	if m.prof != 1 || m.form == nil || m.form.active {
		t.Fatalf("first click: row %d", m.prof)
	}
	if !strings.Contains(screen(m), "Create profile") {
		t.Fatal("the detail panel does not show the new profile form")
	}
	m.Update(tea.MouseClickMsg{X: 4, Y: contentTop + 3, Button: tea.MouseLeft})
	if !m.form.active {
		t.Fatal("second click did not give the keyboard to the form")
	}
}

func fileText(t *testing.T, m *model) string {
	t.Helper()
	b, err := os.ReadFile(m.u.d.Paths.ConfigFile)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestEditInFormAndSave(t *testing.T) {
	m := newTestModel(t)
	send(m, "5", "enter")
	if m.form == nil || !m.form.active {
		t.Fatal("enter on the Schedule tab did not give the keyboard to the form")
	}
	send(m, "ctrl+u", "1", "0", "m")
	if !m.form.dirty() || !strings.Contains(screen(m), "unsaved changes") {
		t.Fatal("the form does not show the unsaved change")
	}
	if quit := send(m, "ctrl+s"); quit {
		t.Fatal("ctrl+s quit the dashboard")
	}
	if !strings.Contains(fileText(t, m), "interval = '10m'") {
		t.Fatalf("ctrl+s did not write the file:\n%s", fileText(t, m))
	}
	if m.form.active || m.form.dirty() || !strings.Contains(screen(m), "Saved the schedule.") {
		t.Fatalf("after the save: active %v, dirty %v", m.form.active, m.form.dirty())
	}
	if !send(m, "q") {
		t.Error("q did not quit")
	}
}

func TestFieldErrorKeepsTheFile(t *testing.T) {
	m := newTestModel(t)
	before := fileText(t, m)
	send(m, "5", "enter", "ctrl+u", "a", "b", "c", "ctrl+s")
	s := screen(m)
	if !strings.Contains(s, "is not a duration") || !strings.Contains(s, "Correct the fields in red.") {
		t.Fatalf("no error under the field:\n%s", s)
	}
	if fileText(t, m) != before {
		t.Error("an invalid form changed the file")
	}
}

func TestEscWithChangesAsksToDiscard(t *testing.T) {
	m := newTestModel(t)
	before := fileText(t, m)
	send(m, "5", "enter", "ctrl+u", "4", "5", "m", "esc")
	if d := m.topDialog(); d == nil || d.title() != "Unsaved changes" {
		t.Fatalf("esc with changes: dialog %v, want Unsaved changes", d)
	}
	send(m, "d")
	if m.form.active || m.form.field("interval").value() != "15m" || fileText(t, m) != before {
		t.Fatalf("after discard: active %v, interval %q", m.form.active, m.form.field("interval").value())
	}
	// Without changes, esc goes back to the list at once.
	send(m, "enter", "esc")
	if m.topDialog() != nil || m.form.active {
		t.Error("esc without changes did not go back to the list")
	}
}

func TestQuitWithUnsavedFormAsks(t *testing.T) {
	m := newTestModel(t)
	send(m, "5", "enter", "ctrl+u", "9", "m")
	if send(m, "ctrl+c") {
		t.Fatal("ctrl+c quit with an unsaved form")
	}
	if d := m.topDialog(); d == nil || d.title() != "Unsaved changes" {
		t.Fatalf("dialog = %v, want Unsaved changes", d)
	}
	if !send(m, "d") {
		t.Fatal("d did not discard and quit")
	}
}

func TestSourceFormSwitchAndQuickKeys(t *testing.T) {
	m := newTestModel(t)
	// The form: tab to "All direct", switch it with space, save.
	send(m, "3", "enter", "tab", "tab", "tab", "space", "ctrl+s")
	if !strings.Contains(fileText(t, m), "all_direct = false") {
		t.Fatalf("the switch in the form was not saved:\n%s", fileText(t, m))
	}
	// The quick key in the list saves at once.
	send(m, "m")
	if !strings.Contains(fileText(t, m), "mentions = false") {
		t.Fatalf("m did not save the mentions switch:\n%s", fileText(t, m))
	}
	send(m, "x", "y")
	if strings.Contains(fileText(t, m), "id = 'work'") {
		t.Fatal("x then y did not remove the source from the file")
	}
}

func TestClickFormField(t *testing.T) {
	m := newTestModel(t)
	send(m, "5")
	lines := strings.Split(screen(m), "\n")
	row := -1
	for i, l := range lines {
		if strings.Contains(l, "Notifications") {
			row = i
		}
	}
	col := strings.Index(lines[row], "Off")
	if row < 0 || col < 0 {
		t.Fatalf("no Notifications switch on the screen")
	}
	x := len([]rune(lines[row][:col]))
	m.Update(tea.MouseClickMsg{X: x, Y: row, Button: tea.MouseLeft})
	if !m.form.active || m.form.field("notify").value() != "off" {
		t.Fatalf("a click on Off: active %v, notify %q", m.form.active, m.form.field("notify").value())
	}
}

func TestDigestViewer(t *testing.T) {
	m := newTestModel(t)
	path := filepath.Join(t.TempDir(), "digest.md")
	if err := os.WriteFile(path, []byte("# i2a batch\nline two"), 0o600); err != nil {
		t.Fatal(err)
	}
	m.dispatches[0].DigestPath = path
	send(m, "6", "enter")
	if d := m.topDialog(); d == nil || !strings.Contains(screen(m), "line two") {
		t.Fatalf("enter on a dispatch did not show the digest: %v", d)
	}
}
