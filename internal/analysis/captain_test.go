package analysis

import (
	"math"
	"testing"
)

// mkSquad builds a legal 15-man shape from per-position score lists, so the
// captaincy behaviour can be tested without the live API.
func mkSquad(gk, def, mid, fwd []float64) []PlayerMetrics {
	var out []PlayerMetrics
	id := 1
	add := func(pos string, scores []float64) {
		for _, s := range scores {
			out = append(out, PlayerMetrics{
				ID: id, Name: pos, Position: pos, Team: "T", Price: 4.0, Score: s,
			})
			id++
		}
	}
	add("GKP", gk)
	add("DEF", def)
	add("MID", mid)
	add("FWD", fwd)
	return out
}

func TestXIValueIsSumPlusCaptain(t *testing.T) {
	// Sum, plus the captain again, plus the vice-captain at his probability of
	// inheriting the armband. The last term is what FPL pays when the captain
	// records no minutes.
	pick := []PlayerMetrics{{Score: 2}, {Score: 7}, {Score: 3}}
	want := 12 + 7.0 + ViceCaptainWeight*3
	if got := xiValue(pick); math.Abs(got-want) > 1e-9 {
		t.Errorf("xiValue = %v, want %v (sum 12, captain 7, vice 3 at %.2f)",
			got, want, ViceCaptainWeight)
	}
	if got := xiValue(nil); got != 0 {
		t.Errorf("xiValue(nil) = %v, want 0", got)
	}
}

// TestObjectivePrefersAPeakOverAFlatSquad is the point of the term. Both squads
// field elevens totalling the same, but one is built around a single high
// scorer who will wear the armband every week. Before the captaincy term the
// optimiser scored these identically.
func TestObjectivePrefersAPeakOverAFlatSquad(t *testing.T) {
	// Eleven starters totalling 20.0 in both cases; bench zeroed so bestXI has
	// no discretion.
	peaked := mkSquad(
		[]float64{1, 0},
		[]float64{1, 1, 1, 0, 0},
		[]float64{10, 1, 1, 1, 1},
		[]float64{1, 1, 0},
	)
	flat := mkSquad(
		[]float64{1.8181818, 0},
		[]float64{1.8181818, 1.8181818, 1.8181818, 0, 0},
		[]float64{1.8181818, 1.8181818, 1.8181818, 1.8181818, 1.8181818},
		[]float64{1.8181818, 1.8181818, 0},
	)

	xiP, _, _ := bestXI(peaked)
	xiF, _, _ := bestXI(flat)
	sum := func(ps []PlayerMetrics) float64 {
		var s float64
		for _, p := range ps {
			s += p.Score
		}
		return s
	}
	if math.Abs(sum(xiP)-sum(xiF)) > 1e-3 {
		t.Fatalf("test setup: elevens should total the same, got %.4f and %.4f", sum(xiP), sum(xiF))
	}

	op, of := objective(peaked, 0, false), objective(flat, 0, false)
	if op <= of {
		t.Errorf("peaked squad scored %.4f, flat scored %.4f — the captaincy term is not applied", op, of)
	}
	// The gap is the captain difference, less whatever the vice-captain term
	// gives back to the flat squad — its second-best is higher, which is exactly
	// the hedge that term exists to price. Assert the captain difference
	// dominates rather than pinning arithmetic that moves whenever the weight
	// changes.
	d := op - of
	captainGap := 10 - 1.8181818
	if d <= 0 || d > captainGap {
		t.Errorf("gap %.4f is not between 0 and the captain difference %.4f",
			d, captainGap)
	}
	if captainGap-d > ViceCaptainWeight*10 {
		t.Errorf("gap %.4f falls short of the captain difference %.4f by more than "+
			"the vice term could account for", d, captainGap)
	}
}

// TestCaptainIsTheHighestScorerInTheXI — the armband is worth double, so it can
// only ever go to the best starter. bestXI returns the eleven sorted, and
// Optimize relies on that when assigning it.
func TestCaptainIsTheHighestScorerInTheXI(t *testing.T) {
	e := roleEngine(t, DefaultWeights(), DefaultRoleRisk())
	sq, err := e.Optimize(OptimizeRequest{Budget: DefaultBudget})
	if err != nil {
		t.Fatalf("optimize: %v", err)
	}
	for _, p := range sq.StartingXI {
		if p.Score > sq.Captain.Score+1e-9 {
			t.Errorf("%s scores %.2f but the armband went to %s on %.2f",
				p.Name, p.Score, sq.Captain.Name, sq.Captain.Score)
		}
	}
	if sq.ViceCaptain.ID == sq.Captain.ID {
		t.Error("captain and vice-captain are the same player")
	}
}

func TestExpectedPointsCountsTheCaptainTwice(t *testing.T) {
	e := roleEngine(t, DefaultWeights(), DefaultRoleRisk())
	sq, err := e.Optimize(OptimizeRequest{Budget: DefaultBudget})
	if err != nil {
		t.Fatalf("optimize: %v", err)
	}
	want := sq.XIScore + sq.Captain.Score
	if math.Abs(sq.ExpectedPoints-want) > 1e-9 {
		t.Errorf("ExpectedPoints = %.4f, want %.4f (XI %.4f plus captain %.4f)",
			sq.ExpectedPoints, want, sq.XIScore, sq.Captain.Score)
	}
	if sq.ExpectedPoints <= sq.XIScore {
		t.Errorf("ExpectedPoints %.2f is not above the plain XI total %.2f", sq.ExpectedPoints, sq.XIScore)
	}
}

// TestCaptainTermIsConstantAcrossFormations records why bestXI's choice does not
// change: every position starts at least one player, so the squad's best is in
// the eleven whatever the shape. If xiMin ever allows a position to field zero,
// this stops holding and bestXI genuinely has to optimise sum-plus-max.
//
// **The vice-captain term is not constant this way.** The squad's second-best
// need not be in the eleven — a formation can leave him out — so a shape that
// seats him is worth ViceCaptainWeight more. bestXIWith already handles it by
// maximising xiValue across formations rather than a plain total, which is why
// nothing broke when the term was added; do not "optimise" that back into a
// sum.
func TestCaptainTermIsConstantAcrossFormations(t *testing.T) {
	for pos, n := range xiMin {
		if n < 1 {
			t.Fatalf("xiMin[%s] = %d; with a position able to field nobody, the captain "+
				"is no longer guaranteed to be in the XI and bestXI needs revisiting", pos, n)
		}
	}
}
