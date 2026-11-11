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

// v9Store writes a store of schema version 9 with 2 sessions.
func v9Store(t *testing.T, path string) {
	t.Helper()
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	at := time.UnixMilli(1_700_000_000_000).UTC()
	for _, ss := range []inbox.Session{
		{Space: "triage", Handle: "term_a", AgentSession: "04ebe19c-0000-4000-8000-000000000001", CreatedAt: at},
		{Space: "unsorted", Handle: "term_b", CreatedAt: at},
	} {
		if err := s.SetSession(ctx, ss); err != nil {
			t.Fatal(err)
		}
	}
	for _, col := range []string{"agent_name", "last_sent_at"} {
		var n int
		if err := s.db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('sessions') WHERE name = ?`, col).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n == 1 {
			if _, err := s.db.Exec(`ALTER TABLE sessions DROP COLUMN ` + col); err != nil {
				t.Fatal(err)
			}
		}
	}
	if _, err := s.db.Exec(`UPDATE schema_version SET version = 9`); err != nil {
		t.Fatal(err)
	}
	s.Close()
}

func TestMigrateV9AddsTheSessionNameAndLastSend(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "inbox.db")
	v9Store(t, path)
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if v, _ := s.Version(ctx); v != 10 {
		t.Fatalf("version = %d, want 10", v)
	}
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM sessions WHERE agent_name = '' AND last_sent_at = 0`).Scan(&n); err != nil || n != 2 {
		t.Fatalf("sessions with no name and no last send = %d, %v, want 2", n, err)
	}
	ss, err := s.Session(ctx, "triage")
	if err != nil || ss.Handle != "term_a" || ss.AgentSession != "04ebe19c-0000-4000-8000-000000000001" {
		t.Errorf("triage session = %+v, %v", ss, err)
	}
	old, err := sql.Open("sqlite", "file:"+path+".v9.bak?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer old.Close()
	var v int
	if err := old.QueryRow(`SELECT version FROM schema_version`).Scan(&v); err != nil || v != 9 {
		t.Errorf("backup version = %d, %v, want 9", v, err)
	}
}

func TestNewStoreHasVersion10AndNoBackup(t *testing.T) {
	s, path := open(t)
	if v, _ := s.Version(context.Background()); v != 10 {
		t.Errorf("version = %d, want 10", v)
	}
	if _, err := os.Stat(path + ".v9.bak"); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("backup stat = %v, want no backup for a new store", err)
	}
}

func TestTouchSessionRecordsTheLastSendAndTheName(t *testing.T) {
	ctx := context.Background()
	s, _ := open(t)
	at := time.UnixMilli(1_700_000_000_000).UTC()
	if err := s.SetSession(ctx, inbox.Session{Space: "triage", Handle: "term_a", AgentSession: "S", Name: "i2a · Triage", CreatedAt: at}); err != nil {
		t.Fatal(err)
	}
	ss, _ := s.Session(ctx, "triage")
	if ss.Name != "i2a · Triage" || !ss.LastSent.IsZero() {
		t.Fatalf("stored = %+v, want the name and no last send", ss)
	}
	sent := at.Add(time.Hour)
	if err := s.TouchSession(ctx, "triage", "term_a", "review", sent); err != nil {
		t.Fatal(err)
	}
	// Another handle: the row of the space has a newer session, so nothing changes.
	if err := s.TouchSession(ctx, "triage", "term_old", "old", sent.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	// An empty name keeps the stored name.
	if err := s.TouchSession(ctx, "triage", "term_a", "", sent.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	all, err := s.Sessions(ctx)
	if err != nil || len(all) != 1 {
		t.Fatalf("sessions = %+v, %v", all, err)
	}
	if got := all[0]; got.Name != "review" || !got.LastSent.Equal(sent.Add(time.Minute)) || got.AgentSession != "S" {
		t.Errorf("session = %+v, want the name review and the last send %v", got, sent.Add(time.Minute))
	}
}
