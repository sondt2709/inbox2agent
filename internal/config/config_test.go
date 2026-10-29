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
	if c.Timing.Interval.D() != 15*time.Minute || c.Timing.Lookback.D() != 24*time.Hour {
		t.Errorf("timing defaults = %+v", c.Timing)
	}
	if len(c.Profiles) != 1 || c.Profiles[0].Driver != DriverDryRun {
		t.Errorf("default profiles = %+v", c.Profiles)
	}
	// A file from an earlier version has no min_urgency.
	old, err := Parse([]byte("[notify]\nenabled = true\n"))
	if err != nil || old.Notify.MinUrgency != "normal" {
		t.Errorf("min_urgency of an earlier file = %q, %v, want normal", old.Notify.MinUrgency, err)
	}
}

func TestValidationErrors(t *testing.T) {
	cases := []struct {
		name, toml, want string
	}{
		{"bad duration", "[timing]\ninterval = \"ten minutes\"\n", "is not a duration"},
		{"unknown profile", "[[profiles]]\nname = \"triage\"\n[[sources]]\nid = \"work\"\ntype = \"mattermost\"\nurl = \"https://x\"\nprofile = \"missing\"\n", `the profile "missing" does not exist`},
		{"unknown channel profile", "[[profiles]]\nname = \"triage\"\n[[sources]]\nid = \"work\"\ntype = \"mattermost\"\nurl = \"https://x\"\n[sources.channel_profiles]\nc1 = \"nope\"\n", `channel_profiles[c1]: the profile "nope" does not exist`},
		{"bad active hours", "[timing]\nactive_hours = \"9-17\"\n", "is not HH:MM-HH:MM"},
		{"inline too big", "[[profiles]]\nname = \"a\"\ninline_max_bytes = 70000\n", "is not between 1024 and 65536"},
		{"template syntax", "[[profiles]]\nname = \"a\"\nbatch = \"{{.Count\"\n", `profile "a".batch: template error`},
		{"duplicate profile", "[[profiles]]\nname = \"a\"\n[[profiles]]\nname = \"a\"\n", "the name is used two times"},
		{"orca needs workspace", "[[profiles]]\nname = \"a\"\ndriver = \"orca\"\n", "needs a workspace"},
		{"unknown field", "[timing]\nintervall = \"5m\"\n", "intervall"},
		{"bad min urgency", "[notify]\nmin_urgency = \"urgent\"\n", `notify.min_urgency: "urgent" is not high, normal or low`},
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
}

func TestSaveLoadRoundTripHasNoSecrets(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "config.toml")
	c := Default()
	s := NewMattermostSource("work", "https://chat.example.com/")
	s.Channels = []string{"c1", "c2"}
	s.Profile = "triage"
	c.Sources = append(c.Sources, s)
	c.Profiles[0].Primer = "line one\nline two"
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
	if got.Sources[0].URL != "https://chat.example.com" || len(got.Sources[0].Channels) != 2 {
		t.Errorf("source = %+v", got.Sources[0])
	}
	if got.Profiles[0].Primer != "line one\nline two" {
		t.Errorf("primer = %q", got.Profiles[0].Primer)
	}
	b, _ := os.ReadFile(path)
	if strings.Contains(strings.ToLower(string(b)), "token =") {
		t.Errorf("config file contains a token field:\n%s", b)
	}
}

func TestLoadMissing(t *testing.T) {
	_, err := Load(filepath.Join(t.TempDir(), "none.toml"))
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestActiveHours(t *testing.T) {
	at := func(h, m int) time.Time { return time.Date(2026, 1, 1, h, m, 0, 0, time.Local) }
	day, _ := ParseActiveHours("09:00-17:00")
	night, _ := ParseActiveHours("22:00-02:00")
	cases := []struct {
		a    ActiveHours
		t    time.Time
		want bool
	}{
		{day, at(9, 0), true}, {day, at(16, 59), true}, {day, at(17, 0), false}, {day, at(20, 0), false},
		{night, at(1, 0), true}, {night, at(23, 0), true}, {night, at(12, 0), false}, {night, at(2, 0), false},
		{ActiveHours{}, at(3, 0), true},
	}
	for i, tc := range cases {
		if got := tc.a.Contains(tc.t); got != tc.want {
			t.Errorf("case %d: Contains(%s) = %v, want %v", i, tc.t.Format("15:04"), got, tc.want)
		}
	}
	for _, bad := range []string{"9-17", "25:00-01:00", "09:00-09:00", "09:00"} {
		if _, err := ParseActiveHours(bad); err == nil {
			t.Errorf("ParseActiveHours(%q) returned no error", bad)
		}
	}
}
