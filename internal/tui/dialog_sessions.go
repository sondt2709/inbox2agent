package tui

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/sondt2709/inbox2agent/internal/agent"
	"github.com/sondt2709/inbox2agent/internal/config"
	"github.com/sondt2709/inbox2agent/internal/inbox"
)

// sessionsDialog shows the session of each space of an agent: the Orca
// terminal handle, the Claude session and its state. The user links an
// existing Claude session to a space, unlinks it, or shows it in Orca.
type sessionsDialog struct {
	agent  config.Agent
	spaces []config.Space
	states map[string]agent.SessionState // by space ID; nil while it loads
	cursor int
	msg    string
	isErr  bool
}

// openDialogMsg opens a dialog after the dialog that sent it closed.
type openDialogMsg struct{ d dialog }

// sessionsNoteMsg is the result of an action of the sessions dialog, from
// work outside the update loop. The dialog shows it: the footer is under it.
type sessionsNoteMsg struct {
	agent, text string
	isErr       bool
}

// sessionStatesMsg carries the states that loadSessions read.
type sessionStatesMsg struct {
	agent  string
	states map[string]agent.SessionState
}

func (m *model) openSessions(a config.Agent) tea.Cmd {
	d := &sessionsDialog{agent: a, spaces: m.u.cfg.SpacesUsing("agent", a.ID)}
	m.openDialog(d)
	return m.loadSessions(a)
}

// shortID returns the first 8 characters of a Claude session ID.
func shortID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

// storedSessions returns the stored sessions by space ID.
func (m *model) storedSessions() map[string]inbox.Session {
	out := map[string]inbox.Session{}
	if m.u.d.Store == nil {
		return out
	}
	all, err := m.u.d.Store.Sessions(m.ctx)
	if err != nil {
		return out
	}
	for _, s := range all {
		out[s.Space] = s
	}
	return out
}

// loadSessions reads the state of each session of the agent outside the
// update loop: it runs orca and reads the Claude process files.
func (m *model) loadSessions(a config.Agent) tea.Cmd {
	stored := m.storedSessions()
	spaces := m.u.cfg.SpacesUsing("agent", a.ID)
	in := m.u.d.Inspector(a)
	ctx := m.ctx
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
		defer cancel()
		out := map[string]agent.SessionState{}
		for _, s := range spaces {
			ss, ok := stored[s.ID]
			if !ok {
				ss = inbox.Session{Space: s.ID}
			}
			out[s.ID] = in.State(ctx, ss)
		}
		return sessionStatesMsg{agent: a.ID, states: out}
	}
}

func (d *sessionsDialog) title() string { return "Sessions of " + d.agent.Title() }

func (d *sessionsDialog) size(W, H int) (int, int) {
	return min(W-4, 100), min(H-4, 2*len(d.spaces)+10)
}

func (d *sessionsDialog) update(m *model, msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		if len(d.spaces) == 0 {
			if msg.String() == "esc" || msg.String() == "enter" {
				m.closeDialog()
			}
			return nil
		}
		sp := d.spaces[d.cursor]
		st := d.states[sp.ID]
		switch msg.String() {
		case "esc":
			m.closeDialog()
		case "up", "k":
			d.cursor = max(0, d.cursor-1)
		case "down", "j":
			d.cursor = min(len(d.spaces)-1, d.cursor+1)
		case "l":
			m.openDialog(newInputDialog("Link a session to "+sp.Title(),
				"Paste the terminal handle (Orca: right-click in the terminal, then Copy Terminal ID) or the Claude session ID.",
				"", "term_… or a Claude session ID", func(v string) (string, tea.Cmd) { return m.linkSession(d, sp, v) }))
		case "u":
			if st.Handle == "" {
				d.msg, d.isErr = sp.Title()+" has no session.", true
				return nil
			}
			m.openDialog(&confirmDialog{name: "Unlink the session",
				text: fmt.Sprintf("Forget the session %s of %s? The tab stays open in Orca. The next batch starts a new session.", st.Handle, sp.Title()),
				choices: []choice{{"y", "Unlink", func() tea.Cmd {
					if err := m.u.d.Store.DeleteSession(m.ctx, sp.ID); err != nil {
						d.msg, d.isErr = "Cannot unlink: "+err.Error(), true
						return nil
					}
					d.msg, d.isErr = "Unlinked the session of "+sp.Title()+".", false
					m.buildForm(true)
					return m.loadSessions(d.agent)
				}}}})
		case "o":
			if st.Handle == "" {
				d.msg, d.isErr = sp.Title()+" has no session.", true
				return nil
			}
			in, ctx, h, id := m.u.d.Inspector(d.agent), m.ctx, st.Handle, d.agent.ID
			return func() tea.Msg {
				if err := in.Switch(ctx, h); err != nil {
					return sessionsNoteMsg{agent: id, text: "Cannot show " + h + " in Orca: " + err.Error(), isErr: true}
				}
				return sessionsNoteMsg{agent: id, text: "Orca shows " + h + "."}
			}
		}
	}
	return nil
}

// linkSession resolves the input and stores the session for the space,
// after a confirmation when the session is outside the workspace of the
// agent or another space has it. It returns an error text for the input.
func (m *model) linkSession(d *sessionsDialog, sp config.Space, input string) (string, tea.Cmd) {
	if m.u.d.Store == nil {
		return "No store yet: run i2a one time first.", nil
	}
	p, err := m.u.d.Inspector(d.agent).Resolve(input)
	if err != nil {
		return err.Error(), nil
	}
	var warns []string
	if ws, err := agent.ExpandWorkspace(d.agent.Workspace); err == nil && !inside(p.Cwd, ws) {
		warns = append(warns, fmt.Sprintf("The session runs in %s, not in the workspace %s of the agent.", p.Cwd, ws))
	}
	for id, s := range m.storedSessions() {
		if id != sp.ID && s.AgentSession == p.SessionID {
			warns = append(warns, fmt.Sprintf("The space %s has this session: the data of the two spaces goes to one session.", m.spaceTitle(id)))
		}
	}
	save := func() tea.Cmd {
		ss := inbox.Session{Space: sp.ID, Handle: p.Handle, AgentSession: p.SessionID, CreatedAt: m.now()}
		if err := m.u.d.Store.SetSession(m.ctx, ss); err != nil {
			d.msg, d.isErr = "Cannot link: "+err.Error(), true
			return nil
		}
		d.msg, d.isErr = fmt.Sprintf("Linked %s (Claude %s) to %s. The next batch goes there.", p.Handle, shortID(p.SessionID), sp.Title()), false
		m.buildForm(true)
		return m.loadSessions(d.agent)
	}
	if len(warns) == 0 {
		return "", save()
	}
	// The input dialog closes; the confirmation takes its place.
	c := &confirmDialog{name: "Link the session", text: strings.Join(warns, " ") + " Link it?", choices: []choice{{"y", "Link", save}}}
	return "", func() tea.Msg { return openDialogMsg{c} }
}

func (m *model) spaceTitle(id string) string {
	for _, s := range m.u.cfg.AllSpaces() {
		if s.ID == id {
			return s.Title()
		}
	}
	return id
}

// inside reports whether dir is ws or a folder in it.
func inside(dir, ws string) bool {
	rel, err := filepath.Rel(ws, filepath.Clean(dir))
	return err == nil && rel != ".." && !strings.HasPrefix(rel, "../")
}

func (d *sessionsDialog) view(w, _ int) string {
	if len(d.spaces) == 0 {
		return "\n" + textStyle.Width(w).Render("No space uses this agent.")
	}
	var rows []string
	for i, sp := range d.spaces {
		line1, line2 := sp.Title(), "  no session"
		if d.states == nil {
			line2 = "  reading…"
		} else if st := d.states[sp.ID]; st.Handle != "" {
			claude := "Claude " + orStr(shortID(st.AgentSession), "?")
			if st.Current != "" {
				claude += " (runs " + shortID(st.Current) + " now)"
			}
			line1 += "  " + st.State
			line2 = "  " + st.Handle + "  " + claude
			if name := orStr(st.Name, st.Title); name != "" {
				line2 += "  " + name
			}
		}
		if i == d.cursor {
			rows = append(rows, selectedRow.Width(w).Render(truncate("▌ "+ansi.Strip(line1), w)))
		} else {
			rows = append(rows, "  "+textStyle.Render(truncate(line1, w-2)))
		}
		// The handle stays on one line: Orca makes term_… a link (cmd+click).
		rows = append(rows, faint.Render(truncate(line2, w)))
	}
	out := "\n" + strings.Join(rows, "\n")
	if d.msg != "" {
		st := faint
		if d.isErr {
			st = errStyle
		}
		out += "\n\n" + st.Width(w).Render(d.msg)
	}
	return out
}

func (d *sessionsDialog) hints() []hint {
	return []hint{{"↑↓", "move"}, {"l", "link"}, {"u", "unlink"}, {"o", "open in Orca"}, {"esc", "close"}}
}
