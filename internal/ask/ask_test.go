package ask

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sondt2709/inbox2agent/internal/agent"
	"github.com/sondt2709/inbox2agent/internal/config"
	"github.com/sondt2709/inbox2agent/internal/inbox"
	"github.com/sondt2709/inbox2agent/internal/render"
	"github.com/sondt2709/inbox2agent/internal/store"
)

// fakeDriver records the asks and returns a set result.
type fakeDriver struct {
	handle string
	wait   time.Duration
	asks   []*render.Rendered
	result agent.Result
	err    error
}

func (f *fakeDriver) Deliver(context.Context, *render.Rendered) (agent.Result, error) {
	return agent.Result{}, errors.New("unexpected batch")
}

func (f *fakeDriver) Ask(_ context.Context, handle string, wait time.Duration, a *render.Rendered) (agent.Result, error) {
	f.handle, f.wait = handle, wait
	f.asks = append(f.asks, a)
	return f.result, f.err
}

type env struct {
	d     Deps
	st    *store.Store
	drv   *fakeDriver
	card  inbox.Card
	msgID int64
	now   time.Time
}

// newEnv makes a store with one message that a dispatch of the profile
// "triage" sent, and one card from the terminal term_card that refers to it.
func newEnv(t *testing.T) *env {
	t.Helper()
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "inbox.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	ctx := context.Background()
	now := time.Date(2026, 10, 29, 13, 0, 0, 0, time.UTC)
	if _, err := st.SavePoll(ctx, "work", []inbox.Message{{SourceID: "work", SourceType: "mattermost", ExternalID: "p1",
		ChannelName: "pf-demo", Author: "lena.wong", Text: "Can PF get their own login?", CreatedAt: now}}, nil, now); err != nil {
		t.Fatal(err)
	}
	pending, _ := st.Pending(ctx)
	id, _ := st.CreateDispatch(ctx, store.Dispatch{Profile: "triage", Driver: config.DriverOrca, CreatedAt: now})
	if err := st.FinishDispatch(ctx, id, store.DispatchSent, "", "triage", inbox.StateSent, []int64{pending[0].ID}); err != nil {
		t.Fatal(err)
	}
	saved, err := st.SaveCards(ctx, []inbox.Card{{Key: "mm:lena-pf-login", Category: inbox.CategoryAction, Urgency: inbox.UrgencyHigh,
		Title: "Lena needs an answer", Body: "No check of the auth code yet.", Refs: []int64{pending[0].ID}, Handle: "term_card"}}, now)
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.Default()
	cfg.Profiles = []config.Profile{config.NewProfile("first"), config.NewProfile("triage")}
	for i := range cfg.Profiles {
		cfg.Profiles[i].Driver = config.DriverOrca
	}
	drv := &fakeDriver{result: agent.Result{Status: agent.StatusSent, Detail: "sent to term_card, turn started", Handle: "term_card"}}
	e := &env{st: st, drv: drv, card: saved[0].Card, msgID: pending[0].ID, now: now}
	e.d = Deps{Store: st, Config: cfg, CardCommand: "/usr/local/bin/i2a", LockDir: dir, Now: func() time.Time { return e.now },
		NewDriver: func(p config.Profile, _ agent.Sessions) (agent.Driver, error) {
			if p.Name != "triage" {
				t.Errorf("driver of profile %q, want triage", p.Name)
			}
			return drv, nil
		}}
	return e
}

func (e *env) asks(t *testing.T) []inbox.Ask {
	t.Helper()
	by, err := e.st.Asks(context.Background(), []int64{e.card.ID})
	if err != nil {
		t.Fatal(err)
	}
	return by[e.card.ID]
}

func TestSendToTheTerminalOfTheCard(t *testing.T) {
	e := newEnv(t)
	a, err := Send(context.Background(), e.d, e.card, "  Check the auth code.\n", Wait)
	if err != nil {
		t.Fatal(err)
	}
	if a.State != inbox.AskSent || a.Handle != "term_card" || a.Profile != "triage" || a.Text != "Check the auth code." {
		t.Errorf("ask = %+v", a)
	}
	if e.drv.handle != "term_card" || e.drv.wait != Wait {
		t.Errorf("driver got handle %q and wait %s", e.drv.handle, e.drv.wait)
	}
	p := e.drv.asks[0].Prompt
	if !strings.Contains(p, "--- ask ---\nCheck the auth code.\n--- end ---") || !strings.Contains(p, `"text":"Can PF get their own login?"`) ||
		!strings.Contains(p, `/usr/local/bin/i2a card add and the key "mm:lena-pf-login"`) {
		t.Errorf("prompt = %s", p)
	}
	got := e.asks(t)
	if len(got) != 1 || got[0].State != inbox.AskSent || got[0].Handle != "term_card" || got[0].SentAt.IsZero() ||
		got[0].Detail != "sent to term_card, turn started" {
		t.Errorf("stored asks = %+v", got)
	}
}

func TestSendBusyAgentStaysQueued(t *testing.T) {
	e := newEnv(t)
	e.drv.result = agent.Result{Status: agent.StatusHeld, Detail: "agent busy in term_card"}
	a, err := Send(context.Background(), e.d, e.card, "more", Wait)
	if err != nil || a.State != inbox.AskQueued {
		t.Fatalf("ask = %+v, %v", a, err)
	}
	if got := e.asks(t); len(got) != 1 || got[0].State != inbox.AskQueued || got[0].Detail != "agent busy in term_card" {
		t.Errorf("stored asks = %+v", got)
	}
}

func TestSendOrcaErrorFails(t *testing.T) {
	e := newEnv(t)
	e.drv.err = errors.New("run orca: no such file or directory")
	if _, err := Send(context.Background(), e.d, e.card, "more", Wait); err == nil || !strings.Contains(err.Error(), "no such file") {
		t.Fatalf("err = %v", err)
	}
	if got := e.asks(t); len(got) != 1 || got[0].State != inbox.AskFailed || !strings.Contains(got[0].Detail, "no such file") {
		t.Errorf("stored asks = %+v", got)
	}
}

func TestSendRefusesBadTextAndDryRun(t *testing.T) {
	e := newEnv(t)
	if _, err := Send(context.Background(), e.d, e.card, " \n ", Wait); err == nil || err.Error() != "the text is empty" {
		t.Errorf("empty text: %v", err)
	}
	if _, err := Send(context.Background(), e.d, e.card, strings.Repeat("x", 4001), Wait); err == nil || !strings.Contains(err.Error(), "the maximum is 4000") {
		t.Errorf("long text: %v", err)
	}
	e.d.Config.Profiles[1].Driver = config.DriverDryRun
	_, err := Send(context.Background(), e.d, e.card, "more", Wait)
	var dr *DryRunError
	if !errors.As(err, &dr) || err.Error() != "The profile triage uses dry-run; no agent reads asks." {
		t.Errorf("dry run: %v", err)
	}
	if got := e.asks(t); len(got) != 0 || len(e.drv.asks) != 0 {
		t.Errorf("asks = %+v, driver calls = %d; want none", got, len(e.drv.asks))
	}
}

func TestResolveProfile(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	tg, err := Resolve(ctx, e.d, e.card)
	if err != nil || tg.Profile.Name != "triage" || len(tg.Messages) != 1 {
		t.Fatalf("target = %+v, %v; want the profile of the message", tg, err)
	}
	if a, s := tg.Agent(), tg.Session(); a != "triage · orca" || s != "terminal term_card (wrote the card)" {
		t.Errorf("agent = %q, session = %q", a, s)
	}
	long := tg
	long.Card.Handle = "term_b9f22da8-13d6-49bc-a1ab-53eba56c9578"
	if s := long.Session(); s != "terminal term_b9f22da8… (wrote the card)" {
		t.Errorf("session = %q", s)
	}

	noRefs := e.card
	noRefs.Refs, noRefs.Handle = nil, ""
	if tg, err := Resolve(ctx, e.d, noRefs); err != nil || tg.Profile.Name != "first" || tg.Session() != "the session of the profile" {
		t.Errorf("card without refs: %+v, %v; want the first profile", tg.Profile.Name, err)
	}

	e.d.Config.Profiles = e.d.Config.Profiles[:1]
	if _, err := Resolve(ctx, e.d, e.card); err == nil || err.Error() != `the profile "triage" of the card is not in the configuration` {
		t.Errorf("removed profile: %v", err)
	}
}

func TestSendWaitsForTheProfileLock(t *testing.T) {
	e := newEnv(t)
	old := LockWait
	LockWait = 200 * time.Millisecond
	t.Cleanup(func() { LockWait = old })
	unlock, err := agent.Lock(context.Background(), e.d.LockDir, "triage")
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()
	a, err := Send(context.Background(), e.d, e.card, "more", Wait)
	if err != nil || a.State != inbox.AskQueued || len(e.drv.asks) != 0 {
		t.Errorf("ask = %+v, %v, driver calls = %d; want queued and no call", a, err, len(e.drv.asks))
	}
}

func TestRetry(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	e.drv.result = agent.Result{Status: agent.StatusHeld, Detail: "agent busy in term_card"}
	a, _ := Send(ctx, e.d, e.card, "more", Wait)

	e.drv.result = agent.Result{Status: agent.StatusSent, Detail: "sent to term_card, turn started", Handle: "term_card"}
	e.now = e.now.Add(10 * time.Minute)
	got, err := Retry(ctx, e.d, a, 24*time.Hour, 5*time.Second)
	if err != nil || got.State != inbox.AskSent || e.drv.wait != 5*time.Second {
		t.Errorf("retry = %+v, %v, wait %s", got, err, e.drv.wait)
	}

	old, _ := Send(ctx, e.d, e.card, "old ask", Wait)
	e.now = e.now.Add(25 * time.Hour)
	if got, err := Retry(ctx, e.d, old, 24*time.Hour, 5*time.Second); err != nil || got.State != inbox.AskFailed || got.Detail != "not sent within 1d" {
		t.Errorf("old ask = %+v, %v", got, err)
	}

	gone := old
	gone.CreatedAt, gone.CardID = e.now, 999
	if got, err := Retry(ctx, e.d, gone, 24*time.Hour, 5*time.Second); err != nil || got.State != inbox.AskFailed || got.Detail != "the card no longer exists" {
		t.Errorf("ask of a deleted card = %+v, %v", got, err)
	}
}
