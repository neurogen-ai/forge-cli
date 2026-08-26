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
	ID          int64      `json:"id"`
	User        User       `json:"user"`
	State       string     `json:"state"`
	Body        string     `json:"body"`
	Official    bool       `json:"official"`
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
	ReviewID  int64      `json:"review_id,omitempty"`
	CreatedAt *time.Time `json:"created_at"`
}
