package cmds

import (
	"fmt"
	"path"
	"sort"

	"forge/internal/cli"
	"forge/internal/gitctx"
)

// BatchReceiptItem is one planned or completed PR creation. In a dry-run
// plan only Branch/Title/Base are set; after --yes, URL/Number fill in on
// success and Error carries the server's message verbatim on failure. The
// plan and the receipt are the same type so the two outputs never drift.
type BatchReceiptItem struct {
	Branch string `json:"branch"`
	Title  string `json:"title"`
	Base   string `json:"base"`
	Number int64  `json:"number,omitempty"`
	URL    string `json:"url,omitempty"`
	Error  string `json:"error,omitempty"`
}

type createBatchCmd struct{}

func (createBatchCmd) Name() string { return "pr create-batch" }
func (createBatchCmd) Summary() string {
	return "open pull requests for branches matching a pattern (dry-run by default, --yes to post)"
}
func (createBatchCmd) RequiresAPI() bool { return true }

func (createBatchCmd) HelpPage() string {
	return `use: forge pr create-batch [--yes] [--base B] [--body TEXT] PATTERN

Open pull requests for every local branch matching PATTERN (path.Match
glob semantics, e.g. "v0.3.0*"). Without --yes nothing is posted: the
command prints a dry-run plan as JSON (branch, title, base per item).
Branches whose tip commit has no subject are skipped with a note on
stderr. --base follows the same precedence as pr create; each head is
the matching branch itself. --yes posts one pull request per planned
branch.`
}

func (createBatchCmd) Run(args []string, ctx *cli.Ctx) error {
	yes := flagBool(args, "--yes") // parsed here; --yes posting lands in the next step
	baseFlag, _ := flagValue(args, "--base")
	_, _ = flagValue(args, "--body") // body is reserved for the --yes step
	_ = yes
	var positional []string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--yes": // boolean flag, no value
		case "--base", "--body": // paired flags: skip name and its value
			if i+1 < len(args) {
				i++
			}
		default:
			positional = append(positional, args[i])
		}
	}
	if len(positional) == 0 {
		return &cli.Error{
			Code: cli.ExitUsage,
			Msg:  "pr create-batch requires PATTERN",
			Hint: "usage: forge pr create-batch [--yes] [--base B] [--body TEXT] PATTERN",
		}
	}
	pattern := positional[0]

	if ctx.Repo == nil {
		return &cli.Error{
			Code: cli.ExitContext,
			Msg:  "not inside a git repository",
			Hint: "forge must run inside a git repository for create-batch",
		}
	}
	matched := []string{}
	for _, name := range gitctx.LocalBranches(ctx.Repo.Root) {
		ok, err := path.Match(pattern, name)
		if err != nil {
			return &cli.Error{
				Code: cli.ExitUsage,
				Msg:  fmt.Sprintf("invalid PATTERN %q", pattern),
				Hint: "pattern must be a valid glob (path.Match syntax), e.g. \"v0.3.0*\"",
			}
		}
		if !ok {
			continue
		}
		matched = append(matched, name)
	}
	if len(matched) == 0 {
		return &cli.Error{
			Code: cli.ExitUsage,
			Msg:  fmt.Sprintf("no branches match %q", pattern),
			Hint: "usage: forge pr create-batch [--yes] [--base B] [--body TEXT] PATTERN",
		}
	}
	sort.Strings(matched)

	cfgBase := ""
	if ctx.Cfg != nil {
		cfgBase = ctx.Cfg.Defaults.Base
	}
	apiBase := func() (string, error) {
		repo, err := ctx.API.GetRepository(ctx.GlobalFlags.Owner, ctx.GlobalFlags.Repo)
		if err != nil {
			return "", err
		}
		return repo.DefaultBranch, nil
	}
	// headFlag is set to matched[0]: batch head will be each branch name at
	// POST time, so the resolver's head leg is unused in the dry-run plan;
	// passing the first branch keeps it deterministic instead of consulting
	// FORGE_HEAD / CurrentBranch. titleFlag uses a placeholder rather than ""
	// because an empty titleFlag makes the resolver infer and validate a title
	// from HEAD, which batch does not want (titles are computed per-branch via
	// CommitSubject below); only Base (and error shapes) are reused.
	d, err := ResolveCreateDefaults("-", matched[0], baseFlag, ctx.Repo, cfgBase, apiBase)
	if err != nil {
		return err
	}

	items := []BatchReceiptItem{}
	for _, branch := range matched {
		title := gitctx.CommitSubject(ctx.Repo.Root, branch)
		if title == "" {
			fmt.Fprintf(ctx.Stderr, "skipped: %s (no commit subject)\n", branch)
			continue
		}
		items = append(items, BatchReceiptItem{Branch: branch, Title: title, Base: d.Base})
	}
	if len(items) == 0 {
		return &cli.Error{
			Code: cli.ExitUsage,
			Msg:  "no commits to title any matching branch",
			Hint: "commit on the matching branches first, or adjust PATTERN",
		}
	}
	return writeJSON(ctx.Stdout, items)
}
