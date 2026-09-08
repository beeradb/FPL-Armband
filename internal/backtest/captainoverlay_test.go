package backtest

// Pure overlay helpers and unit tests for the captaincy HOLD overlays.
//
// shrinkXIScores and the week-engine rebuild are the shared arithmetic behind
// TestDiagCaptainJamesStein. They live here so the unit tests below pin them
// without DIAG=1, the live API, or the archive — a diagnostic must never carry
// an unchecked copy of the thing it is checking, and λ=0 identity is that check.

import (
	"math"
	"testing"

	"armband/internal/analysis"
)

// shrinkXIScores returns a copy of xi with each Score replaced by
// (1−λ)·Score + λ·mean(XI Scores). The mean of the result equals the mean of
// the input for any λ; λ=0 is the identity; λ=1 is the constant-mean vector.
//
// Asserted form, not classical James–Stein: σ² is not estimated (would leak
// realised points or fit in-sample). See the 2026-09-08 prereg.
func shrinkXIScores(xi []analysis.PlayerMetrics, lambda float64) []analysis.PlayerMetrics {
	out := append([]analysis.PlayerMetrics(nil), xi...)
	if len(out) == 0 {
		return out
	}
	var sum float64
	for _, p := range out {
		sum += p.Score
	}
	mean := sum / float64(len(out))
	for i := range out {
		out[i].Score = (1-lambda)*out[i].Score + lambda*mean
	}
	return out
}

// xiMetricsFromIDs builds PlayerMetrics for xiIDs in slice order, so
// CaptainAndVice's strict-> tiebreak keeps the same earlier-element convention
// HoldCaptaincyWeekly already scored.
func xiMetricsFromIDs(e *analysis.Engine, xiIDs []int) []analysis.PlayerMetrics {
	out := make([]analysis.PlayerMetrics, 0, len(xiIDs))
	for _, id := range xiIDs {
		el := e.Boot.ElementByID(id)
		if el == nil {
			continue
		}
		out = append(out, e.Metrics(el))
	}
	return out
}

// holdWeekEngine rebuilds the weekly engine HoldCaptaincyWeekly uses at gw.
// Shipped HOLD (#205) sets Horizon 1 on a copy of cfg.Weights; the opening
// fifteen and the frozen captain still use cfg.Weights. Tiebreak travels with
// the engine. The overlay recomputes only the armband; the identity check
// guards this copy. Banked 2026-09-08 A0/C/F cells predate #205 (horizon-5
// fielding at 4f5aa59c).
func holdWeekEngine(cur, prior *Season, sc SimConfig, gw int) *analysis.Engine {
	idx := sc.priors(cur, prior)
	b, fx := PointInTimeWith(cur, prior, gw-1, sc.Oracles)
	w := sc.Weights
	w.Horizon = 1
	e := analysis.NewEngineFull(b, fx, w, analysis.Congestion{}, analysis.RoleRisk{})
	e.Priors = idx
	e.Recent = sc.recentIndex(cur, gw-1)
	e.TeamForm = newTeamFormIndex(cur, gw-1)
	e.Tiebreak = sc.Tiebreak
	return e
}

func TestShrinkXIScores(t *testing.T) {
	xi := make([]analysis.PlayerMetrics, 11)
	var wantMean float64
	for i := range xi {
		xi[i] = analysis.PlayerMetrics{ID: i + 1, Score: float64(i) + 0.25}
		wantMean += xi[i].Score
	}
	wantMean /= 11

	got0 := shrinkXIScores(xi, 0)
	if len(got0) != 11 {
		t.Fatalf("λ=0 length %d, want 11", len(got0))
	}
	for i := range xi {
		if got0[i].Score != xi[i].Score || got0[i].ID != xi[i].ID {
			t.Fatalf("λ=0 is not identity at i=%d: got id=%d score=%v, want id=%d score=%v",
				i, got0[i].ID, got0[i].Score, xi[i].ID, xi[i].Score)
		}
	}

	got1 := shrinkXIScores(xi, 1)
	for i := range got1 {
		if math.Abs(got1[i].Score-wantMean) > 1e-12 {
			t.Fatalf("λ=1 score[%d]=%v, want constant mean %v", i, got1[i].Score, wantMean)
		}
		if got1[i].ID != xi[i].ID {
			t.Fatalf("λ=1 must preserve ids; i=%d got %d want %d", i, got1[i].ID, xi[i].ID)
		}
	}

	for _, lam := range []float64{0, 0.25, 0.5, 0.75, 1} {
		got := shrinkXIScores(xi, lam)
		var m float64
		for _, p := range got {
			m += p.Score
		}
		m /= float64(len(got))
		if math.Abs(m-wantMean) > 1e-12 {
			t.Fatalf("λ=%g mean %v, want preserved %v", lam, m, wantMean)
		}
	}

	// Input must be untouched — the overlay copies before shrinking.
	if xi[10].Score != 10.25 {
		t.Fatalf("shrink mutated the input: xi[10].Score=%v", xi[10].Score)
	}
}

func TestCaptainOverlayUnshrunkEqualsWalk(t *testing.T) {
	// Unique max at index 3; second at index 7. Slice order is the walk's
	// tiebreak, so a hand walk that picks by scanning must agree.
	xi := make([]analysis.PlayerMetrics, 11)
	for i := range xi {
		xi[i] = analysis.PlayerMetrics{ID: 100 + i, Score: float64(i) * 0.1}
	}
	xi[3].Score = 9.0
	xi[7].Score = 8.0

	cap, vice := analysis.CaptainAndVice(xi)
	if cap.ID != 103 || vice.ID != 107 {
		t.Fatalf("CaptainAndVice = (%d,%d), want (103,107)", cap.ID, vice.ID)
	}
	unshrunk := shrinkXIScores(xi, 0)
	c0, v0 := analysis.CaptainAndVice(unshrunk)
	if c0.ID != cap.ID || v0.ID != vice.ID {
		t.Fatalf("λ=0 walk (%d,%d) disagrees with raw (%d,%d)", c0.ID, v0.ID, cap.ID, vice.ID)
	}
}

func TestCaptainOverlayHalfShrinkPreservesArgmax(t *testing.T) {
	// A common-mean convex combination Score' = (1−λ)Score + λμ is strictly
	// order-preserving for λ ∈ [0, 1): Score'_i − Score'_j = (1−λ)(Score_i − Score_j).
	// So the argmax cannot move on a close pack or a far-clear pack — that is the
	// mechanism, and the diag's weeks_captain_differs VOID check is what catches a
	// run that therefore never bit. λ=1 is excluded from the arms for the same
	// reason (everyone ties; CaptainAndVice captains the first slice element).
	close := make([]analysis.PlayerMetrics, 11)
	for i := range close {
		close[i] = analysis.PlayerMetrics{ID: i + 1, Score: 4.0 + 0.1*float64(i)}
	}
	// Leader only a tenth ahead of second.
	close[10].Score = 5.05
	close[9].Score = 4.95
	rawClose, rawVice := analysis.CaptainAndVice(close)
	shrunkClose := shrinkXIScores(close, 0.5)
	sClose, sVice := analysis.CaptainAndVice(shrunkClose)
	if sClose.ID != rawClose.ID || sVice.ID != rawVice.ID {
		t.Fatalf("λ=0.5 reordered a close pack: raw (%d,%d) shrunk (%d,%d) — "+
			"common-mean shrink must not change the ranking for λ<1",
			rawClose.ID, rawVice.ID, sClose.ID, sVice.ID)
	}

	clear := make([]analysis.PlayerMetrics, 11)
	for i := range clear {
		clear[i] = analysis.PlayerMetrics{ID: i + 1, Score: 3.0}
	}
	clear[0].Score = 20.0
	rawClear, _ := analysis.CaptainAndVice(clear)
	shrunkClear := shrinkXIScores(clear, 0.5)
	sClear, _ := analysis.CaptainAndVice(shrunkClear)
	if rawClear.ID != 1 || sClear.ID != 1 {
		t.Fatalf("far-clear leader must survive λ=0.5: raw=%d shrunk=%d", rawClear.ID, sClear.ID)
	}

	// At λ=1 the walk collapses to slice order (first element), which is why
	// λ=1 is not an arm.
	flat := shrinkXIScores(clear, 1)
	flatCap, _ := analysis.CaptainAndVice(flat)
	if flatCap.ID != clear[0].ID {
		t.Fatalf("λ=1 must captain the first slice element, got %d want %d",
			flatCap.ID, clear[0].ID)
	}
}
