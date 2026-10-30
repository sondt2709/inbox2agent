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
	Space       string // the space of the dispatch that sent the message; "" before a dispatch
}

// Cursor is the position of a source, as key and value pairs.
// Each source decides its keys, for example one key for each channel.
type Cursor map[string]string
