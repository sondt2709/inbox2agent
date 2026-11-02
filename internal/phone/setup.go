package phone

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/sondt2709/inbox2agent/internal/secrets"
)

// Package is the Android package of the app. A Firebase project must register
// an Android app with this package.
const Package = "com.sondt.inbox2agent"

// Keys in the secret store.
const (
	KeyServiceAccount = "phone/service-account"
	KeyAppConfig      = "phone/app-config"
	KeyTopic          = "phone/topic"
	KeyKey            = "phone/key"
	KeyPairedAt       = "phone/paired-at"
)

// ServiceAccount is the part of a Firebase service-account key that the sender uses.
type ServiceAccount struct {
	ProjectID   string `json:"project_id"`
	ClientEmail string `json:"client_email"`
	PrivateKey  string `json:"private_key"`
	TokenURI    string `json:"token_uri"`

	key *rsa.PrivateKey
}

// ParseServiceAccount reads a service-account JSON file and checks its fields and key.
func ParseServiceAccount(b []byte) (ServiceAccount, error) {
	var sa ServiceAccount
	if err := json.Unmarshal(b, &sa); err != nil {
		return sa, fmt.Errorf("the service-account file is not JSON: %w", err)
	}
	for _, f := range []struct{ name, v string }{
		{"project_id", sa.ProjectID}, {"client_email", sa.ClientEmail}, {"private_key", sa.PrivateKey}, {"token_uri", sa.TokenURI},
	} {
		if f.v == "" {
			return sa, fmt.Errorf("the service-account file has no field %q; use the key file of a service account (Firebase console, Project settings, Service accounts)", f.name)
		}
	}
	if !strings.HasPrefix(sa.TokenURI, "https://") {
		return sa, fmt.Errorf("token_uri %q is not an https URL", sa.TokenURI)
	}
	block, _ := pem.Decode([]byte(sa.PrivateKey))
	if block == nil {
		return sa, errors.New("private_key is not PEM")
	}
	k, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		k, err = x509.ParsePKCS1PrivateKey(block.Bytes)
	}
	if err != nil {
		return sa, fmt.Errorf("private_key: %w", err)
	}
	rk, ok := k.(*rsa.PrivateKey)
	if !ok {
		return sa, errors.New("private_key is not an RSA key")
	}
	sa.key = rk
	return sa, nil
}

// AppConfig is the Firebase configuration of the Android app. It goes to the
// phone in the pairing link, so the APK holds no Firebase project.
type AppConfig struct {
	ProjectID string `json:"project_id"`
	SenderID  string `json:"sender_id"`
	AppID     string `json:"app_id"`
	APIKey    string `json:"api_key"`
}

// ParseGoogleServices reads a google-services.json file and takes the client of Package.
func ParseGoogleServices(b []byte) (AppConfig, error) {
	var f struct {
		ProjectInfo struct {
			ProjectID     string `json:"project_id"`
			ProjectNumber string `json:"project_number"`
		} `json:"project_info"`
		Client []struct {
			ClientInfo struct {
				AppID   string `json:"mobilesdk_app_id"`
				Android struct {
					Package string `json:"package_name"`
				} `json:"android_client_info"`
			} `json:"client_info"`
			APIKey []struct {
				Key string `json:"current_key"`
			} `json:"api_key"`
		} `json:"client"`
	}
	if err := json.Unmarshal(b, &f); err != nil {
		return AppConfig{}, fmt.Errorf("the app configuration is not JSON: %w", err)
	}
	if f.ProjectInfo.ProjectID == "" || f.ProjectInfo.ProjectNumber == "" {
		return AppConfig{}, errors.New("the app configuration has no project_info; use the google-services.json file of the Android app")
	}
	for _, c := range f.Client {
		if c.ClientInfo.Android.Package != Package {
			continue
		}
		if c.ClientInfo.AppID == "" || len(c.APIKey) == 0 || c.APIKey[0].Key == "" {
			return AppConfig{}, fmt.Errorf("the client %s has no mobilesdk_app_id or api_key", Package)
		}
		return AppConfig{ProjectID: f.ProjectInfo.ProjectID, SenderID: f.ProjectInfo.ProjectNumber, AppID: c.ClientInfo.AppID, APIKey: c.APIKey[0].Key}, nil
	}
	return AppConfig{}, fmt.Errorf("the app configuration has no client for the package %s; add an Android app with this package in the Firebase project %s", Package, f.ProjectInfo.ProjectID)
}

// Pairing is the topic and the key that the phones get from the QR code.
type Pairing struct {
	Topic    string
	Key      []byte
	PairedAt time.Time
}

// NewPairing makes a random topic and a random key.
func NewPairing(now time.Time) (Pairing, error) {
	t := make([]byte, 16)
	k := make([]byte, KeySize)
	if _, err := rand.Read(t); err != nil {
		return Pairing{}, err
	}
	if _, err := rand.Read(k); err != nil {
		return Pairing{}, err
	}
	return Pairing{Topic: "i2a-" + hex.EncodeToString(t), Key: k, PairedAt: now.UTC().Truncate(time.Second)}, nil
}

// Link is the pairing link that the QR code holds.
func Link(app AppConfig, p Pairing, name string) string {
	q := url.Values{}
	q.Set("v", "1")
	q.Set("p", app.ProjectID)
	q.Set("s", app.SenderID)
	q.Set("a", app.AppID)
	q.Set("ak", app.APIKey)
	q.Set("t", p.Topic)
	q.Set("k", base64.RawURLEncoding.EncodeToString(p.Key))
	q.Set("n", name)
	return "i2a://pair?" + q.Encode()
}

// State is what the secret store holds for the phone push.
type State struct {
	ServiceAccount *ServiceAccount
	App            *AppConfig
	Pairing        *Pairing
}

// Ready reports whether the Mac can send pushes.
func (s State) Ready() bool { return s.ServiceAccount != nil && s.App != nil && s.Pairing != nil }

// Load reads the phone items from the secret store. A missing item is nil.
func Load(st secrets.Store) (State, error) {
	var s State
	get := func(key string) (string, bool, error) {
		v, err := st.Get(key)
		if errors.Is(err, secrets.ErrNotFound) {
			return "", false, nil
		}
		return v, err == nil, err
	}
	if v, ok, err := get(KeyServiceAccount); err != nil {
		return s, err
	} else if ok {
		sa, err := ParseServiceAccount([]byte(v))
		if err != nil {
			return s, fmt.Errorf("stored service account: %w", err)
		}
		s.ServiceAccount = &sa
	}
	if v, ok, err := get(KeyAppConfig); err != nil {
		return s, err
	} else if ok {
		var app AppConfig
		if err := json.Unmarshal([]byte(v), &app); err != nil {
			return s, fmt.Errorf("stored app configuration: %w", err)
		}
		s.App = &app
	}
	topic, okT, err := get(KeyTopic)
	if err != nil {
		return s, err
	}
	key, okK, err := get(KeyKey)
	if err != nil {
		return s, err
	}
	if okT && okK {
		k, err := base64.StdEncoding.DecodeString(key)
		if err != nil || len(k) != KeySize {
			return s, errors.New("the stored pairing key is not valid; run i2a phone reset")
		}
		p := Pairing{Topic: topic, Key: k}
		if at, ok, _ := get(KeyPairedAt); ok {
			p.PairedAt, _ = time.Parse(time.RFC3339, at)
		}
		s.Pairing = &p
	}
	return s, nil
}

// SaveServiceAccount stores the raw service-account file after a check.
func SaveServiceAccount(st secrets.Store, raw []byte) (ServiceAccount, error) {
	sa, err := ParseServiceAccount(raw)
	if err != nil {
		return sa, err
	}
	return sa, st.Set(KeyServiceAccount, string(raw))
}

// SaveAppConfig stores the app configuration.
func SaveAppConfig(st secrets.Store, app AppConfig) error {
	b, err := json.Marshal(app)
	if err != nil {
		return err
	}
	return st.Set(KeyAppConfig, string(b))
}

// SavePairing stores a pairing.
func SavePairing(st secrets.Store, p Pairing) error {
	for k, v := range map[string]string{
		KeyTopic: p.Topic, KeyKey: base64.StdEncoding.EncodeToString(p.Key), KeyPairedAt: p.PairedAt.Format(time.RFC3339),
	} {
		if err := st.Set(k, v); err != nil {
			return err
		}
	}
	return nil
}

// RemoveAll deletes all phone items from the secret store.
func RemoveAll(st secrets.Store) error {
	for _, k := range []string{KeyServiceAccount, KeyAppConfig, KeyTopic, KeyKey, KeyPairedAt} {
		if err := st.Delete(k); err != nil {
			return err
		}
	}
	return nil
}
