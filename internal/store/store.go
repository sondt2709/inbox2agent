// Package store keeps messages, cursors, dispatches, sessions, cards and replies in SQLite.
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite" // registers the "sqlite" driver

	"github.com/sondt2709/inbox2agent/internal/inbox"
)

// SchemaVersion is the current schema version.
const SchemaVersion = 6

const schema = `
CREATE TABLE IF NOT EXISTS schema_version (version INTEGER NOT NULL);
CREATE TABLE IF NOT EXISTS messages (
  id            INTEGER PRIMARY KEY,
  source_id     TEXT NOT NULL,
  source_type   TEXT NOT NULL,
  external_id   TEXT NOT NULL,
  channel_id    TEXT NOT NULL,
  channel_name  TEXT NOT NULL,
  channel_kind  TEXT NOT NULL,
  thread_id     TEXT NOT NULL DEFAULT '',
  author        TEXT NOT NULL,
  text          TEXT NOT NULL,
  url           TEXT NOT NULL,
  created_at    INTEGER NOT NULL,
  collected_at  INTEGER NOT NULL,
  space         TEXT NOT NULL DEFAULT '',
  dispatch_id   INTEGER,
  state         TEXT NOT NULL DEFAULT 'pending',
  UNIQUE(source_id, external_id)
);
CREATE INDEX IF NOT EXISTS messages_state ON messages(state, created_at);
CREATE TABLE IF NOT EXISTS cursors (
  source_id TEXT NOT NULL,
  key       TEXT NOT NULL,
  value     TEXT NOT NULL,
  PRIMARY KEY(source_id, key)
);
CREATE TABLE IF NOT EXISTS dispatches (
  id            INTEGER PRIMARY KEY,
  space         TEXT NOT NULL,
  agent         TEXT NOT NULL DEFAULT '',
  driver        TEXT NOT NULL,
  created_at    INTEGER NOT NULL,
  status        TEXT NOT NULL,
  digest_path   TEXT NOT NULL,
  message_count INTEGER NOT NULL,
  detail        TEXT NOT NULL DEFAULT ''
);
CREATE TABLE IF NOT EXISTS sessions (
  space       TEXT PRIMARY KEY,
  handle      TEXT NOT NULL,
  created_at  INTEGER NOT NULL,
  primer_hash TEXT NOT NULL DEFAULT ''
);
CREATE TABLE IF NOT EXISTS cards (
  id         INTEGER PRIMARY KEY,
  key        TEXT,
  category   TEXT NOT NULL,
  urgency    TEXT NOT NULL,
  grp        TEXT NOT NULL DEFAULT '',
  title      TEXT NOT NULL,
  body       TEXT NOT NULL DEFAULT '',
  reply      TEXT NOT NULL DEFAULT '',
  refs       TEXT NOT NULL DEFAULT '[]',
  state      TEXT NOT NULL DEFAULT 'new',
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL,
  done_at    INTEGER,
  handle     TEXT NOT NULL DEFAULT ''
);
CREATE UNIQUE INDEX IF NOT EXISTS cards_key ON cards(key) WHERE key IS NOT NULL;
CREATE INDEX IF NOT EXISTS cards_state ON cards(state, updated_at);
CREATE TABLE IF NOT EXISTS replies (
  id           INTEGER PRIMARY KEY,
  message_id   INTEGER NOT NULL,
  card_id      INTEGER,
  source_id    TEXT NOT NULL,
  channel_name TEXT NOT NULL,
  place        TEXT NOT NULL,
  text         TEXT NOT NULL,
  by           TEXT NOT NULL,
  state        TEXT NOT NULL,
  pending_id   TEXT NOT NULL,
  external_id  TEXT NOT NULL DEFAULT '',
  url          TEXT NOT NULL DEFAULT '',
  error        TEXT NOT NULL DEFAULT '',
  created_at   INTEGER NOT NULL,
  sent_at      INTEGER
);
CREATE INDEX IF NOT EXISTS replies_card ON replies(card_id);
CREATE INDEX IF NOT EXISTS replies_target ON replies(message_id, place);
CREATE TABLE IF NOT EXISTS asks (
  id         INTEGER PRIMARY KEY,
  card_id    INTEGER NOT NULL,
  space      TEXT NOT NULL,
  handle     TEXT NOT NULL DEFAULT '',
  text       TEXT NOT NULL,
  state      TEXT NOT NULL,
  detail     TEXT NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL,
  sent_at    INTEGER
);
CREATE INDEX IF NOT EXISTS asks_card ON asks(card_id);
CREATE INDEX IF NOT EXISTS asks_state ON asks(state, created_at);
`

// Dispatch statuses.
const (
	DispatchPending = "pending"
	DispatchSent    = "sent"
	DispatchHeld    = "held"
	DispatchFailed  = "failed"
)

// Store is the SQLite store.
type Store struct {
	db   *sql.DB
	path string
}

// Open opens or creates the store file with mode 0600.
func Open(path string) (*Store, error) {
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
	dsn := "file:" + path + "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(on)&_txlock=immediate"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	s := &Store{db: db, path: path}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate %s: %w", path, err)
	}
	return s, nil
}

// Close closes the database.
func (s *Store) Close() error { return s.db.Close() }

func (s *Store) migrate() error {
	// The tables of an earlier version keep their columns, because CREATE
	// TABLE IF NOT EXISTS does not change them. migrateV3 adds
	// sessions.primer_hash, migrateV4 adds messages.thread_id, migrateV5
	// adds cards.handle and migrateV6 renames the profile columns to space.
	if _, err := s.db.Exec(schema); err != nil {
		return err
	}
	var v int
	err := s.db.QueryRow(`SELECT version FROM schema_version`).Scan(&v)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		_, err = s.db.Exec(`INSERT INTO schema_version(version) VALUES (?)`, SchemaVersion)
		return err
	case err != nil:
		return err
	case v > SchemaVersion:
		return fmt.Errorf("the store has schema version %d; this i2a supports %d. Update i2a", v, SchemaVersion)
	case v == SchemaVersion:
		return nil
	}
	if err := s.backup(v); err != nil {
		return err
	}
	if v < 2 {
		if err := s.migrateV2(); err != nil {
			return err
		}
	}
	if v < 3 {
		if err := s.migrateV3(); err != nil {
			return err
		}
	}
	if v < 4 {
		if err := s.migrateV4(); err != nil {
			return err
		}
	}
	if v < 5 {
		if err := s.migrateV5(); err != nil {
			return err
		}
	}
	return s.migrateV6()
}

// backup writes a consistent copy of the store to <file>.v<version>.bak
// before a migration changes it. VACUUM INTO also copies the WAL content.
func (s *Store) backup(version int) error {
	dst := fmt.Sprintf("%s.v%d.bak", s.path, version)
	if err := os.Remove(dst); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if _, err := s.db.Exec(`VACUUM INTO ?`, dst); err != nil {
		return fmt.Errorf("back up the store to %s: %w", dst, err)
	}
	return os.Chmod(dst, 0o600)
}

// migrateV3 adds the primer hash of the sessions. The cards table comes from
// the schema, because it did not exist before.
func (s *Store) migrateV3() error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck // no effect after Commit
	var n int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('sessions') WHERE name = 'primer_hash'`).Scan(&n); err != nil {
		return err
	}
	if n == 0 {
		if _, err := tx.Exec(`ALTER TABLE sessions ADD COLUMN primer_hash TEXT NOT NULL DEFAULT ''`); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(`UPDATE schema_version SET version = 3`); err != nil {
		return err
	}
	return tx.Commit()
}

// migrateV4 adds the thread ID of the messages. The replies table comes from
// the schema, because it did not exist before.
func (s *Store) migrateV4() error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck // no effect after Commit
	var n int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('messages') WHERE name = 'thread_id'`).Scan(&n); err != nil {
		return err
	}
	if n == 0 {
		if _, err := tx.Exec(`ALTER TABLE messages ADD COLUMN thread_id TEXT NOT NULL DEFAULT ''`); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(`UPDATE schema_version SET version = 4`); err != nil {
		return err
	}
	return tx.Commit()
}

// migrateV5 adds the terminal of the cards. The asks table comes from the
// schema, because it did not exist before.
func (s *Store) migrateV5() error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck // no effect after Commit
	var n int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('cards') WHERE name = 'handle'`).Scan(&n); err != nil {
		return err
	}
	if n == 0 {
		if _, err := tx.Exec(`ALTER TABLE cards ADD COLUMN handle TEXT NOT NULL DEFAULT ''`); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(`UPDATE schema_version SET version = 5`); err != nil {
		return err
	}
	return tx.Commit()
}

// migrateV6 renames the profile columns to space and records the agent of
// each dispatch. Version 5 had one agent for each profile, and the
// configuration migration makes one space and one agent for each profile with
// the profile name as the ID, so the old value is valid for both.
func (s *Store) migrateV6() error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck // no effect after Commit
	has := func(table, col string) (bool, error) {
		var n int
		err := tx.QueryRow(`SELECT COUNT(*) FROM pragma_table_info(?) WHERE name = ?`, table, col).Scan(&n)
		return n > 0, err
	}
	for _, table := range []string{"messages", "dispatches", "sessions", "asks"} {
		old, err := has(table, "profile")
		if err != nil {
			return err
		}
		if old {
			if _, err := tx.Exec(`ALTER TABLE ` + table + ` RENAME COLUMN profile TO space`); err != nil {
				return err
			}
		}
	}
	agent, err := has("dispatches", "agent")
	if err != nil {
		return err
	}
	if !agent {
		if _, err := tx.Exec(`ALTER TABLE dispatches ADD COLUMN agent TEXT NOT NULL DEFAULT ''`); err != nil {
			return err
		}
		if _, err := tx.Exec(`UPDATE dispatches SET agent = space`); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(`UPDATE schema_version SET version = 6`); err != nil {
		return err
	}
	return tx.Commit()
}

// migrateV2 adds the previewed state. Version 1 marked the messages of
// dry-run batches sent, so a real driver never got them.
func (s *Store) migrateV2() error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck // no effect after Commit
	if _, err := tx.Exec(`UPDATE messages SET state = 'previewed'
	  WHERE state = 'sent' AND dispatch_id IN (SELECT id FROM dispatches WHERE driver = 'dry-run')`); err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE schema_version SET version = 2`); err != nil {
		return err
	}
	return tx.Commit()
}

// Version returns the schema version in the file.
func (s *Store) Version(ctx context.Context) (int, error) {
	var v int
	err := s.db.QueryRowContext(ctx, `SELECT version FROM schema_version`).Scan(&v)
	return v, err
}

// Cursor returns the saved cursor of a source.
func (s *Store) Cursor(ctx context.Context, sourceID string) (inbox.Cursor, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT key, value FROM cursors WHERE source_id = ?`, sourceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	c := inbox.Cursor{}
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		c[k] = v
	}
	return c, rows.Err()
}

// SavePoll saves the messages of one poll and the new cursor in one transaction.
// It returns the number of messages that were new to the store.
func (s *Store) SavePoll(ctx context.Context, sourceID string, msgs []inbox.Message, cur inbox.Cursor, now time.Time) (int, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback() //nolint:errcheck // no effect after Commit

	ins, err := tx.PrepareContext(ctx, `INSERT INTO messages
	  (source_id, source_type, external_id, channel_id, channel_name, channel_kind, thread_id, author, text, url, created_at, collected_at, state)
	  VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'pending')
	  ON CONFLICT(source_id, external_id) DO NOTHING`)
	if err != nil {
		return 0, err
	}
	defer ins.Close()
	added := 0
	for _, m := range msgs {
		if m.SourceID != sourceID {
			return 0, fmt.Errorf("message %s has source %q, want %q", m.ExternalID, m.SourceID, sourceID)
		}
		res, err := ins.ExecContext(ctx, m.SourceID, m.SourceType, m.ExternalID, m.ChannelID, m.ChannelName,
			m.ChannelKind, m.ThreadID, m.Author, m.Text, m.URL, m.CreatedAt.UnixMilli(), now.UnixMilli())
		if err != nil {
			return 0, fmt.Errorf("save message %s: %w", m.ExternalID, err)
		}
		if n, _ := res.RowsAffected(); n > 0 {
			added++
		}
	}
	for k, v := range cur {
		if _, err := tx.ExecContext(ctx, `INSERT INTO cursors(source_id, key, value) VALUES (?, ?, ?)
		  ON CONFLICT(source_id, key) DO UPDATE SET value = excluded.value`, sourceID, k, v); err != nil {
			return 0, err
		}
	}
	return added, tx.Commit()
}

const messageCols = `id, source_id, source_type, external_id, channel_id, channel_name, channel_kind, thread_id, author, text, url, created_at, state, space`

func scanMessages(rows *sql.Rows) ([]inbox.Message, error) {
	defer rows.Close()
	var out []inbox.Message
	for rows.Next() {
		var m inbox.Message
		var created int64
		if err := rows.Scan(&m.ID, &m.SourceID, &m.SourceType, &m.ExternalID, &m.ChannelID, &m.ChannelName,
			&m.ChannelKind, &m.ThreadID, &m.Author, &m.Text, &m.URL, &created, &m.State, &m.Space); err != nil {
			return nil, err
		}
		m.CreatedAt = time.UnixMilli(created).UTC()
		out = append(out, m)
	}
	return out, rows.Err()
}

// Pending returns the pending and the previewed messages, oldest first.
// A real driver sends both; a dry-run agent takes only the pending ones.
func (s *Store) Pending(ctx context.Context) ([]inbox.Message, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+messageCols+` FROM messages WHERE state IN ('pending', 'previewed') ORDER BY created_at, id`)
	if err != nil {
		return nil, err
	}
	return scanMessages(rows)
}

// Messages returns the messages of a dispatch, oldest first.
func (s *Store) Messages(ctx context.Context, dispatchID int64) ([]inbox.Message, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+messageCols+` FROM messages WHERE dispatch_id = ? ORDER BY created_at, id`, dispatchID)
	if err != nil {
		return nil, err
	}
	return scanMessages(rows)
}

// ExpirePending marks pending and previewed messages created before the time as expired.
func (s *Store) ExpirePending(ctx context.Context, before time.Time) (int64, error) {
	res, err := s.db.ExecContext(ctx, `UPDATE messages SET state = 'expired' WHERE state IN ('pending', 'previewed') AND created_at < ?`, before.UnixMilli())
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// Dispatch is one attempt to deliver a batch.
type Dispatch struct {
	ID           int64
	Space        string
	Agent        string
	Driver       string
	CreatedAt    time.Time
	Status       string
	DigestPath   string
	MessageCount int
	Detail       string
}

// CreateDispatch records a new dispatch with the status pending.
func (s *Store) CreateDispatch(ctx context.Context, d Dispatch) (int64, error) {
	res, err := s.db.ExecContext(ctx, `INSERT INTO dispatches(space, agent, driver, created_at, status, digest_path, message_count, detail)
	  VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, d.Space, d.Agent, d.Driver, d.CreatedAt.UnixMilli(), DispatchPending, d.DigestPath, d.MessageCount, d.Detail)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// SetDigestPath sets the digest path of a dispatch.
func (s *Store) SetDigestPath(ctx context.Context, id int64, path string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE dispatches SET digest_path = ? WHERE id = ?`, path, id)
	return err
}

// FinishDispatch sets the final status of a dispatch. When the status is sent,
// it sets the messages to msgState (sent, or previewed for a dry run) in the same transaction.
func (s *Store) FinishDispatch(ctx context.Context, id int64, status, detail, space, msgState string, messageIDs []int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck // no effect after Commit
	if _, err := tx.ExecContext(ctx, `UPDATE dispatches SET status = ?, detail = ? WHERE id = ?`, status, detail, id); err != nil {
		return err
	}
	if status == DispatchSent && len(messageIDs) > 0 {
		q := `UPDATE messages SET state = ?, dispatch_id = ?, space = ? WHERE state IN ('pending', 'previewed') AND id IN (` +
			strings.TrimSuffix(strings.Repeat("?,", len(messageIDs)), ",") + `)`
		args := []any{msgState, id, space}
		for _, mid := range messageIDs {
			args = append(args, mid)
		}
		if _, err := tx.ExecContext(ctx, q, args...); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// Dispatches returns the newest dispatches, newest first.
func (s *Store) Dispatches(ctx context.Context, limit int) ([]Dispatch, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, space, agent, driver, created_at, status, digest_path, message_count, detail
	  FROM dispatches ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Dispatch
	for rows.Next() {
		var d Dispatch
		var created int64
		if err := rows.Scan(&d.ID, &d.Space, &d.Agent, &d.Driver, &created, &d.Status, &d.DigestPath, &d.MessageCount, &d.Detail); err != nil {
			return nil, err
		}
		d.CreatedAt = time.UnixMilli(created).UTC()
		out = append(out, d)
	}
	return out, rows.Err()
}

// Conv identifies one conversation: the account ID and the conversation ID.
type Conv struct{ Account, ID string }

// PendingCounts returns the number of pending messages of each conversation.
func (s *Store) PendingCounts(ctx context.Context) (map[Conv]int, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT source_id, channel_id, COUNT(*) FROM messages
	  WHERE state = 'pending' GROUP BY source_id, channel_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[Conv]int{}
	for rows.Next() {
		var c Conv
		var n int
		if err := rows.Scan(&c.Account, &c.ID, &n); err != nil {
			return nil, err
		}
		out[c] = n
	}
	return out, rows.Err()
}

// Counts returns the number of messages in each state.
func (s *Store) Counts(ctx context.Context) (map[string]int, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT state, COUNT(*) FROM messages GROUP BY state`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var st string
		var n int
		if err := rows.Scan(&st, &n); err != nil {
			return nil, err
		}
		out[st] = n
	}
	return out, rows.Err()
}

// Session returns the stored terminal handle of a space and the hash of the
// primer that the session got. The handle is "" when none exists.
func (s *Store) Session(ctx context.Context, space string) (handle, primerHash string, err error) {
	err = s.db.QueryRowContext(ctx, `SELECT handle, primer_hash FROM sessions WHERE space = ?`, space).Scan(&handle, &primerHash)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", nil
	}
	return handle, primerHash, err
}

// SetSession stores the terminal handle of a space and the hash of its primer.
func (s *Store) SetSession(ctx context.Context, space, handle, primerHash string, now time.Time) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO sessions(space, handle, created_at, primer_hash) VALUES (?, ?, ?, ?)
	  ON CONFLICT(space) DO UPDATE SET handle = excluded.handle, created_at = excluded.created_at,
	  primer_hash = excluded.primer_hash`, space, handle, now.UnixMilli(), primerHash)
	return err
}

// DeleteSession removes the stored handle of a space.
func (s *Store) DeleteSession(ctx context.Context, space string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE space = ?`, space)
	return err
}

// Prune deletes sent and expired messages, finished dispatches, done cards, replies and asks older than the time.
// It returns the number of deleted messages and the digest paths of the deleted dispatches.
func (s *Store) Prune(ctx context.Context, before time.Time) (int64, []string, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, nil, err
	}
	defer tx.Rollback() //nolint:errcheck // no effect after Commit
	res, err := tx.ExecContext(ctx, `DELETE FROM messages WHERE state IN ('sent', 'expired') AND created_at < ?`, before.UnixMilli())
	if err != nil {
		return 0, nil, err
	}
	n, _ := res.RowsAffected()
	if _, err := tx.ExecContext(ctx, `DELETE FROM cards WHERE state = 'done' AND done_at < ?`, before.UnixMilli()); err != nil {
		return 0, nil, err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM replies WHERE created_at < ?`, before.UnixMilli()); err != nil {
		return 0, nil, err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM asks WHERE created_at < ?`, before.UnixMilli()); err != nil {
		return 0, nil, err
	}
	rows, err := tx.QueryContext(ctx, `SELECT digest_path FROM dispatches WHERE created_at < ? AND status != 'pending'`, before.UnixMilli())
	if err != nil {
		return 0, nil, err
	}
	var paths []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			rows.Close()
			return 0, nil, err
		}
		if p != "" {
			paths = append(paths, p)
		}
	}
	rows.Close()
	if _, err := tx.ExecContext(ctx, `DELETE FROM dispatches WHERE created_at < ? AND status != 'pending'
	  AND id NOT IN (SELECT dispatch_id FROM messages WHERE dispatch_id IS NOT NULL)`, before.UnixMilli()); err != nil {
		return 0, nil, err
	}
	return n, paths, tx.Commit()
}
