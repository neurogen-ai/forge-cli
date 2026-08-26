package api

import "errors"

// Probe endpoints used by the wiring preflight to pinpoint which layer of
// host/token/owner/repo is wrong when something 404s. Each one is a cheap
// single GET; none of them log or expose the token.

// ServerInfo verifies that the host answers and speaks the Forgejo/Gitea API.
// It returns nil on any 2xx from GET /api/v1/version and propagates the error
// otherwise (network failure, or an *APIError when the server is not Forgejo).
func (c *Client) ServerInfo() error {
	var out struct {
		Version string `json:"version"`
	}
	return c.Do("GET", "/version", nil, nil, &out)
}

// CurrentUser verifies that the token is valid and readable. It returns the
// authenticated user on success; a 401/403 surfaces as *APIError.
func (c *Client) CurrentUser() (*User, error) {
	u := &User{}
	if err := c.Do("GET", "/user", nil, nil, u); err != nil {
		return nil, err
	}
	return u, nil
}

// OwnerExists reports whether GET /users/{o} resolves. It returns false only
// for 404 (unknown owner); auth failures and other statuses are errors so a
// scope problem is never mistaken for a missing owner.
func (c *Client) OwnerExists(o string) (bool, error) {
	err := c.Do("GET", "/users/"+o, nil, nil, nil)
	if err == nil {
		return true, nil
	}
	var apiErr *APIError
	if errors.As(err, &apiErr) && apiErr.Status == 404 {
		return false, nil
	}
	return false, err
}
