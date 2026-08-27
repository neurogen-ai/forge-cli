package cmds

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"forge/internal/api"
	"forge/internal/cli"
)

// resolveTestCtx builds an API-only Ctx (no repo/config wiring needed).
func resolveTestCtx(t *testing.T, ts *httptest.Server) *cli.Ctx {
	t.Helper()
	return &cli.Ctx{
		Stdout:      &bytes.Buffer{},
		Stderr:      &bytes.Buffer{},
		GlobalFlags: cli.GlobalFlags{Owner: "o", Repo: "r"},
		API:         api.NewClient(ts.URL, "tok", 0, nil),
	}
}

func TestResolveReceiptHitsExactURLAndBody(t *testing.T) {
	for _, tc := range []struct {
		unresolve bool
		method    string
		body      string
		action    string
	}{
		{false, "PATCH", `{"resolved":true}`, "resolve"},
		{true, "PATCH", `{"resolved":false}`, "unresolve"},
	} {
		var gotPath, gotMethod, gotBody string
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotPath, gotMethod = r.URL.Path, r.Method
			b := make([]byte, r.ContentLength)
			r.Body.Read(b)
			gotBody = string(b)
			w.WriteHeader(200)
		}))
		ctx := resolveTestCtx(t, ts)
		err := (resolveCmd{unresolve: tc.unresolve}).Run([]string{"88"}, ctx)
		if err != nil {
			t.Fatalf("unresolve=%v: %v", tc.unresolve, err)
		}
		if gotMethod != tc.method || gotPath != "/api/v1/repos/o/r/pulls/comments/88/resolve" {
			t.Errorf("unresolve=%v: %s %s, want %s /api/v1/repos/o/r/pulls/comments/88/resolve", tc.unresolve, gotMethod, gotPath, tc.method)
		}
		if gotBody != tc.body {
			t.Errorf("unresolve=%v: body = %s, want %s", tc.unresolve, gotBody, tc.body)
		}
		var receipt ResolutionReceipt
		if err := json.Unmarshal(ctx.Stdout.(*bytes.Buffer).Bytes(), &receipt); err != nil {
			t.Fatalf("unresolve=%v: receipt not JSON: %v", tc.unresolve, err)
		}
		if receipt.ID != 88 || receipt.Action != tc.action {
			t.Errorf("unresolve=%v: receipt = %+v, want id 88 action %q", tc.unresolve, receipt, tc.action)
		}
		ts.Close()
	}
}

func TestResolve404MapsToVersionHint(t *testing.T) {
	for _, unresolve := range []bool{false, true} {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(404)
			w.Write([]byte(`{"message":"Not Found"}`))
		}))
		ctx := resolveTestCtx(t, ts)
		err := (resolveCmd{unresolve: unresolve}).Run([]string{"7"}, ctx)
		ts.Close()
		cerr, ok := err.(*cli.Error)
		if !ok || cerr.Code != cli.ExitRuntime {
			t.Fatalf("unresolve=%v: want ExitRuntime cli.Error, got %v", unresolve, err)
		}
		if !strings.Contains(cerr.Msg, "does not expose the comment-resolution endpoint") {
			t.Errorf("unresolve=%v: msg %q missing version-hint text", unresolve, cerr.Msg)
		}
		if !strings.Contains(cerr.Hint, "conversation resolution") {
			t.Errorf("unresolve=%v: hint %q missing resolution guidance", unresolve, cerr.Hint)
		}
	}
}

func TestResolve422PassesServerMessageVerbatim(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(422)
		w.Write([]byte(`{"message":"already resolved"}`))
	}))
	defer ts.Close()
	ctx := resolveTestCtx(t, ts)
	err := (resolveCmd{}).Run([]string{"7"}, ctx)
	cerr, ok := err.(*cli.Error)
	if !ok || cerr.Code != cli.ExitRuntime {
		t.Fatalf("want ExitRuntime cli.Error, got %v", err)
	}
	if !strings.Contains(cerr.Msg, "already resolved") {
		t.Errorf("server message not verbatim: %q", cerr.Msg)
	}
	if strings.Contains(cerr.Msg, "does not expose") {
		t.Errorf("422 must not be rewritten as a 404-style endpoint error: %q", cerr.Msg)
	}
}

func TestResolveNetworkFailureMapsThroughMapErr(t *testing.T) {
	ctx := resolveTestCtx(t, httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})))
	ctx.API = api.NewClient("http://127.0.0.1:1", "tok", 0, nil) // nothing listens
	err := (resolveCmd{}).Run([]string{"7"}, ctx)
	cerr, ok := err.(*cli.Error)
	if !ok || cerr.Code != cli.ExitNetwork {
		t.Fatalf("want ExitNetwork cli.Error, got %v", err)
	}
	if cerr.Msg != "network failure" {
		t.Errorf("msg = %q, want mapErr's network wording", cerr.Msg)
	}
}

func TestResolveParseInt64ArgUsage(t *testing.T) {
	for _, arg := range [][]string{nil, {"0"}, {"-3"}, {"abc"}} {
		_, err := parseInt64Arg(arg, "pr comment resolve")
		cerr, ok := err.(*cli.Error)
		if !ok || cerr.Code != cli.ExitUsage {
			t.Fatalf("%v: want ExitUsage, got %v", arg, err)
		}
	}
	id, err := parseInt64Arg([]string{"42"}, "pr comment resolve")
	if err != nil || id != 42 {
		t.Fatalf("valid id parse failed: %d, %v", id, err)
	}
}
