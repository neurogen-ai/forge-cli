package cmds

import (
	"fmt"

	"forge/internal/cli"
)

// commentAddCmd posts an issue-backed comment under one of two spellings:
// "pr comment add" or "issue comment add". Forgejo stores PR comments as
// issue comments, so both instances call the same AddComment endpoint and
// print the same receipt. kind affects only the command name and usage text.
// gh marks the gh-spelled canonical verb ("pr comment"); it delegates to the
// same run so both spellings share one implementation and receipt shape.
type commentAddCmd struct {
	kind string // "pr" or "issue"
	gh   bool   // canonical gh spelling: "<kind> comment"
}

func (c commentAddCmd) Name() string {
	if c.gh {
		return c.kind + " comment"
	}
	return c.kind + " comment add"
}
func (c commentAddCmd) Summary() string {
	return "add one comment to a " + c.kind + " [--body T]"
}
func (commentAddCmd) RequiresAPI() bool { return true }

// CommentReceipt is the stable mutation output for issue-backed comments.
type CommentReceipt struct {
	ID      int64  `json:"id"`
	HTMLURL string `json:"html_url"`
}

func (c commentAddCmd) Run(args []string, ctx *cli.Ctx) error { return c.run(args, ctx) }

// run is the one comment-adding implementation; both spellings delegate here
// so receipts stay identical however the user spells the command.
func (c commentAddCmd) run(args []string, ctx *cli.Ctx) error {
	n, err := parseIndex(stripFlags(args, "--body"), c.Name())
	if err != nil {
		return err
	}
	body, _ := flagValue(args, "--body")
	if body == "" {
		return &cli.Error{
			Code: cli.ExitUsage,
			Msg:  c.Name() + ": --body is required",
			Hint: "pass the comment text with --body, or pipe it with --body -",
		}
	}
	data, err := readDashInput(body, ctx.Stdin)
	if err != nil {
		return err
	}
	if len(data) == 0 {
		return &cli.Error{
			Code: cli.ExitUsage,
			Msg:  c.Name() + ": --body is required",
			Hint: "piped stdin for --body - was empty; pass the comment text with --body",
		}
	}
	comment, err := ctx.API.AddComment(ctx.GlobalFlags.Owner, ctx.GlobalFlags.Repo, n, string(data))
	if err != nil {
		return mapErr(err)
	}
	return writeJSON(ctx.Stdout, CommentReceipt{ID: comment.ID, HTMLURL: comment.HTMLURL})
}

func (c commentAddCmd) HelpPage() string {
	if !c.gh {
		return fmt.Sprintf(`use: forge %[1]s N --body T

Add one comment to %[1]s N and print a JSON receipt {id, html_url}.
--body is required; an empty body is a usage error before any request.
--body - reads the comment text from stdin.

Single-shot: one POST, one receipt. The receipt is the full output; --table
is rejected.`, c.kind+" comment add")
	}
	return fmt.Sprintf(`use: forge pr comment N --body T
   or: forge pr comment add N --body T

Add one comment to pr N and print a JSON receipt {id, html_url}.
--body is required; an empty body is a usage error before any request.
--body - reads the comment text from stdin.
"pr comment" is the canonical spelling; "pr comment add" is a compatibility alias with
the same receipt.

Single-shot: one POST, one receipt. The receipt is the full output; --table
is rejected.`)
}
