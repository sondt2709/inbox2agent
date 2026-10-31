package store

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sondt2709/inbox2agent/internal/inbox"
)

// schemaV2 is the schema of version 2 (before cards), as it was released.
const schemaV2 = `
CREATE TABLE schema_version (version INTEGER NOT NULL);
CREATE TABLE messages (
  id INTEGER PRIMARY KEY, source_id TEXT NOT NULL, source_type TEXT NOT NULL, external_id TEXT NOT NULL,
  channel_id TEXT NOT NULL, channel_name TEXT NOT NULL, channel_kind TEXT NOT NULL, author TEXT NOT NULL,
  text TEXT NOT NULL, url TEXT NOT NULL, created_at INTEGER NOT NULL, collected_at INTEGER NOT NULL,
  profile TEXT NOT NULL DEFAULT '', dispatch_id INTEGER, state TEXT NOT NULL DEFAULT 'pending',
  UNIQUE(source_id, external_id)
);
CREATE INDEX messages_state ON messages(state, created_at);
CREATE TABLE cursors (source_id TEXT NOT NULL, key TEXT NOT NULL, value TEXT NOT NULL, PRIMARY KEY(source_id, key));
CREATE TABLE dispatches (
  id INTEGER PRIMARY KEY, profile TEXT NOT NULL, driver TEXT NOT NULL, created_at INTEGER NOT NULL,
  status TEXT NOT NULL, digest_path TEXT NOT NULL, message_count INTEGER NOT NULL, detail TEXT NOT NULL DEFAULT ''
);
CREATE TABLE sessions (profile TEXT PRIMARY KEY, handle TEXT NOT NULL, created_at INTEGER NOT NULL);
INSERT INTO schema_version(version) VALUES (2);
`

func TestMigrateV2WritesBackupAndKeepsData(t *testing.T) {
	path := filepath.Join(t.TempDir(), "inbox.db")
	db, err := sql.Open("sqlite", "file:"+path+"?_pragma=journal_mode(WAL)")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(schemaV2); err != nil {
		t.Fatal(err)
	}
	for i := range 120 {
		if _, err := db.Exec(`INSERT INTO messages(source_id, source_type, external_id, channel_id, channel_name, channel_kind,
		  author, text, url, created_at, collected_at, state) VALUES ('work', 'mattermost', ?, 'c', 'general', 'channel',
		  'bob', 'hi', '', 1, 1, 'sent')`, i); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(`INSERT INTO sessions(profile, handle, created_at) VALUES ('triage', 'term_old', 1)`); err != nil {
		t.Fatal(err)
	}
	db.Close()

	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	if v, _ := s.Version(ctx); v != SchemaVersion {
		t.Errorf("version = %d, want %d", v, SchemaVersion)
	}
	if counts, _ := s.Counts(ctx); counts[inbox.StateSent] != 120 {
		t.Errorf("counts = %v, want 120 sent", counts)
	}
	// A version 2 session has no primer hash, so the driver starts a new session one time.
	if h, ph, err := s.Session(ctx, "triage"); err != nil || h != "term_old" || ph != "" {
		t.Errorf("session = %q %q %v, want term_old with no hash", h, ph, err)
	}
	if _, err := s.SaveCards(ctx, []inbox.Card{{Category: inbox.CategoryInfo, Urgency: inbox.UrgencyLow, Title: "t"}}, time.Now()); err != nil {
		t.Errorf("save a card after the upgrade: %v", err)
	}

	bak := path + ".v2.bak"
	info, err := os.Stat(bak)
	if err != nil {
		t.Fatalf("backup: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("backup mode = %o, want 600", info.Mode().Perm())
	}
	old, err := sql.Open("sqlite", "file:"+bak+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer old.Close()
	var v, n int
	if err := old.QueryRow(`SELECT version FROM schema_version`).Scan(&v); err != nil || v != 2 {
		t.Errorf("backup version = %d, %v, want 2", v, err)
	}
	if err := old.QueryRow(`SELECT COUNT(*) FROM messages`).Scan(&n); err != nil || n != 120 {
		t.Errorf("backup messages = %d, %v, want 120", n, err)
	}
}

func TestNewStoreHasNoBackup(t *testing.T) {
	_, path := open(t)
	if _, err := os.Stat(path + ".v2.bak"); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("backup of a new store: %v, want none", err)
	}
}

func card(key, urgency, title string) inbox.Card {
	return inbox.Card{Key: key, Category: inbox.CategoryQuestion, Urgency: urgency, Group: "Harbor Portal", Title: title,
		Body: "body", Reply: "reply", Refs: []int64{1, 2}}
}

func TestSaveCardsUpsertByKey(t *testing.T) {
	s, _ := open(t)
	ctx := context.Background()
	t0 := time.Date(2026, 10, 29, 9, 0, 0, 0, time.UTC)

	saved, err := s.SaveCards(ctx, []inbox.Card{card("mm:pr-57", inbox.UrgencyNormal, "Review PR #57"), card("", inbox.UrgencyLow, "no key")}, t0)
	if err != nil {
		t.Fatal(err)
	}
	if saved[0].Result != inbox.SaveAdded || saved[1].Result != inbox.SaveAdded || saved[0].ID == 0 {
		t.Fatalf("first save = %+v", saved)
	}
	if err := s.SetCardState(ctx, saved[0].ID, "", inbox.CardRead, t0); err != nil {
		t.Fatal(err)
	}

	// The same card again stays read.
	again, err := s.SaveCards(ctx, []inbox.Card{card("mm:pr-57", inbox.UrgencyNormal, "Review PR #57")}, t0.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if again[0].Result != inbox.SaveUnchanged || again[0].State != inbox.CardRead {
		t.Errorf("same card = %s %s, want unchanged and read", again[0].Result, again[0].State)
	}

	// A higher urgency updates the card and makes it new again.
	up, err := s.SaveCards(ctx, []inbox.Card{card("mm:pr-57", inbox.UrgencyHigh, "Review PR #57 before 18:30")}, t0.Add(2*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if up[0].Result != inbox.SaveUpdated || up[0].PrevUrgency != inbox.UrgencyNormal || up[0].PrevState != inbox.CardRead || up[0].ID != saved[0].ID {
		t.Errorf("update = %+v", up[0])
	}

	// A second card without a key is a new card.
	if _, err := s.SaveCards(ctx, []inbox.Card{card("", inbox.UrgencyLow, "no key")}, t0); err != nil {
		t.Fatal(err)
	}
	all, err := s.Cards(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 3 {
		t.Fatalf("cards = %d, want 3 (one for the key, two without a key)", len(all))
	}
	got := all[0] // the newest update first
	if got.Key != "mm:pr-57" || got.Urgency != inbox.UrgencyHigh || got.State != inbox.CardNew || got.Title != "Review PR #57 before 18:30" {
		t.Errorf("updated card = %+v", got)
	}
	if len(got.Refs) != 2 || got.Refs[0] != 1 {
		t.Errorf("refs = %v", got.Refs)
	}
}

func TestSetCardStateByKeyAndUnknown(t *testing.T) {
	s, _ := open(t)
	ctx := context.Background()
	if _, err := s.SaveCards(ctx, []inbox.Card{card("k1", inbox.UrgencyNormal, "a")}, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := s.SetCardState(ctx, 0, "k1", inbox.CardDone, time.Now()); err != nil {
		t.Fatal(err)
	}
	open, _ := s.Cards(ctx, false)
	if len(open) != 0 {
		t.Errorf("open cards = %d, want 0", len(open))
	}
	if err := s.SetCardState(ctx, 0, "missing", inbox.CardDone, time.Now()); !errors.Is(err, ErrNoCard) {
		t.Errorf("unknown key: %v, want ErrNoCard", err)
	}
	if err := s.SetCardState(ctx, 999, "", inbox.CardDone, time.Now()); !errors.Is(err, ErrNoCard) {
		t.Errorf("unknown id: %v, want ErrNoCard", err)
	}
}

func TestPruneDeletesOldDoneCardsOnly(t *testing.T) {
	s, _ := open(t)
	ctx := context.Background()
	now := time.Now()
	old := now.Add(-8 * 24 * time.Hour)
	saved, err := s.SaveCards(ctx, []inbox.Card{card("done-old", inbox.UrgencyLow, "a"), card("read-old", inbox.UrgencyLow, "b"),
		card("new-old", inbox.UrgencyLow, "c"), card("done-new", inbox.UrgencyLow, "d")}, old.Add(-30*24*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	_ = s.SetCardState(ctx, saved[0].ID, "", inbox.CardDone, old)
	_ = s.SetCardState(ctx, saved[1].ID, "", inbox.CardRead, old)
	_ = s.SetCardState(ctx, saved[3].ID, "", inbox.CardDone, now)

	if _, _, err := s.Prune(ctx, now.Add(-7*24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	all, _ := s.Cards(ctx, true)
	keys := map[string]bool{}
	for _, c := range all {
		keys[c.Key] = true
	}
	if keys["done-old"] || !keys["read-old"] || !keys["new-old"] || !keys["done-new"] {
		t.Errorf("cards after prune = %v, want read-old, new-old and done-new", keys)
	}
}

func TestMessagesByIDKeepsOrderAndSkipsDeleted(t *testing.T) {
	s, _ := open(t)
	ctx := context.Background()
	if _, err := s.SavePoll(ctx, "work", []inbox.Message{msg("p1", time.Now()), msg("p2", time.Now())}, nil, time.Now()); err != nil {
		t.Fatal(err)
	}
	p, _ := s.Pending(ctx)
	got, err := s.MessagesByID(ctx, []int64{p[1].ID, 999, p[0].ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].ID != p[1].ID || got[1].ID != p[0].ID {
		t.Errorf("MessagesByID = %+v", got)
	}
}

func TestCardGetsTheSpaceOfTheSessionOfItsTerminal(t *testing.T) {
	ctx := context.Background()
	s, err := Open(filepath.Join(t.TempDir(), "inbox.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	now := time.Now()
	if err := s.SetSession(ctx, "team", "term_team", "h", now); err != nil {
		t.Fatal(err)
	}
	saved, err := s.SaveCards(ctx, []inbox.Card{
		{Category: inbox.CategoryInfo, Urgency: inbox.UrgencyLow, Title: "from team", Key: "k1", Handle: "term_team"},
		{Category: inbox.CategoryInfo, Urgency: inbox.UrgencyLow, Title: "from elsewhere", Handle: "term_other"},
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	if saved[0].Space != "team" || saved[1].Space != "" {
		t.Fatalf("spaces = %q, %q; want team and none", saved[0].Space, saved[1].Space)
	}
	// An update by key from an unknown terminal keeps the space.
	if _, err := s.SaveCards(ctx, []inbox.Card{{Category: inbox.CategoryInfo, Urgency: inbox.UrgencyHigh, Title: "from team",
		Key: "k1", Handle: "term_other"}}, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	c, err := s.Card(ctx, 0, "k1")
	if err != nil || c.Space != "team" {
		t.Errorf("card after the update = %+v, %v; want the space team", c, err)
	}
}
