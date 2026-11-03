// Package notify shows desktop notifications. A note goes through a chain of
// notifiers: the Orca plugin of i2a, terminal-notifier and osascript. The
// first notifier that succeeds shows it.
package notify

import (
	"context"
	"errors"
	"os"
	"runtime"
	"strings"
	"time"
)

// Note is one desktop notification. Each notifier uses the fields that it
// supports: only terminal-notifier opens Open or activates Activate on a click.
type Note struct {
	Title    string
	Subtitle string
	Body     string
	Open     string // URL to open on a click; see Click
	Activate string // bundle ID of the app to bring to the front on a click
	Group    string // a newer note of the same group replaces the older one
}

// Notifier shows one note. It never blocks the caller for long and ignores
// errors: a missing notification must not stop a dispatch.
type Notifier func(Note)

// Values of [notify] via.
const (
	ViaAuto             = "auto"
	ViaOrca             = "orca"
	ViaTerminalNotifier = "terminal-notifier"
	ViaOsascript        = "osascript"
)

// Vias lists the valid values of [notify] via.
var Vias = []string{ViaAuto, ViaOrca, ViaTerminalNotifier, ViaOsascript}

// Sender is one notifier of the chain.
type Sender interface {
	Name() string
	Send(ctx context.Context, n Note) error
}

// Timeout is the longest time one notifier can take.
const Timeout = 5 * time.Second

// ErrNone means that no notifier of the chain showed the note.
var ErrNone = errors.New("no notifier showed the note")

// Chain is the ordered list of notifiers.
type Chain []Sender

// Send tries each notifier in order and returns the name of the first one
// that showed the note.
func (c Chain) Send(ctx context.Context, n Note) (string, error) {
	var errs []error
	for _, s := range c {
		sctx, cancel := context.WithTimeout(ctx, Timeout)
		err := s.Send(sctx, n)
		cancel()
		if err == nil {
			return s.Name(), nil
		}
		errs = append(errs, err)
	}
	return "", errors.Join(append([]error{ErrNone}, errs...)...)
}

// Options select the notifiers of the chain.
type Options struct {
	Via     string // a value of Vias; "" is ViaAuto
	DataDir string // holds icon.png for terminal-notifier
}

// NewChain returns the chain for o. A selected notifier falls back to
// osascript, so a note is not lost.
func NewChain(o Options) Chain {
	osa := Osascript{}
	tn := TerminalNotifier{DataDir: o.DataDir}
	switch o.Via {
	case ViaOrca:
		return Chain{Orca{}, osa}
	case ViaTerminalNotifier:
		return Chain{tn, osa}
	case ViaOsascript:
		return Chain{osa}
	}
	return Chain{Orca{}, tn, osa}
}

// Off reports whether notifications are off for this process: on a platform
// other than macOS, or with I2A_NOTIFY=off (the e2e harness sets it).
func Off() bool {
	return runtime.GOOS != "darwin" || os.Getenv("I2A_NOTIFY") == "off"
}

// Desktop returns the notifier of this platform for o. It does nothing when
// Off reports true.
func Desktop(o Options) Notifier {
	if Off() {
		return func(Note) {}
	}
	c := NewChain(o)
	return func(n Note) { _, _ = c.Send(context.Background(), n) }
}

// Short cuts s to n runes and adds "..." when it cuts.
func Short(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "..."
}

// Clean removes control characters and joins the lines with one space, so a
// text cannot end an escape sequence or add lines to a notification.
func Clean(s string) string {
	s = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f || (r >= 0x80 && r < 0xa0) {
			return ' '
		}
		return r
	}, s)
	return strings.Join(strings.Fields(s), " ")
}
