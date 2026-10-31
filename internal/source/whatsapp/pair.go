package whatsapp

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"go.mau.fi/whatsmeow"
)

// PairSteps tells the user how to link i2a on the phone.
const PairSteps = "On the phone: WhatsApp › Settings › Linked devices › Link a device."

// UnlinkHint tells the user what to do after a failed logout.
func UnlinkHint(err error) string {
	return fmt.Sprintf("Could not log out the linked device (%v). The file is deleted. On the phone, remove %q in Settings › Linked devices.",
		err, DeviceName)
}

// chatsWait limits the wait for the chats after pairing.
const chatsWait = 30 * time.Second

// ErrPairTimeout is returned when nobody scans the code before WhatsApp stops
// giving codes (after about 160 seconds).
var ErrPairTimeout = errors.New("pairing timed out")

// PairEvent is one step of the pairing.
type PairEvent struct {
	QR   string // a new QR code to show
	Code string // the pairing code to type on the phone
}

// Pair connects a new device and gives the codes to show. With a phone number
// in international format, it gives one pairing code; else a QR code each
// time WhatsApp sends one. After the scan, it waits for the groups and the
// first history sync (at most 30 seconds), so that the chats are in the file.
func (c *Conn) Pair(ctx context.Context, phone string, show func(PairEvent)) error {
	if c.Paired() {
		return errors.New("the source is paired already")
	}
	qrc, err := c.cli.GetQRChannel(ctx)
	if err != nil {
		return err
	}
	if err := c.cli.ConnectContext(ctx); err != nil {
		return err
	}
	asked := false
	for item := range qrc {
		switch {
		case item.Event == whatsmeow.QRChannelEventCode:
			if phone == "" {
				show(PairEvent{QR: item.Code})
				continue
			}
			if !asked {
				asked = true
				code, err := c.cli.PairPhone(ctx, phone, true, whatsmeow.PairClientChrome, "Chrome (Mac OS)")
				if err != nil {
					c.cli.Disconnect()
					return fmt.Errorf("pairing code: %w", err)
				}
				show(PairEvent{Code: code})
			}
		case item == whatsmeow.QRChannelSuccess:
			c.waitChats(ctx)
			return nil
		case item == whatsmeow.QRChannelTimeout:
			return ErrPairTimeout
		case item.Error != nil:
			return item.Error
		default:
			return fmt.Errorf("pairing failed: %s", item.Event)
		}
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return errors.New("pairing stopped")
}

func (c *Conn) waitChats(ctx context.Context) {
	end := time.After(chatsWait)
	for _, ch := range []chan struct{}{c.chats, c.history} {
		select {
		case <-ch:
		case <-end:
			return
		case <-ctx.Done():
			return
		}
	}
}

// openRO opens the file of a source read only. It returns os.ErrNotExist
// when the file does not exist.
func openRO(dataDir, id string) (*sql.DB, error) {
	path := Path(dataDir, id)
	if _, err := os.Stat(path); err != nil {
		return nil, err
	}
	return sql.Open("sqlite", "file:"+path+"?mode=ro&_pragma=busy_timeout(5000)")
}

// IsPaired reads the file of a source and reports whether it has device keys.
// It needs no lock, so it works while the daemon holds the connection.
func IsPaired(ctx context.Context, dataDir, id string) (bool, error) {
	db, err := openRO(dataDir, id)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	defer db.Close()
	var n int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM whatsmeow_device`).Scan(&n); err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return false, nil
		}
		return false, err
	}
	return n > 0, nil
}

// ReadChats reads the chats of a source from its file, the most recent first.
// It needs no lock.
func ReadChats(ctx context.Context, dataDir, id string) ([]Chat, error) {
	db, err := openRO(dataDir, id)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer db.Close()
	return (&DB{sql: db}).Chats(ctx)
}

// Device identifies the account of a paired source for a person.
type Device struct {
	Phone string // international format, for example +15551234567
	Name  string // the profile name (push name) of the account
}

// ReadDevice reads the phone number and the profile name of a paired source
// from its file. It needs no lock and no network. A source that is not paired
// gives an empty Device.
func ReadDevice(ctx context.Context, dataDir, id string) (Device, error) {
	db, err := openRO(dataDir, id)
	if errors.Is(err, os.ErrNotExist) {
		return Device{}, nil
	}
	if err != nil {
		return Device{}, err
	}
	defer db.Close()
	var jid, name string
	err = db.QueryRowContext(ctx, `SELECT jid, push_name FROM whatsmeow_device LIMIT 1`).Scan(&jid, &name)
	if errors.Is(err, sql.ErrNoRows) || err != nil && strings.Contains(err.Error(), "no such table") {
		return Device{}, nil
	}
	if err != nil {
		return Device{}, err
	}
	return Device{Phone: phoneOf(jid), Name: name}, nil
}

// phoneOf returns "+<number>" for a device JID such as "15551234567:12@s.whatsapp.net".
func phoneOf(jid string) string {
	user, _, _ := strings.Cut(jid, "@")
	user, _, _ = strings.Cut(user, ":")
	if user == "" {
		return ""
	}
	return "+" + user
}

// Remove unlinks the device on the phone and deletes the files of the source.
// It waits up to wait for the lock, for example while the daemon closes the
// connection after a configuration reload. It deletes the files also when the
// logout fails, and then returns the error.
func Remove(ctx context.Context, dataDir, id string, wait time.Duration, log *slog.Logger) error {
	var logoutErr error
	if _, err := os.Stat(Path(dataDir, id)); err == nil {
		logoutErr = logout(ctx, dataDir, id, wait, log)
	}
	for _, p := range []string{Path(dataDir, id), Path(dataDir, id) + "-wal", Path(dataDir, id) + "-shm", LockPath(dataDir, id)} {
		if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return logoutErr
}

func logout(ctx context.Context, dataDir, id string, wait time.Duration, log *slog.Logger) error {
	c, err := open(ctx, dataDir, id, 0, log, wait)
	if err != nil {
		return err
	}
	defer c.Close()
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	return c.Logout(ctx)
}
