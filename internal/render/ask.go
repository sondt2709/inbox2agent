package render

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/sondt2709/inbox2agent/internal/config"
	"github.com/sondt2709/inbox2agent/internal/inbox"
)

// AskMaxBytes limits the ask prompt. It is the maximum of inline_max_bytes:
// the prompt is a command argument, and the Orca RPC limit is 1 MiB.
const AskMaxBytes = config.MaxInlineBytes

// AskInput is one ask of the user about a card.
type AskInput struct {
	ID       int64
	Card     inbox.Card
	Messages []inbox.Message // the ref messages of the card that still exist
	Text     string          // the checked text of the user
}

// AskID returns a readable unique ID for an ask: <UTC time>-<space ID>-ask<ask ID>.
// In worktree mode, it names the worktree.
func AskID(now time.Time, space string, askID int64) string {
	return fmt.Sprintf("%s-%s-ask%d", now.UTC().Format("20060102-150405"), space, askID)
}

// Ask renders the prompt of an ask with the template of a batch. The question
// of the user follows the instruction of the agent; the card and its messages
// are in the content.
func Ask(p config.Agent, space, id string, in AskInput, cardCommand string) (*Rendered, error) {
	cmd := cardCommand
	if cmd == "" {
		cmd = "i2a"
	}
	text := strings.TrimSpace(in.Text)
	if strings.Contains(text, fence) {
		return nil, ErrFenceInInstruction
	}
	card := &CardData{ID: in.Card.ID, Key: safe(in.Card.Key), Category: safe(in.Card.Category), Urgency: safe(in.Card.Urgency),
		Group: safe(in.Card.Group), Title: safe(in.Card.Title), Refs: in.Card.Refs}
	body, _ := Sanitize(in.Card.Body, 0)
	reply, _ := Sanitize(in.Card.Reply, 0)
	card.Body, card.Reply = FenceSafe(body), FenceSafe(reply)

	about := fmt.Sprintf("My question about the card %d", in.Card.ID)
	if card.Key != "" {
		about += " (key " + jsonString(card.Key) + ")"
	}
	instruction := strings.TrimSpace(Instruction(p)) + "\n\n" + about + ":\n" + text
	cli := CLIInstruction(cmd) + "\n\n" + askLines(in.Card.ID, card.Key, cmd)

	items := Items(in.Messages, p.MessageMaxChars)
	b := Batch{BatchID: id, Space: space, Agent: p.ID, Count: len(items), Accounts: accounts(in.Messages)}
	r := &Rendered{Batch: b}
	for keep := len(items); keep >= 0; keep-- {
		dropped := len(items) - keep
		content, err := Encode(Content{Ask: in.ID, Space: safe(space), Card: card, Conversations: group(items[:keep]), Dropped: dropped})
		if err != nil {
			return nil, err
		}
		prompt, err := Prompt(instruction, content, cli)
		if err != nil {
			return nil, err
		}
		r.Items, r.Content, r.Prompt = items[:keep], content, prompt
		if dropped > 0 {
			r.Detail = fmt.Sprintf("ask too large, dropped %d messages", dropped)
		}
		if len(prompt) <= AskMaxBytes {
			break
		}
	}
	return r, nil
}

// askLines tell the agent to answer in the card. An answer in the terminal is
// lost: the user reads only the cards.
func askLines(cardID int64, key, cmd string) string {
	s := "I read your answer only in i2a, not in this terminal. Always answer in the card, also when you cannot do the task or need more information from me: then write in the card body what you need.\n"
	if key != "" {
		return s + fmt.Sprintf("When you finish, report the result with %s card add and the key %s. i2a then updates card %d.", cmd, jsonString(key), cardID)
	}
	return s + fmt.Sprintf("This card has no key. When you finish, add a new card with your result with %s card add, then close this card: %s card done --id %d", cmd, cmd, cardID)
}

func jsonString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}
