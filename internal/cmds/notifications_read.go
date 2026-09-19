package cmds

import (
	"fmt"
	"strconv"

	"forge/internal/cli"
)

// ---- notifications read ----

// notificationsReadReceipt is the write receipt for notifications read: the
// IDs the server accepted in the batch request.
type notificationsReadReceipt struct {
	IDs    []int64 `json:"ids"`
	Action string  `json:"action"`
}

type notificationsReadCmd struct{}

func (notificationsReadCmd) Name() string { return "notifications read" }
func (notificationsReadCmd) Summary() string {
	return "mark notifications read by ID (usage: notifications read ID...)"
}
func (notificationsReadCmd) RequiresAPI() bool { return true }

func (notificationsReadCmd) HelpPage() string {
	return `use: forge notifications read ID...

Mark one or more notifications read by ID, in one batch request. Read state
is server-side only; no local ledger is kept. Prints a receipt of the IDs
accepted.`
}

func (notificationsReadCmd) Run(args []string, ctx *cli.Ctx) error {
	rest := stripFlags(args)
	if len(rest) == 0 {
		return &cli.Error{
			Code: cli.ExitUsage,
			Msg:  "notifications read requires one or more IDs",
			Hint: "example: forge notifications read 42 57",
		}
	}
	ids := make([]int64, 0, len(rest))
	for _, a := range rest {
		id, err := strconv.ParseInt(a, 10, 64)
		if err != nil || id <= 0 {
			return &cli.Error{
				Code: cli.ExitUsage,
				Msg:  fmt.Sprintf("%q is not a notification ID", a),
				Hint: "IDs are the numeric id fields from forge notifications",
			}
		}
		ids = append(ids, id)
	}
	if err := ctx.API.MarkNotificationsRead(ids); err != nil {
		return mapErr(err)
	}
	return writeJSON(ctx.Stdout, notificationsReadReceipt{IDs: ids, Action: "read"})
}

// NotificationsCommands returns the notifications subcommands for
// registration in main.
func NotificationsCommands() []cli.Command {
	return []cli.Command{
		notificationListCmd{},
		notificationsReadCmd{},
	}
}
