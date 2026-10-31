package daemon

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"
)

// ErrNoDaemon means that no daemon answers on the socket.
var ErrNoDaemon = errors.New("no i2a daemon answers")

// Client calls the API of the daemon of a data directory.
type Client struct {
	Socket string
	http   *http.Client
}

// NewClient returns a client for the daemon of the data directory.
func NewClient(dataDir string) *Client {
	socket := FindSocket(dataDir)
	d := net.Dialer{Timeout: 2 * time.Second}
	return &Client{Socket: socket, http: &http.Client{Transport: &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return d.DialContext(ctx, "unix", socket)
		},
	}}}
}

func (c *Client) do(ctx context.Context, method, path string, body, out any) error {
	var rd *bytes.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rd = bytes.NewReader(b)
	} else {
		rd = bytes.NewReader(nil)
	}
	req, err := http.NewRequestWithContext(ctx, method, "http://i2a"+path, rd)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("%w: %v", ErrNoDaemon, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		var e struct {
			Error string `json:"error"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&e)
		return fmt.Errorf("the daemon: %s", orText(e.Error, resp.Status))
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func orText(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// Status returns the status of the daemon, or ErrNoDaemon.
func (c *Client) Status(ctx context.Context) (Status, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	var st Status
	err := c.do(ctx, http.MethodGet, "/v1/status", nil, &st)
	return st, err
}

// Run asks the daemon to read all accounts and send the spaces now. No
// spaces means all spaces.
func (c *Client) Run(ctx context.Context, spaces []string) (RunResult, error) {
	var res RunResult
	err := c.do(ctx, http.MethodPost, "/v1/run", RunRequest{Spaces: spaces}, &res)
	return res, err
}

// Events reads the events of the daemon until ctx ends or the daemon stops.
// The channel closes then.
func (c *Client) Events(ctx context.Context) (<-chan Event, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://i2a/v1/events", nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Do(req) //nolint:bodyclose // the goroutine below closes the body of the stream
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrNoDaemon, err)
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("the daemon: %s", resp.Status)
	}
	out := make(chan Event, 16)
	go func() {
		defer close(out)
		defer resp.Body.Close()
		sc := bufio.NewScanner(resp.Body)
		for sc.Scan() {
			data, ok := strings.CutPrefix(sc.Text(), "data: ")
			if !ok {
				continue
			}
			var ev Event
			if json.Unmarshal([]byte(data), &ev) != nil {
				continue
			}
			select {
			case out <- ev:
			case <-ctx.Done():
				return
			}
		}
	}()
	return out, nil
}
