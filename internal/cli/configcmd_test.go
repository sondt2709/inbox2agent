package cli

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sondt2709/inbox2agent/internal/config"
)

func TestConfigCheck(t *testing.T) {
	file := spacesEnv(t)
	r := i2a(t, "", "config", "check")
	if r.err != nil || !strings.Contains(r.out, "is valid: 1 account, 2 agents, 1 schedule, 1 space and Unsorted, 0 conversations") {
		t.Fatalf("check of a valid file: %q, %v", r.out, r.err)
	}
	b, _ := os.ReadFile(file)
	bad := strings.Replace(string(b), "agent = 'claude-work'", "agent = 'nope'", 1) +
		"\n[[spaces.conversations]]\naccount = 'phone'\nid = 'g1@g.us'\nread = 'some'\n"
	other := filepath.Join(t.TempDir(), "next.toml")
	if err := os.WriteFile(other, []byte(bad), 0o600); err != nil {
		t.Fatal(err)
	}
	r = i2a(t, "", "config", "check", other)
	var ee exitError
	if !errors.As(r.err, &ee) || ee.code != 1 {
		t.Fatalf("check of a bad file: %v", r.err)
	}
	for _, want := range []string{`the agent "nope" does not exist`, `.read: "some" is not all, mentions or off`} {
		if !strings.Contains(r.err.Error(), want) {
			t.Errorf("the check does not say %q:\n%v", want, r.err)
		}
	}
	if after, _ := os.ReadFile(other); string(after) != bad {
		t.Error("the check wrote the file")
	}
	if r := i2a(t, "", "config", "check", filepath.Join(t.TempDir(), "missing.toml")); r.err == nil || !strings.Contains(r.err.Error(), "cannot read") {
		t.Errorf("check of a missing file: %v", r.err)
	}
}

func TestConfigSchemaAndHelp(t *testing.T) {
	spacesEnv(t)
	r := i2a(t, "", "config", "schema")
	if r.err != nil || r.out != string(config.Schema()) {
		t.Fatalf("config schema: %v", r.err)
	}
	r = i2a(t, "", "config", "--help")
	for _, want := range []string{"config.toml", "config.schema.json", "i2a conversations --json", "i2a config check", "[[spaces.conversations]]"} {
		if !strings.Contains(r.out, want) {
			t.Errorf("i2a config --help does not name %q:\n%s", want, r.out)
		}
	}
}

func TestConversationsJSON(t *testing.T) {
	file := spacesEnv(t)
	if _, _, err := config.Update(file, func(c *config.Config) error {
		if err := c.SetConversation("phone", "g1@g.us", "acme", config.ReadMentions, "Team"); err != nil {
			return err
		}
		return c.SetLabel("phone", "g1@g.us", "Acme team")
	}); err != nil {
		t.Fatal(err)
	}
	r := i2a(t, "", "conversations", "--json")
	if r.err != nil {
		t.Fatal(r.err)
	}
	var rows []map[string]string
	if err := json.Unmarshal([]byte(r.out), &rows); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, r.out)
	}
	want := map[string]string{"account": "phone", "id": "g1@g.us", "kind": "", "name": "Team", "label": "Acme team", "space": "acme", "read": "mentions"}
	if len(rows) != 1 || len(rows[0]) != len(want) {
		t.Fatalf("rows = %v", rows)
	}
	for k, v := range want {
		if rows[0][k] != v {
			t.Errorf("%s = %q, want %q", k, rows[0][k], v)
		}
	}
}
