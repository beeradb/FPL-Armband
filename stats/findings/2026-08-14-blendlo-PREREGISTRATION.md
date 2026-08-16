# `BlendRateK` below 8: pre-registration

**Written and committed before the run.** `EXP=BLENDLO scripts/replay -run TestDiagRejudge`,
6 arms × 36 cells on the six-season grid (2020-21 … 2025-26), entry gameweeks 1/6/11/16/21/26.

## Why this run exists

`stats/snapshots/2026-08-14-blend/` swept 8/12/16/24 with **8 as both the shipped value and the
lowest rung**, so the ladder starts at its own baseline. That is a design defect rather than a
result: `k=8` has no left-hand neighbour, so the run's one clean feature — 12 worse than both its
neighbours — cannot separate "12 is a trough" from "the surface is rough". Arms below 8 are the
only thing that gives the shipped value a two-sided read.

The whole ladder is re-run in **one** comparison rather than two arms added beside the banked four.
`sweep_inference.R` keys a comparison on `(run_id, sweep)` specifically so two runs of one block do
not pool into an over-confident sample, and the banked run is at a different commit. Six arms is
~16 minutes against the recorded four-arm 10m49s.

## The primary result is SHAPE, and it is not a winner

Picking the highest of six noisy estimates is the argmax this record warns about on every page, and
with six rungs it is worse than with four. **No arm will be recommended on the strength of scoring
highest.** What is readable:

- **Monotone across all six rungs** would be a genuine shape, and it is *not* forced by the
  construction — nothing here removes a line item, so the "monotonicity the construction forces"
  exclusion does not apply.
- **8 in a trough** (both neighbours worse) against **8 on a slope** (the low arms better) is the
  question the banked run could not ask.

## Predictions, stated before any cell is read

1. **The low arms are predicted NON-POSITIVE against 8.** The standing finding is that an argmax
   wants *more* shrinkage than a predictor does, and lower `k` is less shrinkage toward the prior.
   A positive `k=3` or `k=5` therefore counts as evidence **against a documented principle**, not
   as a free improvement, and would be written up that way.
2. **No arm is expected to resolve.** The banked arms carried CR2 SEs of 0.40–0.55 pts/gw against a
   `t_crit` of 2.571 on df 5, giving a detectable effect of roughly **39–54 points a season** while
   the entire recorded ladder spans 24. A null is the **expected** outcome here and is not evidence
   that the constant does nothing — this record's own rule that "unresolved is the expected reading
   for a real effect of that size" applies directly.
3. **`k=8` ships unchanged unless something resolves**, which follows from 2 and is stated here so
   the run cannot be read as a licence to move it on a point estimate.

## What would change a verdict

Only a resolving arm — |t| above `t_crit` at the comparison's own df, surviving Holm over the five
contrasts — or a monotone ladder across all six rungs with a consistent sign across seasons. Two of
six seasons carried the swing in the banked run and dropping both reversed its sign, so **a
leave-one-season-out check is part of the reading**, not an optional extra.

## Metric

**`HOLD`.** `BlendRateK` is a scoring constant; `POLICY` would add the transfer path's own
303-point noise floor to a question about rates. `POLICY` is recorded but not the arbiter.

## Data state

Six-season default grid, all repairs on (no `FPL_NO_XG_REPAIR`, no `FPL_NO_XGC_REPAIR`,
no `FPL_NO_STARTS_REPAIR`). Named here because a recorded level means nothing without it.

## One thing this run cannot answer

`shrinkToLeague` reads `LeagueShrinkK`, not `BlendRateK`, since `c509255`. These arms move **one**
anchor. The pre-split two-anchor intervention is `BLEND2` and is a different question.
