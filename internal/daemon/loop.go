package daemon

import (
	"context"
	"errors"
	"fmt"
	"os"
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

// Run is the daemon loop. It polls the sources on their intervals, dispatches
// on the timing interval inside the active hours, reloads the configuration
// when the file changes, and prunes old data once each hour.
func Run(ctx context.Context, e *Engine, configFile string, load Loader) error {
	log := e.d.Log
	modTime := fileModTime(configFile)
	lastDispatch := e.d.Now()
	var lastPrune time.Time
	log.Info("daemon started", "interval", e.cfg.Timing.Interval.String(), "sources", len(e.cfg.Sources),
		"profiles", len(e.cfg.Profiles), "active_hours", e.cfg.Timing.ActiveHours)

	t := time.NewTicker(tick)
	defer t.Stop()
	for {
		if mt := fileModTime(configFile); !mt.Equal(modTime) {
			modTime = mt
			cfg, err := load()
			if err != nil {
				log.Error("configuration not reloaded, the previous configuration stays", "err", err)
			} else {
				e.SetConfig(cfg)
				log.Info("configuration reloaded", "interval", cfg.Timing.Interval.String(), "sources", len(cfg.Sources))
			}
		}

		e.Poll(ctx, false)

		now := e.d.Now()
		if now.Sub(lastDispatch) >= e.cfg.Timing.Interval.D() {
			lastDispatch = now
			hours, _ := config.ParseActiveHours(e.cfg.Timing.ActiveHours)
			if hours.Contains(now) {
				if _, err := e.Dispatch(ctx); err != nil {
					log.Error("dispatch round failed", "err", err)
				}
			} else {
				counts, _ := e.d.Store.Counts(ctx)
				log.Info("outside active hours, no dispatch", "active_hours", e.cfg.Timing.ActiveHours, "pending", counts["pending"])
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
