# Pre-registration — the two expected-goals backfills, 2026-08-13

Written and **committed before any cell of either run was computed**, at `4d61058`, clean tree.
Everything below is a prediction. What the runs actually returned is in `FINDINGS.md`; where a
prediction failed, it is marked as failed there rather than quietly rewritten here.

The reason for the ceremony is the record's own standing rule: this project has repeatedly
explained a movement after seeing it, and four mechanism claims have been wrong. So the
mechanism claims below are labelled as **hypotheses read off the source**, and each carries the
observation that would falsify it.

---

## Run A — the 2x2 on `FPL_SWEEP_SEASONS=default` (the historical four, 24 cells)

Four corners, four separate processes, because none of these switches can be a sweep arm
(`runPolicySweep` calls `loadPairs` once into a process-global season cache, so an arm setting
one of them replays the same already-parsed season in both arms and reports a tight null on
exactly the thing it was built to measure).

| corner | `FPL_NO_XG_REPAIR` | `FPL_NO_XGC_REPAIR` |
|---|---|---|
| `shipped` | unset | unset |
| `xgcoff` | unset | 1 |
| `xgoff` | 1 | unset |
| `bothoff` | 1 | 1 |

Each corner runs the same arms: `TestDiagBaseline` (1 arm), `TestDiagProjection` (all seven
blocks — `MINHL` `MINW` `BONUS` `DCC` `BENCH` `FIXW` `MINK`, 32 arms) and
`TestDiagViceCaptainFix` (2 arms). That is 35 arms per corner and it is deliberately **not one
cherry-picked block**: every one of those blocks has a figure recorded in the research record,
so each is a reproduction check with something to check against.

### P1 — the 2x2 is structurally degenerate, and `xgoff` will be byte-identical to `bothoff`

**Hypothesis, read off `internal/backtest/xgrepair.go:283-287` and `:366-369`.**
`applyXGRepair` returns early when `noXGRepair()` is true, and the call to `applyXGCRepair` sits
*inside* `applyXGRepair`, after that early return. So `FPL_NO_XG_REPAIR=1` should disable the xGC
reconstruction as well, and the corner "xG off, xGC on" is not reachable through these two
environment variables at all.

**Prediction:** the `xgoff` and `bothoff` cells files are **identical in every cell and every
outcome column**, on both metrics, in all 24 cells.

**If that holds**, the interaction term the 2x2 was commissioned to estimate is zero *by
construction rather than by measurement*, and must be reported that way — not as a null.
What remains estimable is a decomposition into the xGC half (`shipped − xgcoff`) and the
xG-half-plus-any-interaction residual (`xgcoff − bothoff`), which cannot be separated further
without a code change.

**Falsified by:** any cell differing between `xgoff` and `bothoff`.

### P2 — the affected set on this grid is 6 of 24 cells, all of them 2022-23

The four-season grid plays 2022-23, 2023-24, 2024-25, 2025-26 with priors 2021-22, 2022-23,
2023-24, 2024-25.

- The **weekly** channel: only 2022-23's GW1-15 rows are repaired among the played seasons.
- The **prior** channel: `newPriorIndexMulti`'s `stat` closure reads season *aggregates*
  (`q.XG`, `q.XA`, `q.XGC`), and `rebuildXGAggregates` / `rebuildXGCAggregates` fire only where
  `xgRepairs[season].NoAggregate` is true — 2018-19, 2019-20, 2020-21, 2021-22, and **not**
  2022-23. So the prior for 2022-23 (which is 2021-22) moves and the prior for 2023-24 (which is
  2022-23) does not.
- `RateHalfLife` and `PriorRateHalfLife` both ship at 0, so the weekly-row prior path
  (`newPriorIndexRecent`) is not taken and cannot add cells.

**Prediction:** every corner differs from `shipped` in exactly the six 2022-23 cells and is
byte-identical in the eighteen cells of 2023-24, 2024-25 and 2025-26. This reproduces the
6-of-24 count in `stats/snapshots/2026-08-11-0104d9d/`, which is a pipeline check rather than a
discovery.

**Falsified by:** any 2023-24 / 2024-25 / 2025-26 cell moving.

### P3 — the recorded vice-captain figure will have moved again

`internal/backtest/xgcrepair.go` was first added at `7cb769e` on 2026-08-12, the day *after*
the `0104d9d` snapshot. That snapshot measured the vice-captain fallback at **+0.4102** pts/gw
on `HOLD` with the xG repair on and **+0.4210** with it off, on the four-season grid.

**Prediction:** today's `shipped` corner does **not** return +0.4102, because the season parse it
was measured against no longer exists. **The direction is not predicted** and will not be
claimed as confirmation whichever way it goes.

### P4 — no recorded figure resolves against its data-state counterpart

**Prediction:** none of the 35 arms' paired figures differs between corners by more than this
harness can see. The affected set is 6 of 24 cells, so an 18-cell exact-zero block dominates
every mean and season-clustered inference is degenerate (three of four season means identically
zero). Anything reported here is a **bound**, not a measurement, and the write-up must say so.

---

## Run B — `FPL_NO_XGC_REPAIR` on the six-season grid (`FPL_SWEEP_SEASONS` unset, 36 cells)

Two processes, `TestDiagBaseline` — one arm, the shipped settings, nothing varied — so the paired
difference *is* what the repair is worth in points. The two cells files are paired by
(season, start_gw) afterwards.

### P5 — the confinement is 18 of 36 cells

2020-21 and 2021-22 in full (12 cells) plus all six 2022-23 entries, since `PointInTime`
accumulates the repaired GW1-15 rows from GW1. The prior channel adds nothing beyond that, by
the same argument as P2. The eighteen cells of 2023-24, 2024-25 and 2025-26 must be
**byte-identical**.

**Falsified by:** any count other than 18, in either direction. A different count is a finding
and is to be reported as one — the record already carries this number twice, as 12 (stale, in
`extendedPairNames`' comment) and as 18 (in `xgcrepair.go`'s header and in `CLAUDE.md`).

### P6 — composition: defenders and keepers become relatively more attractive

With the repair off, `baseXP90` gates both the clean sheet and the goals-conceded deduction on
`XGC90 > 0`, so every defender and keeper in the blind seasons is scored with **neither** term,
and the clean sheet alone is 26-45% of their score. Restoring it should change the opening
fifteen in most affected cells.

**The DEF+GKP head count cannot move** — FPL forces two keepers and five defenders — so this is a
claim about *which* defenders and keepers and about money, not about how many. The cells CSV does
not carry squad composition, so on this grid the mediator available is `moves` and `hits` on
`POLICY`; the composition mediator exists only at one entry point, in `TestDiagXGCPoints`.

**Prediction:** the opening fifteen changes in a majority of the 18 affected cells, evidenced by
`HOLD` moving in a majority of them.

### P7 — the sign of the POINTS effect is explicitly NOT predicted

This is a declaration, not a hedge. "A better predictor makes a worse policy" is the most
repeated finding in this record — five instances — so a positive result and a negative result are
both consistent with the mechanism, and neither may be written up as confirming it. What is
predicted is P6 (the objective changes) and P5 (where it can change). Whatever sign the points
come out at, it will be reported as a measurement of the repair's cost or benefit and **not** as
evidence for or against the mechanism.

### P8 — expect it not to resolve on `POLICY`, and to be the primary reading on `HOLD`

The repair is a scoring change, so `HOLD` is primary. On the 18 affected cells the
season-clustered standard error rests on **three** seasons — 2020-21, 2021-22, 2022-23 — which is
**df 2** and a two-sided 5% critical value of **4.303**. That is a worse inferential position
than anything in the canonical resolution block, and it is a property of where the repair can
act rather than of the grid.

---

## What is not covered

- `FPL_NO_XG_AGGREGATE` is a fifth process and is lower priority than closing the 2x2. If it is
  run it will be recorded here as an addendum with its own timestamp; if it is not, `FINDINGS.md`
  says so explicitly.
- Note the switch is **misnamed for what it now does**: `xgcrepair.go:437` gates the xGC
  aggregate rebuild on `spec.NoAggregate && !noXGAggregate()`, so `FPL_NO_XG_AGGREGATE` disables
  the *expected-goals-conceded* aggregate rebuild as well as the expected-goals one. That is a
  reading of the source, to be verified rather than asserted.
