package api

import (
	"errors"
	"net/url"
)

// BranchExists reports whether GET /repos/{o}/{r}/branches?branch={b}
// resolves. The branch is sent as a query parameter on the collection path,
// never in the URL path, because branch names may contain slashes.
//
// It returns false only for 404 (unknown branch); auth failures and other
// statuses are errors so a scope problem is never mistaken for a missing
// branch. Shape mirrors OwnerExists in repo.go.
func (c *Client) BranchExists(o, r, b string) (bool, error) {
	q := url.Values{"branch": {b}}
	err := c.Do("GET", "/repos/"+o+"/"+r+"/branches", q, nil, nil)
	if err == nil {
		return true, nil
	}
	var apiErr *APIError
	if errors.As(err, &apiErr) && apiErr.Status == 404 {
		return false, nil
	}
	return false, err
}
