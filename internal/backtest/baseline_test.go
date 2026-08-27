package backtest

// What the replay scores at the shipped settings, right now.
//
// Every other diagnostic here bundles the baseline into a sweep, so the number
// only exists as the first row of whatever was being measured that day — and
// AGENTS.md accumulated three eras of them, each recorded as "the baseline" and
// none carrying the commit it was measured at. Five changes then shipped
// (doubles counting, fixture load on the eleven, fixture load on transfers, the
// vice-captain, legal autosubs) worth about +200 points a season on POLICY, and
// nothing in the file was re-run against them.
//
// So this test does one thing: print the shipped settings' totals over the full
// grid, with nothing varied. Its output is meant to be pasted into AGENTS.md
// *with the commit hash*, and re-run whenever a change lands that moves what the
// replay scores.
//
//	DIAG=1 go test ./internal/backtest -run TestDiagBaseline -v -timeout 2h
//
// The conventions below are not incidental — a total means nothing without
// them, and two of this project's harnesses have already disagreed about the
// last one:
//
//   - the entry points of sweepStarts by the season pairs of sweepPairNames
//     (the grid is named there and deliberately not repeated here —
//     TestTheGridIsDeclaredOnce enforces that, comments included, and the cell
//     count is not restated for the same reason: it went from 24 to 36 when the
//     default widened and every copy of it went stale in place)
//   - BankUpTo: sweepBankLimit (5), Budget: 1000, no hits beyond MaxHitsPerWeek
//   - WeeklyXI: true — the eleven is picked on the imminent gameweek, which is
//     what "fixture load on the XI only" ships as, and what the chip tests use
//   - chips off (SimConfig.Chips is zero-valued)
//   - paired differences are per gameweek *played*; the totals printed here are
//     raw sums and are only comparable to another run at these same conventions
//
// HOLD is the held opening fifteen and POLICY is the full transfer policy. HOLD
// is the quieter line and the one a scoring constant belongs on.

import (
	"testing"
)

func TestDiagBaseline(t *testing.T) {
	requireDiag(t)
	// One variant, nothing applied but the convention. runPolicySweep is reused
	// rather than restated so that this is *by construction* the number every
	// other sweep in this package baselines against — a hand-rolled copy could
	// drift from it, which is how the conventions diverged in the first place.
	runPolicySweep(t, []policyVariant{{
		label: "shipped (baseline)",
		apply: func(sc *SimConfig) { sc.WeeklyXI = true },
	}}, sweepStarts())
}
