package daemon

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/sondt2709/inbox2agent/internal/app"
	"github.com/sondt2709/inbox2agent/internal/config"
	"github.com/sondt2709/inbox2agent/internal/inbox"
	"github.com/sondt2709/inbox2agent/internal/source"
	"github.com/sondt2709/inbox2agent/internal/source/whatsapp"
)

// One process holds the WhatsApp connection of an account, and that is the
// daemon when it runs. So the Feed and the commands send a WhatsApp reply and
// download a file through the daemon, and connect themselves only when no
// daemon answers. A Mattermost reply uses HTTP and needs no daemon.

// platform returns the platform of the account.
func platform(cfg *config.Config, accountID string) (string, error) {
	for _, a := range cfg.Accounts {
		if a.ID == accountID {
			return a.Platform, nil
		}
	}
	return "", fmt.Errorf("account %q is not in the configuration", accountID)
}

// BuildReplier returns the replier of the account for a process that is not
// the daemon. env.DataDir names the data directory of the daemon.
func BuildReplier(cfg *config.Config, accountID string, env source.Env) (source.Replier, error) {
	if err := source.CanReply(cfg, accountID); err != nil {
		return nil, err
	}
	p, _ := platform(cfg, accountID)
	if p != config.PlatformWhatsApp {
		return source.BuildReplier(cfg, accountID, env)
	}
	return &viaDaemon{client: NewClient(env.DataDir), local: func() (source.Replier, error) {
		return source.BuildReplier(cfg, accountID, source.OneShot(env))
	}}, nil
}

// viaDaemon sends a reply through the daemon, or with a connection of this
// process when no daemon answers.
type viaDaemon struct {
	client *Client
	local  func() (source.Replier, error)
}

func (v *viaDaemon) Reply(ctx context.Context, r inbox.ReplyRequest) (inbox.Posted, error) {
	p, err := v.client.Reply(ctx, r.ReplyID)
	if !errors.Is(err, ErrNoDaemon) {
		return p, err
	}
	rp, err := v.local()
	if err != nil {
		return inbox.Posted{}, &refusedError{err}
	}
	if c, ok := rp.(io.Closer); ok {
		defer c.Close()
	}
	return rp.Reply(ctx, r)
}

// Find returns false: a WhatsApp reply sends again with the same message ID.
func (v *viaDaemon) Find(context.Context, inbox.ReplyRequest) (inbox.Posted, bool, error) {
	return inbox.Posted{}, false, nil
}

// Download downloads the file of the stored message to a new file in
// TmpDir of the data directory env.DataDir: through the daemon when it
// answers, else with a connection of this process. The caller moves or
// removes the file.
func Download(ctx context.Context, cfg *config.Config, m inbox.Message, env source.Env) (whatsapp.File, error) {
	p, err := platform(cfg, m.SourceID)
	if err != nil {
		return whatsapp.File{}, err
	}
	if p != config.PlatformWhatsApp {
		return whatsapp.File{}, fmt.Errorf("account %q (%s): %w", m.SourceID, p, source.ErrCannotDownload)
	}
	f, err := NewClient(env.DataDir).Download(ctx, m.ID)
	if !errors.Is(err, ErrNoDaemon) {
		return f, err
	}
	d, err := source.BuildDownloader(cfg, m.SourceID, source.OneShot(env))
	if err != nil {
		return whatsapp.File{}, err
	}
	if c, ok := d.(io.Closer); ok {
		defer c.Close()
	}
	return d.Download(ctx, m.ChannelID, m.ExternalID, tmpDir(env.DataDir))
}

// tmpDir is app.Paths.TmpDir of the data directory.
func tmpDir(dataDir string) string { return (app.Paths{DataDir: dataDir}).TmpDir() }
