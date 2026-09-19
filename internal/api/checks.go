package api

import (
	"fmt"
	"net/url"
)

// CommitStatus is one status on a commit ref. State is one of
// pending|success|error|failure|warning; Forgejo encodes the field as
// "status", not "state".
type CommitStatus struct {
	State       string `json:"status"`
	Context     string `json:"context"`
	Description string `json:"description,omitempty"`
	TargetURL   string `json:"target_url,omitempty"`
}

// ListCommitStatuses lists the statuses attached to a ref (a sha, branch,
// or tag). Statuses exist on every Forgejo version, so this transport is
// not probe-gated.
func (c *Client) ListCommitStatuses(owner, repo, ref string) ([]CommitStatus, error) {
	var out []CommitStatus
	path := fmt.Sprintf("/repos/%s/%s/commits/%s/statuses", owner, repo, url.PathEscape(ref))
	if err := c.Do("GET", path, nil, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}
