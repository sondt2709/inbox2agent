// Package phone sends i2a cards to a paired phone as sealed FCM data messages.
//
// A pairing is one FCM topic and one 256-bit key. The QR code of `i2a phone pair`
// gives both to the phone. The Mac compresses each payload with raw deflate and
// seals it with AES-256-GCM, so FCM and Google see only ciphertext.
package phone

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/sondt2709/inbox2agent/internal/inbox"
)

// Version is the version of the payload and of the data field "v".
const Version = 1

// Payload kinds.
const (
	KindCard = "card"
	KindTest = "test"
)

// MaxSealed is the maximum length of the data field "c". FCM limits a data
// message to 4096 bytes, keys and values together.
const MaxSealed = 3600

// MaxRefText is the length of a source message text after the first cut.
const MaxRefText = 140

// MaxRefs is the number of source messages after the second cut.
const MaxRefs = 3

// Payload is the plaintext of one message.
type Payload struct {
	V     int    `json:"v"`
	Kind  string `json:"kind"`
	TS    int64  `json:"ts"`
	Alert bool   `json:"alert"`
	Card  *Card  `json:"card,omitempty"`
}

// Card is a card as the phone gets it.
type Card struct {
	ID      int64  `json:"id"`
	Key     string `json:"key,omitempty"`
	Cat     string `json:"cat"`
	Urg     string `json:"urg"`
	Title   string `json:"title"`
	Group   string `json:"group,omitempty"`
	Body    string `json:"body,omitempty"`
	Reply   string `json:"reply,omitempty"`
	Created int64  `json:"created"`
	State   string `json:"state"`
	Cut     bool   `json:"cut,omitempty"`
	Refs    []Ref  `json:"refs,omitempty"`
}

// Ref is a source message of a card.
type Ref struct {
	Author  string `json:"author,omitempty"`
	Text    string `json:"text,omitempty"`
	URL     string `json:"url,omitempty"`
	Channel string `json:"channel,omitempty"`
	At      int64  `json:"at"`
}

// FromCard makes the phone card of a stored card and its source messages.
// Only http and https URLs go to the phone, as in the Feed tab.
func FromCard(c inbox.Card, msgs []inbox.Message) Card {
	out := Card{
		ID: c.ID, Key: c.Key, Cat: c.Category, Urg: c.Urgency, Title: c.Title, Group: c.Group,
		Body: c.Body, Reply: c.Reply, Created: c.CreatedAt.Unix(), State: c.State,
	}
	for _, m := range msgs {
		url := ""
		if strings.HasPrefix(m.URL, "https://") || strings.HasPrefix(m.URL, "http://") {
			url = m.URL
		}
		out.Refs = append(out.Refs, Ref{Author: m.Author, Text: m.Text, URL: url, Channel: m.ChannelName, At: m.CreatedAt.Unix()})
	}
	return out
}

// SealFit seals the payload. When the sealed text is longer than MaxSealed, it
// cuts the card in this order and sets Cut: each ref text to MaxRefText
// characters, then the refs after the first MaxRefs, then the body, half at a
// time. It changes p.
func SealFit(key []byte, p *Payload) (string, error) {
	steps := []func() bool{
		func() bool {
			changed := false
			for i := range p.Card.Refs {
				if t, ok := cutRunes(p.Card.Refs[i].Text, MaxRefText); ok {
					p.Card.Refs[i].Text, changed = t, true
				}
			}
			return changed
		},
		func() bool {
			if len(p.Card.Refs) > MaxRefs {
				p.Card.Refs = p.Card.Refs[:MaxRefs]
				return true
			}
			return false
		},
	}
	for {
		s, err := sealPayload(key, p)
		if err != nil || len(s) <= MaxSealed {
			return s, err
		}
		if p.Card == nil {
			return "", fmt.Errorf("the sealed message is %d characters; the limit is %d", len(s), MaxSealed)
		}
		cut := false
		for len(steps) > 0 && !cut {
			cut, steps = steps[0](), steps[1:]
		}
		if !cut {
			n := utf8.RuneCountInString(p.Card.Body)
			switch {
			case n > 0:
				p.Card.Body, _ = cutRunes(p.Card.Body, n/2)
			case p.Card.Reply != "":
				p.Card.Reply = ""
			case len(p.Card.Refs) > 0:
				p.Card.Refs = nil
			default:
				return "", fmt.Errorf("the sealed message is %d characters; the limit is %d", len(s), MaxSealed)
			}
		}
		p.Card.Cut = true
	}
}

func sealPayload(key []byte, p *Payload) (string, error) {
	b, err := json.Marshal(p)
	if err != nil {
		return "", err
	}
	return Seal(key, b)
}

// cutRunes returns the first n characters of s and true when s was longer.
func cutRunes(s string, n int) (string, bool) {
	if utf8.RuneCountInString(s) <= n {
		return s, false
	}
	r := []rune(s)
	return string(r[:n]), true
}
