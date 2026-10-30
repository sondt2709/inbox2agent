package store

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sondt2709/inbox2agent/internal/inbox"
)

// schemaV5 is the schema of version 5 (before spaces), as it was released.
const schemaV5 = `
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
  profile       TEXT NOT NULL DEFAULT '',
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
  profile       TEXT NOT NULL,
  driver        TEXT NOT NULL,
  created_at    INTEGER NOT NULL,
  status        TEXT NOT NULL,
  digest_path   TEXT NOT NULL,
  message_count INTEGER NOT NULL,
  detail        TEXT NOT NULL DEFAULT ''
);
CREATE TABLE IF NOT EXISTS sessions (
  profile     TEXT PRIMARY KEY,
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
  profile    TEXT NOT NULL,
  handle     TEXT NOT NULL DEFAULT '',
  text       TEXT NOT NULL,
  state      TEXT NOT NULL,
  detail     TEXT NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL,
  sent_at    INTEGER
);
CREATE INDEX IF NOT EXISTS asks_card ON asks(card_id);
CREATE INDEX IF NOT EXISTS asks_state ON asks(state, created_at);
INSERT INTO schema_version(version) VALUES (5);
`

func TestMigrateV5RenamesProfileToSpace(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "inbox.db")
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(schemaV5); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 120; i++ {
		if _, err := db.Exec(`INSERT INTO messages(source_id, source_type, external_id, channel_id, channel_name, channel_kind,
		  author, text, url, created_at, collected_at, profile, state) VALUES ('work', 'mattermost', ?, 'c', 'general', 'channel',
		  'bob', 'hi', '', 1, 1, 'triage', 'sent')`, i); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 4; i++ {
		if _, err := db.Exec(`INSERT INTO dispatches(profile, driver, created_at, status, digest_path, message_count)
		  VALUES ('triage', 'orca', 1, 'sent', '', 30)`); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(`INSERT INTO sessions(profile, handle, created_at, primer_hash) VALUES ('triage', 'term_a', 1, 'h1')`); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if _, err := db.Exec(`INSERT INTO asks(card_id, profile, text, state, created_at) VALUES (1, 'triage', 'more', 'sent', 1)`); err != nil {
			t.Fatal(err)
		}
	}
	db.Close()

	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if v, _ := s.Version(ctx); v != 6 {
		t.Fatalf("version = %d, want 6", v)
	}
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM messages WHERE space = 'triage'`).Scan(&n); err != nil || n != 120 {
		t.Errorf("messages of the space triage = %d, %v, want 120", n, err)
	}
	ds, err := s.Dispatches(ctx, 10)
	if err != nil || len(ds) != 4 || ds[0].Space != "triage" || ds[0].Agent != "triage" {
		t.Errorf("dispatches = %+v, %v, want 4 with space and agent triage", ds, err)
	}
	if h, hash, err := s.Session(ctx, "triage"); err != nil || h != "term_a" || hash != "h1" {
		t.Errorf("session of triage = %q %q %v", h, hash, err)
	}
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM asks WHERE space = 'triage'`).Scan(&n); err != nil || n != 2 {
		t.Errorf("asks of triage = %d, %v, want 2", n, err)
	}

	old, err := sql.Open("sqlite", "file:"+path+".v5.bak?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer old.Close()
	var v int
	if err := old.QueryRow(`SELECT version FROM schema_version`).Scan(&v); err != nil || v != 5 {
		t.Errorf("backup version = %d, %v, want 5", v, err)
	}
	if err := old.QueryRow(`SELECT COUNT(*) FROM messages WHERE profile = 'triage'`).Scan(&n); err != nil || n != 120 {
		t.Errorf("backup messages = %d, %v, want 120 with the profile column", n, err)
	}
	if info, err := os.Stat(path + ".v5.bak"); err != nil || info.Mode().Perm() != 0o600 {
		t.Errorf("backup mode = %v, %v, want 0600", info, err)
	}
}

func TestPendingCounts(t *testing.T) {
	ctx := context.Background()
	s, err := Open(filepath.Join(t.TempDir(), "inbox.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	now := time.Now()
	msgs := []inbox.Message{
		{SourceID: "work", SourceType: "mattermost", ExternalID: "a", ChannelID: "C1", CreatedAt: now},
		{SourceID: "work", SourceType: "mattermost", ExternalID: "b", ChannelID: "C1", CreatedAt: now},
	}
	if _, err := s.SavePoll(ctx, "work", msgs, nil, now); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SavePoll(ctx, "phone", []inbox.Message{{SourceID: "phone", SourceType: "whatsapp", ExternalID: "c", ChannelID: "g@g.us", CreatedAt: now}}, nil, now); err != nil {
		t.Fatal(err)
	}
	got, err := s.PendingCounts(ctx)
	if err != nil || got[Conv{"work", "C1"}] != 2 || got[Conv{"phone", "g@g.us"}] != 1 || len(got) != 2 {
		t.Errorf("counts = %v, %v", got, err)
	}
}
