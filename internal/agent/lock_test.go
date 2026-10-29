package agent

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestLockOneDeliveryForEachProfile(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()
	unlock, err := Lock(ctx, dir, "triage")
	if err != nil {
		t.Fatal(err)
	}
	short, cancel := context.WithTimeout(ctx, 300*time.Millisecond)
	defer cancel()
	if _, err := Lock(short, dir, "triage"); !errors.Is(err, ErrBusy) {
		t.Fatalf("second lock of triage: %v, want ErrBusy", err)
	}
	other, err := Lock(ctx, dir, "work")
	if err != nil {
		t.Fatalf("lock of another profile: %v", err)
	}
	other()

	// A waiting lock gets the lock when the holder releases it.
	got := make(chan error, 1)
	go func() {
		wctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		u, err := Lock(wctx, dir, "triage")
		if err == nil {
			u()
		}
		got <- err
	}()
	time.Sleep(250 * time.Millisecond)
	unlock()
	if err := <-got; err != nil {
		t.Errorf("waiting lock: %v", err)
	}
}
