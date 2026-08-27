package cli

import (
	"strings"
	"testing"
)

// captureCmd records the args and ctx it was handed.
type captureCmd struct {
	name    string
	gotArgs []string
	gotCtx  *Ctx
}

func (c *captureCmd) Name() string    { return c.name }
func (c *captureCmd) Summary() string { return "capture" }
func (c *captureCmd) Run(args []string, ctx *Ctx) error {
	c.gotArgs = args
	c.gotCtx = ctx
	return nil
}

func newTestReg(t *testing.T, cmd *captureCmd) *Registry {
	t.Helper()
	reg := NewRegistry()
	reg.Register(cmd)
	return reg
}

func runCapture(t *testing.T, argv []string) (*captureCmd, int) {
	t.Helper()
	cmd := &captureCmd{name: "pr list"}
	reg := newTestReg(t, cmd)
	code := Run(argv, reg, &Ctx{Stdout: &strings.Builder{}, Stderr: &strings.Builder{}, Prepare: nil})
	return cmd, code
}

func TestGlobalFlagsAfterCommandPath(t *testing.T) {
	cmd, code := runCapture(t, []string{"pr", "list", "--token", "garb", "--host", "h.example.com"})
	if code != ExitOK {
		t.Fatalf("exit = %d, want %d", code, ExitOK)
	}
	if cmd.gotCtx == nil || cmd.gotCtx.GlobalFlags.Token != "garb" {
		t.Fatalf("token after command path not applied to GlobalFlags: %+v", cmd.gotCtx)
	}
	if cmd.gotCtx.GlobalFlags.Host != "h.example.com" {
		t.Fatalf("host after command path not applied: %+v", cmd.gotCtx.GlobalFlags)
	}
}

func TestVerboseAfterCommandPathShortAndLong(t *testing.T) {
	for _, flag := range []string{"-v", "--verbose"} {
		cmd, code := runCapture(t, []string{"pr", "list", flag})
		if code != ExitOK {
			t.Fatalf("%s: exit = %d, want %d", flag, code, ExitOK)
		}
		if !cmd.gotCtx.Verbose {
			t.Fatalf("%s: Verbose not set when flag appears after command path", flag)
		}
	}
}

func TestUnknownPostCommandFlagsPassThrough(t *testing.T) {
	cmd, code := runCapture(t, []string{"pr", "list", "--state", "open", "--limit", "5", "--title", "x --token y"})
	if code != ExitOK {
		t.Fatalf("exit = %d, want %d", code, ExitOK)
	}
	want := []string{"--state", "open", "--limit", "5", "--title", "x --token y"}
	if strings.Join(cmd.gotArgs, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("args = %v, want %v (unknown flags must pass through untouched)", cmd.gotArgs, want)
	}
}

func TestPostCommandGlobalFlagMissingValueIsUsage(t *testing.T) {
	_, code := runCapture(t, []string{"pr", "list", "--token"})
	if code != ExitUsage {
		t.Fatalf("exit = %d, want %d for missing --token value", code, ExitUsage)
	}
}

func TestPreCommandParsingUnchanged(t *testing.T) {
	cmd, code := runCapture(t, []string{"--host", "pre.example.com", "-v", "pr", "list"})
	if code != ExitOK {
		t.Fatalf("exit = %d, want %d", code, ExitOK)
	}
	if cmd.gotCtx.GlobalFlags.Host != "pre.example.com" || !cmd.gotCtx.Verbose {
		t.Fatalf("leading global flags broken: %+v verbose=%v", cmd.gotCtx.GlobalFlags, cmd.gotCtx.Verbose)
	}
	if len(cmd.gotArgs) != 0 {
		t.Fatalf("command args = %v, want empty", cmd.gotArgs)
	}
}

// helpCmd is a fake command for the end-to-end help cases; one family member
// per prefix keeps GroupPrefix satisfied in every fixture below.
type helpCmd struct {
	name    string
	runErr  error
	ran     bool
	gotArgs []string
}

func (c *helpCmd) Name() string    { return c.name }
func (c *helpCmd) Summary() string { return c.name + " does things" }
func (c *helpCmd) Run(args []string, ctx *Ctx) error {
	c.ran = true
	c.gotArgs = args
	return c.runErr
}

type helpRunResult struct {
	stdout   string
	stderr   string
	prepared bool
	code     int
	do       *helpCmd
}

// runHelp runs argv against a registry holding "x do" and "x undo". runErr is
// what "x do" returns if it executes.
func runHelp(t *testing.T, argv []string, runErr error) helpRunResult {
	t.Helper()
	res := helpRunResult{do: &helpCmd{name: "x do", runErr: runErr}}
	undo := &helpCmd{name: "x undo"}
	var stdout, stderr strings.Builder
	reg := NewRegistry()
	reg.Register(res.do, undo)
	base := &Ctx{
		Stdout: &stdout,
		Stderr: &stderr,
		Prepare: func(ctx *Ctx, cmd Command) error {
			res.prepared = true
			return nil
		},
	}
	res.code = Run(argv, reg, base)
	res.stdout = stdout.String()
	res.stderr = stderr.String()
	_ = undo
	return res
}

func TestGroupHelpFlagListsFamilyPage(t *testing.T) {
	res := runHelp(t, []string{"x", "-h"}, nil)
	if res.code != ExitOK || res.prepared {
		t.Fatalf("exit = %d prepared = %v; want 0, false", res.code, res.prepared)
	}
	if !strings.Contains(res.stdout, "use: forge x <subcommand>") ||
		!strings.Contains(res.stdout, "does things") {
		t.Fatalf("stdout missing x group page:\n%s", res.stdout)
	}
}

func TestCommandHelpSkipsPrepare(t *testing.T) {
	cmdErr := &Error{Code: ExitUsage, Msg: "would fail without args"}
	res := runHelp(t, []string{"x", "do", "-h"}, cmdErr)
	if res.code != ExitOK || res.prepared || res.do.ran {
		t.Fatalf("exit=%d prepared=%v ran=%v; want 0,false,false", res.code, res.prepared, res.do.ran)
	}
	if !strings.HasPrefix(res.stdout, "use: forge x do") {
		t.Fatalf("stdout should start with the command page:\n%s", res.stdout)
	}
}

func TestUsageErrorPrintsCommandPage(t *testing.T) {
	res := runHelp(t, []string{"x", "do"}, &Error{Code: ExitUsage, Msg: "boom"})
	if res.code != ExitUsage {
		t.Fatalf("exit = %d, want %d", res.code, ExitUsage)
	}
	if !strings.Contains(res.stderr, "boom\n\nuse: forge x do") {
		t.Fatalf("stderr missing error + blank line + page:\n%s", res.stderr)
	}
}

func TestRuntimeErrorStaysOneLiner(t *testing.T) {
	res := runHelp(t, []string{"x", "do"}, &Error{Code: ExitRuntime, Msg: "kaboom"})
	if res.code != ExitRuntime {
		t.Fatalf("exit = %d, want %d", res.code, ExitRuntime)
	}
	if !strings.Contains(res.stderr, "kaboom") {
		t.Fatalf("stderr missing original message:\n%s", res.stderr)
	}
	if strings.Contains(res.stderr, "use: forge") || strings.Contains(res.stderr, "\n\n") {
		t.Fatalf("runtime error must not print a help page:\n%s", res.stderr)
	}
}

func TestBareFamilyPathShowsGroupPage(t *testing.T) {
	res := runHelp(t, []string{"x"}, nil)
	if res.code != ExitOK {
		t.Fatalf("exit = %d, want %d", res.code, ExitOK)
	}
	if !strings.Contains(res.stdout, "use: forge x <subcommand>") {
		t.Fatalf("stdout missing group page:\n%s", res.stdout)
	}
}

func TestTrailingHelpSwallowsLaterArgs(t *testing.T) {
	cmdErr := &Error{Code: ExitUsage, Msg: "never happens"}
	res := runHelp(t, []string{"x", "do", "-h", "--title", "z"}, cmdErr)
	if res.code != ExitOK || res.prepared || res.do.ran {
		t.Fatalf("exit=%d prepared=%v ran=%v; want 0,false,false", res.code, res.prepared, res.do.ran)
	}
	if !strings.HasPrefix(res.stdout, "use: forge x do") {
		t.Fatalf("stdout should be the command page ignoring later flags:\n%s", res.stdout)
	}
}

func TestHelpOnlyArgPrintsUsageToStdout(t *testing.T) {
	res := runHelp(t, []string{"-h"}, nil)
	if res.code != ExitOK {
		t.Fatalf("exit = %d, want %d", res.code, ExitOK)
	}
	if !strings.HasPrefix(res.stdout, "usage:") {
		t.Fatalf("stdout missing top-level usage:\n%s", res.stdout)
	}
	if res.stderr != "" {
		t.Fatalf("-h alone must not write to stderr:\n%s", res.stderr)
	}
}

func TestMissingValueAfterResolvePrintsCommandPage(t *testing.T) {
	res := runHelp(t, []string{"x", "do", "--timeout", "x"}, nil)
	if res.code != ExitUsage {
		t.Fatalf("exit = %d, want %d", res.code, ExitUsage)
	}
	if !strings.Contains(res.stderr, "--timeout must be a positive integer\n\nuse: forge x do") {
		t.Fatalf("stderr missing flag error + blank line + command page:\n%s", res.stderr)
	}
}

func TestLeadingUnknownFlagKeepsTopLevelUsageOnly(t *testing.T) {
	res := runHelp(t, []string{"--bogus", "x", "do"}, nil)
	if res.code != ExitUsage {
		t.Fatalf("exit = %d, want %d", res.code, ExitUsage)
	}
	if !strings.Contains(res.stderr, "unknown global flag --bogus") {
		t.Fatalf("stderr missing unknown-flag error:\n%s", res.stderr)
	}
	if !strings.Contains(res.stderr, "commands:") {
		t.Fatalf("stderr missing top-level usage:\n%s", res.stderr)
	}
	if strings.Contains(res.stderr, "use: forge x do") {
		t.Fatalf("leading-flag failure must not print a command page:\n%s", res.stderr)
	}
}

// Diagnose-hook cases for v0.2.0 lazy diagnosis: the hook runs only after a
// Runtime/Network failure, never on Auth/Context exits whose messages already
// name the culprit.

func TestDiagnoseAfterRuntimeFailure(t *testing.T) {
	var stdout, stderr strings.Builder
	reg := NewRegistry()
	reg.Register(&helpCmd{name: "x do", runErr: &Error{Code: ExitRuntime, Msg: "boom"}})
	code := Run([]string{"x", "do"}, reg, &Ctx{
		Stdout:   &stdout,
		Stderr:   &stderr,
		Diagnose: func() *Error { return &Error{Msg: "repo X", Hint: "check owner"} },
	})
	if code != ExitRuntime {
		t.Fatalf("exit = %d, want %d (diagnosis must not upgrade the code)", code, ExitRuntime)
	}
	err := stderr.String()
	i := strings.Index(err, "boom")
	j := strings.Index(err, "repo X")
	if i < 0 || j < i || !strings.Contains(err[i:j], "\n\n") {
		t.Fatalf("stderr must show boom, blank line, then repo X:\n%s", err)
	}
	if !strings.Contains(err, "hint: check owner") {
		t.Fatalf("stderr missing diagnosis hint:\n%s", err)
	}
}

func TestDiagnoseNilSilent(t *testing.T) {
	var stdout, stderr strings.Builder
	reg := NewRegistry()
	reg.Register(&helpCmd{name: "x do", runErr: &Error{Code: ExitNetwork, Msg: "boom"}})
	code := Run([]string{"x", "do"}, reg, &Ctx{
		Stdout:   &stdout,
		Stderr:   &stderr,
		Diagnose: func() *Error { return nil },
	})
	if code != ExitNetwork {
		t.Fatalf("exit = %d, want %d", code, ExitNetwork)
	}
	err := stderr.String()
	if strings.Count(err, "boom") != 1 || strings.Contains(strings.TrimPrefix(err, "error: "), "\n\n") {
		t.Fatalf("nil diagnose must add nothing beyond the original one-liner:\n%q", err)
	}
}

func TestDiagnoseSkippedOnAuthAndContextExits(t *testing.T) {
	for _, c := range []int{ExitAuth, ExitContext} {
		called := 0
		var stderr strings.Builder
		reg := NewRegistry()
		reg.Register(&helpCmd{name: "x do", runErr: &Error{Code: c, Msg: "culprit named", Hint: "fix it"}})
		code := Run([]string{"x", "do"}, reg, &Ctx{
			Stdout: &strings.Builder{},
			Stderr: &stderr,
			Diagnose: func() *Error {
				called++
				return nil
			},
		})
		if code != c {
			t.Fatalf("code %d: exit = %d, want %d", c, code, c)
		}
		if called != 0 {
			t.Errorf("code %d: Diagnose called %d times, want 0 (exit already names the culprit)", c, called)
		}
		err := stderr.String()
		if strings.Count(err, "hint:") != 1 || !strings.Contains(err, "culprit named") {
			t.Errorf("code %d: unexpected output:\n%s", c, err)
		}
	}
}
