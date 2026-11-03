package notify

import (
	"bytes"
	"context"
	_ "embed"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

//go:embed assets/icon.png
var iconPNG []byte

// IconPNG returns the i2a icon (256 × 256 PNG).
func IconPNG() []byte { return iconPNG }

// tnDirs are the folders where Homebrew installs terminal-notifier. The
// LaunchAgent can have a short PATH, so the lookup also tries them.
var tnDirs = []string{"/opt/homebrew/bin", "/usr/local/bin"}

// TerminalNotifier shows a note with terminal-notifier (Homebrew). It shows
// the i2a icon as an image and opens the source of the note on a click.
type TerminalNotifier struct {
	DataDir string // holds icon.png
}

// Name implements Sender.
func (TerminalNotifier) Name() string { return ViaTerminalNotifier }

// FindTerminalNotifier returns the path of terminal-notifier, or "".
func FindTerminalNotifier() string {
	if p, err := exec.LookPath("terminal-notifier"); err == nil {
		return p
	}
	for _, d := range tnDirs {
		p := filepath.Join(d, "terminal-notifier")
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() && fi.Mode()&0o111 != 0 {
			return p
		}
	}
	return ""
}

// tnText makes a value safe for terminal-notifier: it reads a value that
// starts with "[" or "-" in a special way, so a backslash goes before it.
func tnText(s string) string {
	s = Clean(s)
	if strings.HasPrefix(s, "[") || strings.HasPrefix(s, "-") {
		return `\` + s
	}
	return s
}

// Args returns the terminal-notifier arguments for one note. icon is the
// path of the PNG for -contentImage, or "".
func (TerminalNotifier) Args(n Note, icon string) []string {
	args := []string{"-title", tnText(n.Title), "-message", tnText(n.Body)}
	if n.Subtitle != "" {
		args = append(args, "-subtitle", tnText(n.Subtitle))
	}
	if n.Group != "" {
		args = append(args, "-group", n.Group)
	}
	if icon != "" {
		args = append(args, "-contentImage", icon)
	}
	if AllowedOpen(n.Open) {
		args = append(args, "-open", n.Open)
	} else if n.Activate != "" {
		args = append(args, "-activate", n.Activate)
	}
	return args
}

// Send implements Sender.
func (t TerminalNotifier) Send(ctx context.Context, n Note) error {
	bin := FindTerminalNotifier()
	if bin == "" {
		return errors.New("terminal-notifier is not installed")
	}
	icon, _ := WriteIcon(t.DataDir)
	return exec.CommandContext(ctx, bin, t.Args(n, icon)...).Run()
}

// WriteIcon writes the i2a icon to <dir>/icon.png when the file is missing or
// different, and returns its path.
func WriteIcon(dir string) (string, error) {
	if dir == "" {
		return "", errors.New("no data folder")
	}
	p := filepath.Join(dir, "icon.png")
	if old, err := os.ReadFile(p); err == nil && bytes.Equal(old, iconPNG) {
		return p, nil
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, iconPNG, 0o600); err != nil {
		return "", err
	}
	return p, os.Rename(tmp, p)
}
