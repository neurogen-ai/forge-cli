package cmds

import (
	"forge/internal/api"
	"forge/internal/cli"
)

// labelManageReceipt is the delete receipt: the resolved name the user
// asked about, not a server payload (delete success has no body).
type labelManageReceipt struct {
	Name   string `json:"name"`
	Action string `json:"action"`
}

// ---- label create ----

type labelCreateCmd struct{}

func (labelCreateCmd) Name() string { return "label create" }
func (labelCreateCmd) Summary() string {
	return "create a repository label (usage: label create NAME [--color C] [--desc D])"
}
func (labelCreateCmd) RequiresAPI() bool { return true }

func (labelCreateCmd) HelpPage() string {
	return `use: forge label create NAME [--color C] [--desc D]

Create repository label NAME and print the created label JSON. Color is a
hex string the server validates; the default is the server's. --desc sets
the label description.`
}

func (labelCreateCmd) Run(args []string, ctx *cli.Ctx) error {
	rest := stripFlags(args, "--color", "--desc")
	if len(rest) != 1 {
		return &cli.Error{
			Code: cli.ExitUsage,
			Msg:  "label create requires exactly one NAME",
			Hint: "example: forge label create triage --color #00aabb --desc \"needs triage first\"",
		}
	}
	color, _ := flagValue(args, "--color")
	desc, _ := flagValue(args, "--desc")
	lbl, err := ctx.API.CreateLabel(ctx.GlobalFlags.Owner, ctx.GlobalFlags.Repo, rest[0], color, desc)
	if err != nil {
		return mapErr(err)
	}
	return writeJSON(ctx.Stdout, lbl)
}

// ---- label edit ----

type labelEditCmd struct{}

func (labelEditCmd) Name() string { return "label edit" }
func (labelEditCmd) Summary() string {
	return "edit a repository label by name (usage: label edit NAME [--name N] [--color C] [--desc D])"
}
func (labelEditCmd) RequiresAPI() bool { return true }

func (labelEditCmd) HelpPage() string {
	return `use: forge label edit NAME [--name N] [--color C] [--desc D]

Edit repository label NAME (resolved by exact name match) and print the
updated label JSON. Absent flags leave the field untouched. Color is a hex
string the server validates; --desc sets the description.`
}

func (labelEditCmd) Run(args []string, ctx *cli.Ctx) error {
	rest := stripFlags(args, "--name", "--color", "--desc")
	if len(rest) == 0 {
		return &cli.Error{
			Code: cli.ExitUsage,
			Msg:  "label edit requires a NAME",
			Hint: "example: forge label edit triage --color #00aabb",
		}
	}
	ids, err := resolveLabelIDs(ctx, []string{rest[0]})
	if err != nil {
		return err
	}
	in := api.UpdateLabelInput{}
	if v, ok := flagValue(args, "--name"); ok {
		in.Name = v
	}
	if v, ok := flagValue(args, "--color"); ok {
		in.Color = v
	}
	if v, ok := flagValue(args, "--desc"); ok {
		in.Description = v
	}
	lbl, err := ctx.API.EditLabel(ctx.GlobalFlags.Owner, ctx.GlobalFlags.Repo, ids[0], in)
	if err != nil {
		return mapErr(err)
	}
	return writeJSON(ctx.Stdout, lbl)
}

// ---- label delete ----

type labelDeleteCmd struct{}

func (labelDeleteCmd) Name() string { return "label delete" }
func (labelDeleteCmd) Summary() string {
	return "delete a repository label by name (usage: label delete NAME)"
}
func (labelDeleteCmd) RequiresAPI() bool { return true }

func (labelDeleteCmd) HelpPage() string {
	return `use: forge label delete NAME

Delete repository label NAME (resolved by exact name match) and print a
receipt naming the deleted label. Issues keep their history server-side;
this deletes the label itself.`
}

func (labelDeleteCmd) Run(args []string, ctx *cli.Ctx) error {
	rest := stripFlags(args)
	if len(rest) != 1 {
		return &cli.Error{
			Code: cli.ExitUsage,
			Msg:  "label delete requires exactly one NAME",
			Hint: "example: forge label delete triage",
		}
	}
	ids, err := resolveLabelIDs(ctx, rest)
	if err != nil {
		return err
	}
	if err := ctx.API.DeleteLabel(ctx.GlobalFlags.Owner, ctx.GlobalFlags.Repo, ids[0]); err != nil {
		return mapErr(err)
	}
	return writeJSON(ctx.Stdout, labelManageReceipt{Name: rest[0], Action: "delete"})
}
