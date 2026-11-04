package whatsapp

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"testing"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"google.golang.org/protobuf/proto"

	"github.com/sondt2709/inbox2agent/internal/inbox"
)

// fakeClient records the sends and the downloads of the tests.
type fakeClient struct {
	to      []types.JID
	msgs    []*waE2E.Message
	ids     []types.MessageID
	sendErr error
	data    []byte
	dlErr   error
	dlParts []whatsmeow.DownloadableMessage
}

func (f *fakeClient) SendMessage(_ context.Context, to types.JID, m *waE2E.Message, extra ...whatsmeow.SendRequestExtra) (whatsmeow.SendResponse, error) {
	f.to = append(f.to, to)
	f.msgs = append(f.msgs, m)
	f.ids = append(f.ids, extra[0].ID)
	if f.sendErr != nil {
		return whatsmeow.SendResponse{}, f.sendErr
	}
	return whatsmeow.SendResponse{ID: extra[0].ID, Timestamp: now}, nil
}

func (f *fakeClient) DownloadToFile(_ context.Context, part whatsmeow.DownloadableMessage, file whatsmeow.File) error {
	f.dlParts = append(f.dlParts, part)
	if f.dlErr != nil {
		return f.dlErr
	}
	_, err := file.Write(f.data)
	return err
}

func TestMessageID(t *testing.T) {
	a := MessageID("phone", group1, "1793800000000", 7)
	if a != MessageID("phone", group1, "1793800000000", 7) {
		t.Error("the same reply got two IDs")
	}
	if !regexp.MustCompile(`^3EB0[0-9A-F]{18}$`).MatchString(string(a)) {
		t.Errorf("ID %q does not have the format of GenerateMessageID", a)
	}
	for _, b := range []types.MessageID{MessageID("phone", group1, "1793800000000", 8), MessageID("phone", group2, "1793800000000", 7),
		MessageID("other", group1, "1793800000000", 7), MessageID("phone", group1, "1793800000001", 7)} {
		if a == b {
			t.Errorf("two replies got the same ID %s", a)
		}
	}
}

// target returns the stored message of a fixture as the store of i2a has it.
func target(fx fixture) inbox.Message {
	return inbox.Message{ChannelID: fx.Chat, ExternalID: fx.ID, ChannelKind: fx.Kind}
}

func TestSendReply(t *testing.T) {
	ctx := context.Background()
	db := fixtureDB(t)

	t.Run("plain message in a direct chat", func(t *testing.T) {
		fx := fixtureByID(t, "0B")
		f := &fakeClient{}
		r := inbox.ReplyRequest{Target: target(fx), Text: "ok, 3pm", PendingID: "1793800000000", ReplyID: 4}
		p, err := sendReply(ctx, f, db, "phone", r)
		if err != nil {
			t.Fatal(err)
		}
		want := MessageID("phone", fx.Chat, "1793800000000", 4)
		if p.ID != string(want) || f.ids[0] != want || f.to[0].String() != fx.Chat {
			t.Errorf("posted %+v, sent %v to %v; want ID %s to %s", p, f.ids, f.to, want, fx.Chat)
		}
		if !proto.Equal(f.msgs[0], &waE2E.Message{Conversation: proto.String("ok, 3pm")}) {
			t.Errorf("message = %v; want plain text", f.msgs[0])
		}
	})

	t.Run("quote in a group", func(t *testing.T) {
		fx := fixtureByID(t, "05")
		f := &fakeClient{}
		_, err := sendReply(ctx, f, db, "phone", inbox.ReplyRequest{Target: target(fx), Text: "yes", Thread: true, PendingID: "1", ReplyID: 1})
		if err != nil {
			t.Fatal(err)
		}
		et := f.msgs[0].GetExtendedTextMessage()
		ci := et.GetContextInfo()
		if et.GetText() != "yes" || ci.GetStanzaID() != fx.ID || ci.GetParticipant() != fx.Sender ||
			!proto.Equal(ci.GetQuotedMessage(), fx.Message) || ci.Expiration != nil {
			t.Errorf("message = %v; want a quote of %s from %s", f.msgs[0], fx.ID, fx.Sender)
		}
	})

	t.Run("quote of a deleted row", func(t *testing.T) {
		f := &fakeClient{}
		r := inbox.ReplyRequest{Target: inbox.Message{ChannelID: group1, ExternalID: "3EB0FFFFFFFFFFFFFFFF"}, Text: "yes", Thread: true}
		_, err := sendReply(ctx, f, db, "phone", r)
		if !errors.Is(err, ErrTooOldToQuote) || !answered(err) || len(f.msgs) != 0 {
			t.Errorf("err = %v (answered %t), sends %d; want ErrTooOldToQuote, answered, no send", err, answered(err), len(f.msgs))
		}
	})

	// The error shapes copy the probe output of task 1 and whatsmeow send.go.
	for _, tc := range []struct {
		name     string
		err      error
		answered bool
	}{
		{"not connected (probe)", fmt.Errorf("failed to send message node: %w", whatsmeow.ErrNotConnected), true},
		{"server error", fmt.Errorf("%w %d", whatsmeow.ErrServerReturnedError, 406), true},
		{"not in the group", whatsmeow.ErrNoPermissionInGroup, true},
		{"time-out", whatsmeow.ErrMessageTimedOut, false},
		{"context", context.DeadlineExceeded, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := &fakeClient{sendErr: tc.err}
			_, err := sendReply(ctx, f, db, "phone", inbox.ReplyRequest{Target: target(fixtureByID(t, "0B")), Text: "x"})
			if !errors.Is(err, tc.err) || answered(err) != tc.answered {
				t.Errorf("err = %v, answered %t; want %v, answered %t", err, answered(err), tc.err, tc.answered)
			}
		})
	}
}

func answered(err error) bool {
	var a interface{ Answered() bool }
	return errors.As(err, &a) && a.Answered()
}

func TestClosedSourceDoesNotConnect(t *testing.T) {
	src := New(Settings{ID: "phone", DataDir: t.TempDir()})
	if err := src.Close(); err != nil {
		t.Fatal(err)
	}
	_, err := src.Reply(context.Background(), inbox.ReplyRequest{Text: "x"})
	if !errors.Is(err, ErrClosed) || !answered(err) {
		t.Errorf("Reply after Close = %v; want ErrClosed, answered", err)
	}
	if _, err := os.Stat(Path(src.s.DataDir, "phone")); !os.IsNotExist(err) {
		t.Errorf("a closed source opened its file: %v", err)
	}
}

func TestDownload(t *testing.T) {
	ctx := context.Background()
	db := fixtureDB(t)
	dir := filepath.Join(t.TempDir(), "tmp")

	t.Run("document", func(t *testing.T) {
		fx := fixtureByID(t, "06")
		f := &fakeClient{data: []byte("PK document bytes")}
		got, err := download(ctx, f, db, fx.Chat, fx.ID, dir)
		if err != nil {
			t.Fatal(err)
		}
		b, _ := os.ReadFile(got.Path)
		info, _ := os.Stat(got.Path)
		if string(b) != "PK document bytes" || info.Mode().Perm() != 0o600 || filepath.Dir(got.Path) != dir {
			t.Errorf("file %s = %q, mode %v", got.Path, b, info.Mode())
		}
		want := File{Path: got.Path, Name: "file-9.xlsx", MIME: "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
			Kind: "document", Size: 17}
		if got != want || !proto.Equal(f.dlParts[0].(proto.Message), fx.Message.GetDocumentMessage()) {
			t.Errorf("file = %+v; want %+v", got, want)
		}
	})

	t.Run("image without a name", func(t *testing.T) {
		fx := fixtureByID(t, "01")
		got, err := download(ctx, &fakeClient{data: []byte{0xff, 0xd8}}, db, fx.Chat, fx.ID, dir)
		if err != nil || got.Kind != "image" || got.Name != "" || got.MIME != "image/jpeg" {
			t.Errorf("file = %+v, %v", got, err)
		}
	})

	for _, tc := range []struct {
		name, chat, id string
		client         *fakeClient
		want           error
	}{
		{"text message", fixtureByID(t, "0B").Chat, fixtureByID(t, "0B").ID, &fakeClient{}, ErrNoFile},
		{"row deleted", group1, "3EB0FFFFFFFFFFFFFFFF", &fakeClient{}, ErrGone},
		{"media deleted on the server", fixtureByID(t, "01").Chat, fixtureByID(t, "01").ID,
			&fakeClient{dlErr: fmt.Errorf("download failed: %w", whatsmeow.ErrMediaDownloadFailedWith404)}, ErrGone},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before, _ := os.ReadDir(dir)
			_, err := download(ctx, tc.client, db, tc.chat, tc.id, dir)
			if !errors.Is(err, tc.want) {
				t.Errorf("err = %v; want %v", err, tc.want)
			}
			after, _ := os.ReadDir(dir)
			if len(after) != len(before) {
				t.Errorf("files in %s: %d before, %d after; want no new file", dir, len(before), len(after))
			}
		})
	}

	t.Run("too large", func(t *testing.T) {
		fx := fixtureByID(t, "01")
		m := proto.Clone(fx.Message).(*waE2E.Message)
		m.ImageMessage.FileLength = proto.Uint64(300 << 20)
		raw, _ := proto.Marshal(m)
		if err := db.SaveMessage(ctx, Row{Chat: fx.Chat, ID: "3EB0000000000000BIG1", Sender: fx.Sender, Kind: fx.Kind, Time: now,
			Text: "[image]", Raw: raw, Media: true}); err != nil {
			t.Fatal(err)
		}
		f := &fakeClient{}
		_, err := download(ctx, f, db, fx.Chat, "3EB0000000000000BIG1", dir)
		var tl *TooLargeError
		if !errors.As(err, &tl) || err.Error() != "the file has 300 MB; the maximum is 100 MB" || len(f.dlParts) != 0 {
			t.Errorf("err = %v, downloads %d; want TooLargeError and no download", err, len(f.dlParts))
		}
	})
}

func TestPruneKeepsMediaUntilRetention(t *testing.T) {
	ctx := context.Background()
	db := fixtureDB(t)
	text, image := fixtureByID(t, "0F"), fixtureByID(t, "01")
	add := func(fx fixture, id string, at time.Time) {
		r, _ := rowFromEvent(ctx, fx.event(at), Me{}, noNames)
		r.ID = id
		if err := db.SaveMessage(ctx, r); err != nil {
			t.Fatal(err)
		}
	}
	add(text, "3EB000000000000000T2", now.Add(-48*time.Hour))
	add(image, "3EB000000000000000M2", now.Add(-48*time.Hour))
	add(image, "3EB000000000000000M8", now.Add(-8*24*time.Hour))
	s := Settings{ID: "phone", Lookback: 24 * time.Hour, Retention: 7 * 24 * time.Hour}
	if _, _, err := collect(ctx, db, s, nil, now); err != nil {
		t.Fatal(err)
	}
	for id, want := range map[string]bool{"3EB000000000000000T2": false, "3EB000000000000000M2": true, "3EB000000000000000M8": false} {
		chat := text.Chat
		if id != "3EB000000000000000T2" {
			chat = image.Chat
		}
		r, ok, err := db.Row(ctx, chat, id)
		if err != nil || ok != want {
			t.Errorf("row %s kept = %t, %v; want %t", id, ok, err, want)
		}
		if ok && !r.Media {
			t.Errorf("row %s has media = false", id)
		}
	}
}

func TestOpenAddsMediaColumn(t *testing.T) {
	ctx := context.Background()
	path := Path(t.TempDir(), "old")
	db, err := OpenDB(ctx, path, nil)
	if err != nil {
		t.Fatal(err)
	}
	// The table of the version before the column media.
	if _, err := db.sql.ExecContext(ctx, `DROP TABLE wa_messages; CREATE TABLE wa_messages (chat TEXT NOT NULL, id TEXT NOT NULL,
		sender TEXT NOT NULL, sender_name TEXT NOT NULL DEFAULT '', phone TEXT NOT NULL DEFAULT '', kind TEXT NOT NULL,
		from_me INTEGER NOT NULL DEFAULT 0, mention INTEGER NOT NULL DEFAULT 0, ts INTEGER NOT NULL, text TEXT NOT NULL, raw BLOB,
		PRIMARY KEY (chat, id));
		INSERT INTO wa_messages(chat, id, sender, kind, ts, text) VALUES ('c', 'old', 's', 'direct', 1, '[image]')`); err != nil {
		t.Fatal(err)
	}
	db.Close()
	db, err = OpenDB(ctx, path, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	r, ok, err := db.Row(ctx, "c", "old")
	if err != nil || !ok || r.Media {
		t.Errorf("old row = %+v, %t, %v; want media = false", r, ok, err)
	}
}
