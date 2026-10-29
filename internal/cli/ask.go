package cli

import (
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/sondt2709/inbox2agent/internal/agent"
	"github.com/sondt2709/inbox2agent/internal/ask"
	"github.com/sondt2709/inbox2agent/internal/inbox"
	"github.com/sondt2709/inbox2agent/internal/render"
)

// askDriver builds the driver of a profile for an ask. Tests replace it.
var askDriver = agent.New

func newAskCmd() *cobra.Command {
	var cardRef string
	c := &cobra.Command{
		Use:   "ask --card <id|key> < text",
		Short: "Send an instruction about a card to the agent",
		Long: `Send your instruction about a card to the agent, for example "research more".
The text comes from stdin. The agent answers in the same card with i2a card add.

The ask goes to the Orca terminal that wrote the card when it still runs, else to
the agent of the profile, as a batch does. When the agent is busy, i2a waits up to
2 minutes; then the ask stays queued, and the daemon sends it on the next dispatch.
A dry-run profile does not take asks.

Example:
  i2a ask --card mm:lena-pf-login <<'EOF'
  Check the auth code and give me the effort in hours.
  EOF`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if cardRef == "" {
				return errors.New("give --card <id|key>")
			}
			text, err := readStdin(cmd.InOrStdin())
			if err != nil {
				return err
			}
			e, err := newEnv()
			if err != nil {
				return err
			}
			cfg, err := e.loadConfig()
			if err != nil {
				return err
			}
			st, err := e.openStore()
			if err != nil {
				return err
			}
			defer st.Close()
			ctx := cmd.Context()
			c, err := cardByRef(ctx, st, cardRef)
			if err != nil {
				return err
			}
			exe, err := os.Executable()
			if err != nil {
				exe = "i2a"
			}
			a, err := ask.Send(ctx, ask.Deps{Store: st, Config: cfg, NewDriver: askDriver,
				CardCommand: render.CardCommand(exe, os.Getenv("I2A_HOME")), LockDir: e.paths.DataDir, Now: time.Now}, c, text, ask.Wait)
			if err != nil {
				return err
			}
			if a.State == inbox.AskSent {
				fmt.Fprintf(cmd.OutOrStdout(), "sent to the agent of %s (%s). Ask %d.\n", a.Profile, a.Handle, a.ID)
				return nil
			}
			fmt.Fprintf(cmd.OutOrStdout(), "the agent of %s is busy; the ask is queued for the next dispatch. Ask %d.\n", a.Profile, a.ID)
			return nil
		},
	}
	c.Flags().StringVar(&cardRef, "card", "", "the ID or the key of a card")
	return c
}
