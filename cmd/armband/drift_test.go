package main

import "testing"

// ⚠️ `priorArchiveSeason` shares a concept and nearly a name with
// `priorSeasonName`, which already existed here. They are not interchangeable:
// that one reads the LIVE engine's next deadline and formats for
// `internal/priors`; this one maps an archive label to its predecessor in the
// archive's own two-digit form. These pin the shape so the two cannot quietly
// converge into one wrong function.
func TestPriorArchiveSeasonUsesTheArchivesTwoDigitForm(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"2026-27", "2025-26"},
		{"2020-21", "2019-20"},
		// The century roll is the case a naive "subtract one from each half"
		// gets wrong: 2000-01's predecessor is 1999-00, not 1999-0.
		{"2000-01", "1999-00"},
	} {
		if got := priorArchiveSeason(c.in); got != c.want {
			t.Errorf("priorArchiveSeason(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// ⚠️ A label it cannot parse comes back UNCHANGED rather than mangled, so the
// caller's archive load fails naming something recognisable. Returning an empty
// string would make the prior silently absent, and a missing prior degrades the
// engine to league rates without saying so — a quieter and worse failure than a
// load error.
func TestPriorArchiveSeasonReturnsAnUnparseableLabelUnchanged(t *testing.T) {
	for _, in := range []string{"", "2026", "not-a-season", "20xx-27", "2026/27"} {
		if got := priorArchiveSeason(in); got != in {
			t.Errorf("priorArchiveSeason(%q) = %q; an unparseable label must come back "+
				"unchanged so the failed load names it", in, got)
		}
	}
}

// countChanged is how many of the held fifteen the fresh squad replaced.
func TestCountChangedCountsReplacementsNotMoves(t *testing.T) {
	held := []int{1, 2, 3, 4, 5}

	if got := countChanged(held, []int{1, 2, 3, 4, 5}); got != 0 {
		t.Errorf("an identical squad has changed nothing; got %d", got)
	}
	if got := countChanged(held, []int{9, 9, 9, 9, 9}); got != 5 {
		t.Errorf("a squad sharing nobody has replaced all five; got %d", got)
	}
	if got := countChanged(held, []int{1, 2, 3, 4, 99}); got != 1 {
		t.Errorf("one player swapped is one change; got %d", got)
	}

	// ⚠️ ORDER MUST NOT MATTER. The fresh squad comes out of the optimiser in
	// its own order, and counting positionally would report a full rebuild every
	// week for a squad that merely got sorted differently.
	if got := countChanged(held, []int{5, 4, 3, 2, 1}); got != 0 {
		t.Errorf("the same five in another order is not a change; got %d", got)
	}
}
