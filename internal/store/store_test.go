package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSaveBasic(t *testing.T) {
	dir := t.TempDir()
	p, err := Save(dir, "repo", 7, []byte("{}"))
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(p) != "repo-7.json" {
		t.Errorf("path = %s", p)
	}
}

func TestSaveCollisionSuffixesTimestamp(t *testing.T) {
	dir := t.TempDir()
	orig := timeNowUnix
	timeNowUnix = func() int64 { return 1700000000 }
	defer func() { timeNowUnix = orig }()

	p1, err := Save(dir, "repo", 7, []byte("a"))
	if err != nil {
		t.Fatal(err)
	}
	p2, err := Save(dir, "repo", 7, []byte("b"))
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(p2) != "repo-7-1700000000.json" {
		t.Errorf("collision path = %s", p2)
	}
	data, _ := os.ReadFile(p2)
	if string(data) != "b" {
		t.Errorf("collision overwrote original content")
	}
	_ = p1
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

	removed, err := Flush(root, []string{dir}, false)
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
	if _, err := Flush(root, []string{dir}, false); err != nil {
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

	_, err := Flush(root, []string{dir, outside}, false)
	if err == nil || !strings.Contains(err.Error(), outside) {
		t.Fatalf("want scope error naming %s, got %v", outside, err)
	}
	if _, serr := os.Stat(f); serr != nil {
		t.Error("refused flush must not delete anything")
	}

	// With allowOutside the file goes.
	removed, err := Flush(root, []string{outside}, true)
	if err != nil || len(removed) != 1 {
		t.Fatalf("allowOutside flush = %v %v", removed, err)
	}
}
