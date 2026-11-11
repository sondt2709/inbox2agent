package render

import (
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"

	yaml "go.yaml.in/yaml/v3"

	"github.com/sondt2709/inbox2agent/internal/inbox"
)

// Content is the YAML content of a batch or an ask. All string values pass
// Sanitize and FenceSafe, so the content cannot close the yaml fence.
type Content struct {
	Batch         string         `yaml:"batch,omitempty"`
	Ask           int64          `yaml:"ask,omitempty"`
	Space         string         `yaml:"space"`
	Card          *CardData      `yaml:"card,omitempty"`
	Conversations []Conversation `yaml:"conversations"`
	Dropped       int            `yaml:"dropped,omitempty"` // messages of an ask that are not in the content
}

// Conversation is one conversation of a batch with its messages.
type Conversation struct {
	Name     string `yaml:"name"`
	Account  string `yaml:"account"`
	Platform string `yaml:"platform"`
	Kind     string `yaml:"kind"`
	Messages []Item `yaml:"messages"`
}

// Item is one message in the content.
type Item struct {
	ID        int64  `yaml:"id"` // the row ID of the message in the store; cards refer to it
	From      string `yaml:"from"`
	Time      string `yaml:"time"`
	URL       string `yaml:"url,omitempty"`
	ReplyTo   *Reply `yaml:"reply_to,omitempty"`
	Text      string `yaml:"text"`
	Truncated bool   `yaml:"truncated,omitempty"`

	conv convKey // the conversation of the message; not in the YAML
}

// Reply is the earlier message that a message quotes or answers in a thread.
type Reply struct {
	ID   int64  `yaml:"id"` // the row ID of the earlier message; 0 when the store does not have it
	From string `yaml:"from"`
	Me   bool   `yaml:"me"`
	Text string `yaml:"text"`
}

// CardData is the card of an ask. The card text came from an agent that read
// untrusted messages, so it is data, never instruction.
type CardData struct {
	ID       int64   `yaml:"id"`
	Key      string  `yaml:"key"`
	Category string  `yaml:"category"`
	Urgency  string  `yaml:"urgency"`
	Group    string  `yaml:"group"`
	Title    string  `yaml:"title"`
	Body     string  `yaml:"body"`
	Reply    string  `yaml:"reply"`
	Refs     []int64 `yaml:"refs"`
}

type convKey struct {
	name, account, platform, kind, channelID string
}

// fence is the line that opens and closes a fenced block.
const fence = "```"

// FenceSafe replaces each sequence of three backticks with three apostrophes,
// so the text cannot open or close a fenced block. Only the content builder
// uses it: card bodies and replies keep their Markdown code fences.
func FenceSafe(s string) string { return strings.ReplaceAll(s, fence, "'''") }

// safe sanitizes a short field: one line, at most maxFieldChars characters.
func safe(s string) string { return FenceSafe(field(s)) }

// Items converts messages to content items.
func Items(msgs []inbox.Message, maxChars int) []Item {
	out := make([]Item, 0, len(msgs))
	for _, m := range msgs {
		text, cut := Sanitize(m.Text, maxChars)
		name := m.ChannelName
		if name == "" {
			name = m.ChannelID
		}
		out = append(out, Item{
			ID: m.ID, From: safe(m.Author), Time: m.CreatedAt.Local().Format(time.RFC3339),
			URL: safe(m.URL), ReplyTo: reply(m.ReplyTo), Text: FenceSafe(text), Truncated: cut,
			conv: convKey{name: safe(name), account: safe(m.SourceID), platform: safe(m.SourceType), kind: safe(m.ChannelKind), channelID: m.ChannelID},
		})
	}
	return out
}

// group puts the items in conversations, in the order of the first message
// of each conversation. The messages keep their order.
func group(items []Item) []Conversation {
	out := []Conversation{}
	at := map[convKey]int{}
	for _, it := range items {
		i, ok := at[it.conv]
		if !ok {
			i = len(out)
			at[it.conv] = i
			out = append(out, Conversation{Name: it.conv.name, Account: it.conv.account, Platform: it.conv.platform, Kind: it.conv.kind, Messages: []Item{}})
		}
		out[i].Messages = append(out[i].Messages, it)
	}
	return out
}

// errFenceInContent means that the content still has three backticks.
var errFenceInContent = errors.New("render: the content contains three backticks")

// Encode writes the content as YAML with an indent of two spaces. It decodes
// the YAML again; when the result differs from c (an encoder bug), it writes
// all strings double-quoted. It returns an error when the YAML has three
// backticks or does not decode to c.
func Encode(c Content) (string, error) {
	norm(&c)
	out, err := encode(c, false)
	if err != nil || !same(out, c) {
		if out, err = encode(c, true); err != nil {
			return "", err
		}
		if !same(out, c) {
			return "", errors.New("render: the YAML content does not decode to the messages")
		}
	}
	if strings.Contains(out, fence) {
		return "", errFenceInContent
	}
	return out, nil
}

// norm makes the empty lists non-nil, so a decoded copy is equal to c.
func norm(c *Content) {
	if c.Conversations == nil {
		c.Conversations = []Conversation{}
	}
	for i := range c.Conversations {
		if c.Conversations[i].Messages == nil {
			c.Conversations[i].Messages = []Item{}
		}
		for j := range c.Conversations[i].Messages {
			c.Conversations[i].Messages[j].conv = convKey{}
		}
	}
	if c.Card != nil && c.Card.Refs == nil {
		c.Card.Refs = []int64{}
	}
}

// encode writes c with an indent of two spaces. With quoted, each string is
// double-quoted. It builds the node tree itself: yaml.Node.Encode parses its
// own output again with an indent of four, which has a bug for a block
// scalar that starts with a newline.
func encode(c Content, quoted bool) (string, error) {
	n, err := node(reflect.ValueOf(c), quoted)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	enc := yaml.NewEncoder(&b)
	enc.SetIndent(2)
	if err := enc.Encode(n); err != nil {
		return "", err
	}
	if err := enc.Close(); err != nil {
		return "", err
	}
	return b.String(), nil
}

// node converts the content types to a YAML node: structs with yaml tags and
// omitempty, slices, pointers, strings, integers and booleans.
func node(v reflect.Value, quoted bool) (*yaml.Node, error) {
	switch v.Kind() {
	case reflect.Pointer:
		return node(v.Elem(), quoted)
	case reflect.String:
		n := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: v.String()}
		if quoted {
			n.Style = yaml.DoubleQuotedStyle
		}
		return n, nil
	case reflect.Int, reflect.Int64:
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!int", Value: strconv.FormatInt(v.Int(), 10)}, nil
	case reflect.Bool:
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!bool", Value: strconv.FormatBool(v.Bool())}, nil
	case reflect.Slice:
		n := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
		if v.Type().Elem().Kind() != reflect.Struct {
			n.Style = yaml.FlowStyle // refs: [812, 813]
		}
		for i := range v.Len() {
			e, err := node(v.Index(i), quoted)
			if err != nil {
				return nil, err
			}
			n.Content = append(n.Content, e)
		}
		return n, nil
	case reflect.Struct:
		n := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		t := v.Type()
		for i := range t.NumField() {
			tag, ok := t.Field(i).Tag.Lookup("yaml")
			if !ok || !t.Field(i).IsExported() {
				continue
			}
			name, opt, _ := strings.Cut(tag, ",")
			f := v.Field(i)
			if opt == "omitempty" && f.IsZero() {
				continue
			}
			val, err := node(f, quoted)
			if err != nil {
				return nil, err
			}
			n.Content = append(n.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: name}, val)
		}
		return n, nil
	}
	return nil, fmt.Errorf("render: cannot write %s as YAML", v.Type())
}

func same(out string, c Content) bool {
	var back Content
	if err := yaml.Unmarshal([]byte(out), &back); err != nil {
		return false
	}
	return reflect.DeepEqual(back, c)
}
