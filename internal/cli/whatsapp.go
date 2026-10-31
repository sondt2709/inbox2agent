package cli

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/sondt2709/inbox2agent/internal/config"
	"github.com/sondt2709/inbox2agent/internal/source/whatsapp"
)

// removeWait is the wait for the daemon to close the connection of a removed account.
const removeWait = 5 * time.Second

func newAccountAddWhatsAppCmd() *cobra.Command {
	var id, name, phone string
	cmd := &cobra.Command{
		Use:   "whatsapp --id <id> [--phone <number>]",
		Short: "Pair a WhatsApp account as a linked device and add it",
		Long: `Pair a WhatsApp account as a linked device of the phone, then add the account.
The command prints a QR code to scan. With --phone, it prints a pairing code
to type on the phone instead. The chats of the account start in Unsorted.

WhatsApp does not allow unofficial clients and can ban an account that uses one.
i2a only reads messages, and it never sends read receipts or online presence.

Example:
  i2a account add whatsapp --id phone --name "Personal phone"`,
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
			if _, ok := cfg.Account(id); ok {
				return fmt.Errorf("an account with the ID %q exists. Remove it first with `i2a account remove %s`", id, id)
			}
			a := config.NewWhatsAppAccount(id)
			a.Name = or(name, id)
			next := *cfg
			next.Accounts = append(append([]config.Account(nil), cfg.Accounts...), a)
			if err := next.Validate(); err != nil {
				return err
			}
			if err := e.paths.EnsureDataDir(); err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if err := PairWhatsApp(cmd.Context(), e.paths.DataDir, id, phone, cfg.Settings.Lookback.D(), out); err != nil {
				return err
			}
			if err := addAccount(e.paths.ConfigFile, a); err != nil {
				return err
			}
			fmt.Fprintf(out, "Added account %s to %s. Its chats are in Unsorted.\n", id, e.paths.ConfigFile)
			return nil
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "account ID, for example phone (required)")
	cmd.Flags().StringVar(&name, "name", "", "the name that i2a shows, for example \"Personal phone\" (default: the ID)")
	cmd.Flags().StringVar(&phone, "phone", "", "phone number in international format; prints a pairing code instead of a QR code")
	_ = cmd.MarkFlagRequired("id")
	return cmd
}

// PairWhatsApp pairs the account and prints the codes. An account that is paired
// already is kept. When the pairing fails or the user presses ctrl+c, the
// file of the account is deleted.
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
		fmt.Fprintf(out, "The account %s is paired already.\n", id)
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

// whatsAppState returns "paired" or "not paired". It reads the file of the
// account with no lock.
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

// RemoveWhatsApp logs out the device of a removed account and deletes its file.
// It returns a text for the user when the logout failed.
func RemoveWhatsApp(ctx context.Context, dataDir, id string, log *slog.Logger) string {
	if err := whatsapp.Remove(ctx, dataDir, id, removeWait, log); err != nil {
		return whatsapp.UnlinkHint(err)
	}
	return ""
}
