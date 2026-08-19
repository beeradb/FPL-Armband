# Where a fixed-offset chip plan actually lands, and the two-set repair it exposed

**Run 2026-08-17**, `TestDiagBlanksAndDoublesCensus`, on the **four-season grid at six entry
points — 24 cells**, with `FPL_SWEEP_SEASONS=default`. No `Simulate` call and no cells file: every
figure here is arithmetic over `Season.Fixtures` through `teamGameweeks`, so nothing in it is a
points claim and no detection threshold applies.

It exists because a number that was **asserted rather than measured** had become load-bearing for a
design. A Go comment justifying "chip placement on, banking off" said the fixed-offset control
lands a chip on an ordinary week *"about 78% of the time"*. Nobody had run it. The measured figure
is below, it is not 78%, and it varies by chip by a factor this record would call a different
finding.

## What the placement rules put a chip on

`anchoredPlan` is `sightedWeeks` at full sight, masked by `matchedChips`; `controlPlan` plays the
same chips at fixed offsets from entry. "On the feature" means the round the chip landed in carries
at least one doubling club (bench boost, triple captain) or at least one blanking club (free hit).

| arm | placed | on the feature | on an ORDINARY week | burned on an unplayed round |
|---|---:|---:|---:|---:|
| anchored bench boost | 24/24 | 24/24 | 0% | 0 |
| anchored free hit | 24/24 | 24/24 | 0% | 0 |
| anchored triple captain | 12/24 | 12/12 | 0% | 0 |
| control bench boost | 24/24 | 4/24 | **83%** | 1 |
| control free hit | 24/24 | 2/24 | **92%** | 0 |
| control triple captain | 12/24 | 4/12 | **67%** | 0 |

**Pooled over the three chips the control places: 10 of 60 on the feature, so 83% land on an
ordinary week.** The corrected figure for the comment that said 78%.

⚠️ **The anchored column is 100% BY CONSTRUCTION and carries no information.** `minAnchorClubs` is a
bar `sightedWeeks` applies before it will spend a chip on a week, so an anchored chip cannot land
anywhere else. The informative column is the control's, and it is the only one.

⚠️ **"On the feature" is a weak proxy for "well placed".** It asks whether the round carries any
doubling or blanking club at all, not whether the squad owned one, not how many, and not what the
chip returned. A control chip that lands on a week where one club doubles counts as on the feature
and is a poor bench boost. So 83% is a **floor on how often the control is badly placed**, not an
estimate of the gap between good and bad placement.

⚠️ **The triple captain is placed in only 12 of 24 cells**, by both arms — `matchedChips` takes the
intersection of what every arm can place, and the third chip does not fit in every cell. Its
denominator is 12 and pooling it against 24 would understate it.

⚠️ **One control chip lands on a round carrying no fixture at all** (2022-23's GW7, postponed after
the Queen's death and redistributed). `controlWeeks` has no `played` gate. That is a confound in the
control arm rather than a defect in the census, and the `unplayed` column exists so it is visible.

## The repair this run confirms

`ValidateChipSets` would refuse **0 of 24** anchored plans and **0 of 24** control plans.

Before this branch that column read **6 of 24 for the anchored arm** — every 2025-26 cell.
`ChipSetsFor("2025-26")` is 2, a first-set chip at or after `ChipResetGW` is refused, and 2025-26's
only anchors are GW33 and GW34; `runPolicySweep` records a refusal as an **infeasible cell rather
than fatalling**, so an anchored-chips arm silently lost a whole season while every printed number
stayed plausible. `backtest.SplitChipSets` now routes each chip into the set its week draws from,
and `Simulate` applies it to every `ChipPlanner`.

⚠️ **No anchored-chip cells are banked anywhere in `stats/snapshots/*/cells/`.** Every figure taken
from an anchored-chips arm before this fix was computed on a grid missing its 2025-26 column, and
it can only be **re-measured, never re-derived**.

## The two-set claim, recomputed

`AGENTS.md` rests `ChipResetGW` on "the first half of the season holds 15 of 189 doubling
club-gameweeks". On this four-season grid the same count is **4 in GW1-19 against 85 over the whole
grid** — 2022-23 two, 2023-24 two, 2024-25 none, 2025-26 none. The recorded 15/189 is the
**six-season** figure and includes the COVID-rescheduled 2020-21 round that carries 11 of its 15,
which is not in this grid. Both are correct of what they count; quote the grid with either.

## What this does not establish

- **No points figure, and none is derivable.** Nothing was replayed.
- **It does not show anchored placement is worth anything.** The recorded verdict is that anchoring
  the chips on the calendar is a clean null with an MDE of 34-37 per season-path. This says the
  control is badly placed, which is a fact about the *comparison's contrast*, not about the payoff.
- **83% is a four-season figure at six entry points.** Cells within a season share nearly all their
  calendar, so the effective count is far below 24 and no standard error is offered.
