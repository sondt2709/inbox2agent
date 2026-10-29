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

// schemaV3 is the schema of version 3 (before replies), as it was released.
const schemaV3 = `
CREATE TABLE IF NOT EXISTS schema_version (version INTEGER NOT NULL);
CREATE TABLE IF NOT EXISTS messages (
  id            INTEGER PRIMARY KEY,
  source_id     TEXT NOT NULL,
  source_type   TEXT NOT NULL,
  external_id   TEXT NOT NULL,
  channel_id    TEXT NOT NULL,
  channel_name  TEXT NOT NULL,
  channel_kind  TEXT NOT NULL,
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
INSERT INTO schema_version(version) VALUES (3);
`

func TestMigrateV3WritesBackupAndKeepsData(t *testing.T) {
	path := filepath.Join(t.TempDir(), "inbox.db")
	db, err := sql.Open("sqlite", "file:"+path+"?_pragma=journal_mode(WAL)")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(schemaV3); err != nil {
		t.Fatal(err)
	}
	for i := range 120 {
		if _, err := db.Exec(`INSERT INTO messages(source_id, source_type, external_id, channel_id, channel_name, channel_kind,
		  author, text, url, created_at, collected_at, state) VALUES ('work', 'mattermost', ?, 'c', 'general', 'channel',
		  'bob', 'hi', '', 1, 1, 'sent')`, i); err != nil {
			t.Fatal(err)
		}
	}
	for i := range 5 {
		if _, err := db.Exec(`INSERT INTO cards(category, urgency, title, created_at, updated_at) VALUES ('info', 'low', ?, 1, 1)`, i); err != nil {
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
	if v, _ := s.Version(ctx); v != 4 {
		t.Errorf("version = %d, want 4", v)
	}
	if counts, _ := s.Counts(ctx); counts[inbox.StateSent] != 120 {
		t.Errorf("counts = %v, want 120 sent", counts)
	}
	if cards, _ := s.Cards(ctx, true); len(cards) != 5 {
		t.Errorf("cards = %d, want 5", len(cards))
	}
	msgs, err := s.MessagesByID(ctx, []int64{1})
	if err != nil || len(msgs) != 1 || msgs[0].ThreadID != "" {
		t.Errorf("message 1 = %+v, %v, want an empty thread ID", msgs, err)
	}
	if _, err := s.StartReply(ctx, inbox.Reply{MessageID: 1, SourceID: "work", Place: inbox.PlaceThread, Text: "ok",
		By: inbox.ByUser, PendingID: "u:1", CreatedAt: time.Now()}); err != nil {
		t.Errorf("save a reply after the upgrade: %v", err)
	}

	bak := path + ".v3.bak"
	old, err := sql.Open("sqlite", "file:"+bak+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer old.Close()
	var v, n int
	if err := old.QueryRow(`SELECT version FROM schema_version`).Scan(&v); err != nil || v != 3 {
		t.Errorf("backup version = %d, %v, want 3", v, err)
	}
	if err := old.QueryRow(`SELECT COUNT(*) FROM messages`).Scan(&n); err != nil || n != 120 {
		t.Errorf("backup messages = %d, %v, want 120", n, err)
	}
	if info, err := os.Stat(bak); err != nil || info.Mode().Perm() != 0o600 {
		t.Errorf("backup = %v, %v, want mode 600", info, err)
	}
}

func TestThreadIDRoundTrip(t *testing.T) {
	s, _ := open(t)
	ctx := context.Background()
	now := time.Now()
	m := msg("p2", now)
	m.ThreadID = "root1"
	if _, err := s.SavePoll(ctx, "work", []inbox.Message{msg("p1", now), m}, nil, now); err != nil {
		t.Fatal(err)
	}
	got, err := s.MessagesByID(ctx, []int64{1, 2})
	if err != nil || len(got) != 2 || got[0].ThreadID != "" || got[1].ThreadID != "root1" {
		t.Errorf("thread IDs = %+v, %v, want \"\" and root1", got, err)
	}
}

func reply(msgID, cardID int64, text string, created time.Time) inbox.Reply {
	return inbox.Reply{MessageID: msgID, CardID: cardID, SourceID: "work", ChannelName: "pr-reviews", Place: inbox.PlaceThread,
		Text: text, By: inbox.ByUser, PendingID: "u:1", CreatedAt: created}
}

func TestReplyStatesAndFind(t *testing.T) {
	s, _ := open(t)
	ctx := context.Background()
	now := time.Now()
	id, err := s.StartReply(ctx, reply(812, 3, "Thanks Minh", now))
	if err != nil {
		t.Fatal(err)
	}
	r, ok, err := s.FindReply(ctx, 812, inbox.PlaceThread, "Thanks Minh")
	if err != nil || !ok || r.ID != id || r.State != inbox.ReplySending || r.CardID != 3 || r.PendingID != "u:1" {
		t.Fatalf("find = %+v %v %v, want the sending reply", r, ok, err)
	}
	for _, miss := range []struct {
		place, text string
	}{{inbox.PlaceChannel, "Thanks Minh"}, {inbox.PlaceThread, "Thanks Minh!"}} {
		if _, ok, _ := s.FindReply(ctx, 812, miss.place, miss.text); ok {
			t.Errorf("find %q in %s: found, want none", miss.text, miss.place)
		}
	}
	if err := s.FinishReply(ctx, id, inbox.ReplySent, "post1", "https://chat/pl/post1", "", now); err != nil {
		t.Fatal(err)
	}
	r, _, _ = s.FindReply(ctx, 812, inbox.PlaceThread, "Thanks Minh")
	if r.State != inbox.ReplySent || r.ExternalID != "post1" || r.SentAt.IsZero() {
		t.Errorf("after finish = %+v, want sent with post1 and a send time", r)
	}
	if _, err := s.StartReply(ctx, reply(900, 0, "no card", now)); err != nil {
		t.Fatal(err)
	}
	failed, _ := s.StartReply(ctx, reply(812, 3, "failed one", now))
	if err := s.FinishReply(ctx, failed, inbox.ReplyFailed, "", "", "HTTP 400", now); err != nil {
		t.Fatal(err)
	}
	byCard, err := s.SentReplies(ctx, []int64{3, 4})
	if err != nil || len(byCard[3]) != 1 || byCard[3][0].Text != "Thanks Minh" || len(byCard[4]) != 0 {
		t.Errorf("sent replies = %+v, %v, want only the sent reply of card 3", byCard, err)
	}
}

func TestPruneDeletesOldReplies(t *testing.T) {
	s, _ := open(t)
	ctx := context.Background()
	now := time.Now()
	if _, err := s.StartReply(ctx, reply(1, 0, "old", now.Add(-8*24*time.Hour))); err != nil {
		t.Fatal(err)
	}
	if _, err := s.StartReply(ctx, reply(1, 0, "new", now)); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.Prune(ctx, now.Add(-7*24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := s.FindReply(ctx, 1, inbox.PlaceThread, "old"); ok {
		t.Error("the 8 day old reply still exists")
	}
	if _, ok, _ := s.FindReply(ctx, 1, inbox.PlaceThread, "new"); !ok {
		t.Error("the new reply is deleted")
	}
}

func TestCardByIDAndKey(t *testing.T) {
	s, _ := open(t)
	ctx := context.Background()
	saved, err := s.SaveCards(ctx, []inbox.Card{card("mm:pr-57", inbox.UrgencyNormal, "Review PR 57")}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if c, err := s.Card(ctx, saved[0].ID, ""); err != nil || c.Key != "mm:pr-57" {
		t.Errorf("by ID = %+v, %v", c, err)
	}
	if c, err := s.Card(ctx, 0, "mm:pr-57"); err != nil || c.ID != saved[0].ID {
		t.Errorf("by key = %+v, %v", c, err)
	}
	if _, err := s.Card(ctx, 0, "missing"); err != ErrNoCard {
		t.Errorf("missing key: %v, want ErrNoCard", err)
	}
}
