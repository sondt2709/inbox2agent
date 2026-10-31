// Package config defines the i2a configuration file: the schema, the defaults,
// loading, saving, validation and the migration of version 1 files.
//
// The model has five items. An account is one login to a platform. A
// conversation is one direct conversation, group or channel of an account. A
// space is a group of conversations that the user names; it is the only item
// that names an agent and a schedule. An agent tells i2a how to send a batch.
// A schedule tells i2a when a space sends. The built-in space Unsorted holds
// each conversation that no space lists.
package config

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"text/template"
	"time"

	toml "github.com/pelletier/go-toml/v2"
)

// Version is the version of the file format that this i2a writes.
const Version = 2

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

// Platforms of an account.
const (
	PlatformMattermost = "mattermost"
	PlatformWhatsApp   = "whatsapp"
)

// Read levels of a conversation.
const (
	ReadAll      = "all"      // each message
	ReadMentions = "mentions" // messages for the user: a mention, a reply, or each message of a direct conversation
	ReadOff      = "off"      // no message
)

// ReadLevels lists the read levels in the order of the TUI.
var ReadLevels = []string{ReadAll, ReadMentions, ReadOff}

// UnsortedID is the ID of the built-in space for the conversations that no space lists.
const UnsortedID = "unsorted"

// Days lists the day names of a schedule, Monday first.
var Days = []string{"mon", "tue", "wed", "thu", "fri", "sat", "sun"}

// Limits for inline prompts. Orca passes prompts as command arguments.
const (
	MinInlineBytes = 1024
	MaxInlineBytes = 65536
)

// Config is the whole configuration file.
type Config struct {
	Version   int        `toml:"version" doc:"The version of the file format. i2a writes 2." enum:"1,2"`
	Settings  Settings   `toml:"settings" doc:"The message limits of the store."`
	Notify    Notify     `toml:"notify" doc:"The desktop notifications."`
	Accounts  []Account  `toml:"accounts" doc:"The logins to chat tools. An account only connects. Add one with i2a account add, which keeps the token in the secret store."`
	Agents    []Agent    `toml:"agents" doc:"How i2a sends a batch to an AI agent. A space names one agent."`
	Schedules []Schedule `toml:"schedules" doc:"When a space sends. A space names one schedule."`
	Unsorted  Unsorted   `toml:"unsorted" doc:"The built-in space of each conversation that no space lists."`
	Spaces    []Space    `toml:"spaces" doc:"The groups of conversations that you name. Each space sends its conversations to one agent on one schedule."`

	// Migration is the report of the conversion from version 1, or nil.
	Migration *Migration `toml:"-"`
}

// Settings are the message limits of the store.
type Settings struct {
	Lookback  Duration `toml:"lookback" comment:"First-run window and maximum age of a waiting message." doc:"At least 1m."`
	Retention Duration `toml:"retention" comment:"Keep sent messages and digests for this time."`
}

// Notify controls the desktop notifications.
type Notify struct {
	Enabled    bool   `toml:"enabled" comment:"Show macOS notifications for new cards and failed dispatches."`
	MinUrgency string `toml:"min_urgency" comment:"The lowest card urgency that gives a notification: high, normal or low." enum:"high,normal,low" schema:"default"`
}

// Account is one login to one platform. It only connects: it names no space,
// no agent and no conversation.
type Account struct {
	ID        string   `toml:"id" doc:"The stable ID: 1-32 lowercase letters, digits, - or _. The token in the secret store uses it." schema:"required,id"`
	Name      string   `toml:"name" doc:"The name that i2a shows. Empty: the ID."`
	Platform  string   `toml:"platform" doc:"The chat tool of the account." enum:"mattermost,whatsapp" schema:"required"`
	URL       string   `toml:"url,omitempty" doc:"The server URL of a Mattermost account, with https:// or http://."`
	User      string   `toml:"user,omitempty" doc:"The Mattermost username. i2a writes it after the connection check."`
	ReadEvery Duration `toml:"read_every" doc:"The time between two reads of the account. At least 5s. Default: 1m, or 10s for WhatsApp."`
}

// Agent tells i2a how to send a batch to an AI agent.
type Agent struct {
	ID              string `toml:"id" doc:"The stable ID: 1-32 lowercase letters, digits, - or _. Spaces name it." schema:"required,id"`
	Name            string `toml:"name" doc:"The name that i2a shows. Empty: the ID."`
	Driver          string `toml:"driver" doc:"orca types the batch into an agent terminal of Orca. dry-run only writes the digest file." enum:"orca,dry-run" schema:"default"`
	CLI             string `toml:"cli" doc:"The agent CLI that Orca starts, for example claude."`
	Workspace       string `toml:"workspace" doc:"The folder of the agent terminal. The orca driver needs it."`
	Session         string `toml:"session" doc:"new: a new terminal for each batch. reuse: one terminal for each space while it is live. worktree: a new Git worktree for each batch." enum:"new,reuse,worktree" schema:"default"`
	Payload         string `toml:"payload" doc:"pointer: the prompt names the digest file. inline: the prompt contains the messages." enum:"pointer,inline" schema:"default"`
	InlineMaxBytes  int    `toml:"inline_max_bytes" doc:"The largest inline prompt in bytes, from 1024 to 65536." min:"1024" max:"65536"`
	MessageMaxChars int    `toml:"message_max_chars" doc:"The longest message text in a batch, from 100 to 20000 characters." min:"100" max:"20000"`
	Model           string `toml:"model,omitempty" doc:"The model of the agent CLI. Empty: the default of the CLI."`
	OrcaBin         string `toml:"orca_bin,omitempty" doc:"The path of the orca command. Empty: orca in PATH."`
	Primer          string `toml:"primer,multiline,omitempty" doc:"A Go template of the first prompt of a session. Empty: the built-in primer."`
	Batch           string `toml:"batch,multiline,omitempty" doc:"A Go template of the prompt of each batch. Empty: the built-in prompt."`
}

// Schedule tells i2a when a space sends.
type Schedule struct {
	ID    string   `toml:"id" doc:"The stable ID: 1-32 lowercase letters, digits, - or _. Spaces name it." schema:"required,id"`
	Name  string   `toml:"name" doc:"The name that i2a shows. Empty: the ID."`
	Every Duration `toml:"every" doc:"The time between two sends, from the start of the hours. At least 1m."`
	Days  []string `toml:"days,omitempty" doc:"The days of the sends. Empty: each day." enum:"mon,tue,wed,thu,fri,sat,sun"`
	Hours string   `toml:"hours,omitempty" doc:"The hours of the sends in local time, HH:MM-HH:MM. Empty: all day." pattern:"^\\s*([0-9]{2}:[0-9]{2}\\s*-\\s*[0-9]{2}:[0-9]{2}\\s*)?$"`
}

// Conversation is one direct conversation, group or channel of an account.
type Conversation struct {
	Account string `toml:"account" doc:"The ID of the account of the conversation." schema:"required"`
	ID      string `toml:"id" doc:"The platform ID of the conversation. i2a conversations --json lists them." schema:"required"`
	Name    string `toml:"name,omitempty" doc:"The platform name. The dashboard writes it."`
	Label   string `toml:"label,omitempty" doc:"The name that you give. It replaces the platform name on the screens."`
	Read    string `toml:"read" doc:"all: each message. mentions: the messages for you (a mention, a reply to you, each message of a direct conversation). off: no message. Default: all in a space; the read of unsorted in Unsorted." enum:"all,mentions,off" schema:"default"`
}

// Space is a group of conversations that the user names. Its conversations go
// to one agent on one schedule.
type Space struct {
	ID            string         `toml:"id" doc:"The stable ID: 1-32 lowercase letters, digits, - or _. It cannot be unsorted." schema:"required,id"`
	Name          string         `toml:"name" doc:"The name that i2a shows. Empty: the ID. Two spaces cannot have the same name."`
	Agent         string         `toml:"agent" doc:"The ID of the agent of the space." schema:"required"`
	Schedule      string         `toml:"schedule" doc:"The ID of the schedule of the space." schema:"required"`
	Conversations []Conversation `toml:"conversations,omitempty" doc:"The conversations of the space. A conversation is in one space only."`
}

// Unsorted is the built-in space. Read is the level of each conversation that
// no space lists; Conversations has the unsorted conversations with their own
// level. An empty Agent holds the messages.
type Unsorted struct {
	Agent         string         `toml:"agent" comment:"Empty: hold the messages until you sort their conversations."`
	Schedule      string         `toml:"schedule" doc:"The ID of the schedule of Unsorted. Default: the first schedule."`
	Read          string         `toml:"read" comment:"The read level of a conversation that no space lists: all, mentions or off." enum:"all,mentions,off" schema:"default"`
	Conversations []Conversation `toml:"conversations,omitempty" doc:"The conversations in Unsorted that have their own read level or label."`
}

// Title returns the name of the account, or its ID.
func (a Account) Title() string { return orID(a.Name, a.ID) }

// Title returns the name of the agent, or its ID.
func (a Agent) Title() string { return orID(a.Name, a.ID) }

// Title returns the name of the schedule, or its ID.
func (s Schedule) Title() string { return orID(s.Name, s.ID) }

// Title returns the name of the space, or its ID.
func (s Space) Title() string { return orID(s.Name, s.ID) }

// Title returns the label, the platform name or the ID of the conversation.
func (c Conversation) Title() string { return orID(orID(c.Label, c.Name), c.ID) }

func orID(name, id string) string {
	if name != "" {
		return name
	}
	return id
}

// Default returns a configuration with one dry-run agent, one schedule, and
// an Unsorted space that sends the messages for the user to that agent.
func Default() *Config {
	return &Config{
		Version: Version,
		Settings: Settings{
			Lookback:  Duration(24 * time.Hour),
			Retention: Duration(7 * 24 * time.Hour),
		},
		Notify:    Notify{Enabled: true, MinUrgency: "normal"},
		Agents:    []Agent{NewAgent("triage")},
		Schedules: []Schedule{NewSchedule("always")},
		Unsorted:  Unsorted{Agent: "triage", Schedule: "always", Read: ReadMentions},
	}
}

// NewAgent returns an agent with the default values.
func NewAgent(id string) Agent {
	return Agent{
		ID:              id,
		Name:            id,
		Driver:          DriverDryRun,
		CLI:             "claude",
		Session:         SessionReuse,
		Payload:         PayloadPointer,
		InlineMaxBytes:  8000,
		MessageMaxChars: 2000,
	}
}

// NewSchedule returns a schedule that sends every 15 minutes, all day.
func NewSchedule(id string) Schedule {
	return Schedule{ID: id, Name: id, Every: Duration(15 * time.Minute)}
}

// NewMattermostAccount returns a Mattermost account with the default values.
func NewMattermostAccount(id, url string) Account {
	return Account{ID: id, Name: id, Platform: PlatformMattermost, URL: strings.TrimRight(url, "/"), ReadEvery: Duration(time.Minute)}
}

// NewWhatsAppAccount returns a WhatsApp account with the default values.
// It reads a local file on each poll, so it reads more often than Mattermost.
func NewWhatsAppAccount(id string) Account {
	return Account{ID: id, Name: id, Platform: PlatformWhatsApp, ReadEvery: Duration(10 * time.Second)}
}

// ErrNotFound is returned by Load when the file does not exist.
var ErrNotFound = errors.New("configuration file not found")

// Load reads and validates the configuration file. A version 1 file is
// converted, its old text is kept in <path>.v1.bak (mode 0600), and the new
// file is saved; Config.Migration has the report.
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
	if c.Migration != nil {
		return convert(path)
	}
	_ = writeSchema(path) // keep the schema of this binary next to the file, also before the first save
	return c, nil
}

// convert writes a version 1 file as version 2 under the write lock. It reads
// the file again in the lock, so two processes do not convert at one time.
func convert(path string) (*Config, error) {
	unlock, err := lock(path)
	if err != nil {
		return nil, err
	}
	defer unlock()
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	c, err := Parse(b)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if c.Migration != nil {
		if err := backupV1(path, b, c); err != nil {
			return nil, err
		}
		if _, err := save(path, c); err != nil {
			return nil, fmt.Errorf("save the version 2 file: %w", err)
		}
	}
	return c, nil
}

// backupV1 writes the bytes of a version 1 file to config.toml.v1.bak with
// mode 0600 and records the path in the report.
func backupV1(path string, b []byte, c *Config) error {
	bak := path + ".v1.bak"
	if err := os.WriteFile(bak, b, 0o600); err != nil {
		return fmt.Errorf("back up the version 1 file: %w", err)
	}
	if err := os.Chmod(bak, 0o600); err != nil {
		return err
	}
	c.Migration.Backup = bak
	return nil
}

// Parse decodes and validates configuration bytes. It converts a version 1
// file in memory.
func Parse(b []byte) (*Config, error) {
	v1, err := isV1(b)
	if err != nil {
		return nil, err
	}
	if v1 {
		c, err := migrateV1(b)
		if err != nil {
			return nil, err
		}
		if err := c.Validate(); err != nil {
			return nil, fmt.Errorf("after the conversion from version 1: %w", err)
		}
		return c, nil
	}
	c := &Config{}
	dec := toml.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(c); err != nil {
		return nil, decodeError(err)
	}
	if c.Version > Version {
		return nil, fmt.Errorf("the file has version %d; this i2a supports %d. Update i2a", c.Version, Version)
	}
	c.applyDefaults()
	if err := c.Validate(); err != nil {
		return nil, err
	}
	return c, nil
}

func decodeError(err error) error {
	var serr *toml.StrictMissingError
	if errors.As(err, &serr) {
		return fmt.Errorf("unknown field:\n%s", serr.String())
	}
	var derr *toml.DecodeError
	if errors.As(err, &derr) {
		row, col := derr.Position()
		return fmt.Errorf("line %d column %d: %s", row, col, derr.Error())
	}
	return err
}

func (c *Config) applyDefaults() {
	d := Default()
	c.Version = Version
	if c.Settings.Lookback == 0 {
		c.Settings.Lookback = d.Settings.Lookback
	}
	if c.Settings.Retention == 0 {
		c.Settings.Retention = d.Settings.Retention
	}
	if c.Notify.MinUrgency == "" {
		c.Notify.MinUrgency = d.Notify.MinUrgency
	}
	for i := range c.Accounts {
		a := &c.Accounts[i]
		if a.Name == "" {
			a.Name = a.ID
		}
		if a.ReadEvery == 0 {
			a.ReadEvery = Duration(time.Minute)
			if a.Platform == PlatformWhatsApp {
				a.ReadEvery = Duration(10 * time.Second)
			}
		}
	}
	if len(c.Agents) == 0 {
		c.Agents = d.Agents
	}
	for i := range c.Agents {
		a := &c.Agents[i]
		def := NewAgent(a.ID)
		if a.Name == "" {
			a.Name = a.ID
		}
		if a.Driver == "" {
			a.Driver = def.Driver
		}
		if a.CLI == "" {
			a.CLI = def.CLI
		}
		if a.Session == "" {
			a.Session = def.Session
		}
		if a.Payload == "" {
			a.Payload = def.Payload
		}
		if a.InlineMaxBytes == 0 {
			a.InlineMaxBytes = def.InlineMaxBytes
		}
		if a.MessageMaxChars == 0 {
			a.MessageMaxChars = def.MessageMaxChars
		}
	}
	if len(c.Schedules) == 0 {
		c.Schedules = d.Schedules
	}
	for i := range c.Schedules {
		if c.Schedules[i].Name == "" {
			c.Schedules[i].Name = c.Schedules[i].ID
		}
	}
	if c.Unsorted.Read == "" {
		c.Unsorted.Read = ReadMentions
	}
	if c.Unsorted.Schedule == "" {
		c.Unsorted.Schedule = c.Schedules[0].ID
	}
	for i := range c.Unsorted.Conversations {
		if c.Unsorted.Conversations[i].Read == "" {
			c.Unsorted.Conversations[i].Read = c.Unsorted.Read
		}
	}
	for i := range c.Spaces {
		s := &c.Spaces[i]
		if s.Name == "" {
			s.Name = s.ID
		}
		for j := range s.Conversations {
			if s.Conversations[j].Read == "" {
				s.Conversations[j].Read = ReadAll
			}
		}
	}
}

// Header is the comment after the schema line of each file that i2a writes.
const Header = "# i2a configuration. Edit it with `i2a` or by hand, also while i2a runs.\n" +
	"# i2a writes this file again on each save and removes other comments.\n" +
	"# Tokens are not in this file. They are in the secret store.\n\n"

// Save validates the configuration and writes it with mode 0600. It holds the
// write lock of the file. Use Update to change the file that is on disk now.
func Save(path string, c *Config) error {
	unlock, err := lock(path)
	if err != nil {
		return err
	}
	defer unlock()
	_, err = save(path, c)
	return err
}

// save validates and writes the configuration without the lock, and returns
// the hash of the written bytes. The caller holds the lock.
func save(path string, c *Config) (string, error) {
	if err := c.Validate(); err != nil {
		return "", err
	}
	c.Version = Version
	b, err := toml.Marshal(c)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", err
	}
	b = append([]byte(SchemaLine+Header), b...)
	_ = writeSchema(path) // only an aid for editors: it must not stop a save
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return "", err
	}
	if err := os.Rename(tmp, path); err != nil {
		return "", err
	}
	return Hash(b), nil
}

// Account returns the account with this ID.
func (c *Config) Account(id string) (*Account, bool) {
	for i := range c.Accounts {
		if c.Accounts[i].ID == id {
			return &c.Accounts[i], true
		}
	}
	return nil, false
}

// Agent returns the agent with this ID.
func (c *Config) Agent(id string) (*Agent, bool) {
	for i := range c.Agents {
		if c.Agents[i].ID == id {
			return &c.Agents[i], true
		}
	}
	return nil, false
}

// Schedule returns the schedule with this ID.
func (c *Config) Schedule(id string) (*Schedule, bool) {
	for i := range c.Schedules {
		if c.Schedules[i].ID == id {
			return &c.Schedules[i], true
		}
	}
	return nil, false
}

// Space returns the space with this ID. For UnsortedID it returns a copy that
// describes Unsorted as a space (name "Unsorted"); a change to that copy does
// not change the configuration.
func (c *Config) Space(id string) (*Space, bool) {
	if id == UnsortedID {
		s := c.UnsortedSpace()
		return &s, true
	}
	for i := range c.Spaces {
		if c.Spaces[i].ID == id {
			return &c.Spaces[i], true
		}
	}
	return nil, false
}

// UnsortedSpace describes Unsorted as a space.
func (c *Config) UnsortedSpace() Space {
	return Space{ID: UnsortedID, Name: "Unsorted", Agent: c.Unsorted.Agent, Schedule: c.Unsorted.Schedule,
		Conversations: c.Unsorted.Conversations}
}

// AllSpaces returns Unsorted first, then the spaces.
func (c *Config) AllSpaces() []Space {
	return append([]Space{c.UnsortedSpace()}, c.Spaces...)
}

// conversations returns a pointer to the conversation list of a space.
func (c *Config) conversations(space string) *[]Conversation {
	if space == UnsortedID {
		return &c.Unsorted.Conversations
	}
	for i := range c.Spaces {
		if c.Spaces[i].ID == space {
			return &c.Spaces[i].Conversations
		}
	}
	return nil
}

// Conversation returns the entry of a conversation and the ID of its space.
// A conversation that has no entry is in Unsorted with the level
// Unsorted.Read, and ok is false.
func (c *Config) Conversation(account, id string) (conv Conversation, space string, ok bool) {
	for _, s := range c.AllSpaces() {
		for _, cv := range s.Conversations {
			if cv.Account == account && cv.ID == id {
				return cv, s.ID, true
			}
		}
	}
	return Conversation{Account: account, ID: id, Read: c.Unsorted.Read}, UnsortedID, false
}

// Route returns the ID of the space of a conversation: the space that lists
// it, or Unsorted.
func (c *Config) Route(account, conversation string) string {
	_, space, _ := c.Conversation(account, conversation)
	return space
}

// Level returns the read level of a conversation.
func (c *Config) Level(account, conversation string) string {
	cv, _, _ := c.Conversation(account, conversation)
	return cv.Read
}

// Levels returns the read level of each conversation of an account that has
// an entry. A conversation that is not in the map has the level Unsorted.Read.
func (c *Config) Levels(account string) map[string]string {
	out := map[string]string{}
	for _, s := range c.AllSpaces() {
		for _, cv := range s.Conversations {
			if cv.Account == account {
				out[cv.ID] = cv.Read
			}
		}
	}
	return out
}

// SetConversation puts the conversation in the space with the read level and
// the label, and removes it from its old space. An empty read keeps the old
// level, or uses "all" for a conversation that had no entry or was off and
// goes to a space that is not Unsorted. name refreshes the platform name when
// it is not empty.
func (c *Config) SetConversation(account, id, space, read, name string) error {
	if _, ok := c.Account(account); !ok {
		return fmt.Errorf("the account %q does not exist", account)
	}
	dst := c.conversations(space)
	if dst == nil {
		return fmt.Errorf("the space %q does not exist", space)
	}
	cv, from, had := c.Conversation(account, id)
	if src := c.conversations(from); had && src != nil {
		for i, x := range *src {
			if x.Account == account && x.ID == id {
				*src = append((*src)[:i], (*src)[i+1:]...)
				break
			}
		}
	}
	if name != "" {
		cv.Name = name
	}
	switch {
	case read != "":
		cv.Read = read
	case space != UnsortedID && (!had || cv.Read == ReadOff):
		cv.Read = ReadAll
	}
	dst = c.conversations(space)
	*dst = append(*dst, cv)
	return nil
}

// SetLabel sets the label of a conversation. An empty label shows the
// platform name again. A conversation that has no entry gets an entry in
// Unsorted with the level Unsorted.Read.
func (c *Config) SetLabel(account, id, label string) error {
	cv, space, _ := c.Conversation(account, id)
	if err := c.SetConversation(account, id, space, cv.Read, ""); err != nil {
		return err
	}
	list := c.conversations(space)
	for i := range *list {
		if (*list)[i].Account == account && (*list)[i].ID == id {
			(*list)[i].Label = strings.TrimSpace(label)
		}
	}
	return nil
}

// SpacesUsing returns the spaces (Unsorted first) that use the agent or the
// schedule with this ID. kind is "agent" or "schedule". Unsorted uses a
// schedule only when it has an agent.
func (c *Config) SpacesUsing(kind, id string) []Space {
	var out []Space
	for _, s := range c.AllSpaces() {
		switch {
		case kind == "agent" && s.Agent == id:
			out = append(out, s)
		case kind == "schedule" && s.Schedule == id && s.Agent != "":
			out = append(out, s)
		}
	}
	return out
}

// Replace sets the agent or the schedule old to new in each space that uses
// it, Unsorted too. kind is "agent" or "schedule".
func (c *Config) Replace(kind, old, new string) {
	set := func(agent, schedule *string) {
		if kind == "agent" && *agent == old {
			*agent = new
		}
		if kind == "schedule" && *schedule == old {
			*schedule = new
		}
	}
	set(&c.Unsorted.Agent, &c.Unsorted.Schedule)
	for i := range c.Spaces {
		set(&c.Spaces[i].Agent, &c.Spaces[i].Schedule)
	}
}

// NewID returns a free ID made from a name, for example "Company B" gives
// "company-b", or "company-b-2" when that is used. used reports a used ID.
func NewID(name string, used func(string) bool) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(name) {
		switch {
		case r >= 'a' && r <= 'z' || r >= '0' && r <= '9':
			b.WriteRune(r)
			dash = false
		case !dash && b.Len() > 0:
			b.WriteByte('-')
			dash = true
		}
	}
	id := strings.Trim(b.String(), "-")
	if len(id) > 28 {
		id = strings.Trim(id[:28], "-")
	}
	if id == "" {
		id = "item"
	}
	if id == UnsortedID {
		id = "unsorted-space"
	}
	base := id
	for n := 2; used(id); n++ {
		id = fmt.Sprintf("%s-%d", base, n)
	}
	return id
}

var nameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,31}$`)

// oldField finds a template action that uses a field of version 1.
var oldField = regexp.MustCompile(`\{\{[^}]*\.(Sources|Profile)\b`)

// ValidationError lists all problems in a configuration.
type ValidationError struct{ Problems []string }

func (e *ValidationError) Error() string {
	return "invalid configuration:\n  - " + strings.Join(e.Problems, "\n  - ")
}

// Validate checks the configuration and returns a *ValidationError.
func (c *Config) Validate() error {
	var p []string
	add := func(format string, a ...any) { p = append(p, fmt.Sprintf(format, a...)) }
	id := func(kind string, i int, v string) string {
		if !nameRe.MatchString(v) {
			add("%s[%d].id: %q must be 1-32 lowercase letters, digits, - or _", kind, i, v)
			return fmt.Sprintf("%s[%d]", kind, i)
		}
		return fmt.Sprintf("%s %q", strings.TrimSuffix(kind, "s"), v)
	}

	if c.Settings.Lookback < Duration(time.Minute) {
		add("settings.lookback: %q is shorter than 1m", c.Settings.Lookback)
	}
	switch c.Notify.MinUrgency {
	case "high", "normal", "low":
	default:
		add("notify.min_urgency: %q is not high, normal or low", c.Notify.MinUrgency)
	}

	accounts := map[string]bool{}
	for i, a := range c.Accounts {
		at := id("accounts", i, a.ID)
		if accounts[a.ID] {
			add("%s: the ID is used two times", at)
		}
		accounts[a.ID] = true
		switch a.Platform {
		case PlatformMattermost:
			if !strings.HasPrefix(a.URL, "https://") && !strings.HasPrefix(a.URL, "http://") {
				add("%s.url: %q must start with https:// or http://", at, a.URL)
			}
		case PlatformWhatsApp:
		default:
			add("%s.platform: %q is not supported (use mattermost or whatsapp)", at, a.Platform)
		}
		if a.ReadEvery < Duration(5*time.Second) {
			add("%s.read_every: %q is shorter than 5s", at, a.ReadEvery)
		}
	}

	agents := map[string]bool{}
	if len(c.Agents) == 0 {
		add("agents: at least one agent is necessary")
	}
	for i, a := range c.Agents {
		at := id("agents", i, a.ID)
		if agents[a.ID] {
			add("%s: the ID is used two times", at)
		}
		agents[a.ID] = true
		switch a.Driver {
		case DriverDryRun, DriverOrca:
		default:
			add("%s.driver: %q is not dry-run or orca", at, a.Driver)
		}
		switch a.Session {
		case SessionNew, SessionReuse, SessionWorktree:
		default:
			add("%s.session: %q is not new, reuse or worktree", at, a.Session)
		}
		switch a.Payload {
		case PayloadPointer, PayloadInline:
		default:
			add("%s.payload: %q is not pointer or inline", at, a.Payload)
		}
		if a.InlineMaxBytes < MinInlineBytes || a.InlineMaxBytes > MaxInlineBytes {
			add("%s.inline_max_bytes: %d is not between %d and %d", at, a.InlineMaxBytes, MinInlineBytes, MaxInlineBytes)
		}
		if a.MessageMaxChars < 100 || a.MessageMaxChars > 20000 {
			add("%s.message_max_chars: %d is not between 100 and 20000", at, a.MessageMaxChars)
		}
		if a.Driver == DriverOrca {
			if a.Workspace == "" {
				add("%s.workspace: the orca driver needs a workspace folder", at)
			}
			if a.CLI == "" {
				add("%s.cli: the orca driver needs an agent CLI, for example claude", at)
			}
		}
		for name, text := range map[string]string{"primer": a.Primer, "batch": a.Batch} {
			if text == "" {
				continue
			}
			if m := oldField.FindStringSubmatch(text); m != nil {
				add("%s.%s: the field .%s is gone; use .%s", at, name, m[1], map[string]string{"Sources": "Accounts", "Profile": "Agent"}[m[1]])
				continue
			}
			if _, err := template.New(name).Option("missingkey=error").Parse(text); err != nil {
				add("%s.%s: template error: %v", at, name, err)
			}
		}
	}

	schedules := map[string]bool{}
	if len(c.Schedules) == 0 {
		add("schedules: at least one schedule is necessary")
	}
	for i, s := range c.Schedules {
		at := id("schedules", i, s.ID)
		if schedules[s.ID] {
			add("%s: the ID is used two times", at)
		}
		schedules[s.ID] = true
		if s.Every < Duration(time.Minute) {
			add("%s.every: %q is shorter than 1m", at, s.Every)
		}
		for _, d := range s.Days {
			if !contains(Days, d) {
				add("%s.days: %q is not one of mon, tue, wed, thu, fri, sat, sun", at, d)
			}
		}
		if _, err := ParseHours(s.Hours); err != nil {
			add("%s.hours: %v", at, err)
		}
	}

	seen := map[[2]string]string{}
	level := func(at, read string) {
		if !contains(ReadLevels, read) {
			add("%s.read: %q is not all, mentions or off", at, read)
		}
	}
	conversations := func(at string, list []Conversation) {
		for j, cv := range list {
			cat := fmt.Sprintf("%s.conversations[%d]", at, j)
			if !accounts[cv.Account] {
				add("%s.account: the account %q does not exist", cat, cv.Account)
			}
			if cv.ID == "" {
				add("%s.id: the conversation ID is empty", cat)
			}
			level(cat, cv.Read)
			k := [2]string{cv.Account, cv.ID}
			if other, ok := seen[k]; ok {
				add("conversation %s/%s is in two spaces: %s and %s", cv.Account, cv.ID, other, at)
			}
			seen[k] = at
		}
	}

	level("unsorted", c.Unsorted.Read)
	if c.Unsorted.Agent != "" && !agents[c.Unsorted.Agent] {
		add("unsorted.agent: the agent %q does not exist", c.Unsorted.Agent)
	}
	if c.Unsorted.Schedule != "" && !schedules[c.Unsorted.Schedule] {
		add("unsorted.schedule: the schedule %q does not exist", c.Unsorted.Schedule)
	}
	if c.Unsorted.Agent != "" && c.Unsorted.Schedule == "" {
		add("unsorted.schedule: an Unsorted space with an agent needs a schedule")
	}
	conversations("unsorted", c.Unsorted.Conversations)

	spaces, names := map[string]bool{}, map[string]bool{"unsorted": true}
	for i, s := range c.Spaces {
		at := id("spaces", i, s.ID)
		if s.ID == UnsortedID {
			add("%s: the ID unsorted is for the built-in space", at)
		}
		if spaces[s.ID] {
			add("%s: the ID is used two times", at)
		}
		spaces[s.ID] = true
		if n := strings.ToLower(strings.TrimSpace(s.Title())); names[n] {
			add("%s.name: the name %q is used two times", at, s.Title())
		} else {
			names[n] = true
		}
		if !agents[s.Agent] {
			add("%s.agent: the agent %q does not exist", at, s.Agent)
		}
		if !schedules[s.Schedule] {
			add("%s.schedule: the schedule %q does not exist", at, s.Schedule)
		}
		conversations(at, s.Conversations)
	}

	if len(p) > 0 {
		return &ValidationError{Problems: p}
	}
	return nil
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}
