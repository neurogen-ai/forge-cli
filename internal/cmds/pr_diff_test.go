package cmds

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"forge/internal/api"
	"forge/internal/cli"
	"forge/internal/config"
	"forge/internal/gitctx"
)

const diffBody = "diff --git a/x.go b/x.go\n--- a/x.go\n+++ b/x.go\n@@ -1 +1 @@\n-x\n+y\n"

// diffCtx builds a Ctx with a fake repo root and the pr-conversation
// savedir configured, matching the pull-test harness.
func diffCtx(t *testing.T, ts *httptest.Server, root string) *cli.Ctx {
	t.Helper()
	return &cli.Ctx{
		Stdout:      &bytes.Buffer{},
		Stderr:      &bytes.Buffer{},
		GlobalFlags: cli.GlobalFlags{Owner: "o", Repo: "r"},
		Cfg:         &config.Config{Savedirs: map[string]string{"pr-conversation": ".forge/prs"}},
		Repo:        &gitctx.Repo{Root: root},
		API:         api.NewClient(ts.URL, "tok", 0, nil),
	}
}

func TestPRDiffDefaultWritesBytesToStdout(t *testing.T) {
	var gotPath string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "text/plain")
		fmt.Fprint(w, diffBody)
	}))
	defer ts.Close()

	ctx := diffCtx(t, ts, t.TempDir())
	if err := (prDiffCmd{}).Run([]string{"5"}, ctx); err != nil {
		t.Fatal(err)
	}
	if gotPath != "/api/v1/repos/o/r/pulls/5.diff" {
		t.Errorf("path = %q", gotPath)
	}
	if got := ctx.Stdout.(*bytes.Buffer).String(); got != diffBody {
		t.Errorf("stdout = %q, want byte-for-byte diff bytes", got)
	}
}

func TestPRDiffPatchSuffix(t *testing.T) {
	var gotPath string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		fmt.Fprint(w, diffBody)
	}))
	defer ts.Close()

	ctx := diffCtx(t, ts, t.TempDir())
	if err := (prDiffCmd{}).Run([]string{"5", "--patch"}, ctx); err != nil {
		t.Fatal(err)
	}
	if gotPath != "/api/v1/repos/o/r/pulls/5.patch" {
		t.Errorf("path = %q", gotPath)
	}
	if got := ctx.Stdout.(*bytes.Buffer).String(); got != diffBody {
		t.Errorf("stdout = %q, want byte-for-byte patch bytes", got)
	}
}

func TestPRDiffOutWritesFileAndReceipt(t *testing.T) {
	root := t.TempDir()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, diffBody)
	}))
	defer ts.Close()

	ctx := diffCtx(t, ts, root)
	if err := (prDiffCmd{}).Run([]string{"5", "--out"}, ctx); err != nil {
		t.Fatal(err)
	}
	var receipt DiffReceipt
	if err := json.Unmarshal(ctx.Stdout.(*bytes.Buffer).Bytes(), &receipt); err != nil {
		t.Fatalf("stdout not a DiffReceipt: %v\n%s", err, ctx.Stdout.(*bytes.Buffer).String())
	}
	wantPath := filepath.Join(root, ".forge", "prs", "r-5.diff")
	if receipt.Path != wantPath {
		t.Errorf("receipt.Path = %q, want %q", receipt.Path, wantPath)
	}
	if receipt.Bytes != len(diffBody) {
		t.Errorf("receipt.Bytes = %d, want %d", receipt.Bytes, len(diffBody))
	}
	data, err := os.ReadFile(wantPath)
	if err != nil {
		t.Fatalf("diff file missing: %v", err)
	}
	if string(data) != diffBody {
		t.Errorf("file bytes = %q, want exact diff", data)
	}
}

func TestPRDiffOutReplacesPreviousFile(t *testing.T) {
	root := t.TempDir()
	body := "diff --git a/x.go b/x.go\n"
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, body)
	}))
	defer ts.Close()

	ctx := diffCtx(t, ts, root)
	for range 2 {
		out := &bytes.Buffer{}
		ctx.Stdout = out
		if err := (prDiffCmd{}).Run([]string{"5", "--out"}, ctx); err != nil {
			t.Fatal(err)
		}
	}
	dir := filepath.Join(root, ".forge", "prs")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("savedir holds %d files, want exactly one r-5.diff", len(entries))
	}
	data, err := os.ReadFile(filepath.Join(dir, "r-5.diff"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != body {
		t.Errorf("file bytes = %q, want the second write's bytes", data)
	}
}

func TestPRDiffOutMissingSavedirAndRepo(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, diffBody)
	}))
	defer ts.Close()

	// Missing savedir: repo root present but no [savedir] pr-conversation.
	root := t.TempDir()
	ctx := diffCtx(t, ts, root)
	ctx.Cfg = &config.Config{Savedirs: map[string]string{}}
	err := (prDiffCmd{}).Run([]string{"5", "--out"}, ctx)
	cerr, ok := err.(*cli.Error)
	if !ok || cerr.Code != cli.ExitUsage {
		t.Fatalf("want ExitUsage for missing savedir, got %v", err)
	}

	// Missing repository: savedir present but no repo context.
	ctx2 := diffCtx(t, ts, root)
	ctx2.Repo = nil
	err2 := (prDiffCmd{}).Run([]string{"5", "--out"}, ctx2)
	cerr2, ok2 := err2.(*cli.Error)
	if !ok2 || cerr2.Code != cli.ExitContext {
		t.Fatalf("want ExitContext for missing repo, got %v", err2)
	}
}

func TestPRDiffServerError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
		fmt.Fprint(w, `{"message":"pull request not found"}`)
	}))
	defer ts.Close()

	ctx := diffCtx(t, ts, t.TempDir())
	err := (prDiffCmd{}).Run([]string{"5"}, ctx)
	cerr, ok := err.(*cli.Error)
	if !ok || cerr.Code != cli.ExitRuntime {
		t.Fatalf("want ExitRuntime, got %v", err)
	}
	if !bytes.Contains([]byte(cerr.Msg), []byte("pull request not found")) {
		t.Errorf("msg %q missing server message", cerr.Msg)
	}
}
