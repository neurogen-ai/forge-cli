package gitctx

import (
	"fmt"
	"os/exec"
	"strings"
)

// Repo describes the git repository forge is running inside.
type Repo struct {
	Root      string // absolute, from `git rev-parse --show-toplevel`
	OriginURL string // `git remote get-url origin`, "" if none
}

// Detect inspects the process working directory. It returns an error when not
// inside a git repository. A missing origin remote is not an error.
func Detect() (Repo, error) {
	return detectIn(".")
}

// detectIn shells out to git with dir as working directory so tests can run it
// against a temporary repository instead of the process cwd.
func detectIn(dir string) (Repo, error) {
	out, err := git(dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return Repo{}, err
	}
	root := out

	var repo Repo
	repo.Root = root

	if urlOut, urlErr := git(dir, "remote", "get-url", "origin"); urlErr == nil {
		repo.OriginURL = urlOut
	}
	return repo, nil
}

// CurrentBranch returns the checked-out branch name of the repo at root, or ""
// when it cannot be determined (detached HEAD, unborn branch, not a repo).
// Used only as a hint for pr create --head default.
func CurrentBranch(root string) string {
	out, err := git(root, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return ""
	}
	return out
}

// git runs git in dir and returns trimmed stdout. A failed command whose
// stderr mentions "not a git repository" becomes the Detect error contract;
// other failures keep their stderr detail.
func git(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" || strings.Contains(msg, "not a git repository") {
			return "", fmt.Errorf("not inside a git repository")
		}
		return "", fmt.Errorf("git %s: %s", args[0], msg)
	}
	return strings.TrimSpace(string(out)), nil
}
