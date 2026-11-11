// Package render turns messages into the prompt of a batch or an ask: the
// instruction, the YAML content and the i2a CLI instruction. It also writes
// the digest file.
package render

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/sondt2709/inbox2agent/internal/inbox"
)

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

// replyMax limits the text of the earlier message in reply_to.
const replyMax = 200

func reply(r *inbox.ReplyRef) *Reply {
	if r == nil {
		return nil
	}
	text, cut := Sanitize(strings.Join(strings.Fields(r.Text), " "), replyMax)
	if cut {
		text += "…"
	}
	return &Reply{ID: r.ID, From: safe(r.Author), Me: r.Me, Text: FenceSafe(text)}
}

func field(s string) string {
	v, _ := Sanitize(s, maxFieldChars)
	return strings.ReplaceAll(v, "\n", " ")
}

// RuneCount is a helper for tests and callers that check limits.
func RuneCount(s string) int { return utf8.RuneCountInString(s) }
