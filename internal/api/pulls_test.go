package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// newTestClient returns a client pointed at ts and the requests it saw.
func newTestClient(ts *httptest.Server) *Client {
	return NewClient(ts.URL, "tok", 0, nil)
}

func TestCreatePullRequestPathAndBody(t *testing.T) {
	var gotPath, gotMethod string
	var gotBody map[string]any
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		if got := r.Header.Get("Authorization"); got != "token tok" {
			t.Errorf("Authorization = %q", got)
		}
		json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(201)
		json.NewEncoder(w).Encode(PullRequest{Number: 7, Title: "t"})
	}))
	defer ts.Close()

	pr, err := newTestClient(ts).CreatePullRequest("o", "r", CreatePRInput{Title: "t", Head: "h"})
	if err != nil {
		t.Fatal(err)
	}
	if gotMethod != "POST" || gotPath != "/api/v1/repos/o/r/pulls" {
		t.Errorf("got %s %s", gotMethod, gotPath)
	}
	if pr.Number != 7 {
		t.Errorf("pr.Number = %d", pr.Number)
	}
	if gotBody["base"] != nil || gotBody["body"] != nil {
		t.Errorf("empty base/body must be omitted, got %v", gotBody)
	}
	if gotBody["head"] != "h" || gotBody["title"] != "t" {
		t.Errorf("body = %v", gotBody)
	}
}

func TestGetPullRequest(t *testing.T) {
	var gotPath string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		json.NewEncoder(w).Encode(PullRequest{Number: 3})
	}))
	defer ts.Close()

	pr, err := newTestClient(ts).GetPullRequest("o", "r", 3)
	if err != nil {
		t.Fatal(err)
	}
	if gotPath != "/api/v1/repos/o/r/pulls/3" {
		t.Errorf("path = %q", gotPath)
	}
	if pr == nil || pr.Number != 3 {
		t.Errorf("pr = %+v", pr)
	}
}

func TestListPullRequestsSendsStateAndParsesArray(t *testing.T) {
	var gotQuery string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		json.NewEncoder(w).Encode([]PullRequest{{Number: 1}, {Number: 2}})
	}))
	defer ts.Close()

	prs, err := newTestClient(ts).ListPullRequests("o", "r", "open", 2, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(prs) != 2 {
		t.Fatalf("len = %d", len(prs))
	}
	if gotQuery != "limit=10&page=2&state=open" {
		t.Errorf("query = %q", gotQuery)
	}
}

func TestListPullRequestsEmptyStateOmitsParam(t *testing.T) {
	var gotQuery string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		json.NewEncoder(w).Encode([]PullRequest{})
	}))
	defer ts.Close()

	if _, err := newTestClient(ts).ListPullRequests("o", "r", "", 1, 20); err != nil {
		t.Fatal(err)
	}
	if gotQuery != "limit=20&page=1" {
		t.Errorf("query = %q (state should be omitted)", gotQuery)
	}
}

func TestGetReviewsAndComments(t *testing.T) {
	var paths []string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		switch r.URL.Path {
		case "/api/v1/repos/o/r/pulls/5/reviews":
			json.NewEncoder(w).Encode([]Review{{ID: 11}, {ID: 12}})
		case "/api/v1/repos/o/r/pulls/5/reviews/11/comments":
			w.Write([]byte(`[{"id":91,"pull_request_review_id":11,"commit_id":"sha1"}]`))
		default:
			t.Errorf("unexpected path %q", r.URL.Path)
		}
	}))
	defer ts.Close()
	c := newTestClient(ts)

	reviews, err := c.GetReviews("o", "r", 5)
	if err != nil || len(reviews) != 2 {
		t.Fatalf("reviews: %v %d", err, len(reviews))
	}
	rcs, err := c.GetReviewComments("o", "r", 5, 11)
	if err != nil || len(rcs) != 1 || rcs[0].ReviewID != 11 || rcs[0].CommitID != "sha1" {
		t.Fatalf("review comments: %v %+v", err, rcs)
	}
}

func TestReviewCommentDecodesAnchorsAndResolution(t *testing.T) {
	cases := []string{
		`{"id":9,"commit_id":"abc123","original_commit_id":"def456","position":12,"original_position":8,"line":15,"tree_path":"x/y.go","resolved":true}`,
		`{"id":9,"resolver":{"id":2,"login":"ada"}}`,
		`{"id":9,"resolved":false}`,
		`{"id":9}`,
	}
	want := []bool{true, true, false, false}
	for i, body := range cases {
		var rc ReviewComment
		if err := json.Unmarshal([]byte(body), &rc); err != nil {
			t.Fatal(err)
		}
		if rc.IsResolved() != want[i] {
			t.Errorf("case %d: IsResolved=%v want %v (%+v)", i, rc.IsResolved(), want[i], rc)
		}
	}
}

func TestAPIErrorFromServerMessage(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
		w.Write([]byte(`{"message":"page does not exist"}`))
	}))
	defer ts.Close()

	_, err := newTestClient(ts).GetPullRequest("o", "r", 99)
	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("want *APIError, got %T: %v", err, err)
	}
	if apiErr.Status != 404 || apiErr.Message != "page does not exist" {
		t.Errorf("apiErr = %+v", apiErr)
	}
}
