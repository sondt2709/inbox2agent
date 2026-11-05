package store

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sondt2709/inbox2agent/internal/inbox"
)

// dropV9 removes the columns of schema version 9.
var dropV9 = []string{`ALTER TABLE messages DROP COLUMN reply_ext`, `ALTER TABLE messages DROP COLUMN reply_author`,
	`ALTER TABLE messages DROP COLUMN reply_me`, `ALTER TABLE messages DROP COLUMN reply_text`}

// v8Store writes a store of schema version 8 with 10 messages.
func v8Store(t *testing.T, path string) {
	t.Helper()
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	var msgs []inbox.Message
	for _, id := range strings.Split("a b c d e f g h i j", " ") {
		msgs = append(msgs, msg(id, now.Add(-time.Minute)))
	}
	if _, err := s.SavePoll(context.Background(), "work", msgs, nil, now); err != nil {
		t.Fatal(err)
	}
	for _, q := range append(dropV9, `UPDATE schema_version SET version = 8`) {
		if _, err := s.db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	s.Close()
}

func TestMigrateV8AddsTheReplyReference(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "inbox.db")
	v8Store(t, path)
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if v, _ := s.Version(ctx); v != 9 {
		t.Fatalf("version = %d, want 9", v)
	}
	p, err := s.Pending(ctx)
	if err != nil || len(p) != 10 {
		t.Fatalf("pending = %d messages, %v, want 10", len(p), err)
	}
	for _, m := range p {
		if m.ReplyTo != nil {
			t.Errorf("message %s has the reply reference %+v, want none", m.ExternalID, m.ReplyTo)
		}
	}
	old, err := sql.Open("sqlite", "file:"+path+".v8.bak?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer old.Close()
	var v int
	if err := old.QueryRow(`SELECT version FROM schema_version`).Scan(&v); err != nil || v != 8 {
		t.Errorf("backup version = %d, %v, want 8", v, err)
	}
}

func TestMigrateV9FailureKeepsVersion8(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "inbox.db")
	v8Store(t, path)
	// A trigger blocks the version update, so the transaction rolls back.
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{`CREATE TRIGGER block BEFORE UPDATE ON schema_version WHEN NEW.version = 9
		BEGIN SELECT RAISE(ABORT, 'blocked'); END`} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	db.Close()
	_, err = Open(path)
	if err == nil || !strings.Contains(err.Error(), path+".v8.bak") {
		t.Fatalf("err = %v, want the backup path", err)
	}
	db, _ = sql.Open("sqlite", "file:"+path+"?mode=ro")
	defer db.Close()
	var v int
	if err := db.QueryRowContext(ctx, `SELECT version FROM schema_version`).Scan(&v); err != nil || v != 8 {
		t.Errorf("version = %d, %v, want 8", v, err)
	}
	var n int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM pragma_table_info('messages') WHERE name LIKE 'reply_%'`).Scan(&n); err != nil || n != 0 {
		t.Errorf("reply columns = %d, %v, want 0 after the rollback", n, err)
	}
}

func TestNewStoreHasVersion9AndNoBackup(t *testing.T) {
	s, path := open(t)
	if v, _ := s.Version(context.Background()); v != 9 {
		t.Errorf("version = %d, want 9", v)
	}
	if _, err := os.Stat(path + ".v8.bak"); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("backup stat = %v, want no backup for a new store", err)
	}
}

func TestReplyReferenceFindsTheRowID(t *testing.T) {
	s, _ := open(t)
	ctx := context.Background()
	now := time.Now()
	root := msg("abc", now.Add(-2*time.Minute))
	answer := msg("def", now.Add(-time.Minute))
	answer.ReplyTo = &inbox.ReplyRef{Ext: "abc", Author: "alice", Text: "deploy at 5?"}
	other := msg("xyz", now.Add(-time.Minute))
	other.SourceID = "work2"
	other.ExternalID = "ghi"
	other.ReplyTo = &inbox.ReplyRef{Ext: "abc", Me: true, Text: "mine"} // the same external ID in another account
	if _, err := s.SavePoll(ctx, "work", []inbox.Message{root, answer}, nil, now); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SavePoll(ctx, "work2", []inbox.Message{other}, nil, now); err != nil {
		t.Fatal(err)
	}
	p, err := s.Pending(ctx)
	if err != nil || len(p) != 3 {
		t.Fatalf("pending = %+v, %v", p, err)
	}
	by := map[string]inbox.Message{}
	for _, m := range p {
		by[m.ExternalID] = m
	}
	if by["abc"].ReplyTo != nil {
		t.Errorf("root has the reply reference %+v", by["abc"].ReplyTo)
	}
	want := inbox.ReplyRef{Ext: "abc", ID: by["abc"].ID, Author: "alice", Text: "deploy at 5?"}
	if r := by["def"].ReplyTo; r == nil || *r != want {
		t.Errorf("reply reference = %+v, want %+v", r, want)
	}
	if r := by["ghi"].ReplyTo; r == nil || *r != (inbox.ReplyRef{Ext: "abc", Me: true, Text: "mine"}) {
		t.Errorf("reply reference of another account = %+v, want ID 0", r)
	}
	got, err := s.MessagesByID(ctx, []int64{by["def"].ID})
	if err != nil || len(got) != 1 || got[0].ReplyTo == nil || got[0].ReplyTo.ID != by["abc"].ID {
		t.Errorf("MessagesByID = %+v, %v, want the row ID of the root", got, err)
	}
}

func TestReplyReferenceAfterThePrune(t *testing.T) {
	s, _ := open(t)
	ctx := context.Background()
	now := time.Now()
	root := msg("abc", now.Add(-48*time.Hour))
	answer := msg("def", now.Add(-time.Minute))
	answer.ReplyTo = &inbox.ReplyRef{Ext: "abc", Author: "alice", Text: "deploy at 5?"}
	if _, err := s.SavePoll(ctx, "work", []inbox.Message{root, answer}, nil, now); err != nil {
		t.Fatal(err)
	}
	p, _ := s.Pending(ctx)
	if err := s.SetStates(ctx, map[int64]string{p[0].ID: inbox.StateSent}); err != nil {
		t.Fatal(err)
	}
	if n, _, err := s.Prune(ctx, now.Add(-24*time.Hour)); err != nil || n != 1 {
		t.Fatalf("prune = %d, %v, want 1", n, err)
	}
	p, _ = s.Pending(ctx)
	if len(p) != 1 || p[0].ReplyTo == nil || *p[0].ReplyTo != (inbox.ReplyRef{Ext: "abc", Author: "alice", Text: "deploy at 5?"}) {
		t.Errorf("pending = %+v, want the reference with ID 0", p)
	}
}
