package cmds

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"forge/internal/cli"
)

// browseCmd prints, or with --open launches, the server-provided web URL for
// a pull request, an issue, or the repository. kind is "pr", "issue", or
// "repo". The URL always comes from the server's html_url field; it is never
// constructed from host config. Launching has one home in openInBrowser.
type browseCmd struct{ kind string }

func (c browseCmd) Name() string {
	switch c.kind {
	case "pr":
		return "pr browse"
	case "issue":
		return "issue browse"
	default:
		return "repo browse"
	}
}

func (c browseCmd) Summary() string {
	switch c.kind {
	case "pr":
		return "print the web URL of one pull request [--open]"
	case "issue":
		return "print the web URL of one issue [--open]"
	default:
		return "print the web URL of the repository [--open]"
	}
}

func (browseCmd) RequiresAPI() bool { return true }

func (c browseCmd) Run(args []string, ctx *cli.Ctx) error {
	url, err := c.lookup(args, ctx)
	if err != nil {
		return err
	}
	if url != "" {
		fmt.Fprintln(ctx.Stdout, url)
	}
	if !hasFlag(args, "--open") {
		return nil
	}
	if url == "" {
		return &cli.Error{
			Code: cli.ExitRuntime,
			Msg:  "server did not provide a web URL for " + c.Name(),
			Hint: "the object was fetched successfully but its html_url is empty; --open has nothing to launch",
		}
	}
	return openInBrowser(url, ctx)
}

func (c browseCmd) HelpPage() string {
	switch c.kind {
	case "pr":
		return "use: forge pr browse N [--open]\n\nPrint the server-provided web URL of one pull request. With --open, launch\n$BROWSER on the URL (the URL still prints). The URL is never opened without\n--open."
	case "issue":
		return "use: forge issue browse N [--open]\n\nPrint the server-provided web URL of one issue. With --open, launch\n$BROWSER on the URL (the URL still prints). The URL is never opened without\n--open."
	default:
		return "use: forge repo browse [--open]\n\nPrint the server-provided web URL of the repository. With --open, launch\n$BROWSER on the URL (the URL still prints). The URL is never opened without\n--open."
	}
}

// lookup performs the single API request for the browse kind and returns the
// server-provided web URL (empty when the server sent none).
func (c browseCmd) lookup(args []string, ctx *cli.Ctx) (string, error) {
	o, r := ctx.GlobalFlags.Owner, ctx.GlobalFlags.Repo
	switch c.kind {
	case "pr", "issue":
		n, err := parseIndex(stripBoolFlag(args, "--open"), c.Name())
		if err != nil {
			return "", err
		}
		if c.kind == "pr" {
			pr, err := ctx.API.GetPullRequest(o, r, n)
			if err != nil {
				return "", mapErr(err)
			}
			return pr.HTMLURL, nil
		}
		iss, err := ctx.API.GetIssue(o, r, n)
		if err != nil {
			return "", mapErr(err)
		}
		return iss.HTMLURL, nil
	default:
		repo, err := ctx.API.GetRepository(o, r)
		if err != nil {
			return "", mapErr(err)
		}
		return repo.HTMLURL, nil
	}
}

// RepoCommands returns the repo subcommands for registration in main.
// repo browse is the one repository-surface command; repo view/edit stay out
// of scope for this release.
func RepoCommands() []cli.Command {
	return []cli.Command{browseCmd{kind: "repo"}}
}

// hasFlag reports whether the exact flag token is present in args.
func hasFlag(args []string, name string) bool {
	for _, a := range args {
		if a == name {
			return true
		}
	}
	return false
}

// stripBoolFlag removes every occurrence of a valueless flag token so
// positional arguments can be parsed independently of it.
func stripBoolFlag(args []string, name string) []string {
	out := make([]string, 0, len(args))
	for _, a := range args {
		if a != name {
			out = append(out, a)
		}
	}
	return out
}

// openInBrowser splits non-empty $BROWSER on whitespace, appends url as the
// final argument, and execs it directly; it never invokes a shell and routes
// child output to ctx.Stderr. A missing browser or non-zero exit is a
// runtime error.
func openInBrowser(url string, ctx *cli.Ctx) error {
	browser := os.Getenv("BROWSER")
	fields := strings.Fields(browser)
	if len(fields) == 0 {
		return &cli.Error{
			Code: cli.ExitRuntime,
			Msg:  "no browser to open " + url,
			Hint: "set $BROWSER to the browser command, e.g. BROWSER=firefox",
		}
	}
	cmd := exec.Command(fields[0], append(fields[1:], url)...)
	cmd.Stdout = ctx.Stderr
	cmd.Stderr = ctx.Stderr
	if err := cmd.Run(); err != nil {
		return &cli.Error{
			Code: cli.ExitRuntime,
			Msg:  fmt.Sprintf("browser %q failed: %v", browser, err),
			Hint: "$BROWSER exited non-zero or could not be started; the URL was printed above",
		}
	}
	return nil
}
