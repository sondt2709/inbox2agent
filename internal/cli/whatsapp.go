package cli

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/sondt2709/inbox2agent/internal/config"
	"github.com/sondt2709/inbox2agent/internal/source/whatsapp"
)

// removeWait is the wait for the daemon to close the connection of a removed source.
const removeWait = 5 * time.Second

func newSourceAddWhatsAppCmd() *cobra.Command {
	var id, phone, profile string
	var channels []string
	var allDirect, mentions bool
	cmd := &cobra.Command{
		Use:   "whatsapp --id <id> [--phone <number>]",
		Short: "Pair a WhatsApp account as a linked device and add it",
		Long: `Pair a WhatsApp account as a linked device of the phone, then add the source.
The command prints a QR code to scan. With --phone, it prints a pairing code
to type on the phone instead.

WhatsApp does not allow unofficial clients and can ban an account that uses one.
i2a only reads messages, and it never sends read receipts or online presence.

Example:
  i2a source add whatsapp --id phone`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
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
			sc := config.NewWhatsAppSource(id)
			sc.Channels, sc.AllDirect, sc.Mentions, sc.Profile = channels, allDirect, mentions, profile
			next := *cfg
			next.Sources = append(append([]config.Source(nil), cfg.Sources...), sc)
			if err := next.Validate(); err != nil {
				return err
			}
			if err := e.paths.EnsureDataDir(); err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if err := PairWhatsApp(cmd.Context(), e.paths.DataDir, id, phone, cfg.Timing.Lookback.D(), out); err != nil {
				return err
			}
			if err := config.Save(e.paths.ConfigFile, &next); err != nil {
				return err
			}
			fmt.Fprintf(out, "Added source %s to %s\n", id, e.paths.ConfigFile)
			return nil
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "source ID, for example phone (required)")
	cmd.Flags().StringVar(&phone, "phone", "", "phone number in international format; prints a pairing code instead of a QR code")
	cmd.Flags().StringSliceVar(&channels, "channels", nil, "JIDs of direct chats and groups to follow (see `i2a source channels`)")
	cmd.Flags().BoolVar(&allDirect, "all-direct", true, "follow all direct chats")
	cmd.Flags().BoolVar(&mentions, "mentions", true, "follow group messages that mention you or reply to you")
	cmd.Flags().StringVar(&profile, "profile", "", "profile for this source (default: the first profile)")
	_ = cmd.MarkFlagRequired("id")
	return cmd
}

// PairWhatsApp pairs the source and prints the codes. A source that is paired
// already is kept. When the pairing fails or the user presses ctrl+c, the
// file of the source is deleted.
func PairWhatsApp(ctx context.Context, dataDir, id, phone string, lookback time.Duration, out io.Writer) (err error) {
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()
	conn, err := whatsapp.Open(ctx, dataDir, id, lookback, nil)
	if err != nil {
		return err
	}
	defer func() {
		paired := conn.Paired()
		conn.Close()
		if err != nil && !paired {
			_ = whatsapp.Remove(context.Background(), dataDir, id, 0, nil)
		}
	}()
	if conn.Paired() {
		fmt.Fprintf(out, "The source %s is paired already.\n", id)
		return nil
	}
	fmt.Fprintln(out, whatsapp.PairSteps)
	err = conn.Pair(ctx, phone, func(ev whatsapp.PairEvent) {
		if ev.Code != "" {
			fmt.Fprintf(out, "Tap \"Link with phone number instead\" and type this code: %s\n", ev.Code)
			return
		}
		qr, err := whatsapp.QRText(ev.QR)
		if err != nil {
			fmt.Fprintf(out, "Cannot draw the QR code: %v\n", err)
			return
		}
		fmt.Fprintf(out, "\nScan this QR code (a new code comes each 20 seconds):\n%s\n", qr)
	})
	if err != nil {
		return err
	}
	fmt.Fprintln(out, "Paired")
	return nil
}

// sourcePlace is the URL of a Mattermost source, or the state of a WhatsApp source.
func sourcePlace(s config.Source, dataDir string) string {
	if s.Type != config.SourceWhatsApp {
		return s.URL
	}
	return whatsAppState(dataDir, s.ID)
}

// whatsAppState returns "paired" or "not paired". It reads the file of the
// source with no lock.
func whatsAppState(dataDir, id string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ok, err := whatsapp.IsPaired(ctx, dataDir, id)
	switch {
	case err != nil:
		return "unknown (" + err.Error() + ")"
	case ok:
		return "paired"
	}
	return "not paired"
}

// printWhatsAppChats lists the chats of a WhatsApp source from its file.
func printWhatsAppChats(ctx context.Context, w io.Writer, dataDir string, sc *config.Source) error {
	chats, err := whatsapp.ReadChats(ctx, dataDir, sc.ID)
	if err != nil {
		return err
	}
	tw := tabwriter.NewWriter(w, 2, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tKIND\tNAME\tSELECTED")
	for _, c := range chats {
		sel := ""
		for _, ch := range sc.Channels {
			if ch == c.JID {
				sel = "yes"
			}
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", c.JID, c.Kind, or(c.Name, "-"), sel)
	}
	return tw.Flush()
}

// RemoveWhatsApp logs out the device of a removed source and deletes its file.
// It returns a text for the user when the logout failed.
func RemoveWhatsApp(ctx context.Context, dataDir, id string, log *slog.Logger) string {
	if err := whatsapp.Remove(ctx, dataDir, id, removeWait, log); err != nil {
		return whatsapp.UnlinkHint(err)
	}
	return ""
}
