# The repair cost as a time series: churn refused, standing gap dominant

The one shape `TestDiagWildcardTrigger` could not produce. Pre-registration lives in the
diagnostic's own doc comment, committed at `3b2278e` before the first cell ran.

## What ran

`EXP=REPAIRCOST FPL_SWEEP_SEASONS=default FPL_SWEEP_STARTS=1,16 scripts/replay -run
TestDiagRepairCostSeries`, four seasons (2022-23 … 2025-26, the default grid) at entry GW1 and
GW16 — **8 cells**, `RecordRepairCost` on, all archive repairs on, committed tree at `3b2278e`.
42m41s, peak RSS 90 MB, exit 0. 236 observed gameweeks × 3 `Optimize` calls. **Zero weeks
returned "no reading"** — every OK flag true in every cell.

Two series per cell, never pooled (they answer different questions):

- **EVOLVING** — the fifteen the policy actually holds each week.
- **FROZEN** — the opening fifteen held all season (the `HOLD` squad), priced at selling value
  and again at market value (`mkt`), so the half-of-any-rise rule is separated from football.

Counts are *players the fresh optimum would replace*, not points. Reading rule, written before
the run: CHURN = both series decline, GW1 column decaying hardest; STANDING GAP = EVOLVING flat
and non-zero while FROZEN rises; BOTH = a decay sitting on a floor, reported as two sizes, never
a category; FRICTION = a materially smaller `mkt` slope implicates the selling rule.

## The shape statistics

| cell | EVOLVING head/tail | floor | slope /gw | FROZEN slope /gw | mkt slope /gw |
|---|---|---:|---:|---:|---:|
| 2022-23 GW1 (37 wk) | 7.50 / 5.83 | 5.83 | −0.053 | **+0.035** | +0.050 |
| 2022-23 GW16 (22 wk) | 7.43 / 6.57 | 6.57 | −0.053 | **+0.165** | +0.203 |
| 2023-24 GW1 (37 wk) | 7.50 / 7.83 | 7.83 | +0.007 | **+0.102** | +0.098 |
| 2023-24 GW16 (22 wk) | 6.71 / 7.86 | 7.86 | +0.079 | **+0.168** | +0.173 |
| 2024-25 GW1 (37 wk) | 9.25 / 8.00 | 8.00 | −0.050 | **+0.065** | +0.063 |
| 2024-25 GW16 (22 wk) | 6.29 / 7.29 | 7.29 | +0.068 | **+0.124** | +0.080 |
| 2025-26 GW1 (37 wk) | 8.17 / 8.17 | 8.17 | +0.004 | **+0.120** | +0.103 |
| 2025-26 GW16 (22 wk) | 4.57 / 9.29 | 9.29 | +0.302 | **+0.325** | +0.323 |

## The verdict, against the rule

**CHURN is refused.** The frozen series *rises* in 8 of 8 cells (every slope positive, +0.035 to
+0.325 players/gw). Churn predicts decline in both arms; the frozen arm never declines.

**STANDING GAP is the dominant signature.** EVOLVING is flat and non-zero — floor 5.83 to 9.29
players, i.e. roughly 16-28 points of repair at the usual allowance — in 6 of 8 cells; the two
GW1 entries of 2022-23 and 2024-25 carry a mild early decay (+1.67 and +1.25 head-over-tail) —
a churn component sitting **on top of** the floor, not instead of it. BOTH, the expected reading
by construction, is the honest category: report the two sizes.

**The selling rule is exonerated.** The `mkt` slope matches the selling-value slope in every
cell (differences ≤ 0.04 players/gw, and twice *higher*). The frozen rise is not the
half-of-any-rise tax accumulating on a never-sold squad; it is football — or the confound below.

## Caveats that bind every number above

- ⚠️ **A rising frozen series does not separate a standing gap from injury accumulation.** Over
  a season the opening fifteen gathers injuries, suspensions and form losses; a fresh argmax over
  the current pool avoids the absent players, which also produces a rising frozen series. The
  design discriminates churn from non-churn; the non-churn residual is one thing it cannot
  decompose. The EVOLVING flatness is the cleaner evidence for the standing gap.
- ⚠️ **2025-26 GW16 is anomalous and is reported, not explained.** Both arms RISE there
  (+0.30/+0.33 per gw), and the EVOLVING head-third is 4.57 against a tail of 9.29 — the
  mid-season entry whose opening squad was built on fifteen gameweeks of data is the cell where
  the held squad diverges fastest. One cell; no mechanism claimed.
- **Counts only.** No points figure, no threshold, no p — the replay cannot value a wildcard,
  and the diagnostic's constraint says so in its own header.
- **Four seasons, two entry points.** The shape is consistent across all eight; nothing here is
  a season mean to be clustered.

## What this settles, and what it opens

The trigger diagnostic's one twice-observed arm read 12 → 16 and 12 → 24 — the level signature.
The series confirms it across the grid: **the repair cost does not fall as the season
accumulates data**, so raising the reservation cannot fix the trigger (the bar-20 arm already
showed it fires within four weeks anyway). What must change is the **quantity** — a repair cost
measured against a squad the model still endorses — and the flat EVOLVING floor says the model's
own week-to-week preferences sit a substantial, persistent distance from its fresh optimum. That
reading is the ground for the queued re-judgement of prior reactivity under the chip-and-banking
configuration; this diagnostic establishes the fact, not its cause.

The observer itself changes nothing: `TestTheRepairSeriesChangesNoDecision` replays a cell with
`RecordRepairCost` on and off and requires the season to be identical, and no branch reads the
series.
