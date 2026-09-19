package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSearchIssuesParamsAndDecode(t *testing.T) {
	var gotPath, gotQ, gotType, gotState string
	var calls int
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			gotPath = r.URL.Path
			gotQ = r.URL.Query().Get("q")
			gotType = r.URL.Query().Get("type")
			gotState = r.URL.Query().Get("state")
			w.Header().Set("Link", `<http://`+r.Host+`/api/v1/repos/o/r/issues/search?q=x&type=issues&page=2>; rel="next"`)
			json.NewEncoder(w).Encode([]Issue{{Number: 1, Title: "first"}})
			return
		}
		w.Header().Del("Link")
		json.NewEncoder(w).Encode([]Issue{{Number: 2, Title: "second"}})
	}))
	defer ts.Close()

	got, err := newTestClient(ts).SearchIssues("o", "r", "crash on save", "issues", "")
	if err != nil {
		t.Fatal(err)
	}
	if gotPath != "/api/v1/repos/o/r/issues/search" {
		t.Errorf("path = %s", gotPath)
	}
	if gotQ != "crash on save" || gotType != "issues" {
		t.Errorf("query q=%q type=%q", gotQ, gotType)
	}
	if gotState != "" {
		t.Errorf("state sent when empty: %q", gotState)
	}
	if len(got) != 2 || got[0].Number != 1 || got[1].Title != "second" {
		t.Errorf("results = %+v", got)
	}
}

func TestSearchIssuesStateSent(t *testing.T) {
	var gotState string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotState = r.URL.Query().Get("state")
		json.NewEncoder(w).Encode([]Issue{})
	}))
	defer ts.Close()

	if _, err := newTestClient(ts).SearchIssues("o", "r", "x", "pulls", "open"); err != nil {
		t.Fatal(err)
	}
	if gotState != "open" {
		t.Errorf("state = %q", gotState)
	}
}
