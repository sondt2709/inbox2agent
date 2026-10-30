// Package source defines the Source interface and builds sources from the configuration.
package source

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/sondt2709/inbox2agent/internal/config"
	"github.com/sondt2709/inbox2agent/internal/inbox"
	"github.com/sondt2709/inbox2agent/internal/secrets"
	"github.com/sondt2709/inbox2agent/internal/source/mattermost"
	"github.com/sondt2709/inbox2agent/internal/source/whatsapp"
)

// Source collects new messages from one account.
// Collect returns the messages after the cursor and the next cursor.
// A source does not write to the store.
type Source interface {
	ID() string
	Collect(ctx context.Context, cur inbox.Cursor, now time.Time) ([]inbox.Message, inbox.Cursor, error)
}

// Env is what a source needs besides its configuration entry.
type Env struct {
	Secrets  secrets.Store
	Lookback time.Duration
	HTTP     *http.Client // nil means the default client
	DataDir  string       // the data directory of i2a
	Log      *slog.Logger // nil means no log
}

// ErrNotPaired is returned by a WhatsApp source that has no device keys.
var ErrNotPaired = whatsapp.ErrNotPaired

// Build makes the source for one configuration entry. A Mattermost source
// reads its token from the secret store. A WhatsApp source connects on its
// first Collect and must be closed (io.Closer).
func Build(sc config.Source, env Env) (Source, error) {
	switch sc.Type {
	case config.SourceMattermost:
		token, err := env.Secrets.Get(secrets.TokenKey(sc.ID))
		if err != nil {
			return nil, fmt.Errorf("source %q: read token from %s: %w", sc.ID, env.Secrets.Backend(), err)
		}
		return NewMattermost(sc, token, env.Lookback, env.HTTP), nil
	case config.SourceWhatsApp:
		return whatsapp.New(whatsapp.Settings{ID: sc.ID, DataDir: env.DataDir, Channels: sc.Channels,
			AllDirect: sc.AllDirect, Mentions: sc.Mentions, Lookback: env.Lookback, Log: env.Log}), nil
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
