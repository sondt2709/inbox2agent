package inbox

import "time"

// Session is the agent session of a space: the Orca terminal that i2a types
// into (the address), and the Claude session in it (the identity). One
// terminal can run Claude session A, then a shell, then session B.
type Session struct {
	Space        string
	Handle       string
	AgentSession string    // the Claude session ID; "" when not known
	Name         string    // the name of the Claude session; "" when not known
	CreatedAt    time.Time // when i2a stored the session
	LastSent     time.Time // the last prompt that i2a sent to it; zero for none
}
