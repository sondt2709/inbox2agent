package store

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"github.com/sondt2709/inbox2agent/internal/inbox"
)

const askCols = `id, card_id, profile, handle, text, state, detail, created_at, sent_at`

func scanAsk(sc interface{ Scan(...any) error }) (inbox.Ask, error) {
	var a inbox.Ask
	var sent sql.NullInt64
	var created int64
	if err := sc.Scan(&a.ID, &a.CardID, &a.Profile, &a.Handle, &a.Text, &a.State, &a.Detail, &created, &sent); err != nil {
		return a, err
	}
	a.CreatedAt = time.UnixMilli(created).UTC()
	if sent.Valid {
		a.SentAt = time.UnixMilli(sent.Int64).UTC()
	}
	return a, nil
}

func scanAsks(rows *sql.Rows) ([]inbox.Ask, error) {
	defer rows.Close()
	var out []inbox.Ask
	for rows.Next() {
		a, err := scanAsk(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// StartAsk records a new ask with the state queued and returns its ID.
func (s *Store) StartAsk(ctx context.Context, a inbox.Ask) (int64, error) {
	res, err := s.db.ExecContext(ctx, `INSERT INTO asks(card_id, profile, text, state, detail, created_at)
	  VALUES (?, ?, ?, 'queued', ?, ?)`, a.CardID, a.Profile, a.Text, a.Detail, a.CreatedAt.UnixMilli())
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// FinishAsk sets the state and the detail of an ask. For sent, it also sets
// the terminal that got the ask and the send time.
func (s *Store) FinishAsk(ctx context.Context, id int64, state, handle, detail string, now time.Time) error {
	var sent any
	if state == inbox.AskSent {
		sent = now.UnixMilli()
	}
	_, err := s.db.ExecContext(ctx, `UPDATE asks SET state = ?, handle = ?, detail = ?, sent_at = ? WHERE id = ?`,
		state, handle, detail, sent, id)
	return err
}

// QueuedAsks returns the queued asks, oldest first.
func (s *Store) QueuedAsks(ctx context.Context) ([]inbox.Ask, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+askCols+` FROM asks WHERE state = 'queued' ORDER BY created_at, id`)
	if err != nil {
		return nil, err
	}
	return scanAsks(rows)
}

// Asks returns the asks of the cards, oldest first for each card.
func (s *Store) Asks(ctx context.Context, cardIDs []int64) (map[int64][]inbox.Ask, error) {
	out := map[int64][]inbox.Ask{}
	if len(cardIDs) == 0 {
		return out, nil
	}
	args := make([]any, len(cardIDs))
	for i, id := range cardIDs {
		args[i] = id
	}
	rows, err := s.db.QueryContext(ctx, `SELECT `+askCols+` FROM asks WHERE card_id IN (`+
		strings.TrimSuffix(strings.Repeat("?,", len(cardIDs)), ",")+`) ORDER BY created_at, id`, args...)
	if err != nil {
		return nil, err
	}
	asks, err := scanAsks(rows)
	if err != nil {
		return nil, err
	}
	for _, a := range asks {
		out[a.CardID] = append(out[a.CardID], a)
	}
	return out, nil
}
