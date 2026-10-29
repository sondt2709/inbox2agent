package inbox

import "time"

// Reply states.
const (
	ReplySending = "sending" // in flight, or the result is not known (a time-out)
	ReplySent    = "sent"
	ReplyFailed  = "failed" // the platform answered with an error, so no post exists
)

// Reply places.
const (
	PlaceThread  = "thread"  // in the thread of the target message
	PlaceChannel = "channel" // in the channel, outside a thread
)

// Who sent a reply.
const (
	ByUser  = "user"  // from the Feed
	ByAgent = "agent" // with "i2a reply"
)

// Reply is one reply that i2a sends to a source, as a reply to a stored message.
type Reply struct {
	ID          int64
	MessageID   int64 // the target; the message can be pruned later
	CardID      int64 // 0 when the reply has no card
	SourceID    string
	ChannelName string
	Place       string
	Text        string
	By          string
	State       string
	PendingID   string // the same value for each try of one reply
	ExternalID  string // the ID of the post on the platform, when sent
	URL         string
	Error       string
	CreatedAt   time.Time
	SentAt      time.Time // zero when not sent
}

// ReplyRequest is one try to send a reply. The source reads the channel and
// the thread from the platform, not from the request.
type ReplyRequest struct {
	Target Message // a stored message
	Text   string
	Thread bool // true: in the thread of Target; false: in the channel
	// PendingID is the same for each try of one reply. The source makes the
	// duplicate guard of the platform from it.
	PendingID string
	Since     time.Time // the time of the first try
}

// Posted is a post that a source created.
type Posted struct {
	ID  string
	URL string
}
