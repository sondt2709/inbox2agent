package tui

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/sondt2709/inbox2agent/internal/ask"
	"github.com/sondt2709/inbox2agent/internal/card"
	"github.com/sondt2709/inbox2agent/internal/config"
	"github.com/sondt2709/inbox2agent/internal/inbox"
	"github.com/sondt2709/inbox2agent/internal/render"
	"github.com/sondt2709/inbox2agent/internal/reply"
	"github.com/sondt2709/inbox2agent/internal/source"
	"github.com/sondt2709/inbox2agent/internal/source/whatsapp"
)

// The Feed tab shows the cards of the agent: a list at the left and the
// selected card at the right. Card text comes from an agent that read
// untrusted messages; "i2a card add" sanitized it, and the Feed opens only the
// URLs of the stored source messages, never a URL from the card text.

const (
	sortUrgency = "urgency"
	sortNewest  = "newest"
)

// statusMsg shows a text in the footer, from work outside the UI loop.
type statusMsg struct {
	text  string
	isErr bool
}

// feedMsg carries the cards, their source messages, their sent replies and
// their asks, read outside the UI loop.
type feedMsg struct {
	cards   []inbox.Card
	refs    map[int64]inbox.Message
	replies map[int64][]inbox.Reply
	asks    map[int64][]inbox.Ask
}

// replyDoneMsg is the result of a send from the reply form.
type replyDoneMsg struct {
	cardID int64
	reply  inbox.Reply
	target inbox.Message
	err    error
}

// errNoCopyCommand means that no clipboard command exists; the TUI uses OSC 52.
var errNoCopyCommand = errors.New("no clipboard command")

// copyText copies text with pbcopy when it is in PATH.
func copyText(text string) error {
	bin, err := exec.LookPath("pbcopy")
	if err != nil {
		return errNoCopyCommand
	}
	cmd := exec.Command(bin)
	cmd.Stdin = strings.NewReader(text)
	return cmd.Run()
}

// openURL opens a URL in the browser: open on macOS, xdg-open on other systems.
func openURL(url string) error {
	name := "xdg-open"
	if runtime.GOOS == "darwin" {
		name = "open"
	}
	out, err := exec.Command(name, url).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %v %s", name, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// readFeed reads the cards and the messages that they refer to.
func readFeed(ctx context.Context, st Store, all bool) feedMsg {
	var f feedMsg
	if st == nil {
		return f
	}
	f.cards, _ = st.Cards(ctx, all)
	var ids []int64
	for _, c := range f.cards {
		ids = append(ids, c.Refs...)
	}
	f.refs = map[int64]inbox.Message{}
	if msgs, err := st.MessagesByID(ctx, ids); err == nil {
		for _, m := range msgs {
			f.refs[m.ID] = m
		}
	}
	cardIDs := make([]int64, len(f.cards))
	for i, c := range f.cards {
		cardIDs[i] = c.ID
	}
	f.replies, _ = st.SentReplies(ctx, cardIDs)
	f.asks, _ = st.Asks(ctx, cardIDs)
	return f
}

// setFeed takes new cards and keeps the selection on the same card. It
// returns the card notes for the terminal (see Deps.FeedNotes), or nil.
func (m *model) setFeed(f feedMsg) tea.Cmd {
	notes := m.feedNotes(f)
	m.cards, m.refs, m.replies, m.asks = f.cards, f.refs, f.replies, f.asks
	if m.refs == nil {
		m.refs = map[int64]inbox.Message{}
	}
	v := m.visibleCards()
	if i := slices.IndexFunc(v, func(c inbox.Card) bool { return c.ID == m.feedSelID }); i >= 0 {
		m.feedSel = i
	}
	m.feedSel = min(m.feedSel, max(0, len(v)-1))
	if len(v) > 0 {
		m.feedSelID = v[m.feedSel].ID
	}
	return notes
}

// feedNotes compares the cards of a load with the last load and returns the
// card notes as escape sequences for the terminal. The rule is card.Plan, as
// for "i2a card add"; the first load gives none.
func (m *model) feedNotes(f feedMsg) tea.Cmd {
	saved, since := card.Diff(m.feedLast, m.feedSince, f.cards)
	m.feedSince = since
	m.feedLast = make(map[int64]inbox.Card, len(f.cards))
	for _, c := range f.cards {
		m.feedLast[c.ID] = c
	}
	n := m.u.cfg.Notify
	if m.u.d.FeedNotes == nil || !n.Enabled || len(saved) == 0 {
		return nil
	}
	names := m.spaceNames()
	cx := card.Context{Refs: f.refs, Space: func(c inbox.Card) string {
		return names[card.SpaceOf(c, f.refs, func(id string) bool { _, ok := m.u.cfg.Space(id); return ok }, m.u.cfg.Route)]
	}}
	var cmds []tea.Cmd
	for _, note := range card.Plan(saved, n.MinUrgency, cx) {
		cmds = append(cmds, tea.Raw(m.u.d.FeedNotes(note)))
	}
	return tea.Sequence(cmds...)
}

// cardShown reports whether the done filter shows the card.
func (m *model) cardShown(c inbox.Card) bool {
	if c.State == inbox.CardDone && !m.feedAll {
		return false
	}
	return true
}

// visibleCards returns the cards of the filter in the sort order.
func (m *model) visibleCards() []inbox.Card {
	var v []inbox.Card
	for _, c := range m.cards {
		if !m.cardShown(c) {
			continue
		}
		if sp := m.activeSpace(); sp != "" && m.cardSpace(c) != sp {
			continue
		}
		v = append(v, c)
	}
	slices.SortStableFunc(v, func(a, b inbox.Card) int {
		if m.feedSort == sortUrgency {
			if d := inbox.UrgencyRank(b.Urgency) - inbox.UrgencyRank(a.Urgency); d != 0 {
				return d
			}
		}
		if d := b.UpdatedAt.Compare(a.UpdatedAt); d != 0 {
			return d
		}
		return int(b.ID - a.ID)
	})
	return v
}

func (m *model) selectedCard() (inbox.Card, bool) {
	v := m.visibleCards()
	if m.feedSel < len(v) {
		return v[m.feedSel], true
	}
	return inbox.Card{}, false
}

// setCardState changes the state in the view at once and in the store outside the UI loop.
func (m *model) setCardState(c inbox.Card, state string) tea.Cmd {
	for i := range m.cards {
		if m.cards[i].ID == c.ID {
			m.cards[i].State = state
		}
	}
	st, all := m.u.d.Store, m.feedAll
	if st == nil {
		return nil
	}
	return func() tea.Msg {
		_ = st.SetCardState(m.ctx, c.ID, "", state, m.now())
		return readFeed(m.ctx, st, all)
	}
}

// markRead makes the selected card read when it is new.
func (m *model) markRead() tea.Cmd {
	c, ok := m.selectedCard()
	if !ok || c.State != inbox.CardNew {
		return nil
	}
	return m.setCardState(c, inbox.CardRead)
}

// moveFeed moves the selection, and the card that the user moves to becomes read.
func (m *model) moveFeed(to int) tea.Cmd {
	v := m.visibleCards()
	if len(v) == 0 {
		return nil
	}
	to = max(0, min(len(v)-1, to))
	if to == m.feedSel {
		return nil
	}
	m.feedSel, m.feedSelID = to, v[to].ID
	return m.markRead()
}

// threadURL returns the URL of the first source message that still exists and
// has an http or https URL.
func (m *model) threadURL(c inbox.Card) string {
	for _, id := range c.Refs {
		if msg, ok := m.refs[id]; ok && (strings.HasPrefix(msg.URL, "https://") || strings.HasPrefix(msg.URL, "http://")) {
			return msg.URL
		}
	}
	return ""
}

func (m *model) keyFeed(k string) tea.Cmd {
	if feedRailWidth(m.w) == 0 && m.feedFocus == focusSpaces {
		m.feedFocus = focusCards // the space list is hidden
	}
	switch k {
	case "left", "h":
		if m.feedFocus > focusCards || (m.feedFocus == focusCards && feedRailWidth(m.w) > 0) {
			m.feedFocus--
		}
		return nil
	case "right", "l":
		m.feedFocus = min(focusCard, m.feedFocus+1)
		if m.feedFocus == focusCard && m.form != nil {
			return m.focusForm() // the open reply or ask form takes the keyboard
		}
		return nil
	case "up", "k", "down", "j":
		step := 1
		if k == "up" || k == "k" {
			step = -1
		}
		switch m.feedFocus {
		case focusSpaces:
			sp := m.feedSpaces()
			i := max(0, min(len(sp)-1, slices.Index(sp, m.activeSpace())+step))
			m.setFeedSpace(sp[i])
			return nil
		case focusCard:
			m.cardOff = max(0, m.cardOff+step)
			return nil
		}
		return m.moveFeed(m.feedSel + step)
	case "pgup":
		m.cardOff = max(0, m.cardOff-max(1, m.h-8))
		return nil
	case "pgdown", "pgdn":
		m.cardOff += max(1, m.h-8)
		return nil
	case "s":
		m.feedSort = map[string]string{sortUrgency: sortNewest, sortNewest: sortUrgency}[m.feedSort]
		m.feedSel = 0
		m.setStatus("Sort: "+m.feedSort+".", false)
	case "F":
		m.pickSpaceFilter("card", func(space string) int {
			n := 0
			for _, c := range m.cards {
				if m.cardShown(c) && (space == "" || m.cardSpace(c) == space) {
					n++
				}
			}
			return n
		})
	case "a":
		m.feedAll = !m.feedAll
		m.setStatus(map[bool]string{true: "Done cards are shown.", false: "Done cards are hidden."}[m.feedAll], false)
		st, all := m.u.d.Store, m.feedAll
		return func() tea.Msg { return readFeed(m.ctx, st, all) }
	}
	c, ok := m.selectedCard()
	if !ok {
		return nil
	}
	switch k {
	case "enter":
		return m.markRead()
	case "c":
		if c.Reply == "" {
			m.setStatus("This card has no suggested reply.", true)
			return nil
		}
		read := m.markRead()
		switch err := m.u.d.Copy(c.Reply); {
		case errors.Is(err, errNoCopyCommand):
			m.setStatus("Copied the suggested reply (terminal clipboard).", false)
			return tea.Batch(read, tea.SetClipboard(c.Reply))
		case err != nil:
			m.setStatus("Cannot copy: "+err.Error(), true)
		default:
			m.setStatus("Copied the suggested reply to the clipboard.", false)
		}
		return read
	case "o":
		url := m.threadURL(c)
		if url == "" {
			m.setStatus("This card has no message link.", true)
			return nil
		}
		read := m.markRead()
		open := m.u.d.Open
		m.setStatus("Opening "+url, false)
		return tea.Batch(read, func() tea.Msg {
			if err := open(url); err != nil {
				return statusMsg{text: "Cannot open the link: " + err.Error(), isErr: true}
			}
			return nil
		})
	case "r":
		return m.openReply(c)
	case "t":
		return m.openAsk(c)
	case "d":
		m.setStatus("Done: "+truncate(render.PlainText(c.Title), 60), false)
		return m.setCardState(c, inbox.CardDone)
	case "u":
		m.setStatus("Marked unread.", false)
		return m.setCardState(c, inbox.CardNew)
	}
	return nil
}

// clickFeed selects the clicked space or card. Each card has two rows.
func (m *model) clickFeed(x, y int) tea.Cmd {
	rw := feedRailWidth(m.w)
	if x < rw {
		m.feedFocus = focusSpaces
		if i := y - 2 + m.railOff; y >= 2 && i < len(m.feedSpaces()) {
			m.setFeedSpace(m.feedSpaces()[i])
		}
		return nil
	}
	x -= rw
	if x >= feedListWidth(m.w-rw) {
		m.feedFocus = focusCard
		return nil
	}
	m.feedFocus = focusCards
	row := (y-2)/2 + m.feedOff // the panel border and the panel title
	if y < 2 || row >= len(m.visibleCards()) {
		return nil
	}
	if row == m.feedSel {
		return m.markRead()
	}
	return m.moveFeed(row)
}

func feedListWidth(w int) int { return max(40, w*2/5) }

// feedRailWidth is the width of the space list of the Feed; 0 below 100 columns.
func feedRailWidth(w int) int {
	if w < 100 {
		return 0
	}
	return 24
}

// feedSpaces are the rows of the space list: all, each space, Everything else.
func (m *model) feedSpaces() []string {
	out := []string{""}
	for _, s := range m.u.cfg.Spaces {
		out = append(out, s.ID)
	}
	return append(out, config.UnsortedID)
}

// feedRail draws the space list of the Feed with the card count of each row.
func (m *model) feedRail(w, h int) string {
	var items []string
	cur := 0
	for i, id := range m.feedSpaces() {
		if id == m.activeSpace() {
			cur = i
		}
		items = append(items, listLabel(orStr(m.spaceNames()[id], "All"), fmt.Sprint(m.cardsOf(id)), w-6))
	}
	m.railOff = scrollTo(cur, m.railOff, h-3)
	off := m.railOff
	return detailPanel("Spaces", railRows(items, cur, off, w-4, h-3, true), w, h, m.feedFocus == focusSpaces)
}

var (
	unreadMark = lipgloss.NewStyle().Foreground(lipgloss.Color("#F778BA")).Bold(true)
	catStyles  = map[string]lipgloss.Style{
		inbox.CategoryAction:   lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#111111")).Background(red),
		inbox.CategoryQuestion: lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#111111")).Background(yellow),
		inbox.CategoryInfo:     lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#111111")).Background(blue),
	}
	urgBars = map[string]string{
		inbox.UrgencyHigh:   "▰▰▰",
		inbox.UrgencyNormal: "▰▰▱",
		inbox.UrgencyLow:    "▰▱▱",
	}
	urgStyles = map[string]lipgloss.Style{
		inbox.UrgencyHigh:   errStyle.Bold(true),
		inbox.UrgencyNormal: textStyle,
		inbox.UrgencyLow:    faint,
	}
)

func catBadge(c string) string {
	return catStyles[c].Render(fmt.Sprintf(" %-8s ", strings.ToUpper(c)))
}

// urgBar returns the bars of an urgency: three columns wide.
func urgBar(u string) string { return orStr(urgBars[u], "▱▱▱") }

func urgLabel(u string) string { return urgStyles[u].Render(urgBar(u)) }

// when shows the time of today, or the date of an earlier day.
func when(t, now time.Time) string {
	t = t.Local()
	if y, m, d := t.Date(); y == now.Year() && m == now.Month() && d == now.Day() {
		return t.Format("15:04")
	}
	return t.Format("02 Jan")
}

// feedCounts returns the number of new cards.
func (m *model) feedCounts() (fresh int) {
	for _, c := range m.cards {
		if c.State == inbox.CardNew {
			fresh++
		}
	}
	return fresh
}

// setFeedSpace shows the cards of a space ("" for all) and selects its first
// card. With an open reply or ask form it keeps the space, so a typed text
// is not lost.
func (m *model) setFeedSpace(id string) bool {
	if (m.replyCard != 0 || m.askCard != 0) && id != m.activeSpace() {
		m.setStatus("Send or cancel the open form first (esc).", true)
		return false
	}
	m.spaceFilter, m.feedSel, m.act = id, 0, 0
	if v := m.visibleCards(); len(v) > 0 {
		m.feedSelID = v[0].ID
	}
	return true
}

// The panels of the Feed that can have the keyboard.
const (
	focusSpaces = iota
	focusCards
	focusCard
)

func (m *model) viewFeed(w, h int) string {
	rw := feedRailWidth(w)
	if rw == 0 && m.feedFocus == focusSpaces {
		m.feedFocus = focusCards // the space list is hidden
	}
	w -= rw
	lw := feedListWidth(w)
	v := m.visibleCards()
	visible := max(1, (h-3)/2)
	m.feedOff = scrollTo(m.feedSel, m.feedOff, visible)
	var list []string
	inner := lw - 4
	for i := m.feedOff; i < min(len(v), m.feedOff+visible); i++ {
		list = append(list, m.cardRow(v[i], i == m.feedSel, inner)...)
	}
	body := strings.Join(list, "\n")
	switch {
	case m.u.d.Store == nil, len(m.cards) == 0:
		body = faint.Render("No cards yet. The agent adds cards with\n\"i2a card add\" after each batch.")
	case len(v) == 0:
		body = faint.Render("No card in this space.")
	}
	listTitle := fmt.Sprintf("Cards · %d · %s", len(v), m.feedSort)
	if m.feedAll {
		listTitle += " · done shown"
	}
	left := detailPanel(listTitle, body, lw, h, m.feedFocus == focusCards)

	dw := w - lw
	c, ok := m.selectedCard()
	var right string
	switch {
	case ok && m.form != nil && m.replyCard == c.ID:
		right = detailPanel(fmt.Sprintf("Reply · card %d", c.ID), m.form.render(dw-4, h-4), dw, h, m.form.active || m.feedFocus == focusCard)
	case ok && m.form != nil && m.askCard == c.ID:
		right = detailPanel(fmt.Sprintf("Ask the agent · card %d", c.ID), m.form.render(dw-4, h-4), dw, h, m.form.active || m.feedFocus == focusCard)
	default:
		title, text := "Card", ""
		if ok {
			title, text = m.cardText(c, dw-4, h-3)
		}
		right = detailPanel(title, text, dw, h, m.feedFocus == focusCard)
	}
	if rw > 0 {
		return lipgloss.JoinHorizontal(lipgloss.Top, m.feedRail(rw, h), left, right)
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, left, right)
}

// cardRow renders the two lines of a card in the list. The rows start at the
// left edge: line 1 is the category, the urgency bars, a mark for a new card
// and the title; line 2 is the space, the group in bold and the time.
func (m *model) cardRow(c inbox.Card, selected bool, w int) []string {
	cat := fmt.Sprintf("%-8s ", strings.ToUpper(c.Category))
	mark := ""
	if c.State == inbox.CardNew {
		mark = "● "
	}
	title := truncate(render.PlainText(c.Title), max(5, w-lipgloss.Width(cat)-4-lipgloss.Width(mark)))
	var marks []string
	if len(m.replies[c.ID]) > 0 {
		marks = append(marks, "✓ replied")
	}
	if m.waiting(c) {
		marks = append(marks, "↪ asked")
	}
	if n := stateNote(c); n != "" {
		marks = append(marks, n)
	}
	space := m.spaceNames()[m.cardSpace(c)]
	tail := strings.Join(nonEmpty(strings.Join(marks, " · "), when(c.UpdatedAt, m.now())), " · ")
	if selected {
		meta := strings.Join(nonEmpty(space, c.Group, tail), " · ")
		return []string{
			selectedRow.Width(w).Render(truncate(cat+urgBar(c.Urgency)+" "+mark+title, w)),
			selectedRow.Width(w).Render(truncate(meta, w)),
		}
	}
	t := textStyle
	if c.State != inbox.CardNew {
		t = faint
	}
	line1 := textStyle.Render(cat) + urgLabel(c.Urgency) + " " + unreadMark.Render(mark) + t.Render(title)
	var meta []string
	if space != "" {
		meta = append(meta, faint.Render(space))
	}
	if c.Group != "" {
		meta = append(meta, boldStyle.Render(c.Group))
	}
	if tail != "" {
		meta = append(meta, faint.Render(tail))
	}
	return []string{line1, truncateStyled(strings.Join(meta, faint.Render(" · ")), w)}
}

// cardText returns the title and the visible lines of the card panel,
// scrolled by m.cardOff. Another card starts at its first line.
func (m *model) cardText(c inbox.Card, w, h int) (string, string) {
	h = max(1, h) // a very short window
	if c.ID != m.cardFor {
		m.cardFor, m.cardOff = c.ID, 0
	}
	lines := strings.Split(m.cardDetail(c, w), "\n")
	m.cardOff = max(0, min(m.cardOff, len(lines)-h))
	end := max(m.cardOff, min(len(lines), m.cardOff+h))
	title := "Card " + fmt.Sprint(c.ID)
	if m.cardOff > 0 {
		title += " · ↑"
	}
	if end < len(lines) {
		title += " · ↓ more"
	}
	return title, strings.Join(lines[m.cardOff:end], "\n")
}

func stateNote(c inbox.Card) string {
	switch {
	case c.State == inbox.CardDone:
		return "done"
	case c.UpdatedAt.After(c.CreatedAt):
		return "updated"
	}
	return ""
}

func nonEmpty(parts ...string) []string {
	return slices.DeleteFunc(parts, func(s string) bool { return s == "" })
}

// cardDetail renders the detail panel of a card. w is the inner width.
func (m *model) cardDetail(c inbox.Card, w int) string {
	wrap := lipgloss.NewStyle().Width(w)
	var b strings.Builder
	b.WriteString(boldStyle.Render(wrap.Render(render.PlainText(c.Title))) + "\n")
	var meta strings.Builder // one line when it fits, else wrapped at w
	meta.WriteString(catBadge(c.Category) + " " + urgStyles[c.Urgency].Render(urgBar(c.Urgency)+" "+c.Urgency))
	space := orStr(m.spaceNames()[m.cardSpace(c)], "unknown")
	meta.WriteString(faint.Render(" · space ") + warnStyle.Render(space))
	if c.Group != "" {
		meta.WriteString(faint.Render(" · " + c.Group))
	}
	meta.WriteString(faint.Render(" · " + when(c.UpdatedAt, m.now())))
	if c.Key != "" {
		// The key goes to the next line when the line is too long, so it is not cut.
		sep := " · "
		if lipgloss.Width(meta.String())+lipgloss.Width(sep+"key "+c.Key) > w {
			sep = "\n"
		}
		meta.WriteString(faint.Render(sep + "key " + c.Key))
	}
	b.WriteString(wrap.Render(meta.String()) + "\n\n")
	if c.Body != "" {
		b.WriteString(m.md.body(c.ID, c.UpdatedAt, c.Body, w) + "\n\n")
	}
	if c.Reply != "" {
		b.WriteString(panelTitle.Render("Suggested reply") + faint.Render("  c copies it · r replies") + "\n")
		bar := lipgloss.NewStyle().Border(lipgloss.ThickBorder(), false, false, false, true).BorderForeground(accent).PaddingLeft(1).Width(w)
		b.WriteString(bar.Render(c.Reply) + "\n\n")
	}
	if len(c.Refs) > 0 {
		b.WriteString(panelTitle.Render("Messages") + faint.Render("  o opens the first link") + "\n")
		for _, id := range c.Refs {
			msg, ok := m.refs[id]
			if !ok {
				b.WriteString(faint.Render(fmt.Sprintf("#%d message deleted", id)) + "\n")
				continue
			}
			line := fmt.Sprintf("#%d %s · %s · %s  %s", id, when(msg.CreatedAt, m.now()), msg.ChannelName, msg.Author, msg.Text)
			b.WriteString(faint.Render(truncate(line, w)) + "\n")
		}
	}
	if rs := m.replies[c.ID]; len(rs) > 0 {
		b.WriteString("\n")
		for _, r := range rs {
			by := "you"
			if r.By == inbox.ByAgent {
				by = "the agent"
			}
			line := fmt.Sprintf("✓ Replied %s by %s → %s (%s)", when(r.SentAt, m.now()), by, r.ChannelName, r.Place)
			b.WriteString(okStyle.Render(truncate(line, w)) + "\n")
		}
	}
	if as := m.asks[c.ID]; len(as) > 0 {
		b.WriteString("\n")
		for _, a := range as {
			st, note := okStyle, "sent"
			switch a.State {
			case inbox.AskQueued:
				st, note = warnStyle, "queued (agent busy)"
			case inbox.AskFailed:
				st, note = errStyle, "failed: "+a.Detail
			}
			// Cut the text, not the state, when the line is too long.
			head, tail := fmt.Sprintf("↪ Asked %s: ", when(a.CreatedAt, m.now())), " · "+note
			text := truncate(strings.Join(strings.Fields(a.Text), " "), max(10, w-lipgloss.Width(head)-lipgloss.Width(tail)))
			b.WriteString(st.Render(truncate(head+text+tail, w)) + "\n")
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

// refMessages returns the source messages of the card that still exist.
func (m *model) refMessages(c inbox.Card) []inbox.Message {
	var out []inbox.Message
	for _, id := range c.Refs {
		if msg, ok := m.refs[id]; ok {
			out = append(out, msg)
		}
	}
	return out
}

// openReply opens the reply form of the card in the detail panel. The target
// is always a stored message of the card, never a link from the card text.
func (m *model) openReply(c inbox.Card) tea.Cmd {
	target, ok := reply.DefaultTarget(m.refMessages(c))
	if !ok {
		m.setStatus("The card has no message to reply to.", true)
		return nil
	}
	if err := source.CanReply(m.u.cfg, target.SourceID); err != nil {
		m.setStatus("Cannot reply: "+err.Error()+". c copies the reply.", true)
		return nil
	}
	read := m.markRead()
	m.replyCard = c.ID
	m.buildForm(true)
	for i, it := range m.form.items() {
		if it.field != nil && it.field.key == "text" {
			m.form.setFocus(i)
		}
	}
	return tea.Batch(read, m.focusForm())
}

// placeName names the place of a reply to the message as the form shows it.
// WhatsApp has no threads: the thread place is a quote.
func placeName(place string, m inbox.Message) string {
	if place == inbox.PlaceThread && m.SourceType == whatsapp.SourceType {
		return "quote"
	}
	if place == inbox.PlaceChannel && m.ChannelKind != inbox.KindChannel {
		return "chat"
	}
	return place
}

func (m *model) replyForm(c inbox.Card) *form {
	msgs := m.refMessages(c)
	target, _ := reply.DefaultTarget(msgs)
	fields := []*formField{noteField(fmt.Sprintf("Send to    %s · %s · %s", target.ChannelName, target.SourceType, target.SourceID), textStyle)}
	if len(msgs) > 1 {
		var opts, labels []string
		for _, msg := range msgs {
			opts = append(opts, strconv.FormatInt(msg.ID, 10))
			labels = append(labels, fmt.Sprintf("#%d %s %s", msg.ID, msg.Author, when(msg.CreatedAt, m.now())))
		}
		fields = append(fields, choiceField("to", "Reply to", "the message that the reply answers", opts, labels, strconv.FormatInt(target.ID, 10)))
	} else {
		fields = append(fields, noteField(truncate(fmt.Sprintf("Reply to   #%d %s %s  %s", target.ID, target.Author,
			when(target.CreatedAt, m.now()), target.Text), 200), faint))
	}
	thread, channel := "Thread", "Channel"
	if target.ChannelKind != inbox.KindChannel {
		channel = "Chat"
	}
	hint, note := "Thread: in the thread of the message", "A thread reply keeps the read state. A "+strings.ToLower(channel)+" post marks it read."
	if target.SourceType == whatsapp.SourceType {
		thread = "Quote"
		hint, note = "Quote: the reply quotes the message", "i2a sends no read receipt and no typing state."
	}
	text := areaField("text", "Text", c.Reply, fmt.Sprintf("%d characters at most", reply.MaxText), 6, func(s string) error {
		_, err := reply.CheckText(s)
		return err
	})
	text.area.CharLimit = reply.MaxText
	fields = append(fields,
		choiceField("where", "Where", hint, []string{inbox.PlaceThread, inbox.PlaceChannel},
			[]string{thread, channel}, reply.DefaultPlace(target)),
		noteField(note, faint),
		text)
	fm := &form{fields: fields}
	fm.buttons = []button{
		{label: "Send", primary: true, run: func() tea.Cmd { return m.sendReply(c, target, fm) }},
		{label: "Cancel", run: func() tea.Cmd { return func() tea.Msg { return formEsc{} } }},
	}
	return fm
}

// sendReply sends the reply of the form outside the UI loop.
func (m *model) sendReply(c inbox.Card, target inbox.Message, fm *form) tea.Cmd {
	if !fm.validate() {
		return nil
	}
	to := target.ID
	if f := fm.field("to"); f != nil {
		to, _ = strconv.ParseInt(f.value(), 10, 64)
	}
	req := reply.Request{MessageID: to, CardID: c.ID, Text: fm.field("text").value(), Place: fm.field("where").value(),
		By: inbox.ByUser}
	st, cfg, build, ctx, now := m.u.d.Store, m.u.cfg, m.u.d.Replier, m.ctx, m.now
	if st == nil {
		return nil
	}
	fm.busy = "Sending…"
	fm.err = ""
	return func() tea.Msg {
		r, t, err := reply.Send(ctx, st, func(id string) (source.Replier, error) { return build(cfg, id) }, req, now)
		return replyDoneMsg{cardID: c.ID, reply: r, target: t, err: err}
	}
}

func (m *model) closeReply() {
	m.replyCard, m.form, m.formFor = 0, nil, ""
	m.feedFocus = focusCards // the keyboard goes back to the cards
}

// cancelReply closes the reply form. When the user changed it, it asks first.
func (m *model) cancelReply() {
	if m.form == nil || !m.form.dirty() {
		m.closeReply()
		m.setStatus("Reply cancelled. Nothing is sent.", false)
		return
	}
	m.openDialog(&confirmDialog{name: "Discard the reply", text: "Discard the changes to the reply? Nothing is sent.",
		choices: []choice{{"d", "Discard", func() tea.Cmd {
			m.closeReply()
			m.setStatus("Reply cancelled. Nothing is sent.", false)
			return nil
		}}}})
}

// replyDone shows the result of a send. On an error, the form stays open with
// the text and shows the error under the Text field.
func (m *model) replyDone(msg replyDoneMsg) tea.Cmd {
	open := m.form != nil && m.replyCard == msg.cardID
	if open {
		m.form.busy = ""
	}
	if msg.err != nil {
		if open {
			m.form.field("text").err = msg.err.Error()
			m.form.err = "The reply is not sent."
		}
		m.setStatus("The reply is not sent: "+msg.err.Error(), true)
		return nil
	}
	if open {
		m.closeReply()
	}
	for i := range m.cards {
		if m.cards[i].ID == msg.cardID {
			m.cards[i].State = inbox.CardDone
		}
	}
	m.setStatus(fmt.Sprintf("Sent to %s (%s). The card is done.", msg.target.ChannelName,
		placeName(msg.reply.Place, msg.target)), false)
	st, all := m.u.d.Store, m.feedAll
	return func() tea.Msg { return readFeed(m.ctx, st, all) }
}

// waiting reports whether the agent did not answer the newest ask of the
// card yet: the ask is newer than the last update of the card and did not fail.
func (m *model) waiting(c inbox.Card) bool {
	as := m.asks[c.ID]
	if len(as) == 0 {
		return false
	}
	a := as[len(as)-1]
	return a.State != inbox.AskFailed && a.CreatedAt.After(c.UpdatedAt)
}

// askDoneMsg is the result of a send from the ask form.
type askDoneMsg struct {
	cardID int64
	ask    inbox.Ask
	err    error
}

// openAsk opens the ask form of the card in the detail panel.
func (m *model) openAsk(c inbox.Card) tea.Cmd {
	if _, err := ask.TargetOf(m.u.cfg, c, m.refMessages(c)); err != nil {
		m.setStatus("Cannot ask: "+err.Error(), true)
		return nil
	}
	read := m.markRead()
	m.askCard = c.ID
	m.buildForm(true)
	for i, it := range m.form.items() {
		if it.field != nil && it.field.key == "text" {
			m.form.setFocus(i)
		}
	}
	return tea.Batch(read, m.focusForm())
}

func (m *model) askForm(c inbox.Card) *form {
	t, err := ask.TargetOf(m.u.cfg, c, m.refMessages(c))
	agentLine, sessionLine := "Agent      "+t.Agent.Title()+" · space "+orStr(m.spaceNames()[t.Space.ID], t.Space.Title()), "Session    "+t.Session()
	if err != nil {
		agentLine, sessionLine = "Agent      "+err.Error(), ""
	}
	text := areaField("text", "Text", "", fmt.Sprintf("%d characters at most", reply.MaxText), 6, func(s string) error {
		_, err := ask.CheckText(s)
		return err
	})
	text.area.CharLimit = reply.MaxText
	fm := &form{fields: []*formField{
		noteField(agentLine, textStyle),
		noteField(sessionLine, textStyle),
		noteField("The agent answers in this card.", faint),
		text,
	}}
	fm.buttons = []button{
		{label: "Send", primary: true, run: func() tea.Cmd { return m.sendAsk(c, fm) }},
		{label: "Cancel", run: func() tea.Cmd { return func() tea.Msg { return formEsc{} } }},
	}
	return fm
}

// sendAsk sends the ask of the form outside the UI loop. It can wait up to
// ask.Wait for a busy agent.
func (m *model) sendAsk(c inbox.Card, fm *form) tea.Cmd {
	if !fm.validate() {
		return nil
	}
	st := m.u.d.Store
	if st == nil {
		return nil
	}
	d := ask.Deps{Store: st, Config: m.u.cfg, NewDriver: m.u.d.AskDriver, CardCommand: m.u.d.CardCommand,
		LockDir: m.u.d.Paths.DataDir, Now: m.now}
	text, ctx := fm.field("text").value(), m.ctx
	fm.busy = "Sending… (waits up to 2 minutes for a busy agent)"
	fm.err = ""
	return func() tea.Msg {
		a, err := ask.Send(ctx, d, c, text, ask.Wait)
		return askDoneMsg{cardID: c.ID, ask: a, err: err}
	}
}

func (m *model) closeAsk() {
	m.askCard, m.form, m.formFor = 0, nil, ""
	m.feedFocus = focusCards // the keyboard goes back to the cards
}

// cancelAsk closes the ask form. When the user typed text, it asks first.
func (m *model) cancelAsk() {
	if m.form == nil || !m.form.dirty() {
		m.closeAsk()
		m.setStatus("Ask cancelled. Nothing is sent.", false)
		return
	}
	m.openDialog(&confirmDialog{name: "Discard the ask", text: "Discard the text of the ask? Nothing is sent.",
		choices: []choice{{"d", "Discard", func() tea.Cmd {
			m.closeAsk()
			m.setStatus("Ask cancelled. Nothing is sent.", false)
			return nil
		}}}})
}

// askDone shows the result of an ask. On an error, the form stays open with
// the text and shows the error under the Text field.
func (m *model) askDone(msg askDoneMsg) tea.Cmd {
	open := m.form != nil && m.askCard == msg.cardID
	if open {
		m.form.busy = ""
	}
	if msg.err != nil {
		if open {
			m.form.field("text").err = msg.err.Error()
			m.form.err = "The ask is not sent."
		}
		m.setStatus("The ask is not sent: "+msg.err.Error(), true)
		return nil
	}
	if open {
		m.closeAsk()
	}
	if msg.ask.State == inbox.AskSent {
		m.setStatus(fmt.Sprintf("Sent to the agent of %s. It answers in this card.", orStr(m.spaceNames()[msg.ask.Space], msg.ask.Space)), false)
	} else {
		m.setStatus("The agent is busy. i2a sends the ask on the next dispatch.", false)
	}
	st, all := m.u.d.Store, m.feedAll
	return func() tea.Msg { return readFeed(m.ctx, st, all) }
}

// feedHints are the keys of the panel that has the keyboard.
func (m *model) feedHints() []hint {
	switch m.feedFocus {
	case focusSpaces:
		return []hint{{"↑↓", "space"}, {"→", "cards"}, {"ctrl+r", "run now"}, {"F", "space list"}}
	case focusCard:
		return []hint{{"↑↓", "scroll"}, {"r", "reply"}, {"t", "ask agent"}, {"o", "open thread"}, {"c", "copy reply"}, {"←", "cards"}, {"pgup/pgdn", "page"}}
	}
	return []hint{{"r", "reply"}, {"t", "ask agent"}, {"d", "done"}, {"o", "open thread"}, {"→", "card"}, {"←", "spaces"}, {"c", "copy reply"},
		{"u", "unread"}, {"s", "sort"}, {"a", "done cards"}}
}

// activeSpace returns the space of the filter of the Feed and Activity, or ""
// for all spaces. A space that does not exist any more is all spaces.
func (m *model) activeSpace() string {
	if _, ok := m.u.cfg.Space(m.spaceFilter); ok || m.spaceFilter == config.UnsortedID {
		return m.spaceFilter
	}
	return ""
}

// pickSpaceFilter opens the dialog of the F key: all spaces, then each space,
// Unsorted first, each with count(space) items of the unit ("card", "send").
func (m *model) pickSpaceFilter(unit string, count func(space string) int) {
	items := func(n int) string {
		if n == 1 {
			return "1 " + unit
		}
		return fmt.Sprintf("%d %ss", n, unit)
	}
	set := func(id, name string) func() tea.Cmd {
		return func() tea.Cmd {
			if m.setFeedSpace(id) {
				m.setStatus("Space: "+name+".", false)
			}
			return nil
		}
	}
	d := &pickDialog{name: "Show a space", text: "The Feed and the Send log show the " + unit + "s of this space."}
	d.options = append(d.options, pickOption{label: "All spaces", note: items(count("")), run: set("", "all")})
	names := m.spaceNames()
	for _, id := range m.feedSpaces()[1:] {
		if id == m.activeSpace() {
			d.cursor = len(d.options)
		}
		d.options = append(d.options, pickOption{label: names[id], note: items(count(id)), run: set(id, names[id])})
	}
	m.openDialog(d)
}

// cardSpace returns the ID of the space of a card: the space of its newest ref
// message that a dispatch sent, or else the space of the conversation of its
// newest ref message, or "" when no ref message exists.
func (m *model) cardSpace(c inbox.Card) string {
	return card.SpaceOf(c, m.refs,
		func(id string) bool { _, ok := m.u.cfg.Space(id); return ok },
		m.u.cfg.Route)
}
