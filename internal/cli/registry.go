package cli

import (
	"io"

	"forge/internal/api"
	"forge/internal/config"
	"forge/internal/gitctx"
)

// Format is the resolved data-output kind for this invocation.
type Format int

const (
	FormatDefault Format = iota // per-command default, TTY-aware for tables
	FormatJSON                  // --json won
	FormatTable                 // --table|-t won
)

// TableDefaulter marks commands whose human-facing default is a table
// rather than JSON. Declared here so Run can reject --table on commands
// that cannot render one before anything executes.
type TableDefaulter interface{ DefaultIsTable() bool }

// DeclaresTable reports whether c defaults to tabular output.
func DeclaresTable(c Command) bool {
	t, ok := c.(TableDefaulter)
	return ok && t.DefaultIsTable()
}

// Command is a single forge command path, e.g. "pr conversation".
type Command interface {
	Name() string // e.g. "pr conversation"; "" is never valid
	Summary() string
	Run(args []string, ctx *Ctx) error
}

// Ctx is filled progressively by later branches; fields are added ONLY here,
// never as ad-hoc parameters.
type Ctx struct {
	Stdout, Stderr io.Writer
	Verbose        bool
	Help           bool
	GlobalFlags    GlobalFlags

	// Format is set centrally by Run after flag parsing; commands read it,
	// they never write it.
	Format Format

	// Prepare, when non-nil, is called by Run after global flags are parsed and
	// the command resolved, but before Command.Run. It wires runtime
	// dependencies (config, repo, API client) into ctx; errors are mapped to
	// exit codes exactly like command errors.
	Prepare func(ctx *Ctx, cmd Command) error

	// Diagnose re-runs the staged host/token/owner/repo probes after a
	// command has already failed. It is set only for API commands. Returns
	// nil when nothing further should be printed (call sites are expected
	// to have wrapped diagnose-all-clear outcomes themselves).
	Diagnose func() *Error

	// API is nil unless Prepare built it (commands requiring network).
	API *api.Client
	// Cfg is the merged two-layer config; always set once Prepare ran.
	Cfg *config.Config
	// Repo is nil when not inside a git repository.
	Repo *gitctx.Repo
}

type GlobalFlags struct {
	Host, Owner, Repo, Token, ConfigPath string
	TimeoutSeconds                       int
	JSON                                 bool // --json data output
	Table                                bool // --table|-t data output
}

// Registry maps full dotted command paths ("pr conversation") to commands.
type Registry struct {
	cmds map[string]Command
}

func NewRegistry() *Registry {
	return &Registry{cmds: make(map[string]Command)}
}

// Register panics on duplicate Name().
func (r *Registry) Register(cmds ...Command) {
	for _, c := range cmds {
		r.register(c)
	}
}

func (r *Registry) register(c Command) {
	name := c.Name()
	if name == "" {
		panic("cli: Register called with empty command Name()")
	}
	if _, dup := r.cmds[name]; dup {
		panic("cli: duplicate command registered: " + name)
	}
	r.cmds[name] = c
}

// Lookup returns nil if unknown.
func (r *Registry) Lookup(path string) Command {
	return r.cmds[path]
}
