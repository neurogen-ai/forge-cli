package cmds

import (
	"os"
	"sort"
	"time"

	"forge/internal/api"
	"forge/internal/cli"
	"forge/internal/gitctx"
)

// ---- shared conversation shapes ----

// flatItem is one entry of `pr conversation --format flat`. Every item carries
// its kind; review comments keep the id of their parent review.
type flatItem struct {
	Kind      string     `json:"kind"` // "comment" | "review" | "review-comment"
	ID        int64      `json:"id"`
	User      api.User   `json:"user"`
	Body      string     `json:"body"`
	CreatedAt *time.Time `json:"created_at,omitempty"`
	HTMLURL   string     `json:"html_url,omitempty"`
	State     string     `json:"state,omitempty"`     // reviews only
	Path      string     `json:"path,omitempty"`      // review comments only
	DiffHunk  string     `json:"diff_hunk,omitempty"` // review comments only
	ReviewID  int64      `json:"review_id,omitempty"` // review comments only
}

// groupedReview is one review with its inline comments nested.
type groupedReview struct {
	ID          int64      `json:"id"`
	User        api.User   `json:"user"`
	State       string     `json:"state"`
	Body        string     `json:"body"`
	SubmittedAt *time.Time `json:"submitted_at,omitempty"`
	CreatedAt   *time.Time `json:"created_at,omitempty"`
	Comments    []flatItem `json:"comments"`
}

// groupedConversation is the default `pr conversation` payload.
type groupedConversation struct {
	Comments []api.Comment   `json:"comments"`
	Reviews  []groupedReview `json:"reviews"`
}

// fetchConversation gathers issue-level comments, reviews, and each review's
// inline comments. Used by pr conversation and save pr-conversation.
func fetchConversation(ctx *cli.Ctx, index int) ([]api.Comment, []api.Review, map[int64][]api.ReviewComment, error) {
	o, r := ctx.GlobalFlags.Owner, ctx.GlobalFlags.Repo
	comments, err := ctx.API.GetIssueComments(o, r, index)
	if err != nil {
		return nil, nil, nil, err
	}
	reviews, err := ctx.API.GetReviews(o, r, index)
	if err != nil {
		return nil, nil, nil, err
	}
	perReview := make(map[int64][]api.ReviewComment, len(reviews))
	for _, rev := range reviews {
		rcs, err := ctx.API.GetReviewComments(o, r, index, int(rev.ID))
		if err != nil {
			return nil, nil, nil, err
		}
		perReview[rev.ID] = rcs
	}
	return comments, reviews, perReview, nil
}

// flattenConversation merges everything into one array sorted by created_at
// (nil timestamps last, ties broken by kind then id).
func flattenConversation(comments []api.Comment, reviews []api.Review, perReview map[int64][]api.ReviewComment) []flatItem {
	items := make([]flatItem, 0, len(comments)+len(reviews)+16)
	for _, c := range comments {
		items = append(items, flatItem{
			Kind: "comment", ID: c.ID, User: c.User, Body: c.Body,
			CreatedAt: c.CreatedAt, HTMLURL: c.HTMLURL,
		})
	}
	for _, rev := range reviews {
		revAt := rev.SubmittedAt
		if revAt == nil {
			revAt = rev.CreatedAt
		}
		items = append(items, flatItem{
			Kind: "review", ID: rev.ID, User: rev.User, Body: rev.Body,
			CreatedAt: revAt, State: rev.State,
		})
		for _, rc := range perReview[rev.ID] {
			items = append(items, flatItem{
				Kind: "review-comment", ID: rc.ID, User: rc.User, Body: rc.Body,
				CreatedAt: rc.CreatedAt, Path: rc.Path, DiffHunk: rc.DiffHunk,
				ReviewID: rev.ID,
			})
		}
	}
	rank := map[string]int{"comment": 0, "review": 1, "review-comment": 2}
	sort.SliceStable(items, func(i, j int) bool {
		a, b := items[i], items[j]
		switch {
		case a.CreatedAt == nil && b.CreatedAt != nil:
			return false
		case a.CreatedAt != nil && b.CreatedAt == nil:
			return true
		case a.CreatedAt != nil && b.CreatedAt != nil &&
			!a.CreatedAt.Equal(*b.CreatedAt):
			return a.CreatedAt.Before(*b.CreatedAt)
		case rank[a.Kind] != rank[b.Kind]:
			return rank[a.Kind] < rank[b.Kind]
		default:
			return a.ID < b.ID
		}
	})
	return items
}

// ---- pr create ----

type prCreateCmd struct{}

func (prCreateCmd) Name() string      { return "pr create" }
func (prCreateCmd) Summary() string   { return "open a pull request (--title required)" }
func (prCreateCmd) RequiresAPI() bool { return true }

func (prCreateCmd) Run(args []string, ctx *cli.Ctx) error {
	title, ok := flagValue(args, "--title")
	if !ok || title == "" {
		return &cli.Error{Code: cli.ExitUsage, Msg: "pr create requires --title"}
	}
	head, _ := flagValue(args, "--head")
	if head == "" {
		head = os.Getenv("FORGE_HEAD")
	}
	if head == "" && ctx.Repo != nil {
		head = gitctx.CurrentBranch(ctx.Repo.Root)
	}
	if head == "" {
		return &cli.Error{Code: cli.ExitUsage, Msg: "cannot determine head branch", Hint: "pass --head or run inside a repository on a branch"}
	}
	base, _ := flagValue(args, "--base")
	if base == "" {
		base = os.Getenv("FORGE_BASE")
	}
	if base == "" && ctx.Cfg != nil {
		base = ctx.Cfg.Defaults.Base
	}
	if base == "" {
		return &cli.Error{Code: cli.ExitRuntime, Msg: "cannot determine base branch", Hint: "cannot determine base branch; pass --base"}
	}
	body, _ := flagValue(args, "--body")

	pr, err := ctx.API.CreatePullRequest(ctx.GlobalFlags.Owner, ctx.GlobalFlags.Repo,
		api.CreatePRInput{Title: title, Head: head, Base: base, Body: body})
	if err != nil {
		return mapErr(err)
	}
	return writeJSON(ctx.Stdout, pr)
}

// ---- pr get ----

type prGetCmd struct{}

func (prGetCmd) Name() string      { return "pr get" }
func (prGetCmd) Summary() string   { return "print one pull request as JSON (usage: pr get N)" }
func (prGetCmd) RequiresAPI() bool { return true }

func (prGetCmd) Run(args []string, ctx *cli.Ctx) error {
	n, err := parseIndex(args, "pr get")
	if err != nil {
		return err
	}
	pr, err := ctx.API.GetPullRequest(ctx.GlobalFlags.Owner, ctx.GlobalFlags.Repo, n)
	if err != nil {
		return mapErr(err)
	}
	return writeJSON(ctx.Stdout, pr)
}

// ---- pr list ----

type prListCmd struct{}

func (prListCmd) Name() string { return "pr list" }
func (prListCmd) Summary() string {
	return "list pull requests as a JSON array [--state --page --limit]"
}
func (prListCmd) RequiresAPI() bool { return true }

func (prListCmd) Run(args []string, ctx *cli.Ctx) error {
	state, _ := flagValue(args, "--state")
	page := intFlag(args, "--page", 1)
	limit := intFlag(args, "--limit", 0)
	prs, err := ctx.API.ListPullRequests(ctx.GlobalFlags.Owner, ctx.GlobalFlags.Repo, state, page, limit)
	if err != nil {
		return mapErr(err)
	}
	return writeJSON(ctx.Stdout, prs)
}

// ---- pr conversation ----

type prConvCmd struct{}

func (prConvCmd) Name() string { return "pr conversation" }
func (prConvCmd) Summary() string {
	return "print PR comments + reviews as JSON [--format flat|grouped]"
}
func (prConvCmd) RequiresAPI() bool { return true }

func (prConvCmd) Run(args []string, ctx *cli.Ctx) error {
	n, err := parseIndex(args, "pr conversation")
	if err != nil {
		return err
	}
	format, _ := flagValue(args, "--format")
	if format == "" {
		format = "grouped"
	}
	if format != "flat" && format != "grouped" {
		return &cli.Error{Code: cli.ExitUsage, Msg: "unknown --format " + format, Hint: "use --format flat or --format grouped (default)"}
	}

	comments, reviews, perReview, ferr := fetchConversation(ctx, n)
	if ferr != nil {
		return mapErr(ferr)
	}

	if format == "flat" {
		return writeJSON(ctx.Stdout, flattenConversation(comments, reviews, perReview))
	}

	out := groupedConversation{Comments: comments, Reviews: make([]groupedReview, 0, len(reviews))}
	for _, rev := range reviews {
		gr := groupedReview{
			ID: rev.ID, User: rev.User, State: rev.State, Body: rev.Body,
			SubmittedAt: rev.SubmittedAt, CreatedAt: rev.CreatedAt,
			Comments: make([]flatItem, 0, len(perReview[rev.ID])),
		}
		for _, rc := range perReview[rev.ID] {
			gr.Comments = append(gr.Comments, flatItem{
				Kind: "review-comment", ID: rc.ID, User: rc.User, Body: rc.Body,
				CreatedAt: rc.CreatedAt, Path: rc.Path, DiffHunk: rc.DiffHunk,
				ReviewID: rev.ID,
			})
		}
		out.Reviews = append(out.Reviews, gr)
	}
	return writeJSON(ctx.Stdout, out)
}

// PRCommands returns the pr subcommands for registration in main.
func PRCommands() []cli.Command {
	return []cli.Command{
		prCreateCmd{}, prGetCmd{}, prListCmd{}, prConvCmd{},
	}
}
