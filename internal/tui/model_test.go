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
	"github.com/sondt2709/inbox2agent/internal/source/mattermost"
	"github.com/sondt2709/inbox2agent/internal/store"
)

type fakeStore struct{}

func (fakeStore) Counts(context.Context) (map[string]int, error) {
	return map[string]int{"pending": 4, "sent": 12}, nil
}

func (fakeStore) PendingCounts(context.Context) (map[store.Conv]int, error) {
	return map[store.Conv]int{{Account: "work", ID: "C1"}: 3, {Account: "work", ID: "D1"}: 1}, nil
}

func (fakeStore) Dispatches(context.Context, int) ([]store.Dispatch, error) {
	return []store.Dispatch{{ID: 1, Space: "acme", Agent: "claude-work", Driver: "orca", Status: store.DispatchSent, MessageCount: 3,
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

// newTestModel returns a dashboard of 120 × 40 on the Spaces tab, with one
// Mattermost account that lists 3 conversations, 2 agents, and the space Acme
// with one of them. The other 2 conversations are in Unsorted.
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
	work := config.NewMattermostAccount("work", "https://chat.example.com")
	work.User = "alice"
	cfg.Accounts = append(cfg.Accounts, work)
	cfg.Agents = append(cfg.Agents, config.NewAgent("claude-work"))
	cfg.Spaces = []config.Space{{ID: "acme", Name: "Acme", Agent: "claude-work", Schedule: "always",
		Conversations: []config.Conversation{{Account: "work", ID: "C1", Name: "release", Read: config.ReadAll}}}}
	if err := config.Save(paths.ConfigFile, cfg); err != nil {
		t.Fatal(err)
	}
	u := newUI(Deps{Paths: paths, Config: cfg, Secrets: sec, Store: fakeStore{}, Tab: TabSpaces,
		Check:         func(context.Context, config.Account, string) (string, error) { return "alice", nil },
		ServiceStatus: func() (service.State, error) { return service.State{Loaded: true, State: "running", PID: "42"}, nil }})
	m := newModel(context.Background(), u)
	m.chs["work"] = []mattermost.ChannelInfo{
		{ID: "C1", Kind: inbox.KindChannel, Name: "release", Team: "eng"},
		{ID: "D1", Kind: inbox.KindDirect, Name: "@bob"},
		{ID: "G1", Kind: inbox.KindGroup, Name: "bob, carol"},
	}
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
	case "right":
		return tea.KeyPressMsg{Code: tea.KeyRight}
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

func TestDashboardShowsTheModel(t *testing.T) {
	m := newTestModel(t)
	tabBar := strings.Join(strings.Fields(strings.Split(screen(m), "\n")[1]), " ")
	if want := "1 Feed 2 Activity │ 3 Accounts → 4 Conversations 2 unsorted → 5 Spaces → 6 Agents + 7 Schedules │ 8 Settings"; tabBar != want {
		t.Errorf("tab bar = %q, want %q", tabBar, want)
	}
	if s := screen(m); !strings.Contains(s, "□ Unsorted") || !strings.Contains(s, "Unsorted reads 2 conversations from work → sends to triage") {
		t.Errorf("the Spaces tab does not open on Unsorted:\n%s", s)
	}
	send(m, "down")
	m.Update(tea.WindowSizeMsg{Width: 220, Height: 40}) // the sentence on one line
	s := screen(m)
	for _, want := range []string{"Space: your own group of conversations from any account", "service running · pid 42", "■ Acme",
		"Acme reads 1 conversation from work → sends to claude-work · always (every 15m · every day · all day). Next send: ",
		"with 3 messages now waiting."} {
		if !strings.Contains(s, want) {
			t.Errorf("the dashboard does not show %q:\n%s", want, s)
		}
	}
	if n := strings.Count(s, "\n") + 1; n != 40 {
		t.Errorf("the dashboard has %d lines, want the window height 40", n)
	}
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	send(m, "8", "enter", "ctrl+u", "9")
	lines := strings.Split(screen(m), "\n")
	if !strings.Contains(lines[1], "8 Settings") || !strings.Contains(lines[2], "● unsaved changes") {
		t.Errorf("at 120 columns the tab bar or the unsaved mark does not fit:\n%s\n%s", lines[1], lines[2])
	}
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	if bar := strings.Split(screen(m), "\n")[1]; !strings.Contains(bar, "4 Conversations 2 → 5 Spaces") || !strings.Contains(bar, "8 Settings") {
		t.Errorf("at 100 columns the compact tab bar does not fit: %q", bar)
	}
}

func TestTabKeys(t *testing.T) {
	m := newTestModel(t)
	send(m, "7")
	if m.tab != tabSchedules || !strings.Contains(screen(m), "Schedule: when a space sends") {
		t.Fatalf("key 7: tab %d", m.tab)
	}
	send(m, "tab")
	if m.tab != tabSettings {
		t.Errorf("tab: tab %d, want Settings", m.tab)
	}
	send(m, "tab")
	if m.tab != tabFeed {
		t.Errorf("tab after the last tab: tab %d, want Feed", m.tab)
	}
}

func TestMouseClickOnTab(t *testing.T) {
	m := newTestModel(t)
	// Click where a person sees the tab: the text "6 Agents" on the tab bar (row 1).
	tabBar := strings.Split(screen(m), "\n")[1]
	x := len([]rune(tabBar[:strings.Index(tabBar, "6 Agents")]))
	m.Update(tea.MouseClickMsg{X: x + 2, Y: 1, Button: tea.MouseLeft})
	if m.tab != tabAgents {
		t.Fatalf("click on the Agents tab: tab %d", m.tab)
	}
	// Click the third row of the list ("+ Add agent"), then click it again to edit it.
	m.render()
	m.Update(tea.MouseClickMsg{X: 4, Y: contentTop + 4, Button: tea.MouseLeft})
	if m.agt != 2 || m.form == nil || m.form.active {
		t.Fatalf("first click: row %d", m.agt)
	}
	if !strings.Contains(screen(m), "Create agent") {
		t.Fatal("the detail panel does not show the new agent form")
	}
	m.Update(tea.MouseClickMsg{X: 4, Y: contentTop + 4, Button: tea.MouseLeft})
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
	send(m, "8", "enter")
	if m.form == nil || !m.form.active {
		t.Fatal("enter on the Settings tab did not give the keyboard to the form")
	}
	send(m, "ctrl+u", "2", "d")
	if !m.form.dirty() || !strings.Contains(screen(m), "unsaved changes") {
		t.Fatal("the form does not show the unsaved change")
	}
	if quit := send(m, "ctrl+s"); quit {
		t.Fatal("ctrl+s quit the dashboard")
	}
	if !strings.Contains(fileText(t, m), "lookback = '2d'") {
		t.Fatalf("ctrl+s did not write the file:\n%s", fileText(t, m))
	}
	if m.form.active || m.form.dirty() || !strings.Contains(screen(m), "Saved the settings.") {
		t.Fatalf("after the save: active %v, dirty %v", m.form.active, m.form.dirty())
	}
	if !send(m, "q") {
		t.Error("q did not quit")
	}
}

func TestFieldErrorKeepsTheFile(t *testing.T) {
	m := newTestModel(t)
	before := fileText(t, m)
	send(m, "8", "enter", "ctrl+u", "a", "b", "c", "ctrl+s")
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
	send(m, "8", "enter", "ctrl+u", "4", "5", "m", "esc")
	if d := m.topDialog(); d == nil || d.title() != "Unsaved changes" {
		t.Fatalf("esc with changes: dialog %v, want Unsaved changes", d)
	}
	send(m, "d")
	if m.form.active || m.form.field("lookback").value() != "1d" || fileText(t, m) != before {
		t.Fatalf("after discard: active %v, lookback %q", m.form.active, m.form.field("lookback").value())
	}
	// Without changes, esc goes back to the list at once.
	send(m, "enter", "esc")
	if m.topDialog() != nil || m.form.active {
		t.Error("esc without changes did not go back to the list")
	}
}

func TestQuitWithUnsavedFormAsks(t *testing.T) {
	m := newTestModel(t)
	send(m, "8", "enter", "ctrl+u", "9", "d")
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

func TestConversationsMoveReadLevelAndRename(t *testing.T) {
	m := newTestModel(t)
	send(m, "4")
	s := screen(m)
	for _, want := range []string{"# release", "@ @bob (new)", "⧉ bob, carol (new)", "Unsorted", "Acme"} {
		if !strings.Contains(s, want) {
			t.Fatalf("the table does not show %q:\n%s", want, s)
		}
	}
	// Select the direct conversation and the group, and move both to a new space.
	send(m, "down", "space", "space", "m")
	d, ok := m.topDialog().(*pickDialog)
	if !ok || len(d.options) != 3 {
		t.Fatalf("m did not ask for the space: %v", m.topDialog())
	}
	send(m, "down", "down", "enter")
	for _, k := range "Company B" {
		send(m, string(k))
	}
	send(m, "enter")
	c, err := config.Load(m.u.d.Paths.ConfigFile)
	if err != nil {
		t.Fatal(err)
	}
	if c.Route("work", "D1") != "company-b" || c.Route("work", "G1") != "company-b" || c.Level("work", "D1") != config.ReadAll {
		t.Fatalf("after the move: D1 in %s (%s), G1 in %s:\n%s", c.Route("work", "D1"), c.Level("work", "D1"), c.Route("work", "G1"), fileText(t, m))
	}
	if m.unsortedCount() != 0 || strings.Contains(screen(m), "unsorted ·") && !strings.Contains(screen(m), "0 unsorted") {
		t.Errorf("unsorted count = %d", m.unsortedCount())
	}
	// The read level of the channel, then a new name for it.
	send(m, "esc")
	m.conv = 0
	send(m, "l")
	send(m, "n", "ctrl+u")
	for _, k := range "Release channel" {
		send(m, string(k))
	}
	send(m, "enter")
	c, _ = config.Load(m.u.d.Paths.ConfigFile)
	if cv, _, _ := c.Conversation("work", "C1"); cv.Read != config.ReadMentions || cv.Label != "Release channel" || cv.Name != "release" {
		t.Errorf("C1 = %+v", cv)
	}
	if !strings.Contains(screen(m), "Release channel") {
		t.Error("the table does not show the new name")
	}
}

func TestDeleteAgentAsksForReplacement(t *testing.T) {
	m := newTestModel(t)
	send(m, "6", "down", "x")
	d, ok := m.topDialog().(*pickDialog)
	if !ok || !strings.Contains(d.text, "1 space Acme use the agent claude-work") || len(d.options) != 1 {
		t.Fatalf("x on a used agent: %v", m.topDialog())
	}
	send(m, "enter")
	c, err := config.Load(m.u.d.Paths.ConfigFile)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := c.Agent("claude-work"); ok || c.Spaces[0].Agent != "triage" {
		t.Errorf("after the delete: agents %+v, Acme uses %q", c.Agents, c.Spaces[0].Agent)
	}
}

func TestSpaceRenameAndDelete(t *testing.T) {
	m := newTestModel(t)
	send(m, "down", "enter", "ctrl+u")
	for _, k := range "Acme Corp" {
		send(m, string(k))
	}
	send(m, "ctrl+s")
	if !strings.Contains(fileText(t, m), "name = 'Acme Corp'") || !strings.Contains(fileText(t, m), "id = 'acme'") {
		t.Fatalf("rename:\n%s", fileText(t, m))
	}
	send(m, "x", "y")
	c, _ := config.Load(m.u.d.Paths.ConfigFile)
	if len(c.Spaces) != 0 || c.Route("work", "C1") != config.UnsortedID || len(c.Unsorted.Conversations) != 1 {
		t.Errorf("after the delete: %+v %+v", c.Spaces, c.Unsorted)
	}
}

func TestUnsortedOnHold(t *testing.T) {
	m := newTestModel(t)
	next := clone(m.u.cfg)
	next.Unsorted.Agent = ""
	if err := m.u.commit(next, nil); err != nil {
		t.Fatal(err)
	}
	m.buildForm(true)
	if s := screen(m); !strings.Contains(s, "Unsorted holds 2 conversations from") && !strings.Contains(s, "It does not send to an agent") {
		t.Errorf("the Unsorted panel does not say that it holds:\n%s", s)
	}
}

func TestAccountShowsWhoAndRenames(t *testing.T) {
	m := newTestModel(t)
	m.Update(tea.WindowSizeMsg{Width: 220, Height: 40})
	send(m, "3")
	s := screen(m)
	for _, want := range []string{"Server      https://chat.example.com", "User        @alice", "3 conversations: Unsorted 2 · Acme 1",
		"Read every     1m"} {
		if !strings.Contains(s, want) {
			t.Errorf("the account panel does not show %q:\n%s", want, s)
		}
	}
	send(m, "enter", "ctrl+u")
	for _, k := range "Company A" {
		send(m, string(k))
	}
	send(m, "ctrl+s")
	if !strings.Contains(fileText(t, m), "name = 'Company A'") || !strings.Contains(fileText(t, m), "id = 'work'") {
		t.Fatalf("rename:\n%s", fileText(t, m))
	}
}

func TestScheduleDaysAreChecked(t *testing.T) {
	m := newTestModel(t)
	send(m, "7")
	if s := screen(m); !strings.Contains(s, "every 15m · every day · all day. Next send:") || !strings.Contains(s, "Today · each │ is one send") {
		t.Fatalf("the schedule panel:\n%s", s)
	}
	send(m, "enter")
	m.form.field("days").input.SetValue("monday")
	send(m, "ctrl+s")
	if !strings.Contains(screen(m), `"monday" is not a day`) {
		t.Errorf("no error for a bad day:\n%s", screen(m))
	}
}

func TestClickFormField(t *testing.T) {
	m := newTestModel(t)
	send(m, "8")
	lines := strings.Split(screen(m), "\n")
	row := -1
	for i, l := range lines {
		if strings.Contains(l, "Notifications") && strings.Contains(l, "Off") {
			row = i
		}
	}
	if row < 0 {
		t.Fatalf("no Notifications switch on the screen")
	}
	col := strings.Index(lines[row], "Off")
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
	send(m, "2")
	if s := screen(m); !strings.Contains(s, "Acme") || !strings.Contains(s, "claude-work") || !strings.Contains(s, "4 waiting") {
		t.Errorf("the Activity tab:\n%s", s)
	}
	send(m, "enter")
	if d := m.topDialog(); d == nil || !strings.Contains(screen(m), "line two") {
		t.Fatalf("enter on a send did not show the digest: %v", d)
	}
}
