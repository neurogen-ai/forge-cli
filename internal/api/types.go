package api

import "time"

// User is the account object embedded in most Forgejo payloads.
type User struct {
	ID        int64  `json:"id"`
	Login     string `json:"login"`
	FullName  string `json:"full_name"`
	AvatarURL string `json:"avatar_url"`
}

// Label models a repository label; commands resolve names to these IDs.
type Label struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Color string `json:"color"`
}

// PullRequest models GET/POST /repos/{owner}/{repo}/pulls payloads.
type PullRequest struct {
	Number    int64      `json:"number"`
	Title     string     `json:"title"`
	Body      string     `json:"body"`
	State     string     `json:"state"`
	User      User       `json:"user"`
	HTMLURL   string     `json:"html_url"`
	CreatedAt *time.Time `json:"created_at"`

	Head struct {
		Ref string `json:"ref"`
		Sha string `json:"sha"`
	} `json:"head"`
	Base struct {
		Ref string `json:"ref"`
	} `json:"base"`

	Labels []Label `json:"labels,omitempty"`
}

// Issue models issue payloads. PullRequestBody is non-nil exactly when the
// object is actually a pull request rendered through the issues endpoint,
// letting callers filter PRs out of issue listings.
type Issue struct {
	Number    int64      `json:"number"`
	Title     string     `json:"title"`
	Body      string     `json:"body"`
	State     string     `json:"state"`
	User      User       `json:"user"`
	HTMLURL   string     `json:"html_url"`
	CreatedAt *time.Time `json:"created_at"`
	Labels    []Label    `json:"labels,omitempty"`

	PullRequestBody *struct{} `json:"pull_request,omitempty"`
}

// Comment models a comment on an issue or pull request.
type Comment struct {
	ID        int64      `json:"id"`
	User      User       `json:"user"`
	Body      string     `json:"body"`
	HTMLURL   string     `json:"html_url"`
	CreatedAt *time.Time `json:"created_at"`
}

// Review models a pull request review.
type Review struct {
	ID       int64  `json:"id"`
	User     User   `json:"user"`
	State    string `json:"state"`
	Body     string `json:"body"`
	Official bool   `json:"official"`
	// CommitID pins the review to the head sha it inspected. Stale marks a
	// review left behind by later pushes.
	CommitID    string     `json:"commit_id,omitempty"`
	Stale       bool       `json:"stale,omitempty"`
	SubmittedAt *time.Time `json:"submitted_at"`
	CreatedAt   *time.Time `json:"created_at"`
}

// ReviewComment models an inline review comment anchored to a diff hunk.
type ReviewComment struct {
	ID        int64      `json:"id"`
	User      User       `json:"user"`
	Body      string     `json:"body"`
	DiffHunk  string     `json:"diff_hunk"`
	Path      string     `json:"path"`
	CreatedAt *time.Time `json:"created_at"`

	// Anchors. TreePath repeats Path on some servers; both decoded, callers
	// prefer Path when non-empty.
	TreePath         string `json:"tree_path,omitempty"`
	Position         int64  `json:"position,omitempty"`
	OriginalPosition int64  `json:"original_position,omitempty"`
	Line             int64  `json:"line,omitempty"`
	CommitID         string `json:"commit_id,omitempty"`
	OriginalCommitID string `json:"original_commit_id,omitempty"`

	// ReviewID is the owning review. Servers encode this as
	// pull_request_review_id in list payloads.
	ReviewID int64 `json:"pull_request_review_id,omitempty"`

	// Resolution state. Resolved is present on newer servers; Resolver
	// appears whenever a human resolved the thread. See IsResolved.
	Resolved *bool `json:"resolved,omitempty"`
	Resolver *User `json:"resolver,omitempty"`
}

// IsResolved reports whether this comment's thread is resolved, folding the
// two encodings servers use into one predicate (decision D1).
func (rc ReviewComment) IsResolved() bool {
	if rc.Resolver != nil {
		return true
	}
	return rc.Resolved != nil && *rc.Resolved
}
