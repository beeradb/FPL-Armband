# Bench-boost placement resolves, because the comparison has no transfer path in it

**Run 2026-08-17** at commit `e579f30`, clean tree, `TestDiagBenchBoostPlacement`, two blocks
through `scripts/replay` on the **six-season grid at six entry points — 36 cells** each
(`FPL_SWEEP_SEASONS=extended`, `FPL_MAGNITUDE` unset, every archive repair on, `constants_digest`
`402be5b8e6d8`). Cells, provenance, console and inference banked in
`stats/snapshots/2026-08-17-bench-boost-placement/`. Pre-registered in
`stats/findings/2026-08-17-bench-boost-placement-PREREGISTRATION.md`, committed before the first
cell ran. Peak RSS 133 MB and 140 MB; 7m54s and 8m20s.

**Every figure below is POINTS PER SEASON-PATH.** A chip pays in one gameweek, so the cell total is
the whole effect. Do not divide by weeks played and do not multiply by 38.

## The premise, executed rather than argued

A bench boost is path-invariant on this code: the trigger is consulted after `pickXI` and mutates
only `chipTriggers`; playing the chip reaches `weekScoreWithChip` and nothing else; and
`week.BenchBoostGain` is recorded in every week against the *unchipped* week. Checked three ways
against the no-chip baseline arm, in both blocks:

| check | result |
|---|---|
| `squad_hash`, `moves`, `hits` identical | **36 of 36** cells, both non-baseline arms, both blocks |
| the whole per-week `BenchBoostGain` vector identical | **72 of 72** arm-cells, both blocks |
| `policy_points(arm) − policy_points(baseline) == bench_boost_pts`, exactly, in integers | **36 of 36**, both arms, both blocks |

The third is the sharp one: `weekPointsWithChip` **is** `weekScoreWithChip(...).Points` and
`BenchBoostGain` is the difference of exactly those two calls, so it holds if and only if every
other week scored identically. `HOLD` is 47105 in all three arms of both blocks.

**And the two runs are joinable**: the baseline and control arms are byte-identical across the two
blocks in 36 of 36 cells on `policy_points`, `squad_hash`, `moves`, `hits`, `bench_boost_gw` and
`bench_boost_pts`, so the three arms form one matched set.

That is why the bars this record usually applies are the wrong ones. **The threshold this
comparison carries is 2.06 to 3.31 points a season**, not the `POLICY` median of ~70 and not the
303-point transfer-path floor: there is no transfer-path divergence in it at all.

## Block 1 — the canary. Gate OPEN

Perfect placement (`AxisChipWeek`) against the bench boost at entry+6.

| | mean gain | zero in | min | max |
|---|---:|---:|---:|---:|
| control, bench boost at entry+6 | **3.69** | 11 of 36 | 0 | 15 |
| perfect placement, best week in hindsight | **19.83** | 0 of 36 | 4 | 28 |

**Ceiling = +16.14 points per season-path.** Season-clustered SE 0.8033, df 5.00, threshold
(`t_crit × SE`) **2.06**, 80%-power MDE 2.80; entry-clustered threshold 2.04. One-sided 95% lower
bound **+14.52**. Positive in **36 of 36** cells and **6 of 6** season means; leave-one-season-out
spans +15.63 to +16.50. The oracle chose **17 distinct weeks** and matched the control's week in
**0 of 36** cells.

⚠️ **The t of 20.09 and the wild bootstrap's 0.0009 are MECHANICAL and are not quoted as evidence.**
`C_i ≥ 0` in every cell by construction — the argmax ranges over the slice the control's pick is
drawn from — so a test against zero refers to a null the arithmetic already refuted. The gate was
declared in advance as a **sizing criterion**, and the legitimate reading of a bound is the
one-sided limit above.

The ceiling exceeds this comparison's own threshold by about eightfold, so the rule arm was run.

## Block 2 — the state rule against the fixed offset. It RESOLVES

`BenchBoostTrigger` at `config.DefaultBenchBoostBar` = 16, decaying to zero at the chip's expiry,
with congestion damping off.

**Rule − control = +5.778 points per season-path.**

| estimator | SE | df | t | threshold | MDE |
|---|---:|---:|---:|---:|---:|
| season-clustered (CR2) | 1.0316 | 5.00 | **5.601** | **2.65** | 3.60 |
| entry-point-clustered | 1.2864 | 5.00 | 4.491 | 3.31 | 4.49 |

Wild cluster bootstrap (Webb 6-point, enumerated, 6^6 draws) **p = 0.0096**, `S = 6`, `S_eff = 6`,
floor `6/6^6` = 0.000129 — so the p is far above its own floor and the arm is measurable rather
than pinned. **6 of 6 season means positive** (+2.67 to +9.50), 25 of 36 cells positive.
Leave-one-season-out spans **+5.03 to +6.40** with no sign change; leave-one-**cell**-out spans
+5.23 to +6.23, so no single cell carries it.

**Liveness.** The rule played a bench boost in **36 of 36** cells and put it in a **different week
from the control in 34 of 36**. The mediator over the two blocks' 36 cells reads 882 offered weeks,
521 consulted, 521 weighed — so the lever ran, formed a reading every week it was consulted in, and
fired everywhere. It landed on the oracle's own week in 4 of 36 cells and *below* the control in 8.

**It recovers 0.358 of the ceiling** (+5.778 of +16.139), arithmetic off the joined cells with no
interval attached.

## Why this resolves where nothing else in the chip family does

Not because the effect is large — 5.8 points a season is smaller than most constants this record
calls unresolved. It is because the *comparison* is clean. Both arms hold the identical fifteen in
every week, make the identical transfers and take the identical hits; the only thing that differs is
which of ~30 recorded per-week gains is added to the total. That is the same structural reason the
vice-captain fallback resolved at a threshold of 12.7 on a metric this record calls noisy: **the
mechanism is certain and lands in every cell**, so the paired standard error has nothing in it but
the football.

## What this does NOT show

- ⚠️ **It is not a case for turning the lever on, and the control is weak.** The control's own gain
  averages 3.69 and is **exactly zero in 11 of 36 cells**. Under a bench boost there are no
  autosubs, so a week's gain is the bench's points *minus the substitutions forfeited* — which is
  why a chip dropped on an arbitrary week is worth so little. `+5.778 over a fixed offset` is not
  `+5.778 over shipped`: **shipped plays no bench boost at all.** The rule's level against playing
  no chip is +9.47, which is a **different question** (should the lever be on) in a family that was
  not pre-registered here and carries no threshold in this run.
- **The level is a floor.** No arm plays a wildcard, a free hit or a triple captain, so every arm
  boosts a squad the ordinary objective built — one that credits the bench at almost nothing. A flat
  bench also flattens the gain profile across weeks, so **the ceiling measured here is itself a
  floor on the ceiling**.
- **Simple-effect, at four named settings.** `WeeklyXI = false` (the sweep default), congestion
  damping off, `BenchBoostBar` 16, `BankUpTo` 5. Each is a condition, not a neutral choice, and the
  `WeeklyXI = true` corner is *untested* rather than equivalent — this record warns that a doubles
  arm leaving it false has switched off the fielding half of its own mechanism. Here it cannot
  invalidate the contrast, because every arm shares one XI and one bench in every week, but it sets
  the level of the gain profile.
- **`chipBarBenchBoost` / `DefaultBenchBoostBar` = 16 is ASSERTED, not measured**, and the rule's
  value is a function of it. It was not swept and was not tuned on these cells — it predates the
  run — so this is not an argmax over bars. But a different bar is a different rule and this figure
  does not transport to one.
- **No placement week is recommended and none was chosen.** The rule is a fixed policy, not a search
  over weeks; the oracle's 17 distinct weeks are a mediator, not an answer.
- **Nothing here licenses flipping a default.** The lever ships off and that decision follows the
  measurement rather than being part of it.

## Two things found along the way

- **`OptionPricing.CongestionSensitivity = 0` does NOT switch congestion off — it selects the
  package default of 1.0**, the strongest setting. `analysis.CongestionFactor` reads
  `if sensitivity <= 0 { sensitivity = DefaultCongestionSensitivity }`, which is the correct
  unset-is-a-no-op convention for that struct and a trap for an arm meaning to hold the channel
  still. An arm written that way would be a confounded contrast wearing a clean one's clothes, with
  nothing downstream to say so. The rule arm sets `1e-12` instead;
  `TestCongestionSensitivityZeroIsTheDefaultNotOff` pins both halves.
- **A fixed-offset bench boost is worth almost nothing, and the reason is autosubs rather than the
  bench.** Zero in 11 of 36 cells. The chip's gain is bench points *less* the substitutions it
  forfeits, which is not how the recorded `chipBarBenchBoost` of 16 reads if one thinks of the chip
  as simply "the bench, added".
