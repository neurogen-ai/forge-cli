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

// SubmitReviewInput is the body for POST /repos/{owner}/{repo}/pulls/{index}/reviews.
// Event is APPROVED, REQUEST_CHANGES, or COMMENT. CLI spelling validation lives
// in internal/cmds.
type SubmitReviewInput struct {
	Event string `json:"event"`
	Body  string `json:"body,omitempty"`
}

// SubmitReview posts one pull-request review and decodes the created review.
func (c *Client) SubmitReview(owner, repo string, index int, in SubmitReviewInput) (*Review, error) {
	var rev Review
	path := fmt.Sprintf("/repos/%s/%s/pulls/%d/reviews", owner, repo, index)
	if err := c.Do("POST", path, nil, in, &rev); err != nil {
		return nil, err
	}
	return &rev, nil
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

// patchPull is the shared PATCH implementation for PR state fields.
// Future PR edit fields can reuse this endpoint without duplicating
// request construction or response decoding.
func (c *Client) patchPull(owner, repo string, index int, fields any) (*PullRequest, error) {
	var pr PullRequest
	path := fmt.Sprintf("/repos/%s/%s/pulls/%d", owner, repo, index)
	if err := c.Do("PATCH", path, nil, fields, &pr); err != nil {
		return nil, err
	}
	return &pr, nil
}

// SetPRState opens or closes one pull request. state is "open" or "closed".
// The server response is the updated pull request.
func (c *Client) SetPRState(owner, repo string, index int, state string) (*PullRequest, error) {
	return c.patchPull(owner, repo, index, map[string]string{"state": state})
}

// SetPRDraft clears the draft flag and returns the updated pull request.
func (c *Client) SetPRDraft(owner, repo string, index int) (*PullRequest, error) {
	return c.patchPull(owner, repo, index, map[string]any{"draft": false})
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

// GetPullDiff fetches the raw .diff or .patch representation of a PR.
// format is selected by the command and is exactly "diff" or "patch".
func (c *Client) GetPullDiff(owner, repo string, index int, format string) (*RawResponse, error) {
	path := fmt.Sprintf("/repos/%s/%s/pulls/%d.%s", owner, repo, index, format)
	return c.DoRaw("GET", path, nil, nil)
}

// Thread-resolution encoding pinned by decision D2. The probe script at
// scripts/probe-v0.3.0.sh verifies this shape against a live instance; a
// mismatch means editing exactly this block plus TestThreadResolution.
const threadResolutionMethod = "PATCH"

// ResolveThread marks the review-comment thread rooted at commentID
// resolved. UnresolveThread clears it. Both target ROOT comment ids; reply
// ids yield server errors surfaced verbatim by callers. Re-resolving is
// idempotent server-side and stays safe to retry.
func (c *Client) ResolveThread(owner, repo string, commentID int64) error {
	return c.setThreadResolution(owner, repo, commentID, true)
}

func (c *Client) UnresolveThread(owner, repo string, commentID int64) error {
	return c.setThreadResolution(owner, repo, commentID, false)
}

func (c *Client) setThreadResolution(owner, repo string, commentID int64, resolved bool) error {
	path := fmt.Sprintf("/repos/%s/%s/pulls/comments/%d/resolve", owner, repo, commentID)
	body := struct {
		Resolved bool `json:"resolved"`
	}{resolved}
	return c.Do(threadResolutionMethod, path, nil, body, nil)
}
