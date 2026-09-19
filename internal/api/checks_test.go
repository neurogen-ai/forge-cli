package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestListCommitStatuses(t *testing.T) {
	var gotPath string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		json.NewEncoder(w).Encode([]CommitStatus{
			{State: "success", Context: "ci/build", Description: "done"},
		})
	}))
	defer ts.Close()

	got, err := newTestClient(ts).ListCommitStatuses("o", "r", "abc123")
	if err != nil {
		t.Fatal(err)
	}
	if gotPath != "/api/v1/repos/o/r/commits/abc123/statuses" {
		t.Errorf("path = %s", gotPath)
	}
	if len(got) != 1 || got[0].State != "success" || got[0].Context != "ci/build" {
		t.Errorf("statuses = %+v", got)
	}
}

func TestListRunsForRef(t *testing.T) {
	var gotPath string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		json.NewEncoder(w).Encode(map[string]any{
			"total_count": 2,
			"entries": []map[string]any{
				{"id": 7, "name": "build", "status": "success", "head_sha": "abc123", "head_branch": "main", "url": "/o/r/actions/runs/7"},
				{"id": 8, "name": "build", "status": "running", "head_sha": "def456", "head_branch": "other", "url": "/o/r/actions/runs/8"},
			},
		})
	}))
	defer ts.Close()

	runs, available, err := newTestClient(ts).ListRunsForRef("o", "r", "main")
	if err != nil {
		t.Fatal(err)
	}
	if gotPath != "/api/v1/repos/o/r/actions/tasks" {
		t.Errorf("path = %s", gotPath)
	}
	if !available {
		t.Fatal("expected available")
	}
	if len(runs) != 1 || runs[0].ID != 7 || runs[0].Status != "success" || runs[0].HeadSHA != "abc123" {
		t.Errorf("runs = %+v", runs)
	}
}

func TestListRunsForRefUnavailableIs404(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"message":"Not Found"}`, 404)
	}))
	defer ts.Close()

	runs, available, err := newTestClient(ts).ListRunsForRef("o", "r", "main")
	if err != nil {
		t.Fatalf("404 must be a typed outcome, not an error: %v", err)
	}
	if available {
		t.Error("expected available=false on 404")
	}
	if runs != nil {
		t.Errorf("runs = %+v, want nil", runs)
	}
}

func TestListRunsForRefServerErrorReturnsError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"message":"boom"}`, 500)
	}))
	defer ts.Close()

	_, available, err := newTestClient(ts).ListRunsForRef("o", "r", "main")
	if err == nil {
		t.Fatal("expected error on 500")
	}
	if available {
		t.Error("expected available=false on error")
	}
}
