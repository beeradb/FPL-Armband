package analysis

import "testing"

// TestACardWithNoMinutesIsStillDeducted pins the one ordering decision in
// DecomposeMatch that is not obvious.
//
// FPL deducts for a booking a player collected without taking the field — from the
// bench, in the tunnel, at half time as an unused substitute. Priced behind the
// `Minutes <= 0` return, those rows come back 0 and are the only rows in the whole
// archive that fail to reconcile against `total_points`: fourteen of 253,509,
// thirteen yellows and one red, every one of them a recorded −1 or −3.
//
// It is pinned rather than left to the archive check because the natural
// simplification — "no minutes, no points, return early" — is correct for every
// other channel and reads as obviously right.
func TestACardWithNoMinutesIsStillDeducted(t *testing.T) {
	r := ScoringRulesFor("2023-24")

	for _, c := range []struct {
		name string
		in   RealisedMatch
		want float64
	}{
		{"a yellow from the bench", RealisedMatch{Position: 2, Minutes: 0, Yellow: 1}, -1},
		{"a red from the bench", RealisedMatch{Position: 3, Minutes: 0, Red: 1}, -3},
		{"no card and no minutes pays nothing",
			RealisedMatch{Position: 3, Minutes: 0}, 0},
	} {
		if got := DecomposeMatch(c.in, r).Total(); got != c.want {
			t.Errorf("%s: %g, want %g", c.name, got, c.want)
		}
	}
}

// TestTheDecompositionPricesAWholeMatch checks the channels a diagnostic reads
// individually, so a table with the right total and the wrong split cannot pass.
//
// The example is a defender who played the full match, scored, conceded three and
// was booked, under 2023-24's rules: 2 appearance + 6 goal − 1 concede (three
// conceded is one full block of two) − 1 card = 6.
func TestTheDecompositionPricesAWholeMatch(t *testing.T) {
	got := DecomposeMatch(RealisedMatch{
		Position: 2, Minutes: 90, Goals: 1, GoalsConceded: 3, Yellow: 1, Bonus: 2,
	}, ScoringRulesFor("2023-24"))

	for _, c := range []struct {
		what string
		got  float64
		want float64
	}{
		{"appearance", got.Appearance, 2},
		{"goals", got.Goals, 6},
		{"clean sheet", got.CleanSheet, 0},
		{"concede", got.Conceded, -1},
		{"cards", got.Cards, -1},
		{"bonus", got.Bonus, 2},
		{"total", got.Total(), 8},
	} {
		if c.got != c.want {
			t.Errorf("%s: %g, want %g", c.what, c.got, c.want)
		}
	}

	// Under sixty minutes the appearance channel is the short-play point and the
	// clean sheet is not on offer — the archive's own column carries that rule, so
	// the check here is only that the step is a step.
	if short := DecomposeMatch(RealisedMatch{Position: 2, Minutes: 45},
		ScoringRulesFor("2023-24")).Appearance; short != 1 {
		t.Errorf("a 45-minute appearance pays %g, want 1", short)
	}
}

// TestTheDecompositionRefusesAPositionItCannotPrice is the same guard
// XPointsResidual carries, for the same reason: element_type 5 — 2024-25's
// assistant managers — must not be scored as a footballer whose goals are worth
// nothing.
func TestTheDecompositionRefusesAPositionItCannotPrice(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("an unpriced position was decomposed without complaint; a " +
				"missing goal value must fail rather than delete a channel")
		}
	}()
	DecomposeMatch(RealisedMatch{Position: 5, Minutes: 90}, ScoringRulesFor("2024-25"))
}
