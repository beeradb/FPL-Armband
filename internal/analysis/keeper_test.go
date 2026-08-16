package analysis

import (
	"math"
	"testing"

	"armband/internal/fpl"
)

func TestPoissonFloorDivMatchesKnownValues(t *testing.T) {
	cases := []struct {
		d    int
		lam  float64
		want float64
	}{
		// Computed independently; verified by hand for d=2, lambda=1.
		{2, 0, 0},
		{2, 0.5, 0.09196986},
		{2, 1.0, 0.28383382},
		{2, 1.35, 0.44180138},
		{2, 2.0, 0.75457891},
		{3, 2.0, 0.34012601},
		{3, 3.0, 0.66460291},
		{3, 4.5, 1.16622806},
	}
	for _, c := range cases {
		if got := poissonFloorDiv(c.d, c.lam); math.Abs(got-c.want) > 1e-5 {
			t.Errorf("poissonFloorDiv(%d, %g) = %.6f, want %.6f", c.d, c.lam, got, c.want)
		}
	}
}

// TestPoissonFloorDivIsBelowNaiveDivision is the regression guard. Blocks reset
// each match, so the expected number of whole blocks must always be less than
// the mean divided by the block size — the gap is precisely the remainders FPL
// throws away and the old season-total division was banking.
func TestPoissonFloorDivIsBelowNaiveDivision(t *testing.T) {
	for _, d := range []int{2, 3} {
		for lam := 0.25; lam <= 8; lam += 0.25 {
			got, naive := poissonFloorDiv(d, lam), lam/float64(d)
			if got >= naive {
				t.Errorf("poissonFloorDiv(%d, %g) = %.4f, not below naive %.4f", d, lam, got, naive)
			}
			if got < 0 {
				t.Fatalf("poissonFloorDiv(%d, %g) is negative", d, lam)
			}
		}
	}
	// A side conceding about one a game should barely be deducted at all: it
	// takes two in the same match to cost a point.
	if got := poissonFloorDiv(2, 1.0); got > 0.30 {
		t.Errorf("conceding 1.0 xG per match deducts %.3f, expected well under the naive 0.5", got)
	}
}

// TestConcededPenaltyAppliesToTheRightPositions checks the deduction reaches
// keepers and defenders and nobody else — it was absent entirely, which made
// goalkeepers the worst-calibrated position in the model.
func TestConcededPenaltyAppliesToTheRightPositions(t *testing.T) {
	for _, pos := range []int{1, 2} {
		if concedeBlock[pos] != 2 {
			t.Errorf("position %d should be deducted 1 per 2 conceded, got block %d", pos, concedeBlock[pos])
		}
	}
	for _, pos := range []int{3, 4} {
		if blk, ok := concedeBlock[pos]; ok {
			t.Errorf("position %d must take no goals-conceded deduction, got block %d", pos, blk)
		}
	}

	e := roleEngine(t, DefaultWeights(), DefaultRoleRisk())
	var keepers int
	for i := range e.Boot.Elements {
		el := &e.Boot.Elements[i]
		if el.ElementType != 1 || el.Minutes < 900 {
			continue
		}
		keepers++
		m := e.Metrics(el)
		if m.XGC90 <= 0 {
			continue
		}
		// The deduction must actually bite: a keeper's base rate has to sit
		// below what he would score without it.
		withoutPenalty := m.BaseXP90 + poissonFloorDiv(2, m.XGC90)
		if withoutPenalty <= m.BaseXP90 {
			t.Errorf("%s: goals-conceded deduction is not being applied", el.WebName)
		}
	}
	if keepers == 0 {
		t.Skip("no goalkeepers with enough minutes")
	}
}

// TestConcededPenaltyScalesWithFixtures confirms the deduction is treated as
// opponent-dependent, like the clean sheet it mirrors, rather than being
// carried across the fixture adjustment unchanged.
func TestConcededPenaltyScalesWithFixtures(t *testing.T) {
	e := roleEngine(t, DefaultWeights(), DefaultRoleRisk())
	var el *fpl.Element
	for i := range e.Boot.Elements {
		c := &e.Boot.Elements[i]
		if c.ElementType == 1 && c.Minutes >= 2000 {
			el = c
			break
		}
	}
	if el == nil {
		t.Skip("no established goalkeeper")
	}
	m := e.Metrics(el)
	if m.XGC90 <= 0 {
		t.Skip("no expected goals conceded for this keeper")
	}

	easy := []FixtureBrief{{Difficulty: 2}, {Difficulty: 2}}
	hard := []FixtureBrief{{Difficulty: 5}, {Difficulty: 5}}
	if e.fixtureAdjustedXP90(el, m, hard) >= e.fixtureAdjustedXP90(el, m, easy) {
		t.Error("a goalkeeper facing difficulty-5 fixtures did not score below one facing difficulty-2")
	}
}
