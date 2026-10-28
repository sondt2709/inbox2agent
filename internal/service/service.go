// Package service installs i2a as a macOS LaunchAgent.
package service

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
)

// Label is the LaunchAgent label.
const Label = "io.github.sondt2709.i2a"

// ErrUnsupported is returned on platforms without LaunchAgents.
var ErrUnsupported = errors.New("not supported on this platform")

// basePath is the PATH of the service before the extra directories.
const basePath = "/usr/bin:/bin:/usr/sbin:/sbin:/usr/local/bin:/opt/homebrew/bin"

// passEnv are the variables that the service copies from the install environment.
var passEnv = []string{"I2A_HOME", "XDG_CONFIG_HOME", "XDG_DATA_HOME", "I2A_SECRET_BACKEND"}

// Spec describes the LaunchAgent.
type Spec struct {
	Program  string
	LogFile  string
	ExtraDir []string // directories to add to PATH, for example the orca directory
	Env      map[string]string
}

// PlistPath returns the LaunchAgent file path.
func PlistPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "Library", "LaunchAgents", Label+".plist"), nil
}

// NewSpec makes the spec for the running i2a binary.
func NewSpec(logFile string, extraDirs ...string) (Spec, error) {
	exe, err := os.Executable()
	if err != nil {
		return Spec{}, err
	}
	if exe, err = filepath.EvalSymlinks(exe); err != nil {
		return Spec{}, err
	}
	env := map[string]string{}
	for _, k := range passEnv {
		if v := os.Getenv(k); v != "" {
			env[k] = v
		}
	}
	return Spec{Program: exe, LogFile: logFile, ExtraDir: extraDirs, Env: env}, nil
}

func esc(s string) string {
	var b bytes.Buffer
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}

// Plist returns the LaunchAgent file content.
func Plist(s Spec) string {
	path := basePath
	for _, d := range s.ExtraDir {
		if d != "" && !strings.Contains(":"+path+":", ":"+d+":") {
			path = d + ":" + path
		}
	}
	env := map[string]string{"PATH": path}
	for k, v := range s.Env {
		env[k] = v
	}
	keys := make([]string, 0, len(env))
	for k := range env {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key>
  <string>` + Label + `</string>
  <key>ProgramArguments</key>
  <array>
    <string>` + esc(s.Program) + `</string>
    <string>daemon</string>
  </array>
  <key>RunAtLoad</key>
  <true/>
  <key>KeepAlive</key>
  <true/>
  <key>ThrottleInterval</key>
  <integer>30</integer>
  <key>ProcessType</key>
  <string>Background</string>
  <key>StandardOutPath</key>
  <string>` + esc(s.LogFile) + `</string>
  <key>StandardErrorPath</key>
  <string>` + esc(s.LogFile) + `</string>
  <key>EnvironmentVariables</key>
  <dict>
`)
	for _, k := range keys {
		b.WriteString("    <key>" + esc(k) + "</key>\n    <string>" + esc(env[k]) + "</string>\n")
	}
	b.WriteString("  </dict>\n</dict>\n</plist>\n")
	return b.String()
}

func domain() string { return fmt.Sprintf("gui/%d", os.Getuid()) }

func launchctl(args ...string) (string, error) {
	out, err := exec.Command("launchctl", args...).CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

// Install writes the LaunchAgent file and loads it. It replaces a loaded agent.
func Install(s Spec) (string, error) {
	if runtime.GOOS != "darwin" {
		return "", ErrUnsupported
	}
	path, err := PlistPath()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(s.LogFile), 0o700); err != nil {
		return "", err
	}
	_, _ = launchctl("bootout", domain()+"/"+Label) // not loaded is fine
	if err := os.WriteFile(path, []byte(Plist(s)), 0o644); err != nil {
		return "", err
	}
	if out, err := launchctl("bootstrap", domain(), path); err != nil {
		return "", fmt.Errorf("launchctl bootstrap: %s: %w", out, err)
	}
	return path, nil
}

// Uninstall unloads the LaunchAgent and deletes its file.
func Uninstall() (string, error) {
	if runtime.GOOS != "darwin" {
		return "", ErrUnsupported
	}
	path, err := PlistPath()
	if err != nil {
		return "", err
	}
	_, _ = launchctl("bootout", domain()+"/"+Label)
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	return path, nil
}

// State is the state of the LaunchAgent.
type State struct {
	Loaded bool
	State  string // for example "running"
	PID    string
}

var (
	stateRe = regexp.MustCompile(`(?m)^\s*state = (\S+)`)
	pidRe   = regexp.MustCompile(`(?m)^\s*pid = (\d+)`)
)

// Status reads the LaunchAgent state with launchctl print.
func Status() (State, error) {
	if runtime.GOOS != "darwin" {
		return State{}, ErrUnsupported
	}
	out, err := launchctl("print", domain()+"/"+Label)
	if err != nil {
		return State{}, nil // not loaded
	}
	return ParsePrint(out), nil
}

// ParsePrint reads the state and the process ID from launchctl print output.
func ParsePrint(out string) State {
	st := State{Loaded: true}
	if m := stateRe.FindStringSubmatch(out); m != nil {
		st.State = m[1]
	}
	if m := pidRe.FindStringSubmatch(out); m != nil {
		st.PID = m[1]
	}
	return st
}
