package cmds

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"testing"

	"forge/internal/cli"
	"forge/internal/config"
	"forge/internal/gitctx"
)

// batchRepo creates a temp git repo with the named branches, each carrying a
// commit with the given subject on top of an initial commit on main.
func batchRepo(t *testing.T, branches map[string]string) *gitctx.Repo {
	t.Helper()
	dir := t.TempDir()
	run := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	for _, args := range [][]string{
		{"init", "-q", "-b", "main"},
		{"config", "user.name", "test"},
		{"config", "user.email", "test@example.com"},
	} {
		run(args...)
	}
	run("commit", "--allow-empty", "-m", "init")
	for branch, subject := range branches {
		run("branch", branch)
		cmd := exec.Command("git", "checkout", "-q", branch)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git checkout %s: %v: %s", branch, err, out)
		}
		run("commit", "--allow-empty", "--allow-empty-message", "-m", subject)
	}
	cmd := exec.Command("git", "checkout", "-q", "main")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git checkout main: %v: %s", err, out)
	}
	return &gitctx.Repo{Root: dir}
}

// batchServer returns an httptest server serving the repository default
// branch, and fails the test immediately if POST .../pulls is ever hit.
func batchServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/pulls") {
			t.Errorf("POST %s during dry run", r.URL.Path)
			w.WriteHeader(500)
			return
		}
		fmt.Fprint(w, `{"default_branch":"main"}`)
	}))
}

func TestBatchGlobDoesNotCrossSlash(t *testing.T) {
	repo := batchRepo(t, map[string]string{
		"v0.3.0-a":       "a",
		"release/v0.3.0": "r",
	})
	out := &strings.Builder{}
	stderr := &strings.Builder{}
	ctx := &cli.Ctx{Stdout: out, Stderr: stderr, GlobalFlags: cli.GlobalFlags{Host: "git.example.com", Owner: "o", Repo: "r"}, Repo: repo, Cfg: &config.Config{Defaults: config.Defaults{Base: "main"}}, API: nil}
	err := (createBatchCmd{}).Run([]string{"v0.3.0*"}, ctx)
	if err != nil {
		t.Fatal(err)
	}
	var items []BatchReceiptItem
	if err := json.Unmarshal([]byte(out.String()), &items); err != nil {
		t.Fatalf("plan JSON: %v", err)
	}
	if len(items) != 1 || items[0].Branch != "v0.3.0-a" {
		t.Fatalf("items = %+v, want only v0.3.0-a", items)
	}
}

func TestBatchPlanSortedLexically(t *testing.T) {
	repo := batchRepo(t, map[string]string{"b2": "two", "b1": "one", "b3": "three"})
	out := &strings.Builder{}
	ctx := &cli.Ctx{Stdout: out, Stderr: &strings.Builder{}, GlobalFlags: cli.GlobalFlags{Host: "git.example.com", Owner: "o", Repo: "r"}, Repo: repo, Cfg: &config.Config{Defaults: config.Defaults{Base: "main"}}}
	if err := (createBatchCmd{}).Run([]string{"b*"}, ctx); err != nil {
		t.Fatal(err)
	}
	var items []BatchReceiptItem
	if err := json.Unmarshal([]byte(out.String()), &items); err != nil {
		t.Fatalf("plan JSON: %v", err)
	}
	if len(items) != 3 {
		t.Fatalf("items = %+v, want 3", items)
	}
	for i, want := range []string{"b1", "b2", "b3"} {
		if items[i].Branch != want {
			t.Fatalf("items[%d].Branch = %q, want %q", i, items[i].Branch, want)
		}
	}
}

func TestBatchSkippedEmptySubject(t *testing.T) {
	repo := batchRepo(t, map[string]string{"empty": "", "good": "real title"})
	out := &strings.Builder{}
	stderr := &strings.Builder{}
	ctx := &cli.Ctx{Stdout: out, Stderr: stderr, GlobalFlags: cli.GlobalFlags{Host: "git.example.com", Owner: "o", Repo: "r"}, Repo: repo, Cfg: &config.Config{Defaults: config.Defaults{Base: "main"}}}
	if err := (createBatchCmd{}).Run([]string{"*"}, ctx); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stderr.String(), "skipped: empty (no commit subject)\n") {
		t.Fatalf("stderr = %q, want skipped line", stderr.String())
	}
	var items []BatchReceiptItem
	if err := json.Unmarshal([]byte(out.String()), &items); err != nil {
		t.Fatalf("plan JSON: %v", err)
	}
	var got []string
	for _, it := range items {
		got = append(got, it.Branch)
	}
	if len(got) != 2 || got[0] != "good" || got[1] != "main" {
		t.Fatalf("items = %+v, want good then main (skipped dropped)", items)
	}
}

func TestBatchDryRunNeverPosts(t *testing.T) {
	repo := batchRepo(t, map[string]string{"feat-x": "x"})
	ts := batchServer(t)
	defer ts.Close()
	ctx := testCtx(ts)
	ctx.Repo = repo
	if err := (createBatchCmd{}).Run([]string{"feat-*"}, ctx); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(ctx.Stdout.(*bytes.Buffer).String(), `"branch": "feat-x"`) {
		t.Fatalf("stdout = %q", ctx.Stdout.(*bytes.Buffer).String())
	}
}

func TestBatchPlanJSONShape(t *testing.T) {
	repo := batchRepo(t, map[string]string{"feat-y": "y"})
	ts := batchServer(t)
	defer ts.Close()
	ctx := testCtx(ts)
	ctx.Repo = repo
	if err := (createBatchCmd{}).Run([]string{"feat-*"}, ctx); err != nil {
		t.Fatal(err)
	}
	var raw []map[string]any
	if err := json.Unmarshal(ctx.Stdout.(*bytes.Buffer).Bytes(), &raw); err != nil {
		t.Fatalf("plan JSON: %v", err)
	}
	if len(raw) != 1 {
		t.Fatalf("raw = %+v, want 1 item", raw)
	}
	for _, key := range []string{"branch", "title", "base"} {
		if _, ok := raw[0][key]; !ok {
			t.Errorf("missing key %q in %+v", key, raw[0])
		}
	}
	for _, key := range []string{"number", "url", "error"} {
		if _, ok := raw[0][key]; ok {
			t.Errorf("key %q should be omitted in %+v", key, raw[0])
		}
	}
}

func TestBatchRequiresPattern(t *testing.T) {
	err := (createBatchCmd{}).Run(nil, testCtx(httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))))
	cerr, ok := err.(*cli.Error)
	if !ok || cerr.Code != cli.ExitUsage {
		t.Fatalf("want ExitUsage, got %v", err)
	}
}

func TestBatchNoMatches(t *testing.T) {
	repo := batchRepo(t, map[string]string{"b1": "one"})
	err := (createBatchCmd{}).Run([]string{"no-such-*"}, &cli.Ctx{Stdout: &strings.Builder{}, Stderr: &strings.Builder{}, GlobalFlags: cli.GlobalFlags{Host: "git.example.com", Owner: "o", Repo: "r"}, Repo: repo, Cfg: &config.Config{Defaults: config.Defaults{Base: "main"}}})
	cerr, ok := err.(*cli.Error)
	if !ok || cerr.Code != cli.ExitUsage {
		t.Fatalf("want ExitUsage, got %v", err)
	}
}

func TestBatchRequiresRepo(t *testing.T) {
	err := (createBatchCmd{}).Run([]string{"x*"}, testCtx(httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))))
	cerr, ok := err.(*cli.Error)
	if !ok || cerr.Code != cli.ExitContext {
		t.Fatalf("want ExitContext, got %v", err)
	}
}
