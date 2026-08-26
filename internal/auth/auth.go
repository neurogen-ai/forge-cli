// Package auth resolves the Forgejo API token for a host.
//
// It never reads config files; the caller passes any config-derived token in
// as explicitToken. Tokens are never logged.
package auth

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// ErrNoToken is returned when every token source comes up empty.
var ErrNoToken = errors.New("no token found")

// Resolve returns the first non-empty of, in order (PRD §5):
//
//	explicitToken > FORGE_TOKEN env > git credential fill
//
// The credential fill runs `git credential fill` with protocol=https and the
// given host on stdin and returns its password= line. Any fill failure or an
// empty password returns ErrNoToken.
func Resolve(host, explicitToken string) (string, error) {
	if explicitToken != "" {
		return explicitToken, nil
	}
	if t := os.Getenv("FORGE_TOKEN"); t != "" {
		return t, nil
	}
	return credentialFill(host)
}

// credentialFill consults git's configured credential helper for host.
func credentialFill(host string) (string, error) {
	cmd := exec.Command("git", "credential", "fill")
	// Never allow the helper to hang on an interactive prompt: fail instead.
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GCM_INTERACTIVE=never")

	var out, errOut bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errOut

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return "", fmt.Errorf("%w: stdin pipe: %v", ErrNoToken, err)
	}
	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("%w: start git credential: %v", ErrNoToken, err)
	}

	_, werr := fmt.Fprintf(stdin, "protocol=https\nhost=%s\n\n", host)
	cerr := stdin.Close() // close promptly so helpers see EOF and cannot block
	werr2 := cmd.Wait()

	if werr != nil || cerr != nil {
		return "", fmt.Errorf("%w: writing to git credential: %v", ErrNoToken, first(werr, cerr))
	}
	if werr2 != nil {
		msg := strings.TrimSpace(errOut.String())
		if msg == "" {
			msg = werr2.Error()
		}
		return "", fmt.Errorf("%w: git credential fill: %s", ErrNoToken, msg)
	}
	return passwordFrom(out.String())
}

// passwordFrom extracts the password= line from `git credential fill` output.
func passwordFrom(output string) (string, error) {
	for _, line := range strings.Split(output, "\n") {
		if v, ok := strings.CutPrefix(line, "password="); ok && v != "" {
			return v, nil
		}
	}
	return "", fmt.Errorf("%w: credential helper returned no password", ErrNoToken)
}

func first(errs ...error) error {
	for _, e := range errs {
		if e != nil {
			return e
		}
	}
	return nil
}
