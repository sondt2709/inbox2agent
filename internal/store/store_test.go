package store

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sondt2709/inbox2agent/internal/inbox"
)

func open(t *testing.T) (*Store, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "inbox.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s, path
}

func msg(id string, created time.Time) inbox.Message {
	return inbox.Message{SourceID: "work", SourceType: "mattermost", ExternalID: id, ChannelID: "c1",
		ChannelName: "general", ChannelKind: inbox.KindChannel, Author: "bob", Text: "hi " + id, CreatedAt: created}
}

func TestOpenCreatesPrivateFileWithSchema(t *testing.T) {
	s, path := open(t)
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("mode = %o, want 600", info.Mode().Perm())
	}
	if v, err := s.Version(context.Background()); err != nil || v != SchemaVersion {
		t.Errorf("version = %d, %v", v, err)
	}
}

func TestDuplicateMessageKeepsOneRowAndState(t *testing.T) {
	s, _ := open(t)
	ctx := context.Background()
	now := time.Now()
	n, err := s.SavePoll(ctx, "work", []inbox.Message{msg("p1", now)}, inbox.Cursor{"c1": "1"}, now)
	if err != nil || n != 1 {
		t.Fatalf("first save: n=%d err=%v", n, err)
	}
	pending, _ := s.Pending(ctx)
	if err := s.FinishDispatch(ctx, 1, DispatchSent, "", "triage", inbox.StateSent, []int64{pending[0].ID}); err != nil {
		t.Fatal(err)
	}
	n, err = s.SavePoll(ctx, "work", []inbox.Message{msg("p1", now)}, inbox.Cursor{"c1": "2"}, now)
	if err != nil || n != 0 {
		t.Fatalf("second save: n=%d err=%v, want 0 new", n, err)
	}
	counts, _ := s.Counts(ctx)
	if counts[inbox.StateSent] != 1 || counts[inbox.StatePending] != 0 {
		t.Errorf("counts = %v, want 1 sent and 0 pending", counts)
	}
}

func TestSavePollIsAtomic(t *testing.T) {
	s, _ := open(t)
	ctx := context.Background()
	now := time.Now()
	bad := msg("p2", now)
	bad.SourceID = "other" // rejected inside the transaction
	_, err := s.SavePoll(ctx, "work", []inbox.Message{msg("p1", now), bad}, inbox.Cursor{"c1": "99"}, now)
	if err == nil {
		t.Fatal("SavePoll returned no error")
	}
	cur, _ := s.Cursor(ctx, "work")
	if len(cur) != 0 {
		t.Errorf("cursor = %v, want no change", cur)
	}
	pending, _ := s.Pending(ctx)
	if len(pending) != 0 {
		t.Errorf("pending = %d messages, want 0", len(pending))
	}
}

func TestFailedDispatchKeepsMessagesPending(t *testing.T) {
	s, _ := open(t)
	ctx := context.Background()
	now := time.Now()
	if _, err := s.SavePoll(ctx, "work", []inbox.Message{msg("p1", now), msg("p2", now)}, nil, now); err != nil {
		t.Fatal(err)
	}
	pending, _ := s.Pending(ctx)
	id, err := s.CreateDispatch(ctx, Dispatch{Profile: "triage", Driver: "orca", CreatedAt: now, MessageCount: 2})
	if err != nil {
		t.Fatal(err)
	}
	ids := []int64{pending[0].ID, pending[1].ID}
	if err := s.FinishDispatch(ctx, id, DispatchFailed, "orca: not reachable", "triage", inbox.StateSent, ids); err != nil {
		t.Fatal(err)
	}
	again, _ := s.Pending(ctx)
	if len(again) != 2 {
		t.Fatalf("pending after failure = %d, want 2", len(again))
	}
	ds, _ := s.Dispatches(ctx, 1)
	if ds[0].Status != DispatchFailed || ds[0].Detail != "orca: not reachable" {
		t.Errorf("dispatch = %+v", ds[0])
	}
	if err := s.FinishDispatch(ctx, id, DispatchSent, "", "triage", inbox.StateSent, ids); err != nil {
		t.Fatal(err)
	}
	sent, _ := s.Messages(ctx, id)
	if len(sent) != 2 || sent[0].State != inbox.StateSent {
		t.Errorf("messages of dispatch = %+v", sent)
	}
}

func TestExpireAndPrune(t *testing.T) {
	s, _ := open(t)
	ctx := context.Background()
	now := time.Now()
	old := now.Add(-48 * time.Hour)
	veryOld := now.Add(-8 * 24 * time.Hour)
	if _, err := s.SavePoll(ctx, "work", []inbox.Message{msg("old", old), msg("new", now), msg("veryold", veryOld)}, nil, now); err != nil {
		t.Fatal(err)
	}
	n, err := s.ExpirePending(ctx, now.Add(-24*time.Hour))
	if err != nil || n != 2 {
		t.Fatalf("expired = %d, %v; want 2", n, err)
	}
	pending, _ := s.Pending(ctx)
	if len(pending) != 1 || pending[0].ExternalID != "new" {
		t.Fatalf("pending = %+v", pending)
	}
	deleted, _, err := s.Prune(ctx, now.Add(-7*24*time.Hour))
	if err != nil || deleted != 1 {
		t.Fatalf("pruned = %d, %v; want 1", deleted, err)
	}
	counts, _ := s.Counts(ctx)
	if counts[inbox.StateExpired] != 1 || counts[inbox.StatePending] != 1 {
		t.Errorf("counts = %v", counts)
	}
}

func TestSessions(t *testing.T) {
	s, _ := open(t)
	ctx := context.Background()
	if h, _, _ := s.Session(ctx, "triage"); h != "" {
		t.Fatalf("handle = %q, want empty", h)
	}
	if err := s.SetSession(ctx, "triage", "term_1", "hash1", time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := s.SetSession(ctx, "triage", "term_2", "hash2", time.Now()); err != nil {
		t.Fatal(err)
	}
	if h, ph, _ := s.Session(ctx, "triage"); h != "term_2" || ph != "hash2" {
		t.Fatalf("session = %q %q, want term_2 hash2", h, ph)
	}
	if err := s.DeleteSession(ctx, "triage"); err != nil {
		t.Fatal(err)
	}
	if h, _, _ := s.Session(ctx, "triage"); h != "" {
		t.Fatalf("handle after delete = %q", h)
	}
}

// A version 1 store marked the messages of dry-run batches sent. Version 2 makes them previewed,
// so a real driver sends them.
func TestMigrateV1DryRunMessagesBecomePreviewed(t *testing.T) {
	s, path := open(t)
	ctx := context.Background()
	if _, err := s.SavePoll(ctx, "work", []inbox.Message{msg("p1", time.Now()), msg("p2", time.Now()), msg("p3", time.Now())}, nil, time.Now()); err != nil {
		t.Fatal(err)
	}
	pending, _ := s.Pending(ctx)
	for i, driver := range []string{"dry-run", "orca"} {
		id, err := s.CreateDispatch(ctx, Dispatch{Profile: "triage", Driver: driver, CreatedAt: time.Now()})
		if err != nil {
			t.Fatal(err)
		}
		if err := s.FinishDispatch(ctx, id, DispatchSent, "", "triage", inbox.StateSent, []int64{pending[i].ID}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.db.Exec(`UPDATE schema_version SET version = 1`); err != nil {
		t.Fatal(err)
	}
	s.Close()

	s2, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	if v, _ := s2.Version(ctx); v != SchemaVersion {
		t.Errorf("version = %d, want %d", v, SchemaVersion)
	}
	counts, _ := s2.Counts(ctx)
	if counts[inbox.StatePreviewed] != 1 || counts[inbox.StateSent] != 1 || counts[inbox.StatePending] != 1 {
		t.Errorf("counts = %v, want 1 previewed (dry-run), 1 sent (orca), 1 pending", counts)
	}
	waiting, _ := s2.Pending(ctx)
	if len(waiting) != 2 {
		t.Errorf("Pending = %d messages, want the pending and the previewed one", len(waiting))
	}
}
