package agent

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/sondt2709/inbox2agent/internal/config"
)

// teamsProcs gives the terminal of each running pid; the pids in teams run
// under the Orca agent claude-agent-teams (ORCA_AGENT_TEAMS_TEAM_ID is set).
func teamsProcs(handles map[int]string, teams ...int) func(int) (ProcInfo, error) {
	return func(pid int) (ProcInfo, error) {
		h, ok := handles[pid]
		if !ok {
			return ProcInfo{}, ErrNoProcess
		}
		return ProcInfo{Start: fixtureStart[pid], Handle: h, Teams: slices.Contains(teams, pid)}, nil
	}
}

func TestOrcaAgentFromTheEnvironment(t *testing.T) {
	f := &ClaudeFinder{Dir: copyDir(t, "4242.json", "4343.json"), Proc: teamsProcs(map[int]string{4242: "term_a", 4343: ""}, 4242)}
	all, err := f.All()
	if err != nil {
		t.Fatal(err)
	}
	got := map[int]string{}
	for _, p := range all {
		got[p.PID] = p.OrcaAgent
	}
	if got[4242] != OrcaAgentTeams || got[4343] != "" {
		t.Errorf("Orca agents = %v, want 4242 %s and 4343 none (no Orca terminal)", got, OrcaAgentTeams)
	}
	f.Proc = teamsProcs(map[int]string{4242: "term_a"})
	if procs, _ := f.InTerminal("term_a"); len(procs) != 1 || procs[0].OrcaAgent != OrcaAgentClaude {
		t.Errorf("procs = %+v, want the Orca agent claude", procs)
	}
}

func TestResumeLaunchesTheStoredOrcaAgentWithoutModel(t *testing.T) {
	o, f, sess, r := setup(t, config.SessionReuse, claudeStart(2))
	o.p.Model = "haiku"
	newTerminal(t, o, nil, true)
	proc := o.Claude.Proc
	o.Claude.Proc = func(pid int) (ProcInfo, error) { // Orca launched the new terminal as claude-agent-teams
		p, err := proc(pid)
		p.Teams = p.Handle == launchedHandle
		return p, err
	}
	sess["triage"], sess["triage#agent"], sess["triage#orca"] = "term_old", sessionA, OrcaAgentTeams
	if _, err := o.Deliver(context.Background(), r); err != nil {
		t.Fatal(err)
	}
	launch := f.called("agent launch")[0]
	if flag(launch, "--agent") != OrcaAgentTeams || slices.Contains(launch, "--model") {
		t.Errorf("launch = %v, want --agent %s and no --model", launch, OrcaAgentTeams)
	}
	if got := sentTo(f, launchedHandle); len(got) != 2 || got[0] != "/resume "+sessionA {
		t.Errorf("sends = %q, want /resume then the batch", got)
	}
	if sess["triage#orca"] != OrcaAgentTeams {
		t.Errorf("session = %v, want the Orca agent %s kept", sess, OrcaAgentTeams)
	}
}

func TestNewSessionLaunchesTheCLI(t *testing.T) {
	for _, c := range []struct {
		cli       string
		wantModel bool
	}{{"claude", true}, {OrcaAgentTeams, false}} {
		o, f, sess, r := setup(t, config.SessionReuse, started(nil))
		o.p.CLI, o.p.Model = c.cli, "haiku"
		sess["triage"], sess["triage#agent"], sess["triage#orca"] = "", "", OrcaAgentTeams
		if _, err := o.Deliver(context.Background(), r); err != nil {
			t.Fatal(err)
		}
		launch := f.called("agent launch")[0]
		if flag(launch, "--agent") != c.cli || slices.Contains(launch, "--model") != c.wantModel {
			t.Errorf("cli %s: launch = %v, want --agent %s, --model %v", c.cli, launch, c.cli, c.wantModel)
		}
	}
}

func TestSendToAnAgentThatOrcaCannotObserve(t *testing.T) {
	o, f, sess, r := setup(t, config.SessionReuse, map[string][]string{
		"terminal show": {"show_claude_live.json"}, "terminal wait": {"wait_idle.json"}, "terminal send": {"send_shell.json"},
	})
	o.Claude = &ClaudeFinder{Dir: copyDir(t, "4242.json"), Proc: teamsProcs(map[int]string{4242: "term_a"}, 4242)}
	sess["triage"], sess["triage#agent"] = "term_a", sessionA
	res, err := o.Deliver(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != StatusSent || !strings.Contains(res.Detail, "Orca cannot observe this agent") {
		t.Errorf("result = %+v, want sent with the note", res)
	}
	if n := len(f.called("terminal send")); n != 1 {
		t.Errorf("%d sends, want 1: no second Enter", n)
	}
	if sess["triage"] != "term_a" || sess["triage#orca"] != OrcaAgentTeams {
		t.Errorf("session = %v, want term_a kept with the Orca agent %s", sess, OrcaAgentTeams)
	}
}

func TestUnsupportedWithoutAClaudeProcessIsAnError(t *testing.T) {
	o, _, sess, r := setup(t, config.SessionReuse, map[string][]string{
		"terminal show": {"show_claude_live.json"}, "terminal wait": {"wait_idle.json"}, "terminal send": {"send_shell.json"},
	})
	// Claude runs in term_a at the check, and exits before Orca types: the
	// text goes to the shell.
	gone := false
	o.Claude = &ClaudeFinder{Dir: copyDir(t, "4242.json"), Proc: func(pid int) (ProcInfo, error) {
		if gone {
			return ProcInfo{}, ErrNoProcess
		}
		return ProcInfo{Start: fixtureStart[pid], Handle: "term_a"}, nil
	}}
	run := o.Run
	o.Run = func(ctx context.Context, bin string, args ...string) ([]byte, error) {
		if args[0] == "terminal" && args[1] == "send" {
			gone = true
		}
		return run(ctx, bin, args...)
	}
	sess["triage"], sess["triage#agent"] = "term_a", sessionA
	_, err := o.Deliver(context.Background(), r)
	if err == nil || !strings.Contains(err.Error(), "no agent runs in the terminal term_a") || sess["triage"] != "" {
		t.Errorf("err = %v, session = %v; want the error and no session", err, sess)
	}
}

func TestAdoptRecordsTheOrcaAgent(t *testing.T) {
	o, _, sess, r := setup(t, config.SessionReuse, map[string][]string{
		"terminal show": {"show_claude_live.json"}, "terminal wait": {"wait_idle.json"}, "terminal send": {"send_ok.json"},
	})
	o.Claude = &ClaudeFinder{Dir: copyDir(t, "4242.json"), Proc: teamsProcs(map[int]string{4242: "term_a"}, 4242)}
	sess["triage"] = "term_a" // no Claude session and no Orca agent yet
	if _, err := o.Deliver(context.Background(), r); err != nil {
		t.Fatal(err)
	}
	if sess["triage#agent"] != sessionA || sess["triage#orca"] != OrcaAgentTeams {
		t.Errorf("session = %v, want %s under %s", sess, sessionA, OrcaAgentTeams)
	}
}
