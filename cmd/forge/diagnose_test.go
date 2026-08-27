package main

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"forge/internal/api"
	"forge/internal/cli"
)

// probeFake drives diagnoseRepo without any HTTP. Every field is a scripted
// outcome for the corresponding probe call.
type probeFake struct {
	serverErr   error
	user        *api.User
	userErr     error
	repoExists  bool
	repo404     *api.APIError
	repoErr     error
	ownerExists bool
	ownerErr    error

	serverCalled, userCalled, repoCalled, ownerCalled int
}

func (f *probeFake) ServerInfo() error               { f.serverCalled++; return f.serverErr }
func (f *probeFake) CurrentUser() (*api.User, error) { f.userCalled++; return f.user, f.userErr }
func (f *probeFake) RepoExists(o, r string) (bool, *api.APIError, error) {
	f.repoCalled++
	return f.repoExists, f.repo404, f.repoErr
}
func (f *probeFake) OwnerExists(o string) (bool, error) {
	f.ownerCalled++
	return f.ownerExists, f.ownerErr
}

func codeOf(t *testing.T, err error) int {
	t.Helper()
	var e *cli.Error
	if !errors.As(err, &e) {
		t.Fatalf("err = %v, want *cli.Error", err)
	}
	return e.Code
}

func wantCode(t *testing.T, err error, code int) {
	t.Helper()
	if err == nil {
		t.Fatalf("err = nil, want code %d", code)
	}
	if got := codeOf(t, err); got != code {
		t.Fatalf("code = %d (%v), want %d", got, err, code)
	}
}

const H, O, R = "git.example.com", "alice", "proj"

func TestDiagnoseHappyPath(t *testing.T) {
	f := &probeFake{user: &api.User{Login: O}, repoExists: true}
	if err := diagnoseRepo(f, H, O, R); err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	// Happy path must stop probing: no owner lookup needed.
	if f.ownerCalled != 0 || f.serverCalled != 1 || f.userCalled != 1 || f.repoCalled != 1 {
		t.Fatalf("probe calls server=%d user=%d repo=%d owner=%d", f.serverCalled, f.userCalled, f.repoCalled, f.ownerCalled)
	}
}

func TestDiagnoseHostUnreachable(t *testing.T) {
	// networkError is unexported, so drive the transport branch with a real
	// client pointed at a server that has been shut down.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	url := srv.URL
	srv.Close()
	c := api.NewClient(url, "tok", 2*time.Second, nil)
	host := strings.TrimPrefix(strings.TrimPrefix(url, "http://"), "https://")
	err := diagnoseRepo(c, host, O, R)
	wantCode(t, err, cli.ExitNetwork)
	if !strings.Contains(err.Error(), host) || !strings.Contains(err.Error(), "/api/v1/version") {
		t.Fatalf("message must name host and probed URL: %v", err)
	}
}

func TestDiagnoseHostNotForgejo(t *testing.T) {
	f := &probeFake{serverErr: &api.APIError{Status: 404, Message: "nope"}}
	err := diagnoseRepo(f, H, O, R)
	wantCode(t, err, cli.ExitContext)
	if !strings.Contains(err.Error(), "does not look like a Forgejo") || !strings.Contains(err.Error(), H) {
		t.Fatalf("message must say the host is not Forgejo and name it: %v", err)
	}
}

func TestDiagnoseTokenRejected(t *testing.T) {
	for _, status := range []int{401, 403} {
		f := &probeFake{userErr: &api.APIError{Status: status, Message: "token does not have required scope"}}
		err := diagnoseRepo(f, H, O, R)
		wantCode(t, err, cli.ExitAuth)
		if !strings.Contains(err.Error(), fmt.Sprint(status)) || !strings.Contains(err.Error(), "token does not have required scope") {
			t.Fatalf("%d: message must surface status and server text: %v", status, err)
		}
	}
}

func TestDiagnoseRepo404OwnerMissing(t *testing.T) {
	f := &probeFake{
		user:        &api.User{Login: O},
		repo404:     &api.APIError{Status: 404, Message: "Not Found"},
		ownerExists: false,
	}
	err := diagnoseRepo(f, H, O, R)
	wantCode(t, err, cli.ExitContext)
	if !strings.Contains(err.Error(), fmt.Sprintf("owner %q not found on %s", O, H)) {
		t.Fatalf("message must blame the owner specifically: %v", err)
	}
	if !strings.Contains(err.Error(), "authenticated as "+O) {
		t.Fatalf("message must name the authenticated user: %v", err)
	}
	if !strings.Contains(err.Error(), "/users/"+O) {
		t.Fatalf("hint must cite the owner probe: %v", err)
	}
}

func TestDiagnoseRepo404OwnerPresent(t *testing.T) {
	f := &probeFake{
		user:        &api.User{Login: O},
		repo404:     &api.APIError{Status: 404, Message: "Not Found"},
		ownerExists: true,
	}
	err := diagnoseRepo(f, H, O, R)
	wantCode(t, err, cli.ExitContext)
	if !strings.Contains(err.Error(), fmt.Sprintf("repository %q not found on %s", O+"/"+R, H)) {
		t.Fatalf("message must blame the repo specifically: %v", err)
	}
	if !strings.Contains(err.Error(), "server said:") {
		t.Fatalf("message must quote the server's 404 body: %v", err)
	}
}

func TestDiagnoseRepoOtherErrorMapped(t *testing.T) {
	// A scope failure during the repo probe must be ExitAuth, never a
	// "not found" style message.
	f := &probeFake{user: &api.User{Login: O}, repoErr: &api.APIError{Status: 403, Message: "scope"}}
	err := diagnoseRepo(f, H, O, R)
	wantCode(t, err, cli.ExitAuth)
	if strings.Contains(strings.ToLower(err.Error()), "not found") {
		t.Fatalf("auth failure must not read as missing repo: %v", err)
	}
}

// End-to-end through the real api.Client against httptest servers, proving
// diagnoseRepo composes correctly with actual probe implementations.
func TestDiagnoseAgainstRealClientMissingRepo(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/version":
			fmt.Fprint(w, `{"version":"1.22.0"}`)
		case "/api/v1/user":
			fmt.Fprint(w, `{"id":1,"login":"alice"}`)
		case "/api/v1/repos/alice/proj":
			w.WriteHeader(404)
			w.Write([]byte(`{"message":"Not Found"}`))
		case "/api/v1/users/alice":
			fmt.Fprint(w, `{"id":1,"login":"alice"}`)
		default:
			t.Errorf("unexpected path %q", r.URL.Path)
		}
	}))
	defer srv.Close()

	c := api.NewClient(srv.URL, "tok", 0, nil)
	err := diagnoseRepo(c, "localhost:"+portOf(srv), "alice", "proj")
	wantCode(t, err, cli.ExitContext)
	if !strings.Contains(err.Error(), `repository "alice/proj" not found`) {
		t.Fatalf("must blame the repo: %v", err)
	}
}

func portOf(srv *httptest.Server) string {
	u := srv.URL
	if i := strings.LastIndex(u, ":"); i >= 0 {
		return u[i+1:]
	}
	return u
}

func TestDiagnoseClosureAllClear(t *testing.T) {
	f := &probeFake{user: &api.User{Login: O}, repoExists: true}
	closure := diagnoseClosure(f, H, O, R)
	err := closure()
	if err == nil {
		t.Fatal("closure must not return nil when diagnosis is clear; expect explicit failed-to-diagnose")
	}
	var e *cli.Error
	if !errors.As(err, &e) || e.Msg != "failed to diagnose" {
		t.Fatalf("err = %v, want Msg \"failed to diagnose\"", err)
	}
	if e.Hint == "" {
		t.Fatal("Hint must be non-empty")
	}
}

func TestDiagnoseClosureSurfacesTokenFailure(t *testing.T) {
	f := &probeFake{userErr: &api.APIError{Status: 401, Message: "token rejected"}}
	err := diagnoseClosure(f, H, O, R)()
	wantCode(t, err, cli.ExitAuth)
	if strings.Contains(err.Error(), "failed to diagnose") {
		t.Fatalf("token-stage error must surface verbatim: %v", err)
	}
}

func TestDiagnoseClosureSurfacesRepoFailure(t *testing.T) {
	f := &probeFake{user: &api.User{Login: "bob"}, repoExists: false}
	err := diagnoseClosure(f, H, O, R)()
	if err == nil {
		t.Fatal("repository-not-found error must surface")
	}
	if strings.Contains(err.Error(), "failed to diagnose") {
		t.Fatalf("repo-stage error must surface verbatim: %v", err)
	}
}
