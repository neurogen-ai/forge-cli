// Package store is the only filesystem-writing code in forge: the save
// family and the cache flush/path helpers.
package store

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// timeNowUnix is a variable so tests can pin the collision timestamp.
var timeNowUnix = func() int64 { return time.Now().Unix() }

// Save writes data to <dir>/<repo>-<number>.json. When that file exists, a
// unix timestamp is inserted before the extension: <repo>-<N>-<unix-ts>.json.
// The directory is created (0755) when missing. Returns the written path.
func Save(dir, repo string, number int, data []byte) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("store: create %s: %w", dir, err)
	}
	base := fmt.Sprintf("%s-%d", repo, number)
	path := filepath.Join(dir, base+".json")
	if _, err := os.Stat(path); err == nil {
		// Suffix until we hit a name that does not exist yet; a single
		// suffix step could still collide and silently overwrite.
		for i := int64(1); ; i++ {
			path = filepath.Join(dir, fmt.Sprintf("%s-%d-%d.json", base, timeNowUnix(), i))
			if _, err := os.Stat(path); os.IsNotExist(err) {
				break
			}
		}
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return "", fmt.Errorf("store: write %s: %w", path, err)
	}
	return path, nil
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
func Flush(root string, dirs []string, allowOutside bool) ([]string, error) {
	root = absPath(root)
	var outside []string
	for _, dir := range dirs {
		dir = absPath(dir)
		if !withinRoot(root, dir) && !allowOutside {
			outside = append(outside, dir)
		}
	}
	if len(outside) > 0 {
		return nil, fmt.Errorf("refusing to flush outside %s: %s",
			root, strings.Join(outside, ", "))
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
