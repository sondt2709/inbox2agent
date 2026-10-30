package tui

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/sondt2709/inbox2agent/internal/config"
	"github.com/sondt2709/inbox2agent/internal/inbox"
	"github.com/sondt2709/inbox2agent/internal/source"
	"github.com/sondt2709/inbox2agent/internal/source/mattermost"
	"github.com/sondt2709/inbox2agent/internal/source/whatsapp"
)

// Help texts of the WhatsApp switches.
const (
	waDirectHelp   = "All direct chats. Off: only the chats that you select"
	waMentionsHelp = "Group messages that mention you or reply to a message of yours, in any group."
)

// validPhone accepts an empty value or a phone number in international format.
func validPhone(s string) error {
	if s == "" {
		return nil
	}
	digits := 0
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9':
			digits++
		case r == '+' || r == ' ' || r == '-' || r == '(' || r == ')':
		default:
			return errors.New("use digits, for example +15551234567")
		}
	}
	if digits < 7 || strings.HasPrefix(strings.TrimLeft(s, "+ "), "0") {
		return errors.New("use the international format with the country code, for example +15551234567")
	}
	return nil
}

// pendingSource is a source that the add form connected and that the user did not add yet.
type pendingSource struct {
	sc    config.Source
	token string
	user  string
}

func (m *model) hintsSources() []hint {
	if m.selectedSource() == nil {
		return []hint{{"↑↓", "move"}, {"enter", "add a source"}}
	}
	return []hint{{"↑↓", "move"}, {"enter", "edit"}, {"D", "direct on/off"}, {"m", "mentions on/off"}, {"x", "remove"}}
}

func (m *model) selectedSource() *config.Source {
	if m.src < len(m.u.cfg.Sources) {
		return &m.u.cfg.Sources[m.src]
	}
	return nil
}

func (m *model) keySources(k string) tea.Cmd {
	n := len(m.u.cfg.Sources) + 1
	switch k {
	case "up", "k":
		m.src = max(0, m.src-1)
		return m.loadChannels()
	case "down", "j":
		m.src = min(n-1, m.src+1)
		return m.loadChannels()
	case "a":
		m.src = n - 1
		m.buildForm(false)
		return m.focusForm()
	}
	sc := m.selectedSource()
	if sc == nil {
		return nil
	}
	id := sc.ID
	switch k {
	case "D", "m":
		next := clone(m.u.cfg)
		s, _ := next.Source(id)
		what, on := "All direct messages", false
		if k == "D" {
			s.AllDirect = !s.AllDirect
			on = s.AllDirect
		} else {
			s.Mentions = !s.Mentions
			what, on = "Mentions", s.Mentions
		}
		if err := m.u.commit(next, nil); err != nil {
			m.setStatus("Cannot save: "+err.Error(), true)
			return nil
		}
		m.saved(fmt.Sprintf("%s of %s: %s. Saved.", what, id, onOff(on)))
	case "x":
		text := "Remove the source " + id + " and delete its token?"
		if m.isWhatsApp(id) {
			text = "Remove the source " + id + ", log out the linked device and delete its file?"
		}
		m.openDialog(&confirmDialog{name: "Remove source " + id, text: text,
			choices: []choice{{"y", "Remove", func() tea.Cmd {
				wa := m.isWhatsApp(id)
				next := clone(m.u.cfg)
				next.Sources = slices.DeleteFunc(next.Sources, func(s config.Source) bool { return s.ID == id })
				if err := m.u.commit(next, nil, id); err != nil {
					m.setStatus("Cannot remove: "+err.Error(), true)
					return nil
				}
				m.src = min(m.src, len(next.Sources))
				delete(m.chs, id)
				if wa {
					m.saved("Removed source " + id + ". Logging out the linked device ...")
					return m.removeWhatsApp(id)
				}
				m.saved("Removed source " + id + ".")
				return nil
			}}}})
	}
	return nil
}

func (m *model) viewSources(w, h int) string {
	cfg := m.u.cfg
	items := make([]string, 0, len(cfg.Sources)+1)
	for _, s := range cfg.Sources {
		items = append(items, listLabel(s.ID, s.Type, listWidth(w)-6))
	}
	items = append(items, "+ Add source")
	m.src = min(m.src, len(items)-1)
	title := "New source"
	if sc := m.selectedSource(); sc != nil {
		title = sc.ID + " · " + sc.Type
	}
	return m.masterDetail("Sources", items, m.src, &m.srcOff, title, w, h)
}

// ---- channels ----

// loadChannels reads the channels of the selected source once, in the background.
func (m *model) loadChannels() tea.Cmd {
	sc := m.selectedSource()
	if sc == nil {
		return nil
	}
	if _, done := m.chs[sc.ID]; done || m.chsBusy[sc.ID] {
		return nil
	}
	if sc.Type == config.SourceWhatsApp {
		m.chsBusy[sc.ID] = true
		id, dataDir := sc.ID, m.u.d.Paths.DataDir
		return func() tea.Msg {
			chats, err := whatsapp.ReadChats(m.ctx, dataDir, id)
			return channelsMsg{source: id, chs: chatInfos(chats), err: err}
		}
	}
	token, err := m.u.d.Secrets.Get(secretsKey(sc.ID))
	if err != nil {
		m.chsErr[sc.ID] = "cannot read the token: " + err.Error()
		m.setPickerMsg(sc.ID)
		return nil
	}
	m.chsBusy[sc.ID] = true
	src := source.NewMattermost(*sc, token, m.u.cfg.Timing.Lookback.D(), &http.Client{Timeout: 30 * time.Second})
	id := sc.ID
	return func() tea.Msg {
		chs, err := src.ListChannels(m.ctx)
		return channelsMsg{source: id, chs: chs, err: err}
	}
}

func (m *model) gotChannels(msg channelsMsg) {
	m.chsBusy[msg.source] = false
	if msg.err != nil {
		m.chsErr[msg.source] = msg.err.Error()
	} else {
		m.chs[msg.source] = msg.chs
		delete(m.chsErr, msg.source)
	}
	// Give the list to the open form of that source; keep its other edits.
	if m.formFor != "src:"+msg.source || m.form == nil {
		return
	}
	f := m.form.field("channels")
	if f == nil || f.picker != nil {
		return
	}
	if msg.err != nil {
		f.pickerMsg = errStyle.Render("Cannot read the channels: " + msg.err.Error())
		return
	}
	sc, _ := m.u.cfg.Source(msg.source)
	f.picker = m.newChannelPicker(msg.chs, sc.Channels)
	f.initial = strings.Join(f.picker.selectedIDs(), ",")
	if m.form.active && m.form.current().field == f {
		f.picker.Focus()
	}
}

func (m *model) setPickerMsg(id string) {
	if m.formFor == "src:"+id && m.form != nil {
		if f := m.form.field("channels"); f != nil && f.picker == nil {
			f.pickerMsg = errStyle.Render(m.chsErr[id])
		}
	}
}

func (m *model) newChannelPicker(chs []mattermost.ChannelInfo, selected []string) *picker {
	var items []pickItem
	for _, c := range chs {
		items = append(items, pickItem{label: channelLabel(c), id: c.ID, selected: slices.Contains(selected, c.ID)})
	}
	return newPicker("", items, max(5, min(12, m.h-24)), new([]string))
}

func channelsField(m *model, id string, selected []string) *formField {
	f := &formField{key: "channels", label: "Channels", kind: kindChannels,
		help:    "Type to search · space selects · channels and direct or group conversations, the most recent first",
		initial: strings.Join(selected, ",")}
	switch {
	case m.chs[id] != nil:
		f.picker = m.newChannelPicker(m.chs[id], selected)
		f.initial = strings.Join(f.picker.selectedIDs(), ",")
	case m.chsErr[id] != "":
		f.pickerMsg = errStyle.Render("Cannot read the channels: " + m.chsErr[id])
	default:
		f.pickerMsg = "Loading the channels ..."
	}
	return f
}

// channelLabel is the text of a channel in the picker. The kind is a word in
// the label, so a search for "direct" or "group" finds the conversations.
func channelLabel(c mattermost.ChannelInfo) string {
	switch c.Kind {
	case inbox.KindDirect:
		return "direct / " + c.Name
	case inbox.KindGroup:
		return "group / " + c.Name
	}
	label := c.Name
	if c.Team != "" {
		label = c.Team + " / " + c.Name
	}
	if c.Private {
		label += " (private)"
	}
	return label
}

// ---- forms ----

func validURL(s string) error {
	if !strings.HasPrefix(s, "https://") && !strings.HasPrefix(s, "http://") {
		return errors.New("the URL must start with https:// or http://")
	}
	if len(s) < 10 {
		return errors.New("type the full server URL")
	}
	return nil
}

func (m *model) profileChoice(current string) *formField {
	var names []string
	for _, p := range m.u.cfg.Profiles {
		names = append(names, p.Name)
	}
	return choiceField("profile", "Profile", "The profile that sends the messages of this source to an agent.", names, nil,
		orStr(current, names[0]))
}

// sourceForm edits a saved source.
func (m *model) sourceForm(id string) *form {
	sc, _ := m.u.cfg.Source(id)
	wa := sc.Type == config.SourceWhatsApp
	notWA := func(f *formField) *formField {
		if wa {
			f.hidden = func() bool { return true }
		}
		return f
	}
	directHelp, mentionsHelp := "All direct and group messages. Off: only the conversations that you select below.",
		"Posts that mention you in any channel."
	if wa {
		directHelp, mentionsHelp = waDirectHelp+" below.", waMentionsHelp
	}
	fm := &form{}
	if wa {
		state := m.whatsAppState(id)
		st := okStyle
		if state != "paired" {
			st, state = errStyle, state+": remove this source and add it again to pair"
		}
		fm.fields = append(fm.fields, noteField("WhatsApp · linked device \""+whatsapp.DeviceName+"\" · "+state, st))
	}
	fm.fields = append(fm.fields,
		notWA(textField("url", "Server URL", sc.URL, "https://chat.example.com", "The address of the Mattermost server.", validURL)),
		notWA(secretField("token", "Token", "stored in the "+m.u.d.Secrets.Backend()+" · type a new token to replace it",
			"A personal access token. Leave it empty to keep the saved token.", nil)),
		textField("poll", "Poll every", sc.PollEvery.String(), "1m", "Time between two reads of this source.", minDuration(5*time.Second)),
		switchField("direct", "All direct", directHelp, sc.AllDirect),
		switchField("mentions", "Mentions", mentionsHelp, sc.Mentions),
		m.profileChoice(sc.Profile),
		channelsField(m, id, sc.Channels),
	)
	fm.buttons = []button{
		{label: "Save changes", primary: true, run: func() tea.Cmd { return m.saveSource(id) }},
		{label: "Cancel", run: func() tea.Cmd { m.buildForm(true); return nil }},
		{label: "Remove source", danger: true, run: func() tea.Cmd { m.form.deactivate(); return m.keySources("x") }},
	}
	return fm
}

func (m *model) saveSource(id string) tea.Cmd {
	fm := m.form
	if !fm.validate() {
		return nil
	}
	cur, _ := m.u.cfg.Source(id)
	next := clone(m.u.cfg)
	s, _ := next.Source(id)
	s.URL = fm.field("url").value()
	s.PollEvery, _ = config.ParseDuration(fm.field("poll").value())
	s.AllDirect = fm.field("direct").value() == "on"
	s.Mentions = fm.field("mentions").value() == "on"
	s.Profile = fm.field("profile").value()
	if ch := fm.field("channels").value(); ch != "" {
		s.Channels = strings.Split(ch, ",")
	} else {
		s.Channels = nil
	}
	newToken := fm.field("token").value()
	commit := func() tea.Cmd {
		var tokens map[string]string
		if newToken != "" {
			tokens = map[string]string{id: newToken}
		}
		if err := m.u.commit(next, tokens); err != nil {
			fm.err = "Cannot save: " + err.Error()
			return nil
		}
		if newToken != "" || s.URL != cur.URL {
			delete(m.chs, id) // read the channels again with the new token or server
		}
		m.saved("Saved source " + id + ".")
		return m.loadChannels()
	}
	if newToken == "" && s.URL == cur.URL {
		return commit()
	}
	// A new token or a new server: check the connection before the save.
	token := newToken
	if token == "" {
		saved, err := m.u.d.Secrets.Get(secretsKey(id))
		if err != nil {
			fm.err = "Cannot read the saved token: " + err.Error()
			return nil
		}
		token = saved
	}
	check := *s
	fm.busy = "Checking the connection to " + s.URL + " ..."
	return m.background(func(ctx context.Context) func() tea.Cmd {
		_, err := m.u.d.Check(ctx, check, token)
		return func() tea.Cmd {
			fm.busy = ""
			if err != nil {
				fm.err = "Connection failed. Nothing was saved.\n" + err.Error()
				return nil
			}
			return commit()
		}
	})
}

// newSourceForm is step 1 of "Add source": the account.
func (m *model) newSourceForm() *form {
	u := m.u
	fm := &form{}
	kind := choiceField("type", "Type", "Mattermost: a server and a personal access token. WhatsApp: i2a becomes a linked device of your phone.",
		[]string{config.SourceMattermost, config.SourceWhatsApp}, []string{"Mattermost", "WhatsApp"}, config.SourceMattermost)
	isWA := func() bool { return kind.value() == config.SourceWhatsApp }
	mmOnly := func(f *formField) *formField { f.hidden = isWA; return f }
	phone := textField("phone", "Phone number", "", "optional, for example +15551234567",
		"Empty: scan a QR code. A number in international format: type a pairing code on the phone instead.", validPhone)
	phone.hidden = func() bool { return !isWA() }
	warn := noteField("WhatsApp does not allow unofficial clients and can ban an account. i2a only reads messages, and never sends read receipts or presence.", warnStyle)
	warn.hidden = phone.hidden
	fm.fields = []*formField{
		noteField("Connect a Mattermost account, or pair a WhatsApp account. GitHub and Linear come later.", faint),
		kind,
		textField("id", "Source ID", nextID(u.cfg), "work", "A short name for this account, for example work or phone.", func(s string) error {
			if _, ok := u.cfg.Source(s); ok {
				return fmt.Errorf("the ID %q is used", s)
			}
			return validName(s)
		}),
		mmOnly(textField("url", "Server URL", "", "https://chat.example.com", "The address of the Mattermost server.", validURL)),
		mmOnly(secretField("token", "Token", "personal access token", "Mattermost: Profile › Security › Personal Access Tokens. i2a keeps it in the "+
			u.d.Secrets.Backend()+", not in config.toml.", func(s string) error {
			if s == "" {
				return errors.New("the token is necessary")
			}
			return nil
		})),
		phone,
		warn,
	}
	fm.buttons = []button{{label: "Connect", primary: true, run: func() tea.Cmd {
		if isWA() {
			if !fm.validate() {
				return nil
			}
			return m.startPairing(fm.field("id").value(), fm.field("phone").value())
		}
		return m.connectSource()
	}}}
	return fm
}

func (m *model) connectSource() tea.Cmd {
	fm := m.form
	if !fm.validate() {
		return nil
	}
	sc := config.NewMattermostSource(fm.field("id").value(), fm.field("url").value())
	sc.Profile = m.u.cfg.Profiles[0].Name
	tok := fm.field("token").value()
	fm.busy = "Connecting to " + sc.URL + " ..."
	return m.background(func(ctx context.Context) func() tea.Cmd {
		user, err := m.u.d.Check(ctx, sc, tok)
		var chs []mattermost.ChannelInfo
		if err == nil {
			src := source.NewMattermost(sc, tok, m.u.cfg.Timing.Lookback.D(), &http.Client{Timeout: 30 * time.Second})
			chs, err = src.ListChannels(ctx)
		}
		return func() tea.Cmd {
			fm.busy = ""
			if err != nil {
				fm.err = "Connection failed. Nothing was saved.\n" + err.Error()
				return nil
			}
			m.newSrc = &pendingSource{sc: sc, token: tok, user: user}
			m.chs["+new"] = chs
			m.buildForm(true)
			return m.form.activate() // the focus is on the channel list
		}
	})
}

// newSourceStep2 is step 2 of "Add source": what to watch.
func (m *model) newSourceStep2() *form {
	p := m.newSrc
	ch := &formField{key: "channels", label: "Channels", kind: kindChannels,
		help: "Type to search · space selects · channels and direct or group conversations, the most recent first"}
	ch.picker = m.newChannelPicker(m.chs["+new"], nil)
	note := "✓ Connected as " + p.user + " · " + p.sc.URL + " · source " + p.sc.ID
	directHelp, mentionsHelp := "All direct and group messages. Off: only the conversations that you select above.",
		"Posts that mention you in any channel."
	if p.sc.Type == config.SourceWhatsApp {
		note = "✓ Paired · source " + p.sc.ID + " · select the chats to follow"
		ch.label, ch.help = "Chats", "Type to search · space selects · groups and direct chats, the most recent first"
		directHelp, mentionsHelp = waDirectHelp+" above.", waMentionsHelp
	}
	fm := &form{}
	fm.fields = []*formField{
		noteField(note, okStyle),
		ch,
		switchField("direct", "All direct", directHelp, true),
		switchField("mentions", "Mentions", mentionsHelp, true),
		m.profileChoice(""),
	}
	fm.buttons = []button{
		{label: "Add source", primary: true, run: func() tea.Cmd { return m.addSource() }},
		{label: "Cancel", run: func() tea.Cmd {
			m.newSrc = nil
			m.buildForm(true)
			m.setStatus("Nothing was added.", false)
			return nil
		}},
	}
	return fm
}

func (m *model) addSource() tea.Cmd {
	fm, p := m.form, m.newSrc
	sc := p.sc
	if ch := fm.field("channels").value(); ch != "" {
		sc.Channels = strings.Split(ch, ",")
	}
	sc.AllDirect = fm.field("direct").value() == "on"
	sc.Mentions = fm.field("mentions").value() == "on"
	sc.Profile = fm.field("profile").value()
	next := clone(m.u.cfg)
	next.Sources = append(next.Sources, sc)
	var tokens map[string]string
	if p.token != "" {
		tokens = map[string]string{sc.ID: p.token}
	}
	if err := m.u.commit(next, tokens); err != nil {
		fm.err = "Cannot save: " + err.Error()
		return nil
	}
	m.chs[sc.ID] = m.chs["+new"]
	delete(m.chs, "+new")
	m.newSrc = nil
	m.src = len(next.Sources) - 1
	m.saved("Added source " + sc.ID + ".")
	return nil
}
