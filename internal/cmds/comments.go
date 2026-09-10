package cmds

import (
	"fmt"
	"strconv"

	"forge/internal/api"
	"forge/internal/cli"
)

// commentAddCmd posts an issue-backed comment under one of two spellings:
// "pr comment add" or "issue comment add". Forgejo stores PR comments as
// issue comments, so both instances call the same AddComment endpoint and
// print the same receipt. kind affects only the command name and usage text.
// gh marks the gh-spelled canonical verb ("pr comment"); it delegates to the
// same run so both spellings share one implementation and receipt shape.
type commentAddCmd struct {
	kind string // "pr" or "issue"
	gh   bool   // canonical gh spelling: "<kind> comment"
}

func (c commentAddCmd) Name() string {
	if c.gh {
		return c.kind + " comment"
	}
	return c.kind + " comment add"
}
func (c commentAddCmd) Summary() string {
	return "add one comment to a " + c.kind + " [--body T]"
}
func (commentAddCmd) RequiresAPI() bool { return true }

// CommentReceipt is the stable mutation output for issue-backed comments.
type CommentReceipt struct {
	ID      int64  `json:"id"`
	HTMLURL string `json:"html_url"`
}

// AnchoredCommentReceipt is the mutation output for an anchored inline
// comment: what was anchored, as the user asked it, plus the server URL.
type AnchoredCommentReceipt struct {
	ID      int64  `json:"id"`
	Path    string `json:"path"`
	Line    int64  `json:"line"`
	Side    string `json:"side,omitempty"`
	HTMLURL string `json:"html_url"`
}

func (c commentAddCmd) Run(args []string, ctx *cli.Ctx) error { return c.run(args, ctx) }

// run is the one comment-adding implementation; both spellings delegate here
// so receipts stay identical however the user spells the command.
func (c commentAddCmd) run(args []string, ctx *cli.Ctx) error {
	n, err := parseIndex(stripFlags(args, "--body", "--file", "--line", "--side"), c.Name())
	if err != nil {
		return err
	}
	body, _ := flagValue(args, "--body")
	if body == "" {
		return &cli.Error{
			Code: cli.ExitUsage,
			Msg:  c.Name() + ": --body is required",
			Hint: "pass the comment text with --body, or pipe it with --body -",
		}
	}
	data, err := readDashInput(body, ctx.Stdin)
	if err != nil {
		return err
	}
	if len(data) == 0 {
		return &cli.Error{
			Code: cli.ExitUsage,
			Msg:  c.Name() + ": --body is required",
			Hint: "piped stdin for --body - was empty; pass the comment text with --body",
		}
	}
	text := string(data)
	// Token presence, not flagValue: a dangling anchor flag has no value for
	// flagValue to find, and routing it to the unanchored transport would
	// silently drop the anchor the user asked for. --body's value is stripped
	// first so a body that literally reads "--file" is not mistaken for a flag.
	anchorArgs := stripFlags(args, "--body")
	if hasFlagToken(anchorArgs, "--file") || hasFlagToken(anchorArgs, "--line") || hasFlagToken(anchorArgs, "--side") {
		return c.runAnchored(args, ctx, n, text)
	}
	comment, err := ctx.API.AddComment(ctx.GlobalFlags.Owner, ctx.GlobalFlags.Repo, n, text)
	if err != nil {
		return mapErr(err)
	}
	return writeJSON(ctx.Stdout, CommentReceipt{ID: comment.ID, HTMLURL: comment.HTMLURL})
}

// hasFlagToken reports whether the literal flag token appears in args, even
// when the flag is dangling with no value; flagValue cannot see those.
func hasFlagToken(args []string, name string) bool {
	for _, a := range args {
		if a == name {
			return true
		}
	}
	return false
}

// runAnchored posts the comment as one inline review comment anchored to a
// diff hunk. Anchor flags are validated here before any request; the wire
// encoding lives only in internal/api's AnchorToWire. A server rejection of
// the anchor is the standard exit-1 path; there is no fallback to the
// unanchored issue-comment transport.
func (c commentAddCmd) runAnchored(args []string, ctx *cli.Ctx, n int, text string) error {
	fileTok := hasFlagToken(args, "--file")
	lineTok := hasFlagToken(args, "--line")
	if !fileTok || !lineTok {
		missing := "--file"
		if fileTok {
			missing = "--line"
		}
		return &cli.Error{
			Code: cli.ExitUsage,
			Msg:  fmt.Sprintf("%s: %s requires --file and --line", c.Name(), missing),
			Hint: "anchor to one hunk line with --file P --line L",
		}
	}
	file, hasFile := flagValue(args, "--file")
	if !hasFile {
		return &cli.Error{
			Code: cli.ExitUsage,
			Msg:  c.Name() + ": --file requires a path value",
			Hint: "anchor to one hunk line with --file P --line L",
		}
	}
	lineStr, hasLine := flagValue(args, "--line")
	if !hasLine {
		return &cli.Error{
			Code: cli.ExitUsage,
			Msg:  c.Name() + ": --line requires a line number",
			Hint: "anchor to one hunk line with --file P --line L",
		}
	}
	line, err := strconv.Atoi(lineStr)
	if err != nil || line <= 0 {
		return &cli.Error{
			Code: cli.ExitUsage,
			Msg:  fmt.Sprintf("%s: --line %q is not a positive line number", c.Name(), lineStr),
		}
	}
	side, hasSide := flagValue(args, "--side")
	if hasFlagToken(args, "--side") && !hasSide {
		return &cli.Error{
			Code: cli.ExitUsage,
			Msg:  c.Name() + ": --side requires old or new",
		}
	}
	if side != "" && side != "old" && side != "new" {
		return &cli.Error{
			Code: cli.ExitUsage,
			Msg:  c.Name() + ": --side must be old or new",
		}
	}
	entry, err := api.AnchorToWire(file, int64(line), side)
	if err != nil {
		return &cli.Error{Code: cli.ExitUsage, Msg: c.Name() + ": " + err.Error()}
	}
	entry.Body = text
	return c.postAnchored(ctx, n, entry, side)
}

// postAnchored sends the anchored review comment and prints the receipt.
func (c commentAddCmd) postAnchored(ctx *cli.Ctx, n int, entry api.ReviewCommentInput, side string) error {
	rc, err := ctx.API.CreateAnchoredComment(ctx.GlobalFlags.Owner, ctx.GlobalFlags.Repo, n, api.CreateAnchoredCommentInput{
		Body:     entry.Body,
		Comments: []api.ReviewCommentInput{entry},
	})
	if err != nil {
		return mapErr(err)
	}
	line := entry.NewLineNum
	if side == "old" {
		line = entry.OldLineNum
	}
	return writeJSON(ctx.Stdout, AnchoredCommentReceipt{
		ID:      rc.ID,
		Path:    entry.Path,
		Line:    line,
		Side:    side,
		HTMLURL: rc.HTMLURL,
	})
}

func (c commentAddCmd) HelpPage() string {
	if !c.gh {
		return fmt.Sprintf(`use: forge %[1]s N --body T

Add one comment to %[1]s N and print a JSON receipt {id, html_url}.
--body is required; an empty body is a usage error before any request.
--body - reads the comment text from stdin.

Single-shot: one POST, one receipt. The receipt is the full output; --table
is rejected.`, c.kind+" comment add")
	}
	return fmt.Sprintf(`use: forge pr comment N --body T
   or: forge pr comment add N --body T

Add one comment to pr N and print a JSON receipt {id, html_url}.
--body is required; an empty body is a usage error before any request.
--body - reads the comment text from stdin.

Anchor the comment to one diff hunk line with --file P --line L [--side old|new]:
the comment is posted as an inline review comment on that line; --side defaults
to new. A server that cannot anchor the line fails with the server message;
the comment is never posted unanchored.

"pr comment" is the canonical spelling; "pr comment add" is a compatibility alias with
the same receipt.

Single-shot: one POST, one receipt. The receipt is the full output; --table
is rejected.`)
}
