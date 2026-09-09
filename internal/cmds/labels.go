package cmds

import (
	"strings"

	"forge/internal/cli"
)

// collectLabelNames returns repeated --label values in command-line order,
// the only name-collection path in the command layer. A trailing bare
// --label with no value is ignored, matching the issue-create behaviour
// this collector was extracted from.
func collectLabelNames(args []string, cmdName string) ([]string, error) {
	var names []string
	for i := 0; i < len(args); i++ {
		if args[i] == "--label" && i+1 < len(args) {
			names = append(names, args[i+1])
			i++
		}
	}
	return names, nil
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
