package cli

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/sondt2709/inbox2agent/internal/app"
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
		Long: `Read each account one time, then send the waiting messages of each space
to the agent of the space. This command does not wait for the schedule of a
space. A space with no agent keeps its messages. When the daemon runs, the
daemon does the run, so only one process reads the accounts.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if !once {
				return errors.New("use `i2a run --once`, or `i2a daemon` to run continuously")
			}
			e, err := newEnv()
			if err != nil {
				return err
			}
			// A running daemon does the run: one process reads the accounts.
			if res, err := daemon.NewClient(e.paths.DataDir).Run(cmd.Context(), nil); !errors.Is(err, daemon.ErrNoDaemon) {
				if err != nil {
					return err
				}
				fmt.Fprint(cmd.OutOrStdout(), "the daemon did the run\n"+res.Summary)
				if res.Failed {
					return errors.New("one or more accounts or batches failed")
				}
				return nil
			}
			// No daemon: hold its lock, so a daemon does not start and read the
			// same accounts while this process runs.
			if err := e.paths.EnsureDataDir(); err != nil {
				return err
			}
			unlock, err := daemon.Lock(e.paths.LockFile)
			if errors.Is(err, daemon.ErrLocked) {
				return errors.New("a daemon holds its lock but does not answer yet: try again in a few seconds")
			}
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
			log := slog.New(slog.NewTextHandler(cmd.ErrOrStderr(), &slog.HandlerOptions{Level: slog.LevelWarn}))
			eng := daemon.New(daemon.Deps{Paths: e.paths, Store: st, Secrets: e.secrets, Log: log, Notify: notify.Desktop()}, cfg)
			defer eng.Close()
			polls := eng.Poll(cmd.Context(), true)
			d, err := eng.Dispatch(cmd.Context(), nil)
			if err != nil {
				return err
			}
			fmt.Fprint(cmd.OutOrStdout(), daemon.Summary(polls, d))
			for _, p := range polls {
				if p.Err != nil {
					return errors.New("one or more accounts failed")
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
		Short: "Run continuously: read the accounts and send each space on its schedule",
		Long: `Run in the foreground. Read each account at its read_every time, and send
the waiting messages of each space to its agent at the times of the schedule
of the space. The daemon reads config.toml again when the file changes. Use
"i2a service install" to run it in the background at login.

The daemon serves an HTTP API on the Unix socket <data>/i2a.sock (mode 0600):
GET /v1/status, POST /v1/run and GET /v1/events. Example:
  curl --unix-socket <data>/i2a.sock http://i2a/v1/status`,
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
			defer eng.Close()
			srv := daemon.NewServer(app.Version, e.paths.ConfigFile, time.Now())
			socket, err := srv.Listen(e.paths.DataDir)
			if err != nil {
				return err
			}
			eng.SetAPI(srv)
			srvCtx, stopSrv := context.WithCancel(ctx)
			served := make(chan error, 1)
			go func() { served <- srv.Serve(srvCtx) }()
			defer func() { stopSrv(); <-served }() // the server removes the socket when it stops
			log.Info("API", "socket", socket)
			load := func() (*config.Config, error) {
				c, err := config.Load(e.paths.ConfigFile)
				if err != nil {
					return nil, err
				}
				if c.Migration != nil {
					log.Warn("configuration converted to version 2", "report", c.Migration.Text())
				}
				return c, render.CheckAgents(c)
			}
			return daemon.Run(ctx, eng, e.paths.ConfigFile, load)
		},
	}
}
