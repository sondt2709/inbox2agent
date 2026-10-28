package cli

import (
	"errors"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/sondt2709/inbox2agent/internal/config"
	"github.com/sondt2709/inbox2agent/internal/inbox"
	"github.com/sondt2709/inbox2agent/internal/service"
)

func newStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show the paths, sources, profiles, messages and service state",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			e, err := newEnv()
			if err != nil {
				return err
			}
			w := cmd.OutOrStdout()
			fmt.Fprintf(w, "Configuration: %s\nData:          %s\nSecrets:       %s\n", e.paths.ConfigFile, e.paths.DataDir, e.secrets.Backend())
			cfg, err := config.Load(e.paths.ConfigFile)
			switch {
			case errors.Is(err, config.ErrNotFound):
				fmt.Fprintln(w, "\nNo configuration file. Run `i2a config` to create one.")
			case err != nil:
				fmt.Fprintf(w, "\nThe configuration file has errors:\n%v\n", err)
			default:
				printConfig(w, cfg)
			}
			if err := printStore(w, e); err != nil {
				return err
			}
			printService(w)
			return nil
		},
	}
}

func onOff(b bool) string {
	if b {
		return "on"
	}
	return "off"
}

func printConfig(w io.Writer, c *config.Config) {
	fmt.Fprintf(w, "\nTiming: interval %s, lookback %s, retention %s, active hours %s\n",
		c.Timing.Interval, c.Timing.Lookback, c.Timing.Retention, or(c.Timing.ActiveHours, "always"))
	fmt.Fprintln(w, "\nSources:")
	if len(c.Sources) == 0 {
		fmt.Fprintln(w, "  none. Run `i2a config` to add one.")
	}
	tw := tabwriter.NewWriter(w, 2, 4, 2, ' ', 0)
	for _, s := range c.Sources {
		fmt.Fprintf(tw, "  %s\t%s\t%s\tchannels=%d\tdirect=%s\tmentions=%s\tprofile=%s\n", s.ID, s.Type, s.URL,
			len(s.Channels), onOff(s.AllDirect), onOff(s.Mentions), or(s.Profile, c.Profiles[0].Name))
	}
	tw.Flush()
	fmt.Fprintln(w, "\nProfiles:")
	tw = tabwriter.NewWriter(w, 2, 4, 2, ' ', 0)
	for _, p := range c.Profiles {
		line := fmt.Sprintf("  %s\tdriver=%s\tpayload=%s", p.Name, p.Driver, p.Payload)
		if p.Driver == config.DriverOrca {
			line += fmt.Sprintf("\tagent=%s\tsession=%s\tworkspace=%s", p.Agent, p.Session, p.Workspace)
		}
		fmt.Fprintln(tw, line)
	}
	tw.Flush()
}

func printStore(w io.Writer, e *env) error {
	st, err := e.openStore()
	if err != nil {
		return err
	}
	defer st.Close()
	ctx := contextBackground()
	counts, err := st.Counts(ctx)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "\nMessages: %d pending, %d previewed by dry-run, %d sent, %d expired\n",
		counts[inbox.StatePending], counts[inbox.StatePreviewed], counts[inbox.StateSent], counts[inbox.StateExpired])
	ds, err := st.Dispatches(ctx, 3)
	if err != nil {
		return err
	}
	if len(ds) == 0 {
		fmt.Fprintln(w, "Dispatches: none yet")
	} else {
		fmt.Fprintln(w, "Last dispatches:")
	}
	for _, d := range ds {
		fmt.Fprintf(w, "  %s  %s  %s  %d messages  %s\n", d.CreatedAt.Local().Format("2006-01-02 15:04:05"),
			d.Profile, strings.ToUpper(d.Status), d.MessageCount, d.Detail)
	}
	return nil
}

func printService(w io.Writer) {
	st, err := service.Status()
	switch {
	case errors.Is(err, service.ErrUnsupported):
		fmt.Fprintln(w, "\nService: not supported on this platform")
	case err != nil:
		fmt.Fprintf(w, "\nService: %v\n", err)
	case !st.Loaded:
		fmt.Fprintln(w, "\nService: not installed. Run `i2a service install`.")
	default:
		fmt.Fprintf(w, "\nService: %s (pid %s)\n", or(st.State, "loaded"), or(st.PID, "-"))
	}
}

func or(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
