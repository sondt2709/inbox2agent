package cli

import (
	"context"
	"os"

	"charm.land/fang/v2"
	"github.com/spf13/cobra"

	"github.com/sondt2709/inbox2agent/internal/app"
	"github.com/sondt2709/inbox2agent/internal/tui"
)

func newRoot() *cobra.Command {
	root := &cobra.Command{
		Use:   "i2a",
		Short: "Send new messages from your inbox tools to an AI agent, and read its cards",
		Long: `i2a (inbox2agent) collects new messages from your chat and work tools and
sends them to an AI agent in batches. The agent sends its results back as cards.

"i2a" with no arguments opens the Feed of cards.
Start with "i2a config" to connect a source and set up a profile.
Then run "i2a run --once" to test, and "i2a service install" to run in the background.`,
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runDashboard(cmd, tui.TabFeed)
		},
	}
	root.AddCommand(newConfigCmd(), newRunCmd(), newDaemonCmd(), newStatusCmd(), newSourceCmd(), newServiceCmd(), newCardCmd(), newReplyCmd(), newAskCmd())
	return root
}

// Execute runs the root command and returns the exit code.
func Execute(ctx context.Context) int {
	if err := fang.Execute(ctx, newRoot(), fang.WithVersion(app.Version), fang.WithoutManpage()); err != nil {
		return 1
	}
	return 0
}

// Main is the entry point for cmd/i2a.
func Main() { os.Exit(Execute(context.Background())) }
