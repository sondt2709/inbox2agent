package tui

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sondt2709/inbox2agent/internal/agent"
	"github.com/sondt2709/inbox2agent/internal/config"
	"github.com/sondt2709/inbox2agent/internal/inbox"
)

// The Orca outputs and the Claude files are the fixtures of internal/agent:
// real orca 1.4.240 outputs and real ~/.claude/sessions files with the IDs
// and paths replaced.
const (
	fixSessionA = "04ebe19c-543e-42ff-8f84-ea13cea7a3ec" // 4242.json, cwd /home/user/notes
	fixSessionB = "d9c92c0a-8869-4bbb-954b-16f89a3abd8f" // 4343.json, cwd /home/user/other
)

type sessionsHarness struct {
	*feedHarness
	handles  map[int]string // the terminal of each running Claude pid
	show     string         // the answer of orca terminal show
	switched []string
}

func newSessionsModel(t *testing.T) *sessionsHarness {
	t.Helper()
	h := &sessionsHarness{feedHarness: newFeedModel(t), handles: map[int]string{}, show: "show_claude_live.json"}
	dir := t.TempDir()
	for _, n := range []string{"4242.json", "4343.json"} {
		b, err := os.ReadFile(filepath.Join("..", "agent", "testdata", "claude", "sessions", n))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, n), b, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	start := map[int]time.Time{4242: time.Date(2026, 11, 4, 11, 19, 18, 0, time.UTC), 4343: time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)}
	h.m.u.d.Inspector = func(config.Agent) *agent.Inspector {
		return &agent.Inspector{Bin: "orca",
			Run: func(_ context.Context, _ string, args ...string) ([]byte, error) {
				name := h.show
				if args[0] == "terminal" && args[1] == "switch" {
					h.switched = append(h.switched, args[3])
					name = "switch_ok.json"
				}
				return os.ReadFile(filepath.Join("..", "agent", "testdata", name))
			},
			Claude: &agent.ClaudeFinder{Dir: dir, Proc: func(pid int) (agent.ProcInfo, error) {
				hd, ok := h.handles[pid]
				if !ok {
					return agent.ProcInfo{}, agent.ErrNoProcess
				}
				return agent.ProcInfo{Start: start[pid], Handle: hd}, nil
			}}}
	}
	cfg := h.m.u.cfg
	orca := func(id string) config.Agent {
		a := config.NewAgent(id)
		a.Driver, a.Folder, a.OrcaBin = config.DriverOrca, "/home/user/notes", "/usr/local/bin/orca"
		return a
	}
	cfg.Unsorted.Agent = orca(config.UnsortedID)
	cfg.Spaces = append(cfg.Spaces, config.Space{ID: "acme", Name: "Acme", Agent: orca("acme"), Schedule: "always"})
	at(h.m, tabSpaces)
	return h
}

// open opens the sessions dialog of a space, as enter on its row Session does.
func (h *sessionsHarness) open(space string) {
	sp, _ := h.m.u.cfg.Space(space)
	run(h.m, h.m.openSessions(*sp), 0)
}

func (h *sessionsHarness) session(t *testing.T, space string) inbox.Session {
	t.Helper()
	ss, err := h.st.Session(context.Background(), space)
	if err != nil {
		t.Fatal(err)
	}
	return ss
}

func (h *sessionsHarness) set(t *testing.T, ss inbox.Session) {
	t.Helper()
	ss.CreatedAt = time.Now()
	if err := h.st.SetSession(context.Background(), ss); err != nil {
		t.Fatal(err)
	}
}

func TestSessionRowAndDialog(t *testing.T) {
	h := newSessionsModel(t)
	h.set(t, inbox.Session{Space: config.UnsortedID, Handle: "term_a", AgentSession: fixSessionA})
	h.handles[4242] = "term_a"
	h.m.selectSpace(config.UnsortedID)
	if s := screen(h.m); !strings.Contains(s, "Session    term_a · Claude 04ebe19c") {
		t.Fatalf("the panel of Everything else does not show the session:\n%s", s)
	}
	h.m.selectSpace("acme")
	if s := screen(h.m); !strings.Contains(s, "Session    none") {
		t.Fatalf("the panel of Acme does not show no session:\n%s", s)
	}
	h.m.selectSpace(config.UnsortedID)
	run(h.m, h.m.settingDialog(railItem{kind: railElse, space: config.UnsortedID}, "session"), 0)
	s := screen(h.m)
	for _, want := range []string{"Session of Unsorted", "Unsorted  live", "term_a  Claude 04ebe19c  Mattermost inbox triage"} {
		if !strings.Contains(s, want) {
			t.Errorf("the dialog has no %q:\n%s", want, s)
		}
	}
	if strings.Contains(s, "Acme") && strings.Contains(s, "no session\n") {
		t.Errorf("the dialog of Unsorted shows another space:\n%s", s)
	}
	send(h.m, "o")
	if len(h.switched) != 1 || h.switched[0] != "term_a" {
		t.Errorf("switched = %v, want term_a", h.switched)
	}
	if s := screen(h.m); !strings.Contains(s, "Orca shows term_a.") {
		t.Errorf("the dialog does not show the result of o:\n%s", s)
	}
}

func TestSessionOfAnotherClaudeSession(t *testing.T) {
	h := newSessionsModel(t)
	h.set(t, inbox.Session{Space: config.UnsortedID, Handle: "term_a", AgentSession: fixSessionB})
	h.handles[4242] = "term_a" // term_a runs session A now
	h.open(config.UnsortedID)
	if s := screen(h.m); !strings.Contains(s, "Unsorted  other session") || !strings.Contains(s, "Claude d9c92c0a (runs 04ebe19c now)") {
		t.Errorf("the dialog does not show the other session:\n%s", s)
	}
}

func TestLinkSessionByHandle(t *testing.T) {
	h := newSessionsModel(t)
	h.handles[4242] = "term_c"
	h.open(config.UnsortedID)
	send(h.m, "l")
	h.m.topDialog().(*inputDialog).input.SetValue("term_c")
	send(h.m, "enter")
	ss := h.session(t, config.UnsortedID)
	if ss.Handle != "term_c" || ss.AgentSession != fixSessionA {
		t.Fatalf("session = %+v, want term_c with Claude session A", ss)
	}
	if s := screen(h.m); !strings.Contains(s, "Linked term_c (Claude 04ebe19c) to Unsorted") {
		t.Errorf("no message:\n%s", s)
	}
}

func TestLinkSessionByIDOutsideTheFolder(t *testing.T) {
	h := newSessionsModel(t)
	h.handles[4343] = "term_d" // cwd /home/user/other
	h.open("acme")
	send(h.m, "l")
	h.m.topDialog().(*inputDialog).input.SetValue(fixSessionB)
	send(h.m, "enter")
	s := screen(h.m)
	if !strings.Contains(s, "The session runs in /home/user/other, not in the folder") {
		t.Fatalf("no confirmation for another workspace:\n%s", s)
	}
	if ss := h.session(t, "acme"); ss.Handle != "" {
		t.Fatalf("stored before the confirmation: %+v", ss)
	}
	send(h.m, "y")
	if ss := h.session(t, "acme"); ss.Handle != "term_d" || ss.AgentSession != fixSessionB {
		t.Errorf("session = %+v, want term_d", ss)
	}
}

func TestLinkSessionNoClaudeProcess(t *testing.T) {
	h := newSessionsModel(t)
	h.open(config.UnsortedID)
	send(h.m, "l")
	h.m.topDialog().(*inputDialog).input.SetValue("term_d")
	send(h.m, "enter")
	if s := screen(h.m); !strings.Contains(s, "no running Claude session in term_d") {
		t.Errorf("no error:\n%s", s)
	}
	if ss := h.session(t, config.UnsortedID); ss.Handle != "" {
		t.Errorf("session = %+v, want none", ss)
	}
}

func TestLinkSessionOfAnotherSpace(t *testing.T) {
	h := newSessionsModel(t)
	h.set(t, inbox.Session{Space: "acme", Handle: "term_c", AgentSession: fixSessionA})
	h.handles[4242] = "term_c"
	h.open(config.UnsortedID)
	send(h.m, "l")
	h.m.topDialog().(*inputDialog).input.SetValue("term_c")
	send(h.m, "enter")
	if s := screen(h.m); !strings.Contains(s, "The space Acme has this session") {
		t.Fatalf("no confirmation for the session of another space:\n%s", s)
	}
	send(h.m, "esc")
	if ss := h.session(t, config.UnsortedID); ss.Handle != "" {
		t.Errorf("session = %+v, want none after esc", ss)
	}
}

func TestUnlinkSession(t *testing.T) {
	h := newSessionsModel(t)
	h.set(t, inbox.Session{Space: config.UnsortedID, Handle: "term_a", AgentSession: fixSessionA})
	h.handles[4242] = "term_a"
	h.open(config.UnsortedID)
	send(h.m, "u", "y")
	if ss := h.session(t, config.UnsortedID); ss.Handle != "" {
		t.Errorf("session = %+v, want none", ss)
	}
}
