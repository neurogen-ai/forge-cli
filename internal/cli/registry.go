package cli

import (
	"io"

	"forge/internal/api"
	"forge/internal/config"
	"forge/internal/gitctx"
)

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
	GlobalFlags    GlobalFlags

	// Prepare, when non-nil, is called by Run after global flags are parsed and
	// the command resolved, but before Command.Run. It wires runtime
	// dependencies (config, repo, API client) into ctx; errors are mapped to
	// exit codes exactly like command errors.
	Prepare func(ctx *Ctx, cmd Command) error

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
}

// Registry maps full dotted command paths ("pr conversation") to commands.
type Registry struct {
	cmds map[string]Command
}

func NewRegistry() *Registry {
	return &Registry{cmds: make(map[string]Command)}
}

// Register panics on duplicate Name().
func (r *Registry) Register(c Command) {
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
