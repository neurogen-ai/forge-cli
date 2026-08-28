package cmds

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"forge/internal/cli"
)

// Both comment spellings share one handler, one endpoint, and one receipt
// shape because Forgejo backs PR comments with issue comments.
func TestCommentAddBothSpellings(t *testing.T) {
	cases := []struct {
		cmd  commentAddCmd
		path string
		n    string
	}{
		{commentAddCmd{kind: "pr"}, "/api/v1/repos/o/r/issues/9/comments", "9"},
		{commentAddCmd{kind: "issue"}, "/api/v1/repos/o/r/issues/4/comments", "4"},
	}
	for _, tc := range cases {
		t.Run(tc.cmd.Name(), func(t *testing.T) {
			var gotPath, gotBody string
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotPath = r.URL.Path
				raw, _ := io.ReadAll(r.Body)
				gotBody = string(raw)
				fmt.Fprint(w, `{"id":77,"html_url":"https://git.example.com/o/r/issues/4#issuecomment-77"}`)
			}))
			defer ts.Close()
			ctx := testCtx(ts)
			args := []string{tc.n, "--body", "looks good to me"}
			if err := tc.cmd.Run(args, ctx); err != nil {
				t.Fatal(err)
			}
			if gotPath != tc.path {
				t.Errorf("path = %q want %q", gotPath, tc.path)
			}
			if gotBody != `{"body":"looks good to me"}` {
				t.Errorf("body = %q", gotBody)
			}
			var rc CommentReceipt
			if err := json.Unmarshal([]byte(ctx.Stdout.(*bytes.Buffer).String()), &rc); err != nil {
				t.Fatalf("receipt: %v", err)
			}
			if rc.ID != 77 || rc.HTMLURL != "https://git.example.com/o/r/issues/4#issuecomment-77" {
				t.Errorf("receipt = %+v", rc)
			}
		})
	}
}

// Missing or empty --body is a usage error before any HTTP traffic.
func TestCommentAddRequiresBody(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{"no body flag", []string{"7"}},
		{"empty body", []string{"7", "--body", ""}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			hits := 0
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				hits++
				w.WriteHeader(500)
			}))
			defer ts.Close()
			ctx := testCtx(ts)
			err := (commentAddCmd{kind: "pr"}).Run(tc.args, ctx)
			cerr, ok := err.(*cli.Error)
			if !ok {
				t.Fatalf("err = %v, want *cli.Error", err)
			}
			if cerr.Code != cli.ExitUsage {
				t.Errorf("code = %d want %d", cerr.Code, cli.ExitUsage)
			}
			if hits != 0 {
				t.Errorf("validation sent %d requests, want 0", hits)
			}
		})
	}
}

// Server errors map through mapErr with the server message preserved.
func TestCommentAddServerError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(422)
		w.Write([]byte(`{"message":"issues are locked"}`))
	}))
	defer ts.Close()
	ctx := testCtx(ts)
	err := (commentAddCmd{kind: "issue"}).Run([]string{"3", "--body", "hi"}, ctx)
	cerr, ok := err.(*cli.Error)
	if !ok {
		t.Fatalf("err = %v, want *cli.Error", err)
	}
	if cerr.Code != cli.ExitRuntime {
		t.Errorf("code = %d want %d", cerr.Code, cli.ExitRuntime)
	}
	if !strings.Contains(cerr.Msg, "issues are locked") {
		t.Errorf("msg %q missing server message", cerr.Msg)
	}
}

// Receipts are JSON-only (D5): commentAddCmd declares no DefaultIsTable, so
// the central format parser rejects --table before the command runs.
func TestCommentAddTableRejectedCentrally(t *testing.T) {
	hits := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.WriteHeader(500)
	}))
	defer ts.Close()

	reg := cli.NewRegistry()
	reg.Register(PRCommands()...)
	reg.Register(IssueCommands()...)
	base := testCtx(ts)

	argv := []string{"--table", "pr", "comment", "add", "7", "--body", "x"}
	if code := cli.Run(argv, reg, base); code != cli.ExitUsage {
		t.Errorf("exit code = %d want %d", code, cli.ExitUsage)
	}
	if hits != 0 {
		t.Errorf("--table rejection sent %d requests, want 0", hits)
	}
}
