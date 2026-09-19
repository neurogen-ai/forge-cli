package cmds

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"forge/internal/cli"
)

// checkoutPRServer returns a fake API serving one PR payload whose head
// fields the test controls, plus a Ctx rooted at root with the given origin.
func checkoutPRServer(t *testing.T, root, origin string, headPayload string) (*httptest.Server, *cli.Ctx) {
	t.Helper()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/repos/o/r/pulls/7" {
			fmt.Fprint(w, headPayload)
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(ts.Close)
	ctx := saveCtx(t, ts, root)
	ctx.Repo.OriginURL = origin
	return ts, ctx
}

func checkoutRequireGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git binary not available")
	}
}

// headSHA returns the HEAD sha of the repo at dir.
func headSHA(t *testing.T, dir string) string {
	t.Helper()
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git rev-parse HEAD: %v: %s", err, out)
	}
	return strings.TrimSpace(string(out))
}

// headState reports whether HEAD is detached in the repo at dir.
func headDetached(t *testing.T, dir string) bool {
	t.Helper()
	cmd := exec.Command("git", "symbolic-ref", "-q", "--short", "HEAD")
	cmd.Dir = dir
	return cmd.Run() != nil
}

// initCheckoutRepo creates a git repository at dir with one commit and an
// optional origin remote (mirrors the gitctx test harness).
func initCheckoutRepo(t *testing.T, dir, origin string) {
	t.Helper()
	for _, args := range [][]string{
		{"init"},
		{"config", "user.name", "test"},
		{"config", "user.email", "test@example.com"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("git", "add", ".")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git add: %v: %s", err, out)
	}
	cmd = exec.Command("git", "commit", "-m", "init")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git commit: %v: %s", err, out)
	}
	if origin != "" {
		cmd = exec.Command("git", "remote", "add", "origin", origin)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git remote add: %v: %s", err, out)
		}
	}
}

// setupPRSource creates a source repository with a feature branch and
// returns its path and the feature branch's tip sha.
func setupPRSource(t *testing.T) (string, string) {
	t.Helper()
	src := t.TempDir()
	initCheckoutRepo(t, src, "")
	run := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = src
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	run("checkout", "-b", "feature")
	// Change a file that also exists on the base commit so a conflicting
	// dirty worktree in the target repo gets git's own refusal.
	if err := os.WriteFile(filepath.Join(src, "README.md"), []byte("feature\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", ".")
	run("commit", "-m", "feature")
	return src, headSHA(t, src)
}

func TestPRCheckoutSameRepoDetached(t *testing.T) {
	checkoutRequireGit(t)
	src, sha := setupPRSource(t)
	root := t.TempDir()
	initCheckoutRepo(t, root, src) // same-repo head: origin is the PR's repository

	payload := fmt.Sprintf(`{"number":7,"head":{"ref":"feature","sha":%q,"repo":null}}`, sha)
	_, ctx := checkoutPRServer(t, root, src, payload)

	if err := (prCheckoutCmd{}).Run([]string{"7"}, ctx); err != nil {
		t.Fatalf("pr checkout: %v", err)
	}
	if got := headSHA(t, root); got != sha {
		t.Errorf("HEAD = %s, want %s", got, sha)
	}
	if !headDetached(t, root) {
		t.Error("HEAD is not detached after pr checkout without --branch")
	}
	var receipt CheckoutReceipt
	if err := json.Unmarshal(ctx.Stdout.(*bytes.Buffer).Bytes(), &receipt); err != nil {
		t.Fatalf("receipt decode: %v; stdout=%q", err, ctx.Stdout)
	}
	if receipt.HeadSHA != sha || !receipt.Detached || receipt.Branch != "" {
		t.Errorf("receipt = %+v", receipt)
	}
}

func TestPRCheckoutCrossRepo(t *testing.T) {
	checkoutRequireGit(t)
	// The head lives in another repository entirely.
	src, sha := setupPRSource(t)
	root := t.TempDir()
	initCheckoutRepo(t, root, t.TempDir()) // origin is NOT the head repo

	payload := fmt.Sprintf(`{"number":7,"head":{"ref":"feature","sha":%q,"repo":{"clone_url":%q}}}`, sha, src)
	_, ctx := checkoutPRServer(t, root, "", payload)

	if err := (prCheckoutCmd{}).Run([]string{"7"}, ctx); err != nil {
		t.Fatalf("pr checkout: %v", err)
	}
	if got := headSHA(t, root); got != sha {
		t.Errorf("HEAD = %s, want %s", got, sha)
	}
	if !headDetached(t, root) {
		t.Error("HEAD is not detached after cross-repo pr checkout")
	}
}

func TestPRCheckoutBranchNamesLocalBranch(t *testing.T) {
	checkoutRequireGit(t)
	src, sha := setupPRSource(t)
	root := t.TempDir()
	initCheckoutRepo(t, root, src)

	payload := fmt.Sprintf(`{"number":7,"head":{"ref":"feature","sha":%q,"repo":null}}`, sha)
	_, ctx := checkoutPRServer(t, root, src, payload)

	if err := (prCheckoutCmd{}).Run([]string{"--branch", "pr-7", "7"}, ctx); err != nil {
		t.Fatalf("pr checkout --branch: %v", err)
	}
	if got := headSHA(t, root); got != sha {
		t.Errorf("HEAD = %s, want %s", got, sha)
	}
	if headDetached(t, root) {
		t.Error("HEAD is detached after pr checkout --branch; want branch checkout")
	}
	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(string(out)); got != "pr-7" {
		t.Errorf("HEAD branch = %q, want pr-7", got)
	}
}

func TestPRCheckoutMissingHead(t *testing.T) {
	checkoutRequireGit(t)
	root := t.TempDir()
	initCheckoutRepo(t, root, t.TempDir())
	_, ctx := checkoutPRServer(t, root, "", `{"number":7,"head":{"ref":"","sha":"","repo":null}}`)

	err := (prCheckoutCmd{}).Run([]string{"7"}, ctx)
	if err == nil {
		t.Fatal("pr checkout with no head: want error, got nil")
	}
	cliErr, ok := err.(*cli.Error)
	if !ok || cliErr.Code != cli.ExitRuntime {
		t.Errorf("error = %v, want exit-code 1 runtime error", err)
	}
}

func TestPRCheckoutDirtyWorktreeRefused(t *testing.T) {
	checkoutRequireGit(t)
	src, sha := setupPRSource(t)
	root := t.TempDir()
	initCheckoutRepo(t, root, src)
	// Conflicting uncommitted change to the file the checkout would replace.
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("dirty\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	payload := fmt.Sprintf(`{"number":7,"head":{"ref":"feature","sha":%q,"repo":null}}`, sha)
	_, ctx := checkoutPRServer(t, root, src, payload)

	if err := (prCheckoutCmd{}).Run([]string{"7"}, ctx); err == nil {
		t.Fatal("pr checkout over a conflicting dirty worktree: want git refusal, got nil")
	}
	if got := headSHA(t, root); got == sha {
		t.Error("dirty worktree was overwritten; git's refusal did not hold")
	}
}

func TestPRCheckoutNoOrigin(t *testing.T) {
	checkoutRequireGit(t)
	_, sha := setupPRSource(t)
	root := t.TempDir()
	initCheckoutRepo(t, root, "") // no origin remote, same-repo head

	payload := fmt.Sprintf(`{"number":7,"head":{"ref":"feature","sha":%q,"repo":null}}`, sha)
	_, ctx := checkoutPRServer(t, root, "", payload)

	err := (prCheckoutCmd{}).Run([]string{"7"}, ctx)
	if err == nil {
		t.Fatal("pr checkout with no origin remote: want error, got nil")
	}
	cliErr, ok := err.(*cli.Error)
	if !ok || cliErr.Code != cli.ExitContext {
		t.Errorf("error = %v, want exit-code 3 repo-context error", err)
	}
}

func TestPRCheckoutSHAMismatch(t *testing.T) {
	checkoutRequireGit(t)
	src, _ := setupPRSource(t)
	root := t.TempDir()
	initCheckoutRepo(t, root, src)
	start := headSHA(t, root)

	// The API reports a head sha that differs from the branch tip git would
	// actually fetch: pr checkout must refuse, not check out the wrong commit.
	payload := fmt.Sprintf(`{"number":7,"head":{"ref":"feature","sha":"%064d","repo":null}}`, 1)
	_, ctx := checkoutPRServer(t, root, src, payload)

	err := (prCheckoutCmd{}).Run([]string{"7"}, ctx)
	if err == nil {
		t.Fatal("pr checkout with mismatched head sha: want error, got nil")
	}
	cliErr, ok := err.(*cli.Error)
	if !ok || cliErr.Code != cli.ExitRuntime {
		t.Errorf("error = %v, want exit-code 1 runtime error", err)
	}
	if got := headSHA(t, root); got != start {
		t.Errorf("HEAD moved to %s on a mismatched checkout; want it untouched", got)
	}
}

func TestPRCheckoutExistingBranchRefusedBeforeFetch(t *testing.T) {
	checkoutRequireGit(t)
	_, sha := setupPRSource(t)
	root := t.TempDir()
	initCheckoutRepo(t, root, "/nonexistent/forge-remote") // fetch would fail
	cmd := exec.Command("git", "branch", "pr-7", "HEAD")
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git branch pr-7: %v: %s", err, out)
	}

	payload := fmt.Sprintf(`{"number":7,"head":{"ref":"feature","sha":%q,"repo":null}}`, sha)
	_, ctx := checkoutPRServer(t, root, "/nonexistent/forge-remote", payload)

	err := (prCheckoutCmd{}).Run([]string{"--branch", "pr-7", "7"}, ctx)
	if err == nil {
		t.Fatal("pr checkout over an existing local branch: want error, got nil")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("error = %v, want the branch-exists refusal (before any fetch)", err)
	}
}
