package api

import (
	"net/url"
)

// Notification models one entry of the notifications list. Read state stays
// server-side; nothing in this package fetches or stores more than the list
// and read endpoints return.
type Notification struct {
	ID     int64 `json:"id"`
	Unread bool  `json:"unread"`

	Subject struct {
		Title string `json:"title"`
		Type  string `json:"type"` // Issue|PullRequest|Commit|Release
		URL   string `json:"url"`
	} `json:"subject"`

	Repository Repository `json:"repository"`
}

// markReadInput is the PUT /notifications body. This encoding is the
// un-probed contract guess: scripts/probe-v0.6.0.sh has not run against a
// live instance (see scripts/probe-v0.6.0.sh.findings), so the batch shape
// {"ids":[...]} comes from plans/implementation/v0.6.0.md's foundational
// contracts. If the live probe rejects it, this type, the transport below,
// and the probe script's finding change together.
type markReadInput struct {
	IDs []int64 `json:"ids"`
}

// ListNotifications lists notifications. By default only unread entries
// come back (server-side default); all=true asks the server to include
// read ones. Listing never mutates read state.
func (c *Client) ListNotifications(all bool) ([]Notification, error) {
	q := url.Values{}
	if all {
		q.Set("all", "true")
	}
	return List[Notification](c, "/notifications", q)
}

// MarkNotificationsRead marks the given notification IDs read in one batch
// request. A server that rejects the batch shape surfaces that as a normal
// API error; there is no per-id fallback loop in the transport.
func (c *Client) MarkNotificationsRead(ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	return c.Do("PUT", "/notifications", nil, markReadInput{IDs: ids}, nil)
}
