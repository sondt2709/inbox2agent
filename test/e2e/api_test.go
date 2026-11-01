//go:build e2e

package e2e

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/sondt2709/inbox2agent/internal/config"
	"github.com/sondt2709/inbox2agent/internal/daemon"
	"github.com/sondt2709/inbox2agent/test/e2e/harness"
)

// S41: the daemon serves its API on a Unix socket. curl reads the status,
// "i2a run --once" asks the daemon, and R in the Feed sends now: with the
// daemon, and without it in the dashboard process.
func TestS41DaemonAPIAndRunNow(t *testing.T) {
	w := mm.NewWorld(t)
	p := newPerson(t, "S41")
	p.addAccount(w, w.Selected)
	start := time.Now().Add(2 * time.Hour) // the schedule does not send during the test
	p.editConfig(func(c *config.Config) {
		fastTiming(c)
		c.Schedules[0].Hours = start.Format("15:04") + "-" + start.Add(time.Hour).Format("15:04")
	})

	d := p.term("daemon")
	d.WaitFor(`msg=API socket=`, 30*time.Second)
	socket := daemon.FindSocket(p.home)
	if s, _ := p.run("status"); !strings.Contains(s, "Daemon: running, API") {
		t.Fatalf("status does not show the daemon:\n%s", s)
	}
	curl := harness.Start(t, filepath.Join(outRoot, p.id), "curl -s --unix-socket "+socket+" http://i2a/v1/status", p.env)
	// The terminal breaks the long JSON line, so compare without spaces.
	s := strings.Join(strings.Fields(curl.WaitFor(`\[exit \d+\]`, 10*time.Second)), "")
	if !strings.Contains(s, `"space":"unsorted"`) || !strings.Contains(s, `"pid":`) {
		t.Fatalf("curl /v1/status:\n%s", s)
	}

	w.Post(t, w.Bob, w.Selected, "S41 first message")
	s, code := p.run("run --once")
	if code != 0 || !strings.Contains(s, "the daemon did the run") || !strings.Contains(s, "batch of Unsorted to triage (dry-run, 1 messages)") {
		t.Fatalf("run --once with the daemon: exit %d:\n%s", code, s)
	}
	d.WaitFor(`msg="run from the API"`, 10*time.Second)

	// R in the Feed with the daemon.
	tui := p.term("")
	tui.WaitFor(`1 Feed`, 30*time.Second)
	w.Post(t, w.Bob, w.Selected, "S41 second message")
	tui.Keys("R")
	// The daemon holds its lock, so a success means that the daemon did the run.
	tui.WaitFor(`Read 1 new messages · Unsorted: \w+ \(1\)`, 30*time.Second)

	// R in the Feed without the daemon: the dashboard runs the engine.
	d.Keys("C-c")
	d.WaitFor(`daemon stopped`, 10*time.Second)
	if _, err := os.Stat(socket); !os.IsNotExist(err) {
		t.Errorf("the socket stays after the daemon stopped: %v", err)
	}
	w.Post(t, w.Bob, w.Selected, "S41 third message")
	tui.Keys("R")
	tui.WaitFor(`Read 1 new messages · Unsorted: \w+ \(1\)`, 30*time.Second)
	// R in the Spaces tab without the daemon: the selected space is Unsorted.
	w.Post(t, w.Bob, w.Selected, "S41 fourth message")
	tui.Press("5", `5 Spaces`, 5*time.Second)
	tui.WaitFor(`R send now`, 10*time.Second) // after the status line of the last run
	tui.Keys("R")
	tui.WaitFor(`Read 1 new messages · Unsorted: \w+ \(1\)`, 30*time.Second)
	tui.Keys("q")
	var all []string
	for _, f := range p.digests() {
		all = append(all, texts(readDigest(t, f))...)
	}
	if got := strings.Join(all, " | "); !regexp.MustCompile(`S41 first message.*S41 second message.*S41 third message.*S41 fourth message`).MatchString(got) {
		t.Errorf("digest texts = %q", got)
	}
}
