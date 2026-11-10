package render

import (
	"strings"
	"testing"

	"github.com/sondt2709/inbox2agent/internal/config"
	"github.com/sondt2709/inbox2agent/internal/inbox"
)

// TestPromptTemplate checks the four parts of an inline prompt, in order.
func TestPromptTemplate(t *testing.T) {
	p := config.NewAgent("triage")
	p.Payload = config.PayloadInline
	p.InlineMaxBytes = config.MaxInlineBytes
	p.Primer = "Summarise in Vietnamese"
	r, err := Render(p, "Triage", "b1", []inbox.Message{m("hello"), m("line one\nline two")}, t.TempDir(), "/usr/local/bin/i2a")
	if err != nil {
		t.Fatal(err)
	}
	parts := []string{
		"This message is sent from inbox2agent (i2a).\n\n",
		"This is the instruction from who setup i2a to send this message to you:\n```\nSummarise in Vietnamese\n```\n\n",
		"This is the i2a content:\n",
		"```yaml\n",
		"\n```\n\nThis is the instruction to interact with i2a via CLI:\n",
		"/usr/local/bin/i2a card add",
	}
	rest := r.Prompt
	for _, part := range parts {
		i := strings.Index(rest, part)
		if i < 0 {
			t.Fatalf("the prompt has no %q after the earlier parts:\n%s", part, r.Prompt)
		}
		rest = rest[i+len(part):]
	}
	if !strings.HasPrefix(r.Prompt, parts[0]) {
		t.Errorf("the prompt does not start with the header:\n%s", r.Prompt)
	}
}
