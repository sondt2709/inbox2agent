// Package reply sends a reply to a stored message through its source. The
// Feed and "i2a reply" use the same flow, so the retry and duplicate rules are
// the same for the user and the agent.
package reply

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/sondt2709/inbox2agent/internal/inbox"
	"github.com/sondt2709/inbox2agent/internal/render"
	"github.com/sondt2709/inbox2agent/internal/source"
)

// MaxText is the maximum number of characters of a reply.
const MaxText = 4000

// ErrEmpty is returned for a reply without text.
var ErrEmpty = errors.New("the text is empty")

// Store is the part of the store that Send uses.
type Store interface {
	MessagesByID(ctx context.Context, ids []int64) ([]inbox.Message, error)
	FindReply(ctx context.Context, messageID int64, place, text string) (inbox.Reply, bool, error)
	StartReply(ctx context.Context, r inbox.Reply) (int64, error)
	FinishReply(ctx context.Context, id int64, state, externalID, url, errText string, now time.Time) error
	SetCardState(ctx context.Context, id int64, key, state string, now time.Time) error
}

// Builder returns the replier of a source.
type Builder func(sourceID string) (source.Replier, error)

// Request is one reply that the user or the agent asks for.
type Request struct {
	MessageID int64
	CardID    int64 // 0 when the reply has no card; the card becomes done after the send
	Text      string
	Place     string // inbox.PlaceThread or inbox.PlaceChannel
	By        string // inbox.ByUser or inbox.ByAgent
}

// AlreadySentError is returned when the same reply was sent before.
type AlreadySentError struct{ At time.Time }

func (e *AlreadySentError) Error() string {
	return "this reply was sent at " + e.At.Local().Format("15:04") + "; it is not sent again"
}

// UnknownError is returned when the platform did not answer, so i2a does not
// know whether the post exists. The reply stays sending; a new send of the
// same reply checks for the post first.
type UnknownError struct{ Err error }

func (e *UnknownError) Error() string {
	return fmt.Sprintf("the result is not known (%v). Send again: i2a checks for the post first and does not post it two times", e.Err)
}

func (e *UnknownError) Unwrap() error { return e.Err }

// CheckText removes hidden characters and the spaces at the start and the end.
// It returns an error for empty text and for more than MaxText characters.
func CheckText(s string) (string, error) {
	v, _ := render.Sanitize(s, 0)
	v = strings.TrimSpace(v)
	switch n := utf8.RuneCountInString(v); {
	case n == 0:
		return "", ErrEmpty
	case n > MaxText:
		return "", fmt.Errorf("the text has %d characters; the maximum is %d", n, MaxText)
	}
	return v, nil
}

// DefaultTarget returns the newest message.
func DefaultTarget(msgs []inbox.Message) (inbox.Message, bool) {
	if len(msgs) == 0 {
		return inbox.Message{}, false
	}
	best := msgs[0]
	for _, m := range msgs[1:] {
		if m.CreatedAt.After(best.CreatedAt) || (m.CreatedAt.Equal(best.CreatedAt) && m.ID > best.ID) {
			best = m
		}
	}
	return best, true
}

// DefaultPlace returns the place of a reply to the message: its thread when
// it is in a thread or in a channel, and the channel for a top-level message
// of a direct or group conversation.
func DefaultPlace(m inbox.Message) string {
	if m.ThreadID != "" || m.ChannelKind == inbox.KindChannel {
		return inbox.PlaceThread
	}
	return inbox.PlaceChannel
}

// answered reports whether the platform answered, so that no post exists.
func answered(err error) bool {
	var a interface{ Answered() bool }
	return errors.As(err, &a) && a.Answered()
}

// Send sends the reply and records it. It returns the recorded reply and the target message.
func Send(ctx context.Context, st Store, build Builder, req Request, now func() time.Time) (inbox.Reply, inbox.Message, error) {
	text, err := CheckText(req.Text)
	if err != nil {
		return inbox.Reply{}, inbox.Message{}, err
	}
	if req.Place != inbox.PlaceThread && req.Place != inbox.PlaceChannel {
		return inbox.Reply{}, inbox.Message{}, fmt.Errorf("place %q is not thread or channel", req.Place)
	}
	msgs, err := st.MessagesByID(ctx, []int64{req.MessageID})
	if err != nil {
		return inbox.Reply{}, inbox.Message{}, err
	}
	if len(msgs) == 0 {
		return inbox.Reply{}, inbox.Message{}, fmt.Errorf("the store has no message %d", req.MessageID)
	}
	target := msgs[0]
	rp, err := build(target.SourceID)
	if err != nil {
		return inbox.Reply{}, target, err
	}

	row, found, err := st.FindReply(ctx, target.ID, req.Place, text)
	if err != nil {
		return inbox.Reply{}, target, err
	}
	if found && row.State == inbox.ReplySent {
		return row, target, &AlreadySentError{At: row.SentAt}
	}
	if !found {
		t := now()
		row = inbox.Reply{MessageID: target.ID, CardID: req.CardID, SourceID: target.SourceID, ChannelName: target.ChannelName,
			Place: req.Place, Text: text, By: req.By, State: inbox.ReplySending,
			PendingID: strconv.FormatInt(t.UnixMilli(), 10), CreatedAt: t}
		if row.ID, err = st.StartReply(ctx, row); err != nil {
			return inbox.Reply{}, target, err
		}
	}
	r := inbox.ReplyRequest{Target: target, Text: text, Thread: req.Place == inbox.PlaceThread, PendingID: row.PendingID,
		Since: row.CreatedAt}

	var posted inbox.Posted
	exists := false
	if found && row.State == inbox.ReplySending {
		// The last try did not get an answer. The post can exist.
		if posted, exists, err = rp.Find(ctx, r); err != nil {
			return row, target, &UnknownError{Err: err}
		}
	}
	if !exists {
		posted, err = rp.Reply(ctx, r)
		if err != nil {
			state := inbox.ReplySending
			if answered(err) {
				state = inbox.ReplyFailed
			} else {
				err = &UnknownError{Err: err}
			}
			row.State, row.Error = state, err.Error()
			if ferr := st.FinishReply(ctx, row.ID, state, "", "", err.Error(), now()); ferr != nil {
				return row, target, errors.Join(err, ferr)
			}
			return row, target, err
		}
	}
	t := now()
	if err := st.FinishReply(ctx, row.ID, inbox.ReplySent, posted.ID, posted.URL, "", t); err != nil {
		return row, target, fmt.Errorf("the reply is sent as post %s, but i2a cannot record it: %w", posted.ID, err)
	}
	row.State, row.ExternalID, row.URL, row.Error, row.SentAt = inbox.ReplySent, posted.ID, posted.URL, "", t.UTC()
	if req.CardID != 0 {
		if err := st.SetCardState(ctx, req.CardID, "", inbox.CardDone, t); err != nil {
			return row, target, fmt.Errorf("the reply is sent, but the card is not done: %w", err)
		}
	}
	return row, target, nil
}

// Where describes the place of a sent reply for a person, for example
// "pr-reviews (thread)".
func Where(channel, place string) string {
	return channel + " (" + place + ")"
}
