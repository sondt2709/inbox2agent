package daemon

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/sondt2709/inbox2agent/internal/config"
	"github.com/sondt2709/inbox2agent/internal/inbox"
	"github.com/sondt2709/inbox2agent/internal/source"
	"github.com/sondt2709/inbox2agent/internal/source/whatsapp"
)

// The engine sends a reply and downloads a file for the API. These calls do
// not go through the loop: a batch can wait minutes for a busy agent, and a
// reply must not wait for it. The source has its own lock.

// tmpMaxAge is the age of a file in TmpDir that the prune deletes. The
// download command moves its file at once; an older file is from a command
// that stopped.
const tmpMaxAge = time.Hour

// refusedError is an error after which no message exists.
type refusedError struct{ err error }

func (e *refusedError) Error() string  { return e.err.Error() }
func (e *refusedError) Unwrap() error  { return e.err }
func (e *refusedError) Answered() bool { return true }

// accountOf returns the entry of the account, from the current configuration.
func (e *Engine) accountOf(id string) (config.Account, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, a := range e.cfg.Accounts {
		if a.ID == id {
			return a, nil
		}
	}
	return config.Account{}, fmt.Errorf("account %q is not in the configuration", id)
}

// message returns the stored message with the ID.
func (e *Engine) message(ctx context.Context, id int64) (inbox.Message, error) {
	msgs, err := e.d.Store.MessagesByID(ctx, []int64{id})
	if err != nil {
		return inbox.Message{}, err
	}
	if len(msgs) == 0 {
		return inbox.Message{}, fmt.Errorf("the store has no message %d", id)
	}
	return msgs[0], nil
}

// Reply sends the stored reply with the ID through the source of its
// target message. An error with Answered() == true means that no message
// exists.
func (e *Engine) Reply(ctx context.Context, replyID int64) (inbox.Posted, error) {
	row, err := e.d.Store.ReplyByID(ctx, replyID)
	if err != nil {
		return inbox.Posted{}, &refusedError{fmt.Errorf("reply %d: %w", replyID, err)}
	}
	if row.State == inbox.ReplySent {
		return inbox.Posted{}, &refusedError{fmt.Errorf("reply %d is sent already", replyID)}
	}
	target, err := e.message(ctx, row.MessageID)
	if err != nil {
		return inbox.Posted{}, &refusedError{err}
	}
	sc, err := e.accountOf(target.SourceID)
	if err != nil {
		return inbox.Posted{}, &refusedError{err}
	}
	src, err := e.source(sc)
	if err != nil {
		return inbox.Posted{}, &refusedError{err}
	}
	rp, ok := src.(source.Replier)
	if !ok {
		return inbox.Posted{}, &refusedError{fmt.Errorf("account %q (%s): %w", sc.ID, sc.Platform, source.ErrCannotReply)}
	}
	return rp.Reply(ctx, inbox.ReplyRequest{Target: target, Text: row.Text, Thread: row.Place == inbox.PlaceThread,
		PendingID: row.PendingID, Since: row.CreatedAt, ReplyID: row.ID})
}

// Download writes the file of the stored message with the ID to a new file
// in TmpDir.
func (e *Engine) Download(ctx context.Context, messageID int64) (whatsapp.File, error) {
	m, err := e.message(ctx, messageID)
	if err != nil {
		return whatsapp.File{}, err
	}
	sc, err := e.accountOf(m.SourceID)
	if err != nil {
		return whatsapp.File{}, err
	}
	src, err := e.source(sc)
	if err != nil {
		return whatsapp.File{}, err
	}
	d, ok := src.(source.Downloader)
	if !ok {
		return whatsapp.File{}, fmt.Errorf("account %q (%s): %w", sc.ID, sc.Platform, source.ErrCannotDownload)
	}
	return d.Download(ctx, m.ChannelID, m.ExternalID, e.d.Paths.TmpDir())
}

// pruneTmp deletes the files in TmpDir that are older than tmpMaxAge.
func (e *Engine) pruneTmp() {
	dir := e.d.Paths.TmpDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			e.d.Log.Warn("read the temporary files", "dir", dir, "err", err)
		}
		return
	}
	limit := e.d.Now().Add(-tmpMaxAge)
	for _, en := range entries {
		info, err := en.Info()
		if err != nil || en.IsDir() || info.ModTime().After(limit) {
			continue
		}
		if err := os.Remove(filepath.Join(dir, en.Name())); err != nil {
			e.d.Log.Warn("remove a temporary file", "file", en.Name(), "err", err)
		}
	}
}
