package cmds

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"forge/internal/api"
	"forge/internal/cli"
)

// testCtx builds a Ctx whose API client talks to ts, with owner/repo preset.
func testCtx(ts *httptest.Server) *cli.Ctx {
	return &cli.Ctx{
		Stdout: &bytes.Buffer{},
		Stderr: &bytes.Buffer{},
		GlobalFlags: cli.GlobalFlags{
			Host: "git.example.com", Owner: "o", Repo: "r",
		},
		API: api.NewClient(ts.URL, "tok", 0, nil),
	}
}

func TestPRGet(t *testing.T) {
	var gotPath string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		fmt.Fprint(w, `{"number":5,"title":"t","state":"open"}`)
	}))
	defer ts.Close()

	cmd := prGetCmd{}
	if err := cmd.Run([]string{"5"}, testCtx(ts)); err != nil {
		t.Fatal(err)
	}
	if gotPath != "/api/v1/repos/o/r/pulls/5" {
		t.Errorf("path = %q", gotPath)
	}
}

func TestPRGetRequiresNumber(t *testing.T) {
	err := (prGetCmd{}).Run(nil, testCtx(httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))))
	cerr, ok := err.(*cli.Error)
	if !ok || cerr.Code != cli.ExitUsage {
		t.Fatalf("want ExitUsage, got %v", err)
	}
}

func TestPRCreateRequiresTitle(t *testing.T) {
	t.Setenv("FORGE_HEAD", "")
	t.Setenv("FORGE_BASE", "")
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	defer ts.Close()
	err := (prCreateCmd{}).Run([]string{}, testCtx(ts))
	cerr, ok := err.(*cli.Error)
	if !ok || cerr.Code != cli.ExitUsage {
		t.Fatalf("want ExitUsage, got %v", err)
	}
}

func TestPRListPipedStillJSON(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `[{"number":1},{"number":2}]`)
	}))
	defer ts.Close()

	ctx := testCtx(ts)
	if err := (prListCmd{}).Run([]string{}, ctx); err != nil {
		t.Fatal(err)
	}
	var out []map[string]any
	if err := json.Unmarshal(ctx.Stdout.(*bytes.Buffer).Bytes(), &out); err != nil {
		t.Fatalf("stdout is not JSON array: %v\n%s", err, ctx.Stdout.(*bytes.Buffer).String())
	}
	if len(out) != 2 {
		t.Errorf("len = %d", len(out))
	}

	// Same server, forced table format: padded header row plus separator.
	ctx = testCtx(ts)
	ctx.Format = cli.FormatTable
	if err := (prListCmd{}).Run([]string{}, ctx); err != nil {
		t.Fatal(err)
	}
	got := ctx.Stdout.(*bytes.Buffer).String()
	if !strings.HasPrefix(got, "NUMBER") {
		t.Errorf("table output must start with NUMBER header, got %q", got)
	}
	lines := strings.Split(got, "\n")
	hasSep := false
	for _, l := range lines[1:] {
		if strings.HasPrefix(l, "---") {
			hasSep = true
			break
		}
	}
	if !hasSep {
		t.Errorf("table output must contain a dashed separator line, got %q", got)
	}
}

// ---- deprecation shim tests live in pr_conv_test.go ----

// prCreateTestServer serves the pulls endpoint with the given status/body and
// a configurable branch query result. branchStatus is the status returned for
// GET .../branches?branch=...; branchBody is written on non-200. It records
// whether any /branches request arrived.
func prCreateTestServer(t *testing.T, pullStatus int, pullBody string, branchStatus int, branchBody string) (*httptest.Server, *int) {
	branchHits := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/repos/o/r/pulls":
			w.WriteHeader(pullStatus)
			fmt.Fprint(w, pullBody)
		case "/api/v1/repos/o/r/branches":
			if r.URL.Query().Get("branch") == "" {
				t.Errorf("branch probe missing query parameter: %v", r.URL)
			}
			branchHits++
			w.WriteHeader(branchStatus)
			fmt.Fprint(w, branchBody)
		default:
			t.Errorf("unexpected path %q", r.URL.Path)
			w.WriteHeader(500)
		}
	}))
	return ts, &branchHits
}

func TestPRCreate404DisabledPulls(t *testing.T) {
	body := `{"message":"The target couldn't be found."}`
	ts, _ := prCreateTestServer(t, 404, body, 200, `{}`)
	defer ts.Close()

	err := (prCreateCmd{}).Run([]string{"--title", "t", "--head", "h", "--base", "main"}, testCtx(ts))
	cerr, ok := err.(*cli.Error)
	if !ok || cerr.Code != cli.ExitContext {
		t.Fatalf("want ExitContext cli.Error, got %v", err)
	}
	if !strings.Contains(cerr.Msg, "does not accept pull requests") {
		t.Errorf("Msg = %q", cerr.Msg)
	}
	if !strings.Contains(cerr.Hint, "The target couldn't be found.") {
		t.Errorf("Hint missing server message: %q", cerr.Hint)
	}
}

func TestPRCreate404BaseBranchMissing(t *testing.T) {
	body := `{"message":"The target couldn't be found."}`
	ts, _ := prCreateTestServer(t, 404, body, 404, body)
	defer ts.Close()

	err := (prCreateCmd{}).Run([]string{"--title", "t", "--head", "h", "--base", "main"}, testCtx(ts))
	cerr, ok := err.(*cli.Error)
	if !ok || cerr.Code != cli.ExitContext {
		t.Fatalf("want ExitContext cli.Error, got %v", err)
	}
	if !strings.Contains(cerr.Msg, `base branch "main" not found`) {
		t.Errorf("Msg = %q", cerr.Msg)
	}
	if !strings.Contains(cerr.Hint, "The target couldn't be found.") {
		t.Errorf("Hint missing server message: %q", cerr.Hint)
	}
}

func TestPRCreate404HeadBranchMissing(t *testing.T) {
	const notFound = `{"message":"The target couldn't be found."}`
	branchHits := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/repos/o/r/pulls":
			w.WriteHeader(404)
			fmt.Fprint(w, notFound)
		case "/api/v1/repos/o/r/branches":
			branchHits++
			if r.URL.Query().Get("branch") == "main" {
				fmt.Fprint(w, `{}`)
				return
			}
			w.WriteHeader(404)
			fmt.Fprint(w, notFound)
		default:
			t.Errorf("unexpected path %q", r.URL.Path)
			w.WriteHeader(500)
		}
	}))
	defer ts.Close()

	err := (prCreateCmd{}).Run([]string{"--title", "t", "--head", "nope", "--base", "main"}, testCtx(ts))
	cerr, ok := err.(*cli.Error)
	if !ok || cerr.Code != cli.ExitContext {
		t.Fatalf("want ExitContext cli.Error, got %v", err)
	}
	if !strings.Contains(cerr.Msg, "head branch") {
		t.Errorf("Msg = %q", cerr.Msg)
	}
}

func TestPRCreate500PassesThroughMapErr(t *testing.T) {
	ts, _ := prCreateTestServer(t, 500, `{"message":"boom"}`, 200, `{}`)
	defer ts.Close()

	err := (prCreateCmd{}).Run([]string{"--title", "t", "--head", "h", "--base", "main"}, testCtx(ts))
	cerr, ok := err.(*cli.Error)
	if !ok || cerr.Code != cli.ExitRuntime {
		t.Fatalf("want ExitRuntime cli.Error, got %v", err)
	}
	if cerr.Msg != "500: boom" {
		t.Errorf("Msg = %q, want %q", cerr.Msg, "500: boom")
	}
}

func TestPRCreateSuccessNoProbes(t *testing.T) {
	ts, hits := prCreateTestServer(t, 200, `{"number":1,"title":"t"}`, 200, `{}`)
	defer ts.Close()

	ctx := testCtx(ts)
	if err := (prCreateCmd{}).Run([]string{"--title", "t", "--head", "h", "--base", "main"}, ctx); err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err := json.Unmarshal(ctx.Stdout.(*bytes.Buffer).Bytes(), &out); err != nil {
		t.Fatalf("stdout is not JSON: %v", err)
	}
	if *hits != 0 {
		t.Errorf("branch probes on happy path = %d, want 0", *hits)
	}
}

func TestPRCommandsHaveHelpPages(t *testing.T) {
	for _, c := range PRCommands() {
		pageCmd, ok := c.(interface{ HelpPage() string })
		if !ok {
			t.Errorf("%s does not implement HelpPage", c.Name())
			continue
		}
		if got := pageCmd.HelpPage(); !strings.HasPrefix(got, "use: forge "+c.Name()) {
			t.Errorf("%s help page must start with its synopsis, got %q", c.Name(), got)
		}
	}
}

// v0.2.0 lazy-diagnosis pinning: a successful pr list makes exactly one API
// call, the pulls endpoint itself. No version/token/owner/repo preflight.
func TestPRListMakesNoPreflightCalls(t *testing.T) {
	var mu sync.Mutex
	paths := map[string]bool{}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		paths[r.URL.Path] = true
		mu.Unlock()
		if r.URL.Path != "/api/v1/repos/o/r/pulls" {
			t.Errorf("unexpected request %s %s: only the pulls list is allowed", r.Method, r.URL.Path)
			http.NotFound(w, r)
			return
		}
		fmt.Fprint(w, `[{"number":1}]`)
	}))
	defer ts.Close()

	if err := (prListCmd{}).Run([]string{}, testCtx(ts)); err != nil {
		t.Fatal(err)
	}
	if len(paths) != 1 || !paths["/api/v1/repos/o/r/pulls"] {
		t.Fatalf("distinct paths hit = %v; want exactly {/api/v1/repos/o/r/pulls}", paths)
	}
}
