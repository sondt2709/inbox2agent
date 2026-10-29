package inbox

import "time"

// Ask states.
const (
	AskQueued = "queued" // not sent yet, for example because the agent was busy
	AskSent   = "sent"
	AskFailed = "failed"
)

// Ask is one instruction of the user about a card, for the agent.
type Ask struct {
	ID        int64
	CardID    int64
	Profile   string
	Handle    string // the terminal that got the ask; "" before the send
	Text      string
	State     string
	Detail    string // the result of the last try
	CreatedAt time.Time
	SentAt    time.Time // zero when not sent
}
