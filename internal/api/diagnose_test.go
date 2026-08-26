package api

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestServerInfoOK(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		fmt.Fprint(w, `{"version":"1.22.0"}`)
	}))
	defer srv.Close()

	if err := NewClient(srv.URL, "tok", 0, nil).ServerInfo(); err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if want := "/api/v1/version"; gotPath != want {
		t.Fatalf("path = %q, want %q", gotPath, want)
	}
}

func TestServerInfoNotForgejo(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
		w.Write([]byte(`{"message":"nope"}`))
	}))
	defer srv.Close()

	err := NewClient(srv.URL, "tok", 0, nil).ServerInfo()
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Status != 404 {
		t.Fatalf("err = %v, want APIError{404}", err)
	}
}

func TestCurrentUser(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/user":
			if got := r.Header.Get("Authorization"); got != "token tok" {
				t.Errorf("Authorization = %q", got)
			}
			fmt.Fprint(w, `{"id":1,"login":"alice"}`)
		default:
			t.Errorf("unexpected path %q", r.URL.Path)
		}
	}))
	defer srv.Close()

	u, err := NewClient(srv.URL, "tok", 0, nil).CurrentUser()
	if err != nil || u.Login != "alice" {
		t.Fatalf("u=%+v err=%v", u, err)
	}
}

func TestCurrentUserRejected(t *testing.T) {
	for _, status := range []int{401, 403} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(status)
			w.Write([]byte(`{"message":"token does not have required scope"}`))
		}))
		u, err := NewClient(srv.URL, "bad", 0, nil).CurrentUser()
		srv.Close()
		if u != nil {
			t.Fatalf("%d: user non-nil", status)
		}
		var apiErr *APIError
		if !errors.As(err, &apiErr) || apiErr.Status != status || apiErr.Message == "" {
			t.Fatalf("%d: err = %v, want APIError{%d with message}", status, err, status)
		}
	}
}

func TestOwnerExists(t *testing.T) {
	cases := []struct {
		status int
		want   bool
		isErr  bool
	}{
		{200, true, false},
		{404, false, false},
		{403, false, true}, // scope problem must not read as "missing owner"
	}
	for _, tc := range cases {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if want := "/api/v1/users/o"; r.URL.Path != want {
				t.Errorf("path = %q, want %q", r.URL.Path, want)
			}
			w.WriteHeader(tc.status)
		}))
		ok, err := NewClient(srv.URL, "tok", 0, nil).OwnerExists("o")
		srv.Close()
		if ok != tc.want {
			t.Errorf("%d: ok = %v, want %v", tc.status, ok, tc.want)
		}
		if (err != nil) != tc.isErr {
			t.Errorf("%d: err = %v, isErr want %v", tc.status, err, tc.isErr)
		}
	}
}
