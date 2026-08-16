package analysis

import (
	"testing"

	"armband/internal/fpl"
)

// TestTeamXGCFactorMovesEveryDefensiveTerm — a club-level correction has to
// reach the clean sheet, the goals-conceded block and the keeper's saves
// together, because all three are computed from the same expected goals
// conceded. Applying it to the answer instead of the input would move one and
// leave the others describing a defence that no longer exists.
func TestTeamXGCFactorMovesEveryDefensiveTerm(t *testing.T) {
	e := testEngine(t)

	// A defender at a club with a real defensive record.
	var def *fpl.Element
	for i := range e.Boot.Elements {
		el := &e.Boot.Elements[i]
		if el.ElementType == 2 && el.Minutes > 2000 {
			def = el
			break
		}
	}
	if def == nil {
		t.Skip("no established defender in the pool")
	}

	before := e.Metrics(def)
	if before.XGC90 <= 0 {
		t.Skip("no expected goals conceded for this defender")
	}

	// A leakier defence than the record shows.
	e.TeamXGCFactor = map[int]float64{def.Team: 1.25}
	after := e.Metrics(def)

	if got, want := after.XGC90, before.XGC90*1.25; !nearly(got, want) {
		t.Errorf("xGC90 %.4f, want %.4f", got, want)
	}
	if after.TeamXGCFactor != 1.25 {
		t.Errorf("factor reported as %.2f, want 1.25", after.TeamXGCFactor)
	}
	// More goals conceded is unambiguously worse for a defender: fewer clean
	// sheets and more of the two-goal block.
	if !(after.Score < before.Score) {
		t.Errorf("a leakier defence scored %.3f against %.3f before", after.Score, before.Score)
	}

	// A factor of 1 and an unset factor must both be exact no-ops, since the
	// zero value of the map is what every caller that never sets one gets.
	e.TeamXGCFactor = map[int]float64{def.Team: 1}
	if got := e.Metrics(def); !nearly(got.XGC90, before.XGC90) || got.TeamXGCFactor != 0 {
		t.Errorf("factor 1 changed xGC90 to %.4f (reported %.2f)", got.XGC90, got.TeamXGCFactor)
	}
	e.TeamXGCFactor = nil
	if got := e.Metrics(def); !nearly(got.XGC90, before.XGC90) {
		t.Errorf("unset factor changed xGC90 to %.4f", got.XGC90)
	}
}

func nearly(a, b float64) bool { return a-b < 1e-9 && b-a < 1e-9 }
