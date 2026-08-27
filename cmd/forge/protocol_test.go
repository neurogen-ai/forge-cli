package main

import "testing"

func TestSplitScheme(t *testing.T) {
	tests := []struct{ in, host, scheme string }{
		{"git.example.com", "git.example.com", ""},
		{"http://git.example.com", "git.example.com", "http"},
		{"https://git.example.com/", "git.example.com", "https"},
		{"HTTP://LOCALHOST", "LOCALHOST", "http"},
		{"127.0.0.1", "127.0.0.1", ""},
		{"git.example.com/base", "git.example.com/base", ""},
	}
	for _, tc := range tests {
		host, scheme := splitScheme(tc.in)
		if host != tc.host || scheme != tc.scheme {
			t.Errorf("splitScheme(%q) = (%q, %q), want (%q, %q)", tc.in, host, scheme, tc.host, tc.scheme)
		}
	}
}

func TestIsLocalHost(t *testing.T) {
	for _, h := range []string{"localhost", "foo.localhost", "LocalHost", "127.0.0.1", "::1", "[::1]"} {
		if !isLocalHost(h) {
			t.Errorf("isLocalHost(%q) = false, want true", h)
		}
	}
	for _, h := range []string{"git.example.com", "notlocalhost", ".localhost"} {
		if isLocalHost(h) {
			t.Errorf("isLocalHost(%q) = true, want false", h)
		}
	}
}

func TestResolveBaseURL(t *testing.T) {
	tests := []struct {
		rawHost, configured, baseURL string
		warn                         bool
	}{
		{"git.example.com", "", "https://git.example.com", false},
		{"git.example.com", "http", "http://git.example.com", true},
		{"http://git.example.com/", "", "http://git.example.com", true},
		{"http://localhost:3000", "", "http://localhost:3000", false},
		{"127.0.0.1", "http", "http://127.0.0.1", false},
	}
	for _, tc := range tests {
		url, warn := resolveBaseURL(tc.rawHost, tc.configured)
		if url != tc.baseURL || warn != tc.warn {
			t.Errorf("resolveBaseURL(%q, %q) = (%q, %v), want (%q, %v)", tc.rawHost, tc.configured, url, warn, tc.baseURL, tc.warn)
		}
	}
}
