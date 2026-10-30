package daemon

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
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
	"github.com/sondt2709/inbox2agent/internal/source"
	"github.com/sondt2709/inbox2agent/internal/store"
)

// fakeSource returns the messages in its queue one time.
type fakeSource struct {
	id     string
	mu     sync.Mutex
	msgs   []inbox.Message
	err    error
	closes int
}

func (f *fakeSource) ID() string { return f.id }

func (f *fakeSource) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closes++
	return nil
}
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
	mu         sync.Mutex
	batches    []*render.Rendered
	asks       []*render.Rendered
	askHandles []string
	events     []string // "batch" and "ask", in the order of the calls
	result     agent.Result
	err        error
}

func (r *recorder) Deliver(_ context.Context, b *render.Rendered) (agent.Result, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.batches = append(r.batches, b)
	r.events = append(r.events, "batch")
	if r.err != nil {
		return agent.Result{}, r.err
	}
	return r.result, nil
}

func (r *recorder) Ask(_ context.Context, handle string, _ time.Duration, a *render.Rendered) (agent.Result, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.asks = append(r.asks, a)
	r.askHandles = append(r.askHandles, handle)
	r.events = append(r.events, "ask")
	if r.err != nil {
		return agent.Result{}, r.err
	}
	res := r.result
	if res.Status == agent.StatusSent {
		res.Handle = handle
	}
	return res, nil
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
	for _, sc := range cfg.Accounts {
		h.sources[sc.ID] = &fakeSource{id: sc.ID}
	}
	h.e = New(Deps{
		Paths: paths, Store: st,
		Log:       slog.New(slog.NewTextHandler(syncWriter{&h.logMu, h.log}, nil)),
		NewDriver: func(config.Agent, string, agent.Sessions) (agent.Driver, error) { return h.drv, nil },
		BuildSource: func(sc config.Account, _ source.Env) (source.Source, error) {
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
		c.Accounts = append(c.Accounts, config.NewMattermostAccount(id, "https://x"))
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
	d, err := h.e.Dispatch(ctx, nil)
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
	if counts[inbox.StatePreviewed] != 3 { // the default agent is a dry run
		t.Errorf("counts = %v", counts)
	}
	// The source returns the same messages again: nothing is new.
	polls = h.e.Poll(ctx, true)
	if polls[0].New != 0 {
		t.Errorf("second poll new = %d, want 0", polls[0].New)
	}
	d, _ = h.e.Dispatch(ctx, nil)
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
	d, _ := h.e.Dispatch(ctx, nil)
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
	d, _ = h.e.Dispatch(ctx, nil)
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
	d, _ := h.e.Dispatch(ctx, nil)
	if d.Batches[0].Status != store.DispatchHeld {
		t.Fatalf("status = %s", d.Batches[0].Status)
	}
	pending, _ := h.st.Pending(ctx)
	if len(pending) != 1 {
		t.Errorf("pending = %d, want 1", len(pending))
	}
}

// spaceCfg adds the agent reviewer and the space Review with the
// conversation work/Cr to a configuration with the account work.
func spaceCfg(c *config.Config) *config.Config {
	c.Agents = append(c.Agents, config.NewAgent("reviewer"))
	c.Spaces = []config.Space{{ID: "review", Name: "Review", Agent: "reviewer", Schedule: "always",
		Conversations: []config.Conversation{{Account: "work", ID: "Cr", Read: config.ReadAll}}}}
	return c
}

func TestRoutesMakeOneBatchForEachSpace(t *testing.T) {
	h := newHarness(t, spaceCfg(cfgWith("work")))
	ctx := context.Background()
	h.sources["work"].add("a1", "r1", "r2")
	h.e.Poll(ctx, true)
	d, _ := h.e.Dispatch(ctx, nil)
	got := map[string]int{}
	for _, b := range d.Batches {
		got[b.Space+"/"+b.Agent] = b.Count
	}
	if got["unsorted/triage"] != 1 || got["review/reviewer"] != 2 || len(got) != 2 {
		t.Errorf("batches = %v", got)
	}
	ds, _ := h.st.Dispatches(ctx, 10)
	if len(ds) != 2 || ds[0].Space == ds[1].Space || ds[0].Agent == "" {
		t.Errorf("dispatch rows = %+v", ds)
	}
}

func TestDispatchOnlyGivenSpacesAndMovedConversation(t *testing.T) {
	c := spaceCfg(cfgWith("work"))
	c.Unsorted.Agent = "" // hold
	h := newHarness(t, c)
	ctx := context.Background()
	h.sources["work"].add("a1", "a2", "r1")
	h.e.Poll(ctx, true)
	d, _ := h.e.Dispatch(ctx, nil)
	if len(d.Batches) != 1 || d.Batches[0].Space != "review" || d.Batches[0].Count != 1 {
		t.Fatalf("batches = %+v, want only Review: Unsorted holds", d.Batches)
	}
	if counts, _ := h.st.Counts(ctx); counts[inbox.StatePending] != 2 {
		t.Fatalf("counts = %v, want the 2 held messages pending", counts)
	}

	// The user moves the conversation Ca to Review: its waiting messages go there.
	next := spaceCfg(cfgWith("work"))
	next.Unsorted.Agent = ""
	if err := next.SetConversation("work", "Ca", "review", "", ""); err != nil {
		t.Fatal(err)
	}
	h.e.SetConfig(next)
	if d, _ := h.e.Dispatch(ctx, []string{config.UnsortedID}); len(d.Batches) != 0 {
		t.Errorf("a dispatch of Unsorted sent %+v", d.Batches)
	}
	d, _ = h.e.Dispatch(ctx, []string{"review"})
	if len(d.Batches) != 1 || d.Batches[0].Count != 2 {
		t.Errorf("batches = %+v, want the 2 moved messages in Review", d.Batches)
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
	if !strings.Contains(h.logText(), `account=bad`) {
		t.Errorf("log = %s", h.logText())
	}
}

func TestLoopOutsideTheHoursAndReload(t *testing.T) {
	old := tick
	tick = 20 * time.Millisecond
	t.Cleanup(func() { tick = old })

	c := cfgWith("work")
	c.Schedules[0].Every = config.Duration(50 * time.Millisecond)
	c.Accounts[0].ReadEvery = config.Duration(10 * time.Millisecond)
	start := time.Now().Add(2 * time.Hour)
	c.Schedules[0].Hours = fmt.Sprintf("%s-%s", start.Format("15:04"), start.Add(time.Hour).Format("15:04"))
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
			nc.Schedules[0].Every = config.Duration(50 * time.Millisecond)
			nc.Accounts[0].ReadEvery = config.Duration(10 * time.Millisecond)
			reloaded <- struct{}{}
		}
		return nc, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error)
	go func() { done <- Run(ctx, h.e, cfgFile, load) }()

	waitFor(t, func() bool {
		return strings.Contains(h.logText(), "next send") && strings.Contains(h.logText(), "poll")
	})
	time.Sleep(200 * time.Millisecond)
	if n := h.drv.count(); n != 0 {
		t.Fatalf("dispatched %d batches outside the hours of the schedule", n)
	}

	// Change the file: remove the hours. The daemon reloads and dispatches.
	fc := fileCopy(c)
	fc.Schedules[0].Hours = ""
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

func TestLoopSendsEachSpaceOnItsSchedule(t *testing.T) {
	old := tick
	tick = 10 * time.Millisecond
	t.Cleanup(func() { tick = old })

	c := spaceCfg(cfgWith("work"))
	c.Agents[0].Driver, c.Agents[0].Workspace = config.DriverOrca, "/w"
	c.Agents[1].Driver, c.Agents[1].Workspace = config.DriverOrca, "/w"
	c.Accounts[0].ReadEvery = config.Duration(5 * time.Millisecond)
	c.Schedules = []config.Schedule{{ID: "fast", Every: config.Duration(40 * time.Millisecond)}, {ID: "slow", Every: config.Duration(160 * time.Millisecond)}}
	c.Unsorted.Schedule, c.Spaces[0].Schedule = "fast", "slow"
	h := newHarness(t, c)

	ctx, cancel := context.WithCancel(context.Background())
	stop := make(chan struct{})
	go func() { // new messages in both spaces all the time
		for i := 0; ; i++ {
			select {
			case <-stop:
				return
			case <-time.After(5 * time.Millisecond):
				h.sources["work"].add(fmt.Sprintf("a%d", i), fmt.Sprintf("r%d", i))
			}
		}
	}()
	done := make(chan error)
	go func() {
		done <- Run(ctx, h.e, filepath.Join(t.TempDir(), "none.toml"), func() (*config.Config, error) { return c, nil })
	}()
	time.Sleep(900 * time.Millisecond)
	cancel()
	close(stop)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	h.drv.mu.Lock()
	defer h.drv.mu.Unlock()
	by := map[string]int{}
	for _, b := range h.drv.batches {
		by[b.Space]++
	}
	if by["Unsorted"] < 2*by["Review"] || by["Review"] < 2 {
		t.Errorf("batches by space = %v, want Unsorted (every 40ms) at least 2 times Review (every 160ms)", by)
	}
}

// fileCopy returns a copy of c with intervals that pass validation.
// The engine keeps the short intervals of c.
func fileCopy(c *config.Config) *config.Config {
	fc := *c
	fc.Accounts = append([]config.Account(nil), c.Accounts...)
	fc.Schedules = append([]config.Schedule(nil), c.Schedules...)
	fc.Schedules[0].Every = config.Duration(time.Minute)
	fc.Accounts[0].ReadEvery = config.Duration(5 * time.Second)
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
	if d, _ := h.e.Dispatch(ctx, nil); len(d.Batches) != 1 || d.Batches[0].Count != 3 {
		t.Fatalf("dry-run batches = %+v", d.Batches)
	}
	// A second dry-run round does not preview the same messages again.
	if d, _ := h.e.Dispatch(ctx, nil); len(d.Batches) != 0 {
		t.Fatalf("second dry-run batches = %+v, want none", d.Batches)
	}

	// The user changes the agent to orca, and one new message arrives.
	next := cfgWith("work")
	next.Agents[0].Driver = config.DriverOrca
	h.e.SetConfig(next)
	h.sources["work"].add("a4")
	h.e.Poll(ctx, true)
	d, err := h.e.Dispatch(ctx, nil)
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

// A sent batch gives no notification (the cards of the agent do); a failed
// dispatch to a real driver does, without message text.
func TestNotifyAfterFailedRealDispatchOnly(t *testing.T) {
	cfg := cfgWith("work")
	h := newHarness(t, cfg)
	var got []string
	h.e.d.Notify = func(title, text string) { got = append(got, title+" | "+text) }
	ctx := context.Background()

	h.sources["work"].add("a1", "a2")
	h.e.Poll(ctx, true)
	if _, err := h.e.Dispatch(ctx, nil); err != nil {
		t.Fatal(err)
	} // dry run: no notification
	if len(got) != 0 {
		t.Fatalf("dry run notified: %v", got)
	}

	next := cfgWith("work")
	next.Agents[0].Driver = config.DriverOrca
	h.e.SetConfig(next)
	h.drv.result = agent.Result{Status: agent.StatusSent, Detail: "new session term_1, turn started"}
	if _, err := h.e.Dispatch(ctx, nil); err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("a sent batch notified: %q", got)
	}

	h.sources["work"].add("b1")
	h.e.Poll(ctx, true)
	h.drv.err = errors.New("orca agent launch: runtime not reachable")
	if _, err := h.e.Dispatch(ctx, nil); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || !strings.HasPrefix(got[0], "i2a: dispatch failed | Space Unsorted (agent triage), 1 messages stay pending: orca agent launch") {
		t.Fatalf("notifications = %q", got)
	}
	if strings.Contains(got[0], "text b1") {
		t.Errorf("the notification has message text: %q", got[0])
	}

	next.Notify.Enabled = false
	if _, err := h.e.Dispatch(ctx, nil); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Errorf("notified with notify.enabled = false: %q", got[1:])
	}
}

// The primer of a real driver ends with the card section, which names this
// i2a with I2A_HOME. A dry run has no card section.
func TestPrimerHasCardSectionForRealDriverOnly(t *testing.T) {
	cfg := cfgWith("work")
	h := newHarness(t, cfg)
	h.e.d.CardCommand = render.CardCommand("/opt/i2a/bin/i2a", h.paths.DataDir)
	ctx := context.Background()

	h.sources["work"].add("a1")
	h.e.Poll(ctx, true)
	if _, err := h.e.Dispatch(ctx, nil); err != nil {
		t.Fatal(err)
	}
	next := cfgWith("work")
	next.Agents[0].Driver = config.DriverOrca
	h.e.SetConfig(next)
	if _, err := h.e.Dispatch(ctx, nil); err != nil {
		t.Fatal(err)
	}
	if len(h.drv.batches) != 2 {
		t.Fatalf("batches = %d, want 2", len(h.drv.batches))
	}
	if dry := h.drv.batches[0]; strings.Contains(dry.Primer, "card add") {
		t.Errorf("dry-run primer has the card section")
	}
	orca := h.drv.batches[1]
	want := "I2A_HOME=" + h.paths.DataDir + " /opt/i2a/bin/i2a card add <<'EOF'"
	if !strings.Contains(orca.Primer, want) {
		t.Errorf("orca primer does not contain %q:\n%s", want, orca.Primer)
	}
	if !strings.Contains(orca.Messages, `{"id":`) {
		t.Errorf("the data lines have no id: %s", orca.Messages)
	}
}

// orcaCfg returns a configuration with one account and one orca agent.
func orcaCfg() *config.Config {
	c := cfgWith("work")
	c.Agents[0].Driver = config.DriverOrca
	return c
}

// queueAsk adds a card from the terminal term_card and a queued ask about it.
func queueAsk(t *testing.T, h *harness, created time.Time) inbox.Ask {
	t.Helper()
	ctx := context.Background()
	saved, err := h.st.SaveCards(ctx, []inbox.Card{{Key: "mm:pr-57", Category: inbox.CategoryAction, Urgency: inbox.UrgencyHigh,
		Title: "Review PR 57", Handle: "term_card"}}, created)
	if err != nil {
		t.Fatal(err)
	}
	a := inbox.Ask{CardID: saved[0].ID, Space: config.UnsortedID, Text: "Research more.", CreatedAt: created}
	if a.ID, err = h.st.StartAsk(ctx, a); err != nil {
		t.Fatal(err)
	}
	return a
}

func TestDispatchSendsQueuedAsksBeforeBatches(t *testing.T) {
	h := newHarness(t, orcaCfg())
	ctx := context.Background()
	a := queueAsk(t, h, time.Now())
	h.sources["work"].add("a1")
	h.e.Poll(ctx, true)
	d, err := h.e.Dispatch(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(h.drv.events, ",") != "ask,batch" {
		t.Errorf("driver calls = %v, want the ask first", h.drv.events)
	}
	if h.drv.askHandles[0] != "term_card" || !strings.Contains(h.drv.asks[0].Prompt, "--- ask ---\nResearch more.\n--- end ---") {
		t.Errorf("ask to %q: %s", h.drv.askHandles[0], h.drv.asks[0].Prompt)
	}
	if len(d.Asks) != 1 || d.Asks[0].ID != a.ID || d.Asks[0].State != inbox.AskSent {
		t.Errorf("asks = %+v", d.Asks)
	}
	if q, _ := h.st.QueuedAsks(ctx); len(q) != 0 {
		t.Errorf("queued = %+v, want none", q)
	}
	if s := Summary(nil, d); !strings.Contains(s, fmt.Sprintf("ask %d about card %d for the space unsorted: sent", a.ID, a.CardID)) {
		t.Errorf("summary = %q", s)
	}
}

func TestDispatchBusyAgentKeepsAskQueued(t *testing.T) {
	h := newHarness(t, orcaCfg())
	ctx := context.Background()
	queueAsk(t, h, time.Now())
	h.drv.result = agent.Result{Status: agent.StatusHeld, Detail: "agent busy in term_card"}
	if _, err := h.e.Dispatch(ctx, nil); err != nil {
		t.Fatal(err)
	}
	if q, _ := h.st.QueuedAsks(ctx); len(q) != 1 || q[0].Detail != "agent busy in term_card" {
		t.Errorf("queued = %+v", q)
	}
}

func TestDispatchFailsOldAsk(t *testing.T) {
	h := newHarness(t, orcaCfg())
	ctx := context.Background()
	a := queueAsk(t, h, time.Now().Add(-25*time.Hour))
	d, err := h.e.Dispatch(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(h.drv.asks) != 0 || len(d.Asks) != 1 || d.Asks[0].State != inbox.AskFailed || d.Asks[0].Detail != "not sent within 1d" {
		t.Errorf("asks = %+v, driver calls = %d", d.Asks, len(h.drv.asks))
	}
	by, _ := h.st.Asks(ctx, []int64{a.CardID})
	if by[a.CardID][0].State != inbox.AskFailed {
		t.Errorf("stored ask = %+v", by[a.CardID][0])
	}
}

func TestBatchWaitsForTheSpaceLock(t *testing.T) {
	h := newHarness(t, orcaCfg())
	ctx := context.Background()
	old := lockWait
	lockWait = 200 * time.Millisecond
	t.Cleanup(func() { lockWait = old })
	unlock, err := agent.Lock(ctx, h.paths.DataDir, config.UnsortedID)
	if err != nil {
		t.Fatal(err)
	}
	h.sources["work"].add("a1")
	h.e.Poll(ctx, true)
	d, _ := h.e.Dispatch(ctx, nil)
	if len(d.Batches) != 1 || d.Batches[0].Status != store.DispatchHeld || h.drv.count() != 0 {
		t.Fatalf("batches = %+v, driver calls = %d; want held and no call", d.Batches, h.drv.count())
	}
	unlock()
	d, _ = h.e.Dispatch(ctx, nil)
	if d.Batches[0].Status != store.DispatchSent || h.drv.count() != 1 {
		t.Errorf("after the unlock: batches = %+v, driver calls = %d", d.Batches, h.drv.count())
	}
}

func TestSetConfigAndCloseCloseTheSources(t *testing.T) {
	h := newHarness(t, cfgWith("work"))
	ctx := context.Background()
	src := h.sources["work"]

	h.e.Poll(ctx, true)
	h.e.SetConfig(cfgWith("work"))
	if src.closes != 1 {
		t.Fatalf("closes after SetConfig = %d, want 1", src.closes)
	}
	h.e.Poll(ctx, true) // builds the source again
	h.e.Close()
	if src.closes != 2 {
		t.Fatalf("closes after Close = %d, want 2", src.closes)
	}
	h.e.Close()
	if src.closes != 2 {
		t.Errorf("a second Close closed the source again: %d", src.closes)
	}
}

func TestNotifyNotPairedOnceUntilAGoodPoll(t *testing.T) {
	h := newHarness(t, cfgWith("phone"))
	var got []string
	h.e.d.Notify = func(title, text string) { got = append(got, title+" | "+text) }
	ctx := context.Background()
	src := h.sources["phone"]

	src.err = fmt.Errorf("open: %w", source.ErrNotPaired)
	for range 3 {
		h.e.Poll(ctx, true)
	}
	want := "i2a: WhatsApp not paired | Account phone: pair again in i2a config (Accounts)."
	if len(got) != 1 || got[0] != want {
		t.Fatalf("notifications = %q, want one %q", got, want)
	}

	src.err = errors.New("network down") // other errors do not notify
	h.e.Poll(ctx, true)
	src.err = nil
	h.e.Poll(ctx, true)
	src.err = source.ErrNotPaired
	h.e.Poll(ctx, true)
	if len(got) != 2 {
		t.Errorf("after a good poll, notifications = %q, want 2", got)
	}
}
