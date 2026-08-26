package backtest

// Should the replay grid widen from four played seasons to six?
//
//	DIAG=1 FPL_CELLS=/tmp/gw7.csv FPL_SWEEP_SEASONS=scoring EXP=grid7 \
//	    go test ./internal/backtest -run TestDiagGridWidth -count=1 -v -timeout 3h
//	DIAG=1 FPL_CELLS=/tmp/gw4.csv FPL_SWEEP_SEASONS=default EXP=grid4 \
//	    go test ./internal/backtest -run TestDiagGridWidth -count=1 -v -timeout 3h
//	Rscript stats/grid_width.R /tmp/gw7.csv /tmp/gw4.csv
//
// # The question, and why it is not simply "more seasons is better"
//
// Almost every constant this package argues over is worth 11 to 34 points a season
// and the harness cannot resolve any of them. The reason is the season axis: a
// season-clustered standard error rests on S-1 degrees of freedom — the number of
// independent readings you have of how much a result wobbles — and four seasons give
// three, so the 5% critical value is 3.182 rather than the familiar 2. Six seasons
// give five and 2.571. The threshold scales as t_crit(S-1)/sqrt(S), which is 0.66 of
// today's, so the canonical median of 39 points a season would become about 26 and
// `HOLD`'s 33 about 22. That is the first time the upper half of the 11-to-34 band
// would be reachable.
//
// The alternative — densifying the entry-point grid — buys 20% off the standard error
// and **cannot move the degrees of freedom at all**, because it adds no new football.
//
// So the arithmetic is not in question. What is in question is its premise: **it
// assumes the two added cells are as quiet as the four that ship, and they are
// reconstructed rather than observed.** 2019-20, 2020-21 and 2021-22 carry no FPL
// expected-goals column at all, so `xgRepairs` backfills them from Understat on a
// *borrowed* provider offset — one fitted on other seasons, because these have nothing
// to fit against. The level error that leaves is the benign kind, shared by every
// player in a season, and an argmax consumes an ordering. The per-player dispersion is
// not benign: the record puts the 90th percentile of the per-player ratio between the
// two providers at 1.54, and no rescaling removes it.
//
// A noisier cell **attenuates a paired difference toward zero** as well as widening
// its spread. So six cells do not automatically buy what six clean cells would, and
// the gap between the measured gain and the theoretical 0.66 *is* the price of the
// borrowed correction — a quantity nobody has put a number on.
//
// # PRE-REGISTRATION
//
// Written and committed before the first cell was run, because this package's own
// rule is that a predicted ordering must be committed in advance: picking the
// best-looking result out of several manufactures effects, and a decision about the
// instrument itself is the worst possible place for that.
//
// The design is one sweep of seven arms against one baseline, run on two grids. The
// grids are **nested** — the shipped four pairs are a subset of the extended six,
// which are a subset of the scoring seven — so the four- and six-season answers are
// filtered out of the seven-season cells file rather than measured separately. That
// removes the run-to-run confound entirely: the comparison is literally the same
// cells, plus more of them. `TestTheGridsAreNested` pins the subset relation, and the
// separate `default` run above exists to *verify* the filtering reproduces an
// independently-run four-season sweep cell for cell.
//
// **The positive control** is the vice-captain fallback (`viceCaptainFallback` in
// replay.go: passes the armband to the vice when the captain records no minutes).
// It is the sharpest effect on record — it resolves at 12.7 points a season even on
// the noisier metric, because its mechanism is certain and it lands almost identically
// in every cell. Reported here as `vice off − shipped`, so it carries the opposite
// sign to the +0.46 pts/gw in the record.
//
// **The negative controls** are the four near-null arms from `TestDiagNoiseFloor`:
// 1-2% nudges to scoring constants, far too small to be real model changes and far
// too large to be invisible. None of them resolves on four seasons. Plus the identical
// control, which must difference to exactly zero.
//
// The five conditions, and the ordering predicted for each:
//
//	P1  POSITIVE CONTROL REPRODUCES. The six-season mean paired difference for the
//	    vice arm must keep its sign and lie inside the four-season 95% confidence
//	    interval. Predicted: it does. If a mechanism-certain effect moves when two
//	    seasons are added, the added seasons are not measuring the same thing.
//
//	P2  NEGATIVE CONTROLS STAY NULL. No near-null arm may reach Holm-adjusted
//	    p < 0.05 on HOLD on six seasons. The identical control must read exactly
//	    0.000 with zero spread in every cell. Predicted: they stay null. An arm that
//	    starts resolving is evidence the wider grid manufactures effects, and that
//	    alone refuses adoption regardless of every other column.
//
//	P3  THE THRESHOLD IMPROVES, BY LESS THAN THEORY SAYS. Realised significance
//	    threshold is qt(0.975, df_Satt) x SE_CR2 x 38, in points a season — the same
//	    definition `variance_components.R` uses for `sig_season`, reused rather than
//	    restated so the two cannot drift. Predicted ordering, committed:
//	        sig_season(6) < sig_season(4)  and  sig_season(6)/sig_season(4) > 0.66.
//	    The strict inequality against 0.66 is the interesting half. Theory assumes
//	    equivalent cells; the backfilled cells are not equivalent; so the realised
//	    ratio should fall short of the theoretical one, and by how much is the price
//	    of the borrowed offset.
//
//	P4  ATTENUATION IS NOT MATERIAL. Split the positive control's paired differences
//	    by whether the played season's xG is native or backfilled on a borrowed
//	    offset. Predicted ordering: |mean(backfilled)| <= |mean(native)|, because a
//	    noisier input attenuates. **Material** is pre-declared as the backfilled mean
//	    falling below half the native mean. Below half, the reconstructed cells are
//	    diluting a known effect and six-as-default is refused in favour of six for
//	    confirmation only.
//
//	P5  DEGREES OF FREEDOM ACTUALLY RISE. Report the *realised* Satterthwaite df,
//	    not the nominal S-1. Satterthwaite df on six clusters can land well below
//	    five when the clusters are unbalanced in influence. Predicted: df(6) > df(4).
//	    If it does not rise, the widening bought nothing whatever the point estimates
//	    say.
//
// The adoption rule, also committed in advance, so the verdict is read off the table
// rather than argued from it:
//
//	P2 fails                              -> refuse. The grid manufactures effects.
//	P1 fails                              -> refuse. The cells are not comparable.
//	P4 material, or P5 fails, or P3 shows
//	   no improvement                     -> six for CONFIRMATION ONLY, not default.
//	all five hold                         -> adopt six as default, and say in the same
//	                                         breath what happens to the four-season
//	                                         record.
//
// # What this deliberately does not do
//
// It does not re-tune anything. Re-tuning a constant on the new grid is the payoff and
// it comes after this decision, not inside it — running both at once would let a
// constant's result argue for the grid that produced it.
//
// It also does not change the default. That is a one-line change with the whole
// research record as its blast radius, and it is a human's call.
//
// # POSTSCRIPT, 2026-08-11, written after the run and after review
//
// Everything above is preserved exactly as it was written before any cell ran. A
// pre-registration edited after its result is no longer a pre-registration, so the
// two sentences that are now false — the 12.7 quoted below, and "it also does not
// change the default" — stay as they are. The default was changed at `0b994d5` on a
// separate request.
//
// Three things review established that this document got wrong about itself:
//
//   - **P1 could not have failed under total dilution.** It required the six-season
//     mean inside the four-season interval [0.0834, 0.7369]; two added seasons at
//     exactly zero effect give 0.2735, which passes. With P4 separately powerless,
//     neither dilution guard could fire. Dilution is unmeasured, not excluded.
//   - **P3's shortfall was reported backwards.** This document predicted the realised
//     ratio would fall short of theory's 0.660 and named the shortfall as the price
//     of the reconstruction. The positive control came in at 0.677, which *does* fall
//     short — the prediction was confirmed. VERDICT.txt printed "at or better than
//     theory" by taking a median across three arms, two of which are near-degenerate.
//   - **The threshold definition was restated rather than reused.** This document
//     claims the two cannot drift because the definition is shared. It is not:
//     variance_components.R uses method-of-moments with df = S-1, grid_width.R uses
//     CR2 with Satterthwaite df. That is why the record's 12.7/17.1 and this pass's
//     14.4/12.4 disagree *and swap the metrics over* — different estimators, not a
//     mislabel. Which is canonical is undecided; see AGENTS.md.
//
// The 12.7 in the P1 paragraph is variance_components.R's figure. Do not "correct" it
// to this pass's numbers: they are not the same quantity.

import (
	"os"
	"testing"
)

func TestDiagGridWidth(t *testing.T) {
	if os.Getenv("DIAG") == "" {
		t.Skip("set DIAG=1")
	}

	// The shipped conventions, matching TestDiagBaseline and TestDiagNoiseFloor so
	// the arms are comparable with the noise-floor run they are borrowed from.
	//
	// `viceCaptainFallback` is set explicitly here rather than left alone, and that
	// is load-bearing. It is a package-level var, `apply` mutates it, and arms run in
	// sequence — so if the vice arm turned it off and no later arm turned it back on,
	// every negative control would silently run with the armband fallback disabled
	// and be measuring two changes at once. This package's recorded failures are
	// mostly of exactly this shape: a global left in the state the previous arm put
	// it in, with nothing to say so.
	base := func(sc *SimConfig) {
		sc.WeeklyXI = true
		viceCaptainFallback = true
	}

	v := []policyVariant{
		{label: "shipped (baseline)", apply: base},

		// The positive control. Sign is "vice off minus shipped", so it should be
		// NEGATIVE — turning a real benefit off costs points.
		{label: "vice off (positive control)", apply: func(sc *SimConfig) {
			base(sc)
			viceCaptainFallback = false
		}},

		// The determinism control. Applies precisely what the baseline applies, so
		// its paired difference must be exactly 0.000 in every cell. If it is not,
		// the replay is nondeterministic and every paired figure in the record is
		// measuring that too — a far larger finding than anything about grid width.
		{label: "identical (control)", apply: base},

		// The negative controls, verbatim from TestDiagNoiseFloor so the two runs
		// are directly comparable. None of these resolves on four seasons; the
		// question is whether any starts to on six.
		{label: "minutes_weight +2%", apply: func(sc *SimConfig) {
			base(sc)
			sc.Weights.MinutesWeight = 1.275
		}},
		{label: "minutes_weight -2%", apply: func(sc *SimConfig) {
			base(sc)
			sc.Weights.MinutesWeight = 1.225
		}},
		{label: "fixture_weight +1.5%", apply: func(sc *SimConfig) {
			base(sc)
			sc.Weights.FixtureWeight = 0.66
		}},
		{label: "bonus_weight +0.7%", apply: func(sc *SimConfig) {
			base(sc)
			sc.Weights.BonusWeight = 1.51
		}},
	}

	runPolicySweep(t, v, sweepStarts())

	// Restored for any test sharing this process, on the same reasoning as the
	// explicit set in `base`.
	viceCaptainFallback = true
}

// TestTheGridsAreNested is what licenses filtering the four- and six-season answers
// out of one seven-season cells file instead of running three sweeps.
//
// The saving is real — three runs become one, and 460 cells become 294 — but that is
// not why it matters. Three separate runs would differ by more than their season
// count if anything else drifted between them, and the whole output of this pass is a
// *comparison between grids*: a confound there is a confound in the verdict. Nested
// grids make the comparison literally the same cells plus more of them.
//
// Nesting is asserted **pair by pair**, not season by season. A cell is identified by
// {prior season, played season, start gameweek}, because the model is built from the
// prior and scored on the played one — two grids could agree on which seasons they
// play while pairing them differently, and every filtered cell would then be matched
// against a different model. That is the same class of error as a diagnostic
// measuring a different population from the sweep it is quoted beside, which is what
// `TestTheGridIsDeclaredOnce` exists for.
func TestTheGridsAreNested(t *testing.T) {
	// sweepPairNames honours FPL_SWEEP_SEASONS, so the shipped four must be asked
	// for explicitly rather than inherited from whatever the environment says.
	t.Setenv("FPL_SWEEP_SEASONS", "default")

	shipped, extended, scoring := sweepPairNames(), extendedPairNames(), scoringPairNames()

	// Chronological order is part of the claim: the wider grids must *prepend*
	// earlier seasons, never reorder or replace. A grid that grew in the middle
	// would still pass a set-membership check.
	suffix := func(name string, wide, narrow [][2]string) {
		t.Helper()
		if len(wide) < len(narrow) {
			t.Fatalf("%s has %d pairs, fewer than the %d it must contain",
				name, len(wide), len(narrow))
		}
		off := len(wide) - len(narrow)
		for i := range narrow {
			if wide[off+i] != narrow[i] {
				t.Errorf("%s pair %d is %v, want %v — the wider grids must extend the "+
					"narrower ones backwards in time, so a cell filtered out of the "+
					"wide file is the same cell the narrow run would have produced",
					name, off+i, wide[off+i], narrow[i])
			}
		}
	}
	suffix("extendedPairNames", extended, shipped)
	suffix("scoringPairNames", scoring, extended)

	// The counts the pre-registration's arithmetic rests on. Stated as numbers
	// rather than derived, because "six seasons" appearing in a verdict while the
	// grid quietly held five is exactly the silence FPL_SWEEP_SEASONS panics over.
	if len(shipped) != 4 || len(extended) != 6 || len(scoring) != 7 {
		t.Errorf("grids are %d/%d/%d pairs, want 4/6/7",
			len(shipped), len(extended), len(scoring))
	}

	// The env switch must reach the seventh grid, or the scoring run above silently
	// measures the shipped four while its operator believes otherwise.
	t.Setenv("FPL_SWEEP_SEASONS", "scoring")
	got := sweepPairNames()
	if len(got) != len(scoring) {
		t.Fatalf("FPL_SWEEP_SEASONS=scoring gave %d pairs, want %d", len(got), len(scoring))
	}
	for i := range scoring {
		if got[i] != scoring[i] {
			t.Errorf("FPL_SWEEP_SEASONS=scoring pair %d = %v, want %v", i, got[i], scoring[i])
		}
	}

	// And exactly one season in the seventh grid must be POLICY-incomparable, which
	// is what makes that grid HOLD-only and what the guard in runPolicySweep fires
	// on. Both directions: a grid that quietly became fully comparable would drop
	// the guard, and one that gained a second such season would need a wider caveat
	// than "2019-20".
	var holdOnly []string
	for _, p := range scoring {
		if !TransferPathComparable(p[1]) {
			holdOnly = append(holdOnly, p[1])
		}
	}
	if len(holdOnly) != 1 || holdOnly[0] != "2019-20" {
		t.Errorf("the scoring grid plays %v that POLICY cannot use, want exactly "+
			"[2019-20]", holdOnly)
	}
	for _, p := range extended {
		if !TransferPathComparable(p[1]) {
			t.Errorf("the extended grid plays %s, which POLICY cannot use — extendedPairNames "+
				"is meant to be valid on both metrics and only the seventh pair "+
				"gives that up", p[1])
		}
	}
}
