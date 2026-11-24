// Package linear collects Linear comments and notifications with GraphQL
// queries and a personal API key. It sends no mutation, so it never marks a
// notification as read and never writes to Linear.
package linear

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// ErrUnauthorized is returned when Linear rejects the API key.
var ErrUnauthorized = errors.New("linear: the API key is rejected")

// errRateLimited marks a response that asks the client to wait.
var errRateLimited = errors.New("linear: rate limit")

// APIError is an error response of the GraphQL endpoint.
type APIError struct {
	Status  int
	Code    string // extensions.code of the first error, for example AUTHENTICATION_ERROR
	Message string
	Wait    time.Duration // the Retry-After header of a rate limit; 0 when absent
}

func (e *APIError) Error() string {
	msg := strings.TrimRight(e.Message, ". ")
	switch {
	case e.Code != "" && msg != "":
		return fmt.Sprintf("linear: HTTP %d: %s: %s", e.Status, e.Code, msg)
	case msg != "":
		return fmt.Sprintf("linear: HTTP %d: %s", e.Status, msg)
	default:
		return fmt.Sprintf("linear: HTTP %d", e.Status)
	}
}

func (e *APIError) Unwrap() error {
	switch {
	case e.Status == http.StatusUnauthorized || e.Code == "AUTHENTICATION_ERROR":
		return ErrUnauthorized
	case e.Status == http.StatusTooManyRequests || e.Code == "RATELIMITED":
		return errRateLimited
	}
	return nil
}

// Client sends GraphQL queries to one endpoint with one API key.
type Client struct {
	endpoint   string
	key        string
	http       *http.Client
	maxRetries int
	sleep      func(context.Context, time.Duration) error
	// Sent receives the body of each request; the tests read it.
	Sent func(body []byte)
}

// NewClient returns a client. A nil hc means a client with a 30 s timeout.
func NewClient(endpoint, key string, hc *http.Client) *Client {
	if hc == nil {
		hc = &http.Client{Timeout: 30 * time.Second}
	}
	return &Client{endpoint: endpoint, key: key, http: hc, maxRetries: 3, sleep: sleepCtx}
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

type gqlError struct {
	Message    string `json:"message"`
	Extensions struct {
		Code string `json:"code"`
	} `json:"extensions"`
}

// Query sends one query document with its variables and decodes the field
// data of the response into out. The document must be a query: Query refuses
// another operation. It retries a rate limit with exponential backoff, up to
// 3 times.
func (c *Client) Query(ctx context.Context, doc string, vars map[string]any, out any) error {
	if !isQuery(doc) {
		return errors.New("linear: the client sends only queries")
	}
	payload, err := json.Marshal(map[string]any{"query": doc, "variables": vars})
	if err != nil {
		return err
	}
	backoff := time.Second
	for attempt := 0; ; attempt++ {
		err := c.send(ctx, payload, out)
		var apiErr *APIError
		if errors.Is(err, errRateLimited) && errors.As(err, &apiErr) && attempt < c.maxRetries {
			wait := backoff
			if apiErr.Wait > 0 {
				wait = apiErr.Wait
			}
			if err := c.sleep(ctx, wait); err != nil {
				return err
			}
			backoff *= 2
			continue
		}
		return err
	}
}

func (c *Client) send(ctx context.Context, payload []byte, out any) error {
	if c.Sent != nil {
		c.Sent(payload)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", c.key)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("linear: %w", err)
	}
	b, readErr := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	resp.Body.Close()
	if readErr != nil {
		return readErr
	}
	var body struct {
		Data   json.RawMessage `json:"data"`
		Errors []gqlError      `json:"errors"`
	}
	decodeErr := json.Unmarshal(b, &body)
	if resp.StatusCode >= 300 || len(body.Errors) > 0 {
		apiErr := &APIError{Status: resp.StatusCode}
		if len(body.Errors) > 0 {
			apiErr.Code, apiErr.Message = body.Errors[0].Extensions.Code, body.Errors[0].Message
		}
		if s, err := strconv.Atoi(resp.Header.Get("Retry-After")); err == nil && s > 0 {
			apiErr.Wait = time.Duration(s) * time.Second
		}
		return apiErr
	}
	if decodeErr != nil {
		return fmt.Errorf("linear: decode the response: %w", decodeErr)
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(body.Data, out); err != nil {
		return fmt.Errorf("linear: decode the response: %w", err)
	}
	return nil
}

// isQuery reports whether a document has only query operations: a fragment
// or a query, never a mutation or a subscription.
func isQuery(doc string) bool {
	for _, line := range strings.Split(doc, "\n") {
		w := strings.Fields(line)
		if len(w) == 0 {
			continue
		}
		switch strings.TrimRight(w[0], "({") {
		case "mutation", "subscription":
			return false
		}
	}
	return strings.HasPrefix(strings.TrimSpace(doc), "query")
}
