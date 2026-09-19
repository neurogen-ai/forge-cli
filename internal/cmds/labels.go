package cmds

import (
	"strings"

	"forge/internal/cli"
	"forge/internal/table"
)

// collectLabelNames returns repeated --label values in command-line order,
// delegating to the one repeated-value-flag collector in cmds.go. A trailing
// bare --label with no value is ignored, matching the issue-create behaviour
// this collector was extracted from.
func collectLabelNames(args []string, cmdName string) ([]string, error) {
	return collectFlagValues(args, "--label"), nil
}

// resolveLabelIDs fetches repository labels once and maps exact names to
// ids, preserving input order and duplicates; unknown names are a runtime
// error listing them in input order.
func resolveLabelIDs(ctx *cli.Ctx, names []string) ([]int64, error) {
	if len(names) == 0 {
		return nil, nil
	}
	labels, err := ctx.API.ListLabels(ctx.GlobalFlags.Owner, ctx.GlobalFlags.Repo)
	if err != nil {
		return nil, mapErr(err)
	}
	byName := make(map[string]int64, len(labels))
	for _, l := range labels {
		byName[l.Name] = l.ID
	}
	ids := make([]int64, 0, len(names))
	var unknown []string
	for _, name := range names {
		id, found := byName[name]
		if !found {
			unknown = append(unknown, name)
			continue
		}
		ids = append(ids, id)
	}
	if len(unknown) > 0 {
		return nil, &cli.Error{
			Code: cli.ExitRuntime,
			Msg:  "unknown labels: " + strings.Join(unknown, ", "),
			Hint: "list repository labels to see valid names",
		}
	}
	return ids, nil
}

// LabelReceipt records the names the user asked for, in the same order,
// not the server's label payloads.
type LabelReceipt struct {
	Index  int64    `json:"index"`
	Action string   `json:"action"`
	Labels []string `json:"labels"`
}

// ---- label list ----

type labelListCmd struct{}

func (labelListCmd) Name() string { return "label list" }
func (labelListCmd) Summary() string {
	return "list repository labels (table on a TTY, JSON elsewhere)"
}
func (labelListCmd) RequiresAPI() bool    { return true }
func (labelListCmd) DefaultIsTable() bool { return true }

func (labelListCmd) HelpPage() string {
	return `use: forge label list

List repository labels. Prints a NAME/COLOR/ID table on an interactive
terminal; JSON elsewhere. --json forces JSON, --table forces the table.`
}

func (labelListCmd) Run(args []string, ctx *cli.Ctx) error {
	labels, err := ctx.API.ListLabels(ctx.GlobalFlags.Owner, ctx.GlobalFlags.Repo)
	if err != nil {
		return mapErr(err)
	}
	if ctx.OutputIsJSON(ctx.Stdout, true) {
		return writeJSON(ctx.Stdout, labels)
	}
	return table.Render(ctx.Stdout, labelListColumns, labelListRows(labels))
}

// ---- issue label add / remove ----

type issueLabelCmd struct{ adding bool }

func (c issueLabelCmd) Name() string {
	if c.adding {
		return "issue label add"
	}
	return "issue label remove"
}

func (c issueLabelCmd) Summary() string {
	if c.adding {
		return "add labels to an issue by name (usage: issue label add N --label NAME)"
	}
	return "remove labels from an issue by name (usage: issue label remove N --label NAME)"
}
func (issueLabelCmd) RequiresAPI() bool { return true }

func (c issueLabelCmd) HelpPage() string {
	if c.adding {
		return `use: forge issue label add N --label NAME...

Add labels to issue N by name. Names resolve against the repository label
list (exact match) and one request carries every resolved id.`
	}
	return `use: forge issue label remove N --label NAME...

Remove labels from issue N by name. Names resolve against the repository
label list (exact match); removal runs in the order given and stops on the
first failure.`
}

func (c issueLabelCmd) Run(args []string, ctx *cli.Ctx) error {
	name := c.Name()
	n, err := parseIndex(stripFlags(args, "--label"), name)
	if err != nil {
		return err
	}
	names, err := collectLabelNames(args, name)
	if err != nil {
		return err
	}
	if len(names) == 0 {
		return &cli.Error{Code: cli.ExitUsage, Msg: name + " requires at least one --label NAME"}
	}
	ids, err := resolveLabelIDs(ctx, names)
	if err != nil {
		return err
	}
	if c.adding {
		if _, err := ctx.API.AddLabels(ctx.GlobalFlags.Owner, ctx.GlobalFlags.Repo, n, ids); err != nil {
			return mapErr(err)
		}
	} else {
		for _, id := range ids {
			if err := ctx.API.RemoveLabel(ctx.GlobalFlags.Owner, ctx.GlobalFlags.Repo, n, id); err != nil {
				return mapErr(err)
			}
		}
	}
	action := "remove"
	if c.adding {
		action = "add"
	}
	return writeJSON(ctx.Stdout, LabelReceipt{Index: int64(n), Action: action, Labels: names})
}

// LabelCommands returns the label subcommands for registration in main.
func LabelCommands() []cli.Command {
	return []cli.Command{
		labelListCmd{},
		issueLabelCmd{adding: true},
		issueLabelCmd{adding: false},
	}
}
