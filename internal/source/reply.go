package source

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/sondt2709/inbox2agent/internal/config"
	"github.com/sondt2709/inbox2agent/internal/inbox"
	"github.com/sondt2709/inbox2agent/internal/secrets"
)

// Replier is a source that can send a reply to a stored message.
// An error that has the method Answered() and returns true means that the
// platform answered and created no post. Any other error means that the
// result is not known.
type Replier interface {
	Reply(ctx context.Context, r inbox.ReplyRequest) (inbox.Posted, error)
	// Find looks for a post of the user with the text of r, in the place of r,
	// created after r.Since.
	Find(ctx context.Context, r inbox.ReplyRequest) (inbox.Posted, bool, error)
}

// ErrCannotReply is returned for a source that cannot send replies.
var ErrCannotReply = errors.New("cannot send replies yet")

// BuildReplier builds the source of the account with the ID, for a send.
func BuildReplier(cfg *config.Config, accountID string, sec secrets.Store, hc *http.Client) (Replier, error) {
	for _, sc := range cfg.Accounts {
		if sc.ID != accountID {
			continue
		}
		if sc.Platform != config.PlatformMattermost {
			return nil, fmt.Errorf("account %q (%s): %w", sc.ID, sc.Platform, ErrCannotReply)
		}
		src, err := Build(sc, Env{Secrets: sec, Lookback: cfg.Settings.Lookback.D(), HTTP: hc, Default: config.ReadOff})
		if err != nil {
			return nil, err
		}
		r, ok := src.(Replier)
		if !ok {
			return nil, fmt.Errorf("account %q (%s): %w", sc.ID, sc.Platform, ErrCannotReply)
		}
		return r, nil
	}
	return nil, fmt.Errorf("account %q is not in the configuration", accountID)
}

// CanReply reports whether the account with the ID exists in the configuration
// and can send replies. It does not read the token.
func CanReply(cfg *config.Config, accountID string) error {
	for _, sc := range cfg.Accounts {
		if sc.ID != accountID {
			continue
		}
		if sc.Platform != config.PlatformMattermost {
			return fmt.Errorf("account %q (%s): %w", sc.ID, sc.Platform, ErrCannotReply)
		}
		return nil
	}
	return fmt.Errorf("account %q is not in the configuration", accountID)
}
