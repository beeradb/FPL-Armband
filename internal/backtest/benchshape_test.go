package backtest

// Does shaping the bench beat a flat multiplier?
//
//	DIAG=1 EXP=benchshape FPL_CELLS=/tmp/benchshape.csv \
//	  scripts/replay -run TestDiagBenchShape -v -timeout 2h
//
// # Why this exists: the record answers it twice, incompatibly
//
// `internal/analysis/squad.go`'s slot-weight comment says flat weights are "51
// worse on the tuned three and 79 *better* on 2022-23, so 'shaped beats flat' is
// not established by anything". The optimiser-and-squad note's table says
// flat is 77 behind every shaped variant across four seasons, with 2022-23 a
// **tie** at 1666 against 1666. Net, the first makes flat 28 points *ahead* over
// four seasons and the second puts it 77 behind.
//
// They are not two readings of one table. The proof is internal to them: the
// squad.go block exists to justify 2.4/1.0/0.4/0.2 over 1.9/1.2/0.6/0.3 and
// scores that at "+30 mean and +27 on held-out 2022-23", while the notes table
// scores the same pair at +16 mean and **−44** on 2022-23. Two runs, and they
// disagree about the comparison one of them was written to make.
//
// Dated: squad.go's figures entered at `e299f57` on 2026-08-06, the notes table
// at `337f83d` on 2026-08-07. **Both predate the expected-goals and
// expected-goals-conceded backfills of 2026-08-12**, which move 6 of the 24
// four-season cells and *all six are 2022-23* — precisely the held-out column
// the two disagree about. Both also predate the appearance unification, which
// the notes table's own warning already flags: every figure in that subsection
// was measured under the legacy 0.624 blank rate, and the two rules give
// different bench weights by construction.
//
// So neither figure can be preferred and neither is recoverable as evidence
// about today's tree. This measures the contrast again rather than adjudicating
// between two void ones.
//
// # Pre-registration
//
// Stated before the run, because the temptation afterwards is to explain
// whichever way it went.
//
//   - **P1, the only directional claim.** Shaped beats flat on HOLD: each of the
//     three shaped arms is positive against the flat baseline. This is the claim
//     both records agree a shape sweep *cannot* establish from a mean alone, so
//     what is read is the **sign and the per-cell consistency**, not the size.
//   - **P2, expected unresolved.** 77 points over four GW1 cells is 0.507 pts/gw,
//     about t 1.1 at the HOLD median standard error. Nothing here is expected to
//     clear this comparison's own threshold, which must come from these cells via
//     variance_components.R and never from the global HOLD median of 33.
//   - **P3, the shaped variants tie.** The three shaped arms land within a few
//     points of each other. If one now separates from the others, that is a
//     finding about the appearance rule change rather than about bench shape.
//   - **P4, the mediator, checked before any points column is read.** The number
//     of cells whose opening fifteen differs from the flat arm's. A shape that
//     changes no squad cannot change points, and a null with an inert mediator is
//     the byte-identical null this record calls its signature failure rather than
//     a measurement that shaping does not pay.
//
// # What this is not
//
// It is not the `MinExpectedMinutes` crossing the work queue carries, which needs a
// second floor level and is a six-arm design. This is that design's shape main
// effect at the shipped floor, run early because its own pre-registration cited
// a figure the repository states two ways.
//
// HOLD is the metric: bench slot weights price the fifteen, which HOLD builds,
// and they are not read by `decide()`. POLICY is collected because the sweep
// collects it, and adds the transfer path's 303-point noise to a scoring
// question — do not report it as the answer.

import (
	"os"
	"testing"

	"armband/internal/analysis"
)

func TestDiagBenchShape(t *testing.T) {
	if os.Getenv("DIAG") == "" {
		t.Skip("set DIAG=1")
	}
	starts := sweepStarts()

	// Flat is the baseline, so a positive difference means shaping helps, which
	// is the sign a reader expects from the question. Same convention as
	// TestDiagUnifiedAppearance and TestDiagViceCaptainFix.
	// Restore what was found rather than what the shipped default is assumed to
	// be, and restore it on a fatal. ⚠️ A first version wrote `true` and the
	// shipped tuple as trailing statements: under FPL_FIXED_BENCH_SLOTS=1 — a
	// real way to run this suite — that flipped every *later* diagnostic in the
	// process onto derived slots while the fingerprint still stamped the
	// environment as fixed, and any t.Fatal inside runPolicySweep skipped the
	// restore entirely, leaving one arm's tuple installed for the rest of the run.
	// Neither shows up in a number, which is what makes it worth a defer.
	priorOut, priorGK, priorDerived := analysis.BenchSlotState()
	defer func() {
		analysis.SetDerivedBenchSlots(priorDerived)
		analysis.SetBenchSlots(priorOut, priorGK)
	}()

	shape := func(out [3]float64, gk float64) func(sc *SimConfig) {
		return func(sc *SimConfig) {
			sc.WeeklyXI = true
			analysis.SetDerivedBenchSlots(false)
			analysis.SetBenchSlots(out, gk)
		}
	}
	derived := func(sc *SimConfig) {
		sc.WeeklyXI = true
		analysis.SetDerivedBenchSlots(true)
	}

	runPolicySweep(t, []policyVariant{
		{label: "flat 1/1/1/1 (baseline)", apply: shape([3]float64{1, 1, 1}, 1)},
		{label: "fixed 2.4/1.0/0.4/0.2 (the shipped tuple)", apply: shape([3]float64{2.4, 1.0, 0.4}, 0.2)},
		{label: "fixed 1.9/1.2/0.6/0.3 (its predecessor)", apply: shape([3]float64{1.9, 1.2, 0.6}, 0.3)},
		{label: "derived from the eleven (ships)", apply: derived},
	}, starts)
}
