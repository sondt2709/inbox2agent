package render

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/sondt2709/inbox2agent/internal/config"
	"github.com/sondt2709/inbox2agent/internal/inbox"
)

// Batch holds the facts of a batch for the driver and the dispatch record.
type Batch struct {
	BatchID    string
	Space      string // the name of the space
	Agent      string // the ID of the agent
	Count      int
	Accounts   string
	DigestPath string
}

// Rendered is a batch or an ask that is ready for a driver.
type Rendered struct {
	Batch
	Items   []Item
	Content string // the YAML content
	Prompt  string // the text that the driver sends
	Detail  string // a note for the dispatch record, for example the inline fallback
}

// BatchID returns a readable unique ID: <UTC time>-<space ID>-<dispatch ID>.
func BatchID(now time.Time, space string, dispatchID int64) string {
	return fmt.Sprintf("%s-%s-%d", now.UTC().Format("20060102-150405"), space, dispatchID)
}

// Instruction returns the instruction of a space, or DefaultInstruction.
func Instruction(s string) string {
	if strings.TrimSpace(s) == "" {
		return DefaultInstruction
	}
	return s
}

// Render sanitizes the messages, writes the digest file and renders the
// prompt. instruction and space are the instruction and the name of the space
// of the batch. cardCommand is the command of the CLI instruction; ""
// (dry-run) leaves that part out.
func Render(p config.Agent, instruction, space, id string, msgs []inbox.Message, digestDir, cardCommand string) (*Rendered, error) {
	if len(msgs) == 0 {
		return nil, fmt.Errorf("render: no messages")
	}
	items := Items(msgs, p.MessageMaxChars)
	b := Batch{
		BatchID:    id,
		Space:      space,
		Agent:      p.ID,
		Count:      len(items),
		Accounts:   accounts(msgs),
		DigestPath: filepath.Join(digestDir, id+".md"),
	}
	content, err := Encode(Content{Batch: FenceSafe(id), Space: safe(space), Conversations: group(items)})
	if err != nil {
		return nil, err
	}
	cli := ""
	if cardCommand != "" {
		cli = CLIInstruction(cardCommand)
	}
	full, err := Prompt(Instruction(instruction), content, cli)
	if err != nil {
		return nil, fmt.Errorf("space %q: %w", p.ID, err)
	}
	if err := os.MkdirAll(digestDir, 0o700); err != nil {
		return nil, err
	}
	if err := os.WriteFile(b.DigestPath, []byte(full), 0o600); err != nil {
		return nil, fmt.Errorf("write digest: %w", err)
	}

	r := &Rendered{Batch: b, Items: items, Content: content, Prompt: PointerPrompt(b.DigestPath, id)}
	if p.Payload == config.PayloadInline {
		switch {
		case len(content) > p.InlineMaxBytes:
			r.Detail = fmt.Sprintf("inline too large, sent pointer (content %d bytes > %d)", len(content), p.InlineMaxBytes)
		case len(full) > config.MaxInlineBytes:
			r.Detail = fmt.Sprintf("inline too large, sent pointer (prompt %d bytes > %d)", len(full), config.MaxInlineBytes)
		default:
			r.Prompt = full
		}
	}
	return r, nil
}

func accounts(msgs []inbox.Message) string {
	seen := map[string]bool{}
	var out []string
	for _, m := range msgs {
		k := fmt.Sprintf("%s (%s)", field(m.SourceID), field(m.SourceType))
		if !seen[k] {
			seen[k] = true
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return strings.Join(out, ", ")
}
