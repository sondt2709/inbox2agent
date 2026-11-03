package card

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/sondt2709/inbox2agent/internal/inbox"
	"github.com/sondt2709/inbox2agent/internal/notify"
)

func known(ids ...int64) Known {
	return func([]int64) (map[int64]bool, error) {
		out := map[int64]bool{}
		for _, id := range ids {
			out[id] = true
		}
		return out, nil
	}
}

func TestParseArrayAndObject(t *testing.T) {
	arr, err := Parse(strings.NewReader(`[{"category":"action","title":"a"},{"category":"info","title":"b","refs":[1]}]`))
	if err != nil || len(arr) != 2 || arr[1].Refs[0] != 1 {
		t.Fatalf("array: %+v %v", arr, err)
	}
	one, err := Parse(strings.NewReader(` {"category":"info","title":"b"} `))
	if err != nil || len(one) != 1 || one[0].Title != "b" {
		t.Fatalf("object: %+v %v", one, err)
	}
}

func TestParseRejects(t *testing.T) {
	many := "[" + strings.TrimSuffix(strings.Repeat(`{"category":"info","title":"x"},`, MaxCards+1), ",") + "]"
	cases := map[string]struct{ in, want string }{
		"unknown field": {`[{"category":"info","titel":"x"}]`, `card 1: json: unknown field "titel"`},
		"empty":         {"  ", "no input"},
		"empty array":   {"[]", "no card"},
		"not json":      {"category: info", "invalid character"},
		"too many":      {many, "51 cards in one call; the maximum is 50"},
		"too large":     {strings.Repeat(" ", MaxInputBytes+1), "larger than"},
	}
	for name, c := range cases {
		if _, err := Parse(strings.NewReader(c.in)); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: err = %v, want %q", name, err, c.want)
		}
	}
}

func TestPrepareRejectsWholeCall(t *testing.T) {
	in := []Input{{Category: "action", Title: "ok"}, {Category: "todo", Title: "bad"}}
	cards, _, err := Prepare(in, known())
	if err == nil || cards != nil || !strings.Contains(err.Error(), `card 2: category "todo" is not action, question or info`) {
		t.Errorf("cards=%v err=%v", cards, err)
	}
	for _, c := range []struct {
		in   Input
		want string
	}{
		{Input{Category: "info", Urgency: "urgent", Title: "x"}, `urgency "urgent" is not high, normal or low`},
		{Input{Category: "info", Title: " \u200b\n "}, "the title is empty"},
		{Input{Category: "info", Title: "x", Refs: make([]int64, MaxRefs+1)}, "51 refs"},
	} {
		if _, _, err := Prepare([]Input{c.in}, known()); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%+v: err = %v, want %q", c.in, err, c.want)
		}
	}
}

func TestPrepareSanitizesCutsAndDropsUnknownRefs(t *testing.T) {
	in := []Input{{
		Category: "question",
		Title:    "Fix\u202e the\x1b[2J\nreply  ",
		Group:    "Harbor\nPortal",
		Body:     strings.Repeat("b", 6000),
		Reply:    "line one\nline two",
		Refs:     []int64{812, 99999, 812},
		Key:      "mm:pr-57",
	}}
	cards, warnings, err := Prepare(in, known(812))
	if err != nil {
		t.Fatal(err)
	}
	c := cards[0]
	if c.Title != "Fix the[2J reply" || c.Group != "Harbor Portal" || c.Urgency != inbox.UrgencyNormal {
		t.Errorf("title=%q group=%q urgency=%q", c.Title, c.Group, c.Urgency)
	}
	if len([]rune(c.Body)) != MaxBody || c.Reply != "line one\nline two" {
		t.Errorf("body chars=%d reply=%q", len([]rune(c.Body)), c.Reply)
	}
	if fmt.Sprint(c.Refs) != "[812]" {
		t.Errorf("refs = %v, want [812]", c.Refs)
	}
	got := strings.Join(warnings, "\n")
	for _, want := range []string{"card 1: body cut to 4000 characters", "card 1: message 99999 is not in the store"} {
		if !strings.Contains(got, want) {
			t.Errorf("warnings %q do not contain %q", got, want)
		}
	}
}

func saved(result, cat, urg, prevUrg, prevState, title string) inbox.SavedCard {
	return inbox.SavedCard{Card: inbox.Card{Category: cat, Urgency: urg, Group: "Harbor Portal", Title: title},
		Result: result, PrevUrgency: prevUrg, PrevState: prevState}
}

func notes(n []notify.Note) string {
	var s []string
	for _, x := range n {
		s = append(s, x.Title+" | "+x.Subtitle+" | "+x.Body)
	}
	return strings.Join(s, "\n")
}

// work is the Context of the tests: each card is in the space Work.
var work = Context{Space: func(inbox.Card) string { return "Work" }}

func TestPlan(t *testing.T) {
	add := inbox.SaveAdded
	cases := []struct {
		name  string
		cards []inbox.SavedCard
		min   string
		want  string
	}{
		{"one high and two normal", []inbox.SavedCard{saved(add, "action", "high", "", "", "Fix the reply"),
			saved(add, "question", "normal", "", "", "Review PR"), saved(add, "question", "normal", "", "", "Font?")}, "normal",
			"ACTION · high | Harbor Portal · Work | Fix the reply\ni2a: 2 new cards |  | 2 question"},
		{"one high and one normal", []inbox.SavedCard{saved(add, "action", "high", "", "", "Fix the reply"),
			saved(add, "question", "normal", "", "", "Review PR")}, "normal",
			"ACTION · high | Harbor Portal · Work | Fix the reply\nQUESTION · normal | Harbor Portal · Work | Review PR"},
		{"one normal alone", []inbox.SavedCard{saved(add, "question", "normal", "", "", "Review PR")}, "normal",
			"QUESTION · normal | Harbor Portal · Work | Review PR"},
		{"low is silent", []inbox.SavedCard{saved(add, "info", "low", "", "", "Deployed")}, "normal", ""},
		{"low counts with min low", []inbox.SavedCard{saved(add, "info", "low", "", "", "Deployed")}, "low",
			"INFO · low | Harbor Portal · Work | Deployed"},
		{"min high", []inbox.SavedCard{saved(add, "question", "normal", "", "", "Review PR")}, "high", ""},
		{"unchanged is silent", []inbox.SavedCard{saved(inbox.SaveUnchanged, "action", "high", "high", "read", "x")}, "normal", ""},
		{"update with the same urgency is silent", []inbox.SavedCard{saved(inbox.SaveUpdated, "question", "normal", "normal", "read", "x")}, "normal", ""},
		{"raised urgency", []inbox.SavedCard{saved(inbox.SaveUpdated, "question", "high", "normal", "read", "Review PR before 18:30")}, "normal",
			"QUESTION · high · updated | Harbor Portal · Work | Review PR before 18:30"},
		{"back from done", []inbox.SavedCard{saved(inbox.SaveUpdated, "info", "normal", "normal", "done", "Again")}, "normal",
			"INFO · normal · updated | Harbor Portal · Work | Again"},
	}
	for _, c := range cases {
		if got := notes(Plan(c.cards, c.min, work)); got != c.want {
			t.Errorf("%s:\n got %q\nwant %q", c.name, got, c.want)
		}
	}
}

func TestPlanLimitsHighNotes(t *testing.T) {
	var cards []inbox.SavedCard
	for i := range 10 {
		cards = append(cards, saved(inbox.SaveAdded, "action", "high", "", "", fmt.Sprint("urgent ", i)))
	}
	got := Plan(cards, "normal", work)
	if len(got) != MaxHighNotes+1 {
		t.Fatalf("notes = %d, want %d:\n%s", len(got), MaxHighNotes+1, notes(got))
	}
	if last := got[MaxHighNotes]; last.Title != "i2a: 7 new cards" || last.Body != "7 action" {
		t.Errorf("group note = %+v", last)
	}
}

// The body is the plain title in full; a note opens the newest ref message.
func TestPlanPlainTitleAndClick(t *testing.T) {
	t0 := time.Date(2026, 11, 3, 9, 0, 0, 0, time.UTC)
	refs := map[int64]inbox.Message{
		1: {ID: 1, SourceType: "mattermost", URL: "https://chat.example.com/harbor/pl/old", CreatedAt: t0},
		2: {ID: 2, SourceType: "mattermost", URL: "https://chat.example.com/harbor/pl/new", CreatedAt: t0.Add(time.Minute)},
		3: {ID: 3, SourceType: "whatsapp", ChannelKind: inbox.KindGroup, CreatedAt: t0.Add(2 * time.Minute)},
		4: {ID: 4, SourceType: "whatsapp", URL: "https://wa.me/15550001111", CreatedAt: t0},
	}
	all := func(string) bool { return true }
	long := "**Deploy** of `api` failed on [prod](https://example.com) " + strings.Repeat("x", 150)
	cases := []struct {
		name           string
		refs           []int64
		apps           notify.Apps
		open, activate string
	}{
		{"newest mattermost with the app", []int64{1, 2}, all, "mattermost://chat.example.com/harbor/pl/new", ""},
		{"mattermost without the app", []int64{2}, func(string) bool { return false }, "https://chat.example.com/harbor/pl/new", ""},
		{"whatsapp direct", []int64{4}, all, "whatsapp://send?phone=15550001111", ""},
		{"whatsapp group", []int64{3}, all, "", notify.WhatsAppBundle},
		{"no refs", nil, all, "", ""},
	}
	for _, c := range cases {
		sc := saved(inbox.SaveAdded, "action", "high", "", "", long)
		sc.ID, sc.Refs = 7, c.refs
		got := Plan([]inbox.SavedCard{sc}, "normal", Context{Refs: refs, Apps: c.apps})
		if len(got) != 1 {
			t.Fatalf("%s: notes = %d", c.name, len(got))
		}
		n := got[0]
		if n.Open != c.open || n.Activate != c.activate {
			t.Errorf("%s: open %q activate %q, want %q %q", c.name, n.Open, n.Activate, c.open, c.activate)
		}
		if want := "Deploy of api failed on prod " + strings.Repeat("x", 150); n.Body != want {
			t.Errorf("%s: body = %q", c.name, n.Body)
		}
		if n.Subtitle != "Harbor Portal" || n.Group != "i2a-card-7" {
			t.Errorf("%s: subtitle %q group %q", c.name, n.Subtitle, n.Group)
		}
	}
}

func TestSpaceOf(t *testing.T) {
	t0 := time.Date(2026, 11, 3, 9, 0, 0, 0, time.UTC)
	refs := map[int64]inbox.Message{
		1: {ID: 1, SourceID: "work", ChannelID: "c1", Space: "review", CreatedAt: t0},
		2: {ID: 2, SourceID: "work", ChannelID: "c2", CreatedAt: t0.Add(time.Minute)},
	}
	valid := func(id string) bool { return id != "gone" }
	route := func(src, ch string) string { return "route-" + ch }
	for _, c := range []struct {
		card inbox.Card
		want string
	}{
		{inbox.Card{Space: "ops", Refs: []int64{1}}, "ops"},
		{inbox.Card{Space: "gone", Refs: []int64{1, 2}}, "review"},
		{inbox.Card{Refs: []int64{2}}, "route-c2"},
		{inbox.Card{}, ""},
	} {
		if got := SpaceOf(c.card, refs, valid, route); got != c.want {
			t.Errorf("SpaceOf(%+v) = %q, want %q", c.card, got, c.want)
		}
	}
}

func TestHandle(t *testing.T) {
	for _, tc := range []struct {
		in, want string
		warn     bool
	}{
		{"", "", false},
		{"term_b9f22da8-13d6-49bc-a1ab-53eba56c9578", "term_b9f22da8-13d6-49bc-a1ab-53eba56c9578", false},
		{"term_", "", true},
		{"term_a b", "", true},
		{"term_a\n", "", true},
		{"pane:1", "", true},
		{"term_" + strings.Repeat("a", 65), "", true},
	} {
		got, w := Handle(tc.in)
		if got != tc.want || (w != "") != tc.warn {
			t.Errorf("Handle(%q) = %q, %q; want %q, warning %v", tc.in, got, w, tc.want, tc.warn)
		}
	}
}

func TestDiff(t *testing.T) {
	t0 := time.Date(2026, 11, 3, 9, 0, 0, 0, time.UTC)
	at := func(m int) time.Time { return t0.Add(time.Duration(m) * time.Minute) }
	c := func(id int64, urg, state string, created, updated int) inbox.Card {
		return inbox.Card{ID: id, Category: "action", Urgency: urg, State: state, Title: fmt.Sprint("card ", id),
			CreatedAt: at(created), UpdatedAt: at(updated)}
	}
	first := []inbox.Card{c(1, "normal", "new", 0, 0), c(2, "normal", "read", 1, 1)}
	got, since := Diff(nil, time.Time{}, first)
	if len(got) != 0 || !since.Equal(at(1)) {
		t.Fatalf("first load: %v %v", got, since)
	}
	last := map[int64]inbox.Card{1: first[0], 2: first[1]}
	next := []inbox.Card{
		c(1, "normal", "read", 0, 0),  // the user read it: no change of UpdatedAt
		c(2, "high", "new", 1, 5),     // the agent raised the urgency
		c(3, "normal", "new", 4, 4),   // a new card
		c(4, "normal", "new", -60, 6), // a done card that the agent opened again
		c(5, "low", "done", -90, -90), // a done card that shows with the done filter
	}
	got, since = Diff(last, since, next)
	var s []string
	for _, x := range got {
		s = append(s, fmt.Sprintf("%d %s %s %s", x.ID, x.Result, x.PrevUrgency, x.PrevState))
	}
	want := "2 updated normal read|3 added  |4 updated normal done"
	if strings.Join(s, "|") != want || !since.Equal(at(6)) {
		t.Errorf("Diff = %q since %v, want %q since %v", strings.Join(s, "|"), since, want, at(6))
	}
	// Plan gives a note for the raised, the new and the reopened card.
	if n := Plan(got, "normal", work); len(n) != 2 || n[0].Title != "ACTION · high · updated" || n[1].Title != "i2a: 2 new cards" {
		t.Errorf("Plan = %+v", n)
	}
}
