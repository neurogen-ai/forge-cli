package cmds

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"forge/internal/cli"
)

// mergeRecorder records each request as "METHOD PATH body" and answers like
// a Forgejo server: merge POSTs get 200, ref DELETEs get 204, everything else
// (the PR prefetch) gets a PR payload with a known head ref.
type mergeRecorder struct {
	requests []string
	failRef  bool // DELETE .../git/refs/... fails
	failPost bool // POST .../merge fails
}

func (m *mergeRecorder) serve(w http.ResponseWriter, r *http.Request) {
	body := ""
	if r.Body != nil {
		buf := make([]byte, 1024)
		n, _ := r.Body.Read(buf)
		body = string(buf[:n])
	}
	m.requests = append(m.requests, r.Method+" "+r.URL.Path+" "+body)
	switch {
	case strings.HasSuffix(r.URL.Path, "/merge"):
		if m.failPost {
			w.WriteHeader(409)
			fmt.Fprint(w, `{"message":"merge conflict"}`)
			return
		}
		w.WriteHeader(200)
	case strings.Contains(r.URL.Path, "/git/refs/"):
		if m.failRef {
			w.WriteHeader(403)
			fmt.Fprint(w, `{"message":"forbidden"}`)
			return
		}
		w.WriteHeader(204)
	default:
		fmt.Fprint(w, `{"number":5,"state":"open","head":{"ref":"feature-branch","sha":"abc"}}`)
	}
}

// 1. Missing and multiple strategy validation, with zero requests.
func TestMergeStrategyValidationSendsNoRequest(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{"no strategy", []string{"5"}},
		{"two strategies", []string{"5", "--merge", "--squash"}},
		{"three strategies", []string{"5", "--merge", "--squash", "--rebase"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := &mergeRecorder{}
			ts := httptest.NewServer(http.HandlerFunc(m.serve))
			defer ts.Close()
			err := (prMergeCmd{}).Run(tc.args, testCtx(ts))
			cerr, ok := err.(*cli.Error)
			if !ok {
				t.Fatalf("err = %v, want *cli.Error", err)
			}
			if cerr.Code != cli.ExitUsage {
				t.Errorf("code = %d want %d", cerr.Code, cli.ExitUsage)
			}
			if len(m.requests) != 0 {
				t.Errorf("validation sent %d requests, want 0: %v", len(m.requests), m.requests)
			}
		})
	}
}

// 2. Each strategy's exact merge body. 3. Subject and body field mapping.
func TestMergeStrategyBodies(t *testing.T) {
	cases := []struct {
		name     string
		args     []string
		wantBody string
	}{
		{"merge", []string{"5", "--merge"}, `{"Do":"merge"}`},
		{"squash", []string{"5", "--squash"}, `{"Do":"squash"}`},
		{"rebase", []string{"5", "--rebase"}, `{"Do":"rebase"}`},
		{"subject+body", []string{"5", "--merge", "--subject", "Title", "--body", "Msg"}, `{"Do":"merge","MergeTitleField":"Title","MergeMessageField":"Msg"}`},
		{"subject only", []string{"5", "--squash", "--subject", "Only Title"}, `{"Do":"squash","MergeTitleField":"Only Title"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := &mergeRecorder{}
			ts := httptest.NewServer(http.HandlerFunc(m.serve))
			defer ts.Close()
			ctx := testCtx(ts)
			if err := (prMergeCmd{}).Run(tc.args, ctx); err != nil {
				t.Fatal(err)
			}
			mergeBody := ""
			for _, req := range m.requests {
				if strings.Contains(req, "/merge") {
					mergeBody = strings.SplitN(req, " ", 3)[2]
				}
			}
			if mergeBody != tc.wantBody {
				t.Errorf("merge body = %q want %q", mergeBody, tc.wantBody)
			}
		})
	}
}

// 4. Successful merge without delete: one request, receipt head_deleted false.
func TestMergeSuccessWithoutDelete(t *testing.T) {
	m := &mergeRecorder{}
	ts := httptest.NewServer(http.HandlerFunc(m.serve))
	defer ts.Close()
	ctx := testCtx(ts)
	if err := (prMergeCmd{}).Run([]string{"5", "--rebase"}, ctx); err != nil {
		t.Fatal(err)
	}
	if len(m.requests) != 1 {
		t.Fatalf("requests = %v, want exactly one merge POST", m.requests)
	}
	var rc MergeReceipt
	if err := json.Unmarshal([]byte(ctx.Stdout.(*bytes.Buffer).String()), &rc); err != nil {
		t.Fatalf("receipt: %v", err)
	}
	if rc.Index != 5 || rc.Action != "rebase" || rc.HeadDeleted {
		t.Errorf("receipt = %+v, want {5 rebase false}", rc)
	}
}

// 5. --delete request ordering: GET PR, POST merge, DELETE ref; head_deleted true.
func TestMergeDeleteRequestOrdering(t *testing.T) {
	m := &mergeRecorder{}
	ts := httptest.NewServer(http.HandlerFunc(m.serve))
	defer ts.Close()
	ctx := testCtx(ts)
	if err := (prMergeCmd{}).Run([]string{"5", "--merge", "--delete"}, ctx); err != nil {
		t.Fatal(err)
	}
	want := []string{"GET /api/v1/repos/o/r/pulls/5 ", "POST /api/v1/repos/o/r/pulls/5/merge ", "DELETE /api/v1/repos/o/r/git/refs/heads/feature-branch "}
	if len(m.requests) != 3 {
		t.Fatalf("requests = %v, want 3 in order GET, POST merge, DELETE ref", m.requests)
	}
	for i, req := range m.requests {
		if !strings.HasPrefix(req, want[i]) {
			t.Errorf("request %d = %q, want prefix %q", i, req, want[i])
		}
	}
	var rc MergeReceipt
	if err := json.Unmarshal([]byte(ctx.Stdout.(*bytes.Buffer).String()), &rc); err != nil {
		t.Fatalf("receipt: %v", err)
	}
	if !rc.HeadDeleted {
		t.Error("head_deleted = false after successful delete")
	}
}

// 6. Failed merge: mapped error and no DELETE request.
func TestMergeFailureSendsNoDelete(t *testing.T) {
	m := &mergeRecorder{failPost: true}
	ts := httptest.NewServer(http.HandlerFunc(m.serve))
	defer ts.Close()
	err := (prMergeCmd{}).Run([]string{"5", "--merge", "--delete"}, testCtx(ts))
	cerr, ok := err.(*cli.Error)
	if !ok {
		t.Fatalf("err = %v, want *cli.Error", err)
	}
	if cerr.Code != cli.ExitRuntime || !strings.Contains(cerr.Msg, "merge conflict") {
		t.Errorf("err = %+v, want ExitRuntime with server message", cerr)
	}
	for _, req := range m.requests {
		if strings.HasPrefix(req, "DELETE") {
			t.Errorf("failed merge triggered delete: %v", m.requests)
		}
	}
	if len(m.requests) != 2 {
		t.Errorf("requests = %v, want only GET + POST (no DELETE)", m.requests)
	}
}

// 7. Delete failure after successful merge: receipt with head_deleted false is
// written first, then the mapped error keeps its code and gains a merge-
// succeeded hint while the server message stays unchanged.
func TestMergeDeleteFailureKeepsReceiptAndCode(t *testing.T) {
	m := &mergeRecorder{failRef: true}
	ts := httptest.NewServer(http.HandlerFunc(m.serve))
	defer ts.Close()
	ctx := testCtx(ts)
	err := (prMergeCmd{}).Run([]string{"5", "--merge", "--delete"}, ctx)
	cerr, ok := err.(*cli.Error)
	if !ok {
		t.Fatalf("err = %v, want *cli.Error", err)
	}
	if cerr.Code != cli.ExitAuth {
		t.Errorf("code = %d, want mapped ExitAuth (403)", cerr.Code)
	}
	if !strings.Contains(cerr.Msg, "forbidden") {
		t.Errorf("msg %q missing server message", cerr.Msg)
	}
	if !strings.Contains(cerr.Hint, "merge succeeded but branch cleanup failed") {
		t.Errorf("hint %q missing merge-succeeded context", cerr.Hint)
	}
	var rc MergeReceipt
	if err := json.Unmarshal([]byte(ctx.Stdout.(*bytes.Buffer).String()), &rc); err != nil {
		t.Fatalf("receipt not printed: %v", err)
	}
	if rc.HeadDeleted {
		t.Error("head_deleted = true despite delete failure")
	}
}

// 8. Missing head ref and failed prefetch: no merge, no delete.
func TestMergeMissingHeadRefAndPrefetchFailure(t *testing.T) {
	// Missing head ref: server returns a PR with an empty head.ref.
	m := &mergeRecorder{}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m.requests = append(m.requests, r.Method)
		fmt.Fprint(w, `{"number":5,"state":"open","head":{"ref":"","sha":"abc"}}`)
	}))
	defer ts.Close()
	err := (prMergeCmd{}).Run([]string{"5", "--merge", "--delete"}, testCtx(ts))
	cerr, ok := err.(*cli.Error)
	if !ok {
		t.Fatalf("err = %v, want *cli.Error", err)
	}
	if cerr.Code != cli.ExitRuntime {
		t.Errorf("code = %d, want ExitRuntime", cerr.Code)
	}
	if len(m.requests) != 1 || m.requests[0] != "GET" {
		t.Errorf("requests = %v, want only the prefetch GET with no merge or delete", m.requests)
	}

	// Failed prefetch: the GET fails, so no merge is posted.
	m2 := &mergeRecorder{}
	ts2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m2.requests = append(m2.requests, r.Method+" "+r.URL.Path)
		w.WriteHeader(404)
		fmt.Fprint(w, `{"message":"not found"}`)
	}))
	defer ts2.Close()
	err = (prMergeCmd{}).Run([]string{"5", "--merge", "--delete"}, testCtx(ts2))
	if err == nil {
		t.Fatal("want error on failed prefetch")
	}
	for _, req := range m2.requests {
		if strings.HasPrefix(req, "POST") || strings.HasPrefix(req, "DELETE") {
			t.Errorf("prefetch failure triggered %s", req)
		}
	}
}
