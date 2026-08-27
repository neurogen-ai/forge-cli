package cli

import (
	"fmt"
	"io"
	"strings"
)

// Documented is implemented by commands with a hand-written help page.
// The page's FIRST LINE MUST BE the synopsis: "use: forge <name> [flags] [args]".
type Documented interface {
	HelpPage() string
}

// groupSummary maps a family prefix to the one-line description shown on its
// group page and in top-level usage. New families add one entry here.
var groupSummary = map[string]string{
	"pr":    "pull requests",
	"issue": "issues",
	"cache": "local savedir maintenance",
}

// HelpText returns the command's help page. Fallback for non-Documented
// commands: "use: forge <name>\n\n<Summary()>".
func HelpText(c Command) string {
	if d, ok := c.(Documented); ok {
		return d.HelpPage()
	}
	return "use: forge " + c.Name() + "\n\n" + c.Summary()
}

// PrintHelp writes HelpText(c) followed by a newline.
func PrintHelp(w io.Writer, c Command) {
	fmt.Fprintln(w, HelpText(c))
}

// GroupPrefix returns the shared first token of name ("a" for "a list") when
// two or more registered commands start with that token, else false.
// Grouping derives from registry contents at call time: a later registration
// under an existing single word re-renders that word as a family
// automatically. That is intended, do not fix it.
func (r *Registry) GroupPrefix(name string) (prefix string, ok bool) {
	prefix = name
	if i := strings.IndexByte(name, ' '); i >= 0 {
		prefix = name[:i]
	}
	if prefix == "" {
		return "", false
	}
	members := 0
	for registered := range r.cmds {
		if strings.HasPrefix(registered, prefix+" ") {
			members++
		}
	}
	return prefix, members >= 2
}

// PrintGroupPage writes the family help page: the synopsis, the optional
// groupSummary line (omitted, with its surrounding blank line, when the map
// has no entry or an empty value), one padded line per member subcommand,
// and the per-subcommand hint line.
func (r *Registry) PrintGroupPage(w io.Writer, prefix string) {
	if p := r.groupPages[prefix]; p != "" {
		fmt.Fprintln(w, p)
		return
	}
	fmt.Fprintf(w, "use: forge %s <subcommand> [args]\n", prefix)
	fmt.Fprintln(w)
	if s := groupSummary[prefix]; s != "" {
		fmt.Fprintln(w, s)
	}
	for _, c := range r.Sorted() {
		name := c.Name()
		if !strings.HasPrefix(name, prefix+" ") {
			continue
		}
		fmt.Fprintf(w, "  %-24s %s\n", strings.TrimPrefix(name, prefix+" "), c.Summary())
	}
	fmt.Fprintln(w)
	fmt.Fprintf(w, "run `forge %s <subcommand> -h` for details\n", prefix)
}
