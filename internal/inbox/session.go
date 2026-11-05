package inbox

import "time"

// PrimerPending is the primer hash of a session that the user linked: the
// terminal did not get the primer, so the next send includes it.
const PrimerPending = "pending"

// Session is the agent session of a space: the Orca terminal that i2a types
// into (the address), and the Claude session in it (the identity). One
// terminal can run Claude session A, then a shell, then session B.
type Session struct {
	Space        string
	Handle       string
	AgentSession string // the Claude session ID; "" when not known
	PrimerHash   string
	CreatedAt    time.Time
}
