package daemon

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"syscall"
	"time"

	"github.com/sondt2709/inbox2agent/internal/config"
)

// ErrLocked is returned when another daemon holds the lock.
var ErrLocked = errors.New("another i2a daemon is running")

// Lock takes the exclusive daemon lock. Call the returned function to release it.
func Lock(path string) (func(), error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, ErrLocked
		}
		return nil, fmt.Errorf("lock %s: %w", path, err)
	}
	_ = f.Truncate(0)
	_, _ = fmt.Fprintf(f, "%d\n", os.Getpid())
	return func() {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		f.Close()
	}, nil
}

// Loader loads and checks the configuration file.
type Loader func() (*config.Config, error)

// tick is the time between loop steps. Tests make it shorter.
var tick = time.Second

// askEvery is the most time between two tries of the queued asks. A space
// can send every few hours; an ask of the user must not wait for that.
var askEvery = time.Minute

// Run is the daemon loop. It reads the accounts on their intervals, sends
// each space at the send times of its schedule, tries the queued asks each
// minute, reloads the configuration when the file changes, and prunes old
// data once each hour.
func Run(ctx context.Context, e *Engine, configFile string, load Loader) error {
	log := e.d.Log
	modTime := fileModTime(configFile)
	var lastPrune, lastAsk time.Time
	next := nextSends(e.cfg, e.d.Now())
	log.Info("daemon started", "accounts", len(e.cfg.Accounts), "spaces", len(e.cfg.Spaces), "agents", len(e.cfg.Agents))
	logNext(log, e.cfg, next)
	var runs chan runRequest // nil without an API: the select never takes it
	if e.api != nil {
		runs = e.api.runs
	}
	e.api.setStatus(spaceNexts(e.cfg, next), nil)

	t := time.NewTicker(tick)
	defer t.Stop()
	for {
		if mt := fileModTime(configFile); !mt.Equal(modTime) {
			modTime = mt
			cfg, err := load()
			if err != nil {
				log.Error("configuration not reloaded, the previous configuration stays", "err", err)
				first, _, _ := strings.Cut(err.Error(), "\n")
				e.api.Publish(Event{Kind: EventError, Detail: "config.toml not reloaded: " + first})
			} else {
				e.SetConfig(cfg)
				next = nextSends(cfg, e.d.Now())
				log.Info("configuration reloaded", "accounts", len(cfg.Accounts), "spaces", len(cfg.Spaces))
				logNext(log, cfg, next)
				e.api.Publish(Event{Kind: EventReload, Count: len(cfg.Spaces)})
			}
			e.api.setStatus(spaceNexts(e.cfg, next), nil)
		}

		e.Poll(ctx, false)

		now := e.d.Now()
		var due []string
		for _, s := range e.cfg.AllSpaces() {
			if at, ok := next[s.ID]; ok && !now.Before(at) {
				due = append(due, s.ID)
				if sc, ok := e.cfg.Schedule(s.Schedule); ok {
					next[s.ID] = sc.Next(now)
				}
			}
		}
		if len(due) > 0 {
			e.api.setStatus(spaceNexts(e.cfg, next), nil)
		}
		switch {
		case len(due) > 0:
			lastAsk = now
			if _, err := e.Dispatch(ctx, due); err != nil {
				log.Error("dispatch round failed", "err", err)
			}
		case now.Sub(lastAsk) >= askEvery:
			lastAsk = now
			if _, err := e.Expire(ctx); err != nil {
				log.Error("expire failed", "err", err)
			}
			if _, err := e.SendAsks(ctx); err != nil {
				log.Error("asks failed", "err", err)
			}
		}

		if now.Sub(lastPrune) >= time.Hour {
			lastPrune = now
			if err := e.Prune(ctx); err != nil {
				log.Error("retention failed", "err", err)
			}
		}

		select {
		case <-ctx.Done():
			log.Info("daemon stopped")
			return nil
		case <-t.C:
		case rr := <-runs:
			// A run of the API: the loop does it, so it never runs at the
			// same time as a scheduled send.
			lastAsk = e.d.Now()
			log.Info("run from the API", "spaces", rr.spaces)
			res, err := e.RunNow(ctx, rr.spaces)
			rr.reply <- runReply{res: res, err: err}
		}
	}
}

// nextSends returns the next send time of each space that has an agent.
func nextSends(cfg *config.Config, now time.Time) map[string]time.Time {
	out := map[string]time.Time{}
	for _, s := range cfg.AllSpaces() {
		if s.Agent == "" {
			continue
		}
		if sc, ok := cfg.Schedule(s.Schedule); ok {
			if at := sc.Next(now); !at.IsZero() {
				out[s.ID] = at
			}
		}
	}
	return out
}

// spaceNexts returns the next send of each space for the status of the API.
func spaceNexts(cfg *config.Config, next map[string]time.Time) []SpaceNext {
	out := []SpaceNext{}
	for _, s := range cfg.AllSpaces() {
		n := SpaceNext{Space: s.ID, Name: s.Title(), Agent: s.Agent}
		if at, ok := next[s.ID]; ok {
			n.At = &at
		}
		out = append(out, n)
	}
	return out
}

func logNext(log *slog.Logger, cfg *config.Config, next map[string]time.Time) {
	for _, s := range cfg.AllSpaces() {
		if at, ok := next[s.ID]; ok {
			log.Info("next send", "space", s.ID, "agent", s.Agent, "schedule", s.Schedule, "at", at.Format(time.DateTime))
		} else {
			log.Info("space does not send", "space", s.ID, "agent", s.Agent)
		}
	}
}

func fileModTime(path string) time.Time {
	info, err := os.Stat(path)
	if err != nil {
		return time.Time{}
	}
	return info.ModTime()
}
