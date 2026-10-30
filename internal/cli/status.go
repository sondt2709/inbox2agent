package cli

import (
	"errors"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/sondt2709/inbox2agent/internal/config"
	"github.com/sondt2709/inbox2agent/internal/inbox"
	"github.com/sondt2709/inbox2agent/internal/service"
	"github.com/sondt2709/inbox2agent/internal/store"
)

func newStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show the paths, accounts, spaces, agents, schedules, messages and service state",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			e, err := newEnv()
			if err != nil {
				return err
			}
			w := cmd.OutOrStdout()
			fmt.Fprintf(w, "Configuration: %s\nData:          %s\nSecrets:       %s\n", e.paths.ConfigFile, e.paths.DataDir, e.secrets.Backend())
			st, err := e.openStore()
			if err != nil {
				return err
			}
			defer st.Close()
			pending, err := st.PendingCounts(contextBackground())
			if err != nil {
				return err
			}
			cfg, err := loadReported(e.paths.ConfigFile)
			switch {
			case errors.Is(err, config.ErrNotFound):
				fmt.Fprintln(w, "\nNo configuration file. Run `i2a config` to create one.")
			case err != nil:
				fmt.Fprintf(w, "\nThe configuration file has errors:\n%v\n", err)
			default:
				printConfig(w, cfg, e.paths.DataDir, pending, time.Now())
			}
			if err := printStore(w, st, cfg); err != nil {
				return err
			}
			printService(w)
			return nil
		},
	}
}

// printConfig prints the model: accounts, then each space with its agent,
// schedule, next send and waiting messages, then agents and schedules.
func printConfig(w io.Writer, c *config.Config, dataDir string, pending map[store.Conv]int, now time.Time) {
	fmt.Fprintf(w, "\nSettings: lookback %s, retention %s, notifications %s\n", c.Settings.Lookback, c.Settings.Retention, onOff(c.Notify.Enabled))
	fmt.Fprintln(w, "\nAccounts:")
	if len(c.Accounts) == 0 {
		fmt.Fprintln(w, "  none. Run `i2a config` to add one.")
	}
	tw := tabwriter.NewWriter(w, 2, 4, 2, ' ', 0)
	for _, a := range c.Accounts {
		fmt.Fprintf(tw, "  %s\t%s\t%s\t%s\n", a.ID, a.Title(), a.Platform, accountInfo(a, dataDir))
	}
	tw.Flush()

	waiting := map[string]int{}
	for conv, n := range pending {
		waiting[c.Route(conv.Account, conv.ID)] += n
	}
	fmt.Fprintln(w, "\nSpaces:")
	tw = tabwriter.NewWriter(w, 2, 4, 2, ' ', 0)
	for _, s := range c.AllSpaces() {
		send, agent := "hold: does not send", s.Agent
		if s.Agent != "" {
			send = "never"
			if sc, ok := c.Schedule(s.Schedule); ok {
				if at := sc.Next(now); !at.IsZero() {
					send = "next " + at.Format("15:04")
					if at.YearDay() != now.YearDay() || at.Year() != now.Year() {
						send = "next " + at.Format("Mon 15:04")
					}
				}
			}
		} else {
			agent = "-"
		}
		convs := fmt.Sprintf("%d conversations", len(s.Conversations))
		if s.ID == config.UnsortedID {
			convs = "new conversations: read " + c.Unsorted.Read
		}
		fmt.Fprintf(tw, "  %s\t%s\t%s\t%s\t%s\t%d waiting\t%s\n", s.ID, s.Title(), agent, or(s.Schedule, "-"), send, waiting[s.ID], convs)
	}
	tw.Flush()

	fmt.Fprintln(w, "\nAgents:")
	tw = tabwriter.NewWriter(w, 2, 4, 2, ' ', 0)
	for _, p := range c.Agents {
		line := fmt.Sprintf("  %s\t%s\tdriver=%s\tpayload=%s", p.ID, p.Title(), p.Driver, p.Payload)
		if p.Driver == config.DriverOrca {
			line += fmt.Sprintf("\tcli=%s\tsession=%s\tworkspace=%s", p.CLI, p.Session, p.Workspace)
		}
		fmt.Fprintln(tw, line)
	}
	tw.Flush()
	fmt.Fprintln(w, "\nSchedules:")
	tw = tabwriter.NewWriter(w, 2, 4, 2, ' ', 0)
	for _, s := range c.Schedules {
		fmt.Fprintf(tw, "  %s\t%s\t%s\n", s.ID, s.Title(), s.Text())
	}
	tw.Flush()
}

func onOff(b bool) string {
	if b {
		return "on"
	}
	return "off"
}

func printStore(w io.Writer, st *store.Store, cfg *config.Config) error {
	ctx := contextBackground()
	counts, err := st.Counts(ctx)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "\nMessages: %d waiting, %d previewed by dry-run, %d sent, %d expired\n",
		counts[inbox.StatePending], counts[inbox.StatePreviewed], counts[inbox.StateSent], counts[inbox.StateExpired])
	ds, err := st.Dispatches(ctx, 3)
	if err != nil {
		return err
	}
	if len(ds) == 0 {
		fmt.Fprintln(w, "Sends: none yet")
	} else {
		fmt.Fprintln(w, "Last sends:")
	}
	for _, d := range ds {
		space := d.Space
		if cfg != nil {
			if s, ok := cfg.Space(d.Space); ok {
				space = s.Title()
			}
		}
		fmt.Fprintf(w, "  %s  %s → %s  %s  %d messages  %s\n", d.CreatedAt.Local().Format("2006-01-02 15:04:05"),
			space, or(d.Agent, "-"), strings.ToUpper(d.Status), d.MessageCount, d.Detail)
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
