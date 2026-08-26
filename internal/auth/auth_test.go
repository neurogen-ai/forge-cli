package auth

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// shim installs a fake `git` earlier in PATH. mode controls its behaviour:
//
//	fill  -> prints "password=fromfill"
//	empty -> prints unrelated credential lines, no password
//	fail  -> exits 1
//	boom  -> exits 99 (used to prove the shim was never invoked)
//
// Every invocation appends the child's GIT_TERMINAL_PROMPT value to logPath.
func shim(t *testing.T, mode, logPath string) {
	t.Helper()
	dir := t.TempDir()
	script := "#!/bin/sh\n" +
		"printf '%s\\n' \"$GIT_TERMINAL_PROMPT\" >> \"" + logPath + "\"\n" +
		"case \"$FORGE_TEST_SHIM\" in\n" +
		"  fail) exit 1;;\n" +
		"  boom) exit 99;;\n" +
		"  empty) echo \"protocol=https\"; echo \"host=example.com\";;\n" +
		"  *) echo \"password=fromfill\";;\n" +
		"esac\n"
	if err := os.WriteFile(filepath.Join(dir, "git"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("FORGE_TEST_SHIM", mode)
}

func TestResolveExplicitTokenWins(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "shim.log")
	shim(t, "boom", logPath)
	t.Setenv("FORGE_TOKEN", "fromenv")

	got, err := Resolve("example.com", "explicit")
	if err != nil || got != "explicit" {
		t.Fatalf("Resolve = %q, %v; want \"explicit\", nil", got, err)
	}
	if b, _ := os.ReadFile(logPath); len(b) != 0 {
		t.Fatalf("credential fill invoked despite explicit token: log=%q", b)
	}
}

func TestResolveEnvBeatsFill(t *testing.T) {
	shim(t, "fail", filepath.Join(t.TempDir(), "shim.log"))
	t.Setenv("FORGE_TOKEN", "fromenv")

	got, err := Resolve("example.com", "")
	if err != nil || got != "fromenv" {
		t.Fatalf("Resolve = %q, %v; want \"fromenv\", nil", got, err)
	}
}

func TestResolveUsesCredentialFill(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "shim.log")
	shim(t, "fill", logPath)
	t.Setenv("FORGE_TOKEN", "")

	got, err := Resolve("example.com", "")
	if err != nil || got != "fromfill" {
		t.Fatalf("Resolve = %q, %v; want \"fromfill\", nil", got, err)
	}
	b, _ := os.ReadFile(logPath)
	for i, line := range strings.Split(strings.TrimSpace(string(b)), "\n") {
		if line != "0" {
			t.Fatalf("invocation %d: GIT_TERMINAL_PROMPT = %q, want \"0\"", i+1, line)
		}
	}
}

func TestResolveFillEmptyPassword(t *testing.T) {
	shim(t, "empty", filepath.Join(t.TempDir(), "shim.log"))
	t.Setenv("FORGE_TOKEN", "")

	if _, err := Resolve("example.com", ""); !errors.Is(err, ErrNoToken) {
		t.Fatalf("err = %v, want ErrNoToken", err)
	}
}

func TestResolveFillFailure(t *testing.T) {
	shim(t, "fail", filepath.Join(t.TempDir(), "shim.log"))
	t.Setenv("FORGE_TOKEN", "")

	if _, err := Resolve("example.com", ""); !errors.Is(err, ErrNoToken) {
		t.Fatalf("err = %v, want ErrNoToken", err)
	}
}

func TestResolveMissingGit(t *testing.T) {
	dir := t.TempDir() // empty dir: no git on PATH
	t.Setenv("PATH", dir)
	t.Setenv("FORGE_TOKEN", "")

	if _, err := exec.LookPath("git"); err == nil {
		t.Skip("git resolvable outside PATH")
	}
	if _, err := Resolve("example.com", ""); !errors.Is(err, ErrNoToken) {
		t.Fatalf("err = %v, want ErrNoToken", err)
	}
}
