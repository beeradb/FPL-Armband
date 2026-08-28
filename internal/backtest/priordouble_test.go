package backtest

import (
	"testing"
)

// ⚠️ **`GW.Minutes` is a TOTAL ACROSS FIXTURES.** The field's own comment in
// season.go says so and says why the `Fixtures` count exists: "minutes of 180
// means two full matches, not an impossible one... anything wanting a per-match
// figure has to divide by this."
//
// `newPriorIndexRecent` builds a "weighted mean minutes per gameweek" and
// rescales it by 38. If a double gameweek contributes 180 to that mean without
// being divided by its fixture count, the projection is of a season made of
// double gameweeks — which does not exist.
//
// These pin the arithmetic. They do not assert a verdict about recency; they
// establish whether the recency arm's INPUT is physically possible.

// doubleSeason is a player who plays 90 minutes every gameweek, except one late
// double where he plays both matches — 180 in that gameweek, which is real.
// ⚠️ The blank is not decoration. Every club plays exactly 38 matches, so a
// double is paid for by a blank elsewhere — a fixture with 37 singles and one
// double describes a 39-match season that does not exist, and it would make the
// per-match rescale look wrong when it is right. The first version of this
// fixture did exactly that.
func doubleSeason(doubleAt int) *Season {
	const blankAt = 12
	p := &Player{ID: 1, Code: 7, GWs: map[int]GW{}}
	for gw := 1; gw <= 38; gw++ {
		if gw == blankAt {
			continue // no row at all in a blank, as the archive records it
		}
		m, fx := 90, 1
		if gw == doubleAt {
			m, fx = 180, 2
		}
		p.GWs[gw] = GW{Minutes: m, Fixtures: fx, Starts: fx}
		p.Minutes += m
		p.Starts += fx
	}
	return &Season{Name: "2025-26", Players: map[int]*Player{1: p}}
}

// ⚠️ **The control is the FLAT prior on the same player, not an invented
// ceiling.** An earlier version of this test bounded minutes at 38*90 and the
// flat prior "failed" it — wrongly, because a double gameweek adds a 39th match
// unless a blank pays for it. Comparing against a constant tested the constant;
// comparing against the flat prior tests the code.
//
// For a player who played 90 minutes in every match, a recency-weighted prior
// must agree with the flat one: there is no trend to weight, every match is
// identical, so any gap at all is arithmetic rather than recency.
func TestTheRecencyPriorAgreesWithTheFlatOneWhenNothingChanged(t *testing.T) {
	s := doubleSeason(38)
	truth, ok := newPriorIndex(s).Get(7)
	if !ok {
		t.Fatal("fixture player missing")
	}
	// 36 singles at 90, one double at 180, one blank — 38 matches, 3420 minutes.
	if truth.Minutes != 36*90+180 {
		t.Fatalf("precondition: the flat prior should report the minutes actually "+
			"played, %d; got %d", 36*90+180, truth.Minutes)
	}

	for _, hl := range []float64{2, 4, 8, 12} {
		got, ok := newPriorIndexRecent(s, hl, 0, 0).Get(7)
		if !ok {
			t.Fatalf("half-life %v: fixture player missing", hl)
		}
		// This player is identical every week, so recency has nothing to find.
		if over := got.Minutes - truth.Minutes; over > 45 {
			t.Errorf("half-life %v: the recency prior projects %d minutes against the %d "+
				"actually played — %+d, or %.0f%% — for a player who was unchanged all "+
				"season. A double gameweek's 180 is read as a per-GAMEWEEK rate and "+
				"multiplied by 38, so the projection describes a season made of doubles. "+
				"GW.Minutes is a total across fixtures and this path never divides by "+
				"GW.Fixtures.",
				hl, got.Minutes, truth.Minutes, over,
				100*float64(over)/float64(truth.Minutes))
		}
		// 39 matches exist in this fixture's season, so 39 starts is the ceiling
		// and anything above it is impossible rather than merely high.
		if got.Starts > 38 {
			t.Errorf("half-life %v: prior projects %d starts, and only 38 matches were "+
				"played", hl, got.Starts)
		}
	}
}

// ⚠️ **THE HALF-LIFE MUST NOT MOVE AN UNCHANGED PLAYER, and this is the guard
// that matters most.**
//
// Before the per-match fix, a late double inflated the prior by an amount that
// grew as the half-life sharpened — 26% at 2 against 4% at 12. That is the same
// ORDERING the recency sweep reported and read as "recency loses monotonically,
// worse the sharper it gets". A units artefact and the hypothesis under test
// produced the same shape, so the sweep could not tell them apart.
//
// For a player who is identical in every match there is no trend, so every
// half-life must return the same prior. If this ever fails again, a recency
// sweep is measuring its own arithmetic and its gradient means nothing.
func TestTheHalfLifeDoesNotMoveAPlayerWhoNeverChanged(t *testing.T) {
	s := doubleSeason(38)
	base, ok := newPriorIndexRecent(s, 2, 0, 0).Get(7)
	if !ok {
		t.Fatal("fixture player missing")
	}
	for _, hl := range []float64{4, 8, 12} {
		got, _ := newPriorIndexRecent(s, hl, 0, 0).Get(7)
		// One minute of slack for the integer truncation at each end.
		if d := got.Minutes - base.Minutes; d > 1 || d < -1 {
			t.Errorf("half-life %v reads %d minutes against half-life 2's %d for a player "+
				"who played 90 in every match. A gradient here is the units, not the "+
				"trend, and a sweep over half-lives would report it as the trend",
				hl, got.Minutes, base.Minutes)
		}
	}
	t.Logf("unchanged player, prior minutes across half-lives 2..12: %d (truth 3420)",
		base.Minutes)
}
