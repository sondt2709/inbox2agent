package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// readmeV1 is the version 1 example of the README, as it was released.
const readmeV1 = `[timing]
interval = "15m"
lookback = "1d"
active_hours = "08:00-20:00"
retention = "7d"

[notify]
enabled = true
min_urgency = "normal"

[[sources]]
id = "work"
type = "mattermost"
url = "https://chat.example.com"
poll_every = "1m"
all_direct = true
mentions = true
channels = ["C1", "C2"]
profile = "triage"
[sources.channel_profiles]
"C2" = "reviewer"
"C9" = "reviewer"

[[profiles]]
name = "triage"
driver = "orca"
agent = "claude"
workspace = "~/notes"
session = "reuse"
payload = "pointer"
inline_max_bytes = 8000
message_max_chars = 2000
batch = "{{.Count}} from {{.Sources}} for {{.Profile}}"

[[profiles]]
name = "reviewer"
driver = "dry-run"
`

func TestMigrateV1ProfilesAndChannelRoutes(t *testing.T) {
	c, err := Parse([]byte(readmeV1))
	if err != nil {
		t.Fatal(err)
	}
	if c.Migration == nil {
		t.Fatal("no migration report")
	}
	if len(c.Agents) != 2 || c.Agents[0].ID != "triage" || c.Agents[0].CLI != "claude" || c.Agents[0].Workspace != "~/notes" || c.Agents[1].ID != "reviewer" {
		t.Errorf("agents = %+v", c.Agents)
	}
	if a, ok := c.Account("work"); !ok || a.Platform != PlatformMattermost || a.URL != "https://chat.example.com" || a.ReadEvery.D() != time.Minute {
		t.Errorf("account = %+v", a)
	}
	triage, _ := c.Space("triage")
	reviewer, _ := c.Space("reviewer")
	if len(triage.Conversations) != 1 || triage.Conversations[0].ID != "C1" || triage.Conversations[0].Read != ReadAll || triage.Agent != "triage" {
		t.Errorf("space triage = %+v", triage)
	}
	if len(reviewer.Conversations) != 2 || reviewer.Conversations[0].ID != "C2" || reviewer.Conversations[0].Read != ReadAll ||
		reviewer.Conversations[1].ID != "C9" || reviewer.Conversations[1].Read != ReadMentions {
		t.Errorf("space reviewer = %+v", reviewer)
	}
	if c.Unsorted.Agent != "triage" || c.Unsorted.Read != ReadMentions || c.Unsorted.Schedule != "default" {
		t.Errorf("Unsorted = %+v", c.Unsorted)
	}
	if got := c.Agents[0].Batch; got != "{{.Count}} from {{.Accounts}} for {{.Agent}}" {
		t.Errorf("batch template = %q", got)
	}
	if !strings.Contains(c.Migration.Text(), `Agent "triage": the templates use {{.Accounts}}`) {
		t.Errorf("report:\n%s", c.Migration.Text())
	}
}

func TestMigrateV1Timing(t *testing.T) {
	c, err := Parse([]byte(readmeV1))
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Schedules) != 1 || c.Schedules[0].ID != "default" || c.Schedules[0].Every.D() != 15*time.Minute || c.Schedules[0].Hours != "08:00-20:00" {
		t.Errorf("schedules = %+v", c.Schedules)
	}
	for _, s := range c.Spaces {
		if s.Schedule != "default" {
			t.Errorf("space %s uses %q", s.ID, s.Schedule)
		}
	}
	if c.Settings.Lookback.D() != 24*time.Hour || c.Settings.Retention.D() != 7*24*time.Hour {
		t.Errorf("settings = %+v", c.Settings)
	}
}

func TestMigrateV1ReportsChangedRoutes(t *testing.T) {
	c, err := Parse([]byte(`[[profiles]]
name = "triage"
[[profiles]]
name = "home"
[[sources]]
id = "work"
type = "mattermost"
url = "https://chat.example.com"
all_direct = true
mentions = true
profile = "triage"
[[sources]]
id = "phone"
type = "whatsapp"
all_direct = true
mentions = false
profile = "home"
[notify]
enabled = false
`))
	if err != nil {
		t.Fatal(err)
	}
	text := c.Migration.Text()
	for _, want := range []string{
		`Account "phone": the conversations that no space lists now go to Unsorted and the agent "triage", not "home"`,
		`Account "phone": mentions in channels and groups that no space lists now arrive too`,
	} {
		if !strings.Contains(text, want) {
			t.Errorf("report has no %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, `Account "work"`) {
		t.Errorf("the report names work, which keeps its route:\n%s", text)
	}
	if c.Notify.Enabled {
		t.Error("notify.enabled = false was lost")
	}
	if a, _ := c.Account("phone"); a.Platform != PlatformWhatsApp || a.ReadEvery.D() != 10*time.Second {
		t.Errorf("phone = %+v", a)
	}
}

func TestLoadWritesBackupAndVersion2(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte(readmeV1), 0o600); err != nil {
		t.Fatal(err)
	}
	c, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if c.Migration == nil || c.Migration.Backup != path+".v1.bak" || !strings.Contains(c.Migration.Text(), path+".v1.bak") {
		t.Fatalf("migration = %+v", c.Migration)
	}
	bak, err := os.ReadFile(path + ".v1.bak")
	if err != nil || string(bak) != readmeV1 {
		t.Errorf("backup = %q, %v", bak, err)
	}
	if info, err := os.Stat(path + ".v1.bak"); err != nil || info.Mode().Perm() != 0o600 {
		t.Errorf("backup mode = %v, %v", info, err)
	}
	b, _ := os.ReadFile(path)
	if !strings.Contains(string(b), "version = 2") || strings.Contains(string(b), "[[sources]]") {
		t.Errorf("new file:\n%s", b)
	}
	again, err := Load(path)
	if err != nil || again.Migration != nil {
		t.Errorf("second load: migration = %+v, err = %v", again.Migration, err)
	}
}
