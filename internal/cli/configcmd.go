package cli

import (
	"errors"
	"os"
	"sync/atomic"

	"github.com/spf13/cobra"

	"github.com/sondt2709/inbox2agent/internal/store"
	"github.com/sondt2709/inbox2agent/internal/tui"
)

func newConfigCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "config",
		Short: "Open the dashboard on the Overview tab (sources, channels, profiles, timing, service)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runDashboard(cmd, tui.TabOverview)
		},
	}
}

// runDashboard opens the dashboard on a tab. On the Feed tab it creates the
// store, because the Feed is the main screen. On the other tabs it opens the
// store only when it exists, and later when the daemon creates it.
func runDashboard(cmd *cobra.Command, tab tui.TabID) error {
	e, err := newEnv()
	if err != nil {
		return err
	}
	cfg, err := e.loadOrDefault()
	if err != nil {
		return err
	}
	d := tui.Deps{
		Paths: e.paths, Config: cfg, Secrets: e.secrets, Tab: tab,
		Check: CheckMattermost, Out: cmd.OutOrStdout(),
		InstallService: func() (string, error) {
			saved, err := e.loadConfig() // the service runs the saved file
			if err != nil {
				return "", err
			}
			return installService(e, saved)
		},
	}
	// The dashboard can open the store later, outside its UI loop.
	var opened atomic.Pointer[store.Store]
	defer func() {
		if st := opened.Load(); st != nil {
			st.Close()
		}
	}()
	open := func() (tui.Store, error) {
		st, err := e.openStore()
		if err != nil {
			return nil, err
		}
		opened.Store(st)
		return st, nil
	}
	_, statErr := os.Stat(e.paths.DBFile)
	switch {
	case tab == tui.TabFeed || statErr == nil:
		if d.Store, err = open(); err != nil {
			return err
		}
	case errors.Is(statErr, os.ErrNotExist):
		d.OpenStore = func() (tui.Store, error) {
			if _, err := os.Stat(e.paths.DBFile); err != nil {
				return nil, nil //nolint:nilerr // no store yet is not an error
			}
			return open()
		}
	default:
		return statErr
	}
	return tui.Run(cmd.Context(), d)
}
