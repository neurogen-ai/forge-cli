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
// disambiguated by re-probing the owner (inside checkRepo).
func diagnoseRepo(p repoProbes, host, owner, repoName string) error {
	if err := checkHost(p, host); err != nil {
		return err
	}
	login, err := checkToken(p, host)
	if err != nil {
		return err
	}
	if err := checkRepo(p, host, owner, repoName, login); err != nil {
		return err
	}
	return nil
}

// diagnoseClosure returns the Ctx.Diagnose implementation for an API command:
// run the four-stage probes; on a pinpointed layer return that typed error;
// when every stage passes return the explicit "failed to diagnose" outcome
// rather than silence.
func diagnoseClosure(p repoProbes, host, owner, repoName string) func() *cli.Error {
	return func() *cli.Error {
		if err := diagnoseRepo(p, host, owner, repoName); err != nil {
			return asCLI(err)
		}
		return &cli.Error{
			Code: cli.ExitRuntime,
			Msg:  "failed to diagnose",
			Hint: "host, token, owner, and repository all verify OK; the reported failure is specific to the request itself",
		}
	}
}

// checkHost is stage 1: is the host a Forgejo/Gitea server at all? A nil
// return means GET /api/v1/version answered 2xx.
func checkHost(p repoProbes, host string) *cli.Error {
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
	return nil
}

// checkToken is stage 2: does the token authenticate? On success it returns
// the authenticated user's login so later stages can quote it in hints.
func checkToken(p repoProbes, host string) (string, *cli.Error) {
	user, err := p.CurrentUser()
	if err != nil {
		var apiErr *api.APIError
		switch {
		case errors.As(err, &apiErr) && (apiErr.Status == 401 || apiErr.Status == 403):
			return "", &cli.Error{
				Code: cli.ExitAuth,
				Msg:  fmt.Sprintf("token rejected by %s: %s", host, apiErr),
				Hint: "the stored token is missing or lacks read scopes (read:user, read:repository, read:issue); pass --token or update your git credential helper entry",
			}
		case errors.As(err, &apiErr):
			return "", &cli.Error{Code: cli.ExitRuntime, Msg: fmt.Sprintf("%s failed the token check: %s", host, apiErr)}
		case api.IsNetwork(err):
			return "", &cli.Error{
				Code: cli.ExitNetwork,
				Msg:  fmt.Sprintf("connection to %s dropped while verifying token", host),
				Hint: err.Error(),
			}
		default:
			return "", &cli.Error{Code: cli.ExitRuntime, Msg: fmt.Sprintf("%s failed the token check: %v", host, err)}
		}
	}
	login := ""
	if user != nil {
		login = user.Login
	}
	return login, nil
}

// ownerNotFoundError builds the stage-3 failure: the owner does not resolve
// on this host. Shared by checkOwner and checkRepo's disambiguation path.
func ownerNotFoundError(host, owner, login string) *cli.Error {
	return &cli.Error{
		Code: cli.ExitContext,
		Msg:  fmt.Sprintf("owner %q not found on %s", owner, host),
		Hint: fmt.Sprintf("GET /users/%s returned 404 (authenticated as %s); is the owner an organisation your token cannot see, or is FORGE_OWNER / [defaults] owner wrong?", owner, login),
	}
}

// checkOwner is stage 3: does GET /users/{o} resolve? Used by checkRepo to
// disambiguate a repo 404. Only a 404 counts as "missing"; auth failures and other statuses are errors so a
// scope problem is never mistaken for a missing owner.
func checkOwner(p repoProbes, host, owner, login string) *cli.Error {
	ok, err := p.OwnerExists(owner)
	if err != nil {
		return asCLI(mapWiredErr(err))
	}
	if !ok {
		return ownerNotFoundError(host, owner, login)
	}
	return nil
}

// checkRepo covers stages 3+4: does /repos/{o}/{r} exist? A repo 404 is
// disambiguated by re-probing the owner so we can say WHICH name is wrong:
// unknown owner -> owner error; known owner -> repository error.
func checkRepo(p repoProbes, host, owner, repoName, login string) *cli.Error {
	exists, notFound, err := p.RepoExists(owner, repoName)
	if err != nil {
		return asCLI(mapWiredErr(err))
	}
	if exists {
		return nil
	}
	if oerr := checkOwner(p, host, owner, login); oerr != nil {
		return oerr
	}
	return &cli.Error{
		Code: cli.ExitContext,
		Msg:  fmt.Sprintf("repository %q not found on %s", owner+"/"+repoName, host),
		Hint: fmt.Sprintf("owner %s exists (verified via /users/%s) but this repo 404'd (authenticated as %s); server said: %q. Either the repo name is wrong, it is private and the token lacks read:repository scope, or FORGE_REPO / [defaults] repo is wrong.", owner, owner, login, notFoundMessage(notFound)),
	}
}

// asCLI narrows the error returned by mapWiredErr to *cli.Error; mapWiredErr
// always produces one, so this only satisfies the type checker.
func asCLI(err error) *cli.Error {
	var e *cli.Error
	if errors.As(err, &e) {
		return e
	}
	return &cli.Error{Code: cli.ExitRuntime, Msg: err.Error()}
}

// notFoundMessage extracts the server's own text from a 404 APIError, with a
// fallback when the body carried none.
func notFoundMessage(e *api.APIError) string {
	if e != nil && e.Message != "" {
		return e.Message
	}
	return "(no message in response body)"
}
