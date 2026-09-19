package api

import (
	"fmt"
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

// MarkNotificationsRead marks the given notification IDs read: one batch
// PUT /notifications with {"ids":[...]} when the server accepts it, and
// sequential per-id PUT /notifications/threads/{id} otherwise (Branch H3:
// batch first, sequentially otherwise). Both encodings are the un-probed
// contract guess; scripts/probe-v0.6.0.sh.findings records the amendment
// path if the live probe rejects either shape.
func (c *Client) MarkNotificationsRead(ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	err := c.Do("PUT", "/notifications", nil, markReadInput{IDs: ids}, nil)
	_, isAPIErr := err.(*APIError)
	if err != nil && !isAPIErr {
		// Transport-level failure, not a server rejection: nothing tells us
		// the batch shape is wrong, so surface it instead of retrying.
		return err
	}
	if err != nil {
		// The server rejected the batch shape; mark each ID read in its own
		// request instead.
		for _, id := range ids {
			if err := c.Do("PUT", fmt.Sprintf("/notifications/threads/%d", id), nil, nil, nil); err != nil {
				return err
			}
		}
	}
	return nil
}
