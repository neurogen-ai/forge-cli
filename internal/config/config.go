package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Defaults mirrors the [defaults] section; "" means unset.
type Defaults struct{ Host, Owner, Repo, Base string }

// Config is the merged two-layer configuration (PRD §7).
type Config struct {
	Defaults Defaults
	Savedirs map[string]string // key: "pr-conversation", "issue"
	// Token comes from [auth] token (repo-local first, then global; PRD §5
	// chain positions 3 and 4). "" when absent.
	Token string
	// TimeoutSeconds defaults to 30 when absent from both layers.
	TimeoutSeconds int
	// Protocol is [api] protocol: "http" or "https". "" means https (the
	// default). An embedded scheme in --host overrides it.
	Protocol string
}

// Default savedirs. Since v0.2.0 the defaults resolve to the XDG state
// directory (see DefaultStateDir), overriding PRD §7's repo-local .forge/*
// locations; .forge/* stays supported purely through [savedir] config
// entries. These constants remain as fallbacks for the no-home case, so
// callers never need to know the values either way.
const (
	defaultSavedirPR    = ".forge/prs"
	defaultSavedirIssue = ".forge/issues"
)

// Load reads the global and repo-local config files and merges them,
// local winning per-key. Missing files are not errors: both layers are
// optional and an empty path is skipped entirely.
//
// When expandHome is true, values starting with "~/" have the prefix
// replaced by the user's home directory. Relative savedir values stay
// relative here; resolving them against the repo root happens later in
// internal/cmds/cache.go.
func Load(globalPath, localPath string, expandHome bool) (*Config, error) {
	cfg := &Config{Savedirs: map[string]string{}}
	if state := DefaultStateDir(); state != "" {
		cfg.Savedirs["pr-conversation"] = filepath.Join(state, "prs")
		cfg.Savedirs["issue"] = filepath.Join(state, "issues")
	} else {
		cfg.Savedirs["pr-conversation"] = defaultSavedirPR
		cfg.Savedirs["issue"] = defaultSavedirIssue
	}
	cfg.TimeoutSeconds = 30
	for _, path := range []string{globalPath, localPath} {
		if path == "" {
			continue
		}
		src, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("config: %s: %w", path, err)
		}
		sections, err := ParseTOML(src)
		if err != nil {
			return nil, fmt.Errorf("config: %s: %w", path, err)
		}
		if err := merge(cfg, sections, expandHome); err != nil {
			return nil, fmt.Errorf("config: %s: %w", path, err)
		}
	}
	return cfg, nil
}

// merge applies one parsed layer onto cfg. Later calls win per-key.
func merge(cfg *Config, sections map[string]map[string]string, expandHome bool) error {
	for key, raw := range sections["defaults"] {
		v, err := expand(raw, expandHome)
		if err != nil {
			return err
		}
		switch key {
		case "host":
			cfg.Defaults.Host = v
		case "owner":
			cfg.Defaults.Owner = v
		case "repo":
			cfg.Defaults.Repo = v
		case "base":
			cfg.Defaults.Base = v
		}
	}
	for key, raw := range sections["savedir"] {
		v, err := expand(raw, expandHome)
		if err != nil {
			return err
		}
		cfg.Savedirs[key] = v
	}
	if raw, ok := sections["auth"]["token"]; ok {
		v, err := expand(raw, expandHome)
		if err != nil {
			return err
		}
		cfg.Token = v
	}
	if raw, ok := sections["api"]["protocol"]; ok {
		switch raw {
		case "", "http", "https":
			cfg.Protocol = raw
		default:
			return fmt.Errorf("api.protocol: %q must be \"http\" or \"https\"", raw)
		}
	}
	if raw, ok := sections["api"]["timeout_seconds"]; ok {
		n, err := strconv.Atoi(raw)
		if err != nil {
			return fmt.Errorf("api.timeout_seconds: %q is not an integer", raw)
		}
		cfg.TimeoutSeconds = n
	}
	return nil
}

// expand replaces a leading "~/" with the home directory when asked to.
func expand(v string, expandHome bool) (string, error) {
	if !expandHome || !strings.HasPrefix(v, "~/") {
		return v, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot expand %q: %w", v, err)
	}
	return filepath.Join(home, v[2:]), nil
}

// DefaultGlobalPath returns the global config path:
// $XDG_CONFIG_HOME/forge/config.toml when XDG_CONFIG_HOME is set and
// non-empty, else ~/.config/forge/config.toml. Returns "" when no home
// directory can be determined.
func DefaultGlobalPath() string {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "forge", "config.toml")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "forge", "config.toml")
}

// DefaultStateDir returns forge's state directory (savedir cache home):
// $XDG_STATE_HOME/forge when XDG_STATE_HOME is set and non-empty, else
// ~/.local/state/forge. Returns "" when no home directory can be determined.
func DefaultStateDir() string {
	if xdg := os.Getenv("XDG_STATE_HOME"); xdg != "" {
		return filepath.Join(xdg, "forge")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".local", "state", "forge")
}

// LocalPath returns the repo-local config path <repoRoot>/.forge/config.toml.
func LocalPath(repoRoot string) string {
	return filepath.Join(repoRoot, ".forge", "config.toml")
}
