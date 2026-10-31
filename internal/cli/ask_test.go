package cli

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/sondt2709/inbox2agent/internal/agent"
	"github.com/sondt2709/inbox2agent/internal/app"
	"github.com/sondt2709/inbox2agent/internal/config"
	"github.com/sondt2709/inbox2agent/internal/render"
)

// cliAgent records the asks of "i2a ask" and returns a set result.
type cliAgent struct {
	handles []string
	asks    []*render.Rendered
	result  agent.Result
}

func (a *cliAgent) Deliver(context.Context, *render.Rendered) (agent.Result, error) {
	return agent.Result{}, nil
}

func (a *cliAgent) Ask(_ context.Context, handle string, _ time.Duration, r *render.Rendered) (agent.Result, error) {
	a.handles = append(a.handles, handle)
	a.asks = append(a.asks, r)
	return a.result, nil
}

// askEnv makes an I2A_HOME with an orca agent for Unsorted and the card mm:lena from the terminal term_card.
func askEnv(t *testing.T, driver string) *cliAgent {
	t.Helper()
	cardEnv(t)
	paths, _ := app.ResolvePaths()
	cfg := config.Default()
	cfg.Agents[0].Driver, cfg.Agents[0].Workspace = driver, t.TempDir()
	if err := config.Save(paths.ConfigFile, cfg); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ORCA_TERMINAL_HANDLE", "term_card")
	if r := i2a(t, `{"category":"action","title":"Lena needs an answer","key":"mm:lena"}`, "card", "add"); r.err != nil {
		t.Fatal(r.err)
	}
	a := &cliAgent{result: agent.Result{Status: agent.StatusSent, Detail: "sent to term_card, turn started", Handle: "term_card"}}
	old := askDriver
	askDriver = func(config.Agent, string, agent.Sessions) (agent.Driver, error) { return a, nil }
	t.Cleanup(func() { askDriver = old })
	return a
}

func TestAskCommand(t *testing.T) {
	a := askEnv(t, config.DriverOrca)
	r := i2a(t, "Add the effort in hours.\n", "ask", "--card", "mm:lena")
	if r.err != nil {
		t.Fatalf("ask: %v %s", r.err, r.errOut)
	}
	if r.out != "sent to the agent of the space Unsorted (term_card). Ask 1.\n" {
		t.Errorf("output = %q", r.out)
	}
	if len(a.asks) != 1 || a.handles[0] != "term_card" || !strings.Contains(a.asks[0].Prompt, "--- ask ---\nAdd the effort in hours.\n--- end ---") {
		t.Fatalf("asks = %v to %v", a.asks, a.handles)
	}

	a.result = agent.Result{Status: agent.StatusHeld, Detail: "agent busy in term_card"}
	r = i2a(t, "And the risks.", "ask", "--card", "1")
	if r.err != nil || r.out != "the agent of the space Unsorted is busy; the daemon sends the ask within a minute. Ask 2.\n" {
		t.Errorf("busy: %v %q", r.err, r.out)
	}
}

func TestAskCommandErrors(t *testing.T) {
	askEnv(t, config.DriverOrca)
	for _, tc := range []struct {
		stdin string
		args  []string
		want  string
	}{
		{"x", []string{"ask"}, "give --card <id|key>"},
		{"x", []string{"ask", "--card", "missing"}, `no card has the ID or key "missing"`},
		{"", []string{"ask", "--card", "mm:lena"}, "the text is empty"},
		{strings.Repeat("x", 4500), []string{"ask", "--card", "mm:lena"}, "the text has 4500 characters; the maximum is 4000"},
	} {
		if r := i2a(t, tc.stdin, tc.args...); r.err == nil || r.err.Error() != tc.want {
			t.Errorf("%v: %v, want %q", tc.args, r.err, tc.want)
		}
	}
}

func TestAskCommandDryRun(t *testing.T) {
	a := askEnv(t, config.DriverDryRun)
	r := i2a(t, "more", "ask", "--card", "mm:lena")
	if r.err == nil || r.err.Error() != "The space Unsorted has no agent that reads asks." || len(a.asks) != 0 {
		t.Errorf("dry run: %v, asks = %d", r.err, len(a.asks))
	}
}
