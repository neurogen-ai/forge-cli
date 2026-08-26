package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLoadMergePrecedence(t *testing.T) {
	global := t.TempDir() + "/global.toml"
	local := t.TempDir() + "/local.toml"
	write(t, global, `
[defaults]
host = "global.example.com"
owner = "alice"
[savedir]
issue = ".forge/global-issues"
[api]
timeout_seconds = 45
`)
	write(t, local, `
[defaults]
host = "local.example.com"
base = "main"
`)

	cfg, err := Load(global, local, false)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	// Local wins per-key.
	if cfg.Defaults.Host != "local.example.com" {
		t.Errorf("Host = %q, want local.example.com", cfg.Defaults.Host)
	}
	// Keys set only in global fall through.
	if cfg.Defaults.Owner != "alice" {
		t.Errorf("Owner = %q, want alice", cfg.Defaults.Owner)
	}
	// Keys set only in local are picked up.
	if cfg.Defaults.Base != "main" {
		t.Errorf("Base = %q, want main", cfg.Defaults.Base)
	}
	if cfg.Savedirs["issue"] != ".forge/global-issues" {
		t.Errorf("Savedirs[issue] = %q, want .forge/global-issues", cfg.Savedirs["issue"])
	}
	if cfg.TimeoutSeconds != 45 {
		t.Errorf("TimeoutSeconds = %d, want 45", cfg.TimeoutSeconds)
	}
	// Repo unset anywhere stays "".
	if cfg.Defaults.Repo != "" {
		t.Errorf("Repo = %q, want empty", cfg.Defaults.Repo)
	}
}

func TestLoadMissingFiles(t *testing.T) {
	cfg, err := Load("/nonexistent/forge/global.toml", "/nonexistent/forge/local.toml", false)
	if err != nil {
		t.Fatalf("Load with missing files: %v", err)
	}
	if cfg.TimeoutSeconds != 30 {
		t.Errorf("TimeoutSeconds = %d, want default 30", cfg.TimeoutSeconds)
	}
	if cfg.Defaults.Host != "" || cfg.Defaults.Owner != "" || cfg.Defaults.Repo != "" || cfg.Defaults.Base != "" {
		t.Errorf("Defaults not zero: %+v", cfg.Defaults)
	}
	for key, want := range map[string]string{"pr-conversation": ".forge/prs", "issue": ".forge/issues"} {
		if got := cfg.Savedirs[key]; got != want {
			t.Errorf("Savedirs[%q] = %q, want default %q", key, got, want)
		}
	}
}

func TestLoadHomeExpansion(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")

	local := t.TempDir() + "/local.toml"
	write(t, local, "[savedir]\nissue = \"~/issues\"\npr-conversation = \"~/prs\"\n")

	cfg, err := Load("", local, true)
	if err != nil {
		t.Fatalf("Load expand: %v", err)
	}
	if got, want := cfg.Savedirs["issue"], filepath.Join(home, "issues"); got != want {
		t.Errorf("expanded issue = %q, want %q", got, want)
	}

	cfg, err = Load("", local, false)
	if err != nil {
		t.Fatalf("Load no-expand: %v", err)
	}
	if got := cfg.Savedirs["issue"]; got != "~/issues" {
		t.Errorf("unexpanded issue = %q, want ~/issues", got)
	}
}

func TestLoadInvalidTimeout(t *testing.T) {
	local := t.TempDir() + "/local.toml"
	write(t, local, "[api]\ntimeout_seconds = soon\n")
	if _, err := Load("", local, false); err == nil {
		t.Fatal("expected error for non-integer timeout_seconds")
	} else if !strings.Contains(err.Error(), "timeout_seconds") {
		t.Errorf("error %q should mention timeout_seconds", err.Error())
	}
}

func TestDefaultGlobalPath(t *testing.T) {
	t.Setenv("HOME", "/home/u")
	t.Setenv("XDG_CONFIG_HOME", "")
	if got, want := DefaultGlobalPath(), "/home/u/.config/forge/config.toml"; got != want {
		t.Errorf("DefaultGlobalPath (no XDG) = %q, want %q", got, want)
	}
	t.Setenv("XDG_CONFIG_HOME", "/xdg")
	if got, want := DefaultGlobalPath(), "/xdg/forge/config.toml"; got != want {
		t.Errorf("DefaultGlobalPath (XDG) = %q, want %q", got, want)
	}
}

func TestLocalPath(t *testing.T) {
	if got, want := LocalPath("/repo"), filepath.Join("/repo", ".forge", "config.toml"); got != want {
		t.Errorf("LocalPath = %q, want %q", got, want)
	}
}
