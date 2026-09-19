package api

import (
	"errors"
	"fmt"
	"net/url"
)

// CommitStatus is one status on a commit ref. State is one of
// pending|success|error|failure|warning; Forgejo encodes the field as
// "status", not "state".
type CommitStatus struct {
	State       string `json:"status"`
	Context     string `json:"context"`
	Description string `json:"description,omitempty"`
	TargetURL   string `json:"target_url,omitempty"`
}

// ListCommitStatuses lists the statuses attached to a ref (a sha, branch,
// or tag). Statuses exist on every Forgejo version, so this transport is
// not probe-gated.
func (c *Client) ListCommitStatuses(owner, repo, ref string) ([]CommitStatus, error) {
	var out []CommitStatus
	path := fmt.Sprintf("/repos/%s/%s/commits/%s/statuses", owner, repo, url.PathEscape(ref))
	if err := c.Do("GET", path, nil, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// ActionRun is one Actions run for a ref, decoded from the runs listing
// payload. The subset rule applies: only the fields the pr checks table
// renders. Status is the server's run status string (for example
// success|failure|running); Forgejo/Gitea encode it as "status".
type ActionRun struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	Status     string `json:"status"`
	HeadSHA    string `json:"head_sha"`
	HeadBranch string `json:"head_branch"`
	URL        string `json:"url"`
}

// runsList is the envelope the runs listing endpoint returns.
type runsList struct {
	TotalCount int64       `json:"total_count"`
	Entries    []ActionRun `json:"entries"`
	Runs       []ActionRun `json:"workflow_runs"`
}

// ListRunsForRef lists Actions runs whose head branch (or head sha) matches
// ref. available=false means the instance has no Actions endpoint (404): a
// typed outcome, never a silent empty list. Other API errors return err.
// NOTE: probe-gated. scripts/probe-v0.6.0.sh.findings records that no live
// probe ran in this environment, so the path (/actions/tasks, entries
// envelope) and the head_branch/sha filter are the contract guess; the
// amend-with-probe path is armed (type, transport, and probe finding change
// together if a live probe rejects the guess).
func (c *Client) ListRunsForRef(owner, repo, ref string) (runs []ActionRun, available bool, err error) {
	var list runsList
	path := fmt.Sprintf("/repos/%s/%s/actions/tasks", owner, repo)
	if err := c.Do("GET", path, nil, nil, &list); err != nil {
		var apiErr *APIError
		if errors.As(err, &apiErr) && apiErr.Status == 404 {
			return nil, false, nil
		}
		return nil, false, err
	}
	runs = list.Entries
	if len(runs) == 0 {
		runs = list.Runs
	}
	filtered := make([]ActionRun, 0, len(runs))
	for _, r := range runs {
		if r.HeadBranch == ref || r.HeadSHA == ref {
			filtered = append(filtered, r)
		}
	}
	return filtered, true, nil
}
