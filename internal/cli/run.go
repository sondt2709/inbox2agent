package cli

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/sondt2709/inbox2agent/internal/config"
	"github.com/sondt2709/inbox2agent/internal/daemon"
	"github.com/sondt2709/inbox2agent/internal/notify"
	"github.com/sondt2709/inbox2agent/internal/render"
)

func newRunCmd() *cobra.Command {
	var once bool
	cmd := &cobra.Command{
		Use:   "run --once",
		Short: "Collect new messages and send them now, one time",
		Long: `Poll all sources one time, then send all pending messages to their profiles.
This command ignores the interval and the active hours.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if !once {
				return errors.New("use `i2a run --once`, or `i2a daemon` to run continuously")
			}
			e, err := newEnv()
			if err != nil {
				return err
			}
			cfg, err := e.loadConfig()
			if err != nil {
				return err
			}
			st, err := e.openStore()
			if err != nil {
				return err
			}
			defer st.Close()
			log := slog.New(slog.NewTextHandler(cmd.ErrOrStderr(), &slog.HandlerOptions{Level: slog.LevelWarn}))
			eng := daemon.New(daemon.Deps{Paths: e.paths, Store: st, Secrets: e.secrets, Log: log, Notify: notify.Desktop()}, cfg)
			polls := eng.Poll(cmd.Context(), true)
			d, err := eng.Dispatch(cmd.Context())
			if err != nil {
				return err
			}
			fmt.Fprint(cmd.OutOrStdout(), daemon.Summary(polls, d))
			for _, p := range polls {
				if p.Err != nil {
					return errors.New("one or more sources failed")
				}
			}
			for _, b := range d.Batches {
				if b.Err != nil {
					return errors.New("one or more batches failed")
				}
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&once, "once", false, "run one poll and one dispatch, then exit")
	return cmd
}

func newDaemonCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "daemon",
		Short: "Run continuously: poll the sources and dispatch on the interval",
		Long: `Run in the foreground. Poll each source on its poll interval and dispatch
on the timing interval inside the active hours. The daemon reads the
configuration again when the file changes. Use "i2a service install" to run
it in the background at login.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			e, err := newEnv()
			if err != nil {
				return err
			}
			if err := e.paths.EnsureDataDir(); err != nil {
				return err
			}
			unlock, err := daemon.Lock(e.paths.LockFile)
			if err != nil {
				return err
			}
			defer unlock()
			cfg, err := e.loadConfig()
			if err != nil {
				return err
			}
			st, err := e.openStore()
			if err != nil {
				return err
			}
			defer st.Close()
			log := slog.New(slog.NewTextHandler(cmd.ErrOrStderr(), nil))
			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			eng := daemon.New(daemon.Deps{Paths: e.paths, Store: st, Secrets: e.secrets, Log: log, Notify: notify.Desktop()}, cfg)
			load := func() (*config.Config, error) {
				c, err := config.Load(e.paths.ConfigFile)
				if err != nil {
					return nil, err
				}
				return c, render.CheckProfiles(c)
			}
			return daemon.Run(ctx, eng, e.paths.ConfigFile, load)
		},
	}
}
