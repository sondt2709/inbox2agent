package whatsapp

import (
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
)

// Media is the file of a message: the part that whatsmeow downloads, and
// the values that the sender gave.
type Media struct {
	Part   whatsmeow.DownloadableMessage
	Kind   string // image, video, document, audio, voice or sticker
	Name   string // the file name of a document; "" for the other kinds
	MIME   string
	Length uint64 // the size in bytes that the sender gave
}

// MediaOf returns the file of the message, or false for a message without a
// file. Only the kinds that have a placeholder in Text count.
func MediaOf(m *waE2E.Message) (Media, bool) {
	switch {
	case m.GetImageMessage() != nil:
		p := m.GetImageMessage()
		return Media{Part: p, Kind: "image", MIME: p.GetMimetype(), Length: p.GetFileLength()}, true
	case m.GetVideoMessage() != nil:
		p := m.GetVideoMessage()
		return Media{Part: p, Kind: "video", MIME: p.GetMimetype(), Length: p.GetFileLength()}, true
	case m.GetPtvMessage() != nil:
		p := m.GetPtvMessage()
		return Media{Part: p, Kind: "video", MIME: p.GetMimetype(), Length: p.GetFileLength()}, true
	case m.GetDocumentMessage() != nil:
		p := m.GetDocumentMessage()
		name := p.GetFileName()
		if name == "" {
			name = p.GetTitle()
		}
		return Media{Part: p, Kind: "document", Name: name, MIME: p.GetMimetype(), Length: p.GetFileLength()}, true
	case m.GetAudioMessage() != nil:
		p := m.GetAudioMessage()
		kind := "audio"
		if p.GetPTT() {
			kind = "voice"
		}
		return Media{Part: p, Kind: kind, MIME: p.GetMimetype(), Length: p.GetFileLength()}, true
	case m.GetStickerMessage() != nil:
		p := m.GetStickerMessage()
		return Media{Part: p, Kind: "sticker", MIME: p.GetMimetype(), Length: p.GetFileLength()}, true
	}
	return Media{}, false
}
