package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDurationRoundTrip(t *testing.T) {
	cases := map[string]string{
		"15m": "15m", "2h": "2h", "1h30m": "1h30m", "30s": "30s",
		"7d": "7d", "168h": "7d", "90s": "1m30s", "36h": "36h",
	}
	for in, want := range cases {
		d, err := ParseDuration(in)
		if err != nil {
			t.Fatalf("ParseDuration(%q): %v", in, err)
		}
		if got := d.String(); got != want {
			t.Errorf("ParseDuration(%q).String() = %q, want %q", in, got, want)
		}
	}
	if _, err := ParseDuration("ten minutes"); err == nil {
		t.Error("ParseDuration(\"ten minutes\") returned no error")
	}
}

func TestParseAppliesDefaults(t *testing.T) {
	c, err := Parse([]byte(""))
	if err != nil {
		t.Fatal(err)
	}
	if c.Version != Version || c.Settings.Lookback.D() != 24*time.Hour || c.Settings.Retention.D() != 7*24*time.Hour {
		t.Errorf("defaults = %+v", c)
	}
	if len(c.Agents) != 1 || c.Agents[0].Driver != DriverDryRun || len(c.Schedules) != 1 || c.Schedules[0].Every.D() != 15*time.Minute {
		t.Errorf("default agents and schedules = %+v %+v", c.Agents, c.Schedules)
	}
	if c.Unsorted.Read != ReadMentions || c.Unsorted.Schedule != c.Schedules[0].ID {
		t.Errorf("default Unsorted = %+v", c.Unsorted)
	}
	if c.Migration != nil {
		t.Error("an empty file is not version 1")
	}
}

const v2Base = `version = 2
[[accounts]]
id = "work"
platform = "mattermost"
url = "https://chat.example.com"
[[accounts]]
id = "phone"
platform = "whatsapp"
[[agents]]
id = "claude-work"
[[schedules]]
id = "work-hours"
every = "15m"
days = ["mon", "tue", "wed", "thu", "fri"]
hours = "08:00-18:00"
[unsorted]
agent = ""
read = "mentions"
`

func TestValidationErrors(t *testing.T) {
	space := func(extra string) string {
		return v2Base + "[[spaces]]\nid = \"acme\"\nname = \"Acme\"\nagent = \"claude-work\"\nschedule = \"work-hours\"\n" + extra
	}
	cases := []struct {
		name, toml, want string
	}{
		{"bad duration", v2Base + "[[schedules]]\nid = \"x\"\nevery = \"ten minutes\"\n", "is not a duration"},
		{"short every", v2Base + "[[schedules]]\nid = \"x\"\nevery = \"30s\"\n", `schedule "x".every: "30s" is shorter than 1m`},
		{"bad day", v2Base + "[[schedules]]\nid = \"x\"\nevery = \"1h\"\ndays = [\"monday\"]\n", `"monday" is not one of mon`},
		{"bad hours", v2Base + "[[schedules]]\nid = \"x\"\nevery = \"1h\"\nhours = \"9-17\"\n", "is not HH:MM-HH:MM"},
		{"unknown agent", v2Base + "[[spaces]]\nid = \"acme\"\nagent = \"missing\"\nschedule = \"work-hours\"\n", `the agent "missing" does not exist`},
		{"unknown schedule", v2Base + "[[spaces]]\nid = \"acme\"\nagent = \"claude-work\"\nschedule = \"nope\"\n", `the schedule "nope" does not exist`},
		{"unknown unsorted agent", strings.Replace(v2Base, "agent = \"\"", "agent = \"gone\"", 1), `unsorted.agent: the agent "gone" does not exist`},
		{"two spaces", space("[[spaces.conversations]]\naccount = \"work\"\nid = \"C1\"\n[[unsorted.conversations]]\naccount = \"work\"\nid = \"C1\"\nread = \"off\"\n"), `conversation work/C1 is in Unsorted and in space "acme"`},
		{"two times in one space", space("[[spaces.conversations]]\naccount = \"work\"\nid = \"C1\"\n[[spaces.conversations]]\naccount = \"work\"\nid = \"C1\"\n"), `conversation work/C1 is two times in space "acme"`},
		{"unknown account", space("[[spaces.conversations]]\naccount = \"slack\"\nid = \"C1\"\n"), `the account "slack" does not exist`},
		{"bad read", space("[[spaces.conversations]]\naccount = \"work\"\nid = \"C1\"\nread = \"some\"\n"), `"some" is not all, mentions or off`},
		{"reserved id", v2Base + "[[spaces]]\nid = \"unsorted\"\nagent = \"claude-work\"\nschedule = \"work-hours\"\n", "is for the built-in space"},
		{"same space name", space("[[spaces]]\nid = \"acme2\"\nname = \"acme\"\nagent = \"claude-work\"\nschedule = \"work-hours\"\n"), `the name "acme" is used two times`},
		{"route key in an account", strings.Replace(v2Base, "platform = \"whatsapp\"", "platform = \"whatsapp\"\nprofile = \"triage\"", 1), "unknown field"},
		{"old template field", v2Base + "[[agents]]\nid = \"a\"\nbatch = \"{{.Count}} from {{.Sources}}\"\n", "the field .Sources is gone; use .Accounts"},
		{"template syntax", v2Base + "[[agents]]\nid = \"a\"\nbatch = \"{{.Count\"\n", `agent "a".batch: template error`},
		{"inline too big", v2Base + "[[agents]]\nid = \"a\"\ninline_max_bytes = 70000\n", "is not between 1024 and 65536"},
		{"orca needs workspace", v2Base + "[[agents]]\nid = \"a\"\ndriver = \"orca\"\n", "needs a workspace"},
		{"duplicate agent", v2Base + "[[agents]]\nid = \"claude-work\"\n", "the ID is used two times"},
		{"unknown platform", v2Base + "[[accounts]]\nid = \"chat\"\nplatform = \"slack\"\n", `account "chat".platform: "slack" is not supported`},
		{"mattermost needs url", v2Base + "[[accounts]]\nid = \"w2\"\nplatform = \"mattermost\"\n", `account "w2".url: "" must start with https://`},
		{"bad min urgency", v2Base + "[notify]\nmin_urgency = \"urgent\"\n", `notify.min_urgency: "urgent" is not high, normal or low`},
		{"newer version", "version = 3\n", "this i2a supports 2"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Parse([]byte(tc.toml))
			if err == nil {
				t.Fatalf("Parse returned no error, want %q", tc.want)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %q, want it to contain %q", err, tc.want)
			}
		})
	}
	if _, err := Parse([]byte(v2Base)); err != nil {
		t.Errorf("the base file is not valid: %v", err)
	}
}

func TestSaveLoadRoundTripHasNoSecrets(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "config.toml")
	c := Default()
	c.Accounts = append(c.Accounts, NewMattermostAccount("work", "https://chat.example.com/"), NewWhatsAppAccount("phone"))
	c.Accounts[1].Name = "Personal phone"
	c.Agents[0].Primer = "line one\nline two"
	c.Spaces = []Space{{ID: "acme", Name: "Acme", Agent: "triage", Schedule: "always", Conversations: []Conversation{
		{Account: "work", ID: "c1", Name: "release", Label: "Release channel", Read: ReadAll},
		{Account: "phone", ID: "g1@g.us", Read: ReadMentions},
	}}}
	if err := Save(path, c); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Errorf("mode = %o, want 600", mode)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Accounts[0].URL != "https://chat.example.com" || got.Accounts[1].Title() != "Personal phone" {
		t.Errorf("accounts = %+v", got.Accounts)
	}
	if s, ok := got.Space("acme"); !ok || len(s.Conversations) != 2 || s.Conversations[0].Title() != "Release channel" {
		t.Errorf("space = %+v", s)
	}
	if got.Agents[0].Primer != "line one\nline two" {
		t.Errorf("primer = %q", got.Agents[0].Primer)
	}
	b, _ := os.ReadFile(path)
	if !strings.Contains(string(b), "version = 2") || strings.Contains(strings.ToLower(string(b)), "token =") {
		t.Errorf("config file:\n%s", b)
	}
}

func TestLoadMissing(t *testing.T) {
	_, err := Load(filepath.Join(t.TempDir(), "none.toml"))
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestHours(t *testing.T) {
	at := func(h, m int) time.Time { return time.Date(2026, 1, 1, h, m, 0, 0, time.Local) }
	day, _ := ParseHours("09:00-17:00")
	night, _ := ParseHours("22:00-02:00")
	cases := []struct {
		a    Hours
		t    time.Time
		want bool
	}{
		{day, at(9, 0), true}, {day, at(16, 59), true}, {day, at(17, 0), false}, {day, at(20, 0), false},
		{night, at(1, 0), true}, {night, at(23, 0), true}, {night, at(12, 0), false}, {night, at(2, 0), false},
		{Hours{}, at(3, 0), true},
	}
	for i, tc := range cases {
		if got := tc.a.Contains(tc.t); got != tc.want {
			t.Errorf("case %d: Contains(%s) = %v, want %v", i, tc.t.Format("15:04"), got, tc.want)
		}
	}
	for _, bad := range []string{"9-17", "25:00-01:00", "09:00-09:00", "09:00"} {
		if _, err := ParseHours(bad); err == nil {
			t.Errorf("ParseHours(%q) returned no error", bad)
		}
	}
}

func TestScheduleNext(t *testing.T) {
	// 2026-10-29 is a Thursday.
	at := func(day, h, m int) time.Time { return time.Date(2026, 10, day, h, m, 0, 0, time.Local) }
	work := Schedule{Every: Duration(15 * time.Minute), Days: []string{"mon", "tue", "wed", "thu", "fri"}, Hours: "08:00-18:00"}
	night := Schedule{Every: Duration(time.Hour), Hours: "22:00-02:00"}
	cases := []struct {
		name  string
		s     Schedule
		after time.Time
		want  time.Time
	}{
		{"spec: Thursday 14:37", work, at(29, 14, 37), at(29, 14, 45)},
		{"exact send time is not next", work, at(29, 14, 45), at(29, 15, 0)},
		{"before the hours", work, at(29, 6, 0), at(29, 8, 0)},
		{"spec: Friday 18:05", work, at(30, 18, 5), at(31+2, 8, 0)}, // Monday 2 November
		{"last time inside the hours", work, at(29, 17, 40), at(29, 17, 45)},
		{"all day", NewSchedule("always"), at(29, 23, 50), at(30, 0, 0)},
		{"across midnight, after midnight", night, at(30, 0, 30), at(30, 1, 0)},
		{"across midnight, before the start", night, at(29, 12, 0), at(29, 22, 0)},
		{"two hours", Schedule{Every: Duration(2 * time.Hour), Hours: "18:00-22:00"}, at(29, 20, 0), at(29, 22, 0).Add(20 * time.Hour)},
	}
	for _, tc := range cases {
		if got := tc.s.Next(tc.after); !got.Equal(tc.want) {
			t.Errorf("%s: Next(%s) = %s, want %s", tc.name, tc.after.Format("Mon 15:04"), got.Format("Mon 02 15:04"), tc.want.Format("Mon 02 15:04"))
		}
	}
	if got := (Schedule{Every: Duration(time.Hour), Days: []string{}}).Next(at(29, 1, 0)); got.IsZero() {
		t.Error("an empty day list must mean every day")
	}
	if got := work.Text(); got != "every 15m · Mon–Fri · 08:00–18:00" {
		t.Errorf("Text = %q", got)
	}
}

func TestRouteLevelAndMove(t *testing.T) {
	c, err := Parse([]byte(v2Base + `[[spaces]]
id = "acme"
name = "Acme"
agent = "claude-work"
schedule = "work-hours"
[[spaces.conversations]]
account = "work"
id = "C1"
name = "release"
[[unsorted.conversations]]
account = "phone"
id = "g9@g.us"
read = "off"
`))
	if err != nil {
		t.Fatal(err)
	}
	if c.Route("work", "C1") != "acme" || c.Level("work", "C1") != ReadAll {
		t.Errorf("C1: %s %s", c.Route("work", "C1"), c.Level("work", "C1"))
	}
	if c.Route("work", "C2") != UnsortedID || c.Level("work", "C2") != ReadMentions {
		t.Errorf("unlisted C2: %s %s", c.Route("work", "C2"), c.Level("work", "C2"))
	}
	if lv := c.Levels("phone"); lv["g9@g.us"] != ReadOff || len(lv) != 1 {
		t.Errorf("levels of phone = %v", lv)
	}

	// Move an off chat out of Unsorted: it gets the level all.
	if err := c.SetConversation("phone", "g9@g.us", "acme", "", "Football"); err != nil {
		t.Fatal(err)
	}
	if c.Route("phone", "g9@g.us") != "acme" || c.Level("phone", "g9@g.us") != ReadAll || len(c.Unsorted.Conversations) != 0 {
		t.Errorf("after the move: %s %s %+v", c.Route("phone", "g9@g.us"), c.Level("phone", "g9@g.us"), c.Unsorted)
	}
	// A label survives a refresh of the platform name.
	if err := c.SetLabel("work", "C1", "Release channel"); err != nil {
		t.Fatal(err)
	}
	if err := c.SetConversation("work", "C1", "acme", "", "release-v2"); err != nil {
		t.Fatal(err)
	}
	cv, _, _ := c.Conversation("work", "C1")
	if cv.Title() != "Release channel" || cv.Name != "release-v2" {
		t.Errorf("conversation = %+v", cv)
	}
	if err := c.Validate(); err != nil {
		t.Errorf("after the changes: %v", err)
	}
	if err := c.SetConversation("work", "C1", "nope", "", ""); err == nil {
		t.Error("a move to a missing space returned no error")
	}
}

func TestReplaceAgentAndSchedule(t *testing.T) {
	c := Default()
	c.Agents = append(c.Agents, NewAgent("claude-atlas"))
	c.Spaces = []Space{
		{ID: "acme", Agent: "triage", Schedule: "always"},
		{ID: "borealis", Agent: "triage", Schedule: "always"},
		{ID: "atlas", Agent: "claude-atlas", Schedule: "always"},
	}
	if got := c.SpacesUsing("agent", "triage"); len(got) != 3 || got[0].ID != UnsortedID {
		t.Errorf("users of triage = %+v", got)
	}
	c.Replace("agent", "triage", "claude-atlas")
	if c.Unsorted.Agent != "claude-atlas" || c.Spaces[0].Agent != "claude-atlas" || c.Spaces[1].Agent != "claude-atlas" {
		t.Errorf("after the replace: %+v %+v", c.Unsorted, c.Spaces)
	}
	c.Unsorted.Agent = ""
	if got := c.SpacesUsing("schedule", "always"); len(got) != 3 {
		t.Errorf("users of always = %d, want 3 (Unsorted on hold does not use it)", len(got))
	}
}

func TestNewID(t *testing.T) {
	used := map[string]bool{"company-b": true}
	if got := NewID("Company B", func(s string) bool { return used[s] }); got != "company-b-2" {
		t.Errorf("NewID = %q", got)
	}
	if got := NewID("  Dự án Ánh sáng!! ", func(string) bool { return false }); got != "d-n-nh-s-ng" {
		t.Errorf("NewID = %q", got)
	}
	if got := NewID("Unsorted", func(string) bool { return false }); got == UnsortedID {
		t.Error("NewID returned the reserved ID")
	}
}
