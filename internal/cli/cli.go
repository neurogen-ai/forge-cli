package cli

import (
	"errors"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
)

// Error is a typed command error mapped to an exit code by errors.As in Run.
type Error struct {
	Code int
	Msg  string
	Hint string
}

func (e *Error) Error() string {
	if e.Hint == "" {
		return e.Msg
	}
	return e.Msg + "\nhint: " + e.Hint
}

// Usage prints top-level help: one line per registered command plus the global flags.
func Usage(w io.Writer, reg *Registry) {
	fmt.Fprintln(w, "usage: forge [--host H] [--owner O] [--repo R] [--token T] [--config P] [--timeout S] [-v] <command> [args]")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "commands:")
	for _, c := range reg.Sorted() {
		fmt.Fprintf(w, "  %-24s %s\n", c.Name(), c.Summary())
	}
}

// Sorted returns commands ordered by Name for stable help output.
func (r *Registry) Sorted() []Command {
	names := make([]string, 0, len(r.cmds))
	for n := range r.cmds {
		names = append(names, n)
	}
	sort.Strings(names)
	out := make([]Command, len(names))
	for i, n := range names {
		out[i] = r.cmds[n]
	}
	return out
}

// Run parses leading global flags, dispatches argv[0] as the command path with
// the rest as args, and maps command errors onto exit codes.
func Run(argv []string, reg *Registry, base *Ctx) int {
	ctx := &Ctx{
		Stdout: base.Stdout,
		Stderr: base.Stderr,
	}
	ctx.GlobalFlags = GlobalFlags{TimeoutSeconds: 0} // unset here; config layer owns the default (30s)

	i := 0
	for i < len(argv) {
		arg := argv[i]
		switch {
		case arg == "-v":
			ctx.Verbose = true
			i++
			continue
		case !strings.HasPrefix(arg, "--"):
			// First non-flag token is the command path; stop flag parsing.
		case arg == "--verbose":
			ctx.Verbose = true
			i++
			continue
		default:
			valFlag := map[string]*string{
				"--host":   &ctx.GlobalFlags.Host,
				"--owner":  &ctx.GlobalFlags.Owner,
				"--repo":   &ctx.GlobalFlags.Repo,
				"--token":  &ctx.GlobalFlags.Token,
				"--config": &ctx.GlobalFlags.ConfigPath,
			}
			if p, ok := valFlag[arg]; ok {
				if i+1 >= len(argv) {
					fmt.Fprintf(ctx.Stderr, "forge: %s requires a value\n", arg)
					return ExitUsage
				}
				*p = argv[i+1]
				i += 2
				continue
			}
			if arg == "--timeout" {
				if i+1 >= len(argv) {
					fmt.Fprintln(ctx.Stderr, "forge: --timeout requires an integer number of seconds")
					return ExitUsage
				}
				n, err := strconv.Atoi(argv[i+1])
				if err != nil || n <= 0 {
					fmt.Fprintln(ctx.Stderr, "forge: --timeout must be a positive integer")
					return ExitUsage
				}
				ctx.GlobalFlags.TimeoutSeconds = n
				i += 2
				continue
			}
			fmt.Fprintf(ctx.Stderr, "forge: unknown global flag %s\n", arg)
			return ExitUsage
		}
		break
	}

	if i >= len(argv) {
		Usage(ctx.Stderr, reg)
		return ExitUsage
	}

	path := argv[i]
	cmd := reg.Lookup(path)
	// Multi-word command names ("cache flush", "pr list"): the registry keys
	// on the full name, so try joining the next token before giving up.
	consumed := 1
	if cmd == nil && i+1 < len(argv) {
		if joined := path + " " + argv[i+1]; reg.Lookup(joined) != nil {
			path = joined
			cmd = reg.Lookup(joined)
			consumed = 2
		}
	}
	if cmd == nil {
		fmt.Fprintf(ctx.Stderr, "forge: unknown command %q\n\n", path)
		Usage(ctx.Stderr, reg)
		return ExitUsage
	}

	if base.Prepare != nil {
		if err := base.Prepare(ctx, cmd); err != nil {
			return reportError(ctx.Stderr, err)
		}
	}

	err := cmd.Run(argv[i+consumed:], ctx)
	if err == nil {
		return ExitOK
	}
	return reportError(ctx.Stderr, err)
}

// reportError prints err and returns its mapped exit code.
func reportError(w io.Writer, err error) int {
	var cerr *Error
	if errors.As(err, &cerr) {
		if cerr.Hint != "" {
			fmt.Fprintf(w, "error: %s\nhint: %s\n", cerr.Msg, cerr.Hint)
		} else {
			fmt.Fprintf(w, "error: %s\n", cerr.Msg)
		}
		return cerr.Code
	}
	fmt.Fprintf(w, "error: %v\n", err)
	return ExitRuntime
}
