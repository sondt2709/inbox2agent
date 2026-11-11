package store

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
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
	if v, _ := s.Version(ctx); v != SchemaVersion {
		t.Fatalf("version = %d, want %d", v, SchemaVersion)
	}
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM messages WHERE space = 'triage'`).Scan(&n); err != nil || n != 120 {
		t.Errorf("messages of the space triage = %d, %v, want 120", n, err)
	}
	ds, err := s.Dispatches(ctx, 10)
	if err != nil || len(ds) != 4 || ds[0].Space != "triage" || ds[0].Agent != "triage" {
		t.Errorf("dispatches = %+v, %v, want 4 with space and agent triage", ds, err)
	}
	if ss, err := s.Session(ctx, "triage"); err != nil || ss.Handle != "term_a" {
		t.Errorf("session of triage = %q %v", ss.Handle, err)
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

// schemaV6 is the schema of i2a with store version 6 (internal/store/store.go
// before the deliveries), with its version row.
const schemaV6 = `
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
INSERT INTO schema_version(version) VALUES (6);
`

func TestMigrateV6AddsTheDeliveries(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "inbox.db")
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(schemaV6); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO dispatches(space, agent, driver, created_at, status, digest_path, message_count)
	  VALUES ('triage', 'triage', 'orca', 1, 'sent', '', 3)`); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 4; i++ {
		space, state, dispatch := "triage", "sent", any(1)
		if i == 3 {
			space, state, dispatch = "", "pending", nil
		}
		if _, err := db.Exec(`INSERT INTO messages(source_id, source_type, external_id, channel_id, channel_name, channel_kind,
		  author, text, url, created_at, collected_at, space, dispatch_id, state) VALUES ('work', 'mattermost', ?, 'c', 'general', 'channel',
		  'bob', 'hi', '', 1, 1, ?, ?, ?)`, i, space, dispatch, state); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(`INSERT INTO cards(category, urgency, title, created_at, updated_at, handle) VALUES ('info', 'low', 'old', 1, 1, 'term_a')`); err != nil {
		t.Fatal(err)
	}
	db.Close()

	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if v, _ := s.Version(ctx); v != SchemaVersion {
		t.Fatalf("version = %d, want %d", v, SchemaVersion)
	}
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM deliveries WHERE space = 'triage' AND state = 'sent' AND dispatch_id = 1`).Scan(&n); err != nil || n != 3 {
		t.Errorf("sent deliveries of triage = %d, %v, want 3", n, err)
	}
	p, err := s.Pending(ctx)
	if err != nil || len(p) != 1 || p[0].Delivered != nil || !p[0].ForMe {
		t.Errorf("pending = %+v, %v, want one message with no delivery and the mark 1 of an old message", p, err)
	}
	cs, err := s.Cards(ctx, true)
	if err != nil || len(cs) != 1 || cs[0].Space != "" || cs[0].Handle != "term_a" {
		t.Errorf("cards = %+v, %v, want the old card with no space", cs, err)
	}
	old, err := sql.Open("sqlite", "file:"+path+".v6.bak?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer old.Close()
	var v int
	if err := old.QueryRow(`SELECT version FROM schema_version`).Scan(&v); err != nil || v != 6 {
		t.Errorf("backup version = %d, %v, want 6", v, err)
	}
}

func TestRequeueKeepsTheDeliveries(t *testing.T) {
	s, _ := open(t)
	ctx := context.Background()
	now := time.Now()
	old, mine, other := msg("r0", now.Add(-48*time.Hour)), msg("r1", now.Add(-time.Hour)), msg("r2", now.Add(-time.Hour))
	other.ForMe = false
	mine.ForMe = true
	if _, err := s.SavePoll(ctx, "work", []inbox.Message{old, mine, other}, nil, now); err != nil {
		t.Fatal(err)
	}
	p, _ := s.Pending(ctx)
	var ids []int64
	for _, m := range p {
		ids = append(ids, m.ID)
	}
	id, _ := s.CreateDispatch(ctx, Dispatch{Space: "triage", Agent: "triage", Driver: "orca", CreatedAt: now, MessageCount: 3})
	if err := s.FinishDispatch(ctx, id, DispatchSent, "", "triage", inbox.StateSent, ids); err != nil {
		t.Fatal(err)
	}
	if n, err := s.Requeue(ctx, "work", "c1", now.Add(-24*time.Hour), true, nil); err != nil || n != 1 {
		t.Fatalf("requeue of the messages for me = %d, %v, want 1", n, err)
	}
	if n, _ := s.Requeue(ctx, "work", "c1", now.Add(-24*time.Hour), false, nil); n != 1 {
		t.Fatalf("requeue of all messages = %d, want 1 more (the message for me is pending already)", n)
	}
	if n, _ := s.Requeue(ctx, "work", "other", now.Add(-24*time.Hour), false, nil); n != 0 {
		t.Fatalf("requeue of another conversation = %d", n)
	}
	got, _ := s.Pending(ctx)
	if len(got) != 2 {
		t.Fatalf("pending = %d, want the 2 messages after since", len(got))
	}
	for _, m := range got {
		if m.ExternalID == "r0" || m.Delivered["triage"] != inbox.StateSent {
			t.Errorf("message %s: deliveries %v, want the sent delivery of triage", m.ExternalID, m.Delivered)
		}
	}
}

// An old pending message that a space got stays sent; the others expire.
func TestExpireKeepsAMessageThatASpaceGot(t *testing.T) {
	s, _ := open(t)
	ctx := context.Background()
	now := time.Now()
	if _, err := s.SavePoll(ctx, "work", []inbox.Message{msg("got", now.Add(-time.Hour)), msg("lost", now.Add(-time.Hour))}, nil, now); err != nil {
		t.Fatal(err)
	}
	p, _ := s.Pending(ctx)
	id, _ := s.CreateDispatch(ctx, Dispatch{Space: "triage", Agent: "triage", Driver: "orca", CreatedAt: now, MessageCount: 1})
	if err := s.FinishDispatch(ctx, id, DispatchSent, "", "triage", inbox.StateSent, []int64{p[0].ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Requeue(ctx, "work", "c1", now.Add(-2*time.Hour), false, []string{"other"}); err != nil {
		t.Fatal(err)
	}
	if n, err := s.ExpirePending(ctx, now); err != nil || n != 1 {
		t.Fatalf("expired = %d, %v, want 1", n, err)
	}
	c, _ := s.Counts(ctx)
	if c[inbox.StateSent] != 1 || c[inbox.StateExpired] != 1 {
		t.Errorf("counts = %v, want 1 sent and 1 expired", c)
	}
	var skipped int
	if err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM deliveries WHERE state = 'skipped' AND space = 'other'`).Scan(&skipped); err != nil || skipped != 1 {
		t.Errorf("skipped deliveries = %d, %v, want 1: the requeued message", skipped, err)
	}
}

// A space that a requeue skipped gets the message in a later requeue that
// does not skip it.
func TestRequeueRemovesAnOldSkip(t *testing.T) {
	s, _ := open(t)
	ctx := context.Background()
	now := time.Now()
	if _, err := s.SavePoll(ctx, "work", []inbox.Message{msg("m", now.Add(-time.Hour))}, nil, now); err != nil {
		t.Fatal(err)
	}
	p, _ := s.Pending(ctx)
	id, _ := s.CreateDispatch(ctx, Dispatch{Space: "triage", Agent: "triage", Driver: "orca", CreatedAt: now, MessageCount: 1})
	if err := s.FinishDispatch(ctx, id, DispatchSent, "", "triage", inbox.StateSent, []int64{p[0].ID}); err != nil {
		t.Fatal(err)
	}
	for _, skip := range [][]string{{"review"}, {"team"}} {
		if err := s.SetStates(ctx, map[int64]string{p[0].ID: inbox.StateSent}); err != nil {
			t.Fatal(err)
		}
		if _, err := s.Requeue(ctx, "work", "c1", now.Add(-2*time.Hour), false, skip); err != nil {
			t.Fatal(err)
		}
	}
	got, _ := s.Pending(ctx)
	if len(got) != 1 || got[0].Delivered["review"] != "" || got[0].Delivered["team"] != inbox.DeliverySkipped || got[0].Delivered["triage"] != inbox.StateSent {
		t.Errorf("deliveries = %+v, want review free, team skipped, triage sent", got)
	}
}

// v7Store writes a store of schema version 7 with the session term_a of triage.
func v7Store(t *testing.T, path string) {
	t.Helper()
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetSession(context.Background(), inbox.Session{Space: "triage", Handle: "term_a", CreatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	for _, q := range append(dropV9, `ALTER TABLE sessions DROP COLUMN agent_session`, `ALTER TABLE cards DROP COLUMN agent_session`,
		`UPDATE schema_version SET version = 7`) {
		if _, err := s.db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	s.Close()
}

func TestMigrateV7AddsTheClaudeSessions(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "inbox.db")
	v7Store(t, path)
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if v, _ := s.Version(ctx); v != SchemaVersion {
		t.Fatalf("version = %d, want %d", v, SchemaVersion)
	}
	if ss, err := s.Session(ctx, "triage"); err != nil || ss.Handle != "term_a" || ss.AgentSession != "" {
		t.Errorf("session = %+v, %v, want term_a with no Claude session", ss, err)
	}
	saved, err := s.SaveCards(ctx, []inbox.Card{{Category: inbox.CategoryInfo, Urgency: inbox.UrgencyLow, Title: "t",
		Handle: "term_a", AgentSession: "04ebe19c-543e-42ff-8f84-ea13cea7a3ec"}}, time.Now())
	if err != nil || len(saved) != 1 || saved[0].Space != "triage" {
		t.Fatalf("saved = %+v, %v", saved, err)
	}
	if c, _ := s.Card(ctx, saved[0].ID, ""); c.AgentSession != "04ebe19c-543e-42ff-8f84-ea13cea7a3ec" {
		t.Errorf("card = %+v, want the Claude session", c)
	}
	old, err := sql.Open("sqlite", "file:"+path+".v7.bak?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer old.Close()
	var v int
	if err := old.QueryRow(`SELECT version FROM schema_version`).Scan(&v); err != nil || v != 7 {
		t.Errorf("backup version = %d, %v, want 7", v, err)
	}
}

func TestMigrateV8FailureKeepsVersion7(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "inbox.db")
	v7Store(t, path)
	// A view in the place of the table sessions makes ALTER TABLE fail.
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{`DROP TABLE sessions`, `CREATE VIEW sessions AS SELECT 'triage' AS space`} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	db.Close()
	_, err = Open(path)
	if err == nil || !strings.Contains(err.Error(), path+".v7.bak") {
		t.Fatalf("err = %v, want the backup path", err)
	}
	db, _ = sql.Open("sqlite", "file:"+path+"?mode=ro")
	defer db.Close()
	var v int
	if err := db.QueryRowContext(ctx, `SELECT version FROM schema_version`).Scan(&v); err != nil || v != 7 {
		t.Errorf("version = %d, %v, want 7", v, err)
	}
	var n int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM pragma_table_info('cards') WHERE name = 'agent_session'`).Scan(&n); err != nil || n != 0 {
		t.Errorf("cards.agent_session columns = %d, %v, want 0 after the rollback", n, err)
	}
}

func TestSaveCardsKeepsTheClaudeSessionWithItsHandle(t *testing.T) {
	s, _ := open(t)
	ctx := context.Background()
	now := time.Now()
	c := inbox.Card{Key: "k", Category: inbox.CategoryInfo, Urgency: inbox.UrgencyLow, Title: "t", Handle: "term_a", AgentSession: "s1"}
	if _, err := s.SaveCards(ctx, []inbox.Card{c}, now); err != nil {
		t.Fatal(err)
	}
	// The same card from another terminal of a non-Claude agent: no Claude session.
	c.Handle, c.AgentSession = "term_b", ""
	saved, err := s.SaveCards(ctx, []inbox.Card{c}, now)
	if err != nil || saved[0].Result != inbox.SaveUnchanged || saved[0].Handle != "term_b" || saved[0].AgentSession != "" {
		t.Errorf("saved = %+v, %v, want term_b with no Claude session", saved, err)
	}
	// The same terminal after /clear: a new Claude session for an unchanged card.
	c.AgentSession = "s3"
	saved, _ = s.SaveCards(ctx, []inbox.Card{c}, now)
	if got, _ := s.Card(ctx, saved[0].ID, ""); got.AgentSession != "s3" {
		t.Errorf("card = %+v, want the Claude session s3", got)
	}
	c.AgentSession = ""
	// A call with no terminal keeps both.
	c.Handle, c.AgentSession, c.Title = "", "s9", "t2"
	saved, _ = s.SaveCards(ctx, []inbox.Card{c}, now)
	if saved[0].Handle != "term_b" || saved[0].AgentSession != "s3" {
		t.Errorf("saved = %+v, want term_b and s3 kept", saved)
	}
}
