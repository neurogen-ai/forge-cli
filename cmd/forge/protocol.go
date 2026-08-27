package main

import (
	"strings"
)

// splitScheme accepts "git.example.com", "http://git.example.com", or
// "https://git.example.com/" and returns the host without scheme or trailing
// slash, plus the embedded scheme ("" when none given). The first "/" after
// the scheme terminates nothing: only scheme and one trailing slash are
// stripped, matching the old normalizeHost exactly.
func splitScheme(raw string) (host, scheme string) {
	h := raw
	for _, p := range []string{"https://", "http://"} {
		if strings.HasPrefix(strings.ToLower(h), p) {
			scheme = p[:len(p)-3]
			h = h[len(p):]
			break
		}
	}
	return strings.TrimSuffix(h, "/"), scheme
}

// isLocalHost reports whether h is loopback-safe for plaintext traffic:
// "localhost", "*.localhost", "127.0.0.1", "::1", or "[::1]".
func isLocalHost(h string) bool {
	h = strings.ToLower(h)
	// Trim a numeric :port suffix on a plain host (at most one colon in the
	// string) or after a bracketed IPv6 literal like [::1]:3000.
	if i := strings.LastIndexByte(h, ':'); i >= 0 &&
		(strings.Count(h, ":") == 1 || strings.HasSuffix(h[:i], "]")) && isAllDigits(h[i+1:]) {
		h = h[:i]
	}
	switch h {
	case "localhost":
		return true
	case "127.0.0.1", "::1", "[::1]":
		return true
	}
	return len(h) > 10 && h[len(h)-10:] == ".localhost"
}

// isAllDigits reports whether s consists only of ASCII digits.
func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// resolveBaseURL implements the precedence chain: embedded scheme in the
// host value, then [api] protocol from config, then https as the default.
// warn is true when plaintext http would leave the machine for a remote host.
func resolveBaseURL(rawHost, configuredProtocol string) (baseURL string, warn bool) {
	host, embedded := splitScheme(rawHost)
	protocol := embedded
	if protocol == "" {
		protocol = configuredProtocol
	}
	if protocol == "" {
		protocol = "https"
	}
	warn = protocol == "http" && !isLocalHost(host)
	return protocol + "://" + host, warn
}
