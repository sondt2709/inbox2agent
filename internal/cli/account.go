package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/sondt2709/inbox2agent/internal/config"
	"github.com/sondt2709/inbox2agent/internal/secrets"
	"github.com/sondt2709/inbox2agent/internal/source"
	"github.com/sondt2709/inbox2agent/internal/source/whatsapp"
)

func contextBackground() context.Context { return context.Background() }

// errRenamed is the exit of the old command "i2a source".
var errRenamed = exitError{code: 2, msg: "i2a source is renamed: use i2a account (and i2a conversations)"}

// newSourceCmd only tells the user the new command names.
func newSourceCmd() *cobra.Command {
	return &cobra.Command{
		Use: "source", Hidden: true, DisableFlagParsing: true,
		RunE: func(*cobra.Command, []string) error { return errRenamed },
	}
}

func newAccountCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "account", Short: "Add, list and remove accounts (logins to chat tools)"}
	cmd.AddCommand(newAccountAddCmd(), newAccountListCmd(), newAccountRemoveCmd())
	return cmd
}

func newAccountAddCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "add", Short: "Add an account; its conversations start in Unsorted"}
	var id, name, url string
	var tokenStdin bool
	mm := &cobra.Command{
		Use:   "mattermost --id <id> --url <url> --token-stdin",
		Short: "Add a Mattermost account with a personal access token",
		Long: `Add a Mattermost account. Pipe the personal access token to standard input.
i2a checks the token with the server before it saves anything. The
conversations of the account start in Unsorted; sort them with
"i2a conversation set" or in "i2a config".

Example:
  printf '%s' "$TOKEN" | i2a account add mattermost --id work --url https://chat.example.com --token-stdin`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if !tokenStdin {
				return errors.New("pipe the token to standard input and use --token-stdin")
			}
			token, err := readToken(cmd.InOrStdin())
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
			if _, ok := cfg.Account(id); ok {
				return fmt.Errorf("an account with the ID %q exists. Remove it first with `i2a account remove %s`", id, id)
			}
			a := config.NewMattermostAccount(id, url)
			a.Name = or(name, id)
			next := *cfg
			next.Accounts = append(append([]config.Account(nil), cfg.Accounts...), a)
			if err := next.Validate(); err != nil {
				return err
			}
			user, err := CheckMattermost(cmd.Context(), a, token)
			if err != nil {
				return err
			}
			next.Accounts[len(next.Accounts)-1].User = user
			fmt.Fprintf(cmd.OutOrStdout(), "Connected as %s\n", user)
			if err := e.secrets.Set(secrets.TokenKey(id), token); err != nil {
				return fmt.Errorf("save the token in %s: %w", e.secrets.Backend(), err)
			}
			if err := config.Save(e.paths.ConfigFile, &next); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Added account %s to %s. Its conversations are in Unsorted.\n", id, e.paths.ConfigFile)
			return nil
		},
	}
	mm.Flags().StringVar(&id, "id", "", "account ID, for example work (required)")
	mm.Flags().StringVar(&name, "name", "", "the name that i2a shows, for example \"Company A\" (default: the ID)")
	mm.Flags().StringVar(&url, "url", "", "server URL, for example https://chat.example.com (required)")
	mm.Flags().BoolVar(&tokenStdin, "token-stdin", false, "read the token from standard input")
	_ = mm.MarkFlagRequired("id")
	_ = mm.MarkFlagRequired("url")
	cmd.AddCommand(mm, newAccountAddWhatsAppCmd())
	return cmd
}

func readToken(r io.Reader) (string, error) {
	line, err := bufio.NewReader(io.LimitReader(r, 4096)).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	token := strings.TrimSpace(line)
	if token == "" {
		return "", errors.New("standard input has no token")
	}
	return token, nil
}

// CheckMattermost calls the server with the token and returns the username.
func CheckMattermost(ctx context.Context, a config.Account, token string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	src := source.NewMattermost(a, token, time.Hour, &http.Client{Timeout: 15 * time.Second})
	user, err := src.Check(ctx)
	if err != nil {
		return "", fmt.Errorf("cannot connect to %s: %w", a.URL, err)
	}
	return user, nil
}

// accountInfo identifies an account: the server and the username of
// Mattermost, the phone number and the state of WhatsApp.
func accountInfo(a config.Account, dataDir string) string {
	if a.Platform != config.PlatformWhatsApp {
		if a.User != "" {
			return a.URL + " as @" + a.User
		}
		return a.URL
	}
	state := whatsAppState(dataDir, a.ID)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if d, err := whatsapp.ReadDevice(ctx, dataDir, a.ID); err == nil && d.Phone != "" {
		return d.Phone + " " + state
	}
	return state
}

func newAccountListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List the accounts",
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
			tw := tabwriter.NewWriter(cmd.OutOrStdout(), 2, 4, 2, ' ', 0)
			fmt.Fprintln(tw, "ID\tNAME\tPLATFORM\tACCOUNT\tCONVERSATIONS\tTOKEN")
			for _, a := range cfg.Accounts {
				tok := "saved"
				if a.Platform == config.PlatformWhatsApp {
					tok = "-"
				} else if _, err := e.secrets.Get(secrets.TokenKey(a.ID)); err != nil {
					tok = "missing"
				}
				fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%d in spaces\t%s\n", a.ID, a.Title(), a.Platform, accountInfo(a, e.paths.DataDir),
					len(cfg.Levels(a.ID)), tok)
			}
			return tw.Flush()
		},
	}
}

func newAccountRemoveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "remove <account-id>",
		Short: "Remove an account, its token and its conversations (WhatsApp: log out and delete its file)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			e, err := newEnv()
			if err != nil {
				return err
			}
			cfg, err := e.loadConfig()
			if err != nil {
				return err
			}
			removed, ok := cfg.Account(args[0])
			if !ok {
				return fmt.Errorf("no account with the ID %q", args[0])
			}
			platform := removed.Platform
			id := args[0]
			cfg.Accounts = slices.DeleteFunc(cfg.Accounts, func(a config.Account) bool { return a.ID == id })
			drop := func(list []config.Conversation) []config.Conversation {
				return slices.DeleteFunc(list, func(c config.Conversation) bool { return c.Account == id })
			}
			cfg.Unsorted.Conversations = drop(cfg.Unsorted.Conversations)
			for i := range cfg.Spaces {
				cfg.Spaces[i].Conversations = drop(cfg.Spaces[i].Conversations)
			}
			if err := config.Save(e.paths.ConfigFile, cfg); err != nil {
				return err
			}
			if platform == config.PlatformWhatsApp {
				if warn := RemoveWhatsApp(cmd.Context(), e.paths.DataDir, id, nil); warn != "" {
					fmt.Fprintln(cmd.ErrOrStderr(), warn)
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Removed account %s and its WhatsApp file\n", id)
				return nil
			}
			if err := e.secrets.Delete(secrets.TokenKey(id)); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Removed account %s and its token\n", id)
			return nil
		},
	}
}

// listed is one conversation that an account lists.
type listed struct{ id, kind, name string }

// listConversations lists the conversations of an account: Mattermost over
// the network, WhatsApp from its file.
func (e *env) listConversations(ctx context.Context, cfg *config.Config, a config.Account) ([]listed, error) {
	var out []listed
	if a.Platform == config.PlatformWhatsApp {
		chats, err := whatsapp.ReadChats(ctx, e.paths.DataDir, a.ID)
		if err != nil {
			return nil, err
		}
		for _, c := range chats {
			out = append(out, listed{c.JID, c.Kind, c.Name})
		}
		return out, nil
	}
	token, err := e.secrets.Get(secrets.TokenKey(a.ID))
	if err != nil {
		return nil, fmt.Errorf("read the token of %s: %w", a.ID, err)
	}
	chs, err := source.NewMattermost(a, token, cfg.Settings.Lookback.D(), nil).ListChannels(ctx)
	if err != nil {
		return nil, err
	}
	for _, c := range chs {
		name := c.Name
		if c.Team != "" {
			name = c.Team + " / " + c.Name
		}
		out = append(out, listed{c.ID, c.Kind, name})
	}
	return out, nil
}

func newConversationsCmd() *cobra.Command {
	var account string
	var unsorted bool
	cmd := &cobra.Command{
		Use:   "conversations [--account <id>] [--unsorted]",
		Short: "List the conversations of the accounts with their space and read level",
		Long: `List each conversation: the conversations that the accounts list, and the
conversations in the configuration. A direct conversation, a group and a
channel have the same columns. Use the ID with "i2a conversation set".`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			e, err := newEnv()
			if err != nil {
				return err
			}
			cfg, err := e.loadConfig()
			if err != nil {
				return err
			}
			if account != "" {
				if _, ok := cfg.Account(account); !ok {
					return fmt.Errorf("no account with the ID %q", account)
				}
			}
			tw := tabwriter.NewWriter(cmd.OutOrStdout(), 2, 4, 2, ' ', 0)
			fmt.Fprintln(tw, "ACCOUNT\tID\tKIND\tNAME\tSPACE\tREAD")
			for _, a := range cfg.Accounts {
				if account != "" && a.ID != account {
					continue
				}
				rows, err := e.listConversations(cmd.Context(), cfg, a)
				if err != nil {
					fmt.Fprintf(cmd.ErrOrStderr(), "account %s: %v\n", a.ID, err)
				}
				seen := map[string]bool{}
				for _, s := range cfg.AllSpaces() {
					for _, cv := range s.Conversations {
						if cv.Account == a.ID && !slices.ContainsFunc(rows, func(r listed) bool { return r.id == cv.ID }) {
							rows = append(rows, listed{cv.ID, "-", cv.Name})
						}
					}
				}
				for _, r := range rows {
					if seen[r.id] {
						continue
					}
					seen[r.id] = true
					cv, space, _ := cfg.Conversation(a.ID, r.id)
					if unsorted && space != config.UnsortedID {
						continue
					}
					s, _ := cfg.Space(space)
					fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n", a.ID, r.id, or(r.kind, "-"), or(cv.Label, or(r.name, "-")), s.Title(), cv.Read)
				}
			}
			return tw.Flush()
		},
	}
	cmd.Flags().StringVar(&account, "account", "", "only this account")
	cmd.Flags().BoolVar(&unsorted, "unsorted", false, "only the conversations in Unsorted")
	return cmd
}

func newConversationCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "conversation", Short: "Change one conversation"}
	var space, read, label string
	set := &cobra.Command{
		Use:   "set <account> <conversation-id> [--space <id>] [--read all|mentions|off] [--label <text>]",
		Short: "Move a conversation to a space, set its read level, or rename it",
		Long: `Move a conversation to a space (or to "unsorted"), set its read level, or
give it a label. Find the IDs with "i2a conversations".

Example:
  i2a conversation set work 4xp9fdt1ajr3 --space acme --read mentions`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if space == "" && read == "" && !cmd.Flags().Changed("label") {
				return errors.New("give --space, --read or --label")
			}
			e, err := newEnv()
			if err != nil {
				return err
			}
			cfg, err := e.loadConfig()
			if err != nil {
				return err
			}
			acc, id := args[0], args[1]
			cur, curSpace, _ := cfg.Conversation(acc, id)
			if read != "" && !slices.Contains(config.ReadLevels, read) {
				return fmt.Errorf("--read %q is not all, mentions or off", read)
			}
			if err := cfg.SetConversation(acc, id, or(space, curSpace), or(read, map[bool]string{true: cur.Read}[space == ""]), ""); err != nil {
				return err
			}
			if cmd.Flags().Changed("label") {
				if err := cfg.SetLabel(acc, id, label); err != nil {
					return err
				}
			}
			if err := config.Save(e.paths.ConfigFile, cfg); err != nil {
				return err
			}
			cv, sp, _ := cfg.Conversation(acc, id)
			s, _ := cfg.Space(sp)
			fmt.Fprintf(cmd.OutOrStdout(), "%s/%s: space %s, read %s\n", acc, id, s.Title(), cv.Read)
			return nil
		},
	}
	set.Flags().StringVar(&space, "space", "", "the ID of the space, or unsorted")
	set.Flags().StringVar(&read, "read", "", "all, mentions or off")
	set.Flags().StringVar(&label, "label", "", "the name that i2a shows; empty shows the platform name")
	cmd.AddCommand(set)
	return cmd
}
