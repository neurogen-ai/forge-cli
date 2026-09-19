package cmds

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
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

// TestEditReviewersOrdering pins the reviewer contract: the title/body
// PATCH first, then one request per reviewer flag in flag order (adds then
// removes), each posting {"reviewers":[one]}, and the final server echo
// printed.
func TestEditReviewersOrdering(t *testing.T) {
	var calls []string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		calls = append(calls, r.Method+" "+r.URL.Path+" "+string(raw))
		fmt.Fprint(w, `{"number":5,"title":"t"}`)
	}))
	defer ts.Close()
	ctx := testCtx(ts)
	out := ctx.Stdout.(*bytes.Buffer)

	if err := (editCmd{kind: "pr"}).Run([]string{
		"5", "--title", "t", "--add-reviewer", "ana", "--remove-reviewer", "bo",
	}, ctx); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"PATCH /api/v1/repos/o/r/pulls/5 {\"title\":\"t\"}",
		"POST /api/v1/repos/o/r/pulls/5/requested_reviewers {\"reviewers\":[\"ana\"]}",
		"DELETE /api/v1/repos/o/r/pulls/5/requested_reviewers {\"reviewers\":[\"bo\"]}",
	}
	if !reflect.DeepEqual(calls, want) {
		t.Errorf("calls = %q\nwant       %q", calls, want)
	}
	if !strings.Contains(out.String(), `"number": 5`) {
		t.Errorf("stdout = %q, want the updated pull request", out.String())
	}
}

// TestEditOnlyReviewersSendsNoPatch: reviewer flags alone are valid and send
// exactly one request per flag, never an empty PATCH.
func TestEditOnlyReviewersSendsNoPatch(t *testing.T) {
	var calls []string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.Path)
		fmt.Fprint(w, `{"number":5}`)
	}))
	defer ts.Close()
	ctx := testCtx(ts)

	if err := (editCmd{kind: "pr"}).Run([]string{"5", "--add-reviewer", "ana", "--add-reviewer", "cy"}, ctx); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"POST /api/v1/repos/o/r/pulls/5/requested_reviewers",
		"POST /api/v1/repos/o/r/pulls/5/requested_reviewers",
	}
	if !reflect.DeepEqual(calls, want) {
		t.Errorf("calls = %q, want %q", calls, want)
	}
}

// TestEditClearFlags pins the explicit-clearing PATCH: an empty-string field
// on the wire, and a usage error when it conflicts with the value flag.
func TestEditClearFlags(t *testing.T) {
	var gotBody string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		gotBody = string(raw)
		fmt.Fprint(w, `{"number":5,"title":""}`)
	}))
	defer ts.Close()
	ctx := testCtx(ts)

	if err := (editCmd{kind: "pr"}).Run([]string{"5", "--clear-title"}, ctx); err != nil {
		t.Fatal(err)
	}
	if gotBody != `{"title":""}` {
		t.Errorf("body = %q, want {\"title\":\"\"}", gotBody)
	}
}

func TestEditClearFlagConflictsAreUsageErrors(t *testing.T) {
	for _, args := range [][]string{
		{"5", "--clear-title", "--title", "t"},
		{"5", "--clear-body", "--body", "b"},
	} {
		hits := 0
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits++ }))
		defer ts.Close()
		ctx := testCtx(ts)

		err := (editCmd{kind: "pr"}).Run(args, ctx)
		cliErr, ok := err.(*cli.Error)
		if !ok || cliErr.Code != cli.ExitUsage {
			t.Fatalf("%v: err = %v, want usage error", args, err)
		}
		if hits != 0 {
			t.Errorf("%v: server saw %d requests, want 0", args, hits)
		}
	}
}

// TestEditReviewerFlagsRejectedOnIssue keeps reviewer flags pr-only.
func TestEditReviewerFlagsRejectedOnIssue(t *testing.T) {
	hits := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits++ }))
	defer ts.Close()
	ctx := testCtx(ts)

	err := (editCmd{kind: "issue"}).Run([]string{"5", "--add-reviewer", "ana"}, ctx)
	cliErr, ok := err.(*cli.Error)
	if !ok || cliErr.Code != cli.ExitUsage {
		t.Fatalf("err = %v, want usage error", err)
	}
	if hits != 0 {
		t.Errorf("server saw %d requests, want 0", hits)
	}
}

// TestEditReviewerFailureStillPrintsPatch: a failed reviewer call surfaces
// as a normal API error while the earlier patch result still prints.
func TestEditReviewerFailureStillPrintsPatch(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "PATCH" {
			fmt.Fprint(w, `{"number":5,"title":"t"}`)
			return
		}
		w.WriteHeader(404)
		io.WriteString(w, `{"message":"user not found"}`)
	}))
	defer ts.Close()
	ctx := testCtx(ts)
	out := ctx.Stdout.(*bytes.Buffer)

	err := (editCmd{kind: "pr"}).Run([]string{"5", "--title", "t", "--add-reviewer", "ana"}, ctx)
	cliErr, ok := err.(*cli.Error)
	if !ok || cliErr.Code != cli.ExitRuntime || !strings.Contains(cliErr.Msg, "user not found") {
		t.Fatalf("err = %v (%T), want runtime error carrying the server message", err, err)
	}
	if !strings.Contains(out.String(), `"number": 5`) {
		t.Errorf("stdout = %q, want the earlier patch result", out.String())
	}
}
