package backtest

import (
	"context"
	"math"
	"testing"

	"armband/internal/analysis"
)

// decompositionCheckSeason is the season this reconciles against.
//
// 2025-26 rather than an older one, because it is the only season carrying
// defensive contribution — the newest channel, the one with a threshold FPL does
// not publish, and therefore the one most likely to be wrong. Every other channel
// is exercised by every season.
const decompositionCheckSeason = "2025-26"

// TestTheRealisedDecompositionReconcilesWithTheArchive is the gate on
// `analysis.DecomposeMatch`, and the gate is **identically zero**, never "close".
//
// # Why the archive is the right target
//
// The decomposition takes four channels from `analysis.ScoringRulesFor` and prices
// the other seven from package constants that are NOT season-pinned — an exposure
// `ScoringRules`' own docstring declares and deliberately leaves open. The archive
// publishes `total_points`, computed by FPL under the rules actually in force. So
// the sum of the parts against that column is a check the derivation cannot have
// been fitted to, in the same shape as `TestDefConAggregateMatchesTheArchivesOwnTotal`.
//
// This project's standing rule is that a reconstruction is judged on an exactly
// zero residual and never on a good fit, because mis-coding one sibling feature
// returns plausible coefficients at an 88%-exact match. Measured when this was
// written: **zero residual on all 253,509 priced rows across all ten archived
// seasons**, 2016-17 to 2025-26. That is also the measurement behind the claim
// that FPL's appearance rule — one point for any minutes, two from sixty — did not
// change over the archive: a season where it had would show a residual on most of
// its rows rather than on none of them.
//
// A future season that genuinely changes a rule will fail here, and that is a
// finding about FPL rather than a tolerance to widen.
func TestTheRealisedDecompositionReconcilesWithTheArchive(t *testing.T) {
	cfg := loadConfig(t)
	// Through the byte mirror, for the reason `mirrorArchive` gives: this reads
	// five megabytes of raw CSV that `Load`'s parsed cache cannot serve, and the
	// host returns 429 under load — which would turn a real assertion into a
	// permanent skip. It introduces no staleness class the parsed cache does not
	// already have.
	mirrorArchive(t, cfg)
	season, err := Load(context.Background(), cfg.CacheDir, decompositionCheckSeason)
	if err != nil {
		t.Skipf("archive unreachable: %v", err)
	}
	rules := analysis.ScoringRulesFor(decompositionCheckSeason)
	paid := DefconScoredIn(decompositionCheckSeason)

	var priced, off int
	var worst float64
	var worstAt string
	_, err = season.gameweekRows(context.Background(),
		func(rec []string, col map[string]int, p *Player, gw int) {
			if !rules.Prices(p.Type) {
				return
			}
			priced++
			got := analysis.DecomposeMatch(analysis.RealisedMatch{
				Position:        p.Type,
				Minutes:         ival(rec, col, "minutes"),
				Goals:           ival(rec, col, "goals_scored"),
				Assists:         ival(rec, col, "assists"),
				CleanSheets:     ival(rec, col, "clean_sheets"),
				GoalsConceded:   ival(rec, col, "goals_conceded"),
				Saves:           ival(rec, col, "saves"),
				Bonus:           ival(rec, col, "bonus"),
				Yellow:          ival(rec, col, "yellow_cards"),
				Red:             ival(rec, col, "red_cards"),
				OwnGoals:        ival(rec, col, "own_goals"),
				PenaltiesSaved:  ival(rec, col, "penalties_saved"),
				PenaltiesMissed: ival(rec, col, "penalties_missed"),
				DefCon:          ival(rec, col, "defensive_contribution"),
				DefConPaid:      paid,
			}, rules)
			d := got.Total() - float64(ival(rec, col, "total_points"))
			if d == 0 {
				return
			}
			off++
			if math.Abs(d) > math.Abs(worst) {
				worst = d
				worstAt = p.WebName
			}
		})
	if err != nil {
		// FAIL, not skip. The walk is over a file `Load` has already fetched, so
		// the network is not in question by this point — an error here is a CSV
		// the parser could not read, and reporting that as "archive unreachable"
		// would turn a real defect into a silent skip.
		t.Fatalf("walking %s: %v", decompositionCheckSeason, err)
	}

	// A zero row count reads exactly like a pass, which is the failure this
	// package catalogues more often than any other.
	if priced == 0 {
		t.Fatalf("no priced row in %s, so the reconciliation did not run",
			decompositionCheckSeason)
	}
	if off != 0 {
		t.Errorf("%d of %d rows in %s do not reconcile against the archive's own "+
			"total_points, worst %+.0f on %s.\n\n"+
			"The gate is an identically zero residual, not a good fit: mis-coding "+
			"one channel returns a plausible total on most rows. Either a scoring "+
			"rule moved — which belongs in analysis.ScoringRulesFor with the season "+
			"it moved in — or a channel is priced wrong.",
			off, priced, decompositionCheckSeason, worst, worstAt)
	}
	t.Logf("%s reconciles on %d priced rows with a zero residual",
		decompositionCheckSeason, priced)
}
