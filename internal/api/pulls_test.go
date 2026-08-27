package api

import (
	"encoding/json"
	"io"
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

func TestSubmitReview(t *testing.T) {
	var gotMethod, gotPath, gotBody string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		if got := r.Header.Get("Authorization"); got != "token tok" {
			t.Errorf("Authorization = %q", got)
		}
		raw, _ := io.ReadAll(r.Body)
		gotBody = string(raw)
		w.WriteHeader(200)
		json.NewEncoder(w).Encode(Review{ID: 42, State: "APPROVED"})
	}))
	defer ts.Close()
	c := newTestClient(ts)

	// Body present: request bytes are exactly {"event":"APPROVED","body":"..."}.
	rev, err := c.SubmitReview("o", "r", 5, SubmitReviewInput{Event: "APPROVED", Body: "looks good"})
	if err != nil {
		t.Fatal(err)
	}
	if gotMethod != "POST" || gotPath != "/api/v1/repos/o/r/pulls/5/reviews" {
		t.Errorf("got %s %s", gotMethod, gotPath)
	}
	if gotBody != `{"event":"APPROVED","body":"looks good"}` {
		t.Errorf("body = %q", gotBody)
	}
	if rev.ID != 42 || rev.State != "APPROVED" {
		t.Errorf("review = %+v", rev)
	}

	// Empty Body is omitted from the request bytes.
	rev, err = c.SubmitReview("o", "r", 5, SubmitReviewInput{Event: "COMMENT"})
	if err != nil {
		t.Fatal(err)
	}
	if gotBody != `{"event":"COMMENT"}` {
		t.Errorf("empty-body request = %q", gotBody)
	}
	if rev.ID != 42 {
		t.Errorf("review.ID = %d", rev.ID)
	}
}

func TestSubmitReviewAPIError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(422)
		w.Write([]byte(`{"message":"event must be one of"}`))
	}))
	defer ts.Close()

	_, err := newTestClient(ts).SubmitReview("o", "r", 5, SubmitReviewInput{Event: "BOGUS"})
	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("want *APIError, got %T: %v", err, err)
	}
	if apiErr.Status != 422 || apiErr.Message != "event must be one of" {
		t.Errorf("apiErr = %+v", apiErr)
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

func TestGetReviewsFollowPagination(t *testing.T) {
	var srv *httptest.Server
	var visited []string
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Path; got != "/api/v1/repos/o/r/pulls/5/reviews" {
			t.Errorf("path = %q", got)
		}
		visited = append(visited, r.URL.Query().Encode())
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("page") == "" || r.URL.Query().Get("page") == "1" {
			w.Header().Set("Link", `<`+srv.URL+`/api/v1/repos/o/r/pulls/5/reviews?page=2>; rel="next"`)
			reviews := make([]Review, 30)
			for i := range reviews {
				reviews[i].ID = int64(i + 1)
			}
			json.NewEncoder(w).Encode(reviews)
			return
		}
		json.NewEncoder(w).Encode([]Review{{ID: 31}, {ID: 32}})
	}))
	defer srv.Close()

	got, err := newTestClient(srv).GetReviews("o", "r", 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 32 {
		t.Fatalf("len(reviews) = %d, want 32", len(got))
	}
	if got[0].ID != 1 || got[29].ID != 30 || got[30].ID != 31 || got[31].ID != 32 {
		t.Errorf("review IDs out of order across pages: first=%d last=%d", got[0].ID, got[31].ID)
	}
	if len(visited) != 2 {
		t.Fatalf("visited %d pages, want 2: %v", len(visited), visited)
	}
	first, second := visited[0], visited[1]
	if second != "page=2" {
		t.Errorf("second page query = %q, want page=2", second)
	}
	if first != "" && first != "page=1" {
		t.Errorf("first page query = %q, want empty or page=1", first)
	}
}

func TestGetReviewCommentsFollowPagination(t *testing.T) {
	var srv *httptest.Server
	var visited []string
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Path; got != "/api/v1/repos/o/r/pulls/5/reviews/11/comments" {
			t.Errorf("path = %q", got)
		}
		visited = append(visited, r.URL.Query().Encode())
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("page") == "" || r.URL.Query().Get("page") == "1" {
			w.Header().Set("Link", `<`+srv.URL+`/api/v1/repos/o/r/pulls/5/reviews/11/comments?page=2>; rel="next"`)
			json.NewEncoder(w).Encode([]ReviewComment{
				{ID: 91, ReviewID: 11}, {ID: 92, ReviewID: 11}, {ID: 93, ReviewID: 11},
			})
			return
		}
		json.NewEncoder(w).Encode([]ReviewComment{{ID: 94, ReviewID: 11}})
	}))
	defer srv.Close()

	got, err := newTestClient(srv).GetReviewComments("o", "r", 5, 11)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 4 || got[3].ID != 94 {
		t.Fatalf("comments = %+v, want 4 ending in ID 94", got)
	}
	for _, rc := range got {
		if rc.ReviewID != 11 {
			t.Errorf("comment %d has review id %d, want 11", rc.ID, rc.ReviewID)
		}
	}
	if len(visited) != 2 {
		t.Fatalf("visited %d pages, want 2: %v", len(visited), visited)
	}
	first, second := visited[0], visited[1]
	if second != "page=2" {
		t.Errorf("second page query = %q, want page=2", second)
	}
	if first != "" && first != "page=1" {
		t.Errorf("first page query = %q, want empty or page=1", first)
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

func TestThreadResolution(t *testing.T) {
	var gotMethod, gotPath, gotBody string
	nextStatus := 200
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		if got := r.Header.Get("Authorization"); got != "token tok" {
			t.Errorf("Authorization = %q", got)
		}
		raw, _ := io.ReadAll(r.Body)
		gotBody = string(raw)
		w.WriteHeader(nextStatus)
		if nextStatus != 200 {
			w.Write([]byte(`{"message":"Not Found"}`))
		}
	}))
	defer ts.Close()
	c := newTestClient(ts)

	// Resolve: PATCH with body exactly {"resolved":true}, 2xx -> nil error.
	nextStatus = 200
	if err := c.ResolveThread("o", "r", 88); err != nil {
		t.Fatalf("ResolveThread: %v", err)
	}
	if gotMethod != threadResolutionMethod || gotPath != "/api/v1/repos/o/r/pulls/comments/88/resolve" {
		t.Errorf("resolve got %s %s", gotMethod, gotPath)
	}
	if gotBody != `{"resolved":true}` {
		t.Errorf("resolve body = %q", gotBody)
	}

	// Unresolve: same endpoint, body exactly {"resolved":false}.
	nextStatus = 200
	if err := c.UnresolveThread("o", "r", 88); err != nil {
		t.Fatalf("UnresolveThread: %v", err)
	}
	if gotMethod != threadResolutionMethod || gotPath != "/api/v1/repos/o/r/pulls/comments/88/resolve" {
		t.Errorf("unresolve got %s %s", gotMethod, gotPath)
	}
	if gotBody != `{"resolved":false}` {
		t.Errorf("unresolve body = %q", gotBody)
	}

	// Error path: non-2xx surfaces as *APIError with status and server message.
	nextStatus = 404
	err := c.ResolveThread("o", "r", 99)
	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("want *APIError, got %T: %v", err, err)
	}
	if apiErr.Status != 404 || apiErr.Message != "Not Found" {
		t.Errorf("apiErr = %+v", apiErr)
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

func TestSetPRState(t *testing.T) {
	var gotMethod, gotPath, gotBody string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		raw, _ := io.ReadAll(r.Body)
		gotBody = string(raw)
		w.WriteHeader(200)
		json.NewEncoder(w).Encode(PullRequest{Number: 7, State: "closed"})
	}))
	defer ts.Close()
	c := newTestClient(ts)

	pr, err := c.SetPRState("o", "r", 7, "closed")
	if err != nil {
		t.Fatal(err)
	}
	if gotMethod != "PATCH" || gotPath != "/api/v1/repos/o/r/pulls/7" {
		t.Errorf("got %s %s", gotMethod, gotPath)
	}
	if gotBody != `{"state":"closed"}` {
		t.Errorf("body = %q", gotBody)
	}
	if pr.Number != 7 || pr.State != "closed" {
		t.Errorf("pr = %+v", pr)
	}

	// Reopen sends {"state":"open"} to the same endpoint.
	pr, err = c.SetPRState("o", "r", 7, "open")
	if err != nil {
		t.Fatal(err)
	}
	if gotBody != `{"state":"open"}` {
		t.Errorf("open body = %q", gotBody)
	}
	if pr.State != "closed" {
		t.Errorf("decoded State = %q", pr.State)
	}
}

func TestSetPRDraft(t *testing.T) {
	var gotMethod, gotPath, gotBody string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		raw, _ := io.ReadAll(r.Body)
		gotBody = string(raw)
		w.WriteHeader(200)
		json.NewEncoder(w).Encode(PullRequest{Number: 9, State: "open"})
	}))
	defer ts.Close()

	pr, err := newTestClient(ts).SetPRDraft("o", "r", 9)
	if err != nil {
		t.Fatal(err)
	}
	if gotMethod != "PATCH" || gotPath != "/api/v1/repos/o/r/pulls/9" {
		t.Errorf("got %s %s", gotMethod, gotPath)
	}
	if gotBody != `{"draft":false}` {
		t.Errorf("body = %q", gotBody)
	}
	if pr.Number != 9 || pr.State != "open" {
		t.Errorf("pr = %+v", pr)
	}
}

func TestSetPRDraftAPIError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(422)
		w.Write([]byte(`{"message":"draft flag is not supported"}`))
	}))
	defer ts.Close()

	_, err := newTestClient(ts).SetPRDraft("o", "r", 9)
	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("want *APIError, got %T: %v", err, err)
	}
	if apiErr.Status != 422 || apiErr.Message != "draft flag is not supported" {
		t.Errorf("apiErr = %+v", apiErr)
	}
}
