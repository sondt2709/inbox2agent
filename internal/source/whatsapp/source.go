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
	Log      *slog.Logger
}

// Source collects the messages of one WhatsApp account.
type Source struct {
	s Settings

	mu   sync.Mutex
	conn *Conn
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
	if src.conn != nil {
		if !src.conn.Paired() { // the phone unlinked i2a
			src.conn.Close()
			src.conn = nil
			return nil, ErrNotPaired
		}
		return src.conn, nil
	}
	conn, err := Open(ctx, src.s.DataDir, src.s.ID, src.s.Lookback, src.s.Log)
	if err != nil {
		return nil, err
	}
	if err := conn.Connect(ctx); err != nil {
		conn.Close()
		return nil, err
	}
	conn.WaitOffline(ctx, offlineWait)
	src.conn = conn
	return conn, nil
}

// Close disconnects and releases the lock.
func (src *Source) Close() error {
	src.mu.Lock()
	defer src.mu.Unlock()
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
		msgs = append(msgs, message(s.ID, r))
	}
	if err := db.Prune(ctx, since.Add(-time.Hour)); err != nil {
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
