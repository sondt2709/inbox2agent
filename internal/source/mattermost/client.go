// Package mattermost collects new posts from a Mattermost server with read-only API calls.
package mattermost

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// ErrUnauthorized is returned when the server rejects the token.
var ErrUnauthorized = errors.New("the server rejected the token (HTTP 401)")

// APIError is an error response from the server.
type APIError struct {
	Status  int
	ID      string `json:"id"`
	Message string `json:"message"`
}

func (e *APIError) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("mattermost: HTTP %d: %s", e.Status, strings.TrimRight(e.Message, ". "))
	}
	return fmt.Sprintf("mattermost: HTTP %d", e.Status)
}

func (e *APIError) Unwrap() error {
	if e.Status == http.StatusUnauthorized {
		return ErrUnauthorized
	}
	return nil
}

// User is a Mattermost user.
type User struct {
	ID       string `json:"id"`
	Username string `json:"username"`
}

// Team is a Mattermost team.
type Team struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	DisplayName string `json:"display_name"`
}

// Channel is a Mattermost channel. Type is O (public), P (private), D (direct) or G (group).
type Channel struct {
	ID          string `json:"id"`
	TeamID      string `json:"team_id"`
	Type        string `json:"type"`
	Name        string `json:"name"`
	DisplayName string `json:"display_name"`
	LastPostAt  int64  `json:"last_post_at"`
	DeleteAt    int64  `json:"delete_at"`
}

// Post is a Mattermost post.
type Post struct {
	ID        string `json:"id"`
	ChannelID string `json:"channel_id"`
	UserID    string `json:"user_id"`
	RootID    string `json:"root_id"`
	Message   string `json:"message"`
	Type      string `json:"type"`
	CreateAt  int64  `json:"create_at"`
	DeleteAt  int64  `json:"delete_at"`
}

// PostList is one page of posts. Order holds the post IDs of the page, newest first.
// Posts can also hold thread roots that are not in Order.
type PostList struct {
	Order []string        `json:"order"`
	Posts map[string]Post `json:"posts"`
}

// Client is a small typed client for the Mattermost REST API v4.
type Client struct {
	base       string
	token      string
	http       *http.Client
	maxRetries int
	sleep      func(context.Context, time.Duration) error
}

// NewClient returns a client for the server URL and token.
func NewClient(serverURL, token string, hc *http.Client) *Client {
	if hc == nil {
		hc = &http.Client{Timeout: 30 * time.Second}
	}
	return &Client{
		base:       strings.TrimRight(serverURL, "/") + "/api/v4",
		token:      token,
		http:       hc,
		maxRetries: 3,
		sleep:      sleepCtx,
	}
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// do sends a request and decodes the JSON response into out.
// It retries HTTP 429 with exponential backoff, up to maxRetries times.
func (c *Client) do(ctx context.Context, method, path string, body any, out any) error {
	var payload []byte
	if body != nil {
		var err error
		if payload, err = json.Marshal(body); err != nil {
			return err
		}
	}
	backoff := time.Second
	for attempt := 0; ; attempt++ {
		var rd io.Reader
		if payload != nil {
			rd = bytes.NewReader(payload)
		}
		req, err := http.NewRequestWithContext(ctx, method, c.base+path, rd)
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+c.token)
		req.Header.Set("Accept", "application/json")
		if payload != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		resp, err := c.http.Do(req)
		if err != nil {
			return fmt.Errorf("mattermost: %s %s: %w", method, path, err)
		}
		b, readErr := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
		resp.Body.Close()
		if readErr != nil {
			return readErr
		}
		if resp.StatusCode == http.StatusTooManyRequests && attempt < c.maxRetries {
			wait := backoff
			if s, err := strconv.Atoi(resp.Header.Get("Retry-After")); err == nil && s > 0 {
				wait = time.Duration(s) * time.Second
			}
			if err := c.sleep(ctx, wait); err != nil {
				return err
			}
			backoff *= 2
			continue
		}
		if resp.StatusCode >= 300 {
			apiErr := &APIError{Status: resp.StatusCode}
			_ = json.Unmarshal(b, apiErr)
			return apiErr
		}
		if out == nil {
			return nil
		}
		if err := json.Unmarshal(b, out); err != nil {
			return fmt.Errorf("mattermost: decode %s: %w", path, err)
		}
		return nil
	}
}

// Me returns the user of the token.
func (c *Client) Me(ctx context.Context) (User, error) {
	var u User
	err := c.do(ctx, http.MethodGet, "/users/me", nil, &u)
	return u, err
}

// Teams returns the teams of the user.
func (c *Client) Teams(ctx context.Context) ([]Team, error) {
	var t []Team
	err := c.do(ctx, http.MethodGet, "/users/me/teams", nil, &t)
	return t, err
}

// Channels returns the channels of the user in a team, including direct and group channels.
func (c *Client) Channels(ctx context.Context, teamID string) ([]Channel, error) {
	var ch []Channel
	err := c.do(ctx, http.MethodGet, "/users/me/teams/"+url.PathEscape(teamID)+"/channels", nil, &ch)
	return ch, err
}

// PostsPage returns one page of posts of a channel, newest first.
func (c *Client) PostsPage(ctx context.Context, channelID string, page, perPage int) (PostList, error) {
	var pl PostList
	q := url.Values{"page": {strconv.Itoa(page)}, "per_page": {strconv.Itoa(perPage)}}
	err := c.do(ctx, http.MethodGet, "/channels/"+url.PathEscape(channelID)+"/posts?"+q.Encode(), nil, &pl)
	return pl, err
}

// UsersByIDs returns the users with these IDs.
func (c *Client) UsersByIDs(ctx context.Context, ids []string) ([]User, error) {
	var u []User
	err := c.do(ctx, http.MethodPost, "/users/ids", ids, &u)
	return u, err
}
