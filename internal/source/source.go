// Package source defines the Source interface and builds sources from the configuration.
package source

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/sondt2709/inbox2agent/internal/config"
	"github.com/sondt2709/inbox2agent/internal/inbox"
	"github.com/sondt2709/inbox2agent/internal/secrets"
	"github.com/sondt2709/inbox2agent/internal/source/mattermost"
)

// Source collects new messages from one account.
// Collect returns the messages after the cursor and the next cursor.
// A source does not write to the store.
type Source interface {
	ID() string
	Collect(ctx context.Context, cur inbox.Cursor, now time.Time) ([]inbox.Message, inbox.Cursor, error)
}

// Build makes the source for one configuration entry. It reads the token from the secret store.
func Build(sc config.Source, sec secrets.Store, lookback time.Duration, hc *http.Client) (Source, error) {
	token, err := sec.Get(secrets.TokenKey(sc.ID))
	if err != nil {
		return nil, fmt.Errorf("source %q: read token from %s: %w", sc.ID, sec.Backend(), err)
	}
	switch sc.Type {
	case config.SourceMattermost:
		return NewMattermost(sc, token, lookback, hc), nil
	default:
		return nil, fmt.Errorf("source %q: unknown type %q", sc.ID, sc.Type)
	}
}

// NewMattermost makes a Mattermost source with a known token.
func NewMattermost(sc config.Source, token string, lookback time.Duration, hc *http.Client) *mattermost.Source {
	return mattermost.New(mattermost.Settings{
		ID: sc.ID, URL: sc.URL, Channels: sc.Channels, AllDirect: sc.AllDirect, Mentions: sc.Mentions, Lookback: lookback,
	}, mattermost.NewClient(sc.URL, token, hc))
}
