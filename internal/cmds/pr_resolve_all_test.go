package cmds

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"forge/internal/api"
	"forge/internal/cli"
)

// resolveAllFixture drives a fake Gitea server for resolve-all scenarios.
// It records every PATCH to the thread-resolution endpoint.
type resolveAllFixture struct {
	t          *testing.T
	patchCalls []int64 // comment ids resolved, in arrival order
	reviews    []api.Review
	comments   map[int64][]api.ReviewComment // reviewID -> comments
	resolveErr map[int64]*api.APIError       // commentID -> forced failure
	mu         sync.Mutex
}

func newResolveAllFixture(t *testing.T) *resolveAllFixture {
	return &resolveAllFixture{
		t:          t,
		reviews:    []api.Review{{ID: 11}, {ID: 12}},
		comments:   map[int64][]api.ReviewComment{},
		resolveErr: map[int64]*api.APIError{},
	}
}

func (f *resolveAllFixture) server() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		switch {
		case strings.HasSuffix(r.URL.Path, "/reviews") && r.Method == http.MethodGet:
			writeTestJSON(f.t, w, f.reviews)
		case strings.Contains(r.URL.Path, "/comments") && r.Method == http.MethodGet:
			var rid int64
			fmt.Sscanf(r.URL.Path, "/api/v1/repos/o/r/pulls/7/reviews/%d/comments", &rid)
			writeTestJSON(f.t, w, f.comments[rid])
		case r.Method == http.MethodPatch && strings.HasSuffix(r.URL.Path, "/resolve"):
			var cid int64
			fmt.Sscanf(r.URL.Path, "/api/v1/repos/o/r/pulls/comments/%d/resolve", &cid)
			if apiErr, ok := f.resolveErr[cid]; ok {
				w.WriteHeader(apiErr.Status)
				w.Write([]byte(`{"message":"` + apiErr.Message + `"}`))
				return
			}
			f.patchCalls = append(f.patchCalls, cid)
			w.WriteHeader(200)
		default:
			http.NotFound(w, r)
		}
	}))
}

func containsInt(s []int64, v int64) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

func writeTestJSON(t *testing.T, w http.ResponseWriter, v any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		t.Fatalf("fixture encode: %v", err)
	}
}

func unresolved(id int64, resolved bool) api.ReviewComment {
	rc := api.ReviewComment{ID: id}
	if !resolved {
		false_ := false
		rc.Resolved = &false_
	} else {
		true_ := true
		rc.Resolved = &true_
	}
	return rc
}

func runResolveAll(t *testing.T, ts *httptest.Server, args ...string) (*cli.Ctx, error) {
	t.Helper()
	ctx := resolveTestCtx(t, ts)
	ctx.GlobalFlags.Owner, ctx.GlobalFlags.Repo = "o", "r"
	err := (resolveAllCmd{}).Run(append([]string{"7"}, args...), ctx)
	return ctx, err
}

func decodeJSON[T any](t *testing.T, b []byte) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(b, &v); err != nil {
		t.Fatalf("decode %s: %v", b, err)
	}
	return v
}

// 1. Dry-run prints exactly the sorted unresolved ids and issues zero PATCHes.
func TestResolveAllDryRunPrintsIdsOnlyAndNeverMutates(t *testing.T) {
	f := newResolveAllFixture(t)
	f.comments[11] = []api.ReviewComment{unresolved(33, false), unresolved(21, true), unresolved(27, false)}
	f.comments[12] = []api.ReviewComment{unresolved(15, false), unresolved(40, true)}
	ts := f.server()
	defer ts.Close()

	ctx, err := runResolveAll(t, ts) // no --yes
	if err != nil {
		t.Fatalf("dry-run: %v", err)
	}
	got := ctx.Stdout.(*bytes.Buffer).String()
	// Plain JSON array of int64 ids, ascending, no wrapper object.
	var ids []int64
	if err := json.Unmarshal([]byte(got), &ids); err != nil {
		t.Fatalf("stdout %q not a bare id array: %v", got, err)
	}
	want := []int64{15, 27, 33}
	if len(ids) != 3 || ids[0] != want[0] || ids[1] != want[1] || ids[2] != want[2] {
		t.Fatalf("ids = %v, want %v", ids, want)
	}
	if len(f.patchCalls) != 0 {
		t.Fatalf("dry-run issued %d PATCH calls, want 0", len(f.patchCalls))
	}
}

// 2. --yes happy path resolves everything and prints the summary.
func TestResolveAllYesHappyPath(t *testing.T) {
	f := newResolveAllFixture(t)
	f.comments[11] = []api.ReviewComment{unresolved(30, false), unresolved(20, false)}
	f.comments[12] = []api.ReviewComment{unresolved(10, false)}
	ts := f.server()
	defer ts.Close()

	ctx, err := runResolveAll(t, ts, "--yes")
	if err != nil {
		t.Fatalf("--yes: %v", err)
	}
	sum := decodeJSON[ResolveAllSummary](t, ctx.Stdout.(*bytes.Buffer).Bytes())
	if sum.Requested != 3 || sum.Resolved != 3 || sum.Skipped != 0 || len(sum.Failed) != 0 {
		t.Fatalf("summary = %+v", sum)
	}
	// Deterministic ascending order despite fixture ordering.
	if len(f.patchCalls) != 3 || f.patchCalls[0] != 10 || f.patchCalls[1] != 20 || f.patchCalls[2] != 30 {
		t.Fatalf("patch order = %v, want [10 20 30]", f.patchCalls)
	}
}

// 3. Partial failure: 500 on one target carries a verbatim Failed entry and a
// runtime error naming the count; summary still lands on stdout first.
func TestResolveAllPartialFailureKeepsStdoutIntact(t *testing.T) {
	f := newResolveAllFixture(t)
	f.comments[11] = []api.ReviewComment{unresolved(10, false), unresolved(20, false), unresolved(30, false)}
	f.resolveErr[20] = &api.APIError{Status: 500, Message: "boom inside"}
	ts := f.server()
	defer ts.Close()

	ctx, err := runResolveAll(t, ts, "--yes")
	cerr, ok := err.(*cli.Error)
	if !ok || cerr.Code != cli.ExitRuntime {
		t.Fatalf("want ExitRuntime cli.Error, got %v", err)
	}
	if !strings.Contains(cerr.Msg, "1 of 3 resolutions failed") {
		t.Fatalf("msg = %q, want count wording", cerr.Msg)
	}
	out := ctx.Stdout.(*bytes.Buffer).String()
	if !strings.HasPrefix(out, "{") {
		t.Fatalf("summary not printed first: stdout=%q stderr=%q", out, ctx.Stderr.(*bytes.Buffer).String())
	}
	sum := decodeJSON[ResolveAllSummary](t, []byte(out))
	if sum.Resolved != 2 || sum.Requested != 3 || len(sum.Failed) != 1 ||
		sum.Failed[0].ID != 20 || sum.Failed[0].Text != "boom inside" {
		t.Fatalf("summary = %+v", sum)
	}
	// Serial execution: all three targets attempted, including the failing one.
	if len(f.patchCalls)+len(f.resolveErr) < 3 || !containsInt(f.patchCalls, 10) {
		t.Fatalf("attempts = %v, want all three targets tried", f.patchCalls)
	}
}

// 4. Missing endpoint (404): abort immediately with the loud version hint,
// with no further per-thread PATCH recorded after the failure.
func TestResolveAllAbortsWithVersionHintOn404(t *testing.T) {
	f := newResolveAllFixture(t)
	f.comments[11] = []api.ReviewComment{unresolved(10, false), unresolved(20, false)}
	seen := map[int64]int{}
	base := f.server()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPatch {
			var cid int64
			fmt.Sscanf(r.URL.Path, "/api/v1/repos/o/r/pulls/comments/%d/resolve", &cid)
			seen[cid]++
			if cid == 10 {
				w.WriteHeader(404)
				w.Write([]byte(`{"message":"Not Found"}`))
				return
			}
		}
		base.Config.Handler.ServeHTTP(w, r)
	}))
	defer ts.Close()
	defer base.Close()

	_, err := runResolveAll(t, ts, "--yes")
	cerr, ok := err.(*cli.Error)
	if !ok || cerr.Code != cli.ExitRuntime {
		t.Fatalf("want ExitRuntime cli.Error, got %v", err)
	}
	if !strings.Contains(cerr.Msg, "does not expose the comment-resolution endpoint") ||
		!strings.Contains(cerr.Hint, "conversation resolution") {
		t.Fatalf("missing version hint: %+v", cerr)
	}
	if seen[20] != 0 {
		t.Fatalf("thread 20 patched after 404 abort: %d calls", seen[20])
	}
	if seen[10] != 1 {
		t.Fatalf("first target patched %d times, want exactly 1", seen[10])
	}
}

// 5. Rerun semantics: everything already resolved → empty summary, exit 0,
// Skipped pinned at zero.
func TestResolveAllRerunIsZeroWork(t *testing.T) {
	f := newResolveAllFixture(t)
	f.comments[11] = []api.ReviewComment{unresolved(5, true), unresolved(6, true)}
	f.comments[12] = []api.ReviewComment{unresolved(9, true)}
	ts := f.server()
	defer ts.Close()

	ctx, err := runResolveAll(t, ts, "--yes")
	if err != nil {
		t.Fatalf("rerun --yes: %v", err)
	}
	var sum struct {
		Requested int `json:"requested"`
		Resolved  int `json:"resolved"`
		Skipped   int `json:"skipped"`
	}
	if err := json.Unmarshal(ctx.Stdout.(*bytes.Buffer).Bytes(), &sum); err != nil {
		t.Fatalf("summary not JSON: %v", err)
	}
	if sum.Requested != 0 || sum.Resolved != 0 || sum.Skipped != 0 {
		t.Fatalf("summary = %+v, want all zeros", sum)
	}
	if len(f.patchCalls) != 0 {
		t.Fatalf("rerun issued %d PATCHes, want 0", len(f.patchCalls))
	}
}

// 6. --review filters targets; an unknown review id exits with usage error
// naming the available reviews.
func TestResolveAllReviewFilterRestrictsAndErrors(t *testing.T) {
	f := newResolveAllFixture(t)
	f.comments[11] = []api.ReviewComment{unresolved(101, false)}
	f.comments[12] = []api.ReviewComment{unresolved(202, false)}
	ts := f.server()
	defer ts.Close()

	// Restrictive filter: only review 12's thread resolved.
	ctx, err := runResolveAll(t, ts, "--yes", "--review", "12")
	if err != nil {
		t.Fatalf("--review 12: %v", err)
	}
	sum := decodeJSON[ResolveAllSummary](t, ctx.Stdout.(*bytes.Buffer).Bytes())
	if sum.Resolved != 1 || len(f.patchCalls) != 1 || f.patchCalls[0] != 202 {
		t.Fatalf("filter failed: %+v patchCalls=%v", sum, f.patchCalls)
	}

	// Mismatched filter names available reviews with usage exit code.
	bad := newResolveAllFixture(t)
	tsBad := bad.server()
	defer tsBad.Close()
	_, err = runResolveAll(t, tsBad, "--yes", "--review", "99")
	cerr, ok := err.(*cli.Error)
	if !ok || cerr.Code != cli.ExitUsage {
		t.Fatalf("bad review: want ExitUsage, got %v", err)
	}
	if !strings.Contains(cerr.Hint, "11") || !strings.Contains(cerr.Hint, "12") {
		t.Fatalf("hint missing available reviews: %+v", cerr)
	}

	// Non-numeric review value also exits 2.
	_, err = runResolveAll(t, tsBad, "--review", "abc")
	if cerr, ok := err.(*cli.Error); !ok || cerr.Code != cli.ExitUsage {
		t.Fatalf("non-numeric review: want ExitUsage, got %v", err)
	}
}
