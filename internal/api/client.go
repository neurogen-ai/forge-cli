// Package api implements the Forgejo HTTP client used by all forge commands.
package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// Logger receives verbose request/response lines. Implementations must never
// be handed header values.
type Logger interface {
	Logf(format string, args ...any)
}

// Client talks to a Forgejo instance's /api/v1 surface.
type Client struct {
	http    *http.Client
	baseURL string // no trailing slash, no /api/v1 suffix
	token   string
	log     Logger
}

// NewClient builds a client for baseURL like "https://git.example.com".
// The /api/v1 prefix is appended by Do, never by callers.
func NewClient(baseURL, token string, timeout time.Duration, log Logger) *Client {
	return &Client{
		http:    &http.Client{Timeout: timeout},
		baseURL: trimTrailingSlash(baseURL),
		token:   token,
		log:     log,
	}
}

func trimTrailingSlash(s string) string {
	for len(s) > 0 && s[len(s)-1] == '/' {
		s = s[:len(s)-1]
	}
	return s
}

// networkError tags transport-level failures so callers can map them to
// cli.ExitNetwork without string matching.
type networkError struct{ err error }

func (e *networkError) Error() string { return e.err.Error() }
func (e *networkError) Unwrap() error { return e.err }

// IsNetwork reports whether err originated from a transport failure
// (DNS, connection refused, timeout) rather than an API error response.
func IsNetwork(err error) bool {
	var ne *networkError
	return errors.As(err, &ne)
}

// Do performs one API call. path starts with "/" and is appended to
// <baseURL>/api/v1. query may be nil. When body is non-nil it is encoded as
// JSON; when out is non-nil a 2xx response body is decoded into it.
// Non-2xx responses return *APIError. Transport failures return an error for
// which IsNetwork is true.
func (c *Client) Do(method, path string, query url.Values, body any, out any) error {
	full := c.baseURL + "/api/v1" + path
	if len(query) > 0 {
		full += "?" + query.Encode()
	}

	var payload io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("api: encode request body: %w", err)
		}
		payload = bytes.NewReader(encoded)
	}

	req, err := http.NewRequest(method, full, payload)
	if err != nil {
		return fmt.Errorf("api: build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.token != "" {
		req.Header.Set("Authorization", "token "+c.token)
	}

	start := time.Now()
	resp, err := c.http.Do(req)
	latency := time.Since(start)
	if err != nil {
		c.logf("%s %s -> transport error (%s)", method, full, latency.Round(time.Millisecond))
		return &networkError{err: fmt.Errorf("api: %s %s: %w", method, path, err)}
	}
	defer resp.Body.Close()

	// Log only method, URL, status, latency. Header values (including
	// Authorization) are deliberately never formatted into this line.
	c.logf("%s %s -> %d (%s)", method, full, resp.StatusCode, latency.Round(time.Millisecond))

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("api: read response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return apiErrorFrom(resp.StatusCode, data)
	}

	if out != nil && len(bytes.TrimSpace(data)) > 0 {
		if err := json.Unmarshal(data, out); err != nil {
			return fmt.Errorf("api: decode response: %w", err)
		}
	}
	return nil
}

func apiErrorFrom(status int, body []byte) *APIError {
	msg := http.StatusText(status)
	var decoded struct {
		Message string `json:"message"`
	}
	if json.Unmarshal(body, &decoded) == nil && decoded.Message != "" {
		msg = decoded.Message
	}
	return &APIError{Status: status, Message: msg}
}

func (c *Client) logf(format string, args ...any) {
	if c.log != nil {
		c.log.Logf(format, args...)
	}
}
