package source

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/sondt2709/inbox2agent/internal/config"
	"github.com/sondt2709/inbox2agent/internal/inbox"
	"github.com/sondt2709/inbox2agent/internal/source/whatsapp"
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

// Downloader is a source that can download the file of a stored message.
type Downloader interface {
	// Download writes the file of the message to a new file in dir.
	Download(ctx context.Context, chat, id, dir string) (whatsapp.File, error)
}

// ErrCannotReply is returned for a source that cannot send replies.
var ErrCannotReply = errors.New("cannot send replies yet")

// ErrCannotDownload is returned for a source that cannot download files.
var ErrCannotDownload = errors.New("cannot download files")

// account returns the entry of the account with the ID.
func account(cfg *config.Config, accountID string) (config.Account, error) {
	for _, sc := range cfg.Accounts {
		if sc.ID == accountID {
			return sc, nil
		}
	}
	return config.Account{}, fmt.Errorf("account %q is not in the configuration", accountID)
}

// canReply lists the platforms that send replies.
func canReply(p string) bool {
	return p == config.PlatformMattermost || p == config.PlatformWhatsApp
}

// OneShot returns env for a source of one send or one download in a process
// that is not the daemon: it waits for the WhatsApp lock that a closing
// daemon or dashboard can hold, and does not wait for offline messages.
func OneShot(env Env) Env {
	env.OneShot, env.LockWait, env.Default = true, 10*time.Second, config.ReadOff
	return env
}

// BuildReplier builds the source of the account with the ID, for a send in
// this process. A WhatsApp source holds a connection: close it (io.Closer).
func BuildReplier(cfg *config.Config, accountID string, env Env) (Replier, error) {
	sc, err := account(cfg, accountID)
	if err != nil {
		return nil, err
	}
	if !canReply(sc.Platform) {
		return nil, fmt.Errorf("account %q (%s): %w", sc.ID, sc.Platform, ErrCannotReply)
	}
	env.Lookback, env.Retention = cfg.Settings.Lookback.D(), cfg.Settings.Retention.D()
	src, err := Build(sc, env)
	if err != nil {
		return nil, err
	}
	r, ok := src.(Replier)
	if !ok {
		return nil, fmt.Errorf("account %q (%s): %w", sc.ID, sc.Platform, ErrCannotReply)
	}
	return r, nil
}

// BuildDownloader builds the source of the account with the ID, for a
// download in this process. Close it (io.Closer).
func BuildDownloader(cfg *config.Config, accountID string, env Env) (Downloader, error) {
	sc, err := account(cfg, accountID)
	if err != nil {
		return nil, err
	}
	env.Lookback, env.Retention = cfg.Settings.Lookback.D(), cfg.Settings.Retention.D()
	src, err := Build(sc, env)
	if err != nil {
		return nil, err
	}
	d, ok := src.(Downloader)
	if !ok {
		return nil, fmt.Errorf("account %q (%s): %w", sc.ID, sc.Platform, ErrCannotDownload)
	}
	return d, nil
}

// CanReply reports whether the account with the ID exists in the configuration
// and can send replies. It does not read the token.
func CanReply(cfg *config.Config, accountID string) error {
	sc, err := account(cfg, accountID)
	if err != nil {
		return err
	}
	if !canReply(sc.Platform) {
		return fmt.Errorf("account %q (%s): %w", sc.ID, sc.Platform, ErrCannotReply)
	}
	return nil
}
