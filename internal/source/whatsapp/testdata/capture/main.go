// Command capture exports real rows of wa_messages as test fixtures, with the
// personal data replaced.
//
// Pair a WhatsApp source, receive the messages that the tests need, and run
// from the repository root:
//
//	go run ./internal/source/whatsapp/testdata/capture -db <data>/whatsapp-<id>.db
//
// It writes internal/source/whatsapp/testdata/messages.jsonl: at most -per rows
// for each kind of message (direct or group, own or not, mention, quote, media
// type). Each line holds the row columns and the stored waE2E.Message as
// protojson. The tool replaces each address with a fake one (the account of
// the user becomes 15550009999 and 100000000099999), each message ID with a
// fake ID, each text, name and file name with a neutral text, and removes all
// bytes (media keys, hashes, thumbnails). Read the output before you commit it.
package main

import (
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	_ "modernc.org/sqlite"
)

// Fake account of the user in the fixtures.
const (
	mePhone = "15550009999"
	meLID   = "100000000099999"
)

// Fixture is one line of messages.jsonl.
type Fixture struct {
	Chat    string          `json:"chat"`
	ID      string          `json:"id"`
	Sender  string          `json:"sender"`
	Kind    string          `json:"kind"`
	FromMe  bool            `json:"from_me"`
	Mention bool            `json:"mention"`
	Phone   string          `json:"phone"`
	Message json.RawMessage `json:"message"`
}

func main() {
	dbPath := flag.String("db", "", "the file of a paired source (whatsapp-<id>.db)")
	out := flag.String("out", "internal/source/whatsapp/testdata/messages.jsonl", "the output file")
	per := flag.Int("per", 2, "the rows to keep for each kind of message")
	flag.Parse()
	if *dbPath == "" {
		log.Fatal("give -db")
	}
	db, err := sql.Open("sqlite", "file:"+*dbPath+"?mode=ro")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	s := newScrubber()
	var jid string
	var lid sql.NullString
	if err := db.QueryRow(`SELECT jid, lid FROM whatsmeow_device LIMIT 1`).Scan(&jid, &lid); err != nil {
		log.Fatalf("read the device: %v", err)
	}
	s.fix(jid, mePhone)
	if lid.Valid {
		s.fix(lid.String, meLID)
	}

	rows, err := db.Query(`SELECT chat, id, sender, kind, from_me, mention, phone, raw FROM wa_messages ORDER BY rowid`)
	if err != nil {
		log.Fatal(err)
	}
	defer rows.Close()
	seen := map[string]int{}
	var lines []string
	for rows.Next() {
		var f Fixture
		var raw []byte
		if err := rows.Scan(&f.Chat, &f.ID, &f.Sender, &f.Kind, &f.FromMe, &f.Mention, &f.Phone, &raw); err != nil {
			log.Fatal(err)
		}
		m := &waE2E.Message{}
		if err := proto.Unmarshal(raw, m); err != nil {
			log.Fatal(err)
		}
		key := category(f, m)
		if seen[key] >= *per {
			continue
		}
		seen[key]++
		f.Chat, f.Sender, f.ID = s.jid(f.Chat), s.jid(f.Sender), s.msgID(f.ID)
		if f.Phone != "" {
			f.Phone = s.jid(f.Phone + "@" + types.DefaultUserServer)
			f.Phone = strings.TrimSuffix(f.Phone, "@"+types.DefaultUserServer)
		}
		s.known(m)
		s.message(m.ProtoReflect())
		if f.Message, err = protojson.Marshal(m); err != nil {
			log.Fatal(err)
		}
		line, err := json.Marshal(f)
		if err != nil {
			log.Fatal(err)
		}
		lines = append(lines, string(line))
	}
	if err := rows.Err(); err != nil {
		log.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(*out), 0o755); err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile(*out, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("wrote %d rows to %s\n", len(lines), *out)
	for k, n := range seen {
		fmt.Printf("  %-50s %d\n", k, min(n, *per))
	}
}

// category is the kind of a message, for the selection.
func category(f Fixture, m *waE2E.Message) string {
	c := m.GetExtendedTextMessage().GetContextInfo()
	media := "text"
	switch {
	case m.GetImageMessage() != nil:
		media, c = "image", m.GetImageMessage().GetContextInfo()
	case m.GetVideoMessage() != nil:
		media, c = "video", m.GetVideoMessage().GetContextInfo()
	case m.GetDocumentMessage() != nil:
		media, c = "document", m.GetDocumentMessage().GetContextInfo()
	case m.GetAudioMessage() != nil:
		media, c = "audio", m.GetAudioMessage().GetContextInfo()
	case m.GetStickerMessage() != nil:
		media = "sticker"
	case m.GetConversation() == "" && m.GetExtendedTextMessage() == nil:
		media = "other"
	}
	return fmt.Sprintf("%s from_me=%t mention=%t quote=%t mentions=%t %s", f.Kind, f.FromMe, f.Mention,
		c.GetQuotedMessage() != nil, len(c.GetMentionedJID()) > 0, media)
}

// scrubber replaces the personal data with stable fake values, so that the
// same address gives the same fake address in all rows.
type scrubber struct {
	users map[string]string // user part of an address → fake user part
	ids   map[string]string
	texts int
	next  map[string]int
}

func newScrubber() *scrubber {
	return &scrubber{users: map[string]string{}, ids: map[string]string{}, next: map[string]int{}}
}

func (s *scrubber) fix(addr, user string) {
	if j, err := types.ParseJID(addr); err == nil {
		s.users[j.User] = user
	}
}

func (s *scrubber) user(server, u string) string {
	if f, ok := s.users[u]; ok {
		return f
	}
	s.next[server]++
	n := s.next[server]
	var f string
	switch server {
	case types.GroupServer:
		f = fmt.Sprintf("120363000000%06d", n)
	case types.HiddenUserServer:
		f = fmt.Sprintf("100000000%06d", n)
	default:
		f = fmt.Sprintf("1555000%04d", n)
	}
	s.users[u] = f
	return f
}

// jid replaces the user part of an address and keeps the device and server.
func (s *scrubber) jid(addr string) string {
	j, err := types.ParseJID(addr)
	if err != nil || j.User == "" {
		return addr
	}
	j.User = s.user(j.Server, j.User)
	return j.String()
}

func (s *scrubber) msgID(id string) string {
	if f, ok := s.ids[id]; ok {
		return f
	}
	f := fmt.Sprintf("3EB0%016X", len(s.ids)+1)
	s.ids[id] = f
	return f
}

var (
	jidRe     = regexp.MustCompile(`^[0-9]+(:[0-9]+)?@(s\.whatsapp\.net|lid|g\.us)$`)
	anyJIDRe  = regexp.MustCompile(`[0-9]+(:[0-9]+)?@(s\.whatsapp\.net|lid|g\.us)`)
	mentionRe = regexp.MustCompile(`@[0-9]{6,}`)
)

// keep are the string fields that hold no personal data.
var keep = map[string]bool{"mimetype": true}

// known maps all addresses in a message first, so that a mention in a text
// gets the same fake number as its address.
func (s *scrubber) known(m *waE2E.Message) {
	b, err := protojson.Marshal(m)
	if err != nil {
		log.Fatal(err)
	}
	for _, a := range anyJIDRe.FindAllString(string(b), -1) {
		s.jid(a)
	}
}

// text gives a neutral text and keeps the mentions ("@<number>") in it.
func (s *scrubber) text(field, v string) string {
	s.texts++
	out := fmt.Sprintf("%s %d", field, s.texts)
	for _, m := range mentionRe.FindAllString(v, -1) {
		u := m[1:]
		if _, ok := s.users[u]; !ok {
			// the number of a mention can be a phone number or a LID
			server := types.DefaultUserServer
			if len(u) > 13 {
				server = types.HiddenUserServer
			}
			s.user(server, u)
		}
		out += " @" + s.users[u]
	}
	return out
}

func (s *scrubber) str(fd protoreflect.FieldDescriptor, v string) string {
	name := string(fd.Name())
	switch {
	case v == "" || keep[name]:
		return v
	case jidRe.MatchString(v):
		return s.jid(v)
	case strings.EqualFold(name, "stanzaID") || strings.EqualFold(name, "ID"):
		return s.msgID(v)
	case name == "fileName":
		return fmt.Sprintf("file-%d%s", s.texts+1, filepath.Ext(v))
	}
	return s.text(name, v)
}

// message replaces the strings and removes the bytes in a message, recursively.
func (s *scrubber) message(m protoreflect.Message) {
	type field struct {
		fd protoreflect.FieldDescriptor
		v  protoreflect.Value
	}
	var fields []field
	m.Range(func(fd protoreflect.FieldDescriptor, v protoreflect.Value) bool {
		fields = append(fields, field{fd, v})
		return true
	})
	for _, f := range fields {
		switch {
		case f.fd.IsMap():
			m.Clear(f.fd)
		case f.fd.IsList():
			l := f.v.List()
			for i := 0; i < l.Len(); i++ {
				switch f.fd.Kind() {
				case protoreflect.MessageKind, protoreflect.GroupKind:
					s.message(l.Get(i).Message())
				case protoreflect.StringKind:
					l.Set(i, protoreflect.ValueOfString(s.str(f.fd, l.Get(i).String())))
				case protoreflect.BytesKind:
					l.Set(i, protoreflect.ValueOfBytes(nil))
				}
			}
		case f.fd.Kind() == protoreflect.MessageKind || f.fd.Kind() == protoreflect.GroupKind:
			s.message(f.v.Message())
		case f.fd.Kind() == protoreflect.StringKind:
			m.Set(f.fd, protoreflect.ValueOfString(s.str(f.fd, f.v.String())))
		case f.fd.Kind() == protoreflect.BytesKind:
			m.Clear(f.fd)
		}
	}
}
