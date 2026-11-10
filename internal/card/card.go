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
	"time"

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

// SessionEnv names the variable in which Claude Code gives each command the
// ID of its current session. /clear and /resume change it.
const SessionEnv = "CLAUDE_CODE_SESSION_ID"

var sessionRe = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// AgentSession returns the Claude session ID in v, or "" and a warning when
// v is set and is not a UUID. An ask about the card goes to its terminal
// only while that terminal runs this session.
func AgentSession(v string) (string, string) {
	switch {
	case v == "":
		return "", ""
	case sessionRe.MatchString(v):
		return v, ""
	}
	return "", fmt.Sprintf("%s is not a Claude session ID; the cards have no Claude session", SessionEnv)
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

// Context gives Plan the facts that are not in a card.
type Context struct {
	Refs  map[int64]inbox.Message   // the ref messages of the cards
	Space func(c inbox.Card) string // the space name of a card; nil or "" when unknown
	Apps  notify.Apps               // installed apps for the click target; nil is notify.InstalledApp
}

// SpaceOf returns the space ID of a card: the space of the session of the
// terminal that added it, else the space of its newest ref message that was
// sent, else the first space that routes its newest ref message. valid
// reports whether a space exists; route gives the route of a conversation.
func SpaceOf(c inbox.Card, refs map[int64]inbox.Message, valid func(string) bool, route func(source, channel string) string) string {
	if c.Space != "" && valid(c.Space) {
		return c.Space
	}
	var newest, sent inbox.Message
	for _, id := range c.Refs {
		msg, ok := refs[id]
		if !ok {
			continue
		}
		if msg.CreatedAt.After(newest.CreatedAt) {
			newest = msg
		}
		if msg.Space != "" && msg.CreatedAt.After(sent.CreatedAt) {
			sent = msg
		}
	}
	switch {
	case sent.Space != "":
		return sent.Space
	case newest.SourceID != "":
		return route(newest.SourceID, newest.ChannelID)
	}
	return ""
}

// clickOf returns the click target of a card: the source of its newest ref
// message with a URL, or the WhatsApp app for a WhatsApp message with no URL.
func (cx Context) clickOf(c inbox.Card) (open, activate string) {
	var newest, whatsapp inbox.Message
	for _, id := range c.Refs {
		m, ok := cx.Refs[id]
		if !ok {
			continue
		}
		if m.URL != "" && m.CreatedAt.After(newest.CreatedAt) {
			newest = m
		}
		if m.SourceType == "whatsapp" && m.CreatedAt.After(whatsapp.CreatedAt) {
			whatsapp = m
		}
	}
	switch {
	case newest.URL != "":
		return notify.Click(newest.SourceType, newest.URL, cx.Apps)
	case whatsapp.SourceType != "":
		return notify.Click("whatsapp", "", cx.Apps)
	}
	return "", ""
}

func (cx Context) single(c inbox.SavedCard) notify.Note {
	title := fmt.Sprintf("%s · %s", strings.ToUpper(c.Category), c.Urgency)
	if c.Result == inbox.SaveUpdated {
		title += " · updated"
	}
	var sub []string
	if c.Group != "" {
		sub = append(sub, c.Group)
	}
	if cx.Space != nil {
		if s := cx.Space(c.Card); s != "" {
			sub = append(sub, s)
		}
	}
	n := notify.Note{Title: title, Subtitle: strings.Join(sub, " · "), Body: render.PlainText(c.Title)}
	if c.ID != 0 {
		n.Group = fmt.Sprintf("i2a-card-%d", c.ID)
	}
	n.Open, n.Activate = cx.clickOf(c.Card)
	return n
}

// Plan returns the notifications for the cards of one call.
func Plan(saved []inbox.SavedCard, minUrgency string, cx Context) []notify.Note {
	min := inbox.UrgencyRank(minUrgency)
	var notes []notify.Note
	var group []inbox.SavedCard
	for _, c := range saved {
		if !counts(c) || inbox.UrgencyRank(c.Urgency) < min {
			continue
		}
		if c.Urgency == inbox.UrgencyHigh && len(notes) < MaxHighNotes {
			notes = append(notes, cx.single(c))
			continue
		}
		group = append(group, c)
	}
	switch {
	case len(group) == 1:
		notes = append(notes, cx.single(group[0]))
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
		notes = append(notes, notify.Note{Title: fmt.Sprintf("i2a: %d new cards", len(group)), Body: strings.Join(parts, ", "), Group: "i2a-cards"})
	}
	return notes
}

// Diff compares the cards of a new load of the Feed with the cards of the
// last load (last, by ID). It returns the saved results for Plan and the
// newest UpdatedAt for the next call. Only an agent save changes UpdatedAt,
// so a card counts only when its UpdatedAt is after since: a card that shows
// because the user shows the done cards gives no note. A changed card that
// the last load did not have was done (the Feed hides done cards). The first
// load passes a zero since and gets no result.
func Diff(last map[int64]inbox.Card, since time.Time, cards []inbox.Card) ([]inbox.SavedCard, time.Time) {
	newest := since
	for _, c := range cards {
		if c.UpdatedAt.After(newest) {
			newest = c.UpdatedAt
		}
	}
	if since.IsZero() {
		return nil, newest
	}
	var out []inbox.SavedCard
	for _, c := range cards {
		if !c.UpdatedAt.After(since) {
			continue
		}
		sv := inbox.SavedCard{Card: c, Result: inbox.SaveAdded}
		if old, ok := last[c.ID]; ok {
			sv.Result, sv.PrevUrgency, sv.PrevState = inbox.SaveUpdated, old.Urgency, old.State
		} else if c.CreatedAt.Before(c.UpdatedAt) && !c.CreatedAt.After(since) {
			sv.Result, sv.PrevUrgency, sv.PrevState = inbox.SaveUpdated, c.Urgency, inbox.CardDone
		}
		out = append(out, sv)
	}
	return out, newest
}
