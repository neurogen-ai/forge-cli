package api

import "errors"

// RepoExists reports whether GET /repos/{o}/{r} succeeds.
//
// exists is true only on 2xx. On 404 it returns exists=false with notFound
// carrying the server's APIError so callers can quote the server's own
// message in diagnostics. Any other non-2xx status (401, 403, 5xx) or
// transport failure is returned as err so a scope problem is never mistaken
// for a missing repository.
func (c *Client) RepoExists(o, r string) (exists bool, notFound *APIError, err error) {
	err = c.Do("GET", "/repos/"+o+"/"+r, nil, nil, nil)
	if err == nil {
		return true, nil, nil
	}
	var apiErr *APIError
	if errors.As(err, &apiErr) && apiErr.Status == 404 {
		return false, apiErr, nil
	}
	return false, nil, err
}
