package api

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestBranchExists(t *testing.T) {
	const path = "/api/v1/repos/o/r/branches"

	t.Run("200 means branch exists via query param", func(t *testing.T) {
		var gotPath, gotBranch string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotPath = r.URL.Path
			gotBranch = r.URL.Query().Get("branch")
			w.Write([]byte(`{}`))
		}))
		defer srv.Close()

		c := NewClient(srv.URL, "tok", 0, nil)
		ok, err := c.BranchExists("o", "r", "main")
		if err != nil {
			t.Fatalf("BranchExists: %v", err)
		}
		if !ok {
			t.Error("ok = false, want true")
		}
		if gotPath != path {
			t.Errorf("path = %q, want %q", gotPath, path)
		}
		if gotBranch != "main" {
			t.Errorf("branch query param = %q, want %q (query form must be used)", gotBranch, "main")
		}
	})

	t.Run("404 means branch does not exist", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != path {
				t.Errorf("path = %q, want %q", r.URL.Path, path)
			}
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte(`{"message":"The target couldn't be found."}`))
		}))
		defer srv.Close()

		c := NewClient(srv.URL, "tok", 0, nil)
		ok, err := c.BranchExists("o", "r", "main")
		if err != nil {
			t.Fatalf("BranchExists: %v", err)
		}
		if ok {
			t.Error("ok = true, want false on 404")
		}
	})

	t.Run("403 is an error, not a missing branch", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != path {
				t.Errorf("path = %q, want %q", r.URL.Path, path)
			}
			w.WriteHeader(http.StatusForbidden)
		}))
		defer srv.Close()

		c := NewClient(srv.URL, "tok", 0, nil)
		ok, err := c.BranchExists("o", "r", "main")
		if err == nil {
			t.Fatal("BranchExists: err = nil, want non-nil on 403")
		}
		if ok {
			t.Error("ok = true, want false when error returned")
		}
		var apiErr *APIError
		if !errors.As(err, &apiErr) || apiErr.Status != http.StatusForbidden {
			t.Errorf("err = %v, want wrapped *APIError with Status 403", err)
		}
	})
}
