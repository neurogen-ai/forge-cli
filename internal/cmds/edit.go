package cmds

import (
	"fmt"

	"forge/internal/api"
	"forge/internal/cli"
)

// editCmd patches the title and body of one pull request or issue. kind is
// "pr" or "issue"; it selects the command name, help text, and API call.
// Both print the updated object returned by the server. At least one of
// --title/--body/--clear-title/--clear-body (and, for pr, the reviewer
// flags) must be supplied; that check runs before any request.
//
// Reviewer flags are pr-only: all reviewer mutations run after the
// title/body PATCH, adds first in flag order, then removes in flag order.
// A failed reviewer call surfaces as a normal error while the earlier
// PATCH result still prints.
type editCmd struct{ kind string } // "pr" or "issue"

func (c editCmd) Name() string { return c.kind + " edit" }
func (c editCmd) Summary() string {
	return "edit one " + c.kind + "'s title and body [--title T] [--body B]"
}
func (editCmd) RequiresAPI() bool { return true }

// editInput extracts the supplied flags. Empty values are indistinguishable
// from absent ones (flagValue scans "--name value" pairs), and both mean the
// field stays untouched. --clear-title/--clear-body are the explicit
// clearing path: they send an empty-string PATCH, which the omitempty rule
// would otherwise drop.
func editInput(args []string) (in api.EditPullInput, clearTitle, clearBody, ok bool, err error) {
	title, _ := flagValue(args, "--title")
	body, _ := flagValue(args, "--body")
	clearTitle = hasFlag(args, "--clear-title")
	clearBody = hasFlag(args, "--clear-body")
	if clearTitle && title != "" {
		return in, false, false, false, &cli.Error{
			Code: cli.ExitUsage,
			Msg:  "--clear-title and --title are mutually exclusive",
			Hint: "drop one of the two flags",
		}
	}
	if clearBody && body != "" {
		return in, false, false, false, &cli.Error{
			Code: cli.ExitUsage,
			Msg:  "--clear-body and --body are mutually exclusive",
			Hint: "drop one of the two flags",
		}
	}
	ok = title != "" || body != "" || clearTitle || clearBody
	return api.EditPullInput{Title: title, Body: body}, clearTitle, clearBody, ok, nil
}

// editPatchBody is the PATCH payload. With no clearing requested it stays
// the plain EditPullInput, so the v0.4.2 wire body is byte-identical.
// Clearing needs an explicit empty string, which EditPullInput's omitempty
// tags would drop, so the cleared map carries "" literally.
func editPatchBody(in api.EditPullInput, clearTitle, clearBody bool) any {
	if !clearTitle && !clearBody {
		return in
	}
	fields := map[string]any{}
	if clearTitle || in.Title != "" {
		fields["title"] = in.Title
	}
	if clearBody || in.Body != "" {
		fields["body"] = in.Body
	}
	return fields
}

func (c editCmd) Run(args []string, ctx *cli.Ctx) error {
	n, err := parseIndex(stripFlags(args, "--title", "--body", "--add-reviewer", "--remove-reviewer"), c.Name())
	if err != nil {
		return err
	}
	in, clearTitle, clearBody, ok, err := editInput(args)
	if err != nil {
		return err
	}
	// collectFlagValues is the single repeated-value-flag path.
	add := collectFlagValues(args, "--add-reviewer")
	remove := collectFlagValues(args, "--remove-reviewer")
	if c.kind == "pr" {
		ok = ok || len(add) > 0 || len(remove) > 0
	}
	if !ok {
		return &cli.Error{
			Code: cli.ExitUsage,
			Msg:  c.Name() + " requires a non-empty --title or --body",
			Hint: "example: forge " + c.Name() + " " + fmt.Sprint(n) + " --title \"new title\"",
		}
	}
	if c.kind != "pr" && (hasFlag(args, "--add-reviewer") || hasFlag(args, "--remove-reviewer")) {
		return &cli.Error{
			Code: cli.ExitUsage,
			Msg:  "reviewer flags only apply to pr edit",
			Hint: "use forge pr edit " + fmt.Sprint(n) + " --add-reviewer USER",
		}
	}

	if c.kind == "pr" {
		return c.runPull(ctx, n, in, clearTitle, clearBody, add, remove)
	}
	fields := editPatchBody(in, clearTitle, clearBody)
	iss, err := ctx.API.EditIssueFields(ctx.GlobalFlags.Owner, ctx.GlobalFlags.Repo, n, fields)
	if err != nil {
		return mapErr(err)
	}
	return writeJSON(ctx.Stdout, iss)
}

// runPull patches title/body (when anything is set) and then applies the
// reviewer mutations in order. The last successful server response prints,
// even when a later reviewer call fails, so the receipt always carries what
// the server accepted.
func (c editCmd) runPull(ctx *cli.Ctx, n int, in api.EditPullInput, clearTitle, clearBody bool, add, remove []string) error {
	owner, repo := ctx.GlobalFlags.Owner, ctx.GlobalFlags.Repo
	last := (*api.PullRequest)(nil)
	var pr *api.PullRequest
	var err error
	hasPatch := clearTitle || clearBody || in.Title != "" || in.Body != ""
	if hasPatch {
		pr, err = ctx.API.EditPullRequestFields(owner, repo, n, editPatchBody(in, clearTitle, clearBody))
		if err != nil {
			return mapErr(err)
		}
		last = pr
	}
	for _, u := range add {
		pr, err = ctx.API.RequestReviewers(owner, repo, n, []string{u})
		if err != nil {
			if last != nil {
				_ = writeJSON(ctx.Stdout, last)
			}
			return mapErr(err)
		}
		last = pr
	}
	for _, u := range remove {
		pr, err = ctx.API.RemoveReviewers(owner, repo, n, []string{u})
		if err != nil {
			if last != nil {
				_ = writeJSON(ctx.Stdout, last)
			}
			return mapErr(err)
		}
		last = pr
	}
	return writeJSON(ctx.Stdout, last)
}

func (c editCmd) HelpPage() string {
	if c.kind == "pr" {
		return `use: forge pr edit N [--title T] [--body B] [--clear-title] [--clear-body]
                [--add-reviewer U]... [--remove-reviewer U]...

Edit pull request N and print the updated pull request JSON. --title and
--body patch those fields; --clear-title/--clear-body send an explicit
empty-string patch (they cannot combine with the matching value flag).
--add-reviewer and --remove-reviewer are repeatable and run after the
title/body patch, adds first, then removes. A failed reviewer call surfaces
as an error while the earlier patch result still prints. At least one flag
must be supplied; empty or absent title/body flags leave the field
untouched. Single-shot: one patch plus at most one request per reviewer
flag, no prompt, no retry.`
	}
	return `use: forge issue edit N [--title T] [--body B] [--clear-title] [--clear-body]

Edit issue N and print the updated issue JSON. --title and --body patch
those fields; --clear-title/--clear-body send an explicit empty-string patch
(they cannot combine with the matching value flag). Reviewer flags are pr
edit only. At least one flag must be supplied; empty or absent title/body
flags leave the field untouched. Single-shot: one PATCH, no prompt, no
retry.`
}
