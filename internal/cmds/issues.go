package cmds

import (
	"strings"

	"forge/internal/api"
	"forge/internal/cli"
	"forge/internal/table"
)

// ---- issue create ----

type issueCreateCmd struct{}

func (issueCreateCmd) Name() string { return "issue create" }
func (issueCreateCmd) Summary() string {
	return "open an issue (--title required; repeatable --label NAME)"
}
func (issueCreateCmd) RequiresAPI() bool { return true }

func (issueCreateCmd) Run(args []string, ctx *cli.Ctx) error {
	title, ok := flagValue(args, "--title")
	if !ok || title == "" {
		return &cli.Error{Code: cli.ExitUsage, Msg: "issue create requires --title"}
	}
	body, _ := flagValue(args, "--body")

	var labelNames []string
	for i := 0; i < len(args); i++ {
		if args[i] == "--label" && i+1 < len(args) {
			labelNames = append(labelNames, args[i+1])
			i++
		}
	}

	var labelIDs []int
	if len(labelNames) > 0 {
		labels, err := ctx.API.ListLabels(ctx.GlobalFlags.Owner, ctx.GlobalFlags.Repo)
		if err != nil {
			return mapErr(err)
		}
		byName := make(map[string]int64, len(labels))
		for _, l := range labels {
			byName[l.Name] = l.ID
		}
		var unknown []string
		for _, name := range labelNames {
			id, found := byName[name]
			if !found {
				unknown = append(unknown, name)
				continue
			}
			labelIDs = append(labelIDs, int(id))
		}
		if len(unknown) > 0 {
			return &cli.Error{
				Code: cli.ExitRuntime,
				Msg:  "unknown labels: " + strings.Join(unknown, ", "),
				Hint: "list repository labels to see valid names",
			}
		}
	}

	iss, err := ctx.API.CreateIssue(ctx.GlobalFlags.Owner, ctx.GlobalFlags.Repo,
		api.CreateIssueInput{Title: title, Body: body, Labels: labelIDs})
	if err != nil {
		return mapErr(err)
	}
	return writeJSON(ctx.Stdout, iss)
}

func (issueCreateCmd) HelpPage() string {
	return `use: forge issue create --title T [--body TEXT] [--label NAME]...

Open an issue. --label repeats; names must exist on the repo (unknown names
are rejected before the request is sent).`
}

// ---- issue get ----

type issueGetCmd struct{}

func (issueGetCmd) Name() string      { return "issue get" }
func (issueGetCmd) Summary() string   { return "print one issue as JSON (usage: issue get N)" }
func (issueGetCmd) RequiresAPI() bool { return true }

func (issueGetCmd) Run(args []string, ctx *cli.Ctx) error {
	n, err := parseIndex(args, "issue get")
	if err != nil {
		return err
	}
	iss, err := ctx.API.GetIssue(ctx.GlobalFlags.Owner, ctx.GlobalFlags.Repo, n)
	if err != nil {
		return mapErr(err)
	}
	return writeJSON(ctx.Stdout, iss)
}

func (issueGetCmd) HelpPage() string {
	return `use: forge issue get N

Print one issue as JSON.`
}

// ---- issue list ----

type issueListCmd struct{}

func (issueListCmd) HelpPage() string {
	return `use: forge issue list [--state open|closed|all] [--page N] [--limit M]

List issues as a JSON array. Defaults: state open, page 1, no limit.
Prints a table on an interactive terminal; JSON elsewhere. --json forces JSON,
--table forces the table.`
}

func (issueListCmd) Name() string         { return "issue list" }
func (issueListCmd) Summary() string      { return "list issues as a JSON array [--state --page --limit]" }
func (issueListCmd) RequiresAPI() bool    { return true }
func (issueListCmd) DefaultIsTable() bool { return true }

func (issueListCmd) Run(args []string, ctx *cli.Ctx) error {
	state, _ := flagValue(args, "--state")
	page := intFlag(args, "--page", 1)
	limit := intFlag(args, "--limit", 0)
	issues, err := ctx.API.ListIssues(ctx.GlobalFlags.Owner, ctx.GlobalFlags.Repo, state, page, limit)
	if err != nil {
		return mapErr(err)
	}
	if ctx.OutputIsJSON(ctx.Stdout, true) {
		return writeJSON(ctx.Stdout, issues)
	}
	return table.Render(ctx.Stdout, issueListColumns, issueListRows(issues))
}

// ---- issue close/open ----

type issueStateCmd struct{ closing bool }

func (c issueStateCmd) Name() string {
	if c.closing {
		return "issue close"
	}
	return "issue open"
}
func (c issueStateCmd) Summary() string {
	if c.closing {
		return "close an issue (usage: issue close N)"
	}
	return "reopen an issue (usage: issue open N)"
}
func (issueStateCmd) RequiresAPI() bool { return true }

func (c issueStateCmd) HelpPage() string {
	if c.closing {
		return `use: forge issue close N

Close issue N.`
	}
	return `use: forge issue open N

Reopen (re-open) issue N.`
}

func (c issueStateCmd) Run(args []string, ctx *cli.Ctx) error {
	name := "issue open"
	state := "open"
	if c.closing {
		name = "issue close"
		state = "closed"
	}
	n, err := parseIndex(args, name)
	if err != nil {
		return err
	}
	iss, err := ctx.API.SetIssueState(ctx.GlobalFlags.Owner, ctx.GlobalFlags.Repo, n, state)
	if err != nil {
		return mapErr(err)
	}
	return writeJSON(ctx.Stdout, iss)
}

// IssueCommands returns the issue subcommands for registration in main.
func IssueCommands() []cli.Command {
	return []cli.Command{
		issueCreateCmd{}, issueGetCmd{}, issueListCmd{},
		issueStateCmd{closing: true}, issueStateCmd{closing: false},
		commentAddCmd{kind: "issue"},
	}
}
