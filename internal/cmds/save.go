package cmds

import (
	"encoding/json"
	"fmt"
	"path/filepath"

	"forge/internal/api"
	"forge/internal/cli"
	"forge/internal/store"
)

// ---- save pr-conversation / save issue ----

type saveCmd struct{ kind string }

func (c saveCmd) Name() string {
	if c.kind == "issue" {
		return "save issue"
	}
	return "save pr-conversation"
}

func (c saveCmd) Summary() string {
	if c.kind == "issue" {
		return "fetch an issue and write it as JSON to the configured savedir (usage: save issue N [--dir])"
	}
	return "fetch a PR conversation and write it as JSON to the configured savedir (usage: save pr-conversation N [--dir])"
}

func (c saveCmd) HelpPage() string {
	if c.kind == "issue" {
		return `use: forge save issue N [--dir DIR]

Fetch one issue and write pretty-printed JSON to the configured savedir.
Dir resolution: --dir, then [savedir] issue in config. Prints the written
file path on stdout.`
	}
	return "use: forge save pr-conversation N [--dir DIR]\n\nFetch a PR conversation (same payload as `pr conversation --format grouped`)\nand write it as JSON to the configured savedir. Dir resolution: --dir, then\n[savedir] pr-conversation in config. Prints the written file path on stdout."
}

func (c saveCmd) RequiresAPI() bool { return true }

// savedirFor resolves the target directory: --dir flag, then cfg Savedirs,
// then ExitUsage naming the config key.
func savedirFor(ctx *cli.Ctx, args []string, key string) (string, error) {
	if dir, ok := flagValue(args, "--dir"); ok {
		return dir, nil
	}
	if ctx.Cfg != nil {
		if dir, ok := ctx.Cfg.Savedirs[key]; ok && dir != "" {
			return dir, nil
		}
	}
	return "", &cli.Error{
		Code: cli.ExitUsage,
		Msg:  "no savedir for " + key,
		Hint: "pass --dir or set [savedir] " + key + " in your config",
	}
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

// stripFlag removes all occurrences of "--name value" from args so positional
// arguments can be parsed independently of flags.
func stripFlag(args []string, name string) []string {
	out := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		if args[i] == name && i+1 < len(args) {
			i++
			continue
		}
		out = append(out, args[i])
	}
	return out
}

// storeSaver is a variable so tests can capture Save calls.
var storeSaver = store.Save

// groupedPayload renders the same grouped conversation shape as pr conversation.
func groupedPayload(comments []api.Comment, reviews []api.Review, perReview map[int64][]api.ReviewComment) groupedConversation {
	out := groupedConversation{Comments: comments, Reviews: make([]groupedReview, 0, len(reviews))}
	for _, rev := range reviews {
		gr := groupedReview{
			ID: rev.ID, User: rev.User, State: rev.State, Body: rev.Body,
			SubmittedAt: rev.SubmittedAt, CreatedAt: rev.CreatedAt,
			Comments: make([]flatItem, 0, len(perReview[rev.ID])),
		}
		for _, rc := range perReview[rev.ID] {
			gr.Comments = append(gr.Comments, flatItem{
				Kind: "review-comment", ID: rc.ID, User: rc.User, Body: rc.Body,
				CreatedAt: rc.CreatedAt, Path: rc.Path, DiffHunk: rc.DiffHunk,
				ReviewID: rev.ID,
			})
		}
		out.Reviews = append(out.Reviews, gr)
	}
	return out
}

func (c saveCmd) Run(args []string, ctx *cli.Ctx) error {
	positional := stripFlag(args, "--dir")
	n, err := parseIndex(positional, "save "+c.kind)
	if err != nil {
		return err
	}
	dir, err := savedirFor(ctx, args, c.kind)
	if err != nil {
		return err
	}
	root, err := resolveRoot(ctx)
	if err != nil {
		return err
	}

	var data []byte
	switch c.kind {
	case "pr-conversation":
		comments, reviews, perReview, ferr := fetchConversation(ctx, n)
		if ferr != nil {
			return mapErr(ferr)
		}
		data, err = json.MarshalIndent(groupedPayload(comments, reviews, perReview), "", "  ")
	case "issue":
		var iss *api.Issue
		iss, err = ctx.API.GetIssue(ctx.GlobalFlags.Owner, ctx.GlobalFlags.Repo, n)
		if err == nil {
			data, err = json.MarshalIndent(iss, "", "  ")
		}
	default:
		err = fmt.Errorf("unknown save kind %q", c.kind)
	}
	if err != nil {
		var cerr *cli.Error
		if asCliError(err, &cerr) {
			return cerr
		}
		return mapErr(err)
	}

	absDir := dir
	if !filepath.IsAbs(absDir) {
		absDir = filepath.Join(root, dir)
	}
	path, werr := storeSaver(absDir, ctx.GlobalFlags.Repo, n, data)
	if werr != nil {
		return mapErr(werr)
	}
	fmt.Fprintln(ctx.Stdout, path)
	return nil
}

// asCliError reports whether err is a *cli.Error.
func asCliError(err error, target **cli.Error) bool {
	e, ok := err.(*cli.Error)
	if ok {
		*target = e
	}
	return ok
}

// SaveCommands registers both save subcommands.
func SaveCommands() []cli.Command {
	return []cli.Command{saveCmd{kind: "pr-conversation"}, saveCmd{kind: "issue"}}
}
