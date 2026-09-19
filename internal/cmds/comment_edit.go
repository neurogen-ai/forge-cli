package cmds

import "forge/internal/cli"

// commentEditCmd edits one issue comment's body under the "pr comment edit"
// and "issue comment edit" spellings. Forgejo stores PR comments as issue
// comments, so both instances call the same EditComment endpoint; kind
// affects only the command name and usage text.
type commentEditCmd struct{ kind string } // "pr" or "issue"

func (c commentEditCmd) Name() string { return c.kind + " comment edit" }
func (c commentEditCmd) Summary() string {
	return "edit one " + c.kind + " comment's body (usage: " + c.Name() + " ID --body B)"
}
func (commentEditCmd) RequiresAPI() bool { return true }

// commentDeleteReceipt records the deleted comment id and the action taken,
// because the DELETE endpoint returns no body.
type commentDeleteReceipt struct {
	ID     int64  `json:"id"`
	Action string `json:"action"`
}

// commentDeleteCmd deletes one issue comment under the "pr comment delete"
// and "issue comment delete" spellings. Both delegate to one run.
type commentDeleteCmd struct{ kind string } // "pr" or "issue"

func (c commentDeleteCmd) Name() string { return c.kind + " comment delete" }
func (c commentDeleteCmd) Summary() string {
	return "delete one " + c.kind + " comment (usage: " + c.Name() + " ID)"
}
func (commentDeleteCmd) RequiresAPI() bool { return true }

func (c commentEditCmd) Run(args []string, ctx *cli.Ctx) error {
	n, err := parseIndex(stripFlags(args, "--body"), c.Name())
	id := int64(n)
	if err != nil {
		return err
	}
	body, _ := flagValue(args, "--body")
	if body == "" {
		return &cli.Error{
			Code: cli.ExitUsage,
			Msg:  c.Name() + ": --body is required",
			Hint: "pass the new text with --body, or pipe it with --body -",
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
			Hint: "piped stdin for --body - was empty; pass the new text with --body",
		}
	}
	com, err := ctx.API.EditComment(ctx.GlobalFlags.Owner, ctx.GlobalFlags.Repo, id, string(data))
	if err != nil {
		return mapErr(err)
	}
	return writeJSON(ctx.Stdout, com)
}

func (c commentDeleteCmd) Run(args []string, ctx *cli.Ctx) error {
	n, err := parseIndex(args, c.Name())
	id := int64(n)
	if err != nil {
		return err
	}
	if err := ctx.API.DeleteComment(ctx.GlobalFlags.Owner, ctx.GlobalFlags.Repo, id); err != nil {
		return mapErr(err)
	}
	return writeJSON(ctx.Stdout, commentDeleteReceipt{ID: id, Action: "delete"})
}

func (c commentEditCmd) HelpPage() string {
	if c.kind == "pr" {
		return `use: forge pr comment edit ID --body B

Edit the body of issue comment ID and print the updated comment JSON.
--body is required; an empty body is a usage error before any request.
--body - reads the new text from stdin.

IDs are issue-comment ids, the numbers in a comment's html_url
(#issuecomment-N). Review comments live in a separate id space and are not
reachable here; resolve review threads with pr comment resolve instead.

Single-shot: one PATCH, no prompt, no retry.`
	}
	return `use: forge issue comment edit ID --body B

Edit the body of issue comment ID and print the updated comment JSON.
--body is required; an empty body is a usage error before any request.
--body - reads the new text from stdin.

IDs are issue-comment ids, the numbers in a comment's html_url
(#issuecomment-N). Review comments live in a separate id space and are not
reachable from this verb.

Single-shot: one PATCH, no prompt, no retry.`
}

func (c commentDeleteCmd) HelpPage() string {
	if c.kind == "pr" {
		return `use: forge pr comment delete ID

Delete issue comment ID and print a JSON receipt {id, action:"delete"}.

IDs are issue-comment ids, the numbers in a comment's html_url
(#issuecomment-N). Review comments live in a separate id space and are not
reachable here; resolve review threads with pr comment resolve instead.

Single-shot: one DELETE, no prompt, no confirmation.`
	}
	return `use: forge issue comment delete ID

Delete issue comment ID and print a JSON receipt {id, action:"delete"}.

IDs are issue-comment ids, the numbers in a comment's html_url
(#issuecomment-N). Review comments live in a separate id space and are not
reachable from this verb.

Single-shot: one DELETE, no prompt, no confirmation.`
}
