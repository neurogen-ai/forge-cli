package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCreateIssueBody(t *testing.T) {
	var gotPath string
	var gotBody map[string]any
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(201)
		json.NewEncoder(w).Encode(Issue{Number: 4})
	}))
	defer ts.Close()

	iss, err := newTestClient(ts).CreateIssue("o", "r", CreateIssueInput{Title: "t", Body: "b", Labels: []int{1, 2}})
	if err != nil {
		t.Fatal(err)
	}
	if gotPath != "/api/v1/repos/o/r/issues" {
		t.Errorf("path = %q", gotPath)
	}
	if iss.Number != 4 {
		t.Errorf("number = %d", iss.Number)
	}
	labels, ok := gotBody["labels"].([]any)
	if !ok || len(labels) != 2 || labels[0] != float64(1) {
		t.Errorf("labels = %v, want [1 2]", gotBody["labels"])
	}
}

func TestGetIssue(t *testing.T) {
	var gotPath string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		json.NewEncoder(w).Encode(Issue{Number: 9})
	}))
	defer ts.Close()

	iss, err := newTestClient(ts).GetIssue("o", "r", 9)
	if err != nil || iss.Number != 9 {
		t.Fatalf("%v %+v", err, iss)
	}
	if gotPath != "/api/v1/repos/o/r/issues/9" {
		t.Errorf("path = %q", gotPath)
	}
}

func TestListIssuesSendsTypeIssues(t *testing.T) {
	var gotQuery string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		json.NewEncoder(w).Encode([]Issue{{Number: 1}})
	}))
	defer ts.Close()

	if _, err := newTestClient(ts).ListIssues("o", "r", "open", 1, 5); err != nil {
		t.Fatal(err)
	}
	want := "limit=5&page=1&state=open&type=issues"
	if gotQuery != want {
		t.Errorf("query = %q, want %q", gotQuery, want)
	}
}

func TestListIssuesEmptyStateOmitsParam(t *testing.T) {
	var gotQuery string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		json.NewEncoder(w).Encode([]Issue{})
	}))
	defer ts.Close()

	if _, err := newTestClient(ts).ListIssues("o", "r", "", 1, 10); err != nil {
		t.Fatal(err)
	}
	if gotQuery != "limit=10&page=1&type=issues" {
		t.Errorf("query = %q (state should be omitted)", gotQuery)
	}
}

func TestGetIssueComments(t *testing.T) {
	var gotPath string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		json.NewEncoder(w).Encode([]Comment{{ID: 1}, {ID: 2}})
	}))
	defer ts.Close()

	cs, err := newTestClient(ts).GetIssueComments("o", "r", 6)
	if err != nil || len(cs) != 2 {
		t.Fatalf("%v %d", err, len(cs))
	}
	if gotPath != "/api/v1/repos/o/r/issues/6/comments" {
		t.Errorf("path = %q", gotPath)
	}
}

func TestListLabels(t *testing.T) {
	var gotPath string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		json.NewEncoder(w).Encode([]Label{{ID: 1, Name: "bug"}, {ID: 2, Name: "docs"}})
	}))
	defer ts.Close()

	ls, err := newTestClient(ts).ListLabels("o", "r")
	if err != nil || len(ls) != 2 || ls[1].Name != "docs" {
		t.Fatalf("%v %+v", err, ls)
	}
	if gotPath != "/api/v1/repos/o/r/labels" {
		t.Errorf("path = %q", gotPath)
	}
}

func TestSetIssueState(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody map[string]any
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		json.NewDecoder(r.Body).Decode(&gotBody)
		json.NewEncoder(w).Encode(Issue{Number: 7, State: "closed"})
	}))
	defer ts.Close()

	iss, err := newTestClient(ts).SetIssueState("o", "r", 7, "closed")
	if err != nil || iss.State != "closed" {
		t.Fatalf("%v %+v", err, iss)
	}
	if gotMethod != "PATCH" {
		t.Errorf("method = %q, want PATCH", gotMethod)
	}
	if gotPath != "/api/v1/repos/o/r/issues/7" {
		t.Errorf("path = %q", gotPath)
	}
	if gotBody["state"] != "closed" {
		t.Errorf("body state = %v, want closed", gotBody["state"])
	}

	iss, err = newTestClient(ts).SetIssueState("o", "r", 7, "open")
	if err != nil || gotBody["state"] != "open" {
		t.Fatalf("%v %v body=%v", err, iss, gotBody["state"])
	}
}
