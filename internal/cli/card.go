package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/sondt2709/inbox2agent/internal/card"
	"github.com/sondt2709/inbox2agent/internal/inbox"
	"github.com/sondt2709/inbox2agent/internal/notify"
	"github.com/sondt2709/inbox2agent/internal/store"
)

// cardNotifier shows the card notifications. Tests replace it.
var cardNotifier = notify.Desktop

func newCardCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "card",
		Short: "Add, close and list the cards that the agent reports",
		Long: `A card is one result of the agent: a topic with a summary and a suggested reply.
The agent adds cards with "i2a card add". You read them in the Feed tab of "i2a".`,
	}
	c.AddCommand(newCardAddCmd(), newCardDoneCmd(), newCardListCmd())
	return c
}

func newCardAddCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "add < cards.json",
		Short: "Add cards from a JSON array on stdin",
		Long: `Read a JSON array of cards (or one card) from stdin and save them in one transaction.

Fields: category (action, question or info; required), urgency (high, normal or low;
default normal), title (required), group, body, reply, refs (message IDs) and key.
A card with a key that i2a already has updates that card.

Example:
  i2a card add <<'EOF'
  [{"category":"action","urgency":"high","group":"Harbor Portal","title":"Correct your reply to Lena",
    "body":"...","reply":"...","refs":[812],"key":"mm:lena-demo-login"}]
  EOF`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			in, err := card.Parse(cmd.InOrStdin())
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
			cards, warnings, err := card.Prepare(in, func(ids []int64) (map[int64]bool, error) {
				msgs, err := st.MessagesByID(ctx, ids)
				out := map[int64]bool{}
				for _, m := range msgs {
					out[m.ID] = true
				}
				return out, err
			})
			for _, w := range warnings {
				fmt.Fprintln(cmd.ErrOrStderr(), "warning:", w)
			}
			if err != nil {
				return err
			}
			saved, err := st.SaveCards(ctx, cards, time.Now())
			if err != nil {
				return err
			}
			n := map[string]int{}
			for _, s := range saved {
				n[s.Result]++
			}
			var parts []string
			for _, r := range []string{inbox.SaveAdded, inbox.SaveUpdated, inbox.SaveUnchanged} {
				if n[r] > 0 {
					parts = append(parts, fmt.Sprintf("%d %s", n[r], r))
				}
			}
			notes := 0
			if cfg.Notify.Enabled {
				show := cardNotifier()
				for _, note := range card.Plan(saved, cfg.Notify.MinUrgency) {
					show(note.Title, note.Text)
					notes++
				}
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%d cards saved: %s. Notifications: %d.\n", len(saved), strings.Join(parts, ", "), notes)
			return nil
		},
	}
}

func newCardDoneCmd() *cobra.Command {
	var key string
	var id int64
	c := &cobra.Command{
		Use:   "done --key <key> | --id <id>",
		Short: "Mark a card done",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if (key == "") == (id == 0) {
				return errors.New("give --key or --id")
			}
			e, err := newEnv()
			if err != nil {
				return err
			}
			st, err := e.openStore()
			if err != nil {
				return err
			}
			defer st.Close()
			err = st.SetCardState(cmd.Context(), id, key, inbox.CardDone, time.Now())
			switch {
			case errors.Is(err, store.ErrNoCard) && key != "":
				return fmt.Errorf("no card has the key %q", key)
			case errors.Is(err, store.ErrNoCard):
				return fmt.Errorf("no card has the ID %d", id)
			case err != nil:
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "The card is done.")
			return nil
		},
	}
	c.Flags().StringVar(&key, "key", "", "the key of the card")
	c.Flags().Int64Var(&id, "id", 0, "the ID of the card")
	return c
}

// cardJSON is one card in the output of "i2a card list --json".
type cardJSON struct {
	ID        int64   `json:"id"`
	Key       string  `json:"key"`
	Category  string  `json:"category"`
	Urgency   string  `json:"urgency"`
	Group     string  `json:"group"`
	Title     string  `json:"title"`
	Body      string  `json:"body"`
	Reply     string  `json:"reply"`
	Refs      []int64 `json:"refs"`
	State     string  `json:"state"`
	CreatedAt string  `json:"created_at"`
	UpdatedAt string  `json:"updated_at"`
}

func newCardListCmd() *cobra.Command {
	var asJSON, all bool
	c := &cobra.Command{
		Use:   "list",
		Short: "List the open cards (new and read), newest first",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			e, err := newEnv()
			if err != nil {
				return err
			}
			st, err := e.openStore()
			if err != nil {
				return err
			}
			defer st.Close()
			cards, err := st.Cards(cmd.Context(), all)
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if asJSON {
				list := make([]cardJSON, 0, len(cards))
				for _, c := range cards {
					refs := c.Refs
					if refs == nil {
						refs = []int64{}
					}
					list = append(list, cardJSON{ID: c.ID, Key: c.Key, Category: c.Category, Urgency: c.Urgency, Group: c.Group,
						Title: c.Title, Body: c.Body, Reply: c.Reply, Refs: refs, State: c.State,
						CreatedAt: c.CreatedAt.Format(time.RFC3339), UpdatedAt: c.UpdatedAt.Format(time.RFC3339)})
				}
				enc := json.NewEncoder(out)
				enc.SetEscapeHTML(false)
				enc.SetIndent("", "  ")
				return enc.Encode(list)
			}
			if len(cards) == 0 {
				fmt.Fprintln(out, "No open cards.")
				return nil
			}
			for _, c := range cards {
				title := c.Title
				if c.Group != "" {
					title = c.Group + ": " + title
				}
				fmt.Fprintf(out, "%-5d %-4s %-8s %-6s %s\n", c.ID, c.State, c.Category, c.Urgency, title)
			}
			return nil
		},
	}
	c.Flags().BoolVar(&asJSON, "json", false, "print a JSON array")
	c.Flags().BoolVar(&all, "all", false, "include the done cards")
	return c
}
