package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/sondt2709/inbox2agent/internal/config"
	"github.com/sondt2709/inbox2agent/internal/inbox"
	"github.com/sondt2709/inbox2agent/internal/service"
	"github.com/sondt2709/inbox2agent/internal/source/mattermost"
	"github.com/sondt2709/inbox2agent/internal/source/whatsapp"
	"github.com/sondt2709/inbox2agent/internal/store"
)

type tabID int

const (
	tabFeed tabID = iota
	tabActivity
	tabAccounts
	tabConversations
	tabSpaces
	tabAgents
	tabSchedules
	tabSettings
)

var tabNames = []string{"Feed", "Activity", "Accounts", "Conversations", "Spaces", "Agents", "Schedules", "Settings"}

// tabBefore is the mark before each tab in the tab bar. The setup tabs show
// the model: Accounts → Conversations → Spaces → Agents + Schedules.
var tabBefore = []string{"", "", "│", "→", "→", "→", "+", "│"}

// tabDefs defines the item of each tab in one sentence, under the tab bar.
var tabDefs = []string{
	"Feed: the cards that your agents send back. r replies, t asks the agent.",
	"Activity: each send of a space to its agent, and the result.",
	"Account: one login to a chat tool. It only connects; i2a finds its conversations.",
	"Conversation: one DM, group or channel. Each is in one space, with a read level.",
	"Space: your own group of conversations from any account. It sends to one agent on one schedule.",
	"Agent: how i2a sends a batch to an AI agent. Each space has its own session.",
	"Schedule: when a space sends. Outside it, i2a still reads, and the messages wait.",
	"Settings: how long i2a keeps messages, notifications, and the background service.",
}

// refreshEvery is the time between two reads of the store and the service state.
const refreshEvery = 2 * time.Second

// Layout rows: the header bar is row 0, the tab bar is row 1, the definition
// of the tab is row 2, the content starts at row 3, and the footer is the
// last row.
const contentTop = 3

type (
	asyncMsg struct {
		id   int
		then func() tea.Cmd
	}
	refreshMsg struct{}
	dataMsg    struct {
		file       fileState
		counts     map[string]int
		dispatches []store.Dispatch
		feed       feedMsg
		opened     Store // the store that load opened, or nil
		svc        service.State
		svcErr     error
		waStates   map[string]string          // the state of each WhatsApp account
		waDevices  map[string]whatsapp.Device // the phone and the name of each WhatsApp account
		pending    map[store.Conv]int         // the waiting messages of each conversation
	}
	channelsMsg struct {
		account string
		user    string // the Mattermost username; "" for WhatsApp
		chs     []mattermost.ChannelInfo
		err     error
	}
)

// zone is a screen area that a mouse click selects.
type zone struct {
	x0, x1, y int
	tab       tabID
}

// model is the dashboard.
type model struct {
	ctx  context.Context
	u    *ui
	tab  tabID
	w, h int

	dialogs []dialog
	spin    spinner.Model
	busyID  int
	cancel  context.CancelFunc

	status    string
	statusErr bool
	statusAt  time.Time

	counts     map[string]int
	dispatches []store.Dispatch
	svc        service.State
	svcErr     error
	loaded     bool

	acc, conv, spc, agt, sch, act                      int // cursors of the lists
	accOff, convOff, spcOff, agtOff, schOff, actOffset int // first visible rows

	convFilter  string          // "", "unsorted", "space:<id>", "account:<id>" or "kind:<kind>"
	convSel     map[string]bool // the selected conversations (convKey)
	search      textinput.Model // the search of the Conversations tab
	searching   bool            // the search has the keyboard
	spaceFilter string          // the space of the Feed and Activity filter (F); "" is all spaces

	cards      []inbox.Card            // the Feed: open cards, and done cards when feedAll is true
	refs       map[int64]inbox.Message // the source messages of the cards
	feedSel    int                     // index in visibleCards
	feedSelID  int64                   // the selected card, kept across reloads
	feedOff    int
	feedSort   string                  // sortUrgency or sortNewest
	feedFilter int                     // index in feedFilters
	feedAll    bool                    // show the done cards too
	replies    map[int64][]inbox.Reply // the sent replies of the cards, by card ID
	replyCard  int64                   // the card of the open reply form; 0 when no reply form is open
	asks       map[int64][]inbox.Ask   // the asks of the cards, by card ID
	askCard    int64                   // the card of the open ask form; 0 when no ask form is open
	md         markdown                // the rendered card bodies

	// form is the form of the detail panel; formFor names what it edits.
	form    *form
	formFor string
	pair    *pairing // the WhatsApp pairing that runs, or nil

	waStates  map[string]string          // "paired" or "not paired" for each WhatsApp account, from the last load
	waDevices map[string]whatsapp.Device // the phone and the name of each WhatsApp account
	mmUsers   map[string]string          // the Mattermost username of each account, from the last list
	pending   map[store.Conv]int         // the waiting messages of each conversation

	chs      map[string][]mattermost.ChannelInfo // the conversations that each account lists
	chsErr   map[string]string
	chsBusy  map[string]bool
	tabZones []zone
	savedAny bool
	now      func() time.Time
}

func newModel(ctx context.Context, u *ui) *model {
	return &model{
		ctx: ctx, u: u, now: time.Now, tab: u.d.Tab, feedSort: sortUrgency, refs: map[int64]inbox.Message{},
		spin: spinner.New(spinner.WithSpinner(spinner.MiniDot)),
		chs:  map[string][]mattermost.ChannelInfo{}, chsErr: map[string]string{}, chsBusy: map[string]bool{},
		convSel: map[string]bool{}, mmUsers: map[string]string{}, search: newSearch(),
	}
}

// Init reads the store and lists the conversations of the accounts in the
// background, so the unsorted count of the tab bar is right from the start.
func (m *model) Init() tea.Cmd { return tea.Batch(m.load(), m.spin.Tick, m.loadConversations()) }

// load reads the store and the service state outside the UI loop.
func (m *model) load() tea.Cmd {
	all, st, open := m.feedAll, m.u.d.Store, m.u.d.OpenStore
	var wa []string
	for _, a := range m.u.cfg.Accounts {
		if a.Platform == config.PlatformWhatsApp {
			wa = append(wa, a.ID)
		}
	}
	dataDir := m.u.d.Paths.DataDir
	cfgPath, cfgHash := m.u.d.Paths.ConfigFile, m.u.cfgHash
	return func() tea.Msg {
		var d dataMsg
		d.file = readConfigFile(cfgPath, cfgHash)
		if st == nil && open != nil {
			// The dashboard does not create the store; it opens it when the daemon did.
			if s, err := open(); err == nil && s != nil {
				st, d.opened = s, s
			}
		}
		if st != nil {
			d.counts, _ = st.Counts(m.ctx)
			d.dispatches, _ = st.Dispatches(m.ctx, 50)
			d.pending, _ = st.PendingCounts(m.ctx)
			d.feed = readFeed(m.ctx, st, all)
		}
		d.svc, d.svcErr = m.u.d.ServiceStatus()
		d.waStates, d.waDevices = map[string]string{}, map[string]whatsapp.Device{}
		for _, id := range wa {
			d.waStates[id] = m.whatsAppState(id)
			d.waDevices[id], _ = whatsapp.ReadDevice(m.ctx, dataDir, id)
		}
		return d
	}
}

func (m *model) setStatus(text string, isErr bool) {
	m.status, m.statusErr, m.statusAt = text, isErr, m.now()
}

// ---- dialogs ----

func (m *model) openDialog(d dialog) { m.dialogs = append(m.dialogs, d) }

func (m *model) closeDialog() {
	if len(m.dialogs) > 0 {
		m.dialogs = m.dialogs[:len(m.dialogs)-1]
	}
}

func (m *model) topDialog() dialog {
	if len(m.dialogs) == 0 {
		return nil
	}
	return m.dialogs[len(m.dialogs)-1]
}

// async runs work outside the UI loop and shows a busy dialog with the label.
// work returns the function that continues in the UI loop.
func (m *model) async(label string, work func(ctx context.Context) func() tea.Cmd) tea.Cmd {
	m.openDialog(&busyDialog{label: label})
	return m.background(work)
}

// background runs work outside the UI loop with no dialog. The caller shows the progress.
func (m *model) background(work func(ctx context.Context) func() tea.Cmd) tea.Cmd {
	ctx, cancel := context.WithCancel(m.ctx)
	m.busyID++
	id := m.busyID
	m.cancel = cancel
	return func() tea.Msg {
		then := work(ctx)
		cancel()
		return asyncMsg{id: id, then: then}
	}
}

func (m *model) cancelBusy() {
	if m.cancel != nil {
		m.cancel()
	}
	m.busyID++ // the result of the cancelled action is ignored
	m.closeDialog()
	m.setStatus("Cancelled.", true)
}

// ---- forms ----

// buildForm makes the form of the detail panel for the current tab and row.
// It keeps the open form when it edits the same item, unless force is true.
func (m *model) buildForm(force bool) {
	key, build := m.formTarget()
	if !force && key == m.formFor && m.form != nil {
		return
	}
	m.formFor = key
	if build == nil {
		m.form = nil
		return
	}
	m.form = build()
	m.form.setFocus(0)
	if kind, id := formItem(key); kind != "" {
		if base, ok := config.Item(m.u.cfg, kind, id); ok {
			m.form.kind, m.form.id, m.form.base = kind, id, base
		}
	}
}

func (m *model) formTarget() (string, func() *form) {
	switch m.tab {
	case tabAccounts:
		if a := m.selectedAccount(); a != nil {
			id := a.ID
			return "acc:" + id, func() *form { return m.accountForm(id) }
		}
		if m.pair != nil {
			return "acc:+pair", m.pairForm
		}
		return "acc:+", m.newAccountForm
	case tabSpaces:
		if id, ok := m.selectedSpace(); ok {
			if id == config.UnsortedID {
				return "spc:" + id, m.unsortedForm
			}
			return "spc:" + id, func() *form { return m.spaceForm(id) }
		}
		return "spc:+", func() *form { return m.spaceForm("") }
	case tabAgents:
		if m.agt < len(m.u.cfg.Agents) {
			id := m.u.cfg.Agents[m.agt].ID
			return "agt:" + id, func() *form { return m.agentForm(id) }
		}
		return "agt:+", func() *form { return m.agentForm("") }
	case tabSchedules:
		if m.sch < len(m.u.cfg.Schedules) {
			id := m.u.cfg.Schedules[m.sch].ID
			return "sch:" + id, func() *form { return m.scheduleForm(id) }
		}
		return "sch:+", func() *form { return m.scheduleForm("") }
	case tabSettings:
		return "settings", m.settingsForm
	case tabFeed:
		if c, ok := m.selectedCard(); ok && c.ID == m.replyCard {
			return fmt.Sprintf("reply:%d", c.ID), func() *form { return m.replyForm(c) }
		}
		if c, ok := m.selectedCard(); ok && c.ID == m.askCard {
			return fmt.Sprintf("ask:%d", c.ID), func() *form { return m.askForm(c) }
		}
		m.replyCard, m.askCard = 0, 0 // the selection moved away from the card of the form
	}
	return "", nil
}

func (m *model) focusForm() tea.Cmd {
	if m.form == nil {
		return nil
	}
	return m.form.activate()
}

// leaveForm gives the keyboard back to the list. With unsaved changes, it asks first.
func (m *model) leaveForm() {
	if m.form == nil {
		return
	}
	if !m.form.dirty() {
		m.form.deactivate()
		return
	}
	m.openDialog(&confirmDialog{name: "Unsaved changes", text: "Discard the changes in this form?",
		choices: []choice{{"d", "Discard", func() tea.Cmd {
			m.buildForm(true)
			m.setStatus("Changes discarded.", false)
			return nil
		}}}})
}

// saved shows the message and makes the form again from the saved configuration.
func (m *model) saved(text string) {
	m.savedAny = true
	m.setStatus(text, false)
	m.buildForm(true)
}

// ---- update ----

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	return m, m.update(msg)
}

func (m *model) update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
		if m.pair != nil {
			m.rebuildPairForm() // the QR code needs a tall window
		}
		return nil
	case refreshMsg:
		return m.load()
	case dataMsg:
		if msg.opened != nil && m.u.d.Store == nil {
			m.u.d.Store = msg.opened
		}
		reload := m.applyFile(msg.file)
		m.counts, m.dispatches, m.svc, m.svcErr, m.loaded = msg.counts, msg.dispatches, msg.svc, msg.svcErr, true
		m.waStates, m.waDevices, m.pending = msg.waStates, msg.waDevices, msg.pending
		m.setFeed(msg.feed)
		return tea.Batch(reload, tea.Tick(refreshEvery, func(time.Time) tea.Msg { return refreshMsg{} }))
	case feedMsg:
		m.setFeed(msg)
		return nil
	case statusMsg:
		m.setStatus(msg.text, msg.isErr)
		return nil
	case channelsMsg:
		return m.gotChannels(msg)
	case asyncMsg:
		if msg.id != m.busyID {
			return nil // cancelled
		}
		if _, ok := m.topDialog().(*busyDialog); ok {
			m.closeDialog()
		}
		m.cancel = nil
		return msg.then()
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spin, cmd = m.spin.Update(msg)
		return cmd
	case formEsc:
		if m.replyCard != 0 {
			m.cancelReply()
			return nil
		}
		if m.askCard != 0 {
			m.cancelAsk()
			return nil
		}
		if m.pair != nil && m.formFor == "acc:+pair" {
			m.cancelPairing()
			return nil
		}
		m.leaveForm()
		return nil
	case pairStepMsg:
		return m.pairStep(msg)
	case pairDoneMsg:
		return m.pairDone(msg)
	case replyDoneMsg:
		return m.replyDone(msg)
	case askDoneMsg:
		return m.askDone(msg)
	}

	if d := m.topDialog(); d != nil {
		return d.update(m, msg)
	}

	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		if m.form != nil && m.form.active {
			if msg.String() == "ctrl+c" {
				return m.quit()
			}
			return m.form.key(msg)
		}
		if m.searching {
			return m.searchKey(msg)
		}
		return m.key(msg.String())
	case tea.MouseClickMsg:
		return m.click(msg.Mouse())
	case tea.MouseWheelMsg:
		if m.form != nil && m.form.active {
			return nil
		}
		switch msg.Mouse().Button {
		case tea.MouseWheelUp:
			return m.tabKey("up")
		case tea.MouseWheelDown:
			return m.tabKey("down")
		}
	default:
		// Cursor blinks and other messages of the focused input.
		if m.form != nil && m.form.active {
			if f := m.form.current().field; f != nil {
				var cmd tea.Cmd
				switch f.kind {
				case kindText, kindSecret:
					f.input, cmd = f.input.Update(msg)
				case kindArea:
					f.area, cmd = f.area.Update(msg)
				}
				return cmd
			}
		}
	}
	return nil
}

// key handles a key when the list has the keyboard: first the global keys,
// then the keys of the tab.
func (m *model) key(k string) tea.Cmd {
	switch k {
	case "1", "2", "3", "4", "5", "6", "7", "8":
		return m.setTab(tabID(k[0] - '1'))
	case "tab", "]":
		return m.setTab((m.tab + 1) % tabID(len(tabNames)))
	case "shift+tab", "[":
		return m.setTab((m.tab + tabID(len(tabNames)) - 1) % tabID(len(tabNames)))
	case "q", "ctrl+c":
		return m.quit()
	case "?":
		m.openDialog(helpDialog())
		return nil
	case "ctrl+r":
		return m.load()
	case "enter", "right", "l", "e":
		if m.form != nil {
			return m.focusForm()
		}
	}
	return m.tabKey(k)
}

func (m *model) tabKey(k string) tea.Cmd {
	var cmd tea.Cmd
	switch m.tab {
	case tabFeed:
		cmd = m.keyFeed(k)
	case tabActivity:
		cmd = m.keyActivity(k)
	case tabAccounts:
		cmd = m.keyAccounts(k)
	case tabConversations:
		cmd = m.keyConversations(k)
	case tabSpaces:
		cmd = m.keySpaces(k)
	case tabAgents:
		cmd = m.keyAgents(k)
	case tabSchedules:
		cmd = m.keySchedules(k)
	case tabSettings:
		cmd = m.keySettings(k)
	}
	m.buildForm(false)
	return cmd
}

func (m *model) setTab(t tabID) tea.Cmd {
	m.tab = t
	m.searching = false
	m.buildForm(true)
	switch t {
	case tabAccounts, tabConversations, tabSpaces:
		return m.loadConversations()
	}
	return nil
}

func (m *model) click(ms tea.Mouse) tea.Cmd {
	if ms.Button != tea.MouseLeft {
		return nil
	}
	for _, z := range m.tabZones {
		if ms.Y == z.y && ms.X >= z.x0 && ms.X < z.x1 {
			return m.setTab(z.tab)
		}
	}
	if ms.Y < contentTop {
		return nil
	}
	x, y := ms.X, ms.Y-contentTop
	// A click in the detail panel goes to its form.
	if m.form != nil {
		ox := 2 // the panel border and the padding
		oy := 2 // the panel border and the panel title
		switch m.tab {
		case tabSettings:
		case tabFeed:
			ox += feedListWidth(m.w)
			oy++ // the filter bar
		default:
			ox += listWidth(m.w)
		}
		if x >= ox {
			cmd := m.focusForm()
			return tea.Batch(cmd, m.form.click(x-ox, y-oy))
		}
	}
	if m.form != nil && m.form.active {
		m.form.deactivate()
	}
	switch m.tab {
	case tabFeed:
		return m.clickFeed(x, y-1) // the filter bar is above the panels
	case tabAccounts:
		return m.clickList(y, &m.acc, m.accOff, len(m.u.cfg.Accounts)+1)
	case tabSpaces:
		return m.clickList(y, &m.spc, m.spcOff, len(m.u.cfg.Spaces)+2)
	case tabAgents:
		return m.clickList(y, &m.agt, m.agtOff, len(m.u.cfg.Agents)+1)
	case tabSchedules:
		return m.clickList(y, &m.sch, m.schOff, len(m.u.cfg.Schedules)+1)
	case tabConversations:
		return m.clickConversations(y)
	case tabActivity:
		// The table has a header row under the panel title.
		row := y - 3 + m.actOffset
		if row >= 0 && row < len(m.visibleDispatches()) {
			if row == m.act {
				return m.keyActivity("enter")
			}
			m.act = row
		}
	}
	return nil
}

// clickList selects the clicked row of a list panel at the left. A click on
// the selected row opens its form.
func (m *model) clickList(y int, cursor *int, offset, n int) tea.Cmd {
	row := y - 2 + offset // the panel border and the panel title
	if row < 0 || row >= n {
		return nil
	}
	if row == *cursor {
		return m.focusForm()
	}
	*cursor = row
	m.buildForm(false)
	return nil
}

func (m *model) quit() tea.Cmd {
	if m.form == nil || !m.form.dirty() {
		return tea.Quit
	}
	m.openDialog(&confirmDialog{
		name: "Unsaved changes",
		text: "The form has changes that are not saved. Quit and discard them?",
		choices: []choice{
			{"d", "Discard and quit", func() tea.Cmd { return tea.Quit }},
		},
	})
	return nil
}

const helpText = `The model   Account → Conversation → Space → Agent + Schedule
            An account only connects. Each DM, group or channel is
            in one space, with a read level: all, mentions (messages
            for you) or off. A space sends its conversations to one
            agent on one schedule. Unsorted holds the others.

Tabs        1-8 or tab / shift+tab, or click a tab
Move        ↑ ↓ or j k in a list, or the mouse wheel
Edit        enter or → opens the form at the right; or click it
Form        tab / shift+tab or ↑ ↓ move between the fields
            ←/→ or space change a choice or a switch
            ctrl+s saves the form · esc goes back to the list
Quit        q or ctrl+c · Refresh ctrl+r

Feed           r reply · t ask the agent · c copy the reply
               o open the thread · d done · u unread · s sort
               f category · F space · a done cards
Activity       enter show the digest file · F space (as the Feed)
Accounts       enter edit · x remove
Conversations  space select · a select all shown · m move to a space
               l read level · n rename · / search · f filter
Spaces         enter edit · a add conversations · x delete
               c show its conversations
Agents         enter edit · x delete (choose a replacement)
Schedules      enter edit · x delete (choose a replacement)
Settings       enter edit · i install the service · u uninstall it`

// helpDialog shows the key help as wide as its longest line.
func helpDialog() *viewerDialog {
	d := newViewer("Keys", helpText)
	for l := range strings.SplitSeq(helpText, "\n") {
		d.width = max(d.width, lipgloss.Width(l)+4)
	}
	d.lines = strings.Count(helpText, "\n") + 1
	return d
}

// ---- view ----

func (m *model) View() tea.View {
	v := tea.NewView(m.render())
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion
	return v
}

func (m *model) render() string {
	if m.w == 0 || m.h == 0 {
		return ""
	}
	if m.formFor == "" {
		m.buildForm(false)
	}
	header := m.renderHeader()
	tabs := m.renderTabs()
	def := m.renderDef()
	contentH := m.h - 4
	content := lipgloss.NewStyle().Width(m.w).Height(contentH).MaxHeight(contentH).Render(m.renderTab(m.w, contentH))
	footer := m.renderFooter()
	base := lipgloss.JoinVertical(lipgloss.Left, header, tabs, def, content, footer)

	d := m.topDialog()
	if d == nil {
		return base
	}
	dw, dh := d.size(m.w, m.h)
	dw, dh = max(20, dw), max(5, dh)
	innerW, innerH := dw-4, dh-3
	var body string
	if bd, ok := d.(*busyDialog); ok {
		body = "\n" + m.spin.View() + " " + textStyle.Render(bd.label) + "\n\n" + faint.Render("Press esc to cancel.")
	} else {
		body = d.view(innerW, innerH)
	}
	box := dialogStyle.Width(dw).Height(dh).MaxHeight(dh).Render(
		dialogTitle.Render(d.title()) + "\n" + clipLines(body, innerW, innerH))
	x, y := (m.w-dw)/2, max(contentTop, (m.h-dh)/2)
	return lipgloss.NewCompositor(
		lipgloss.NewLayer(base),
		lipgloss.NewLayer(box).X(x).Y(y).Z(1),
	).Render()
}

func (m *model) renderHeader() string {
	left := brandStyle.Render("◆ i2a") + faint.Render("  inbox2agent")
	right := m.serviceBadge() + " "
	gap := max(1, m.w-lipgloss.Width(left)-lipgloss.Width(right))
	return headerStyle.Width(m.w).Render(left + strings.Repeat(" ", gap) + right)
}

func (m *model) serviceBadge() string {
	switch {
	case m.svcErr != nil:
		return faint.Render("○ service not supported here")
	case m.svc.State == "running":
		return okStyle.Render("●") + textStyle.Render(" service running · pid "+m.svc.PID)
	case m.svc.Loaded:
		return warnStyle.Render("●") + textStyle.Render(" service "+orStr(m.svc.State, "loaded"))
	case !m.loaded:
		return faint.Render("○ service …")
	}
	return faint.Render("○ service not installed")
}

func (m *model) renderTabs() string {
	if bar := m.tabBar(false); lipgloss.Width(bar) <= m.w {
		return bar
	}
	return m.tabBar(true) // a narrow window: less padding, the unsorted count only
}

// tabBar draws the tab bar and records the mouse zone of each tab.
func (m *model) tabBar(compact bool) string {
	m.tabZones = m.tabZones[:0]
	x := 0
	var b strings.Builder
	active, inactive := tabActive, tabInactive
	if compact {
		active, inactive = tabActive.Padding(0, 0), tabInactive.Padding(0, 0)
	}
	for i, name := range tabNames {
		if mark := tabBefore[i]; mark != "" {
			if compact {
				mark = " " + mark + " "
			}
			b.WriteString(faint.Render(mark))
			x += lipgloss.Width(mark)
		} else if compact && i > 0 {
			b.WriteString("  ")
			x += 2
		}
		badge := ""
		if tabID(i) == tabConversations {
			if n := m.unsortedCount(); n > 0 {
				badge = warnStyle.Render(fmt.Sprintf("%d unsorted", n)) + " "
				if compact {
					badge = " " + warnStyle.Render(fmt.Sprint(n))
				}
			}
		}
		var s string
		if tabID(i) == m.tab {
			s = active.Render(fmt.Sprintf("%d %s", i+1, name)) + badge
		} else {
			s = inactive.Render(tabKey.Render(fmt.Sprint(i+1))+faint.Render(" "+name)) + badge
		}
		w := lipgloss.Width(s)
		m.tabZones = append(m.tabZones, zone{x0: x, x1: x + w, y: 1, tab: tabID(i)})
		b.WriteString(s)
		x += w
	}
	return b.String()
}

// renderDef is the definition of the current tab, with the unsaved mark of a
// form at the right.
func (m *model) renderDef() string {
	right := ""
	if m.form != nil && m.form.dirty() && m.replyCard == 0 && m.askCard == 0 {
		right = dirtyStyle.Render("● unsaved changes") + faint.Render(" · ctrl+s save · esc discard ")
	}
	left := " " + faint.Render(truncate(tabDefs[m.tab], max(10, m.w-2-lipgloss.Width(right))))
	gap := max(1, m.w-lipgloss.Width(left)-lipgloss.Width(right))
	return left + strings.Repeat(" ", gap) + right
}

func (m *model) renderFooter() string {
	formActive := m.form != nil && m.form.active
	right := renderHints([]hint{{"tab", "next tab"}, {"?", "keys"}, {"q", "quit"}})
	if m.topDialog() != nil || formActive {
		right = ""
	}
	var left string
	switch d := m.topDialog(); {
	case d != nil:
		left = fitHints(d.hints(), m.w-2)
	case m.status != "" && m.now().Sub(m.statusAt) < 6*time.Second:
		st := okStyle
		if m.statusErr {
			st = errStyle
		}
		left = st.Render(truncate(m.status, m.w-lipgloss.Width(right)-4))
	case formActive && (m.replyCard != 0 || m.askCard != 0):
		left = fitHints([]hint{{"tab", "fields"}, {"←/→ space", "choose"}, {"ctrl+s", "send"}, {"esc", "cancel"}}, m.w-2)
	case formActive:
		left = fitHints([]hint{{"tab ↑↓", "fields"}, {"←/→ space", "choose"}, {"ctrl+s", "save"}, {"esc", "back to the list"}}, m.w-2)
	default:
		left = fitHints(m.tabHints(), m.w-4-lipgloss.Width(right))
	}
	gap := m.w - 1 - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		return " " + truncateStyled(left, m.w-1)
	}
	return " " + left + strings.Repeat(" ", gap) + right
}

func (m *model) renderTab(w, h int) string {
	switch m.tab {
	case tabActivity:
		return m.viewActivity(w, h)
	case tabAccounts:
		return m.viewAccounts(w, h)
	case tabConversations:
		return m.viewConversations(w, h)
	case tabSpaces:
		return m.viewSpaces(w, h)
	case tabAgents:
		return m.viewAgents(w, h)
	case tabSchedules:
		return m.viewSchedules(w, h)
	case tabSettings:
		return m.viewSettings(w, h)
	}
	return m.viewFeed(w, h)
}

func (m *model) tabHints() []hint {
	switch m.tab {
	case tabActivity:
		return []hint{{"↑↓", "move"}, {"enter", "show digest"}, {"F", "space"}}
	case tabAccounts:
		return m.hintsAccounts()
	case tabConversations:
		return m.hintsConversations()
	case tabSpaces:
		return m.hintsSpaces()
	case tabAgents:
		return m.hintsAgents()
	case tabSchedules:
		return m.hintsSchedules()
	case tabSettings:
		return []hint{{"enter", "edit"}, {"i", "install service"}, {"u", "uninstall"}}
	}
	return feedHints()
}

// listWidth is the width of the list panel at the left of a master-detail tab.
func listWidth(w int) int { return max(24, min(32, w/4)) }

// masterDetail draws a list panel at the left and a detail panel at the right.
// When the form of the detail panel has the keyboard, the detail panel has the
// accent border and the list is grey.
func (m *model) masterDetail(listTitle string, items []string, cursor int, offset *int, detailTitle string, w, h int) string {
	lw := listWidth(w)
	rows := h - 3
	*offset = scrollTo(cursor, *offset, rows)
	active := m.form != nil && m.form.active
	list := listRows(items, cursor, *offset, lw-4, rows)
	if active {
		list = faint.Render(ansi.Strip(list))
	}
	left := panel(listTitle, list, lw, h)
	dw, dh := detailSize(w, h, false)
	body := ""
	if m.form != nil {
		body = m.form.render(dw, dh-1)
	}
	right := detailPanel(detailTitle, body, w-lw, h, active)
	return lipgloss.JoinHorizontal(lipgloss.Top, left, right)
}

// detailPanel is a panel whose border has the accent color when its form has the keyboard.
func detailPanel(title, body string, w, h int, active bool) string {
	if !active {
		return panel(title, body, w, h)
	}
	inner := max(1, w-4)
	content := panelTitle.Render(truncate(title, inner)) + "\n" + body
	return panelStyle.BorderForeground(accent).Width(w).Height(h).MaxHeight(h).Render(clipLines(content, inner, h-2))
}

// detailSize returns the inner size of the detail panel.
func detailSize(w, h int, full bool) (int, int) {
	if full {
		return w - 4, h - 3
	}
	return w - listWidth(w) - 4, h - 3
}

func orStr(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
