package cmds

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"forge/internal/cli"
)

// TestEditSendsOnlySuppliedFields covers both kinds: exact method, path,
// request body, and the updated-object JSON printed on success.
func TestEditSendsOnlySuppliedFields(t *testing.T) {
	cases := []struct {
		kind string
		path string
		body string
		want string
	}{
		{"pr", "/api/v1/repos/o/r/pulls/5", `{"title":"t"}`, `"title": "t"`},
		{"issue", "/api/v1/repos/o/r/issues/5", `{"body":"b"}`, `"body": "b"`},
		{"pr", "/api/v1/repos/o/r/pulls/5", `{"title":"t","body":"b"}`, `"title": "t"`},
	}
	for _, tc := range cases {
		t.Run(tc.kind+" "+tc.body, func(t *testing.T) {
			var gotMethod, gotPath, gotBody string
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotMethod, gotPath = r.Method, r.URL.Path
				raw, _ := io.ReadAll(r.Body)
				gotBody = string(raw)
				fmt.Fprint(w, `{"number":5,"title":"t","body":"b"}`)
			}))
			defer ts.Close()
			ctx := testCtx(ts)
			out := ctx.Stdout.(*bytes.Buffer)

			var args []string
			args = append(args, "5")
			if strings.Contains(tc.body, `"title"`) {
				args = append(args, "--title", "t")
			}
			if strings.Contains(tc.body, `"body"`) {
				args = append(args, "--body", "b")
			}
			if err := (editCmd{kind: tc.kind}).Run(args, ctx); err != nil {
				t.Fatal(err)
			}
			if gotMethod != "PATCH" || gotPath != tc.path {
				t.Errorf("got %s %s, want PATCH %s", gotMethod, gotPath, tc.path)
			}
			if gotBody != tc.body {
				t.Errorf("body = %q, want %q", gotBody, tc.body)
			}
			if !strings.Contains(out.String(), tc.want) {
				t.Errorf("stdout = %q, want it to contain %q", out.String(), tc.want)
			}
		})
	}
}

// TestEditRejectsEmptyFlagSet pins the usage-error contract: no field set or
// only empty values must fail before any HTTP request.
func TestEditRejectsEmptyFlagSet(t *testing.T) {
	cases := [][]string{
		{"5"},
		{"5", "--title", ""},
		{"5", "--body", ""},
		{"5", "--title", "", "--body", ""},
	}
	for _, kind := range []string{"pr", "issue"} {
		for _, args := range cases {
			hits := 0
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				hits++
			}))
			defer ts.Close()
			ctx := testCtx(ts)

			err := (editCmd{kind: kind}).Run(args, ctx)
			cliErr, ok := err.(*cli.Error)
			if !ok {
				t.Fatalf("%s %v: err = %T, want *cli.Error", kind, args, err)
			}
			if cliErr.Code != cli.ExitUsage {
				t.Errorf("%s %v: code = %d, want ExitUsage", kind, args, cliErr.Code)
			}
			if hits != 0 {
				t.Errorf("%s %v: server saw %d requests, want 0", kind, args, hits)
			}
		}
	}
}

// TestEditRejectsMissingIndex keeps the positional-number contract shared
// with every other object command.
func TestEditRejectsMissingIndex(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer ts.Close()
	ctx := testCtx(ts)

	err := (editCmd{kind: "pr"}).Run([]string{"--title", "t"}, ctx)
	cliErr, ok := err.(*cli.Error)
	if !ok || cliErr.Code != cli.ExitUsage {
		t.Fatalf("err = %v, want usage error", err)
	}
}

// TestEditCommandsRegistered guards the registration seam for both kinds.
func TestEditCommandsRegistered(t *testing.T) {
	want := map[string]bool{"pr edit": false, "issue edit": false}
	for _, c := range PRCommands() {
		if _, ok := want[c.Name()]; ok {
			want[c.Name()] = true
		}
	}
	for _, c := range IssueCommands() {
		if _, ok := want[c.Name()]; ok {
			want[c.Name()] = true
		}
	}
	for name, found := range want {
		if !found {
			t.Errorf("%s not registered", name)
		}
	}
}
