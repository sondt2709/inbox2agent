//go:build e2e

package e2e

import (
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/sondt2709/inbox2agent/internal/config"
)

// S37: a version 1 file (sources, profiles, timing) is converted when i2a
// loads it. The old text stays in config.toml.v1.bak, and the report says
// what changed.
func TestS37MigrateVersion1(t *testing.T) {
	w := mm.NewWorld(t)
	p := newPerson(t, "S37")
	v1 := `[timing]
interval = "10m"
active_hours = "08:00-20:00"

[[sources]]
id = "work"
type = "mattermost"
url = "` + mm.URL + `"
all_direct = true
mentions = true
channels = ["` + w.Selected + `"]
profile = "triage"
[sources.channel_profiles]
"` + w.Selected + `" = "reviewer"

[[profiles]]
name = "triage"
driver = "dry-run"
batch = "{{.Count}} new messages from {{.Sources}}"

[[profiles]]
name = "reviewer"
driver = "dry-run"
`
	if err := os.WriteFile(p.configFile(), []byte(v1), 0o600); err != nil {
		t.Fatal(err)
	}
	s, code := p.run("status")
	if code != 0 || !strings.Contains(s, "i2a converted the configuration to version 2") || !strings.Contains(s, "config.toml.v1.bak") {
		t.Fatalf("status: exit %d:\n%s", code, s)
	}
	for _, want := range []string{"templates use {{.Accounts}}", "reviewer  reviewer", "unsorted  Unsorted   triage"} {
		if !strings.Contains(strings.Join(strings.Fields(s), " "), strings.Join(strings.Fields(want), " ")) {
			t.Errorf("status has no %q:\n%s", want, s)
		}
	}
	bak, err := os.ReadFile(p.configFile() + ".v1.bak")
	if err != nil || string(bak) != v1 {
		t.Fatalf("backup = %q, %v", bak, err)
	}
	c := p.config()
	if c.Version != 2 || c.Route("work", w.Selected) != "reviewer" || c.Schedules[0].Every.String() != "10m" ||
		c.Schedules[0].Hours != "08:00-20:00" || c.Agents[0].Batch != "{{.Count}} new messages from {{.Accounts}}" {
		t.Fatalf("converted config = %+v", c)
	}
	// The second load has nothing to convert.
	if s, _ := p.run("status"); strings.Contains(s, "converted") {
		t.Errorf("the second load converted again:\n%s", s)
	}
}

// S38: one account feeds two spaces with two agents. The channel goes to the
// space Review and its agent; the direct message stays in Unsorted.
func TestS38TwoSpacesFromOneAccount(t *testing.T) {
	w := mm.NewWorld(t)
	p := newPerson(t, "S38")
	p.addAccount(w)
	p.editConfig(func(c *config.Config) {
		c.Agents = append(c.Agents, config.NewAgent("reviewer"))
		c.Spaces = append(c.Spaces, config.Space{ID: "review", Name: "Review", Agent: "reviewer", Schedule: "always"})
	})
	// An agent adds the channel to Review by hand: Review is the last [[spaces]].
	s := p.writeConfigText(func(text string) string {
		return text + "\n[[spaces.conversations]]\naccount = 'work'\nid = '" + w.Selected + "'\nread = 'all'\nlabel = 'Releases'\n"
	})
	// The terminal can break the line in a word, so compare without spaces.
	squash := func(s string) string { return strings.Join(strings.Fields(s), "") }
	if !strings.Contains(squash(s), squash("is valid: 1 account, 2 agents, 1 schedule, 1 space and Unsorted, 1 conversation")) {
		t.Fatalf("config check:\n%s", s)
	}
	s, code := p.run("conversations --account work --json")
	if code != 0 || !regexp.MustCompile(`"id": "`+regexp.QuoteMeta(w.Selected)+`",\s+"kind": "channel",\s+"name": "[^"]+",\s+"label": "Releases",\s+"space": "review",\s+"read": "all"`).MatchString(s) {
		t.Fatalf("conversations --json: exit %d:\n%s", code, s)
	}
	s, code = p.run("conversations --account work")
	if code != 0 || !regexp.MustCompile(regexp.QuoteMeta(w.Selected)+`\s+channel\s+Releases\s+Review\s+all`).MatchString(s) {
		t.Fatalf("conversations: exit %d:\n%s", code, s)
	}
	w.Post(t, w.Bob, w.Selected, "S38 release notes are ready")
	w.Post(t, w.Bob, w.Direct, "S38 lunch at noon?")
	s, code = p.run("run --once")
	if code != 0 || !strings.Contains(s, "batch of Unsorted to triage (dry-run, 1 messages)") ||
		!strings.Contains(s, "batch of Review to reviewer (dry-run, 1 messages)") {
		t.Fatalf("run --once: exit %d:\n%s", code, s)
	}
	var all []string
	for _, d := range p.digests() {
		all = append(all, texts(readDigest(t, d))...)
	}
	slices.Sort(all)
	if !slices.Equal(all, []string{"S38 lunch at noon?", "S38 release notes are ready"}) {
		t.Fatalf("digest texts = %q", all)
	}
}

// S39: Unsorted holds the messages of a conversation that no space lists.
// When the user moves the conversation to a space, its waiting messages go to
// the agent of that space.
func TestS39SortAHeldConversation(t *testing.T) {
	w := mm.NewWorld(t)
	p := newPerson(t, "S39")
	p.addAccount(w)
	p.editConfig(func(c *config.Config) {
		c.Unsorted.Agent = ""
		c.Spaces = append(c.Spaces, config.Space{ID: "team", Name: "Team", Agent: "triage", Schedule: "always"})
	})
	w.Post(t, w.Bob, w.Direct, "S39 can you review my PR?")
	s, code := p.run("run --once")
	if code != 0 || !strings.Contains(s, "collected 1 new messages") || !strings.Contains(s, "no batch to send") {
		t.Fatalf("first run: exit %d:\n%s", code, s)
	}
	s, _ = p.run("status")
	if !regexp.MustCompile(`unsorted\s+Unsorted\s+-\s+always\s+hold: does not send\s+1 waiting`).MatchString(s) {
		t.Fatalf("status:\n%s", s)
	}
	// An agent sorts the conversation by hand: Team is the last [[spaces]].
	p.writeConfigText(func(text string) string {
		return text + "\n[[spaces.conversations]]\naccount = 'work'\nid = '" + w.Direct + "'\nread = 'all'\n"
	})
	s, code = p.run("run --once")
	if code != 0 || !strings.Contains(s, "batch of Team to triage (dry-run, 1 messages)") {
		t.Fatalf("second run: exit %d:\n%s", code, s)
	}
	if got := texts(readDigest(t, p.digests()[0])); !slices.Equal(got, []string{"S39 can you review my PR?"}) {
		t.Fatalf("digest texts = %q", got)
	}
}

// S40: the TUI is open. Another process adds a space to config.toml by hand.
// The TUI shows it, and a save of the Settings tab keeps it.
func TestS40EditFromAnotherProcess(t *testing.T) {
	p := newPerson(t, "S40")
	c := config.Default()
	c.Spaces = []config.Space{{ID: "team", Name: "Team", Agent: "triage", Schedule: "always"}}
	if err := config.Save(p.configFile(), c); err != nil {
		t.Fatal(err)
	}
	tm := p.term("config")
	tm.WaitFor(`5 Spaces`, 30*time.Second)
	f, err := os.OpenFile(p.configFile(), os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.WriteString("\n[[spaces]]\nid = 'side-project'\nname = 'Side project'\nagent = 'triage'\nschedule = 'always'\n")
	f.Close()
	if err != nil {
		t.Fatal(err)
	}
	s := tm.WaitFor(`Side project`, 5*time.Second)
	if !strings.Contains(s, "changed in another process") {
		t.Errorf("the status line does not say that the file changed:\n%s", s)
	}
	tm.Press("8", `Lookback\s+1d`, 5*time.Second)
	tm.Press("Enter", `▌ Lookback`, 5*time.Second)
	tm.Replace("2d")
	tm.Press("C-s", `Saved the settings`, 5*time.Second)
	tm.Press("q", `Saved /`, 5*time.Second)
	c = p.config()
	if c.Settings.Lookback.String() != "2d" {
		t.Errorf("lookback = %s, want 2d", c.Settings.Lookback)
	}
	if _, ok := c.Space("side-project"); !ok {
		raw, _ := os.ReadFile(p.configFile())
		t.Fatalf("the save removed the space of the other process:\n%s", raw)
	}
}
