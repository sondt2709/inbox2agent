// Package whatsapp reads the messages of a personal WhatsApp account. i2a
// connects as a linked device with whatsmeow. The event handler writes each
// message to the file of the source (wa_messages), and Collect reads that
// file after a row ID cursor. i2a never sends read receipts or presence.
package whatsapp

import (
	"context"
	"log/slog"
	"strconv"
	"sync"
	"time"

	"go.mau.fi/whatsmeow/proto/waE2E"
	"google.golang.org/protobuf/proto"

	"github.com/sondt2709/inbox2agent/internal/config"
	"github.com/sondt2709/inbox2agent/internal/inbox"
)

// SourceType is the type name in the configuration.
const SourceType = "whatsapp"

// offlineWait limits the wait for the offline messages after a connect.
const offlineWait = 30 * time.Second

// batch limits the rows that one Collect reads.
const batch = 1000

// Settings are the values that the source needs from the configuration.
type Settings struct {
	ID       string
	DataDir  string
	Levels   map[string]string // the read level of each chat (JID) that has an entry
	Default  string            // the read level of the other chats (unsorted.read)
	Lookback time.Duration
	// Retention keeps the messages with a file, so that i2a download can
	// save the file later. 0 means the rule of the other messages.
	Retention time.Duration
	Log       *slog.Logger
	// LockWait waits for the lock of the connection when another process
	// holds it, for a send or a download of one process. 0 fails at once.
	LockWait time.Duration
	// NoOfflineWait connects without the wait for the offline messages,
	// for a send or a download of one process.
	NoOfflineWait bool
}

// Source collects the messages of one WhatsApp account.
type Source struct {
	s Settings

	mu     sync.Mutex
	conn   *Conn
	closed bool
}

// New returns a source. It connects on the first Collect.
func New(s Settings) *Source { return &Source{s: s} }

// ID returns the source ID.
func (src *Source) ID() string { return src.s.ID }

// Collect connects when necessary and returns the messages after the cursor.
func (src *Source) Collect(ctx context.Context, cur inbox.Cursor, now time.Time) ([]inbox.Message, inbox.Cursor, error) {
	conn, err := src.connect(ctx)
	if err != nil {
		return nil, cur, err
	}
	return collect(ctx, conn.db, src.s, cur, now)
}

// connect opens the file and connects one time; it waits for the offline
// messages after the connect. A source that is not paired is closed again,
// so that the TUI can pair it.
func (src *Source) connect(ctx context.Context) (*Conn, error) {
	src.mu.Lock()
	defer src.mu.Unlock()
	return src.connectLocked(ctx)
}

// connectLocked is connect for a caller that holds src.mu.
func (src *Source) connectLocked(ctx context.Context) (*Conn, error) {
	if src.closed {
		return nil, ErrClosed
	}
	if src.conn != nil {
		if !src.conn.Paired() { // the phone unlinked i2a
			src.conn.Close()
			src.conn = nil
			return nil, ErrNotPaired
		}
		return src.conn, nil
	}
	conn, err := open(ctx, src.s.DataDir, src.s.ID, src.s.Lookback, src.s.Log, src.s.LockWait)
	if err != nil {
		return nil, err
	}
	if err := conn.Connect(ctx); err != nil {
		conn.Close()
		return nil, err
	}
	if !src.s.NoOfflineWait {
		conn.WaitOffline(ctx, offlineWait)
	}
	src.conn = conn
	return conn, nil
}

// Close disconnects and releases the lock. A closed source does not connect
// again.
func (src *Source) Close() error {
	src.mu.Lock()
	defer src.mu.Unlock()
	src.closed = true
	if src.conn == nil {
		return nil
	}
	err := src.conn.Close()
	src.conn = nil
	return err
}

// collect reads the rows after the cursor, applies the filters and deletes
// the rows older than the lookback and one hour.
func collect(ctx context.Context, db *DB, s Settings, cur inbox.Cursor, now time.Time) ([]inbox.Message, inbox.Cursor, error) {
	after, _ := strconv.ParseInt(cur["rowid"], 10, 64)
	rows, err := db.Messages(ctx, after, batch)
	if err != nil {
		return nil, cur, err
	}
	since := now.Add(-s.Lookback)
	var msgs []inbox.Message
	for _, r := range rows {
		after = r.RowID
		if r.FromMe || r.Time.Before(since) || !wanted(s, r) {
			continue
		}
		m := message(s.ID, r)
		if r.QuoteID != "" {
			m.ReplyTo = replyRef(ctx, db, r)
		}
		msgs = append(msgs, m)
	}
	textBefore := since.Add(-time.Hour)
	mediaBefore := textBefore
	if keep := now.Add(-s.Retention); s.Retention > 0 && keep.Before(mediaBefore) {
		mediaBefore = keep
	}
	if err := db.Prune(ctx, textBefore, mediaBefore); err != nil {
		return nil, cur, err
	}
	return msgs, inbox.Cursor{"rowid": strconv.FormatInt(after, 10)}, nil
}

// wanted applies the read level of the chat. With the level mentions, each
// message of a direct chat is for the user; in a group only a mention or a
// quote of the user is.
func wanted(s Settings, r Row) bool {
	level, ok := s.Levels[r.Chat]
	if !ok {
		level = s.Default
	}
	switch level {
	case config.ReadAll:
		return true
	case config.ReadMentions:
		return r.Kind == inbox.KindDirect || r.Mention
	default:
		return false
	}
}

// replyRef returns the reference to the message that the row quotes. The
// quoted text comes from raw. When the author is not known, it is the sender
// name of the quoted row in the file, if the file has it.
func replyRef(ctx context.Context, db *DB, r Row) *inbox.ReplyRef {
	ref := &inbox.ReplyRef{Ext: r.QuoteID, Author: r.QuoteAuthor, Me: r.QuoteMe}
	var msg waE2E.Message
	if err := proto.Unmarshal(r.Raw, &msg); err == nil {
		ref.Text = quoted(&msg)
	}
	if ref.Author == "" && !ref.Me {
		if q, ok, err := db.Row(ctx, r.Chat, r.QuoteID); err == nil && ok {
			ref.Me = q.FromMe
			if !q.FromMe {
				ref.Author = q.SenderName
			}
		}
	}
	return ref
}

func message(sourceID string, r Row) inbox.Message {
	m := inbox.Message{
		SourceID:    sourceID,
		SourceType:  SourceType,
		ExternalID:  r.ID,
		ChannelID:   r.Chat,
		ChannelName: r.ChatName,
		ChannelKind: r.Kind,
		Author:      r.SenderName,
		Text:        r.Text,
		CreatedAt:   r.Time,
		ForMe:       r.Kind == inbox.KindDirect || r.Mention,
	}
	if m.ChannelName == "" {
		m.ChannelName = r.SenderName
		if r.Kind == inbox.KindGroup {
			m.ChannelName = "group"
		}
	}
	if r.Kind == inbox.KindDirect && r.Phone != "" {
		m.URL = "https://wa.me/" + r.Phone
	}
	return m
}
