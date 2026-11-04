package whatsapp

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go.mau.fi/whatsmeow/store/sqlstore"
	waLog "go.mau.fi/whatsmeow/util/log"
	_ "modernc.org/sqlite" // the database/sql driver "sqlite"
)

// Path returns the file of a source: the device keys and the received messages.
func Path(dataDir, id string) string { return filepath.Join(dataDir, "whatsapp-"+id+".db") }

// LockPath returns the lock file that the holder of the connection locks.
func LockPath(dataDir, id string) string { return filepath.Join(dataDir, "whatsapp-"+id+".lock") }

const schema = `
CREATE TABLE IF NOT EXISTS wa_messages (
  chat        TEXT NOT NULL,
  id          TEXT NOT NULL,
  sender      TEXT NOT NULL,
  sender_name TEXT NOT NULL DEFAULT '',
  phone       TEXT NOT NULL DEFAULT '',
  kind        TEXT NOT NULL,
  from_me     INTEGER NOT NULL DEFAULT 0,
  mention     INTEGER NOT NULL DEFAULT 0,
  ts          INTEGER NOT NULL,
  text        TEXT NOT NULL,
  raw         BLOB,
  media       INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY (chat, id)
);
CREATE INDEX IF NOT EXISTS wa_messages_ts ON wa_messages(ts);
CREATE TABLE IF NOT EXISTS wa_chats (
  jid     TEXT PRIMARY KEY,
  name    TEXT NOT NULL DEFAULT '',
  kind    TEXT NOT NULL,
  last_ts INTEGER NOT NULL DEFAULT 0
);
`

// DB is the file of one source. It holds the whatsmeow tables and the
// tables wa_messages and wa_chats.
type DB struct {
	sql       *sql.DB
	Container *sqlstore.Container
}

// OpenDB opens or creates the file with the mode 0600, in a directory with the mode 0700.
func OpenDB(ctx context.Context, path string, log waLog.Logger) (*DB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	f.Close()
	if err := os.Chmod(path, 0o600); err != nil {
		return nil, err
	}
	dsn := "file:" + path + "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(10000)&_pragma=foreign_keys(on)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	if _, err := db.ExecContext(ctx, schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("create tables in %s: %w", path, err)
	}
	if err := addMediaColumn(ctx, db); err != nil {
		db.Close()
		return nil, fmt.Errorf("add the column media to %s: %w", path, err)
	}
	c := sqlstore.NewWithDB(db, "sqlite", log)
	if err := c.Upgrade(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("upgrade %s: %w", path, err)
	}
	return &DB{sql: db, Container: c}, nil
}

// addMediaColumn adds wa_messages.media to a file of an older version. The
// old rows get 0, so the prune deletes them after lookback as before.
func addMediaColumn(ctx context.Context, db *sql.DB) error {
	var n int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM pragma_table_info('wa_messages') WHERE name = 'media'`).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	_, err := db.ExecContext(ctx, `ALTER TABLE wa_messages ADD COLUMN media INTEGER NOT NULL DEFAULT 0`)
	return err
}

// Close closes the file.
func (d *DB) Close() error { return d.sql.Close() }

// Paired reports whether the file has the keys of a paired device.
func (d *DB) Paired(ctx context.Context) (bool, error) {
	var n int
	err := d.sql.QueryRowContext(ctx, `SELECT count(*) FROM whatsmeow_device`).Scan(&n)
	return n > 0, err
}

// Row is one received message.
type Row struct {
	RowID      int64
	Chat       string // the chat JID
	ID         string // the message ID
	Sender     string // the sender JID
	SenderName string
	Phone      string // the phone number of a direct chat; "" when not known
	Kind       string // inbox.KindDirect or inbox.KindGroup
	FromMe     bool
	Mention    bool // the message mentions the user or quotes a message of the user
	Time       time.Time
	Text       string
	Raw        []byte // the protobuf of waE2E.Message
	Media      bool   // the message has a file that i2a download can save
	ChatName   string // from wa_chats, when read
}

// SaveMessage stores a message. It ignores a message that is stored already.
func (d *DB) SaveMessage(ctx context.Context, r Row) error {
	_, err := d.sql.ExecContext(ctx, `INSERT INTO wa_messages(chat, id, sender, sender_name, phone, kind, from_me, mention, ts, text, raw, media)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(chat, id) DO NOTHING`,
		r.Chat, r.ID, r.Sender, r.SenderName, r.Phone, r.Kind, r.FromMe, r.Mention, r.Time.UnixMilli(), r.Text, r.Raw, r.Media)
	return err
}

// Row returns the stored message with the chat and the ID, and false when
// the file has no such row (never stored, or deleted by the prune).
func (d *DB) Row(ctx context.Context, chat, id string) (Row, bool, error) {
	var r Row
	var ts int64
	err := d.sql.QueryRowContext(ctx, `SELECT rowid, chat, id, sender, sender_name, phone, kind, from_me, mention, ts, text, raw, media
		FROM wa_messages WHERE chat = ? AND id = ?`, chat, id).Scan(&r.RowID, &r.Chat, &r.ID, &r.Sender, &r.SenderName, &r.Phone,
		&r.Kind, &r.FromMe, &r.Mention, &ts, &r.Text, &r.Raw, &r.Media)
	if errors.Is(err, sql.ErrNoRows) {
		return Row{}, false, nil
	}
	if err != nil {
		return Row{}, false, err
	}
	r.Time = time.UnixMilli(ts)
	return r, true, nil
}

// Messages returns at most limit messages after the row ID, oldest first,
// with the chat name.
func (d *DB) Messages(ctx context.Context, after int64, limit int) ([]Row, error) {
	rows, err := d.sql.QueryContext(ctx, `SELECT m.rowid, m.chat, m.id, m.sender, m.sender_name, m.phone, m.kind, m.from_me,
		m.mention, m.ts, m.text, m.raw, coalesce(c.name, '')
		FROM wa_messages m LEFT JOIN wa_chats c ON c.jid = m.chat
		WHERE m.rowid > ? ORDER BY m.rowid LIMIT ?`, after, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Row
	for rows.Next() {
		var r Row
		var ts int64
		if err := rows.Scan(&r.RowID, &r.Chat, &r.ID, &r.Sender, &r.SenderName, &r.Phone, &r.Kind, &r.FromMe,
			&r.Mention, &ts, &r.Text, &r.Raw, &r.ChatName); err != nil {
			return nil, err
		}
		r.Time = time.UnixMilli(ts)
		out = append(out, r)
	}
	return out, rows.Err()
}

// Prune deletes the messages without a file older than textBefore, and the
// messages with a file older than mediaBefore.
func (d *DB) Prune(ctx context.Context, textBefore, mediaBefore time.Time) error {
	_, err := d.sql.ExecContext(ctx, `DELETE FROM wa_messages WHERE (media = 0 AND ts < ?) OR (media <> 0 AND ts < ?)`,
		textBefore.UnixMilli(), mediaBefore.UnixMilli())
	return err
}

// Chat is one chat for the picker.
type Chat struct {
	JID  string
	Name string
	Kind string // inbox.KindDirect or inbox.KindGroup
	Last time.Time
}

// SaveChat adds or updates a chat. An empty name keeps the stored name, and
// an older time keeps the stored time.
func (d *DB) SaveChat(ctx context.Context, c Chat) error {
	var last int64
	if !c.Last.IsZero() {
		last = c.Last.UnixMilli()
	}
	_, err := d.sql.ExecContext(ctx, `INSERT INTO wa_chats(jid, name, kind, last_ts) VALUES(?,?,?,?)
		ON CONFLICT(jid) DO UPDATE SET
		  name = CASE WHEN excluded.name <> '' THEN excluded.name ELSE wa_chats.name END,
		  last_ts = max(wa_chats.last_ts, excluded.last_ts)`, c.JID, strings.TrimSpace(c.Name), c.Kind, last)
	return err
}

// Chats returns the chats, the most recent first.
func (d *DB) Chats(ctx context.Context) ([]Chat, error) {
	rows, err := d.sql.QueryContext(ctx, `SELECT jid, name, kind, last_ts FROM wa_chats ORDER BY last_ts DESC, name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Chat
	for rows.Next() {
		var c Chat
		var last int64
		if err := rows.Scan(&c.JID, &c.Name, &c.Kind, &last); err != nil {
			return nil, err
		}
		if last > 0 {
			c.Last = time.UnixMilli(last)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
