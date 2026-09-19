package cmds

import (
	"forge/internal/api"
	"forge/internal/cli"
)

// openIssuesSection lists the repository's open issues: everything not yet
// resolved, filtered to nothing. It reuses the issue list machinery
// (ListIssues already sends type=issues, so PRs never leak in).
type openIssuesSection struct{}

func (openIssuesSection) Title() string { return "Open issues" }

func (openIssuesSection) Fetch(ctx *cli.Ctx, me *api.User) ([]statusEntry, error) {
	issues, err := ctx.API.ListIssues(ctx.GlobalFlags.Owner, ctx.GlobalFlags.Repo, "open", 0, 0)
	if err != nil {
		return nil, err
	}
	out := make([]statusEntry, 0, len(issues))
	for _, i := range issues {
		out = append(out, statusEntry{
			Kind:   "issue",
			Index:  i.Number,
			Title:  i.Title,
			State:  i.State,
			Reason: "open",
			URL:    i.HTMLURL,
		})
	}
	return out, nil
}
