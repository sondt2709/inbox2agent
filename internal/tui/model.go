package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/sondt2709/inbox2agent/internal/config"
	"github.com/sondt2709/inbox2agent/internal/inbox"
	"github.com/sondt2709/inbox2agent/internal/service"
	"github.com/sondt2709/inbox2agent/internal/source/mattermost"
	"github.com/sondt2709/inbox2agent/internal/store"
)

type tabID int

const (
	tabFeed tabID = iota
	tabOverview
	tabSources
	tabProfiles
	tabSchedule
	tabActivity
)

var tabNames = []string{"Feed", "Overview", "Sources", "Profiles", "Schedule", "Activity"}

// refreshEvery is the time between two reads of the store and the service state.
const refreshEvery = 2 * time.Second

// Layout rows: the header bar is row 0, the tab bar is row 1, the content
// starts at row 2, and the footer is the last row.
const contentTop = 2

type (
	asyncMsg struct {
		id   int
		then func() tea.Cmd
	}
	refreshMsg struct{}
	dataMsg    struct {
		counts     map[string]int
		dispatches []store.Dispatch
		feed       feedMsg
		opened     Store // the store that load opened, or nil
		svc        service.State
		svcErr     error
		waStates   map[string]string // the state of each WhatsApp source
	}
	channelsMsg struct {
		source string
		chs    []mattermost.ChannelInfo
		err    error
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

	src, prof, act             int // cursors of the lists
	srcOff, profOff, actOffset int // first visible rows

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
	newSrc  *pendingSource // the source that the add form connected, before it is added
	pair    *pairing       // the WhatsApp pairing that runs, or nil

	waStates map[string]string // "paired" or "not paired" for each WhatsApp source, from the last load

	chs      map[string][]mattermost.ChannelInfo // the channels of each source
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
	}
}

func (m *model) Init() tea.Cmd { return tea.Batch(m.load(), m.spin.Tick) }

// load reads the store and the service state outside the UI loop.
func (m *model) load() tea.Cmd {
	all, st, open := m.feedAll, m.u.d.Store, m.u.d.OpenStore
	var wa []string
	for _, sc := range m.u.cfg.Sources {
		if sc.Type == config.SourceWhatsApp {
			wa = append(wa, sc.ID)
		}
	}
	return func() tea.Msg {
		var d dataMsg
		if st == nil && open != nil {
			// The dashboard does not create the store; it opens it when the daemon did.
			if s, err := open(); err == nil && s != nil {
				st, d.opened = s, s
			}
		}
		if st != nil {
			d.counts, _ = st.Counts(m.ctx)
			d.dispatches, _ = st.Dispatches(m.ctx, 50)
			d.feed = readFeed(m.ctx, st, all)
		}
		d.svc, d.svcErr = m.u.d.ServiceStatus()
		d.waStates = map[string]string{}
		for _, id := range wa {
			d.waStates[id] = m.whatsAppState(id)
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
}

func (m *model) formTarget() (string, func() *form) {
	switch m.tab {
	case tabSources:
		if sc := m.selectedSource(); sc != nil {
			id := sc.ID
			return "src:" + id, func() *form { return m.sourceForm(id) }
		}
		if m.newSrc != nil {
			return "src:+connected", m.newSourceStep2
		}
		if m.pair != nil {
			return "src:+pair", m.pairForm
		}
		return "src:+", m.newSourceForm
	case tabProfiles:
		if m.prof < len(m.u.cfg.Profiles) {
			name := m.u.cfg.Profiles[m.prof].Name
			return "prof:" + name, func() *form { return m.profileForm(name) }
		}
		return "prof:+", func() *form { return m.profileForm("") }
	case tabSchedule:
		return "sched", m.scheduleForm
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
	cmd := m.form.activate()
	if m.tab == tabSources {
		return tea.Batch(cmd, m.loadChannels())
	}
	return cmd
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
			m.newSrc = nil
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
		m.counts, m.dispatches, m.svc, m.svcErr, m.loaded = msg.counts, msg.dispatches, msg.svc, msg.svcErr, true
		m.waStates = msg.waStates
		m.setFeed(msg.feed)
		return tea.Tick(refreshEvery, func(time.Time) tea.Msg { return refreshMsg{} })
	case feedMsg:
		m.setFeed(msg)
		return nil
	case statusMsg:
		m.setStatus(msg.text, msg.isErr)
		return nil
	case channelsMsg:
		m.gotChannels(msg)
		return nil
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
		if m.pair != nil && m.formFor == "src:+pair" {
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
	case "1", "2", "3", "4", "5", "6":
		return m.setTab(tabID(k[0] - '1'))
	case "tab", "]":
		return m.setTab((m.tab + 1) % tabID(len(tabNames)))
	case "shift+tab", "[":
		return m.setTab((m.tab + tabID(len(tabNames)) - 1) % tabID(len(tabNames)))
	case "q", "ctrl+c":
		return m.quit()
	case "?":
		m.openDialog(&msgDialog{name: "Keys", text: helpText})
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
	case tabOverview:
		cmd = m.keyOverview(k)
	case tabSources:
		cmd = m.keySources(k)
	case tabProfiles:
		cmd = m.keyProfiles(k)
	case tabActivity:
		cmd = m.keyActivity(k)
	}
	m.buildForm(false)
	return cmd
}

func (m *model) setTab(t tabID) tea.Cmd {
	m.tab = t
	m.buildForm(true)
	if t == tabSources {
		return m.loadChannels()
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
		case tabSchedule:
			oy += 2 // the state line and an empty line
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
	case tabSources:
		return m.clickList(y, &m.src, m.srcOff, len(m.u.cfg.Sources)+1)
	case tabProfiles:
		return m.clickList(y, &m.prof, m.profOff, len(m.u.cfg.Profiles)+1)
	case tabActivity:
		// The table has a header row under the panel title.
		row := y - 3 + m.actOffset
		if row >= 0 && row < len(m.dispatches) {
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

const helpText = `Tabs        1-6 or tab / shift+tab, or click a tab
Move        ↑ ↓ or j k in a list, or the mouse wheel
Edit        enter or → opens the form at the right; or click it
Form        tab / shift+tab or ↑ ↓ move between the fields
            ←/→ or space change a choice or a switch
            enter goes to the next field; in the last field it saves
            ctrl+s saves the form · esc goes back to the list
Quit        q or ctrl+c
Refresh     ctrl+r

Feed        r reply (ctrl+s sends, esc cancels) · c copy the reply
            t ask the agent about the card (ctrl+s sends, esc cancels)
            o open the thread · d done · u unread
            s sort (urgency or newest) · f filter · a show done cards
Sources     enter edit · D direct on/off · m mentions on/off · x remove
Profiles    enter edit · x delete
Overview    i install or restart the service · u uninstall it
Activity    enter show the digest file

Channel list: type to search · space select · ctrl+a all shown · esc clear`

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
	contentH := m.h - 3
	content := lipgloss.NewStyle().Width(m.w).Height(contentH).MaxHeight(contentH).Render(m.renderTab(m.w, contentH))
	footer := m.renderFooter()
	base := lipgloss.JoinVertical(lipgloss.Left, header, tabs, content, footer)

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
	m.tabZones = m.tabZones[:0]
	x := 1
	var b strings.Builder
	b.WriteString(" ")
	for i, name := range tabNames {
		var s string
		if tabID(i) == m.tab {
			s = tabActive.Render(fmt.Sprintf("%d %s", i+1, name))
		} else {
			s = tabInactive.Render(tabKey.Render(fmt.Sprint(i+1)) + faint.Render(" "+name))
		}
		w := lipgloss.Width(s)
		m.tabZones = append(m.tabZones, zone{x0: x, x1: x + w, y: 1, tab: tabID(i)})
		b.WriteString(s + " ")
		x += w + 1
	}
	left := b.String()
	right := ""
	if m.form != nil && m.form.dirty() && m.replyCard == 0 && m.askCard == 0 {
		right = dirtyStyle.Render("● unsaved changes") + faint.Render(" · ctrl+s save · esc discard ")
	}
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
		left = renderHints(d.hints())
	case m.status != "" && m.now().Sub(m.statusAt) < 6*time.Second:
		st := okStyle
		if m.statusErr {
			st = errStyle
		}
		left = st.Render(truncate(m.status, m.w-lipgloss.Width(right)-4))
	case formActive && (m.replyCard != 0 || m.askCard != 0):
		left = renderHints([]hint{{"tab", "fields"}, {"←/→ space", "choose"}, {"ctrl+s", "send"}, {"esc", "cancel"}})
	case formActive:
		left = renderHints([]hint{{"tab ↑↓", "fields"}, {"←/→ space", "choose"}, {"ctrl+s", "save"}, {"esc", "back to the list"}})
	default:
		left = renderHints(m.tabHints())
	}
	gap := m.w - 1 - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		return " " + truncateStyled(left, m.w-1)
	}
	return " " + left + strings.Repeat(" ", gap) + right
}

func (m *model) renderTab(w, h int) string {
	switch m.tab {
	case tabFeed:
		return m.viewFeed(w, h)
	case tabSources:
		return m.viewSources(w, h)
	case tabProfiles:
		return m.viewProfiles(w, h)
	case tabSchedule:
		return m.viewSchedule(w, h)
	case tabActivity:
		return m.viewActivity(w, h)
	}
	return m.viewOverview(w, h)
}

func (m *model) tabHints() []hint {
	switch m.tab {
	case tabFeed:
		return feedHints()
	case tabSources:
		return m.hintsSources()
	case tabProfiles:
		return m.hintsProfiles()
	case tabSchedule:
		return []hint{{"enter", "edit the schedule"}}
	case tabActivity:
		return []hint{{"↑↓", "move"}, {"enter", "show digest"}}
	}
	return []hint{{"i", "install service"}, {"u", "uninstall"}, {"3", "sources"}}
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
