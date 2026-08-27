package cmds

import (
	"strings"
	"testing"

	"forge/internal/cli"
)

func TestPRGroupHelpText(t *testing.T) {
	reg := cli.NewRegistry()
	reg.Register(PRCommands()...)
	reg.Register(PullCommands()...) // "pr pull" must appear in the index too
	page := PRGroupHelpText(reg)

	// Starts verbatim with conv's own use-line.
	const firstLine = "use: forge pr conv N [--all] [--min-unresolved N]"
	if !strings.HasPrefix(page, firstLine) {
		t.Errorf("page does not start with %q:\n%s", firstLine, page)
	}

	// Exactly one occurrence of the conv page (the landing copy), no second.
	if got := strings.Count(page, "use: forge pr conv N"); got != 1 {
		t.Errorf("conv page appears %d times, want 1", got)
	}

	// Index body must not mention conv or conversation at all.
	// Index entries must not be the conv commands themselves (conv's own page
	// already leads; "conversation" is a removed shim). Summaries may mention
	// the word conversation (e.g. pr pull), so match entry names only.
	idx := page[strings.Index(page, "Other pr commands:"):]
	for _, bad := range []string{"\n  conv ", "\n  conversation "} {
		if strings.Contains(idx, bad) {
			t.Errorf("index body contains entry %q:\n%s", bad, idx)
		}
	}

	// One index line per non-conv verb registered by PRCommands().
	for _, want := range []string{
		"  create", "  get", "  list", "  review list",
		"  comment resolve", "  comment unresolve", "  resolve-all",
		"  pull",
	} {
		if !strings.Contains(idx, want+" ") && !strings.Contains(idx, want+"\n") {
			t.Errorf("index missing entry %q\n%s", want, idx)
		}
	}

	// Ends with the hint line.
	if tail := "\nrun `forge pr <subcommand> -h` for details\n"; !strings.HasSuffix(page, tail) {
		t.Errorf("page does not end with hint line; got ending: %q", page[len(page)-60:])
	}
}
