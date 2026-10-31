package cli

import (
	"strings"
	"testing"

	"github.com/sondt2709/inbox2agent/internal/app"
	"github.com/sondt2709/inbox2agent/internal/daemon"
)

func TestRunAndDaemonHelpUseTheWordsOfTheModel(t *testing.T) {
	for _, cmd := range []string{"run", "daemon"} {
		r := i2a(t, "", cmd, "--help")
		if r.err != nil {
			t.Fatal(r.err)
		}
		for _, old := range []string{"profile", "source", "interval", "active hours"} {
			if strings.Contains(strings.ToLower(r.out), old) {
				t.Errorf("i2a %s --help says %q:\n%s", cmd, old, r.out)
			}
		}
		if !strings.Contains(r.out, "space") || !strings.Contains(r.out, "account") {
			t.Errorf("i2a %s --help does not name the account and the space:\n%s", cmd, r.out)
		}
	}
}

func TestRunOnceWaitsForADaemonThatHoldsItsLock(t *testing.T) {
	spacesEnv(t)
	paths, err := app.ResolvePaths()
	if err != nil {
		t.Fatal(err)
	}
	unlock, err := daemon.Lock(paths.LockFile) // a daemon that starts: it holds the lock, it does not listen yet
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()
	r := i2a(t, "", "run", "--once")
	if r.err == nil || !strings.Contains(r.err.Error(), "a daemon holds its lock") {
		t.Fatalf("run --once while a daemon holds its lock: %v", r.err)
	}
}
