package cmds

import (
	"fmt"
	"strings"

	"forge/internal/cli"
	"forge/internal/gitctx"
)

// SyncStatusReceipt is the JSON view of the one-shot answer. Status is
// exactly "in-sync", "out-of-sync", or "no-pr". The PR fields are filled
// only when a PR was found; out-of-sync is data, not an error, and the
// command exits 0 for all three statuses.
type SyncStatusReceipt struct {
	Branch    string `json:"branch"`
	Status    string `json:"status"`
	HeadSHA   string `json:"head_sha"`
	PRNumber  int64  `json:"pr_number,omitempty"`
	PRHeadSHA string `json:"pr_head_sha,omitempty"`
	PRURL     string `json:"pr_url,omitempty"`
}

type prSyncStatusCmd struct{}

func (prSyncStatusCmd) Name() string { return "pr sync-status" }

func (prSyncStatusCmd) Summary() string {
	return "print one branch's PR sync state: in-sync, out-of-sync, or no-pr"
}

func (prSyncStatusCmd) RequiresAPI() bool { return true }

func (prSyncStatusCmd) HelpPage() string {
	return `use: forge pr sync-status [BRANCH]

One request, one answer: whether the open pull request for BRANCH (default:
the current local branch) carries the branch's local head SHA. Prints
in-sync, out-of-sync, or no-pr and exits 0 in all three cases; out-of-sync
is data, not an error. The caller owns any polling -- this command never
retries, watches, or waits. The single API request is the open-PR list for
the repository, matched against the branch name.`
}

// Run resolves the branch locally, makes exactly one API request (the
// open-PR list), and compares head SHAs. No PR for the branch and a SHA
// mismatch are both stdout data with exit 0.
func (prSyncStatusCmd) Run(args []string, ctx *cli.Ctx) error {
	rest := stripFlags(args)
	root, err := resolveRoot(ctx)
	if err != nil {
		return err
	}
	branch := ""
	if len(rest) == 1 {
		branch = rest[0]
	} else if len(rest) > 1 {
		return &cli.Error{
			Code: cli.ExitUsage,
			Msg:  "pr sync-status takes at most one BRANCH",
			Hint: "example: forge pr sync-status feature-x",
		}
	} else {
		branch = gitctx.CurrentBranch(root)
		if branch == "" {
			return &cli.Error{
				Code: cli.ExitContext,
				Msg:  "cannot determine the current branch (detached HEAD?)",
				Hint: "pass the branch explicitly: forge pr sync-status BRANCH",
			}
		}
	}
	sha, err := gitctx.BranchHead(root, branch)
	if err != nil {
		if strings.Contains(err.Error(), "not inside a git repository") {
			return &cli.Error{
				Code: cli.ExitContext,
				Msg:  "not inside a git repository",
				Hint: "run from a clone of the repository",
			}
		}
		return &cli.Error{
			Code: cli.ExitContext,
			Msg:  fmt.Sprintf("unknown local branch %q", branch),
			Hint: fmt.Sprintf("git rev-parse refs/heads/%s failed: %v; list local branches with git branch", branch, err),
		}
	}

	prs, err := ctx.API.ListOpenPullRequestsOnePage(ctx.GlobalFlags.Owner, ctx.GlobalFlags.Repo)
	if err != nil {
		return mapErr(err)
	}
	receipt := SyncStatusReceipt{Branch: branch, HeadSHA: sha, Status: "no-pr"}
	for i := range prs {
		if prs[i].Head.Ref == branch {
			receipt.PRNumber = prs[i].Number
			receipt.PRHeadSHA = prs[i].Head.Sha
			receipt.PRURL = prs[i].HTMLURL
			if prs[i].Head.Sha == sha {
				receipt.Status = "in-sync"
			} else {
				receipt.Status = "out-of-sync"
			}
			break
		}
	}
	if ctx.OutputIsJSON(ctx.Stdout, false) {
		return writeJSON(ctx.Stdout, receipt)
	}
	fmt.Fprintln(ctx.Stdout, receipt.Status)
	return nil
}
