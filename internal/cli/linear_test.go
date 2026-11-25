package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sondt2709/inbox2agent/internal/app"
	"github.com/sondt2709/inbox2agent/internal/config"
	"github.com/sondt2709/inbox2agent/internal/secrets"
)

// linearFake answers each request with a captured response of the Linear API
// (internal/source/linear/testdata) and records the keys.
func linearFake(t *testing.T, name string) (*httptest.Server, *[]string) {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "source", "linear", "testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	var fx struct {
		Status int             `json:"status"`
		Body   json.RawMessage `json:"body"`
	}
	if err := json.Unmarshal(b, &fx); err != nil {
		t.Fatal(err)
	}
	var keys []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		keys = append(keys, r.Header.Get("Authorization"))
		w.WriteHeader(fx.Status)
		_, _ = w.Write(fx.Body)
	}))
	t.Cleanup(srv.Close)
	return srv, &keys
}

func TestAccountAddLinear(t *testing.T) {
	t.Setenv("I2A_HOME", t.TempDir())
	t.Setenv("I2A_SECRET_BACKEND", "file")
	srv, keys := linearFake(t, "viewer.json")
	r := i2a(t, "lin_api_one\n", "account", "add", "linear", "--id", "lin", "--url", srv.URL, "--token-stdin")
	if r.err != nil {
		t.Fatal(r.err)
	}
	if !strings.Contains(r.out, "Connected as me.user@acme") || !strings.Contains(r.out, "feeds agent and inbox") {
		t.Errorf("output:\n%s", r.out)
	}
	if len(*keys) != 1 || (*keys)[0] != "lin_api_one" {
		t.Errorf("keys = %v, want the piped key", *keys)
	}
	// A second account with another key.
	if r := i2a(t, "lin_api_two\n", "account", "add", "linear", "--id", "lin-b", "--url", srv.URL, "--token-stdin"); r.err != nil {
		t.Fatal(r.err)
	}
	paths, _ := app.ResolvePaths()
	cfg, err := config.Load(paths.ConfigFile)
	if err != nil {
		t.Fatal(err)
	}
	a, _ := cfg.Account("lin")
	if a.User != "me.user@acme" || len(a.Feeds) != 2 || a.Feeds[0].Prefix != "/agent" || a.Feeds[1].Kind != config.FeedInbox {
		t.Errorf("account = %+v", a)
	}
	sec, err := secrets.Open(paths.SecretsFile)
	if err != nil {
		t.Fatal(err)
	}
	if k, _ := sec.Get(secrets.TokenKey("lin-b")); k != "lin_api_two" {
		t.Errorf("key of lin-b = %q", k)
	}

	r = i2a(t, "", "conversations", "--account", "lin", "--json")
	if r.err != nil {
		t.Fatal(r.err)
	}
	var rows []conversationRow
	if err := json.Unmarshal([]byte(r.out), &rows); err != nil {
		t.Fatalf("%v:\n%s", err, r.out)
	}
	if len(rows) != 2 || rows[0].ID != "agent" || rows[0].Name != "Agent requests" || rows[0].Space != config.UnsortedID || rows[1].ID != "inbox" {
		t.Errorf("conversations = %+v", rows)
	}

	if r := i2a(t, "", "account", "remove", "lin"); r.err != nil {
		t.Fatal(r.err)
	}
	if _, err := sec.Get(secrets.TokenKey("lin")); err == nil {
		t.Error("account remove kept the API key")
	}
}

func TestAccountAddLinearRejectedKey(t *testing.T) {
	t.Setenv("I2A_HOME", t.TempDir())
	t.Setenv("I2A_SECRET_BACKEND", "file")
	srv, _ := linearFake(t, "auth_error.json")
	r := i2a(t, "lin_api_bad\n", "account", "add", "linear", "--id", "lin", "--url", srv.URL, "--token-stdin")
	if r.err == nil || !strings.Contains(r.err.Error(), "the API key is rejected") && !strings.Contains(r.err.Error(), "AUTHENTICATION_ERROR") {
		t.Fatalf("err = %v, want the authentication error", r.err)
	}
	paths, _ := app.ResolvePaths()
	if _, err := os.Stat(paths.ConfigFile); err == nil {
		if cfg, err := config.Load(paths.ConfigFile); err == nil {
			if _, ok := cfg.Account("lin"); ok {
				t.Error("a rejected key added the account")
			}
		}
	}
}
