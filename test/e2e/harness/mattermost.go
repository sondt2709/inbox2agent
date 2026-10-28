package harness

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

// Password is the password of the fixture users. The server is local and temporary.
const Password = "Passw0rd!e2e"

// Mattermost is the Docker server from test/e2e/mattermost/compose.yaml.
type Mattermost struct {
	URL string
}

// MattermostFromEnv returns the server at I2A_E2E_MM_URL, or the default port.
func MattermostFromEnv() *Mattermost {
	u := os.Getenv("I2A_E2E_MM_URL")
	if u == "" {
		u = "http://localhost:18065"
	}
	return &Mattermost{URL: u}
}

// WaitReady waits until the server answers the ping.
func (m *Mattermost) WaitReady(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		resp, err := http.Get(m.URL + "/api/v4/system/ping")
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == 200 {
				return nil
			}
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("mattermost at %s is not ready after %s: %v", m.URL, timeout, err)
		}
		time.Sleep(2 * time.Second)
	}
}

func (m *Mattermost) call(t *testing.T, token, method, path string, body any, out any) http.Header {
	t.Helper()
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, m.URL+"/api/v4"+path, rd)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		t.Fatalf("%s %s: HTTP %d: %s", method, path, resp.StatusCode, b)
	}
	if out != nil {
		if err := json.Unmarshal(b, out); err != nil {
			t.Fatalf("%s %s: decode: %v", method, path, err)
		}
	}
	return resp.Header
}

// User is a fixture user with a session token.
type User struct {
	ID, Name, Session string
}

// World is the data of one test run: two new users, a team, two channels and a direct channel.
type World struct {
	MM                      *Mattermost
	Alice, Bob              User
	TeamID, TeamName        string
	Selected, Other, Direct string // channel IDs
	AlicePAT                string
}

func suffix() string {
	b := make([]byte, 3)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// AdminName is the first user on a new server, so the server makes it a system admin.
const AdminName = "e2e-admin"

// admin creates or logs in the admin user and checks that it is a system admin.
func (m *Mattermost) admin(t *testing.T) string {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"email": AdminName + "@example.com", "username": AdminName, "password": Password})
	if resp, err := http.Post(m.URL+"/api/v4/users", "application/json", bytes.NewReader(body)); err == nil {
		resp.Body.Close() // fails with HTTP 400 when the user exists
	}
	h := m.call(t, "", "POST", "/users/login", map[string]string{"login_id": AdminName, "password": Password}, nil)
	token := h.Get("Token")
	var me struct {
		Roles string `json:"roles"`
	}
	m.call(t, token, "GET", "/users/me", nil, &me)
	if !strings.Contains(me.Roles, "system_admin") {
		t.Fatalf("%s is not a system admin (roles %q). It must be the first user on the server. "+
			"Recreate the server: docker compose -f test/e2e/mattermost/compose.yaml down -v && docker compose -f test/e2e/mattermost/compose.yaml up -d",
			AdminName, me.Roles)
	}
	return token
}

// NewWorld creates new users and channels, so each run starts with no old posts.
func (m *Mattermost) NewWorld(t *testing.T) *World {
	t.Helper()
	admin := m.admin(t)
	s := suffix()
	w := &World{MM: m, TeamName: "e2e" + s}
	mk := func(name string) User {
		var u struct {
			ID string `json:"id"`
		}
		m.call(t, "", "POST", "/users", map[string]string{"email": name + "@example.com", "username": name, "password": Password}, &u)
		h := m.call(t, "", "POST", "/users/login", map[string]string{"login_id": name, "password": Password}, nil)
		return User{ID: u.ID, Name: name, Session: h.Get("Token")}
	}
	w.Alice, w.Bob = mk("alice-"+s), mk("bob-"+s)
	var team struct {
		ID string `json:"id"`
	}
	m.call(t, w.Alice.Session, "POST", "/teams", map[string]string{"name": w.TeamName, "display_name": "E2E " + s, "type": "O"}, &team)
	w.TeamID = team.ID
	m.call(t, w.Alice.Session, "POST", "/teams/"+w.TeamID+"/members", map[string]string{"team_id": w.TeamID, "user_id": w.Bob.ID}, nil)
	ch := func(name string) string {
		var c struct {
			ID string `json:"id"`
		}
		m.call(t, w.Alice.Session, "POST", "/channels", map[string]string{"team_id": w.TeamID, "name": name, "display_name": name, "type": "O"}, &c)
		m.call(t, w.Alice.Session, "POST", "/channels/"+c.ID+"/members", map[string]string{"user_id": w.Bob.ID}, nil)
		return c.ID
	}
	w.Selected, w.Other = ch("selected"), ch("other")
	var dm struct {
		ID string `json:"id"`
	}
	m.call(t, w.Bob.Session, "POST", "/channels/direct", []string{w.Bob.ID, w.Alice.ID}, &dm)
	w.Direct = dm.ID
	// Personal access tokens need the user access token role.
	m.call(t, admin, "PUT", "/users/"+w.Alice.ID+"/roles", map[string]string{"roles": "system_user system_user_access_token"}, nil)
	var pat struct {
		Token string `json:"token"`
	}
	m.call(t, w.Alice.Session, "POST", "/users/"+w.Alice.ID+"/tokens", map[string]string{"description": "i2a e2e"}, &pat)
	w.AlicePAT = pat.Token
	return w
}

// Post writes a post as the user and returns its ID.
func (w *World) Post(t *testing.T, as User, channelID, text string) string {
	t.Helper()
	var p struct {
		ID string `json:"id"`
	}
	w.MM.call(t, as.Session, "POST", "/posts", map[string]string{"channel_id": channelID, "message": text}, &p)
	time.Sleep(20 * time.Millisecond) // keep create_at values apart
	return p.ID
}

// Unread is the unread message and mention count of a user in the team.
type Unread struct {
	Messages, Mentions int
}

// UnreadOf reads the unread counts of the user in the world team.
func (w *World) UnreadOf(t *testing.T, u User) Unread {
	t.Helper()
	var teams []struct {
		TeamID       string `json:"team_id"`
		MsgCount     int    `json:"msg_count"`
		MentionCount int    `json:"mention_count"`
	}
	w.MM.call(t, u.Session, "GET", "/users/me/teams/unread", nil, &teams)
	for _, tm := range teams {
		if tm.TeamID == w.TeamID {
			return Unread{Messages: tm.MsgCount, Mentions: tm.MentionCount}
		}
	}
	return Unread{}
}
