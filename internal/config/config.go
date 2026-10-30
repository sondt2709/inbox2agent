// Package config defines the i2a configuration file: the schema, the defaults,
// loading, saving and validation.
package config

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"text/template"
	"time"

	toml "github.com/pelletier/go-toml/v2"
)

// Driver names.
const (
	DriverDryRun = "dry-run"
	DriverOrca   = "orca"
)

// Session modes for the orca driver.
const (
	SessionNew      = "new"
	SessionReuse    = "reuse"
	SessionWorktree = "worktree"
)

// Payload modes.
const (
	PayloadPointer = "pointer"
	PayloadInline  = "inline"
)

// Source types.
const (
	SourceMattermost = "mattermost"
	SourceWhatsApp   = "whatsapp"
)

// Limits for inline prompts. Orca passes prompts as command arguments.
const (
	MinInlineBytes = 1024
	MaxInlineBytes = 65536
)

// Config is the whole configuration file.
type Config struct {
	Timing   Timing    `toml:"timing"`
	Notify   Notify    `toml:"notify"`
	Sources  []Source  `toml:"sources"`
	Profiles []Profile `toml:"profiles"`
}

// Timing controls when i2a dispatches.
type Timing struct {
	Interval    Duration `toml:"interval" comment:"Time between dispatches."`
	Lookback    Duration `toml:"lookback" comment:"First-run window and maximum message age."`
	ActiveHours string   `toml:"active_hours" comment:"HH:MM-HH:MM in local time. Empty means always."`
	Retention   Duration `toml:"retention" comment:"Keep sent messages and digests for this time."`
}

// Notify controls the desktop notifications.
type Notify struct {
	Enabled    bool   `toml:"enabled" comment:"Show macOS notifications for new cards and failed dispatches."`
	MinUrgency string `toml:"min_urgency" comment:"The lowest card urgency that gives a notification: high, normal or low."`
}

// Source is one account on one service.
type Source struct {
	ID              string            `toml:"id"`
	Type            string            `toml:"type"`
	URL             string            `toml:"url"`
	PollEvery       Duration          `toml:"poll_every"`
	AllDirect       bool              `toml:"all_direct"`
	Mentions        bool              `toml:"mentions"`
	Channels        []string          `toml:"channels"`
	Profile         string            `toml:"profile"`
	ChannelProfiles map[string]string `toml:"channel_profiles,omitempty"`
}

// Profile tells i2a how to deliver a batch to an agent.
type Profile struct {
	Name            string `toml:"name"`
	Driver          string `toml:"driver"`
	Agent           string `toml:"agent"`
	Workspace       string `toml:"workspace"`
	Session         string `toml:"session"`
	Payload         string `toml:"payload"`
	InlineMaxBytes  int    `toml:"inline_max_bytes"`
	MessageMaxChars int    `toml:"message_max_chars"`
	Model           string `toml:"model,omitempty"`
	OrcaBin         string `toml:"orca_bin,omitempty"`
	Primer          string `toml:"primer,multiline,omitempty"`
	Batch           string `toml:"batch,multiline,omitempty"`
}

// Default returns a configuration with one dry-run profile and no sources.
func Default() *Config {
	c := &Config{
		Timing: Timing{
			Interval:  Duration(15 * time.Minute),
			Lookback:  Duration(24 * time.Hour),
			Retention: Duration(7 * 24 * time.Hour),
		},
		Notify:   Notify{Enabled: true, MinUrgency: "normal"},
		Profiles: []Profile{NewProfile("triage")},
	}
	return c
}

// NewProfile returns a profile with the default values.
func NewProfile(name string) Profile {
	return Profile{
		Name:            name,
		Driver:          DriverDryRun,
		Agent:           "claude",
		Session:         SessionReuse,
		Payload:         PayloadPointer,
		InlineMaxBytes:  8000,
		MessageMaxChars: 2000,
	}
}

// NewMattermostSource returns a Mattermost source with the default values.
func NewMattermostSource(id, url string) Source {
	return Source{
		ID:        id,
		Type:      SourceMattermost,
		URL:       strings.TrimRight(url, "/"),
		PollEvery: Duration(time.Minute),
		AllDirect: true,
		Mentions:  true,
	}
}

// NewWhatsAppSource returns a WhatsApp source with the default values.
// It reads a local file on each poll, so it polls more often than Mattermost.
func NewWhatsAppSource(id string) Source {
	return Source{
		ID:        id,
		Type:      SourceWhatsApp,
		PollEvery: Duration(10 * time.Second),
		AllDirect: true,
		Mentions:  true,
	}
}

// ErrNotFound is returned by Load when the file does not exist.
var ErrNotFound = errors.New("configuration file not found")

// Load reads and validates the configuration file.
func Load(path string) (*Config, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	c, err := Parse(b)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return c, nil
}

// Parse decodes and validates configuration bytes.
func Parse(b []byte) (*Config, error) {
	c := Default()
	c.Profiles = nil
	dec := toml.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(c); err != nil {
		var serr *toml.StrictMissingError
		if errors.As(err, &serr) {
			return nil, fmt.Errorf("unknown field:\n%s", serr.String())
		}
		var derr *toml.DecodeError
		if errors.As(err, &derr) {
			row, col := derr.Position()
			return nil, fmt.Errorf("line %d column %d: %s", row, col, derr.Error())
		}
		return nil, err
	}
	c.applyDefaults()
	if err := c.Validate(); err != nil {
		return nil, err
	}
	return c, nil
}

func (c *Config) applyDefaults() {
	d := Default()
	if c.Timing.Interval == 0 {
		c.Timing.Interval = d.Timing.Interval
	}
	if c.Timing.Lookback == 0 {
		c.Timing.Lookback = d.Timing.Lookback
	}
	if c.Timing.Retention == 0 {
		c.Timing.Retention = d.Timing.Retention
	}
	for i := range c.Sources {
		if c.Sources[i].PollEvery == 0 {
			c.Sources[i].PollEvery = Duration(time.Minute)
		}
	}
	if len(c.Profiles) == 0 {
		c.Profiles = d.Profiles
	}
	for i := range c.Profiles {
		p := &c.Profiles[i]
		def := NewProfile(p.Name)
		if p.Driver == "" {
			p.Driver = def.Driver
		}
		if p.Agent == "" {
			p.Agent = def.Agent
		}
		if p.Session == "" {
			p.Session = def.Session
		}
		if p.Payload == "" {
			p.Payload = def.Payload
		}
		if p.InlineMaxBytes == 0 {
			p.InlineMaxBytes = def.InlineMaxBytes
		}
		if p.MessageMaxChars == 0 {
			p.MessageMaxChars = def.MessageMaxChars
		}
	}
}

// Save validates the configuration and writes it with mode 0600.
func Save(path string, c *Config) error {
	if err := c.Validate(); err != nil {
		return err
	}
	b, err := toml.Marshal(c)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	header := []byte("# i2a configuration. Edit with `i2a config` or by hand.\n# Tokens are not in this file. They are in the secret store.\n\n")
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(header, b...), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// Profile returns the profile with this name.
func (c *Config) Profile(name string) (*Profile, bool) {
	for i := range c.Profiles {
		if c.Profiles[i].Name == name {
			return &c.Profiles[i], true
		}
	}
	return nil, false
}

// Source returns the source with this ID.
func (c *Config) Source(id string) (*Source, bool) {
	for i := range c.Sources {
		if c.Sources[i].ID == id {
			return &c.Sources[i], true
		}
	}
	return nil, false
}

var nameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,31}$`)

// ValidationError lists all problems in a configuration.
type ValidationError struct{ Problems []string }

func (e *ValidationError) Error() string {
	return "invalid configuration:\n  - " + strings.Join(e.Problems, "\n  - ")
}

// Validate checks the configuration and returns a *ValidationError.
func (c *Config) Validate() error {
	var p []string
	add := func(format string, a ...any) { p = append(p, fmt.Sprintf(format, a...)) }

	if c.Timing.Interval < Duration(5*time.Second) {
		add("timing.interval: %q is shorter than 5s", c.Timing.Interval)
	}
	if c.Timing.Lookback < Duration(time.Minute) {
		add("timing.lookback: %q is shorter than 1m", c.Timing.Lookback)
	}
	if _, err := ParseActiveHours(c.Timing.ActiveHours); err != nil {
		add("timing.active_hours: %v", err)
	}
	switch c.Notify.MinUrgency {
	case "high", "normal", "low":
	default:
		add("notify.min_urgency: %q is not high, normal or low", c.Notify.MinUrgency)
	}
	if len(c.Profiles) == 0 {
		add("profiles: at least one profile is necessary")
	}

	profiles := map[string]bool{}
	for i, pr := range c.Profiles {
		at := fmt.Sprintf("profiles[%d]", i)
		if !nameRe.MatchString(pr.Name) {
			add("%s.name: %q must be 1-32 lowercase letters, digits, - or _", at, pr.Name)
		} else {
			at = fmt.Sprintf("profile %q", pr.Name)
		}
		if profiles[pr.Name] {
			add("%s: the name is used two times", at)
		}
		profiles[pr.Name] = true
		switch pr.Driver {
		case DriverDryRun, DriverOrca:
		default:
			add("%s.driver: %q is not dry-run or orca", at, pr.Driver)
		}
		switch pr.Session {
		case SessionNew, SessionReuse, SessionWorktree:
		default:
			add("%s.session: %q is not new, reuse or worktree", at, pr.Session)
		}
		switch pr.Payload {
		case PayloadPointer, PayloadInline:
		default:
			add("%s.payload: %q is not pointer or inline", at, pr.Payload)
		}
		if pr.InlineMaxBytes < MinInlineBytes || pr.InlineMaxBytes > MaxInlineBytes {
			add("%s.inline_max_bytes: %d is not between %d and %d", at, pr.InlineMaxBytes, MinInlineBytes, MaxInlineBytes)
		}
		if pr.MessageMaxChars < 100 || pr.MessageMaxChars > 20000 {
			add("%s.message_max_chars: %d is not between 100 and 20000", at, pr.MessageMaxChars)
		}
		if pr.Driver == DriverOrca {
			if pr.Workspace == "" {
				add("%s.workspace: the orca driver needs a workspace folder", at)
			}
			if pr.Agent == "" {
				add("%s.agent: the orca driver needs an agent", at)
			}
		}
		for name, text := range map[string]string{"primer": pr.Primer, "batch": pr.Batch} {
			if text == "" {
				continue
			}
			if _, err := template.New(name).Option("missingkey=error").Parse(text); err != nil {
				add("%s.%s: template error: %v", at, name, err)
			}
		}
	}

	ids := map[string]bool{}
	for i, s := range c.Sources {
		at := fmt.Sprintf("sources[%d]", i)
		if !nameRe.MatchString(s.ID) {
			add("%s.id: %q must be 1-32 lowercase letters, digits, - or _", at, s.ID)
		} else {
			at = fmt.Sprintf("source %q", s.ID)
		}
		if ids[s.ID] {
			add("%s: the ID is used two times", at)
		}
		ids[s.ID] = true
		switch s.Type {
		case SourceMattermost:
			if !strings.HasPrefix(s.URL, "https://") && !strings.HasPrefix(s.URL, "http://") {
				add("%s.url: %q must start with https:// or http://", at, s.URL)
			}
		case SourceWhatsApp:
		default:
			add("%s.type: %q is not supported (use mattermost or whatsapp)", at, s.Type)
		}
		if s.PollEvery < Duration(5*time.Second) {
			add("%s.poll_every: %q is shorter than 5s", at, s.PollEvery)
		}
		if s.Profile != "" && !profiles[s.Profile] {
			add("%s.profile: the profile %q does not exist", at, s.Profile)
		}
		for ch, pr := range s.ChannelProfiles {
			if !profiles[pr] {
				add("%s.channel_profiles[%s]: the profile %q does not exist", at, ch, pr)
			}
		}
	}

	if len(p) > 0 {
		return &ValidationError{Problems: p}
	}
	return nil
}

// ActiveHours is a daily time range in minutes after midnight.
// When End is before Start, the range crosses midnight.
type ActiveHours struct {
	Start, End int
	Set        bool
}

// Contains reports whether the local time t is in the range.
// An unset range contains all times.
func (a ActiveHours) Contains(t time.Time) bool {
	if !a.Set {
		return true
	}
	m := t.Hour()*60 + t.Minute()
	if a.Start <= a.End {
		return m >= a.Start && m < a.End
	}
	return m >= a.Start || m < a.End
}

// ParseActiveHours parses "HH:MM-HH:MM". An empty string is an unset range.
func ParseActiveHours(s string) (ActiveHours, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return ActiveHours{}, nil
	}
	a, b, ok := strings.Cut(s, "-")
	if !ok {
		return ActiveHours{}, fmt.Errorf("%q is not HH:MM-HH:MM", s)
	}
	start, err1 := parseClock(a)
	end, err2 := parseClock(b)
	if err1 != nil || err2 != nil {
		return ActiveHours{}, fmt.Errorf("%q is not HH:MM-HH:MM", s)
	}
	if start == end {
		return ActiveHours{}, fmt.Errorf("%q has the same start and end", s)
	}
	return ActiveHours{Start: start, End: end, Set: true}, nil
}

func parseClock(s string) (int, error) {
	h, m, ok := strings.Cut(strings.TrimSpace(s), ":")
	if !ok || len(h) != 2 || len(m) != 2 {
		return 0, errors.New("bad clock")
	}
	hh, err1 := strconv.Atoi(h)
	mm, err2 := strconv.Atoi(m)
	if err1 != nil || err2 != nil || hh < 0 || hh > 23 || mm < 0 || mm > 59 {
		return 0, errors.New("bad clock")
	}
	return hh*60 + mm, nil
}
