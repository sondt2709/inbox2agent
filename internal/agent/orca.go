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
	p            config.Profile
	sessions     Sessions
	Run          Runner
	Now          func() time.Time
	IdleTimeout  time.Duration // wait for a busy agent in reuse mode
	StartTimeout time.Duration // wait for a new agent to be ready
	SubmitWait   time.Duration
}

// NewOrca returns the orca driver of a profile.
func NewOrca(p config.Profile, sessions Sessions) *Orca {
	return &Orca{p: p, sessions: sessions, Run: ExecRunner, Now: time.Now,
		IdleTimeout: 5 * time.Second, StartTimeout: 90 * time.Second, SubmitWait: 30 * time.Second}
}

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
		Handle    string          `json:"handle"`
		Connected bool            `json:"connected"`
		Writable  bool            `json:"writable"`
		ExitCause json.RawMessage `json:"exitCause"`
	} `json:"terminal"`
}

type sendResult struct {
	Send struct {
		Accepted bool `json:"accepted"`
		Prompt   struct {
			Stages []string `json:"stages"`
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

// workspace returns the absolute workspace path.
func (o *Orca) workspace() (string, error) {
	ws := o.p.Workspace
	if strings.HasPrefix(ws, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		ws = filepath.Join(home, ws[2:])
	}
	if !filepath.IsAbs(ws) {
		return "", fmt.Errorf("profile %q: workspace %q is not an absolute path", o.p.Name, o.p.Workspace)
	}
	return filepath.Clean(ws), nil
}

// Deliver sends the batch in the session mode of the profile.
func (o *Orca) Deliver(ctx context.Context, b *render.Rendered) (Result, error) {
	switch o.p.Session {
	case config.SessionNew:
		h, err := o.launch(ctx)
		if err != nil {
			return Result{}, err
		}
		detail, err := o.submit(ctx, h, joinPrompts(b))
		if err != nil {
			return Result{}, err
		}
		return Result{Status: StatusSent, Detail: "new session " + h + ", " + detail}, nil
	case config.SessionWorktree:
		return o.worktree(ctx, b)
	default:
		return o.reuse(ctx, b)
	}
}

func joinPrompts(b *render.Rendered) string {
	return strings.TrimSpace(b.Primer) + "\n\n" + strings.TrimSpace(b.Prompt)
}

// launchArgs starts an agent without --prompt. With --prompt, Orca types the
// prompt while the agent starts, and the prompt can stay in the input box
// without Enter. The driver submits the prompt itself (see submit).
func (o *Orca) launchArgs(ws string) []string {
	args := []string{"agent", "launch", "--agent", o.p.Agent, "--surface", "terminal", "--worktree", "path:" + ws}
	if o.p.Model != "" {
		args = append(args, "--model", o.p.Model)
	}
	return args
}

// launch starts a new agent terminal in the workspace and returns its handle.
// When Orca does not know the workspace and it is a git repository, launch adds
// it with "orca repo add" and tries one more time.
func (o *Orca) launch(ctx context.Context) (string, error) {
	ws, err := o.workspace()
	if err != nil {
		return "", err
	}
	var lr launchResult
	err = o.call(ctx, &lr, o.launchArgs(ws)...)
	if isCode(err, "selector_not_found") {
		if addErr := o.call(ctx, nil, "repo", "add", "--path", ws); addErr != nil {
			return "", fmt.Errorf("orca does not know the workspace %s and cannot add it (%v). Add the folder in Orca, then try again", ws, addErr)
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

// submit waits until a new agent is ready, then sends the prompt with Enter.
func (o *Orca) submit(ctx context.Context, handle, prompt string) (string, error) {
	err := o.call(ctx, nil, "terminal", "wait", "--terminal", handle, "--for", "tui-idle",
		"--timeout-ms", fmt.Sprint(o.StartTimeout.Milliseconds()))
	if isCode(err, "timeout") {
		return "", fmt.Errorf("the agent in %s was not ready after %s", handle, o.StartTimeout)
	}
	if err != nil {
		return "", err
	}
	return o.send(ctx, handle, prompt)
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
	if slices.Contains(sr.Send.Prompt.Stages, "turn_started") {
		return "turn started", nil
	}
	if err := o.call(ctx, nil, "terminal", "send", "--terminal", handle, "--enter"); err != nil {
		return "turn not proven, Enter again failed: " + err.Error(), nil
	}
	return "turn not proven, Enter sent again", nil
}

// live reports whether a stored terminal still runs.
func (o *Orca) live(ctx context.Context, handle string) (bool, error) {
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
	return t.Connected && t.Writable && !exited, nil
}

func (o *Orca) reuse(ctx context.Context, b *render.Rendered) (Result, error) {
	handle, err := o.sessions.Session(ctx, o.p.Name)
	if err != nil {
		return Result{}, err
	}
	if handle != "" {
		ok, err := o.live(ctx, handle)
		if err != nil {
			return Result{}, err
		}
		if !ok {
			handle = ""
		}
	}
	if handle == "" {
		h, err := o.launch(ctx)
		if err != nil {
			return Result{}, err
		}
		detail, err := o.submit(ctx, h, joinPrompts(b))
		if err != nil {
			return Result{}, err
		}
		// Store the handle only after the primer arrived, so a later batch never
		// goes to a session without the primer.
		if err := o.sessions.SetSession(ctx, o.p.Name, h, o.Now()); err != nil {
			return Result{}, err
		}
		return Result{Status: StatusSent, Detail: "started session " + h + " with the primer, " + detail}, nil
	}

	err = o.call(ctx, nil, "terminal", "wait", "--terminal", handle, "--for", "tui-idle",
		"--timeout-ms", fmt.Sprint(o.IdleTimeout.Milliseconds()))
	if isCode(err, "timeout") {
		return Result{Status: StatusHeld, Detail: "agent busy in " + handle}, nil
	}
	if err != nil {
		return Result{}, err
	}
	detail, err := o.send(ctx, handle, b.Prompt)
	if err != nil {
		return Result{}, err
	}
	return Result{Status: StatusSent, Detail: "sent to " + handle + ", " + detail}, nil
}

func (o *Orca) worktree(ctx context.Context, b *render.Rendered) (Result, error) {
	ws, err := o.workspace()
	if err != nil {
		return Result{}, err
	}
	args := []string{"worktree", "create", "--repo", "path:" + ws, "--name", "i2a-" + b.BatchID, "--no-parent", "--agent", o.p.Agent}
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
	detail, err := o.submit(ctx, h, joinPrompts(b))
	if err != nil {
		return Result{}, err
	}
	return Result{Status: StatusSent, Detail: "worktree " + wr.Worktree.ID + " session " + h + ", " + detail}, nil
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
	return "", errors.New("cannot find the orca executable. Install Orca, or set orca_bin in the profile")
}
