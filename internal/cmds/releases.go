package cmds

import (
	"path/filepath"

	"forge/internal/cli"
	"forge/internal/store"
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
		releaseDownloadCmd{},
	}
}

// ---- release download ----

// ReleaseDownloadFile records one written asset. Path is the resolved
// location on disk; Bytes is the written byte count.
type ReleaseDownloadFile struct {
	Name  string `json:"name"`
	Path  string `json:"path"`
	Bytes int    `json:"bytes"`
}

// ReleaseDownloadReceipt records the download the user asked for, printed
// once, only after every selected asset succeeded. Files is non-nil, so an
// asset-less release serializes as "files": [].
type ReleaseDownloadReceipt struct {
	Tag   string                `json:"tag"`
	Files []ReleaseDownloadFile `json:"files"`
}

type releaseDownloadCmd struct{}

func (releaseDownloadCmd) Name() string { return "release download" }
func (releaseDownloadCmd) Summary() string {
	return "download release assets into the releases savedir"
}
func (releaseDownloadCmd) RequiresAPI() bool { return true }

func (releaseDownloadCmd) HelpPage() string {
	return `use: forge release download TAG [--asset NAME]...

Download release TAG's assets into the seeded [savedir] releases directory
(.forge/cache/releases by default). Assets keep their exact names; an
existing file with the same name is overwritten. --asset selects named
assets, repeatable, in the order given. Prints one per-file receipt only
after every selected asset succeeds.`
}

func (releaseDownloadCmd) Run(args []string, ctx *cli.Ctx) error {
	if len(args) == 0 {
		return &cli.Error{Code: cli.ExitUsage, Msg: "release download requires a tag"}
	}
	tag := args[0]

	var assets []string
	for i := 0; i < len(args); i++ {
		if args[i] != "--asset" {
			continue
		}
		if i+1 >= len(args) {
			return &cli.Error{Code: cli.ExitUsage, Msg: "--asset requires a name"}
		}
		assets = append(assets, args[i+1])
		i++
	}

	rel, err := ctx.API.GetRelease(ctx.GlobalFlags.Owner, ctx.GlobalFlags.Repo, tag)
	if err != nil {
		return mapErr(err)
	}

	selected := rel.Assets
	if len(assets) > 0 {
		selected = nil
		for _, name := range assets {
			for _, a := range rel.Assets {
				if a.Name == name {
					selected = append(selected, a)
					break
				}
			}
		}
		for i, name := range assets {
			if i >= len(selected) || selected[i].Name != name {
				return &cli.Error{
					Code: cli.ExitRuntime,
					Msg:  "release " + tag + " has no asset " + name,
					Hint: "run release get " + tag + " to list asset names",
				}
			}
		}
	}

	// No --dir flag: release download always lands in the configured
	// releases savedir.
	dir, derr := resolveConfiguredSavedir(ctx, "releases")
	if derr != nil {
		return derr
	}
	root, rerr := resolveRoot(ctx)
	if rerr != nil {
		return rerr
	}
	absDir := dir
	if !filepath.IsAbs(absDir) {
		absDir = filepath.Join(root, dir)
	}

	// Sequential downloads, one receipt row per file, printed only after
	// every selected asset succeeded. A failure leaves earlier files in
	// place and prints nothing.
	receipt := ReleaseDownloadReceipt{Tag: tag, Files: make([]ReleaseDownloadFile, 0, len(selected))}
	for _, a := range selected {
		data, derr := ctx.API.Download(a.BrowserDownloadURL)
		if derr != nil {
			return mapErr(derr)
		}
		path, werr := store.WriteFile(absDir, a.Name, data)
		if werr != nil {
			return mapErr(werr)
		}
		receipt.Files = append(receipt.Files, ReleaseDownloadFile{Name: a.Name, Path: path, Bytes: len(data)})
	}
	return writeJSON(ctx.Stdout, receipt)
}
