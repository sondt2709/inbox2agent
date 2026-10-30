package whatsapp

import (
	"strings"

	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
)

// quoteMax limits the quoted text in front of a reply.
const quoteMax = 200

// Text returns the text of a message for the agent: the text, or a media
// placeholder and the caption, with one "> " line for a quoted message.
// It returns "" for a message that has no text for the agent, for example a
// reaction, an edit, a delete or a poll vote.
func Text(m *waE2E.Message) string {
	body := body(m)
	if body == "" {
		return ""
	}
	if q := quoted(m); q != "" {
		return "> " + q + "\n" + body
	}
	return body
}

// body is the text of the message with no quote.
func body(m *waE2E.Message) string {
	if m == nil {
		return ""
	}
	with := func(tag, caption string) string {
		caption = strings.TrimSpace(caption)
		if caption == "" {
			return tag
		}
		return tag + " " + caption
	}
	switch {
	case m.GetConversation() != "":
		return strings.TrimSpace(m.GetConversation())
	case m.GetExtendedTextMessage() != nil:
		return strings.TrimSpace(m.GetExtendedTextMessage().GetText())
	case m.GetImageMessage() != nil:
		return with("[image]", m.GetImageMessage().GetCaption())
	case m.GetVideoMessage() != nil:
		return with("[video]", m.GetVideoMessage().GetCaption())
	case m.GetPtvMessage() != nil:
		return "[video message]"
	case m.GetDocumentMessage() != nil:
		d := m.GetDocumentMessage()
		name := d.GetFileName()
		if name == "" {
			name = d.GetTitle()
		}
		return with("[document: "+name+"]", d.GetCaption())
	case m.GetAudioMessage() != nil:
		if m.GetAudioMessage().GetPTT() {
			return "[voice message]"
		}
		return "[audio]"
	case m.GetStickerMessage() != nil:
		return "[sticker]"
	case m.GetContactMessage() != nil:
		return "[contact: " + m.GetContactMessage().GetDisplayName() + "]"
	case m.GetContactsArrayMessage() != nil:
		return "[contacts: " + m.GetContactsArrayMessage().GetDisplayName() + "]"
	case m.GetLocationMessage() != nil:
		l := m.GetLocationMessage()
		return "[location: " + strings.TrimSpace(l.GetName()+" "+l.GetAddress()) + "]"
	case m.GetLiveLocationMessage() != nil:
		return with("[live location]", m.GetLiveLocationMessage().GetCaption())
	case poll(m) != nil:
		return "[poll] " + poll(m).GetName()
	case m.GetEventMessage() != nil:
		return "[event] " + m.GetEventMessage().GetName()
	case m.GetGroupInviteMessage() != nil:
		return with("[group invite: "+m.GetGroupInviteMessage().GetGroupName()+"]", m.GetGroupInviteMessage().GetCaption())
	}
	return ""
}

func poll(m *waE2E.Message) *waE2E.PollCreationMessage {
	for _, p := range []*waE2E.PollCreationMessage{m.GetPollCreationMessage(), m.GetPollCreationMessageV2(),
		m.GetPollCreationMessageV3(), m.GetPollCreationMessageV5(), m.GetPollCreationMessageV6()} {
		if p != nil {
			return p
		}
	}
	return nil
}

// quoted returns at most quoteMax characters of the quoted message, on one line.
func quoted(m *waE2E.Message) string {
	ci := contextInfo(m)
	if ci == nil || ci.GetQuotedMessage() == nil {
		return ""
	}
	q := strings.Join(strings.Fields(body(ci.GetQuotedMessage())), " ")
	if r := []rune(q); len(r) > quoteMax {
		q = string(r[:quoteMax]) + "…"
	}
	return q
}

type withContext interface {
	GetContextInfo() *waE2E.ContextInfo
}

// contextInfo returns the context of the message (mentions and quote), or nil.
// The protobuf getters accept a nil receiver.
func contextInfo(m *waE2E.Message) *waE2E.ContextInfo {
	if m == nil {
		return nil
	}
	for _, c := range []withContext{m.GetExtendedTextMessage(), m.GetImageMessage(), m.GetVideoMessage(), m.GetPtvMessage(),
		m.GetDocumentMessage(), m.GetAudioMessage(), m.GetStickerMessage(), m.GetContactMessage(), m.GetLocationMessage(),
		m.GetLiveLocationMessage(), poll(m)} {
		if ci := c.GetContextInfo(); ci != nil {
			return ci
		}
	}
	return nil
}

// Me holds the user parts of the addresses of the account: the phone number
// and the LID. Either can be empty.
type Me struct {
	Phone string
	LID   string
}

func (me Me) is(j types.JID) bool {
	return j.User != "" && (j.User == me.Phone || j.User == me.LID)
}

// MentionsMe reports whether the message mentions the user or quotes a
// message of the user.
func MentionsMe(m *waE2E.Message, me Me) bool {
	ci := contextInfo(m)
	if ci == nil {
		return false
	}
	for _, s := range ci.GetMentionedJID() {
		if j, err := types.ParseJID(s); err == nil && me.is(j) {
			return true
		}
	}
	if p := ci.GetParticipant(); p != "" && ci.GetQuotedMessage() != nil {
		if j, err := types.ParseJID(p); err == nil && me.is(j) {
			return true
		}
	}
	return false
}
