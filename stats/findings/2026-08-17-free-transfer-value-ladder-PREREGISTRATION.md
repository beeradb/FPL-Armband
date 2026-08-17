# The flat `free_transfer_value` ladder: pre-registration

**Written and committed before the run.**
`EXP=FREEVALUE FPL_SWEEP_SEASONS=extended scripts/replay -run TestDiagFreeTransferValue`,
5 arms × 36 cells on the six-season grid (2020-21 … 2025-26), entry gameweeks 1/6/11/16/21/26.

## Why this run exists

`free_transfer_value` — the points a free transfer is charged in
`SimConfig.FreeCost`, read by `decide` at `simulate.go` — **ships at 2.0 and has never been
varied in any banked sweep.** Every `stats/snapshots/*/cells/*.provenance.csv` stamps it at 2 as
an ambient constant, so the level is an untested prior question, not a re-tune.

The immediate motive is attribution. The newly merged option-value taper
(`SimConfig.TaperFreeTransferValue`) multiplies this same constant by a decay-and-congestion
factor, so it moves the **mean** charge as well as the charge's **shape**. Without a flat ladder,
a taper result cannot be separated from a level nobody has established. This run produces the
level ladder; it does not test the taper, and the taper is off in every arm here.

There is a second reason. The one existing figure for this constant — charging the full
four-point hit value dropped transfers from 73 to 39 and scored *below* charging nothing — comes
from single-path GW1 replays that predate cell banking, paired differences, the doubles fix, the
selling-price fix and the zero-penalty fix. It is a **direction with no threshold**, and the 4.0
arm below is close to that regime.

## The arms

`free_transfer_value` at **2.0 (ships, baseline)**, then **1.0, 1.5, 3.0, 4.0**. The shipped value
is `variants[0]` because every paired difference is taken against it.

Nothing else moves. `WeeklyXI` stays at the `runPolicySweep` default of `false`, which is what the
sibling `min_gain` ladder ran at; the taper, both chip-preparation switches, banking lookahead and
every oracle are off. `BankUpTo` is pinned at `sweepBankLimit` = 5 for every cell, as every sweep
in this package is.

## Metric

**`POLICY`.** This constant is *about* transfers, which is the only case this record admits for
the noisy metric. `policy_xpoints` is reported **beside** it and never instead of it.

`HOLD` must be **byte-identical across every arm**. `FreeCost` is read only inside the weekly
transfer decision and `HOLD` makes no transfers, so a moved `HOLD` means the constant is leaking
into scoring and the experiment is measuring two things. That is a **confinement**, not a result,
and the liveness check below is its mandatory pair.

## Estimand, stated before any cell is read

The **paired per-cell difference against the shipped 2.0**, on `per_gw`, converted with
**`per_gw × 38`**. Not a pooled total divided by 36: the six entry points give cells of
38/33/28/23/18/13 gameweeks, mean 25.5, so dividing by the cell count understates by about a
third.

## Threshold

Computed **from this comparison**, as `t_crit(df) × SE × 38`, with the **df taken from the
contrast** rather than assumed to be 5. Both estimators are reported **side by side as a range** —
season-clustered (CR2) and start-fixed — with no end picked. Clustering is not always conservative
here: where the start-point main effect is large it makes the standard error *smaller*.

A **wild cluster bootstrap p** (Webb 6-point weights, enumerated) is quoted beside every clustered
t, with `S_eff` (the number of seasons that can actually contribute) and the floor `6/6^S_eff`.
**An arm whose floor exceeds 0.05 is unmeasurable, not null.** **Holm** over the four non-shipped
arms.

## The primary result is SHAPE, not a winner

Picking the highest of five noisy estimates is this record's most load-bearing warning, and it is
what would be worth the least here. **No value will be recommended on the strength of scoring
highest**, and no new default is proposed by this run whatever it returns. What is readable:

- **Monotone across all five rungs** would be a genuine shape, and it is not forced by the
  construction — nothing here removes a line item, so the "monotonicity the construction forces"
  exclusion does not apply.
- **A plateau with a cliff** — flat over some interval and falling outside it — would locate the
  interval the constant is insensitive within, which is the more likely readable outcome and the
  more useful one.
- **2.0 in a trough** (both neighbours worse) against **2.0 on a slope** is the two-sided question
  this ladder exists to ask, and the reason the arms bracket the shipped value on both sides.

## Predictions, stated before any cell is read

1. **The 4.0 arm is predicted negative**, and predicted to cut moves sharply. That is the recorded
   direction from the old GW1 replays. It is a **direction only**: the recorded figure carries no
   threshold, so reproducing the sign is corroboration and failing to reproduce it is not a
   refutation of a magnitude that was never established.
2. **Moves fall monotonically as the charge rises.** This is close to forced by the gate arithmetic
   — a higher bar accepts a subset — and is therefore the **liveness** check rather than a result.
   See below.
3. **No arm is expected to resolve on points.** This grid's own `POLICY` median threshold is about
   70 points a season and the sibling `min_gain` ladder's arms carried thresholds of 21.7 to 34
   with nothing clearing. A null is the **expected** outcome and is not evidence that the constant
   does nothing.
4. **2.0 ships unchanged**, which follows from 3 and is stated here so the run cannot be read as a
   licence to move it on a point estimate.

## Liveness — the check that says the constant arrived

`moves` and `hits` **must differ across the ladder**. They are integer counts observed without
noise, and they are the sharpest arrival evidence this sweep has. **If they do not differ, the
constant did not reach the decision and the deliverable is that fact** — a byte-identical result
is a comparison that never ran, not a tie, and would be written up as such rather than as a null.

Round-trips (a player sold and later bought back within one replay) are counted per arm as a
second, independent reading, because the recorded claim being checked — *"a volume brake, not an
anti-churn device"* — is about the **proportion** of moves that are round-trips, which no points
column can see. Counts only; no threshold is claimed for them.

## What would change a verdict

Only a resolving arm — |t| above `t_crit` at the comparison's own df, surviving Holm over the four
contrasts, with a wild bootstrap floor below 0.05 — or a monotone ladder across all five rungs with
a consistent sign across seasons. A **leave-one-season-out** check is part of the reading rather
than an optional extra: this record has twice had a ladder whose sign reversed on dropping two
seasons.

**A result below the detection threshold of its own comparison is not a result**, and will be
reported plainly as such.

## Data state

Six-season extended grid, pinned explicitly with `FPL_SWEEP_SEASONS=extended` rather than taken
from the default — on the `scoring` grid `runPolicySweep` prints
`--- POLICY: NOT REPORTED ---` and emits no paired differences on the primary metric, which would
produce a complete-looking cells file with the result silently absent.

All repairs on: no `FPL_NO_XG_REPAIR`, no `FPL_NO_XGC_REPAIR`, no `FPL_NO_XG_AGGREGATE`, no
`FPL_NO_STARTS_REPAIR`. `FPL_MAGNITUDE` **unset**. Named here because a recorded level means
nothing without it.

⚠️ Run on **arm64**. `policy_xpoints` and `hold_xpoints` are banked at full float64 and this
project records that Go's transcendentals are not bit-identical across architectures, so those
columns are not expected to reproduce byte-for-byte on amd64. The points columns are integers and
`squad_hash` is a digest, so both reproduce unless a decision flips.
