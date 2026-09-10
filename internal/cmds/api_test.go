package cmds

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"forge/internal/cli"
)

func TestFlagValuesPreservesOrder(t *testing.T) {
	args := []string{"repos", "--q", "a=1", "--q", "b=2", "--q", "c=3"}
	got := flagValues(args, "--q")
	want := []string{"a=1", "b=2", "c=3"}
	if len(got) != len(want) {
		t.Fatalf("flagValues = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("flagValues = %v, want %v", got, want)
		}
	}
	if got := flagValues([]string{"--q"}, "--q"); len(got) != 0 {
		t.Fatalf("trailing flag without value should be skipped, got %v", got)
	}
}

func TestIsJSONMediaType(t *testing.T) {
	cases := map[string]bool{
		"application/json":                true,
		"application/json; charset=utf-8": true,
		"application/vnd.api+json":        true,
		"Application/JSON":                true,
		"text/plain":                      false,
		"text/html; charset=utf-8":        false,
		"":                                false,
		"application/octet-stream":        false,
		"jsonish":                         false,
	}
	for ct, want := range cases {
		if got := isJSONMediaType(ct); got != want {
			t.Errorf("isJSONMediaType(%q) = %v, want %v", ct, got, want)
		}
	}
}

func TestWriteAPIResponse(t *testing.T) {
	var w bytes.Buffer
	if err := writeAPIResponse(&w, "application/json; charset=utf-8", []byte(`{"b":1,"a":[1,2]}`)); err != nil {
		t.Fatal(err)
	}
	if got, want := w.String(), "{\n  \"a\": [\n    1,\n    2\n  ],\n  \"b\": 1\n}\n"; got != want {
		t.Fatalf("json pretty-print = %q, want %q", got, want)
	}

	w.Reset()
	raw := []byte("<html>hi</html>")
	if err := writeAPIResponse(&w, "text/html", raw); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(w.Bytes(), raw) {
		t.Fatalf("text passthrough = %q, want byte-exact %q", w.Bytes(), raw)
	}

	// JSON content type with an undecodable body passes through byte-exact.
	w.Reset()
	bad := []byte("{not json")
	if err := writeAPIResponse(&w, "application/json", bad); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(w.Bytes(), bad) {
		t.Fatalf("bad json passthrough = %q, want %q", w.Bytes(), bad)
	}

	w.Reset()
	if err := writeAPIResponse(&w, "application/json", nil); err != nil {
		t.Fatal(err)
	}
	if w.Len() != 0 {
		t.Fatalf("empty body wrote %q", w.String())
	}
}

func TestAPIPathRejectsAbsoluteURLs(t *testing.T) {
	if _, err := apiPath("https://evil.example.com/repos"); err == nil {
		t.Fatal("absolute URL must be rejected")
	}
	p, err := apiPath("repos/o/r")
	if err != nil || p != "/repos/o/r" {
		t.Fatalf("apiPath(repos/o/r) = %q, %v; want /repos/o/r", p, err)
	}
	if _, err := apiPath(""); err == nil {
		t.Fatal("empty path must be rejected")
	}
}

// newTestServer starts an httptest server whose every request goes through
// handler, mirroring the pr_test.go server style.
func newTestServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	ts := httptest.NewServer(handler)
	t.Cleanup(ts.Close)
	return ts
}

func TestAPIPassthroughCore(t *testing.T) {
	var gotMethod, gotPath, gotQuery string
	ts := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath, gotQuery = r.Method, r.URL.Path, r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ok":true}`))
	})
	ctx := testCtx(ts)

	if err := (apiCmd{}).Run([]string{"repos/o/r", "--q", "page=2"}, ctx); err != nil {
		t.Fatal(err)
	}
	if gotMethod != "GET" || gotPath != "/api/v1/repos/o/r" || gotQuery != "page=2" {
		t.Fatalf("request = %s %s?%s", gotMethod, gotPath, gotQuery)
	}
	if got := ctx.Stdout.(*bytes.Buffer).String(); got != "{\n  \"ok\": true\n}\n" {
		t.Fatalf("stdout = %q", got)
	}
}

func TestAPIPassthroughInputForms(t *testing.T) {
	var gotBody string
	ts := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.Write([]byte("done"))
	})

	// Inline JSON.
	ctx := testCtx(ts)
	if err := (apiCmd{}).Run([]string{"repos/o/r/issues", "--method", "post", "--input", `{"title":"t"}`}, ctx); err != nil {
		t.Fatal(err)
	}
	if gotBody != `{"title":"t"}` {
		t.Fatalf("inline body = %q", gotBody)
	}

	// @file.
	ctx = testCtx(ts)
	dir := t.TempDir()
	file := filepath.Join(dir, "body.json")
	os.WriteFile(file, []byte(`{"title":"f"}`), 0o644)
	if err := (apiCmd{}).Run([]string{"repos/o/r/issues", "--method", "POST", "--input", "@" + file}, ctx); err != nil {
		t.Fatal(err)
	}
	if gotBody != `{"title":"f"}` {
		t.Fatalf("@file body = %q", gotBody)
	}

	// - reads stdin.
	ctx = testCtx(ts)
	ctx.Stdin = strings.NewReader(`{"title":"s"}`)
	if err := (apiCmd{}).Run([]string{"repos/o/r/issues", "--method", "POST", "--input", "-"}, ctx); err != nil {
		t.Fatal(err)
	}
	if gotBody != `{"title":"s"}` {
		t.Fatalf("stdin body = %q", gotBody)
	}

	// - with nil stdin is a usage error before any request.
	ctx = testCtx(ts)
	err := apiCmd{}.Run([]string{"repos/o/r", "--input", "-"}, ctx)
	if e, ok := err.(*cli.Error); !ok || e.Code != cli.ExitUsage {
		t.Fatalf("nil stdin err = %v, want usage", err)
	}
}

func TestAPIPassthroughErrors(t *testing.T) {
	// Non-2xx maps to the standard runtime error with the server message.
	ts := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"message":"Not Found"}`))
	})
	ctx := testCtx(ts)
	err := apiCmd{}.Run([]string{"repos/o/r/nope"}, ctx)
	e, ok := err.(*cli.Error)
	if !ok || e.Code != cli.ExitRuntime || !strings.Contains(e.Msg, "Not Found") {
		t.Fatalf("404 err = %v, want runtime with server message", err)
	}

	// Repeated value flag is a usage error.
	ctx = testCtx(ts)
	if err := (apiCmd{}).Run([]string{"repos/o/r", "--method", "GET", "--method", "POST"}, ctx); err == nil {
		t.Fatal("repeated --method must fail")
	}

	// Malformed --q pair.
	ctx = testCtx(ts)
	if err := (apiCmd{}).Run([]string{"repos/o/r", "--q", "page"}, ctx); err == nil {
		t.Fatal("malformed --q must fail")
	}

	// Too many positional args.
	ctx = testCtx(ts)
	if err := (apiCmd{}).Run([]string{"repos/o/r", "extra"}, ctx); err == nil {
		t.Fatal("extra positional arg must fail")
	}
}
