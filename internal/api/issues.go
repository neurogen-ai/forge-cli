package api

import (
	"fmt"
)

// CreateIssueInput is the POST /repos/{owner}/{repo}/issues body.
type CreateIssueInput struct {
	Title     string   `json:"title"`
	Body      string   `json:"body,omitempty"`
	Labels    []int64  `json:"labels,omitempty"` // label IDs; resolved from names by callers
	Assignees []string `json:"assignees,omitempty"`
}

// CreateIssue opens an issue.
func (c *Client) CreateIssue(owner, repo string, in CreateIssueInput) (*Issue, error) {
	var iss Issue
	path := fmt.Sprintf("/repos/%s/%s/issues", owner, repo)
	if err := c.Do("POST", path, nil, in, &iss); err != nil {
		return nil, err
	}
	return &iss, nil
}

// AddComment posts an issue comment. Forgejo uses the same collection for PR
// comments because pull requests are issue-backed.
func (c *Client) AddComment(owner, repo string, index int, body string) (*Comment, error) {
	var com Comment
	in := struct {
		Body string `json:"body"`
	}{body}
	path := fmt.Sprintf("/repos/%s/%s/issues/%d/comments", owner, repo, index)
	if err := c.Do("POST", path, nil, in, &com); err != nil {
		return nil, err
	}
	return &com, nil
}

// GetIssue fetches one issue by index.
func (c *Client) GetIssue(owner, repo string, index int) (*Issue, error) {
	var iss Issue
	path := fmt.Sprintf("/repos/%s/%s/issues/%d", owner, repo, index)
	if err := c.Do("GET", path, nil, nil, &iss); err != nil {
		return nil, err
	}
	return &iss, nil
}

// ListIssues lists issues. type=issues is always sent so pull requests are
// excluded from the result (Forgejo serves PRs through the issues endpoint).
func (c *Client) ListIssues(owner, repo, state string, page, limit int) ([]Issue, error) {
	q := PageParams(state, page, limit)
	q.Set("type", "issues")
	return List[Issue](c, fmt.Sprintf("/repos/%s/%s/issues", owner, repo), q)
}

// GetIssueComments lists the timeline comments of an issue or pull request
// (the pulls endpoint shares the issues comment collection).
func (c *Client) GetIssueComments(owner, repo string, index int) ([]Comment, error) {
	var out []Comment
	path := fmt.Sprintf("/repos/%s/%s/issues/%d/comments", owner, repo, index)
	if err := c.Do("GET", path, nil, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// EditIssueInput is the PATCH /repos/{owner}/{repo}/issues/{index} body for
// partial edits. Zero-value fields are omitted from the wire, so callers
// patch exactly the fields the user supplied.
type EditIssueInput struct {
	Title string `json:"title,omitempty"`
	Body  string `json:"body,omitempty"`
}

// EditIssue patches title/body and returns the updated issue.
func (c *Client) EditIssue(owner, repo string, index int, in EditIssueInput) (*Issue, error) {
	return c.patchIssue(owner, repo, index, in)
}

// patchIssue is the shared PATCH implementation for issue fields;
// SetIssueState delegates here and keeps its signature.
func (c *Client) patchIssue(owner, repo string, index int, fields any) (*Issue, error) {
	var iss Issue
	path := fmt.Sprintf("/repos/%s/%s/issues/%d", owner, repo, index)
	if err := c.Do("PATCH", path, nil, fields, &iss); err != nil {
		return nil, err
	}
	return &iss, nil
}

// SetIssueState opens or closes an issue via PATCH /repos/{o}/{r}/issues/{index}.
// state is "open" or "closed". Returns the updated payload.
func (c *Client) SetIssueState(owner, repo string, index int, state string) (*Issue, error) {
	return c.patchIssue(owner, repo, index, map[string]string{"state": state})
}

// LockIssue locks the conversation on issue N. PRs ride the issue
// endpoints, so pr lock uses the same call. reason is optional and
// server-validated; an empty reason sends no body at all. An instance
// without lock support returns a normal *APIError carrying the server
// message verbatim, with no special error kind.
func (c *Client) LockIssue(owner, repo string, index int, reason string) error {
	path := fmt.Sprintf("/repos/%s/%s/issues/%d/lock", owner, repo, index)
	if reason == "" {
		return c.Do("POST", path, nil, nil, nil)
	}
	return c.Do("POST", path, nil, map[string]string{"reason": reason}, nil)
}

// UnlockIssue removes the conversation lock from issue N. Success has no body.
func (c *Client) UnlockIssue(owner, repo string, index int) error {
	path := fmt.Sprintf("/repos/%s/%s/issues/%d/lock", owner, repo, index)
	return c.Do("DELETE", path, nil, nil, nil)
}

// ListLabels lists repository labels (used to resolve names to IDs).
func (c *Client) ListLabels(owner, repo string) ([]Label, error) {
	var out []Label
	path := fmt.Sprintf("/repos/%s/%s/labels", owner, repo)
	if err := c.Do("GET", path, nil, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// LabelIDsInput is the POST /repos/{owner}/{repo}/issues/{index}/labels body.
type LabelIDsInput struct {
	Labels []int64 `json:"labels"`
}

// AddLabels adds labels by repository label id and returns the server's
// label payload for the issue.
func (c *Client) AddLabels(owner, repo string, index int, ids []int64) ([]Label, error) {
	var out []Label
	path := fmt.Sprintf("/repos/%s/%s/issues/%d/labels", owner, repo, index)
	if err := c.Do("POST", path, nil, LabelIDsInput{Labels: ids}, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// RemoveLabel removes one label by id. Success has no body.
func (c *Client) RemoveLabel(owner, repo string, index int, id int64) error {
	path := fmt.Sprintf("/repos/%s/%s/issues/%d/labels/%d", owner, repo, index, id)
	return c.Do("DELETE", path, nil, nil, nil)
}
