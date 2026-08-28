package main

import "testing"

// The sign convention is the whole risk in `armband overrides`. The gap is the
// model's NATURAL estimate minus the override, so a negative gap means the model
// wants fewer minutes than the override asserts and the override is lifting the
// player. Inverting that would relabel every row in the table while the numbers
// stayed correct, which is the kind of defect a reader confirms rather than
// catches.
func TestTheOverrideVerdictReadsTheGapsSign(t *testing.T) {
	const within = 10.0

	cases := []struct {
		name      string
		gap       float64
		want      string
		redundant bool
	}{
		// An override of 80 against a natural 50: gap −30, the override is
		// holding the estimate UP.
		{"model says far less than the override", -30, "still lifting him", false},
		// An override of 20 against a natural 70: gap +50, the override is
		// holding the estimate DOWN.
		{"model says far more than the override", 50, "still holding him down", false},

		{"model agrees, slightly under", -3, "redundant — the model now agrees", true},
		{"model agrees, slightly over", 3, "redundant — the model now agrees", true},
		{"model agrees exactly", 0, "redundant — the model now agrees", true},

		// ⚠️ The boundary is exclusive, and it is asserted rather than left to
		// whichever way the comparison happened to be written. A gap exactly at
		// the tolerance reports as still acting, which keeps an override rather
		// than inviting its deletion — the safe direction, given a redundant
		// override may still be what is holding the estimate in place.
		{"exactly at the tolerance, above", within, "still holding him down", false},
		{"exactly at the tolerance, below", -within, "still lifting him", false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, redundant := overrideVerdict(c.gap, within)
			if got != c.want || redundant != c.redundant {
				t.Errorf("overrideVerdict(%+.0f, %.0f) = %q/%v, want %q/%v",
					c.gap, within, got, redundant, c.want, c.redundant)
			}
		})
	}
}

// A tolerance of zero must call nothing redundant, including an exact tie. The
// count printed under the table is "how many overrides could be reconsidered",
// and a zero tolerance asking for none is the reading that cannot mislead.
func TestAZeroToleranceCallsNothingRedundant(t *testing.T) {
	for _, gap := range []float64{-5, -0.001, 0, 0.001, 5} {
		if _, redundant := overrideVerdict(gap, 0); redundant {
			t.Errorf("gap %+.3f reported redundant at a tolerance of zero", gap)
		}
	}
}
