package analysis

import "testing"

// TestCheapBodiesAndCorrectedPlayersClearTheMinutesFloor pins the two exemptions
// on the squad pool's total-minutes floor.
//
// # What went wrong
//
// The pool had two minutes screens. The expected-minutes cliff exempted cheap
// bench fodder, with a comment saying exactly why: "a £4.0m reserve who never
// plays is exactly what belongs in slots 12-15, and excluding him would force
// real money onto the bench." The total-minutes floor ran FIRST, carried neither
// exemption, and so made the cliff's unreachable for the population it was
// written for.
//
// Two consequences, both observed live pre-season, and neither one failed
// anything:
//
//   - No £4.0m goalkeeper was in the pool at all, because pre-season a
//     third-choice keeper's record is zero minutes. The optimiser had to buy two
//     keepers at £4.5m, and with the squad already spending its whole budget the
//     spare £0.5m came out of the eleven. That is precisely what the fodder
//     comment predicted would happen.
//   - Two deliberately corrected players — Coventry defenders given explicit
//     minutes overrides, with the reasoning recorded in config — were scored,
//     reported to the agent, and then silently unbuyable. The optimiser took a
//     0.48 points-per-gameweek player at the same £4.0m instead.
//
// The second is the worse one. `blend.go` states in so many words that an
// override leaves "whether he clears the minutes floor" to be recomputed rather
// than dictated, which is the whole argument for preferring a minutes correction
// over a lock: the optimiser must be able to decline. It could not decline,
// because the player never reached it. And `swaps.go` has no minutes floor at
// all, so the weekly transfer search would have offered the same players the
// following week — the recorded "an override the transfer search ignores is worse
// than no override" failure, with the two solvers the other way round.
func TestCheapBodiesAndCorrectedPlayersClearTheMinutesFloor(t *testing.T) {
	const (
		floor  = 600 // total minutes, the shipped pre-season value
		fodder = 4.5
	)

	cases := []struct {
		name       string
		minutes    int
		price      float64
		overridden bool
		want       bool
		why        string
	}{{
		name:    "regular starter clears it on his record",
		minutes: 2800, price: 6.5,
		want: true,
		why:  "the ordinary path, and the only one that worked before",
	}, {
		name:    "expensive player with no record is still screened out",
		minutes: 0, price: 9.0,
		want: false,
		why:  "the floor is a sample-size test and it must still bite; this is the case it exists for",
	}, {
		name:    "cheap reserve keeper with no record is admitted as a compliance body",
		minutes: 0, price: 4.0,
		want: true,
		why:  "the squad rules force fifteen players and this is what slots 12-15 are for",
	}, {
		name:    "a body priced exactly at the fodder line is admitted",
		minutes: 0, price: 4.5,
		want: true,
		why:  "the boundary is inclusive, or the cheapest keeper in some seasons is excluded",
	}, {
		name:    "a body a tenth above the fodder line is not",
		minutes: 0, price: 4.6,
		want: false,
		why:  "the exemption is for compliance slots, not a general waiver",
	}, {
		name:    "corrected player below the floor is admitted",
		minutes: 0, price: 4.0, overridden: true,
		want: true,
		why:  "an override is a claim about a role the record does not show",
	}, {
		name:    "corrected EXPENSIVE player below the floor is admitted",
		minutes: 400, price: 9.0, overridden: true,
		want: true,
		why:  "this is the half the price exemption alone would not have fixed",
	}}

	for _, c := range cases {
		m := PlayerMetrics{Minutes: c.minutes, Price: c.price}
		if got := clearsMinutesFloor(m, floor, fodder, c.overridden); got != c.want {
			t.Errorf("%s: got %v, want %v\n  %s", c.name, got, c.want, c.why)
		}
	}
}

// TestTheFodderExemptionCanBeSwitchedOff guards the opt-out. A caller that sets
// BenchMinExpectedMinutes is asking for every squad slot to clear a minutes bar,
// which zeroes fodderPrice — and that must disable the price exemption rather
// than being ignored, or the caller silently gets fodder anyway.
//
// The override exemption deliberately survives it: switching off cheap fodder is
// a statement about how deep a squad the caller wants, not a rejection of a
// correction the operator has made with a recorded reason.
func TestTheFodderExemptionCanBeSwitchedOff(t *testing.T) {
	cheap := PlayerMetrics{Minutes: 0, Price: 4.0}

	if clearsMinutesFloor(cheap, 600, 0, false) {
		t.Error("a £4.0m body with no record cleared the floor at fodderPrice 0; " +
			"the caller asked for every slot to clear a minutes bar")
	}
	if !clearsMinutesFloor(cheap, 600, 0, true) {
		t.Error("a corrected player was rejected at fodderPrice 0; " +
			"disabling cheap fodder should not discard an explicit minutes override")
	}
}
