package cmds

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"forge/internal/api"
	"forge/internal/cli"
)

// ---- conv pipeline ----

// ConvPayload is pr conv's JSON shape.
type ConvPayload struct {
	Comments []api.Comment `json:"comments"` // issue-level comments, unfiltered
	Reviews  []ConvReview  `json:"reviews"`  // filtered per CLI flags
}

type ConvReview struct {
	ID          int64      `json:"id"`
	User        api.User   `json:"user"`
	State       string     `json:"state"`
	Body        string     `json:"body"`
	SubmittedAt *time.Time `json:"submitted_at,omitempty"`
	CreatedAt   *time.Time `json:"created_at,omitempty"`
	Stale       bool       `json:"stale,omitempty"`
	CommitID    string     `json:"commit_id,omitempty"`

	UnresolvedCount int                 `json:"unresolved_count"`
	TotalCount      int                 `json:"total_count"`
	Comments        []api.ReviewComment `json:"comments"`
}

// buildConv assembles the filtered payload.
// Filter: reviews pass when UnresolvedCount >= minUnresolved, or when showAll
// is true. Default call site: minUnresolved=1, showAll=false.
// Ordering: across reviews oldest SubmittedAt first (falling back to
// CreatedAt; nil last); within each review comments sorted unresolved-first,
// ties broken by Path then Position then ID.
func buildConv(comments []api.Comment, reviews []api.Review, perReview map[int64][]api.ReviewComment, showAll bool, minUnresolved int) ConvPayload {
	out := ConvPayload{Comments: comments, Reviews: make([]ConvReview, 0, len(reviews))}
	for _, rev := range reviews {
		rcs := perReview[rev.ID]
		unresolved := 0
		for _, rc := range rcs {
			if !rc.IsResolved() {
				unresolved++
			}
		}
		if !showAll && unresolved < minUnresolved {
			continue
		}
		cr := ConvReview{
			ID:              rev.ID,
			User:            rev.User,
			State:           rev.State,
			Body:            rev.Body,
			SubmittedAt:     rev.SubmittedAt,
			CreatedAt:       rev.CreatedAt,
			Stale:           rev.Stale,
			CommitID:        rev.CommitID,
			UnresolvedCount: unresolved,
			TotalCount:      len(rcs),
			Comments:        append([]api.ReviewComment(nil), rcs...),
		}
		orderCommentsUnresolvedFirst(cr.Comments)
		out.Reviews = append(out.Reviews, cr)
	}
	sortReviewsOldestFirst(out.Reviews)
	return out
}

// reviewStamp is the review's ordering timestamp: SubmittedAt, falling back to
// CreatedAt.
func reviewStamp(r ConvReview) *time.Time {
	if r.SubmittedAt != nil {
		return r.SubmittedAt
	}
	return r.CreatedAt
}

// sortReviewsOldestFirst orders reviews oldest-first by their stamp. Nil
// stamps sort last; equal stamps fall back to ID ascending. Stable so
// otherwise-identical entries keep input order.
func sortReviewsOldestFirst(reviews []ConvReview) {
	sort.SliceStable(reviews, func(i, j int) bool {
		a, b := reviewStamp(reviews[i]), reviewStamp(reviews[j])
		switch {
		case a == nil && b == nil:
			return reviews[i].ID < reviews[j].ID
		case a == nil:
			return false
		case b == nil:
			return true
		case !a.Equal(*b):
			return a.Before(*b)
		default:
			return reviews[i].ID < reviews[j].ID
		}
	})
}

// orderCommentsUnresolvedFirst sorts inline comments unresolved-first; within
// each group ties break on Path lexicographically, then Position ascending,
// then ID ascending. Stable sort keeps fully-tied entries in input order.
func orderCommentsUnresolvedFirst(rcs []api.ReviewComment) {
	sort.SliceStable(rcs, func(i, j int) bool {
		a, b := rcs[i], rcs[j]
		ra, rb := a.IsResolved(), b.IsResolved()
		switch {
		case ra != rb:
			return rb // unresolved (false) before resolved (true)
		case a.Path != b.Path:
			return a.Path < b.Path
		case a.Position != b.Position:
			return a.Position < b.Position
		default:
			return a.ID < b.ID
		}
	})
}

// ---- command ----

type prConvCmd struct{}

func (prConvCmd) Name() string {
	return "pr conv"
}
func (prConvCmd) Summary() string {
	return "conversation view, unresolved threads first [--all] [--min-unresolved N]"
}
func (prConvCmd) RequiresAPI() bool    { return true }
func (prConvCmd) DefaultIsTable() bool { return true } // makes --table legal; rendering is sectioned, not gridded

func (prConvCmd) Run(args []string, ctx *cli.Ctx) error {
	n, err := parseIndex(args, "pr conv")
	if err != nil {
		return err
	}
	showAll := flagBool(args, "--all")
	minUnresolved := intFlag(args, "--min-unresolved", 1)
	comments, reviews, perReview, ferr := fetchConversation(ctx, n)
	if ferr != nil {
		return mapErr(ferr)
	}
	payload := buildConv(comments, reviews, perReview, showAll, minUnresolved)
	if ctx.OutputIsJSON(ctx.Stdout, false) { // conv defaults to JSON even on a TTY
		return writeJSON(ctx.Stdout, payload)
	}
	return renderConvSections(ctx.Stdout, payload)
}

func (prConvCmd) HelpPage() string {
	return `use: forge pr conv N [--all] [--min-unresolved N]

Print one pull request's conversation: issue comments plus reviews with their
inline review comments nested underneath, unresolved threads first.

Reviews whose threads are fully resolved are dropped by default; --all keeps
them. --min-unresolved N (default 1) requires at least N unresolved threads
for a review to appear; --min-unresolved 0 behaves like --all for filtering.
Reviews sort oldest first; within each review, unresolved comments come before
resolved ones, then by path and position.

conv prints JSON everywhere -- there is no table grid here. --table forces the
sectioned text render instead of JSON.`
}

// renderConvSections prints one header line per review then indented comment
// blocks. No colour, no ANSI, ever.
func renderConvSections(w io.Writer, p ConvPayload) error {
	for _, rev := range p.Reviews {
		fmt.Fprintf(w, "# review %d  %s  by %s", rev.ID, rev.State, rev.User.Login)
		if rev.Stale {
			fmt.Fprint(w, "  [stale]")
		}
		fmt.Fprintf(w, "  unresolved %d/%d\n", rev.UnresolvedCount, rev.TotalCount)
		for _, c := range rev.Comments {
			marker := ""
			if c.IsResolved() {
				marker = " [resolved]"
			}
			fmt.Fprintf(w, "\n  %s:%d  comment %d%s by %s\n",
				convPath(c), convLine(c), c.ID, marker, c.User.Login)
			for _, l := range strings.Split(strings.TrimRight(c.Body, "\n"), "\n") {
				fmt.Fprintf(w, "    %s\n", l)
			}
		}
		fmt.Fprintln(w)
	}
	return nil
}

// convPath is the comment's Path, falling back to TreePath when the server
// sent only that.
func convPath(c api.ReviewComment) string {
	if c.Path != "" {
		return c.Path
	}
	return c.TreePath
}

// convLine is the comment's Line, falling back to Position when the thread is
// anchored that way.
func convLine(c api.ReviewComment) int64 {
	if c.Line != 0 {
		return c.Line
	}
	return c.Position
}
