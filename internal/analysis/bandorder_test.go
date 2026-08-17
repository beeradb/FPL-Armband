package analysis

import (
	"fmt"
	"testing"

	"armband/internal/fpl"
)

// bandTieFixtures is a round robin whose goals-per-match are chosen to put a
// three-way tie exactly on a band boundary.
//
// Each club scores a constant `rate` in every one of its matches, so its
// goals-per-match *is* that rate and its goals conceded per match is the mean of
// its opponents' — which makes both orderings a function of one number per club
// and the construction easy to reason about.
//
// The rates are 0, 1, 1, 1, 2, 3, 4, 5 for clubs 1..8. The worst three attacks
// are therefore club 1 and **two of clubs 2, 3 and 4**, which are indistinguishable
// on the sort key. That is a boundary tie: it decides band membership rather than
// merely the order inside a band, and it is the only kind that can change what the
// engine scores. Ties wholly inside a band are invisible, which is why a test that
// relies on whatever ties an archive happens to contain has no guaranteed power.
func bandTieFixtures() []fpl.Fixture {
	rate := map[int]int{1: 0, 2: 1, 3: 1, 4: 1, 5: 2, 6: 3, 7: 4, 8: 5}
	var fx []fpl.Fixture
	id := 0
	for h := 1; h <= 8; h++ {
		for a := h + 1; a <= 8; a++ {
			id++
			hs, as := rate[h], rate[a]
			fx = append(fx, fpl.Fixture{
				ID: id, Finished: true,
				TeamH: h, TeamA: a,
				TeamHScore: &hs, TeamAScore: &as,
			})
		}
	}
	return fx
}

// TestBandAssignmentIsDeterministic pins `teamBands` shut, in the mould of
// TestSeedOrderIsDeterministic and for the same reason.
//
// # The defect it closes
//
// `teamBands` ranged a `map[int]*rec` into its candidate slice and ordered that
// slice with `sort.Slice`, which is not stable. Two clubs on the same
// goals-per-match — common early, being a small integer over the same small
// number of matches — were therefore separated by Go's randomised map iteration
// order, and *which* of them landed in the bottom or top three changed from one
// run to the next.
//
// This is the third instance of the class in this repository, after `Optimize`'s
// map-ordered bench fill and `newTeamFormIndex`. The first is pinned by
// TestSeedOrderIsDeterministic; this path had no equivalent, and the recorded
// cost of that was not hypothetical: two replays of one sweep at one commit with
// `band_strength 1` differed in 3 of 36 `hold_points` cells, 12 of 36
// `policy_points`, and 7 each on moves and hits. Decisions, not only scores.
//
// # Why it observes the cause rather than a consequence
//
// It asserts on the band assignment itself, not on a score or a squad that a band
// change may or may not move. TestSeedOrderIsDeterministic's own comment records
// why: the original optimiser reproducer watched the final fifteen, and it stopped
// reproducing mid-investigation when an unrelated pool change reset the landscape,
// leaving a census that returned a clean null from a harness never shown to detect
// anything. Watch the cause and no later change can silence it.
//
// # Why the fixtures are constructed rather than loaded
//
// The boundary tie is built in, so this test is a positive control for itself: the
// guard below fails if the construction ever stops containing the tie it is named
// for. An archive-driven version would pass whenever the season it happened to load
// had no boundary tie — 2024-25 at cutoff 7 is exactly such a season, with interior
// ties and no boundary one — and would then be asserting nothing while looking
// green. It also needs no network and no archive, so it never skips.
func TestBandAssignmentIsDeterministic(t *testing.T) {
	fx := bandTieFixtures()

	// The positive control. Three clubs tie on the sort key at the boundary
	// between the worst three attacks and the middle, so the assignment is
	// underdetermined by the key alone and only a total order can settle it.
	// If this ever stops holding, the test below is vacuous and says so here
	// rather than passing quietly.
	{
		e := &Engine{Fixtures: fx}
		b := e.teamBands()
		if !b.ready {
			t.Fatal("no bands were produced from the constructed fixtures, so this " +
				"test would assert determinism over an empty assignment")
		}
		tied := 0
		for _, id := range []int{2, 3, 4} {
			if b.attack[id] == bandWorst {
				tied++
			}
		}
		if tied != bandSize-1 {
			t.Fatalf("the constructed boundary tie is gone: %d of clubs 2, 3, 4 are in "+
				"the worst attack band, want %d. Without a tie ON a band boundary this "+
				"test cannot detect the defect it exists for.", tied, bandSize-1)
		}
	}

	// Fresh engines, because teamBands caches behind a sync.Once — a second call
	// on one engine returns the first call's answer and would pass whatever the
	// ordering did.
	const runs = 16
	seen := map[string]int{}
	for i := 0; i < runs; i++ {
		e := &Engine{Fixtures: fx}
		b := e.teamBands()
		s := ""
		for id := 1; id <= 8; id++ {
			s += fmt.Sprintf("%d:a%d/d%d;", id, b.attack[id], b.defence[id])
		}
		seen[s]++
	}

	t.Logf("%d identical teamBands calls: %d distinct band assignments", runs, len(seen))

	if len(seen) != 1 {
		t.Errorf("teamBands produced %d distinct band assignments from byte-identical "+
			"fixtures across %d calls, so the attack/defence bands are non-deterministic "+
			"again.\n\n"+
			"Look at how the candidate slice is built and ordered in bands.go. It must "+
			"be a TOTAL order: sort.SliceStable alone does not fix this, because "+
			"stability preserves the input order and the input order is the randomised "+
			"map one. Ties break on club id.\n\n"+
			"This matters beyond one squad. `FPL_WEIGHT=band=1` reaches a live "+
			"`armband review`, so a user can reach it; and every byte-identical "+
			"invariance in the research record is only evidence if what produced it "+
			"was deterministic.", len(seen), runs)
	}
}

// TestBandTiesBreakTowardTheLowerClubID pins the tie-break itself, which is the
// one part of the ordering a reader cannot infer from the band sizes.
//
// Separate from the test above on purpose: that one asserts the invariant that
// matters (the same input gives the same answer), this one records WHICH total
// order was chosen. Any total order would satisfy determinism, so changing this
// one is a legitimate decision — but it re-bands clubs, so it must be a decision
// rather than a side effect of an edit, and this test is what makes it one.
func TestBandTiesBreakTowardTheLowerClubID(t *testing.T) {
	e := &Engine{Fixtures: bandTieFixtures()}
	b := e.teamBands()

	// Clubs 2, 3 and 4 all score exactly one goal a match. Two of the three join
	// club 1 in the worst attack band; the lower ids win.
	if b.attack[2] != bandWorst || b.attack[3] != bandWorst || b.attack[4] != bandMiddle {
		t.Errorf("clubs 2, 3, 4 tie on goals per match and landed attack bands "+
			"%d/%d/%d; want the two lowest ids (2 and 3) in the worst band and 4 in "+
			"the middle. The tie-break is club id, ascending.",
			b.attack[2], b.attack[3], b.attack[4])
	}
}
