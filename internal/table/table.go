// Package table renders rows of cells as fixed-width text columns for
// human-facing command output. It owns layout only; column specs and cell
// formatting live beside the data they describe, in internal/cmds.
package table

import (
	"fmt"
	"io"
	"strings"
	"unicode/utf8"
)

// Column is one rendered column. Content exceeding Width truncates with a
// trailing ".." (always width-fitting).
type Column struct {
	Name  string // printed as-is in the header row; write headers already uppercased
	Width int    // display columns; 0 drops the column entirely
}

// Render writes a padded header row, one dashed separator line sized per
// column, then each row. Short rows pad with empty cells; extra cells drop.
func Render(w io.Writer, cols []Column, rows [][]string) error {
	if len(cols) == 0 || len(rows) == 0 {
		return nil
	}
	if err := writeRow(w, cols, headers(cols)); err != nil {
		return err
	}
	if err := writeRow(w, cols, seps(cols)); err != nil {
		return err
	}
	for _, r := range rows {
		if err := writeRow(w, cols, r); err != nil {
			return err
		}
	}
	return nil
}

func headers(cols []Column) []string {
	out := make([]string, len(cols))
	for i, c := range cols {
		out[i] = truncate(c.Name, c.Width)
	}
	return out
}

func seps(cols []Column) []string {
	out := make([]string, len(cols))
	for i := range cols {
		out[i] = strings.Repeat("-", cols[i].Width)
	}
	return out
}

func writeRow(w io.Writer, cols []Column, cells []string) error {
	parts := make([]string, len(cols))
	for i, c := range cols {
		cell := ""
		if i < len(cells) {
			cell = cells[i]
		}
		parts[i] = fmt.Sprintf("%-*s", c.Width, truncate(cell, c.Width))
	}
	_, err := fmt.Fprintln(w, strings.TrimRight(strings.Join(parts, " "), " "))
	return err
}

func truncate(s string, width int) string {
	r := utf8.RuneCountInString(s)
	if r <= width {
		return s
	}
	out, count := "", 0
	for _, rn := range s {
		w := utf8.RuneLen(rn)
		if count+w > width-2 {
			break
		}
		out += string(rn)
		count += w
	}
	return out + ".."
}
