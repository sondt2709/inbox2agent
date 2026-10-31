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
	"github.com/sondt2709/inbox2agent/internal/source"
	"github.com/sondt2709/inbox2agent/internal/source/whatsapp"
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

func validURL(s string) error {
	if !strings.HasPrefix(s, "https://") && !strings.HasPrefix(s, "http://") {
		return errors.New("the URL must start with https:// or http://")
	}
	if len(s) < 10 {
		return errors.New("type the full server URL")
	}
	return nil
}

func (m *model) hintsAccounts() []hint {
	if m.selectedAccount() == nil {
		return []hint{{"↑↓", "move"}, {"enter", "add an account"}}
	}
	return []hint{{"↑↓", "move"}, {"enter", "edit"}, {"x", "remove"}, {"4", "its conversations"}}
}

func (m *model) selectedAccount() *config.Account {
	if m.acc < len(m.u.cfg.Accounts) {
		return &m.u.cfg.Accounts[m.acc]
	}
	return nil
}

func (m *model) keyAccounts(k string) tea.Cmd {
	n := len(m.u.cfg.Accounts) + 1
	switch k {
	case "up", "k":
		m.acc = max(0, m.acc-1)
	case "down", "j":
		m.acc = min(n-1, m.acc+1)
	case "a":
		m.acc = n - 1
		m.buildForm(false)
		return m.focusForm()
	case "x":
		if a := m.selectedAccount(); a != nil {
			return m.removeAccount(a.ID)
		}
	}
	return nil
}

func (m *model) viewAccounts(w, h int) string {
	cfg := m.u.cfg
	items := make([]string, 0, len(cfg.Accounts)+1)
	for _, a := range cfg.Accounts {
		items = append(items, listLabel(a.Title(), platformName(a.Platform), listWidth(w)-6))
	}
	items = append(items, "+ Add account")
	m.acc = min(m.acc, len(items)-1)
	title := "New account"
	if a := m.selectedAccount(); a != nil {
		title = a.Title() + " · " + platformName(a.Platform) + " · ID " + a.ID
	}
	return m.masterDetail("Accounts", items, m.acc, &m.accOff, title, w, h)
}

func platformName(p string) string {
	switch p {
	case config.PlatformMattermost:
		return "Mattermost"
	case config.PlatformWhatsApp:
		return "WhatsApp"
	}
	return p
}

// accountInfo returns the lines that identify an account for a person.
func (m *model) accountInfo(a config.Account) []string {
	switch a.Platform {
	case config.PlatformWhatsApp:
		d, state := m.waDevices[a.ID], orStr(m.waStates[a.ID], "…")
		lines := []string{"Phone       " + orStr(d.Phone, "unknown")}
		if d.Name != "" {
			lines = append(lines, "Name        "+d.Name)
		}
		return append(lines, "Device      linked device \""+whatsapp.DeviceName+"\" · "+state)
	case config.PlatformMattermost:
		user := orStr(m.mmUsers[a.ID], a.User)
		if user != "" {
			user = "@" + user
		}
		return []string{"Server      " + a.URL, "User        " + orStr(user, "… (checked when i2a lists the conversations)")}
	}
	return nil
}

// accountSpaces describes where the conversations of an account are, for
// example "Acme 3 · Personal 2 · Unsorted 5".
func (m *model) accountSpaces(id string) string {
	by := map[string]int{}
	total := 0
	for _, r := range m.convRows() {
		if r.Account.ID == id {
			by[r.Space]++
			total++
		}
	}
	if total == 0 {
		return "No conversation found yet."
	}
	var parts []string
	for _, s := range m.u.cfg.AllSpaces() {
		if n := by[s.ID]; n > 0 {
			parts = append(parts, fmt.Sprintf("%s %d", s.Title(), n))
		}
	}
	return plural(total, "conversation", "conversations") + ": " + strings.Join(parts, " · ")
}

// accountForm edits a saved account.
func (m *model) accountForm(id string) *form {
	a, _ := m.u.cfg.Account(id)
	mm := a.Platform == config.PlatformMattermost
	mmOnly := func(f *formField) *formField {
		if !mm {
			f.hidden = func() bool { return true }
		}
		return f
	}
	fm := &form{}
	for _, l := range m.accountInfo(*a) {
		st := faint
		if strings.Contains(l, "not paired") {
			st = errStyle
		}
		fm.fields = append(fm.fields, noteField(l, st))
	}
	fm.fields = append(fm.fields,
		noteField(m.accountSpaces(id), textStyle),
		noteField("This account does not choose a space or an agent. Move its conversations to spaces in tab 4.", faint),
		textField("name", "Name", a.Title(), a.ID, "The name that i2a shows, for example Company A or Personal phone.", required),
		mmOnly(textField("url", "Server URL", a.URL, "https://chat.example.com", "The address of the Mattermost server.", validURL)),
		mmOnly(secretField("token", "Token", "stored in the "+m.u.d.Secrets.Backend()+" · type a new token to replace it",
			"A personal access token. Leave it empty to keep the saved token.", nil)),
		textField("read", "Read every", a.ReadEvery.String(), "1m",
			"Advanced. Time between two reads of this account. This is not when the agent gets messages: that is the schedule of each space.",
			minDuration(5*time.Second)),
	)
	fm.buttons = []button{
		{label: "Save changes", primary: true, run: func() tea.Cmd { return m.saveAccount(id) }},
		{label: "Cancel", run: func() tea.Cmd { m.buildForm(true); return nil }},
		{label: "Remove account", danger: true, run: func() tea.Cmd { m.form.deactivate(); return m.removeAccount(id) }},
	}
	return fm
}

func (m *model) saveAccount(id string) tea.Cmd {
	fm := m.form
	if !fm.validate() {
		return nil
	}
	cur, ok := m.u.cfg.Account(id)
	if !ok {
		fm.err = "Cannot save: " + config.MissingError{Kind: config.KindAccount, ID: id}.Error()
		return nil
	}
	acc := *cur
	a := &acc
	a.Name = fm.field("name").value()
	a.ReadEvery, _ = config.ParseDuration(fm.field("read").value())
	if a.Platform == config.PlatformMattermost {
		a.URL = strings.TrimRight(fm.field("url").value(), "/")
	}
	newToken := fm.field("token").value()
	commit := func(user string) tea.Cmd {
		var tokens map[string]string
		if newToken != "" {
			tokens = map[string]string{id: newToken}
		}
		if user != "" {
			a.User = user
		}
		if err := m.commitForm(fm, func(c *config.Config) error {
			x, ok := c.Account(id)
			if !ok {
				return config.MissingError{Kind: config.KindAccount, ID: id}
			}
			*x = *a
			return nil
		}, tokens); err != nil {
			saveError(fm, err)
			return nil
		}
		if newToken != "" || a.URL != cur.URL {
			delete(m.chs, id) // list the conversations again with the new token or server
		}
		m.saved("Saved account " + a.Title() + ".")
		return m.loadConversations()
	}
	if a.Platform != config.PlatformMattermost || newToken == "" && a.URL == cur.URL {
		return commit("")
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
	check := *a
	fm.busy = "Checking the connection to " + a.URL + " ..."
	return m.background(func(ctx context.Context) func() tea.Cmd {
		user, err := m.u.d.Check(ctx, check, token)
		return func() tea.Cmd {
			fm.busy = ""
			if err != nil {
				fm.err = "Connection failed. Nothing was saved.\n" + err.Error()
				return nil
			}
			return commit(user)
		}
	})
}

// removeAccount asks, then removes the account, its token, and its
// conversations from all spaces. A WhatsApp account also logs out its device.
func (m *model) removeAccount(id string) tea.Cmd {
	a, ok := m.u.cfg.Account(id)
	if !ok {
		return nil
	}
	title, wa := a.Title(), a.Platform == config.PlatformWhatsApp
	text := "Remove the account " + title + ", delete its token, and remove its conversations from all spaces?"
	if wa {
		text = "Remove the account " + title + ", log out the linked device, delete its file, and remove its chats from all spaces?"
	}
	m.openDialog(&confirmDialog{name: "Remove account " + title, text: text,
		choices: []choice{{"y", "Remove", func() tea.Cmd {
			if err := m.u.commit(func(c *config.Config) error {
				c.Accounts = slices.DeleteFunc(c.Accounts, func(a config.Account) bool { return a.ID == id })
				drop := func(list []config.Conversation) []config.Conversation {
					return slices.DeleteFunc(list, func(cv config.Conversation) bool { return cv.Account == id })
				}
				c.Unsorted.Conversations = drop(c.Unsorted.Conversations)
				for i := range c.Spaces {
					c.Spaces[i].Conversations = drop(c.Spaces[i].Conversations)
				}
				return nil
			}, nil, id); err != nil {
				m.setStatus("Cannot remove: "+err.Error(), true)
				return nil
			}
			m.acc = min(m.acc, len(m.u.cfg.Accounts))
			delete(m.chs, id)
			if wa {
				m.saved("Removed account " + title + ". Logging out the linked device ...")
				return m.removeWhatsApp(id)
			}
			m.saved("Removed account " + title + ".")
			return nil
		}}}})
	return nil
}

// newAccountForm adds an account. After the connection, its conversations are in Unsorted.
func (m *model) newAccountForm() *form {
	u := m.u
	fm := &form{}
	kind := choiceField("platform", "Platform", "Mattermost: a server and a personal access token. WhatsApp: i2a becomes a linked device of your phone.",
		[]string{config.PlatformMattermost, config.PlatformWhatsApp}, []string{"Mattermost", "WhatsApp"}, config.PlatformMattermost)
	isWA := func() bool { return kind.value() == config.PlatformWhatsApp }
	mmOnly := func(f *formField) *formField { f.hidden = isWA; return f }
	phone := textField("phone", "Phone number", "", "optional, for example +15551234567",
		"Empty: scan a QR code. A number in international format: type a pairing code on the phone instead.", validPhone)
	phone.hidden = func() bool { return !isWA() }
	warn := noteField("WhatsApp does not allow unofficial clients and can ban an account. i2a only reads messages, and never sends read receipts or presence.", warnStyle)
	warn.hidden = phone.hidden
	used := func(s string) bool { _, ok := u.cfg.Account(s); return ok }
	fm.fields = []*formField{
		noteField("An account only connects. After the connection, its conversations are in Unsorted; sort them in tab 4. GitHub and Linear come later.", faint),
		kind,
		textField("id", "Account ID", newID("work", used), "work", "A short ID that does not change, for example work or phone.", func(s string) error {
			if used(s) {
				return fmt.Errorf("the ID %q is used", s)
			}
			return validName(s)
		}),
		textField("name", "Name", "", "for example Company A", "The name that i2a shows. Empty: the ID.", nil),
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
		if !fm.validate() {
			return nil
		}
		if isWA() {
			return m.startPairing(fm.field("id").value(), fm.field("name").value(), fm.field("phone").value())
		}
		return m.connectAccount()
	}}}
	return fm
}

// connectAccount checks a new Mattermost account and saves it.
func (m *model) connectAccount() tea.Cmd {
	fm := m.form
	a := config.NewMattermostAccount(fm.field("id").value(), fm.field("url").value())
	a.Name = orStr(fm.field("name").value(), a.ID)
	tok := fm.field("token").value()
	fm.busy = "Connecting to " + a.URL + " ..."
	return m.background(func(ctx context.Context) func() tea.Cmd {
		user, err := m.u.d.Check(ctx, a, tok)
		return func() tea.Cmd {
			fm.busy = ""
			if err != nil {
				fm.err = "Connection failed. Nothing was saved.\n" + err.Error()
				return nil
			}
			a.User = user
			return m.addAccount(a, tok, "Connected as "+user+". Its conversations are in Unsorted: sort them in tab 4.")
		}
	})
}

// addAccount saves a new account and lists its conversations.
func (m *model) addAccount(a config.Account, token, status string) tea.Cmd {
	var tokens map[string]string
	if token != "" {
		tokens = map[string]string{a.ID: token}
	}
	if err := m.u.commit(func(c *config.Config) error {
		c.Accounts = append(c.Accounts, a)
		return nil
	}, tokens); err != nil {
		if m.form != nil {
			m.form.err = "Cannot save: " + err.Error()
		}
		m.setStatus("Cannot save: "+err.Error(), true)
		return nil
	}
	m.acc = len(m.u.cfg.Accounts) - 1
	m.saved(status)
	return m.loadConversations()
}

// ---- conversations of the accounts ----

// loadConversations lists the conversations of each account once, in the
// background: Mattermost over the network, WhatsApp from its local file.
func (m *model) loadConversations() tea.Cmd {
	var cmds []tea.Cmd
	for _, a := range m.u.cfg.Accounts {
		if _, done := m.chs[a.ID]; done || m.chsBusy[a.ID] {
			continue
		}
		a := a
		if a.Platform == config.PlatformWhatsApp {
			m.chsBusy[a.ID] = true
			dataDir := m.u.d.Paths.DataDir
			cmds = append(cmds, func() tea.Msg {
				chats, err := whatsapp.ReadChats(m.ctx, dataDir, a.ID)
				return channelsMsg{account: a.ID, chs: chatInfos(chats), err: err}
			})
			continue
		}
		token, err := m.u.d.Secrets.Get(secretsKey(a.ID))
		if err != nil {
			m.chsErr[a.ID] = "cannot read the token: " + err.Error()
			continue
		}
		m.chsBusy[a.ID] = true
		cmds = append(cmds, func() tea.Msg {
			user, err := m.u.d.Check(m.ctx, a, token)
			if err != nil {
				return channelsMsg{account: a.ID, err: err}
			}
			src := source.NewMattermost(a, token, m.u.cfg.Settings.Lookback.D(), &http.Client{Timeout: 30 * time.Second})
			chs, err := src.ListChannels(m.ctx)
			return channelsMsg{account: a.ID, user: user, chs: chs, err: err}
		})
	}
	return tea.Batch(cmds...)
}

// gotChannels keeps the conversations of an account, and the username of a
// Mattermost account. It saves a new username in the configuration when no
// form has changes.
func (m *model) gotChannels(msg channelsMsg) tea.Cmd {
	m.chsBusy[msg.account] = false
	if msg.err != nil {
		m.chsErr[msg.account] = msg.err.Error()
		return nil
	}
	if msg.chs == nil {
		msg.chs = []mattermostChannel{}
	}
	m.chs[msg.account] = msg.chs
	delete(m.chsErr, msg.account)
	if msg.user == "" {
		return nil
	}
	m.mmUsers[msg.account] = msg.user
	if a, ok := m.u.cfg.Account(msg.account); ok && a.User != msg.user && (m.form == nil || !m.form.dirty()) {
		if err := m.u.commit(func(c *config.Config) error {
			na, ok := c.Account(msg.account)
			if !ok {
				return config.MissingError{Kind: config.KindAccount, ID: msg.account}
			}
			na.User = msg.user
			return nil
		}, nil); err == nil {
			m.buildForm(true)
		}
	}
	return nil
}
