package backtest

// What unifying the two estimators of P(appears) is worth on the replay.
//
//	DIAG=1 FPL_CELLS=/tmp/cells.csv go test ./internal/backtest \
//	  -run TestDiagUnifiedAppearance -v -timeout 240m
//
// # What moved and what did not
//
// P(appears) had two estimators — playsAtAll(ExpectedMinutes) and
// 1 - 0.624 x (1 - StartShare) — and the second one was consumed by the derived
// bench slot weights and by defconPerGameweek. See internal/analysis/appearance.go.
//
// The appearance TERM itself is untouched: appearanceFactor already read
// playsAtAll, so what a player is paid for turning up has not changed by a single
// bit. Only the two consumers of the other estimator moved, which means the replay
// sees this through exactly two channels:
//
//   - the bench slot weights, in every season, since they price an eleven's blank
//     probabilities and therefore change squad selection;
//   - defconPerGameweek, in 2025-26 ONLY, because defensive contribution did not
//     exist as a scoring category before then and DefconScoredIn gates it.
//
// # Why four arms and not two
//
// A two-arm sweep would confound the two channels, and a null could be them
// cancelling. Pinning the fixed bench tuple with SetDerivedBenchSlots(false)
// removes the first channel entirely, so the second pair isolates the defcon one.
// That is the FPL_FIXED_BENCH_SLOTS second arm the review asked for, wired as a
// sweep variant rather than an environment variable so both arms run in one
// process and pair cell by cell.
//
// # What to expect
//
// Not a resolvable effect. The canonical detection threshold is a median 39 points
// a season per comparison, and this moves a term worth hundredths of a point in one
// season of four plus a bench weight in a replay that builds near-nailed elevens by
// construction — the regime in which AGENTS.md already records the derived bench
// slots as a dead heat against the tuple. It is run because a correctness fix should
// be checked for a surprise, not because the number decides anything: the case for
// the change is that one quantity must not have two estimators, and that the one
// removed was measurably the worse of them (rms 0.1784 against 0.1112 on the
// realised appearance rate, biased +0.0804).
//
// HOLD is the metric. This is scoring and squad selection, not transfers.

import (
	"os"
	"testing"

	"armband/internal/analysis"
)

func TestDiagUnifiedAppearance(t *testing.T) {
	if os.Getenv("DIAG") == "" {
		t.Skip("set DIAG=1")
	}
	starts := sweepStarts()

	// Baseline is the PRE-change arm, so the reported difference is "what the
	// unification is worth" and carries the sign a reader expects. Same convention
	// as TestDiagViceCaptainFix.
	set := func(unified, derived bool) func(sc *SimConfig) {
		return func(sc *SimConfig) {
			sc.WeeklyXI = true
			analysis.SetUnifiedAppearance(unified)
			analysis.SetDerivedBenchSlots(derived)
		}
	}
	runPolicySweep(t, []policyVariant{
		{label: "two estimators, derived slots (pre-change)", apply: set(false, true)},
		{label: "one estimator, derived slots (ships)", apply: set(true, true)},
		{label: "two estimators, fixed bench tuple", apply: set(false, false)},
		{label: "one estimator, fixed bench tuple", apply: set(true, false)},
	}, starts)

	analysis.SetUnifiedAppearance(true)
	analysis.SetDerivedBenchSlots(true)
}
