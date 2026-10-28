// Package render turns messages into a safe data block, a digest file and agent prompts.
package render

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/sondt2709/inbox2agent/internal/inbox"
)

// FencePrefix starts the label of each data block.
const FencePrefix = "i2a-data-"

// maxFieldChars limits the short fields (author, channel, source).
const maxFieldChars = 200

// hidden reports whether a rune must be removed: bidi controls, zero-width
// characters, and C0 and C1 control characters except newline and tab.
func hidden(r rune) bool {
	switch {
	case r == '\n' || r == '\t':
		return false
	case r >= 0x202A && r <= 0x202E, r >= 0x2066 && r <= 0x2069:
		return true // bidi embedding, override and isolate controls
	case r >= 0x200B && r <= 0x200F, r == 0x2060, r == 0xFEFF:
		return true // zero-width characters and marks
	case r == 0x061C:
		return true // Arabic letter mark
	case unicode.IsControl(r):
		return true // C0, DEL and C1
	}
	return false
}

// Sanitize removes hidden characters, normalizes line ends and truncates the
// text to maxChars characters. It reports whether it truncated the text.
func Sanitize(s string, maxChars int) (string, bool) {
	s = strings.ToValidUTF8(s, "\uFFFD")
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	var b strings.Builder
	b.Grow(len(s))
	n := 0
	truncated := false
	for _, r := range s {
		if hidden(r) {
			continue
		}
		if maxChars > 0 && n >= maxChars {
			truncated = true
			break
		}
		b.WriteRune(r)
		n++
	}
	return b.String(), truncated
}

// Item is one message in a data block. Each item is one JSON line.
type Item struct {
	N         int    `json:"n"`
	Source    string `json:"source"`
	SourceID  string `json:"source_id"`
	Channel   string `json:"channel"`
	Kind      string `json:"kind"`
	Author    string `json:"author"`
	Time      string `json:"time"`
	URL       string `json:"url"`
	Text      string `json:"text"`
	Truncated bool   `json:"truncated"`
}

func field(s string) string {
	v, _ := Sanitize(s, maxFieldChars)
	return strings.ReplaceAll(v, "\n", " ")
}

// Items converts messages to sanitized items.
func Items(msgs []inbox.Message, maxChars int) []Item {
	out := make([]Item, 0, len(msgs))
	for i, m := range msgs {
		text, cut := Sanitize(m.Text, maxChars)
		out = append(out, Item{
			N: i + 1, Source: field(m.SourceType), SourceID: field(m.SourceID), Channel: field(m.ChannelName),
			Kind: field(m.ChannelKind), Author: field(m.Author), Time: m.CreatedAt.UTC().Format(time.RFC3339),
			URL: field(m.URL), Text: text, Truncated: cut,
		})
	}
	return out
}

// NewNonce returns 12 hexadecimal characters from a cryptographic random source.
func NewNonce() string {
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		panic("crypto/rand failed: " + err.Error())
	}
	return hex.EncodeToString(b)
}

// Block returns the fenced data block. Each line between the fences is one
// JSON object, so each line starts with "{" and a message cannot close the fence.
func Block(items []Item, nonce string) string {
	var b bytes.Buffer
	b.WriteString("```" + FencePrefix + nonce + "\n")
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	for _, it := range items {
		_ = enc.Encode(it) // Encode writes one line and a newline; Item always encodes
	}
	b.WriteString("```")
	return b.String()
}

// RuneCount is a helper for tests and callers that check limits.
func RuneCount(s string) int { return utf8.RuneCountInString(s) }
