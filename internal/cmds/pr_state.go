package cmds

import (
	"fmt"

	"forge/internal/api"
	"forge/internal/cli"
)

// prStateCmd runs one single-shot PR state transition. action is "close",
// "reopen", or "ready"; it selects the command name, the help text, and the
// API call. All three print the updated pull request returned by the server.
type prStateCmd struct{ action string } // "close", "reopen", or "ready"

func (c prStateCmd) Name() string { return "pr " + c.action }
func (c prStateCmd) Summary() string {
	switch c.action {
	case "ready":
		return "mark a draft pull request ready for review"
	default:
		return c.action + " one pull request"
	}
}
func (prStateCmd) RequiresAPI() bool { return true }

// applyState maps the action to its single API write. The action word is
// fixed at registration, so an unknown value is a programming error and
// surfaces as a usage error rather than reaching the network.
func (c prStateCmd) applyState(ctx *cli.Ctx, n int) (*api.PullRequest, error) {
	switch c.action {
	case "close":
		return ctx.API.SetPRState(ctx.GlobalFlags.Owner, ctx.GlobalFlags.Repo, n, "closed")
	case "reopen":
		return ctx.API.SetPRState(ctx.GlobalFlags.Owner, ctx.GlobalFlags.Repo, n, "open")
	case "ready":
		return ctx.API.SetPRDraft(ctx.GlobalFlags.Owner, ctx.GlobalFlags.Repo, n)
	default:
		return nil, &cli.Error{Code: cli.ExitUsage, Msg: fmt.Sprintf("prStateCmd: unknown action %q", c.action)}
	}
}

func (c prStateCmd) Run(args []string, ctx *cli.Ctx) error {
	n, err := parseIndex(args, c.Name())
	if err != nil {
		return err
	}
	pr, err := c.applyState(ctx, n)
	if err != nil {
		return mapErr(err)
	}
	return writeJSON(ctx.Stdout, pr)
}

func (c prStateCmd) HelpPage() string {
	switch c.action {
	case "close":
		return `use: forge pr close N

Close pull request N and print the updated pull request JSON.
Single-shot: one PATCH {"state":"closed"}, no prompt, no retry.`
	case "reopen":
		return `use: forge pr reopen N

Reopen pull request N and print the updated pull request JSON.
Single-shot: one PATCH {"state":"open"}, no prompt, no retry.`
	default:
		return `use: forge pr ready N

Clear the draft flag on pull request N and print the updated pull request
JSON. Single-shot: one PATCH {"draft":false}, no prompt, no retry. A server
that rejects draft changes reports its message through the normal error path.`
	}
}
