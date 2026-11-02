package cli

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/sondt2709/inbox2agent/internal/phone"
)

// fakeFCM is an OAuth token endpoint and an FCM endpoint. The phone package
// tests check the JWT signature; this fake checks only the CLI flow.
type fakeFCM struct {
	srv  *httptest.Server
	code int

	mu       sync.Mutex
	messages []map[string]any
}

func (f *fakeFCM) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.messages)
}

// phoneEnv makes a card environment with a fake Google, and files for setup.
func phoneEnv(t *testing.T) (f *fakeFCM, saFile, appFile string) {
	t.Helper()
	f = &fakeFCM{code: http.StatusOK}
	f.srv = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		switch r.URL.Path {
		case "/token":
			_, _ = w.Write([]byte(`{"access_token":"tok"}`))
		case "/v1/projects/demo-123/messages:send":
			if f.code != http.StatusOK {
				http.Error(w, "unavailable", f.code)
				return
			}
			var m map[string]any
			_ = json.NewDecoder(r.Body).Decode(&m)
			f.messages = append(f.messages, m["message"].(map[string]any))
			_, _ = w.Write([]byte(`{}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(f.srv.Close)
	old := newPhoneSender
	newPhoneSender = func(s phone.State) *phone.Sender {
		snd := phone.NewSender(s)
		snd.FCMBase, snd.Client = f.srv.URL, f.srv.Client()
		return snd
	}
	t.Cleanup(func() { newPhoneSender = old })

	dir := t.TempDir()
	k, _ := rsa.GenerateKey(rand.Reader, 2048)
	der, _ := x509.MarshalPKCS8PrivateKey(k)
	sa, _ := json.Marshal(map[string]string{
		"type": "service_account", "project_id": "demo-123", "client_email": "push@demo-123.iam.gserviceaccount.com",
		"private_key": string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})), "token_uri": f.srv.URL + "/token",
	})
	saFile, appFile = filepath.Join(dir, "sa.json"), filepath.Join(dir, "google-services.json")
	app := `{"project_info":{"project_number":"100200300","project_id":"demo-123"},
	  "client":[{"client_info":{"mobilesdk_app_id":"1:100200300:android:bbbb","android_client_info":{"package_name":"com.sondt.inbox2agent"}},
	  "api_key":[{"current_key":"demo-api-key"}]}]}`
	if err := os.WriteFile(saFile, sa, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(appFile, []byte(app), 0o600); err != nil {
		t.Fatal(err)
	}
	return f, saFile, appFile
}

func mustRun(t *testing.T, stdin string, args ...string) result {
	t.Helper()
	r := i2a(t, stdin, args...)
	if r.err != nil {
		t.Fatalf("i2a %v: %v\n%s", args, r.err, r.errOut)
	}
	return r
}

func pairLink(t *testing.T) url.Values {
	t.Helper()
	r := mustRun(t, "", "phone", "pair", "--link")
	u, err := url.Parse(strings.TrimSpace(r.out))
	if err != nil {
		t.Fatal(err)
	}
	return u.Query()
}

func TestPhoneSetupErrors(t *testing.T) {
	cardEnv(t)
	_, saFile, appFile := phoneEnv(t)
	if r := i2a(t, "", "phone", "setup"); r.err == nil {
		t.Fatal("setup with no flag passed")
	}
	if r := i2a(t, "", "phone", "setup", "--service-account", appFile); r.err == nil || !strings.Contains(r.err.Error(), "private_key") && !strings.Contains(r.err.Error(), "project_id") {
		t.Fatalf("google-services.json as the service account: %v", r.err)
	}
	other := filepath.Join(t.TempDir(), "other.json")
	b, _ := os.ReadFile(appFile)
	_ = os.WriteFile(other, []byte(strings.ReplaceAll(string(b), "demo-123", "other-9")), 0o600)
	r := i2a(t, "", "phone", "setup", "--service-account", saFile, "--app-config", other)
	if r.err == nil || !strings.Contains(r.err.Error(), "demo-123") || !strings.Contains(r.err.Error(), "other-9") {
		t.Fatalf("two projects: %v", r.err)
	}
	if r := i2a(t, "", "phone", "pair"); r.err == nil || !strings.Contains(r.err.Error(), "i2a phone setup") {
		t.Fatalf("pair before setup: %v", r.err)
	}
}

func TestPhoneSetupPairResetStatus(t *testing.T) {
	cardEnv(t)
	f, saFile, appFile := phoneEnv(t)
	r := mustRun(t, "", "phone", "setup", "--service-account", saFile, "--app-config", appFile)
	if !strings.Contains(r.out, "demo-123") || !strings.Contains(r.out, "i2a phone pair") {
		t.Fatalf("setup output: %s", r.out)
	}
	q := pairLink(t)
	if q.Get("p") != "demo-123" || q.Get("s") != "100200300" || q.Get("ak") != "demo-api-key" || !strings.HasPrefix(q.Get("t"), "i2a-") || q.Get("k") == "" {
		t.Fatalf("link = %v", q)
	}
	if again := pairLink(t); again.Get("t") != q.Get("t") || again.Get("k") != q.Get("k") {
		t.Fatal("the second pair made a new pairing")
	}
	full := mustRun(t, "", "phone", "pair")
	if !strings.Contains(full.out, "\x1b[30;47m") || !strings.Contains(full.out, "i2a://pair?") {
		t.Fatalf("pair output has no QR code or link:\n%s", full.out)
	}
	st := mustRun(t, "", "phone", "status")
	if !strings.Contains(st.out, "demo-123") || !strings.Contains(st.out, q.Get("t")[:12]) || strings.Contains(st.out, q.Get("k")) || strings.Contains(st.out, "PRIVATE KEY") {
		t.Fatalf("status:\n%s", st.out)
	}
	mustRun(t, "", "phone", "test")
	if f.count() != 1 {
		t.Fatalf("test push: %d messages", f.count())
	}
	mustRun(t, "", "phone", "reset")
	if n := pairLink(t); n.Get("t") == q.Get("t") || n.Get("k") == q.Get("k") {
		t.Fatal("reset kept the topic or the key")
	}
	mustRun(t, "", "phone", "reset", "--remove")
	if r := i2a(t, "", "phone", "pair"); r.err == nil {
		t.Fatal("pair after reset --remove passed")
	}
}

func TestCardAddPushesToPhone(t *testing.T) {
	cardEnv(t)
	f, saFile, appFile := phoneEnv(t)
	mustRun(t, "", "phone", "setup", "--service-account", saFile, "--app-config", appFile)
	q := pairLink(t)
	in := `[{"category":"action","urgency":"high","title":"Approve","key":"k1","body":"Body https://example.com"},
	        {"category":"info","urgency":"low","title":"FYI","key":"k2"}]`
	r := mustRun(t, in, "card", "add")
	if !strings.Contains(r.out, "Phone: 2.") {
		t.Fatalf("output: %s / %s", r.out, r.errOut)
	}
	if f.count() != 2 {
		t.Fatalf("messages = %d, want 2", f.count())
	}
	key, err := decodeLinkKey(q.Get("k"))
	if err != nil {
		t.Fatal(err)
	}
	for i, want := range []struct {
		title, prio string
		alert       bool
	}{{"Approve", "high", true}, {"FYI", "normal", false}} {
		m := f.messages[i]
		if m["topic"] != q.Get("t") || m["android"].(map[string]any)["priority"] != want.prio {
			t.Fatalf("message %d: %v", i, m)
		}
		plain, err := phone.Open(key, m["data"].(map[string]any)["c"].(string))
		if err != nil {
			t.Fatal(err)
		}
		var p phone.Payload
		_ = json.Unmarshal(plain, &p)
		if p.Kind != phone.KindCard || p.Card.Title != want.title || p.Alert != want.alert {
			t.Fatalf("payload %d: %+v", i, p)
		}
	}
	// The same cards again are unchanged: no message.
	mustRun(t, in, "card", "add")
	if f.count() != 2 {
		t.Fatalf("unchanged cards sent messages: %d", f.count())
	}
}

func TestCardAddPhoneFailureKeepsCards(t *testing.T) {
	cardEnv(t)
	f, saFile, appFile := phoneEnv(t)
	mustRun(t, "", "phone", "setup", "--service-account", saFile, "--app-config", appFile)
	mustRun(t, "", "phone", "pair", "--link")
	f.code = http.StatusServiceUnavailable
	r := i2a(t, `[{"category":"action","urgency":"high","title":"Approve"}]`, "card", "add")
	if r.err != nil || !strings.Contains(r.errOut, "warning: phone push:") || !strings.Contains(r.errOut, "HTTP 503") {
		t.Fatalf("err = %v, stderr = %s", r.err, r.errOut)
	}
	if strings.Contains(r.out, "Phone:") {
		t.Fatalf("output counts a failed push: %s", r.out)
	}
	if got := list(t); len(got) != 1 {
		t.Fatalf("cards = %d, want 1", len(got))
	}
}

func TestCardAddNoPairingOrNotifyOff(t *testing.T) {
	cardEnv(t)
	f, saFile, appFile := phoneEnv(t)
	mustRun(t, "", "phone", "setup", "--service-account", saFile, "--app-config", appFile)
	r := mustRun(t, `[{"category":"action","urgency":"high","title":"One","key":"a"}]`, "card", "add")
	if f.count() != 0 || strings.Contains(r.out, "Phone:") {
		t.Fatalf("no pairing: %d messages, output %s", f.count(), r.out)
	}
	mustRun(t, "", "phone", "pair", "--link")
	t.Setenv("I2A_NOTIFY", "off")
	mustRun(t, `[{"category":"action","urgency":"high","title":"Two","key":"b"}]`, "card", "add")
	if f.count() != 0 {
		t.Fatalf("I2A_NOTIFY=off: %d messages", f.count())
	}
}

func TestCardAddPhoneAlertCap(t *testing.T) {
	cardEnv(t)
	f, saFile, appFile := phoneEnv(t)
	mustRun(t, "", "phone", "setup", "--service-account", saFile, "--app-config", appFile)
	mustRun(t, "", "phone", "pair", "--link")
	var cards []string
	for i := 0; i < 8; i++ {
		cards = append(cards, `{"category":"action","urgency":"high","title":"T`+string(rune('a'+i))+`"}`)
	}
	mustRun(t, "["+strings.Join(cards, ",")+"]", "card", "add")
	high := 0
	for _, m := range f.messages {
		if m["android"].(map[string]any)["priority"] == "high" {
			high++
		}
	}
	if f.count() != 8 || high != MaxPhoneAlerts {
		t.Fatalf("messages = %d, high = %d, want 8 and %d", f.count(), high, MaxPhoneAlerts)
	}
}

func decodeLinkKey(s string) ([]byte, error) { return base64.RawURLEncoding.DecodeString(s) }
