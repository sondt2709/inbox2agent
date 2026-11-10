// Package inbox holds the types that the sources, the store, the renderer
// and the drivers share.
package inbox

import "time"

// Channel kinds.
const (
	KindChannel = "channel"
	KindDirect  = "direct"
	KindGroup   = "group"
)

// Message states.
const (
	StatePending   = "pending"
	StatePreviewed = "previewed" // in a dry-run batch; a real driver still sends it
	StateSent      = "sent"
	StateExpired   = "expired"
)

// DeliverySkipped is a delivery state only: the space does not get the
// message. A requeue for new spaces writes it for the other spaces.
const DeliverySkipped = "skipped"

// Message is one message from one source.
type Message struct {
	ID          int64 // row ID in the store; 0 before the store saves it
	SourceID    string
	SourceType  string
	ExternalID  string
	ChannelID   string
	ChannelName string
	ChannelKind string
	ThreadID    string // the external ID of the thread root; "" for a top-level message
	Author      string
	Text        string
	URL         string
	CreatedAt   time.Time
	State       string
	Space       string // the space of the last dispatch that sent the message; "" before a dispatch
	// ForMe is true for a message for the user: a message of a direct
	// conversation, a mention of the user or a reply to the user. A link
	// with the read level mentions gets only these messages.
	ForMe bool
	// ReplyTo is the earlier message that this message quotes (WhatsApp) or
	// answers in a thread (Mattermost); nil when it answers no message.
	ReplyTo *ReplyRef
	// Delivered is the state of the delivery of the message to each space
	// that got it (sent or previewed). Store.Pending fills it.
	Delivered map[string]string
}

// Cursor is the position of a source, as key and value pairs.
// Each source decides its keys, for example one key for each channel.
type Cursor map[string]string

// ReplyRef is the reference of a message to an earlier message.
type ReplyRef struct {
	Ext    string // the external ID of the earlier message
	ID     int64  // the row ID of the earlier message in the store; 0 when the store does not have it
	Author string // the author of the earlier message; "" when not known or when Me is true
	Me     bool   // the user wrote the earlier message
	Text   string // the start of the text of the earlier message
}
