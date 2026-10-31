package whatsapp

import (
	"context"
	"reflect"
	"slices"
	"testing"
	"time"

	waLog "go.mau.fi/whatsmeow/util/log"

	"github.com/sondt2709/inbox2agent/internal/config"
	"github.com/sondt2709/inbox2agent/internal/inbox"
)

var now = time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)

// fixtureDB stores all fixtures as the event handler does, with the user
// mentioned in fixture 05, and the names of group 2 and direct chat 7.
func fixtureDB(t *testing.T) *DB {
	t.Helper()
	ctx := context.Background()
	db, err := OpenDB(ctx, Path(t.TempDir(), "phone"), waLog.Noop)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	me := Me{Phone: fxPhone, LID: "100000000000004"}
	for i, fx := range fixtures(t) {
		r, ok := rowFromEvent(ctx, fx.event(now.Add(-time.Hour+time.Duration(i)*time.Minute)), me, noNames)
		if !ok {
			t.Fatalf("fixture %s skipped", fx.ID)
		}
		r.Phone, r.SenderName = fx.Phone, "Sender "+fx.Sender[:4]
		if err := db.SaveMessage(ctx, r); err != nil {
			t.Fatal(err)
		}
	}
	for _, c := range []Chat{{JID: group2, Name: "Group two", Kind: inbox.KindGroup}, {JID: direct7, Name: "Carol", Kind: inbox.KindDirect}} {
		if err := db.SaveChat(ctx, c); err != nil {
			t.Fatal(err)
		}
	}
	return db
}

func ids(msgs []inbox.Message) []string {
	var out []string
	for _, m := range msgs {
		out = append(out, m.ExternalID[len(m.ExternalID)-2:])
	}
	return out
}

func TestCollectFilters(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name string
		s    Settings
		want []string
	}{
		{"unsorted mentions, one group all", Settings{Levels: map[string]string{group1: config.ReadAll}, Default: config.ReadMentions},
			[]string{"01", "02", "05", "0B", "0D", "0F", "11", "13", "14", "17"}},
		{"one direct chat all, others off", Settings{Levels: map[string]string{direct5: config.ReadAll}, Default: config.ReadOff}, []string{"0F", "11"}},
		{"mentions in a direct chat is each message", Settings{Levels: map[string]string{direct5: config.ReadMentions}, Default: config.ReadOff}, []string{"0F", "11"}},
		{"mentions in one group", Settings{Levels: map[string]string{group2: config.ReadMentions}, Default: config.ReadOff}, []string{"05"}},
		{"group off, others mentions", Settings{Levels: map[string]string{group2: config.ReadOff}, Default: config.ReadMentions}, []string{"0B", "0D", "0F", "11", "13"}},
		{"all off", Settings{Default: config.ReadOff}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := fixtureDB(t)
			tc.s.ID, tc.s.Lookback = "phone", 24*time.Hour
			msgs, cur, err := collect(ctx, db, tc.s, nil, now)
			if err != nil {
				t.Fatal(err)
			}
			if got := ids(msgs); !slices.Equal(got, tc.want) {
				t.Errorf("messages = %v, want %v", got, tc.want)
			}
			if cur["rowid"] != "19" {
				t.Errorf("cursor = %v, want the last row 19 also for skipped rows", cur)
			}
		})
	}
}

func TestCollectFields(t *testing.T) {
	db := fixtureDB(t)
	msgs, _, err := collect(context.Background(), db, Settings{ID: "phone", Levels: map[string]string{group1: config.ReadAll, group2: config.ReadAll, direct5: config.ReadAll, direct7: config.ReadAll},
		Lookback: 24 * time.Hour}, nil, now)
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]inbox.Message{}
	for _, m := range msgs {
		byID[m.ExternalID[len(m.ExternalID)-2:]] = m
	}
	want := map[string]inbox.Message{
		"0D": {SourceID: "phone", SourceType: "whatsapp", ExternalID: "3EB0000000000000000D", ChannelID: direct7, ChannelName: "Carol",
			ChannelKind: inbox.KindDirect, Author: "Sender 1000", Text: "[image] caption 20", URL: "https://wa.me/15550000001",
			CreatedAt: now.Add(-time.Hour + 9*time.Minute), ForMe: true},
		"05": {SourceID: "phone", SourceType: "whatsapp", ExternalID: "3EB00000000000000005", ChannelID: group2, ChannelName: "Group two",
			ChannelKind: inbox.KindGroup, Author: "Sender 1000", Text: "text 6 @100000000000004", CreatedAt: now.Add(-time.Hour + 3*time.Minute),
			ForMe: true}, // the fixture has mention = true
	}
	for id, w := range want {
		got := byID[id]
		got.CreatedAt, w.CreatedAt = got.CreatedAt.UTC(), w.CreatedAt.UTC()
		if !reflect.DeepEqual(got, w) {
			t.Errorf("message %s =\n %+v\nwant\n %+v", id, got, w)
		}
	}
	if m := byID["01"]; m.ChannelName != "group" || m.URL != "" {
		t.Errorf("group with no name: channel %q, URL %q", m.ChannelName, m.URL)
	}
	if m := byID["0F"]; m.ChannelName != m.Author {
		t.Errorf("direct chat with no name: channel %q, want the sender %q", m.ChannelName, m.Author)
	}
}

func TestCollectCursorLookbackAndPrune(t *testing.T) {
	ctx := context.Background()
	db := fixtureDB(t)
	s := Settings{ID: "phone", Default: config.ReadMentions, Lookback: 24 * time.Hour}

	_, cur, err := collect(ctx, db, s, nil, now)
	if err != nil {
		t.Fatal(err)
	}
	msgs, cur2, err := collect(ctx, db, s, cur, now)
	if err != nil || len(msgs) != 0 || cur2["rowid"] != cur["rowid"] {
		t.Fatalf("second collect = %v, %v, %v; want nothing new", ids(msgs), cur2, err)
	}

	fx := fixtureByID(t, "0F")
	add := func(id string, at time.Time) {
		r, _ := rowFromEvent(ctx, fx.event(at), Me{}, noNames)
		r.ID = id
		if err := db.SaveMessage(ctx, r); err != nil {
			t.Fatal(err)
		}
	}
	add("3EB000000000000000A1", now.Add(-25*time.Hour-time.Minute)) // older than lookback plus one hour
	add("3EB000000000000000A2", now.Add(-24*time.Hour-time.Minute)) // older than lookback
	add("3EB000000000000000A3", now.Add(-time.Minute))
	add("3EB000000000000000A3", now) // the same message again
	msgs, cur3, err := collect(ctx, db, s, cur, now)
	if err != nil {
		t.Fatal(err)
	}
	if got := ids(msgs); !slices.Equal(got, []string{"A3"}) || cur3["rowid"] != "22" {
		t.Errorf("collect = %v, cursor %v; want [A3], 22", got, cur3)
	}

	rows, err := db.Messages(ctx, 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	var stored []string
	for _, r := range rows {
		stored = append(stored, r.ID[len(r.ID)-2:])
	}
	if slices.Contains(stored, "A1") || !slices.Contains(stored, "A2") || len(stored) != 21 {
		t.Errorf("stored after prune = %v; want A1 deleted and A2 kept", stored)
	}
}

func TestSaveChatKeepsNameAndNewestTime(t *testing.T) {
	ctx := context.Background()
	db := fixtureDB(t)
	if err := db.SaveChat(ctx, Chat{JID: group2, Kind: inbox.KindGroup, Last: now}); err != nil {
		t.Fatal(err)
	}
	if err := db.SaveChat(ctx, Chat{JID: group2, Kind: inbox.KindGroup, Last: now.Add(-time.Hour)}); err != nil {
		t.Fatal(err)
	}
	chats, err := db.Chats(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(chats) != 2 || chats[0].JID != group2 || chats[0].Name != "Group two" || !chats[0].Last.Equal(now) {
		t.Errorf("chats = %+v; want group 2 first with its name and the newest time", chats)
	}
}

func TestPhoneOfDeviceJID(t *testing.T) {
	for jid, want := range map[string]string{"15550000001:12@s.whatsapp.net": "+15550000001", "15550000001@s.whatsapp.net": "+15550000001", "": ""} {
		if got := phoneOf(jid); got != want {
			t.Errorf("phoneOf(%q) = %q, want %q", jid, got, want)
		}
	}
}
