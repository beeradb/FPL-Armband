# Running notes — the two expected-goals backfill runs

Kept in the repo and committed as each process finishes, because this task has now
killed three subagents on connection errors and the previous attempt survived only
because its cells were committed before its analysis. Cells are written **into the
repo**, never `/tmp`.

Repo state: branch `worktree-agent-abb7d9e9dfa570f3b`, branched from `main` at `4d61058`.
Pre-registration committed at `f6b43c7`, before any cell was computed.

The archive cache is reached through `.cache/fpl`, a symlink to the main checkout's
copy, so no season is re-downloaded. `.cache/` is gitignored, the tree is clean, and
every run below is stamped from a clean tree — which is the provenance caveat the
2026-08-11 snapshot could not discharge.

## Timing, and the scope cut it forced

One arm over 24 cells is **96 s** including the season parse (`TestDiagBaseline`,
`FPL_SWEEP_SEASONS=default`, measured 2026-08-12 on this machine, peak RSS 95 MB
through `scripts/replay`).

⚠️ **That 96 s is the machine's total throughput, not one process's share.** Five
concurrent sweeps returned about 3.1 cells a minute each — 15.5 a minute in aggregate,
which is what a *single* process returns on its own. `scripts/replay` removes the `go`
driver's gigabyte and lets sweeps run side by side without being OOM-killed; it does not
make the machine faster, and on six cores one replay already saturates it. So concurrency
here buys safety, not speed, and a sweep's cost should be budgeted as
**96 s per 24-cell arm, summed over every arm of every process**.

The first Run A launch asked for all seven `TestDiagProjection` blocks in five corners:
35 arms a corner, 175 arms, **4.7 hours**. Cut to two named blocks.

**Which two, and why — stated before the second launch rather than after the numbers.**

- **`MINHL`, the minutes half-life ladder (5 arms).** The most-cited constant table in
  the record, four-season and `HOLD`-metric, so there is a recorded figure to check
  reproduction against. Its mechanism acts in **every** cell, which matters here for the
  reason the next bullet gives.
- **`FIXW`, the fixture weight ladder (4 arms).** Chosen on *mechanism*: `fixture_weight`
  scales the attacking and defensive multipliers, and those multiply exactly the
  quantities the two backfills repair — xG and xGC. If any recorded comparison moves with
  the data state, this is the one that should.

**`DCC` was the thematically obvious pick and is provably useless here, which is itself
worth recording.** `DefConCleanCoupling` acts through the defensive-contribution term,
and defensive contribution scores in **2025-26 alone**; on the four-season grid the two
backfills reach **2022-23 alone**. The two live sets are disjoint, so a `DCC`-by-data-state
interaction is identically zero *by construction*, and running it would have produced a
tight null on a comparison that could not have moved. That is this package's signature
failure and it was avoided by arithmetic rather than by a run.

**Not covered, and this is a scope cut rather than a null:** `MINW`, `BONUS`, `DCC`,
`BENCH`, `MINK`. Each is a recorded comparison and none was run under the four data
states.

## Plan and status

| # | run | grid | arms | cells file | status |
|---|---|---|---|---|---|
| 1 | Run B, xGC repair **on** (ships) | six-season, 36 cells | 1 | `cells/runB-xgc-on.csv` | **done**, exit 0, 36 cells |
| 2 | Run B, xGC repair **off** | six-season, 36 cells | 1 | `cells/runB-xgc-off.csv` | **done**, exit 0, 36 cells |
| 3 | Run A `MINHL`, `shipped` | four-season, 24 cells | 8 | `cells/runA-minhl-shipped.csv` | **done**, exit 0, 192 cells |
| 4 | Run A `MINHL`, `xgcoff` | four-season, 24 cells | 8 | `cells/runA-minhl-xgcoff.csv` | **done**, exit 0, 192 cells |
| 5 | Run A `MINHL`, `xgoff` | four-season, 24 cells | 8 | `cells/runA-minhl-xgoff.csv` | **done**, exit 0, 192 cells |
| 6 | Run A `MINHL`, `bothoff` | four-season, 24 cells | 8 | `cells/runA-minhl-bothoff.csv` | **done**, exit 0, 192 cells |
| 7 | Run A `MINHL`, `aggoff` (`FPL_NO_XG_AGGREGATE=1`) | four-season, 24 cells | 8 | `cells/runA-minhl-aggoff.csv` | **done**, exit 0, 192 cells |
| 8-12 | Run A `FIXW`, the same five corners | four-season, 24 cells | 4 each | `cells/runA-fixw-*.csv` | **done**, exit 0, 96 cells each |

All twelve processes exited 0 with the cell count their provenance sidecar declared. Total
replay cost: 2 x 36 + 5 x 192 + 5 x 96 = **1,512 cells**, about 100 minutes of wall clock.

`fpl-stats-review` was run over the Run B write-up before anything was recorded as established,
and eleven of its findings are applied in `FINDINGS.md`. Three changed a claim rather than
softening it — see the ⚠️ marks there.

Each `MINHL` process is `TestDiagBaseline` (1 arm) + `TestDiagProjection` at `EXP=MINHL`
(5) + `TestDiagViceCaptainFix` (2) = 8 arms. Each `FIXW` process is
`TestDiagProjection` at `EXP=FIXW` alone (4 arms), since the baseline and the vice
comparison are already carried by the `MINHL` process at the same corner.

The `aggoff` corner is the `FPL_NO_XG_AGGREGATE` arm the brief names as the cheapest and
best-specified piece of work here. It is not optional and it is not last in priority; it
is simply the fifth process of a batch that runs together.

## Log
