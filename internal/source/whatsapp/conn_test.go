package whatsapp

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	waLog "go.mau.fi/whatsmeow/util/log"
	"google.golang.org/protobuf/proto"

	"github.com/sondt2709/inbox2agent/internal/inbox"
)

// fakeNames is the contact store of the tests, by address.
type fakeNames struct {
	names, phones map[string]string
}

func (f fakeNames) Name(_ context.Context, j types.JID) string  { return f.names[j.String()] }
func (f fakeNames) Phone(_ context.Context, j types.JID) string { return f.phones[j.String()] }

var noNames = fakeNames{}

func TestRowFromEvent(t *testing.T) {
	ctx := context.Background()
	ts := time.Date(2026, 10, 29, 9, 0, 0, 0, time.UTC)
	me := Me{Phone: fxPhone, LID: "100000000000004"}

	t.Run("group mention", func(t *testing.T) {
		fx := fixtureByID(t, "05")
		r, ok := rowFromEvent(ctx, fx.event(ts), me, fakeNames{names: map[string]string{fx.Sender: "Bob"}})
		if !ok {
			t.Fatal("skipped")
		}
		want := Row{Chat: group2, ID: fx.ID, Sender: fx.Sender, SenderName: "Bob", Kind: inbox.KindGroup, Mention: true,
			Time: ts, Text: "text 6 @100000000000004"}
		r.Raw = checkRaw(t, r.Raw, fx.Message)
		if !reflect.DeepEqual(r, want) {
			t.Errorf("row = %+v\nwant  %+v", r, want)
		}
	})

	t.Run("direct LID chat with the phone in SenderAlt", func(t *testing.T) {
		fx := fixtureByID(t, "0B")
		evt := fx.event(ts)
		evt.Info.SenderAlt = types.NewJID("15550000001", types.DefaultUserServer)
		r, ok := rowFromEvent(ctx, evt, me, fakeNames{names: map[string]string{"15550000001@s.whatsapp.net": "Carol"}})
		if !ok || r.Kind != inbox.KindDirect || r.Phone != "15550000001" || r.SenderName != "Carol" || r.Mention {
			t.Errorf("row = %+v, %t", r, ok)
		}
	})

	t.Run("direct LID chat with the phone from the LID store and the push name", func(t *testing.T) {
		fx := fixtureByID(t, "0F")
		evt := fx.event(ts)
		evt.Info.PushName = "Dan"
		r, _ := rowFromEvent(ctx, evt, me, fakeNames{phones: map[string]string{direct5: "15550000002"}})
		if r.Phone != "15550000002" || r.SenderName != "Dan" {
			t.Errorf("phone, name = %q, %q", r.Phone, r.SenderName)
		}
	})

	t.Run("own direct message", func(t *testing.T) {
		fx := fixtureByID(t, "0E")
		evt := fx.event(ts)
		evt.Info.RecipientAlt = types.NewJID("15550000002", types.DefaultUserServer)
		r, ok := rowFromEvent(ctx, evt, me, fakeNames{names: map[string]string{fx.Sender: "Me"}})
		if !ok || !r.FromMe || r.SenderName != "" || r.Phone != "15550000002" {
			t.Errorf("row = %+v, %t", r, ok)
		}
	})

	t.Run("sender with no name gets the phone number", func(t *testing.T) {
		fx := fixtureByID(t, "02")
		r, _ := rowFromEvent(ctx, fx.event(ts), me, fakeNames{phones: map[string]string{fx.Sender: "15550000003"}})
		if r.SenderName != "+15550000003" || r.Phone != "" {
			t.Errorf("name, phone = %q, %q", r.SenderName, r.Phone)
		}
	})

	skips := map[string]func(fx fixture) fixture{
		"status update": func(fx fixture) fixture { fx.Chat = types.StatusBroadcastJID.String(); return fx },
		"broadcast list": func(fx fixture) fixture {
			fx.Chat = "1793000000@" + types.BroadcastServer
			return fx
		},
		"newsletter": func(fx fixture) fixture { fx.Chat = "120363000000000009@" + types.NewsletterServer; return fx },
		"reaction": func(fx fixture) fixture {
			fx.Message = &waE2E.Message{ReactionMessage: &waE2E.ReactionMessage{Text: proto.String("👍")}}
			return fx
		},
		"no message": func(fx fixture) fixture { fx.Message = nil; return fx },
	}
	for name, change := range skips {
		t.Run("skip "+name, func(t *testing.T) {
			fx := change(fixtureByID(t, "02"))
			if r, ok := rowFromEvent(ctx, fx.event(ts), me, noNames); ok {
				t.Errorf("not skipped: %+v", r)
			}
		})
	}
	t.Run("skip edit", func(t *testing.T) {
		evt := fixtureByID(t, "02").event(ts)
		evt.IsEdit = true
		if _, ok := rowFromEvent(ctx, evt, me, noNames); ok {
			t.Error("not skipped")
		}
	})
}

// checkRaw checks that the stored protobuf is the message, and returns nil.
func checkRaw(t *testing.T, raw []byte, want *waE2E.Message) []byte {
	t.Helper()
	got := &waE2E.Message{}
	if err := proto.Unmarshal(raw, got); err != nil || !proto.Equal(got, want) {
		t.Errorf("raw is not the message (err %v)", err)
	}
	return nil
}

func TestLockFile(t *testing.T) {
	path := LockPath(t.TempDir(), "phone")
	unlock, err := lockFile(path, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := lockFile(path, 0); !errors.Is(err, ErrBusy) {
		t.Fatalf("second lock: err = %v, want ErrBusy", err)
	}
	start := time.Now()
	if _, err := lockFile(path, 300*time.Millisecond); !errors.Is(err, ErrBusy) || time.Since(start) < 300*time.Millisecond {
		t.Fatalf("lock with a wait: err = %v after %s", err, time.Since(start))
	}
	unlock()
	unlock2, err := lockFile(path, 0)
	if err != nil {
		t.Fatalf("lock after unlock: %v", err)
	}
	unlock2()
}

func TestFileModeAndNotPaired(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir() + "/data"
	if ok, err := IsPaired(ctx, dir, "phone"); ok || err != nil {
		t.Fatalf("IsPaired with no file = %t, %v", ok, err)
	}
	db, err := OpenDB(ctx, Path(dir, "phone"), waLog.Noop)
	if err != nil {
		t.Fatal(err)
	}
	db.Close()
	for path, want := range map[string]os.FileMode{dir: 0o700, Path(dir, "phone"): 0o600} {
		if fi, err := os.Stat(path); err != nil || fi.Mode().Perm() != want {
			t.Errorf("mode of %s = %v, %v, want %v", path, fi.Mode().Perm(), err, want)
		}
	}
	if ok, err := IsPaired(ctx, dir, "phone"); ok || err != nil {
		t.Errorf("IsPaired of a new file = %t, %v", ok, err)
	}
}

func TestOpenNotPairedThenRemove(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	c, err := Open(ctx, dir, "phone", time.Hour, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Open(ctx, dir, "phone", time.Hour, nil); !errors.Is(err, ErrBusy) {
		t.Errorf("second Open: err = %v, want ErrBusy", err)
	}
	if err := c.Connect(ctx); !errors.Is(err, ErrNotPaired) {
		t.Errorf("Connect: err = %v, want ErrNotPaired", err)
	}
	c.Close()

	if err := Remove(ctx, dir, "phone", time.Second, nil); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		t.Errorf("file left after Remove: %s", e.Name())
	}
}

func TestWhatsmeowLogsSkipNoise(t *testing.T) {
	var buf bytes.Buffer
	l := slogLogger{log: slog.New(slog.NewTextHandler(&buf, nil))}.Sub("client").Sub("Socket")
	l.Warnf("Error sending close to websocket: %v", "EOF")
	l.Infof("connected")
	if buf.Len() != 0 {
		t.Errorf("logged: %s", buf.String())
	}
	l.Warnf("Failed to send %s", "presence")
	if s := buf.String(); !strings.Contains(s, "whatsmeow: Failed to send presence") || !strings.Contains(s, "module=client/Socket") {
		t.Errorf("log = %q", s)
	}
}
