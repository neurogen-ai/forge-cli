package cmds

import (
	"fmt"

	"forge/internal/api"
	"forge/internal/cli"
)

// prMergeCmd submits one explicit merge request. A strategy flag is required.
// With --delete, the head branch is fetched before merging and deleted only
// after the merge succeeds.
type prMergeCmd struct{}

func (prMergeCmd) Name() string { return "pr merge" }
func (prMergeCmd) Summary() string {
	return "merge a pull request [--merge|--squash|--rebase] [--subject S] [--body T] [--delete]"
}
func (prMergeCmd) RequiresAPI() bool { return true }

// MergeReceipt is emitted after the merge request. head_deleted is always
// present so a partial cleanup is visible.
type MergeReceipt struct {
	Index       int64  `json:"index"`
	Action      string `json:"action"`
	HeadDeleted bool   `json:"head_deleted"`
}

// mergeAction validates the strategy flags: exactly one of --merge, --squash,
// or --rebase must be present. It returns the lower-case Do value.
func mergeAction(args []string) (string, error) {
	selected := ""
	for _, flag := range []string{"--merge", "--squash", "--rebase"} {
		if !flagBool(args, flag) {
			continue
		}
		if selected != "" {
			return "", &cli.Error{
				Code: cli.ExitUsage,
				Msg:  "pr merge: exactly one strategy flag is required (--merge, --squash, or --rebase)",
			}
		}
		selected = flag
	}
	switch selected {
	case "":
		return "", &cli.Error{
			Code: cli.ExitUsage,
			Msg:  "pr merge: a strategy flag is required (--merge, --squash, or --rebase)",
		}
	case "--merge":
		return "merge", nil
	case "--squash":
		return "squash", nil
	default:
		return "rebase", nil
	}
}

func (prMergeCmd) Run(args []string, ctx *cli.Ctx) error {
	action, err := mergeAction(args)
	if err != nil {
		return err
	}
	n, err := parseIndex(args, "pr merge")
	if err != nil {
		return err
	}

	in := api.MergeInput{Do: action}
	if v, ok := flagValue(args, "--subject"); ok {
		in.MergeTitleField = v
	}
	if v, ok := flagValue(args, "--body"); ok {
		in.MergeMessageField = v
	}

	head := ""
	if flagBool(args, "--delete") {
		pr, err := ctx.API.GetPullRequest(ctx.GlobalFlags.Owner, ctx.GlobalFlags.Repo, n)
		if err != nil {
			return mapErr(err)
		}
		if pr.Head.Ref == "" {
			return &cli.Error{
				Code: cli.ExitRuntime,
				Msg:  fmt.Sprintf("pr merge: pull request %d has no head ref; refusing to merge with --delete", n),
			}
		}
		head = pr.Head.Ref
	}

	if err := ctx.API.MergePull(ctx.GlobalFlags.Owner, ctx.GlobalFlags.Repo, n, in); err != nil {
		return mapErr(err)
	}

	receipt := MergeReceipt{Index: int64(n), Action: action, HeadDeleted: false}
	if !flagBool(args, "--delete") {
		return writeJSON(ctx.Stdout, receipt)
	}

	if err := ctx.API.DeleteRef(ctx.GlobalFlags.Owner, ctx.GlobalFlags.Repo, head); err != nil {
		// The merge succeeded; make that visible before returning the
		// cleanup error. The receipt shows head_deleted false, the mapped
		// exit code is preserved, and the server message stays unchanged.
		if werr := writeJSON(ctx.Stdout, receipt); werr != nil {
			return werr
		}
		cerr, ok := mapErr(err).(*cli.Error)
		if !ok {
			return mapErr(err)
		}
		hint := "merge succeeded but branch cleanup failed"
		if cerr.Hint != "" {
			hint += "; " + cerr.Hint
		}
		cerr.Hint = hint
		return cerr
	}
	receipt.HeadDeleted = true
	return writeJSON(ctx.Stdout, receipt)
}

func (prMergeCmd) HelpPage() string {
	return `use: forge pr merge N --merge|--squash|--rebase [--subject S] [--body T] [--delete]

Merge pull request N. Exactly one strategy flag is required:
--merge, --squash, or --rebase. --subject sets the merge commit title
and --body the merge message; empty values are omitted.

--delete removes the head branch, but only after the merge succeeds:
the PR is fetched first to capture the head ref, the merge is posted,
and the branch is deleted last. A failed merge never triggers a delete.

Conflicts, WIP, and protection failures are returned by the server as
errors; forge never force-merges or retries. The JSON receipt
{index, action, head_deleted} is printed on success. --table is
rejected: this is a JSON receipt.`
}
