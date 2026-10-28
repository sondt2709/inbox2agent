package daemon

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/sondt2709/inbox2agent/internal/agent"
	"github.com/sondt2709/inbox2agent/internal/app"
	"github.com/sondt2709/inbox2agent/internal/config"
	"github.com/sondt2709/inbox2agent/internal/inbox"
	"github.com/sondt2709/inbox2agent/internal/render"
	"github.com/sondt2709/inbox2agent/internal/secrets"
	"github.com/sondt2709/inbox2agent/internal/source"
	"github.com/sondt2709/inbox2agent/internal/store"
)

// fakeSource returns the messages in its queue one time.
type fakeSource struct {
	id   string
	mu   sync.Mutex
	msgs []inbox.Message
	err  error
}

func (f *fakeSource) ID() string { return f.id }
func (f *fakeSource) Collect(_ context.Context, cur inbox.Cursor, _ time.Time) ([]inbox.Message, inbox.Cursor, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return nil, nil, f.err
	}
	out := f.msgs
	return out, inbox.Cursor{"n": fmt.Sprint(len(cur) + len(out))}, nil
}

func (f *fakeSource) add(ids ...string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, id := range ids {
		f.msgs = append(f.msgs, inbox.Message{SourceID: f.id, SourceType: "fake", ExternalID: id, ChannelID: "C" + id[:1],
			ChannelName: "chan", ChannelKind: inbox.KindChannel, Author: "bob", Text: "text " + id, CreatedAt: time.Now()})
	}
}

// recorder is a driver that records batches and returns a set result.
type recorder struct {
	mu      sync.Mutex
	batches []*render.Rendered
	result  agent.Result
	err     error
}

func (r *recorder) Deliver(_ context.Context, b *render.Rendered) (agent.Result, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.batches = append(r.batches, b)
	if r.err != nil {
		return agent.Result{}, r.err
	}
	return r.result, nil
}

type harness struct {
	e       *Engine
	st      *store.Store
	sources map[string]*fakeSource
	drv     *recorder
	log     *bytes.Buffer
	logMu   sync.Mutex
	paths   app.Paths
}

func newHarness(t *testing.T, cfg *config.Config) *harness {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("I2A_HOME", dir)
	paths, _ := app.ResolvePaths()
	st, err := store.Open(paths.DBFile)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	h := &harness{st: st, sources: map[string]*fakeSource{}, drv: &recorder{result: agent.Result{Status: agent.StatusSent}},
		log: &bytes.Buffer{}, paths: paths}
	for _, sc := range cfg.Sources {
		h.sources[sc.ID] = &fakeSource{id: sc.ID}
	}
	h.e = New(Deps{
		Paths: paths, Store: st,
		Log:       slog.New(slog.NewTextHandler(syncWriter{&h.logMu, h.log}, nil)),
		NewDriver: func(config.Profile, agent.Sessions) (agent.Driver, error) { return h.drv, nil },
		BuildSource: func(sc config.Source, _ secrets.Store, _ time.Duration, _ *http.Client) (source.Source, error) {
			return h.sources[sc.ID], nil
		},
	}, cfg)
	return h
}

type syncWriter struct {
	mu *sync.Mutex
	b  *bytes.Buffer
}

func (w syncWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.b.Write(p)
}

func (h *harness) logText() string {
	h.logMu.Lock()
	defer h.logMu.Unlock()
	return h.log.String()
}

func (r *recorder) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.batches)
}

func cfgWith(sources ...string) *config.Config {
	c := config.Default()
	for _, id := range sources {
		s := config.NewMattermostSource(id, "https://x")
		c.Sources = append(c.Sources, s)
	}
	return c
}

func TestPollThenDispatchThenNothingNew(t *testing.T) {
	h := newHarness(t, cfgWith("work"))
	ctx := context.Background()
	h.sources["work"].add("a1", "a2", "a3")
	polls := h.e.Poll(ctx, true)
	if len(polls) != 1 || polls[0].New != 3 {
		t.Fatalf("polls = %+v", polls)
	}
	d, err := h.e.Dispatch(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Batches) != 1 || d.Batches[0].Count != 3 || d.Batches[0].Status != store.DispatchSent {
		t.Fatalf("batches = %+v", d.Batches)
	}
	if len(h.drv.batches) != 1 || h.drv.batches[0].Count != 3 {
		t.Fatalf("driver got %d batches", len(h.drv.batches))
	}
	if _, err := os.Stat(d.Batches[0].DigestPath); err != nil {
		t.Errorf("digest: %v", err)
	}
	counts, _ := h.st.Counts(ctx)
	if counts[inbox.StatePreviewed] != 3 { // the default profile is a dry run
		t.Errorf("counts = %v", counts)
	}
	// The source returns the same messages again: nothing is new.
	polls = h.e.Poll(ctx, true)
	if polls[0].New != 0 {
		t.Errorf("second poll new = %d, want 0", polls[0].New)
	}
	d, _ = h.e.Dispatch(ctx)
	if len(d.Batches) != 0 {
		t.Errorf("second dispatch batches = %+v, want none", d.Batches)
	}
	if s := Summary(polls, d); !strings.Contains(s, "collected 0 new messages") || !strings.Contains(s, "no batch to send") {
		t.Errorf("summary = %q", s)
	}
}

func TestDriverErrorThenRetry(t *testing.T) {
	h := newHarness(t, cfgWith("work"))
	ctx := context.Background()
	h.sources["work"].add("a1", "a2")
	h.e.Poll(ctx, true)
	h.drv.err = errors.New("orca agent launch: runtime not reachable")
	d, _ := h.e.Dispatch(ctx)
	if d.Batches[0].Status != store.DispatchFailed || d.Batches[0].Err == nil {
		t.Fatalf("batch = %+v", d.Batches[0])
	}
	pending, _ := h.st.Pending(ctx)
	if len(pending) != 2 {
		t.Fatalf("pending = %d, want 2", len(pending))
	}
	ds, _ := h.st.Dispatches(ctx, 1)
	if ds[0].Status != store.DispatchFailed || !strings.Contains(ds[0].Detail, "runtime not reachable") {
		t.Errorf("dispatch record = %+v", ds[0])
	}
	h.drv.err = nil
	d, _ = h.e.Dispatch(ctx)
	if len(d.Batches) != 1 || d.Batches[0].Count != 2 || d.Batches[0].Status != store.DispatchSent {
		t.Fatalf("retry batches = %+v", d.Batches)
	}
}

func TestHeldKeepsMessagesPending(t *testing.T) {
	h := newHarness(t, cfgWith("work"))
	ctx := context.Background()
	h.sources["work"].add("a1")
	h.e.Poll(ctx, true)
	h.drv.result = agent.Result{Status: agent.StatusHeld, Detail: "agent busy"}
	d, _ := h.e.Dispatch(ctx)
	if d.Batches[0].Status != store.DispatchHeld {
		t.Fatalf("status = %s", d.Batches[0].Status)
	}
	pending, _ := h.st.Pending(ctx)
	if len(pending) != 1 {
		t.Errorf("pending = %d, want 1", len(pending))
	}
}

func TestRoutesMakeOneBatchForEachProfile(t *testing.T) {
	c := cfgWith("work")
	c.Profiles = append(c.Profiles, config.NewProfile("reviewer"))
	c.Sources[0].ChannelProfiles = map[string]string{"Cr": "reviewer"}
	h := newHarness(t, c)
	ctx := context.Background()
	h.sources["work"].add("a1", "r1", "r2")
	h.e.Poll(ctx, true)
	d, _ := h.e.Dispatch(ctx)
	got := map[string]int{}
	for _, b := range d.Batches {
		got[b.Profile] = b.Count
	}
	if got["triage"] != 1 || got["reviewer"] != 2 {
		t.Errorf("batches = %v", got)
	}
}

func TestSourceErrorDoesNotStopOthers(t *testing.T) {
	h := newHarness(t, cfgWith("bad", "good"))
	h.sources["bad"].err = errors.New("mattermost: HTTP 401")
	h.sources["good"].add("g1")
	polls := h.e.Poll(context.Background(), true)
	if polls[0].Err == nil || polls[1].New != 1 {
		t.Fatalf("polls = %+v", polls)
	}
	if !strings.Contains(h.logText(), `source=bad`) {
		t.Errorf("log = %s", h.logText())
	}
}

func TestLoopOutsideActiveHoursAndReload(t *testing.T) {
	old := tick
	tick = 20 * time.Millisecond
	t.Cleanup(func() { tick = old })

	c := cfgWith("work")
	c.Timing.Interval = config.Duration(50 * time.Millisecond)
	c.Sources[0].PollEvery = config.Duration(10 * time.Millisecond)
	now := time.Now()
	start := now.Add(2 * time.Hour)
	c.Timing.ActiveHours = fmt.Sprintf("%s-%s", start.Format("15:04"), start.Add(time.Hour).Format("15:04"))
	h := newHarness(t, c)
	h.sources["work"].add("a1")

	cfgFile := filepath.Join(t.TempDir(), "config.toml")
	if err := config.Save(cfgFile, fileCopy(c)); err != nil {
		t.Fatal(err)
	}
	reloaded := make(chan struct{}, 1)
	load := func() (*config.Config, error) {
		nc, err := config.Load(cfgFile)
		if err == nil {
			nc.Timing.Interval = config.Duration(50 * time.Millisecond)
			nc.Sources[0].PollEvery = config.Duration(10 * time.Millisecond)
			reloaded <- struct{}{}
		}
		return nc, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error)
	go func() { done <- Run(ctx, h.e, cfgFile, load) }()

	waitFor(t, func() bool { return strings.Contains(h.logText(), "outside active hours") })
	if n := h.drv.count(); n != 0 {
		t.Fatalf("dispatched %d batches outside active hours", n)
	}

	// Change the file: remove the active hours. The daemon reloads and dispatches.
	fc := fileCopy(c)
	fc.Timing.ActiveHours = ""
	time.Sleep(20 * time.Millisecond) // a new modification time
	if err := config.Save(cfgFile, fc); err != nil {
		t.Fatal(err)
	}
	<-reloaded
	waitFor(t, func() bool { return strings.Contains(h.logText(), "configuration reloaded") })
	waitFor(t, func() bool { return h.drv.count() == 1 })
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

// fileCopy returns a copy of c with intervals that pass validation.
// The engine keeps the short intervals of c.
func fileCopy(c *config.Config) *config.Config {
	fc := *c
	fc.Sources = append([]config.Source(nil), c.Sources...)
	fc.Timing.Interval = config.Duration(5 * time.Second)
	fc.Sources[0].PollEvery = config.Duration(5 * time.Second)
	return &fc
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("condition not met in 5s")
}

func TestLock(t *testing.T) {
	path := filepath.Join(t.TempDir(), "daemon.lock")
	unlock, err := Lock(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Lock(path); !errors.Is(err, ErrLocked) {
		t.Fatalf("second lock: err = %v, want ErrLocked", err)
	}
	unlock()
	unlock2, err := Lock(path)
	if err != nil {
		t.Fatalf("lock after unlock: %v", err)
	}
	unlock2()
}

func TestDryRunPreviewThenRealDriverSendsAll(t *testing.T) {
	cfg := cfgWith("work")
	h := newHarness(t, cfg)
	ctx := context.Background()
	h.sources["work"].add("a1", "a2", "a3")
	h.e.Poll(ctx, true)
	if d, _ := h.e.Dispatch(ctx); len(d.Batches) != 1 || d.Batches[0].Count != 3 {
		t.Fatalf("dry-run batches = %+v", d.Batches)
	}
	// A second dry-run round does not preview the same messages again.
	if d, _ := h.e.Dispatch(ctx); len(d.Batches) != 0 {
		t.Fatalf("second dry-run batches = %+v, want none", d.Batches)
	}

	// The user changes the profile to orca, and one new message arrives.
	next := cfgWith("work")
	next.Profiles[0].Driver = config.DriverOrca
	h.e.SetConfig(next)
	h.sources["work"].add("a4")
	h.e.Poll(ctx, true)
	d, err := h.e.Dispatch(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Batches) != 1 || d.Batches[0].Count != 4 || d.Batches[0].Driver != config.DriverOrca {
		t.Fatalf("orca batches = %+v, want one batch of 4", d.Batches)
	}
	counts, _ := h.st.Counts(ctx)
	if counts[inbox.StateSent] != 4 || counts[inbox.StatePending] != 0 {
		t.Errorf("counts = %v, want 4 sent", counts)
	}
}

func TestNotifyAfterRealDispatchOnly(t *testing.T) {
	cfg := cfgWith("work")
	h := newHarness(t, cfg)
	var got []string
	h.e.d.Notify = func(title, text string) { got = append(got, title+" | "+text) }
	ctx := context.Background()

	h.sources["work"].add("a1", "a2")
	h.e.Poll(ctx, true)
	if _, err := h.e.Dispatch(ctx); err != nil {
		t.Fatal(err)
	} // dry run: no notification
	if len(got) != 0 {
		t.Fatalf("dry run notified: %v", got)
	}

	next := cfgWith("work")
	next.Profiles[0].Driver = config.DriverOrca
	h.e.SetConfig(next)
	h.drv.result = agent.Result{Status: agent.StatusSent, Detail: "new session term_1, turn started"}
	if _, err := h.e.Dispatch(ctx); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != "i2a: 2 messages sent | Profile triage to claude. new session term_1, turn started" {
		t.Fatalf("notifications = %q", got)
	}
	for _, m := range []string{"text a1", "text a2"} {
		if strings.Contains(got[0], m) {
			t.Errorf("the notification has message text %q", m)
		}
	}

	h.sources["work"].add("b1")
	h.e.Poll(ctx, true)
	h.drv.err = errors.New("orca agent launch: runtime not reachable")
	if _, err := h.e.Dispatch(ctx); err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || !strings.HasPrefix(got[1], "i2a: dispatch failed | Profile triage, 1 messages stay pending: orca agent launch") {
		t.Fatalf("notifications = %q", got)
	}

	next.Notify.Enabled = false
	h.drv.err = nil
	if _, err := h.e.Dispatch(ctx); err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Errorf("notified with notify.enabled = false: %q", got[2:])
	}
}
