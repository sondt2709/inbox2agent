package agent

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/sondt2709/inbox2agent/internal/config"
	"github.com/sondt2709/inbox2agent/internal/inbox"
	"github.com/sondt2709/inbox2agent/internal/render"
)

// The testdata files are real orca 1.4.240 outputs with local paths replaced.

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// fakeOrca answers each orca subcommand with a queue of fixture files.
type fakeOrca struct {
	t       *testing.T
	answers map[string][]string // "agent launch" -> fixture names, used in order
	calls   [][]string
}

func (f *fakeOrca) run(_ context.Context, bin string, args ...string) ([]byte, error) {
	f.calls = append(f.calls, args)
	key := strings.Join(args[:2], " ")
	q := f.answers[key]
	if len(q) == 0 {
		f.t.Fatalf("unexpected orca call: %s %v", bin, args)
	}
	f.answers[key] = q[1:]
	if q[0] == "exec-error" {
		return nil, errors.New("run orca: no such file or directory")
	}
	return fixture(f.t, q[0]), nil
}

func (f *fakeOrca) called(sub string) [][]string {
	var out [][]string
	for _, c := range f.calls {
		if strings.Join(c[:2], " ") == sub {
			out = append(out, c)
		}
	}
	return out
}

func flag(args []string, name string) string {
	for i, a := range args {
		if a == name && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}

// memSessions keeps the handle of a space under its ID and the Claude
// session under "<id>#agent".
type memSessions map[string]string

func (m memSessions) Session(_ context.Context, p string) (inbox.Session, error) {
	return inbox.Session{Space: p, Handle: m[p], AgentSession: m[p+"#agent"]}, nil
}
func (m memSessions) SetSession(_ context.Context, s inbox.Session) error {
	m[s.Space], m[s.Space+"#agent"] = s.Handle, s.AgentSession
	return nil
}
func (m memSessions) DeleteSession(_ context.Context, p string) error {
	delete(m, p)
	delete(m, p+"#agent")
	return nil
}

var (
	spaceTriage   = config.Space{ID: "triage", Name: "Triage"}
	spaceBorealis = config.Space{ID: "borealis", Name: "Project Borealis"}
)

func setup(t *testing.T, session string, answers map[string][]string) (*Orca, *fakeOrca, memSessions, *render.Rendered) {
	t.Helper()
	p := config.NewAgent("triage")
	p.Driver = config.DriverOrca
	p.Session = session
	p.Workspace = "/home/user/notes"
	sess := memSessions{}
	o := NewOrca(p, spaceTriage, sess)
	o.Claude = nil // the Orca check; a test sets a fixture finder
	o.Sleep = func(time.Duration) {}
	f := &fakeOrca{t: t, answers: answers}
	o.Run = f.run
	msg := inbox.Message{SourceID: "work", SourceType: "mattermost", ExternalID: "p1", Author: "bob", Text: "hi", CreatedAt: time.Now()}
	r, err := render.Render(p, "Triage", "20260102-030405-triage-1", []inbox.Message{msg}, t.TempDir(), "/usr/local/bin/i2a")
	if err != nil {
		t.Fatal(err)
	}
	return o, f, sess, r
}

const (
	launchedHandle = "term_f8323417-9685-4934-a061-20c3fc4ccbff" // the handle in launch_no_prompt.json
	worktreeHandle = "term_a064be47-dfe2-4514-bc43-db02bbe10606" // the handle in worktree_create_no_prompt.json
)

// started are the answers for a new agent: launch, the agent runs (Orca
// reports agentIdentity), ready, prompt submitted. The "terminal show" answers
// of extra come first; the show of the new terminal follows them.
func started(extra map[string][]string) map[string][]string {
	a := map[string][]string{
		"agent launch": {"launch_no_prompt.json"}, "terminal rename": {"rename_ok.json"},
		"terminal wait": {"wait_after_launch.json"}, "terminal send": {"send_ok.json"},
	}
	for k, v := range extra {
		a[k] = v
	}
	a["terminal show"] = append(slices.Clone(extra["terminal show"]), "show_claude_live.json")
	return a
}

func TestNewModeLaunchesThenSubmitsTheBatch(t *testing.T) {
	o, f, sess, r := setup(t, config.SessionNew, started(nil))
	res, err := o.Deliver(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != StatusSent || !strings.Contains(res.Detail, launchedHandle) || !strings.Contains(res.Detail, "turn started") {
		t.Errorf("result = %+v", res)
	}
	launch := f.called("agent launch")[0]
	if flag(launch, "--surface") != "terminal" || flag(launch, "--worktree") != "path:/home/user/notes" || flag(launch, "--agent") != "claude" {
		t.Errorf("launch args = %v", launch)
	}
	if slices.Contains(launch, "--prompt") {
		t.Errorf("launch has --prompt; Orca can leave it in the input box: %v", launch)
	}
	if launch[len(launch)-1] != "--json" {
		t.Errorf("last arg = %q, want --json", launch[len(launch)-1])
	}
	wait := f.called("terminal wait")[0]
	if flag(wait, "--terminal") != launchedHandle || flag(wait, "--for") != "tui-idle" || flag(wait, "--timeout-ms") != "90000" {
		t.Errorf("wait args = %v", wait)
	}
	send := f.called("terminal send")[0]
	prompt := flag(send, "--text")
	if flag(send, "--terminal") != launchedHandle || !slices.Contains(send, "--enter") || flag(send, "--wait-submit") != "30" {
		t.Errorf("send args = %v", send)
	}
	if prompt != r.Prompt {
		t.Errorf("prompt = %q", prompt)
	}
	if sess["triage"] != "" {
		t.Errorf("new mode stored a session: %v", sess)
	}
}

func TestNewAgentNotReady(t *testing.T) {
	o, f, _, r := setup(t, config.SessionNew, started(map[string][]string{"terminal wait": {"wait_timeout.json"}}))
	_, err := o.Deliver(context.Background(), r)
	if err == nil || !strings.Contains(err.Error(), "was not ready after 1m30s") {
		t.Fatalf("err = %v", err)
	}
	if len(f.called("terminal send")) != 0 {
		t.Error("the prompt was sent to an agent that was not ready")
	}
}

func TestReuseWithoutSessionStartsOne(t *testing.T) {
	o, f, sess, r := setup(t, config.SessionReuse, started(nil))
	res, err := o.Deliver(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	if sess["triage"] != launchedHandle || res.Status != StatusSent || !strings.HasPrefix(res.Detail, "started session "+launchedHandle+", turn started") {
		t.Errorf("session=%v result=%+v", sess, res)
	}
	if prompt := flag(f.called("terminal send")[0], "--text"); prompt != r.Prompt {
		t.Errorf("the first prompt is not the batch: %q", prompt)
	}
}

func TestReuseDoesNotStoreSessionWhenSubmitFails(t *testing.T) {
	o, _, sess, r := setup(t, config.SessionReuse, started(map[string][]string{"terminal wait": {"wait_timeout.json"}}))
	if _, err := o.Deliver(context.Background(), r); err == nil {
		t.Fatal("no error")
	}
	if sess["triage"] != "" {
		t.Errorf("stored a session that did not get the prompt: %v", sess)
	}
}

func TestReuseLiveIdleSendsBatchOnly(t *testing.T) {
	o, f, sess, r := setup(t, config.SessionReuse, map[string][]string{
		"terminal show": {"show_claude_live.json"}, "terminal wait": {"wait_idle.json"}, "terminal send": {"send_ok.json"},
	})
	sess["triage"] = "term_live"
	res, err := o.Deliver(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != StatusSent || !strings.Contains(res.Detail, "turn started") {
		t.Errorf("result = %+v", res)
	}
	send := f.called("terminal send")[0]
	if flag(send, "--terminal") != "term_live" || flag(send, "--text") != r.Prompt || !slices.Contains(send, "--enter") {
		t.Errorf("send args = %v", send)
	}
	wait := f.called("terminal wait")[0]
	if flag(wait, "--for") != "tui-idle" || flag(wait, "--timeout-ms") != "5000" {
		t.Errorf("wait args = %v", wait)
	}
}

func TestReuseBackgroundAgentTerminalIsLive(t *testing.T) {
	// A real agent terminal with no visible pane: orphaned is true, but it runs.
	o, f, sess, r := setup(t, config.SessionReuse, map[string][]string{
		"terminal show": {"show_agent_background.json"}, "terminal wait": {"wait_idle.json"}, "terminal send": {"send_ok.json"},
	})
	sess["triage"] = "term_bg"
	if _, err := o.Deliver(context.Background(), r); err != nil {
		t.Fatal(err)
	}
	if len(f.called("agent launch")) != 0 || len(f.called("terminal send")) != 1 {
		t.Errorf("calls = %v, want one send and no launch", f.calls)
	}
}

func TestReuseBusyIsHeld(t *testing.T) {
	o, f, sess, r := setup(t, config.SessionReuse, map[string][]string{
		"terminal show": {"show_claude_live.json"}, "terminal wait": {"wait_timeout.json"},
	})
	sess["triage"] = "term_live"
	res, err := o.Deliver(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != StatusHeld {
		t.Errorf("result = %+v, want held", res)
	}
	if len(f.called("terminal send")) != 0 {
		t.Error("terminal send was called for a busy agent")
	}
}

func TestReuseClosedOrStaleTerminalStartsNewSession(t *testing.T) {
	for _, show := range []string{"show_closed.json", "show_stale.json"} {
		o, _, sess, r := setup(t, config.SessionReuse, started(map[string][]string{"terminal show": {show}}))
		sess["triage"] = "term_old"
		res, err := o.Deliver(context.Background(), r)
		if err != nil {
			t.Fatalf("%s: %v", show, err)
		}
		if sess["triage"] != launchedHandle || !strings.Contains(res.Detail, "started session") {
			t.Errorf("%s: session=%v result=%+v", show, sess, res)
		}
	}
}

// The prompt can stay in the input box of the agent. When Orca cannot prove
// that the turn started, the driver presses Enter one more time and does not
// type the text again.
func TestUnprovenTurnSendsOneBareEnter(t *testing.T) {
	o, f, _, r := setup(t, config.SessionNew, started(map[string][]string{"terminal send": {"send_unproven.json", "send_enter.json"}}))
	res, err := o.Deliver(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != StatusSent || !strings.Contains(res.Detail, "turn not proven, Enter sent again") {
		t.Errorf("result = %+v", res)
	}
	sends := f.called("terminal send")
	if len(sends) != 2 {
		t.Fatalf("sends = %v, want 2", sends)
	}
	if slices.Contains(sends[1], "--text") || !slices.Contains(sends[1], "--enter") || flag(sends[1], "--terminal") != launchedHandle {
		t.Errorf("second send = %v, want a bare Enter", sends[1])
	}
}

func TestWorktreeModeCreatesThenSubmits(t *testing.T) {
	o, f, _, r := setup(t, config.SessionWorktree, map[string][]string{
		"worktree create": {"worktree_create_no_prompt.json"}, "terminal rename": {"rename_ok.json"}, "terminal show": {"show_claude_live.json"},
		"terminal wait": {"wait_after_launch.json"}, "terminal send": {"send_ok.json"},
	})
	res, err := o.Deliver(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != StatusSent || !strings.Contains(res.Detail, worktreeHandle) || !strings.Contains(res.Detail, "turn started") {
		t.Errorf("result = %+v", res)
	}
	create := f.called("worktree create")[0]
	if flag(create, "--name") != "i2a-"+r.BatchID || flag(create, "--repo") != "path:/home/user/notes" || slices.Contains(create, "--prompt") {
		t.Errorf("create args = %v", create)
	}
	if send := f.called("terminal send")[0]; flag(send, "--terminal") != worktreeHandle {
		t.Errorf("send args = %v", send)
	}
	if rn := f.called("terminal rename"); len(rn) != 1 || flag(rn[0], "--terminal") != worktreeHandle || flag(rn[0], "--title") != "triage · Triage" {
		t.Errorf("rename = %v, want the tab name of the worktree terminal", rn)
	}
}

func TestUnknownWorkspaceIsAddedOnce(t *testing.T) {
	o, f, _, r := setup(t, config.SessionNew, started(map[string][]string{
		"agent launch": {"launch_selector_not_found.json", "launch_no_prompt.json"}, "repo add": {"repo_add_ok.json"},
	}))
	if _, err := o.Deliver(context.Background(), r); err != nil {
		t.Fatal(err)
	}
	add := f.called("repo add")
	if len(add) != 1 || flag(add[0], "--path") != "/home/user/notes" {
		t.Errorf("repo add calls = %v", add)
	}
}

func TestUnknownFolderThatIsNotGit(t *testing.T) {
	o, _, _, r := setup(t, config.SessionNew, map[string][]string{
		"agent launch": {"launch_selector_not_found.json"}, "repo add": {"repo_add_not_git.json"},
	})
	_, err := o.Deliver(context.Background(), r)
	if err == nil || !strings.Contains(err.Error(), "Add the folder in Orca") || !strings.Contains(err.Error(), "Not a valid git repository") {
		t.Fatalf("err = %v", err)
	}
}

func TestOrcaNotReachable(t *testing.T) {
	o, _, _, r := setup(t, config.SessionNew, map[string][]string{"agent launch": {"exec-error"}})
	if _, err := o.Deliver(context.Background(), r); err == nil || !strings.Contains(err.Error(), "no such file") {
		t.Fatalf("err = %v", err)
	}
}

func TestReuseKeepsOneSessionForEachSpace(t *testing.T) {
	o, _, sess, r := setup(t, config.SessionReuse, started(nil))
	if _, err := o.Deliver(context.Background(), r); err != nil {
		t.Fatal(err)
	}
	other := NewOrca(o.p, spaceBorealis, sess)
	other.Claude = nil
	f := &fakeOrca{t: t, answers: started(nil)}
	other.Run = f.run
	res, err := other.Deliver(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	if len(f.called("agent launch")) != 1 || sess["borealis"] != launchedHandle || !strings.Contains(res.Detail, "started session") {
		t.Errorf("the second space did not start its own session: calls %v, sessions %v", f.calls, sess)
	}
}

// askFor renders an ask about one card for the profile of the setup.
func askFor(t *testing.T, o *Orca) *render.Rendered {
	t.Helper()
	c := inbox.Card{ID: 4, Key: "mm:lena-pf-login", Category: inbox.CategoryAction, Urgency: inbox.UrgencyHigh, Title: "Lena"}
	a, err := render.Ask(o.p, "Triage", "20260102-030405-triage-ask7", render.AskInput{ID: 7, Card: c, Text: "Research more."}, "/usr/local/bin/i2a")
	if err != nil {
		t.Fatal(err)
	}
	return a
}

const cardHandle = "term_b9f22da8-13d6-49bc-a1ab-53eba56c9578"

func TestAskLiveCardTerminalGetsPromptOnly(t *testing.T) {
	o, f, sess, _ := setup(t, config.SessionReuse, map[string][]string{
		"terminal show": {"show_claude_live.json"}, "terminal wait": {"wait_idle.json"}, "terminal send": {"send_ok.json"},
	})
	sess["triage"] = "term_profile"
	a := askFor(t, o)
	res, err := o.Ask(context.Background(), Terminal{Handle: cardHandle}, 2*time.Minute, a)
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != StatusSent || res.Handle != cardHandle {
		t.Errorf("result = %+v", res)
	}
	if show := f.called("terminal show")[0]; flag(show, "--terminal") != cardHandle {
		t.Errorf("show args = %v", show)
	}
	if wait := f.called("terminal wait")[0]; flag(wait, "--terminal") != cardHandle || flag(wait, "--timeout-ms") != "120000" {
		t.Errorf("wait args = %v", wait)
	}
	send := f.called("terminal send")[0]
	if flag(send, "--terminal") != cardHandle || flag(send, "--text") != a.Prompt {
		t.Errorf("send args = %v", send)
	}
}

func TestAskBusyCardTerminalIsHeld(t *testing.T) {
	o, f, _, _ := setup(t, config.SessionReuse, map[string][]string{
		"terminal show": {"show_claude_live.json"}, "terminal wait": {"wait_timeout.json"},
	})
	res, err := o.Ask(context.Background(), Terminal{Handle: cardHandle}, 2*time.Minute, askFor(t, o))
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != StatusHeld || res.Detail != "agent busy in "+cardHandle {
		t.Errorf("result = %+v", res)
	}
	if len(f.called("terminal send")) != 0 {
		t.Error("the ask was sent to a busy agent")
	}
}

func TestAskClosedCardTerminalGoesToProfileSession(t *testing.T) {
	o, f, sess, _ := setup(t, config.SessionReuse, map[string][]string{
		"terminal show": {"show_closed.json", "show_claude_live.json"}, "terminal wait": {"wait_idle.json"}, "terminal send": {"send_ok.json"},
	})
	sess["triage"] = "term_profile"
	a := askFor(t, o)
	res, err := o.Ask(context.Background(), Terminal{Handle: "term_old"}, 2*time.Minute, a)
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != StatusSent || res.Handle != "term_profile" {
		t.Errorf("result = %+v", res)
	}
	if wait := f.called("terminal wait")[0]; flag(wait, "--terminal") != "term_profile" || flag(wait, "--timeout-ms") != "120000" {
		t.Errorf("wait args = %v", wait)
	}
	if send := f.called("terminal send")[0]; flag(send, "--text") != a.Prompt {
		t.Errorf("the space session did not get the ask: %v", send)
	}
	if o.IdleTimeout != 5*time.Second {
		t.Errorf("idle timeout = %s after the ask, want 5s for the next batch", o.IdleTimeout)
	}
}

func TestAskWithoutSessionStartsOne(t *testing.T) {
	o, f, sess, _ := setup(t, config.SessionReuse, started(nil))
	a := askFor(t, o)
	res, err := o.Ask(context.Background(), Terminal{}, 2*time.Minute, a)
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != StatusSent || res.Handle != launchedHandle || sess["triage"] != launchedHandle {
		t.Errorf("result = %+v, sessions = %v", res, sess)
	}
	prompt := flag(f.called("terminal send")[0], "--text")
	if prompt != a.Prompt {
		t.Errorf("prompt = %q", prompt)
	}
}

func TestDryRunRefusesAsk(t *testing.T) {
	if _, err := (DryRun{}).Ask(context.Background(), Terminal{}, time.Minute, &render.Rendered{}); !errors.Is(err, ErrDryRunAsk) {
		t.Errorf("err = %v", err)
	}
}

// After /exit, the terminal runs zsh: connected, writable, no exit cause,
// and agentIdentity null (show_agent_exited.json). The driver must not type
// the batch into the shell.
func TestReuseAgentExited(t *testing.T) {
	o, f, sess, r := setup(t, config.SessionReuse, started(map[string][]string{
		"terminal show": {"show_agent_exited.json"},
		"terminal wait": {"wait_idle.json", "wait_after_launch.json"},
	}))
	sess["triage"] = "term_old"
	if _, err := o.Deliver(context.Background(), r); err != nil {
		t.Fatal(err)
	}
	for _, s := range f.called("terminal send") {
		if flag(s, "--terminal") == "term_old" {
			t.Fatalf("the batch went to the shell of term_old: %v", s)
		}
	}
	if sess["triage"] != launchedHandle {
		t.Errorf("session = %v, want a new session %s", sess, launchedHandle)
	}
}

// finder gives the driver the fixture Claude processes: pid -> terminal.
func finder(t *testing.T, handles map[int]string) *ClaudeFinder {
	t.Helper()
	return &ClaudeFinder{Dir: copyDir(t, "4242.json", "4343.json"), Proc: fakeProcs(handles)}
}

func sentTo(f *fakeOrca, handle string) []string {
	var out []string
	for _, s := range f.called("terminal send") {
		if flag(s, "--terminal") == handle {
			out = append(out, flag(s, "--text"))
		}
	}
	return out
}

func TestSendIntoShellIsAnError(t *testing.T) {
	o, f, sess, r := setup(t, config.SessionReuse, map[string][]string{
		"terminal show": {"show_claude_live.json"}, "terminal wait": {"wait_idle.json"}, "terminal send": {"send_shell.json"},
	})
	sess["triage"] = "term_live"
	_, err := o.Deliver(context.Background(), r)
	if err == nil || !strings.Contains(err.Error(), "no agent runs in the terminal term_live") {
		t.Fatalf("err = %v", err)
	}
	if n := len(f.called("terminal send")); n != 1 {
		t.Errorf("%d sends, want 1: no Enter after a send into a shell", n)
	}
	if sess["triage"] != "" {
		t.Errorf("the session of the space stayed: %v", sess)
	}
}

func TestReuseSameClaudeSessionSendsBatch(t *testing.T) {
	o, f, sess, r := setup(t, config.SessionReuse, map[string][]string{
		"terminal show": {"show_agent_exited.json"}, "terminal wait": {"wait_idle.json"}, "terminal send": {"send_ok.json"},
	})
	// Orca still shows no agent (agentIdentity null), but the Claude
	// process proves the session; the finder wins over the Orca field.
	o.Claude = finder(t, map[int]string{4242: "term_a"})
	sess["triage"], sess["triage#agent"] = "term_a", sessionA
	if _, err := o.Deliver(context.Background(), r); err != nil {
		t.Fatal(err)
	}
	if got := sentTo(f, "term_a"); len(got) != 1 || got[0] != r.Prompt {
		t.Errorf("sent to term_a = %q, want the batch only", got)
	}
}

func TestReuseOtherClaudeSessionStartsNewSession(t *testing.T) {
	// The user ran /clear or started claude again: term_a runs session B now.
	o, f, sess, r := setup(t, config.SessionReuse, started(nil))
	o.Claude = finder(t, map[int]string{4343: "term_a", 4242: launchedHandle})
	sess["triage"], sess["triage#agent"] = "term_a", sessionA+"x"
	if _, err := o.Deliver(context.Background(), r); err != nil {
		t.Fatal(err)
	}
	if got := sentTo(f, "term_a"); len(got) != 0 {
		t.Errorf("sent to term_a: %q", got)
	}
	if sess["triage"] != launchedHandle || sess["triage#agent"] != sessionA {
		t.Errorf("session = %v, want %s with Claude session %s", sess, launchedHandle, sessionA)
	}
}

func TestReuseFollowsTheSessionToAnotherTerminal(t *testing.T) {
	o, f, sess, r := setup(t, config.SessionReuse, map[string][]string{
		"terminal show": {"show_claude_live.json"}, "terminal wait": {"wait_idle.json"}, "terminal send": {"send_ok.json"},
	})
	o.Claude = finder(t, map[int]string{4242: "term_b"})
	sess["triage"], sess["triage#agent"] = "term_old", sessionA
	if _, err := o.Deliver(context.Background(), r); err != nil {
		t.Fatal(err)
	}
	if got := sentTo(f, "term_b"); len(got) != 1 {
		t.Errorf("sends to term_b = %q, want 1", got)
	}
	if flag(f.called("terminal show")[0], "--terminal") != "term_b" || sess["triage"] != "term_b" {
		t.Errorf("show = %v, session = %v; want term_b", f.called("terminal show"), sess)
	}
}

func TestReuseSessionInTwoTerminalsStartsNewSession(t *testing.T) {
	o, f, sess, r := setup(t, config.SessionReuse, started(nil))
	dir := copyDir(t, "4242.json")
	b, _ := os.ReadFile(filepath.Join(dir, "4242.json"))
	if err := os.WriteFile(filepath.Join(dir, "4343.json"), []byte(strings.ReplaceAll(string(b), `"pid":4242`, `"pid":4343`)), 0o600); err != nil {
		t.Fatal(err)
	}
	// 4545 is the Claude process of the new terminal, with another session.
	other := strings.ReplaceAll(strings.ReplaceAll(string(b), `"pid":4242`, `"pid":4545`), sessionA, sessionB)
	if err := os.WriteFile(filepath.Join(dir, "4545.json"), []byte(other), 0o600); err != nil {
		t.Fatal(err)
	}
	o.Claude = &ClaudeFinder{Dir: dir, Proc: func(pid int) (ProcInfo, error) {
		return ProcInfo{Start: fixtureStart[4242], Handle: map[int]string{4242: "term_b", 4343: "term_c", 4545: launchedHandle}[pid]}, nil
	}}
	sess["triage"], sess["triage#agent"] = "term_old", sessionA
	if _, err := o.Deliver(context.Background(), r); err != nil {
		t.Fatal(err)
	}
	if len(sentTo(f, "term_b"))+len(sentTo(f, "term_c")) != 0 || sess["triage"] != launchedHandle {
		t.Errorf("calls = %v, session = %v; want a new session", f.calls, sess)
	}
}

func TestReuseAdoptsClaudeSessionOfOldRow(t *testing.T) {
	o, f, sess, r := setup(t, config.SessionReuse, map[string][]string{
		"terminal show": {"show_claude_live.json"}, "terminal wait": {"wait_idle.json"}, "terminal send": {"send_ok.json"},
	})
	o.Claude = finder(t, map[int]string{4242: "term_a"})
	sess["triage"] = "term_a" // a row of schema version 7: no Claude session
	if _, err := o.Deliver(context.Background(), r); err != nil {
		t.Fatal(err)
	}
	if len(sentTo(f, "term_a")) != 1 || sess["triage#agent"] != sessionA {
		t.Errorf("sends = %v, session = %v; want the batch in term_a and %s stored", f.calls, sess, sessionA)
	}
}

func TestReuseUnknownClaudeFilesUsesOrcaCheck(t *testing.T) {
	for show, wantNew := range map[string]bool{"show_claude_live.json": false, "show_agent_exited.json": true} {
		o, f, sess, r := setup(t, config.SessionReuse, started(map[string][]string{
			"terminal show": {show}, "terminal wait": {"wait_idle.json", "wait_after_launch.json"},
		}))
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "1.json"), []byte(`{"process": 1}`), 0o600); err != nil {
			t.Fatal(err)
		}
		o.Claude = &ClaudeFinder{Dir: dir, Proc: fakeProcs(nil)}
		sess["triage"], sess["triage#agent"] = "term_a", sessionA
		res, err := o.Deliver(context.Background(), r)
		if err != nil {
			t.Fatalf("%s: %v", show, err)
		}
		if !wantNew && !strings.Contains(res.Detail, "cannot read the Claude sessions, Orca check only") {
			t.Errorf("%s: detail = %q, want the note about the Orca check", show, res.Detail)
		}
		if got := sess["triage"] == launchedHandle; got != wantNew || (len(sentTo(f, "term_a")) == 0) != wantNew {
			t.Errorf("%s: session = %v, sends = %v; want new session %v", show, sess, f.called("terminal send"), wantNew)
		}
	}
}

func TestReuseLinkedSessionGetsTheBatch(t *testing.T) {
	o, f, sess, r := setup(t, config.SessionReuse, map[string][]string{
		"terminal show": {"show_claude_live.json"}, "terminal wait": {"wait_idle.json"}, "terminal send": {"send_ok.json"},
	})
	o.Claude = finder(t, map[int]string{4242: "term_c"})
	sess["triage"], sess["triage#agent"] = "term_c", sessionA
	res, err := o.Deliver(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	got := sentTo(f, "term_c")
	if len(got) != 1 || got[0] != r.Prompt {
		t.Errorf("sent to term_c = %q, want the batch", got)
	}
	if len(f.called("agent launch")) != 0 || !strings.Contains(res.Detail, "sent to term_c") {
		t.Errorf("session = %v, result = %+v", sess, res)
	}
}

func TestNewSessionGetsTabName(t *testing.T) {
	o, f, _, r := setup(t, config.SessionReuse, started(nil))
	res, err := o.Deliver(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	rn := f.called("terminal rename")
	if len(rn) != 1 || flag(rn[0], "--terminal") != launchedHandle || flag(rn[0], "--title") != "triage · Triage" {
		t.Errorf("rename = %v", rn)
	}
	if strings.Contains(res.Detail, "tab name") {
		t.Errorf("detail = %q", res.Detail)
	}
}

func TestFailedTabNameKeepsTheBatch(t *testing.T) {
	o, _, _, r := setup(t, config.SessionNew, started(map[string][]string{"terminal rename": {"show_stale.json"}}))
	res, err := o.Deliver(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != StatusSent || !strings.Contains(res.Detail, "tab name not set: orca terminal rename: terminal_handle_stale") {
		t.Errorf("result = %+v", res)
	}
}

func TestAskCardTerminalRunsAnotherSession(t *testing.T) {
	o, f, sess, _ := setup(t, config.SessionReuse, map[string][]string{
		"terminal show": {"show_claude_live.json"}, "terminal wait": {"wait_idle.json"}, "terminal send": {"send_ok.json"},
	})
	// term_a wrote the card in session B (4343) and runs session A (4242) now;
	// the space has the live session term_b.
	o.Claude = finder(t, map[int]string{4242: "term_a", 4343: "term_b"})
	sess["triage"], sess["triage#agent"] = "term_b", sessionB
	if _, err := o.Ask(context.Background(), Terminal{Handle: "term_a", AgentSession: sessionB + "x"}, time.Minute, askFor(t, o)); err != nil {
		t.Fatal(err)
	}
	if len(sentTo(f, "term_a")) != 0 || len(sentTo(f, "term_b")) != 1 {
		t.Errorf("sends = %v, want the ask in term_b only", f.called("terminal send"))
	}
}

// goneAfterWait makes the Claude process of term_a exit while the driver
// waits for tui-idle: the queued /exit ran at the end of the turn.
func goneAfterWait(t *testing.T, o *Orca, f *fakeOrca) {
	t.Helper()
	gone := false
	// 4242 runs session A in term_a until the wait; then the new terminal runs 4343.
	o.Claude = &ClaudeFinder{Dir: copyDir(t, "4242.json", "4343.json"), Proc: func(pid int) (ProcInfo, error) {
		switch {
		case pid == 4242 && !gone:
			return ProcInfo{Start: fixtureStart[pid], Handle: "term_a"}, nil
		case pid == 4343 && gone:
			return ProcInfo{Start: fixtureStart[pid], Handle: launchedHandle}, nil
		}
		return ProcInfo{}, ErrNoProcess
	}}
	run := o.Run
	o.Run = func(ctx context.Context, bin string, args ...string) ([]byte, error) {
		out, err := run(ctx, bin, args...)
		if args[0] == "terminal" && args[1] == "wait" && flag(args, "--terminal") == "term_a" {
			gone = true
		}
		return out, err
	}
}

func TestReuseSessionEndsDuringWait(t *testing.T) {
	o, f, sess, r := setup(t, config.SessionReuse, started(map[string][]string{
		"terminal show": {"show_claude_live.json"}, "terminal wait": {"wait_idle.json", "wait_after_launch.json"},
	}))
	goneAfterWait(t, o, f)
	sess["triage"], sess["triage#agent"] = "term_a", sessionA
	res, err := o.Deliver(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	if got := sentTo(f, "term_a"); len(got) != 0 {
		t.Errorf("sent to term_a after the session left it: %q", got)
	}
	if sess["triage"] != launchedHandle || !strings.Contains(res.Detail, "Claude session ended, started session") {
		t.Errorf("session = %v, result = %+v; want a new session", sess, res)
	}
}

func TestAskSessionEndsDuringWait(t *testing.T) {
	o, f, _, _ := setup(t, config.SessionReuse, started(map[string][]string{
		"terminal show": {"show_claude_live.json"}, "terminal wait": {"wait_idle.json", "wait_after_launch.json"},
	}))
	goneAfterWait(t, o, f)
	if _, err := o.Ask(context.Background(), Terminal{Handle: "term_a", AgentSession: sessionA}, time.Minute, askFor(t, o)); err != nil {
		t.Fatal(err)
	}
	if got := sentTo(f, "term_a"); len(got) != 0 || len(sentTo(f, launchedHandle)) != 1 {
		t.Errorf("sends = %v; want the ask in a new session only", f.called("terminal send"))
	}
}

// Each prompt has the instruction and the CLI instruction, so a change of
// the instruction between two batches keeps the session of the space.
func TestReuseKeepsSessionWhenInstructionChanges(t *testing.T) {
	o, f, sess, r1 := setup(t, config.SessionReuse, started(map[string][]string{
		"terminal show": {"show_claude_live.json"}, "terminal wait": {"wait_after_launch.json", "wait_idle.json"},
		"terminal send": {"send_ok.json", "send_ok.json"},
	}))
	if _, err := o.Deliver(context.Background(), r1); err != nil {
		t.Fatal(err)
	}
	o.p.Primer = "Một hướng dẫn khác: answer in Vietnamese."
	msg := inbox.Message{SourceID: "work", SourceType: "mattermost", ExternalID: "p2", Author: "bob", Text: "again", CreatedAt: time.Now()}
	r2, err := render.Render(o.p, "Triage", "20260102-030405-triage-2", []inbox.Message{msg}, t.TempDir(), "/usr/local/bin/i2a")
	if err != nil {
		t.Fatal(err)
	}
	res, err := o.Deliver(context.Background(), r2)
	if err != nil {
		t.Fatal(err)
	}
	if len(f.called("agent launch")) != 1 || sess["triage"] != launchedHandle || strings.Contains(res.Detail, "started session") {
		t.Errorf("session = %v, result = %+v, want the second batch in %s", sess, res, launchedHandle)
	}
	if got := sentTo(f, launchedHandle); len(got) != 2 || got[1] != r2.Prompt {
		t.Errorf("sends = %q, want the second prompt", got)
	}
	if b, _ := os.ReadFile(r2.DigestPath); !strings.Contains(string(b), "Một hướng dẫn khác") {
		t.Errorf("the second batch does not have the new instruction:\n%s", b)
	}
}

// A new terminal shows a shell first, and a shell is idle too. The driver
// sends the prompt only after the agent runs in the terminal.
func TestNewTerminalWaitsForTheAgent(t *testing.T) {
	o, f, _, r := setup(t, config.SessionNew, started(map[string][]string{
		"terminal show": {"show_agent_exited.json", "show_agent_exited.json"},
	}))
	res, err := o.Deliver(context.Background(), r)
	if err != nil || res.Status != StatusSent {
		t.Fatalf("result = %+v, err = %v", res, err)
	}
	if n := len(f.called("terminal show")); n != 3 {
		t.Errorf("terminal show calls = %d, want 3 (shell, shell, agent)", n)
	}
	var order []string
	for _, c := range f.calls {
		order = append(order, c[0]+" "+c[1])
	}
	if i, j := slices.Index(order, "terminal show"), slices.Index(order, "terminal send"); i < 0 || j < i {
		t.Errorf("calls = %v; want the agent check before the send", order)
	}
}

// No agent starts: the prompt would run as shell commands, so the driver
// sends nothing and stores no session.
func TestNoAgentStartedSendsNothing(t *testing.T) {
	for _, name := range []string{"orca check", "claude check"} {
		shells := []string{"show_agent_exited.json", "show_agent_exited.json", "show_agent_exited.json", "show_agent_exited.json"}
		o, f, sess, r := setup(t, config.SessionReuse, map[string][]string{
			"agent launch": {"launch_no_prompt.json"}, "terminal rename": {"rename_ok.json"}, "terminal show": shells,
		})
		o.StartTimeout = 3 * time.Second
		if name == "claude check" {
			o.Claude = finder(t, map[int]string{4242: "term_other"}) // no Claude process in the new terminal
		}
		_, err := o.Deliver(context.Background(), r)
		if !errors.Is(err, errNoAgentStarted) || !strings.Contains(err.Error(), "i2a sent nothing") {
			t.Errorf("%s: err = %v, want errNoAgentStarted", name, err)
		}
		if len(f.called("terminal send")) != 0 || len(f.called("terminal wait")) != 0 || sess["triage"] != "" {
			t.Errorf("%s: calls = %v, session = %v; want no wait, no send and no session", name, f.calls, sess)
		}
	}
}
