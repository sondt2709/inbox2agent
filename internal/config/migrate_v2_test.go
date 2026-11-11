package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	toml "github.com/pelletier/go-toml/v2"
)

// TestMigrateV2OneAgentForEachSpace converts a file in the shape of a real
// version 2 file: each space gets its agent and its instruction, Unsorted on
// hold is paused, and the file has no [[agents]].
func TestMigrateV2OneAgentForEachSpace(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("testdata", "v2_one_agent_each.toml"))
	if err != nil {
		t.Fatal(err)
	}
	c, err := Parse(b)
	if err != nil {
		t.Fatal(err)
	}
	out, err := toml.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	text := string(out)
	for _, want := range []string{
		"version = 3",
		"[spaces.agent]",
		"folder = '/Users/me/work/docs'",
		"instruction = 'Help the on-site operators. Send a reply only when someone asks you.'",
		"instruction = \"\"\"\nTóm tắt các tin nhắn",
		"[unsorted.agent]",
		"paused = true",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("the converted file has no %q:\n%s", want, text)
		}
	}
	for _, gone := range []string{"[[agents]]", "workspace =", "primer =", "agent = '"} {
		if strings.Contains(text, gone) {
			t.Errorf("the converted file still has %q", gone)
		}
	}
}

func TestMigrateV2Report(t *testing.T) {
	b, _ := os.ReadFile(filepath.Join("testdata", "v2_one_agent_each.toml"))
	c, err := Parse(b)
	if err != nil {
		t.Fatal(err)
	}
	onsite, _ := c.Space("onsite")
	if onsite.Agent.Folder != "/Users/me/work/docs" || onsite.Agent.InlineMaxBytes != 16000 || onsite.Agent.ID != "onsite" ||
		onsite.Agent.Name != "Onsite group" || onsite.Instruction != "Help the on-site operators. Send a reply only when someone asks you." {
		t.Errorf("space onsite = %+v", onsite)
	}
	if !c.Unsorted.Paused || c.Unsorted.Agent.Folder != "/Users/me/work/notes" || c.Unsorted.Agent.ID != UnsortedID {
		t.Errorf("Unsorted = %+v", c.Unsorted)
	}
	text := c.Migration.Text()
	for _, want := range []string{
		"from version 2 to version 3",
		`Space "iso" uses the dry-run driver. To stop its sends and keep its agent, set paused = true`,
		"Unsorted held its messages (agent = ''); it is paused now",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("report has no %q:\n%s", want, text)
		}
	}
}

func TestMigrateV2SharedAndUnusedAgents(t *testing.T) {
	v2 := `version = 2
[[agents]]
id = 'claude-work'
driver = 'orca'
cli = 'claude'
workspace = '~/work'
primer = 'Summarise.'
[[agents]]
id = 'old'
[[schedules]]
id = 'always'
every = '15m'
[unsorted]
agent = 'claude-work'
schedule = 'always'
[[spaces]]
id = 'acme'
agent = 'claude-work'
schedule = 'always'
[[spaces]]
id = 'borealis'
agent = 'claude-work'
schedule = 'always'
`
	c, err := Parse([]byte(v2))
	if err != nil {
		t.Fatal(err)
	}
	acme, _ := c.Space("acme")
	borealis, _ := c.Space("borealis")
	if acme.Agent.Folder != "~/work" || borealis.Agent.Folder != "~/work" || acme.Instruction != "Summarise." || borealis.Instruction != "Summarise." {
		t.Fatalf("spaces = %+v %+v", acme, borealis)
	}
	if c.Unsorted.Paused || c.Unsorted.Agent.Title() != "orca · claude" || c.Unsorted.Instruction != "Summarise." {
		t.Errorf("Unsorted = %+v", c.Unsorted)
	}
	text := c.Migration.Text()
	for _, want := range []string{
		`Agent "claude-work": copied into the spaces acme, borealis and unsorted. A change of one space no longer changes the other.`,
		`Agent "old" was used by no space; i2a removed it. Its settings are in the backup file.`,
	} {
		if !strings.Contains(text, want) {
			t.Errorf("report has no %q:\n%s", want, text)
		}
	}
	// A change of one copy does not change the other.
	acme.Agent.Folder = "~/acme"
	if b, _ := c.Space("borealis"); b.Agent.Folder != "~/work" {
		t.Error("the change of acme changed borealis")
	}
}

func TestLoadWritesVersion2Backup(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	b, _ := os.ReadFile(filepath.Join("testdata", "v2_one_agent_each.toml"))
	if err := os.WriteFile(path, b, 0o600); err != nil {
		t.Fatal(err)
	}
	c, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if c.Migration == nil || c.Migration.Backup != path+".v2.bak" || !strings.Contains(c.Migration.Text(), path+".v2.bak") {
		t.Fatalf("migration = %+v", c.Migration)
	}
	if bak, err := os.ReadFile(path + ".v2.bak"); err != nil || string(bak) != string(b) {
		t.Errorf("backup differs: %v", err)
	}
	if info, err := os.Stat(path + ".v2.bak"); err != nil || info.Mode().Perm() != 0o600 {
		t.Errorf("backup mode = %v, %v", info, err)
	}
	again, err := Load(path)
	if err != nil || again.Migration != nil || len(again.Spaces) != 4 {
		t.Errorf("second load: migration = %+v, spaces %d, err = %v", again.Migration, len(again.Spaces), err)
	}
}
