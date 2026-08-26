package api

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRepoExistsTrue(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.WriteHeader(200)
		w.Write([]byte(`{"full_name":"o/r"}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "tok", 0, nil)
	ok, err := c.RepoExists("o", "r")
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v, want true/nil", ok, err)
	}
	if want := "/api/v1/repos/o/r"; gotPath != want {
		t.Fatalf("path = %q, want %q", gotPath, want)
	}
}

func TestRepoExists404IsFalseNotError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
		w.Write([]byte(`{"message":"Not Found"}`))
	}))
	defer srv.Close()

	ok, err := NewClient(srv.URL, "tok", 0, nil).RepoExists("o", "missing")
	if err != nil {
		t.Fatalf("404 must not be an error: %v", err)
	}
	if ok {
		t.Fatal("404 must report false")
	}
}

// A rejected token or server error must NOT be mistaken for a missing repo.
func TestRepoExistsOtherStatusesAreErrors(t *testing.T) {
	for _, status := range []int{401, 403, 500} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(status)
			w.Write([]byte(`{"message":"boom"}`))
		}))
		ok, err := NewClient(srv.URL, "bad", 0, nil).RepoExists("o", "r")
		srv.Close()
		if ok {
			t.Fatalf("%d: ok=true, want false", status)
		}
		var apiErr *APIError
		if !errors.As(err, &apiErr) || apiErr.Status != status {
			t.Fatalf("%d: err = %v, want APIError{%d}", status, err, status)
		}
	}
}
