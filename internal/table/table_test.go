package table

import (
	"bytes"
	"strings"
	"testing"
)

// NOTE (known limitation): widths are counted in runes via utf8, so East Asian
// wide characters occupy two terminal display columns but are treated as one.
// Rendering such content may produce misaligned output; do not add tests
// asserting CJK behaviour until the renderer gains display-width awareness.

func render(t *testing.T, cols []Column, rows [][]string) string {
	t.Helper()
	var buf bytes.Buffer
	if err := Render(&buf, cols, rows); err != nil {
		t.Fatalf("Render returned error: %v", err)
	}
	return buf.String()
}

// TestRenderGoldenShape pins the exact header/separator/row layout at fixed
// widths: cells left-padded to their column width, columns joined by a single
// space, trailing whitespace trimmed, one newline per line.
func TestRenderGoldenShape(t *testing.T) {
	cols := []Column{
		{Name: "A", Width: 4},
		{Name: "BB", Width: 6},
		{Name: "CCC", Width: 5},
	}
	got := render(t, cols, [][]string{
		{"x", "yy", "zz"},
	})

	want := "A    BB     CCC\n" +
		"---- ------ -----\n" +
		"x    yy     zz\n"
	if got != want {
		t.Errorf("golden mismatch:\n got: %q\nwant: %q", got, want)
	}
}

// TestTruncatesLongCell pins that a long cell becomes exactly Width display
// columns wide and ends in "..". Long title has 43 runes; at width 20 it must
// render as the first 18 runes plus ".." filling the column exactly.
func TestTruncatesLongCell(t *testing.T) {
	cols := []Column{
		{Name: "TITLE", Width: 20},
	}
	long := "The Quick Brown Fox Jumps Over The Lazy Dog" // 43 runes
	got := render(t, cols, [][]string{{long}})

	const cut = "The Quick Brown Fo" // first 18 runes
	want := "TITLE\n" + "--------------------\n" + cut + "..\n"
	if got != want {
		t.Errorf("truncation mismatch:\n got: %q\nwant: %q", got, want)
	}
	if !strings.HasSuffix(got, "..\n") {
		t.Errorf("truncated cell must end in %q, got %q", "..", got)
	}
}

// TestEmptyRowsRenderNothing pins that zero data rows produce no output at
// all — not even a header.
func TestEmptyRowsRenderNothing(t *testing.T) {
	cols := []Column{{Name: "A", Width: 4}}
	if got := render(t, cols, nil); got != "" {
		t.Errorf("nil rows: got %q, want empty output", got)
	}
	if got := render(t, cols, [][]string{}); got != "" {
		t.Errorf("empty rows: got %q, want empty output", got)
	}
}

// TestShortRowsPadWithEmpties pins that a row with fewer cells than columns
// fills missing cells as blank space, keeping the other columns aligned.
func TestShortRowsPadWithEmpties(t *testing.T) {
	cols := []Column{
		{Name: "A", Width: 4},
		{Name: "B", Width: 6},
		{Name: "C", Width: 5},
	}
	got := render(t, cols, [][]string{{"only"}})

	// Missing B and C cells render as all-blank columns; trailing blanks trim,
	// leaving only column A's content.
	want := "A    B      C\n" +
		"---- ------ -----\n" +
		"only\n"
	if got != want {
		t.Errorf("short row mismatch:\n got: %q\nwant: %q", got, want)
	}
}

// TestExtraCellsDrop pins that cells beyond len(cols) are silently discarded.
func TestExtraCellsDrop(t *testing.T) {
	cols := []Column{
		{Name: "A", Width: 4},
		{Name: "B", Width: 6},
		{Name: "C", Width: 5},
	}
	got := render(t, cols, [][]string{{"a", "bb", "ccc", "dddd", "eeeee"}})

	want := "A    B      C\n" +
		"---- ------ -----\n" +
		"a    bb     ccc\n"
	if got != want {
		t.Errorf("extra-cell row mismatch:\n got: %q\nwant: %q", got, want)
	}
}

// TestEmptyColsRenderNothing covers the symmetric guard on the column side.
func TestEmptyColsRenderNothing(t *testing.T) {
	if got := render(t, nil, [][]string{{"a"}}); got != "" {
		t.Errorf("nil cols: got %q, want empty output", got)
	}
}
