package cmds

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"sync/atomic"
	"testing"

	"forge/internal/cli"
	"forge/internal/gitctx"
)

// syncServer serves one open-PR list response and counts the requests it
// received, so tests can pin the one-request rule.
func syncServer(t *testing.T, root, pullsPayload string, hits *int64) *cli.Ctx {
	t.Helper()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(hits, 1)
		if r.URL.Path == "/api/v1/repos/o/r/pulls" {
			if got := r.URL.Query().Get("state"); got != "open" {
				t.Errorf("state query = %q, want open", got)
			}
			fmt.Fprint(w, pullsPayload)
			return
		}
		t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		http.NotFound(w, r)
	}))
	t.Cleanup(ts.Close)
	return saveCtx(t, ts, root)
}

// syncCtx formats like the default TTY-less run and returns the stdout buffer.
func syncCtx(t *testing.T, ctx *cli.Ctx) *bytes.Buffer {
	t.Helper()
	ctx.Format = cli.FormatTable
	return ctx.Stdout.(*bytes.Buffer)
}

func branchOf(t *testing.T, root string) string {
	t.Helper()
	b := gitctx.CurrentBranch(root)
	if b == "" {
		t.Fatal("no current branch in test repo")
	}
	return b
}

func gitCmd(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
}

// detachHead checks out the current commit detached, so CurrentBranch is "".
func detachHead(t *testing.T, root string) {
	t.Helper()
	cmd := exec.Command("git", "checkout", "--detach", "HEAD")
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git checkout --detach: %v: %s", err, out)
	}
}

func TestSyncStatusInSync(t *testing.T) {
	checkoutRequireGit(t)
	root := t.TempDir()
	initCheckoutRepo(t, root, "")
	branch := branchOf(t, root)
	sha := headSHA(t, root)
	var hits int64
	payload := fmt.Sprintf(`[{"number":7,"html_url":"u/7","head":{"ref":%q,"sha":%q}}]`, branch, sha)
	ctx := syncServer(t, root, payload, &hits)
	out := syncCtx(t, ctx)
	if err := (prSyncStatusCmd{}).Run(nil, ctx); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out.String()) != "in-sync" {
		t.Errorf("stdout = %q, want in-sync", out.String())
	}
	if atomic.LoadInt64(&hits) != 1 {
		t.Errorf("API requests = %d, want 1", hits)
	}
}

func TestSyncStatusOutOfSyncExitsZero(t *testing.T) {
	checkoutRequireGit(t)
	root := t.TempDir()
	initCheckoutRepo(t, root, "")
	branch := branchOf(t, root)
	var hits int64
	payload := fmt.Sprintf(`[{"number":7,"head":{"ref":%q,"sha":"deadbeefdeadbeefdeadbeefdeadbeefdeadbeef"}}]`, branch)
	ctx := syncServer(t, root, payload, &hits)
	out := syncCtx(t, ctx)
	if err := (prSyncStatusCmd{}).Run(nil, ctx); err != nil {
		t.Fatalf("out-of-sync must not error: %v", err)
	}
	if strings.TrimSpace(out.String()) != "out-of-sync" {
		t.Errorf("stdout = %q, want out-of-sync", out.String())
	}
	if atomic.LoadInt64(&hits) != 1 {
		t.Errorf("API requests = %d, want 1", hits)
	}
}

func TestSyncStatusNoPR(t *testing.T) {
	checkoutRequireGit(t)
	root := t.TempDir()
	initCheckoutRepo(t, root, "")
	var hits int64
	ctx := syncServer(t, root, `[]`, &hits)
	out := syncCtx(t, ctx)
	if err := (prSyncStatusCmd{}).Run(nil, ctx); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out.String()) != "no-pr" {
		t.Errorf("stdout = %q, want no-pr", out.String())
	}
	if atomic.LoadInt64(&hits) != 1 {
		t.Errorf("API requests = %d, want 1", hits)
	}
}

func TestSyncStatusJSONReceipt(t *testing.T) {
	checkoutRequireGit(t)
	root := t.TempDir()
	initCheckoutRepo(t, root, "")
	branch := branchOf(t, root)
	sha := headSHA(t, root)
	var hits int64
	payload := fmt.Sprintf(`[{"number":9,"html_url":"u/9","head":{"ref":%q,"sha":%q}}]`, branch, sha)
	ctx := syncServer(t, root, payload, &hits)
	out := ctx.Stdout.(*bytes.Buffer)
	if err := (prSyncStatusCmd{}).Run(nil, ctx); err != nil {
		t.Fatal(err)
	}
	var r SyncStatusReceipt
	if err := json.Unmarshal(out.Bytes(), &r); err != nil {
		t.Fatalf("json: %v: %s", err, out.String())
	}
	if r.Status != "in-sync" || r.Branch != branch || r.PRNumber != 9 || r.HeadSHA != sha {
		t.Errorf("receipt = %+v", r)
	}
}

func TestSyncStatusExplicitBranch(t *testing.T) {
	checkoutRequireGit(t)
	root := t.TempDir()
	initCheckoutRepo(t, root, "")
	gitCmd(t, root, "branch", "topic")
	var hits int64
	// Server returns a PR for a different branch than the asked one; the
	// explicit arg must win, yielding no-pr.
	ctx := syncServer(t, root, `[{"number":3,"head":{"ref":"other","sha":"a"}}]`, &hits)
	out := syncCtx(t, ctx)
	if err := (prSyncStatusCmd{}).Run([]string{"topic"}, ctx); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out.String()) != "no-pr" {
		t.Errorf("stdout = %q, want no-pr", out.String())
	}
}

func TestSyncStatusUnknownBranchIsContextError(t *testing.T) {
	checkoutRequireGit(t)
	root := t.TempDir()
	initCheckoutRepo(t, root, "")
	var hits int64
	ctx := syncServer(t, root, `[]`, &hits)
	err := (prSyncStatusCmd{}).Run([]string{"ghost"}, ctx)
	if err == nil {
		t.Fatal("want error for unknown branch")
	}
	var cerr *cli.Error
	if !errors.As(err, &cerr) || cerr.Code != cli.ExitContext {
		t.Errorf("err = %v, want ExitContext", err)
	}
	if atomic.LoadInt64(&hits) != 0 {
		t.Errorf("API requests = %d, want 0 (fail before any request)", hits)
	}
}

func TestSyncStatusDetachedHeadHintsExplicitBranch(t *testing.T) {
	checkoutRequireGit(t)
	root := t.TempDir()
	initCheckoutRepo(t, root, "")
	detachHead(t, root)
	var hits int64
	ctx := syncServer(t, root, `[]`, &hits)
	err := (prSyncStatusCmd{}).Run(nil, ctx)
	if err == nil {
		t.Fatal("want error when HEAD is detached and no branch given")
	}
	var cerr *cli.Error
	if !errors.As(err, &cerr) || cerr.Code != cli.ExitContext {
		t.Errorf("err = %v, want ExitContext", err)
	}
}
