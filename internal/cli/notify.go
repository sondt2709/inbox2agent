package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"

	"github.com/sondt2709/inbox2agent/extensions"
	"github.com/sondt2709/inbox2agent/internal/config"
	"github.com/sondt2709/inbox2agent/internal/notify"
)

// notifyDetect and brewInstall are replaced by the tests.
var (
	notifyDetect = notify.Detect
	brewPath     = func() string { p, _ := exec.LookPath("brew"); return p }
	brewInstall  = func(ctx context.Context, brew string, out, errOut io.Writer) error {
		c := exec.CommandContext(ctx, brew, "install", "terminal-notifier")
		c.Stdout, c.Stderr = out, errOut
		return c.Run()
	}
	notifyChain = func(o notify.Options) notify.Chain { return notify.NewChain(o) }
)

func newNotifyCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "notify",
		Short: "Set up and test the desktop notifications",
		Long: `i2a shows a notification through the first notifier that works:
the Orca plugin of i2a, then terminal-notifier, then osascript ([notify] via = "auto").
While the Feed is open in iTerm2, the Feed shows the card notifications ([notify] terminal).`,
	}
	c.AddCommand(newNotifySetupCmd(), newNotifyStatusCmd(), newNotifyTestCmd())
	return c
}

// notifyConfig loads the configuration, or the defaults when no file exists.
func notifyConfig(e *env) (*config.Config, error) {
	cfg, err := e.loadConfig()
	if err != nil {
		if _, statErr := os.Stat(e.paths.ConfigFile); errors.Is(statErr, os.ErrNotExist) {
			return config.Default(), nil
		}
		return nil, err
	}
	return cfg, nil
}

func printNotifiers(out io.Writer, cfg *config.Config, found []notify.Found) {
	for _, f := range found {
		mark := "✗"
		if f.OK {
			mark = "✓"
		}
		fmt.Fprintf(out, "%s %-18s %s\n", mark, f.Name, f.Detail)
	}
	fmt.Fprintf(out, "[notify] via = %q: i2a tries %s.\n", cfg.Notify.Via, notify.Describe(cfg.Notify.Via, found))
	if !cfg.Notify.Enabled {
		fmt.Fprintln(out, "[notify] enabled = false: i2a shows no notification.")
	}
}

func newNotifyStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show the notifiers that i2a finds; change nothing",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			e, err := newEnv()
			if err != nil {
				return err
			}
			cfg, err := notifyConfig(e)
			if err != nil {
				return err
			}
			printNotifiers(cmd.OutOrStdout(), cfg, notifyDetect())
			return nil
		},
	}
}

// sendTest sends one test note through the chain and prints the notifier.
func sendTest(ctx context.Context, out io.Writer, cfg *config.Config, dataDir string) error {
	if notify.Off() {
		return errors.New("notifications are off in this process (I2A_NOTIFY=off, or not macOS)")
	}
	name, err := notifyChain(notify.Options{Via: cfg.Notify.Via, DataDir: dataDir}).Send(ctx, notify.Note{
		Title: "i2a · test", Subtitle: "notify", Body: "This is a test notification of i2a.", Group: "i2a-test",
	})
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "The test notification showed through %s.\n", name)
	return nil
}

func newNotifyTestCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "test",
		Short: "Send one test notification and print the notifier that showed it",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			e, err := newEnv()
			if err != nil {
				return err
			}
			cfg, err := notifyConfig(e)
			if err != nil {
				return err
			}
			if err := e.paths.EnsureDataDir(); err != nil {
				return err
			}
			return sendTest(cmd.Context(), cmd.OutOrStdout(), cfg, e.paths.DataDir)
		},
	}
}

func newNotifySetupCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "setup",
		Short: "Find or install terminal-notifier, write the Orca plugin, and send a test",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			out := cmd.OutOrStdout()
			e, err := newEnv()
			if err != nil {
				return err
			}
			cfg, err := notifyConfig(e)
			if err != nil {
				return err
			}
			if err := e.paths.EnsureDataDir(); err != nil {
				return err
			}
			found := notifyDetect()
			printNotifiers(out, cfg, found)

			fmt.Fprintln(out, "\nterminal-notifier")
			if !foundOK(found, notify.ViaTerminalNotifier) {
				brew := brewPath()
				switch {
				case brew == "":
					fmt.Fprintln(out, "  Homebrew is not installed. Install terminal-notifier from https://github.com/julienXX/terminal-notifier.")
				case askYes(cmd.InOrStdin(), out, "  Install terminal-notifier with Homebrew? [y/N] "):
					if err := brewInstall(cmd.Context(), brew, out, cmd.ErrOrStderr()); err != nil {
						return fmt.Errorf("brew install terminal-notifier: %w", err)
					}
					found = notifyDetect()
				default:
					fmt.Fprintln(out, "  Not installed.")
				}
			} else {
				fmt.Fprintln(out, "  Installed. In System Settings › Notifications › terminal-notifier, select Persistent to keep the notes on the screen.")
			}

			dir, err := extensions.Write(extensions.Orca, e.paths.DataDir)
			if err != nil {
				return err
			}
			fmt.Fprintf(out, "\nOrca plugin (optional, experimental)\n  Wrote %s.\n", dir)
			fmt.Fprintln(out, "  In Orca: Settings › Plugins › turn on the plugin system › install from a local folder › select that folder › allow notifications:show.")

			fmt.Fprintln(out, "\nTest")
			fmt.Fprintf(out, "  i2a tries %s.\n  ", notify.Describe(cfg.Notify.Via, found))
			return sendTest(cmd.Context(), out, cfg, e.paths.DataDir)
		},
	}
}

func foundOK(found []notify.Found, name string) bool {
	for _, f := range found {
		if f.Name == name {
			return f.OK
		}
	}
	return false
}

// askYes prints q and reports whether the answer is y or yes.
func askYes(in io.Reader, out io.Writer, q string) bool {
	fmt.Fprint(out, q)
	line, _ := bufio.NewReader(in).ReadString('\n')
	a := strings.ToLower(strings.TrimSpace(line))
	return a == "y" || a == "yes"
}
