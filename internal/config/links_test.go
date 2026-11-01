package config

import (
	"reflect"
	"testing"
)

// linked has the channel work/C1 in Review (all) and Team (mentions).
func linked(t *testing.T) *Config {
	t.Helper()
	c := Default()
	c.Accounts = []Account{NewMattermostAccount("work", "https://chat.example.com")}
	c.Spaces = []Space{
		{ID: "review", Name: "Review", Agent: "triage", Schedule: "always",
			Conversations: []Conversation{{Account: "work", ID: "C1", Read: ReadAll}}},
		{ID: "team", Name: "Team", Agent: "triage", Schedule: "always",
			Conversations: []Conversation{{Account: "work", ID: "C1", Read: ReadMentions, Label: "Releases"}}},
	}
	if err := c.Validate(); err != nil {
		t.Fatalf("a conversation in two spaces is not valid: %v", err)
	}
	return c
}

func TestLinksOfAConversationInTwoSpaces(t *testing.T) {
	c := linked(t)
	want := []Link{{Space: "review", Read: ReadAll}, {Space: "team", Read: ReadMentions, Label: "Releases"}}
	if got := c.Links("work", "C1"); !reflect.DeepEqual(got, want) {
		t.Errorf("links = %+v, want %+v", got, want)
	}
	if got := c.Routes("work", "C1"); !reflect.DeepEqual(got, []string{"review", "team"}) {
		t.Errorf("routes = %v", got)
	}
	if got := c.Links("work", "C9"); !reflect.DeepEqual(got, []Link{{Space: UnsortedID, Read: ReadMentions}}) {
		t.Errorf("links of an unlisted conversation = %+v", got)
	}
	if l := c.Level("work", "C1"); l != ReadAll {
		t.Errorf("level = %s, want the widest: all", l)
	}
	c.Spaces[0].Conversations[0].Read = ReadOff
	if l, ls := c.Level("work", "C1"), c.Levels("work")["C1"]; l != ReadMentions || ls != ReadMentions {
		t.Errorf("level = %s, levels = %s, want mentions", l, ls)
	}
	if cv, sp, _ := c.Conversation("work", "C1"); sp != "review" || cv.Label != "Releases" {
		t.Errorf("conversation = %+v in %s, want the first space and the first label", cv, sp)
	}
	for _, tc := range []struct {
		read  string
		forMe bool
		want  bool
	}{{ReadAll, false, true}, {ReadMentions, false, false}, {ReadMentions, true, true}, {ReadOff, true, false}} {
		if Wants(tc.read, tc.forMe) != tc.want {
			t.Errorf("Wants(%s, %v) != %v", tc.read, tc.forMe, tc.want)
		}
	}
}

func TestAddLinkMoveAndSetLevel(t *testing.T) {
	c := Default()
	c.Accounts = []Account{NewMattermostAccount("work", "https://chat.example.com")}
	c.Spaces = []Space{{ID: "review", Agent: "triage", Schedule: "always"}, {ID: "team", Agent: "triage", Schedule: "always"}}
	if err := c.SetLabel("work", "C1", "Releases"); err != nil { // an entry in Unsorted
		t.Fatal(err)
	}
	if err := c.AddLink("work", "C1", "review", ReadAll); err != nil {
		t.Fatal(err)
	}
	if err := c.AddLink("work", "C1", "team", ReadMentions); err != nil {
		t.Fatal(err)
	}
	if got := c.Routes("work", "C1"); !reflect.DeepEqual(got, []string{"review", "team"}) || len(c.Unsorted.Conversations) != 0 {
		t.Fatalf("routes = %v, unsorted = %v; want two spaces and no Unsorted entry", got, c.Unsorted.Conversations)
	}
	if err := c.AddLink("work", "C1", "team", ReadAll); err == nil {
		t.Error("a second link to the same space is possible")
	}
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	if err := c.SetLevel("work", "C1", ReadOff); err != nil {
		t.Fatal(err)
	}
	for _, l := range c.Links("work", "C1") {
		if l.Read != ReadOff || l.Label != "Releases" {
			t.Errorf("link %+v after SetLevel off", l)
		}
	}
	if err := c.SetConversation("work", "C1", "team", ReadAll, ""); err != nil {
		t.Fatal(err)
	}
	if got := c.Links("work", "C1"); !reflect.DeepEqual(got, []Link{{Space: "team", Read: ReadAll, Label: "Releases"}}) {
		t.Errorf("links after the move = %+v, want only team", got)
	}
}
