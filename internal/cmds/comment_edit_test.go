package cmds

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"forge/internal/cli"
)

// All four spellings ride the same issue-comment endpoints; the id is the
// comment id, not the pr/issue number.
func TestCommentEditBothSpellings(t *testing.T) {
	cases := []struct {
		cmd  commentEditCmd
		path string
	}{
		{commentEditCmd{kind: "pr"}, "/api/v1/repos/o/r/issues/comments/77"},
		{commentEditCmd{kind: "issue"}, "/api/v1/repos/o/r/issues/comments/77"},
	}
	for _, tc := range cases {
		t.Run(tc.cmd.Name(), func(t *testing.T) {
			var gotPath, gotMethod, gotBody string
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotPath, gotMethod = r.URL.Path, r.Method
				raw := readBody(t, r)
				gotBody = string(raw)
				w.Write([]byte(`{"id":77,"body":"edited","html_url":"u"}`))
			}))
			defer ts.Close()
			ctx := testCtx(ts)
			if err := tc.cmd.Run([]string{"77", "--body", "edited"}, ctx); err != nil {
				t.Fatal(err)
			}
			if gotMethod != "PATCH" || gotPath != tc.path || gotBody != `{"body":"edited"}` {
				t.Errorf("request = %s %s body %s", gotMethod, gotPath, gotBody)
			}
			if out := ctx.Stdout.(*bytes.Buffer).String(); !strings.Contains(out, "edited") {
				t.Errorf("stdout = %q", out)
			}
		})
	}
}

func TestCommentDeleteBothSpellings(t *testing.T) {
	cases := []struct {
		cmd  commentDeleteCmd
		path string
	}{
		{commentDeleteCmd{kind: "pr"}, "/api/v1/repos/o/r/issues/comments/77"},
		{commentDeleteCmd{kind: "issue"}, "/api/v1/repos/o/r/issues/comments/77"},
	}
	for _, tc := range cases {
		t.Run(tc.cmd.Name(), func(t *testing.T) {
			var gotPath, gotMethod string
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotPath, gotMethod = r.URL.Path, r.Method
				w.WriteHeader(http.StatusNoContent)
			}))
			defer ts.Close()
			ctx := testCtx(ts)
			if err := tc.cmd.Run([]string{"77"}, ctx); err != nil {
				t.Fatal(err)
			}
			if gotMethod != "DELETE" || gotPath != tc.path {
				t.Errorf("request = %s %s", gotMethod, gotPath)
			}
			if out := ctx.Stdout.(*bytes.Buffer).String(); !strings.Contains(out, "delete") {
				t.Errorf("stdout = %q", out)
			}
		})
	}
}

// Missing --body is a usage error before any request.
func TestCommentEditRequiresBody(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{"no body flag", []string{"77"}},
		{"empty body", []string{"77", "--body", ""}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			hits := 0
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				hits++
				w.WriteHeader(500)
			}))
			defer ts.Close()
			err := (commentEditCmd{kind: "pr"}).Run(tc.args, testCtx(ts))
			cerr, ok := err.(*cli.Error)
			if !ok || cerr.Code != cli.ExitUsage {
				t.Fatalf("err = %v, want usage error", err)
			}
			if hits != 0 {
				t.Errorf("requests = %d, want 0", hits)
			}
		})
	}
}

// --body - reads stdin; an empty stream is a usage error before any request.
func TestCommentEditDashBody(t *testing.T) {
	hits := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.Write([]byte(`{"id":77}`))
	}))
	defer ts.Close()
	ctx := testCtxStdin(ts, strings.NewReader("from stdin\n"))
	if err := (commentEditCmd{kind: "issue"}).Run([]string{"77", "--body", "-"}, ctx); err != nil {
		t.Fatal(err)
	}
	if hits != 1 {
		t.Fatalf("requests = %d, want 1", hits)
	}

	hits = 0
	ctx2 := testCtxStdin(ts, strings.NewReader(""))
	err := (commentEditCmd{kind: "issue"}).Run([]string{"77", "--body", "-"}, ctx2)
	if hits != 0 {
		t.Fatalf("requests = %d, want 0", hits)
	}
	cerr, ok := err.(*cli.Error)
	if !ok || cerr.Code != cli.ExitUsage {
		t.Fatalf("err = %v, want usage error", err)
	}
}

// A 404 (e.g. the id belongs to the review-comment space) rides the standard
// error path; no special casing.
func TestCommentEditNotFoundIsRuntimeError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"message":"Not Found"}`))
	}))
	defer ts.Close()
	err := (commentDeleteCmd{kind: "pr"}).Run([]string{"999"}, testCtx(ts))
	cerr, ok := err.(*cli.Error)
	if !ok {
		t.Fatalf("err = %v, want *cli.Error", err)
	}
	if cerr.Code == cli.ExitUsage {
		t.Errorf("code = %d, want non-usage", cerr.Code)
	}
}

func readBody(t *testing.T, r *http.Request) []byte {
	t.Helper()
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
