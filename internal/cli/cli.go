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

// Usage prints top-level help: one line per registered command family (or
// standalone command) plus the global flags, pointing each family at -h.
func Usage(w io.Writer, reg *Registry) {
	fmt.Fprintln(w, "usage: forge [--host H] [--owner O] [--repo R] [--token T] [--config P] [--timeout S] [-v] <command> [args]")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "commands:")
	seen := map[string]bool{}
	for _, c := range reg.Sorted() {
		name := c.Name()
		if prefix, grouped := reg.GroupPrefix(name); grouped {
			if seen[prefix] {
				continue
			}
			seen[prefix] = true
			fmt.Fprintf(w, "  %-24s %s  (forge %s -h)\n", prefix, groupSummary[prefix], prefix)
			continue
		}
		fmt.Fprintf(w, "  %-24s %s\n", name, c.Summary())
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "run `forge <command> -h` for details")
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
		Stdout:   base.Stdout,
		Stderr:   base.Stderr,
		Diagnose: base.Diagnose,
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
		case arg == "-h" || arg == "--help":
			ctx.Help = true
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
					fmt.Fprintln(ctx.Stderr)
					Usage(ctx.Stderr, reg)
					return ExitUsage
				}
				*p = argv[i+1]
				i += 2
				continue
			}
			if arg == "--timeout" {
				if i+1 >= len(argv) {
					fmt.Fprintln(ctx.Stderr, "forge: --timeout requires an integer number of seconds")
					fmt.Fprintln(ctx.Stderr)
					Usage(ctx.Stderr, reg)
					return ExitUsage
				}
				n, err := strconv.Atoi(argv[i+1])
				if err != nil || n <= 0 {
					fmt.Fprintln(ctx.Stderr, "forge: --timeout must be a positive integer")
					fmt.Fprintln(ctx.Stderr)
					Usage(ctx.Stderr, reg)
					return ExitUsage
				}
				ctx.GlobalFlags.TimeoutSeconds = n
				i += 2
				continue
			}
			fmt.Fprintf(ctx.Stderr, "forge: unknown global flag %s\n", arg)
			fmt.Fprintln(ctx.Stderr)
			Usage(ctx.Stderr, reg)
			return ExitUsage
		}
		break
	}

	if i >= len(argv) {
		if ctx.Help {
			Usage(ctx.Stdout, reg)
			return ExitOK
		}
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
	// -h/--help anywhere after the command path wins and swallows everything
	// after it; following flags are ignored entirely and Prepare never runs.
	wantsHelp := ctx.Help
	for _, a := range argv[i+consumed:] {
		if a == "-h" || a == "--help" {
			wantsHelp = true
			break
		}
	}

	if cmd == nil {
		// The command path does not resolve. When it names a family (bare
		// `forge pr`), print that family's page instead of an error.
		if prefix, ok := reg.GroupPrefix(path); ok {
			reg.PrintGroupPage(ctx.Stdout, prefix)
			return ExitOK
		}
		if wantsHelp {
			Usage(ctx.Stdout, reg)
			return ExitOK
		}
		fmt.Fprintf(ctx.Stderr, "forge: unknown command %q\n\n", path)
		Usage(ctx.Stderr, reg)
		return ExitUsage
	}
	if wantsHelp {
		PrintHelp(ctx.Stdout, cmd)
		return ExitOK
	}

	// Global flags are accepted after the command path too (users naturally
	// write `forge pr list -v --host x`). Known global flags are consumed here;
	// everything else passes through to the command untouched.
	rest := argv[i+consumed:]
	kept, err := applyGlobalArgs(rest, ctx)
	if err != nil {
		fmt.Fprintln(ctx.Stderr, err.Error())
		fmt.Fprintln(ctx.Stderr)
		PrintHelp(ctx.Stderr, cmd)
		return ExitUsage
	}

	if base.Prepare != nil {
		if err := base.Prepare(ctx, cmd); err != nil {
			return reportError(ctx.Stderr, err)
		}
	}

	err = cmd.Run(kept, ctx)
	if err == nil {
		return ExitOK
	}
	code := reportError(ctx.Stderr, err)
	var cerr *Error
	if errors.As(err, &cerr) {
		switch cerr.Code {
		case ExitUsage:
			fmt.Fprintln(ctx.Stderr)
			PrintHelp(ctx.Stderr, cmd)
		case ExitRuntime, ExitNetwork:
			if ctx.Diagnose != nil {
				if d := ctx.Diagnose(); d != nil {
					fmt.Fprintln(ctx.Stderr)
					fmt.Fprintln(ctx.Stderr, d.Msg)
					if d.Hint != "" {
						fmt.Fprintf(ctx.Stderr, "hint: %s\n", d.Hint)
					}
				}
			}
		}
	}
	return code
}

// applyGlobalArgs scans args for known global flags, applies them to ctx,
// and returns the args that remain for the command. Unknown flags are kept.
func applyGlobalArgs(args []string, ctx *Ctx) ([]string, error) {
	valFlag := func(name string) (*string, bool) {
		switch name {
		case "--host":
			return &ctx.GlobalFlags.Host, true
		case "--owner":
			return &ctx.GlobalFlags.Owner, true
		case "--repo":
			return &ctx.GlobalFlags.Repo, true
		case "--token":
			return &ctx.GlobalFlags.Token, true
		case "--config":
			return &ctx.GlobalFlags.ConfigPath, true
		case "--timeout":
			return nil, true // integer-valued; handled below
		}
		return nil, false
	}
	out := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "-h" || arg == "--help" {
			continue // defense in depth; Run returns before this for real help
		}
		if arg == "-v" || arg == "--verbose" {
			ctx.Verbose = true
			continue
		}
		ptr, known := valFlag(arg)
		if !known || !strings.HasPrefix(arg, "--") {
			out = append(out, arg)
			continue
		}
		if i+1 >= len(args) {
			return nil, fmt.Errorf("forge: %s requires a value", arg)
		}
		val := args[i+1]
		i++
		if ptr == nil { // --timeout
			n, err := strconv.Atoi(val)
			if err != nil || n <= 0 {
				return nil, fmt.Errorf("forge: --timeout must be a positive integer")
			}
			ctx.GlobalFlags.TimeoutSeconds = n
			continue
		}
		*ptr = val
	}
	return out, nil
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
