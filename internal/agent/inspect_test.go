package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/sondt2709/inbox2agent/internal/inbox"
)

func inspector(t *testing.T, handles map[int]string, answers map[string][]string) (*Inspector, *fakeOrca) {
	t.Helper()
	f := &fakeOrca{t: t, answers: answers}
	return &Inspector{Bin: "orca", Run: f.run, Claude: finder(t, handles)}, f
}

func TestInspectorState(t *testing.T) {
	for _, tc := range []struct {
		name    string
		ss      inbox.Session
		handles map[int]string
		show    string
		want    string
	}{
		{"live", inbox.Session{Handle: "term_a", AgentSession: sessionA}, map[int]string{4242: "term_a"}, "show_claude_live.json", StateLive},
		{"no Claude", inbox.Session{Handle: "term_a", AgentSession: sessionA}, nil, "show_agent_exited.json", StateNoClaude},
		{"other session", inbox.Session{Handle: "term_a", AgentSession: sessionB}, map[int]string{4242: "term_a"}, "show_claude_live.json", StateOtherSession},
		{"closed", inbox.Session{Handle: "term_a", AgentSession: sessionA}, map[int]string{4242: "term_a"}, "show_closed.json", StateClosed},
		{"stale", inbox.Session{Handle: "term_a"}, nil, "show_stale.json", StateClosed},
	} {
		in, _ := inspector(t, tc.handles, map[string][]string{"terminal show": {tc.show}})
		st := in.State(context.Background(), tc.ss)
		if st.State != tc.want {
			t.Errorf("%s: state = %q, want %q", tc.name, st.State, tc.want)
		}
		if tc.want == StateLive && (st.Name != "Mattermost inbox triage" || st.Title != "Claude Code") {
			t.Errorf("%s: name %q, title %q", tc.name, st.Name, st.Title)
		}
		if tc.want == StateOtherSession && st.Current != sessionA {
			t.Errorf("%s: current = %q, want %s", tc.name, st.Current, sessionA)
		}
	}
	in, _ := inspector(t, nil, map[string][]string{"terminal show": {"show_agent_exited.json"}})
	in.Claude = nil // another CLI: the Orca field decides
	if st := in.State(context.Background(), inbox.Session{Handle: "term_a"}); st.State != StateNoAgent {
		t.Errorf("state = %q, want %q", st.State, StateNoAgent)
	}
}

func TestInspectorResolve(t *testing.T) {
	in, _ := inspector(t, map[int]string{4242: "term_a", 4343: ""}, nil)
	p, err := in.Resolve(" term_a ")
	if err != nil || p.SessionID != sessionA {
		t.Errorf("by handle: %+v, %v", p, err)
	}
	p, err = in.Resolve(strings.ToUpper(sessionA))
	if err != nil || p.Handle != "term_a" {
		t.Errorf("by session: %+v, %v", p, err)
	}
	for in2, want := range map[string]string{
		"term_d":    "no running Claude session in term_d",
		sessionB:    "does not run in an Orca terminal",
		"triage":    "enter a terminal handle",
		"term_a; x": "no running Claude session",
	} {
		if _, err := in.Resolve(in2); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("Resolve(%q) = %v, want %q", in2, err, want)
		}
	}
}

func TestInspectorSwitch(t *testing.T) {
	in, f := inspector(t, nil, map[string][]string{"terminal switch": {"switch_ok.json", "switch_exited.json"}})
	if err := in.Switch(context.Background(), "term_a"); err != nil {
		t.Fatal(err)
	}
	if flag(f.called("terminal switch")[0], "--terminal") != "term_a" {
		t.Errorf("calls = %v", f.calls)
	}
	if err := in.Switch(context.Background(), "term_a"); err == nil || !strings.Contains(err.Error(), "terminal_exited") {
		t.Errorf("err = %v", err)
	}
}
