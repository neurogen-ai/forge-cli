package api

import (
	"fmt"
	"net/url"
)

// SearchIssues searches one repository's issues or pull requests over
// GET /repos/{owner}/{repo}/issues/search. kind is "issues" or "pulls" and
// becomes the query's type parameter; Forgejo returns issue-shaped payloads
// for both, so one decode serves both kinds. state is sent only when
// non-empty. page and limit ride the shared PageParams rules (omitted when
// <= 0); pagination follows the shared List helper's Link-header path.
func (c *Client) SearchIssues(owner, repo, q, kind, state string, page, limit int) ([]Issue, error) {
	query := url.Values{}
	query.Set("q", q)
	query.Set("type", kind)
	if state != "" {
		query.Set("state", state)
	}
	for k, vs := range PageParams("", page, limit) {
		query[k] = vs
	}
	return List[Issue](c, fmt.Sprintf("/repos/%s/%s/issues/search", owner, repo), query)
}
