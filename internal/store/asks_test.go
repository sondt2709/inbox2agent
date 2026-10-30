package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/sondt2709/inbox2agent/internal/inbox"
)

// schemaV4 is the schema of version 4 (before asks), as it was released.
const schemaV4 = `
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
  done_at    INTEGER
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
INSERT INTO schema_version(version) VALUES (4);
`

func TestMigrateV4WritesBackupAndKeepsData(t *testing.T) {
	path := filepath.Join(t.TempDir(), "inbox.db")
	db, err := sql.Open("sqlite", "file:"+path+"?_pragma=journal_mode(WAL)")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(schemaV4); err != nil {
		t.Fatal(err)
	}
	for i := range 120 {
		if _, err := db.Exec(`INSERT INTO messages(source_id, source_type, external_id, channel_id, channel_name, channel_kind,
		  author, text, url, created_at, collected_at, profile, state) VALUES ('work', 'mattermost', ?, 'c', 'general', 'channel',
		  'bob', 'hi', '', 1, 1, 'triage', 'sent')`, i); err != nil {
			t.Fatal(err)
		}
	}
	for i := range 5 {
		if _, err := db.Exec(`INSERT INTO cards(category, urgency, title, created_at, updated_at) VALUES ('info', 'low', ?, 1, 1)`, i); err != nil {
			t.Fatal(err)
		}
	}
	for i := range 3 {
		if _, err := db.Exec(`INSERT INTO replies(message_id, source_id, channel_name, place, text, by, state, pending_id, created_at)
		  VALUES (1, 'work', 'general', 'thread', ?, 'user', 'sent', 'p', 1)`, i); err != nil {
			t.Fatal(err)
		}
	}
	db.Close()

	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	if v, _ := s.Version(ctx); v != SchemaVersion {
		t.Errorf("version = %d, want 5", v)
	}
	if counts, _ := s.Counts(ctx); counts[inbox.StateSent] != 120 {
		t.Errorf("counts = %v, want 120 sent", counts)
	}
	cards, _ := s.Cards(ctx, true)
	if len(cards) != 5 || cards[0].Handle != "" {
		t.Errorf("cards = %+v, want 5 cards without a handle", cards)
	}
	if rs, _ := s.SentReplies(ctx, []int64{0}); len(rs) != 0 {
		t.Errorf("replies of no card = %v", rs)
	}
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM replies`).Scan(&n); err != nil || n != 3 {
		t.Errorf("replies = %d, %v, want 3", n, err)
	}
	msgs, err := s.MessagesByID(ctx, []int64{1})
	if err != nil || len(msgs) != 1 || msgs[0].Space != "triage" {
		t.Errorf("message 1 = %+v, %v, want the space triage", msgs, err)
	}
	if _, err := s.StartAsk(ctx, inbox.Ask{CardID: cards[0].ID, Space: "triage", Text: "more", CreatedAt: time.Now()}); err != nil {
		t.Errorf("save an ask after the upgrade: %v", err)
	}

	old, err := sql.Open("sqlite", "file:"+path+".v4.bak?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer old.Close()
	var v int
	if err := old.QueryRow(`SELECT version FROM schema_version`).Scan(&v); err != nil || v != 4 {
		t.Errorf("backup version = %d, %v, want 4", v, err)
	}
	if err := old.QueryRow(`SELECT COUNT(*) FROM cards`).Scan(&n); err != nil || n != 5 {
		t.Errorf("backup cards = %d, %v, want 5", n, err)
	}
}

func TestCardHandle(t *testing.T) {
	s, _ := open(t)
	ctx := context.Background()
	now := time.Now()
	c := inbox.Card{Key: "mm:pr-57", Category: inbox.CategoryInfo, Urgency: inbox.UrgencyLow, Title: "t", Handle: "term_a"}
	if _, err := s.SaveCards(ctx, []inbox.Card{c}, now); err != nil {
		t.Fatal(err)
	}
	get := func() inbox.Card {
		t.Helper()
		got, err := s.Card(ctx, 0, "mm:pr-57")
		if err != nil {
			t.Fatal(err)
		}
		return got
	}
	if got := get(); got.Handle != "term_a" || got.State != inbox.CardNew {
		t.Fatalf("added card = %+v, want handle term_a", got)
	}
	if err := s.SetCardState(ctx, 0, "mm:pr-57", inbox.CardRead, now); err != nil {
		t.Fatal(err)
	}

	// Only the handle changes: the card stays unchanged and read.
	c.Handle = "term_b"
	saved, err := s.SaveCards(ctx, []inbox.Card{c}, now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if saved[0].Result != inbox.SaveUnchanged || saved[0].Handle != "term_b" {
		t.Errorf("saved = %+v, want unchanged with handle term_b", saved[0])
	}
	if got := get(); got.Handle != "term_b" || got.State != inbox.CardRead {
		t.Errorf("card = %+v, want handle term_b and read", got)
	}

	// An update without a handle keeps the stored handle.
	c.Handle, c.Title = "", "new title"
	saved, err = s.SaveCards(ctx, []inbox.Card{c}, now.Add(2*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if saved[0].Result != inbox.SaveUpdated || saved[0].Handle != "term_b" {
		t.Errorf("saved = %+v, want updated with handle term_b", saved[0])
	}
	if got := get(); got.Handle != "term_b" || got.Title != "new title" {
		t.Errorf("card = %+v, want handle term_b", got)
	}

	// An update with a handle replaces it.
	c.Handle, c.Title = "term_c", "third title"
	if _, err := s.SaveCards(ctx, []inbox.Card{c}, now.Add(3*time.Second)); err != nil {
		t.Fatal(err)
	}
	if got := get(); got.Handle != "term_c" {
		t.Errorf("card = %+v, want handle term_c", got)
	}
}

func TestAsks(t *testing.T) {
	s, _ := open(t)
	ctx := context.Background()
	now := time.Now()
	a1, err := s.StartAsk(ctx, inbox.Ask{CardID: 4, Space: "triage", Text: "research more", CreatedAt: now})
	if err != nil {
		t.Fatal(err)
	}
	a2, _ := s.StartAsk(ctx, inbox.Ask{CardID: 4, Space: "triage", Text: "and the effort", CreatedAt: now.Add(time.Second)})
	a3, _ := s.StartAsk(ctx, inbox.Ask{CardID: 5, Space: "triage", Text: "other", CreatedAt: now.Add(2 * time.Second)})

	q, err := s.QueuedAsks(ctx)
	if err != nil || len(q) != 3 || q[0].ID != a1 || q[0].State != inbox.AskQueued || q[0].Text != "research more" {
		t.Fatalf("queued = %+v, %v", q, err)
	}
	if err := s.FinishAsk(ctx, a1, inbox.AskSent, "term_a", "turn started", now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := s.FinishAsk(ctx, a3, inbox.AskFailed, "", "orca: not reachable", now); err != nil {
		t.Fatal(err)
	}
	if err := s.FinishAsk(ctx, a2, inbox.AskQueued, "", "agent busy", now); err != nil {
		t.Fatal(err)
	}
	q, _ = s.QueuedAsks(ctx)
	if len(q) != 1 || q[0].ID != a2 || q[0].Detail != "agent busy" {
		t.Errorf("queued = %+v, want only ask %d", q, a2)
	}
	by, err := s.Asks(ctx, []int64{4, 5, 6})
	if err != nil {
		t.Fatal(err)
	}
	if len(by[4]) != 2 || by[4][0].ID != a1 || by[4][0].Handle != "term_a" || by[4][0].SentAt.IsZero() || !by[4][1].SentAt.IsZero() {
		t.Errorf("asks of card 4 = %+v", by[4])
	}
	if len(by[5]) != 1 || by[5][0].State != inbox.AskFailed || len(by[6]) != 0 {
		t.Errorf("asks = %+v", by)
	}

	if _, _, err := s.Prune(ctx, now.Add(1500*time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	by, _ = s.Asks(ctx, []int64{4, 5})
	if len(by[4]) != 0 || len(by[5]) != 1 {
		t.Errorf("after prune = %+v, want only the ask of card 5", by)
	}
}
