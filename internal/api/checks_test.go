package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestListCommitStatusesDecode(t *testing.T) {
	var gotPath string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		if got := r.Header.Get("Authorization"); got != "token tok" {
			t.Errorf("Authorization = %q", got)
		}
		w.Write([]byte(`[
			{"status":"success","context":"ci/build","description":"build passed","target_url":"https://ci.test/b/1"},
			{"status":"pending","context":"ci/test"}
		]`))
	}))
	defer ts.Close()

	got, err := newTestClient(ts).ListCommitStatuses("o", "r", "abc123")
	if err != nil {
		t.Fatal(err)
	}
	if gotPath != "/api/v1/repos/o/r/commits/abc123/statuses" {
		t.Errorf("path = %q", gotPath)
	}
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	if got[0].State != "success" || got[0].Context != "ci/build" ||
		got[0].Description != "build passed" || got[0].TargetURL != "https://ci.test/b/1" {
		t.Errorf("status[0] = %+v", got[0])
	}
	if got[1].State != "pending" || got[1].Context != "ci/test" ||
		got[1].Description != "" || got[1].TargetURL != "" {
		t.Errorf("status[1] = %+v", got[1])
	}
}

func TestListCommitStatusesEscapesRef(t *testing.T) {
	var gotPath string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.EscapedPath()
		w.Write([]byte(`[]`))
	}))
	defer ts.Close()

	if _, err := newTestClient(ts).ListCommitStatuses("o", "r", "feature/x"); err != nil {
		t.Fatal(err)
	}
	if gotPath != "/api/v1/repos/o/r/commits/feature%2Fx/statuses" {
		t.Errorf("path = %q", gotPath)
	}
}

func TestListCommitStatusesError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
		w.Write([]byte(`{"message":"repo does not exist"}`))
	}))
	defer ts.Close()

	if _, err := newTestClient(ts).ListCommitStatuses("o", "r", "main"); err == nil {
		t.Fatal("want error")
	}
}
