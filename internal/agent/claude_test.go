package agent

import (
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

// The files in testdata/claude/sessions are real ~/.claude/sessions/<pid>.json
// files of Claude Code 2.1.318 with the IDs, names and paths replaced.

const (
	sessionA = "04ebe19c-543e-42ff-8f84-ea13cea7a3ec" // 4242.json
	sessionB = "d9c92c0a-8869-4bbb-954b-16f89a3abd8f" // 4343.json
)

// fixtureStart is procStart of each fixture file.
var fixtureStart = map[int]time.Time{
	4242: time.Date(2026, 11, 4, 11, 19, 18, 400e6, time.UTC),
	4343: time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC),
}

// fakeProcs gives the terminal handle of each running pid.
func fakeProcs(handles map[int]string) func(int) (ProcInfo, error) {
	return func(pid int) (ProcInfo, error) {
		h, ok := handles[pid]
		if !ok {
			return ProcInfo{}, ErrNoProcess
		}
		return ProcInfo{Start: fixtureStart[pid], Handle: h}, nil
	}
}

func copyDir(t *testing.T, names ...string) string {
	t.Helper()
	dir := t.TempDir()
	for _, n := range names {
		b, err := os.ReadFile(filepath.Join("testdata", "claude", "sessions", n))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, n), b, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestClaudeFinderInTerminal(t *testing.T) {
	f := &ClaudeFinder{Dir: copyDir(t, "4242.json", "4343.json"), Proc: fakeProcs(map[int]string{4242: "term_a", 4343: "term_b"})}
	got, err := f.InTerminal("term_a")
	if err != nil {
		t.Fatal(err)
	}
	want := []ClaudeProc{{PID: 4242, SessionID: sessionA, Name: "Mattermost inbox triage", Cwd: "/home/user/notes", Handle: "term_a", OrcaAgent: OrcaAgentClaude}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
	got, _ = f.WithSession(sessionB)
	if len(got) != 1 || got[0].Handle != "term_b" {
		t.Errorf("WithSession = %+v", got)
	}
}

func TestClaudeFinderNoProcess(t *testing.T) {
	// The pid of 4242.json does not run: Claude exited and the file stayed.
	f := &ClaudeFinder{Dir: copyDir(t, "4242.json"), Proc: fakeProcs(nil)}
	got, err := f.InTerminal("term_a")
	if err != nil || len(got) != 0 {
		t.Errorf("got %+v, %v; want none", got, err)
	}
}

func TestClaudeFinderPidOfAnotherProcess(t *testing.T) {
	// The OS gave the pid of an old file to a process that started later.
	f := &ClaudeFinder{Dir: copyDir(t, "4242.json"), Proc: func(int) (ProcInfo, error) {
		return ProcInfo{Start: fixtureStart[4242].Add(time.Hour), Handle: "term_a"}, nil
	}}
	if got, _ := f.InTerminal("term_a"); len(got) != 0 {
		t.Errorf("got %+v; want none", got)
	}
}

func TestClaudeFinderOneSessionInTwoTerminals(t *testing.T) {
	dir := copyDir(t, "4242.json")
	b, _ := os.ReadFile(filepath.Join(dir, "4242.json"))
	if err := os.WriteFile(filepath.Join(dir, "4343.json"), []byte(strings.ReplaceAll(string(b), `"pid":4242`, `"pid":4343`)), 0o600); err != nil {
		t.Fatal(err)
	}
	f := &ClaudeFinder{Dir: dir, Proc: func(pid int) (ProcInfo, error) {
		return ProcInfo{Start: fixtureStart[4242], Handle: map[int]string{4242: "term_b", 4343: "term_c"}[pid]}, nil
	}}
	got, err := f.WithSession(sessionA)
	if err != nil || len(got) != 2 {
		t.Errorf("got %+v, %v; want 2 processes", got, err)
	}
}

func TestClaudeFinderUnknownFormat(t *testing.T) {
	dir := copyDir(t, "4242.json")
	if err := os.WriteFile(filepath.Join(dir, "5000.json"), []byte(`{"process": 5000}`), 0o600); err != nil {
		t.Fatal(err)
	}
	f := &ClaudeFinder{Dir: dir, Proc: fakeProcs(map[int]string{4242: "term_a"})}
	if _, err := f.InTerminal("term_a"); !errors.Is(err, ErrClaudeUnknown) {
		t.Errorf("err = %v, want ErrClaudeUnknown", err)
	}
	f.Dir = filepath.Join(dir, "missing")
	if _, err := f.All(); !errors.Is(err, ErrClaudeUnknown) {
		t.Errorf("missing folder: err = %v, want ErrClaudeUnknown", err)
	}
}

func TestNewClaudeFinderUsesConfigDir(t *testing.T) {
	t.Setenv("CLAUDE_CONFIG_DIR", "/home/user/claude-work")
	if f := NewClaudeFinder(); f.Dir != "/home/user/claude-work/sessions" {
		t.Errorf("Dir = %q", f.Dir)
	}
}

func TestParseProcArgs(t *testing.T) {
	b := make([]byte, 4)
	binary.LittleEndian.PutUint32(b, 2)
	b = append(b, "/usr/local/bin/claude\x00\x00\x00\x00claude\x00--resume\x00HOME=/home/user\x00ORCA_TERMINAL_HANDLE=term_a\x00\x00junk"...)
	env := parseProcArgs(b)
	if want := []string{"HOME=/home/user", "ORCA_TERMINAL_HANDLE=term_a"}; !reflect.DeepEqual(env, want) {
		t.Errorf("env = %q, want %q", env, want)
	}
	if envValue(env, "ORCA_TERMINAL_HANDLE") != "term_a" {
		t.Error("no handle")
	}
	// An argument that looks like the variable is not the environment.
	b2 := make([]byte, 4)
	binary.LittleEndian.PutUint32(b2, 2)
	b2 = append(b2, "/bin/sh\x00sh\x00ORCA_TERMINAL_HANDLE=term_x\x00\x00"...)
	if v := envValue(parseProcArgs(b2), "ORCA_TERMINAL_HANDLE"); v != "" {
		t.Errorf("handle from an argument: %q", v)
	}
}

func TestIsClaude(t *testing.T) {
	for cli, want := range map[string]bool{"claude": true, "/opt/bin/claude": true, "claude-agent-teams": true, "codex": false, "opencode": false} {
		if IsClaude(cli) != want {
			t.Errorf("IsClaude(%q) = %v", cli, !want)
		}
	}
}

func TestClaudeFinderIgnoresBackgroundProcess(t *testing.T) {
	// 4444-bg.json is a real file of a process after "Move to background and
	// exit": it keeps the handle of the terminal, but the terminal shows zsh.
	f := &ClaudeFinder{Dir: copyDir(t, "4444-bg.json"), Proc: func(int) (ProcInfo, error) {
		return ProcInfo{Start: time.Date(2026, 11, 4, 14, 54, 46, 0, time.UTC), Handle: "term_a"}, nil
	}}
	if got, err := f.InTerminal("term_a"); err != nil || len(got) != 0 {
		t.Errorf("InTerminal = %+v, %v; want no process", got, err)
	}
	if got, _ := f.WithSession("b3847aa1-e42b-424e-9b77-4aeff9a68086"); len(got) != 0 {
		t.Errorf("WithSession = %+v; want no process", got)
	}
}
