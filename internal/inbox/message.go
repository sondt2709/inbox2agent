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
	Author      string
	Text        string
	URL         string
	CreatedAt   time.Time
	State       string
}

// Cursor is the position of a source, as key and value pairs.
// Each source decides its keys, for example one key for each channel.
type Cursor map[string]string
