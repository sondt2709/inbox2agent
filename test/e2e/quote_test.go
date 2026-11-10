//go:build e2e

package e2e

import (
	"os"
	"strings"
	"testing"

	"github.com/sondt2709/inbox2agent/internal/config"
)

// S44: thread replies in Mattermost. A reply under a root of alice is for
// alice at the level mentions, and each reply names its root in reply_to.
func TestS44ThreadReplyNamesItsRoot(t *testing.T) {
	w := mm.NewWorld(t)
	p := newPerson(t, "S44")
	p.addAccount(w)
	p.editConfig(func(c *config.Config) {
		c.Spaces = append(c.Spaces, config.Space{ID: "team", Name: "Team", Agent: "triage", Schedule: "always",
			Conversations: []config.Conversation{
				{Account: "work", ID: w.Other, Read: config.ReadMentions},
				{Account: "work", ID: w.Selected, Read: config.ReadAll},
			}})
	})
	mine := w.Post(t, w.Alice, w.Other, "S44 who can review the release?")
	w.Post(t, w.Bob, w.Other, "S44 noise")
	w.PostReply(t, w.Bob, w.Other, mine, "S44 I can")
	bobs := w.Post(t, w.Bob, w.Other, "S44 bob asks the team")
	w.PostReply(t, w.Bob, w.Other, bobs, "S44 bob adds a note")
	deploy := w.Post(t, w.Bob, w.Selected, "S44 deploy at 5?")
	w.PostReply(t, w.Bob, w.Selected, deploy, "S44 yes, at 5")

	s, code := p.run("run --once")
	if code != 0 || !strings.Contains(s, "batch of Team to triage (dry-run, 3 messages)") {
		t.Fatalf("run --once: exit %d:\n%s", code, s)
	}
	digests := p.digests()
	if len(digests) != 1 {
		t.Fatalf("digests = %q, want 1", digests)
	}
	by := map[string]item{}
	for _, it := range readDigest(t, digests[0]) {
		by[it.Text] = it
	}
	if len(by) != 3 {
		t.Fatalf("digest texts = %q, want the reply to alice, the root and the reply in selected", texts(readDigest(t, digests[0])))
	}
	if r := by["S44 I can"].ReplyTo; r == nil || r.ID != 0 || r.Author != "" || !r.Me || r.Text != "S44 who can review the release?" {
		t.Errorf("reply under the root of alice: reply_to = %+v", r)
	}
	root := by["S44 deploy at 5?"]
	if root.ReplyTo != nil {
		t.Errorf("root has reply_to %+v", root.ReplyTo)
	}
	if r := by["S44 yes, at 5"].ReplyTo; r == nil || r.ID != root.ID || r.Author != w.Bob.Name || r.Me || r.Text != "S44 deploy at 5?" {
		t.Errorf("reply under the root of bob: reply_to = %+v, want the id %d of the root", r, root.ID)
	}
	b, _ := os.ReadFile(digests[0])
	if !strings.Contains(string(b), "`reply_to` is the earlier message that a message quotes or answers in a thread") {
		t.Errorf("digest has no reply_to note:\n%s", b)
	}
}
