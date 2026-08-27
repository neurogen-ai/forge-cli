package api

import (
	"fmt"
	"net/url"
)

// CreatePRInput is the POST /repos/{owner}/{repo}/pulls body. Base and Body
// are omitted from the JSON when empty.
type CreatePRInput struct {
	Title string `json:"title"`
	Head  string `json:"head"`
	Base  string `json:"base,omitempty"`
	Body  string `json:"body,omitempty"`
}

// CreatePullRequest opens a pull request.
func (c *Client) CreatePullRequest(owner, repo string, in CreatePRInput) (*PullRequest, error) {
	var pr PullRequest
	path := fmt.Sprintf("/repos/%s/%s/pulls", owner, repo)
	if err := c.Do("POST", path, nil, in, &pr); err != nil {
		return nil, err
	}
	return &pr, nil
}

// GetPullRequest fetches one pull request by index.
func (c *Client) GetPullRequest(owner, repo string, index int) (*PullRequest, error) {
	var pr PullRequest
	path := fmt.Sprintf("/repos/%s/%s/pulls/%d", owner, repo, index)
	if err := c.Do("GET", path, nil, nil, &pr); err != nil {
		return nil, err
	}
	return &pr, nil
}

// ListPullRequests lists pull requests; state may be "" (server default),
// "open", "closed", or "all".
func (c *Client) ListPullRequests(owner, repo, state string, page, limit int) ([]PullRequest, error) {
	q := PageParams(state, page, limit)
	return List[PullRequest](c, fmt.Sprintf("/repos/%s/%s/pulls", owner, repo), q)
}

// GetReviews lists all reviews of a pull request, following Link headers
// until exhausted (the server caps pages around 30; a truncated tail could
// hide a review's only unresolved comment).
func (c *Client) GetReviews(owner, repo string, index int) ([]Review, error) {
	path := fmt.Sprintf("/repos/%s/%s/pulls/%d/reviews", owner, repo, index)
	return List[Review](c, path, url.Values{})
}

// GetReviewComments lists all inline comments of one review, paginated like
// GetReviews.
func (c *Client) GetReviewComments(owner, repo string, index, reviewID int) ([]ReviewComment, error) {
	path := fmt.Sprintf("/repos/%s/%s/pulls/%d/reviews/%d/comments", owner, repo, index, reviewID)
	return List[ReviewComment](c, path, url.Values{})
}
