// Package agent delivers rendered batches to AI agents through drivers.
package agent

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/sondt2709/inbox2agent/internal/config"
	"github.com/sondt2709/inbox2agent/internal/inbox"
	"github.com/sondt2709/inbox2agent/internal/render"
)

// Result statuses. They match the dispatch statuses in the store.
const (
	StatusSent = "sent"
	StatusHeld = "held"
)

// Result is the outcome of one delivery.
type Result struct {
	Status string
	Detail string
	Handle string // the terminal that got the prompt; "" when no terminal got it
}

// Driver delivers one batch or one ask. An error means that nothing was delivered.
type Driver interface {
	Deliver(ctx context.Context, b *render.Rendered) (Result, error)
	// Ask sends the prompt of an ask (render.Ask). When the terminal of the
	// card still runs its Claude session, the ask goes there, and Ask waits
	// up to wait for the agent to be idle. Otherwise the ask goes to the
	// agent of the space as a batch does, with the same wait for a busy
	// reuse session.
	Ask(ctx context.Context, t Terminal, wait time.Duration, a *render.Rendered) (Result, error)
}

// Terminal is the terminal of a card and the Claude session that wrote it.
type Terminal struct {
	Handle       string
	AgentSession string
}

// Sessions stores the session of each space: the terminal handle, its
// Claude session and the hash of the primer that the session got. Two
// spaces with the same agent have two sessions, so the data of one space
// never enters the session of another.
type Sessions interface {
	Session(ctx context.Context, space string) (inbox.Session, error)
	SetSession(ctx context.Context, s inbox.Session) error
	DeleteSession(ctx context.Context, space string) error
}

// New returns the driver of an agent for one space. The ID of the space is
// the key of the session; its name is in the tab name of a new session.
func New(p config.Agent, space config.Space, sessions Sessions) (Driver, error) {
	switch p.Driver {
	case config.DriverDryRun:
		return DryRun{}, nil
	case config.DriverOrca:
		return NewOrca(p, space, sessions), nil
	default:
		return nil, fmt.Errorf("agent %q: unknown driver %q", p.ID, p.Driver)
	}
}

// DryRun only keeps the digest file, which the renderer already wrote.
type DryRun struct{}

// Deliver returns sent without starting any process.
func (DryRun) Deliver(_ context.Context, b *render.Rendered) (Result, error) {
	return Result{Status: StatusSent, Detail: "dry-run: digest only"}, nil
}

// ErrDryRunAsk is the error of an ask to a dry-run agent: no agent reads it.
var ErrDryRunAsk = errors.New("dry-run: no agent reads asks")

// Ask returns ErrDryRunAsk.
func (DryRun) Ask(context.Context, Terminal, time.Duration, *render.Rendered) (Result, error) {
	return Result{}, ErrDryRunAsk
}
