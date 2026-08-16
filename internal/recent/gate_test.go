package recent

import (
	"testing"

	"armband/internal/analysis"
	"armband/internal/fpl"
)

// TestTheBlendGateIsThinAndNonZero pins the history_past path's expression of
// the prior-blend gate against analysis.ShouldBlendPrior.
//
// This is the second of three implementations of one rule; see the identical
// test in internal/priors for why the expectation is derived from the shipped
// predicate rather than written out.
//
// # The trap this path carries and the other two do not
//
// blendPast drops zero-minute seasons from the history before it does anything
// else, because a season with no minutes contributes no rate and would only
// distort SeasonsAgo. That means hist[0] is the most recent season he PLAYED,
// which is not the most recent season on record. Gating on hist[0].Minutes —
// which is what shipped before the gate — asks "was the last season he played a
// thin one", and for a player who sat out last season entirely that question is
// about the year before, so he blends. The gate must read the last row of
// `past`, minutes or none, and the second case below is the one that catches it.
//
// # What "not blended" means here, since it differs by path
//
// hist[0] unchanged — exactly what halfLife <= 0 returns a few lines above. So
// for the two excluded populations this path is byte-identical to the shipped
// model, which is the property that lets prior_half_life be turned on without
// touching anyone the feature is not for.
func TestTheBlendGateIsThinAndNonZero(t *testing.T) {
	const halfLife = 1.0
	season := func(name string, minutes int) fpl.PastSeason {
		return fpl.PastSeason{SeasonName: name, Minutes: minutes,
			Starts: minutes / 90, Bonus: 30, TotalPoints: 150}
	}

	for _, lastMinutes := range []int{0, 1, 90, 900, ThinSeason - 1, ThinSeason, ThinSeason + 1, 3420} {
		// Oldest first, which is the order FPL returns and the order blendPast
		// relies on. TestBlendPastReversesFPLsOrder pins that separately.
		//
		// The middle season is deliberately THIN and the oldest FULL, and both
		// halves of that matter. Full behind him means a blend has somewhere to
		// go, so a blend that should happen is visible. Thin immediately behind
		// him is what makes the zero case discriminating: if the season before
		// last were full, the pre-gate rule would decline to blend anyway — for
		// the wrong reason, because it asks about the last season he PLAYED —
		// and this test would pass over the bug it exists for.
		past := []fpl.PastSeason{
			season("2023-24", 3000),
			season("2024-25", 900),
			season("2025-26", lastMinutes),
		}
		got, ok := blendPast(past, halfLife)

		// The baseline is the SHIPPED model, reconstructed, and not
		// blendPast(past, 0). That distinction was the whole of a real bug.
		// prior_half_life at 0 does not take blendPast's halfLife <= 0 branch —
		// it stops recent.LoadPriors being called at all (cmd/armband/main.go),
		// and the prior comes from priors.Load instead: ONE season, carrying
		// Minutes 0 for a man who did not play. So comparing against
		// blendPast(past, 0) compares against a path production never executes,
		// and on the non-zero cases it compares an expression with itself —
		// `return hist[0].PriorPlayer` against `return hist[0].PriorPlayer`,
		// three lines apart, which cannot fail.
		//
		// shippedIsUsable says whether the shipped prior would reach
		// blendRates' `!ok || p.Minutes == 0` gate: it is the last season on
		// record, so a zero there means shrinkToLeague.
		shippedIsUsable := lastMinutes > 0
		s := past[len(past)-1]
		last := analysis.PriorPlayer{Minutes: s.Minutes, Starts: s.Starts, Bonus: s.Bonus}

		if analysis.ShouldBlendPrior(lastMinutes) {
			if !ok {
				t.Fatalf("last season %d minutes: blendPast reported no prior at all, "+
					"but two seasons with minutes were offered", lastMinutes)
			}
			if got.Minutes == last.Minutes {
				t.Errorf("last season %d minutes: the prior came back at %d minutes, "+
					"which is last season unchanged. He is thin but played, the single "+
					"population prior_half_life exists for, and the older seasons were "+
					"not folded in.", lastMinutes, got.Minutes)
			}
			continue
		}

		// Not blended. What must hold is that the model ends up believing the
		// same thing it believes with the setting OFF.
		usable := ok && got.Minutes > 0
		if usable != shippedIsUsable {
			t.Errorf("last season %d minutes: with the setting ON the model %s a prior, "+
				"with it OFF it %s one. A player who recorded no minutes last season "+
				"must reach shrinkToLeague either way — that is the shipped answer for "+
				"someone with no usable history, and handing him a season at least two "+
				"years old instead is the half of this feature that measured WORSE. "+
				"blendPast drops zero-minute rows, so hist[0] is the last season he "+
				"PLAYED and carries minutes, which silently defeats the gate on the one "+
				"path cmd/armband actually runs.",
				lastMinutes, usableWord(usable), usableWord(shippedIsUsable))
			continue
		}
		if usable && got != last {
			t.Errorf("last season %d minutes: prior %+v is not last season %+v. A full "+
				"season stands alone.", lastMinutes, got, last)
		}
	}
}

func usableWord(b bool) string {
	if b {
		return "has"
	}
	return "has no"
}
