package cmds

import (
	"testing"
	"time"

	"forge/internal/api"
)

// ---- helpers ----

func ts(s string) *time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return &t
}

func mkRev(id int64, submitted, created *time.Time) api.Review {
	return api.Review{ID: id, State: "COMMENTED", SubmittedAt: submitted, CreatedAt: created}
}

func mkRC(id int64, path string, pos int64, resolved bool) api.ReviewComment {
	rc := api.ReviewComment{ID: id, Path: path, Position: pos}
	if resolved {
		yes := true
		rc.Resolved = &yes
	}
	return rc
}

// convIds extracts review ids for order assertions.
func convIds(p ConvPayload) []int64 {
	ids := make([]int64, 0, len(p.Reviews))
	for _, r := range p.Reviews {
		ids = append(ids, r.ID)
	}
	return ids
}

// rcIds extracts comment ids within one ConvReview.
func rcIds(r ConvReview) []int64 {
	ids := make([]int64, 0, len(r.Comments))
	for _, c := range r.Comments {
		ids = append(ids, c.ID)
	}
	return ids
}

func eqInt64(t *testing.T, name string, got, want []int64) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s: got %v want %v", name, got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("%s: got %v want %v", name, got, want)
		}
	}
}

// ---- tests ----

// Three reviews with staggered timestamps incl. one nil-timestamp review that
// must end last. The nil-timestamp review falls back to nothing and thus sorts
// after every stamped review.
func TestBuildConvOrdersOldestFirstWithNilLast(t *testing.T) {
	reviews := []api.Review{
		mkRev(3, ts("2024-03-01T00:00:00Z"), nil),
		mkRev(1, ts("2024-01-01T00:00:00Z"), nil),
		mkRev(2, nil, nil), // no timestamps at all: last
	}
	perReview := map[int64][]api.ReviewComment{
		1: {mkRC(101, "a.go", 1, false)},
		2: {mkRC(102, "a.go", 1, false)},
		3: {mkRC(103, "a.go", 1, false)},
	}
	p := buildConv(nil, reviews, perReview, true, 0)
	eqInt64(t, "review order", convIds(p), []int64{1, 3, 2})
}

// A review with only a nil SubmittedAt but a set CreatedAt uses the fallback.
func TestBuildConvSubmittedAtFallsBackToCreatedAt(t *testing.T) {
	reviews := []api.Review{
		mkRev(9, nil, ts("2024-02-01T00:00:00Z")),
		mkRev(8, ts("2024-05-01T00:00:00Z"), nil),
	}
	perReview := map[int64][]api.ReviewComment{}
	p := buildConv(nil, reviews, perReview, true, 0)
	eqInt64(t, "fallback order", convIds(p), []int64{9, 8})
}

// showAll=false (default) drops fully-resolved reviews; count==0 < minUnresolved=1.
func TestBuildConvDropsFullyResolvedReviews(t *testing.T) {
	reviews := []api.Review{
		mkRev(1, ts("2024-01-01T00:00:00Z"), nil),
		mkRev(2, ts("2024-02-01T00:00:00Z"), nil),
	}
	perReview := map[int64][]api.ReviewComment{
		1: {mkRC(11, "a.go", 1, true)}, // fully resolved -> dropped
		2: {mkRC(21, "a.go", 1, false)},
	}
	p := buildConv(nil, reviews, perReview, false, 1)
	eqInt64(t, "filtered reviews", convIds(p), []int64{2})

	// showAll=true keeps everything; ordering unchanged.
	p = buildConv(nil, reviews, perReview, true, 1)
	eqInt64(t, "showAll keeps both", convIds(p), []int64{1, 2})
}

// minUnresolved=2 drops reviews with unresolved_count == 1 even when not fully resolved.
func TestBuildConvMinUnresolvedTwoDropsCountOne(t *testing.T) {
	reviews := []api.Review{
		mkRev(1, ts("2024-01-01T00:00:00Z"), nil),
		mkRev(2, ts("2024-02-01T00:00:00Z"), nil),
		mkRev(3, ts("2024-03-01T00:00:00Z"), nil),
	}
	perReview := map[int64][]api.ReviewComment{
		1: {mkRC(11, "a.go", 1, false), mkRC(12, "b.go", 1, false)}, // 2 unresolved: kept
		2: {mkRC(21, "a.go", 1, false), mkRC(22, "b.go", 1, true)},  // 1 unresolved: dropped
		3: {mkRC(31, "a.go", 1, true)},                              // 0 unresolved: dropped
	}
	p := buildConv(nil, reviews, perReview, false, 2)
	eqInt64(t, "minUnresolved=2", convIds(p), []int64{1})
	if p.Reviews[0].UnresolvedCount != 2 || p.Reviews[0].TotalCount != 2 {
		t.Fatalf("counts: unresolved=%d total=%d", p.Reviews[0].UnresolvedCount, p.Reviews[0].TotalCount)
	}
}

// --all semantics: showAll affects filtering only, never ordering. Compare
// against a hand-built payload of the same reviews.
func TestBuildConvShowAllAffectsFilteringNotOrdering(t *testing.T) {
	reviews := []api.Review{
		mkRev(5, ts("2024-05-01T00:00:00Z"), nil),
		mkRev(4, ts("2024-04-01T00:00:00Z"), nil),
		mkRev(6, nil, nil),
	}
	perReview := map[int64][]api.ReviewComment{
		5: {mkRC(51, "a.go", 1, true)},
		4: {mkRC(41, "a.go", 1, false)},
		6: {},
	}
	all := buildConv(nil, reviews, perReview, true, 1)
	filtered := buildConv(nil, reviews, perReview, false, 1)

	// Ordering identical to showAll run minus the dropped review.
	eqInt64(t, "showAll order", convIds(all), []int64{4, 5, 6})
	eqInt64(t, "filtered order", convIds(filtered), []int64{4})
	// Even with filtering on, surviving reviews keep oldest-first order.
	allInverted := buildConv(
		nil,
		[]api.Review{reviews[0], reviews[1]}, // input shuffled, still ordered by stamp
		map[int64][]api.ReviewComment{},
		true, 1,
	)
	eqInt64(t, "shuffle irrelevant", convIds(allInverted), []int64{4, 5})
}

// Comparator ties: equal stamps fall back to ID asc; inside reviews,
// unresolved before resolved, then Path/Position/ID; fully-tied entries stay
// stable relative to input (sort.SliceStable semantics).
func TestBuildConvComparatorTiesStable(t *testing.T) {
	reviews := []api.Review{
		mkRev(7, ts("2024-01-01T00:00:00Z"), nil),
		mkRev(2, ts("2024-01-01T00:00:00Z"), nil), // same stamp as 7 -> id asc puts 2 first
	}
	p := buildConv(nil, reviews, map[int64][]api.ReviewComment{}, true, 0)
	eqInt64(t, "stamp tie falls back to id", convIds(p), []int64{2, 7})

	// Comment ordering: unresolved first, then Path/Position/ID, ties stable.
	dup := mkRC(7, "b.go", 2, false) // same Path/Position/ID as the clone below
	rcs := []api.ReviewComment{
		mkRC(30, "z.go", 1, true), // resolved, stays after all unresolved
		mkRC(20, "b.go", 2, false),
		mkRC(10, "a.go", 3, false), // a.go first
		dup,
		{ID: 7, Path: "b.go", Position: 2}, // full tie with dup: stable keeps input order
		mkRC(12, "b.go", 1, false),         // same path but lower position than 20
	}
	orderCommentsUnresolvedFirst(rcs)
	eqInt64(t, "comment order", rcIds(ConvReview{Comments: rcs}), []int64{10, 12, 7, 7, 20, 30})

	// Resolver-set threads also count as resolved via IsResolved.
	withResolver := []api.ReviewComment{
		{ID: 41, Path: "x.go", Resolver: &api.User{Login: "r1"}},
		mkRC(40, "y.go", 1, false),
	}
	orderCommentsUnresolvedFirst(withResolver)
	if withResolver[0].ID != 40 {
		t.Fatalf("resolver thread should sort last: %v", withResolver)
	}
}
