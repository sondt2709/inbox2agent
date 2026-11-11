package config

import (
	"bytes"
	"fmt"
	"strings"

	toml "github.com/pelletier/go-toml/v2"
)

// The version 2 schema, as it was released: agents are items that spaces name.
type v2Config struct {
	Version   int        `toml:"version"`
	Settings  Settings   `toml:"settings"`
	Notify    Notify     `toml:"notify"`
	Accounts  []Account  `toml:"accounts"`
	Agents    []v2Agent  `toml:"agents"`
	Schedules []Schedule `toml:"schedules"`
	Unsorted  v2Unsorted `toml:"unsorted"`
	Spaces    []v2Space  `toml:"spaces"`
}

type v2Agent struct {
	ID              string `toml:"id"`
	Name            string `toml:"name"`
	Driver          string `toml:"driver"`
	CLI             string `toml:"cli"`
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

type v2Space struct {
	ID            string         `toml:"id"`
	Name          string         `toml:"name"`
	Agent         string         `toml:"agent"`
	Schedule      string         `toml:"schedule"`
	Conversations []Conversation `toml:"conversations"`
}

type v2Unsorted struct {
	Agent         string         `toml:"agent"`
	Schedule      string         `toml:"schedule"`
	Read          string         `toml:"read"`
	Conversations []Conversation `toml:"conversations"`
}

// newV2Agent returns a version 2 agent with the defaults of version 2.
func newV2Agent(id string) v2Agent {
	d := NewAgent(id)
	return v2Agent{ID: id, Name: id, Driver: d.Driver, CLI: d.CLI, Session: d.Session, Payload: d.Payload,
		InlineMaxBytes: d.InlineMaxBytes, MessageMaxChars: d.MessageMaxChars}
}

// version reads the version key of a file; 0 when it has none.
func version(b []byte) (int, error) {
	var keys struct {
		Version int `toml:"version"`
	}
	if err := toml.Unmarshal(b, &keys); err != nil {
		return 0, decodeError(err)
	}
	return keys.Version, nil
}

// parseV2 decodes a version 2 file and converts it to version 3.
func parseV2(b []byte) (*Config, error) {
	old := v2Config{}
	dec := toml.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&old); err != nil {
		return nil, decodeError(err)
	}
	return migrateV2(&old, &Migration{From: 2}), nil
}

// migrateV2 converts version 2 to version 3 as the design of space-owns-agent
// says (D2): each space gets a copy of its agent, with workspace as folder
// and primer as instruction; Unsorted on hold is paused; an agent that no
// space uses is removed. m gets a note for each change in behavior.
func migrateV2(old *v2Config, m *Migration) *Config {
	note := func(format string, a ...any) { m.Notes = append(m.Notes, fmt.Sprintf(format, a...)) }
	agents := map[string]v2Agent{}
	for _, a := range old.Agents {
		agents[a.ID] = a
	}
	users := map[string][]string{} // the spaces of each agent, Unsorted too
	convert := func(id string, space string) (Agent, string) {
		a, ok := agents[id]
		if !ok {
			return NewAgent(space), ""
		}
		users[id] = append(users[id], space)
		if a.Batch != "" {
			note("Space %q: the batch template of the agent %q is not used.", space, id)
		}
		return Agent{ID: space, Driver: a.Driver, CLI: a.CLI, Folder: a.Workspace, Session: a.Session, Payload: a.Payload,
			InlineMaxBytes: a.InlineMaxBytes, MessageMaxChars: a.MessageMaxChars, Model: a.Model, OrcaBin: a.OrcaBin}, a.Primer
	}

	c := &Config{Version: Version, Settings: old.Settings, Notify: old.Notify, Accounts: old.Accounts, Schedules: old.Schedules}
	for _, s := range old.Spaces {
		a, instruction := convert(s.Agent, s.ID)
		if a.Driver == DriverDryRun {
			note("Space %q uses the dry-run driver. To stop its sends and keep its agent, set paused = true and the driver orca.", s.ID)
		}
		c.Spaces = append(c.Spaces, Space{ID: s.ID, Name: s.Name, Schedule: s.Schedule, Instruction: instruction, Agent: a,
			Conversations: s.Conversations})
	}

	u := Unsorted{Schedule: old.Unsorted.Schedule, Read: old.Unsorted.Read, Conversations: old.Unsorted.Conversations}
	if old.Unsorted.Agent == "" {
		u.Paused = true
		switch {
		case len(c.Spaces) > 0:
			u.Agent = c.Spaces[0].Agent
		default:
			u.Agent = NewAgent(UnsortedID)
		}
		u.Agent.ID = UnsortedID
		note("Unsorted held its messages (agent = ''); it is paused now (paused = true), with a copy of the agent of the first space.")
	} else {
		u.Agent, u.Instruction = convert(old.Unsorted.Agent, UnsortedID)
	}
	c.Unsorted = u

	for _, a := range old.Agents {
		switch spaces := users[a.ID]; {
		case len(spaces) == 0:
			note("Agent %q was used by no space; i2a removed it. Its settings are in the backup file.", a.ID)
		case len(spaces) > 1:
			list := strings.Join(spaces[:len(spaces)-1], ", ") + " and " + spaces[len(spaces)-1]
			note("Agent %q: copied into the spaces %s. A change of one space no longer changes the other.", a.ID, list)
		}
	}
	c.applyDefaults()
	c.Migration = m
	return c
}
