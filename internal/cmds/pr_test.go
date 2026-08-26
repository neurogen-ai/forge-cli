package cmds

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"forge/internal/api"
	"forge/internal/cli"
)

// testCtx builds a Ctx whose API client talks to ts, with owner/repo preset.
func testCtx(ts *httptest.Server) *cli.Ctx {
	return &cli.Ctx{
		Stdout: &bytes.Buffer{},
		Stderr: &bytes.Buffer{},
		GlobalFlags: cli.GlobalFlags{
			Host: "git.example.com", Owner: "o", Repo: "r",
		},
		API: api.NewClient(ts.URL, "tok", 0, nil),
	}
}

func TestPRGet(t *testing.T) {
	var gotPath string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		fmt.Fprint(w, `{"number":5,"title":"t","state":"open"}`)
	}))
	defer ts.Close()

	cmd := prGetCmd{}
	if err := cmd.Run([]string{"5"}, testCtx(ts)); err != nil {
		t.Fatal(err)
	}
	if gotPath != "/api/v1/repos/o/r/pulls/5" {
		t.Errorf("path = %q", gotPath)
	}
}

func TestPRGetRequiresNumber(t *testing.T) {
	err := (prGetCmd{}).Run(nil, testCtx(httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))))
	cerr, ok := err.(*cli.Error)
	if !ok || cerr.Code != cli.ExitUsage {
		t.Fatalf("want ExitUsage, got %v", err)
	}
}

func TestPRCreateRequiresTitle(t *testing.T) {
	err := (prCreateCmd{}).Run([]string{}, nil)
	cerr, ok := err.(*cli.Error)
	if !ok || cerr.Code != cli.ExitUsage {
		t.Fatalf("want ExitUsage, got %v", err)
	}
}

func TestPRListOutputIsJSONArray(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `[{"number":1},{"number":2}]`)
	}))
	defer ts.Close()

	ctx := testCtx(ts)
	if err := (prListCmd{}).Run([]string{}, ctx); err != nil {
		t.Fatal(err)
	}
	var out []map[string]any
	if err := json.Unmarshal(ctx.Stdout.(*bytes.Buffer).Bytes(), &out); err != nil {
		t.Fatalf("stdout is not JSON array: %v\n%s", err, ctx.Stdout.(*bytes.Buffer).String())
	}
	if len(out) != 2 {
		t.Errorf("len = %d", len(out))
	}
}

func groupedTestServer(t *testing.T) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/repos/o/r/issues/7/comments":
			fmt.Fprint(w, `[{"id":1,"user":{"login":"a"},"body":"c1"}]`)
		case "/api/v1/repos/o/r/pulls/7/reviews":
			fmt.Fprint(w, `[{"id":10,"state":"APPROVED","user":{"login":"b"}}]`)
		case "/api/v1/repos/o/r/pulls/7/reviews/10/comments":
			fmt.Fprint(w, `[{"id":20,"path":"x.go","diff_hunk":"@@ -1 +1 @@"}]`)
		default:
			t.Errorf("unexpected path %q", r.URL.Path)
		}
	}))
}

func TestPRConversationGroupedDefault(t *testing.T) {
	ts := groupedTestServer(t)
	defer ts.Close()

	ctx := testCtx(ts)
	if err := (prConvCmd{}).Run([]string{"7"}, ctx); err != nil {
		t.Fatal(err)
	}
	var out struct {
		Comments []map[string]any `json:"comments"`
		Reviews  []struct {
			ID       int64            `json:"id"`
			State    string           `json:"state"`
			Comments []map[string]any `json:"comments"`
		} `json:"reviews"`
	}
	if err := json.Unmarshal(ctx.Stdout.(*bytes.Buffer).Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Comments) != 1 || len(out.Reviews) != 1 || out.Reviews[0].ID != 10 ||
		len(out.Reviews[0].Comments) != 1 {
		t.Errorf("grouped shape wrong: %+v", out)
	}
}

func TestPRConversationFlatSortedAndTagged(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/repos/o/r/issues/7/comments":
			fmt.Fprint(w, `[{"id":1,"created_at":"2024-01-02T00:00:00Z"}]`)
		case "/api/v1/repos/o/r/pulls/7/reviews":
			fmt.Fprint(w, `[{"id":10,"created_at":"2024-01-01T00:00:00Z"}]`)
		case "/api/v1/repos/o/r/pulls/7/reviews/10/comments":
			fmt.Fprint(w, `[{"id":20,"review_id":10,"created_at":"2024-01-03T00:00:00Z"}]`)
		default:
			t.Errorf("unexpected path %q", r.URL.Path)
		}
	}))
	defer ts.Close()

	ctx := testCtx(ts)
	if err := (prConvCmd{}).Run([]string{"7", "--format", "flat"}, ctx); err != nil {
		t.Fatal(err)
	}
	var items []struct {
		Kind     string `json:"kind"`
		ID       int64  `json:"id"`
		ReviewID int64  `json:"review_id"`
	}
	b := ctx.Stdout.(*bytes.Buffer).Bytes()
	if err := json.Unmarshal(b, &items); err != nil {
		t.Fatal(err)
	}
	if len(items) != 3 {
		t.Fatalf("len = %d", len(items))
	}
	wantKinds := []string{"review", "comment", "review-comment"}
	for i, k := range wantKinds {
		if items[i].Kind != k {
			t.Errorf("items[%d].kind = %q, want %q (order: %v)", i, items[i].Kind, k, b)
		}
	}
	if items[2].ReviewID != 10 {
		t.Errorf("review-comment review_id = %d", items[2].ReviewID)
	}
}
