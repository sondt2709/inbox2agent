package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/sondt2709/inbox2agent/internal/inbox"
)

const replyCols = `id, message_id, card_id, source_id, channel_name, place, text, by, state,
  pending_id, external_id, url, error, created_at, sent_at`

func scanReply(sc interface{ Scan(...any) error }) (inbox.Reply, error) {
	var r inbox.Reply
	var card, sent sql.NullInt64
	var created int64
	if err := sc.Scan(&r.ID, &r.MessageID, &card, &r.SourceID, &r.ChannelName, &r.Place, &r.Text, &r.By,
		&r.State, &r.PendingID, &r.ExternalID, &r.URL, &r.Error, &created, &sent); err != nil {
		return r, err
	}
	r.CardID = card.Int64
	r.CreatedAt = time.UnixMilli(created).UTC()
	if sent.Valid {
		r.SentAt = time.UnixMilli(sent.Int64).UTC()
	}
	return r, nil
}

// StartReply records a new reply with the state sending and returns its ID.
func (s *Store) StartReply(ctx context.Context, r inbox.Reply) (int64, error) {
	var card any
	if r.CardID != 0 {
		card = r.CardID
	}
	res, err := s.db.ExecContext(ctx, `INSERT INTO replies(message_id, card_id, source_id, channel_name, place,
	  text, by, state, pending_id, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, 'sending', ?, ?)`,
		r.MessageID, card, r.SourceID, r.ChannelName, r.Place, r.Text, r.By, r.PendingID, r.CreatedAt.UnixMilli())
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// FinishReply sets the state of a reply. For sent, it also sets the post ID,
// the URL and the send time. For sending, it clears the error of a failed try.
func (s *Store) FinishReply(ctx context.Context, id int64, state, externalID, url, errText string, now time.Time) error {
	var sent any
	if state == inbox.ReplySent {
		sent = now.UnixMilli()
	}
	_, err := s.db.ExecContext(ctx, `UPDATE replies SET state = ?, external_id = ?, url = ?, error = ?, sent_at = ? WHERE id = ?`,
		state, externalID, url, errText, sent, id)
	return err
}

// FindReply returns the newest reply to the message with the same place and text.
func (s *Store) FindReply(ctx context.Context, messageID int64, place, text string) (inbox.Reply, bool, error) {
	r, err := scanReply(s.db.QueryRowContext(ctx, `SELECT `+replyCols+` FROM replies
	  WHERE message_id = ? AND place = ? AND text = ? ORDER BY id DESC LIMIT 1`, messageID, place, text))
	if errors.Is(err, sql.ErrNoRows) {
		return r, false, nil
	}
	return r, err == nil, err
}

// SentReplies returns the sent replies of the cards, oldest first, by card ID.
func (s *Store) SentReplies(ctx context.Context, cardIDs []int64) (map[int64][]inbox.Reply, error) {
	out := map[int64][]inbox.Reply{}
	if len(cardIDs) == 0 {
		return out, nil
	}
	args := make([]any, len(cardIDs))
	for i, id := range cardIDs {
		args[i] = id
	}
	rows, err := s.db.QueryContext(ctx, `SELECT `+replyCols+` FROM replies WHERE state = 'sent' AND card_id IN (`+
		strings.TrimSuffix(strings.Repeat("?,", len(cardIDs)), ",")+`) ORDER BY sent_at, id`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		r, err := scanReply(rows)
		if err != nil {
			return nil, err
		}
		out[r.CardID] = append(out[r.CardID], r)
	}
	return out, rows.Err()
}

// Card returns the card with the ID, or with the key when id is 0.
// It returns ErrNoCard when no card matches.
func (s *Store) Card(ctx context.Context, id int64, key string) (inbox.Card, error) {
	where, arg := `id = ?`, any(id)
	if id == 0 {
		where, arg = `key = ?`, key
	}
	c, err := scanCard(s.db.QueryRowContext(ctx, `SELECT `+cardCols+` FROM cards WHERE `+where, arg))
	if errors.Is(err, sql.ErrNoRows) {
		return c, ErrNoCard
	}
	return c, err
}
