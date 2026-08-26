package main

import (
	"errors"
	"fmt"

	"forge/internal/api"
	"forge/internal/cli"
)

// repoProbes is the subset of *api.Client the staged diagnosis needs. The
// interface exists so tests can drive every failure branch with httptest
// servers or fakes.
type repoProbes interface {
	ServerInfo() error
	CurrentUser() (*api.User, error)
	OwnerExists(o string) (bool, error)
	RepoExists(o, r string) (bool, *api.APIError, error)
}

// diagnoseRepo pinpoints which layer of host/token/owner/repo is broken,
// stopping at the first failing layer and naming the concrete resolved
// values in every message. nil means everything checked out.
//
// Stage order: host answers (version) -> token valid (/user) -> owner exists
// (/users/{o}) -> repo exists (/repos/{o}/{r}). A 404 on the repo check is
// disambiguated by re-probing the owner.
func diagnoseRepo(p repoProbes, host, owner, repoName string) error {
	// Stage 1: is the host a Forgejo/Gitea server at all?
	if err := p.ServerInfo(); err != nil {
		var apiErr *api.APIError
		switch {
		case api.IsNetwork(err):
			return &cli.Error{
				Code: cli.ExitNetwork,
				Msg:  fmt.Sprintf("cannot reach %s", host),
				Hint: fmt.Sprintf("check the host name and your network; tried https://%s/api/v1/version (%v)", host, err),
			}
		case errors.As(err, &apiErr):
			return &cli.Error{
				Code: cli.ExitContext,
				Msg:  fmt.Sprintf("%s answered but does not look like a Forgejo/Gitea server", host),
				Hint: fmt.Sprintf("GET /api/v1/version returned %s; check --host / [defaults] host — expected an API at https://%s/api/v1", apiErr, host),
			}
		default:
			return &cli.Error{Code: cli.ExitRuntime, Msg: err.Error()}
		}
	}

	// Stage 2: does the token authenticate?
	user, err := p.CurrentUser()
	if err != nil {
		var apiErr *api.APIError
		switch {
		case errors.As(err, &apiErr) && (apiErr.Status == 401 || apiErr.Status == 403):
			return &cli.Error{
				Code: cli.ExitAuth,
				Msg:  fmt.Sprintf("token rejected by %s: %s", host, apiErr),
				Hint: "the stored token is missing or lacks read scopes (read:user, read:repository, read:issue); pass --token or update your git credential helper entry",
			}
		case errors.As(err, &apiErr):
			return &cli.Error{Code: cli.ExitRuntime, Msg: fmt.Sprintf("%s failed the token check: %s", host, apiErr)}
		case api.IsNetwork(err):
			return &cli.Error{
				Code: cli.ExitNetwork,
				Msg:  fmt.Sprintf("connection to %s dropped while verifying token", host),
				Hint: err.Error(),
			}
		default:
			return &cli.Error{Code: cli.ExitRuntime, Msg: fmt.Sprintf("%s failed the token check: %v", host, err)}
		}
	}
	login := ""
	if user != nil {
		login = user.Login
	}

	// Stage 3+4: does the repo exist? A repo 404 is disambiguated against
	// the owner so we can say WHICH name is wrong.
	exists, notFound, err := p.RepoExists(owner, repoName)
	if err != nil {
		return mapWiredErr(err)
	}
	if exists {
		return nil
	}
	ownerOK, oerr := p.OwnerExists(owner)
	if oerr != nil {
		return mapWiredErr(oerr)
	}
	if !ownerOK {
		return &cli.Error{
			Code: cli.ExitContext,
			Msg:  fmt.Sprintf("owner %q not found on %s", owner, host),
			Hint: fmt.Sprintf("GET /users/%s returned 404 (authenticated as %s); is the owner an organisation your token cannot see, or is FORGE_OWNER / [defaults] owner wrong?", owner, login),
		}
	}
	return &cli.Error{
		Code: cli.ExitContext,
		Msg:  fmt.Sprintf("repository %q not found on %s", owner+"/"+repoName, host),
		Hint: fmt.Sprintf("owner %s exists (verified via /users/%s) but this repo 404'd (authenticated as %s); server said: %q. Either the repo name is wrong, it is private and the token lacks read:repository scope, or FORGE_REPO / [defaults] repo is wrong.", owner, owner, login, notFoundMessage(notFound)),
	}
}

// notFoundMessage extracts the server's own text from a 404 APIError, with a
// fallback when the body carried none.
func notFoundMessage(e *api.APIError) string {
	if e != nil && e.Message != "" {
		return e.Message
	}
	return "(no message in response body)"
}
