package render

import (
	"strings"
	"testing"

	"github.com/sondt2709/inbox2agent/internal/config"
	"github.com/sondt2709/inbox2agent/internal/inbox"
)

func TestDataLineStartsWithMessageID(t *testing.T) {
	msg := m("hi")
	msg.ID = 812
	block := Block(Items([]inbox.Message{msg}, 100), "abc")
	if line := dataLines(t, block, "abc")[0]; !strings.HasPrefix(line, `{"id":812,"n":1,`) {
		t.Errorf("line = %s", line)
	}
}

func TestCustomPrimerKeepsCardSection(t *testing.T) {
	p := config.NewProfile("triage")
	p.Primer = "Tóm tắt tin nhắn bằng tiếng Việt."
	r, err := Render(p, "b1", []inbox.Message{m("hi")}, t.TempDir(), "/usr/local/bin/i2a")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(r.Primer, p.Primer+"\n\n## Report to i2a") {
		t.Errorf("primer does not start with the custom primer and the section:\n%s", r.Primer)
	}
	for _, want := range []string{"/usr/local/bin/i2a card add <<'EOF'", "/usr/local/bin/i2a card done --key", `"refs":[<id>]`} {
		if !strings.Contains(r.Primer, want) {
			t.Errorf("primer has no %q", want)
		}
	}
	if strings.Contains(r.Prompt, "card add") {
		t.Error("the batch prompt has the card section; only the primer must have it")
	}

	none, err := Render(p, "b1", []inbox.Message{m("hi")}, t.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	if none.Primer != p.Primer {
		t.Errorf("primer without a card command = %q", none.Primer)
	}
}

func TestPrimerHash(t *testing.T) {
	p := config.NewProfile("triage")
	p.Primer = "Batch {{.BatchID}}: summarise."
	render := func(id, primer, cmd string) string {
		t.Helper()
		p.Primer = primer
		r, err := Render(p, id, []inbox.Message{m("hi")}, t.TempDir(), cmd)
		if err != nil {
			t.Fatal(err)
		}
		return r.PrimerHash
	}
	a := render("b1", "Batch {{.BatchID}}: summarise.", "/bin/i2a")
	if len(a) != 16 {
		t.Errorf("hash %q has %d characters, want 16", a, len(a))
	}
	if b := render("b2", "Batch {{.BatchID}}: summarise.", "/bin/i2a"); b != a {
		t.Errorf("another batch ID changed the hash: %s, %s", a, b)
	}
	if c := render("b1", "Batch {{.BatchID}}: summarise all.", "/bin/i2a"); c == a {
		t.Error("another primer template has the same hash")
	}
	if d := render("b1", "Batch {{.BatchID}}: summarise.", "/usr/bin/i2a"); d == a {
		t.Error("another card command has the same hash")
	}
}

func TestCardCommandQuotes(t *testing.T) {
	cases := []struct{ exe, home, want string }{
		{"/usr/local/bin/i2a", "", "/usr/local/bin/i2a"},
		{"/Users/me/My Tools/i2a", "", "'/Users/me/My Tools/i2a'"},
		{"/usr/local/bin/i2a", "/tmp/i2a home", "I2A_HOME='/tmp/i2a home' /usr/local/bin/i2a"},
		{"/x/it's/i2a", "", `'/x/it'\''s/i2a'`},
	}
	for _, c := range cases {
		if got := CardCommand(c.exe, c.home); got != c.want {
			t.Errorf("CardCommand(%q, %q) = %q, want %q", c.exe, c.home, got, c.want)
		}
	}
}
