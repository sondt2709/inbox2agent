package tui

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/sondt2709/inbox2agent/internal/config"
)

func TestAddLinearAccount(t *testing.T) {
	m := newTestModel(t)
	var checked []string
	m.u.d.Check = func(_ context.Context, a config.Account, key string) (string, error) {
		if a.Platform == config.PlatformLinear {
			checked = append(checked, key)
			return "me.user@acme", nil
		}
		return "alice", nil
	}
	m.Update(tea.WindowSizeMsg{Width: 220, Height: 50})
	at(m, tabAccounts)
	send(m, "a", "right", "right")
	s := screen(m)
	for _, want := range []string{"Linear", "API key", "agent (your comments that start with /agent)", "Connect"} {
		if !strings.Contains(s, want) {
			t.Errorf("the Linear add form has no %q:\n%s", want, s)
		}
	}
	for _, not := range []string{"Server URL", "Phone number"} {
		if strings.Contains(s, not) {
			t.Errorf("the Linear add form shows %q", not)
		}
	}
	m.form.field("id").input.SetValue("lin")
	m.form.field("key").input.SetValue("lin_api_test")
	for _, b := range m.form.buttons {
		if b.label == "Connect" {
			run(m, b.run(), 0)
		}
	}
	// Connect checks the key, then the listing of the new account checks it again.
	if len(checked) == 0 || checked[0] != "lin_api_test" {
		t.Fatalf("checked keys = %v; form error %q", checked, m.form.err)
	}
	a, ok := m.u.cfg.Account("lin")
	if !ok || a.User != "me.user@acme" || len(a.Feeds) != 2 {
		t.Fatalf("account = %+v, %v", a, ok)
	}
	if k, _ := m.u.d.Secrets.Get(secretsKey("lin")); k != "lin_api_test" {
		t.Errorf("saved key = %q", k)
	}
	if !strings.Contains(m.status, "Connected as me.user@acme") {
		t.Errorf("status = %q", m.status)
	}

	// The detail of the account and its feeds as conversations in Unsorted.
	m.acc = 1
	m.buildForm(true)
	s = screen(m)
	for _, want := range []string{"User        me.user", "Workspace   acme", "Feed agent · its conversation is in Everything else",
		"Feed inbox · its conversation is in Everything else", "API key", "2 conversations: Unsorted 2"} {
		if !strings.Contains(s, want) {
			t.Errorf("the Linear account panel has no %q:\n%s", want, s)
		}
	}
	if strings.Contains(s, "Server URL") {
		t.Error("the Linear account panel shows the server URL")
	}
}
