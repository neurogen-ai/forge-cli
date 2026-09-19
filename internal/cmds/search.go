package cmds

import (
	"strconv"

	"forge/internal/api"
	"forge/internal/cli"
	"forge/internal/table"
)

// searchKind captures everything that differs between the two searchable
// kinds: the query's type parameter and the table rendering. Rows reuse the
// existing issue/pr list specs; a new kind is one entry here.
type searchKind struct {
	kind    string // "issues" or "prs"; becomes the query type parameter
	columns []table.Column
	rows    func([]api.Issue) [][]string
}

var searchKinds = map[string]searchKind{
	"issues": {kind: "issues", columns: issueListColumns, rows: issueListRows},
	"prs":    {kind: "pulls", columns: prListColumns, rows: searchPRRows},
}

// searchPRRows renders prListColumns from issue-shaped search payloads:
// Forgejo's search returns issues for type=pulls too, and prListRows' UPDATED
// column draws on CreatedAt the same way here as prListRows does there.
func searchPRRows(iss []api.Issue) [][]string {
	rows := make([][]string, 0, len(iss))
	for _, i := range iss {
		rows = append(rows, []string{
			strconv.FormatInt(i.Number, 10),
			i.Title,
			i.State,
			i.User.Login,
			timeShort(i.CreatedAt),
		})
	}
	return rows
}

// searchCmd is the one implementation behind both spellings. kind selects
// the searchKinds entry.
type searchCmd struct{ kind string } // "issues" or "prs"

func (c searchCmd) Name() string { return "search " + c.kind }
func (c searchCmd) Summary() string {
	return "search " + c.kind + " in this repository (usage: search " + c.kind + " QUERY [--state S])"
}
func (searchCmd) RequiresAPI() bool    { return true }
func (searchCmd) DefaultIsTable() bool { return true }

func (c searchCmd) Run(args []string, ctx *cli.Ctx) error {
	rest := stripFlags(args, "--state", "--page", "--limit")
	if len(rest) != 1 {
		return &cli.Error{
			Code: cli.ExitUsage,
			Msg:  "search " + c.kind + " requires exactly one QUERY",
			Hint: "example: forge search " + c.kind + " \"crash on save\" --state open",
		}
	}
	spec, ok := searchKinds[c.kind]
	if !ok {
		return &cli.Error{Code: cli.ExitUsage, Msg: "unknown search kind " + c.kind}
	}
	state, _ := flagValue(args, "--state")
	page := intFlag(args, "--page", 1)
	limit := intFlag(args, "--limit", 0)
	issues, err := ctx.API.SearchIssues(ctx.GlobalFlags.Owner, ctx.GlobalFlags.Repo, rest[0], spec.kind, state, page, limit)
	if err != nil {
		return mapErr(err)
	}
	if ctx.OutputIsJSON(ctx.Stdout, true) {
		return writeJSON(ctx.Stdout, issues)
	}
	return table.Render(ctx.Stdout, spec.columns, spec.rows(issues))
}

func (searchCmd) HelpPage() string {
	return `use: forge search issues QUERY [--state open|closed|all] [--page N] [--limit M]
      forge search prs QUERY [--state open|closed|all] [--page N] [--limit M]

Search this repository's issues or pull requests for QUERY and print the
matches as a JSON array. Prints a table on an interactive terminal; JSON
elsewhere. --json forces JSON, --table forces the table. Search is
repository-scoped; there is no cross-repo mode.`
}

// SearchCommands registers both spellings over the one implementation.
func SearchCommands() []cli.Command {
	return []cli.Command{searchCmd{kind: "issues"}, searchCmd{kind: "prs"}}
}
