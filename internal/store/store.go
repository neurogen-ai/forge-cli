// Package store is the only filesystem-writing code in forge: the Write
// family and the cache flush/path helpers.
package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Write stores data as pretty-printed JSON at <dir>/<repo>-<number>.json,
// replacing any previous copy in place. Pulled dumps are current snapshots,
// one file per item; there are no timestamped variants. Returns the path.
func Write(dir, repo string, number int, data []byte) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("store: create %s: %w", dir, err)
	}
	path := filepath.Join(dir, fmt.Sprintf("%s-%d.json", repo, number))
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return "", fmt.Errorf("store: write %s: %w", path, err)
	}
	return path, nil
}

// WriteJSON marshals v with two-space indent then delegates to Write.
func WriteJSON(dir, repo string, number int, v any) (string, error) {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return "", fmt.Errorf("store: encode %s-%d: %w", repo, number, err)
	}
	return Write(dir, repo, number, data)
}

// ResolveDirs expands ~ and makes relative savedir paths absolute against
// root, preserving map iteration order via sorted keys for determinism.
func ResolveDirs(savedirs map[string]string, root, home string) []string {
	out := make([]string, 0, len(savedirs))
	for _, key := range sortedKeys(savedirs) {
		dir := savedirs[key]
		switch {
		case strings.HasPrefix(dir, "~/"):
			dir = filepath.Join(home, dir[2:])
		case !filepath.IsAbs(dir):
			dir = filepath.Join(root, dir)
		}
		out = append(out, dir)
	}
	return out
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// Flush removes regular files directly inside each dir — never recursive,
// never directories themselves. A directory left empty is removed too.
// If any dir resolves outside root and allowOutside is false, nothing is
// removed and an error listing the offending paths is returned.
//
// forgeStateRoot carves out forge's own managed area: when non-empty, a dir
// outside root but under forgeStateRoot counts as safe and is deleted
// without --yes. An empty forgeStateRoot disables the carve-out entirely.
//
// protectedFiles names configuration files Flush must never endanger,
// regardless of allowOutside: a candidate dir is refused when it contains
// a regular file named "config.toml", or when it equals the directory of
// any protectedFile after absolute-path resolution — e.g. an XDG-style
// savedir next to ~/.config/forge/config.toml or the repo-local
// .forge/config.toml sibling of the pr cache. All refusals are collected
// across every dir first; the whole flush then aborts with nothing touched.
func Flush(root, forgeStateRoot string, dirs []string, allowOutside bool, protectedFiles ...string) ([]string, error) {
	root = absPath(root)
	if forgeStateRoot != "" {
		forgeStateRoot = absPath(forgeStateRoot)
	}
	var outside []string
	for _, dir := range dirs {
		dir = absPath(dir)
		if !withinRoot(root, dir) && !isUnder(forgeStateRoot, dir) && !allowOutside {
			outside = append(outside, dir)
		}
	}
	if len(outside) > 0 {
		return nil, fmt.Errorf("refusing to flush outside %s: %s",
			root, strings.Join(outside, ", "))
	}

	var refused []string
	protectedBasename := "config.toml"
	for _, dir := range dirs {
		dir = absPath(dir)
		entries, err := os.ReadDir(dir)
		if err == nil {
			for _, e := range entries {
				if !e.IsDir() && e.Type().IsRegular() && e.Name() == protectedBasename {
					refused = append(refused, dir)
					break
				}
			}
		}
		// Missing/unreadable dirs skip only the basename scan; the parent
		// match still applies.
		for _, p := range protectedFiles {
			if filepath.Dir(absPath(p)) == dir {
				refused = append(refused, dir)
				break
			}
		}
	}
	if len(refused) > 0 {
		return nil, fmt.Errorf("refusing to flush protected locations: %s",
			strings.Join(refused, ", "))
	}

	var removed []string
	for _, dir := range dirs {
		dir = absPath(dir)
		entries, err := os.ReadDir(dir)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return removed, fmt.Errorf("store: read %s: %w", dir, err)
		}
		empty := true
		for _, e := range entries {
			p := filepath.Join(dir, e.Name())
			if e.IsDir() {
				empty = false
				continue
			}
			if !e.Type().IsRegular() {
				empty = false
				continue
			}
			if err := os.Remove(p); err != nil {
				return removed, fmt.Errorf("store: remove %s: %w", p, err)
			}
			removed = append(removed, p)
		}
		if empty {
			if err := os.Remove(dir); err != nil && !os.IsNotExist(err) {
				return removed, fmt.Errorf("store: remove %s: %w", dir, err)
			}
		}
	}
	return removed, nil
}

// absPath makes p absolute without requiring it to exist.
func absPath(p string) string {
	a, err := filepath.Abs(p)
	if err != nil {
		return p
	}
	return a
}

// withinRoot reports whether path is root itself or inside it.
func withinRoot(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return rel == "." || (!strings.HasPrefix(rel, "..") && rel != "")
}

// isUnder reports whether dir lies strictly inside parent (or equals it).
// An empty parent is never considered a container: callers use "" to mean
// "no carve-out", and the filesystem root must not match that.
func isUnder(parent, dir string) bool {
	if parent == "" || dir == "" {
		return false
	}
	return withinRoot(parent, dir)
}
