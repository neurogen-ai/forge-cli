package cmds

import (
	"fmt"

	"forge/internal/cli"
)

// commentAddCmd posts an issue-backed comment under one of two spellings:
// "pr comment add" or "issue comment add". Forgejo stores PR comments as
// issue comments, so both instances call the same AddComment endpoint and
// print the same receipt. kind affects only the command name and usage text.
type commentAddCmd struct{ kind string } // "pr" or "issue"

func (c commentAddCmd) Name() string { return c.kind + " comment add" }
func (c commentAddCmd) Summary() string {
	return "add one comment to a " + c.kind + " [--body T]"
}
func (commentAddCmd) RequiresAPI() bool { return true }

// CommentReceipt is the stable mutation output for issue-backed comments.
type CommentReceipt struct {
	ID      int64  `json:"id"`
	HTMLURL string `json:"html_url"`
}

func (c commentAddCmd) Run(args []string, ctx *cli.Ctx) error {
	n, err := parseIndex(args, c.Name())
	if err != nil {
		return err
	}
	body, _ := flagValue(args, "--body")
	if body == "" {
		return &cli.Error{
			Code: cli.ExitUsage,
			Msg:  c.Name() + ": --body is required",
			Hint: "pass the comment text with --body",
		}
	}
	comment, err := ctx.API.AddComment(ctx.GlobalFlags.Owner, ctx.GlobalFlags.Repo, n, body)
	if err != nil {
		return mapErr(err)
	}
	return writeJSON(ctx.Stdout, CommentReceipt{ID: comment.ID, HTMLURL: comment.HTMLURL})
}

func (c commentAddCmd) HelpPage() string {
	return fmt.Sprintf(`use: forge %s N --body T

Add one comment to %[2]s N and print a JSON receipt {id, html_url}.
--body is required; an empty body is a usage error before any request.

Single-shot: one POST, one receipt. The receipt is the full output; --table
is rejected.`, c.Name(), c.kind)
}
