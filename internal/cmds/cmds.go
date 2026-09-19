// Package cmds implements the forge commands on top of internal/api.
package cmds

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	"forge/internal/api"
	"forge/internal/cli"
)

// flagValue scans args for "--name value" and reports whether it was present.
func flagValue(args []string, name string) (string, bool) {
	for i := 0; i+1 < len(args); i++ {
		if args[i] == name {
			return args[i+1], true
		}
	}
	return "", false
}

// intFlag returns the integer value of "--name", or def when absent.
func intFlag(args []string, name string, def int) int {
	if v, ok := flagValue(args, name); ok {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			return n
		}
	}
	return def
}

// parseIndex parses the required numeric object index, accepting the first
// strictly positive numeric token wherever it appears in args. Call sites that
// take value flags strip them with stripFlags first, so a numeric flag value
// is never mistaken for the index. With the index in the leading position
// (the historical shape) the scan returns exactly what the old args[0] parse
// returned, including the error message.
func parseIndex(args []string, cmdName string) (int, error) {
	if len(args) == 0 {
		return 0, &cli.Error{Code: cli.ExitUsage, Msg: cmdName + " requires an issue or PR number"}
	}
	for _, arg := range args {
		if n, err := strconv.Atoi(arg); err == nil && n > 0 {
			return n, nil
		}
	}
	return 0, &cli.Error{Code: cli.ExitUsage, Msg: fmt.Sprintf("%s: %q is not a valid number", cmdName, args[0])}
}

// writeJSON pretty-prints v to w.
func writeJSON(w interface{ Write([]byte) (int, error) }, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

// collectFlagValues returns every "--name value" pair's values for one
// flag, in command-line order. It is the single repeated-value-flag
// collection path: a value that reads like a flag is never taken, and a
// trailing bare --name with no value consumes nothing. Named flags must be
// value-taking; strip them with stripFlags at index-parsing call sites.
func collectFlagValues(args []string, name string) []string {
	var values []string
	for i := 0; i+1 < len(args); i++ {
		if args[i] == name {
			values = append(values, args[i+1])
			i++
		}
	}
	return values
}

// stripFlags removes every "--name value" pair for the named flags from args,
// preserving the order of the remaining arguments. A named flag with no
// following token consumes nothing. Call sites list only their value-taking
// flags; boolean flags carry no value and must not be listed.
func stripFlags(args []string, names ...string) []string {
	out := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		consumed := false
		for _, name := range names {
			if args[i] == name && i+1 < len(args) {
				i++
				consumed = true
				break
			}
		}
		if !consumed {
			out = append(out, args[i])
		}
	}
	return out
}

// resolveRoot returns the repo root; saving is only meaningful inside a repo.
func resolveRoot(ctx *cli.Ctx) (string, error) {
	if ctx.Repo == nil {
		return "", &cli.Error{
			Code: cli.ExitContext,
			Msg:  "not inside a git repository",
			Hint: "savedirs are resolved against the repository root",
		}
	}
	return ctx.Repo.Root, nil
}

// mapErr converts api-layer errors into typed cli errors with exit codes:
// transport failure => ExitNetwork, auth rejection (401/403) => ExitAuth,
// other API errors => ExitRuntime.
func mapErr(err error) error {
	if err == nil {
		return nil
	}
	if api.IsNetwork(err) {
		return &cli.Error{Code: cli.ExitNetwork, Msg: "network failure", Hint: err.Error()}
	}
	var apiErr *api.APIError
	if errors.As(err, &apiErr) {
		if apiErr.Status == 401 || apiErr.Status == 403 {
			return &cli.Error{
				Code: cli.ExitAuth,
				Msg:  apiErr.Error(),
				Hint: "token rejected by server; pass a valid --token or fix your git credential helper entry",
			}
		}
		return &cli.Error{Code: cli.ExitRuntime, Msg: apiErr.Error()}
	}
	return &cli.Error{Code: cli.ExitRuntime, Msg: err.Error()}
}
