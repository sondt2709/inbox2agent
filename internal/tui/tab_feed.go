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
	"github.com/sondt2709/inbox2agent/internal/inbox"
	"github.com/sondt2709/inbox2agent/internal/reply"
	"github.com/sondt2709/inbox2agent/internal/source"
)

// The Feed tab shows the cards of the agent: a list at the left and the
// selected card at the right. Card text comes from an agent that read
// untrusted messages; "i2a card add" sanitized it, and the Feed opens only the
// URLs of the stored source messages, never a URL from the card text.

const (
	sortUrgency = "urgency"
	sortNewest  = "newest"
)

// feedFilters are the filters of the f key, in order. "" is all categories.
var feedFilters = append([]string{""}, inbox.Categories...)

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

// setFeed takes new cards and keeps the selection on the same card.
func (m *model) setFeed(f feedMsg) {
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
}

// visibleCards returns the cards of the filter in the sort order.
func (m *model) visibleCards() []inbox.Card {
	var v []inbox.Card
	for _, c := range m.cards {
		if c.State == inbox.CardDone && !m.feedAll {
			continue
		}
		if f := feedFilters[m.feedFilter]; f != "" && c.Category != f {
			continue
		}
		if sp := m.feedSpaces()[min(m.feedSpace, len(m.feedSpaces())-1)]; sp != "" && m.cardSpace(c) != sp {
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
	switch k {
	case "up", "k":
		return m.moveFeed(m.feedSel - 1)
	case "down", "j":
		return m.moveFeed(m.feedSel + 1)
	case "s":
		m.feedSort = map[string]string{sortUrgency: sortNewest, sortNewest: sortUrgency}[m.feedSort]
		m.feedSel = 0
		m.setStatus("Sort: "+m.feedSort+".", false)
	case "f":
		m.feedFilter = (m.feedFilter + 1) % len(feedFilters)
		m.feedSel = 0
		m.setStatus("Filter: "+orStr(feedFilters[m.feedFilter], "all")+".", false)
	case "F":
		sp := m.feedSpaces()
		m.feedSpace = (m.feedSpace + 1) % len(sp)
		m.feedSel = 0
		m.setStatus("Space: "+orStr(m.spaceNames()[sp[m.feedSpace]], "all")+".", false)
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
		m.setStatus("Done: "+truncate(c.Title, 60), false)
		return m.setCardState(c, inbox.CardDone)
	case "u":
		m.setStatus("Marked unread.", false)
		return m.setCardState(c, inbox.CardNew)
	}
	return nil
}

// clickFeed selects the clicked card. Each card has two rows.
func (m *model) clickFeed(x, y int) tea.Cmd {
	if x >= feedListWidth(m.w) {
		return nil
	}
	row := (y-2)/2 + m.feedOff // the panel border and the panel title
	if y < 2 || row >= len(m.visibleCards()) {
		return nil
	}
	if row == m.feedSel {
		return m.markRead()
	}
	return m.moveFeed(row)
}

func feedListWidth(w int) int { return max(40, w*9/20) }

var (
	unreadMark = lipgloss.NewStyle().Foreground(lipgloss.Color("#F778BA")).Bold(true)
	catStyles  = map[string]lipgloss.Style{
		inbox.CategoryAction:   lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#111111")).Background(red),
		inbox.CategoryQuestion: lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#111111")).Background(yellow),
		inbox.CategoryInfo:     lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#111111")).Background(blue),
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

func urgLabel(u string) string { return urgStyles[u].Render(fmt.Sprintf("%-6s", u)) }

// when shows the time of today, or the date of an earlier day.
func when(t, now time.Time) string {
	t = t.Local()
	if y, m, d := t.Date(); y == now.Year() && m == now.Month() && d == now.Day() {
		return t.Format("15:04")
	}
	return t.Format("02 Jan")
}

func (m *model) feedCounts() (fresh, actions, questions int) {
	for _, c := range m.cards {
		if c.State == inbox.CardDone {
			continue
		}
		if c.State == inbox.CardNew {
			fresh++
		}
		switch c.Category {
		case inbox.CategoryAction:
			actions++
		case inbox.CategoryQuestion:
			questions++
		}
	}
	return
}

func (m *model) viewFeed(w, h int) string {
	fresh, actions, questions := m.feedCounts()
	var bar strings.Builder
	for i, f := range feedFilters {
		n := 0
		for _, c := range m.cards {
			if (c.State != inbox.CardDone || m.feedAll) && (f == "" || c.Category == f) {
				n++
			}
		}
		label := fmt.Sprintf("%s %d", orStr(strings.ToUpper(f[:min(1, len(f))])+f[min(1, len(f)):], "All"), n)
		if i == m.feedFilter {
			bar.WriteString(tabActive.Render(label))
		} else {
			bar.WriteString(tabInactive.Render(label))
		}
		bar.WriteString(" ")
	}
	right := faint.Render(fmt.Sprintf("%d new · %d action · %d question · sort: ", fresh, actions, questions)) + warnStyle.Render(m.feedSort)
	if m.feedAll {
		right += faint.Render(" · done shown")
	}
	if sp := m.feedSpaces()[min(m.feedSpace, len(m.feedSpaces())-1)]; sp != "" {
		right = faint.Render("space: ") + warnStyle.Render(m.spaceNames()[sp]) + faint.Render(" · ") + right
	}
	gap := max(1, w-lipgloss.Width(bar.String())-lipgloss.Width(right)-1)
	top := bar.String() + strings.Repeat(" ", gap) + right
	h--

	lw := feedListWidth(w)
	v := m.visibleCards()
	rowsPerCard := 2
	visible := max(1, (h-3)/rowsPerCard)
	m.feedOff = scrollTo(m.feedSel, m.feedOff, visible)
	var list []string
	inner := lw - 4
	for i := m.feedOff; i < min(len(v), m.feedOff+visible); i++ {
		c := v[i]
		mark, plainMark := "  ", "  "
		if c.State == inbox.CardNew {
			mark, plainMark = unreadMark.Render("● "), "● "
		}
		titleW := max(5, inner-2-12-7-2)
		title := truncate(c.Title, titleW)
		replied := ""
		if len(m.replies[c.ID]) > 0 {
			replied = "✓ replied"
		}
		asked := ""
		if m.waiting(c) {
			asked = "↪ asked"
		}
		meta := truncate(strings.Join(nonEmpty(m.spaceNames()[m.cardSpace(c)], c.Group, when(c.UpdatedAt, m.now()), stateNote(c), replied, asked), " · "), inner-2)
		if i == m.feedSel {
			list = append(list, selectedRow.Width(inner).Render("▌ "+plainMark+fmt.Sprintf(" %-8s  %-6s ", strings.ToUpper(c.Category), c.Urgency)+title))
			list = append(list, selectedRow.Width(inner).Render("▌     "+meta))
			continue
		}
		t := textStyle
		if c.State != inbox.CardNew {
			t = faint
		}
		list = append(list, "  "+mark+catBadge(c.Category)+" "+urgLabel(c.Urgency)+" "+t.Render(title))
		list = append(list, "      "+faint.Render(meta))
	}
	body := strings.Join(list, "\n")
	switch {
	case m.u.d.Store == nil:
		body = faint.Render("No cards yet. The agent adds cards with\n\"i2a card add\" after each batch.")
	case len(v) == 0 && len(m.cards) == 0:
		body = faint.Render("No cards yet. The agent adds cards with\n\"i2a card add\" after each batch.")
	case len(v) == 0:
		body = faint.Render("No card matches the filter. Press f to change it.")
	}
	left := panel(fmt.Sprintf("Cards · %d", len(v)), body, lw, h)

	dw := w - lw
	detail := ""
	title := "Card"
	c, ok := m.selectedCard()
	if ok {
		title = "Card " + fmt.Sprint(c.ID)
		detail = m.cardDetail(c, dw-4)
	}
	right2 := panel(title, detail, dw, h)
	if ok && m.form != nil && m.replyCard == c.ID {
		right2 = detailPanel(fmt.Sprintf("Reply · card %d", c.ID), m.form.render(dw-4, h-4), dw, h, m.form.active)
	}
	if ok && m.form != nil && m.askCard == c.ID {
		right2 = detailPanel(fmt.Sprintf("Ask the agent · card %d", c.ID), m.form.render(dw-4, h-4), dw, h, m.form.active)
	}
	return top + "\n" + lipgloss.JoinHorizontal(lipgloss.Top, left, right2)
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
	b.WriteString(boldStyle.Render(wrap.Render(c.Title)) + "\n")
	b.WriteString(catBadge(c.Category) + " " + urgStyles[c.Urgency].Render(c.Urgency))
	space := orStr(m.spaceNames()[m.cardSpace(c)], "unknown")
	b.WriteString(faint.Render(" · space ") + warnStyle.Render(space))
	if c.Group != "" {
		b.WriteString(faint.Render(" · " + c.Group))
	}
	b.WriteString(faint.Render(" · " + when(c.UpdatedAt, m.now())))
	if c.Key != "" {
		b.WriteString(faint.Render(" · key " + c.Key))
	}
	b.WriteString("\n\n")
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

// placeName names the place of a reply as the form shows it.
func placeName(place, kind string) string {
	if place == inbox.PlaceChannel && kind != inbox.KindChannel {
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
	channel := "Channel"
	if target.ChannelKind != inbox.KindChannel {
		channel = "Chat"
	}
	text := areaField("text", "Text", c.Reply, fmt.Sprintf("%d characters at most", reply.MaxText), 6, func(s string) error {
		_, err := reply.CheckText(s)
		return err
	})
	text.area.CharLimit = reply.MaxText
	fields = append(fields,
		choiceField("where", "Where", "Thread: in the thread of the message", []string{inbox.PlaceThread, inbox.PlaceChannel},
			[]string{"Thread", channel}, reply.DefaultPlace(target)),
		noteField("A thread reply keeps the read state. A "+strings.ToLower(channel)+" post marks it read.", faint),
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
		placeName(msg.reply.Place, msg.target.ChannelKind)), false)
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
	agentLine, sessionLine := "Agent      "+t.AgentText(), "Session    "+t.Session()
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

func feedHints() []hint {
	return []hint{{"↑↓", "move"}, {"r", "reply"}, {"t", "ask agent"}, {"c", "copy reply"}, {"o", "open thread"}, {"d", "done"},
		{"u", "unread"}, {"s", "sort"}, {"f", "category"}, {"F", "space"}, {"a", "done cards"}}
}

// feedSpaces returns the space filters of the F key: "" (all), then each
// space, Unsorted first.
func (m *model) feedSpaces() []string {
	out := []string{""}
	for _, s := range m.u.cfg.AllSpaces() {
		out = append(out, s.ID)
	}
	return out
}

// cardSpace returns the ID of the space of a card: the space of its newest ref
// message that a dispatch sent, or else the space of the conversation of its
// newest ref message, or "" when no ref message exists.
func (m *model) cardSpace(c inbox.Card) string {
	var newest, sent inbox.Message
	for _, id := range c.Refs {
		msg, ok := m.refs[id]
		if !ok {
			continue
		}
		if msg.CreatedAt.After(newest.CreatedAt) {
			newest = msg
		}
		if msg.Space != "" && msg.CreatedAt.After(sent.CreatedAt) {
			sent = msg
		}
	}
	switch {
	case sent.Space != "":
		return sent.Space
	case newest.SourceID != "":
		return m.u.cfg.Route(newest.SourceID, newest.ChannelID)
	}
	return ""
}
