package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/sondt2709/inbox2agent/internal/inbox"
)

// ErrNoCard is returned when no card matches an ID or a key.
var ErrNoCard = errors.New("no card")

const cardCols = `id, key, category, urgency, grp, title, body, reply, refs, state, created_at, updated_at, handle, space`

func scanCard(sc interface{ Scan(...any) error }) (inbox.Card, error) {
	var c inbox.Card
	var key sql.NullString
	var refs string
	var created, updated int64
	if err := sc.Scan(&c.ID, &key, &c.Category, &c.Urgency, &c.Group, &c.Title, &c.Body, &c.Reply,
		&refs, &c.State, &created, &updated, &c.Handle, &c.Space); err != nil {
		return c, err
	}
	c.Key = key.String
	if err := json.Unmarshal([]byte(refs), &c.Refs); err != nil {
		return c, fmt.Errorf("card %d: refs: %w", c.ID, err)
	}
	c.CreatedAt = time.UnixMilli(created).UTC()
	c.UpdatedAt = time.UnixMilli(updated).UTC()
	return c, nil
}

func refsJSON(refs []int64) string {
	if refs == nil {
		refs = []int64{}
	}
	b, _ := json.Marshal(refs)
	return string(b)
}

func sameContent(a, b inbox.Card) bool {
	return a.Category == b.Category && a.Urgency == b.Urgency && a.Group == b.Group && a.Title == b.Title &&
		a.Body == b.Body && a.Reply == b.Reply && slices.Equal(a.Refs, b.Refs)
}

// SaveCards saves the cards in one transaction. A card with a key that the
// store has updates that card and makes it new again, unless all its fields
// are the same. A card without a key is always added. The handle is not a
// field of the agent: a new handle replaces the stored one, also for an
// unchanged card, and an empty handle keeps it.
func (s *Store) SaveCards(ctx context.Context, cards []inbox.Card, now time.Time) ([]inbox.SavedCard, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback() //nolint:errcheck // no effect after Commit
	out := make([]inbox.SavedCard, 0, len(cards))
	for _, c := range cards {
		// The space of the card is the space of the session of its terminal.
		c.Space = ""
		if c.Handle != "" {
			err := tx.QueryRowContext(ctx, `SELECT space FROM sessions WHERE handle = ?`, c.Handle).Scan(&c.Space)
			if err != nil && !errors.Is(err, sql.ErrNoRows) {
				return nil, err
			}
		}
		sv := inbox.SavedCard{Card: c}
		var old inbox.Card
		found := false
		if c.Key != "" {
			old, err = scanCard(tx.QueryRowContext(ctx, `SELECT `+cardCols+` FROM cards WHERE key = ?`, c.Key))
			switch {
			case err == nil:
				found = true
			case !errors.Is(err, sql.ErrNoRows):
				return nil, err
			}
		}
		switch {
		case !found:
			var key any
			if c.Key != "" {
				key = c.Key
			}
			res, err := tx.ExecContext(ctx, `INSERT INTO cards(key, category, urgency, grp, title, body, reply, refs, state, created_at, updated_at, handle, space)
			  VALUES (?, ?, ?, ?, ?, ?, ?, ?, 'new', ?, ?, ?, ?)`, key, c.Category, c.Urgency, c.Group, c.Title, c.Body, c.Reply,
				refsJSON(c.Refs), now.UnixMilli(), now.UnixMilli(), c.Handle, c.Space)
			if err != nil {
				return nil, err
			}
			sv.ID, _ = res.LastInsertId()
			sv.Result, sv.State, sv.CreatedAt, sv.UpdatedAt = inbox.SaveAdded, inbox.CardNew, now.UTC(), now.UTC()
		case sameContent(old, c):
			if c.Handle != "" && c.Handle != old.Handle {
				if _, err := tx.ExecContext(ctx, `UPDATE cards SET handle = ? WHERE id = ?`, c.Handle, old.ID); err != nil {
					return nil, err
				}
				old.Handle = c.Handle
			}
			if c.Space != "" && c.Space != old.Space {
				if _, err := tx.ExecContext(ctx, `UPDATE cards SET space = ? WHERE id = ?`, c.Space, old.ID); err != nil {
					return nil, err
				}
				old.Space = c.Space
			}
			sv.Card = old
			sv.Result, sv.PrevUrgency, sv.PrevState = inbox.SaveUnchanged, old.Urgency, old.State
		default:
			if _, err := tx.ExecContext(ctx, `UPDATE cards SET category = ?, urgency = ?, grp = ?, title = ?, body = ?, reply = ?,
			  refs = ?, state = 'new', updated_at = ?, done_at = NULL, handle = CASE WHEN ? = '' THEN handle ELSE ? END,
			  space = CASE WHEN ? = '' THEN space ELSE ? END
			  WHERE id = ?`, c.Category, c.Urgency, c.Group, c.Title, c.Body, c.Reply, refsJSON(c.Refs), now.UnixMilli(),
				c.Handle, c.Handle, c.Space, c.Space, old.ID); err != nil {
				return nil, err
			}
			if c.Handle == "" {
				sv.Handle = old.Handle
			}
			if c.Space == "" {
				sv.Space = old.Space
			}
			sv.ID, sv.State, sv.CreatedAt, sv.UpdatedAt = old.ID, inbox.CardNew, old.CreatedAt, now.UTC()
			sv.Result, sv.PrevUrgency, sv.PrevState = inbox.SaveUpdated, old.Urgency, old.State
		}
		out = append(out, sv)
	}
	return out, tx.Commit()
}

// Cards returns the new and read cards, or all cards when all is true, newest update first.
func (s *Store) Cards(ctx context.Context, all bool) ([]inbox.Card, error) {
	q := `SELECT ` + cardCols + ` FROM cards`
	if !all {
		q += ` WHERE state != 'done'`
	}
	rows, err := s.db.QueryContext(ctx, q+` ORDER BY updated_at DESC, id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []inbox.Card
	for rows.Next() {
		c, err := scanCard(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// SetCardState sets the state of the card with the ID, or with the key when
// id is 0. It returns ErrNoCard when no card matches.
func (s *Store) SetCardState(ctx context.Context, id int64, key, state string, now time.Time) error {
	var doneAt any
	if state == inbox.CardDone {
		doneAt = now.UnixMilli()
	}
	where, arg := `id = ?`, any(id)
	if id == 0 {
		where, arg = `key = ?`, key
	}
	res, err := s.db.ExecContext(ctx, `UPDATE cards SET state = ?, done_at = ? WHERE `+where, state, doneAt, arg)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNoCard
	}
	return nil
}

// MessagesByID returns the messages with the IDs that still exist, in the order of the IDs.
func (s *Store) MessagesByID(ctx context.Context, ids []int64) ([]inbox.Message, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	rows, err := s.db.QueryContext(ctx, `SELECT `+messageCols+` FROM messages WHERE id IN (`+
		strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")+`)`, args...)
	if err != nil {
		return nil, err
	}
	msgs, err := scanMessages(rows)
	if err != nil {
		return nil, err
	}
	byID := map[int64]inbox.Message{}
	for _, m := range msgs {
		byID[m.ID] = m
	}
	out := make([]inbox.Message, 0, len(msgs))
	for _, id := range ids {
		if m, ok := byID[id]; ok {
			out = append(out, m)
		}
	}
	return out, nil
}
