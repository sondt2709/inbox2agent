package cli

import (
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/sondt2709/inbox2agent/internal/agent"
	"github.com/sondt2709/inbox2agent/internal/config"
	"github.com/sondt2709/inbox2agent/internal/service"
)

func newServiceCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "service",
		Short: "Run i2a in the background at login (macOS LaunchAgent)",
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "install",
		Short: "Install and start the LaunchAgent",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			e, err := newEnv()
			if err != nil {
				return err
			}
			cfg, err := e.loadConfig()
			if err != nil {
				return err
			}
			path, err := installService(e, cfg)
			if err != nil {
				return unsupported(err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Installed %s\nLog: %s\n", path, e.paths.LogFile)
			st, _ := service.Status()
			fmt.Fprintf(cmd.OutOrStdout(), "State: %s (pid %s)\n", or(st.State, "loaded"), or(st.PID, "-"))
			return nil
		},
	}, &cobra.Command{
		Use:   "uninstall",
		Short: "Stop and remove the LaunchAgent",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			path, err := service.Uninstall()
			if err != nil {
				return unsupported(err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Removed %s\n", path)
			return nil
		},
	}, &cobra.Command{
		Use:   "status",
		Short: "Show the LaunchAgent state",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			st, err := service.Status()
			if err != nil {
				return unsupported(err)
			}
			if !st.Loaded {
				fmt.Fprintln(cmd.OutOrStdout(), "not installed")
				return nil
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s (pid %s)\n", or(st.State, "loaded"), or(st.PID, "-"))
			return nil
		},
	})
	return cmd
}

func unsupported(err error) error {
	if errors.Is(err, service.ErrUnsupported) {
		return errors.New("i2a service: not supported on this platform")
	}
	return err
}

// installService writes and starts the LaunchAgent for the saved configuration,
// and waits up to 5 seconds until it runs.
func installService(e *env, cfg *config.Config) (string, error) {
	var extra []string
	for _, p := range cfg.Profiles {
		if p.Driver != config.DriverOrca {
			continue
		}
		bin, err := agent.ResolveBin(p.OrcaBin)
		if err != nil {
			return "", fmt.Errorf("profile %q: %w", p.Name, err)
		}
		extra = append(extra, filepath.Dir(bin))
	}
	spec, err := service.NewSpec(e.paths.LogFile, extra...)
	if err != nil {
		return "", err
	}
	path, err := service.Install(spec)
	if err != nil {
		return "", err
	}
	// launchd needs a moment to start the process ("xpcproxy" while it starts).
	for i := 0; i < 25; i++ {
		if st, _ := service.Status(); st.State == "running" {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	return path, nil
}
