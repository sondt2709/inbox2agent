package cli

import (
	"context"
	"os"

	"charm.land/fang/v2"
	"github.com/spf13/cobra"

	"github.com/sondt2709/inbox2agent/internal/app"
)

func newRoot() *cobra.Command {
	root := &cobra.Command{
		Use:   "i2a",
		Short: "Send new messages from your inbox tools to an AI agent",
		Long: `i2a (inbox2agent) collects new messages from your chat and work tools and
sends them to an AI agent in batches.

Start with "i2a config" to connect a source and set up a profile.
Then run "i2a run --once" to test, and "i2a service install" to run in the background.`,
		SilenceUsage: true,
	}
	root.AddCommand(newConfigCmd(), newRunCmd(), newDaemonCmd(), newStatusCmd(), newSourceCmd(), newServiceCmd())
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
