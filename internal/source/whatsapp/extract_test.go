package whatsapp

import (
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"
)

func TestTextOfRealMessages(t *testing.T) {
	for _, tc := range []struct {
		id, want string
	}{
		{"01", "[image]"},
		{"02", "conversation 3"},
		{"03", "> conversation 5\ntext 4"},
		{"05", "text 6 @100000000000004"},
		{"06", "[document: file-9.xlsx]"},
		{"08", "> text 12 @100000000000005\ntext 11"},
		{"0D", "[image] caption 20"},
		{"13", "> [image] caption 33\ntext 29"},
		{"17", "[document: file-43.docx] caption 44"},
	} {
		if got := Text(fixtureByID(t, tc.id).Message); got != tc.want {
			t.Errorf("Text(%s) = %q, want %q", tc.id, got, tc.want)
		}
	}
}

func TestQuoteIsOneLineOf200Characters(t *testing.T) {
	msg := fixtureByID(t, "03").Message // a real reply
	q := msg.GetExtendedTextMessage().GetContextInfo().GetQuotedMessage()
	q.Conversation = proto.String("first line\nsecond  line " + strings.Repeat("é", 300))

	got := Text(msg)
	line, body, _ := strings.Cut(got, "\n")
	if body != "text 4" {
		t.Errorf("body = %q, want %q", body, "text 4")
	}
	if !strings.HasPrefix(line, "> first line second line éé") {
		t.Errorf("quote line = %q, want the quote on one line", line)
	}
	if n := len([]rune(strings.TrimPrefix(line, "> "))); n != quoteMax+1 { // 200 characters and "…"
		t.Errorf("quote has %d characters, want %d", n, quoteMax+1)
	}
}

func TestMentionsMe(t *testing.T) {
	for _, tc := range []struct {
		name string
		id   string
		me   Me
		want bool
	}{
		{"mention of the user (LID)", "05", Me{Phone: fxPhone, LID: "100000000000004"}, true},
		{"mention of another member", "05", Me{Phone: fxPhone, LID: fxLID}, false},
		{"reply to a message of the user", "03", Me{LID: "100000000000003"}, true},
		{"reply to a message of another member", "03", Me{Phone: fxPhone, LID: fxLID}, false},
		{"reply in a direct chat to the user", "0B", Me{Phone: fxPhone, LID: fxLID}, true},
		{"reply to the user by phone number", "13", Me{Phone: "15550000001"}, true},
		{"no context", "02", Me{Phone: fxPhone, LID: fxLID}, false},
		{"empty account", "05", Me{}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := MentionsMe(fixtureByID(t, tc.id).Message, tc.me); got != tc.want {
				t.Errorf("MentionsMe = %t, want %t", got, tc.want)
			}
		})
	}
}
