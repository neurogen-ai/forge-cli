package cmds

import (
	"fmt"

	"forge/internal/cli"
)

// prLockReceipt is the write receipt for pr lock/pr unlock. Locking is an
// issue-backed mutation (PRs ride the issue endpoints, like close/reopen)
// and the lock endpoints return no body, so the receipt carries what the
// command did rather than a server payload.
type prLockReceipt struct {
	Index  int64  `json:"index"`
	Action string `json:"action"`
	Reason string `json:"reason,omitempty"`
}

// prLockCmd locks or unlocks one pull request's conversation. locking is
// fixed at registration; true is pr lock, false is pr unlock.
type prLockCmd struct{ locking bool }

func (c prLockCmd) Name() string {
	if c.locking {
		return "pr lock"
	}
	return "pr unlock"
}

func (c prLockCmd) Summary() string {
	if c.locking {
		return "lock a pull request's conversation [--reason R]"
	}
	return "unlock a pull request's conversation"
}

func (prLockCmd) RequiresAPI() bool { return true }

func (c prLockCmd) Run(args []string, ctx *cli.Ctx) error {
	name := c.Name()
	n, err := parseIndex(stripFlags(args, "--reason"), name)
	if err != nil {
		return err
	}
	reason := ""
	if c.locking {
		if hasFlagToken(args, "--reason") {
			var ok bool
			reason, ok = flagValue(args, "--reason")
			if !ok {
				return &cli.Error{
					Code: cli.ExitUsage,
					Msg:  "--reason requires a value",
					Hint: "example: forge pr lock " + fmt.Sprint(n) + " --reason off-topic",
				}
			}
		}
		if err := c.lock(ctx, n, reason); err != nil {
			return err
		}
	} else if err := ctx.API.UnlockIssue(ctx.GlobalFlags.Owner, ctx.GlobalFlags.Repo, n); err != nil {
		return mapErr(err)
	}
	return writeJSON(ctx.Stdout, prLockReceipt{Index: int64(n), Action: c.action(), Reason: reason})
}

func (c prLockCmd) action() string {
	if c.locking {
		return "lock"
	}
	return "unlock"
}

// lock applies the lock write. A server without lock support returns its
// message through the normal API error path; there is no special casing.
func (c prLockCmd) lock(ctx *cli.Ctx, n int, reason string) error {
	return mapErr(ctx.API.LockIssue(ctx.GlobalFlags.Owner, ctx.GlobalFlags.Repo, n, reason))
}

func (c prLockCmd) HelpPage() string {
	if c.locking {
		return `use: forge pr lock N [--reason R]

Lock the conversation on pull request N and print a receipt JSON. R is an
optional reason passed through to the server, which owns validation. The
verb is issue-backed underneath, matching pr close and pr reopen: PRs ride
the issue lock endpoints. A server that cannot lock reports its message
through the normal error path. Single-shot: one POST, no prompt, no retry.`
	}
	return `use: forge pr unlock N

Remove the conversation lock from pull request N and print a receipt JSON.
The verb is issue-backed underneath, matching pr close and pr reopen: PRs
ride the issue lock endpoints. Single-shot: one DELETE, no prompt, no retry.`
}
