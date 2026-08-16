# `BlendRateK`: pre-registration, written before the sweep ran

**Written 2026-08-14 at `e3c50b4`, before `EXP=BLEND` was launched.** The results go in
`FINDINGS.md` beside this file; this document is not edited after the run except to mark, in place,
anything it got wrong.

## Why this run exists

The schedule screen (`stats/snapshots/2026-08-14-schedule-screen/`) was queued as free R over
committed cells, with **`BlendRateK` named as its strongest candidate**. It could not screen it:
the `want("BLEND")` block has existed in `internal/backtest/transferpolicy_test.go:805-818` and was
**never run to the archive**. Zero rows carrying `BlendRateK` under `stats/snapshots/*/cells/`.

So this banks them. It is the third instance of "a constant having been *swept* does not mean its
cells were *banked*", after `BandStrength`.

## The arm

```
EXP=BLEND FPL_CELLS=stats/snapshots/2026-08-14-blend/cells/blend.csv \
  scripts/replay -run TestDiagRejudge -v -timeout 3h
```

Four arms — `BlendRateK` **8 (ships)**, 12, 16, 24 — on the shipped six-season grid at entry
gameweeks 1/6/11/16/21/26. **144 cells**, 36 per arm. `FPL_SWEEP_SEASONS` is left unset, which is
`extendedPairNames()`, the six that ship. `scripts/replay` sets `DIAG=1`.

## The metric, and the consumer that licenses it

**`HOLD`.** `BlendRateK` is a scoring constant, and the standing rule puts scoring constants on
`HOLD`. Checked against the consumer rather than assumed, per *"check that a setting is read on the
path you are about to score, before you score it"*:

`internal/analysis/blend.go:359` reads `e.Weights.BlendRateK` into `rk` and mixes eight per-90 rates
— `XG90`, `XA90`, `XGC90`, `DefCon90`, `Bonus90`, `Saves90`, `Yellow90`, `Red90` — plus `b.Weight`,
at `w = n90/(n90+rk)` where `n90 = el.Minutes/90`. Those feed `PlayerMetrics`, which both the
opening `Optimize` and the weekly XI re-pick consume, and `HOLD` runs both. It is **not** confined
to `decide()`, so `HOLD` is a measurement here rather than a theorem.

**`POLICY` is reported but is not the arm.** The transfer path also reads these rates, so `POLICY`
will move; it is the noisier metric and this is not a transfer constant.

## Predictions, registered in advance

**1. Points, and this one is not mine — it is already in the block's own comment**, written before
any of this session's work: *"raising k should help, and should help most in the middle regime."*
The reasoning recorded there is that `TestDiagCalibrationDrift` puts the expected/actual ratio at
1.013 through GW12, **0.916 from GW16 to GW28** and 1.004 by GW32, with the *expected* column moving
while actuals stay flat — the model gets more confident rather than worse — and more shrinkage pulls
an inflated tail down relative to the middle. **Predicted sign: positive** for 12, 16 and 24 against
8 on `HOLD`.

⚠️ **MARKED IN PLACE 2026-08-14, after the run: the endpoint illustration below is wrong on the
source, though the prediction it supports is not.** "At `n90 = 0`" does not describe a GW1 build.
Two facts, both checkable: `internal/backtest/replay.go:82` sets `el.Minutes, el.Starts =
q.Minutes, q.Starts`, so pre-season the element carries **last season's** minutes and `n90` is ~25-30
for an ever-present, not 0 — which is CLAUDE.md's own standing rule that FPL's aggregates carry last
season's totals until GW1 completes. And `k` cannot touch the opening fifteen for a different reason
entirely: `blendFor` returns from its `played == 0` branch at `internal/analysis/blend.go:297`,
**before** `rk := e.Weights.BlendRateK` is read at `:359`. GW1 is a separate code path, not the limit
of the same curve. The in-season leverage argument — which is what the prediction rests on — is
unaffected.

**2. The schedule question, which is what this run is for.** `k`'s leverage is monotone in `n90`:
at `n90 = 0` the current season carries zero weight *whatever* `k` is, and the weight rises toward 1
as 90s accumulate. So a late entrant spends its whole scored window at high `n90`, where `k` is
doing the most work, while a GW1 entrant's 38 weeks average over the low-`n90` opening as well.
Combined with prediction 1 — that the benefit lives in the GW16-28 regime, which is *all* of a GW26
entrant's window and about a third of a GW1 entrant's — the pre-registered sign is:

> **`d(slope)/d(entry gw) > 0`**: the ladder's slope should become more positive at later entry
> points.

**3. What a positive result would NOT establish, registered now so it cannot be claimed later.**
Entry point moves the evidence regime and the scored calendar window **together**, and here they
point the same way, so a positive result cannot distinguish "the constant wants to be a function of
evidence" from "it wants to be a function of the calendar". For a blend weight those two readings
coincide more than for any other constant here — which is why this is the case the confound hurts
least — but coinciding is not the same as being separated.

## Power, registered in advance so the null is not read as news

The schedule screen's MDE on the six comparable ladders was **152 to 349 season points**, against
this grid's own median detectable of 39. `BlendRateK`'s ladder spans 8→24, a range of 16.

**The expectation is that this does not resolve.** That is not a reason to skip it: what the run
buys is the point estimate, the *shape* across four settings, and closing a gap where the record
currently has nothing at all. A null here is a tie — and specifically **not** evidence that
`BlendRateK` is flat.

⚠️ **This is an interaction formed BETWEEN cells**, across entry points that are different squads
scoring different football, so the recorded prior that an interaction is the *cheap* contrast
(CR2 SE 0.216 against 0.599) **does not apply** — that holds for a difference of differences within
one cell.

## What will be read, and in what order

1. `stats/sweep_inference.R` on the cells — the per-arm paired differences on `HOLD`, with CR2 and
   the fixed-block estimator, and Holm over the three non-baseline arms.
2. `stats/schedule_screen.R` on the same cells — the six per-entry columns, the ladder slope per
   column, and the schedule test with its MDE.
3. The ladder's **shape** across 8/12/16/24, because this record decides on shape rather than on
   which single value scored highest — an argmax over four noisy settings manufactures effects.

**The arms are 8/12/16/24 and the shipped value is the bottom of the range**, so a monotone increase
is confounded with "more shrinkage is always better" and cannot distinguish an interior optimum from
an unbounded one. If the ladder is monotone up at 24, the honest conclusion is that the range was
chosen badly, not that 24 ships.

## Two things this run cannot do

- **It cannot test `BONUS`**, the case that founded the schedule hypothesis, which is a
  two-dimensional family the screen refuses.
- **It cannot screen any transfer constant**, which are byte-identical on `HOLD`.
