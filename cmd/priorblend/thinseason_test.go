package main

import (
	"testing"

	"armband/internal/priors"
	"armband/internal/recent"
)

// TestTheHalfSeasonBarIsOneNumber pins the two reachable declarations of 1,710
// minutes together.
//
// # Why here, of all places
//
// The bar — "below this the most recent season stops being trusted on its own" —
// is declared four times: `priors.ThinSeason` for the live path, `recent.ThinSeason`
// for the `history_past` path, `thinPrior` unexported inside `internal/backtest`
// for the replay, and once more in this experiment's own classification. Three of
// those packages cannot import one another, so there is no natural home for a
// check. This command already imports two of them for real work, so the check
// costs one file and no new dependency.
//
// It is worth having because the failure is silent in the expensive direction. If
// one copy moved to 1,600 the two paths would disagree about which players are
// blended at all, the live command and the replay would answer differently for
// the same footballer, and nothing would error — the replay would simply be
// measuring a slightly different feature from the one that ships.
//
// # What it does NOT cover, stated so nobody reads more into it
//
// `internal/backtest`'s copy is unexported, so it cannot be named from here even
// though this command imports that package for real work. It is pinned by
// `TestTheHalfSeasonBarIsOneNumber` INSIDE `internal/backtest` — the three lines
// an earlier version of this comment said belonged to whoever owned the file.
//
// All four are now aliases of `analysis.ThinSeason` rather than four literals, so
// what these assertions really check is that nobody has un-aliased one back into
// a number free to drift.
func TestTheHalfSeasonBarIsOneNumber(t *testing.T) {
	if priors.ThinSeason != recent.ThinSeason {
		t.Fatalf("the half-season bar is %d in internal/priors and %d in internal/recent. "+
			"These decide the same thing — whether a player's most recent season is thin "+
			"enough to blend older ones into — for two paths that must agree about the "+
			"same footballer.", priors.ThinSeason, recent.ThinSeason)
	}
	// Half a season, and the value every measurement of this feature was made at.
	// A deliberate change to the bar should fail here and be re-argued, not slide
	// through because both copies were edited together.
	if priors.ThinSeason != 1710 {
		t.Fatalf("the half-season bar is %d, not 1710. Half of a 38-match season at "+
			"ninety minutes is 1,710, and every recorded figure about prior_half_life — "+
			"the population counts, the bias reduction, the split by mechanism — was "+
			"measured at that value.", priors.ThinSeason)
	}
}
