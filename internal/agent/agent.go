// Package agent delivers rendered batches to AI agents through drivers.
package agent

import (
	"context"
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
}

// Driver delivers one batch. An error means the batch was not delivered.
type Driver interface {
	Deliver(ctx context.Context, b *render.Rendered) (Result, error)
}

// Sessions stores the terminal handle of each profile and the hash of the
// primer that the session got.
type Sessions interface {
	Session(ctx context.Context, profile string) (handle, primerHash string, err error)
	SetSession(ctx context.Context, profile, handle, primerHash string, now time.Time) error
	DeleteSession(ctx context.Context, profile string) error
}

// New returns the driver of a profile.
func New(p config.Profile, sessions Sessions) (Driver, error) {
	switch p.Driver {
	case config.DriverDryRun:
		return DryRun{}, nil
	case config.DriverOrca:
		return NewOrca(p, sessions), nil
	default:
		return nil, fmt.Errorf("profile %q: unknown driver %q", p.Name, p.Driver)
	}
}

// Route returns the profile name for a message: the channel route of its source,
// or else the profile of its source, or else the first profile.
func Route(c *config.Config, m inbox.Message) string {
	if s, ok := c.Source(m.SourceID); ok {
		if p, ok := s.ChannelProfiles[m.ChannelID]; ok {
			return p
		}
		if s.Profile != "" {
			return s.Profile
		}
	}
	if len(c.Profiles) > 0 {
		return c.Profiles[0].Name
	}
	return ""
}

// DryRun only keeps the digest file, which the renderer already wrote.
type DryRun struct{}

// Deliver returns sent without starting any process.
func (DryRun) Deliver(_ context.Context, b *render.Rendered) (Result, error) {
	return Result{Status: StatusSent, Detail: "dry-run: digest only"}, nil
}
