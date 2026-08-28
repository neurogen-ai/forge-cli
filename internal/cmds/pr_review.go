package cmds

import (
	"fmt"
	"strconv"
	"time"

	"forge/internal/api"
	"forge/internal/cli"
	"forge/internal/table"
)

// ---- pr review list ----

type reviewListCmd struct{}

func (reviewListCmd) Name() string         { return "pr review list" }
func (reviewListCmd) Summary() string      { return "per-review unresolved counts for one PR [--state S]" }
func (reviewListCmd) RequiresAPI() bool    { return true }
func (reviewListCmd) DefaultIsTable() bool { return true }

// ReviewRow is one roster entry. JSON shape mirrors the table columns
// exactly, so scripts get the same view humans do.
type ReviewRow struct {
	ID              int64      `json:"id"`
	User            string     `json:"user"`
	State           string     `json:"state"`
	SubmittedAt     *time.Time `json:"submitted_at,omitempty"`
	UnresolvedCount int        `json:"unresolved_count"`
	TotalCount      int        `json:"total_count"`
}

func (reviewListCmd) Run(args []string, ctx *cli.Ctx) error {
	n, err := parseIndex(args, "pr review list")
	if err != nil {
		return err
	}
	state, _ := flagValue(args, "--state")
	o, r := ctx.GlobalFlags.Owner, ctx.GlobalFlags.Repo
	reviews, err := ctx.API.GetReviews(o, r, n)
	if err != nil {
		return mapErr(err)
	}
	rows := make([]ReviewRow, 0, len(reviews))
	for _, rev := range reviews {
		if state != "" && rev.State != state {
			continue
		}
		rcs, err := ctx.API.GetReviewComments(o, r, n, int(rev.ID))
		if err != nil {
			return mapErr(err)
		}
		unresolved := 0
		for _, rc := range rcs {
			if !rc.IsResolved() {
				unresolved++
			}
		}
		rows = append(rows, ReviewRow{
			ID:              rev.ID,
			User:            rev.User.Login,
			State:           rev.State,
			SubmittedAt:     rev.SubmittedAt,
			UnresolvedCount: unresolved,
			TotalCount:      len(rcs),
		})
	}
	if ctx.OutputIsJSON(ctx.Stdout, true) {
		return writeJSON(ctx.Stdout, rows)
	}
	return table.Render(ctx.Stdout, reviewListColumns, reviewListRows(rows))
}

// reviewListRows maps ReviewRow entries to the roster grid. Server order is
// preserved; no resorting here.
func reviewListRows(rows []ReviewRow) [][]string {
	out := make([][]string, 0, len(rows))
	for _, rw := range rows {
		out = append(out, []string{
			strconv.FormatInt(rw.ID, 10),
			rw.User,
			rw.State,
			strconv.Itoa(rw.UnresolvedCount),
			strconv.Itoa(rw.TotalCount),
		})
	}
	return out
}

func (reviewListCmd) HelpPage() string {
	return `use: forge pr review list N [--state S]

Print a per-review roster for pull request N: one row per review with its
unresolved and total inline-comment counts. Rows appear in server order.
--state filters reviews by exact state (e.g. APPROVED, CHANGES_REQUESTED).

Prints a table on an interactive terminal; JSON elsewhere. --json forces JSON,
--table forces the table.`
}

// ---- pr review submit ----

type reviewSubmitCmd struct{}

func (reviewSubmitCmd) Name() string { return "pr review submit" }
func (reviewSubmitCmd) Summary() string {
	return "submit one review on a pull request [--state S] [--body T]"
}
func (reviewSubmitCmd) RequiresAPI() bool { return true }

// ReviewReceipt is the stable mutation output for review submission.
type ReviewReceipt struct {
	ID    int64  `json:"id"`
	State string `json:"state"`
}

// reviewEvent maps a CLI --state spelling to the exact Forgejo review event.
// request-changes requires a non-empty body; the other events do not.
func reviewEvent(state, body string) (string, error) {
	switch state {
	case "approve":
		return "APPROVED", nil
	case "comment":
		return "COMMENT", nil
	case "request-changes":
		if body == "" {
			return "", &cli.Error{
				Code: cli.ExitUsage,
				Msg:  "pr review submit: --state request-changes requires --body",
				Hint: "explain what must change, or use --state comment for a non-blocking note",
			}
		}
		return "REQUEST_CHANGES", nil
	default:
		return "", &cli.Error{
			Code: cli.ExitUsage,
			Msg:  fmt.Sprintf("pr review submit: --state must be approve, request-changes, or comment (got %q)", state),
		}
	}
}

func (reviewSubmitCmd) Run(args []string, ctx *cli.Ctx) error {
	n, err := parseIndex(args, "pr review submit")
	if err != nil {
		return err
	}
	state, _ := flagValue(args, "--state")
	body, _ := flagValue(args, "--body")
	event, err := reviewEvent(state, body)
	if err != nil {
		return err
	}
	review, err := ctx.API.SubmitReview(ctx.GlobalFlags.Owner, ctx.GlobalFlags.Repo, n, api.SubmitReviewInput{Event: event, Body: body})
	if err != nil {
		return mapErr(err)
	}
	return writeJSON(ctx.Stdout, ReviewReceipt{ID: review.ID, State: review.State})
}

func (reviewSubmitCmd) HelpPage() string {
	return `use: forge pr review submit N --state approve|request-changes|comment [--body T]

Submit one review on pull request N and print a JSON receipt {id, state}.
--state is required and accepts exactly one of:

  approve          APPROVED
  request-changes  REQUEST_CHANGES (requires --body)
  comment          COMMENT

--body supplies the review text; it is required for request-changes and
optional otherwise.

Single-shot: one POST, one receipt. The receipt is the full output; --table
is rejected.`
}
