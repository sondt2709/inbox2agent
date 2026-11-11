package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/sondt2709/inbox2agent/internal/config"
)

// newTerminal gives the driver the Claude processes when the stored session
// sessionA runs in no terminal (pid -> terminal in before), and the new
// terminal runs session B (4343). When resumes is true, the new terminal
// runs session A (4242) after "/resume <sessionA>" arrives there, as Claude
// Code 2.1.322 does. It returns the sends of slash commands.
func newTerminal(t *testing.T, o *Orca, before map[int]string, resumes bool) {
	t.Helper()
	resumed := false
	o.Claude = &ClaudeFinder{Dir: copyDir(t, "4242.json", "4343.json"), Proc: func(pid int) (ProcInfo, error) {
		h, ok := before[pid]
		switch {
		case pid == 4343 && !resumed:
			h, ok = launchedHandle, true
		case pid == 4242 && resumed:
			h, ok = launchedHandle, true
		}
		if !ok {
			return ProcInfo{}, ErrNoProcess
		}
		return ProcInfo{Start: fixtureStart[pid], Handle: h}, nil
	}}
	run := o.Run
	o.Run = func(ctx context.Context, bin string, args ...string) ([]byte, error) {
		out, err := run(ctx, bin, args...)
		if resumes && args[0] == "terminal" && args[1] == "send" && flag(args, "--terminal") == launchedHandle &&
			flag(args, "--text") == "/resume "+sessionA {
			resumed = true
		}
		return out, err
	}
}

// claudeStart are the answers for a new Claude terminal: launch, tab name, ready,
// then n sends, each followed by a wait for tui-idle.
func claudeStart(n int) map[string][]string {
	a := map[string][]string{
		"agent launch": {"launch_no_prompt.json"}, "terminal rename": {"rename_ok.json"},
		"terminal wait": {"wait_after_launch.json"},
	}
	for range n {
		a["terminal send"] = append(a["terminal send"], "send_ok.json")
		a["terminal wait"] = append(a["terminal wait"], "wait_idle.json")
	}
	return a
}

func TestReuseResumesTheStoredSessionInANewTerminal(t *testing.T) {
	o, f, sess, r := setup(t, config.SessionReuse, claudeStart(2))
	newTerminal(t, o, nil, true) // term_old is gone and runs nothing
	sess["triage"], sess["triage#agent"] = "term_old", sessionA
	res, err := o.Deliver(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	got := sentTo(f, launchedHandle)
	if len(got) != 2 || got[0] != "/resume "+sessionA || got[1] != r.Prompt {
		t.Fatalf("sends to the new terminal = %q, want /resume then the batch", got)
	}
	if !strings.HasPrefix(res.Detail, "resumed Claude session 04ebe19c in "+launchedHandle+", turn started") {
		t.Errorf("detail = %q", res.Detail)
	}
	if sess["triage"] != launchedHandle || sess["triage#agent"] != sessionA || sess["triage#name"] != "Mattermost inbox triage" {
		t.Errorf("session = %v, want %s, %s and the name of the resumed session", sess, launchedHandle, sessionA)
	}
	if sess["triage#sent"] != testNow.Format("2006-01-02T15:04:05Z07:00") {
		t.Errorf("last send = %q, want %v", sess["triage#sent"], testNow)
	}
}

func TestReuseResumesWhenTheSessionEndsDuringTheWait(t *testing.T) {
	o, f, sess, r := setup(t, config.SessionReuse, claudeStart(2))
	// term_a runs session A until the wait; then A runs nowhere.
	newTerminal(t, o, nil, true)
	waited := false
	finder := o.Claude.Proc
	o.Claude.Proc = func(pid int) (ProcInfo, error) {
		if pid == 4242 && !waited {
			return ProcInfo{Start: fixtureStart[pid], Handle: "term_a"}, nil
		}
		return finder(pid)
	}
	run := o.Run
	o.Run = func(ctx context.Context, bin string, args ...string) ([]byte, error) {
		if args[0] == "terminal" && args[1] == "show" {
			return fixture(t, "show_claude_live.json"), nil
		}
		out, err := run(ctx, bin, args...)
		if args[0] == "terminal" && args[1] == "wait" && flag(args, "--terminal") == "term_a" {
			waited = true
		}
		return out, err
	}
	f.answers["terminal wait"] = append([]string{"wait_idle.json"}, f.answers["terminal wait"]...)
	sess["triage"], sess["triage#agent"] = "term_a", sessionA
	res, err := o.Deliver(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	if len(sentTo(f, "term_a")) != 0 || !strings.HasPrefix(res.Detail, "Claude session ended, resumed Claude session 04ebe19c") {
		t.Errorf("sends = %v, detail = %q", f.called("terminal send"), res.Detail)
	}
}

func TestReuseDoesNotResumeAfterClear(t *testing.T) {
	// term_a runs session B (4343) after /clear, and session A runs nowhere.
	// After the launch, the new terminal runs 4242.
	o, f, sess, r := setup(t, config.SessionReuse, claudeStart(2))
	o.Claude = &ClaudeFinder{Dir: copyDir(t, "4242.json", "4343.json"), Proc: fakeProcs(map[int]string{4343: "term_a"})}
	run := o.Run
	o.Run = func(ctx context.Context, bin string, args ...string) ([]byte, error) {
		if args[0] == "agent" && args[1] == "launch" {
			o.Claude.Proc = fakeProcs(map[int]string{4343: "term_a", 4242: launchedHandle})
		}
		return run(ctx, bin, args...)
	}
	sess["triage"], sess["triage#agent"] = "term_a", sessionA
	if _, err := o.Deliver(context.Background(), r); err != nil {
		t.Fatal(err)
	}
	got := sentTo(f, launchedHandle)
	if len(got) != 2 || got[0] != "/rename i2a · Triage" || got[1] != r.Prompt || len(sentTo(f, "term_a")) != 0 {
		t.Errorf("sends = %v, want /rename then the batch in the new terminal, no /resume", f.called("terminal send"))
	}
}

func TestReuseDoesNotResumeASessionThatRunsOutsideOrca(t *testing.T) {
	o, f, sess, r := setup(t, config.SessionReuse, claudeStart(2))
	// 4242 runs session A in a terminal of another app (no handle).
	newTerminal(t, o, map[int]string{4242: ""}, false)
	sess["triage"], sess["triage#agent"] = "term_old", sessionA
	if _, err := o.Deliver(context.Background(), r); err != nil {
		t.Fatal(err)
	}
	for _, text := range sentTo(f, launchedHandle) {
		if strings.HasPrefix(text, "/resume") {
			t.Errorf("sent %q: session A runs in another process", text)
		}
	}
	if sess["triage#agent"] != sessionB {
		t.Errorf("session = %v, want the new session %s", sess, sessionB)
	}
}

func TestResumeFailsSoTheNewSessionGetsAName(t *testing.T) {
	o, f, sess, r := setup(t, config.SessionReuse, claudeStart(3))
	newTerminal(t, o, nil, false) // Claude cannot open session A
	sess["triage"], sess["triage#agent"] = "term_old", sessionA
	res, err := o.Deliver(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	got := sentTo(f, launchedHandle)
	want := []string{"/resume " + sessionA, "/rename i2a · Triage", r.Prompt}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("sends = %q, want %q", got, want)
	}
	if !strings.HasPrefix(res.Detail, "could not resume Claude session 04ebe19c; started session "+launchedHandle) {
		t.Errorf("detail = %q", res.Detail)
	}
	if sess["triage"] != launchedHandle || sess["triage#agent"] != sessionB {
		t.Errorf("session = %v, want %s with %s", sess, launchedHandle, sessionB)
	}
}

func TestNoResumeWithoutTheClaudeFiles(t *testing.T) {
	// o.Claude is nil: another CLI, or the Orca check only.
	o, f, sess, r := setup(t, config.SessionReuse, started(map[string][]string{"terminal show": {"show_closed.json"}}))
	sess["triage"], sess["triage#agent"] = "term_old", sessionA
	if _, err := o.Deliver(context.Background(), r); err != nil {
		t.Fatal(err)
	}
	if got := sentTo(f, launchedHandle); len(got) != 1 || got[0] != r.Prompt {
		t.Errorf("sends = %q, want the batch only", got)
	}
}

func TestNewModeNamesTheSessionAndDoesNotResume(t *testing.T) {
	o, f, sess, r := setup(t, config.SessionNew, claudeStart(2))
	newTerminal(t, o, nil, true)
	sess["triage"], sess["triage#agent"] = "term_old", sessionA
	if _, err := o.Deliver(context.Background(), r); err != nil {
		t.Fatal(err)
	}
	got := sentTo(f, launchedHandle)
	if len(got) != 2 || got[0] != "/rename i2a · Triage" || got[1] != r.Prompt {
		t.Errorf("sends = %q, want /rename then the batch", got)
	}
}

func TestReuseRecordsTheLastSendAndTheName(t *testing.T) {
	o, _, sess, r := setup(t, config.SessionReuse, map[string][]string{
		"terminal show": {"show_claude_live.json"}, "terminal wait": {"wait_idle.json"}, "terminal send": {"send_ok.json"},
	})
	o.Claude = finder(t, map[int]string{4242: "term_a"})
	sess["triage"], sess["triage#agent"], sess["triage#name"] = "term_a", sessionA, "old name"
	if _, err := o.Deliver(context.Background(), r); err != nil {
		t.Fatal(err)
	}
	if sess["triage#name"] != "Mattermost inbox triage" || sess["triage#sent"] != testNow.Format("2006-01-02T15:04:05Z07:00") {
		t.Errorf("session = %v, want the current name and the last send", sess)
	}
}

func TestSessionName(t *testing.T) {
	cases := map[string]string{
		"Triage":                    "i2a · Triage",
		"Acme\nrm -rf ~\x1b[2J":     "i2a · Acme rm -rf ~[2J",
		strings.Repeat("ab", 60):    "i2a · " + strings.Repeat("ab", 37),
		"  Acme x Borealis  ":       "i2a · Acme x Borealis",
		"Tóm tắt\u200b\u202edanger": "i2a · Tóm tắtdanger",
	}
	for in, want := range cases {
		if got := sessionName(config.Space{ID: "x", Name: in}); got != want {
			t.Errorf("sessionName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestResumeNeedsAUUID(t *testing.T) {
	for id, want := range map[string]bool{sessionA: true, "04EBE19C-543E-42FF-8F84-EA13CEA7A3EC": false, sessionA + "x": false, "": false, "a; rm -rf ~": false} {
		if got := isSessionID(id); got != want {
			t.Errorf("isSessionID(%q) = %v, want %v", id, got, want)
		}
	}
}

func TestNoResumeOfAStoredIDThatIsNotAUUID(t *testing.T) {
	o, f, sess, r := setup(t, config.SessionReuse, claudeStart(2))
	newTerminal(t, o, nil, false)
	sess["triage"], sess["triage#agent"] = "term_old", "04ebe19c; rm -rf ~"
	if _, err := o.Deliver(context.Background(), r); err != nil {
		t.Fatal(err)
	}
	if got := sentTo(f, launchedHandle); len(got) != 2 || got[0] != "/rename i2a · Triage" {
		t.Errorf("sends = %q, want /rename then the batch, no /resume", got)
	}
}
