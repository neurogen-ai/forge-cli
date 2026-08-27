package cmds

import (
	"fmt"
	"os"

	"forge/internal/cli"
	"forge/internal/config"
	"forge/internal/store"
)

// ---- cache path ----

type cachePathCmd struct{}

func (cachePathCmd) Name() string      { return "cache path" }
func (cachePathCmd) Summary() string   { return "print resolved savedir paths, one per line" }
func (cachePathCmd) RequiresAPI() bool { return false } // never triggers auth or host validation

func (cachePathCmd) HelpPage() string {
	return `use: forge cache path

Print every resolved savedir path, one per line. Never contacts the server.`
}

func (cachePathCmd) Run(args []string, ctx *cli.Ctx) error {
	root, err := resolveRoot(ctx)
	if err != nil {
		return err
	}
	home, _ := os.UserHomeDir()
	for _, dir := range store.ResolveDirs(ctx.Cfg.Savedirs, root, home) {
		fmt.Fprintln(ctx.Stdout, dir)
	}
	return nil
}

// ---- cache flush ----

type cacheFlushCmd struct{}

func (cacheFlushCmd) Name() string { return "cache flush" }

func (cacheFlushCmd) HelpPage() string {
	return `use: forge cache flush [--yes]

Delete cached JSON files in every configured savedir, printing each removed
path. Paths outside the repository root require --yes. Savedirs under
forge's state directory (~/.local/state/forge) flush without --yes.`
}

func (cacheFlushCmd) Summary() string {
	return "delete cached JSON files in every savedir [--yes for outside-root dirs]"
}
func (cacheFlushCmd) RequiresAPI() bool { return false }

func (cacheFlushCmd) Run(args []string, ctx *cli.Ctx) error {
	root, err := resolveRoot(ctx)
	if err != nil {
		return err
	}
	home, _ := os.UserHomeDir()
	dirs := store.ResolveDirs(ctx.Cfg.Savedirs, root, home)

	removed, err := store.Flush(root, config.DefaultStateDir(), dirs, false,
		config.LocalPath(root), config.DefaultGlobalPath())
	if err != nil && !flagBool(args, "--yes") {
		return &cli.Error{
			Code: cli.ExitUsage,
			Msg:  err.Error(),
			Hint: "re-run with --yes to delete these paths",
		}
	}
	if err != nil {
		removed, err = store.Flush(root, config.DefaultStateDir(), dirs, true,
			config.LocalPath(root), config.DefaultGlobalPath())
		if err != nil {
			return mapErr(err)
		}
	}
	for _, p := range removed {
		fmt.Fprintln(ctx.Stdout, p)
	}
	return nil
}

// flagBool reports whether a boolean flag is present in args.
func flagBool(args []string, name string) bool {
	for _, a := range args {
		if a == name {
			return true
		}
	}
	return false
}

// CacheCommands registers the cache subcommands.
func CacheCommands() []cli.Command {
	return []cli.Command{cachePathCmd{}, cacheFlushCmd{}}
}
