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

// Env is what a source needs besides its account entry.
type Env struct {
	Secrets  secrets.Store
	Lookback time.Duration
	Levels   map[string]string // the read level of each conversation of the account that has an entry
	Default  string            // the read level of the other conversations (unsorted.read)
	HTTP     *http.Client      // nil means the default client
	DataDir  string            // the data directory of i2a
	Log      *slog.Logger      // nil means no log
}

// ErrNotPaired is returned by a WhatsApp source that has no device keys.
var ErrNotPaired = whatsapp.ErrNotPaired

// Build makes the source that reads one account. A Mattermost source reads
// its token from the secret store. A WhatsApp source connects on its first
// Collect and must be closed (io.Closer).
func Build(a config.Account, env Env) (Source, error) {
	switch a.Platform {
	case config.PlatformMattermost:
		token, err := env.Secrets.Get(secrets.TokenKey(a.ID))
		if err != nil {
			return nil, fmt.Errorf("account %q: read token from %s: %w", a.ID, env.Secrets.Backend(), err)
		}
		return mattermost.New(mattermost.Settings{ID: a.ID, URL: a.URL, Levels: env.Levels, Default: env.Default, Lookback: env.Lookback},
			mattermost.NewClient(a.URL, token, env.HTTP)), nil
	case config.PlatformWhatsApp:
		return whatsapp.New(whatsapp.Settings{ID: a.ID, DataDir: env.DataDir, Levels: env.Levels, Default: env.Default,
			Lookback: env.Lookback, Log: env.Log}), nil
	default:
		return nil, fmt.Errorf("account %q: unknown platform %q", a.ID, a.Platform)
	}
}

// NewMattermost makes a Mattermost source with a known token, to check the
// token and to list the channels. It reads no channel.
func NewMattermost(a config.Account, token string, lookback time.Duration, hc *http.Client) *mattermost.Source {
	return mattermost.New(mattermost.Settings{ID: a.ID, URL: a.URL, Default: config.ReadOff, Lookback: lookback},
		mattermost.NewClient(a.URL, token, hc))
}
