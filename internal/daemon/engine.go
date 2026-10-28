// Package daemon collects messages from the sources and dispatches batches to the drivers.
package daemon

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/sondt2709/inbox2agent/internal/agent"
	"github.com/sondt2709/inbox2agent/internal/app"
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

// Deps are the parts that the engine uses. Tests replace NewDriver and BuildSource.
type Deps struct {
	Paths       app.Paths
	Store       *store.Store
	Secrets     secrets.Store
	Log         *slog.Logger
	HTTP        *http.Client
	Now         func() time.Time
	NewDriver   func(p config.Profile, s agent.Sessions) (agent.Driver, error)
	BuildSource func(sc config.Source, sec secrets.Store, lookback time.Duration, hc *http.Client) (source.Source, error)
	Notify      notify.Notifier
}

// Engine runs polls and dispatches for one configuration.
type Engine struct {
	d        Deps
	cfg      *config.Config
	sources  map[string]source.Source
	lastPoll map[string]time.Time
}

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
	e := &Engine{d: d, lastPoll: map[string]time.Time{}}
	e.SetConfig(cfg)
	return e
}

// SetConfig replaces the configuration. Sources are built again on the next poll.
func (e *Engine) SetConfig(cfg *config.Config) {
	e.cfg = cfg
	e.sources = map[string]source.Source{}
}

// Config returns the current configuration.
func (e *Engine) Config() *config.Config { return e.cfg }

// PollResult is the result of one poll of one source.
type PollResult struct {
	SourceID string
	New      int
	Err      error
}

// Poll polls each source whose poll interval has passed. When force is true, it polls all sources.
func (e *Engine) Poll(ctx context.Context, force bool) []PollResult {
	var out []PollResult
	now := e.d.Now()
	for _, sc := range e.cfg.Sources {
		if !force && now.Sub(e.lastPoll[sc.ID]) < sc.PollEvery.D() {
			continue
		}
		e.lastPoll[sc.ID] = now
		n, err := e.pollOne(ctx, sc, now)
		if err != nil {
			e.d.Log.Error("poll failed", "source", sc.ID, "err", err)
		} else {
			e.d.Log.Info("poll", "source", sc.ID, "new", n)
		}
		out = append(out, PollResult{SourceID: sc.ID, New: n, Err: err})
	}
	return out
}

func (e *Engine) pollOne(ctx context.Context, sc config.Source, now time.Time) (int, error) {
	src, ok := e.sources[sc.ID]
	if !ok {
		var err error
		src, err = e.d.BuildSource(sc, e.d.Secrets, e.cfg.Timing.Lookback.D(), e.d.HTTP)
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

// BatchResult is the result of one dispatch.
type BatchResult struct {
	Profile    string
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
	Batches []BatchResult
}

// Dispatch expires old messages, routes the pending messages to profiles and
// delivers one batch for each profile.
func (e *Engine) Dispatch(ctx context.Context) (DispatchResult, error) {
	var res DispatchResult
	now := e.d.Now()
	n, err := e.d.Store.ExpirePending(ctx, now.Add(-e.cfg.Timing.Lookback.D()))
	if err != nil {
		return res, err
	}
	res.Expired = n
	if n > 0 {
		e.d.Log.Info("expired old pending messages", "count", n, "lookback", e.cfg.Timing.Lookback.String())
	}
	pending, err := e.d.Store.Pending(ctx)
	if err != nil {
		return res, err
	}
	groups := map[string][]inbox.Message{}
	for _, m := range pending {
		p := agent.Route(e.cfg, m)
		groups[p] = append(groups[p], m)
	}
	for _, p := range e.cfg.Profiles {
		msgs := groups[p.Name]
		delete(groups, p.Name)
		if p.Driver == config.DriverDryRun {
			// A dry run previews each message one time.
			msgs = slices.DeleteFunc(msgs, func(m inbox.Message) bool { return m.State == inbox.StatePreviewed })
		}
		if len(msgs) == 0 {
			continue
		}
		br := e.deliver(ctx, p, msgs, now)
		e.notify(p, br)
		res.Batches = append(res.Batches, br)
	}
	for name, msgs := range groups {
		e.d.Log.Warn("messages route to a profile that does not exist", "profile", name, "count", len(msgs))
	}
	return res, nil
}

func (e *Engine) deliver(ctx context.Context, p config.Profile, msgs []inbox.Message, now time.Time) BatchResult {
	br := BatchResult{Profile: p.Name, Driver: p.Driver, Count: len(msgs)}
	fail := func(id int64, err error) BatchResult {
		br.Status, br.Err = store.DispatchFailed, err
		if id > 0 {
			if ferr := e.d.Store.FinishDispatch(ctx, id, store.DispatchFailed, err.Error(), p.Name, "", nil); ferr != nil {
				e.d.Log.Error("record dispatch", "err", ferr)
			}
		}
		e.d.Log.Error("dispatch failed", "profile", p.Name, "driver", p.Driver, "messages", len(msgs), "err", err)
		return br
	}
	id, err := e.d.Store.CreateDispatch(ctx, store.Dispatch{Profile: p.Name, Driver: p.Driver, CreatedAt: now, MessageCount: len(msgs)})
	if err != nil {
		return fail(0, err)
	}
	r, err := render.Render(p, render.BatchID(now, p.Name, id), msgs, e.d.Paths.DigestDir)
	if err != nil {
		return fail(id, err)
	}
	br.DigestPath = r.DigestPath
	if err := e.d.Store.SetDigestPath(ctx, id, r.DigestPath); err != nil {
		return fail(id, err)
	}
	drv, err := e.d.NewDriver(p, e.d.Store)
	if err != nil {
		return fail(id, err)
	}
	dctx, cancel := context.WithTimeout(ctx, deliverTimeout)
	defer cancel()
	out, err := drv.Deliver(dctx, r)
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
	if err := e.d.Store.FinishDispatch(ctx, id, out.Status, br.Detail, p.Name, state, ids); err != nil {
		return fail(id, err)
	}
	e.d.Log.Info("dispatch", "profile", p.Name, "driver", p.Driver, "messages", len(msgs), "status", out.Status,
		"detail", br.Detail, "digest", r.DigestPath)
	return br
}

// notify shows a desktop notification for a dispatch to a real driver.
// The text has the profile, the count and the driver detail, never message text.
func (e *Engine) notify(p config.Profile, br BatchResult) {
	if !e.cfg.Notify.Enabled || p.Driver == config.DriverDryRun {
		return
	}
	switch {
	case br.Err != nil:
		e.d.Notify("i2a: dispatch failed", notify.Short(fmt.Sprintf("Profile %s, %d messages stay pending: %v", p.Name, br.Count, br.Err), 200))
	case br.Status == store.DispatchSent:
		e.d.Notify(fmt.Sprintf("i2a: %d messages sent", br.Count), notify.Short(fmt.Sprintf("Profile %s to %s. %s", p.Name, p.Agent, br.Detail), 200))
	}
}

// Prune deletes old sent and expired messages and their digest files.
func (e *Engine) Prune(ctx context.Context) error {
	before := e.d.Now().Add(-e.cfg.Timing.Retention.D())
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
			fmt.Fprintf(&b, "source %s: error: %v\n", p.SourceID, p.Err)
			continue
		}
		total += p.New
		fmt.Fprintf(&b, "source %s: %d new messages\n", p.SourceID, p.New)
	}
	fmt.Fprintf(&b, "collected %d new messages\n", total)
	if d.Expired > 0 {
		fmt.Fprintf(&b, "expired %d messages older than the lookback\n", d.Expired)
	}
	if len(d.Batches) == 0 {
		b.WriteString("no batch to send\n")
	}
	for _, br := range d.Batches {
		if br.Err != nil {
			fmt.Fprintf(&b, "batch for %s (%s, %d messages): failed: %v\n", br.Profile, br.Driver, br.Count, br.Err)
			continue
		}
		fmt.Fprintf(&b, "batch for %s (%s, %d messages): %s", br.Profile, br.Driver, br.Count, br.Status)
		if br.Detail != "" {
			fmt.Fprintf(&b, " - %s", br.Detail)
		}
		fmt.Fprintf(&b, "\n  digest: %s\n", br.DigestPath)
	}
	return b.String()
}
