// Package harness drives the real i2a binary through a terminal, as a person does.
//
// Each Term runs in a detached tmux session on a private tmux socket, so the
// tests never attach to, or take focus from, the terminal of the developer.
package harness

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

var seq atomic.Int64

// tmuxBin returns the tmux executable: I2A_E2E_TMUX, or else the first tmux in
// PATH that is not a wrapper script from an agent tool (for example Orca's shim).
func tmuxBin() string {
	if b := os.Getenv("I2A_E2E_TMUX"); b != "" {
		return b
	}
	for _, dir := range filepath.SplitList(os.Getenv("PATH")) {
		p := filepath.Join(dir, "tmux")
		if strings.Contains(p, "/.orca/") {
			continue
		}
		if info, err := os.Stat(p); err == nil && !info.IsDir() {
			return p
		}
	}
	return "tmux"
}

// Term is one terminal window.
type Term struct {
	t       *testing.T
	socket  string
	session string
	outDir  string
	shots   int
}

// Start runs the shell command in a new 120x40 terminal. env is added to the
// environment of the command. Screen captures go to outDir.
func Start(t *testing.T, outDir, shellCmd string, env map[string]string) *Term {
	t.Helper()
	n := seq.Add(1)
	term := &Term{
		t:       t,
		socket:  fmt.Sprintf("i2a-e2e-%d", os.Getpid()),
		session: fmt.Sprintf("s%d", n),
		outDir:  outDir,
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatal(err)
	}
	var exports []string
	for k, v := range env {
		exports = append(exports, fmt.Sprintf("export %s=%s;", k, shellQuote(v)))
	}
	// Keep the window open after the command exits, so the test can read the last screen.
	script := strings.Join(exports, " ") + " " + shellCmd + "; code=$?; echo; echo \"[exit $code]\"; sleep 3600"
	term.tmux("new-session", "-d", "-s", term.session, "-x", "120", "-y", "40", "sh", "-c", script)
	t.Cleanup(term.Close)
	return term
}

func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

func (term *Term) tmux(args ...string) string {
	term.t.Helper()
	out, err := exec.Command(tmuxBin(), append([]string{"-L", term.socket}, args...)...).CombinedOutput()
	if err != nil {
		term.t.Fatalf("tmux %v: %v: %s", args, err, out)
	}
	return string(out)
}

// Close ends the session.
func (term *Term) Close() {
	_ = exec.Command(tmuxBin(), "-L", term.socket, "kill-session", "-t", term.session).Run()
}

// Keys sends tmux key names, for example "Enter", "Down", "Space", "C-u", "Escape".
func (term *Term) Keys(keys ...string) {
	term.t.Helper()
	for _, k := range keys {
		term.tmux("send-keys", "-t", term.session, k)
		time.Sleep(60 * time.Millisecond)
	}
}

// Type types the text as literal characters.
func (term *Term) Type(text string) {
	term.t.Helper()
	term.tmux("send-keys", "-t", term.session, "-l", text)
	time.Sleep(80 * time.Millisecond)
}

// Screen returns the visible text of the terminal.
func (term *Term) Screen() string {
	term.t.Helper()
	return term.tmux("capture-pane", "-p", "-t", term.session)
}

// WaitFor waits until the screen matches the pattern, then saves a capture.
// It fails the test after the timeout and saves the last screen.
func (term *Term) WaitFor(pattern string, timeout time.Duration) string {
	term.t.Helper()
	re := regexp.MustCompile(pattern)
	deadline := time.Now().Add(timeout)
	for {
		s := term.Screen()
		if re.MatchString(s) {
			term.Shot("wait " + pattern)
			return s
		}
		if time.Now().After(deadline) {
			path := term.Shot("TIMEOUT " + pattern)
			term.t.Fatalf("screen did not show %q in %s. Last screen (%s):\n%s", pattern, timeout, path, s)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

var unsafe = regexp.MustCompile(`[^a-zA-Z0-9]+`)

// Shot saves the screen to the output directory and returns the file path.
func (term *Term) Shot(label string) string {
	term.t.Helper()
	term.shots++
	name := fmt.Sprintf("%02d-%s-%s.txt", term.shots, term.session, strings.Trim(unsafe.ReplaceAllString(label, "-"), "-"))
	if len(name) > 90 {
		name = name[:90] + ".txt"
	}
	path := filepath.Join(term.outDir, name)
	_ = os.WriteFile(path, []byte(term.Screen()), 0o644)
	return path
}

// Press sends one key and waits for the pattern. If the screen does not change
// in one second, the key was lost (this can happen while the program starts),
// so Press sends it again, at most 3 times. It never repeats a key that changed the screen.
func (term *Term) Press(key, pattern string, timeout time.Duration) string {
	term.t.Helper()
	re := regexp.MustCompile(pattern)
	for attempt := 0; attempt < 3; attempt++ {
		before := term.Screen()
		term.Keys(key)
		deadline := time.Now().Add(time.Second)
		for time.Now().Before(deadline) {
			s := term.Screen()
			if re.MatchString(s) {
				term.Shot("press " + key + " " + pattern)
				return s
			}
			if s != before {
				return term.WaitFor(pattern, timeout)
			}
			time.Sleep(50 * time.Millisecond)
		}
	}
	return term.WaitFor(pattern, timeout)
}

// cursorLine matches the focused option of a Huh select: "> text".
var cursorLine = regexp.MustCompile(`(?m)^\s*[>❯┃│]?\s*> (.*)$`)

// Choose moves the cursor of a select list to the first option that starts
// with the text, presses Enter, and waits for the pattern. The option must
// start with the text, because other text (a side panel) can be on the same line.
func (term *Term) Choose(text, pattern string) string {
	term.t.Helper()
	for i := 0; i < 40; i++ {
		s := term.Screen()
		if m := cursorLine.FindStringSubmatch(s); m != nil && strings.HasPrefix(m[1], text) {
			return term.Press("Enter", pattern, 10*time.Second)
		}
		term.Keys("Down")
		time.Sleep(100 * time.Millisecond)
	}
	path := term.Shot("choose " + text)
	term.t.Fatalf("no option %q in the list (%s):\n%s", text, path, term.Screen())
	return ""
}

// Replace clears the focused text input and types a new value.
func (term *Term) Replace(text string) {
	term.t.Helper()
	term.Keys("End", "C-u")
	term.Type(text)
}
