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

	"forge/internal/api"
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

// Bare "pr comment" resolves to the gh-spelled verb and exits 2 on a missing
// body instead of falling through to the pr family page; "pr comment add"
// still matches its own command through longest-match dispatch.
func TestCommentVerbDispatch(t *testing.T) {
	hits := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		fmt.Fprint(w, `{"id":77,"html_url":"https://git.example.com/o/r/issues/7#issuecomment-77"}`)
	}))
	defer ts.Close()

	reg := cli.NewRegistry()
	reg.Register(PRCommands()...)
	reg.Register(IssueCommands()...)
	base := testCtx(ts)
	base.Prepare = func(c *cli.Ctx, _ cli.Command) error { // main's wire sets the API client
		c.API = api.NewClient(ts.URL, "tok", 0, nil)
		return nil
	}

	if code := cli.Run([]string{"pr", "comment"}, reg, base); code != cli.ExitUsage {
		t.Errorf("bare verb exit = %d want %d", code, cli.ExitUsage)
	}
	if hits != 0 {
		t.Errorf("missing body sent %d requests, want 0", hits)
	}

	if code := cli.Run([]string{"pr", "comment", "7", "--body", "hi"}, reg, base); code != cli.ExitOK {
		t.Errorf("verb exit = %d want %d", code, cli.ExitOK)
	}
	if hits != 1 {
		t.Errorf("hits = %d want 1", hits)
	}

	base.Stdout.(*bytes.Buffer).Reset()
	if code := cli.Run([]string{"pr", "comment", "add", "7", "--body", "hi"}, reg, base); code != cli.ExitOK {
		t.Errorf("alias exit = %d want %d", code, cli.ExitOK)
	}
	if hits != 2 {
		t.Errorf("hits = %d want 2", hits)
	}
	var rc CommentReceipt
	if err := json.Unmarshal(base.Stdout.(*bytes.Buffer).Bytes(), &rc); err != nil {
		t.Fatalf("receipt: %v", err)
	}
	if rc.ID != 77 {
		t.Errorf("receipt = %+v", rc)
	}
}

// "--body -" reads the comment text from ctx.Stdin for both comment kinds;
// a "-" with no stdin, or an empty piped body, is a usage error before any
// request.
func TestCommentBodyFromStdin(t *testing.T) {
	cases := []struct {
		cmd   commentAddCmd
		path  string
		stdin string
	}{
		{commentAddCmd{kind: "pr", gh: true}, "/api/v1/repos/o/r/issues/9/comments", "piped body\n"},
		{commentAddCmd{kind: "issue"}, "/api/v1/repos/o/r/issues/9/comments", "issue piped"},
	}
	for _, tc := range cases {
		t.Run(tc.cmd.Name(), func(t *testing.T) {
			var gotPath, gotBody string
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotPath = r.URL.Path
				raw, _ := io.ReadAll(r.Body)
				gotBody = string(raw)
				fmt.Fprint(w, `{"id":77,"html_url":"u"}`)
			}))
			defer ts.Close()
			ctx := testCtxStdin(ts, strings.NewReader(tc.stdin))
			if err := tc.cmd.Run([]string{"9", "--body", "-"}, ctx); err != nil {
				t.Fatal(err)
			}
			if gotPath != tc.path {
				t.Errorf("path = %q want %q", gotPath, tc.path)
			}
			want, _ := json.Marshal(map[string]string{"body": tc.stdin})
			if gotBody != string(want) {
				t.Errorf("body = %s want %s", gotBody, want)
			}
		})
	}
}

func TestCommentStdinErrorsBeforeRequest(t *testing.T) {
	cases := []struct {
		name  string
		args  []string
		stdin io.Reader
	}{
		{"dash with nil stdin", []string{"7", "--body", "-"}, nil},
		{"dash with empty stdin", []string{"7", "--body", "-"}, strings.NewReader("")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			hits := 0
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				hits++
			}))
			defer ts.Close()
			ctx := testCtxStdin(ts, tc.stdin)
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

// The issue comment help page must only advertise the registered spelling;
// "forge issue comment" is not a command and the help must not claim it is.
func TestIssueCommentHelpRegisteredSpelling(t *testing.T) {
	page := (commentAddCmd{kind: "issue"}).HelpPage()
	if !strings.HasPrefix(page, "use: forge issue comment add N --body T") {
		t.Errorf("issue help synopsis = %q, want use: forge issue comment add N --body T", firstLine(page))
	}
	if strings.Contains(page, "forge issue comment N") || strings.Contains(page, "canonical spelling") {
		t.Errorf("issue help page advertises unregistered alias:\n%s", page)
	}
	prPage := (commentAddCmd{kind: "pr", gh: true}).HelpPage()
	for _, want := range []string{"use: forge pr comment N --body T", "or: forge pr comment add N --body T", "canonical spelling"} {
		if !strings.Contains(prPage, want) {
			t.Errorf("pr help page missing %q:\n%s", want, prPage)
		}
	}
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

// Anchor flags present route the comment to the review-comment transport:
// one COMMENT review carrying a single inline entry, and an anchored receipt.
func TestCommentAnchoredSuccess(t *testing.T) {
	cases := []struct {
		name    string
		args    []string
		want    string
		receipt AnchoredCommentReceipt
	}{
		{
			name:    "new side default",
			args:    []string{"5", "--body", "off by one", "--file", "main.go", "--line", "12"},
			want:    `{"event":"COMMENT","body":"off by one","comments":[{"path":"main.go","body":"off by one","new_line_num":12}]}`,
			receipt: AnchoredCommentReceipt{ID: 77, Path: "main.go", Line: 12, HTMLURL: "https://h/o/r/pulls/5#discussion_r77"},
		},
		{
			name:    "old side recorded",
			args:    []string{"5", "--body", "legacy line", "--file", "main.go", "--line", "3", "--side", "old"},
			want:    `{"event":"COMMENT","body":"legacy line","comments":[{"path":"main.go","body":"legacy line","old_line_num":3}]}`,
			receipt: AnchoredCommentReceipt{ID: 77, Path: "main.go", Line: 3, Side: "old", HTMLURL: "https://h/o/r/pulls/5#discussion_r77"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var requests []string
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				raw, _ := io.ReadAll(r.Body)
				requests = append(requests, r.Method+" "+r.URL.Path+" "+string(raw))
				fmt.Fprint(w, `{"id":9,"state":"COMMENT","comments":[{"id":77,"path":"main.go","html_url":"https://h/o/r/pulls/5#discussion_r77"}]}`)
			}))
			defer ts.Close()
			ctx := testCtx(ts)
			cmd := commentAddCmd{kind: "pr", gh: true}
			if err := cmd.Run(tc.args, ctx); err != nil {
				t.Fatal(err)
			}
			if len(requests) != 1 {
				t.Fatalf("requests = %v", requests)
			}
			if requests[0] != "POST /api/v1/repos/o/r/pulls/5/reviews "+tc.want {
				t.Errorf("request = %q", requests[0])
			}
			var rc AnchoredCommentReceipt
			if err := json.Unmarshal([]byte(ctx.Stdout.(*bytes.Buffer).String()), &rc); err != nil {
				t.Fatalf("receipt: %v", err)
			}
			if rc != tc.receipt {
				t.Errorf("receipt = %+v want %+v", rc, tc.receipt)
			}
		})
	}
}

// A server that rejects the anchor is the standard exit-1 path with the
// server message, and no comment is posted by the ordinary path.
func TestCommentAnchoredServerReject(t *testing.T) {
	var paths []string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		w.WriteHeader(422)
		w.Write([]byte(`{"message":"line 404 is not part of the diff"}`))
	}))
	defer ts.Close()
	ctx := testCtx(ts)
	cmd := commentAddCmd{kind: "pr", gh: true}
	err := cmd.Run([]string{"5", "--body", "hi", "--file", "f.go", "--line", "404"}, ctx)
	mapped, ok := err.(*cli.Error)
	if !ok {
		t.Fatalf("want *cli.Error, got %T: %v", err, err)
	}
	if mapped.Code != cli.ExitRuntime {
		t.Errorf("code = %d", mapped.Code)
	}
	if !strings.Contains(mapped.Msg, "line 404 is not part of the diff") {
		t.Errorf("server message lost: %q", mapped.Msg)
	}
	if len(paths) != 1 || paths[0] != "/api/v1/repos/o/r/pulls/5/reviews" {
		t.Fatalf("anchor must be the only request, got %v", paths)
	}
}

// Flag-validation cases exit 2 before any HTTP traffic.
func TestCommentAnchoredValidation(t *testing.T) {
	cases := []struct {
		name string
		args []string
		msg  string
	}{
		{"file without line", []string{"5", "--body", "hi", "--file", "f.go"}, "--line requires"},
		{"line without file", []string{"5", "--body", "hi", "--line", "3"}, "--file requires"},
		{"dangling file flag", []string{"5", "--body", "hi", "--file"}, "--line requires"},
		{"dangling line flag", []string{"5", "--body", "hi", "--file", "f.go", "--line"}, "--line requires a line number"},
		{"dangling file with line", []string{"5", "--body", "hi", "--line", "3", "--file"}, "--file requires a path value"},
		{"dangling side flag", []string{"5", "--body", "hi", "--file", "f.go", "--line", "3", "--side"}, "--side requires old or new"},
		{"bad side", []string{"5", "--body", "hi", "--file", "f.go", "--line", "3", "--side", "sideways"}, "--side must be old or new"},
		{"zero line", []string{"5", "--body", "hi", "--file", "f.go", "--line", "0"}, "--line"},
		{"negative line", []string{"5", "--body", "hi", "--file", "f.go", "--line", "-2"}, "--line"},
		{"non-numeric line", []string{"5", "--body", "hi", "--file", "f.go", "--line", "x"}, "--line"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			hits := 0
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				hits++
				fmt.Fprint(w, `{}`)
			}))
			defer ts.Close()
			ctx := testCtx(ts)
			cmd := commentAddCmd{kind: "pr", gh: true}
			err := cmd.Run(tc.args, ctx)
			mapped, ok := err.(*cli.Error)
			if !ok || mapped.Code != cli.ExitUsage {
				t.Fatalf("want usage error, got %T: %v", err, err)
			}
			if !strings.Contains(mapped.Msg, tc.msg) {
				t.Errorf("msg = %q, want substring %q", mapped.Msg, tc.msg)
			}
			if hits != 0 {
				t.Errorf("%d requests sent; validation must happen before any request", hits)
			}
		})
	}
}
