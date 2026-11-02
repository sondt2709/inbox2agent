package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/sondt2709/inbox2agent/internal/card"
	"github.com/sondt2709/inbox2agent/internal/inbox"
	"github.com/sondt2709/inbox2agent/internal/phone"
	"github.com/sondt2709/inbox2agent/internal/qrcode"
	"github.com/sondt2709/inbox2agent/internal/secrets"
	"github.com/sondt2709/inbox2agent/internal/store"
)

// newPhoneSender makes the sender of a ready state. Tests replace it to use fake endpoints.
var newPhoneSender = phone.NewSender

// MaxPhoneAlerts is the number of cards of one `i2a card add` call that give a
// phone notification. The other cards go to the phone with no notification.
const MaxPhoneAlerts = 5

// phonePushLimit is the time limit of the push step of `i2a card add`.
const phonePushLimit = 15 * time.Second

func newPhoneCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "phone",
		Short: "Send cards to an Android phone as sealed push messages",
		Long: `Send each new or updated card to a paired Android phone through Firebase Cloud Messaging.
The Mac seals each card with a key that only the paired phones have, so Google sees ciphertext only.

Steps:
  1. In a Firebase project, add an Android app with the package ` + phone.Package + `.
     Download its google-services.json and a service-account key (Project settings, Service accounts).
  2. i2a phone setup --service-account <key.json> --app-config <google-services.json>
  3. i2a phone pair, then scan the QR code with the i2a app.
  4. i2a phone test`,
	}
	cmd.AddCommand(newPhoneSetupCmd(), newPhonePairCmd(), newPhoneResetCmd(), newPhoneTestCmd(), newPhoneStatusCmd())
	return cmd
}

func newPhoneSetupCmd() *cobra.Command {
	var saFile, appFile string
	cmd := &cobra.Command{
		Use:   "setup --service-account <file> --app-config <file>",
		Short: "Store the Firebase service-account key and the app configuration",
		Long: `Store the Firebase service-account key (it sends the pushes) and the google-services.json
file of the Android app (the pairing link gives it to the phone) in the secret store.
Both files must be of one Firebase project. Give one flag to replace only that item.
Use a service account with only the role "Firebase Cloud Messaging API Admin".`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if saFile == "" && appFile == "" {
				return errors.New("give --service-account, --app-config or both")
			}
			e, err := newEnv()
			if err != nil {
				return err
			}
			cur, err := phone.Load(e.secrets)
			if err != nil {
				return err
			}
			var sa *phone.ServiceAccount
			var raw []byte
			if saFile != "" {
				if raw, err = os.ReadFile(saFile); err != nil {
					return err
				}
				s, err := phone.ParseServiceAccount(raw)
				if err != nil {
					return fmt.Errorf("%s: %w", saFile, err)
				}
				sa = &s
			} else {
				sa = cur.ServiceAccount
			}
			var app *phone.AppConfig
			if appFile != "" {
				b, err := os.ReadFile(appFile)
				if err != nil {
					return err
				}
				a, err := phone.ParseGoogleServices(b)
				if err != nil {
					return fmt.Errorf("%s: %w", appFile, err)
				}
				app = &a
			} else {
				app = cur.App
			}
			if sa != nil && app != nil && sa.ProjectID != app.ProjectID {
				return fmt.Errorf("the service account is of project %s, but the app configuration is of project %s; use files of one project", sa.ProjectID, app.ProjectID)
			}
			if saFile != "" {
				if _, err := phone.SaveServiceAccount(e.secrets, raw); err != nil {
					return err
				}
			}
			if appFile != "" {
				if err := phone.SaveAppConfig(e.secrets, *app); err != nil {
					return err
				}
			}
			project := ""
			if sa != nil {
				project = sa.ProjectID
			} else {
				project = app.ProjectID
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Stored in the %s for the Firebase project %s.\n", e.secrets.Backend(), project)
			switch {
			case sa == nil:
				fmt.Fprintln(cmd.OutOrStdout(), "Next: i2a phone setup --service-account <file>")
			case app == nil:
				fmt.Fprintln(cmd.OutOrStdout(), "Next: i2a phone setup --app-config <google-services.json>")
			default:
				fmt.Fprintln(cmd.OutOrStdout(), "Next: i2a phone pair")
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&saFile, "service-account", "", "the service-account key file (JSON)")
	cmd.Flags().StringVar(&appFile, "app-config", "", "the google-services.json file of the Android app")
	return cmd
}

func newPhonePairCmd() *cobra.Command {
	var linkOnly bool
	cmd := &cobra.Command{
		Use:   "pair",
		Short: "Show the QR code that pairs a phone",
		Long: `Show the pairing as a QR code and as a link. The first call makes a random topic and key.
Later calls show the same pairing, so more phones can scan it. Each person who sees
the QR code can read new pushes; run i2a phone reset to make a new pairing.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			e, err := newEnv()
			if err != nil {
				return err
			}
			s, err := phone.Load(e.secrets)
			if err != nil {
				return err
			}
			if s.ServiceAccount == nil || s.App == nil {
				return errors.New("no Firebase setup; run i2a phone setup --service-account <file> --app-config <google-services.json>")
			}
			if s.Pairing == nil {
				p, err := phone.NewPairing(time.Now())
				if err != nil {
					return err
				}
				if err := phone.SavePairing(e.secrets, p); err != nil {
					return err
				}
				s.Pairing = &p
			}
			link := phone.Link(*s.App, *s.Pairing, hostName())
			out := cmd.OutOrStdout()
			if linkOnly {
				fmt.Fprintln(out, link)
				return nil
			}
			qr, err := qrcode.Text(link)
			if err != nil {
				return err
			}
			fmt.Fprintln(out, "Scan this QR code in the i2a app (Pair, Scan QR code):")
			fmt.Fprintln(out)
			fmt.Fprintln(out, qr)
			fmt.Fprintln(out)
			fmt.Fprintln(out, "Or paste this link in the app:")
			fmt.Fprintln(out, link)
			fmt.Fprintln(out)
			fmt.Fprintln(out, "Keep the code private: it holds the key of your pushes. Then run: i2a phone test")
			return nil
		},
	}
	cmd.Flags().BoolVar(&linkOnly, "link", false, "print the link only")
	return cmd
}

func newPhoneResetCmd() *cobra.Command {
	var remove bool
	cmd := &cobra.Command{
		Use:   "reset",
		Short: "Make a new pairing, so the paired phones get no more pushes",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			e, err := newEnv()
			if err != nil {
				return err
			}
			if remove {
				if err := phone.RemoveAll(e.secrets); err != nil {
					return err
				}
				fmt.Fprintln(cmd.OutOrStdout(), "Removed the Firebase setup and the pairing.")
				return nil
			}
			p, err := phone.NewPairing(time.Now())
			if err != nil {
				return err
			}
			if err := phone.SavePairing(e.secrets, p); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "Made a new pairing. Phones with the old pairing get no more pushes. Run i2a phone pair to pair again.")
			return nil
		},
	}
	cmd.Flags().BoolVar(&remove, "remove", false, "delete the Firebase setup and the pairing from the secret store")
	return cmd
}

func newPhoneTestCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "test",
		Short: "Send a test push",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			e, err := newEnv()
			if err != nil {
				return err
			}
			s, err := phone.Load(e.secrets)
			if err != nil {
				return err
			}
			if !s.Ready() {
				return errors.New("the phone push is not ready; run i2a phone status")
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), phonePushLimit)
			defer cancel()
			if err := newPhoneSender(s).Send(ctx, &phone.Payload{Kind: phone.KindTest, Alert: true}); err != nil {
				return fmt.Errorf("phone push: %w", err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Sent a test push to topic %s….\n", s.Pairing.Topic[:12])
			return nil
		},
	}
}

func newPhoneStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show the Firebase setup and the pairing",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			e, err := newEnv()
			if err != nil {
				return err
			}
			s, err := phone.Load(e.secrets)
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "Secret store     %s\n", e.secrets.Backend())
			if s.ServiceAccount != nil {
				fmt.Fprintf(out, "Service account  %s (project %s)\n", s.ServiceAccount.ClientEmail, s.ServiceAccount.ProjectID)
			} else {
				fmt.Fprintln(out, "Service account  none. Run: i2a phone setup --service-account <file>")
			}
			if s.App != nil {
				fmt.Fprintf(out, "App config       project %s, app %s\n", s.App.ProjectID, s.App.AppID)
			} else {
				fmt.Fprintln(out, "App config       none. Run: i2a phone setup --app-config <google-services.json>")
			}
			if s.Pairing != nil {
				fmt.Fprintf(out, "Pairing          topic %s…, made %s\n", s.Pairing.Topic[:12], s.Pairing.PairedAt.Local().Format("2006-01-02 15:04"))
			} else {
				fmt.Fprintln(out, "Pairing          none. Run: i2a phone pair")
			}
			if os.Getenv("I2A_NOTIFY") == "off" {
				fmt.Fprintln(out, "Push             off (I2A_NOTIFY=off)")
			} else if s.Ready() {
				fmt.Fprintln(out, "Push             on: i2a card add sends new and updated cards")
			}
			return nil
		},
	}
}

func hostName() string {
	h, err := os.Hostname()
	if err != nil {
		return "Mac"
	}
	return strings.TrimSuffix(h, ".local")
}

// pushCards sends the added and updated cards to the paired phone. It returns
// the number of messages sent. It makes no network call when no pairing exists.
func pushCards(ctx context.Context, sec secrets.Store, st *store.Store, saved []inbox.SavedCard, notifyOn bool, minUrgency string, warn io.Writer) int {
	if os.Getenv("I2A_NOTIFY") == "off" {
		return 0
	}
	if _, err := sec.Get(phone.KeyTopic); err != nil {
		return 0
	}
	s, err := phone.Load(sec)
	if err != nil {
		fmt.Fprintln(warn, "warning: phone push:", err)
		return 0
	}
	if !s.Ready() {
		return 0
	}
	ctx, cancel := context.WithTimeout(ctx, phonePushLimit)
	defer cancel()
	sender := newPhoneSender(s)
	min, alerts, sent := inbox.UrgencyRank(minUrgency), 0, 0
	for _, c := range saved {
		if c.Result != inbox.SaveAdded && c.Result != inbox.SaveUpdated {
			continue
		}
		alert := notifyOn && card.Counts(c) && inbox.UrgencyRank(c.Urgency) >= min && alerts < MaxPhoneAlerts
		if alert {
			alerts++
		}
		msgs, err := st.MessagesByID(ctx, c.Refs)
		if err != nil {
			fmt.Fprintln(warn, "warning: phone push:", err)
		}
		pc := phone.FromCard(c.Card, msgs)
		if err := sender.Send(ctx, &phone.Payload{Kind: phone.KindCard, Alert: alert, Card: &pc}); err != nil {
			fmt.Fprintln(warn, "warning: phone push:", err)
			return sent
		}
		sent++
	}
	return sent
}
