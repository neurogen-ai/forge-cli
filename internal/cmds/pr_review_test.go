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

// reviewRosterServer serves two reviews whose ids come back deliberately out
// of chronological order, with mixed-resolution comment threads per review.
// Server order must be preserved by the roster.
func reviewRosterServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/repos/o/r/pulls/7/reviews":
			fmt.Fprint(w, `[
				{"id":20,"state":"COMMENTED","submitted_at":"2024-02-01T00:00:00Z","user":{"login":"beth"}},
				{"id":10,"state":"APPROVED","submitted_at":"2024-01-01T00:00:00Z","user":{"login":"ann"}},
				{"id":30,"state":"APPROVED","submitted_at":"2024-03-01T00:00:00Z","user":{"login":"cyd"}}]`)
		case "/api/v1/repos/o/r/pulls/7/reviews/10/comments":
			fmt.Fprint(w, `[{"id":1,"path":"a.go","resolved":true},{"id":2,"path":"a.go","resolved":false}]`)
		case "/api/v1/repos/o/r/pulls/7/reviews/20/comments":
			fmt.Fprint(w, `[{"id":3,"path":"b.go","resolved":true}]`)
		case "/api/v1/repos/o/r/pulls/7/reviews/30/comments":
			fmt.Fprint(w, `[]`)
		default:
			t.Errorf("unexpected path %q", r.URL.Path)
			w.WriteHeader(500)
		}
	}))
}

func TestReviewListPreservesServerOrderAndCounts(t *testing.T) {
	ts := reviewRosterServer(t)
	defer ts.Close()
	ctx := testCtx(ts)
	if err := (reviewListCmd{}).Run([]string{"7"}, ctx); err != nil {
		t.Fatal(err)
	}
	got := ctx.Stdout.(*bytes.Buffer).String()
	var rows []ReviewRow
	if err := json.Unmarshal([]byte(got), &rows); err != nil {
		t.Fatalf("json: %v\n%s", err, got)
	}
	if len(rows) != 3 {
		t.Fatalf("got %d rows want 3", len(rows))
	}
	// Server order: 20, 10, 30 — no resorting.
	for i, wantID := range []int64{20, 10, 30} {
		if rows[i].ID != wantID {
			t.Errorf("row %d id=%d want %d", i, rows[i].ID, wantID)
		}
	}
	// Counts in server order: 20 = 0 unresolved / 1 total; 10 = 1/2; 30 = 0/0.
	wantCounts := [][2]int{{0, 1}, {1, 2}, {0, 0}}
	for i, wc := range wantCounts {
		if rows[i].UnresolvedCount != wc[0] || rows[i].TotalCount != wc[1] {
			t.Errorf("row %d counts %d/%d want %d/%d",
				i, rows[i].UnresolvedCount, rows[i].TotalCount, wc[0], wc[1])
		}
	}
}

func TestReviewListStateFilter(t *testing.T) {
	ts := reviewRosterServer(t)
	defer ts.Close()
	ctx := testCtx(ts)
	if err := (reviewListCmd{}).Run([]string{"7", "--state", "APPROVED"}, ctx); err != nil {
		t.Fatal(err)
	}
	var rows []ReviewRow
	if err := json.Unmarshal([]byte(ctx.Stdout.(*bytes.Buffer).String()), &rows); err != nil {
		t.Fatal(err)
	}
	ids := make([]int64, 0, len(rows))
	for _, r := range rows {
		ids = append(ids, r.ID)
	}
	eqInt64(t, "filtered ids", ids, []int64{10, 30})
}

func TestReviewListTableRender(t *testing.T) {
	ts := reviewRosterServer(t)
	defer ts.Close()
	ctx := testCtx(ts)
	ctx.Format = cli.FormatTable
	if err := (reviewListCmd{}).Run([]string{"7"}, ctx); err != nil {
		t.Fatal(err)
	}
	out := ctx.Stdout.(*bytes.Buffer).String()
	if !strings.HasPrefix(out, "REVIEW") {
		t.Errorf("table output should start with REVIEW header:\n%s", out)
	}
	if !strings.Contains(out, "---") {
		t.Errorf("table output missing dashed separator:\n%s", out)
	}
	if !strings.Contains(out, "beth") || !strings.Contains(out, "ann") ||
		!strings.Contains(out, "APPROVED") {
		t.Errorf("table output missing expected row data:\n%s", out)
	}
}
