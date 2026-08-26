package gitctx

import (
	"fmt"
	"net/url"
	"strings"
)

// Remote holds the pieces forge identifies a repository by.
type Remote struct {
	Host  string
	Owner string
	Repo  string
}

// ParseRemoteURL accepts https, ssh:// (with optional user and port), and
// scp-like (git@host:owner/repo) remote URLs. It strips the .git suffix,
// user prefix, and port, and requires both an owner and a repo segment.
func ParseRemoteURL(raw string) (Remote, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return Remote{}, fmt.Errorf("gitctx: cannot parse remote URL %q", raw)
	}

	var host, path string

	switch {
	case strings.Contains(raw, "://"):
		u, err := url.Parse(raw)
		if err != nil {
			return Remote{}, fmt.Errorf("gitctx: cannot parse remote URL %q: %w", raw, err)
		}
		if u.Host == "" {
			return Remote{}, fmt.Errorf("gitctx: cannot parse remote URL %q: missing host", raw)
		}
		host = u.Hostname() // drops :port
		path = strings.TrimPrefix(u.Path, "/")
	case strings.Contains(raw, "@") && strings.Contains(strings.SplitN(raw, "/", 2)[0], ":"):
		// scp-like: git@host:owner/repo(.git); host part is everything before the
		// first colon that appears before any slash.
		hostPart, rest, _ := strings.Cut(raw, ":")
		if i := strings.LastIndex(hostPart, "@"); i >= 0 {
			hostPart = hostPart[i+1:]
		}
		host = hostPart
		path = strings.TrimPrefix(rest, "/")
	default:
		return Remote{}, fmt.Errorf("gitctx: cannot parse remote URL %q", raw)
	}

	segs := strings.Split(path, "/")
	// Filter empty segments so trailing or doubled slashes do not count.
	var parts []string
	for _, s := range segs {
		if s != "" {
			parts = append(parts, s)
		}
	}
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		return Remote{}, fmt.Errorf("gitctx: cannot parse remote URL %q: need owner and repo", raw)
	}

	repo := strings.TrimSuffix(parts[1], ".git")
	if repo == "" {
		return Remote{}, fmt.Errorf("gitctx: cannot parse remote URL %q: empty repository name", raw)
	}
	return Remote{Host: host, Owner: parts[0], Repo: repo}, nil
}
