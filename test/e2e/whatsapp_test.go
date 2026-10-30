//go:build e2e

package e2e

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/sondt2709/inbox2agent/internal/config"
)

// TestS33WhatsAppNotPaired: a WhatsApp source in config.toml that was never
// paired. No connection to WhatsApp is necessary.
func TestS33WhatsAppNotPaired(t *testing.T) {
	p := newPerson(t, "S33")
	c := config.Default()
	c.Sources = append(c.Sources, config.NewWhatsAppSource("phone"))
	if err := config.Save(p.configFile(), c); err != nil {
		t.Fatal(err)
	}

	s, code := p.run("status")
	if code != 0 || !regexp.MustCompile(`phone\s+whatsapp\s+not paired`).MatchString(s) {
		t.Fatalf("status: exit %d, want the source phone with the state not paired:\n%s", code, s)
	}
	s, code = p.run("source list")
	if code != 0 || !regexp.MustCompile(`phone\s+whatsapp\s+not paired`).MatchString(s) {
		t.Fatalf("source list: exit %d:\n%s", code, s)
	}
	s, code = p.run("run --once")
	if code == 0 || !strings.Contains(s, "not paired: pair again in i2a config") {
		t.Fatalf("run --once: exit %d, want the not paired error:\n%s", code, s)
	}
	if fi, err := os.Stat(filepath.Join(p.home, "whatsapp-phone.db")); err != nil || fi.Mode().Perm() != 0o600 {
		t.Errorf("whatsapp-phone.db: %v, err %v; want the mode 0600", fi, err)
	}

	tm := p.term("config")
	tm.WaitFor(`2 Overview`, 30*time.Second)
	s = tm.Press("3", `not paired: remove this source`, 5*time.Second)
	if strings.Contains(s, "Server URL") || strings.Contains(s, "Token") {
		t.Errorf("the form of a WhatsApp source shows Mattermost fields:\n%s", s)
	}
}

// TestS34WhatsAppAddForm: the add form shows the WhatsApp fields. The test
// does not press Connect, so it needs no phone.
func TestS34WhatsAppAddForm(t *testing.T) {
	p := newPerson(t, "S34")
	tm := p.term("config")
	tm.WaitFor(`2 Overview`, 30*time.Second)
	tm.Press("3", `Connect a Mattermost account`, 5*time.Second)
	tm.Press("Enter", `▌ Type`, 5*time.Second)
	s := tm.Press("Right", `Phone number`, 5*time.Second)
	for _, want := range []string{"can ban an account", "Connect"} {
		if !strings.Contains(s, want) {
			t.Errorf("the WhatsApp form has no %q:\n%s", want, s)
		}
	}
	for _, not := range []string{"Server URL", "Token"} {
		if strings.Contains(s, not) {
			t.Errorf("the WhatsApp form shows %q:\n%s", not, s)
		}
	}
	tm.Press("Tab", `▌ Source ID`, 5*time.Second)
	tm.Press("Tab", `▌ Phone number`, 5*time.Second)
	tm.Type("12345")
	tm.Press("Enter", `international format`, 5*time.Second) // Enter in the last field submits; the phone is wrong
	tm.Keys("C-c")
	tm.WaitFor(`Unsaved changes|\[exit 0\]`, 5*time.Second)
	if _, err := os.Stat(p.configFile()); err == nil && strings.Contains(p.readConfigText(), "whatsapp") {
		t.Error("a WhatsApp source was saved with no pairing")
	}
}

func (p *person) readConfigText() string {
	b, _ := os.ReadFile(p.configFile())
	return string(b)
}
