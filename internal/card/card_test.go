package card

import (
	"fmt"
	"strings"
	"testing"

	"github.com/sondt2709/inbox2agent/internal/inbox"
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

func notes(n []Note) string {
	var s []string
	for _, x := range n {
		s = append(s, x.Title+" | "+x.Text)
	}
	return strings.Join(s, "\n")
}

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
			"i2a · ACTION · high | Harbor Portal: Fix the reply\ni2a: 2 new cards | 2 question"},
		{"one high and one normal", []inbox.SavedCard{saved(add, "action", "high", "", "", "Fix the reply"),
			saved(add, "question", "normal", "", "", "Review PR")}, "normal",
			"i2a · ACTION · high | Harbor Portal: Fix the reply\ni2a · QUESTION · normal | Harbor Portal: Review PR"},
		{"one normal alone", []inbox.SavedCard{saved(add, "question", "normal", "", "", "Review PR")}, "normal",
			"i2a · QUESTION · normal | Harbor Portal: Review PR"},
		{"low is silent", []inbox.SavedCard{saved(add, "info", "low", "", "", "Deployed")}, "normal", ""},
		{"low counts with min low", []inbox.SavedCard{saved(add, "info", "low", "", "", "Deployed")}, "low",
			"i2a · INFO · low | Harbor Portal: Deployed"},
		{"min high", []inbox.SavedCard{saved(add, "question", "normal", "", "", "Review PR")}, "high", ""},
		{"unchanged is silent", []inbox.SavedCard{saved(inbox.SaveUnchanged, "action", "high", "high", "read", "x")}, "normal", ""},
		{"update with the same urgency is silent", []inbox.SavedCard{saved(inbox.SaveUpdated, "question", "normal", "normal", "read", "x")}, "normal", ""},
		{"raised urgency", []inbox.SavedCard{saved(inbox.SaveUpdated, "question", "high", "normal", "read", "Review PR before 18:30")}, "normal",
			"i2a · QUESTION · high (updated) | Harbor Portal: Review PR before 18:30"},
		{"back from done", []inbox.SavedCard{saved(inbox.SaveUpdated, "info", "normal", "normal", "done", "Again")}, "normal",
			"i2a · INFO · normal (updated) | Harbor Portal: Again"},
	}
	for _, c := range cases {
		if got := notes(Plan(c.cards, c.min)); got != c.want {
			t.Errorf("%s:\n got %q\nwant %q", c.name, got, c.want)
		}
	}
}

func TestPlanLimitsHighNotes(t *testing.T) {
	var cards []inbox.SavedCard
	for i := range 10 {
		cards = append(cards, saved(inbox.SaveAdded, "action", "high", "", "", fmt.Sprint("urgent ", i)))
	}
	got := Plan(cards, "normal")
	if len(got) != MaxHighNotes+1 {
		t.Fatalf("notes = %d, want %d:\n%s", len(got), MaxHighNotes+1, notes(got))
	}
	if last := got[MaxHighNotes]; last.Title != "i2a: 7 new cards" || last.Text != "7 action" {
		t.Errorf("group note = %+v", last)
	}
}

func TestPlanCutsLongText(t *testing.T) {
	got := Plan([]inbox.SavedCard{saved(inbox.SaveAdded, "action", "high", "", "", strings.Repeat("x", 500))}, "normal")
	if n := len([]rune(got[0].Text)); n > 203 {
		t.Errorf("text has %d characters, want 200 and the ellipsis", n)
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
