package agent

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/sondt2709/inbox2agent/internal/config"
	"github.com/sondt2709/inbox2agent/internal/inbox"
)

// States of a stored session for the dashboard.
const (
	StateLive         = "live"
	StateNoClaude     = "no Claude"     // the terminal runs, but no Claude process runs in it
	StateOtherSession = "other session" // the terminal runs another Claude session
	StateClosed       = "closed"        // Orca does not run the terminal
	StateNoAgent      = "no agent"      // Orca reports no agent in the terminal
	StateUnknown      = "unknown"       // i2a cannot read the Claude sessions or Orca
)

// SessionState is what the dashboard shows about the session of a space.
type SessionState struct {
	inbox.Session
	State   string
	Name    string // the name of the Claude session
	Title   string // the title of the Orca tab
	Current string // the Claude session that runs in the terminal now, when it is another one
	Err     string // the Orca error when State is unknown
}

// Inspector reads the sessions of an agent for the dashboard. It never
// types into a terminal.
type Inspector struct {
	Bin    string
	Run    Runner
	Claude *ClaudeFinder // nil for an agent that is not Claude Code
}

// NewInspector returns the inspector of an agent.
func NewInspector(p config.Agent) *Inspector {
	in := &Inspector{Bin: p.OrcaBin, Run: ExecRunner}
	if in.Bin == "" {
		in.Bin = "orca"
	}
	if IsClaude(p.CLI) {
		in.Claude = NewClaudeFinder()
	}
	return in
}

func (in *Inspector) orca() *Orca {
	return &Orca{p: config.Agent{OrcaBin: in.Bin}, Run: in.Run}
}

// State returns the state of a stored session.
func (in *Inspector) State(ctx context.Context, ss inbox.Session) SessionState {
	st := SessionState{Session: ss}
	if ss.Handle == "" {
		return st
	}
	claude := StateUnknown
	if in.Claude != nil {
		procs, err := in.Claude.InTerminal(ss.Handle)
		switch {
		case err != nil:
		case len(procs) == 0:
			claude = StateNoClaude
		case len(procs) == 1 && (ss.AgentSession == "" || procs[0].SessionID == ss.AgentSession):
			claude, st.Name = StateLive, procs[0].Name
		default:
			claude, st.Name, st.Current = StateOtherSession, procs[0].Name, procs[0].SessionID
		}
	}
	var sr showResult
	err := in.orca().call(ctx, &sr, "terminal", "show", "--terminal", ss.Handle)
	switch {
	case isCode(err, "terminal_handle_stale") || isCode(err, "terminal_not_found"):
		st.State = StateClosed
		return st
	case err != nil:
		st.State, st.Err = StateUnknown, err.Error()
		return st
	}
	t := sr.Terminal
	st.Title = t.Title
	exited := len(t.ExitCause) > 0 && string(t.ExitCause) != "null"
	switch {
	case !t.Connected || !t.Writable || exited:
		st.State = StateClosed
	case claude != StateUnknown:
		st.State = claude
	case t.AgentIdentity == "":
		st.State = StateNoAgent
	default:
		st.State = StateLive
	}
	return st
}

var uuidRe = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// Resolve finds the one running Claude process of a terminal handle
// (term_…) or of a Claude session ID. The user links it to a space.
func (in *Inspector) Resolve(input string) (ClaudeProc, error) {
	v := strings.TrimSpace(input)
	if in.Claude == nil {
		return ClaudeProc{}, errors.New("the CLI of this agent is not Claude Code: i2a cannot read its sessions")
	}
	var procs []ClaudeProc
	var err error
	switch {
	case strings.HasPrefix(v, "term_"):
		procs, err = in.Claude.InTerminal(v)
	case uuidRe.MatchString(strings.ToLower(v)):
		v = strings.ToLower(v)
		procs, err = in.Claude.WithSession(v)
	default:
		return ClaudeProc{}, errors.New("enter a terminal handle (term_…) or a Claude session ID")
	}
	if err != nil {
		return ClaudeProc{}, err
	}
	switch {
	case len(procs) == 0:
		return ClaudeProc{}, fmt.Errorf("no running Claude session in %s", v)
	case len(procs) > 1:
		return ClaudeProc{}, fmt.Errorf("%d Claude processes run %s; close one, then try again", len(procs), v)
	case procs[0].Handle == "":
		return ClaudeProc{}, fmt.Errorf("the Claude session %s does not run in an Orca terminal", procs[0].SessionID)
	}
	return procs[0], nil
}

// Switch shows the terminal in Orca.
func (in *Inspector) Switch(ctx context.Context, handle string) error {
	return in.orca().call(ctx, nil, "terminal", "switch", "--terminal", handle)
}
