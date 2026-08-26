package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"forge/internal/api"
	"forge/internal/auth"
	"forge/internal/cli"
	"forge/internal/cmds"
	"forge/internal/config"
	"forge/internal/gitctx"
)

// versionCmd is a builtin placeholder; real commands arrive in later branches.
type versionCmd struct{}

func (versionCmd) Name() string    { return "version" }
func (versionCmd) Summary() string { return "print the forge-cli version" }
func (versionCmd) HelpPage() string {
	return "use: forge version\n\nPrint the forge-cli version."
}
func (versionCmd) Run(args []string, ctx *cli.Ctx) error {
	fmt.Fprintln(ctx.Stdout, "forge-cli v0.1")
	return nil
}

// stderrLogger is the verbose sink. The api client formats complete request
// lines itself and never hands over header values, so Authorization cannot
// leak through here.
type stderrLogger struct{ w io.Writer }

func (l stderrLogger) Logf(format string, args ...any) {
	fmt.Fprintf(l.w, format+"\n", args...)
}

// wire resolves configuration, git context, and (for commands that need it)
// auth plus an API client into ctx before Command.Run. Resolution order per
// key is flag > env > local config > global config > git-derived; Load already
// merges local over global, so one cfg lookup covers positions 3 and 4.
func wire(ctx *cli.Ctx, cmd cli.Command) error {
	var repo *gitctx.Repo
	if r, err := gitctx.Detect(); err == nil {
		repo = &r
	}
	localPath := ""
	if repo != nil {
		localPath = config.LocalPath(repo.Root)
	}
	globalPath := ctx.GlobalFlags.ConfigPath
	if globalPath == "" {
		globalPath = config.DefaultGlobalPath()
	}
	cfg, err := config.Load(globalPath, localPath, true)
	if err != nil {
		return &cli.Error{Code: cli.ExitRuntime, Msg: err.Error()}
	}
	ctx.Cfg = cfg
	ctx.Repo = repo

	var rem gitctx.Remote // zero value when outside a repo or no origin
	if repo != nil && repo.OriginURL != "" {
		if parsed, perr := gitctx.ParseRemoteURL(repo.OriginURL); perr == nil {
			rem = parsed
		}
	}

	host := firstNonEmpty(ctx.GlobalFlags.Host, os.Getenv("FORGE_HOST"), cfg.Defaults.Host, rem.Host)
	host = normalizeHost(host)
	owner := firstNonEmpty(ctx.GlobalFlags.Owner, os.Getenv("FORGE_OWNER"), cfg.Defaults.Owner, rem.Owner)
	repoName := firstNonEmpty(ctx.GlobalFlags.Repo, os.Getenv("FORGE_REPO"), cfg.Defaults.Repo, rem.Repo)
	// Commands read the resolved values back out of GlobalFlags so the whole
	// chain (flag > env > config > git) is visible in one place.
	ctx.GlobalFlags.Host = host
	ctx.GlobalFlags.Owner = owner
	ctx.GlobalFlags.Repo = repoName

	explicitToken := firstNonEmpty(ctx.GlobalFlags.Token, cfg.Token)

	if !cli.RequiresAPI(cmd) {
		return nil
	}

	var missing []string
	if host == "" {
		missing = append(missing, "host")
	}
	if owner == "" {
		missing = append(missing, "owner")
	}
	if repoName == "" {
		missing = append(missing, "repo")
	}
	if len(missing) > 0 {
		return &cli.Error{
			Code: cli.ExitContext,
			Msg:  "cannot determine " + strings.Join(missing, ", "),
			Hint: "run inside a repository with an origin remote, or pass --host/--owner/--repo",
		}
	}

	token, err := auth.Resolve(host, explicitToken)
	if err != nil {
		if errors.Is(err, auth.ErrNoToken) {
			return &cli.Error{
				Code: cli.ExitAuth,
				Msg:  "no token found",
				Hint: "check your git credential helper stores a token for this host, or pass --token",
			}
		}
		return &cli.Error{Code: cli.ExitAuth, Msg: err.Error()}
	}

	timeout := ctx.GlobalFlags.TimeoutSeconds
	if timeout == 0 {
		timeout = cfg.TimeoutSeconds
	}
	var logger api.Logger
	if ctx.Verbose {
		logger = stderrLogger{w: ctx.Stderr}
	}
	ctx.API = api.NewClient("https://"+host, token, time.Duration(timeout)*time.Second, logger)

	// Staged preflight: pinpoint which layer (host, token, owner, repo) is
	// wrong so a 404 never surfaces as a vague "target not found".
	return diagnoseRepo(ctx.API, host, owner, repoName)
}

// mapWiredErr converts errors from the wiring-time repo check into typed cli
// errors: transport => ExitNetwork, auth rejection => ExitAuth, otherwise
// ExitRuntime.
func mapWiredErr(err error) error {
	var e *cli.Error
	if errors.As(err, &e) {
		return e
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

// normalizeHost strips an optional scheme and trailing slash from a host
// value, accepting both "git.example.com" and "https://git.example.com/".
func normalizeHost(h string) string {
	for _, p := range []string{"https://", "http://"} {
		h = strings.TrimPrefix(h, p)
	}
	return strings.TrimSuffix(h, "/")
}

// firstNonEmpty returns the first non-empty string.
func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func main() {
	reg := cli.NewRegistry()
	reg.Register(versionCmd{})
	reg.Register(cmds.PRCommands()...)
	reg.Register(cmds.IssueCommands()...)
	reg.Register(cmds.SaveCommands()...)
	reg.Register(cmds.CacheCommands()...)

	base := &cli.Ctx{
		Stdout:  os.Stdout,
		Stderr:  os.Stderr,
		Prepare: wire,
	}

	if len(os.Args) <= 1 {
		cli.Usage(os.Stdout, reg)
		os.Exit(cli.ExitOK)
	}

	os.Exit(cli.Run(os.Args[1:], reg, base))
}
