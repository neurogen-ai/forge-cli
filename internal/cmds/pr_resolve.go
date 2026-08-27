package cmds

import (
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"forge/internal/api"
	"forge/internal/cli"
)

// resolveCmd implements "pr comment resolve" and "pr comment unresolve".
// Both target ROOT review-comment ids: reply ids yield server errors passed
// through verbatim. Re-resolving (or re-unresolving) is idempotent
// server-side; there is no local dedup or pre-probing.
type resolveCmd struct{ unresolve bool }

func (c resolveCmd) Name() string {
	if c.unresolve {
		return "pr comment unresolve"
	}
	return "pr comment resolve"
}

func (c resolveCmd) Summary() string {
	if c.unresolve {
		return "mark a review-comment thread unresolved"
	}
	return "mark a review-comment thread resolved"
}

func (c resolveCmd) RequiresAPI() bool { return true }

// ResolutionReceipt is printed as JSON regardless of format flags.
type ResolutionReceipt struct {
	ID     int64  `json:"id"`
	Action string `json:"action"` // "resolve" | "unresolve"
}

func (c resolveCmd) Run(args []string, ctx *cli.Ctx) error {
	id, err := parseInt64Arg(args, c.Name())
	if err != nil {
		return err
	}
	var apiErr error
	action := "resolve"
	if c.unresolve {
		apiErr = ctx.API.UnresolveThread(ctx.GlobalFlags.Owner, ctx.GlobalFlags.Repo, id)
		action = "unresolve"
	} else {
		apiErr = ctx.API.ResolveThread(ctx.GlobalFlags.Owner, ctx.GlobalFlags.Repo, id)
	}
	if apiErr != nil {
		return mapResolveErr(apiErr)
	}
	return writeJSON(ctx.Stdout, ResolutionReceipt{ID: id, Action: action})
}

// parseInt64Arg parses args[0] as a positive int64, with exit-2-style usage
// errors on missing, unparsable, or non-positive input.
func parseInt64Arg(args []string, cmdName string) (int64, error) {
	if len(args) == 0 {
		return 0, &cli.Error{Code: cli.ExitUsage, Msg: cmdName + " requires a comment id"}
	}
	id, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil || id <= 0 {
		return 0, &cli.Error{Code: cli.ExitUsage, Msg: fmt.Sprintf("%s: %q is not a valid comment id", cmdName, args[0])}
	}
	return id, nil
}

// mapResolveErr converts endpoint failures into loud, navigable errors.
// Status 404 reads as: this server has no conversation-resolution API at
// this patch level (decision D2). Any other failure passes through mapErr
// untouched so server messages stay verbatim.
func mapResolveErr(err error) error {
	var apiErr *api.APIError
	if errors.As(err, &apiErr) && apiErr.Status == http.StatusNotFound {
		return &cli.Error{
			Code: cli.ExitRuntime,
			Msg:  "this server does not expose the comment-resolution endpoint",
			Hint: fmt.Sprintf("PATCH .../pulls/comments/{id}/resolve returned 404; server said %q. Check your Forgejo/Gitea version supports conversation resolution.", apiErr.Message),
		}
	}
	return mapErr(err)
}

func (c resolveCmd) HelpPage() string {
	use := "use: forge pr comment resolve COMMENT_ID"
	if c.unresolve {
		use = "use: forge pr comment unresolve COMMENT_ID"
	}
	action := "resolved"
	if c.unresolve {
		action = "unresolved"
	}
	return use + `

Mark the review-comment thread rooted at COMMENT_ID ` + action + `. The id must
be the ROOT comment of the thread; reply ids yield server errors surfaced
verbatim. Re-applying the same resolution succeeds silently server-side — it
is safe to retry.

On success prints exactly this JSON receipt regardless of --json/--table:

  { "id": <COMMENT_ID>, "action": "` + mapUnresolve(c.unresolve) + `" }

If the server answers 404, your Forgejo/Gitea version does not expose the
conversation-resolution endpoint; nothing was changed.`
}

// ---- pr resolve-all ----

// resolveAllCmd implements "pr resolve-all": a dry-run by default, resolving
// every open thread of every (or one filtered) review with --yes.
type resolveAllCmd struct{}

func (resolveAllCmd) Name() string { return "pr resolve-all" }
func (resolveAllCmd) Summary() string {
	return "dry-run by default; with --yes resolves every open thread [--review R]"
}
func (resolveAllCmd) RequiresAPI() bool { return true }

// ResolveAllSummary prints after a --yes run. Dry-runs print a JSON array
// of int64 root-comment ids only, no wrapper object.
type ResolveAllSummary struct {
	Requested int                 `json:"requested"`
	Resolved  int                 `json:"resolved"`
	Skipped   int                 `json:"skipped"` // already resolved at call time; safe to rerun; stays zero unless server answers success-with-no-change observably
	Failed    []ResolutionFailure `json:"failed,omitempty"`
}

// ResolutionFailure records one thread that the server refused, with its
// message verbatim.
type ResolutionFailure struct {
	ID   int64  `json:"id"`
	Text string `json:"text"` // server message verbatim
}

func (resolveAllCmd) Run(args []string, ctx *cli.Ctx) error {
	n, err := parseIndex(args, "pr resolve-all")
	if err != nil {
		return err
	}
	reviewFilterStr, ok := flagValue(args, "--review")
	var reviewFilter int64
	if ok {
		reviewFilter, err = strconv.ParseInt(reviewFilterStr, 10, 64)
		if err != nil || reviewFilter <= 0 {
			return &cli.Error{Code: cli.ExitUsage, Msg: fmt.Sprintf("pr resolve-all: %q is not a valid review id", reviewFilterStr)}
		}
	}
	dryRun := !flagBool(args, "--yes")

	o, r := ctx.GlobalFlags.Owner, ctx.GlobalFlags.Repo
	reviews, err := ctx.API.GetReviews(o, r, n)
	if err != nil {
		return mapErr(err)
	}
	if ok {
		found := false
		for _, rev := range reviews {
			if rev.ID == reviewFilter {
				found = true
				break
			}
		}
		if !found {
			ids := make([]string, 0, len(reviews))
			for _, rev := range reviews {
				ids = append(ids, strconv.FormatInt(rev.ID, 10))
			}
			return &cli.Error{
				Code: cli.ExitUsage,
				Msg:  fmt.Sprintf("pr resolve-all: review %d not found on PR %d", reviewFilter, n),
				Hint: fmt.Sprintf("available reviews: %s", strings.Join(ids, ", ")),
			}
		}
		reviews = []api.Review{{ID: reviewFilter}}
	}

	// Collect targets: root comments of unresolved threads, sorted ascending
	// for deterministic runs.
	type target struct{ commentID int64 }
	var targets []target
	for _, rev := range reviews {
		rcs, err := ctx.API.GetReviewComments(o, r, n, int(rev.ID))
		if err != nil {
			return mapErr(err)
		}
		for _, rc := range rcs {
			if !rc.IsResolved() { // review comments are thread roots in this view
				targets = append(targets, target{commentID: rc.ID})
			}
		}
	}
	sort.SliceStable(targets, func(i, j int) bool { return targets[i].commentID < targets[j].commentID })

	ids := make([]int64, len(targets))
	for i, t := range targets {
		ids[i] = t.commentID
	}
	if dryRun {
		return writeJSON(ctx.Stdout, ids)
	}

	summary := ResolveAllSummary{Requested: len(targets)}
	for _, t := range targets {
		err := ctx.API.ResolveThread(o, r, t.commentID)
		if err != nil {
			var apiErr *api.APIError
			if errors.As(err, &apiErr) && apiErr.Status == http.StatusNotFound {
				// Decision D2: abort immediately on missing endpoint; reuse
				// mapResolveErr so the version hint stays in one place.
				return mapResolveErr(err)
			}
			summary.Failed = append(summary.Failed, ResolutionFailure{ID: t.commentID, Text: serverMessage(apiErr)})
			continue
		}
		summary.Resolved++
	}
	// Print the summary FIRST so stdout stays intact even on partial failure.
	if err := writeJSON(ctx.Stdout, summary); err != nil {
		return err
	}
	if len(summary.Failed) > 0 {
		return &cli.Error{
			Code: cli.ExitRuntime,
			Msg:  fmt.Sprintf("%d of %d resolutions failed", len(summary.Failed), summary.Requested),
		}
	}
	return nil
}

func (resolveAllCmd) HelpPage() string {
	return `use: forge pr resolve-all PR_INDEX [--yes] [--review REVIEW_ID]

Resolve every open review-comment thread on one pull request.

By default this is a DRY RUN: it prints a JSON array of the root-comment ids
it would resolve and changes nothing. Pass --yes to actually resolve them;
the threads are processed serially in ascending root-comment-id order and a
JSON summary is printed:

  { "requested": <int>, "resolved": <int>, "skipped": <int>, "failed": [...] }

"failed" lists { "id", "text" } pairs with server messages verbatim and is
omitted when empty. The summary prints even when some resolutions failed, so
stdout remains parseable; the command then exits non-zero naming the count.

Filter to one review's threads with --review REVIEW_ID. If that review does
not exist on the PR, the error names the available reviews.

Threads already resolved are never touched; a rerun after success is a no-op.
If your Forgejo/Gitea version lacks the conversation-resolution endpoint the
run aborts loudly on the first resolution attempt.`
}

func mapUnresolve(unresolve bool) string {
	if unresolve {
		return "unresolve"
	}
	return "resolve"
}
