package whatsapp

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	waLog "go.mau.fi/whatsmeow/util/log"

	"github.com/sondt2709/inbox2agent/internal/config"
	"github.com/sondt2709/inbox2agent/internal/inbox"
)

// quoteMe is the user of the fixtures: fixture 0B quotes a message of fxLID.
var quoteMe = Me{Phone: fxPhone, LID: fxLID}

func TestRowFromEventQuote(t *testing.T) {
	ctx := context.Background()
	ts := time.Date(2026, 10, 29, 9, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name, fx string
		n        fakeNames
		id       string
		author   string
		me       bool
	}{
		{"quote of a contact", "03", fakeNames{names: map[string]string{"100000000000003@lid": "Dana"}}, "3EB00000000000000004", "Dana", false},
		{"author by phone", "03", fakeNames{phones: map[string]string{"100000000000003@lid": "15550000003"}}, "3EB00000000000000004", "+15550000003", false},
		{"author not known", "03", noNames, "3EB00000000000000004", "", false},
		{"quote of the user", "0B", fakeNames{names: map[string]string{fxLID + "@lid": "Me"}}, "3EB0000000000000000C", "", true},
		{"no quote", "01", noNames, "", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r, ok := rowFromEvent(ctx, fixtureByID(t, tc.fx).event(ts), quoteMe, tc.n)
			if !ok {
				t.Fatal("fixture skipped")
			}
			if r.QuoteID != tc.id || r.QuoteAuthor != tc.author || r.QuoteMe != tc.me {
				t.Errorf("quote = %q %q %v, want %q %q %v", r.QuoteID, r.QuoteAuthor, r.QuoteMe, tc.id, tc.author, tc.me)
			}
		})
	}
}

// quoteDB stores all fixtures as the event handler does, with the name Dana
// for the author of the message that fixture 03 quotes.
func quoteDB(t *testing.T) *DB {
	t.Helper()
	ctx := context.Background()
	db, err := OpenDB(ctx, Path(t.TempDir(), "phone"), waLog.Noop)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	n := fakeNames{names: map[string]string{"100000000000003@lid": "Dana"}}
	for i, fx := range fixtures(t) {
		r, ok := rowFromEvent(ctx, fx.event(now.Add(-time.Hour+time.Duration(i)*time.Minute)), quoteMe, n)
		if !ok {
			t.Fatalf("fixture %s skipped", fx.ID)
		}
		if !r.FromMe {
			r.SenderName = "Sender " + fx.Sender[:4]
		}
		if err := db.SaveMessage(ctx, r); err != nil {
			t.Fatal(err)
		}
	}
	return db
}

func collectAll(t *testing.T, db *DB) map[string]inbox.Message {
	t.Helper()
	msgs, _, err := collect(context.Background(), db, Settings{ID: "phone", Default: config.ReadAll, Lookback: 24 * time.Hour}, nil, now)
	if err != nil {
		t.Fatal(err)
	}
	by := map[string]inbox.Message{}
	for _, m := range msgs {
		by[m.ExternalID[len(m.ExternalID)-2:]] = m
	}
	return by
}

func TestCollectQuote(t *testing.T) {
	by := collectAll(t, quoteDB(t))
	for id, want := range map[string]inbox.ReplyRef{
		"03": {Ext: "3EB00000000000000004", Author: "Dana", Text: "conversation 5"},
		// The contact store does not know the author; the quoted row in the file does.
		"13": {Ext: "3EB0000000000000000D", Author: "Sender 1000", Text: "[image] caption 33"},
		"0B": {Ext: "3EB0000000000000000C", Me: true, Text: "conversation 18"},
	} {
		got := by[id].ReplyTo
		if got == nil || *got != want {
			t.Errorf("message %s: reply reference = %+v, want %+v", id, got, want)
		}
		if !strings.HasPrefix(by[id].Text, "> ") {
			t.Errorf("message %s: text = %q, want the > line", id, by[id].Text)
		}
	}
	if r := by["01"].ReplyTo; r != nil {
		t.Errorf("message 01 quotes nothing, reply reference = %+v", r)
	}
}

func TestQuoteColumnsOfAnOlderFile(t *testing.T) {
	ctx := context.Background()
	path := Path(t.TempDir(), "phone")
	db, err := OpenDB(ctx, path, waLog.Noop)
	if err != nil {
		t.Fatal(err)
	}
	r, _ := rowFromEvent(ctx, fixtureByID(t, "03").event(now.Add(-time.Minute)), quoteMe, noNames)
	if err := db.SaveMessage(ctx, r); err != nil {
		t.Fatal(err)
	}
	db.Close()
	// The file of an older version: no quote columns.
	raw, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []string{"quote_id", "quote_author", "quote_me"} {
		if _, err := raw.Exec(`ALTER TABLE wa_messages DROP COLUMN ` + c); err != nil {
			t.Fatal(err)
		}
	}
	raw.Close()

	db, err = OpenDB(ctx, path, waLog.Noop)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	m := collectAll(t, db)["03"]
	if m.ReplyTo != nil || !strings.HasPrefix(m.Text, "> conversation 5\n") {
		t.Errorf("old row: reply reference %+v, text %q; want none and the > line", m.ReplyTo, m.Text)
	}
}
