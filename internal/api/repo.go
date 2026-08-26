package api

import "errors"

// RepoExists reports whether GET /repos/{o}/{r} succeeds. It returns false
// specifically for 404; any other non-2xx status is returned as an error so
// callers do not mistake auth failures or server errors for a missing repo.
func (c *Client) RepoExists(o, r string) (bool, error) {
	err := c.Do("GET", "/repos/"+o+"/"+r, nil, nil, nil)
	if err == nil {
		return true, nil
	}
	var apiErr *APIError
	if errors.As(err, &apiErr) && apiErr.Status == 404 {
		return false, nil
	}
	return false, err
}
