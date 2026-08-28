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
	"time"

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
	// NoCommit creates the branch without a tip commit, so its tip
	// equals its branch point (usually main) and is contained in main.
	NoCommit bool
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
			if spec.NoCommit {
				delete(pending, branch)
				progress = true
				continue
			}
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
	// (now) init commit deterministically. Pattern "*" also matches main,
	// whose tip is the base itself: contained, hence the second note.
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
	if !strings.Contains(stderr.String(), "skipped: main (already in base)\n") {
		t.Fatalf("stderr = %q, want already-in-base line for main", stderr.String())
	}
	var items []BatchReceiptItem
	if err := json.Unmarshal([]byte(out.String()), &items); err != nil {
		t.Fatalf("plan JSON: %v", err)
	}
	if len(items) != 1 || items[0].Branch != "good" {
		t.Fatalf("items = %+v, want only good (skipped dropped)", items)
	}
}

func TestBatchAllSkippedExits2(t *testing.T) {
	// Every matching branch is either contained in the resolved base
	// (stale sits at main's tip, main is the base itself) or lacks a
	// commit subject (empty). The empty-plan error must name both skip
	// reasons and keep exit code 2.
	repo := batchRepoDated(t, map[string]batchBranch{
		"stale": {NoCommit: true},
		"empty": {Subject: "", Date: 1700000000},
	})
	out := &strings.Builder{}
	stderr := &strings.Builder{}
	ctx := &cli.Ctx{Stdout: out, Stderr: stderr, GlobalFlags: cli.GlobalFlags{Host: "git.example.com", Owner: "o", Repo: "r"}, Repo: repo, Cfg: &config.Config{Defaults: config.Defaults{Base: "main"}}}
	err := (createBatchCmd{}).Run([]string{"*"}, ctx)
	cerr, ok := err.(*cli.Error)
	if !ok || cerr.Code != cli.ExitUsage {
		t.Fatalf("want ExitUsage, got %v", err)
	}
	if cerr.Msg != "all matching branches already contained in base or lack commit subjects" {
		t.Fatalf("Msg = %q", cerr.Msg)
	}
	for _, want := range []string{
		"skipped: empty (no commit subject)\n",
		"skipped: main (already in base)\n",
		"skipped: stale (already in base)\n",
	} {
		if !strings.Contains(stderr.String(), want) {
			t.Fatalf("stderr = %q, want %q", stderr.String(), want)
		}
	}
	if out.String() != "" {
		t.Fatalf("stdout = %q, want empty plan", out.String())
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
	// html_url points at the test server so the page-availability probe hits
	// the handler's 200 fallback instead of the network. The prefix is filled
	// in after the server starts.
	var htmlURLPrefix string
	ts := batchPostServer(t, func(n int, w http.ResponseWriter, r *http.Request) {
		if n == 3 {
			w.WriteHeader(422)
			fmt.Fprint(w, `{"message":"validation failed"}`)
			return
		}
		fmt.Fprintf(w, `{"number":%d,"html_url":%q}`, 100+n, htmlURLPrefix+fmt.Sprintf("/o/r/pull/%d", 100+n))
	})
	htmlURLPrefix = ts.URL
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
	if items[0].Number != 101 || items[0].URL != ts.URL+"/o/r/pull/101" || items[0].Error != "" {
		t.Fatalf("items[0] = %+v", items[0])
	}
	if items[1].Number != 102 || items[1].URL != ts.URL+"/o/r/pull/102" || items[1].Error != "" {
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
		var htmlURLPrefix string
		ts := batchPostServer(t, func(n int, w http.ResponseWriter, r *http.Request) {
			fmt.Fprintf(w, `{"number":7,"html_url":%q}`, htmlURLPrefix+"/o/r/pull/7")
		})
		htmlURLPrefix = ts.URL
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
	var htmlURLPrefix string
	ts := batchPostServer(t, func(n int, w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"number":%d,"html_url":%q}`, n, htmlURLPrefix+fmt.Sprintf("/o/r/pull/%d", n))
	})
	htmlURLPrefix = ts.URL
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

// ---- pollPRPage ----

// countingPageServer returns a server counting GETs and serving the given
// status sequence (last value repeats).
func countingPageServer(t *testing.T, statuses ...int) (*httptest.Server, func() int) {
	t.Helper()
	n := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n++
		status := statuses[len(statuses)-1]
		if n <= len(statuses) {
			status = statuses[n-1]
		}
		w.WriteHeader(status)
	}))
	return ts, func() int { return n }
}

func TestPollPRPageReadyAfterNAttempts(t *testing.T) {
	ts, count := countingPageServer(t, 500, 500, 200)
	defer ts.Close()
	if !pollPRPage(ts.Client(), ts.URL, 5, time.Millisecond) {
		t.Fatal("want ready=true once a status < 400 arrives")
	}
	if got := count(); got != 3 {
		t.Fatalf("GETs = %d, want 3 (stops as soon as ready)", got)
	}
}

func TestPollPRPageNeverReady(t *testing.T) {
	ts, count := countingPageServer(t, 404, 404, 404)
	defer ts.Close()
	if pollPRPage(ts.Client(), ts.URL, 3, time.Millisecond) {
		t.Fatal("want ready=false when every status is >= 400")
	}
	if got := count(); got != 3 {
		t.Fatalf("GETs = %d, want 3 (bounded by attempts)", got)
	}
}

func TestPollPRPageTransportErrorIsNotReady(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	ts.Close() // shut down immediately: every GET is a transport error
	if pollPRPage(ts.Client(), ts.URL, 2, time.Millisecond) {
		t.Fatal("want ready=false on transport errors")
	}
}

func TestPollPRPageRespectsAttemptsAndDelay(t *testing.T) {
	ts, count := countingPageServer(t, 404, 404, 200)
	defer ts.Close()
	start := time.Now()
	ok := pollPRPage(ts.Client(), ts.URL, 3, 25*time.Millisecond)
	elapsed := time.Since(start)
	if !ok {
		t.Fatal("want ready=true once a status < 400 arrives")
	}
	if got := count(); got != 3 {
		t.Fatalf("GETs = %d, want 3", got)
	}
	// attempts-1 = 2 sleeps of 25ms between attempts; the first GET is
	// immediate. Generous upper bound to absorb scheduler noise.
	if elapsed < 40*time.Millisecond || elapsed > 2*time.Second {
		t.Fatalf("elapsed = %s, want >= ~50ms of delay and well under the 500ms default", elapsed)
	}
}

// TestBatchYesNotesWhenPageNeverAvailable uses a POST handler whose
// html_url resolves to a permanently 503 path, so the probe exhausts its
// attempts and the loop must print the note and keep going (release doc
// §2: timeout is not an error).
func TestBatchYesNotesWhenPageNeverAvailable(t *testing.T) {
	repo := batchRepo(t, map[string]string{"b1": "one"})
	var htmlURL string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/pulls") {
			fmt.Fprintf(w, `{"number":1,"html_url":%q}`, htmlURL)
			return
		}
		if r.URL.Path == "/unavailable" {
			w.WriteHeader(503)
			return
		}
		fmt.Fprint(w, `{"default_branch":"main"}`)
	}))
	htmlURL = ts.URL + "/unavailable"
	defer ts.Close()
	origAttempts, origDelay := pagePollAttempts, pagePollDelay
	pagePollAttempts, pagePollDelay = 2, time.Millisecond
	defer func() { pagePollAttempts, pagePollDelay = origAttempts, origDelay }()
	ctx := testCtx(ts)
	ctx.Repo = repo
	if err := (createBatchCmd{}).Run([]string{"--yes", "--base", "main", "b*"}, ctx); err != nil {
		t.Fatal(err)
	}
	if want := "note: b1 page not confirmed available before continuing\n"; !strings.Contains(ctx.Stderr.(*bytes.Buffer).String(), want) {
		t.Fatalf("stderr = %q, want %q", ctx.Stderr.(*bytes.Buffer).String(), want)
	}
	var items []BatchReceiptItem
	if err := json.Unmarshal(ctx.Stdout.(*bytes.Buffer).Bytes(), &items); err != nil {
		t.Fatalf("receipt JSON: %v", err)
	}
	if len(items) != 1 || items[0].Number != 1 || items[0].Error != "" {
		t.Fatalf("items = %+v, want one successful item (timeout is not a failure)", items)
	}
}
