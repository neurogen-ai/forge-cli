package cmds

import (
	"testing"

	"forge/internal/gitctx"
)

func TestPlanBatchOrdersByTipDate(t *testing.T) {
	repo := batchRepoDated(t, map[string]batchBranch{
		"b-newest": {Subject: "newest", Date: 1700000300},
		"b-old":    {Subject: "old", Date: 1700000100},
		"b-mid":    {Subject: "mid", Date: 1700000200},
	})
	// Matched arrives in a non-lexical, non-chronological order.
	items, notes := planBatchBranches(repo.Root, []string{"b-newest", "b-old", "b-mid"}, "main")
	if len(notes) != 0 {
		t.Fatalf("notes = %v, want none", notes)
	}
	var got []string
	for _, it := range items {
		got = append(got, it.Branch)
	}
	want := []string{"b-old", "b-mid", "b-newest"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("order = %v, want %v (tip date ascending)", got, want)
		}
	}
	for _, it := range items {
		if it.Base != "main" {
			t.Fatalf("item %s Base = %q, want the resolved base", it.Branch, it.Base)
		}
		if it.Title == "" {
			t.Fatalf("item %s Title empty", it.Branch)
		}
	}
}

func TestPlanBatchAncestryBreaksEqualDateTies(t *testing.T) {
	// a-child stacks on b-parent and both tips carry the same pinned date.
	// Lexical order would put a-child first; the ancestry tie-break must
	// put the parent first.
	repo := batchRepoDated(t, map[string]batchBranch{
		"b-parent": {Subject: "parent", Date: 1700000000},
		"a-child":  {Subject: "child", Date: 1700000000, From: "b-parent"},
	})
	if anc, err := gitctx.IsAncestor(repo.Root, "b-parent", "a-child"); err != nil || !anc {
		t.Fatalf("fixture broken: b-parent ancestor of a-child = %v, err %v", anc, err)
	}
	items, notes := planBatchBranches(repo.Root, []string{"a-child", "b-parent"}, "main")
	if len(notes) != 0 {
		t.Fatalf("notes = %v, want none", notes)
	}
	if len(items) != 2 || items[0].Branch != "b-parent" || items[1].Branch != "a-child" {
		t.Fatalf("items = %+v, want b-parent (stack parent) before a-child", items)
	}
}

func TestPlanBatchLexicalBreaksEqualDateTies(t *testing.T) {
	// Same pinned date, no ancestry between the siblings: lexical order.
	repo := batchRepoDated(t, map[string]batchBranch{
		"zeta":    {Subject: "z", Date: 1700000000},
		"alpha":   {Subject: "a", Date: 1700000000},
		"midmaid": {Subject: "m", Date: 1700000000},
	})
	items, notes := planBatchBranches(repo.Root, []string{"zeta", "alpha", "midmaid"}, "main")
	if len(notes) != 0 {
		t.Fatalf("notes = %v, want none", notes)
	}
	var got []string
	for _, it := range items {
		got = append(got, it.Branch)
	}
	want := []string{"alpha", "midmaid", "zeta"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("order = %v, want %v (lexical fallback)", got, want)
		}
	}
}

func TestPlanBatchSkipsEmptySubjectWithNote(t *testing.T) {
	repo := batchRepoDated(t, map[string]batchBranch{
		"empty": {Subject: "", Date: 1700000000},
		"good":  {Subject: "real title", Date: 1700000000},
	})
	items, notes := planBatchBranches(repo.Root, []string{"empty", "good"}, "main")
	if len(notes) != 1 || notes[0] != "skipped: empty (no commit subject)" {
		t.Fatalf("notes = %q, want exactly one no-commit-subject note", notes)
	}
	if len(items) != 1 || items[0].Branch != "good" {
		t.Fatalf("items = %+v, want only good", items)
	}
}

func TestPlanBatchSkipsContainedInBase(t *testing.T) {
	// stale sits at main's tip (NoCommit), so UniqueCommitCount(main..stale)
	// is 0 and it is skipped locally, before any POST. merged carries a new
	// commit and stays in the plan.
	repo := batchRepoDated(t, map[string]batchBranch{
		"stale":  {NoCommit: true},
		"merged": {Subject: "merged work", Date: 1700000000},
	})
	items, notes := planBatchBranches(repo.Root, []string{"stale", "merged"}, "main")
	if len(notes) != 1 || notes[0] != "skipped: stale (already in base)" {
		t.Fatalf("notes = %q, want exactly one already-in-base note", notes)
	}
	if len(items) != 1 || items[0].Branch != "merged" {
		t.Fatalf("items = %+v, want only merged", items)
	}
}

func TestPlanBatchUnresolvableBaseKeepsBranches(t *testing.T) {
	// A base that does not resolve makes the containment check fail
	// (inconclusive), which degrades to keeping the branch in the plan
	// rather than erroring or skipping.
	repo := batchRepoDated(t, map[string]batchBranch{
		"stale":  {NoCommit: true},
		"merged": {Subject: "merged work", Date: 1700000000},
	})
	items, notes := planBatchBranches(repo.Root, []string{"stale", "merged"}, "no-such-base")
	if len(notes) != 0 {
		t.Fatalf("notes = %v, want none", notes)
	}
	var got []string
	for _, it := range items {
		got = append(got, it.Branch)
	}
	if len(got) != 2 || got[0] != "merged" || got[1] != "stale" {
		t.Fatalf("items = %v, want both branches kept (merged first: stale's tip is main's now-dated init commit)", got)
	}
}

func TestPlanBatchUnresolvableRefDegradesToNote(t *testing.T) {
	// An unresolvable branch name degrades to "" subject and 0 date, so
	// it surfaces as a skip note rather than an error, and the remaining
	// equal-date branches still order lexically.
	repo := batchRepoDated(t, map[string]batchBranch{
		"real-b": {Subject: "b", Date: 1700000000},
		"real-a": {Subject: "a", Date: 1700000000},
	})
	items, notes := planBatchBranches(repo.Root, []string{"ghost-z", "real-b", "real-a"}, "main")
	if len(notes) != 1 || notes[0] != "skipped: ghost-z (no commit subject)" {
		t.Fatalf("notes = %q, want one note for ghost-z", notes)
	}
	var got []string
	for _, it := range items {
		got = append(got, it.Branch)
	}
	want := []string{"real-a", "real-b"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("order = %v, want %v", got, want)
		}
	}
}

func TestPlanBatchSameTipCommitsOrderLexically(t *testing.T) {
	// twin-a and twin-b point at the same tip commit, so IsAncestor
	// reports ancestry in both directions (equal refs are ancestors, git's
	// convention). The comparator must stay a strict weak ordering: the
	// pair falls back to lexical, and equal-date siblings are unaffected.
	repo := batchRepoDated(t, map[string]batchBranch{
		"shared-tip": {Subject: "tip", Date: 1700000000},
		"twin-b":     {NoCommit: true, From: "shared-tip"},
		"twin-a":     {NoCommit: true, From: "shared-tip"},
		"mid-way":    {Subject: "sibling", Date: 1700000000},
	})
	items, notes := planBatchBranches(repo.Root, []string{"twin-b", "twin-a", "mid-way"}, "main")
	if len(notes) != 0 {
		t.Fatalf("notes = %v, want none", notes)
	}
	var got []string
	for _, it := range items {
		got = append(got, it.Branch)
	}
	want := []string{"mid-way", "twin-a", "twin-b"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("order = %v, want %v (same-tip pair lexical, siblings unaffected)", got, want)
		}
	}
}
