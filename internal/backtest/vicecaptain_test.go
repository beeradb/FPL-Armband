package backtest

// Measures what the vice-captain fallback (see viceCaptainFallback in
// replay.go) is worth on HOLD and POLICY, at the standard paired-difference
// resolution — sweepPairNames by sweepStarts.
//
// ⚠️ **Two recorded figures follow and they have different populations; the data
// state below attaches to the SECOND.** The +4.86 naive / +3.28 clustered HOLD
// figure is this diagnostic's own, measured when this grid was four seasons by
// six start gameweeks and on an estimator that no longer exists. The 261-point
// forfeit is TestDiagBlankHandling's, counted over gameweek-decisions rather than
// over cells, so a grid width does not apply to it at all. The distinction is
// worth the sentence because the shipped constant has both a six-season and a
// four-season value on record, and a reader who carries "four seasons" onto the
// wrong line reads a data state onto a figure that never had one.
//
// TestDiagBlankHandling already measured the mechanism directly: 261 points of
// vice-bonus forfeited across 612 gameweek-decisions, ~16 points a season. That
// is itself a before/after paired quantity (pre-fix always credited zero), but
// this runs it through the harness's own convention so it is comparable to every
// other constant in AGENTS.md.
//
//	DIAG=1 FPL_CELLS=/tmp/cells.csv \
//	    go test ./internal/backtest -run TestDiagViceCaptainFix -count=1 -v -timeout 1h
//	Rscript stats/sweep_inference.R /tmp/cells.csv
//
// # This used to be its own harness, and that was the bug
//
// It reimplemented the whole thing: its own cell map, its own paired difference,
// its own naive SE and its own average-the-seasons cluster SE — a line-for-line
// copy of what transferpolicy_test.go had, and the same estimator that was
// retired there for having no small-sample correction and no principled df. So
// the recorded finding (+4.86 naive / +3.28 clustered on HOLD) came from an
// estimator that no longer exists, and nothing would have flagged the two copies
// drifting apart.
//
// It is a two-arm sweep over the standard grid, which is exactly what
// runPolicySweep does, so it is now one. `viceCaptainFallback` is a package var
// and a variant can flip it, the same way the captain-shrink and fixture-load
// blocks flip theirs.
//
// It also had a quieter duplicate: the denominator was computed as
// `38 - start + 1` rather than read from `len(res.Weeks)`. The two agree today —
// Simulate appends one Week per gameweek from the start to 38 — but that is a
// second expression of one quantity, and this file's standing lesson is that the
// measured one then stops being the one that runs. runPolicySweep divides by
// len(res.Weeks) and emits it as the `weeks` column, so R re-derives it.

import (
	"testing"
)

func TestDiagViceCaptainFix(t *testing.T) {
	requireDiag(t)
	starts := sweepStarts()

	// Baseline is the *pre-fix* arm rather than the shipped one, deliberately, so
	// the reported difference is "what the fix is worth" and keeps the sign every
	// figure in AGENTS.md was recorded with. Block D of TestDiagTransferPolicy
	// baselines on "off (was shipped)" for the same reason.
	//
	// Both metrics matter here and HOLD is the primary one: the armband is
	// re-picked weekly at no cost, so this is a scoring correction rather than
	// anything about transfers.
	runPolicySweep(t, []policyVariant{
		{label: "vice off (pre-fix)", apply: func(sc *SimConfig) {
			sc.WeeklyXI = true
			viceCaptainFallback = false
		}},
		{label: "vice on (ships)", apply: func(sc *SimConfig) {
			sc.WeeklyXI = true
			viceCaptainFallback = true
		}},
	}, starts)
	viceCaptainFallback = true
}
