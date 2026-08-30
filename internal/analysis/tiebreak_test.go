package analysis

import (
	"math"
	"testing"
)

func pm(id int, pos string, score, own, price float64) PlayerMetrics {
	return PlayerMetrics{ID: id, Position: pos, Score: score, Ownership: own, Price: price}
}

// The defining property: the nudge may reorder players inside the band and must
// never reorder players outside it. That is the whole reason it is a bounded
// additive term rather than a comparator.
func TestTiebreakNeverOverturnsAGapWiderThanTheBand(t *testing.T) {
	const band = 0.41
	// b is far below a on Score and far above him on ownership. If the nudge can
	// overturn a gap this size the policy is no longer a tiebreak, it is a
	// re-weighting of the objective.
	all := []PlayerMetrics{
		pm(1, "MID", 6.00, 0.1, 5.0),
		pm(2, "MID", 6.00-band-0.001, 90.0, 5.0),
	}
	applyTiebreak(all, Tiebreak{Signal: TiebreakOwnership, Band: band})
	if all[1].Score >= all[0].Score {
		t.Fatalf("a %.3f gap was overturned by the tiebreak (%.4f vs %.4f); the "+
			"nudge must be strictly smaller than the band",
			band+0.001, all[1].Score, all[0].Score)
	}
}

func TestTiebreakDoesReorderInsideTheBand(t *testing.T) {
	const band = 0.41
	all := []PlayerMetrics{
		pm(1, "MID", 6.00, 0.1, 5.0),
		pm(2, "MID", 5.99, 90.0, 5.0),
	}
	applyTiebreak(all, Tiebreak{Signal: TiebreakOwnership, Band: band})
	if all[1].Score <= all[0].Score {
		t.Fatalf("a 0.01 gap was NOT overturned by a 90-point ownership edge "+
			"(%.4f vs %.4f); inside the band is where the policy is supposed to act",
			all[1].Score, all[0].Score)
	}
}

// Off is off. The shipped default must leave every number untouched, or every
// existing measurement silently moves.
func TestTiebreakOffChangesNothing(t *testing.T) {
	for _, tb := range []Tiebreak{
		TiebreakOff,
		{Signal: TiebreakOwnership, Band: 0},  // no band
		{Signal: "", Band: 0.41},              // no signal
		{Signal: "somethingelse", Band: 0.41}, // unknown signal
	} {
		all := []PlayerMetrics{pm(1, "MID", 6.0, 1, 5), pm(2, "MID", 5.9, 50, 9)}
		before := []float64{all[0].Score, all[1].Score}
		if n := applyTiebreak(all, tb); len(n) != 0 {
			t.Errorf("%+v returned %d nudges; it must return none", tb, len(n))
		}
		for i := range all {
			if all[i].Score != before[i] {
				t.Errorf("%+v moved a Score: %.6f -> %.6f", tb, before[i], all[i].Score)
			}
		}
	}
}

// Positions are separate pools. A keeper owned by 40% and a forward owned by 40%
// are not competing for a slot, and the record is explicit that the pooled
// reading of this channel was a position artefact.
func TestTiebreakRanksWithinPositionNotAcrossThem(t *testing.T) {
	all := []PlayerMetrics{
		pm(1, "GKP", 4.0, 10, 4.5),
		pm(2, "GKP", 4.0, 20, 5.0),
		pm(3, "FWD", 8.0, 30, 12.0),
		pm(4, "FWD", 8.0, 40, 13.0),
	}
	n := applyTiebreak(all, Tiebreak{Signal: TiebreakOwnership, Band: 0.4})
	// The lower-owned member of EACH position gets the zero nudge. If ranking were
	// pooled, both keepers would sit at the bottom and only player 1 would be 0.
	if n[1] != 0 && n[1] != 0.0 {
		t.Errorf("lowest-owned GKP took a nudge of %v, so ranking is pooled", n[1])
	}
	if got, ok := n[3]; ok && got != 0 {
		t.Errorf("lowest-owned FWD took a nudge of %v, so ranking is pooled across "+
			"positions rather than within them", got)
	}
	if n[2] <= 0 || n[4] <= 0 {
		t.Errorf("the higher-owned member of a position took no nudge: GKP %v, FWD %v",
			n[2], n[4])
	}
}

// Players the signal cannot separate must not be separated. Most of a positional
// pool sits at near-zero ownership, and breaking those ties by pool order would
// be a hidden dependence on element id dressed up as a policy.
func TestTiebreakLeavesEqualSignalsTied(t *testing.T) {
	all := []PlayerMetrics{
		pm(1, "MID", 5.0, 2.0, 5.0),
		pm(2, "MID", 5.0, 2.0, 5.0),
		pm(3, "MID", 5.0, 2.0, 5.0),
		pm(4, "MID", 5.0, 9.0, 5.0),
	}
	n := applyTiebreak(all, Tiebreak{Signal: TiebreakOwnership, Band: 0.4})
	if n[1] != n[2] || n[2] != n[3] {
		t.Errorf("three players on identical ownership got different nudges: "+
			"%v / %v / %v", n[1], n[2], n[3])
	}
	if n[4] <= n[1] {
		t.Errorf("the distinctly higher-owned player did not out-rank the tied "+
			"group: %v vs %v", n[4], n[1])
	}
}

// The nudge must come back out, or the arm with the tiebreak on reports a higher
// expected-points figure for free and every A/B against it is invalid.
func TestRemoveTiebreakRestoresTheExactScore(t *testing.T) {
	all := []PlayerMetrics{
		pm(1, "MID", 6.0, 5, 5.0), pm(2, "MID", 5.5, 50, 7.0), pm(3, "DEF", 4.5, 25, 4.5),
	}
	want := []float64{6.0, 5.5, 4.5}
	n := applyTiebreak(all, Tiebreak{Signal: TiebreakOwnership, Band: 0.41})
	if len(n) == 0 {
		t.Fatal("no nudge applied, so this proves nothing")
	}
	removeTiebreak(all, n)
	for i := range all {
		if math.Abs(all[i].Score-want[i]) > 1e-12 {
			t.Errorf("player %d: %.15f, want %.15f", all[i].ID, all[i].Score, want[i])
		}
	}
}

// Price is the rival signal, and the record's own six-season evidence says it is
// the better one. It must work identically.
func TestTiebreakOnPriceOrdersByPrice(t *testing.T) {
	all := []PlayerMetrics{
		pm(1, "FWD", 7.0, 50, 6.0),
		pm(2, "FWD", 7.0, 1, 11.0),
	}
	applyTiebreak(all, Tiebreak{Signal: TiebreakPrice, Band: 0.4})
	if all[1].Score <= all[0].Score {
		t.Errorf("on the price signal the £11.0m player did not out-rank the "+
			"£6.0m one at equal Score: %.4f vs %.4f", all[1].Score, all[0].Score)
	}
}

// The haul signal ranks on the caller-supplied ceiling table, and is inert
// without one — a missing table must not quietly make this the baseline while
// still reporting as an arm.
func TestTiebreakOnHaulRanksOnTheSuppliedTable(t *testing.T) {
	all := []PlayerMetrics{
		pm(1, "MID", 6.00, 50, 9.0), // high owned, dear, LOW ceiling
		pm(2, "MID", 5.99, 1, 5.0),  // low owned, cheap, HIGH ceiling
	}
	tb := Tiebreak{Signal: TiebreakHaul, Band: 0.41, HaulRate: map[int]float64{1: 0.05, 2: 0.40}}
	applyTiebreak(all, tb)
	if all[1].Score <= all[0].Score {
		t.Errorf("the higher-ceiling player did not overturn a 0.01 gap: %.4f vs %.4f",
			all[1].Score, all[0].Score)
	}
}

func TestTiebreakOnHaulIsInertWithoutATable(t *testing.T) {
	all := []PlayerMetrics{pm(1, "MID", 6.0, 1, 5), pm(2, "MID", 5.9, 50, 9)}
	before := []float64{all[0].Score, all[1].Score}
	if n := applyTiebreak(all, Tiebreak{Signal: TiebreakHaul, Band: 0.41}); len(n) != 0 {
		t.Errorf("a haul tiebreak with no HaulRate table applied %d nudges; it must "+
			"apply none rather than rank every player at zero", len(n))
	}
	for i := range all {
		if all[i].Score != before[i] {
			t.Errorf("score moved without a table: %.6f -> %.6f", before[i], all[i].Score)
		}
	}
}

// A missing id is a zero ceiling, not a missing value: no prior-season history is
// not evidence of one.
func TestTiebreakOnHaulTreatsAMissingIDAsZero(t *testing.T) {
	all := []PlayerMetrics{pm(1, "MID", 6.0, 1, 5), pm(2, "MID", 6.0, 1, 5)}
	n := applyTiebreak(all, Tiebreak{Signal: TiebreakHaul, Band: 0.4,
		HaulRate: map[int]float64{2: 0.30}}) // 1 absent
	if n[1] != 0 {
		t.Errorf("the player absent from the table took a nudge of %v, want 0", n[1])
	}
	if n[2] <= 0 {
		t.Errorf("the player with a 0.30 ceiling took no nudge")
	}
}
