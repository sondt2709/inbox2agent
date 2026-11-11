package agent

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ClaudeProc is one running Claude Code process.
type ClaudeProc struct {
	PID       int
	SessionID string // the Claude session ID; /clear and /resume change it
	Name      string // the name of the Claude session
	Cwd       string
	Handle    string // ORCA_TERMINAL_HANDLE of the process; "" when it does not run in Orca
	OrcaAgent string // the Orca agent that started it (OrcaAgentClaude, OrcaAgentTeams); "" outside Orca
}

// The Orca agents that run Claude Code. Orca starts claude-agent-teams as
// "claude --teammate-mode auto …" with ORCA_AGENT_TEAMS_TEAM_ID set, and
// claude as "claude --plugin-dir <Orca agent view> …" (Orca 1.4.253).
// agentIdentity of "orca terminal show" does not tell them apart.
const (
	OrcaAgentClaude = "claude"
	OrcaAgentTeams  = "claude-agent-teams"
)

// ErrClaudeUnknown means that i2a cannot read the Claude sessions: the
// folder is missing, a file does not decode, or the OS is not supported.
// The driver then uses the Orca check only.
var ErrClaudeUnknown = errors.New("cannot read the Claude sessions")

// ErrNoProcess means that the pid does not run, or another user owns it.
var ErrNoProcess = errors.New("no such process")

// ProcInfo is what the OS tells about a pid.
type ProcInfo struct {
	Start  time.Time
	Handle string // ORCA_TERMINAL_HANDLE from the environment of the process
	Teams  bool   // ORCA_AGENT_TEAMS_TEAM_ID is set: Orca started it as claude-agent-teams
}

// ClaudeFinder finds the running Claude Code processes. Claude Code writes
// <claude dir>/sessions/<pid>.json for each interactive process, keeps
// sessionId current after /clear and /resume, and deletes the file at exit.
// The process inherits ORCA_TERMINAL_HANDLE from its Orca terminal.
type ClaudeFinder struct {
	Dir  string                          // the sessions folder
	Proc func(pid int) (ProcInfo, error) // ErrNoProcess for a pid that does not run
}

// NewClaudeFinder returns the finder of the Claude config folder of the user:
// CLAUDE_CONFIG_DIR, else ~/.claude.
func NewClaudeFinder() *ClaudeFinder {
	dir := os.Getenv("CLAUDE_CONFIG_DIR")
	if dir == "" {
		home, _ := os.UserHomeDir()
		dir = filepath.Join(home, ".claude")
	}
	return &ClaudeFinder{Dir: filepath.Join(dir, "sessions"), Proc: readProc}
}

type claudeFile struct {
	PID       int    `json:"pid"`
	SessionID string `json:"sessionId"`
	Cwd       string `json:"cwd"`
	Name      string `json:"name"`
	ProcStart string `json:"procStart"` // time.ANSIC in UTC, the start of the process
	Kind      string `json:"kind"`      // "interactive" for the TUI; "bg" after "Move to background and exit"
}

// All returns the running interactive Claude processes. A background
// process ("kind": "bg") keeps ORCA_TERMINAL_HANDLE of its terminal, but the
// terminal shows the shell, so it does not count.
func (f *ClaudeFinder) All() ([]ClaudeProc, error) {
	if f == nil {
		return nil, ErrClaudeUnknown
	}
	files, err := filepath.Glob(filepath.Join(f.Dir, "*.json"))
	if err != nil {
		return nil, ErrClaudeUnknown
	}
	if _, err := os.Stat(f.Dir); err != nil {
		return nil, ErrClaudeUnknown
	}
	var out []ClaudeProc
	for _, name := range files {
		b, err := os.ReadFile(name)
		if errors.Is(err, os.ErrNotExist) {
			continue // the process exited after the glob
		}
		var c claudeFile
		if err != nil || json.Unmarshal(b, &c) != nil || c.PID <= 0 || c.SessionID == "" {
			return nil, ErrClaudeUnknown
		}
		p, err := f.Proc(c.PID)
		if errors.Is(err, ErrNoProcess) {
			continue
		}
		if err != nil {
			return nil, ErrClaudeUnknown
		}
		if !sameStart(c.ProcStart, p.Start) {
			continue // the file of an old process, and the OS gave its pid to another one
		}
		if c.Kind != "" && c.Kind != "interactive" {
			continue
		}
		agent := ""
		switch {
		case p.Teams:
			agent = OrcaAgentTeams
		case p.Handle != "":
			agent = OrcaAgentClaude
		}
		out = append(out, ClaudeProc{PID: c.PID, SessionID: c.SessionID, Name: c.Name, Cwd: c.Cwd, Handle: p.Handle, OrcaAgent: agent})
	}
	return out, nil
}

// sameStart reports whether procStart names the start of the process. An
// empty or unknown value is not a reason to drop the process.
func sameStart(procStart string, start time.Time) bool {
	t, err := time.ParseInLocation(time.ANSIC, procStart, time.UTC)
	if err != nil || start.IsZero() {
		return true
	}
	d := t.Sub(start.UTC().Truncate(time.Second))
	return d < 2*time.Second && d > -2*time.Second
}

// InTerminal returns the Claude processes of the Orca terminal handle.
func (f *ClaudeFinder) InTerminal(handle string) ([]ClaudeProc, error) {
	return f.filter(func(p ClaudeProc) bool { return p.Handle == handle })
}

// WithSession returns the Claude processes of the Claude session id.
func (f *ClaudeFinder) WithSession(id string) ([]ClaudeProc, error) {
	return f.filter(func(p ClaudeProc) bool { return p.SessionID == id })
}

func (f *ClaudeFinder) filter(keep func(ClaudeProc) bool) ([]ClaudeProc, error) {
	all, err := f.All()
	if err != nil {
		return nil, err
	}
	var out []ClaudeProc
	for _, p := range all {
		if keep(p) {
			out = append(out, p)
		}
	}
	return out, nil
}

// IsClaude reports whether the cli of an agent is Claude Code, so the
// driver can check its sessions.
func IsClaude(cli string) bool {
	return strings.HasPrefix(filepath.Base(strings.TrimSpace(cli)), "claude")
}

// envValue returns the value of key in NUL-separated environment strings.
func envValue(env []string, key string) string {
	for _, kv := range env {
		if v, ok := strings.CutPrefix(kv, key+"="); ok {
			return v
		}
	}
	return ""
}

// parseProcArgs returns the environment strings of a kern.procargs2 buffer:
// argc (int32), the executable path, NUL padding, argc arguments, then the
// environment, each string ending with NUL.
func parseProcArgs(b []byte) []string {
	if len(b) < 4 {
		return nil
	}
	argc := int(binary.LittleEndian.Uint32(b[:4]))
	rest := b[4:]
	i := bytes.IndexByte(rest, 0)
	if i < 0 {
		return nil
	}
	rest = bytes.TrimLeft(rest[i:], "\x00")
	fields := bytes.Split(rest, []byte{0})
	if len(fields) < argc {
		return nil
	}
	var env []string
	for _, f := range fields[argc:] {
		if len(f) == 0 {
			break
		}
		env = append(env, string(f))
	}
	return env
}
