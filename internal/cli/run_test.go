package cli

import (
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/sondt2709/inbox2agent/internal/app"
	"github.com/sondt2709/inbox2agent/internal/config"
	"github.com/sondt2709/inbox2agent/internal/daemon"
	"github.com/sondt2709/inbox2agent/internal/inbox"
)

func TestRunAndDaemonHelpUseTheWordsOfTheModel(t *testing.T) {
	for _, cmd := range []string{"run", "daemon"} {
		r := i2a(t, "", cmd, "--help")
		if r.err != nil {
			t.Fatal(r.err)
		}
		for _, old := range []string{"profile", "source", "interval", "active hours"} {
			if strings.Contains(strings.ToLower(r.out), old) {
				t.Errorf("i2a %s --help says %q:\n%s", cmd, old, r.out)
			}
		}
		if !strings.Contains(r.out, "space") || !strings.Contains(r.out, "account") {
			t.Errorf("i2a %s --help does not name the account and the space:\n%s", cmd, r.out)
		}
	}
}

func TestRunOnceWaitsForADaemonThatHoldsItsLock(t *testing.T) {
	spacesEnv(t)
	paths, err := app.ResolvePaths()
	if err != nil {
		t.Fatal(err)
	}
	unlock, err := daemon.Lock(paths.LockFile) // a daemon that starts: it holds the lock, it does not listen yet
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()
	r := i2a(t, "", "run", "--once")
	if r.err == nil || !strings.Contains(r.err.Error(), "a daemon holds its lock") {
		t.Fatalf("run --once while a daemon holds its lock: %v", r.err)
	}
}

func TestStatusCountsTheWaitingMessagesOfEachLink(t *testing.T) {
	c := config.Default()
	c.Accounts = []config.Account{config.NewMattermostAccount("work", "https://chat.example.com")}
	c.Spaces = []config.Space{
		{ID: "review", Name: "Review", Agent: "triage", Schedule: "always",
			Conversations: []config.Conversation{{Account: "work", ID: "C1", Read: config.ReadAll}}},
		{ID: "team", Name: "Team", Agent: "triage", Schedule: "always",
			Conversations: []config.Conversation{{Account: "work", ID: "C1", Read: config.ReadMentions}}},
	}
	pending := []inbox.Message{
		{SourceID: "work", ChannelID: "C1", State: inbox.StatePending, ForMe: true, Delivered: map[string]string{"review": inbox.StateSent}},
		{SourceID: "work", ChannelID: "C1", State: inbox.StatePending, ForMe: false},
	}
	var b strings.Builder
	printConfig(&b, c, t.TempDir(), pending, time.Now())
	for _, want := range []*regexp.Regexp{regexp.MustCompile(`review\s+Review.*\s1 waiting`), regexp.MustCompile(`team\s+Team.*\s1 waiting`)} {
		if !want.MatchString(b.String()) {
			t.Errorf("status does not match %s:\n%s", want, b.String())
		}
	}
}
