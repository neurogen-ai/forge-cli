package cmds

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"testing"

	"forge/internal/cli"
	"forge/internal/gitctx"
)

// initGitRepo creates a throwaway git repository at a temp dir with local
// identity configured and, when wantCommit is true, exactly one empty commit.
// It returns the repo root.
func initGitRepo(t *testing.T, commits int) string {
	t.Helper()
	root := t.TempDir()
	if out, err := exec.Command("git", "init", root).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	for i := 0; i < commits; i++ {
		cmd := exec.Command("git",
			"-c", "user.email=t@example.com", "-c", "user.name=T",
			"-C", root, "commit", "--allow-empty", "-m",
			fmt.Sprintf("add feature A (%d)", i))
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git commit: %v\n%s", err, out)
		}
	}
	return root
}

func TestResolveCreateDefaultsExplicitTitleSkipsInference(t *testing.T) {
	// Neutralize any ambient environment influence.
	t.Setenv("FORGE_HEAD", "")
	t.Setenv("FORGE_BASE", "")

	// An empty non-git temp dir: every git inference here would fail/return
	// "". Because --title is explicit, none of that is ever consulted, and
	// Resolve still succeeds.
	got, err := ResolveCreateDefaults("t", "h", "b-fix", &gitctx.Repo{Root: "/nonexistent-probe-does-not-matter"}, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != "t" || got.Head != "h" || got.Base != "b-fix" {
		t.Errorf("got %+v", got)
	}
}

func TestResolveCreateDefaultsNoCommitsErrors(t *testing.T) {
	t.Setenv("FORGE_HEAD", "")
	t.Setenv("FORGE_BASE", "")

	// Fresh repo with unborn HEAD: inferred title "" routes through the
	// usage error (head flag is explicit since an unborn branch cannot
	// resolve as head).
	repo := &gitctx.Repo{Root: initGitRepo(t, 0)}
	_, err := ResolveCreateDefaults("", "h", "main-fix", repo, "", nil)
	cerr, ok := err.(*cli.Error)
	if !ok || cerr.Code != cli.ExitUsage {
		t.Fatalf("want ExitUsage cli.Error, got %v", err)
	}
	if cerr.Msg != "no commits to title this pull request" {
		t.Errorf("Msg = %q", cerr.Msg)
	}
	if !strings.Contains(cerr.Hint, "--title") {
		t.Errorf("Hint = %q", cerr.Hint)
	}

	// Zero unique commits: HEAD has a subject, but base == HEAD means
	// base..HEAD is empty. The check runs only because the title was
	// inferred; with an explicit --title this repo would succeed.
	repo = &gitctx.Repo{Root: initGitRepo(t, 2)}
	_, err = ResolveCreateDefaults("", "", "HEAD", repo, "", nil)
	cerr, ok = err.(*cli.Error)
	if !ok || cerr.Code != cli.ExitUsage {
		t.Fatalf("zero unique commits: want ExitUsage cli.Error, got %v", err)
	}
	if cerr.Msg != "no commits to title this pull request" {
		t.Errorf("Msg = %q", cerr.Msg)
	}
}

func TestResolveCreateDefaultsBaseChain(t *testing.T) {
	t.Setenv("FORGE_HEAD", "")

	calls := 0
	apiHit := func() (string, error) { calls++; return "trunk-from-server", nil }

	// 1. --base flag wins outright; apiBase is never consulted.
	got, err := ResolveCreateDefaults("t", "h", "b1", &gitctx.Repo{Root: t.TempDir()}, "cfg-base", apiHit)
	if err != nil {
		t.Fatal(err)
	}
	if got.Base != "b1" || calls != 0 {
		t.Errorf("flag case: Base=%q calls=%d", got.Base, calls)
	}

	// 2. $FORGE_BASE beats config and everything after it.
	t.Setenv("FORGE_BASE", "env-b")
	got, err = ResolveCreateDefaults("t", "h", "", &gitctx.Repo{Root: t.TempDir()}, "cfg-base", apiHit)
	if err != nil {
		t.Fatal(err)
	}
	if got.Base != "env-b" || calls != 0 {
		t.Errorf("env case: Base=%q calls=%d", got.Base, calls)
	}
	t.Setenv("FORGE_BASE", "")

	// 3. [defaults].base config value next.
	got, err = ResolveCreateDefaults("t", "h", "", &gitctx.Repo{Root: t.TempDir()}, "cfg-base", apiHit)
	if err != nil {
		t.Fatal(err)
	}
	if got.Base != "cfg-base" || calls != 0 {
		t.Errorf("config case: Base=%q calls=%d", got.Base, calls)
	}

	// 4. Empty flag/env/config and unusable repo Root (RemoteHead ""):
	// falls through to the API default branch.
	got, err = ResolveCreateDefaults("t", "h", "", &gitctx.Repo{Root: t.TempDir()}, "", apiHit)
	if err != nil {
		t.Fatal(err)
	}
	if got.Base != "trunk-from-server" || calls != 1 {
		t.Errorf("apiBase case: Base=%q calls=%d", got.Base, calls)
	}

	// 5. Everything empty without apiBase → the runtime no-base error.
	_, err = ResolveCreateDefaults("t", "h", "", &gitctx.Repo{Root: t.TempDir()}, "", nil)
	cerr, ok := err.(*cli.Error)
	if !ok || cerr.Code != cli.ExitUsage {
		t.Fatalf("no-base: want ExitUsage cli.Error, got %v", err)
	}
	if cerr.Msg != "no base branch" {
		t.Errorf("Msg = %q", cerr.Msg)
	}
}

func TestResolveCreateDefaultsAPIBaseFailureIsNoBaseError(t *testing.T) {
	t.Setenv("FORGE_HEAD", "")
	t.Setenv("FORGE_BASE", "")

	apiBase := func() (string, error) { return "", errors.New("boom") }
	_, err := ResolveCreateDefaults("t", "h", "", &gitctx.Repo{Root: t.TempDir()}, "", apiBase)
	cerr, ok := err.(*cli.Error)
	if !ok || cerr.Code != cli.ExitUsage {
		t.Fatalf("want ExitUsage cli.Error, got %v", err)
	}
	if cerr.Msg != "no base branch" {
		t.Errorf("Msg = %q", cerr.Msg)
	}
	if !strings.Contains(cerr.Hint, "pass --base") {
		t.Errorf("Hint = %q", cerr.Hint)
	}
}

// End-to-end: pr create with no --title/--base auto-titles from the branch tip
// commit subject and resolves base from the server default_branch endpoint.
func TestPRCreateAutoTitleAndServerDefaultBranch(t *testing.T) {
	t.Setenv("FORGE_HEAD", "")
	t.Setenv("FORGE_BASE", "")

	var posted map[string]any
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/repos/o/r":
			fmt.Fprint(w, `{"default_branch":"trunk"}`)
		case "/api/v1/repos/o/r/pulls":
			if r.Method != http.MethodPost {
				t.Errorf("method = %s, want POST", r.Method)
			}
			if err := json.NewDecoder(r.Body).Decode(&posted); err != nil {
				t.Errorf("decode body: %v", err)
			}
			fmt.Fprint(w, `{"number":9}`)
		default:
			t.Errorf("unexpected path %q", r.URL.Path)
			w.WriteHeader(500)
		}
	}))
	defer ts.Close()

	ctx := testCtx(ts)
	root := initGitRepo(t, 2)
	// The server reports default_branch "trunk"; give the local repo that
	// ref one commit behind HEAD so the release §3 zero-unique-commits
	// preflight can count trunk..HEAD as exactly 1.
	if out, err := exec.Command("git", "-C", root, "branch", "trunk", "HEAD~").CombinedOutput(); err != nil {
		t.Fatalf("git branch trunk: %v\n%s", err, out)
	}
	ctx.Repo = &gitctx.Repo{Root: root}
	if err := (prCreateCmd{}).Run([]string{"--head", "h"}, ctx); err != nil {
		t.Fatal(err)
	}
	if posted["title"] != "add feature A (1)" || posted["base"] != "trunk" || posted["head"] != "h" {
		t.Errorf("POSTed %+v", posted)
	}
}
