package cli

import (
	"strings"
	"testing"
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
