package cmds

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"forge/internal/cli"
)

// stubObjectServer serves one object payload per kind and counts requests.
func stubObjectServer(t *testing.T, payload string) (*httptest.Server, *int32) {
	t.Helper()
	var hits int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		fmt.Fprint(w, payload)
	}))
	t.Cleanup(ts.Close)
	return ts, &hits
}

// writeBrowserScript builds an executable $BROWSER that records its arguments
// in order (one per line) into outfile, prints child-err on stderr and
// child-out on stdout, and exits 0.
func writeBrowserScript(t *testing.T, outfile string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "fakebrowser")
	script := fmt.Sprintf("#!/bin/sh\nprintf '%%s\\n' \"$@\" >> %q\necho child-err >&2\necho child-out\nexit 0\n", outfile)
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestBrowsePrintsURLForAllKinds(t *testing.T) {
	cases := []struct {
		kind    string
		args    []string
		payload string
		want    string
	}{
		{"pr", []string{"5"}, `{"number":5,"title":"t","html_url":"https://git.example.com/o/r/pulls/5"}`, "https://git.example.com/o/r/pulls/5"},
		{"issue", []string{"9"}, `{"number":9,"title":"t","html_url":"https://git.example.com/o/r/issues/9"}`, "https://git.example.com/o/r/issues/9"},
		{"repo", nil, `{"id":7,"name":"r","html_url":"https://git.example.com/o/r"}`, "https://git.example.com/o/r"},
	}
	for _, tc := range cases {
		t.Run(tc.kind, func(t *testing.T) {
			ts, hits := stubObjectServer(t, tc.payload)
			ctx := testCtx(ts)
			ctx.Stdout = &bytes.Buffer{}
			if err := (browseCmd{kind: tc.kind}).Run(tc.args, ctx); err != nil {
				t.Fatal(err)
			}
			if got := ctx.Stdout.(*bytes.Buffer).String(); got != tc.want+"\n" {
				t.Fatalf("stdout = %q, want %q", got, tc.want+"\n")
			}
			if atomic.LoadInt32(hits) != 1 {
				t.Fatalf("server saw %d requests, want exactly 1", hits)
			}
		})
	}
}

func TestBrowseRepoHitsRepoEndpoint(t *testing.T) {
	var gotPath string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		fmt.Fprint(w, `{"id":7,"name":"r","html_url":"https://git.example.com/o/r"}`)
	}))
	defer ts.Close()
	if err := (browseCmd{kind: "repo"}).Run(nil, testCtx(ts)); err != nil {
		t.Fatal(err)
	}
	if want := "/api/v1/repos/o/r"; gotPath != want {
		t.Fatalf("path = %q, want %q", gotPath, want)
	}
}

// Without --open the browser must never launch, even when BROWSER points at
// something that fails loudly.
func TestBrowseNoLaunchWithoutFlag(t *testing.T) {
	ts, _ := stubObjectServer(t, `{"number":5,"html_url":"https://git.example.com/o/r/pulls/5"}`)
	t.Setenv("BROWSER", "/bin/false")
	ctx := testCtx(ts)
	ctx.Stdout = &bytes.Buffer{}
	if err := (browseCmd{kind: "pr"}).Run([]string{"5"}, ctx); err != nil {
		t.Fatal(err)
	}
	if got := ctx.Stdout.(*bytes.Buffer).String(); !strings.Contains(got, "https://git.example.com/o/r/pulls/5") {
		t.Fatalf("URL not printed: %q", got)
	}
}

func TestBrowseOpenLaunchesBrowserWithURLAsFinalArg(t *testing.T) {
	ts, _ := stubObjectServer(t, `{"number":5,"html_url":"https://git.example.com/o/r/pulls/5"}`)
	out := filepath.Join(t.TempDir(), "args")
	browser := writeBrowserScript(t, out)
	t.Setenv("BROWSER", browser+" --new-window")

	ctx := testCtx(ts)
	ctx.Stdout = &bytes.Buffer{}
	ctx.Stderr = &bytes.Buffer{}
	if err := (browseCmd{kind: "pr"}).Run([]string{"5", "--open"}, ctx); err != nil {
		t.Fatal(err)
	}

	raw, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("browser never ran: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	if len(lines) != 2 || lines[0] != "--new-window" || lines[1] != "https://git.example.com/o/r/pulls/5" {
		t.Fatalf("browser args = %q, want [--new-window URL-as-final]", lines)
	}
	// URL still prints, and child output routes to stderr, never stdout.
	if got := ctx.Stdout.(*bytes.Buffer).String(); got != "https://git.example.com/o/r/pulls/5\n" {
		t.Fatalf("stdout = %q, want just the URL", got)
	}
	if errOut := ctx.Stderr.(*bytes.Buffer).String(); !strings.Contains(errOut, "child-err") || !strings.Contains(errOut, "child-out") {
		t.Fatalf("stderr = %q, want child output routed there", errOut)
	}
}

func TestBrowseOpenLaunchFailureIsRuntimeError(t *testing.T) {
	ts, _ := stubObjectServer(t, `{"number":5,"html_url":"https://git.example.com/o/r/pulls/5"}`)
	t.Setenv("BROWSER", "/bin/false")
	ctx := testCtx(ts)
	ctx.Stdout = &bytes.Buffer{}
	err := (browseCmd{kind: "pr"}).Run([]string{"5", "--open"}, ctx)
	cerr, ok := err.(*cli.Error)
	if !ok || cerr.Code != cli.ExitRuntime {
		t.Fatalf("want ExitRuntime, got %v", err)
	}
	// The URL still printed before the failed launch.
	if got := ctx.Stdout.(*bytes.Buffer).String(); got == "" {
		t.Fatal("URL must still print when opening")
	}
}

func TestBrowseOpenMissingBrowserIsRuntimeError(t *testing.T) {
	ts, _ := stubObjectServer(t, `{"number":5,"html_url":"https://git.example.com/o/r/pulls/5"}`)
	t.Setenv("BROWSER", "")
	err := (browseCmd{kind: "pr"}).Run([]string{"5", "--open"}, testCtx(ts))
	cerr, ok := err.(*cli.Error)
	if !ok || cerr.Code != cli.ExitRuntime {
		t.Fatalf("want ExitRuntime, got %v", err)
	}
}

func TestBrowseOpenWithEmptyURLIsRuntimeError(t *testing.T) {
	ts, _ := stubObjectServer(t, `{"number":5,"html_url":""}`)
	t.Setenv("BROWSER", "/bin/echo")
	err := (browseCmd{kind: "pr"}).Run([]string{"5", "--open"}, testCtx(ts))
	if cerr, ok := err.(*cli.Error); !ok || cerr.Code != cli.ExitRuntime {
		t.Fatalf("want ExitRuntime, got %v", err)
	}
}

// Empty html_url without --open prints nothing and succeeds.
func TestBrowseEmptyURLWithoutOpenPrintsNothing(t *testing.T) {
	ts, _ := stubObjectServer(t, `{"number":5,"html_url":""}`)
	t.Setenv("BROWSER", "/bin/false")
	ctx := testCtx(ts)
	ctx.Stdout = &bytes.Buffer{}
	if err := (browseCmd{kind: "issue"}).Run([]string{"5"}, ctx); err != nil {
		t.Fatal(err)
	}
	if got := ctx.Stdout.(*bytes.Buffer).String(); got != "" {
		t.Fatalf("stdout = %q, want empty", got)
	}
}

func TestBrowseRequiresNumber(t *testing.T) {
	ts, _ := stubObjectServer(t, `{}`)
	err := (browseCmd{kind: "pr"}).Run([]string{"--open"}, testCtx(ts))
	cerr, ok := err.(*cli.Error)
	if !ok || cerr.Code != cli.ExitUsage {
		t.Fatalf("want ExitUsage, got %v", err)
	}
}

func TestBrowseCommandsRegistered(t *testing.T) {
	reg := cli.NewRegistry()
	reg.Register(PRCommands()...)
	reg.Register(IssueCommands()...)
	reg.Register(RepoCommands()...)
	for _, name := range []string{"pr browse", "issue browse", "repo browse"} {
		if reg.Lookup(name) == nil {
			t.Errorf("command %q not registered", name)
		}
	}
}
