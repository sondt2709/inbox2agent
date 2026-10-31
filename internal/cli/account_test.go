package cli

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

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
	c.Agents = append(c.Agents, config.NewAgent("claude-work"))
	c.Spaces = []config.Space{{ID: "acme", Name: "Acme", Agent: "claude-work", Schedule: "always"}}
	if err := config.Save(paths.ConfigFile, c); err != nil {
		t.Fatal(err)
	}
	return paths.ConfigFile
}

func TestSourceCommandIsRenamed(t *testing.T) {
	spacesEnv(t)
	r := i2a(t, "", "source", "list")
	var ee exitError
	if !errors.As(r.err, &ee) || ee.code != 2 || !strings.Contains(r.err.Error(), "use i2a account") {
		t.Fatalf("i2a source list: %v", r.err)
	}
}

func TestConversationSet(t *testing.T) {
	file := spacesEnv(t)
	r := i2a(t, "", "conversation", "set", "phone", "g1@g.us", "--space", "acme", "--read", "mentions", "--label", "Acme team")
	if r.err != nil || r.out != "phone/g1@g.us: space Acme, read mentions\n" {
		t.Fatalf("set: %q, %v", r.out, r.err)
	}
	c, err := config.Load(file)
	if err != nil {
		t.Fatal(err)
	}
	if cv, sp, _ := c.Conversation("phone", "g1@g.us"); sp != "acme" || cv.Read != config.ReadMentions || cv.Label != "Acme team" {
		t.Errorf("conversation = %+v in %s", cv, sp)
	}
	// Back to Unsorted, with the level off. The label stays.
	if r := i2a(t, "", "conversation", "set", "phone", "g1@g.us", "--space", "unsorted", "--read", "off"); r.err != nil {
		t.Fatal(r.err)
	}
	c, _ = config.Load(file)
	if cv, sp, _ := c.Conversation("phone", "g1@g.us"); sp != config.UnsortedID || cv.Read != config.ReadOff || cv.Label != "Acme team" {
		t.Errorf("conversation = %+v in %s", cv, sp)
	}
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"conversation", "set", "phone", "g1@g.us"}, "give --space, --read or --label"},
		{[]string{"conversation", "set", "phone", "g1@g.us", "--read", "some"}, `--read "some" is not all, mentions or off`},
		{[]string{"conversation", "set", "phone", "g1@g.us", "--space", "nope"}, `the space "nope" does not exist`},
		{[]string{"conversation", "set", "slack", "x", "--space", "acme"}, `the account "slack" does not exist`},
	} {
		if r := i2a(t, "", tc.args...); r.err == nil || r.err.Error() != tc.want {
			t.Errorf("%v: %v, want %q", tc.args, r.err, tc.want)
		}
	}
}

func TestStatusShowsSpaces(t *testing.T) {
	spacesEnv(t)
	r := i2a(t, "", "status")
	if r.err != nil {
		t.Fatal(r.err)
	}
	for _, want := range []string{"Accounts:", "phone", "not paired", "Spaces:", "unsorted  Unsorted", "acme      Acme",
		"claude-work", "0 waiting", "new conversations: read mentions", "Agents:", "Schedules:", "every 15m · every day · all day"} {
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
	if r.err != nil || !strings.Contains(string(b[:n]), "i2a converted the configuration to version 2") ||
		!strings.Contains(string(b[:n]), paths.ConfigFile+".v1.bak") {
		t.Errorf("stderr = %q, err = %v", b[:n], r.err)
	}
}
