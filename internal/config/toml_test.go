package config

import (
	"strings"
	"testing"
)

func TestParseTOML(t *testing.T) {
	tests := []struct {
		name    string
		src     string
		want    map[string]map[string]string
		wantErr string // substring of expected error, "" if none
	}{
		{
			name: "sections keys ints quoted strings",
			src: `
[defaults]
host = "git.example.com"
base = "master"

[savedir]
issue = ".forge/issues"

[api]
timeout_seconds = 30
`,
			want: map[string]map[string]string{
				"defaults": {"host": "git.example.com", "base": "master"},
				"savedir":  {"issue": ".forge/issues"},
				"api":      {"timeout_seconds": "30"},
			},
		},
		{
			name: "comments and blank lines",
			src: `# leading comment

[defaults] # trailing on header
# another full-line comment
host = "h"   # trailing comment after value
`,
			want: map[string]map[string]string{
				"defaults": {"host": "h"},
			},
		},
		{
			name: "quoted string keeps spaces and hash",
			src: `[defaults]
title = "fix: x # not a comment"
body = "two  spaces inside"
`,
			want: map[string]map[string]string{
				"defaults": {"title": "fix: x # not a comment", "body": "two  spaces inside"},
			},
		},
		{
			name: "bare word value",
			src: `[savedir]
issue = relative/path
`,
			want: map[string]map[string]string{
				"savedir": {"issue": "relative/path"},
			},
		},
		{
			name: "bare word with mid-word apostrophe",
			src:  "[a]\nx = it's\n",
			want: map[string]map[string]string{
				"a": {"x": "it's"},
			},
		},
		{
			name:    "duplicate key in same section",
			src:     "[a]\nx = \"1\"\nx = \"2\"\n",
			wantErr: `config: line 3: duplicate key "x" in [a]`,
		},
		{
			name:    "key before any section",
			src:     "x = \"1\"\n",
			wantErr: `config: line 1: key "x" outside any [section]`,
		},
		{
			name:    "missing equals",
			src:     "[a]\njustakey\n",
			wantErr: "config: line 2: expected key = value",
		},
		{
			name:    "unterminated string",
			src:     "[a]\nx = \"oops\n",
			wantErr: "config: line 2: unterminated string",
		},
		{
			name: "single-quoted literal string",
			src:  "[defaults]\nowner = 'Neurogenesis Org'\n",
			want: map[string]map[string]string{
				"defaults": {"owner": "Neurogenesis Org"},
			},
		},
		{
			name: "apostrophe inside double quotes preserved",
			src:  "[a]\nx = \"it's fine\"\n",
			want: map[string]map[string]string{
				"a": {"x": "it's fine"},
			},
		},
		{
			name: "double quote inside single quotes preserved",
			src: `[a]
x = 'say "hi"'
`,
			want: map[string]map[string]string{
				"a": {"x": `say "hi"`},
			},
		},
		{
			name:    "unterminated single-quoted string",
			src:     "[a]\nx = 'oops\n",
			wantErr: "config: line 2: unterminated string",
		},
		{
			name:    "malformed section header",
			src:     "[a\nx = \"1\"\n",
			wantErr: "config: line 1: malformed section header",
		},
		{
			name: "reopened section merges",
			src:  "[a]\nx = \"1\"\n[a]\ny = \"2\"\n",
			want: map[string]map[string]string{
				"a": {"x": "1", "y": "2"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseTOML([]byte(tt.src))
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("ParseTOML() error = nil, want containing %q", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("ParseTOML() error = %q, want containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseTOML() error = %v", err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("ParseTOML() got %d sections, want %d: %v", len(got), len(tt.want), got)
			}
			for sec, kv := range tt.want {
				gotKV, ok := got[sec]
				if !ok {
					t.Fatalf("ParseTOML() missing section [%s]", sec)
				}
				for k, v := range kv {
					if gotKV[k] != v {
						t.Errorf("[%s] %s = %q, want %q", sec, k, gotKV[k], v)
					}
				}
			}
		})
	}
}
