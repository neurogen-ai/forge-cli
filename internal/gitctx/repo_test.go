package gitctx

import (
	"os"
	"os/exec"
	"strconv"
	"strings"
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

func TestCommitSubject(t *testing.T) {
	dir := t.TempDir()
	initRepo(t, dir, "")

	run := gitRunner(t, dir)
	run("checkout", "-b", "topic")
	if err := os.WriteFile(dir+"/file.txt", []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", ".")
	run("commit", "-m", "feat: add file\n\nbody line")

	if got := CommitSubject(dir, "topic"); got != "feat: add file" {
		t.Errorf("CommitSubject = %q, want %q", got, "feat: add file")
	}
}

func TestCommitSubjectMissingRefIsEmpty(t *testing.T) {
	dir := t.TempDir()
	initRepo(t, dir, "")

	if got := CommitSubject(dir, "no-such-ref"); got != "" {
		t.Errorf("CommitSubject of missing ref = %q, want \"\"", got)
	}
}

func TestBranchTipDate(t *testing.T) {
	dir := t.TempDir()
	initRepo(t, dir, "")

	run := gitRunner(t, dir)
	run("checkout", "-b", "topic")
	if err := os.WriteFile(dir+"/file.txt", []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", ".")
	run("commit", "-m", "tip commit")

	cmd := exec.Command("git", "log", "-1", "--format=%ct", "topic")
	cmd.Dir = dir
	wantOut, err := cmd.Output()
	if err != nil {
		t.Fatalf("git log committerdate: %v", err)
	}
	want, err := strconv.ParseInt(strings.TrimSpace(string(wantOut)), 10, 64)
	if err != nil {
		t.Fatalf("parsing committer date %q: %v", wantOut, err)
	}

	if got := BranchTipDate(dir, "topic"); got != want {
		t.Errorf("BranchTipDate = %d, want %d", got, want)
	}
}

func TestBranchTipDateMissingRefIsZero(t *testing.T) {
	dir := t.TempDir()
	initRepo(t, dir, "")

	if got := BranchTipDate(dir, "no-such-ref"); got != 0 {
		t.Errorf("BranchTipDate of missing ref = %d, want 0", got)
	}
}

func TestUniqueCommitCount(t *testing.T) {
	dir := t.TempDir()
	initRepo(t, dir, "")

	run := gitRunner(t, dir)
	base := CurrentBranch(dir) // default branch, main or master depending on git

	run("checkout", "-b", "feature")

	commit := func(msg string) {
		t.Helper()
		if err := os.WriteFile(dir+"/f.txt", []byte(msg+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		run("add", ".")
		run("commit", "-m", msg)
	}

	commit("first unique commit")
	n, err := UniqueCommitCount(dir, base, "feature")
	if err != nil {
		t.Fatalf("UniqueCommitCount: %v", err)
	}
	if n != 1 {
		t.Errorf("after one commit: count = %d, want 1", n)
	}

	commit("second unique commit")
	n, err = UniqueCommitCount(dir, base, "feature")
	if err != nil {
		t.Fatalf("UniqueCommitCount second pass: %v", err)
	}
	if n != 2 {
		t.Errorf("after two commits: count = %d, want 2", n)
	}
}

func TestUniqueCommitCountMissingBaseErrors(t *testing.T) {
	dir := t.TempDir()
	initRepo(t, dir, "")

	if _, err := UniqueCommitCount(dir, "no-such-base", "HEAD"); err == nil {
		t.Error("UniqueCommitCount with missing base: want error, got nil")
	}
}

func TestLocalBranchesContentsAndOrdering(t *testing.T) {
	dir := t.TempDir()
	initRepo(t, dir, "")

	run := gitRunner(t, dir)
	run("branch", "alpha")
	run("branch", "beta")

	branches := LocalBranches(dir)
	if len(branches) != 3 {
		t.Fatalf("LocalBranches = %v, want 3 branches", branches)
	}
	for i := 1; i < len(branches); i++ {
		if branches[i-1] >= branches[i] {
			t.Errorf("LocalBranches not sorted: %v", branches)
		}
	}
	found := map[string]bool{}
	for _, b := range branches {
		found[b] = true
	}
	base := CurrentBranch(dir)
	for _, want := range []string{base, "alpha", "beta"} {
		if !found[want] {
			t.Errorf("LocalBranches missing branch %q (got %v)", want, branches)
		}
	}
}

// setOriginHead creates refs/remotes/origin/<branch> at HEAD and points
// refs/remotes/origin/HEAD at it, simulating a cloned remote's default branch.
func setOriginHead(t *testing.T, dir, branch string) {
	t.Helper()
	run := gitRunner(t, dir)
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = dir
	shaOut, err := cmd.Output()
	if err != nil {
		t.Fatalf("git rev-parse HEAD: %v: %s", err, shaOut)
	}
	sha := strings.TrimSpace(string(shaOut))
	run("update-ref", "refs/remotes/origin/"+branch, sha)
	run("symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/"+branch)
}

func TestRemoteHeadPointsAtDefaultBranch(t *testing.T) {
	dir := t.TempDir()
	initRepo(t, dir, "https://git.example.com/alice/proj.git")

	branch := CurrentBranch(dir) // main or master depending on git
	setOriginHead(t, dir, branch)

	if got := RemoteHead(dir); got != branch {
		t.Errorf("RemoteHead = %q, want %q", got, branch)
	}
}

func TestRemoteHeadUnsetWhenNoRemoteRefs(t *testing.T) {
	dir := t.TempDir()
	initRepo(t, dir, "https://git.example.com/alice/proj.git")

	if got := RemoteHead(dir); got != "" {
		t.Errorf("RemoteHead without origin/HEAD = %q, want \"\"", got)
	}
}

// gitRunner returns a run helper that fails the test when a git command in
// dir errors.
func gitRunner(t *testing.T, dir string) func(args ...string) {
	t.Helper()
	requireGit(t)
	return func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
}

func filepathIsAbs(p string) bool {
	return len(p) > 0 && os.IsPathSeparator(p[0])
}
