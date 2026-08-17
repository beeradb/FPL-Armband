# Bench-boost placement is measurable at a threshold of 2.65, because the comparison has no transfer path in it

**Run 2026-08-17**, `TestDiagBenchBoostPlacement`, two blocks through `scripts/replay` on the
**six-season grid at six entry points — 36 cells** each (`FPL_SWEEP_SEASONS=extended`, every archive
repair on, `constants_digest` `402be5b8e6d8`, `dirty=false`). Peak RSS 133 MB and 140 MB; 7m54s and
8m20s. Cells, provenance, console and inference banked in
`stats/snapshots/2026-08-17-bench-boost-placement/`. Pre-registered in
`stats/findings/2026-08-17-bench-boost-placement-PREREGISTRATION.md`, committed before the first
cell ran. Inference: `stats/bench_boost_placement.R`.

⚠️ **The two blocks stamp different commits** — `b44088d` for `BBCEILING` and `e579f30` for
`BBRULE`. The intervening diff is `stats/bench_boost_placement.R` alone, which the replay does not
read, and **the evidence for that is byte-identity rather than the commit stamp**: the baseline and
control arms match across the two blocks on `policy_points`, `squad_hash`, `moves`, `hits`,
`bench_boost_gw` and `bench_boost_pts` in 36 of 36 cells, checked by the script's `--ceiling` join.

⚠️ **`FPL_MAGNITUDE` unset and the repair switches unset are the operator's record of the
invocation, not a stamped fact** — `*.provenance.csv` records `commit`, `dirty`,
`constants_digest`, `bank_up_to`, the seasons, the starts, the arms and the config constants, and
carries only `FPL_SWEEP_SEASONS` from the environment.

**Every figure below is POINTS PER SEASON-PATH.** A chip pays in one gameweek, so the cell total is
the whole effect. Do not divide by weeks played and do not multiply by 38.

## The premise, executed rather than argued

A bench boost is path-invariant on this code: `consult` runs after `pickXI` and mutates only
`chipTriggers`; playing the chip reaches `weekScoreWithChip` and nothing else; and
`week.BenchBoostGain` is recorded in every week against the *unchipped* week.

Checked as a **confinement** against the no-chip baseline arm, in both blocks:

| check | result |
|---|---|
| `squad_hash`, `moves`, `hits` identical | 36 of 36 cells, both non-baseline arms, both blocks |
| the whole per-week `BenchBoostGain` vector identical | 36 of 36 cells, both arms, both blocks |
| `Δpolicy_points == bench_boost_pts`, exactly, in integers | 36 of 36 cells, both arms, both blocks |

⚠️ **Read the denominators before reading that as strong.** For the `BBCEILING` oracle arm the
identity is `0 == 0` — `AxisChipWeek` plays no chip, and `mustNotMoveForAxis` already declares that
invariance — so it is **powerless by construction there**. It has content in **25 of 36** cells for
the control and **33 of 36** for the rule. The liveness half, which is the check with power, is the
chip's week moving in **34 of 36** cells.

⚠️ **The invariance is a property of the chip PLUS four switches, not of the chip.** Four channels
would break it and are all off here: `playWildcard` reads `cfg.plays(slotBenchBoost, gw+1)` through
`wildcardBuildsForBoost`, which is **on by default**, so a planned boost after a wildcast changes
`OptimizeRequest.BenchBoost` and therefore the squad; `chipCreditFor` reaches the transfer
valuation when `PrepareBenchBoost` is on; `trig.anyPlays` lets a planned boost suppress another
rule's firing; and with `TaperFreeTransferValue` on, this arm's own `congestionOff` would reach
`freeCost` and diverge the transfer path. **Do not quote "a bench boost is path-invariant"
unqualified.**

## Block 1 — the canary. Gate OPEN

| | mean gain | zero in | min | max | mean GW |
|---|---:|---:|---:|---:|---:|
| control, bench boost at entry+6 | **3.69** | 11 of 36 | 0 | 15 | 19.50 |
| perfect placement, best week in hindsight | **19.83** | 0 of 36 | 4 | 28 | 27.75 |

**Ceiling = +16.139.** Season-clustered SE 0.8033, df 5.00, threshold 2.06, 80%-power MDE 2.80;
entry-clustered threshold 2.04. One-sided 95% lower bound **+14.52**. Positive in 36 of 36 cells and
6 of 6 season means; LOSO +15.63 to +16.50. The oracle chose 17 distinct weeks and matched the
control's week in 0 of 36.

⚠️ **The t of 20.09 and the wild bootstrap's 0.0009 are MECHANICAL and are not evidence.** `C_i ≥ 0`
in every cell by construction. The gate was declared in advance as a **sizing criterion**.

⚠️ **The gate used the wrong arm's dispersion, and in the direction that flatters the instrument.**
What gates a rule arm is the *rule arm's* threshold, 2.65, not the canary's 2.06 — 22% looser. It
did not bite at a sixfold margin, but the pre-registration predicted a *marginal* gate, and on a
marginal one this decides the run. Recorded as a standing rule: **a canary is a necessary condition,
not a sufficient one.**

⚠️ **The ceiling is a mixture of six argmax problems.** The argmax ranges over 38 weeks at a GW1
entry and 13 at GW26, and the ceiling falls monotonically with it: 18.83 / 17.33 / 15.50 / 17.00 /
14.67 / 13.50. So +16.139 is nobody's season figure. It also picks the **entry week** in 2 of 36
cells, which `gw > start` forbids the rule and the offset cannot reach.

## Block 2 — the state rule against the fixed offset

A **decaying option reservation** based at `config.DefaultBenchBoostBar` 16, falling to zero at the
chip's expiry through `analysis.ChipBarAt`, congestion damping off. ⚠️ **"bar 16" mis-describes it**:
the realised bar at firing spans **3.79 to 18.14** across the 36 cells.

**Rule − control = +5.778.**

| estimator | SE | df | t | threshold | MDE |
|---|---:|---:|---:|---:|---:|
| season-clustered (CR2) | 1.0316 | 5.00 | **5.601** | **2.65** | 3.60 |
| entry-point-clustered | 1.2864 | 5.00 | 4.491 | 3.31 | 4.49 |
| naive, unclustered | 1.4377 | — | 4.019 | — | — |

All three reject. Wild cluster bootstrap **p 0.0096**, `S_eff` 6, floor 0.000129. **6 of 6 season
means positive** (+2.67 to +9.50), 25 of 36 cells positive. LOSO **+5.03 to +6.40** — sign stability
there is arithmetic, since each subset shares five of six seasons — and a post-hoc
leave-one-**cell**-out spans +5.23 to +6.23, so no single cell carries it.

**Liveness.** The rule placed the chip in **36 of 36** cells and in a **different week from the
control in 34 of 36**. Mediator: 882 offered weeks, 521 consulted, 521 weighed. It landed on the
oracle's own week in 4 of 36 and *below* the control in 8.

**Recovered fraction 0.358, Fieller 95% [0.198, 0.518]** — season-clustered, df 5, off the joined
cells, with the two marginal SEs checked against `se_cr` rather than trusted. It rejects both 0 and
1. ⚠️ **By entry point it spans 0.049 to 0.704**, and by season 0.186 to 0.509.

## ⚠️ The finding that limits everything above: the control is a straw man on TIMING

The rule plays the chip **8.5 gameweeks later** than the control — mean GW **27.97** against
**19.50**, and the oracle's own **27.75**. And that lateness is **designed in, not discovered**:
`ChipBarAt` decays the bar to the chip's expiry, so a rule that finds nothing early fires late by
construction.

**So this contrast cannot separate "reads state" from "plays late."** The free evidence cuts both
ways and does not settle it: the control column *is* a six-point fixed-week ladder — GW7/12/17/22/27/32
reading 3.17 / 4.33 / 3.67 / 4.17 / 5.00 / 1.83 — with no trend, so "later is generically better" is
unsupported; but those six points are confounded with entry point and squad, and GW32 is a
specifically bad week while the oracle wants GW34-37.

**The comparator that would settle it is a calendar anchor on the known big double** — this record
already holds that the biggest double is known in advance, and `anchoredPlan` already expresses it.
**It is unrun**, and it is the single thing owed by this work. It was deliberately not bolted on
here: an arm chosen after seeing the numbers is exactly the family growth the pre-registration was
written to prevent, so it wants its own pre-registration rather than a third column.

## What this does NOT show

- ⚠️ **Not a case for turning the lever on.** The rule's level against no chip is +9.47 and the
  control's is 3.69, and **both are ≥ 0 in every cell by construction** — under a bench boost the
  autosub step is skipped and the substitutes are a subset of the bench players who played, so
  `BenchBoostGain` cannot be negative. That is the ceiling's mechanical status again. Quote no t, no
  p and no threshold on either. **Nothing here weighs the chip against its own opportunity cost**,
  which is what "should the lever be on" asks.
- ⚠️ **The control is an AVERAGE week, not a bad one** — and this corrects an apology made before the
  numbers were read. Against the banked `bench_boost_median_pts`, the median week beats the fixed
  offset by **+0.583**, season-clustered SE 0.533, t 1.09, threshold 1.37 — **does not resolve**. So
  the fixed offset is indistinguishable from a randomly chosen week, which is precisely what a
  placement control should be. The content of "the control is bad" is a fact about the *chip*: a
  bench boost on an arbitrary week is worth about four points, because it forfeits the autosubs.
- **One floor argument and five conditions, and they are different things.** The floor:
  `prepare_squad_for_chips` is **off** (`prep_weeks` 0 in 36 of 36) and no arm plays a wildcard, free
  hit or triple captain, so every arm boosts a bench the ordinary objective credits at almost
  nothing. A flat bench *plausibly* flattens the gain profile across weeks, making the measured
  ceiling a floor on the ceiling — **a mechanism argument, not a measurement**, and the autosub
  credit a stronger bench would also earn runs the other way. **The conditions carry no direction at
  all** and are simple-effect: `WeeklyXI` false, congestion off, `BankUpTo` 5,
  `DefaultOptionHalfLife` 8 (asserted, and it is what sets *when* the rule fires), and the bar.
- ⚠️ **`WeeklyXI = false` is a SHAPE-setter here, not a level-setter.** The bench is what `pickXI`
  left out, and at `WeeklyXI` false the eleven is chosen on the five-week horizon — so the fielding
  half of a doubles mechanism is off, while 16 of 36 oracle picks are GW ≥ 33. There is an internal
  asymmetry too: the rule's own reading comes from `oneWeekEngine`, which *does* see the imminent
  double, while the bench it prices was chosen on a five-week view. The paired contrast survives,
  because both arms share the bench; the gain profile does not transport.
- ⚠️ **There are TWO bar-16 rules in this record and they are not the same rule.** `firstClearing`
  (behind `*_threshold_pts`) takes the first week whose **realised** gain clears 16;
  `BenchBoostTrigger` scores a **projection** before the deadline. Levels against no chip **17.889**
  and **9.472**, agreeing in 6 of 36 cells — a gap of 8.417 that is one week of foresight, not a
  verdict on either. And the bar is two independent literals, `chipBarBenchBoost` in
  `chips_test.go` and `config.DefaultBenchBoostBar`, with no reference between them and no equality
  test.
- ⚠️ **The threshold transports to no other chip arm.** The anchoring arm plays a wildcard and a free
  hit, so its arms diverge on the transfer path and its MDE of 34-37 is what that costs — unchanged
  and not refuted here. `ChipCredit`'s bench channel is *preparation*, moves transfer decisions, and
  its +13.3 is on `per_gw × 38` where everything here is `per_path`. The recorded timing levels
  (+8.3, +21.9) use `oracle − firstClearing` and `oracle − median` **summed over two chips**, against
  this arm's fixed entry+6 on one. Three different comparators, none of them this one.
- **No placement week is recommended and none was chosen.** The rule is a fixed policy specified
  before the run; its bar predates the run and was not tuned on these cells. The oracle's 17 distinct
  weeks are a mediator, not an answer.
- **It says nothing about the other three chips.** The free hit replaces the fielded fifteen and the
  wildcard replaces the squad, so neither is path-invariant; the triple captain probably is, and that
  is untested.

## ⚠️ The pre-registration predicted two of these wrong, and the miss is the calibrating fact

It predicted the gate would be **marginal** (it cleared by eightfold) and that `R` would **not
resolve** (t 5.60). It held the same path-invariance argument this finding uses to *explain* the
resolution, so what was miscalibrated is not the mechanism but the **assumed per-cell dispersion of
a one-week chip gain**: the season-clustered SE came in at 0.80 on the ceiling and 1.03 on the rule.
**Size the next chip arm off those, not off the recorded 8-22 levels.**

## The neighbouring bullet's open item, closed as arithmetic

`AGENTS.md` recorded "nothing has been re-measured under the banked schema; a re-sweep is owed" for
the chip-timing levels. `bbceiling.csv` banks all four readings for both scoring chips in 36 of 36
cells under the current schema, so that is now arithmetic off this tree:

| | bench boost | triple captain | sum over the two chips | recorded |
|---|---:|---:|---:|---:|
| timing (`oracle − threshold`) | +1.944 | +4.444 | **+6.39** | +8.3 |
| threshold rule (`oracle − median`) | +15.556 | +14.597 | **+30.15** | +21.9 |

The two move in **opposite directions** from the recorded pair. Different grid and different data
state, so read them beside rather than instead; both remain ≥ 0 in every cell by construction, so
neither has a t or a threshold.
