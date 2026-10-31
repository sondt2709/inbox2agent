package cli

import (
	"errors"
	"fmt"
	"os"
	"sync/atomic"

	"github.com/spf13/cobra"

	"github.com/sondt2709/inbox2agent/internal/config"
	"github.com/sondt2709/inbox2agent/internal/render"
	"github.com/sondt2709/inbox2agent/internal/store"
	"github.com/sondt2709/inbox2agent/internal/tui"
)

func newConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Open the dashboard on the Spaces tab; check config.toml or print its schema",
		Long: `"i2a config" opens the dashboard on the Spaces tab. You can also edit the
configuration file. It is the interface for scripts and AI agents.

The file: config.toml in the configuration directory (see "i2a status"). Next
to it, config.schema.json describes each key; the first line of config.toml
("#:schema ./config.schema.json") gives the schema to editors with Taplo or
Tombi. "i2a config schema" prints the schema.

The items:
  [[accounts]]               one login. Add it with "i2a account add": the
                             token goes to the secret store, not to the file.
  [[agents]]                 how to send a batch (driver orca or dry-run).
  [[schedules]]              when a space sends (every, days, hours).
  [unsorted]                 the space of each conversation that no space
                             lists; agent = '' holds its messages.
  [[spaces]]                 id, name, agent, schedule, and its
  [[spaces.conversations]]   conversations: account, id, read (all,
                             mentions or off) and label. A conversation is in
                             one space only.
  [settings], [notify]       limits and notifications.

To move a conversation, remove its entry from one space and add it to the
other. "i2a conversations --json" gives the account and the ID of each
conversation.

Write the file safely: write a temporary file in the same directory, run
"i2a config check <temporary file>", then rename it to config.toml. A running
dashboard and daemon load the new file within seconds. The dashboard keeps
your change of an item when it saves a different item. A save of i2a
removes the comments in the file.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runDashboard(cmd, tui.TabSpaces)
		},
	}
	cmd.AddCommand(newConfigCheckCmd(), newConfigSchemaCmd())
	return cmd
}

func newConfigCheckCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "check [file]",
		Short: "Check a configuration file; exit 1 with each problem",
		Long: `Check a configuration file with the same checks as the daemon: the TOML
format, unknown keys, the values, the references between items (a space names
an agent and a schedule that exist), the IDs that are used two times and the
templates. With no file, check config.toml. The command writes no file.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := ""
			if len(args) == 1 {
				path = args[0]
			} else {
				e, err := newEnv()
				if err != nil {
					return err
				}
				path = e.paths.ConfigFile
			}
			b, err := os.ReadFile(path)
			if err != nil {
				return exitError{code: 1, msg: "cannot read " + path + ": " + err.Error()}
			}
			c, err := config.Parse(b)
			if err == nil {
				err = render.CheckAgents(c)
			}
			if err != nil {
				return exitError{code: 1, msg: path + " is not valid: " + err.Error()}
			}
			n := 0
			for _, s := range c.AllSpaces() {
				n += len(s.Conversations)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s is valid: %s, %s, %s, %s and Unsorted, %s\n", path,
				count(len(c.Accounts), "account"), count(len(c.Agents), "agent"), count(len(c.Schedules), "schedule"),
				count(len(c.Spaces), "space"), count(n, "conversation"))
			return nil
		},
	}
}

func newConfigSchemaCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "schema",
		Short: "Print the JSON Schema of config.toml",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, err := cmd.OutOrStdout().Write(config.Schema())
			return err
		},
	}
}

// count writes "1 space" or "2 spaces".
func count(n int, unit string) string {
	if n == 1 {
		return "1 " + unit
	}
	return fmt.Sprintf("%d %ss", n, unit)
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
