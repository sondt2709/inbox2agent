package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/sondt2709/inbox2agent/internal/config"
	"github.com/sondt2709/inbox2agent/internal/secrets"
	"github.com/sondt2709/inbox2agent/internal/source"
)

func contextBackground() context.Context { return context.Background() }

func newSourceCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "source", Short: "Add, list and remove sources"}
	cmd.AddCommand(newSourceAddCmd(), newSourceListCmd(), newSourceChannelsCmd(), newSourceRemoveCmd())
	return cmd
}

func newSourceAddCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "add", Short: "Add a source"}
	var id, url, profile string
	var channels []string
	var tokenStdin, allDirect, mentions bool
	mm := &cobra.Command{
		Use:   "mattermost --id <id> --url <url> --token-stdin",
		Short: "Add a Mattermost source with a personal access token",
		Long: `Add a Mattermost source. Pipe the personal access token to standard input.
i2a checks the token with the server before it saves anything.

Example:
  printf '%s' "$TOKEN" | i2a source add mattermost --id work --url https://chat.example.com --token-stdin`,
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
			if _, ok := cfg.Source(id); ok {
				return fmt.Errorf("a source with the ID %q exists. Remove it first with `i2a source remove %s`", id, id)
			}
			sc := config.NewMattermostSource(id, url)
			sc.Channels, sc.AllDirect, sc.Mentions, sc.Profile = channels, allDirect, mentions, profile
			next := *cfg
			next.Sources = append(append([]config.Source(nil), cfg.Sources...), sc)
			if err := next.Validate(); err != nil {
				return err
			}
			user, err := CheckMattermost(cmd.Context(), sc, token)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Connected as %s\n", user)
			if err := e.secrets.Set(secrets.TokenKey(id), token); err != nil {
				return fmt.Errorf("save the token in %s: %w", e.secrets.Backend(), err)
			}
			if err := config.Save(e.paths.ConfigFile, &next); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Added source %s to %s\n", id, e.paths.ConfigFile)
			return nil
		},
	}
	mm.Flags().StringVar(&id, "id", "", "source ID, for example work (required)")
	mm.Flags().StringVar(&url, "url", "", "server URL, for example https://chat.example.com (required)")
	mm.Flags().BoolVar(&tokenStdin, "token-stdin", false, "read the token from standard input")
	mm.Flags().StringSliceVar(&channels, "channels", nil, "IDs of channels and direct or group conversations to watch (see `i2a source channels`)")
	mm.Flags().BoolVar(&allDirect, "all-direct", true, "watch all direct and group messages")
	mm.Flags().BoolVar(&mentions, "mentions", true, "watch mentions of you in any channel")
	mm.Flags().StringVar(&profile, "profile", "", "profile for this source (default: the first profile)")
	_ = mm.MarkFlagRequired("id")
	_ = mm.MarkFlagRequired("url")
	cmd.AddCommand(mm, newSourceAddWhatsAppCmd())
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
func CheckMattermost(ctx context.Context, sc config.Source, token string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	src := source.NewMattermost(sc, token, time.Hour, &http.Client{Timeout: 15 * time.Second})
	user, err := src.Check(ctx)
	if err != nil {
		return "", fmt.Errorf("cannot connect to %s: %w", sc.URL, err)
	}
	return user, nil
}

func newSourceListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List the sources",
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
			fmt.Fprintln(tw, "ID\tTYPE\tURL OR STATE\tCHANNELS\tDIRECT\tMENTIONS\tPROFILE\tTOKEN")
			for _, s := range cfg.Sources {
				tok := "saved"
				if s.Type == config.SourceWhatsApp {
					tok = "-"
				} else if _, err := e.secrets.Get(secrets.TokenKey(s.ID)); err != nil {
					tok = "missing"
				}
				fmt.Fprintf(tw, "%s\t%s\t%s\t%d\t%s\t%s\t%s\t%s\n", s.ID, s.Type, sourcePlace(s, e.paths.DataDir), len(s.Channels),
					onOff(s.AllDirect), onOff(s.Mentions), or(s.Profile, cfg.Profiles[0].Name), tok)
			}
			return tw.Flush()
		},
	}
}

func newSourceChannelsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "channels <source-id>",
		Short: "List the channels of a source, with their IDs",
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
			sc, ok := cfg.Source(args[0])
			if !ok {
				return fmt.Errorf("no source with the ID %q", args[0])
			}
			if sc.Type == config.SourceWhatsApp {
				return printWhatsAppChats(cmd.Context(), cmd.OutOrStdout(), e.paths.DataDir, sc)
			}
			token, err := e.secrets.Get(secrets.TokenKey(sc.ID))
			if err != nil {
				return fmt.Errorf("read the token of %s: %w", sc.ID, err)
			}
			src := source.NewMattermost(*sc, token, cfg.Timing.Lookback.D(), nil)
			chs, err := src.ListChannels(cmd.Context())
			if err != nil {
				return err
			}
			tw := tabwriter.NewWriter(cmd.OutOrStdout(), 2, 4, 2, ' ', 0)
			fmt.Fprintln(tw, "ID\tTEAM\tKIND\tNAME\tSELECTED")
			for _, c := range chs {
				sel := ""
				if c.Selected {
					sel = "yes"
				}
				fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", c.ID, or(c.Team, "-"), c.Kind, c.Name, sel)
			}
			return tw.Flush()
		},
	}
}

func newSourceRemoveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "remove <source-id>",
		Short: "Remove a source and its token (WhatsApp: log out and delete its file)",
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
			removed, ok := cfg.Source(args[0])
			if !ok {
				return fmt.Errorf("no source with the ID %q", args[0])
			}
			kind := removed.Type
			var keep []config.Source
			for _, s := range cfg.Sources {
				if s.ID != args[0] {
					keep = append(keep, s)
				}
			}
			cfg.Sources = keep
			if err := config.Save(e.paths.ConfigFile, cfg); err != nil {
				return err
			}
			if kind == config.SourceWhatsApp {
				if warn := RemoveWhatsApp(cmd.Context(), e.paths.DataDir, args[0], nil); warn != "" {
					fmt.Fprintln(cmd.ErrOrStderr(), warn)
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Removed source %s and its WhatsApp file\n", args[0])
				return nil
			}
			if err := e.secrets.Delete(secrets.TokenKey(args[0])); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Removed source %s and its token\n", args[0])
			return nil
		},
	}
}
