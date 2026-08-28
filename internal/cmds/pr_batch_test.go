package cmds

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"

	"forge/internal/cli"
	"forge/internal/config"
	"forge/internal/gitctx"
)

// batchBranch is one branch spec for batchRepoDated.
type batchBranch struct {
	Subject string
	// Date pins the tip commit's author and committer dates as unix
	// seconds ("@<unix>"), so ordering assertions are deterministic
	// across second boundaries. 0 leaves the commit's natural date.
	Date int64
	// From is the branch point (existing branch or ref) the new branch
	// starts from; empty means main. Stacked branches set From to their
	// parent so ancestry tie-breaks are testable.
	From string
}

// batchRepoDated creates a temp git repo with the named branches, each
// carrying a commit with the spec's subject on top of an initial commit on
// main. The Date field pins the tip commit's dates via GIT_AUTHOR_DATE and
// GIT_COMMITTER_DATE.
func batchRepoDated(t *testing.T, branches map[string]batchBranch) *gitctx.Repo {
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
	// Process specs after their From branch exists; map iteration order is
	// random, so stacked branches need dependency-aware ordering.
	pending := make(map[string]batchBranch, len(branches))
	for branch, spec := range branches {
		pending[branch] = spec
	}
	for len(pending) > 0 {
		progress := false
		for branch, spec := range pending {
			start := spec.From
			if start == "" {
				start = "main"
			}
			if start != "main" {
				if _, ok := branches[start]; !ok && start != "HEAD" {
					continue
				}
				if _, ok := pending[start]; ok {
					continue
				}
			}
			run("checkout", "-q", "-b", branch, start)
			cmd := exec.Command("git", "commit", "--allow-empty", "--allow-empty-message", "-m", spec.Subject)
			if spec.Date != 0 {
				d := fmt.Sprintf("@%d +0000", spec.Date)
				cmd.Env = append(os.Environ(), "GIT_AUTHOR_DATE="+d, "GIT_COMMITTER_DATE="+d)
			}
			cmd.Dir = dir
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("git commit on %s: %v: %s", branch, err, out)
			}
			delete(pending, branch)
			progress = true
		}
		if !progress {
			t.Fatalf("unresolvable From chain in branches %v", pending)
		}
	}
	cmd := exec.Command("git", "checkout", "-q", "main")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git checkout main: %v: %s", err, out)
	}
	return &gitctx.Repo{Root: dir}
}

// batchRepo creates a temp git repo with the named branches, each carrying a
// commit with the given subject on top of an initial commit on main. Tip
// dates are natural (now); use batchRepoDated to pin them.
func batchRepo(t *testing.T, branches map[string]string) *gitctx.Repo {
	specs := make(map[string]batchBranch, len(branches))
	for branch, subject := range branches {
		specs[branch] = batchBranch{Subject: subject}
	}
	return batchRepoDated(t, specs)
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
	// Equal pinned dates force the lexical tie-break; without pinning the
	// three commits could land in different seconds.
	repo := batchRepoDated(t, map[string]batchBranch{
		"b1": {Subject: "one", Date: 1700000000},
		"b2": {Subject: "two", Date: 1700000000},
		"b3": {Subject: "three", Date: 1700000000},
	})
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
	// Pinned in the past so both branch tips sort before main's natural
	// (now) init commit deterministically.
	repo := batchRepoDated(t, map[string]batchBranch{
		"empty": {Subject: "", Date: 1700000000},
		"good":  {Subject: "real title", Date: 1700000000},
	})
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

// batchPostServer serves the repository default branch and routes POST
// .../pulls through the caller-supplied per-request handler (n is the
// 1-based POST count).
func batchPostServer(t *testing.T, post func(n int, w http.ResponseWriter, r *http.Request)) *httptest.Server {
	t.Helper()
	n := 0
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/pulls") {
			n++
			post(n, w, r)
			return
		}
		fmt.Fprint(w, `{"default_branch":"main"}`)
	}))
}

func TestBatchYesStopsOnFirstFailure(t *testing.T) {
	repo := batchRepoDated(t, map[string]batchBranch{
		"b1": {Subject: "one", Date: 1700000000},
		"b2": {Subject: "two", Date: 1700000000},
		"b3": {Subject: "three", Date: 1700000000},
	})
	ts := batchPostServer(t, func(n int, w http.ResponseWriter, r *http.Request) {
		if n == 3 {
			w.WriteHeader(422)
			fmt.Fprint(w, `{"message":"validation failed"}`)
			return
		}
		fmt.Fprintf(w, `{"number":%d,"html_url":"https://git.example.com/o/r/pull/%d"}`, 100+n, 100+n)
	})
	defer ts.Close()
	ctx := testCtx(ts)
	ctx.Repo = repo
	err := (createBatchCmd{}).Run([]string{"--yes", "--base", "main", "b*"}, ctx)
	cerr, ok := err.(*cli.Error)
	if !ok || cerr.Code != cli.ExitRuntime {
		t.Fatalf("want ExitRuntime, got %v", err)
	}
	if cerr.Msg != "batch stopped: b3 failed" {
		t.Fatalf("Msg = %q", cerr.Msg)
	}
	var items []BatchReceiptItem
	if err := json.Unmarshal(ctx.Stdout.(*bytes.Buffer).Bytes(), &items); err != nil {
		t.Fatalf("receipt JSON: %v", err)
	}
	if len(items) != 3 {
		t.Fatalf("items = %+v, want 3 (partial receipt includes the failure)", items)
	}
	if items[0].Number != 101 || items[0].URL != "https://git.example.com/o/r/pull/101" || items[0].Error != "" {
		t.Fatalf("items[0] = %+v", items[0])
	}
	if items[1].Number != 102 || items[1].URL != "https://git.example.com/o/r/pull/102" || items[1].Error != "" {
		t.Fatalf("items[1] = %+v", items[1])
	}
	if items[2].Error != "validation failed" {
		t.Fatalf("items[2].Error = %q, want server message verbatim", items[2].Error)
	}
	if items[2].Number != 0 || items[2].URL != "" {
		t.Fatalf("items[2] = %+v, want Number/URL unset", items[2])
	}
}

func TestBatchYesFailsOnFirstPost(t *testing.T) {
	repo := batchRepo(t, map[string]string{"b1": "one"})
	ts := batchPostServer(t, func(n int, w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(422)
		fmt.Fprint(w, `{"message":"boom"}`)
	})
	defer ts.Close()
	ctx := testCtx(ts)
	ctx.Repo = repo
	err := (createBatchCmd{}).Run([]string{"--yes", "--base", "main", "b*"}, ctx)
	cerr, ok := err.(*cli.Error)
	if !ok || cerr.Code != cli.ExitRuntime {
		t.Fatalf("want ExitRuntime, got %v", err)
	}
	var items []BatchReceiptItem
	if err := json.Unmarshal(ctx.Stdout.(*bytes.Buffer).Bytes(), &items); err != nil {
		t.Fatalf("receipt JSON: %v", err)
	}
	if len(items) != 1 || items[0].Error != "boom" {
		t.Fatalf("items = %+v, want exactly one item with Error \"boom\"", items)
	}
}

func TestBatchYesStatelessSecondRun(t *testing.T) {
	repo := batchRepo(t, map[string]string{"b1": "one"})
	run := func() {
		ts := batchPostServer(t, func(n int, w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, `{"number":7,"html_url":"https://git.example.com/o/r/pull/7"}`)
		})
		defer ts.Close()
		ctx := testCtx(ts)
		ctx.Repo = repo
		if err := (createBatchCmd{}).Run([]string{"--yes", "--base", "main", "b*"}, ctx); err != nil {
			t.Fatal(err)
		}
		var items []BatchReceiptItem
		if err := json.Unmarshal(ctx.Stdout.(*bytes.Buffer).Bytes(), &items); err != nil {
			t.Fatalf("receipt JSON: %v", err)
		}
		if len(items) != 1 || items[0].Number != 7 || items[0].Error != "" {
			t.Fatalf("items = %+v, want one successful item", items)
		}
	}
	run()
	run() // fresh testCtx + fresh fixture server: no state carried between runs
}

func TestBatchYesAllSucceed(t *testing.T) {
	repo := batchRepoDated(t, map[string]batchBranch{
		"b1": {Subject: "one", Date: 1700000000},
		"b2": {Subject: "two", Date: 1700000000},
	})
	ts := batchPostServer(t, func(n int, w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"number":%d,"html_url":"https://git.example.com/o/r/pull/%d"}`, n, n)
	})
	defer ts.Close()
	ctx := testCtx(ts)
	ctx.Repo = repo
	if err := (createBatchCmd{}).Run([]string{"--yes", "--base", "main", "b*"}, ctx); err != nil {
		t.Fatal(err)
	}
	var raw []map[string]any
	if err := json.Unmarshal(ctx.Stdout.(*bytes.Buffer).Bytes(), &raw); err != nil {
		t.Fatalf("receipt JSON: %v", err)
	}
	if len(raw) != 2 {
		t.Fatalf("raw = %+v, want 2 items", raw)
	}
	for i, want := range []string{"b1", "b2"} {
		if raw[i]["branch"] != want {
			t.Fatalf("raw[%d] = %+v, want branch %q", i, raw[i], want)
		}
		for _, key := range []string{"number", "url"} {
			if _, ok := raw[i][key]; !ok {
				t.Errorf("raw[%d] missing %q", i, key)
			}
		}
		if _, ok := raw[i]["error"]; ok {
			t.Errorf("raw[%d] has unexpected \"error\" key", i)
		}
	}
}
