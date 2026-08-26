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

func (c *captureCmd) Name() string        { return c.name }
func (c *captureCmd) Summary() string     { return "capture" }
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
