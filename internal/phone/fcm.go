package phone

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Scope is the OAuth scope of the FCM HTTP v1 API.
const Scope = "https://www.googleapis.com/auth/firebase.messaging"

// TTL is the time that FCM keeps a message for a phone that is offline.
const TTL = 7 * 24 * time.Hour

// Sender sends sealed messages to the topic of a pairing with FCM HTTP v1.
// It gets one access token and uses it for all messages.
type Sender struct {
	SA      ServiceAccount
	Topic   string
	Key     []byte
	FCMBase string // "https://fcm.googleapis.com"; tests set an httptest URL
	Client  *http.Client
	Now     func() time.Time

	token string
}

// NewSender returns a sender for a ready state.
func NewSender(s State) *Sender {
	return &Sender{SA: *s.ServiceAccount, Topic: s.Pairing.Topic, Key: s.Pairing.Key,
		FCMBase: "https://fcm.googleapis.com", Client: &http.Client{Timeout: 10 * time.Second}, Now: time.Now}
}

// Send seals p and sends it. It changes p when the card must be cut.
func (s *Sender) Send(ctx context.Context, p *Payload) error {
	p.V = Version
	if p.TS == 0 {
		p.TS = s.Now().Unix()
	}
	c, err := SealFit(s.Key, p)
	if err != nil {
		return err
	}
	if s.token == "" {
		if s.token, err = s.accessToken(ctx); err != nil {
			return err
		}
	}
	priority := "normal"
	if p.Alert {
		priority = "high"
	}
	msg := map[string]any{"message": map[string]any{
		"topic": s.Topic,
		"data":  map[string]string{"v": fmt.Sprint(Version), "c": c},
		"android": map[string]any{
			"priority": priority,
			"ttl":      fmt.Sprintf("%ds", int(TTL.Seconds())),
		},
	}}
	body, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	u := strings.TrimSuffix(s.FCMBase, "/") + "/v1/projects/" + s.SA.ProjectID + "/messages:send"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+s.token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.Client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("FCM returned HTTP %d: %s", resp.StatusCode, short(resp.Body))
	}
	return nil
}

// accessToken signs a JWT with the service-account key (RS256) and exchanges it
// at token_uri (RFC 7523).
func (s *Sender) accessToken(ctx context.Context) (string, error) {
	if s.SA.key == nil {
		return "", errors.New("the service account has no parsed key")
	}
	now := s.Now()
	enc := base64.RawURLEncoding
	head, _ := json.Marshal(map[string]string{"alg": "RS256", "typ": "JWT"})
	claims, _ := json.Marshal(map[string]any{
		"iss": s.SA.ClientEmail, "scope": Scope, "aud": s.SA.TokenURI,
		"iat": now.Unix(), "exp": now.Add(time.Hour).Unix(),
	})
	signing := enc.EncodeToString(head) + "." + enc.EncodeToString(claims)
	sum := sha256.Sum256([]byte(signing))
	sig, err := rsa.SignPKCS1v15(rand.Reader, s.SA.key, crypto.SHA256, sum[:])
	if err != nil {
		return "", err
	}
	form := "grant_type=" + "urn%3Aietf%3Aparams%3Aoauth%3Agrant-type%3Ajwt-bearer" + "&assertion=" + signing + "." + enc.EncodeToString(sig)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.SA.TokenURI, strings.NewReader(form))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := s.Client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("the token endpoint returned HTTP %d: %s", resp.StatusCode, short(resp.Body))
	}
	var tok struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tok); err != nil || tok.AccessToken == "" {
		return "", errors.New("the token endpoint returned no access_token")
	}
	return tok.AccessToken, nil
}

// short returns the start of an error body on one line.
func short(r io.Reader) string {
	b, _ := io.ReadAll(io.LimitReader(r, 300))
	return strings.Join(strings.Fields(string(b)), " ")
}
