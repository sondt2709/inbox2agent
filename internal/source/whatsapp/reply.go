package whatsapp

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"google.golang.org/protobuf/proto"

	"github.com/sondt2709/inbox2agent/internal/inbox"
)

// connectWait limits the wait for the login after a connect, before a send
// or a download.
const connectWait = 30 * time.Second

// client is the part of the whatsmeow client that a reply and a download
// use. Tests replace it.
type client interface {
	SendMessage(ctx context.Context, to types.JID, m *waE2E.Message, extra ...whatsmeow.SendRequestExtra) (whatsmeow.SendResponse, error)
	DownloadToFile(ctx context.Context, msg whatsmeow.DownloadableMessage, f whatsmeow.File) error
}

// AnsweredError is an error after which no message exists: WhatsApp refused
// it, or i2a did not send it. reply.Send records the reply as failed.
type AnsweredError struct{ Err error }

func (e *AnsweredError) Error() string  { return e.Err.Error() }
func (e *AnsweredError) Unwrap() error  { return e.Err }
func (e *AnsweredError) Answered() bool { return true }

// ErrTooOldToQuote is returned for a quote of a message that the file of the
// source no longer has.
var ErrTooOldToQuote = errors.New("the message is too old to quote: choose Chat to send it without a quote")

// ErrClosed is returned by a source that was closed, for example after a
// change of the configuration.
var ErrClosed = errors.New("the WhatsApp connection was closed (the configuration changed): send again")

// refused are the whatsmeow errors after which no message exists.
var refused = []error{whatsmeow.ErrNotLoggedIn, whatsmeow.ErrNotConnected, whatsmeow.ErrRecipientNotOnWhatsApp,
	whatsmeow.ErrNoPermissionInGroup, whatsmeow.ErrServerReturnedError, whatsmeow.ErrUnknownServer,
	whatsmeow.ErrRecipientADJID, whatsmeow.ErrBroadcastListUnsupported, ErrNotPaired, ErrBusy, ErrClosed}

// classify marks an error after which no message exists. Other errors (a
// time-out) leave the result unknown.
func classify(err error) error {
	if err == nil {
		return nil
	}
	var a *AnsweredError
	if errors.As(err, &a) {
		return err
	}
	for _, r := range refused {
		if errors.Is(err, r) {
			return &AnsweredError{Err: err}
		}
	}
	return err
}

// MessageID returns the WhatsApp message ID of a reply. Each try of one
// reply gets the same ID, in the format of whatsmeow GenerateMessageID.
func MessageID(account, chat, pendingID string, replyID int64) types.MessageID {
	sum := sha256.Sum256([]byte(account + "\x00" + chat + "\x00" + pendingID + "\x00" + strconv.FormatInt(replyID, 10)))
	return types.MessageID(whatsmeow.WebMessageIDPrefix + strings.ToUpper(hex.EncodeToString(sum[:9])))
}

// replyMessage builds the message of a reply: plain text in the chat, or a
// quote of the stored target message. In a chat with disappearing messages,
// the reply gets the timer of the target message, as WhatsApp clients do.
func replyMessage(ctx context.Context, db *DB, r inbox.ReplyRequest) (*waE2E.Message, error) {
	row, ok, err := db.Row(ctx, r.Target.ChannelID, r.Target.ExternalID)
	if err != nil {
		return nil, err
	}
	var target *waE2E.Message
	if ok {
		target = &waE2E.Message{}
		if err := proto.Unmarshal(row.Raw, target); err != nil {
			return nil, &AnsweredError{Err: fmt.Errorf("read the stored message: %w", err)}
		}
	}
	expiration := contextInfo(target).GetExpiration()
	if !r.Thread {
		if expiration == 0 {
			return &waE2E.Message{Conversation: proto.String(r.Text)}, nil
		}
		return &waE2E.Message{ExtendedTextMessage: &waE2E.ExtendedTextMessage{Text: proto.String(r.Text),
			ContextInfo: &waE2E.ContextInfo{Expiration: proto.Uint32(expiration)}}}, nil
	}
	if !ok {
		return nil, &AnsweredError{Err: ErrTooOldToQuote}
	}
	ci := &waE2E.ContextInfo{
		StanzaID:      proto.String(row.ID),
		Participant:   proto.String(row.Sender),
		QuotedMessage: target,
	}
	if expiration > 0 {
		ci.Expiration = proto.Uint32(expiration)
	}
	return &waE2E.Message{ExtendedTextMessage: &waE2E.ExtendedTextMessage{Text: proto.String(r.Text), ContextInfo: ci}}, nil
}

// sendReply sends the reply with the client.
func sendReply(ctx context.Context, c client, db *DB, account string, r inbox.ReplyRequest) (inbox.Posted, error) {
	to, err := types.ParseJID(r.Target.ChannelID)
	if err != nil {
		return inbox.Posted{}, &AnsweredError{Err: fmt.Errorf("chat address %q: %w", r.Target.ChannelID, err)}
	}
	msg, err := replyMessage(ctx, db, r)
	if err != nil {
		return inbox.Posted{}, err
	}
	id := MessageID(account, r.Target.ChannelID, r.PendingID, r.ReplyID)
	resp, err := c.SendMessage(ctx, to, msg, whatsmeow.SendRequestExtra{ID: id})
	if err != nil {
		return inbox.Posted{}, classify(err)
	}
	return inbox.Posted{ID: string(resp.ID), URL: r.Target.URL}, nil
}

// Reply sends a reply to the chat of the target message: a quote of the
// target when r.Thread is true, else plain text. It connects when necessary.
func (src *Source) Reply(ctx context.Context, r inbox.ReplyRequest) (inbox.Posted, error) {
	src.mu.Lock()
	defer src.mu.Unlock()
	conn, err := src.connectLocked(ctx)
	if err != nil {
		return inbox.Posted{}, classify(err)
	}
	if !conn.cli.WaitForConnection(connectWait) {
		return inbox.Posted{}, &AnsweredError{Err: whatsmeow.ErrNotConnected}
	}
	return sendReply(ctx, conn.cli, conn.db, src.s.ID, r)
}

// Find returns false: WhatsApp has no search for the messages of the user.
// A new try sends again with the same message ID (MessageID).
func (src *Source) Find(context.Context, inbox.ReplyRequest) (inbox.Posted, bool, error) {
	return inbox.Posted{}, false, nil
}
