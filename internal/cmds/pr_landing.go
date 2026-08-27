package cmds

import (
	"fmt"
	"strings"

	"forge/internal/cli"
)

// PRGroupHelpText renders the pr family landing page: the conv help page
// verbatim, then an index of the other pr subcommands built from the
// registered members at call time, so a newly added verb shows up without an
// edit here.
func PRGroupHelpText(reg *cli.Registry) string {
	conv := prConvCmd{}
	page := conv.HelpPage()
	var idx strings.Builder
	for _, c := range reg.Sorted() {
		name := c.Name()
		if !strings.HasPrefix(name, "pr ") || name == "pr conv" || name == "pr conversation" {
			continue
		}
		fmt.Fprintf(&idx, "  %-24s %s\n", strings.TrimPrefix(name, "pr "), c.Summary())
	}
	return page + "\nOther pr commands:\n\n" + idx.String() +
		"\nrun `forge pr <subcommand> -h` for details\n"
}
