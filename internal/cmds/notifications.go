package cmds

import (
	"forge/internal/api"
	"forge/internal/cli"
	"forge/internal/table"
)

// notificationListColumns is the notifications table spec: subject, repo,
// kind. The spec lives here beside the command; internal/table owns layout.
var notificationListColumns = []table.Column{
	{Name: "SUBJECT", Width: 40},
	{Name: "REPO", Width: 25},
	{Name: "KIND", Width: 12},
}

func notificationListRows(ns []api.Notification) [][]string {
	rows := make([][]string, len(ns))
	for i, n := range ns {
		rows[i] = []string{n.Subject.Title, n.Repository.FullName, n.Subject.Type}
	}
	return rows
}

type notificationListCmd struct{}

func (notificationListCmd) Name() string         { return "notifications" }
func (notificationListCmd) Summary() string      { return "list notifications, unread first [--all]" }
func (notificationListCmd) RequiresAPI() bool    { return true }
func (notificationListCmd) DefaultIsTable() bool { return true }

func (notificationListCmd) HelpPage() string {
	return `use: forge notifications [--all]

List notifications, unread first. The server returns unread entries by
default; --all includes read ones. Read state stays server-side: listing
never marks anything read, and nothing is written to disk. Prints a table
on an interactive terminal; JSON elsewhere.`
}

func (notificationListCmd) Run(args []string, ctx *cli.Ctx) error {
	ns, err := ctx.API.ListNotifications(hasFlagToken(args, "--all"))
	if err != nil {
		return mapErr(err)
	}
	if ctx.OutputIsJSON(ctx.Stdout, true) {
		return writeJSON(ctx.Stdout, ns)
	}
	return table.Render(ctx.Stdout, notificationListColumns, notificationListRows(ns))
}
