package tui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/sondt2709/inbox2agent/internal/config"
	"github.com/sondt2709/inbox2agent/internal/source/mattermost"
	"github.com/sondt2709/inbox2agent/internal/source/whatsapp"
)

// pairSteps tells the user how to link i2a on the phone.
const pairSteps = whatsapp.PairSteps

// removeWait is the wait for the daemon to close the connection of a removed source.
const removeWait = 5 * time.Second

// pairing is a WhatsApp pairing that runs in the background.
type pairing struct {
	id, phone string
	qr        string // the QR code, rendered
	code      string // the pairing code
	err       string
	events    chan tea.Msg
	cancel    context.CancelFunc
}

type (
	// pairStepMsg is a new code to show.
	pairStepMsg struct {
		p  *pairing
		ev whatsapp.PairEvent
	}
	// pairDoneMsg ends a pairing.
	pairDoneMsg struct {
		p     *pairing
		chats []whatsapp.Chat
		err   error
	}
)

// startPairing opens the file of the new source and pairs it in the background.
func (m *model) startPairing(id, phone string) tea.Cmd {
	if m.pair != nil {
		return nil // one pairing at a time
	}
	ctx, cancel := context.WithCancel(m.ctx)
	p := &pairing{id: id, phone: phone, events: make(chan tea.Msg, 8), cancel: cancel}
	m.pair = p
	m.buildForm(true)
	dataDir, lookback := m.u.d.Paths.DataDir, m.u.cfg.Timing.Lookback.D()
	go func() {
		done := pairDoneMsg{p: p}
		defer func() { p.events <- done }()
		if err := m.u.d.Paths.EnsureDataDir(); err != nil {
			done.err = err
			return
		}
		conn, err := whatsapp.Open(ctx, dataDir, id, lookback, nil)
		if err != nil {
			done.err = err
			return
		}
		if !conn.Paired() {
			err = conn.Pair(ctx, phone, func(ev whatsapp.PairEvent) { p.events <- pairStepMsg{p: p, ev: ev} })
		}
		if err == nil {
			done.chats, err = conn.DB().Chats(context.Background())
		}
		conn.Close()
		if err != nil && !conn.Paired() {
			_ = whatsapp.Remove(context.Background(), dataDir, id, 0, nil) // nothing to keep
		}
		done.err = err
	}()
	return tea.Batch(m.form.activate(), m.waitPairing())
}

// rebuildPairForm draws the pairing panel again and keeps its focus.
func (m *model) rebuildPairForm() {
	if m.formFor != "src:+pair" {
		return
	}
	active := m.form != nil && m.form.active
	m.buildForm(true)
	if active && m.form != nil {
		m.form.activate()
	}
}

func (m *model) waitPairing() tea.Cmd {
	p := m.pair
	if p == nil {
		return nil
	}
	return func() tea.Msg { return <-p.events }
}

func (m *model) pairStep(msg pairStepMsg) tea.Cmd {
	p := m.pair
	if p == nil || p != msg.p {
		return nil
	}
	if msg.ev.Code != "" {
		p.code = msg.ev.Code
	}
	if msg.ev.QR != "" {
		qr, err := whatsapp.QRText(msg.ev.QR)
		if err != nil {
			p.err = "Cannot draw the QR code: " + err.Error()
		}
		p.qr = qr
	}
	m.rebuildPairForm()
	return m.waitPairing()
}

func (m *model) pairDone(msg pairDoneMsg) tea.Cmd {
	p := m.pair
	if p == nil || p != msg.p {
		return nil
	}
	m.pair = nil
	if msg.err != nil {
		text := msg.err.Error()
		switch {
		case errors.Is(msg.err, context.Canceled):
			text = "Pairing cancelled. Nothing was added."
		case errors.Is(msg.err, whatsapp.ErrPairTimeout):
			text = "Pairing timed out. Nothing was added."
		case errors.Is(msg.err, whatsapp.ErrBusy):
			text = "Another i2a process holds the source " + p.id + ". Nothing was added."
		}
		m.buildForm(true)
		if m.form != nil {
			m.form.err = text
		}
		m.setStatus(text, true)
		return nil
	}
	sc := config.NewWhatsAppSource(p.id)
	sc.Profile = m.u.cfg.Profiles[0].Name
	m.newSrc = &pendingSource{sc: sc}
	m.chs["+new"] = chatInfos(msg.chats)
	m.buildForm(true)
	if m.form == nil {
		return nil
	}
	return m.form.activate()
}

// cancelPairing stops a running pairing.
func (m *model) cancelPairing() {
	if m.pair != nil {
		m.pair.cancel()
	}
}

// qrChrome is the number of window rows around the QR code in the pairing panel.
const qrChrome = 14

// pairForm shows the QR code or the pairing code.
func (m *model) pairForm() *form {
	p := m.pair
	fm := &form{}
	steps := pairSteps + " Device name: \"" + whatsapp.DeviceName + "\"."
	switch {
	case p.code != "":
		fm.fields = []*formField{
			noteField(steps, faint),
			noteField("Tap \"Link with phone number instead\" and type this code:", faint),
			{kind: kindNote, note: "    " + boldStyle.Render(p.code)},
		}
	case p.qr != "" && m.h > 0 && m.h < strings.Count(p.qr, "\n")+1+qrChrome:
		fm.fields = []*formField{noteField(fmt.Sprintf("The window is too short for the QR code. Make it at least %d rows tall, "+
			"or press esc, Cancel, and type a phone number for a pairing code.", strings.Count(p.qr, "\n")+1+qrChrome), warnStyle)}
	case p.qr != "":
		fm.fields = []*formField{
			{kind: kindNote, note: p.qr},
			noteField(steps+" A new code comes each 20 seconds.", faint),
		}
	default:
		fm.fields = []*formField{noteField("Connecting to WhatsApp ...", faint)}
	}
	if p.err != "" {
		fm.fields = append(fm.fields, noteField(p.err, errStyle))
	}
	fm.buttons = []button{{label: "Cancel", run: func() tea.Cmd { m.cancelPairing(); return nil }}}
	return fm
}

// chatInfos converts WhatsApp chats for the channel picker.
func chatInfos(chats []whatsapp.Chat) []mattermost.ChannelInfo {
	out := make([]mattermost.ChannelInfo, 0, len(chats))
	for _, c := range chats {
		name := c.Name
		if name == "" {
			name = strings.SplitN(c.JID, "@", 2)[0]
			if strings.HasSuffix(c.JID, "@s.whatsapp.net") {
				name = "+" + name
			}
		}
		out = append(out, mattermost.ChannelInfo{ID: c.JID, Kind: c.Kind, Name: name, LastPost: c.Last})
	}
	return out
}

// isWhatsApp reports whether the source with the ID is a WhatsApp source.
func (m *model) isWhatsApp(id string) bool {
	sc, ok := m.u.cfg.Source(id)
	return ok && sc.Type == config.SourceWhatsApp
}

// whatsAppState returns "paired" or "not paired" for a saved source.
func (m *model) whatsAppState(id string) string {
	ok, err := whatsapp.IsPaired(m.ctx, m.u.d.Paths.DataDir, id)
	switch {
	case err != nil:
		return "unknown: " + err.Error()
	case ok:
		return "paired"
	}
	return "not paired"
}

// removeWhatsApp logs out a removed source and deletes its file in the background.
func (m *model) removeWhatsApp(id string) tea.Cmd {
	dataDir := m.u.d.Paths.DataDir
	return func() tea.Msg {
		if err := whatsapp.Remove(context.Background(), dataDir, id, removeWait, nil); err != nil {
			return statusMsg{text: whatsapp.UnlinkHint(err), isErr: true}
		}
		return statusMsg{text: "Removed source " + id + " and logged out the linked device."}
	}
}
