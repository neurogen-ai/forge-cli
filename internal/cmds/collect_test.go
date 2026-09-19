package cmds

import (
	"reflect"
	"testing"
)

// TestCollectFlagValues pins the one repeated-value-flag collection path:
// order, duplicates, skipping a trailing bare flag, and the exact match on
// the flag name.
func TestCollectFlagValues(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want []string
	}{
		{"absent", []string{"5", "--other", "x"}, nil},
		{"single", []string{"--label", "bug", "5"}, []string{"bug"}},
		{"repeated in order", []string{"5", "--label", "bug", "--title", "t", "--label", "ui"}, []string{"bug", "ui"}},
		{"duplicates kept", []string{"--label", "bug", "--label", "bug"}, []string{"bug", "bug"}},
		{"trailing bare flag ignored", []string{"--label", "bug", "--label"}, []string{"bug"}},
		{"similar flag untouched", []string{"--labels", "x", "--label", "bug"}, []string{"bug"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := collectFlagValues(tc.args, "--label"); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("collectFlagValues(%q, \"--label\") = %q, want %q", tc.args, got, tc.want)
			}
		})
	}
}

// TestCollectLabelNamesDelegates pins the delegation: identical results to
// collectFlagValues for the --label flag.
func TestCollectLabelNamesDelegates(t *testing.T) {
	args := []string{"5", "--label", "bug", "--label", "ui", "--label"}
	names, err := collectLabelNames(args, "issue label")
	if err != nil {
		t.Fatal(err)
	}
	want := collectFlagValues(args, "--label")
	if !reflect.DeepEqual(names, want) {
		t.Errorf("collectLabelNames = %q, want %q", names, want)
	}
}
