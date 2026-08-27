package cmds

import (
	"os"

	"forge/internal/cli"
	"forge/internal/gitctx"
)

// createDefaults carries the fully-resolved inputs to POST /pulls. Title and
// Base are proven non-empty by ResolveCreateDefaults; Head may still be "" only
// when the caller cannot run inside a repo (pr create errors before
// constructing createDefaults in that case).
type createDefaults struct {
	Title, Head, Base string
}

// ResolveCreateDefaults applies the release v0.3.1 §3 precedence for title,
// head, and base (first match wins):
//
//	Title: --title flag → commit subject at HEAD (inferred; requires at least
//	       one unique commit vs the resolved base)
//	Head:  headFlag → $FORGE_HEAD → current git branch of repo
//	Base:  baseFlag → $FORGE_BASE → cfgBase ([defaults].base) → origin/HEAD →
//	       apiBase() (server default_branch) → "no base branch" error
//
// apiBase fetches the server default branch via GetRepository and may be nil in
// tests. An apiBase error or empty result falls through to the no-base error;
// it is not propagated.
func ResolveCreateDefaults(titleFlag, headFlag, baseFlag string, repo *gitctx.Repo, cfgBase string, apiBase func() (string, error)) (createDefaults, error) {
	head := headFlag
	if head == "" {
		head = os.Getenv("FORGE_HEAD")
	}
	if head == "" && repo != nil {
		head = gitctx.CurrentBranch(repo.Root)
	}
	if head == "" {
		return createDefaults{}, &cli.Error{Code: cli.ExitUsage, Msg: "cannot determine head branch", Hint: "pass --head or run inside a repository on a branch"}
	}

	base := baseFlag
	if base == "" {
		base = os.Getenv("FORGE_BASE")
	}
	if base == "" {
		base = cfgBase
	}
	if base == "" && repo != nil {
		base = gitctx.RemoteHead(repo.Root)
	}
	if base == "" && apiBase != nil {
		base, _ = apiBase()
	}
	if base == "" {
		return createDefaults{}, &cli.Error{Code: cli.ExitUsage, Msg: "no base branch", Hint: "cannot determine base branch; pass --base"} // exit 2 per releases/v0.3.1.md §3
	}

	title := titleFlag
	inferred := false
	switch {
	case title != "":
	case repo == nil:
		return createDefaults{}, &cli.Error{Code: cli.ExitUsage, Msg: "no commits to title this pull request", Hint: "pass --title, or commit on the branch first"}
	default:
		title = gitctx.CommitSubject(repo.Root, "HEAD")
		inferred = true
	}
	if inferred {
		n, err := gitctx.UniqueCommitCount(repo.Root, base, "HEAD")
		if err != nil || n == 0 || title == "" {
			return createDefaults{}, &cli.Error{Code: cli.ExitUsage, Msg: "no commits to title this pull request", Hint: "pass --title, or commit on the branch first"}
		}
	}
	return createDefaults{Title: title, Head: head, Base: base}, nil
}
