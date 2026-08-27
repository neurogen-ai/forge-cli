package cmds

import (
	"testing"
	"time"

	"forge/internal/api"
)

func TestTimeShort(t *testing.T) {
	if got := timeShort(nil); got != "" {
		t.Errorf("timeShort(nil) = %q, want empty string", got)
	}
	ts := time.Date(2024, 5, 1, 15, 30, 45, 0, time.UTC)
	if got := timeShort(&ts); got != "2024-05-01" {
		t.Errorf("timeShort = %q, want %q", got, "2024-05-01")
	}
}

func TestPRListRows(t *testing.T) {
	t1 := time.Date(2024, 5, 1, 9, 0, 0, 0, time.UTC)
	prs := []api.PullRequest{
		{
			Number:    12,
			Title:     "Fix login flow",
			State:     "open",
			User:      api.User{Login: "alice"},
			CreatedAt: &t1,
		},
		{
			Number: 3456,
			Title:  "Drop stale sessions",
			State:  "merged",
			User:   api.User{Login: "bob"},
			// CreatedAt nil on purpose.
		},
	}

	rows := prListRows(prs)
	if len(rows) != len(prs) {
		t.Fatalf("got %d rows, want %d", len(rows), len(prs))
	}
	want := [][]string{
		{"12", "Fix login flow", "open", "alice", "2024-05-01"},
		{"3456", "Drop stale sessions", "merged", "bob", ""},
	}
	for i, row := range rows {
		for j, cell := range row {
			if cell != want[i][j] {
				t.Errorf("row %d col %d = %q, want %q", i, j, cell, want[i][j])
			}
		}
	}
	if len(rows[0]) != len(prListColumns) {
		t.Errorf("row width %d != prListColumns width %d", len(rows[0]), len(prListColumns))
	}
}

func TestIssueListRows(t *testing.T) {
	t1 := time.Date(2024, 7, 4, 23, 59, 59, 0, time.UTC)
	iss := []api.Issue{
		{
			Number:    7,
			Title:     "Spinner hangs",
			State:     "open",
			User:      api.User{Login: "carol"},
			CreatedAt: &t1,
		},
		{
			Number: 89,
			Title:  "Typo in help",
			State:  "closed",
			User:   api.User{Login: "dave"},
		},
	}

	rows := issueListRows(iss)
	if len(rows) != len(iss) {
		t.Fatalf("got %d rows, want %d", len(rows), len(iss))
	}
	want := [][]string{
		{"7", "Spinner hangs", "open", "carol"},
		{"89", "Typo in help", "closed", "dave"},
	}
	for i, row := range rows {
		for j, cell := range row {
			if cell != want[i][j] {
				t.Errorf("row %d col %d = %q, want %q", i, j, cell, want[i][j])
			}
		}
	}
	if len(rows[0]) != len(issueListColumns) {
		t.Errorf("row width %d != issueListColumns width %d (UPDATED omitted by spec)", len(rows[0]), len(issueListColumns))
	}
}
