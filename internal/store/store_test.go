package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteOverwrites(t *testing.T) {
	dir := t.TempDir()

	// First write: exactly <repo>-7.json, no timestamp/suffix variant.
	p, err := Write(dir, "repo", 7, []byte("{}"))
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(p) != "repo-7.json" {
		t.Errorf("path = %s", p)
	}

	// Second write with the same args must replace the file in place.
	p2, err := Write(dir, "repo", 7, []byte("{\"v\":2}"))
	if err != nil {
		t.Fatal(err)
	}
	if p2 != p {
		t.Errorf("rewrite path = %s, want %s", p2, p)
	}
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "{\"v\":2}" {
		t.Errorf("content = %q, want replaced content", data)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Errorf("entry count = %d (%s), want 1", len(entries), entries[0].Name())
	}
}

func TestWriteJSONPrettyPrints(t *testing.T) {
	dir := t.TempDir()
	p, err := WriteJSON(dir, "repo", 3, map[string]int{"a": 1})
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(p) != "repo-3.json" {
		t.Errorf("path = %s", p)
	}
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "{\n  \"a\": 1\n}" {
		t.Errorf("content = %q, want two-space indented JSON", data)
	}
}

func TestResolveDirsExpandsHomeAndRoot(t *testing.T) {
	dirs := ResolveDirs(map[string]string{
		"issue": ".forge/issues",
	}, "/repo", "/home/u")
	want := filepath.Join("/repo", ".forge/issues")
	if dirs[0] != want {
		t.Errorf("relative: %s, want %s", dirs[0], want)
	}
	dirs = ResolveDirs(map[string]string{"issue": "~/saved"}, "/repo", "/home/u")
	if dirs[0] != filepath.Join("/home/u", "saved") {
		t.Errorf("~ expansion: %s", dirs[0])
	}
	dirs = ResolveDirs(map[string]string{"issue": "/abs"}, "/repo", "/home/u")
	if dirs[0] != "/abs" {
		t.Errorf("absolute: %s", dirs[0])
	}
}

func TestFlushRemovesFilesNotSubdirs(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, ".forge", "issues")
	os.MkdirAll(filepath.Join(dir, "sub"), 0o755)
	f1 := filepath.Join(dir, "r-1.json")
	f2 := filepath.Join(dir, "r-2.json")
	os.WriteFile(f1, []byte("{}"), 0o644)
	os.WriteFile(f2, []byte("{}"), 0o644)

	removed, err := Flush(root, "", []string{dir}, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(removed) != 2 {
		t.Fatalf("removed = %v", removed)
	}
	for _, f := range []string{f1, f2} {
		if _, err := os.Stat(f); !os.IsNotExist(err) {
			t.Errorf("%s still exists", f)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "sub")); err != nil {
		t.Error("subdirectory must survive a flush")
	} else {
		// The parent dir must also survive because it is not empty.
		if _, err := os.Stat(dir); err != nil {
			t.Error("non-empty cache dir must not be removed")
		}
	}
}

func TestFlushEmptyDirRemoved(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, ".forge", "issues")
	os.MkdirAll(dir, 0o755)
	if _, err := Flush(root, "", []string{dir}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Error("emptied dir should be removed")
	}
}

func TestFlushRefusesOutsideRoot(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	dir := filepath.Join(root, ".forge", "issues")
	os.MkdirAll(dir, 0o755)
	f := filepath.Join(outside, "victim.json")
	os.WriteFile(f, []byte("{}"), 0o644)

	_, err := Flush(root, "", []string{dir, outside}, false)
	if err == nil || !strings.Contains(err.Error(), outside) {
		t.Fatalf("want scope error naming %s, got %v", outside, err)
	}
	if _, serr := os.Stat(f); serr != nil {
		t.Error("refused flush must not delete anything")
	}

	// With allowOutside the file goes.
	removed, err := Flush(root, "", []string{outside}, true)
	if err != nil || len(removed) != 1 {
		t.Fatalf("allowOutside flush = %v %v", removed, err)
	}
}

func TestFlushForgeStateCarveOut(t *testing.T) {
	stateRoot := t.TempDir()
	outside := t.TempDir()

	t.Run("outside root but under forge state dir flushes without yes", func(t *testing.T) {
		dir := filepath.Join(stateRoot, "forge", "prs")
		os.MkdirAll(dir, 0o755)
		f := filepath.Join(dir, "repo-1.json")
		os.WriteFile(f, []byte("{}"), 0o644)

		removed, err := Flush(outside, stateRoot, []string{dir}, false)
		if err != nil {
			t.Fatal(err)
		}
		if len(removed) != 1 || removed[0] != f {
			t.Fatalf("removed = %v, want [%s]", removed, f)
		}
	})

	t.Run("sibling of forge state dir still refused without yes", func(t *testing.T) {
		sibling := filepath.Join(filepath.Dir(stateRoot), "other-tool", "cache")
		os.MkdirAll(sibling, 0o755)
		f := filepath.Join(sibling, "victim.json")
		os.WriteFile(f, []byte("{}"), 0o644)

		_, err := Flush(outside, stateRoot, []string{sibling}, false)
		if err == nil || !strings.Contains(err.Error(), sibling) {
			t.Fatalf("want refusal naming %s, got %v", sibling, err)
		}
		if _, serr := os.Stat(f); serr != nil {
			t.Error("refused flush must not delete anything")
		}

		// With --yes the carve-out does not matter; deletion proceeds.
		if removed, err := Flush(outside, stateRoot, []string{sibling}, true); err != nil || len(removed) != 1 {
			t.Fatalf("--yes flush = %v %v", removed, err)
		}
	})

	t.Run("empty forgeStateRoot restores old strictness", func(t *testing.T) {
		dir := filepath.Join(stateRoot, "forge", "prs")
		os.MkdirAll(dir, 0o755)
		f := filepath.Join(dir, "repo-1.json")
		os.WriteFile(f, []byte("{}"), 0o644)

		if _, err := Flush(outside, "", []string{dir}, false); err == nil {
			t.Fatal("want refusal when carve-out disabled")
		}
		if _, err := Flush(outside, "", []string{dir}, true); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("root itself inside forge state dir still uses root containment once", func(t *testing.T) {
		// Root sits inside the state root; a dir under root is flushed via
		// root containment exactly like before — no double-count or skip.
		root := filepath.Join(stateRoot, "workspace")
		dir := filepath.Join(root, ".forge", "issues")
		os.MkdirAll(dir, 0o755)
		os.WriteFile(filepath.Join(dir, "a.json"), []byte("{}"), 0o644)
		os.WriteFile(filepath.Join(dir, "b.json"), []byte("{}"), 0o644)

		removed, err := Flush(root, stateRoot, []string{dir}, false)
		if err != nil {
			t.Fatal(err)
		}
		if len(removed) != 2 {
			t.Fatalf("removed = %v, want both files once", removed)
		}
	})
}
