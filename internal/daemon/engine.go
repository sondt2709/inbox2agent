// Package daemon reads the accounts and sends the waiting messages of each
// space to its agent on the schedule of the space.
package daemon

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/sondt2709/inbox2agent/internal/agent"
	"github.com/sondt2709/inbox2agent/internal/app"
	"github.com/sondt2709/inbox2agent/internal/ask"
	"github.com/sondt2709/inbox2agent/internal/config"
	"github.com/sondt2709/inbox2agent/internal/inbox"
	"github.com/sondt2709/inbox2agent/internal/notify"
	"github.com/sondt2709/inbox2agent/internal/render"
	"github.com/sondt2709/inbox2agent/internal/secrets"
	"github.com/sondt2709/inbox2agent/internal/source"
	"github.com/sondt2709/inbox2agent/internal/store"
)

// deliverTimeout limits one driver call.
const deliverTimeout = 3 * time.Minute

// askIdleWait is the wait for a busy agent when the daemon sends a queued ask,
// the same as for a batch in reuse mode.
const askIdleWait = 5 * time.Second

// lockWait limits the wait for the delivery lock of a space, which an ask
// can hold. Tests make it shorter.
var lockWait = ask.LockWait

// Deps are the parts that the engine uses. Tests replace NewDriver and BuildSource.
type Deps struct {
	Paths       app.Paths
	Store       *store.Store
	Secrets     secrets.Store
	Log         *slog.Logger
	HTTP        *http.Client
	Now         func() time.Time
	NewDriver   func(a config.Agent, space string, s agent.Sessions) (agent.Driver, error)
	BuildSource func(a config.Account, env source.Env) (source.Source, error)
	Notify      notify.Notifier
	// CardCommand runs this i2a for the agent ("i2a card add"). The default is
	// the path of this executable, with I2A_HOME when it is set.
	CardCommand string
}

// Engine runs reads and dispatches for one configuration.
type Engine struct {
	d        Deps
	cfg      *config.Config
	sources  map[string]source.Source
	lastPoll map[string]time.Time
	unpaired map[string]bool // WhatsApp accounts that got ErrNotPaired since their last good read
	api      *Server         // the API of the daemon, or nil
	reads    map[string]AccountRead
}

// SetAPI connects the API server: the loop serves its runs, and the engine
// publishes its events and its status.
func (e *Engine) SetAPI(s *Server) { e.api = s }

// New returns an engine. Nil fields in Deps get the default values.
func New(d Deps, cfg *config.Config) *Engine {
	if d.Now == nil {
		d.Now = time.Now
	}
	if d.Log == nil {
		d.Log = slog.New(slog.NewTextHandler(os.Stderr, nil))
	}
	if d.HTTP == nil {
		d.HTTP = &http.Client{Timeout: 30 * time.Second}
	}
	if d.NewDriver == nil {
		d.NewDriver = agent.New
	}
	if d.BuildSource == nil {
		d.BuildSource = source.Build
	}
	if d.Notify == nil {
		d.Notify = func(string, string) {}
	}
	if d.CardCommand == "" {
		exe, err := os.Executable()
		if err != nil {
			exe = "i2a"
		}
		d.CardCommand = render.CardCommand(exe, os.Getenv("I2A_HOME"))
	}
	e := &Engine{d: d, lastPoll: map[string]time.Time{}, unpaired: map[string]bool{}, reads: map[string]AccountRead{}}
	e.SetConfig(cfg)
	return e
}

// SetConfig replaces the configuration. It closes the old sources; sources
// are built again on the next read, with the new read levels.
func (e *Engine) SetConfig(cfg *config.Config) {
	e.Close()
	e.cfg = cfg
}

// Close closes the sources that hold a connection (a WhatsApp source).
func (e *Engine) Close() {
	for id, src := range e.sources {
		if c, ok := src.(io.Closer); ok {
			if err := c.Close(); err != nil {
				e.d.Log.Error("close source", "account", id, "err", err)
			}
		}
	}
	e.sources = map[string]source.Source{}
}

// Config returns the current configuration.
func (e *Engine) Config() *config.Config { return e.cfg }

// PollResult is the result of one read of one account.
type PollResult struct {
	AccountID string
	New       int
	Err       error
}

// Poll reads each account whose read interval has passed. When force is true, it reads all accounts.
func (e *Engine) Poll(ctx context.Context, force bool) []PollResult {
	var out []PollResult
	now := e.d.Now()
	for _, sc := range e.cfg.Accounts {
		if !force && now.Sub(e.lastPoll[sc.ID]) < sc.ReadEvery.D() {
			continue
		}
		e.lastPoll[sc.ID] = now
		n, err := e.pollOne(ctx, sc, now)
		if err != nil {
			e.d.Log.Error("poll failed", "account", sc.ID, "err", err)
		} else {
			e.d.Log.Info("poll", "account", sc.ID, "new", n)
		}
		e.notifyPaired(sc.ID, err)
		out = append(out, PollResult{AccountID: sc.ID, New: n, Err: err})
	}
	e.publishReads(out, now)
	return out
}

// publishReads records the reads in the status of the API and publishes a
// read event for each read with new messages or an error.
func (e *Engine) publishReads(polls []PollResult, now time.Time) {
	if e.api == nil || len(polls) == 0 {
		return
	}
	for _, p := range polls {
		r := AccountRead{Account: p.AccountID, At: now, New: p.New}
		if p.Err != nil {
			r.Error = p.Err.Error()
		}
		e.reads[p.AccountID] = r
		if p.New > 0 || p.Err != nil {
			e.api.Publish(Event{At: now, Kind: EventRead, Account: p.AccountID, Count: p.New, Detail: r.Error})
		}
	}
	var reads []AccountRead
	for _, a := range e.cfg.Accounts {
		if r, ok := e.reads[a.ID]; ok {
			reads = append(reads, r)
		}
	}
	e.api.setStatus(nil, reads)
}

// publishDispatch publishes a send event for each batch and an ask event for
// each ask that a dispatch tried.
func (e *Engine) publishDispatch(d DispatchResult) {
	for _, b := range d.Batches {
		ev := Event{Kind: EventSend, Space: b.Space, Agent: b.Agent, Count: b.Count, Status: b.Status, Detail: b.Detail}
		if b.Err != nil {
			ev.Status, ev.Detail = "failed", b.Err.Error()
		}
		e.api.Publish(ev)
	}
	for _, a := range d.Asks {
		e.api.Publish(Event{Kind: EventAsk, Space: a.Space, Status: a.State, Detail: a.Detail})
	}
}

func (e *Engine) pollOne(ctx context.Context, sc config.Account, now time.Time) (int, error) {
	src, ok := e.sources[sc.ID]
	if !ok {
		var err error
		src, err = e.d.BuildSource(sc, source.Env{Secrets: e.d.Secrets, Lookback: e.cfg.Settings.Lookback.D(), HTTP: e.d.HTTP,
			DataDir: e.d.Paths.DataDir, Log: e.d.Log, Levels: e.cfg.Levels(sc.ID), Default: e.cfg.Unsorted.Read})
		if err != nil {
			return 0, err
		}
		e.sources[sc.ID] = src
	}
	cur, err := e.d.Store.Cursor(ctx, sc.ID)
	if err != nil {
		return 0, err
	}
	msgs, next, err := src.Collect(ctx, cur, now)
	if err != nil {
		return 0, err
	}
	return e.d.Store.SavePoll(ctx, sc.ID, msgs, next, now)
}

// BatchResult is the result of one dispatch of one space.
type BatchResult struct {
	Space      string // the ID of the space
	SpaceName  string
	Agent      string // the ID of the agent
	Driver     string
	Count      int
	Status     string
	Detail     string
	DigestPath string
	Err        error
}

// DispatchResult is the result of one dispatch round.
type DispatchResult struct {
	Expired int64
	Asks    []inbox.Ask // the queued asks after this dispatch tried to send them
	Batches []BatchResult
}

// Dispatch expires old messages, sends the queued asks, routes the pending
// messages to their spaces and delivers one batch for each given space that
// has an agent. spaces nil means all spaces. The messages of other spaces stay
// pending.
func (e *Engine) Dispatch(ctx context.Context, spaces []string) (DispatchResult, error) {
	var res DispatchResult
	now := e.d.Now()
	n, err := e.Expire(ctx)
	if err != nil {
		return res, err
	}
	res.Expired = n
	if res.Asks, err = e.SendAsks(ctx); err != nil {
		return res, err
	}
	pending, err := e.d.Store.Pending(ctx)
	if err != nil {
		return res, err
	}
	groups := map[string][]inbox.Message{}
	for _, m := range pending {
		sp := e.cfg.Route(m.SourceID, m.ChannelID)
		groups[sp] = append(groups[sp], m)
	}
	for _, s := range e.cfg.AllSpaces() {
		if s.Agent == "" || (spaces != nil && !slices.Contains(spaces, s.ID)) {
			continue
		}
		a, ok := e.cfg.Agent(s.Agent)
		if !ok {
			continue
		}
		msgs := groups[s.ID]
		if a.Driver == config.DriverDryRun {
			// A dry run previews each message one time.
			msgs = slices.DeleteFunc(msgs, func(m inbox.Message) bool { return m.State == inbox.StatePreviewed })
		}
		if len(msgs) == 0 {
			continue
		}
		br := e.deliver(ctx, s, *a, msgs, now)
		e.notify(*a, br)
		res.Batches = append(res.Batches, br)
	}
	e.publishDispatch(res)
	return res, nil
}

// RunNow reads all accounts and sends the given spaces (nil: all spaces), as
// "i2a run --once" does. An unknown space is an error, and nothing runs.
func (e *Engine) RunNow(ctx context.Context, spaces []string) (RunResult, error) {
	for _, id := range spaces {
		if _, ok := e.cfg.Space(id); !ok && id != config.UnsortedID {
			return RunResult{}, fmt.Errorf("%w: %q", ErrUnknownSpace, id)
		}
	}
	if len(spaces) == 0 {
		spaces = nil
	}
	e.api.Publish(Event{Kind: EventRun, Status: "started", Detail: strings.Join(spaces, ",")})
	polls := e.Poll(ctx, true)
	d, err := e.Dispatch(ctx, spaces)
	if err != nil {
		e.api.Publish(Event{Kind: EventError, Detail: err.Error()})
		return RunResult{}, err
	}
	res := RunResult{Summary: Summary(polls, d), Reads: []AccountRead{}, Batches: []Batch{}}
	for _, p := range polls {
		r := AccountRead{Account: p.AccountID, At: e.d.Now(), New: p.New}
		if p.Err != nil {
			r.Error, res.Failed = p.Err.Error(), true
		}
		res.Reads = append(res.Reads, r)
	}
	for _, b := range d.Batches {
		x := Batch{Space: b.Space, Agent: b.Agent, Driver: b.Driver, Count: b.Count, Status: b.Status, Detail: b.Detail}
		if b.Err != nil {
			x.Error, res.Failed = b.Err.Error(), true
		}
		res.Batches = append(res.Batches, x)
	}
	e.api.Publish(Event{Kind: EventRun, Status: "done", Count: len(d.Batches)})
	return res, nil
}

// Expire marks the pending messages older than the lookback expired. It also
// ends the wait of messages that Unsorted holds.
func (e *Engine) Expire(ctx context.Context) (int64, error) {
	n, err := e.d.Store.ExpirePending(ctx, e.d.Now().Add(-e.cfg.Settings.Lookback.D()))
	if err == nil && n > 0 {
		e.d.Log.Info("expired old pending messages", "count", n, "lookback", e.cfg.Settings.Lookback.String())
	}
	return n, err
}

// SendAsks sends the queued asks, oldest first. An ask that stays busy stays
// queued for the next try.
func (e *Engine) SendAsks(ctx context.Context) ([]inbox.Ask, error) {
	queued, err := e.d.Store.QueuedAsks(ctx)
	if err != nil || len(queued) == 0 {
		return nil, err
	}
	d := ask.Deps{Store: e.d.Store, Config: e.cfg, NewDriver: e.d.NewDriver, CardCommand: e.d.CardCommand,
		LockDir: e.d.Paths.DataDir, Now: e.d.Now}
	out := make([]inbox.Ask, 0, len(queued))
	for _, a := range queued {
		actx, cancel := context.WithTimeout(ctx, deliverTimeout+ask.LockWait)
		a, err := ask.Retry(actx, d, a, e.cfg.Settings.Lookback.D(), askIdleWait)
		cancel()
		if err != nil {
			e.d.Log.Error("ask failed", "ask", a.ID, "card", a.CardID, "space", a.Space, "err", err)
		} else {
			e.d.Log.Info("ask", "ask", a.ID, "card", a.CardID, "space", a.Space, "state", a.State, "detail", a.Detail)
		}
		out = append(out, a)
	}
	return out, nil
}

func (e *Engine) deliver(ctx context.Context, s config.Space, p config.Agent, msgs []inbox.Message, now time.Time) BatchResult {
	br := BatchResult{Space: s.ID, SpaceName: s.Title(), Agent: p.ID, Driver: p.Driver, Count: len(msgs)}
	fail := func(id int64, err error) BatchResult {
		br.Status, br.Err = store.DispatchFailed, err
		if id > 0 {
			if ferr := e.d.Store.FinishDispatch(ctx, id, store.DispatchFailed, err.Error(), s.ID, "", nil); ferr != nil {
				e.d.Log.Error("record dispatch", "err", ferr)
			}
		}
		e.d.Log.Error("dispatch failed", "space", s.ID, "agent", p.ID, "driver", p.Driver, "messages", len(msgs), "err", err)
		return br
	}
	id, err := e.d.Store.CreateDispatch(ctx, store.Dispatch{Space: s.ID, Agent: p.ID, Driver: p.Driver, CreatedAt: now, MessageCount: len(msgs)})
	if err != nil {
		return fail(0, err)
	}
	cardCommand := e.d.CardCommand
	if p.Driver == config.DriverDryRun {
		cardCommand = "" // no agent reads a dry run
	}
	r, err := render.Render(p, s.Title(), render.BatchID(now, s.ID, id), msgs, e.d.Paths.DigestDir, cardCommand)
	if err != nil {
		return fail(id, err)
	}
	br.DigestPath = r.DigestPath
	if err := e.d.Store.SetDigestPath(ctx, id, r.DigestPath); err != nil {
		return fail(id, err)
	}
	drv, err := e.d.NewDriver(p, s.ID, e.d.Store)
	if err != nil {
		return fail(id, err)
	}
	out, err := e.deliverLocked(ctx, s.ID, p, drv, r)
	if err != nil {
		return fail(id, err)
	}
	br.Status = out.Status
	br.Detail = strings.Trim(strings.Join([]string{r.Detail, out.Detail}, "; "), "; ")
	ids := make([]int64, len(msgs))
	for i, m := range msgs {
		ids[i] = m.ID
	}
	state := inbox.StateSent
	if p.Driver == config.DriverDryRun {
		state = inbox.StatePreviewed
	}
	if err := e.d.Store.FinishDispatch(ctx, id, out.Status, br.Detail, s.ID, state, ids); err != nil {
		return fail(id, err)
	}
	e.d.Log.Info("dispatch", "space", s.ID, "agent", p.ID, "driver", p.Driver, "messages", len(msgs), "status", out.Status,
		"detail", br.Detail, "digest", r.DigestPath)
	return br
}

// deliverLocked calls the driver while it holds the delivery lock of the
// space, so a batch and an ask never type into one terminal at the same
// time. When an ask holds the lock longer than lockWait, the batch is held.
func (e *Engine) deliverLocked(ctx context.Context, space string, p config.Agent, drv agent.Driver, r *render.Rendered) (agent.Result, error) {
	if p.Driver != config.DriverDryRun {
		lctx, cancel := context.WithTimeout(ctx, lockWait)
		unlock, err := agent.Lock(lctx, e.d.Paths.DataDir, space)
		cancel()
		if errors.Is(err, agent.ErrBusy) {
			return agent.Result{Status: agent.StatusHeld, Detail: err.Error()}, nil
		}
		if err != nil {
			return agent.Result{}, err
		}
		defer unlock()
	}
	dctx, cancel := context.WithTimeout(ctx, deliverTimeout)
	defer cancel()
	return drv.Deliver(dctx, r)
}

// notify shows a desktop notification for a failed dispatch to a real driver.
// A sent batch gives no notification: the cards of the agent do (i2a card add).
// The text has the space, the agent, the count and the error, never message text.
func (e *Engine) notify(p config.Agent, br BatchResult) {
	if !e.cfg.Notify.Enabled || p.Driver == config.DriverDryRun || br.Err == nil {
		return
	}
	e.d.Notify("i2a: dispatch failed", notify.Short(fmt.Sprintf("Space %s (agent %s), %d messages stay pending: %v", br.SpaceName, p.Title(), br.Count, br.Err), 200))
}

// notifyPaired shows one notification when an account becomes not paired, and
// again only after a good read of that account.
func (e *Engine) notifyPaired(id string, err error) {
	if err == nil {
		delete(e.unpaired, id)
		return
	}
	if !errors.Is(err, source.ErrNotPaired) || e.unpaired[id] {
		return
	}
	e.unpaired[id] = true
	if e.cfg.Notify.Enabled {
		e.d.Notify("i2a: WhatsApp not paired", "Account "+id+": pair again in i2a config (Accounts).")
	}
}

// Prune deletes old sent and expired messages and their digest files.
func (e *Engine) Prune(ctx context.Context) error {
	before := e.d.Now().Add(-e.cfg.Settings.Retention.D())
	n, paths, err := e.d.Store.Prune(ctx, before)
	if err != nil {
		return err
	}
	removed := 0
	for _, p := range paths {
		if err := os.Remove(p); err == nil {
			removed++
		} else if !errors.Is(err, os.ErrNotExist) {
			e.d.Log.Warn("remove digest", "path", p, "err", err)
		}
	}
	if n > 0 || removed > 0 {
		e.d.Log.Info("retention", "messages_deleted", n, "digests_deleted", removed)
	}
	return nil
}

// Summary returns a short text for a poll and a dispatch, for the CLI.
func Summary(polls []PollResult, d DispatchResult) string {
	var b strings.Builder
	total := 0
	for _, p := range polls {
		if p.Err != nil {
			fmt.Fprintf(&b, "account %s: error: %v\n", p.AccountID, p.Err)
			continue
		}
		total += p.New
		fmt.Fprintf(&b, "account %s: %d new messages\n", p.AccountID, p.New)
	}
	fmt.Fprintf(&b, "collected %d new messages\n", total)
	if d.Expired > 0 {
		fmt.Fprintf(&b, "expired %d messages older than the lookback\n", d.Expired)
	}
	for _, a := range d.Asks {
		fmt.Fprintf(&b, "ask %d about card %d for the space %s: %s", a.ID, a.CardID, a.Space, a.State)
		if a.Detail != "" {
			fmt.Fprintf(&b, " - %s", a.Detail)
		}
		b.WriteString("\n")
	}
	if len(d.Batches) == 0 {
		b.WriteString("no batch to send\n")
	}
	for _, br := range d.Batches {
		if br.Err != nil {
			fmt.Fprintf(&b, "batch of %s to %s (%s, %d messages): failed: %v\n", br.SpaceName, br.Agent, br.Driver, br.Count, br.Err)
			continue
		}
		fmt.Fprintf(&b, "batch of %s to %s (%s, %d messages): %s", br.SpaceName, br.Agent, br.Driver, br.Count, br.Status)
		if br.Detail != "" {
			fmt.Fprintf(&b, " - %s", br.Detail)
		}
		fmt.Fprintf(&b, "\n  digest: %s\n", br.DigestPath)
	}
	return b.String()
}
