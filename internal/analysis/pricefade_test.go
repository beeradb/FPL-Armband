package analysis

import (
	"math"
	"testing"

	"armband/internal/fpl"
)

// fadeEngine is priceEngine with a controllable number of FINISHED gameweeks, so
// the calendar fade can be exercised without touching the clock.
func fadeEngine(w float64, played int) *Engine {
	e := priceEngine(w)
	e.Boot.Events = nil
	for gw := 1; gw <= 38; gw++ {
		e.Boot.Events = append(e.Boot.Events, fpl.Event{
			ID: gw, Name: "Gameweek",
			Finished: gw <= played,
			IsNext:   gw == played+1,
		})
	}
	return e
}

// ⚠️ **The tilt fades on the CALENDAR as well as on the player's own minutes,
// and the two are different claims.** The per-player fade asks "how much do we
// still need a prior for him"; this one asks "is price still the thing that was
// measured". FPL revises price on transfer activity, so a January signing's
// price encodes months of bandwagon rather than a pre-season forecast — and the
// evidence behind this lever covers GW1-10 ordering only.
func TestThePriceTiltFadesOutByTheEarlySeason(t *testing.T) {
	const w = 0.5
	dear := func(e *Engine) float64 {
		return e.priceMinutesTilt(e.Boot.ElementByID(5)) // the dearest midfielder
	}

	// ⚠️ Not asserted as 1+w. The tilt is `1 + w*(2p-1)` on a MID-RANK
	// percentile, so the dearest of five distinct prices sits at p = 0.9 rather
	// than 1.0 and reaches 1.4, not 1.5. Asserting the extreme would be testing
	// an assumption about the fixture instead of the fade.
	pre := dear(fadeEngine(w, 0))
	if pre <= 1 {
		t.Fatalf("pre-season the dearest player should be tilted UP; got %v", pre)
	}

	// Monotone decay: never increasing as gameweeks are played.
	prev := pre
	for played := 1; played < priceTiltFadesByGW; played++ {
		got := dear(fadeEngine(w, played))
		if got > prev+1e-9 {
			t.Errorf("the tilt grew between %d and %d gameweeks played: %v then %v",
				played-1, played, prev, got)
		}
		if got < 1 {
			t.Errorf("at %d gameweeks the tilt fell below neutral for the DEAREST "+
				"player: %v. Fading must approach 1, not cross it", played, got)
		}
		prev = got
	}

	// ⚠️ And it reaches exactly neutral, not merely small. A tilt that never
	// quite stopped would keep nudging an argmax on a signal the measurement
	// does not cover.
	for _, played := range []int{priceTiltFadesByGW, priceTiltFadesByGW + 5, 38} {
		if got := dear(fadeEngine(w, played)); got != 1 {
			t.Errorf("at %d gameweeks played the tilt must be exactly 1; got %v",
				played, got)
		}
	}
}

// Half-way through the fade the tilt should be half as strong — the fade is
// linear on purpose, so there is no cliff a squad can sit either side of.
func TestThePriceFadeIsLinearWithNoCliff(t *testing.T) {
	const w = 0.5
	dear := func(played int) float64 {
		e := fadeEngine(w, played)
		return e.priceMinutesTilt(e.Boot.ElementByID(5)) - 1
	}
	full := dear(0)
	half := dear(priceTiltFadesByGW / 2)
	// Integer division on an odd fade point, so allow the rounding.
	if want := full * (1 - float64(priceTiltFadesByGW/2)/float64(priceTiltFadesByGW)); math.Abs(half-want) > 1e-9 {
		t.Errorf("at the half-way gameweek the tilt should be %v of full; got %v", want, half)
	}
}
