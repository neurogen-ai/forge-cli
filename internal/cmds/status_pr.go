package cmds

import (
	"forge/internal/api"
	"forge/internal/cli"
)

// openPRCache fetches the repository's open pull requests once and shares
// them across the PR sections, so one dashboard run lists PRs exactly once
// no matter how many sections need them.
type openPRCache struct {
	fetched bool
	prs     []api.PullRequest
}

func (c *openPRCache) get(ctx *cli.Ctx) ([]api.PullRequest, error) {
	if c.fetched {
		return c.prs, nil
	}
	prs, err := ctx.API.ListPullRequests(ctx.GlobalFlags.Owner, ctx.GlobalFlags.Repo, "open", 0, 0)
	if err != nil {
		return nil, err
	}
	c.prs, c.fetched = prs, true
	return c.prs, nil
}

// reviewRequestSection lists open PRs whose RequestedReviewers includes the
// current user: review requests awaiting the caller.
type reviewRequestSection struct{ cache *openPRCache }

func (reviewRequestSection) Title() string { return "Review requests awaiting you" }

func (s reviewRequestSection) Fetch(ctx *cli.Ctx, me *api.User) ([]statusEntry, error) {
	prs, err := s.cache.get(ctx)
	if err != nil {
		return nil, err
	}
	var out []statusEntry
	for _, p := range prs {
		for _, u := range p.RequestedReviewers {
			if u.Login == me.Login {
				out = append(out, statusEntry{
					Kind:   "pr",
					Index:  p.Number,
					Title:  p.Title,
					State:  p.State,
					Reason: "review-requested",
					URL:    p.HTMLURL,
				})
				break
			}
		}
	}
	return out, nil
}

// assignedPRSection lists open PRs whose assignees include the current
// user. The assignees slice rides the PullRequest subset struct.
type assignedPRSection struct{ cache *openPRCache }

func (assignedPRSection) Title() string { return "Pull requests assigned to you" }

func (s assignedPRSection) Fetch(ctx *cli.Ctx, me *api.User) ([]statusEntry, error) {
	prs, err := s.cache.get(ctx)
	if err != nil {
		return nil, err
	}
	var out []statusEntry
	for _, p := range prs {
		for _, u := range p.Assignees {
			if u.Login == me.Login {
				out = append(out, statusEntry{
					Kind:   "pr",
					Index:  p.Number,
					Title:  p.Title,
					State:  p.State,
					Reason: "assigned",
					URL:    p.HTMLURL,
				})
				break
			}
		}
	}
	return out, nil
}
