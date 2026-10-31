// Package ask sends an instruction of the user about a card to the agent. The
// Feed, "i2a ask" and the daemon use the same flow. The text of the user is
// the instruction; the card and its messages go to the agent as data, because
// the card came from an agent that read untrusted messages.
package ask

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/sondt2709/inbox2agent/internal/agent"
	"github.com/sondt2709/inbox2agent/internal/config"
	"github.com/sondt2709/inbox2agent/internal/inbox"
	"github.com/sondt2709/inbox2agent/internal/render"
	"github.com/sondt2709/inbox2agent/internal/reply"
	"github.com/sondt2709/inbox2agent/internal/store"
)

// Wait is the time that the Feed and "i2a ask" wait for a busy agent.
const Wait = 2 * time.Minute

// LockWait is the time that an ask waits for another delivery to the same
// space: longer than Wait and the 30 second submit. Tests make it shorter.
var LockWait = 3 * time.Minute

// Store is the part of the store that an ask uses.
type Store interface {
	agent.Sessions
	Card(ctx context.Context, id int64, key string) (inbox.Card, error)
	MessagesByID(ctx context.Context, ids []int64) ([]inbox.Message, error)
	StartAsk(ctx context.Context, a inbox.Ask) (int64, error)
	FinishAsk(ctx context.Context, id int64, state, handle, detail string, now time.Time) error
}

// Deps are the parts of i2a that an ask uses.
type Deps struct {
	Store       Store
	Config      *config.Config
	NewDriver   func(a config.Agent, space string, s agent.Sessions) (agent.Driver, error) // default agent.New
	CardCommand string                                                                     // the command of the card section; see render.CardCommand
	LockDir     string                                                                     // the directory of the space locks
	Now         func() time.Time
}

// NoAgentError is the error of an ask about a card of a space that has no
// agent (Unsorted on hold) or a dry-run agent.
type NoAgentError struct{ Space string }

func (e *NoAgentError) Error() string {
	return fmt.Sprintf("The space %s has no agent that reads asks.", e.Space)
}

// CheckText applies the text rule of a reply: no hidden characters, no spaces
// at the start and the end, 1 to reply.MaxText characters.
func CheckText(s string) (string, error) { return reply.CheckText(s) }

// Target is the agent that gets an ask about a card.
type Target struct {
	Card     inbox.Card
	Messages []inbox.Message // the ref messages that still exist
	Space    config.Space
	Agent    config.Agent
}

// AgentText names the agent, its driver and the space for a person, for
// example "Claude for work · orca · space Acme".
func (t Target) AgentText() string {
	return t.Agent.Title() + " · " + t.Agent.Driver + " · space " + t.Space.Title()
}

// Session names the session that gets the ask for a person, for example
// "terminal term_b9f22da8… (wrote the card)". When the terminal of the card
// no longer runs, the ask goes to the session of the space instead.
func (t Target) Session() string {
	switch {
	case t.Card.Handle != "":
		h := t.Card.Handle
		if len(h) > 13 {
			h = h[:13] + "…" // term_ and 8 characters
		}
		return "terminal " + h + " (wrote the card)"
	case t.Agent.Session == config.SessionReuse:
		return "the session of the space"
	case t.Agent.Session == config.SessionWorktree:
		return "a new worktree"
	}
	return "a new session"
}

// Resolve finds the space of the card and its agent: the space of the newest
// ref message that has one, or else the space that lists the conversation of
// the newest ref message, or else Unsorted.
func Resolve(ctx context.Context, d Deps, c inbox.Card) (Target, error) {
	msgs, err := d.Store.MessagesByID(ctx, c.Refs)
	if err != nil {
		return Target{}, err
	}
	return TargetOf(d.Config, c, msgs)
}

// TargetOf is Resolve with the ref messages of the card that the caller read.
func TargetOf(cfg *config.Config, c inbox.Card, msgs []inbox.Message) (Target, error) {
	if c.Space != "" { // the space of the session of the terminal that added the card
		if _, ok := cfg.Space(c.Space); ok {
			return target(cfg, c, msgs, c.Space)
		}
	}
	space := ""
	var newest time.Time
	for _, m := range msgs {
		if m.Space != "" && (space == "" || m.CreatedAt.After(newest)) {
			space, newest = m.Space, m.CreatedAt
		}
	}
	if space == "" && len(msgs) > 0 {
		last := msgs[0]
		for _, m := range msgs {
			if m.CreatedAt.After(last.CreatedAt) {
				last = m
			}
		}
		space = cfg.Route(last.SourceID, last.ChannelID)
	}
	return target(cfg, c, msgs, space)
}

func target(cfg *config.Config, c inbox.Card, msgs []inbox.Message, space string) (Target, error) {
	if space == "" {
		space = config.UnsortedID
	}
	s, ok := cfg.Space(space)
	if !ok {
		return Target{}, fmt.Errorf("the space %q of the card is not in the configuration", space)
	}
	a, ok := cfg.Agent(s.Agent)
	if !ok || a.Driver == config.DriverDryRun {
		return Target{}, &NoAgentError{Space: s.Title()}
	}
	return Target{Card: c, Messages: msgs, Space: *s, Agent: *a}, nil
}

// Send checks the text, records the ask as queued and sends it. When the agent
// stays busy for wait, the ask stays queued and Send returns no error; the
// daemon sends it later.
func Send(ctx context.Context, d Deps, c inbox.Card, text string, wait time.Duration) (inbox.Ask, error) {
	text, err := CheckText(text)
	if err != nil {
		return inbox.Ask{}, err
	}
	t, err := Resolve(ctx, d, c)
	if err != nil {
		return inbox.Ask{}, err
	}
	a := inbox.Ask{CardID: c.ID, Space: t.Space.ID, Text: text, State: inbox.AskQueued, CreatedAt: d.Now().UTC()}
	if a.ID, err = d.Store.StartAsk(ctx, a); err != nil {
		return inbox.Ask{}, err
	}
	return deliver(ctx, d, t, a, wait)
}

// Retry sends a queued ask again, for the daemon. An ask older than maxAge,
// an ask of a card that no longer exists and an ask of a space that is not
// in the configuration or has no agent become failed.
func Retry(ctx context.Context, d Deps, a inbox.Ask, maxAge, wait time.Duration) (inbox.Ask, error) {
	now := d.Now()
	if now.Sub(a.CreatedAt) > maxAge {
		return finish(ctx, d, a, inbox.AskFailed, "", "not sent within "+config.Duration(maxAge).String())
	}
	c, err := d.Store.Card(ctx, a.CardID, "")
	if errors.Is(err, store.ErrNoCard) {
		return finish(ctx, d, a, inbox.AskFailed, "", "the card no longer exists")
	}
	if err != nil {
		return a, err
	}
	msgs, err := d.Store.MessagesByID(ctx, c.Refs)
	if err != nil {
		return a, err
	}
	t, err := target(d.Config, c, msgs, a.Space)
	if err != nil {
		return finish(ctx, d, a, inbox.AskFailed, "", err.Error())
	}
	return deliver(ctx, d, t, a, wait)
}

func deliver(ctx context.Context, d Deps, t Target, a inbox.Ask, wait time.Duration) (inbox.Ask, error) {
	lctx, cancel := context.WithTimeout(ctx, LockWait)
	unlock, err := agent.Lock(lctx, d.LockDir, t.Space.ID)
	cancel()
	if errors.Is(err, agent.ErrBusy) {
		return finish(ctx, d, a, inbox.AskQueued, "", err.Error())
	}
	if err != nil {
		return a, err
	}
	defer unlock()

	r, err := render.Ask(t.Agent, t.Space.Title(), render.AskID(d.Now(), t.Space.ID, a.ID),
		render.AskInput{ID: a.ID, Card: t.Card, Messages: t.Messages, Text: a.Text}, d.CardCommand)
	if err != nil {
		return finish(ctx, d, a, inbox.AskFailed, "", err.Error())
	}
	newDriver := d.NewDriver
	if newDriver == nil {
		newDriver = agent.New
	}
	drv, err := newDriver(t.Agent, t.Space.ID, d.Store)
	if err != nil {
		return finish(ctx, d, a, inbox.AskFailed, "", err.Error())
	}
	res, err := drv.Ask(ctx, t.Card.Handle, wait, r)
	if err != nil {
		a, ferr := finish(ctx, d, a, inbox.AskFailed, "", err.Error())
		return a, errors.Join(err, ferr)
	}
	detail := strings.Trim(strings.Join([]string{r.Detail, res.Detail}, "; "), "; ")
	if res.Status != agent.StatusSent {
		return finish(ctx, d, a, inbox.AskQueued, "", detail)
	}
	return finish(ctx, d, a, inbox.AskSent, res.Handle, detail)
}

// finish records the state of the ask and returns the ask with it.
func finish(ctx context.Context, d Deps, a inbox.Ask, state, handle, detail string) (inbox.Ask, error) {
	now := d.Now()
	if err := d.Store.FinishAsk(ctx, a.ID, state, handle, detail, now); err != nil {
		return a, err
	}
	a.State, a.Handle, a.Detail = state, handle, detail
	if state == inbox.AskSent {
		a.SentAt = now.UTC()
	}
	return a, nil
}
