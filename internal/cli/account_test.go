package cli

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sondt2709/inbox2agent/internal/app"
	"github.com/sondt2709/inbox2agent/internal/config"
)

// spacesEnv makes an I2A_HOME with a configuration: the WhatsApp account
// phone, the agent claude-work and the space Acme.
func spacesEnv(t *testing.T) string {
	t.Helper()
	t.Setenv("I2A_HOME", t.TempDir())
	t.Setenv("I2A_SECRET_BACKEND", "file")
	paths, err := app.ResolvePaths()
	if err != nil {
		t.Fatal(err)
	}
	c := config.Default()
	c.Accounts = []config.Account{config.NewWhatsAppAccount("phone")}
	acme := config.NewAgent("acme")
	acme.Driver, acme.CLI, acme.Folder = config.DriverOrca, "claude", "/w"
	c.Spaces = []config.Space{{ID: "acme", Name: "Acme", Agent: acme, Schedule: "always"}}
	if err := config.Save(paths.ConfigFile, c); err != nil {
		t.Fatal(err)
	}
	return paths.ConfigFile
}

func TestConversationSetIsRemoved(t *testing.T) {
	file := spacesEnv(t)
	before, _ := os.ReadFile(file)
	r := i2a(t, "", "conversation", "set", "phone", "g1@g.us", "--space", "acme")
	var ee exitError
	if !errors.As(r.err, &ee) || ee.code != 2 || !strings.Contains(r.err.Error(), "[[spaces.conversations]]") ||
		!strings.Contains(r.err.Error(), "i2a config --help") {
		t.Fatalf("i2a conversation set: %v", r.err)
	}
	if after, _ := os.ReadFile(file); string(after) != string(before) {
		t.Error("the removed command changed config.toml")
	}
	if r := i2a(t, "", "source", "list"); r.err == nil || !strings.Contains(r.err.Error(), "unknown command") {
		t.Errorf("i2a source list: %v, want unknown command", r.err)
	}
}

func TestStatusShowsSpaces(t *testing.T) {
	spacesEnv(t)
	r := i2a(t, "", "status")
	if r.err != nil {
		t.Fatal(r.err)
	}
	for _, want := range []string{"Accounts:", "phone", "not paired", "Spaces:", "unsorted  Unsorted", "acme      Acme",
		"orca · claude", "0 waiting", "new conversations: read mentions", "Schedules:", "every 15m · every day · all day"} {
		if !strings.Contains(r.out, want) {
			t.Errorf("status has no %q:\n%s", want, r.out)
		}
	}
}

func TestLoadPrintsTheMigrationReport(t *testing.T) {
	t.Setenv("I2A_HOME", t.TempDir())
	t.Setenv("I2A_SECRET_BACKEND", "file")
	paths, _ := app.ResolvePaths()
	if err := os.MkdirAll(filepath.Dir(paths.ConfigFile), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(paths.ConfigFile, []byte("[[profiles]]\nname = \"triage\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	stderr := os.Stderr
	rd, wr, _ := os.Pipe()
	os.Stderr = wr
	r := i2a(t, "", "account", "list")
	wr.Close()
	os.Stderr = stderr
	b := make([]byte, 4096)
	n, _ := rd.Read(b)
	if r.err != nil || !strings.Contains(string(b[:n]), "i2a converted the configuration from version 1 to version 3") ||
		!strings.Contains(string(b[:n]), paths.ConfigFile+".v1.bak") {
		t.Errorf("stderr = %q, err = %v", b[:n], r.err)
	}
}

func TestAccountRemoveWaitsForTheLockOfAnotherWriter(t *testing.T) {
	file := spacesEnv(t)
	locked, done := make(chan struct{}), make(chan error)
	go func() {
		_, _, err := config.Update(file, func(c *config.Config) error {
			close(locked)
			time.Sleep(300 * time.Millisecond) // another writer holds the lock
			c.Spaces = append(c.Spaces, config.Space{ID: "side-project", Name: "Side project", Agent: config.NewAgent("side-project"), Schedule: "always"})
			return nil
		})
		done <- err
	}()
	<-locked
	if r := i2a(t, "", "account", "remove", "phone"); r.err != nil {
		t.Fatal(r.err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	c, err := config.Load(file)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := c.Space("side-project"); !ok {
		t.Errorf("the command removed the space of the other writer")
	}
	if _, ok := c.Account("phone"); ok {
		t.Errorf("the command did not remove the account")
	}
}

func TestStatusShowsAPausedSpace(t *testing.T) {
	file := spacesEnv(t)
	if _, _, err := config.Update(file, func(c *config.Config) error { c.Spaces[0].Paused = true; return nil }); err != nil {
		t.Fatal(err)
	}
	r := i2a(t, "", "status")
	if r.err != nil {
		t.Fatal(r.err)
	}
	line := ""
	for l := range strings.SplitSeq(r.out, "\n") {
		if strings.Contains(l, "acme") {
			line = l
		}
	}
	if !strings.Contains(line, "orca · claude") || !strings.Contains(line, "paused: does not send") || !strings.Contains(line, "0 waiting") {
		t.Errorf("status line of acme = %q", line)
	}
	if strings.Contains(r.out, "Agents:") {
		t.Errorf("status has an Agents part:\n%s", r.out)
	}
}
