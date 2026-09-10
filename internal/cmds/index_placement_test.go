package cmds

// Tests for options-anywhere index placement: every N-taking command resolves
// the first numeric token as the index whether it leads the arguments or
// follows its flags, and a numeric value flag (--min-unresolved 2, --subject
// 42, --body 11, --review 11) is never mistaken for the index.

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"forge/internal/api"
)

// indexFixture records every request path and answers the minimal payloads
// the N-taking commands need to reach their index use.
type indexFixture struct {
	paths []string
}

func (f *indexFixture) serve(w http.ResponseWriter, r *http.Request) {
	f.paths = append(f.paths, r.Method+" "+r.URL.Path)
	switch {
	case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/comments"):
		w.Write([]byte(`{"id":77,"html_url":"https://git.example.com/o/r/issues/11#issuecomment-77"}`))
	case strings.HasSuffix(r.URL.Path, "/reviews") && r.Method == http.MethodGet:
		w.Write([]byte(`[{"id":3,"state":"APPROVED"}]`))
	case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/reviews"):
		w.Write([]byte(`{"id":9,"state":"APPROVED"}`))
	default:
		w.Write([]byte(`[]`))
	}
}

// wantPaths asserts the server saw a request for want and never one for the
// numeric flag value decoy (notWant, empty to skip).
func wantPaths(t *testing.T, f *indexFixture, want, notWant string) {
	t.Helper()
	joined := strings.Join(f.paths, "\n")
	if !strings.Contains(joined, want) {
		t.Errorf("requests = %v, want a request containing %q", f.paths, want)
	}
	if notWant != "" && strings.Contains(joined, notWant) {
		t.Errorf("requests = %v, flag value %q was mistaken for the index", f.paths, notWant)
	}
}

func TestIndexPlacementCommentAdd(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want string
	}{
		{"index first", []string{"11", "--body", "ok"}, "/issues/11/comments"},
		{"flags first", []string{"--body", "ok", "11"}, "/issues/11/comments"},
		{"numeric flag value", []string{"--body", "11", "12"}, "/issues/12/comments"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := &indexFixture{}
			ts := httptest.NewServer(http.HandlerFunc(f.serve))
			defer ts.Close()
			if err := (commentAddCmd{kind: "pr"}).Run(tc.args, testCtx(ts)); err != nil {
				t.Fatal(err)
			}
			wantPaths(t, f, tc.want, "")
		})
	}
}

func TestIndexPlacementConv(t *testing.T) {
	cases := []struct {
		name    string
		args    []string
		notWant string
	}{
		{"index first", []string{"11"}, ""},
		{"flags first", []string{"--all", "11"}, ""},
		{"numeric flag value", []string{"--min-unresolved", "2", "11"}, "/pulls/2"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := &indexFixture{}
			ts := httptest.NewServer(http.HandlerFunc(f.serve))
			defer ts.Close()
			if err := (prConvCmd{}).Run(tc.args, testCtx(ts)); err != nil {
				t.Fatal(err)
			}
			wantPaths(t, f, "/pulls/11/reviews", tc.notWant)
		})
	}
}

func TestIndexPlacementMerge(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{"index first", []string{"11", "--merge"}},
		{"flags first", []string{"--merge", "11"}},
		{"numeric flag value", []string{"--subject", "42", "11", "--merge"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := &mergeRecorder{}
			ts := httptest.NewServer(http.HandlerFunc(m.serve))
			defer ts.Close()
			if err := (prMergeCmd{}).Run(tc.args, testCtx(ts)); err != nil {
				t.Fatal(err)
			}
			joined := strings.Join(m.requests, "\n")
			if !strings.Contains(joined, "POST /api/v1/repos/o/r/pulls/11/merge") {
				t.Errorf("requests = %v, want merge POST for index 11", m.requests)
			}
			if strings.Contains(joined, "/pulls/42") {
				t.Errorf("requests = %v, --subject value 42 was mistaken for the index", m.requests)
			}
		})
	}
}

func TestIndexPlacementReviewList(t *testing.T) {
	cases := []struct {
		name    string
		args    []string
		want    string
		notWant string
	}{
		{"index first", []string{"11"}, "/pulls/11/reviews", ""},
		{"flags first", []string{"--state", "APPROVED", "11"}, "/pulls/11/reviews", ""},
		{"numeric flag value", []string{"--state", "11", "12"}, "/pulls/12/reviews", "/pulls/11/"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := &indexFixture{}
			ts := httptest.NewServer(http.HandlerFunc(f.serve))
			defer ts.Close()
			if err := (reviewListCmd{}).Run(tc.args, testCtx(ts)); err != nil {
				t.Fatal(err)
			}
			wantPaths(t, f, tc.want, tc.notWant)
		})
	}
}

func TestIndexPlacementReviewSubmit(t *testing.T) {
	cases := []struct {
		name    string
		args    []string
		want    string
		notWant string
	}{
		{"index first", []string{"11", "--state", "approve"}, "/pulls/11/reviews", ""},
		{"flags first", []string{"--state", "approve", "11"}, "/pulls/11/reviews", ""},
		{"numeric flag value", []string{"--body", "11", "12", "--state", "approve"}, "/pulls/12/reviews", "/pulls/11/"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := &indexFixture{}
			ts := httptest.NewServer(http.HandlerFunc(f.serve))
			defer ts.Close()
			if err := (reviewSubmitCmd{}).Run(tc.args, testCtx(ts)); err != nil {
				t.Fatal(err)
			}
			wantPaths(t, f, tc.want, tc.notWant)
		})
	}
}

func TestIndexPlacementResolveAll(t *testing.T) {
	// The resolve-all fixture's fake server only knows pull 7, so a mis-scan
	// of the numeric --review value 11 as the index would fail the run.
	cases := []struct {
		name string
		args []string
	}{
		{"index first", []string{"7"}},
		{"flags first", []string{"--yes", "7"}},
		{"numeric flag value", []string{"--review", "11", "7", "--yes"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fx := newResolveAllFixture(t)
			ts := fx.server()
			defer ts.Close()
			if err := (resolveAllCmd{}).Run(tc.args, testCtx(ts)); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestIndexPlacementEdit(t *testing.T) {
	cases := []struct {
		name    string
		kind    string
		args    []string
		want    string
		notWant string
	}{
		{"index first", "pr", []string{"11", "--title", "t"}, "/pulls/11", ""},
		{"flags first", "issue", []string{"--body", "b", "11"}, "/issues/11", ""},
		{"numeric flag value", "pr", []string{"--title", "42", "--body", "x", "11"}, "/pulls/11", "/pulls/42"},
	}
	for _, tc := range cases {
		t.Run(tc.name+" "+tc.kind, func(t *testing.T) {
			var gotPath string
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotPath = r.URL.Path
				fmt.Fprint(w, `{"number":11,"title":"t"}`)
			}))
			defer ts.Close()
			if err := (editCmd{kind: tc.kind}).Run(tc.args, testCtx(ts)); err != nil {
				t.Fatal(err)
			}
			if !strings.HasSuffix(gotPath, tc.want) {
				t.Errorf("request path = %q, want suffix %q", gotPath, tc.want)
			}
			if tc.notWant != "" && strings.Contains(gotPath, tc.notWant) {
				t.Errorf("request path = %q, flag value 42 was mistaken for the index", gotPath)
			}
		})
	}
}

func TestIndexPlacementIssueLabel(t *testing.T) {
	cases := []struct {
		name    string
		args    []string
		want    string
		notWant string
	}{
		{"index first", []string{"7", "--label", "bug"}, "/issues/7/labels", ""},
		{"flags first", []string{"--label", "bug", "7"}, "/issues/7/labels", ""},
		{"numeric flag value", []string{"--label", "5", "7"}, "/issues/7/labels", "/issues/5"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var gotPath string
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.Method + " " + r.URL.Path {
				case "GET /api/v1/repos/o/r/labels":
					json.NewEncoder(w).Encode([]api.Label{{ID: 5, Name: "5"}, {ID: 6, Name: "bug"}})
					return
				default:
					gotPath = r.URL.Path
					json.NewEncoder(w).Encode([]api.Label{{ID: 5}})
					return
				}
			}))
			defer ts.Close()
			if err := (issueLabelCmd{adding: true}).Run(tc.args, testCtx(ts)); err != nil {
				t.Fatal(err)
			}
			wantPaths(t, &indexFixture{paths: []string{gotPath}}, tc.want, tc.notWant)
		})
	}
}
