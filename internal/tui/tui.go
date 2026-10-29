// Package tui is the configuration dashboard (i2a config). It is one
// full-screen Bubble Tea program with tabs. Each tab is a list and a detail
// panel; add and edit are forms in the detail panel. A form writes the file
// and the secrets when the user saves it.
package tui

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
	toml "github.com/pelletier/go-toml/v2"

	"github.com/sondt2709/inbox2agent/internal/agent"
	i2aapp "github.com/sondt2709/inbox2agent/internal/app"
	"github.com/sondt2709/inbox2agent/internal/ask"
	"github.com/sondt2709/inbox2agent/internal/config"
	"github.com/sondt2709/inbox2agent/internal/inbox"
	"github.com/sondt2709/inbox2agent/internal/render"
	"github.com/sondt2709/inbox2agent/internal/secrets"
	"github.com/sondt2709/inbox2agent/internal/service"
	"github.com/sondt2709/inbox2agent/internal/source"
	"github.com/sondt2709/inbox2agent/internal/store"
)

// OrcaWarning is shown when a profile uses the orca driver.
const OrcaWarning = "Orca can start agents with permission prompts off. " +
	"Message text can contain instructions from other people. " +
	"i2a marks message text as untrusted data, but an agent can still obey it. " +
	"Turn off permission bypass for this agent in Orca if you can."

// Store is the part of the message store that the dashboard reads and writes.
type Store interface {
	Counts(ctx context.Context) (map[string]int, error)
	Dispatches(ctx context.Context, limit int) ([]store.Dispatch, error)
	Cards(ctx context.Context, all bool) ([]inbox.Card, error)
	SetCardState(ctx context.Context, id int64, key, state string, now time.Time) error
	MessagesByID(ctx context.Context, ids []int64) ([]inbox.Message, error)
	FindReply(ctx context.Context, messageID int64, place, text string) (inbox.Reply, bool, error)
	StartReply(ctx context.Context, r inbox.Reply) (int64, error)
	FinishReply(ctx context.Context, id int64, state, externalID, url, errText string, now time.Time) error
	SentReplies(ctx context.Context, cardIDs []int64) (map[int64][]inbox.Reply, error)
	ask.Store
	Asks(ctx context.Context, cardIDs []int64) (map[int64][]inbox.Ask, error)
}

// TabID names a tab that Run can open first.
type TabID = tabID

// Tabs that Run can open first.
const (
	TabFeed     TabID = tabFeed
	TabOverview TabID = tabOverview
)

// Deps are the inputs of the TUI. Nil functions get the default values.
type Deps struct {
	Paths   i2aapp.Paths
	Config  *config.Config
	Secrets secrets.Store
	Store   Store // nil when the store does not exist yet
	// OpenStore opens the store when Store is nil and the store file exists
	// now. It returns nil and no error when the file does not exist.
	OpenStore func() (Store, error)
	Tab       tabID // the first tab: TabFeed or TabOverview
	Check     func(ctx context.Context, sc config.Source, token string) (string, error)
	Out       io.Writer

	Copy func(text string) error // copies a card reply; the default runs pbcopy
	Open func(url string) error  // opens a message link; the default runs open or xdg-open
	// Replier builds the source that sends a reply; the default is source.BuildReplier.
	Replier func(cfg *config.Config, sourceID string) (source.Replier, error)
	// AskDriver builds the driver that sends an ask; the default is agent.New.
	AskDriver func(p config.Profile, s agent.Sessions) (agent.Driver, error)
	// CardCommand is the command of the card section in the primer of a new
	// session; the default is this executable, with I2A_HOME when it is set.
	CardCommand string

	ServiceStatus    func() (service.State, error)
	InstallService   func() (string, error) // installs or restarts the LaunchAgent; returns its path
	UninstallService func() (string, error)
}

type ui struct {
	d     Deps
	cfg   *config.Config // the saved configuration
	theme huh.Theme
}

func newUI(d Deps) *ui {
	if d.ServiceStatus == nil {
		d.ServiceStatus = service.Status
	}
	if d.UninstallService == nil {
		d.UninstallService = service.Uninstall
	}
	if d.Copy == nil {
		d.Copy = copyText
	}
	if d.Open == nil {
		d.Open = openURL
	}
	if d.Replier == nil {
		sec := d.Secrets
		d.Replier = func(cfg *config.Config, sourceID string) (source.Replier, error) {
			return source.BuildReplier(cfg, sourceID, sec, nil)
		}
	}
	if d.AskDriver == nil {
		d.AskDriver = agent.New
	}
	if d.CardCommand == "" {
		exe, err := os.Executable()
		if err != nil {
			exe = "i2a"
		}
		d.CardCommand = render.CardCommand(exe, os.Getenv("I2A_HOME"))
	}
	return &ui{d: d, cfg: clone(d.Config), theme: huh.ThemeFunc(huh.ThemeCharm)}
}

// Run shows the dashboard until the user quits.
func Run(ctx context.Context, d Deps) error {
	m := newModel(ctx, newUI(d))
	if _, err := tea.NewProgram(m, tea.WithContext(ctx)).Run(); err != nil && !errors.Is(err, tea.ErrInterrupted) {
		return err
	}
	if m.savedAny {
		fmt.Fprintf(d.Out, "Saved %s\n", d.Paths.ConfigFile)
	} else {
		fmt.Fprintln(d.Out, "Exit without saving. The configuration file did not change.")
	}
	return nil
}

func marshal(c *config.Config) []byte {
	b, err := toml.Marshal(c)
	if err != nil {
		panic(err)
	}
	return b
}

// clone copies a configuration through TOML, so the working copy shares no slices or maps.
func clone(c *config.Config) *config.Config {
	out := &config.Config{}
	if err := toml.Unmarshal(marshal(c), out); err != nil {
		panic(err)
	}
	return out
}

// commit validates next, writes the tokens, deletes the tokens of removed
// sources, and writes the configuration file. On success next becomes the
// configuration of the dashboard. On error nothing changes.
func (u *ui) commit(next *config.Config, tokens map[string]string, removed ...string) error {
	if err := next.Validate(); err != nil {
		return err
	}
	if err := render.CheckProfiles(next); err != nil {
		return err
	}
	for id, tok := range tokens {
		if err := u.d.Secrets.Set(secretsKey(id), tok); err != nil {
			return fmt.Errorf("save the token of %s in %s: %w", id, u.d.Secrets.Backend(), err)
		}
	}
	if err := config.Save(u.d.Paths.ConfigFile, next); err != nil {
		return err
	}
	for _, id := range removed {
		_ = u.d.Secrets.Delete(secretsKey(id))
	}
	u.cfg = next
	return nil
}

func secretsKey(id string) string { return secrets.TokenKey(id) }

// ---- helpers ----

func validName(s string) error {
	if len(s) == 0 || len(s) > 32 {
		return errors.New("use 1 to 32 characters")
	}
	for i, r := range s {
		ok := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || (i > 0 && (r == '-' || r == '_'))
		if !ok {
			return errors.New("use lowercase letters, digits, - and _")
		}
	}
	return nil
}

func required(s string) error {
	if strings.TrimSpace(s) == "" {
		return errors.New("this value is necessary")
	}
	return nil
}

func validWorkspace(s string) error {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "/") && !strings.HasPrefix(s, "~/") {
		return errors.New("use an absolute path, or a path that starts with ~/")
	}
	return nil
}

func minDuration(minimum time.Duration) func(string) error {
	return func(s string) error {
		d, err := config.ParseDuration(s)
		if err != nil {
			return err
		}
		if d.D() < minimum {
			return fmt.Errorf("use %s or more", config.Duration(minimum))
		}
		return nil
	}
}

func intRange(lo, hi int) func(string) error {
	return func(s string) error {
		n, err := strconv.Atoi(strings.TrimSpace(s))
		if err != nil || n < lo || n > hi {
			return fmt.Errorf("use a number from %d to %d", lo, hi)
		}
		return nil
	}
}

func nextID(c *config.Config) string {
	for i := 0; ; i++ {
		id := "work"
		if i > 0 {
			id = fmt.Sprintf("work%d", i+1)
		}
		if _, ok := c.Source(id); !ok {
			return id
		}
	}
}

func nextProfileName(c *config.Config) string {
	for i := 2; ; i++ {
		n := fmt.Sprintf("profile%d", i)
		if _, ok := c.Profile(n); !ok {
			return n
		}
	}
}
