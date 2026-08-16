package backtest

import (
	"testing"

	"armband/internal/analysis"
)

// TestTheHalfSeasonBarIsOneNumber is the three lines cmd/priorblend's own
// version of this test says belong to whoever owns this file.
//
// That command pins priors.ThinSeason against recent.ThinSeason and can reach no
// further, because this package's copy is unexported. It is now an alias of
// analysis.ThinSeason rather than a fourth literal, so the check is that the
// alias has not been un-aliased back into a number that can drift.
func TestTheHalfSeasonBarIsOneNumber(t *testing.T) {
	if thinPrior != analysis.ThinSeason {
		t.Fatalf("the half-season bar is %d in internal/backtest and %d in "+
			"internal/analysis. These decide the same thing for the replay and for "+
			"everything that ships, so a replay measuring one bar would be pricing a "+
			"feature nobody runs.", thinPrior, analysis.ThinSeason)
	}
}

// TestTheBlendGateIsThinAndNonZero pins the replay's expression of the
// prior-blend gate against analysis.ShouldBlendPrior.
//
// This is the third of three implementations of one rule — the others are in
// internal/priors and internal/recent, each with a test of this name — and it is
// the one every recorded measurement of prior_half_life came through. If the
// three disagree, the replay prices a feature the live command does not run.
//
// # What "not blended" means on this path, since it differs from the other two
//
// Two different things, and the difference is the point:
//
//   - A FULL last season becomes his prior, unchanged.
//   - A last season of NO minutes leaves him OUT OF THE INDEX ALTOGETHER, so
//     priorIndex.Get returns false and blendRates sends him to shrinkToLeague —
//     the shipped answer for a player with no usable history. That is exactly
//     what the halfLife <= 0 branch does with him (`q.Minutes > 0`), which is the
//     property being pinned: turning prior_half_life on must not move a player
//     the feature is not for.
//
// Blending him instead hands him a season at least two years old. Measured on
// six seasons of one-gameweek-ahead prediction, that population's bias crosses
// from under-prediction into OVER-prediction while the thin-but-played
// population's under-prediction shrinks by about two thirds — opposite
// directions, which is why one gate separates them rather than one setting
// covering both.
func TestTheBlendGateIsThinAndNonZero(t *testing.T) {
	const (
		code     = 4242
		halfLife = 1.0
	)
	season := func(name string, minutes int) *Season {
		return &Season{Name: name, Players: map[int]*Player{
			1: {ID: 1, Code: code, WebName: "Test", Minutes: minutes,
				Starts: minutes / 90, Bonus: 30, TotalPoints: 150, XG: 8},
		}}
	}

	for _, lastMinutes := range []int{0, 1, 90, 900, thinPrior - 1, thinPrior, thinPrior + 1, 3420} {
		// Most recent first here, unlike FPL's history_past. The middle season is
		// thin and the oldest full for the reason given in internal/recent's copy
		// of this test: a full season immediately behind him would let the wrong
		// rule decline to blend for the wrong reason.
		seasons := []*Season{
			season("2025-26", lastMinutes),
			season("2024-25", 900),
			season("2023-24", 3000),
		}
		got, inIndex := newPriorIndexMulti(seasons, halfLife).Get(code)
		flat, inFlat := newPriorIndexMulti(seasons, 0).Get(code)

		if analysis.ShouldBlendPrior(lastMinutes) {
			if !inIndex {
				t.Errorf("last season %d minutes: no prior at all. He is thin but "+
					"played, which is the population prior_half_life exists for.", lastMinutes)
				continue
			}
			if inFlat && *got == *flat {
				t.Errorf("last season %d minutes: the blended prior is identical to the "+
					"unblended one, so the older seasons were not folded in.", lastMinutes)
			}
			continue
		}

		if lastMinutes == 0 {
			if inIndex {
				t.Errorf("last season %d minutes: he came back with a prior of %+v. He "+
					"must be absent from the index, exactly as prior_half_life 0 leaves "+
					"him, so blendRates' `!ok || p.Minutes == 0` sends him to "+
					"shrinkToLeague instead of to a season two years old.", lastMinutes, *got)
			}
			if inFlat {
				t.Errorf("last season %d minutes: the UNBLENDED index carries him too, so "+
					"this test is not comparing against the shipped answer and its "+
					"premise has gone.", lastMinutes)
			}
			continue
		}
		if !inIndex || !inFlat {
			t.Fatalf("last season %d minutes: a played full season produced no prior "+
				"(blended %v, unblended %v)", lastMinutes, inIndex, inFlat)
		}
		if *got != *flat {
			t.Errorf("last season %d minutes: blended prior %+v differs from the "+
				"unblended %+v. A full season stands alone: it is the best evidence "+
				"there is, and smoothing an older one into it dilutes genuine "+
				"improvement, which is most players most of the time.",
				lastMinutes, *got, *flat)
		}
	}
}
