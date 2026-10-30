package config

import (
	"bytes"
	"fmt"
	"regexp"
	"slices"
	"time"

	toml "github.com/pelletier/go-toml/v2"
)

// Migration is the report of the conversion of a version 1 file.
type Migration struct {
	Backup string   // the path of the old file; set by Load
	Notes  []string // each change in behavior, one sentence each
}

// Text returns the report for a person.
func (m *Migration) Text() string {
	var b bytes.Buffer
	b.WriteString("i2a converted the configuration to version 2 (accounts, conversations, spaces, agents, schedules).\n")
	if m.Backup != "" {
		fmt.Fprintf(&b, "The old file is %s.\n", m.Backup)
	}
	for _, n := range m.Notes {
		b.WriteString("- " + n + "\n")
	}
	return b.String()
}

// The version 1 schema, as it was released.
type v1Config struct {
	Timing   v1Timing    `toml:"timing"`
	Notify   Notify      `toml:"notify"`
	Sources  []v1Source  `toml:"sources"`
	Profiles []v1Profile `toml:"profiles"`
}

type v1Timing struct {
	Interval    Duration `toml:"interval"`
	Lookback    Duration `toml:"lookback"`
	ActiveHours string   `toml:"active_hours"`
	Retention   Duration `toml:"retention"`
}

type v1Source struct {
	ID              string            `toml:"id"`
	Type            string            `toml:"type"`
	URL             string            `toml:"url"`
	PollEvery       Duration          `toml:"poll_every"`
	AllDirect       bool              `toml:"all_direct"`
	Mentions        bool              `toml:"mentions"`
	Channels        []string          `toml:"channels"`
	Profile         string            `toml:"profile"`
	ChannelProfiles map[string]string `toml:"channel_profiles"`
}

type v1Profile struct {
	Name            string `toml:"name"`
	Driver          string `toml:"driver"`
	Agent           string `toml:"agent"`
	Workspace       string `toml:"workspace"`
	Session         string `toml:"session"`
	Payload         string `toml:"payload"`
	InlineMaxBytes  int    `toml:"inline_max_bytes"`
	MessageMaxChars int    `toml:"message_max_chars"`
	Model           string `toml:"model"`
	OrcaBin         string `toml:"orca_bin"`
	Primer          string `toml:"primer"`
	Batch           string `toml:"batch"`
}

// isV1 reports whether the bytes are a version 1 file: no version key, and a
// key that only version 1 has.
func isV1(b []byte) (bool, error) {
	var keys map[string]any
	if err := toml.Unmarshal(b, &keys); err != nil {
		return false, decodeError(err)
	}
	if _, ok := keys["version"]; ok {
		return false, nil
	}
	for _, k := range []string{"sources", "profiles", "timing"} {
		if _, ok := keys[k]; ok {
			return true, nil
		}
	}
	return false, nil
}

var (
	reSources = regexp.MustCompile(`\.Sources\b`)
	reProfile = regexp.MustCompile(`\.Profile\b`)
)

// migrateV1 converts a version 1 file as the design of introduce-spaces says
// (D9): sources become accounts, profiles become agents, the timing becomes
// the schedule "default", each profile that a channel uses becomes a space,
// and the fallback of the other conversations becomes Unsorted.
func migrateV1(b []byte) (*Config, error) {
	old := v1Config{Notify: Notify{Enabled: true, MinUrgency: "normal"}} // the version 1 defaults
	dec := toml.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&old); err != nil {
		return nil, decodeError(err)
	}
	c := Default()
	c.Agents, c.Schedules = nil, nil
	m := &Migration{}
	note := func(format string, a ...any) { m.Notes = append(m.Notes, fmt.Sprintf(format, a...)) }

	if old.Timing.Lookback != 0 {
		c.Settings.Lookback = old.Timing.Lookback
	}
	if old.Timing.Retention != 0 {
		c.Settings.Retention = old.Timing.Retention
	}
	c.Notify = old.Notify

	every := old.Timing.Interval
	if every == 0 {
		every = Duration(15 * time.Minute)
	}
	if every < Duration(time.Minute) {
		note("The interval %s is shorter than 1m; the schedule default sends every 1m.", every)
		every = Duration(time.Minute)
	}
	c.Schedules = []Schedule{{ID: "default", Name: "default", Every: every, Hours: old.Timing.ActiveHours}}

	for _, p := range old.Profiles {
		a := NewAgent(p.Name)
		a.Driver, a.CLI, a.Workspace, a.Session, a.Payload = orStr(p.Driver, a.Driver), orStr(p.Agent, a.CLI), p.Workspace, orStr(p.Session, a.Session), orStr(p.Payload, a.Payload)
		if p.InlineMaxBytes != 0 {
			a.InlineMaxBytes = p.InlineMaxBytes
		}
		if p.MessageMaxChars != 0 {
			a.MessageMaxChars = p.MessageMaxChars
		}
		a.Model, a.OrcaBin = p.Model, p.OrcaBin
		a.Primer, a.Batch = rewriteTemplate(p.Primer), rewriteTemplate(p.Batch)
		if a.Primer != p.Primer || a.Batch != p.Batch {
			note("Agent %q: the templates use {{.Accounts}} and {{.Agent}} now, not {{.Sources}} and {{.Profile}}.", a.ID)
		}
		c.Agents = append(c.Agents, a)
	}
	if len(c.Agents) == 0 {
		c.Agents = []Agent{NewAgent("triage")}
	}
	first := c.Agents[0].ID

	space := func(profile string) *[]Conversation {
		for i := range c.Spaces {
			if c.Spaces[i].ID == profile {
				return &c.Spaces[i].Conversations
			}
		}
		c.Spaces = append(c.Spaces, Space{ID: profile, Name: profile, Agent: profile, Schedule: "default"})
		return &c.Spaces[len(c.Spaces)-1].Conversations
	}

	read := ReadOff
	for _, s := range old.Sources {
		if s.AllDirect || s.Mentions {
			read = ReadMentions
		}
	}
	c.Unsorted = Unsorted{Agent: first, Schedule: "default", Read: read}
	if len(old.Sources) > 0 {
		c.Unsorted.Agent = orStr(old.Sources[0].Profile, first)
	}

	for _, s := range old.Sources {
		var a Account
		switch s.Type {
		case PlatformWhatsApp:
			a = NewWhatsAppAccount(s.ID)
		default:
			a = NewMattermostAccount(s.ID, s.URL)
			a.Platform = s.Type
		}
		if s.PollEvery != 0 {
			a.ReadEvery = s.PollEvery
		}
		c.Accounts = append(c.Accounts, a)

		dest := orStr(s.Profile, first)
		for _, ch := range s.Channels {
			list := space(orStr(s.ChannelProfiles[ch], dest))
			*list = append(*list, Conversation{Account: s.ID, ID: ch, Read: ReadAll})
		}
		keys := make([]string, 0, len(s.ChannelProfiles))
		for ch := range s.ChannelProfiles {
			if !slices.Contains(s.Channels, ch) {
				keys = append(keys, ch)
			}
		}
		slices.Sort(keys)
		for _, ch := range keys {
			list := space(s.ChannelProfiles[ch])
			*list = append(*list, Conversation{Account: s.ID, ID: ch, Read: ReadMentions})
		}

		if (s.AllDirect || s.Mentions) && dest != c.Unsorted.Agent {
			note("Account %q: the conversations that no space lists now go to Unsorted and the agent %q, not %q. Move them to the space %q in i2a config › Conversations.",
				s.ID, c.Unsorted.Agent, dest, dest)
		}
		if read == ReadMentions && !s.AllDirect {
			note("Account %q: direct messages now arrive too, because the read level mentions includes direct conversations. Set them to off in Unsorted to stop them.", s.ID)
		}
		if read == ReadMentions && !s.Mentions {
			note("Account %q: mentions in channels and groups that no space lists now arrive too. Set them to off in Unsorted to stop them.", s.ID)
		}
	}
	c.Migration = m
	return c, nil
}

func rewriteTemplate(s string) string {
	return reProfile.ReplaceAllString(reSources.ReplaceAllString(s, ".Accounts"), ".Agent")
}

func orStr(s, def string) string {
	if s != "" {
		return s
	}
	return def
}
