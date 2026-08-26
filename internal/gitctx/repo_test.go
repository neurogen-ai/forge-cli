package gitctx

import (
	"os"
	"os/exec"
	"testing"
)

func requireGit(t *testing.T) string {
	t.Helper()
	p, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git binary not available")
	}
	return p
}

// initRepo creates a git repository in dir with one commit so HEAD resolves,
// and optionally adds an origin remote.
func initRepo(t *testing.T, dir, originURL string) {
	t.Helper()
	requireGit(t)
	run := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	for _, args := range [][]string{
		{"init"},
		{"config", "user.name", "test"},
		{"config", "user.email", "test@example.com"},
	} {
		run(args...)
	}
	if err := os.WriteFile(dir+"/README.md", []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", ".")
	run("commit", "-m", "init")
	if originURL != "" {
		run("remote", "add", "origin", originURL)
	}
}

func TestDetectInRepoWithOrigin(t *testing.T) {
	dir := t.TempDir()
	initRepo(t, dir, "https://git.example.com/alice/proj.git")

	repo, err := detectIn(dir)
	if err != nil {
		t.Fatalf("detectIn: %v", err)
	}
	if repo.Root == "" || !filepathIsAbs(repo.Root) {
		t.Errorf("Root = %q, want absolute path", repo.Root)
	}
	if repo.OriginURL != "https://git.example.com/alice/proj.git" {
		t.Errorf("OriginURL = %q, want https://git.example.com/alice/proj.git", repo.OriginURL)
	}
}

func TestDetectInRepoWithoutOrigin(t *testing.T) {
	dir := t.TempDir()
	initRepo(t, dir, "")

	repo, err := detectIn(dir)
	if err != nil {
		t.Fatalf("detectIn: %v", err)
	}
	if repo.OriginURL != "" {
		t.Errorf("OriginURL = %q, want empty when no origin remote", repo.OriginURL)
	}
}

func TestDetectOutsideRepo(t *testing.T) {
	dir := t.TempDir() // no git init

	_, err := detectIn(dir)
	if err == nil {
		t.Fatal("detectIn outside a repository: want error, got nil")
	}
	if want := "not inside a git repository"; err.Error() != want {
		t.Errorf("error = %q, want %q", err.Error(), want)
	}
}

func TestCurrentBranch(t *testing.T) {
	dir := t.TempDir()
	initRepo(t, dir, "") // one commit, so HEAD is born

	got := CurrentBranch(dir)
	if got == "" {
		t.Fatal("CurrentBranch = \"\", want non-empty branch name")
	}
}

func TestCurrentBranchOutsideRepoIsEmpty(t *testing.T) {
	if got := CurrentBranch(t.TempDir()); got != "" {
		t.Errorf("CurrentBranch outside repo = %q, want \"\"", got)
	}
}

func filepathIsAbs(p string) bool {
	return len(p) > 0 && os.IsPathSeparator(p[0])
}
