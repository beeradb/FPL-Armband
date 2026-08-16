package main

import "testing"

// TestEveryCellsFileGetsItsOwnInferenceDirectory.
//
// The inference directory used to be the bare -inference path whenever exactly one
// cells file was supplied, so a twelve-cell demo run overwrote stats/out/mde.csv
// with figures that are not a replayed season — a mean paired difference of 6.1
// points a gameweek at t = 68 — and the next several snapshots read them as
// current. Nothing looked wrong because the output carried no clue which cells it
// came from.
//
// The rule now is one directory per input, always. This test pins the derivation
// rather than the call site, because the call site is a one-line join that is easy
// to "simplify" back to the special case that caused the problem.
func TestEveryCellsFileGetsItsOwnInferenceDirectory(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"cells.csv", "cells"},
		{"minutes_half_life_cells.csv", "minutes_half_life_cells"},
		{"a b.2.csv", "a_b_2"},
		{".csv", "cells"},
	} {
		if got := sanitise(tc.in); got != tc.want {
			t.Errorf("sanitise(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
	// Two names differing only in punctuation must not collide, or the second
	// file's inference silently replaces the first's — which is the same failure
	// one level down.
	if sanitise("a-b.csv") == sanitise("a.b.csv") {
		t.Error("two cells files would share an inference directory")
	}
}
