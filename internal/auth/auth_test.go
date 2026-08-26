package auth

import (
	"bytes"
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

// TestResolveNonTerminalStdinDoesNotPrompt: when the non-interactive fill
// finds nothing and stdin is not a terminal (pipes, CI, tests), Resolve must
// return ErrNoToken after exactly ONE fill attempt — no interactive retry.
func TestResolveNonTerminalStdinDoesNotPrompt(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "shim.log")
	shim(t, "empty", logPath)
	t.Setenv("FORGE_TOKEN", "")

	prev := stdinIsTerminal
	stdinIsTerminal = func() bool { return false }
	t.Cleanup(func() { stdinIsTerminal = prev })

	if _, err := Resolve("example.com", ""); !errors.Is(err, ErrNoToken) {
		t.Fatalf("err = %v, want ErrNoToken", err)
	}
	b, _ := os.ReadFile(logPath)
	if got := strings.Count(strings.TrimSpace(string(b)), "\n") + 1; string(b) != "" && got != 1 {
		t.Fatalf("credential fill invoked %d times, want exactly 1", got)
	}
}

// shimStage installs a fake `git` whose behaviour changes per invocation:
// call 1 exits 1 (non-interactive fill finds nothing), call 2 prints a
// password as if the user had typed it at git's prompt. Each invocation logs
// the child's GIT_TERMINAL_PROMPT value to logPath.
func shimStage(t *testing.T, logPath string) {
	t.Helper()
	dir := t.TempDir()
	countPath := filepath.Join(dir, "count")
	script := "#!/bin/sh\n" +
		"printf '%s\\n' \"$GIT_TERMINAL_PROMPT\" >> \"" + logPath + "\"\n" +
		"n=$(cat \"" + countPath + "\" 2>/dev/null || echo 0)\n" +
		"n=$((n+1)); echo $n > \"" + countPath + "\"\n" +
		"if [ \"$n\" = 1 ]; then exit 1; fi\n" +
		"echo \"password=fromprompt\"\n"
	if err := os.WriteFile(filepath.Join(dir, "git"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestResolveFallsBackToInteractivePrompt(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "shim.log")
	shimStage(t, logPath)
	t.Setenv("FORGE_TOKEN", "")

	prev := stdinIsTerminal
	stdinIsTerminal = func() bool { return true }
	t.Cleanup(func() { stdinIsTerminal = prev })

	got, err := Resolve("example.com", "")
	if err != nil || got != "fromprompt" {
		t.Fatalf("Resolve = %q, %v; want \"fromprompt\", nil", got, err)
	}
	b, _ := os.ReadFile(logPath)
	lines := strings.Split(strings.TrimSuffix(string(b), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected exactly 2 fill invocations, log=%q", b)
	}
	if lines[0] != "0" {
		t.Fatalf("first invocation: GIT_TERMINAL_PROMPT = %q, want \"0\"", lines[0])
	}
	if lines[1] == "0" {
		t.Fatal("second (interactive) invocation still had GIT_TERMINAL_PROMPT=0")
	}
}

func TestCredentialFillInteractivePassesRequestAndParsesPassword(t *testing.T) {
	dir := t.TempDir()
	stdinPath := filepath.Join(dir, "child-stdin")
	script := "#!/bin/sh\n" +
		"cat > \"" + stdinPath + "\"\n" +
		"echo \"username=u\"\n" +
		"echo \"password=typed\"\n"
	if err := os.WriteFile(filepath.Join(dir, "git"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	var stderr bytes.Buffer
	got, err := credentialFillInteractive("example.com", strings.NewReader("user input\n"), &stderr)
	if err != nil || got != "typed" {
		t.Fatalf("credentialFillInteractive = %q, %v; want \"typed\", nil", got, err)
	}
	in, _ := os.ReadFile(stdinPath)
	wantPrefix := "protocol=https\nhost=example.com\n\n"
	if !strings.HasPrefix(string(in), wantPrefix) {
		t.Fatalf("child stdin = %q, want prefix %q", in, wantPrefix)
	}
	if !strings.HasSuffix(string(in), "user input\n") {
		t.Fatalf("child stdin lost simulated terminal input: %q", in)
	}
}

func TestCredentialFillInteractiveEmptyPassword(t *testing.T) {
	dir := t.TempDir()
	script := "#!/bin/sh\necho \"protocol=https\"\n" // no password line
	if err := os.WriteFile(filepath.Join(dir, "git"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	if _, err := credentialFillInteractive("example.com", strings.NewReader(""), nil); !errors.Is(err, ErrNoToken) {
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
