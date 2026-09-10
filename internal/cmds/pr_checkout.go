package cmds

import (
	"fmt"

	"forge/internal/api"
	"forge/internal/cli"
	"forge/internal/gitctx"
)

// CheckoutReceipt prints instead of payloads on stdout, always JSON,
// regardless of --json/--table. Branch is empty and Detached true for the
// default checkout; Branch names the local branch when --branch was passed.
type CheckoutReceipt struct {
	Number   int64  `json:"number"`
	HeadSHA  string `json:"head_sha"`
	Branch   string `json:"branch,omitempty"`
	Detached bool   `json:"detached"`
}

// ---- pr checkout ----

type prCheckoutCmd struct{}

func (prCheckoutCmd) Name() string { return "pr checkout" }

func (prCheckoutCmd) Summary() string {
	return "fetch a PR head into the local repository and check it out (detached by default)"
}

func (prCheckoutCmd) RequiresAPI() bool { return true }

func (prCheckoutCmd) HelpPage() string {
	return `use: forge pr checkout N [--branch B]

Fetch the pull request's head into the local repository and check it out.
Without --branch the head SHA is checked out detached. --branch B creates
the local branch B at the head SHA and checks it out; an existing branch is
refused, never overwritten. A cross-repo head fetches from the head
repository's clone URL. The only Forgejo call is the metadata fetch; the
rest is local git, which refuses to overwrite uncommitted local state.`
}

// headFetchURL resolves where the PR head lives: the head repository's
// clone URL for a cross-repo head, the local origin for a same-repo head.
func headFetchURL(pr *api.PullRequest, originURL string) (string, error) {
	if pr.Head.Repo != nil && pr.Head.Repo.CloneURL != "" {
		return pr.Head.Repo.CloneURL, nil
	}
	if pr.Head.Repo == nil && originURL != "" {
		return originURL, nil
	}
	return "", fmt.Errorf("pull request head has no fetchable repository")
}

func (prCheckoutCmd) Run(args []string, ctx *cli.Ctx) error {
	positional := stripFlags(args, "--branch")
	n, err := parseIndex(positional, "pr checkout")
	if err != nil {
		return err
	}
	branch, hasBranch := flagValue(args, "--branch")

	pr, err := ctx.API.GetPullRequest(ctx.GlobalFlags.Owner, ctx.GlobalFlags.Repo, n)
	if err != nil {
		return mapErr(err)
	}
	if pr.Head.Ref == "" && pr.Head.Sha == "" {
		return &cli.Error{
			Code: cli.ExitRuntime,
			Msg:  fmt.Sprintf("pull request %d has no head to check out", n),
			Hint: "the PR may be closed with its branch deleted, or the instance omitted head metadata",
		}
	}
	root, rerr := resolveRoot(ctx)
	if rerr != nil {
		return rerr
	}
	url, uerr := headFetchURL(pr, ctx.Repo.OriginURL)
	if uerr != nil {
		return &cli.Error{
			Code: cli.ExitContext,
			Msg:  uerr.Error(),
			Hint: "pr checkout fetches the head from its repository; add an origin remote or check out from a repository that has one",
		}
	}

	sha, ferr := gitctx.FetchSHA(root, url, pr.Head.Ref)
	if ferr != nil {
		return mapErr(ferr)
	}

	receipt := CheckoutReceipt{Number: int64(n), HeadSHA: sha}
	if hasBranch {
		if cerr := gitctx.CreateBranchAt(root, branch, sha); cerr != nil {
			return mapErr(cerr)
		}
		if cerr := gitctx.CheckoutBranch(root, branch); cerr != nil {
			return mapErr(cerr)
		}
		receipt.Branch = branch
	} else {
		if cerr := gitctx.CheckoutDetached(root, sha); cerr != nil {
			return mapErr(cerr)
		}
		receipt.Detached = true
	}
	return writeJSON(ctx.Stdout, receipt)
}
