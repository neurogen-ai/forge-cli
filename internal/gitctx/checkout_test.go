package gitctx

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// commitFile writes content to path in dir, commits it, and returns the new
// HEAD sha.
func commitFile(t *testing.T, dir, path, content string) string {
	t.Helper()
	run := gitRunner(t, dir)
	if err := os.MkdirAll(filepath.Join(dir, filepath.Dir(path)), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, path), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", ".")
	run("commit", "-m", "update "+path)
	return revParse(t, dir, "HEAD")
}

// revParse resolves ref in dir with the fail-fast runner.
func revParse(t *testing.T, dir, ref string) string {
	t.Helper()
	cmd := exec.Command("git", "rev-parse", ref)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git rev-parse %s: %v: %s", ref, err, out)
	}
	return strings.TrimSpace(string(out))
}

func TestFetchSHAResolvesFetchedRef(t *testing.T) {
	requireGit(t)
	remote := t.TempDir()
	local := t.TempDir()
	initRepo(t, remote, "")
	initRepo(t, local, "")
	run := gitRunner(t, remote)
	run("checkout", "-b", "feature")
	want := commitFile(t, remote, "feature.go", "f\n")

	got, err := FetchSHA(local, remote, "feature")
	if err != nil {
		t.Fatalf("FetchSHA: %v", err)
	}
	if got != want {
		t.Errorf("FetchSHA = %s, want %s", got, want)
	}
}

func TestFetchSHAOutsideRepo(t *testing.T) {
	requireGit(t)
	dir := t.TempDir() // not a git repository
	if _, err := FetchSHA(dir, t.TempDir(), "main"); err == nil {
		t.Fatal("FetchSHA outside a repository: want error, got nil")
	} else if !strings.Contains(err.Error(), "not inside a git repository") {
		t.Errorf("error = %v, want the repo-context contract", err)
	}
}

func TestFetchSHATagDoesNotShadowBranch(t *testing.T) {
	requireGit(t)
	remote := t.TempDir()
	local := t.TempDir()
	initRepo(t, remote, "")
	initRepo(t, local, "")
	run := gitRunner(t, remote)
	run("tag", "feature") // a tag sharing the branch's name, at the base commit
	run("checkout", "-b", "feature")
	want := commitFile(t, remote, "feature.go", "branch\n")

	got, err := FetchSHA(local, remote, "feature")
	if err != nil {
		t.Fatalf("FetchSHA: %v", err)
	}
	if got != want {
		t.Errorf("FetchSHA = %s, want the branch tip %s (tag must not shadow)", got, want)
	}
}

func TestFetchSHAUnknownRef(t *testing.T) {
	requireGit(t)
	remote := t.TempDir()
	local := t.TempDir()
	initRepo(t, remote, "")
	initRepo(t, local, "")
	if _, err := FetchSHA(local, remote, "no-such-branch"); err == nil {
		t.Fatal("FetchSHA with unknown ref: want error, got nil")
	}
}

func TestCheckoutDetachedMovesHead(t *testing.T) {
	requireGit(t)
	dir := t.TempDir()
	initRepo(t, dir, "")
	first := revParse(t, dir, "HEAD")
	second := commitFile(t, dir, "b.txt", "b\n")

	if err := CheckoutDetached(dir, first); err != nil {
		t.Fatalf("CheckoutDetached: %v", err)
	}
	if got := revParse(t, dir, "HEAD"); got != first {
		t.Errorf("HEAD = %s, want %s", got, first)
	}
	ref := exec.Command("git", "symbolic-ref", "-q", "--short", "HEAD")
	ref.Dir = dir
	if err := ref.Run(); err == nil {
		t.Error("HEAD still resolves to a branch after CheckoutDetached; want detached")
	}

	if err := CheckoutDetached(dir, second); err != nil {
		t.Fatalf("CheckoutDetached back: %v", err)
	}
	if got := revParse(t, dir, "HEAD"); got != second {
		t.Errorf("HEAD = %s, want %s", got, second)
	}
}

func TestCheckoutDetachedRefusesConflictingDirtyWorktree(t *testing.T) {
	requireGit(t)
	dir := t.TempDir()
	initRepo(t, dir, "")
	first := revParse(t, dir, "HEAD")
	commitFile(t, dir, "a.txt", "second\n")
	// Dirty a.txt so checking out first would overwrite uncommitted state.
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("dirty\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := CheckoutDetached(dir, first); err == nil {
		t.Fatal("CheckoutDetached over conflicting dirty worktree: want git refusal, got nil")
	}
}

func TestCreateBranchAtPointsAtSHA(t *testing.T) {
	requireGit(t)
	dir := t.TempDir()
	initRepo(t, dir, "")
	sha := revParse(t, dir, "HEAD")

	if err := CreateBranchAt(dir, "pr-5", sha); err != nil {
		t.Fatalf("CreateBranchAt: %v", err)
	}
	if got := revParse(t, dir, "pr-5"); got != sha {
		t.Errorf("branch pr-5 = %s, want %s", got, sha)
	}

	// An existing branch must be refused, not moved.
	if err := CreateBranchAt(dir, "pr-5", sha); err == nil {
		t.Fatal("CreateBranchAt over an existing branch: want error, got nil")
	}
}
