package daemon

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/sondt2709/inbox2agent/internal/agent"
	"github.com/sondt2709/inbox2agent/internal/config"
	"github.com/sondt2709/inbox2agent/internal/inbox"
	"github.com/sondt2709/inbox2agent/internal/render"
)

// twoSpaces has the channel work/Cr in Review (all) and in Team (mentions),
// each with its own orca agent.
func twoSpaces() *config.Config {
	c := cfgWith("work")
	c.Agents = append(c.Agents, config.NewAgent("reviewer"), config.NewAgent("teamer"))
	for i := range c.Agents {
		c.Agents[i].Driver, c.Agents[i].Workspace = config.DriverOrca, "/w"
	}
	c.Spaces = []config.Space{
		{ID: "review", Name: "Review", Agent: "reviewer", Schedule: "always",
			Conversations: []config.Conversation{{Account: "work", ID: "Cr", Read: config.ReadAll}}},
		{ID: "team", Name: "Team", Agent: "teamer", Schedule: "always",
			Conversations: []config.Conversation{{Account: "work", ID: "Cr", Read: config.ReadMentions}}},
	}
	return c
}

func (f *fakeSource) addFor(id string, forMe bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.msgs = append(f.msgs, inbox.Message{SourceID: f.id, SourceType: "fake", ExternalID: id, ChannelID: "Cr",
		ChannelName: "release", ChannelKind: inbox.KindChannel, Author: "bob", Text: "text " + id, CreatedAt: time.Now(), ForMe: forMe})
}

// spaceDriver sends to a recorder for each space, so a test can make one fail.
type spaceDriver struct {
	by  map[string]*recorder
	err map[string]error
}

func (h *harness) bySpace(errs map[string]error) *spaceDriver {
	d := &spaceDriver{by: map[string]*recorder{}, err: errs}
	h.e.d.NewDriver = func(_ config.Agent, space string, _ agent.Sessions) (agent.Driver, error) {
		r, ok := d.by[space]
		if !ok {
			r = &recorder{result: agent.Result{Status: agent.StatusSent}, err: d.err[space]}
			d.by[space] = r
		}
		return r, nil
	}
	return d
}

func batchIDs(r *recorder) [][]string {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	var out [][]string
	for _, b := range r.batches {
		out = append(out, ids(b))
	}
	return out
}

func ids(b *render.Rendered) []string {
	var out []string
	for _, it := range b.Items {
		out = append(out, strings.TrimPrefix(it.Text, "text "))
	}
	return out
}

func TestOneMessageGoesToEachLinkedSpace(t *testing.T) {
	h := newHarness(t, twoSpaces())
	d := h.bySpace(nil)
	ctx := context.Background()
	h.sources["work"].addFor("plain", false)
	h.sources["work"].addFor("mention", true)
	h.e.Poll(ctx, true)
	res, err := h.e.Dispatch(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Batches) != 2 {
		t.Fatalf("batches = %+v, want one for Review and one for Team", res.Batches)
	}
	if got := batchIDs(d.by["review"]); !slices.Equal(got[0], []string{"plain", "mention"}) {
		t.Errorf("Review got %v, want both messages (all)", got)
	}
	if got := batchIDs(d.by["team"]); !slices.Equal(got[0], []string{"mention"}) {
		t.Errorf("Team got %v, want only the message for the user (mentions)", got)
	}
	if p, _ := h.st.Pending(ctx); len(p) != 0 {
		t.Errorf("pending = %+v, want none: each space that wants a message has it", p)
	}
	if res, _ := h.e.Dispatch(ctx, nil); len(res.Batches) != 0 {
		t.Errorf("the second round sent %+v, want nothing", res.Batches)
	}
}

func TestAFailedSpaceDoesNotResendToTheOther(t *testing.T) {
	h := newHarness(t, twoSpaces())
	d := h.bySpace(map[string]error{"team": errors.New("orca: not reachable")})
	ctx := context.Background()
	h.sources["work"].addFor("mention", true)
	h.e.Poll(ctx, true)
	if _, err := h.e.Dispatch(ctx, nil); err != nil {
		t.Fatal(err)
	}
	p, _ := h.st.Pending(ctx)
	if len(p) != 1 || p[0].State != inbox.StatePending || p[0].Delivered["review"] != inbox.StateSent || p[0].Delivered["team"] != "" {
		t.Fatalf("pending = %+v, want the message pending with a delivery to Review only", p)
	}
	d.by["team"].err = nil // Team works again
	if _, err := h.e.Dispatch(ctx, nil); err != nil {
		t.Fatal(err)
	}
	if got := batchIDs(d.by["review"]); len(got) != 1 {
		t.Errorf("Review got %d batches, want 1: the message was sent to it", len(got))
	}
	if got := batchIDs(d.by["team"]); len(got) != 2 || !slices.Equal(got[1], []string{"mention"}) {
		t.Errorf("Team batches = %v, want the failed one and then the message", got)
	}
	if p, _ := h.st.Pending(ctx); len(p) != 0 {
		t.Errorf("pending = %+v, want none", p)
	}
}
