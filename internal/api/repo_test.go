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
	ok, notFound, err := c.RepoExists("o", "r")
	if err != nil || !ok || notFound != nil {
		t.Fatalf("ok=%v notFound=%v err=%v, want true/nil/nil", ok, notFound, err)
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

	ok, notFound, err := NewClient(srv.URL, "tok", 0, nil).RepoExists("o", "missing")
	if err != nil {
		t.Fatalf("404 must not be an error: %v", err)
	}
	if ok {
		t.Fatal("404 must report false")
	}
	if notFound == nil || notFound.Status != 404 || notFound.Message != "Not Found" {
		t.Fatalf("notFound = %v, want APIError{404, Not Found}", notFound)
	}
}

// A rejected token or server error must NOT be mistaken for a missing repo.
func TestRepoExistsOtherStatusesAreErrors(t *testing.T) {
	for _, status := range []int{401, 403, 500} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(status)
			w.Write([]byte(`{"message":"boom"}`))
		}))
		ok, notFound, err := NewClient(srv.URL, "bad", 0, nil).RepoExists("o", "r")
		srv.Close()
		if ok || notFound != nil {
			t.Fatalf("%d: ok=%v notFound=%v, want false/nil", status, ok, notFound)
		}
		var apiErr *APIError
		if !errors.As(err, &apiErr) || apiErr.Status != status {
			t.Fatalf("%d: err = %v, want APIError{%d}", status, err, status)
		}
	}
}

func TestGetRepository(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.WriteHeader(200)
		w.Write([]byte(`{"id":7,"name":"r","full_name":"o/r","default_branch":"trunk","owner":{"id":3,"login":"o"}}`))
	}))
	defer srv.Close()

	repo, err := NewClient(srv.URL, "tok", 0, nil).GetRepository("o", "r")
	if err != nil {
		t.Fatalf("GetRepository: %v", err)
	}
	if want := "/api/v1/repos/o/r"; gotPath != want {
		t.Fatalf("path = %q, want %q", gotPath, want)
	}
	if repo.ID != 7 || repo.Name != "r" || repo.FullName != "o/r" || repo.DefaultBranch != "trunk" {
		t.Fatalf("repo = %+v", repo)
	}
	if repo.Owner.ID != 3 || repo.Owner.Login != "o" {
		t.Fatalf("owner = %+v", repo.Owner)
	}
}

func TestGetRepository404ReturnsAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
		w.Write([]byte(`{"message":"Not Found"}`))
	}))
	defer srv.Close()

	repo, err := NewClient(srv.URL, "tok", 0, nil).GetRepository("o", "missing")
	if repo != nil {
		t.Fatalf("repo = %+v, want nil on 404", repo)
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Status != 404 {
		t.Fatalf("err = %v, want APIError{404}", err)
	}
}
