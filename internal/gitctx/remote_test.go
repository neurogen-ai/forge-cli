package gitctx

import (
	"strings"
	"testing"
)

func TestParseRemoteURL(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		want    Remote
		wantErr bool
	}{
		{
			name: "https with .git",
			raw:  "https://git.example.com/alice/proj.git",
			want: Remote{Host: "git.example.com", Owner: "alice", Repo: "proj"},
		},
		{
			name: "https without .git",
			raw:  "https://git.example.com/alice/proj",
			want: Remote{Host: "git.example.com", Owner: "alice", Repo: "proj"},
		},
		{
			name: "scp-like with .git",
			raw:  "git@git.example.com:alice/proj.git",
			want: Remote{Host: "git.example.com", Owner: "alice", Repo: "proj"},
		},
		{
			name: "scp-like without .git",
			raw:  "git@git.example.com:alice/proj",
			want: Remote{Host: "git.example.com", Owner: "alice", Repo: "proj"},
		},
		{
			name: "ssh with user and .git",
			raw:  "ssh://git@git.example.com/alice/proj.git",
			want: Remote{Host: "git.example.com", Owner: "alice", Repo: "proj"},
		},
		{
			name: "ssh without user and .git",
			raw:  "ssh://git.example.com/alice/proj.git",
			want: Remote{Host: "git.example.com", Owner: "alice", Repo: "proj"},
		},
		{
			name: "ssh with port and .git",
			raw:  "ssh://git@git.example.com:2222/alice/proj.git",
			want: Remote{Host: "git.example.com", Owner: "alice", Repo: "proj"},
		},
		{name: "empty string", raw: "", wantErr: true},
		{name: "single path segment", raw: "https://host/onlyone", wantErr: true},
		{name: "no host no path", raw: "not a url", wantErr: true},
		{name: "scheme only", raw: "https://", wantErr: true},
		{name: "dot git only repo", raw: "git@host:owner/.git", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseRemoteURL(tt.raw)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("ParseRemoteURL(%q) = %+v, want error", tt.raw, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseRemoteURL(%q) error: %v", tt.raw, err)
			}
			if got != tt.want {
				t.Errorf("ParseRemoteURL(%q) = %+v, want %+v", tt.raw, got, tt.want)
			}
		})
	}
}

func TestParseRemoteURLErrorsMentionInput(t *testing.T) {
	_, err := ParseRemoteURL("bogus")
	if err == nil || !strings.Contains(err.Error(), "bogus") {
		t.Errorf("error should mention the offending URL, got %v", err)
	}
}
