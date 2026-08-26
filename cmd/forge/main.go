package main

import (
	"fmt"
	"os"

	"forge/internal/cli"
)

// versionCmd is a builtin placeholder; real commands arrive in later branches.
type versionCmd struct{}

func (versionCmd) Name() string    { return "version" }
func (versionCmd) Summary() string { return "print the forge-cli version" }
func (versionCmd) Run(args []string, ctx *cli.Ctx) error {
	fmt.Fprintln(ctx.Stdout, "forge-cli v0.1")
	return nil
}

func main() {
	reg := cli.NewRegistry()
	reg.Register(versionCmd{})

	ctx := &cli.Ctx{
		Stdout: os.Stdout,
		Stderr: os.Stderr,
	}

	if len(os.Args) <= 1 {
		cli.Usage(os.Stdout, reg)
		os.Exit(cli.ExitOK)
	}

	os.Exit(cli.Run(os.Args[1:], reg, ctx))
}
