package cmds

import (
	"fmt"

	"forge/internal/cli"
	"forge/internal/table"
)

// prChecksCmd shows commit statuses (always) and Actions runs (when the
// instance has Actions) for one pull request's head ref. The head ref is
// resolved once, via the same GetPullRequest path every pr command uses.
// A missing Actions capability is data, not an error: the command prints a
// note to stderr and still exits 0 with the statuses.
type prChecksCmd struct{}

func (prChecksCmd) Name() string { return "pr checks" }
func (prChecksCmd) Summary() string {
	return "show commit statuses and Actions runs for one pull request (usage: pr checks N)"
}
func (prChecksCmd) RequiresAPI() bool    { return true }
func (prChecksCmd) DefaultIsTable() bool { return true }

var prChecksColumns = []table.Column{
	{Name: "STATE", Width: 9},
	{Name: "CONTEXT", Width: 24},
	{Name: "DESCRIPTION", Width: 40},
	{Name: "UPDATED", Width: 20},
}

// checkRow is one merged check line: a commit status or an Actions run.
type checkRow struct {
	kind        string // "status" or "run"
	state       string
	context     string
	description string
	detail      string // target URL for statuses, run URL for runs
}

func prChecksRows(rows []checkRow) [][]string {
	out := make([][]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, []string{r.kind + "/" + r.state, r.context, r.description, r.detail})
	}
	return out
}

func (prChecksCmd) Run(args []string, ctx *cli.Ctx) error {
	n, err := parseIndex(args, "pr checks")
	if err != nil {
		return err
	}
	owner, repo := ctx.GlobalFlags.Owner, ctx.GlobalFlags.Repo

	// One head-ref resolution: statuses and runs both hang off the PR head.
	pr, err := ctx.API.GetPullRequest(owner, repo, n)
	if err != nil {
		return mapErr(err)
	}
	ref := pr.Head.Sha
	if ref == "" {
		ref = pr.Head.Ref
	}
	if ref == "" {
		return &cli.Error{
			Code: cli.ExitRuntime,
			Msg:  fmt.Sprintf("pr checks: pull request %d has no head ref or sha", n),
		}
	}

	statuses, err := ctx.API.ListCommitStatuses(owner, repo, ref)
	if err != nil {
		return mapErr(err)
	}
	runs, available, err := ctx.API.ListRunsForRef(owner, repo, ref)
	if err != nil {
		return mapErr(err)
	}
	if !available {
		fmt.Fprintln(ctx.Stderr, "note: Actions not available on this instance")
	}

	// Deterministic merge: statuses first, then runs, server order within
	// each (the statuses endpoint returns most recent first).
	rows := make([]checkRow, 0, len(statuses)+len(runs))
	for _, s := range statuses {
		rows = append(rows, checkRow{kind: "status", state: s.State, context: s.Context, description: s.Description, detail: s.TargetURL})
	}
	for _, r := range runs {
		rows = append(rows, checkRow{kind: "run", state: r.Status, context: r.Name, description: "Actions run #" + fmt.Sprint(r.ID), detail: r.URL})
	}

	if ctx.OutputIsJSON(ctx.Stdout, true) {
		return writeJSON(ctx.Stdout, map[string]any{
			"ref":               ref,
			"head_sha":          pr.Head.Sha,
			"statuses":          statuses,
			"runs":              runs,
			"actions_available": available,
		})
	}
	return table.Render(ctx.Stdout, prChecksColumns, prChecksRows(rows))
}

func (prChecksCmd) HelpPage() string {
	return `use: forge pr checks N

Show commit statuses and Forgejo Actions runs for the head ref of pull
request N. Statuses exist on every instance; when Actions is not available
the command prints "note: Actions not available on this instance" and still
exits 0 with the statuses. This is a read: no polling, no gating, no
re-runs (dispatch is a later release). Table by default on a TTY, JSON
elsewhere.`
}
