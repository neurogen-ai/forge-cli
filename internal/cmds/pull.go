package cmds

import (
	"fmt"
	"path/filepath"
	"time"

	"forge/internal/api"
	"forge/internal/cli"
	"forge/internal/store"
)

// ---- dump payloads ----

// PRCache is the on-disk dump for a pulled PR conversation. It nests raw
// api objects (decision D3): one current copy per item, keyed by filename.
type PRCache struct {
	Comments []api.Comment   `json:"comments"`
	Reviews  []PRCacheReview `json:"reviews"`
}

type PRCacheReview struct {
	ID          int64               `json:"id"`
	User        api.User            `json:"user"`
	State       string              `json:"state"`
	Body        string              `json:"body"`
	SubmittedAt *time.Time          `json:"submitted_at,omitempty"`
	CreatedAt   *time.Time          `json:"created_at,omitempty"`
	CommitID    string              `json:"commit_id,omitempty"`
	Stale       bool                `json:"stale,omitempty"`
	Comments    []api.ReviewComment `json:"comments"` // dumps stay pure server-order; ordering logic belongs to conv views only
}

// IssueCache pairs the issue payload with its timeline comments.
type IssueCache struct {
	Issue    *api.Issue    `json:"issue"`
	Comments []api.Comment `json:"comments"`
}

// PullReceipt prints instead of payloads on stdout, always JSON, ignoring
// both format flags.
//
// Items counts every comment object stored in the dump: for a PR that is the
// issue-level comments plus all nested review comments; for an issue it is
// len(Comments)+1 for the issue payload itself. Reviews and Unresolved are
// PR-only; Unresolved counts threads needing attention at pull time.
type PullReceipt struct {
	Path       string `json:"path"`
	Items      int    `json:"items"`
	Reviews    *int   `json:"reviews,omitempty"`
	Unresolved *int   `json:"unresolved,omitempty"`
}

// ---- pr pull / issue pull ----

type pullCmd struct{ kind string } // "pr" | "issue"

func (c pullCmd) Name() string { return c.kind + " pull" }

func (c pullCmd) Summary() string {
	if c.kind == "issue" {
		return "fetch an issue and its comments into .forge/cache/issues and print a receipt"
	}
	return "fetch a PR's conversation into .forge/cache/prs and print a receipt"
}

func (c pullCmd) RequiresAPI() bool { return true }

func (c pullCmd) HelpPage() string {
	if c.kind == "issue" {
		return `use: forge issue pull N [--dir DIR]

Fetch one issue plus its timeline comments and write them as JSON to the
configured savedir ([savedir] issue). Re-running replaces the previous
snapshot. Prints a JSON receipt on stdout regardless of --json/--table.`
	}
	return `use: forge pr pull N [--dir DIR]

Fetch a PR conversation (issue comments, reviews, inline review comments)
and write it as JSON to the configured savedir ([savedir]
pr-conversation). Re-running replaces the previous snapshot. Prints a JSON
receipt on stdout regardless of --json/--table.`
}

// savedirKey maps a pull kind onto its historical config key; keys do NOT
// get renamed ("pr-conversation" stays for PR pulls, "issue" for issues).
func savedirKey(kind string) string {
	if kind == "pr" {
		return "pr-conversation"
	}
	return kind
}

// unresolvedCount reports how many review comments still need attention at
// pull time (IsResolved()==false), summed across every pulled review.
func unresolvedCount(reviews []PRCacheReview) int {
	n := 0
	for _, rev := range reviews {
		for _, rc := range rev.Comments {
			if !rc.IsResolved() {
				n++
			}
		}
	}
	return n
}

func (c pullCmd) Run(args []string, ctx *cli.Ctx) error {
	positional := stripFlag(args, "--dir")
	n, err := parseIndex(positional, c.kind+" pull")
	if err != nil {
		return err
	}
	key := savedirKey(c.kind)

	var payload any
	receipt := PullReceipt{}
	switch c.kind {
	case "pr":
		comments, reviews, perReview, ferr := fetchConversation(ctx, n)
		if ferr != nil {
			return mapErr(ferr)
		}
		cache := PRCache{Comments: comments, Reviews: make([]PRCacheReview, 0, len(reviews))}
		items := len(comments)
		for _, rev := range reviews {
			rcs := perReview[rev.ID]
			items += len(rcs)
			cache.Reviews = append(cache.Reviews, PRCacheReview{
				ID: rev.ID, User: rev.User, State: rev.State, Body: rev.Body,
				SubmittedAt: rev.SubmittedAt, CreatedAt: rev.CreatedAt,
				CommitID: rev.CommitID, Stale: rev.Stale,
				Comments: rcs,
			})
		}
		payload = cache
		unresolved := unresolvedCount(cache.Reviews)
		counts := len(cache.Reviews)
		receipt.Items = items
		receipt.Reviews = &counts
		receipt.Unresolved = &unresolved
	case "issue":
		iss, gerr := ctx.API.GetIssue(ctx.GlobalFlags.Owner, ctx.GlobalFlags.Repo, n)
		if gerr != nil {
			return mapErr(gerr)
		}
		comments, cerr := ctx.API.GetIssueComments(ctx.GlobalFlags.Owner, ctx.GlobalFlags.Repo, n)
		if cerr != nil {
			return mapErr(cerr)
		}
		payload = IssueCache{Issue: iss, Comments: comments}
		receipt.Items = len(comments) + 1
	default:
		return fmt.Errorf("unknown pull kind %q", c.kind)
	}

	// Dir resolution: --dir flag, then [savedir] <key> in config.
	dir := ""
	if d, ok := flagValue(args, "--dir"); ok && d != "" {
		dir = d
	} else if ctx.Cfg != nil {
		if d, ok := ctx.Cfg.Savedirs[key]; ok && d != "" {
			dir = d
		}
	}
	if dir == "" {
		return &cli.Error{
			Code: cli.ExitUsage,
			Msg:  "no savedir for " + key,
			Hint: "pass --dir or restore the seeded [savedir] " + key + " config entry",
		}
	}
	root, rerr := resolveRoot(ctx)
	if rerr != nil {
		return rerr
	}

	absDir := dir
	if !filepath.IsAbs(absDir) {
		absDir = filepath.Join(root, dir)
	}
	path, werr := store.WriteJSON(absDir, ctx.GlobalFlags.Repo, n, payload)
	if werr != nil {
		return mapErr(werr)
	}
	receipt.Path = path
	return writeJSON(ctx.Stdout, receipt)
}

// PullCommands registers both pull subcommands.
func PullCommands() []cli.Command {
	return []cli.Command{pullCmd{kind: "pr"}, pullCmd{kind: "issue"}}
}
