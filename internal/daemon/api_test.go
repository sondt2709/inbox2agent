package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sondt2709/inbox2agent/internal/config"
	"github.com/sondt2709/inbox2agent/internal/inbox"
)

// startAPI runs the loop and the API of the harness on the socket of dataDir.
func startAPI(t *testing.T, h *harness, dataDir string) (*Client, context.CancelFunc, chan error) {
	t.Helper()
	old := tick
	tick = 10 * time.Millisecond
	t.Cleanup(func() { tick = old })
	srv := NewServer("test", "config.toml", time.Now())
	if _, err := srv.Listen(dataDir); err != nil {
		t.Fatal(err)
	}
	h.e.SetAPI(srv)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 2)
	go func() { done <- srv.Serve(ctx) }()
	c := h.e.Config()
	go func() {
		done <- Run(ctx, h.e, filepath.Join(t.TempDir(), "none.toml"), func() (*config.Config, error) { return c, nil })
	}()
	t.Cleanup(cancel)
	return NewClient(dataDir), cancel, done
}

// shortDir returns a data directory with a short path, so the socket is in it.
func shortDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "i2a")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return dir
}

func (f *fakeSource) addText(id, channel, text string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.msgs = append(f.msgs, inbox.Message{SourceID: f.id, SourceType: "fake", ExternalID: id, ChannelID: channel,
		ChannelName: "chan", ChannelKind: inbox.KindChannel, Author: "bob", Text: text, CreatedAt: time.Now(), ForMe: true})
}

func TestAPIStatusRunAndEvents(t *testing.T) {
	h := newHarness(t, spaceCfg(cfgWith("work")))
	dir := shortDir(t)
	client, cancel, done := startAPI(t, h, dir)
	ctx := context.Background()

	fi, err := os.Stat(filepath.Join(dir, SocketName))
	if err != nil || fi.Mode().Perm() != 0o600 || fi.Mode()&os.ModeSocket == 0 {
		t.Fatalf("socket: %v %v, want a socket with the mode 0600", fi, err)
	}
	var st Status
	waitFor(t, func() bool { st, err = client.Status(ctx); return err == nil && len(st.Spaces) == 2 })
	if st.PID != os.Getpid() || st.Version != "test" || st.Spaces[1].Space != "review" || st.Spaces[1].At == nil {
		t.Fatalf("status = %+v", st)
	}

	events, err := client.Events(ctx)
	if err != nil {
		t.Fatal(err)
	}
	h.sources["work"].addText("a1", "Ca", "secret plan for unsorted")
	h.sources["work"].addText("r1", "Cr", "secret plan for review")
	res, err := client.Run(ctx, []string{"review"})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Batches) != 1 || res.Batches[0].Space != "review" || res.Batches[0].Count != 1 || res.Failed ||
		!strings.Contains(res.Summary, "batch of Review (dry-run, 1 messages)") {
		t.Fatalf("run = %+v", res)
	}
	// The agent reviewer is a dry run: its message is previewed. The message of Unsorted waits.
	p, _ := h.st.Pending(ctx)
	for _, m := range p {
		if want := map[string]string{"a1": inbox.StatePending, "r1": inbox.StatePreviewed}[m.ExternalID]; m.State != want {
			t.Errorf("message %s is %s after a run of review, want %s", m.ExternalID, m.State, want)
		}
	}
	var kinds []string
	var raw []string
	deadline := time.After(5 * time.Second)
	for len(kinds) == 0 || kinds[len(kinds)-1] != EventRun+":done" {
		select {
		case ev := <-events:
			kinds = append(kinds, ev.Kind+":"+ev.Status)
			b, _ := json.Marshal(ev)
			raw = append(raw, string(b))
		case <-deadline:
			t.Fatalf("events = %v, want a run that ends", kinds)
		}
	}
	want := []string{"run:started", "read:", "send:sent", "run:done"}
	if strings.Join(kinds, " ") != strings.Join(want, " ") {
		t.Errorf("events = %v, want %v", kinds, want)
	}
	b, _ := json.Marshal(res)
	st, _ = client.Status(ctx)
	sb, _ := json.Marshal(st)
	for _, text := range append(raw, string(b), string(sb)) {
		if strings.Contains(text, "secret plan") {
			t.Errorf("the API returned message text: %s", text)
		}
	}

	if _, err := client.Run(ctx, []string{"nope"}); err == nil || !strings.Contains(err.Error(), `the space does not exist: "nope"`) {
		t.Errorf("run of an unknown space: %v", err)
	}
	cancel()
	for range 2 {
		if err := <-done; err != nil {
			t.Fatal(err)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, SocketName)); !os.IsNotExist(err) {
		t.Errorf("the socket stays after the daemon stopped: %v", err)
	}
	if _, err := client.Status(ctx); err == nil {
		t.Error("a stopped daemon answers")
	}
}

func TestSocketOfALongDataPath(t *testing.T) {
	dir := filepath.Join(shortDir(t), strings.Repeat("d", 100))
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	socket, pointer := SocketPath(dir)
	if len(socket) > maxSocketPath || !strings.HasPrefix(socket, os.TempDir()) || pointer != filepath.Join(dir, "i2a.sock.path") {
		t.Fatalf("socket %q, pointer %q", socket, pointer)
	}
	h := newHarness(t, cfgWith("work"))
	client, cancel, done := startAPI(t, h, dir)
	if b, err := os.ReadFile(pointer); err != nil || string(b) != socket {
		t.Fatalf("i2a.sock.path = %q, %v", b, err)
	}
	if client.Socket != socket {
		t.Fatalf("the client dials %q, want %q", client.Socket, socket)
	}
	waitFor(t, func() bool { _, err := client.Status(context.Background()); return err == nil })
	cancel()
	<-done
	<-done
	for _, f := range []string{socket, pointer} {
		if _, err := os.Stat(f); !os.IsNotExist(err) {
			t.Errorf("%s stays after the daemon stopped", f)
		}
	}
}

func TestOpenAPIFileHasEachPath(t *testing.T) {
	b, err := os.ReadFile("../../docs/daemon-api.yaml")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range Paths {
		if !strings.Contains(string(b), "\n  "+p+":\n") {
			t.Errorf("docs/daemon-api.yaml has no path %s", p)
		}
	}
}

// A daemon that took the request and then failed is not "no daemon": the
// caller must not do the run itself.
func TestOnlyADialErrorIsNoDaemon(t *testing.T) {
	dir := shortDir(t)
	c := NewClient(dir)
	if _, err := c.Run(context.Background(), nil); !errors.Is(err, ErrNoDaemon) {
		t.Fatalf("no socket: %v, want ErrNoDaemon", err)
	}
	l, err := net.Listen("unix", filepath.Join(dir, SocketName))
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	go func() {
		for {
			conn, err := l.Accept()
			if err != nil {
				return
			}
			conn.Close() // the daemon stops after it took the request
		}
	}()
	if _, err := c.Run(context.Background(), nil); err == nil || errors.Is(err, ErrNoDaemon) {
		t.Fatalf("a closed connection: %v, want a daemon error that is not ErrNoDaemon", err)
	}
}

// A run of the API reads a changed config.toml first: a dashboard can add a
// space and press ctrl+r a moment later, before the next step of the loop.
func TestAPIRunReloadsAChangedFile(t *testing.T) {
	old := tick
	tick = time.Hour // the loop does not step again by itself
	t.Cleanup(func() { tick = old })
	h := newHarness(t, cfgWith("work"))
	dir := shortDir(t)
	cfgFile := filepath.Join(t.TempDir(), "config.toml")
	if err := config.Save(cfgFile, fileCopy(h.e.Config())); err != nil {
		t.Fatal(err)
	}
	srv := NewServer("test", cfgFile, time.Now())
	if _, err := srv.Listen(dir); err != nil {
		t.Fatal(err)
	}
	h.e.SetAPI(srv)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = srv.Serve(ctx) }()
	go func() { _ = Run(ctx, h.e, cfgFile, func() (*config.Config, error) { return config.Load(cfgFile) }) }()
	client := NewClient(dir)
	waitFor(t, func() bool { _, err := client.Status(ctx); return err == nil })
	waitFor(t, func() bool { return strings.Contains(h.logText(), "poll") })

	fc := fileCopy(h.e.Config())
	fc.Spaces = append(fc.Spaces, config.Space{ID: "fresh", Name: "Fresh", Agent: config.NewAgent("fresh"), Schedule: fc.Schedules[0].ID})
	time.Sleep(20 * time.Millisecond) // a new modification time
	if err := config.Save(cfgFile, fc); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Run(ctx, []string{"fresh"}); err != nil {
		t.Fatalf("run of a space that the file has now: %v", err)
	}
}
