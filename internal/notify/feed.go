package notify

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

// FeedLockFile is the lock that the Feed holds while it shows the card
// notifications in its terminal. "i2a card add" then shows none.
const FeedLockFile = "feed-notify.lock"

// HoldFeedLock takes <dir>/feed-notify.lock without waiting. It returns the
// release function, or false when another process holds the lock.
func HoldFeedLock(dir string) (func(), bool) {
	f, err := os.OpenFile(filepath.Join(dir, FeedLockFile), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, false
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		return nil, false
	}
	return func() {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		f.Close()
	}, true
}

// FeedShows reports whether a Feed holds <dir>/feed-notify.lock.
func FeedShows(dir string) bool {
	f, err := os.OpenFile(filepath.Join(dir, FeedLockFile), os.O_RDONLY, 0)
	if err != nil {
		return false // no lock file: no Feed took it
	}
	defer f.Close()
	err = syscall.Flock(int(f.Fd()), syscall.LOCK_SH|syscall.LOCK_NB)
	if err == nil {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		return false
	}
	return errors.Is(err, syscall.EWOULDBLOCK)
}

// Env reads one environment variable. Tests replace it.
type Env func(key string) string

// ITerm2 reports whether this process writes to iTerm2: TERM_PROGRAM is
// iTerm.app, or LC_TERMINAL is iTerm2 (iTerm2 sets it, and it passes tmux).
func ITerm2(env Env) bool {
	return env("TERM_PROGRAM") == "iTerm.app" || env("LC_TERMINAL") == "iTerm2"
}

// InTmux reports whether this process runs inside tmux.
func InTmux(env Env) bool { return env("TMUX") != "" }

// TmuxPassthrough reports whether tmux passes escape sequences to the outer
// terminal: allow-passthrough is on or all.
func TmuxPassthrough(ctx context.Context) bool {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "tmux", "show-options", "-gv", "allow-passthrough").Output()
	if err != nil {
		return false
	}
	v := strings.TrimSpace(string(out))
	return v == "on" || v == "all"
}

// OSC9Text returns the text of a card note for OSC 9: iTerm2 shows only a
// body, so the title, the subtitle and the body go in one line.
func OSC9Text(n Note) string {
	head := Clean(n.Title)
	if s := Clean(n.Subtitle); s != "" {
		head += " · " + s
	}
	if b := Clean(n.Body); b != "" {
		return head + ": " + b
	}
	return head
}

// OSC9 returns the escape sequence that shows text as a notification in
// iTerm2. In tmux the sequence goes in the passthrough, with each ESC doubled.
func OSC9(text string, tmux bool) string {
	seq := "\x1b]9;" + Clean(text) + "\x07"
	if !tmux {
		return seq
	}
	return "\x1bPtmux;" + strings.ReplaceAll(seq, "\x1b", "\x1b\x1b") + "\x1b\\"
}
