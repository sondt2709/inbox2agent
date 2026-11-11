package agent

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/sondt2709/inbox2agent/internal/config"
	"github.com/sondt2709/inbox2agent/internal/inbox"
)

// A Claude Code session that i2a starts in Orca keeps the arguments that
// Orca gives to claude (the plugin folder of the Orca agent view and the
// permission mode). So the driver starts each terminal with
// "orca agent launch" and types the Claude commands /resume <id> and
// /rename <name> into it (checked on Claude Code 2.1.322 and Orca 1.4.253).
// "claude --resume" in "orca terminal create --command" loses these
// arguments.

// isSessionID reports whether s has the form of a Claude session ID. The
// driver types only an ID of this form after /resume.
func isSessionID(s string) bool { return uuidRe.MatchString(s) }

// maxSessionName is the longest Claude session name that i2a sets, in characters.
const maxSessionName = 80

// sessionName is the name of a Claude session that i2a starts: the tab name,
// on one line, with no control or format characters, at most 80 characters.
func sessionName(s config.Space) string {
	var b strings.Builder
	for _, r := range TabTitle(s) {
		switch {
		case r == '\n' || r == '\r' || r == '\t':
			b.WriteRune(' ')
		case unicode.IsControl(r) || unicode.Is(unicode.Cf, r):
		default:
			b.WriteRune(r)
		}
	}
	name := strings.Join(strings.Fields(b.String()), " ")
	if r := []rune(name); len(r) > maxSessionName {
		name = strings.TrimSpace(string(r[:maxSessionName]))
	}
	return name
}

// shortID is the first part of a Claude session ID, for the detail.
func shortID(id string) string {
	if i := strings.IndexByte(id, '-'); i > 0 {
		return id[:i]
	}
	return id
}

// resumable returns the Claude session of the stored row that the driver
// can resume in a new terminal, or "". The session must have the form of an
// ID, the stored terminal must run no Claude process (it is gone, or /exit
// left a shell; another session there means that the user ran /clear or a
// new claude), and no Claude process may run the session: two processes
// must not write one session.
func (o *Orca) resumable(ss inbox.Session) string {
	if o.Claude == nil || o.p.Session != config.SessionReuse || ss.Handle == "" || !isSessionID(ss.AgentSession) {
		return ""
	}
	if procs, err := o.Claude.InTerminal(ss.Handle); err != nil || len(procs) != 0 {
		return ""
	}
	if procs, err := o.Claude.WithSession(ss.AgentSession); err != nil || len(procs) != 0 {
		return ""
	}
	return ss.AgentSession
}

// opened is a new terminal that is ready for its first prompt.
type opened struct {
	session   string // the Claude session in the terminal; "" when not known
	name      string // its name
	orcaAgent string // the Orca agent of its process
	resumed   bool   // the terminal runs the session that the driver resumed
	prefix    string // starts the detail: the resume failed
	note      string // ends the detail: the name was not set
}

// open makes a new terminal ready for the first prompt: the agent runs in it
// and is idle. For Claude Code, open resumes the session resume when it is
// not "", else it names the new session.
func (o *Orca) open(ctx context.Context, handle, resume string) (opened, error) {
	var op opened
	if err := o.waitAgent(ctx, handle); err != nil {
		return op, err
	}
	if err := o.waitIdle(ctx, handle); err != nil {
		return op, err
	}
	if _, err := o.Claude.All(); err != nil {
		return op, nil // another CLI (nil finder), or the Claude files cannot be read
	}
	if resume != "" {
		if err := o.command(ctx, handle, "/resume "+resume); err != nil {
			return op, err
		}
		if p, ok := o.waitSession(handle, resume); ok {
			if err := o.waitIdle(ctx, handle); err != nil {
				return op, err
			}
			return opened{session: p.SessionID, name: p.Name, orcaAgent: p.OrcaAgent, resumed: true}, nil
		}
		op.prefix = "could not resume Claude session " + shortID(resume) + "; "
	}
	err := o.command(ctx, handle, "/rename "+sessionName(o.space))
	switch {
	case errors.Is(err, errNoAgent):
		return op, err
	case err != nil:
		op.note = "; session name not set: " + err.Error()
	default:
		if err := o.waitIdle(ctx, handle); err != nil {
			return op, err
		}
	}
	p := o.claudeProc(handle)
	op.session, op.name, op.orcaAgent = p.SessionID, p.Name, p.OrcaAgent
	return op, nil
}

// waitSession reads the Claude process of the terminal each second, for up
// to ResumeTimeout, until it runs the session id.
func (o *Orca) waitSession(handle, id string) (ClaudeProc, bool) {
	for waited := time.Duration(0); ; waited += time.Second {
		procs, err := o.Claude.InTerminal(handle)
		if err == nil && len(procs) == 1 && procs[0].SessionID == id {
			return procs[0], true
		}
		if waited >= o.ResumeTimeout {
			return ClaudeProc{}, false
		}
		o.Sleep(time.Second)
	}
}

// command types a Claude command, for example /resume, and presses Enter one
// time. Unlike send, it never presses Enter again: the result of the command
// shows in the session file, not as a turn.
func (o *Orca) command(ctx context.Context, handle, text string) error {
	var sr sendResult
	if err := o.call(ctx, &sr, "terminal", "send", "--terminal", handle, "--text", text, "--enter",
		"--wait-submit", fmt.Sprint(int(o.SubmitWait.Seconds()))); err != nil {
		return err
	}
	if !sr.Send.Accepted {
		return fmt.Errorf("orca terminal send: the terminal %s did not accept the input", handle)
	}
	if sr.Send.Prompt.Observation == "unsupported" && !o.claudeRuns(handle) {
		return fmt.Errorf("%w %s", errNoAgent, handle)
	}
	return nil
}

// claudeRuns reports whether exactly one interactive Claude process runs in
// the terminal. Orca reports observation "unsupported" for each send to
// claude-agent-teams, so with the Claude files that observation does not
// mean a shell.
func (o *Orca) claudeRuns(handle string) bool {
	if o.Claude == nil {
		return false
	}
	procs, err := o.Claude.InTerminal(handle)
	return err == nil && len(procs) == 1
}
