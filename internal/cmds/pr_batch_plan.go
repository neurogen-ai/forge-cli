package cmds

import (
	"fmt"
	"sort"

	"forge/internal/gitctx"
)

// planBatchBranches turns matched branch names into the ordered create-batch
// plan. Skip reasons become notes; the caller owns where notes go. Ordering:
// tip date ascending, ancestry breaks ties (parent first), then lexical.
// Every git lookup degrades to a default (0 date, "" subject, ancestry
// false), so there is no error return; an unresolvable base leaves the
// containment check inconclusive and the branch in the plan.
func planBatchBranches(root string, matched []string, base string) (items []BatchReceiptItem, notes []string) {
	type cand struct {
		item BatchReceiptItem
		date int64
	}
	cands := []cand{}
	for _, branch := range matched {
		title := gitctx.CommitSubject(root, branch)
		if title == "" {
			notes = append(notes, fmt.Sprintf("skipped: %s (no commit subject)", branch))
			continue
		}
		// Local containment preflight (release v0.4.1 §2): a tip already
		// contained in the resolved base would open a PR with an empty
		// diff, so it is skipped before any POST. UniqueCommitCount == 0
		// means contained; IsAncestor confirms. Either lookup failing
		// leaves the check inconclusive and the branch in the plan.
		if n, err := gitctx.UniqueCommitCount(root, base, branch); err == nil && n == 0 {
			if anc, aerr := gitctx.IsAncestor(root, branch, base); aerr == nil && anc {
				notes = append(notes, fmt.Sprintf("skipped: %s (already in base)", branch))
				continue
			}
		}
		cands = append(cands, cand{
			item: BatchReceiptItem{Branch: branch, Title: title, Base: base},
			date: gitctx.BranchTipDate(root, branch),
		})
	}
	sort.SliceStable(cands, func(i, j int) bool {
		a, b := cands[i], cands[j]
		if a.date != b.date {
			return a.date < b.date
		}
		// Equal dates: a branch that is an ancestor of another is its
		// stack parent and precedes it. Equal refs count as ancestors
		// (git's convention), so if both directions report ancestry
		// (same tip) or both lookups fail, ancestry is inconclusive and
		// the tie falls through to lexical.
		aAnc, aErr := gitctx.IsAncestor(root, a.item.Branch, b.item.Branch)
		bAnc, bErr := gitctx.IsAncestor(root, b.item.Branch, a.item.Branch)
		if aErr == nil && aAnc && !(bErr == nil && bAnc) {
			return true
		}
		if bErr == nil && bAnc && !(aErr == nil && aAnc) {
			return false
		}
		return a.item.Branch < b.item.Branch
	})
	items = make([]BatchReceiptItem, 0, len(cands))
	for _, c := range cands {
		items = append(items, c.item)
	}
	return items, notes
}
