package tui

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"slices"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/sondt2709/inbox2agent/internal/inbox"
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

// feedMsg carries the cards and their source messages, read outside the UI loop.
type feedMsg struct {
	cards []inbox.Card
	refs  map[int64]inbox.Message
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
	return f
}

// setFeed takes new cards and keeps the selection on the same card.
func (m *model) setFeed(f feedMsg) {
	m.cards, m.refs = f.cards, f.refs
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
		meta := truncate(strings.Join(nonEmpty(c.Group, when(c.UpdatedAt, m.now()), stateNote(c)), " · "), inner-2)
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
	if c, ok := m.selectedCard(); ok {
		title = "Card " + fmt.Sprint(c.ID)
		detail = m.cardDetail(c, dw-4)
	}
	right2 := panel(title, detail, dw, h)
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
	if c.Group != "" {
		b.WriteString(faint.Render(" · " + c.Group))
	}
	b.WriteString(faint.Render(" · " + when(c.UpdatedAt, m.now())))
	if c.Key != "" {
		b.WriteString(faint.Render(" · key " + c.Key))
	}
	b.WriteString("\n\n")
	if c.Body != "" {
		b.WriteString(textStyle.Render(wrap.Render(c.Body)) + "\n\n")
	}
	if c.Reply != "" {
		b.WriteString(panelTitle.Render("Suggested reply") + faint.Render("  c copies it") + "\n")
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
	return strings.TrimRight(b.String(), "\n")
}

func feedHints() []hint {
	return []hint{{"↑↓", "move"}, {"c", "copy reply"}, {"o", "open thread"}, {"d", "done"}, {"u", "unread"},
		{"s", "sort"}, {"f", "filter"}, {"a", "done cards"}}
}
