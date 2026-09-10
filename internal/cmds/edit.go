package cmds

import (
	"fmt"

	"forge/internal/api"
	"forge/internal/cli"
)

// editCmd patches the title and body of one pull request or issue. kind is
// "pr" or "issue"; it selects the command name, help text, and API call.
// Both print the updated object returned by the server. At least one of
// --title/--body must be non-empty; that check runs before any request.
type editCmd struct{ kind string } // "pr" or "issue"

func (c editCmd) Name() string { return c.kind + " edit" }
func (c editCmd) Summary() string {
	return "edit one " + c.kind + "'s title and body [--title T] [--body B]"
}
func (editCmd) RequiresAPI() bool { return true }

// editInput extracts the supplied flags. Empty values are indistinguishable
// from absent ones (flagValue scans "--name value" pairs), and both mean the
// field stays untouched.
func editInput(args []string) (in api.EditPullInput, ok bool) {
	title, _ := flagValue(args, "--title")
	body, _ := flagValue(args, "--body")
	return api.EditPullInput{Title: title, Body: body}, title != "" || body != ""
}

func (c editCmd) Run(args []string, ctx *cli.Ctx) error {
	n, err := parseIndex(stripFlags(args, "--title", "--body"), c.Name())
	if err != nil {
		return err
	}
	in, ok := editInput(args)
	if !ok {
		return &cli.Error{
			Code: cli.ExitUsage,
			Msg:  c.Name() + " requires a non-empty --title or --body",
			Hint: "example: forge " + c.Name() + " " + fmt.Sprint(n) + " --title \"new title\"",
		}
	}
	if c.kind == "pr" {
		pr, err := ctx.API.EditPullRequest(ctx.GlobalFlags.Owner, ctx.GlobalFlags.Repo, n, in)
		if err != nil {
			return mapErr(err)
		}
		return writeJSON(ctx.Stdout, pr)
	}
	iss, err := ctx.API.EditIssue(ctx.GlobalFlags.Owner, ctx.GlobalFlags.Repo, n, api.EditIssueInput(in))
	if err != nil {
		return mapErr(err)
	}
	return writeJSON(ctx.Stdout, iss)
}

func (c editCmd) HelpPage() string {
	if c.kind == "pr" {
		return `use: forge pr edit N [--title T] [--body B]

Edit the title and body of pull request N and print the updated pull request
JSON. At least one flag must be non-empty; empty or absent flags leave the
field untouched. Single-shot: one PATCH, no prompt, no retry.`
	}
	return `use: forge issue edit N [--title T] [--body B]

Edit the title and body of issue N and print the updated issue JSON. At least
one flag must be non-empty; empty or absent flags leave the field untouched.
Single-shot: one PATCH, no prompt, no retry.`
}
