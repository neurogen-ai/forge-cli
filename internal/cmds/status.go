package cmds

import (
	"fmt"
	"strconv"

	"forge/internal/api"
	"forge/internal/cli"
	"forge/internal/table"
)

// statusEntry is one row of the dashboard: one thing awaiting the caller.
// Reason records why it is on the dashboard ("assigned", "review-requested",
// "open"); Kind selects how a section renders it ("pr" or "issue").
type statusEntry struct {
	Kind   string `json:"kind"`
	Index  int64  `json:"index"`
	Title  string `json:"title"`
	State  string `json:"state"`
	Reason string `json:"reason"`
	URL    string `json:"url"`
}

// statusSection is one dashboard block. Fetch receives the resolved current
// user so a section never re-resolves "me"; the dashboard calls CurrentUser
// exactly once per invocation and hands the result to every section.
type statusSection interface {
	Title() string
	Fetch(ctx *cli.Ctx, me *api.User) ([]statusEntry, error)
}

// statusSections is the registration order; the dashboard renders sections
// in this order. A new section is one type plus one entry here. The PR
// sections share one openPRCache, so a run fetches the open-PR list once.
func statusSections() []statusSection {
	cache := &openPRCache{}
	return []statusSection{
		reviewRequestSection{cache: cache},
		assignedPRSection{cache: cache},
	}
}

// statusSectionOut is the JSON shape: sections in registration order, each
// with its fetched entries.
type statusSectionOut struct {
	Title   string        `json:"title"`
	Entries []statusEntry `json:"entries"`
}

// statusColumns is the compact entry table shared by all sections.
var statusColumns = []table.Column{
	{Name: "NUMBER", Width: 8},
	{Name: "TITLE", Width: 44},
	{Name: "STATE", Width: 7},
	{Name: "REASON", Width: 20},
}

func statusRows(entries []statusEntry) [][]string {
	rows := make([][]string, 0, len(entries))
	for _, e := range entries {
		rows = append(rows, []string{
			strconv.FormatInt(e.Index, 10),
			e.Title,
			e.State,
			e.Reason,
		})
	}
	return rows
}

// statusCmd renders one repository's action dashboard: registered sections,
// fetched in registration order, each rendered under its heading. Repo
// comes from the caller's repo context; the dashboard never queries a
// second repository. Empty sections are skipped: their heading prints only
// when they have entries.
type statusCmd struct{}

func (statusCmd) Name() string      { return "status" }
func (statusCmd) Summary() string   { return "show one repo's PRs and issues awaiting you" }
func (statusCmd) RequiresAPI() bool { return true }

func (statusCmd) HelpPage() string {
	return `use: forge status

Show everything in this repository awaiting you: review requests made of
you, pull requests assigned to you, and open issues. One repo only; cross-
repo dashboards stay out of scope. Table output on a terminal, JSON
elsewhere. Empty sections are skipped.`
}

func (statusCmd) Run(args []string, ctx *cli.Ctx) error {
	return runStatusDashboard(ctx, statusSections())
}

// runStatusDashboard resolves "me" once, fetches every section in
// registration order, and renders. Sections and the renderer are separated
// so tests can drive the loop with fake sections.
func runStatusDashboard(ctx *cli.Ctx, sections []statusSection) error {
	me, err := ctx.API.CurrentUser()
	if err != nil {
		return mapErr(err)
	}
	out := make([]statusSectionOut, 0, len(sections))
	for _, s := range sections {
		entries, err := s.Fetch(ctx, me)
		if err != nil {
			return mapErr(err)
		}
		if len(entries) == 0 {
			continue
		}
		out = append(out, statusSectionOut{Title: s.Title(), Entries: entries})
	}
	if ctx.OutputIsJSON(ctx.Stdout, true) {
		return writeJSON(ctx.Stdout, out)
	}
	for _, sec := range out {
		fmt.Fprintln(ctx.Stdout, sec.Title)
		if err := table.Render(ctx.Stdout, statusColumns, statusRows(sec.Entries)); err != nil {
			return err
		}
		fmt.Fprintln(ctx.Stdout)
	}
	return nil
}

// StatusCommand registers the top-level status family.
func StatusCommand() cli.Command { return statusCmd{} }
