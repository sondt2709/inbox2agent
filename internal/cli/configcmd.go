package cli

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/sondt2709/inbox2agent/internal/tui"
)

func newConfigCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "config",
		Short: "Open the configuration screens (sources, channels, profiles, timing, service)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			e, err := newEnv()
			if err != nil {
				return err
			}
			cfg, err := e.loadOrDefault()
			if err != nil {
				return err
			}
			d := tui.Deps{
				Paths: e.paths, Config: cfg, Secrets: e.secrets,
				Check: CheckMattermost, Out: cmd.OutOrStdout(),
				InstallService: func() (string, error) {
					saved, err := e.loadConfig() // the service runs the saved file
					if err != nil {
						return "", err
					}
					return installService(e, saved)
				},
			}
			// Read the store only when it exists: the dashboard does not create it.
			if _, err := os.Stat(e.paths.DBFile); err == nil {
				st, err := e.openStore()
				if err != nil {
					return err
				}
				defer st.Close()
				d.Store = st
			}
			return tui.Run(cmd.Context(), d)
		},
	}
}
