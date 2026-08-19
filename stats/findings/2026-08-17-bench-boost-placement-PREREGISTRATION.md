# Bench-boost placement, sized against its own canary: pre-registration

**Written and committed before a single cell was replayed.**
`TestDiagBenchBoostPlacement`, `internal/backtest/benchboostplacement_diag_test.go`, run through
`scripts/replay` with `FPL_SWEEP_SEASONS=extended` — the six-season grid (2020-21 … 2025-26) at
entry gameweeks 1/6/11/16/21/26, **36 cells**. `FPL_MAGNITUDE` unset. All archive repairs on.

Two blocks, and **the second is gated on the first**:

| block | arms | question |
|---|---|---|
| `BBCEILING` | no chip / bench boost at entry+6 / `AxisChipWeek` | how big is perfect placement over the fixed offset? |
| `BBRULE` | no chip / bench boost at entry+6 / `BenchBoostTrigger` | does the state rule beat the fixed offset? |

## Why a canary comes first

This record's clean-sheet family spent 180 cells discovering that its candidate was about 4x below
detection **before the run started**, and its own conclusion is *size a candidate against a canary
before spending*. The canary here is exact rather than approximate: a placement rule's pick is one
element of the per-week gain slice the oracle's argmax maximises over, so on an identical path

    rule_i − control_i  ≤  ceiling_i  =  max_w gain_i(w) − gain_i(control week)

**in every cell, by construction.** If the ceiling cannot be told from the cell-to-cell dispersion
of this comparison, no rule can be.

## The premise: a bench boost is PATH-INVARIANT, and it is checked, not assumed

Three code facts in `Simulate`: the trigger is consulted after `pickXI` and `consult` mutates only
`chipTriggers`; playing the chip sets `chip = chipBenchBoost` and reaches `weekScoreWithChip` alone,
touching neither `free` nor the wallet nor `held`; and `week.BenchBoostGain` is recorded in **every**
week against the *unchipped* week. So the arms hold the identical fifteen every week, make the
identical transfers, and differ only in which week is scored under the chip.

**That argument is not the evidence.** The diagnostic executes three checks against the no-chip
baseline arm, each a `t.Error`:

1. `squad_hash`, `moves`, `hits` identical — the weak one, a code fact.
2. The **whole per-week `BenchBoostGain` vector** identical across every arm of the block. This is
   what makes the oracle's argmax and the control's pick two readings of *one* slice.
3. `policy_points(arm) − policy_points(baseline) == bench_boost_pts(arm)`, **exactly, in integers**.
   `weekPointsWithChip` is `weekScoreWithChip(...).Points` and `BenchBoostGain` is the difference of
   exactly those two calls, so this holds if and only if every other week scored identically.

**If any of the three fails, the design is void and the run reports that instead of a figure.**

## Estimand: PER SEASON-PATH. Named now, because the two disagree by 32% on this channel

A chip pays in **one gameweek**, so the cell total is the whole effect. Every difference below is
`points per season-path` — the cell total itself, one number per cell, six entry points weighted
equally. It is **not** divided by weeks played and **not** multiplied by 38. `reportChipCells`
already carries this instruction for the sibling quantity; this record separately measures
`per_gw × 38` and `per_path` disagreeing by 32% on the bench channel, so naming which is used is
not a formality.

Per cell `i`:

- **ceiling** `C_i = bench_boost_oracle_pts_i (arm 2) − bench_boost_pts_i (arm 1)`
- **rule** `R_i = bench_boost_pts_i (arm 2, BBRULE) − bench_boost_pts_i (arm 1)`

Both are cross-arm reads on the same cell key `(season, start_gw)`, which is legitimate **only**
under check 2 above.

## Inference

`stats/bench_boost_placement.R`, sourcing `cells_common.R` for the reader, the CR2 estimator, the
`t_crit(df) × SE` threshold and the wild cluster bootstrap. No copy of any of them.

- **Threshold** is `t_crit(df) × SE`, with **df taken from the contrast** (Satterthwaite, CR2), not
  assumed to be 5. Reported on **both** estimators side by side as a range: season-clustered
  (primary) and entry-point-clustered.
- **80%-power MDE** beside it, from `sig_and_mde`.
- **Wild cluster bootstrap** p beside every clustered t, Webb 6-point weights enumerated exactly,
  quoted with `S_eff` and the floor `6/6^S_eff`. An arm whose floor exceeds 0.05 is **unmeasurable**,
  not null.
- **Leave-one-season-out** on both arms, with the standing caveat that subsets sharing five of six
  seasons make sign stability arithmetic rather than evidence.

## The gate, stated before the numbers exist

**Proceed to `BBRULE` if and only if `mean(C) > t_crit(df) × SE(C)` on the season-clustered
estimator.**

⚠️ **This is a SIZING criterion and not a hypothesis test, and its p-value is meaningless.**
`C_i ≥ 0` in every cell by construction, so a t against zero is mechanical — the same status this
record assigns the perfect armband's t of 20.4. What the criterion asks is whether the *largest
attainable* effect is big relative to this comparison's own dispersion. The legitimate reading of
the ceiling itself is a **one-sided lower confidence bound**, which is also reported.

If the gate fails, `BBRULE` is not run and the deliverable is the ceiling with its bound.

## Holm family

**One member per stage.** Stage 1 tests exactly one contrast (the ceiling, as a sizing); stage 2, if
it runs, tests exactly one (`R`). No Holm correction is applied and none is owed: a single contrast
at each stage, with stage 2 conditional on stage 1. This is declared here so the family cannot grow
after the numbers are seen.

## Cells that cannot carry the intervention

Counted and printed **before** either sweep, because a cell where the chip never plays is a
comparison that could not run and is not a zero.

- **Control placement.** `benchBoostControlPlan` puts the boost at `start + 6`, which is GW7…GW32 on
  this grid, so all 36 cells are expected to place it. The census prints the per-cell week and the
  run fails outright if none places.
- **Rule placement.** The trigger may never fire. **The liveness count is `bench-boost gameweek
  differs from the control` out of 36. If that is zero, the deliverable IS that count**, not a null:
  the rule did not act, so there is nothing for a paired difference to be about. The
  `bb_trig_offered/weighed/consulted` funnel is printed per cell so "off", "blocked all season" and
  "reached its bar and declined" are distinguishable.
- The sibling anchored triple captain places in only 23 of 36 cells, which is why this is checked
  rather than assumed.

## Predictions, stated before any cell is read

1. **The ceiling is predicted positive and its size is unpredicted.** Positivity is arithmetic and
   counts as nothing. The recorded family figures — timing **+8.3** a season over the threshold rule
   and **+21.9** over the median week, both functions of the asserted `chipBarBenchBoost` of 16 —
   are an interval on a bound and **not** a prediction for this arm: this control is the fixed
   offset, which is neither of those comparators.
2. **The gate is predicted to be marginal.** If the ceiling lands near the recorded 8-22 and the
   per-cell dispersion of a one-week chip gain is of the same order, the ceiling will sit close to
   its own threshold. A gate failure is a live outcome and is not a failed run.
3. **`R` is predicted not to resolve even if the gate opens.** `R ≤ C` cellwise, so the rule arm
   competes for a fraction of a quantity that only just cleared.
4. **Nothing will be recommended for shipping.** The lever ships off and the deliverable is a
   measurement, not a default change.

## What would change a verdict

Only a resolving contrast: `|t|` above `t_crit` at the contrast's own df, with the wild bootstrap
not withdrawing it and its floor below 0.05, and a leave-one-season-out that does not change sign.
An argmax over placement weeks is **not** a deliverable and no winning week will be reported as one.

## Conditions, named because a level means nothing without them

- **`WeeklyXI = false`**, the `runPolicySweep` default. This is a **simple-effect** condition: this
  record warns that a doubles arm leaving `WeeklyXI` false has switched off the fielding half of its
  own mechanism. Here it does not invalidate the contrast — every arm shares one XI and one bench in
  every week, which check 2 verifies — but it does set the *level* of the gain profile, so the
  `WeeklyXI = true` corner is **untested rather than equivalent**.
- **No wildcard, no free hit, no triple captain in any arm.** The wildcard is out because pinning it
  to a common week put the boost immediately after the rebuild in 30 of 30 cells for one arm and 3-5
  of 30 for the others. The free hit is out because it replaces `fielded` for a round, which would
  make `BenchBoostGain` describe a borrowed bench. The triple captain is out because it occupies a
  week the boost could otherwise use.
  ⚠️ **The consequence, stated rather than buried:** every arm boosts a squad the ordinary objective
  built, and that objective credits the bench at almost nothing. The floor cancels for a *placement*
  contrast, since both arms carry it — but a flat bench flattens the gain profile across weeks, so
  **the ceiling measured here is itself a floor on the ceiling** and a null is weaker evidence than
  it looks.
- **`BBRULE` runs with congestion damping OFF**, so the decay shape is identified on its own.
  ⚠️ **`OptionPricing.CongestionSensitivity = 0` does NOT do that.**
  `analysis.CongestionFactor` reads `if sensitivity <= 0 { sensitivity = DefaultCongestionSensitivity }`,
  so a zero silently selects the package default of **1.0** — the strongest setting, not the absent
  one. An arm written that way would be a comparison that never ran wearing the clothes of one that
  did. The arm therefore sets `1e-12`, at which the factor is within 1e-12 of exactly 1 for every
  load the archive can produce. `TestCongestionSensitivityZeroIsTheDefaultNotOff` pins both halves.
- **`BenchBoostBar` at `config.DefaultBenchBoostBar` = 16**, which is `chipBarBenchBoost` carried
  across and is **asserted, not measured**. It is not varied here: the change under test is
  placement, and moving the level at the same time would make the two inseparable.
- **`BankUpTo = 5`** everywhere (`sweepBankLimit`), so absolute totals are not comparable with
  figures measured on `BankLimitFor`.
- All archive repairs on; no `FPL_NO_XG_REPAIR`, `FPL_NO_XGC_REPAIR`, `FPL_NO_XG_AGGREGATE` or
  `FPL_NO_STARTS_REPAIR`. `FPL_MAGNITUDE` unset.
