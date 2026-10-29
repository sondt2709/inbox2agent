// Package card reads the cards that an agent sends with "i2a card add" and
// plans the desktop notifications for them. Card text comes from an agent
// that reads untrusted messages, so all text is sanitized like message text.
package card

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"slices"
	"strings"

	"github.com/sondt2709/inbox2agent/internal/inbox"
	"github.com/sondt2709/inbox2agent/internal/notify"
	"github.com/sondt2709/inbox2agent/internal/render"
)

// Limits of one "i2a card add" call.
const (
	MaxInputBytes = 1 << 20
	MaxCards      = 50
	MaxRefs       = 50
	MaxTitle      = 200
	MaxGroup      = 100
	MaxKey        = 200
	MaxBody       = 4000
	MaxReply      = 2000
)

// HandleEnv names the variable in which Orca gives the agent terminal its handle.
const HandleEnv = "ORCA_TERMINAL_HANDLE"

var handleRe = regexp.MustCompile(`^term_[A-Za-z0-9-]{1,64}$`)

// Handle returns the terminal handle in v, or "" and a warning when v is set
// and is not a handle. An ask about the card goes back to this terminal.
func Handle(v string) (string, string) {
	switch {
	case v == "":
		return "", ""
	case handleRe.MatchString(v):
		return v, ""
	}
	return "", fmt.Sprintf("%s is not a terminal handle; the cards have no terminal", HandleEnv)
}

// Input is one card as the agent writes it.
type Input struct {
	Category string  `json:"category"`
	Urgency  string  `json:"urgency"`
	Group    string  `json:"group"`
	Title    string  `json:"title"`
	Body     string  `json:"body"`
	Reply    string  `json:"reply"`
	Refs     []int64 `json:"refs"`
	Key      string  `json:"key"`
}

// Parse reads a JSON array of cards, or one card, from r. It rejects unknown
// fields, more than MaxCards cards and more than MaxInputBytes bytes.
func Parse(r io.Reader) ([]Input, error) {
	b, err := io.ReadAll(io.LimitReader(r, MaxInputBytes+1))
	if err != nil {
		return nil, err
	}
	if len(b) > MaxInputBytes {
		return nil, fmt.Errorf("the input is larger than %d bytes", MaxInputBytes)
	}
	b = bytes.TrimSpace(b)
	if len(b) == 0 {
		return nil, errors.New("no input: write a JSON array of cards to stdin")
	}
	var raw []json.RawMessage
	if b[0] == '[' {
		if err := json.Unmarshal(b, &raw); err != nil {
			return nil, fmt.Errorf("the input is not a JSON array of cards: %w", err)
		}
	} else {
		raw = []json.RawMessage{b}
	}
	if len(raw) == 0 {
		return nil, errors.New("the array has no card")
	}
	if len(raw) > MaxCards {
		return nil, fmt.Errorf("%d cards in one call; the maximum is %d", len(raw), MaxCards)
	}
	out := make([]Input, len(raw))
	for i, one := range raw {
		dec := json.NewDecoder(bytes.NewReader(one))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&out[i]); err != nil {
			return nil, fmt.Errorf("card %d: %w", i+1, err)
		}
	}
	return out, nil
}

// Known reports which of the message IDs exist in the store.
type Known func(ids []int64) (map[int64]bool, error)

// Prepare validates and sanitizes the cards. A card that is not valid makes
// the whole call fail. Text that is too long is cut, and each cut and each
// unknown message reference gives a warning.
func Prepare(in []Input, known Known) ([]inbox.Card, []string, error) {
	var warnings []string
	var all []int64
	for _, c := range in {
		all = append(all, c.Refs...)
	}
	exists := map[int64]bool{}
	if len(all) > 0 && known != nil {
		var err error
		if exists, err = known(all); err != nil {
			return nil, nil, err
		}
	}
	out := make([]inbox.Card, 0, len(in))
	for i, c := range in {
		n := i + 1
		cut := func(field, s string, max int, oneLine bool) string {
			v, truncated := render.Sanitize(s, max)
			if oneLine {
				v = strings.Join(strings.Fields(v), " ")
			} else {
				v = strings.TrimSpace(v)
			}
			if truncated {
				warnings = append(warnings, fmt.Sprintf("card %d: %s cut to %d characters", n, field, max))
			}
			return v
		}
		if !slices.Contains(inbox.Categories, c.Category) {
			return nil, nil, fmt.Errorf("card %d: category %q is not action, question or info", n, c.Category)
		}
		if c.Urgency == "" {
			c.Urgency = inbox.UrgencyNormal
		}
		if !slices.Contains(inbox.Urgencies, c.Urgency) {
			return nil, nil, fmt.Errorf("card %d: urgency %q is not high, normal or low", n, c.Urgency)
		}
		if len(c.Refs) > MaxRefs {
			return nil, nil, fmt.Errorf("card %d: %d refs; the maximum is %d", n, len(c.Refs), MaxRefs)
		}
		card := inbox.Card{
			Category: c.Category,
			Urgency:  c.Urgency,
			Title:    cut("title", c.Title, MaxTitle, true),
			Group:    cut("group", c.Group, MaxGroup, true),
			Key:      cut("key", c.Key, MaxKey, true),
			Body:     cut("body", c.Body, MaxBody, false),
			Reply:    cut("reply", c.Reply, MaxReply, false),
		}
		if card.Title == "" {
			return nil, nil, fmt.Errorf("card %d: the title is empty", n)
		}
		for _, id := range c.Refs {
			switch {
			case !exists[id]:
				warnings = append(warnings, fmt.Sprintf("card %d: message %d is not in the store; the reference is dropped", n, id))
			case !slices.Contains(card.Refs, id):
				card.Refs = append(card.Refs, id)
			}
		}
		out = append(out, card)
	}
	return out, warnings, nil
}

// Note is one desktop notification.
type Note struct {
	Title string
	Text  string
}

// MaxHighNotes is the number of single notifications for high cards in one call.
// More high cards go into the grouped notification, so an agent that a message
// tricks into many urgent cards cannot flood the screen.
const MaxHighNotes = 3

// counts reports whether a saved card gives a notification: it is new, or its
// urgency went up, or it came back from done.
func counts(c inbox.SavedCard) bool {
	switch c.Result {
	case inbox.SaveAdded:
		return true
	case inbox.SaveUpdated:
		return inbox.UrgencyRank(c.Urgency) > inbox.UrgencyRank(c.PrevUrgency) || c.PrevState == inbox.CardDone
	}
	return false
}

func single(c inbox.SavedCard) Note {
	title := fmt.Sprintf("i2a · %s · %s", strings.ToUpper(c.Category), c.Urgency)
	if c.Result == inbox.SaveUpdated {
		title += " (updated)"
	}
	text := c.Title
	if c.Group != "" {
		text = c.Group + ": " + c.Title
	}
	return Note{Title: title, Text: notify.Short(text, 200)}
}

// Plan returns the notifications for the cards of one call.
func Plan(saved []inbox.SavedCard, minUrgency string) []Note {
	min := inbox.UrgencyRank(minUrgency)
	var notes []Note
	var group []inbox.SavedCard
	for _, c := range saved {
		if !counts(c) || inbox.UrgencyRank(c.Urgency) < min {
			continue
		}
		if c.Urgency == inbox.UrgencyHigh && len(notes) < MaxHighNotes {
			notes = append(notes, single(c))
			continue
		}
		group = append(group, c)
	}
	switch {
	case len(group) == 1:
		notes = append(notes, single(group[0]))
	case len(group) > 0:
		n := map[string]int{}
		for _, c := range group {
			n[c.Category]++
		}
		var parts []string
		for _, cat := range inbox.Categories {
			if n[cat] > 0 {
				parts = append(parts, fmt.Sprintf("%d %s", n[cat], cat))
			}
		}
		notes = append(notes, Note{Title: fmt.Sprintf("i2a: %d new cards", len(group)), Text: strings.Join(parts, ", ")})
	}
	return notes
}
