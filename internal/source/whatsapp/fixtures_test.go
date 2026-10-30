package whatsapp

import (
	"bufio"
	"encoding/json"
	"os"
	"testing"
	"time"

	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	"google.golang.org/protobuf/encoding/protojson"
)

// fixture is one real message from testdata/messages.jsonl (made by
// testdata/capture, with the personal data replaced).
type fixture struct {
	Chat    string          `json:"chat"`
	ID      string          `json:"id"`
	Sender  string          `json:"sender"`
	Kind    string          `json:"kind"`
	FromMe  bool            `json:"from_me"`
	Phone   string          `json:"phone"`
	Raw     json.RawMessage `json:"message"`
	Message *waE2E.Message  `json:"-"`
}

// The fake account of the user in the fixtures.
const (
	fxPhone = "15550009999"
	fxLID   = "100000000099999"
)

// Chats in the fixtures.
const (
	group1  = "120363000000000001@g.us"
	group2  = "120363000000000002@g.us"
	direct5 = "100000000000005@lid"
	direct7 = "100000000000007@lid"
)

// fixtures returns the messages in the file order.
func fixtures(t *testing.T) []fixture {
	t.Helper()
	f, err := os.Open("testdata/messages.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var out []fixture
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		var fx fixture
		if err := json.Unmarshal(sc.Bytes(), &fx); err != nil {
			t.Fatal(err)
		}
		fx.Message = &waE2E.Message{}
		if err := protojson.Unmarshal(fx.Raw, fx.Message); err != nil {
			t.Fatal(err)
		}
		out = append(out, fx)
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

// fixtureByID returns the message with the ID "3EB0…<suffix>".
func fixtureByID(t *testing.T, suffix string) fixture {
	t.Helper()
	for _, fx := range fixtures(t) {
		if len(fx.ID) >= len(suffix) && fx.ID[len(fx.ID)-len(suffix):] == suffix {
			return fx
		}
	}
	t.Fatalf("no fixture %s", suffix)
	return fixture{}
}

// event makes the whatsmeow event of a fixture, as the client gives it.
func (fx fixture) event(ts time.Time) *events.Message {
	return &events.Message{
		Info: types.MessageInfo{
			MessageSource: types.MessageSource{
				Chat:     types.NewJID(split(fx.Chat)),
				Sender:   types.NewJID(split(fx.Sender)),
				IsFromMe: fx.FromMe,
				IsGroup:  fx.Kind == "group",
			},
			ID:        fx.ID,
			Timestamp: ts,
		},
		Message: fx.Message,
	}
}

func split(addr string) (string, string) {
	j, err := types.ParseJID(addr)
	if err != nil {
		panic(err)
	}
	return j.User, j.Server
}
