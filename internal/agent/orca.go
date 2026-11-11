package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/sondt2709/inbox2agent/internal/config"
	"github.com/sondt2709/inbox2agent/internal/inbox"
	"github.com/sondt2709/inbox2agent/internal/render"
)

// Runner runs the orca executable and returns its standard output.
// A non-zero exit is not an error when the output is a JSON envelope.
type Runner func(ctx context.Context, bin string, args ...string) (stdout []byte, err error)

// ExecRunner runs a real process.
func ExecRunner(ctx context.Context, bin string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, bin, args...)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	err := cmd.Run()
	if err != nil && out.Len() == 0 {
		msg := strings.TrimSpace(errb.String())
		if msg == "" {
			msg = err.Error()
		}
		return nil, fmt.Errorf("run %s: %s", bin, msg)
	}
	return out.Bytes(), nil
}

// Orca delivers batches through the orca CLI.
type Orca struct {
	p             config.Agent
	space         config.Space // its ID is the key of the reuse session
	sessions      Sessions
	Claude        *ClaudeFinder // finds the Claude session of a terminal; nil for another CLI
	orcaOnly      bool          // the last check could not read the Claude sessions
	Run           Runner
	Now           func() time.Time
	Sleep         func(time.Duration)
	IdleTimeout   time.Duration // wait for a busy agent in reuse mode
	StartTimeout  time.Duration // wait for a new agent to be ready
	SubmitWait    time.Duration
	ResumeTimeout time.Duration // wait for the session ID after /resume
}

// NewOrca returns the orca driver of an agent for one space.
func NewOrca(p config.Agent, space config.Space, sessions Sessions) *Orca {
	o := &Orca{p: p, space: space, sessions: sessions, Run: ExecRunner, Now: time.Now, Sleep: time.Sleep,
		IdleTimeout: 5 * time.Second, StartTimeout: 90 * time.Second, SubmitWait: 30 * time.Second, ResumeTimeout: 10 * time.Second}
	if IsClaude(p.CLI) {
		o.Claude = NewClaudeFinder()
	}
	return o
}

// errSessionGone means that the Claude session left the terminal while the
// driver waited for the agent to be idle, for example after /exit.
var errSessionGone = errors.New("the Claude session left the terminal")

// errNoAgent is the error of a send that Orca reports with the observation
// "unsupported": no agent runs in the terminal, for example a shell after /exit.
var errNoAgent = errors.New("no agent runs in the terminal")

// OrcaError is an error that the orca CLI reported.
type OrcaError struct {
	Code    string
	Message string
	Command string
}

func (e *OrcaError) Error() string {
	if e.Message != "" && e.Message != e.Code {
		return fmt.Sprintf("orca %s: %s: %s", e.Command, e.Code, e.Message)
	}
	return fmt.Sprintf("orca %s: %s", e.Command, e.Code)
}

type envelope struct {
	OK     bool            `json:"ok"`
	Result json.RawMessage `json:"result"`
	Error  *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

type launchResult struct {
	Outcome struct {
		Kind   string `json:"kind"`
		Handle string `json:"handle"`
	} `json:"outcome"`
}

type showResult struct {
	Terminal struct {
		Handle        string          `json:"handle"`
		Title         string          `json:"title"`
		Connected     bool            `json:"connected"`
		Writable      bool            `json:"writable"`
		ExitCause     json.RawMessage `json:"exitCause"`
		AgentIdentity string          `json:"agentIdentity"` // null after the agent exits
	} `json:"terminal"`
}

type sendResult struct {
	Send struct {
		Accepted bool `json:"accepted"`
		Prompt   struct {
			Stages      []string `json:"stages"`
			Observation string   `json:"observation"` // "unsupported" when no agent runs in the terminal
		} `json:"prompt"`
	} `json:"send"`
}

type worktreeResult struct {
	Worktree struct {
		ID string `json:"id"`
	} `json:"worktree"`
	Launch *launchResult `json:"launch"`
}

func (o *Orca) bin() string {
	if o.p.OrcaBin != "" {
		return o.p.OrcaBin
	}
	return "orca"
}

// call runs one orca command with --json and decodes the result into out.
func (o *Orca) call(ctx context.Context, out any, args ...string) error {
	name := strings.Join(args[:min(2, len(args))], " ")
	b, err := o.Run(ctx, o.bin(), append(args, "--json")...)
	if err != nil {
		return err
	}
	var env envelope
	if err := json.Unmarshal(b, &env); err != nil {
		return fmt.Errorf("orca %s: cannot read the output: %w: %.300s", name, err, b)
	}
	if !env.OK {
		e := &OrcaError{Command: name, Code: "unknown_error"}
		if env.Error != nil {
			e.Code, e.Message = env.Error.Code, env.Error.Message
		}
		return e
	}
	if out != nil {
		if err := json.Unmarshal(env.Result, out); err != nil {
			return fmt.Errorf("orca %s: cannot read the result: %w", name, err)
		}
	}
	return nil
}

func isCode(err error, code string) bool {
	var oe *OrcaError
	return errors.As(err, &oe) && oe.Code == code
}

// workspace returns the absolute path of the folder of the agent.
func (o *Orca) workspace() (string, error) {
	ws, err := ExpandWorkspace(o.p.Folder)
	if err != nil {
		return "", fmt.Errorf("space %q: %w", o.p.ID, err)
	}
	return ws, nil
}

// ExpandWorkspace returns the absolute path of the folder of an agent: ~/ is
// the home folder.
func ExpandWorkspace(ws string) (string, error) {
	if strings.HasPrefix(ws, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		ws = filepath.Join(home, ws[2:])
	}
	if !filepath.IsAbs(ws) {
		return "", fmt.Errorf("the folder %q is not an absolute path", ws)
	}
	return filepath.Clean(ws), nil
}

// Deliver sends the batch in the session mode of the agent.
func (o *Orca) Deliver(ctx context.Context, b *render.Rendered) (Result, error) {
	switch o.p.Session {
	case config.SessionNew:
		h, note, err := o.launch(ctx)
		if err != nil {
			return Result{}, err
		}
		op, detail, err := o.submit(ctx, h, "", b.Prompt)
		if err != nil {
			return Result{}, err
		}
		return Result{Status: StatusSent, Detail: "new session " + h + ", " + detail + op.note + note, Handle: h}, nil
	case config.SessionWorktree:
		return o.worktree(ctx, b)
	default:
		return o.reuse(ctx, b)
	}
}

// launchArgs starts an agent without --prompt. With --prompt, Orca types the
// prompt while the agent starts, and the prompt can stay in the input box
// without Enter. The driver submits the prompt itself (see submit).
func (o *Orca) launchArgs(ws string) []string {
	args := []string{"agent", "launch", "--agent", o.p.CLI, "--surface", "terminal", "--worktree", "path:" + ws}
	if o.p.Model != "" {
		args = append(args, "--model", o.p.Model)
	}
	return args
}

// launch starts a new agent terminal in the workspace, names its tab, and
// returns its handle and a note for the detail. When Orca does not know the
// workspace and it is a git repository, launch adds it with "orca repo add"
// and tries one more time.
func (o *Orca) launch(ctx context.Context) (string, string, error) {
	h, err := o.launchTerminal(ctx)
	if err != nil {
		return "", "", err
	}
	return h, o.rename(ctx, h), nil
}

// TabTitle is the name of the Orca tab of a session that i2a starts.
func TabTitle(s config.Space) string { return "i2a · " + s.Title() }

// rename names the tab of a terminal that i2a started. A failure is a note
// for the detail; the batch still goes.
func (o *Orca) rename(ctx context.Context, handle string) string {
	if err := o.call(ctx, nil, "terminal", "rename", "--terminal", handle, "--title", TabTitle(o.space)); err != nil {
		return "; tab name not set: " + err.Error()
	}
	return ""
}

func (o *Orca) launchTerminal(ctx context.Context) (string, error) {
	ws, err := o.workspace()
	if err != nil {
		return "", err
	}
	var lr launchResult
	err = o.call(ctx, &lr, o.launchArgs(ws)...)
	if isCode(err, "selector_not_found") {
		if addErr := o.call(ctx, nil, "repo", "add", "--path", ws); addErr != nil {
			return "", fmt.Errorf("orca does not know the folder %s and cannot add it (%v). Add the folder in Orca, then try again", ws, addErr)
		}
		err = o.call(ctx, &lr, o.launchArgs(ws)...)
	}
	if err != nil {
		return "", err
	}
	if lr.Outcome.Handle == "" {
		return "", errors.New("orca agent launch: no terminal handle in the result")
	}
	return lr.Outcome.Handle, nil
}

// submit makes a new terminal ready (open), then sends the prompt with Enter.
func (o *Orca) submit(ctx context.Context, handle, resume, prompt string) (opened, string, error) {
	op, err := o.open(ctx, handle, resume)
	if err != nil {
		return op, "", err
	}
	detail, err := o.send(ctx, handle, prompt)
	return op, detail, err
}

// waitIdle waits up to StartTimeout until the TUI of a new agent is idle.
func (o *Orca) waitIdle(ctx context.Context, handle string) error {
	err := o.call(ctx, nil, "terminal", "wait", "--terminal", handle, "--for", "tui-idle",
		"--timeout-ms", fmt.Sprint(o.StartTimeout.Milliseconds()))
	if isCode(err, "timeout") {
		return fmt.Errorf("the agent in %s was not ready after %s", handle, o.StartTimeout)
	}
	return err
}

// errNoAgentStarted is the error of a new terminal in which no agent started.
var errNoAgentStarted = errors.New("no agent started in the terminal")

// waitAgent checks each second, for up to StartTimeout, that the agent runs
// in a new terminal: exactly one Claude process for claude (else the Orca
// agentIdentity, also when the Claude sessions cannot be read). A new
// terminal shows a shell first, and a shell is idle too: a prompt typed into
// it runs as commands, and the message lines of the content are plain text.
// So when no agent starts, i2a sends nothing.
func (o *Orca) waitAgent(ctx context.Context, handle string) error {
	for waited := time.Duration(0); ; waited += time.Second {
		if o.Claude != nil {
			if procs, err := o.Claude.InTerminal(handle); err == nil {
				if len(procs) == 1 {
					return nil
				}
			} else if ok, err := o.orcaLive(ctx, handle, true); err != nil || ok {
				return err
			}
		} else if ok, err := o.orcaLive(ctx, handle, true); err != nil || ok {
			return err
		}
		if waited >= o.StartTimeout {
			return fmt.Errorf("%w %s after %s; i2a sent nothing", errNoAgentStarted, handle, o.StartTimeout)
		}
		o.Sleep(time.Second)
	}
}

// send types the prompt and presses Enter. When Orca cannot prove that the
// turn started, the text can still be in the input box, so send presses Enter
// one more time. It never types the text again.
func (o *Orca) send(ctx context.Context, handle, prompt string) (string, error) {
	var sr sendResult
	if err := o.call(ctx, &sr, "terminal", "send", "--terminal", handle, "--text", prompt, "--enter",
		"--wait-submit", fmt.Sprint(int(o.SubmitWait.Seconds()))); err != nil {
		return "", err
	}
	if !sr.Send.Accepted {
		return "", fmt.Errorf("orca terminal send: the terminal %s did not accept the input", handle)
	}
	if sr.Send.Prompt.Observation == "unsupported" {
		// The text is in a shell or another program now. Do not press Enter again.
		return "", fmt.Errorf("%w %s", errNoAgent, handle)
	}
	if slices.Contains(sr.Send.Prompt.Stages, "turn_started") {
		return "turn started", nil
	}
	if err := o.call(ctx, nil, "terminal", "send", "--terminal", handle, "--enter"); err != nil {
		return "turn not proven, Enter again failed: " + err.Error(), nil
	}
	return "turn not proven, Enter sent again", nil
}

// orcaLive reports whether Orca runs the terminal and it can take input.
// With needAgent, Orca must also report an agent in it (agentIdentity);
// the field stays set for some seconds after the agent exits.
func (o *Orca) orcaLive(ctx context.Context, handle string, needAgent bool) (bool, error) {
	var sr showResult
	err := o.call(ctx, &sr, "terminal", "show", "--terminal", handle)
	if isCode(err, "terminal_handle_stale") || isCode(err, "terminal_not_found") {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	// An agent terminal without a visible pane reports orphaned: true and still runs,
	// so orphaned is not a liveness signal. A closed terminal has an exit cause.
	t := sr.Terminal
	exited := len(t.ExitCause) > 0 && string(t.ExitCause) != "null"
	return t.Connected && t.Writable && !exited && (!needAgent || t.AgentIdentity != ""), nil
}

// check reports whether a stored session can take a prompt, and returns it
// with the terminal that runs it now:
//   - the terminal runs exactly one Claude process with the stored Claude
//     session (or the stored row has no Claude session yet: adopt it);
//   - else exactly one other terminal runs the stored Claude session: follow it;
//   - when the Claude sessions cannot be read, Orca must report an agent.
func (o *Orca) check(ctx context.Context, ss inbox.Session) (inbox.Session, bool, error) {
	o.orcaOnly = false
	if ss.Handle == "" {
		return ss, false, nil
	}
	if o.Claude != nil {
		procs, err := o.Claude.InTerminal(ss.Handle)
		o.orcaOnly = err != nil
		if err == nil {
			if len(procs) == 1 && (ss.AgentSession == "" || procs[0].SessionID == ss.AgentSession) {
				ss.AgentSession = procs[0].SessionID
				ok, err := o.orcaLive(ctx, ss.Handle, false)
				return ss, ok, err
			}
			if ss.AgentSession == "" {
				return ss, false, nil
			}
			others, err := o.Claude.WithSession(ss.AgentSession)
			if err != nil || len(others) != 1 || others[0].Handle == "" || others[0].Handle == ss.Handle {
				return ss, false, nil
			}
			ok, err := o.orcaLive(ctx, others[0].Handle, false)
			if !ok || err != nil {
				return ss, false, err
			}
			ss.Handle = others[0].Handle
			return ss, true, nil
		}
	}
	ok, err := o.orcaLive(ctx, ss.Handle, true)
	return ss, ok, err
}

// claudeProc returns the Claude process of a terminal that i2a started, or
// a zero ClaudeProc when it is not known. Claude writes its file while it
// starts, so claudeProc tries for up to 5 seconds.
func (o *Orca) claudeProc(handle string) ClaudeProc {
	if o.Claude == nil {
		return ClaudeProc{}
	}
	for i := 0; i < 5; i++ {
		procs, err := o.Claude.InTerminal(handle)
		if err != nil {
			return ClaudeProc{}
		}
		if len(procs) == 1 {
			return procs[0]
		}
		o.Sleep(time.Second)
	}
	return ClaudeProc{}
}

// touch records a prompt sent to the terminal: the time and the current
// name of its Claude session. The store changes only when the terminal is
// the stored session of the space. It returns a note for the detail.
func (o *Orca) touch(ctx context.Context, handle string) string {
	name := ""
	if o.Claude != nil {
		if procs, err := o.Claude.InTerminal(handle); err == nil && len(procs) == 1 {
			name = procs[0].Name
		}
	}
	if err := o.sessions.TouchSession(ctx, o.space.ID, handle, name, o.Now()); err != nil {
		return "; last send not stored: " + err.Error()
	}
	return ""
}

// forget deletes the stored session of the space when it is handle.
func (o *Orca) forget(ctx context.Context, handle string) {
	if ss, err := o.sessions.Session(ctx, o.space.ID); err == nil && ss.Handle == handle {
		_ = o.sessions.DeleteSession(ctx, o.space.ID)
	}
}

func (o *Orca) reuse(ctx context.Context, b *render.Rendered) (Result, error) {
	ss, err := o.sessions.Session(ctx, o.space.ID)
	if err != nil {
		return Result{}, err
	}
	stored := ss
	// Each prompt has the instruction and the CLI instruction, so any live
	// session of the space can take it: no prompt change starts a new session.
	if ss.Handle != "" {
		cur, ok, err := o.check(ctx, ss)
		if err != nil {
			return Result{}, err
		}
		if !ok {
			ss.Handle = ""
		} else if cur != ss {
			// Adopted the Claude session of the terminal, or followed the
			// session to another terminal.
			if err := o.sessions.SetSession(ctx, cur); err != nil {
				return Result{}, err
			}
			ss = cur
		}
	}
	if ss.Handle == "" {
		return o.startSession(ctx, b, "", o.resumable(stored))
	}
	res, err := o.sendIdle(ctx, ss.Handle, ss.AgentSession, o.IdleTimeout, b.Prompt)
	if errors.Is(err, errSessionGone) {
		return o.startSession(ctx, b, "Claude session ended, ", o.resumable(ss))
	}
	if err == nil && o.orcaOnly {
		res.Detail += "; " + ErrClaudeUnknown.Error() + ", Orca check only"
	}
	if err == nil && res.Status == StatusSent {
		res.Detail += o.touch(ctx, ss.Handle)
	}
	return res, err
}

// startSession launches a new terminal, resumes the Claude session resume
// in it when resume is not "", sends the prompt, and stores the session for
// the space. why starts the detail.
func (o *Orca) startSession(ctx context.Context, b *render.Rendered, why, resume string) (Result, error) {
	h, note, err := o.launch(ctx)
	if err != nil {
		return Result{}, err
	}
	op, detail, err := o.submit(ctx, h, resume, b.Prompt)
	if err != nil {
		return Result{}, err
	}
	// Store the handle only after the prompt arrived: a terminal that never
	// took a prompt is not the session of the space.
	now := o.Now()
	ss := inbox.Session{Space: o.space.ID, Handle: h, AgentSession: op.session, Name: op.name, CreatedAt: now, LastSent: now}
	if err := o.sessions.SetSession(ctx, ss); err != nil {
		return Result{}, err
	}
	what := op.prefix + "started session " + h
	if op.resumed {
		what = "resumed Claude session " + shortID(op.session) + " in " + h
	}
	return Result{Status: StatusSent, Detail: why + what + ", " + detail + op.note + note, Handle: h}, nil
}

// sendIdle waits up to wait for the agent in a live terminal to be idle, then
// sends the prompt. A busy agent gives held. A shell is idle too, so when
// session is known, sendIdle checks again that the terminal runs it, just
// before it types: else errSessionGone. When no agent runs in the terminal,
// the stored session of the space is deleted.
func (o *Orca) sendIdle(ctx context.Context, handle, session string, wait time.Duration, prompt string) (Result, error) {
	err := o.call(ctx, nil, "terminal", "wait", "--terminal", handle, "--for", "tui-idle",
		"--timeout-ms", fmt.Sprint(wait.Milliseconds()))
	if isCode(err, "timeout") {
		return Result{Status: StatusHeld, Detail: "agent busy in " + handle}, nil
	}
	if err != nil {
		return Result{}, err
	}
	if o.Claude != nil && session != "" {
		procs, err := o.Claude.InTerminal(handle)
		if err == nil && (len(procs) != 1 || procs[0].SessionID != session) {
			return Result{}, fmt.Errorf("%w %s", errSessionGone, handle)
		}
	}
	detail, err := o.send(ctx, handle, prompt)
	if errors.Is(err, errNoAgent) {
		o.forget(ctx, handle)
	}
	if err != nil {
		return Result{}, err
	}
	return Result{Status: StatusSent, Detail: "sent to " + handle + ", " + detail, Handle: handle}, nil
}

// Ask sends an ask to the terminal of the card, when that terminal (or the
// terminal that runs its Claude session now) can take it. Otherwise the ask
// goes to the agent of the space as a batch does: the reuse session, or a new
// session or worktree.
func (o *Orca) Ask(ctx context.Context, t Terminal, wait time.Duration, a *render.Rendered) (Result, error) {
	if t.Handle != "" {
		cur, ok, err := o.check(ctx, inbox.Session{Handle: t.Handle, AgentSession: t.AgentSession})
		if err != nil {
			return Result{}, err
		}
		if ok {
			res, err := o.sendIdle(ctx, cur.Handle, cur.AgentSession, wait, a.Prompt)
			if err == nil && res.Status == StatusSent {
				res.Detail += o.touch(ctx, cur.Handle)
			}
			if !errors.Is(err, errSessionGone) {
				return res, err
			}
		}
	}
	saved := o.IdleTimeout
	o.IdleTimeout = wait
	defer func() { o.IdleTimeout = saved }()
	return o.Deliver(ctx, a)
}

func (o *Orca) worktree(ctx context.Context, b *render.Rendered) (Result, error) {
	ws, err := o.workspace()
	if err != nil {
		return Result{}, err
	}
	args := []string{"worktree", "create", "--repo", "path:" + ws, "--name", "i2a-" + b.BatchID, "--no-parent", "--agent", o.p.CLI}
	if o.p.Model != "" {
		args = append(args, "--model", o.p.Model)
	}
	var wr worktreeResult
	if err := o.call(ctx, &wr, args...); err != nil {
		return Result{}, err
	}
	if wr.Launch == nil || wr.Launch.Outcome.Handle == "" {
		return Result{}, fmt.Errorf("orca worktree create: worktree %s has no agent terminal in the result", wr.Worktree.ID)
	}
	h := wr.Launch.Outcome.Handle
	note := o.rename(ctx, h)
	op, detail, err := o.submit(ctx, h, "", b.Prompt)
	if err != nil {
		return Result{}, err
	}
	return Result{Status: StatusSent, Detail: "worktree " + wr.Worktree.ID + " session " + h + ", " + detail + op.note + note, Handle: h}, nil
}

// ResolveBin returns the absolute path of the orca executable, or an error
// that tells the user what to do.
func ResolveBin(configured string) (string, error) {
	if configured != "" {
		if _, err := os.Stat(configured); err != nil {
			return "", fmt.Errorf("orca executable %s: %w", configured, err)
		}
		return configured, nil
	}
	if p, err := exec.LookPath("orca"); err == nil {
		return filepath.Abs(p)
	}
	const appBin = "/Applications/Orca.app/Contents/Resources/bin/orca"
	if _, err := os.Stat(appBin); err == nil {
		return appBin, nil
	}
	return "", errors.New("cannot find the orca executable. Install Orca, or set orca_bin in the agent")
}
