package analysis

import (
	"math"
	"testing"
)

// oneRecentPlayer is a RecentForm holding exactly one player, so a test can vary
// his blank run and nothing else.
type oneRecentPlayer struct {
	code int
	p    RecentPlayer
}

func (o oneRecentPlayer) Get(code int) (RecentPlayer, bool) {
	if code == o.code {
		return o.p, true
	}
	return RecentPlayer{}, false
}

// TestExpectedMinutesCarriesTheBlankRunDiscount pins the coupling that makes
// `blankRunFactor`'s own calibration circular, so the guard that exists because
// of it cannot quietly stop being needed.
//
// `Metrics` assigns `ExpectedMinutes` from `blendFor`'s output, and `blendRates`
// applies `blankRunFactor` on the way. So the reported minutes for a player one
// gameweek into an absence are already discounted — which means re-running
// `TestDiagAvailability`, whose whole quantity is expected-against-actual
// minutes, at shipped config fits the term to its own output.
// `TestDiagAvailabilityByPosition` refuses to run without `FPL_NO_BLANK_RUN=1`
// for exactly that reason, and this test is what keeps that refusal honest: if
// `ExpectedMinutes` ever stopped carrying the discount, the guard would be
// insisting on a switch that no longer mattered and the next calibration would
// be read against the wrong population.
//
// It asserts the RELATIONSHIP, not a level. The multiplier is whatever
// `blankRunFactor` returns, so re-tuning 0.75 leaves this passing and removing
// the coupling fails it. That is the invariant; the number is not.
//
// ⚠️ **The exact equality below holds only with no prior loaded, and that is a
// narrower fact than it looks.** `blendRates` applies `blankRunFactor` to the
// current-season minutes and *then* mixes the prior in, so with a prior present
// the effective multiplier is `w·p + (1−w)·1` rather than `p`, and it varies
// with how many gameweeks have been played. `NewEngineFull` leaves `Priors` nil,
// which is why the equality is reachable here — but `collectAvailabilityObs`
// sets `e.Priors`, so on the path the calibration actually runs the discount is
// **not** exactly divisible back out of an `ExpectedMinutes` column. The
// directional assertion above is the general one; this one pins the mechanism at
// a configuration where the arithmetic is clean, and the two are labelled apart
// so nobody reads the second as licence for the first.
func TestExpectedMinutesCarriesTheBlankRunDiscount(t *testing.T) {
	if !blankRunAdjust {
		t.Skip("FPL_NO_BLANK_RUN is set, so the coupling is deliberately absent")
	}
	// Two more switches can make the term a no-op without disabling it, and
	// without this the failure below would blame blendRates for an env var.
	// envDefaultAbove accepts both, so neither is caught by blankRunAdjust.
	if e := (&Engine{}); e.blankRunFactor(1) == 1 {
		t.Skip("FPL_BLANK_RUN_PENALTY or FPL_BLANK_RUN_MAX makes the term inert")
	}
	e := roleEngine(t, DefaultWeights(), DefaultRoleRisk())
	// Part-way through a season, so the recency index is consulted at all:
	// blendRates reads e.Recent only once GameweeksPlayed() > 0.
	playGameweeks(t, e, 20)
	el := findEverPresent(t, e)

	// The same recent form twice, differing only in the trailing blank run.
	// Matches must be > 0 or the index entry is ignored.
	form := RecentPlayer{MinutesPerMatch: 88, StartShare: 0.95, Matches: 20}

	form.BlankRun = 0
	e.Recent = oneRecentPlayer{code: el.Code, p: form}
	settled := e.Metrics(el).ExpectedMinutes

	form.BlankRun = 1
	e.Recent = oneRecentPlayer{code: el.Code, p: form}
	blanked := e.Metrics(el).ExpectedMinutes

	if settled <= 0 {
		t.Fatalf("a settled ever-present reported %v expected minutes; the "+
			"recency index is not reaching Metrics at all, so this test is "+
			"asserting nothing", settled)
	}
	if blanked >= settled {
		t.Fatalf("one trailing blank left ExpectedMinutes at %v against %v "+
			"settled. blendRates has stopped applying blankRunFactor, or "+
			"Metrics has stopped reporting the blended figure — either way the "+
			"minutes calibration is no longer measuring the model's own output "+
			"and TestDiagAvailabilityByPosition's FPL_NO_BLANK_RUN guard is "+
			"guarding nothing", blanked, settled)
	}
	want := settled * e.blankRunFactor(1)
	if math.Abs(blanked-want) > 1e-9 {
		t.Errorf("ExpectedMinutes with one blank is %v, want %v = settled x "+
			"blankRunFactor(1). Something else now scales the reported minutes, "+
			"so the discount can no longer be divided back out of a calibration",
			blanked, want)
	}
}
