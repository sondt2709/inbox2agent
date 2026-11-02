package phone

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/sondt2709/inbox2agent/internal/secrets"
)

// FakeGoogle is an OAuth token endpoint and an FCM endpoint for tests.
type fakeGoogle struct {
	t       *testing.T
	pub     *rsa.PublicKey
	srv     *httptest.Server
	fcmCode int

	mu       sync.Mutex
	tokens   int
	messages []map[string]any
}

func newFakeGoogle(t *testing.T, pub *rsa.PublicKey) *fakeGoogle {
	f := &fakeGoogle{t: t, pub: pub, fcmCode: http.StatusOK}
	f.srv = httptest.NewTLSServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeGoogle) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	switch r.URL.Path {
	case "/token":
		body, _ := io.ReadAll(r.Body)
		q, _ := url.ParseQuery(string(body))
		if q.Get("grant_type") != "urn:ietf:params:oauth:grant-type:jwt-bearer" {
			http.Error(w, "bad grant", http.StatusBadRequest)
			return
		}
		parts := strings.Split(q.Get("assertion"), ".")
		if len(parts) != 3 {
			http.Error(w, "bad jwt", http.StatusBadRequest)
			return
		}
		sig, _ := base64.RawURLEncoding.DecodeString(parts[2])
		sum := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
		if err := rsa.VerifyPKCS1v15(f.pub, crypto.SHA256, sum[:], sig); err != nil {
			http.Error(w, "bad signature", http.StatusUnauthorized)
			return
		}
		raw, _ := base64.RawURLEncoding.DecodeString(parts[1])
		var claims map[string]any
		_ = json.Unmarshal(raw, &claims)
		if claims["scope"] != Scope || claims["iss"] != "push@demo-123.iam.gserviceaccount.com" || claims["aud"] != f.srv.URL+"/token" {
			http.Error(w, "bad claims", http.StatusUnauthorized)
			return
		}
		f.tokens++
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "tok-1", "expires_in": 3599})
	case "/v1/projects/demo-123/messages:send":
		if r.Header.Get("Authorization") != "Bearer tok-1" {
			http.Error(w, "no auth", http.StatusUnauthorized)
			return
		}
		if f.fcmCode != http.StatusOK {
			http.Error(w, `{"error":{"status":"UNAVAILABLE"}}`, f.fcmCode)
			return
		}
		var m map[string]any
		_ = json.NewDecoder(r.Body).Decode(&m)
		f.messages = append(f.messages, m["message"].(map[string]any))
		_, _ = w.Write([]byte(`{"name":"projects/demo-123/messages/1"}`))
	default:
		http.NotFound(w, r)
	}
}

// testServiceAccount makes a service-account file with a new RSA key.
func testServiceAccount(t *testing.T, tokenURI string) ([]byte, *rsa.PublicKey) {
	t.Helper()
	k, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	der, _ := x509.MarshalPKCS8PrivateKey(k)
	pemKey := string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))
	b, _ := json.Marshal(map[string]string{
		"type": "service_account", "project_id": "demo-123", "client_email": "push@demo-123.iam.gserviceaccount.com",
		"private_key": pemKey, "token_uri": tokenURI,
	})
	return b, &k.PublicKey
}

// googleServices is an invented google-services.json in the shape of the Firebase file.
const googleServices = `{
  "project_info": {"project_number": "100200300", "project_id": "demo-123", "storage_bucket": "demo-123.appspot.com"},
  "client": [
    {"client_info": {"mobilesdk_app_id": "1:100200300:android:aaaa", "android_client_info": {"package_name": "com.example.other"}},
     "api_key": [{"current_key": "other-key"}]},
    {"client_info": {"mobilesdk_app_id": "1:100200300:android:bbbb", "android_client_info": {"package_name": "com.sondt.inbox2agent"}},
     "oauth_client": [], "api_key": [{"current_key": "demo-api-key"}], "services": {}}
  ],
  "configuration_version": "1"
}`

func TestParseServiceAccount(t *testing.T) {
	b, _ := testServiceAccount(t, "https://oauth2.example.com/token")
	sa, err := ParseServiceAccount(b)
	if err != nil {
		t.Fatal(err)
	}
	if sa.ProjectID != "demo-123" || sa.key == nil {
		t.Fatalf("sa = %+v", sa)
	}
	if _, err := ParseServiceAccount([]byte(googleServices)); err == nil || !strings.Contains(err.Error(), `"project_id"`) && !strings.Contains(err.Error(), `"private_key"`) {
		t.Fatalf("google-services.json as a service account: err = %v", err)
	}
	var m map[string]string
	_ = json.Unmarshal(b, &m)
	m["private_key"] = "not pem"
	bad, _ := json.Marshal(m)
	if _, err := ParseServiceAccount(bad); err == nil {
		t.Fatal("a bad private_key passed")
	}
}

func TestParseGoogleServices(t *testing.T) {
	app, err := ParseGoogleServices([]byte(googleServices))
	if err != nil {
		t.Fatal(err)
	}
	want := AppConfig{ProjectID: "demo-123", SenderID: "100200300", AppID: "1:100200300:android:bbbb", APIKey: "demo-api-key"}
	if app != want {
		t.Fatalf("app = %+v, want %+v", app, want)
	}
	other := strings.ReplaceAll(googleServices, "com.sondt.inbox2agent", "com.example.third")
	if _, err := ParseGoogleServices([]byte(other)); err == nil || !strings.Contains(err.Error(), Package) {
		t.Fatalf("no client: err = %v", err)
	}
}

func TestLink(t *testing.T) {
	p, err := NewPairing(time.Unix(1793700000, 0))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(p.Topic, "i2a-") || len(p.Topic) != 36 || len(p.Key) != KeySize {
		t.Fatalf("pairing = %q, %d bytes", p.Topic, len(p.Key))
	}
	app, _ := ParseGoogleServices([]byte(googleServices))
	link := Link(app, p, "Mac mini")
	u, err := url.Parse(link)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	k, _ := base64.RawURLEncoding.DecodeString(q.Get("k"))
	if u.Scheme != "i2a" || u.Host != "pair" || q.Get("v") != "1" || q.Get("p") != "demo-123" || q.Get("s") != "100200300" ||
		q.Get("a") != app.AppID || q.Get("ak") != "demo-api-key" || q.Get("t") != p.Topic || string(k) != string(p.Key) || q.Get("n") != "Mac mini" {
		t.Fatalf("link = %s", link)
	}
	p2, _ := NewPairing(time.Now())
	if p2.Topic == p.Topic || string(p2.Key) == string(p.Key) {
		t.Fatal("two pairings are equal")
	}
}

func fileSecrets(t *testing.T) secrets.Store {
	t.Setenv("I2A_SECRET_BACKEND", "file")
	st, err := secrets.Open(filepath.Join(t.TempDir(), "secrets.json"))
	if err != nil {
		t.Fatal(err)
	}
	return st
}

func TestStateSaveLoad(t *testing.T) {
	st := fileSecrets(t)
	s, err := Load(st)
	if err != nil || s.ServiceAccount != nil || s.App != nil || s.Pairing != nil || s.Ready() {
		t.Fatalf("empty state = %+v, %v", s, err)
	}
	raw, _ := testServiceAccount(t, "https://oauth2.example.com/token")
	if _, err := SaveServiceAccount(st, raw); err != nil {
		t.Fatal(err)
	}
	app, _ := ParseGoogleServices([]byte(googleServices))
	if err := SaveAppConfig(st, app); err != nil {
		t.Fatal(err)
	}
	p, _ := NewPairing(time.Unix(1793700000, 0))
	if err := SavePairing(st, p); err != nil {
		t.Fatal(err)
	}
	s, err = Load(st)
	if err != nil || !s.Ready() {
		t.Fatalf("state = %+v, %v", s, err)
	}
	if s.Pairing.Topic != p.Topic || string(s.Pairing.Key) != string(p.Key) || !s.Pairing.PairedAt.Equal(p.PairedAt) || *s.App != app {
		t.Fatalf("loaded %+v", s)
	}
	if err := RemoveAll(st); err != nil {
		t.Fatal(err)
	}
	if s, _ = Load(st); s.ServiceAccount != nil || s.Pairing != nil {
		t.Fatalf("after RemoveAll: %+v", s)
	}
}

func readySender(t *testing.T) (*Sender, *fakeGoogle) {
	t.Helper()
	f := newFakeGoogle(t, nil)
	raw, pub := testServiceAccount(t, f.srv.URL+"/token")
	f.pub = pub
	sa, err := ParseServiceAccount(raw)
	if err != nil {
		t.Fatal(err)
	}
	p, _ := NewPairing(time.Now())
	s := NewSender(State{ServiceAccount: &sa, App: &AppConfig{}, Pairing: &p})
	s.FCMBase = f.srv.URL
	s.Client = f.srv.Client()
	return s, f
}

func TestSenderSends(t *testing.T) {
	s, f := readySender(t)
	ctx := context.Background()
	alert := &Payload{Kind: KindCard, Alert: true, Card: &Card{ID: 1, Cat: "action", Urg: "high", Title: "A", Created: 1, State: "new"}}
	silent := &Payload{Kind: KindCard, Card: &Card{ID: 2, Cat: "info", Urg: "low", Title: "B", Created: 1, State: "new"}}
	for _, p := range []*Payload{alert, silent} {
		if err := s.Send(ctx, p); err != nil {
			t.Fatal(err)
		}
	}
	if f.tokens != 1 {
		t.Fatalf("token requests = %d, want 1", f.tokens)
	}
	if len(f.messages) != 2 {
		t.Fatalf("messages = %d", len(f.messages))
	}
	for i, wantPrio := range []string{"high", "normal"} {
		m := f.messages[i]
		android := m["android"].(map[string]any)
		data := m["data"].(map[string]any)
		if m["topic"] != s.Topic || android["priority"] != wantPrio || android["ttl"] != "604800s" || data["v"] != "1" {
			t.Fatalf("message %d = %v", i, m)
		}
		plain, err := Open(s.Key, data["c"].(string))
		if err != nil {
			t.Fatal(err)
		}
		var got Payload
		_ = json.Unmarshal(plain, &got)
		if got.V != 1 || got.Card.ID != int64(i+1) || got.Alert != (i == 0) || got.TS == 0 {
			t.Fatalf("payload %d = %+v", i, got)
		}
	}
}

func TestSenderFCMDown(t *testing.T) {
	s, f := readySender(t)
	f.fcmCode = http.StatusServiceUnavailable
	err := s.Send(context.Background(), &Payload{Kind: KindTest, Alert: true})
	if err == nil || !strings.Contains(err.Error(), "HTTP 503") {
		t.Fatalf("err = %v", err)
	}
}

func TestSenderBadSignature(t *testing.T) {
	s, f := readySender(t)
	other, _ := rsa.GenerateKey(rand.Reader, 2048)
	f.pub = &other.PublicKey
	if err := s.Send(context.Background(), &Payload{Kind: KindTest}); err == nil || !strings.Contains(err.Error(), "HTTP 401") {
		t.Fatalf("err = %v", err)
	}
}
