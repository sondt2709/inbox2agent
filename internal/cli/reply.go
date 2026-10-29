package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"time"

	"github.com/spf13/cobra"

	"github.com/sondt2709/inbox2agent/internal/config"
	"github.com/sondt2709/inbox2agent/internal/inbox"
	"github.com/sondt2709/inbox2agent/internal/notify"
	"github.com/sondt2709/inbox2agent/internal/reply"
	"github.com/sondt2709/inbox2agent/internal/secrets"
	"github.com/sondt2709/inbox2agent/internal/source"
	"github.com/sondt2709/inbox2agent/internal/store"
)

// replyNotifier shows the notification of a reply. Tests replace it.
var replyNotifier = notify.Desktop

// replyBuilder builds the replier of a source. Tests replace it.
var replyBuilder = func(cfg *config.Config, sec secrets.Store) reply.Builder {
	return func(sourceID string) (source.Replier, error) {
		return source.BuildReplier(cfg, sourceID, sec, nil)
	}
}

func newReplyCmd() *cobra.Command {
	var to int64
	var cardRef string
	var thread, channel bool
	c := &cobra.Command{
		Use:   "reply --to <message id> | --card <id|key> [--thread | --channel] < text",
		Short: "Send a reply to a stored message, as the user of the source",
		Long: `Send a reply to a message that i2a collected. The text comes from stdin.
With --card and no text on stdin, the reply is the suggested reply of the card.
With --card and no --to, the reply goes to the newest message of the card, and
the card is done after the send.

The reply goes to the thread of the message, or to the channel for a top-level
message of a direct or group conversation. --thread and --channel change this.
i2a does not change the read state. Mattermost keeps it for a reply in a thread,
and marks the channel read for a post in the channel.

i2a does not send the same text to the same message two times. When a send
times out, run the command again: i2a looks for the post first.

Example:
  i2a reply --to 812 <<'EOF'
  Thanks Minh, noted. I will check the fix today.
  EOF`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if to == 0 && cardRef == "" {
				return errors.New("give --to <message id> or --card <id|key>")
			}
			if thread && channel {
				return errors.New("give --thread or --channel, not both")
			}
			text, err := readStdin(cmd.InOrStdin())
			if err != nil {
				return err
			}
			e, err := newEnv()
			if err != nil {
				return err
			}
			cfg, err := e.loadOrDefault()
			if err != nil {
				return err
			}
			st, err := e.openStore()
			if err != nil {
				return err
			}
			defer st.Close()
			ctx := cmd.Context()

			var c inbox.Card
			if cardRef != "" {
				if c, err = cardByRef(ctx, st, cardRef); err != nil {
					return err
				}
				if text == "" {
					text = c.Reply
				}
			}
			if to == 0 {
				msgs, err := st.MessagesByID(ctx, c.Refs)
				if err != nil {
					return err
				}
				m, ok := reply.DefaultTarget(msgs)
				if !ok {
					return fmt.Errorf("card %q has no message to reply to", cardRef)
				}
				to = m.ID
			}
			msgs, err := st.MessagesByID(ctx, []int64{to})
			if err != nil {
				return err
			}
			if len(msgs) == 0 {
				return fmt.Errorf("the store has no message %d", to)
			}
			place := reply.DefaultPlace(msgs[0])
			switch {
			case thread:
				place = inbox.PlaceThread
			case channel:
				place = inbox.PlaceChannel
			}

			r, target, err := reply.Send(ctx, st, replyBuilder(cfg, e.secrets), reply.Request{
				MessageID: to, CardID: c.ID, Text: text, Place: place, By: inbox.ByAgent,
			}, time.Now)
			if err != nil {
				return err
			}
			if cfg.Notify.Enabled {
				replyNotifier()("i2a · the agent sent a reply", notify.Short(target.ChannelName+": "+r.Text, 200))
			}
			where := inbox.PlaceChannel
			if place == inbox.PlaceThread {
				where = "thread of " + target.Author
			}
			done := ""
			if c.ID != 0 {
				done = " The card is done."
			}
			fmt.Fprintf(cmd.OutOrStdout(), "sent to %s (%s). Reply %d.%s\n", target.ChannelName, where, r.ID, done)
			return nil
		},
	}
	c.Flags().Int64Var(&to, "to", 0, "the ID of the message to reply to")
	c.Flags().StringVar(&cardRef, "card", "", "the ID or the key of a card")
	c.Flags().BoolVar(&thread, "thread", false, "reply in the thread of the message")
	c.Flags().BoolVar(&channel, "channel", false, "post in the channel, not in a thread")
	return c
}

// cardByRef returns the card with the ID or the key ref.
func cardByRef(ctx context.Context, st *store.Store, ref string) (inbox.Card, error) {
	id, _ := strconv.ParseInt(ref, 10, 64)
	key := ""
	if id == 0 {
		key = ref
	}
	c, err := st.Card(ctx, id, key)
	if errors.Is(err, store.ErrNoCard) {
		return c, fmt.Errorf("no card has the ID or key %q", ref)
	}
	return c, err
}

// readStdin reads the text on stdin, or returns "" when stdin is a terminal.
func readStdin(in io.Reader) (string, error) {
	if f, ok := in.(*os.File); ok {
		if info, err := f.Stat(); err == nil && info.Mode()&os.ModeCharDevice != 0 {
			return "", nil
		}
	}
	b, err := io.ReadAll(io.LimitReader(in, 1<<20))
	return string(b), err
}
