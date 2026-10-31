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

// memSessions keeps the handle of a space under its ID and the primer
// hash under "<name>#hash".
type memSessions map[string]string

func (m memSessions) Session(_ context.Context, p string) (string, string, error) {
	return m[p], m[p+"#hash"], nil
}
func (m memSessions) SetSession(_ context.Context, p, h, hash string, _ time.Time) error {
	m[p], m[p+"#hash"] = h, hash
	return nil
}
func (m memSessions) DeleteSession(_ context.Context, p string) error { delete(m, p); return nil }

func setup(t *testing.T, session string, answers map[string][]string) (*Orca, *fakeOrca, memSessions, *render.Rendered) {
	t.Helper()
	p := config.NewAgent("triage")
	p.Driver = config.DriverOrca
	p.Session = session
	p.Workspace = "/home/user/notes"
	sess := memSessions{}
	o := NewOrca(p, "triage", sess)
	f := &fakeOrca{t: t, answers: answers}
	o.Run = f.run
	msg := inbox.Message{SourceID: "work", SourceType: "mattermost", ExternalID: "p1", Author: "bob", Text: "hi", CreatedAt: time.Now()}
	r, err := render.Render(p, "Triage", "20260102-030405-triage-1", []inbox.Message{msg}, t.TempDir(), "/usr/local/bin/i2a")
	if err != nil {
		t.Fatal(err)
	}
	sess["triage#hash"] = r.PrimerHash // a stored session got the current primer, unless a test changes it
	return o, f, sess, r
}

const (
	launchedHandle = "term_f8323417-9685-4934-a061-20c3fc4ccbff" // the handle in launch_no_prompt.json
	worktreeHandle = "term_a064be47-dfe2-4514-bc43-db02bbe10606" // the handle in worktree_create_no_prompt.json
)

// started are the answers for a new agent: launch, ready, prompt submitted.
func started(extra map[string][]string) map[string][]string {
	a := map[string][]string{
		"agent launch": {"launch_no_prompt.json"}, "terminal wait": {"wait_after_launch.json"}, "terminal send": {"send_ok.json"},
	}
	for k, v := range extra {
		a[k] = v
	}
	return a
}

func TestNewModeLaunchesThenSubmitsPrimerAndBatch(t *testing.T) {
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
	if !strings.HasPrefix(prompt, strings.TrimSpace(r.Primer)) || !strings.HasSuffix(prompt, strings.TrimSpace(r.Prompt)) {
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
	if sess["triage"] != launchedHandle || res.Status != StatusSent || !strings.Contains(res.Detail, "with the primer") {
		t.Errorf("session=%v result=%+v", sess, res)
	}
	if prompt := flag(f.called("terminal send")[0], "--text"); !strings.HasPrefix(prompt, strings.TrimSpace(r.Primer)) {
		t.Errorf("the first prompt has no primer: %q", prompt)
	}
}

func TestReuseDoesNotStoreSessionWhenSubmitFails(t *testing.T) {
	o, _, sess, r := setup(t, config.SessionReuse, started(map[string][]string{"terminal wait": {"wait_timeout.json"}}))
	if _, err := o.Deliver(context.Background(), r); err == nil {
		t.Fatal("no error")
	}
	if sess["triage"] != "" {
		t.Errorf("stored a session without the primer: %v", sess)
	}
}

func TestReuseLiveIdleSendsBatchOnly(t *testing.T) {
	o, f, sess, r := setup(t, config.SessionReuse, map[string][]string{
		"terminal show": {"show_live.json"}, "terminal wait": {"wait_idle.json"}, "terminal send": {"send_ok.json"},
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
	if strings.Contains(flag(send, "--text"), "You are my inbox assistant") {
		t.Error("the primer was sent again")
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
		"terminal show": {"show_live.json"}, "terminal wait": {"wait_timeout.json"},
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
		if sess["triage"] != launchedHandle || !strings.Contains(res.Detail, "with the primer") {
			t.Errorf("%s: session=%v result=%+v", show, sess, res)
		}
	}
}

// A session that got another primer (or no hash, from schema version 2) is
// replaced: the driver does not even check the old terminal.
func TestReusePrimerChangedStartsNewSession(t *testing.T) {
	for _, old := range []string{"", "0123456789abcdef"} {
		o, f, sess, r := setup(t, config.SessionReuse, started(nil))
		sess["triage"], sess["triage#hash"] = "term_live", old
		res, err := o.Deliver(context.Background(), r)
		if err != nil {
			t.Fatalf("hash %q: %v", old, err)
		}
		if sess["triage"] != launchedHandle || sess["triage#hash"] != r.PrimerHash {
			t.Errorf("hash %q: session = %v, want the new handle and hash", old, sess)
		}
		if !strings.Contains(res.Detail, "primer changed, started session") || len(f.called("terminal show")) != 0 {
			t.Errorf("hash %q: result = %+v, calls = %v", old, res, f.calls)
		}
		if text := flag(f.called("terminal send")[0], "--text"); !strings.Contains(text, "/usr/local/bin/i2a card add") {
			t.Errorf("hash %q: the new session did not get the card section: %.200q", old, text)
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
		"worktree create": {"worktree_create_no_prompt.json"}, "terminal wait": {"wait_after_launch.json"}, "terminal send": {"send_ok.json"},
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
	other := NewOrca(o.p, "borealis", sess)
	f := &fakeOrca{t: t, answers: started(nil)}
	other.Run = f.run
	res, err := other.Deliver(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	if len(f.called("agent launch")) != 1 || sess["borealis"] != launchedHandle || !strings.Contains(res.Detail, "with the primer") {
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

const cardHandle = "term_b9f22da8-13d6-49bc-a1ab-53eba56c9578" // the handle in show_live.json

func TestAskLiveCardTerminalGetsPromptOnly(t *testing.T) {
	o, f, sess, _ := setup(t, config.SessionReuse, map[string][]string{
		"terminal show": {"show_live.json"}, "terminal wait": {"wait_idle.json"}, "terminal send": {"send_ok.json"},
	})
	sess["triage"] = "term_profile"
	a := askFor(t, o)
	res, err := o.Ask(context.Background(), cardHandle, 2*time.Minute, a)
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
		"terminal show": {"show_live.json"}, "terminal wait": {"wait_timeout.json"},
	})
	res, err := o.Ask(context.Background(), cardHandle, 2*time.Minute, askFor(t, o))
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
		"terminal show": {"show_closed.json", "show_live.json"}, "terminal wait": {"wait_idle.json"}, "terminal send": {"send_ok.json"},
	})
	sess["triage"] = "term_profile"
	a := askFor(t, o)
	res, err := o.Ask(context.Background(), "term_old", 2*time.Minute, a)
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
		t.Errorf("the profile session got the primer again: %v", send)
	}
	if o.IdleTimeout != 5*time.Second {
		t.Errorf("idle timeout = %s after the ask, want 5s for the next batch", o.IdleTimeout)
	}
}

func TestAskWithoutSessionStartsOneWithPrimer(t *testing.T) {
	o, f, sess, _ := setup(t, config.SessionReuse, started(nil))
	a := askFor(t, o)
	res, err := o.Ask(context.Background(), "", 2*time.Minute, a)
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != StatusSent || res.Handle != launchedHandle || sess["triage"] != launchedHandle {
		t.Errorf("result = %+v, sessions = %v", res, sess)
	}
	prompt := flag(f.called("terminal send")[0], "--text")
	if !strings.HasPrefix(prompt, strings.TrimSpace(a.Primer)) || !strings.HasSuffix(prompt, strings.TrimSpace(a.Prompt)) {
		t.Errorf("prompt = %q", prompt)
	}
}

func TestDryRunRefusesAsk(t *testing.T) {
	if _, err := (DryRun{}).Ask(context.Background(), "", time.Minute, &render.Rendered{}); !errors.Is(err, ErrDryRunAsk) {
		t.Errorf("err = %v", err)
	}
}
