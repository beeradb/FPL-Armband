# Pre-registration — the six-season aggregate arm and the four uncovered blocks

Written and **committed before any cell of either task was computed**. Second pre-registration on
this branch; the first is `PREREGISTRATION.md` in the same directory and covered the runs written
up in `FINDINGS.md`.

**This task begins by reading two reviews that did report, to the coordinating session rather than
to me** — captured at `reviews/2026-08-13-backfill-runs/review.md` on
`worktree-prior-half-life-on-repaired-xgc`. Their ~30 findings are **not** being applied here, but
nothing refuted there may be restated, and three of them shape what follows:

- **A3 withdraws "the weekly fill carries essentially all of the effect."** The aggregate term's
  own standard error on the four-season grid is **1.481 pts/gw ≈ 56 points a season, |t| = 0.07** —
  it never distinguished "worth nothing" from "worth the whole +82.7" — and its six live cells are
  a **cancellation** (+3.89 / −1.70 / +0.25 / −5.78 / +3.67 / −0.92), not a null. Task A is the
  measurement that would replace it.
- **A7 refutes the throughput figure that truncated the last run.** "One 24-cell arm per 96 s in
  aggregate" appears nowhere in my own logs: `MINHL` ran 8 arms in 16:08-16:54 *per process* with
  five concurrent, and Run B at two concurrent ran 5.6 s/cell against Run A's 5.3 s/cell at five.
  Per-process rate did not degrade; concurrency scaled. Task B is affordable and was cut on a
  wrong number.
- **A4 refutes "the ordering is stable in all four corners."** 18 of 24 cells were identical across
  corners, so a stable ordering counted once per corner is one observation counted four times, and
  restricted to the six responding cells the ordering **inverts**.

---

## Task A — the aggregate half, on the six-season grid

**Two new processes only.** `TestDiagBaseline`, `FPL_SWEEP_SEASONS` unset (six seasons, 36 cells),
one arm each, ~3.5 minutes apiece from Run B's own logs. Two of the four corners already exist and
are **reused, not re-run**: `cells/runB-xgc-on.csv` *is* six-season `shipped` and
`cells/runB-xgc-off.csv` *is* six-season `xgcoff`, both verified byte-identical to Run A on their
four-season subsets.

### The estimands, stated against the nesting rather than against the switch names

The two switches are not orthogonal and neither is named for what it does. Writing the corners as
data states rather than as flags:

| corner | expected goals weekly | xGC weekly | season aggregates |
|---|---|---|---|
| `shipped` (existing) | filled | reconstructed | rebuilt |
| `xgcoff` (existing) | filled | **absent** | rebuilt (xG side only — there is no xGC to rebuild) |
| **`norepair`** (`FPL_NO_XG_REPAIR=1`) | **absent** | **absent** | **not rebuilt** |
| **`aggoff`** (`FPL_NO_XG_AGGREGATE=1`) | filled | reconstructed | **not rebuilt, both quantities** |

So the contrasts available are:

| contrast | what it is a contrast OF |
|---|---|
| `shipped` − `xgcoff` | the xGC reconstruction, **given expected goals repaired** |
| `xgcoff` − `norepair` | the expected-goals backfill **entire (weekly and aggregate), given xGC absent** |
| `shipped` − `norepair` | **both backfills entire** |
| `shipped` − `aggoff` | the **season-aggregate half of both backfills together** |

⚠️ **There is no clean "xG/xA alone" term and these switches cannot produce one.**
`FPL_NO_XG_REPAIR=1` is *neither repair*, because `applyXGCRepair` sits inside `applyXGRepair`
after its early return; `FPL_NO_XG_AGGREGATE=1` governs the xGC season aggregate as well as the
expected-goals one. Any reader inferring an xG/xA-alone figure from these four corners is inferring
something not measured. Labelling the third corner "xG off" would invite exactly that, so it is
labelled `norepair` throughout.

### P9 — 18 live cells, for every contrast

Played seasons on this grid are 2020-21 through 2025-26; the expected-goals repair table covers
2018-19 through 2022-23. Weekly channel: 2020-21, 2021-22, 2022-23. Prior channel: those three
seasons' priors are 2019-20, 2020-21 and 2021-22, all of which are `NoAggregate` and so all of
which lose their rebuilt totals under `aggoff`. Both channels land on the same three played
seasons, so **18 of 36 for every contrast, and 18 byte-identical**.

**Falsified by** any count other than 18, in either direction, for any contrast.

### P10 — the aggregate arm moves every one of the 18, and I do not predict its sign

Under `aggoff` the prior for each of the three affected played seasons loses its expected-goals,
expected-assists and expected-goals-conceded season totals **entirely** — the archive publishes no
such column for those seasons, so `rebuildXGAggregates` not running leaves them at zero rather than
at a slightly different value. That is a large change to every player's prior, so **all 18 cells
should move**.

⚠️ **The sign is explicitly not predicted, and the reason is a correction rather than caution.**
The four-season reading of −3.7 points a season had |t| = 0.07 against its own standard error of
about 56 a season. That is not a small effect; it is **no information about the sign at all**, and
treating it as a prior would be reading a number that was already withdrawn. What I will report is
the point estimate with its standard error and the count of cells each way, and if it fails to
resolve — which on df 2 and a 4.303 critical value is the likely outcome — that is what will be
written.

### P11 — this supersedes the four-season decomposition rather than adding to it

The Run A level table is one played season with no season axis. If Task A returns 18 live cells
across three seasons, the four-season figures (−53.8 / +82.7 / −3.7 / +136.5) become a subset
reading and must not be quoted beside the new ones as though they were independent.

---

## Task B — `MINW`, `BONUS`, `BENCH`, `MINK` across the three distinct data states

19 arms — `MINW` 5, `BONUS` 5, `BENCH` 4, `MINK` 5 — on `FPL_SWEEP_SEASONS=default`, under
`shipped`, `xgcoff` and `norepair`. **Three data states, not five**: `bothoff` *is* `norepair` by
construction and `aggoff` is a different question already answered by Task A, so a fourth and fifth
corner would buy process determinism rather than evidence. 57 arms, twelve processes.

`DCC` stays excluded and the exclusion is **closed on mechanism, not deferred**: defensive
contribution scores in 2025-26 alone, the backfills reach 2022-23 alone on this grid, so a
`DCC`-by-data-state interaction is identically zero. It will be recorded as closed rather than
listed beside genuinely uncovered blocks.

### P12 — 6 live cells of 24, all 2022-23, for every block and every contrast

The same mechanism as Run A: on the four-season grid only 2022-23 is a played season the backfills
reach, and only 2021-22 is a prior whose aggregates are rebuilt. **Any ladder difference between
data states can only come from those six cells.**

**The consequence, pre-registered so it cannot be forgotten later:** an ordering counted once per
data state is one observation counted three times, because 18 of the 24 cells are identical across
states by construction. **Every ordering claim will be computed on the six responding cells and
will state how many cells it rests on.**

### P13 — nothing resolves, in any block, in any data state

These constants are worth 11 to 34 points a season by this record's own accounting, against
per-comparison thresholds whose median is 39 and which run to 232. **Unresolved is the expected
reading for a real effect here and is not evidence against one.** A standard error will be quoted
beside every ladder entry, because on the previous run all 28 entries had |t| < 2.2 and 25 of 28
below 1.6 — a level "moving by up to 0.53 pts/gw" was moving by less than one standard error of
itself.

### P14 — the orderings will NOT be stable across data states on the responding cells

Review finding A4 established that on the six responding cells `MINHL`'s best arm inverts between
`shipped` and `xgoff`, and `FIXW`'s best alternative changes in two of four corners. I expect the
same instability here, and I am predicting it in advance so that finding it cannot be presented as
a discovery. **If an ordering does hold on the six responding cells across all three states, that
is the surprise and is worth more than any gap.**
