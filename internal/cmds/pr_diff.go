package cmds

import (
	"fmt"
	"path/filepath"

	"forge/internal/cli"
	"forge/internal/store"
)

// prDiffCmd prints a pull request's raw diff or patch bytes. Stdout is the
// server's text verbatim; --out stores the same bytes under the savedir
// instead and prints a JSON receipt.
type prDiffCmd struct{}

func (prDiffCmd) Name() string      { return "pr diff" }
func (prDiffCmd) Summary() string   { return "print a pull request's raw diff [--patch] [--out]" }
func (prDiffCmd) RequiresAPI() bool { return true }

// DiffReceipt is printed when --out stores the response.
type DiffReceipt struct {
	Path  string `json:"path"`
	Bytes int    `json:"bytes"`
}

func (prDiffCmd) Run(args []string, ctx *cli.Ctx) error {
	n, err := parseIndex(args, "pr diff")
	if err != nil {
		return err
	}
	format := "diff"
	if flagBool(args, "--patch") {
		format = "patch"
	}
	raw, err := ctx.API.GetPullDiff(ctx.GlobalFlags.Owner, ctx.GlobalFlags.Repo, n, format)
	if err != nil {
		return mapErr(err)
	}
	if !flagBool(args, "--out") {
		// Byte-for-byte: no trailing newline, no JSON encoding.
		_, err = ctx.Stdout.Write(raw.Body)
		return err
	}

	// --out resolves the [savedir] pr-conversation directory like pr pull.
	dir := ""
	if ctx.Cfg != nil {
		if d, ok := ctx.Cfg.Savedirs["pr-conversation"]; ok && d != "" {
			dir = d
		}
	}
	if dir == "" {
		return &cli.Error{
			Code: cli.ExitUsage,
			Msg:  "no savedir for pr-conversation",
			Hint: "restore the seeded [savedir] pr-conversation config entry, or drop --out to print the diff to stdout",
		}
	}
	root, rerr := resolveRoot(ctx)
	if rerr != nil {
		return rerr
	}
	absDir := dir
	if !filepath.IsAbs(absDir) {
		absDir = filepath.Join(root, dir)
	}
	path, werr := store.WriteFile(absDir, fmt.Sprintf("%s-%d.%s", ctx.GlobalFlags.Repo, n, format), raw.Body)
	if werr != nil {
		return mapErr(werr)
	}
	return writeJSON(ctx.Stdout, DiffReceipt{Path: path, Bytes: len(raw.Body)})
}

func (prDiffCmd) HelpPage() string {
	return `use: forge pr diff N [--patch] [--out]

Print pull request N's diff exactly as the server returns it: raw text on
stdout, byte for byte, with no trailing newline. --patch selects the .patch
representation instead of the default .diff.

--out stores the same bytes below the [savedir] pr-conversation directory as
<repo>-<N>.diff (or <repo>-<N>.patch with --patch), replacing any previous
copy, and prints a JSON receipt {path, bytes} instead of the diff.

The diff is never cached implicitly; files are written only with --out.
--table is rejected: stdout is either raw text or one JSON receipt.`
}
