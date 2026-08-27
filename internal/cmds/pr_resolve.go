package cmds

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"

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

func mapUnresolve(unresolve bool) string {
	if unresolve {
		return "unresolve"
	}
	return "resolve"
}
