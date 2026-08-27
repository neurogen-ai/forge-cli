package api

import (
	"fmt"
)

// CreateIssueInput is the POST /repos/{owner}/{repo}/issues body.
type CreateIssueInput struct {
	Title     string   `json:"title"`
	Body      string   `json:"body,omitempty"`
	Labels    []int    `json:"labels,omitempty"` // label IDs; resolved from names by callers
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

// SetIssueState opens or closes an issue via PATCH /repos/{o}/{r}/issues/{index}.
// state is "open" or "closed". Returns the updated payload.
func (c *Client) SetIssueState(owner, repo string, index int, state string) (*Issue, error) {
	var iss Issue
	body := map[string]string{"state": state}
	path := fmt.Sprintf("/repos/%s/%s/issues/%d", owner, repo, index)
	if err := c.Do("PATCH", path, nil, body, &iss); err != nil {
		return nil, err
	}
	return &iss, nil
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
