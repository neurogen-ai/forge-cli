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

// TestPRStateActions covers all three commands against one handler: exact
// method, path, request body, and the updated-PR JSON printed on success.
func TestPRStateActions(t *testing.T) {
	cases := []struct {
		action string
		state  string
		body   string
	}{
		{"close", "closed", `{"state":"closed"}`},
		{"reopen", "open", `{"state":"open"}`},
		{"ready", "open", `{"draft":false}`},
	}
	for _, tc := range cases {
		t.Run(tc.action, func(t *testing.T) {
			var gotMethod, gotPath, gotBody string
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotMethod = r.Method
				gotPath = r.URL.Path
				raw, _ := io.ReadAll(r.Body)
				gotBody = string(raw)
				fmt.Fprint(w, `{"number":5,"title":"t","state":"`+tc.state+`"}`)
			}))
			defer ts.Close()
			ctx := testCtx(ts)
			if err := (prStateCmd{action: tc.action}).Run([]string{"5"}, ctx); err != nil {
				t.Fatal(err)
			}
			if gotMethod != "PATCH" || gotPath != "/api/v1/repos/o/r/pulls/5" {
				t.Errorf("got %s %s", gotMethod, gotPath)
			}
			if gotBody != tc.body {
				t.Errorf("body = %q, want %q", gotBody, tc.body)
			}
			stdout := ctx.Stdout.(*bytes.Buffer).String()
			if !strings.Contains(stdout, `"number": 5`) || !strings.Contains(stdout, `"state": "`+tc.state+`"`) {
				t.Errorf("stdout missing updated PR fields:\n%s", stdout)
			}
		})
	}
}

// TestPRStateBadIndex checks malformed numbers fail as usage errors with no
// request.
func TestPRStateBadIndex(t *testing.T) {
	hits := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.WriteHeader(500)
	}))
	defer ts.Close()
	ctx := testCtx(ts)
	err := (prStateCmd{action: "close"}).Run([]string{"not-a-number"}, ctx)
	cerr, ok := err.(*cli.Error)
	if !ok {
		t.Fatalf("err = %v, want *cli.Error", err)
	}
	if cerr.Code != cli.ExitUsage {
		t.Errorf("code = %d, want %d", cerr.Code, cli.ExitUsage)
	}
	if hits != 0 {
		t.Errorf("bad index sent %d requests, want 0", hits)
	}
}

// TestPRStateReadyError checks that a server which rejects the draft change
// keeps its message and maps to the runtime exit code.
func TestPRStateReadyError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(422)
		fmt.Fprint(w, `{"message":"draft is not supported"}`)
	}))
	defer ts.Close()
	ctx := testCtx(ts)
	err := (prStateCmd{action: "ready"}).Run([]string{"5"}, ctx)
	cerr, ok := err.(*cli.Error)
	if !ok {
		t.Fatalf("err = %v, want *cli.Error", err)
	}
	if cerr.Code != cli.ExitRuntime {
		t.Errorf("code = %d, want %d", cerr.Code, cli.ExitRuntime)
	}
	if !strings.Contains(cerr.Msg, "draft is not supported") {
		t.Errorf("msg %q missing server message", cerr.Msg)
	}
}

// TestPRStateNoExtraGET verifies each action performs exactly one request
// and it is the PATCH, not a fetch of the pull request.
func TestPRStateNoExtraGET(t *testing.T) {
	for _, action := range []string{"close", "reopen", "ready"} {
		t.Run(action, func(t *testing.T) {
			var methods []string
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				methods = append(methods, r.Method)
				fmt.Fprint(w, `{"number":5,"state":"open"}`)
			}))
			defer ts.Close()
			if err := (prStateCmd{action: action}).Run([]string{"5"}, testCtx(ts)); err != nil {
				t.Fatal(err)
			}
			if len(methods) != 1 || methods[0] != "PATCH" {
				t.Errorf("requests = %v, want exactly one PATCH", methods)
			}
		})
	}
}
