package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// maxPages caps List so a server advertising a self-referential rel="next"
// cannot loop forever.
const maxPages = 50

// PageParams builds the common list-query values. state is omitted when "",
// page and limit are omitted when <= 0.
func PageParams(state string, page, limit int) url.Values {
	q := url.Values{}
	if state != "" {
		q.Set("state", state)
	}
	if page > 0 {
		q.Set("page", strconv.Itoa(page))
	}
	if limit > 0 {
		q.Set("limit", strconv.Itoa(limit))
	}
	return q
}

// NextLink extracts the URL of the rel="next" target from an RFC 5988 Link
// response header. It reports false when no next link is present.
func NextLink(resp *http.Response) (string, bool) {
	header := resp.Header.Get("Link")
	if header == "" {
		return "", false
	}
	for _, field := range strings.Split(header, ",") {
		var link, rel string
		for _, part := range strings.Split(field, ";") {
			part = strings.TrimSpace(part)
			switch {
			case strings.HasPrefix(part, "<") && strings.HasSuffix(part, ">"):
				link = strings.Trim(part, "<>")
			case strings.HasPrefix(part, "rel=\""):
				rel = strings.Trim(strings.TrimPrefix(part, "rel=\""), "\"")
			case strings.HasPrefix(part, "rel="):
				rel = strings.TrimPrefix(part, "rel=")
			}
		}
		if rel == "next" && link != "" {
			return link, true
		}
	}
	return "", false
}

// List fetches startPath and follows rel="next" links until the server stops
// advertising them, returning every item in order. It fails after maxPages
// consecutive fetches to guard against pagination loops.
func List[T any](c *Client, startPath string, q url.Values) ([]T, error) {
	next := c.pageURL(startPath, q)
	var all []T
	for range maxPages {
		var items []T
		resp, err := c.getPage(next, &items)
		if err != nil {
			return nil, err
		}
		all = append(all, items...)

		link, ok := NextLink(resp)
		if !ok {
			return all, nil
		}
		next = link
	}
	return nil, fmt.Errorf("api: pagination did not terminate after %d pages", maxPages)
}

// pageURL joins the client base URL, the fixed /api/v1 prefix, an endpoint
// path, and optional query values into one absolute URL.
func (c *Client) pageURL(path string, q url.Values) string {
	full := c.baseURL + "/api/v1" + path
	if len(q) > 0 {
		full += "?" + q.Encode()
	}
	return full
}

// getPage performs one authenticated GET against an absolute URL, decoding a
// 2xx body into out. The returned Response has its body consumed and closed;
// its header remains readable for NextLink. Non-2xx yields *APIError and
// transport failures yield errors matching IsNetwork, mirroring Do.
func (c *Client) getPage(fullURL string, out any) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodGet, fullURL, nil)
	if err != nil {
		return nil, fmt.Errorf("api: build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", "token "+c.token)
	}

	start := time.Now()
	resp, err := c.http.Do(req)
	latency := time.Since(start)
	if err != nil {
		c.logf("GET %s -> transport error (%s)", fullURL, latency.Round(time.Millisecond))
		return nil, &networkError{err: fmt.Errorf("api: GET %s: %w", fullURL, err)}
	}
	defer resp.Body.Close()

	c.logf("GET %s -> %d (%s)", fullURL, resp.StatusCode, latency.Round(time.Millisecond))

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp, fmt.Errorf("api: read response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return resp, apiErrorFrom(resp.StatusCode, data)
	}
	if out != nil && len(data) > 0 {
		if err := json.Unmarshal(data, out); err != nil {
			return resp, fmt.Errorf("api: decode response: %w", err)
		}
	}
	return resp, nil
}
