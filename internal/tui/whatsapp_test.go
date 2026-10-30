package tui

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/sondt2709/inbox2agent/internal/source/whatsapp"
)

func TestValidPhone(t *testing.T) {
	for _, ok := range []string{"", "+15551234567", "+1 (555) 123-4567", "15551234567"} {
		if err := validPhone(ok); err != nil {
			t.Errorf("validPhone(%q) = %v, want nil", ok, err)
		}
	}
	for _, bad := range []string{"+1555abc4567", "0901234567", "+12345", "555 12"} {
		if validPhone(bad) == nil {
			t.Errorf("validPhone(%q) = nil, want an error", bad)
		}
	}
}

func TestAddFormTypeWhatsApp(t *testing.T) {
	m := newTestModel(t)
	send(m, "3", "a")
	if s := screen(m); !strings.Contains(s, "Server URL") || strings.Contains(s, "Phone number") {
		t.Fatalf("the Mattermost add form is wrong:\n%s", s)
	}
	send(m, "right")
	s := screen(m)
	for _, want := range []string{"WhatsApp", "Phone number", "can ban an account", "Connect"} {
		if !strings.Contains(s, want) {
			t.Errorf("the WhatsApp add form has no %q:\n%s", want, s)
		}
	}
	for _, not := range []string{"Server URL", "Token"} {
		if strings.Contains(s, not) {
			t.Errorf("the WhatsApp add form shows %q", not)
		}
	}
}

// TestPairingPanelThenStep2 gives the pairing events by hand, as the pairing
// goroutine does after a real connection.
func TestPairingPanelThenStep2(t *testing.T) {
	m := newTestModel(t)
	send(m, "3", "a", "right")
	p := &pairing{id: "phone", events: make(chan tea.Msg, 8), cancel: func() {}}
	m.pair = p
	m.buildForm(true)
	if !strings.Contains(screen(m), "Connecting to WhatsApp") {
		t.Fatalf("no connect note:\n%s", screen(m))
	}

	// The length of a real code: a reference and three base64 keys.
	qr := "2@" + strings.Repeat("R", 88) + "," + strings.Repeat("N", 44) + "," + strings.Repeat("I", 44) + "," + strings.Repeat("A", 44)
	m.Update(pairStepMsg{p: p, ev: whatsapp.PairEvent{QR: qr}})
	if s := screen(m); !strings.Contains(s, "too short for the QR code") || strings.Contains(s, "▀") {
		t.Fatalf("a 40-row window shows the QR code:\n%s", s)
	}
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 50})
	if s := screen(m); !strings.Contains(s, "▀") || !strings.Contains(s, "Linked devices") {
		t.Fatalf("a 50-row window does not show the QR code:\n%s", s)
	}

	m.Update(pairStepMsg{p: p, ev: whatsapp.PairEvent{Code: "ABCD-1234"}})
	if s := screen(m); !strings.Contains(s, "ABCD-1234") || !strings.Contains(s, "Link with phone number instead") {
		t.Fatalf("no pairing code:\n%s", s)
	}

	_, cmd := m.Update(pairDoneMsg{p: p, chats: []whatsapp.Chat{
		{JID: "120363000000000001@g.us", Name: "Team chat", Kind: "group", Last: time.Now()},
		{JID: "15550000001@s.whatsapp.net", Kind: "direct"},
	}})
	run(m, cmd, 0)
	s := screen(m)
	for _, want := range []string{"Paired", "Team chat", "+15550000001", "Add source"} {
		if !strings.Contains(s, want) {
			t.Errorf("step 2 has no %q:\n%s", want, s)
		}
	}
	if m.pair != nil {
		t.Error("the pairing is still open after done")
	}
	send(m, "ctrl+s")
	f := fileText(t, m)
	for _, want := range []string{"id = 'phone'", "type = 'whatsapp'"} {
		if !strings.Contains(f, want) {
			t.Errorf("the file has no %q:\n%s", want, f)
		}
	}
}

func TestPairingErrorsSaveNothing(t *testing.T) {
	for name, tc := range map[string]struct {
		err  error
		want string
	}{
		"timeout": {whatsapp.ErrPairTimeout, "Pairing timed out. Nothing was added."},
		"busy":    {whatsapp.ErrBusy, "Another i2a process holds the source phone. Nothing was added."},
	} {
		t.Run(name, func(t *testing.T) {
			m := newTestModel(t)
			before := fileText(t, m)
			send(m, "3", "a", "right")
			p := &pairing{id: "phone", events: make(chan tea.Msg, 8), cancel: func() {}}
			m.pair = p
			m.Update(pairDoneMsg{p: p, err: tc.err})
			if !strings.Contains(screen(m), tc.want) || fileText(t, m) != before {
				t.Errorf("screen has no %q, or the file changed:\n%s", tc.want, screen(m))
			}
		})
	}
}
