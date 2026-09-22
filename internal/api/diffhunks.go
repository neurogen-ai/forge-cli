package api

import (
	"strconv"
	"strings"
)

// Hunk is one @@-delimited range of a unified diff, in git's 1-based
// start / count form.
type Hunk struct {
	OldStart int
	OldLines int
	NewStart int
	NewLines int
}

// CoversOld reports whether a 1-based line number falls inside the hunk's
// old-side range (context or removed lines). A hunk with no old lines —
// file creation — covers nothing on the old side.
func (h Hunk) CoversOld(line int) bool {
	return h.OldLines > 0 && line >= h.OldStart && line < h.OldStart+h.OldLines
}

// CoversNew reports whether a 1-based line number falls inside the hunk's
// new-side range (context or added lines). A hunk with no new lines —
// file deletion — covers nothing on the new side.
func (h Hunk) CoversNew(line int) bool {
	return h.NewLines > 0 && line >= h.NewStart && line < h.NewStart+h.NewLines
}

// ParseDiffHunks extracts the hunk ranges of path from a unified diff, as
// the raw GET /pulls/{index}.diff response returns it. A path matches
// either the old or the new file name, so a renamed file still resolves.
// found is false when the path does not appear in the diff at all.
//
// Known limits, accepted on purpose: custom --src-prefix/--dst-prefix spellings
// and git's quoted-path escaping are not decoded; a/ and b/ prefixes are.
func ParseDiffHunks(diff []byte, path string) (hunks []Hunk, found bool) {
	var gitOld, gitNew, oldPath, newPath string
	inHeader := false
	sectionPaths := func() (string, string) {
		// git omits the ---/+++ headers for binary sections; the diff --git
		// line is the fallback name source.
		if oldPath == "" && newPath == "" {
			return gitOld, gitNew
		}
		return oldPath, newPath
	}
	flush := func() {
		o, n := sectionPaths()
		if o == path || n == path {
			found = true
		}
		oldPath, newPath, gitOld, gitNew, inHeader = "", "", "", "", false
	}
	for _, line := range strings.Split(string(diff), "\n") {
		switch {
		case strings.HasPrefix(line, "diff --git "):
			// Hunk body lines always carry a +/-/space prefix, so a section
			// header is unambiguous even when content repeats the phrase.
			flush()
			fields := strings.Fields(line)
			if len(fields) >= 3 {
				gitOld = diffHeaderPath(fields[1])
				gitNew = diffHeaderPath(fields[2])
			}
			inHeader = true
		case inHeader && strings.HasPrefix(line, "--- "):
			oldPath = diffHeaderPath(line[4:])
		case inHeader && strings.HasPrefix(line, "+++ "):
			newPath = diffHeaderPath(line[4:])
			inHeader = false
		case strings.HasPrefix(line, "@@ "):
			inHeader = false
			o, n := sectionPaths()
			if o != path && n != path {
				continue
			}
			if h, ok := parseHunkHeader(line); ok {
				hunks = append(hunks, h)
			}
		}
	}
	flush()
	return hunks, found
}

// diffHeaderPath decodes one ---/+++ header argument: /dev/null means the
// side is absent, a/ and b/ prefixes are stripped, quoted paths are unquoted.
func diffHeaderPath(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		s = s[1 : len(s)-1]
	}
	if s == "/dev/null" {
		return ""
	}
	if len(s) > 2 && (s[0] == 'a' || s[0] == 'b') && s[1] == '/' {
		return s[2:]
	}
	return s
}

// parseHunkHeader parses "@@ -l,s +l,s @@ ..." where either count may be
// omitted (git drops ",1") and 0,0 marks an empty side.
func parseHunkHeader(line string) (Hunk, bool) {
	fields := strings.Fields(line)
	if len(fields) < 3 || fields[0] != "@@" {
		return Hunk{}, false
	}
	os, ol, ok := parseHunkRange(fields[1])
	if !ok {
		return Hunk{}, false
	}
	ns, nl, ok := parseHunkRange(fields[2])
	if !ok {
		return Hunk{}, false
	}
	return Hunk{OldStart: os, OldLines: ol, NewStart: ns, NewLines: nl}, true
}

func parseHunkRange(field string) (start, lines int, ok bool) {
	s := strings.TrimPrefix(field, "-")
	s = strings.TrimPrefix(s, "+")
	if i := strings.IndexByte(s, ','); i >= 0 {
		start, err := strconv.Atoi(s[:i])
		if err != nil {
			return 0, 0, false
		}
		lines, err = strconv.Atoi(s[i+1:])
		if err != nil || lines < 0 {
			return 0, 0, false
		}
		return start, lines, true
	}
	start, err := strconv.Atoi(s)
	if err != nil {
		return 0, 0, false
	}
	return start, 1, true
}
