package api

import "testing"

const diffFixture = `diff --git a/plans/releases/v0.1.md b/plans/releases/v0.1.md
index aaa..bbb 100644
--- a/plans/releases/v0.1.md
+++ b/plans/releases/v0.1.md
@@ -1,3 +1,4 @@ plan header
 context
+added line
 context
-removed
 context
diff --git a/created.go b/created.go
new file mode 100644
index 000..ccc
--- /dev/null
+++ b/created.go
@@ -0,0 +1,2 @@
+one
+two
diff --git a/deleted.go b/deleted.go
deleted file mode 100644
index ddd..000
--- a/deleted.go
+++ /dev/null
@@ -1,2 +0,0 @@
-one
-two
diff --git a/old-name.go b/new-name.go
similarity index 90%
rename from old-name.go
rename to new-name.go
index eee..fff 100644
--- a/old-name.go
+++ b/new-name.go
@@ -7 +7,2 @@
 context
+tail
diff --git a/img.png b/img.png
index 111..222 100644
Binary files a/img.png and b/img.png differ
`

func TestParseDiffHunksRanges(t *testing.T) {
	cases := []struct {
		path string
		want []Hunk
	}{
		{"plans/releases/v0.1.md", []Hunk{{OldStart: 1, OldLines: 3, NewStart: 1, NewLines: 4}}},
		{"created.go", []Hunk{{OldStart: 0, OldLines: 0, NewStart: 1, NewLines: 2}}},
		{"deleted.go", []Hunk{{OldStart: 1, OldLines: 2, NewStart: 0, NewLines: 0}}},
		{"new-name.go", []Hunk{{OldStart: 7, OldLines: 1, NewStart: 7, NewLines: 2}}},
	}
	for _, tc := range cases {
		hunks, found := ParseDiffHunks([]byte(diffFixture), tc.path)
		if !found {
			t.Errorf("%s: not found", tc.path)
			continue
		}
		if len(hunks) != len(tc.want) || hunks[0] != tc.want[0] {
			t.Errorf("%s: hunks = %+v want %+v", tc.path, hunks, tc.want)
		}
	}
}

// Renamed files match their old name too; binary files are found but hunkless;
// unknown paths are not found.
func TestParseDiffHunksLookup(t *testing.T) {
	hunks, found := ParseDiffHunks([]byte(diffFixture), "old-name.go")
	if !found || len(hunks) != 1 {
		t.Errorf("rename lookup: found=%v hunks=%v", found, hunks)
	}
	hunks, found = ParseDiffHunks([]byte(diffFixture), "img.png")
	if !found || len(hunks) != 0 {
		t.Errorf("binary: found=%v hunks=%v", found, hunks)
	}
	if _, found := ParseDiffHunks([]byte(diffFixture), "absent.go"); found {
		t.Error("absent path reported found")
	}
}

// Coverage respects the empty sides of creations and deletions.
func TestHunkCovers(t *testing.T) {
	create := Hunk{OldStart: 0, OldLines: 0, NewStart: 1, NewLines: 2}
	if create.CoversOld(0) || create.CoversOld(1) {
		t.Error("creation covers old lines")
	}
	if !create.CoversNew(1) || !create.CoversNew(2) || create.CoversNew(3) {
		t.Error("creation new coverage wrong")
	}
	del := Hunk{OldStart: 1, OldLines: 2, NewStart: 0, NewLines: 0}
	if !del.CoversOld(2) || del.CoversOld(3) {
		t.Error("deletion old coverage wrong")
	}
	if del.CoversNew(0) || del.CoversNew(1) {
		t.Error("deletion covers new lines")
	}
}

// git omits ",1" from a one-line hunk range.
func TestParseDiffHunksOmittedCount(t *testing.T) {
	diff := []byte("diff --git a/f.go b/f.go\n--- a/f.go\n+++ b/f.go\n@@ -7 +7,2 @@\n c\n+x\n")
	hunks, found := ParseDiffHunks(diff, "f.go")
	if !found || len(hunks) != 1 || hunks[0] != (Hunk{OldStart: 7, OldLines: 1, NewStart: 7, NewLines: 2}) {
		t.Errorf("hunks = %+v found=%v", hunks, found)
	}
}
