package cmds

import (
	"errors"
	"fmt"
	"net/http"

	"forge/internal/api"
	"forge/internal/cli"
	"forge/internal/table"
)

// fetchConversation gathers issue-level comments, reviews, and each review's
// inline comments. Used by pr conv and pr pull.
func fetchConversation(ctx *cli.Ctx, index int) ([]api.Comment, []api.Review, map[int64][]api.ReviewComment, error) {
	o, r := ctx.GlobalFlags.Owner, ctx.GlobalFlags.Repo
	comments, err := ctx.API.GetIssueComments(o, r, index)
	if err != nil {
		return nil, nil, nil, err
	}
	reviews, err := ctx.API.GetReviews(o, r, index)
	if err != nil {
		return nil, nil, nil, err
	}
	perReview := make(map[int64][]api.ReviewComment, len(reviews))
	for _, rev := range reviews {
		rcs, err := ctx.API.GetReviewComments(o, r, index, int(rev.ID))
		if err != nil {
			return nil, nil, nil, err
		}
		perReview[rev.ID] = rcs
	}
	return comments, reviews, perReview, nil
}

// ---- pr create ----

type prCreateCmd struct{}

func (prCreateCmd) Name() string      { return "pr create" }
func (prCreateCmd) Summary() string   { return "open a pull request (auto-titled from branch tip)" }
func (prCreateCmd) RequiresAPI() bool { return true }

func (prCreateCmd) Run(args []string, ctx *cli.Ctx) error {
	title, _ := flagValue(args, "--title")
	head, _ := flagValue(args, "--head")
	base, _ := flagValue(args, "--base")
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
	d, err := ResolveCreateDefaults(title, head, base, ctx.Repo, cfgBase, apiBase)
	if err != nil {
		return err
	}
	body, _ := flagValue(args, "--body")

	pr, err := ctx.API.CreatePullRequest(ctx.GlobalFlags.Owner, ctx.GlobalFlags.Repo,
		api.CreatePRInput{Title: d.Title, Head: d.Head, Base: d.Base, Body: body})
	if err != nil {
		return mapCreateErr(ctx, ctx.GlobalFlags.Owner, ctx.GlobalFlags.Repo, d.Head, d.Base, err)
	}
	return writeJSON(ctx.Stdout, pr)
}

// serverMessage quotes an APIError's body text, with a fallback when the
// server sent none.
func serverMessage(e *api.APIError) string {
	if e != nil && e.Message != "" {
		return e.Message
	}
	return "(no message in response body)"
}

// mapCreateErr converts a CreatePullRequest failure into a typed cli error,
// disambiguating 404s per releases/v0.1.2.md: base missing, head missing,
// or pull requests disabled. Any other status passes through mapErr
// unchanged.
func mapCreateErr(ctx *cli.Ctx, owner, repo, head, base string, err error) error {
	var apiErr *api.APIError
	if !errors.As(err, &apiErr) || apiErr.Status != http.StatusNotFound {
		return mapErr(err)
	}
	if ok, perr := ctx.API.BranchExists(owner, repo, base); perr == nil && !ok {
		return &cli.Error{
			Code: cli.ExitContext,
			Msg:  fmt.Sprintf("base branch %q not found in %s/%s", base, owner, repo),
			Hint: fmt.Sprintf("POST /repos/%s/%s/pulls returned 404; GET .../branches?branch=%s also 404'd; server said: %q. Check --base / FORGE_BASE / [defaults] base.", owner, repo, base, serverMessage(apiErr)),
		}
	}
	if ok, perr := ctx.API.BranchExists(owner, repo, head); perr == nil && !ok {
		return &cli.Error{
			Code: cli.ExitContext,
			Msg:  fmt.Sprintf("head branch %q not found in %s/%s", head, owner, repo),
			Hint: fmt.Sprintf("POST /repos/%s/%s/pulls returned 404; GET .../branches?branch=%s also 404'd; server said: %q. Check --head / FORGE_HEAD / your current git branch.", owner, repo, head, serverMessage(apiErr)),
		}
	}
	return &cli.Error{
		Code: cli.ExitContext,
		Msg:  fmt.Sprintf("repository %s/%s does not accept pull requests", owner, repo),
		Hint: fmt.Sprintf("branches verified but POST /repos/%s/%s/pulls returned 404; server said: %q. Pull requests are likely disabled for this repo (or it is a mirror); enable them in repo Settings, or check --repo.", owner, repo, serverMessage(apiErr)),
	}
}

// ---- pr get ----

func (prCreateCmd) HelpPage() string {
	return `use: forge pr create [--title T] [--head B] [--base B] [--body TEXT]

Open a pull request. Head defaults to $FORGE_HEAD, then your current git
branch. Title defaults to the branch tip commit subject when omitted.
Base defaults to --base, then $FORGE_BASE, then [defaults].base in
config, then origin/HEAD, then the server default_branch.

A 404 here is diagnosed: missing base branch, missing head branch, or pull
requests disabled for the repo.`
}

type prGetCmd struct{}

func (prGetCmd) Name() string      { return "pr get" }
func (prGetCmd) Summary() string   { return "print one pull request as JSON (usage: pr get N)" }
func (prGetCmd) RequiresAPI() bool { return true }

func (prGetCmd) Run(args []string, ctx *cli.Ctx) error {
	n, err := parseIndex(args, "pr get")
	if err != nil {
		return err
	}
	pr, err := ctx.API.GetPullRequest(ctx.GlobalFlags.Owner, ctx.GlobalFlags.Repo, n)
	if err != nil {
		return mapErr(err)
	}
	return writeJSON(ctx.Stdout, pr)
}

// ---- pr list ----

func (prGetCmd) HelpPage() string {
	return `use: forge pr get N

Print one pull request as JSON. N is the PR number.`
}

type prListCmd struct{}

func (prListCmd) Name() string { return "pr list" }
func (prListCmd) Summary() string {
	return "list pull requests as a JSON array [--state --page --limit]"
}
func (prListCmd) RequiresAPI() bool    { return true }
func (prListCmd) DefaultIsTable() bool { return true }

func (prListCmd) Run(args []string, ctx *cli.Ctx) error {
	state, _ := flagValue(args, "--state")
	page := intFlag(args, "--page", 1)
	limit := intFlag(args, "--limit", 0)
	prs, err := ctx.API.ListPullRequests(ctx.GlobalFlags.Owner, ctx.GlobalFlags.Repo, state, page, limit)
	if err != nil {
		return mapErr(err)
	}
	if ctx.OutputIsJSON(ctx.Stdout, true) {
		return writeJSON(ctx.Stdout, prs)
	}
	return table.Render(ctx.Stdout, prListColumns, prListRows(prs))
}

func (prListCmd) HelpPage() string {
	return `use: forge pr list [--state open|closed|all] [--page N] [--limit M]

List pull requests as a JSON array. Defaults: state open, page 1, no limit.
Prints a table on an interactive terminal; JSON elsewhere. --json forces JSON,
--table forces the table.`
}

// ---- deprecation shim ----

// deprecatedPrConvCmd replaces the v0.2.x "pr conversation" command, which was
// removed in v0.3.0 in favour of "pr conv". It never touches the API.
type deprecatedPrConvCmd struct{}

func (deprecatedPrConvCmd) Name() string    { return "pr conversation" }
func (deprecatedPrConvCmd) Summary() string { return "removed: use forge pr conv" }

func (deprecatedPrConvCmd) Run([]string, *cli.Ctx) error {
	return &cli.Error{
		Code: cli.ExitUsage,
		Msg:  "pr conversation was removed in v0.3.0",
		Hint: "use forge pr conv N (unresolved-first view), or forge pr pull N to download",
	}
}

func (deprecatedPrConvCmd) HelpPage() string {
	return `use: forge pr conversation (removed)

pr conversation was removed in v0.3.0. Use forge pr conv N instead, or
forge pr pull N to download the conversation.`
}

// PRCommands returns the pr subcommands for registration in main.
func PRCommands() []cli.Command {
	return []cli.Command{
		prCreateCmd{}, createBatchCmd{}, prGetCmd{}, prListCmd{}, prConvCmd{},
		reviewListCmd{}, resolveCmd{unresolve: false}, resolveCmd{unresolve: true},
		resolveAllCmd{},
		deprecatedPrConvCmd{},
	}
}
