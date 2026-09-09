package cmds

import (
	"strconv"

	"forge/internal/api"
	"forge/internal/cli"
	"forge/internal/table"
)

// ---- release list ----

type releaseListCmd struct{}

func (releaseListCmd) Name() string { return "release list" }
func (releaseListCmd) Summary() string {
	return "list repository releases (table on a TTY, JSON elsewhere)"
}
func (releaseListCmd) RequiresAPI() bool    { return true }
func (releaseListCmd) DefaultIsTable() bool { return true }

func (releaseListCmd) HelpPage() string {
	return `use: forge release list

List repository releases in server order. Prints a TAG/NAME/DRAFT/PRERELEASE/
PUBLISHED table on an interactive terminal; JSON elsewhere. --json forces
JSON, --table forces the table.`
}

func (releaseListCmd) Run(args []string, ctx *cli.Ctx) error {
	rels, err := ctx.API.ListReleases(ctx.GlobalFlags.Owner, ctx.GlobalFlags.Repo)
	if err != nil {
		return mapErr(err)
	}
	if ctx.OutputIsJSON(ctx.Stdout, true) {
		return writeJSON(ctx.Stdout, rels)
	}
	return table.Render(ctx.Stdout, releaseListColumns, releaseListRows(rels))
}

// ---- release get ----

type releaseGetCmd struct{}

func (releaseGetCmd) Name() string         { return "release get" }
func (releaseGetCmd) Summary() string      { return "print one release by tag as JSON" }
func (releaseGetCmd) RequiresAPI() bool    { return true }
func (releaseGetCmd) DefaultIsTable() bool { return false }

func (releaseGetCmd) HelpPage() string {
	return `use: forge release get TAG

Print release TAG as JSON. Tags containing "/" are sent URL-path escaped.`
}

func (releaseGetCmd) Run(args []string, ctx *cli.Ctx) error {
	if len(args) == 0 {
		return &cli.Error{Code: cli.ExitUsage, Msg: "release get requires a tag"}
	}
	tag := args[0]
	rel, err := ctx.API.GetRelease(ctx.GlobalFlags.Owner, ctx.GlobalFlags.Repo, tag)
	if err != nil {
		return mapErr(err)
	}
	return writeJSON(ctx.Stdout, rel)
}

// ReleaseCommands returns the release subcommands for registration in main.
func ReleaseCommands() []cli.Command {
	return []cli.Command{
		releaseListCmd{},
		releaseGetCmd{},
	}
}

// ---- release views ----

var releaseListColumns = []table.Column{
	{Name: "TAG", Width: 14},
	{Name: "NAME", Width: 36},
	{Name: "DRAFT", Width: 5},
	{Name: "PRERELEASE", Width: 10},
	{Name: "PUBLISHED", Width: 10},
}

func releaseListRows(rels []api.Release) [][]string {
	rows := make([][]string, 0, len(rels))
	for _, r := range rels {
		rows = append(rows, []string{
			r.TagName,
			r.Name,
			strconv.FormatBool(r.Draft),
			strconv.FormatBool(r.Prerelease),
			timeShort(r.PublishedAt),
		})
	}
	return rows
}
