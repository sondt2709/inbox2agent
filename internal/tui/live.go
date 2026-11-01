package tui

import (
	"errors"
	"os"
	"reflect"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/sondt2709/inbox2agent/internal/config"
	"github.com/sondt2709/inbox2agent/internal/render"
)

// errConflict means that another process changed the item of the form after
// the form was made.
var errConflict = errors.New("another process changed this item")

// formItem returns the configuration item of a form key from formTarget, or
// "" for a form of a new item and for the forms of the Feed.
func formItem(key string) (kind, id string) {
	pre, id, _ := strings.Cut(key, ":")
	if key == "settings" {
		return config.KindSettings, ""
	}
	if id == "" || strings.HasPrefix(id, "+") {
		return "", ""
	}
	switch pre {
	case "acc":
		return config.KindAccount, id
	case "agt":
		return config.KindAgent, id
	case "sch":
		return config.KindSchedule, id
	case "spc":
		if id == config.UnsortedID {
			return config.KindUnsorted, ""
		}
		return config.KindSpace, id
	}
	return "", ""
}

// commitForm saves a change of the open form. Before the change, it checks
// that the item of the form in the file is the item when the form was made.
// When another process changed it, commitForm opens the conflict dialog and
// returns errConflict. When another process removed it, it returns a
// config.MissingError.
func (m *model) commitForm(fm *form, change func(*config.Config) error, tokens map[string]string) error {
	err := m.u.commit(func(c *config.Config) error {
		if fm != nil && fm.kind != "" && !fm.overwrite {
			cur, ok := config.Item(c, fm.kind, fm.id)
			if !ok {
				return config.MissingError{Kind: fm.kind, ID: fm.id}
			}
			if !reflect.DeepEqual(cur, fm.base) {
				return errConflict
			}
		}
		return change(c)
	}, tokens)
	if fm != nil {
		fm.overwrite = false
	}
	if errors.Is(err, errConflict) {
		m.conflictDialog(fm)
	}
	return err
}

// saveError shows the error of a save under the form. The conflict dialog
// already explains a conflict.
func saveError(fm *form, err error) {
	if !errors.Is(err, errConflict) {
		fm.err = "Cannot save: " + err.Error()
	}
}

// conflictDialog asks what to do when another process changed the item of
// the form: Reload discards the edits; Overwrite saves them.
func (m *model) conflictDialog(fm *form) {
	what := fm.kind + " " + fm.id
	if fm.id == "" {
		what = fm.kind
	}
	m.openDialog(&confirmDialog{name: "Changed in another process",
		text:  "Another process changed the " + what + " in config.toml after you opened this form. Reload discards your edits and shows the file. Overwrite saves your edits over the file.",
		enter: "r",
		choices: []choice{
			{"r", "Reload", func() tea.Cmd {
				st := readConfigFile(m.u.d.Paths.ConfigFile, m.u.cfgHash)
				if st.err != nil {
					m.setStatus("Cannot reload: config.toml has an error: "+st.err.Error(), true)
					return nil
				}
				if st.cfg != nil {
					m.u.cfg, m.u.cfgHash = st.cfg, st.hash
				}
				m.buildForm(true)
				m.setStatus("Reloaded the "+what+" from config.toml.", false)
				return m.loadConversations()
			}},
			{"o", "Overwrite", func() tea.Cmd {
				for _, b := range fm.buttons {
					if b.primary && m.form == fm {
						fm.overwrite = true
						return b.run()
					}
				}
				return nil
			}},
		}})
}

// fileState is the result of the check of config.toml on a refresh.
type fileState struct {
	base    string         // the hash of the dashboard when the check started
	hash    string         // the hash of the file; "" when the file did not change or cannot be read
	cfg     *config.Config // the loaded configuration, or nil when err is set
	err     error          // the file is invalid
	checked bool
}

// readConfigFile reads config.toml and parses it when its hash is not base.
// It runs outside the UI loop.
func readConfigFile(path, base string) fileState {
	st := fileState{base: base, checked: true}
	b, err := os.ReadFile(path)
	if err != nil {
		return st // no file, or it cannot be read: keep the configuration
	}
	if h := config.Hash(b); h != base {
		st.hash = h
		st.cfg, st.err = config.Parse(b)
		if st.err == nil {
			st.err = render.CheckAgents(st.cfg)
		}
		if st.err != nil {
			st.cfg = nil
		}
	}
	return st
}

// applyFile uses the configuration that another process wrote. An invalid file
// keeps the configuration of the dashboard. A form with unsaved edits stays;
// the other forms show the file.
func (m *model) applyFile(st fileState) tea.Cmd {
	if !st.checked || st.hash == "" || st.base != m.u.cfgHash {
		return nil // no change, or the dashboard saved while the check ran
	}
	if st.err != nil {
		// Show the error again on each refresh, so it stays until the file is valid.
		first, _, _ := strings.Cut(st.err.Error(), "\n")
		m.setStatus("config.toml has an error. The dashboard keeps the last valid configuration: "+first, true)
		return nil
	}
	old := m.u.cfg
	m.u.cfg, m.u.cfgHash = st.cfg, st.hash
	for _, a := range old.Accounts {
		if n, ok := st.cfg.Account(a.ID); !ok || n.URL != a.URL || n.Platform != a.Platform {
			delete(m.chs, a.ID) // list its conversations again
			delete(m.chsErr, a.ID)
		}
	}
	c := m.u.cfg
	m.acc, m.agt, m.sch, m.spc = min(m.acc, len(c.Accounts)), min(m.agt, len(c.Agents)), min(m.sch, len(c.Schedules)), min(m.spc, len(c.Spaces))
	var cmd tea.Cmd
	if m.tab == tabFeed {
		// The forms of the Feed (reply, ask) do not show the configuration.
	} else if m.form != nil && m.form.dirty() {
		m.setStatus("config.toml changed in another process. Your unsaved edits stay; ctrl+s saves only this item.", false)
	} else {
		active, focus := m.form != nil && m.form.active, 0
		if m.form != nil {
			focus = m.form.focus
		}
		m.buildForm(true)
		if active && m.form != nil {
			m.form.focus = focus
			cmd = m.form.activate()
		}
		m.setStatus("config.toml changed in another process. The dashboard shows the new file.", false)
	}
	return tea.Batch(cmd, m.loadConversations())
}
