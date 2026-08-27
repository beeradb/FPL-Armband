package backtest

// Does the clean sheet want its level corrected, its slope corrected, or both?
//
//	DIAG=1 EXP=CSXGC FPL_CELLS=/tmp/csxgc.csv \
//	  scripts/replay -run TestDiagCleanSheetXGC -v -timeout 2h
//
// # ⚠️ RUN 2026-08-15. Banked at stats/snapshots/2026-08-15-clean-sheet-2x2/.
//
// Nothing resolves: +1.9 / +6.2 / +7.0 a season against thresholds 23 / 16 / 20,
// Holm 1.000, all arms movable 6/6. The CANARY is the result — halving every
// clean sheet costs only −21.6 against its own threshold of 28, so this family
// sits about 4x below detection and the design was underpowered before it
// started. **Size a candidate against a canary BEFORE spending 180 cells.**
//
// **Do not re-run this file at the refitted constants.** The arms below are
// fitted against REALISED SINGLE-MATCH xGC; on XGC90 — what the knobs actually
// multiply — the joint fit is f = 0.992, flat = 0.939, a factor arm that is a
// no-op. TestDiagCleanSheetRegressor is the pair.
//
// Two statements below came out wrong and are marked rather than removed:
//   - "the flat scale is ordering-inert within a position" — WITHDRAWN. It
//     multiplies one additive component of Score, so defenders differing in
//     clean-sheet share reorder under it.
//   - the candidate arms' concentration in the reconstructed-xGC seasons is
//     UNEXPLAINED; "the factor amplifies regressor error in the exponent" is
//     refuted by `flat only` moving 4/6/4 in the same seasons.
//
// # Why this is a 2x2 and not a ladder
//
// exp(-xGC) over-predicts clean sheets by about 30%, and the record has argued
// about the remedy twice without running it. Fitting -ln p against xGC per
// observation — on native-xGC rows, with the `Fixtures != 1` guard four sibling
// diagnostics received after the doubles fix — gives
//
//	-ln p = 0.1003 + 1.1731*x   =>   P = 0.9046 * exp(-1.1731*x)
//
// with b > 1 in 4 of 4 seasons. **Both one-parameter restrictions of that fit
// are rejected**: neither a pure level correction nor a pure slope correction is
// an adequate family. That is the whole reason this is a 2x2. A ladder over the
// factor alone would be made to carry a level it cannot express, because
// exp(-(f-1)x) depends on x — at f = 1.27 the within-position spread goes to
// 1.75x where the clean fit wants 1.44x, over-correcting the ordering by roughly
// 40%. Two levers one mechanism names get a 2x2, never two sweeps.
//
// The two arms are also different KINDS of change, which is why the interaction
// is worth a cell rather than an assumption:
//
//   - The **flat** scale is a level on every clean sheet. Shared by every player
//     at a position, so ordering-inert within one — it is a points-level question
//     only. It is NOT inert across positions: cleanSheetPoints is 4 for GKP and
//     DEF, 1 for MID and 0 for FWD, and FPL's formations let the squad trade a
//     defender for a forward, so it can still move a squad.
//   - The **factor** is the within-position reprice. exp(-0.1731x) runs
//     0.920 / 0.794 / 0.640 across the 10th/50th/90th xGC percentiles
//     (0.480 / 1.330 / 2.578), a 1.44x spread between a defender behind a good
//     defence and one behind a leaky one. The position-wide exemption that lets
//     the ~30% over-prediction ship uncorrected does NOT cover this arm.
//
// # Pre-registration
//
// Stated before the run, because the temptation afterwards is to explain
// whichever way it went.
//
//   - **P1, the mediator, read BEFORE any points column.** The number of cells
//     whose opening fifteen differs from the shipped arm's, per arm. A knob that
//     changes no squad cannot change points, and this package's signature
//     failure is a byte-identical null read as "the knob does nothing" when it
//     means "the knob did not arrive". The CANARY arm exists to make that
//     failure loud: it must move the fifteen in most cells, and if it does not,
//     **no other arm's null in this run means anything** and the run is void.
//   - **P2, expected UNRESOLVED on points.** This is the expected reading and is
//     not evidence against an effect. The threshold must come from these cells
//     via variance_components.R and never from the global HOLD median of 33.
//     Nearly every constant in this record is worth 11-34 points a season
//     against a median detectable 39.
//   - **P3, direction, the only directional claim.** Both corrections reduce
//     predicted clean sheets, so both arms move points DOWN relative to shipped
//     on any measure that rewards the old over-prediction — and the question is
//     whether the reordering pays for it. No sign is predicted for the paired
//     HOLD difference. Predicting one would be dishonest: the record's own rule
//     is that correcting a measured bias has lost points five times.
//   - **P4, the interaction is the reason for the design, and it is NOT expected
//     to resolve either.** Its estimate is reported with the two main effects
//     whatever it does. A difference of differences within one cell cancels the
//     path divergence a single difference carries, which is why its clustered SE
//     should be SMALLER than the main effects' — measured elsewhere in this
//     record at 0.216 against 0.599. If it comes back larger, something is wrong
//     with the pairing, not with the football.
//   - **P5, multiplicity is bounded and stated in advance.** Three contrasts at
//     k = 2 (two main effects and the interaction), which is 2^k - 1 and is free
//     at two levers. No fourth contrast is licensed by this design, and the
//     "both" arm's level against shipped is NOT a fourth finding — it is the sum
//     of the two main effects and the interaction by construction.
//
// # What this is not
//
// It is not a re-run of the recorded four-total ladder — 8116 / 8191 / 8106 /
// 8115 — which is retracted on provenance: four bare season totals with no cell
// count, no paired difference and no SE, whose cells were never banked, which
// predate the doubles fix by a day and a half so every arm ran ~106-115 a season
// light, and whose baseline is unit-identical to a MinutesWeightByPosition value
// the same commit withdrew. Those totals must not be re-quoted as a measurement
// and nothing here reproduces them.
//
// It does not re-open the mechanism argument. The term ships uncorrected because
// a bias shared by every player in a position is not an ordering error and FPL
// forces five defenders regardless. That argument is independent of the points
// and survives any null here; this run can only bear on whether the points
// CONTRADICT it.
//
// It is not the defensive half of FPL_DEF_FIXTURE_SCALE, which sits in the same
// exponent and joins this 2x2 if it is ever picked up, rather than getting its
// own sweep.
//
// HOLD is the metric: this is a scoring constant, and HOLD excludes the transfer
// path's 303-point noise. POLICY is collected because the sweep collects it — do
// not report it as the answer.

import (
	"testing"

	"armband/internal/analysis"
)

func TestDiagCleanSheetXGC(t *testing.T) {
	requireDiag(t)
	starts := sweepStarts()

	// Restore what was found rather than what the shipped default is assumed to
	// be, and restore it on a fatal: both knobs are env-settable, so running this
	// suite under FPL_CS_XGC_FACTOR would otherwise leave one arm's value
	// installed for every later diagnostic in the process while the fingerprint
	// still stamped the environment's value.
	priorFactor, priorScale := analysis.CleanSheetState()
	defer func() {
		analysis.SetCleanSheetXGCFactor(priorFactor)
		analysis.SetCleanSheetScale(priorScale)
	}()

	// Both arms come from the JOINT fit and never from the constrained ones —
	// the constrained fits are the two families this design exists to avoid
	// choosing between.
	const (
		shippedFactor = 1.0
		shippedScale  = 1.0
		fittedFactor  = 1.1731
		fittedScale   = 0.9046
	)

	cs := func(factor, scale float64) func(sc *SimConfig) {
		return func(sc *SimConfig) {
			sc.WeeklyXI = true
			analysis.SetCleanSheetXGCFactor(factor)
			analysis.SetCleanSheetScale(scale)
		}
	}

	runPolicySweep(t, []policyVariant{
		{label: "shipped f=1.0 flat=1.0 (baseline)", apply: cs(shippedFactor, shippedScale)},
		{label: "factor only f=1.1731 flat=1.0", apply: cs(fittedFactor, shippedScale)},
		{label: "flat only f=1.0 flat=0.9046", apply: cs(shippedFactor, fittedScale)},
		{label: "both f=1.1731 flat=0.9046 (the joint fit)", apply: cs(fittedFactor, fittedScale)},
		// The canary. Halving every clean sheet is not a candidate setting and is
		// not read as one; it exists so that a byte-identical null on the four
		// arms above can be told apart from a knob that never arrived. P1 voids
		// the run if this one does not move the fifteen.
		{label: "CANARY f=1.0 flat=0.5 (must move the fifteen)", apply: cs(shippedFactor, 0.5)},
	}, starts)
}
