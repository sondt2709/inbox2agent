package cli

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/sondt2709/inbox2agent/internal/inbox"
)

func TestStatusPrintsTheSessionOfEachSpace(t *testing.T) {
	sent := time.Date(2026, 11, 11, 9, 4, 0, 0, time.Local)
	var b bytes.Buffer
	printSessions(&b, []inbox.Session{
		{Space: "triage", Handle: "term_a", AgentSession: "04ebe19c-543e-42ff-8f84-ea13cea7a3ec", Name: "i2a · Triage", LastSent: sent},
		{Space: "unsorted", Handle: "term_b"},
	})
	out := b.String()
	for _, want := range []string{
		"Sessions:\n",
		`  triage       term_a  Claude 04ebe19c-543e-42ff-8f84-ea13cea7a3ec  "i2a · Triage"  last send 2026-11-11 09:04`,
		"  unsorted     term_b  Claude unknown  no send yet",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("status has no %q:\n%s", want, out)
		}
	}
	b.Reset()
	printSessions(&b, nil)
	if b.Len() != 0 {
		t.Errorf("no sessions printed %q, want nothing", b.String())
	}
}
