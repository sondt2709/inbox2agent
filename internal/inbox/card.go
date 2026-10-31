package inbox

import "time"

// Card categories.
const (
	CategoryAction   = "action"   // the user must do something
	CategoryQuestion = "question" // someone asks the user
	CategoryInfo     = "info"     // no action
)

// Categories lists the card categories in display order.
var Categories = []string{CategoryAction, CategoryQuestion, CategoryInfo}

// Card urgencies.
const (
	UrgencyHigh   = "high"
	UrgencyNormal = "normal"
	UrgencyLow    = "low"
)

// Urgencies lists the urgencies from high to low.
var Urgencies = []string{UrgencyHigh, UrgencyNormal, UrgencyLow}

// UrgencyRank returns 2 for high, 1 for normal and 0 for low or an unknown value.
func UrgencyRank(u string) int {
	switch u {
	case UrgencyHigh:
		return 2
	case UrgencyNormal:
		return 1
	}
	return 0
}

// Card states.
const (
	CardNew  = "new"
	CardRead = "read"
	CardDone = "done"
)

// Card is one result that an agent reports: a topic with a summary and an
// optional suggested reply. Refs are the row IDs of the source messages.
type Card struct {
	ID        int64
	Key       string // "" when the card has no key
	Category  string
	Urgency   string
	Group     string
	Title     string
	Body      string
	Reply     string
	Refs      []int64
	State     string
	Handle    string // the Orca terminal of the agent that wrote the card; "" when not known
	Space     string // the space of the session of Handle; "" when no session has it
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Results of a card save.
const (
	SaveAdded     = "added"
	SaveUpdated   = "updated"
	SaveUnchanged = "unchanged"
)

// SavedCard is a card after a save, with the state that it had before.
type SavedCard struct {
	Card
	Result      string // added, updated or unchanged
	PrevUrgency string // "" for an added card
	PrevState   string // "" for an added card
}
