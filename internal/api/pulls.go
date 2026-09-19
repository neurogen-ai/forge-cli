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

// ReviewCommentInput is one inline comment entry in a review comment payload.
// The encoding is the documented Gitea/Forgejo shape (plan contract skeleton,
// probe candidate "line-num"); anchorToWire in cmds is the only writer.
// NOTE(owner decision, F2): the F1 probe could not run against a live
// instance, so this encoding ships unconfirmed; see plans/releases/v0.5.0.md.
type ReviewCommentInput struct {
	Path       string `json:"path"`
	Body       string `json:"body"`
	OldLineNum int64  `json:"old_line_num,omitempty"`
	NewLineNum int64  `json:"new_line_num,omitempty"`
}

// CreateAnchoredCommentInput posts one COMMENT review whose content is a
// single inline comment entry. Event is fixed to COMMENT here so callers
// cannot post APPROVED/REQUEST_CHANGES through the comment transport.
type CreateAnchoredCommentInput struct {
	Body     string               `json:"body"`
	Comments []ReviewCommentInput `json:"comments"`
}

// reviewCommentWire is the request body for the anchored-comment transport:
// one COMMENT review carrying inline comment entries. Separate from
// SubmitReviewInput because the comments field only exists on this shape.
type reviewCommentWire struct {
	Event    string               `json:"event"`
	Body     string               `json:"body,omitempty"`
	Comments []ReviewCommentInput `json:"comments"`
}

// reviewWithComments decodes the POST /reviews response, which is a review
// whose comments array carries the created inline entries on servers that
// echo them back.
type reviewWithComments struct {
	Review
	Comments []ReviewComment `json:"comments"`
}

// CreateAnchoredComment posts one COMMENT review carrying a single inline
// comment entry and returns the created review comment.
func (c *Client) CreateAnchoredComment(owner, repo string, index int, in CreateAnchoredCommentInput) (*ReviewComment, error) {
	var resp reviewWithComments
	path := fmt.Sprintf("/repos/%s/%s/pulls/%d/reviews", owner, repo, index)
	body := reviewCommentWire{Event: "COMMENT", Body: in.Body, Comments: in.Comments}
	if err := c.Do("POST", path, nil, body, &resp); err != nil {
		return nil, err
	}
	if len(resp.Comments) > 0 {
		return &resp.Comments[0], nil
	}
	// Some servers return the review without echoing the comments array;
	// rebuild the entry from what was asked for so receipts stay honest.
	in0 := ReviewCommentInput{}
	if len(in.Comments) > 0 {
		in0 = in.Comments[0]
	}
	return &ReviewComment{ID: resp.ID, Body: in0.Body, Path: in0.Path}, nil
}

// AnchorToWire maps --file/--line/--side(old|new) onto the wire encoding the
// contract documents. A side of "" means the new line. It is the only place
// the wire encoding lives, so gh-shaped flags stay put across encoding drift.
func AnchorToWire(file string, line int64, side string) (ReviewCommentInput, error) {
	if file == "" {
		return ReviewCommentInput{}, fmt.Errorf("--file is required")
	}
	if line <= 0 {
		return ReviewCommentInput{}, fmt.Errorf("--line must be a positive line number")
	}
	in := ReviewCommentInput{Path: file, Body: ""}
	switch side {
	case "", "new":
		in.NewLineNum = line
	case "old":
		in.OldLineNum = line
	default:
		return ReviewCommentInput{}, fmt.Errorf("--side must be old or new, not %q", side)
	}
	return in, nil
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

// ListOpenPullRequestsOnePage returns the first page of open pull requests
// without following pagination links. It backs pr sync-status, whose
// contract is exactly one API request per invocation; a PR beyond the
// server's first page is a no-pr answer, not a second request.
func (c *Client) ListOpenPullRequestsOnePage(owner, repo string) ([]PullRequest, error) {
	q := url.Values{"state": {"open"}}
	path := c.pageURL(fmt.Sprintf("/repos/%s/%s/pulls", owner, repo), q)
	var prs []PullRequest
	if _, err := c.getPage(path, &prs); err != nil {
		return nil, err
	}
	return prs, nil
}

// EditPullInput is the PATCH /repos/{owner}/{repo}/pulls/{index} body for
// partial edits. Zero-value fields are omitted from the wire, so callers
// patch exactly the fields the user supplied.
type EditPullInput struct {
	Title string `json:"title,omitempty"`
	Body  string `json:"body,omitempty"`
}

// EditPullRequest patches title/body and returns the updated pull request.
func (c *Client) EditPullRequest(owner, repo string, index int, in EditPullInput) (*PullRequest, error) {
	return c.patchPull(owner, repo, index, in)
}

// EditPullRequestFields patches arbitrary PR fields and returns the updated
// pull request. It backs pr edit's explicit clearing, which sends empty
// strings the omitempty tags on EditPullInput would drop.
func (c *Client) EditPullRequestFields(owner, repo string, index int, fields any) (*PullRequest, error) {
	return c.patchPull(owner, repo, index, fields)
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

// reviewersInput is the body for the dedicated reviewer endpoints.
// Encoding note: no live probe ran against these endpoints
// (scripts/probe-v0.6.0.sh.findings records the gap); the
// {"reviewers":[...]} shape is the contract guess pending a probe.
type reviewersInput struct {
	Reviewers []string `json:"reviewers"`
}

// RequestReviewers adds review requests on pull request N via
// POST /repos/{owner}/{repo}/pulls/{index}/requested_reviewers and returns
// the updated pull request, so the receipt carries the server's view of the
// request list.
func (c *Client) RequestReviewers(owner, repo string, index int, users []string) (*PullRequest, error) {
	return c.reviewers(owner, repo, index, "POST", users)
}

// RemoveReviewers removes review requests on pull request N via
// DELETE /repos/{owner}/{repo}/pulls/{index}/requested_reviewers and returns
// the updated pull request.
func (c *Client) RemoveReviewers(owner, repo string, index int, users []string) (*PullRequest, error) {
	return c.reviewers(owner, repo, index, "DELETE", users)
}

// reviewers is the shared transport for both reviewer endpoints; only the
// method differs.
func (c *Client) reviewers(owner, repo string, index int, method string, users []string) (*PullRequest, error) {
	var pr PullRequest
	path := fmt.Sprintf("/repos/%s/%s/pulls/%d/requested_reviewers", owner, repo, index)
	if err := c.Do(method, path, nil, reviewersInput{Reviewers: users}, &pr); err != nil {
		return nil, err
	}
	return &pr, nil
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

// MergeInput is the Forgejo merge request body. The capitalized JSON field
// names are part of the Forgejo API contract. Strategy values are lower-case
// "merge", "squash", and "rebase"; CLI validation lives in internal/cmds.
type MergeInput struct {
	Do                string `json:"Do"`
	MergeTitleField   string `json:"MergeTitleField,omitempty"`
	MergeMessageField string `json:"MergeMessageField,omitempty"`
}

// MergePull submits one merge request. A successful response body is ignored;
// the caller owns any post-merge cleanup.
func (c *Client) MergePull(owner, repo string, index int, in MergeInput) error {
	path := fmt.Sprintf("/repos/%s/%s/pulls/%d/merge", owner, repo, index)
	return c.Do("POST", path, nil, in, nil)
}

// DeleteRef deletes a branch name below refs/heads after a successful merge.
// ref is the server-provided branch name, not a locally guessed one.
func (c *Client) DeleteRef(owner, repo, ref string) error {
	path := fmt.Sprintf("/repos/%s/%s/git/refs/heads/%s", owner, repo, ref)
	return c.Do("DELETE", path, nil, nil, nil)
}
