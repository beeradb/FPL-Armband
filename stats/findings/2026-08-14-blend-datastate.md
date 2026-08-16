# The xG/xGC backfills do not explain the `BlendRateK` reversal — and they reach only one season of four

⚠️ **Corrected 2026-08-14 after review. Three claims below were over-stated and are marked in place; the run itself is sound.**

**Run 2026-08-14** at `19f3050`,
`EXP=BLEND FPL_SWEEP_SEASONS=default FPL_NO_XG_REPAIR=1 scripts/replay -run TestDiagRejudge`.
4 arms × 24 cells on the four-season grid, **96 cells**, 7m11s, peak RSS 94 MB. Cells in
`cells/blend-old.csv`; both switches are recorded in its provenance sidecar. Predictions in
`2026-08-14-blend-datastate-PREREGISTRATION.md`, committed before the run.

## The answer

| state | k=12 | k=16 | k=24 |
|---|---:|---:|---:|
| **recorded**, pre-2026-08-10, 24 cells | −0.632 | −0.740 | **−1.509** |
| **current** data state, same four seasons | −0.445 | +0.302 | +0.124 |
| **old** data state — repairs off — *this run* | **−0.326** | **−0.052** | **+0.351** |

`HOLD` pts/gw, paired against `BlendRateK=8`. CR2 t on this run: −0.46, −0.08, +0.59 on 3 df.

⚠️ **"Does not reproduce" is withdrawn.** Recovering the recorded SEs from the recorded `t` (0.290 / 0.471 / 0.665), the gaps are t **0.44 / 0.82 / 1.84** — only `k=24` is arguably discrepant, and it is *inside* this run's CR2 interval. What is true: At `k=24` the record says −1.509 and
the old data state says **+0.351** — opposite in sign, and 1.86 pts/gw apart, which is **71 points a
season**. The old data state sits much closer to today's than to the record's.

**The pre-registered prior was correct.** It was registered that this would fail to reproduce,
because the backfills move only 2022-23 cells among these four seasons and dropping 2022-23 from the
banked run already left `k=16` and `k=24` positive. Registering it in advance is what makes this a
test rather than a rationalisation.

⚠️ **The regime comparison was a category error and is withdrawn.** The recorded triple
+0.936 / −0.611 / −1.783 is the **`POLICY` `k=12`** row, not `HOLD` — its three equal-sized phases
average to −0.486, which is that row's mean exactly. On the metric it was actually claimed on,
"helps early, hurts middle" **reproduces** here (+0.672 / −0.812 / +0.105); only the late leg fails.

## Why the recipe cannot reach it, which is the transferable finding

CLAUDE.md carries: *"Reproducing anything recorded before 2026-08-10 needs two switches:
`FPL_SWEEP_SEASONS=default` **and** `FPL_NO_XG_REPAIR=1`, which disables the xGC reconstruction
too."* Both were set. It still does not reproduce, and the reason is structural:

⚠️ **`internal/backtest/season.go` — the archive loader — contains ZERO `os.Getenv` calls.**
Everything it does at load is unswitchable. And it changed **nine times** between the recorded
table's commit and today, including the archive *defect* repairs at `7cb769e` and `8e0c5ce`
(2026-08-12): the **59 phantom matches in 2019-20 and 10 duplicate rows in 2025-26** that CLAUDE.md
records as "dropped at load, counted, and pinned to those exact numbers".

**2025-26 is one of the four seasons in the recorded table.** So that table was measured with ten
duplicate rows still in it, and **there is no switch that puts them back.**

> **The reproduction recipe governs the xG/xGC backfills. It does not govern the archive defect
> repairs, which are ungated. So a pre-2026-08-12 figure is not *guaranteed* reproducible by
> switches alone**, and the recipe should say so.

⚠️ **Not "is not reproducible" — the absolute form is refuted by CLAUDE.md itself**, which records
the 08-11 vice-captain snapshot reproducing *bit-exactly* across those repairs. The gap is real,
named and small: in 2025-26 the drops are 10 rows. It is a reachability caveat, not a licence to
attribute any failed reproduction to code.

## What is now known, and what is not

**Known.** Not grid width, and not the xG/xGC backfills — which on this grid reach **24 of 96 cells,
all 2022-23**, so three seasons could not run the intervention at all. ⚠️ Only `k=24` is arguably
discrepant once the recorded SEs are recovered (t 0.44 / 0.82 / 1.84), and **45% of that arm's gap is
the `LeagueShrinkK` split** (`-twoanchor/`). The residual is inside the comparison's noise.

**Not known: which change.** Ungated load-time repairs, an engine change, or a method difference in
how the original table was computed are all still live. ⚠️ **Do not write a cause.** In particular
this record's own rule that *"an estimator swap reads as a data change"* has not been excluded: the
recorded table predates the R pipeline, so its numbers came from the hand-rolled Go statistics that
`sweep_inference.R` was built to replace.

**One confound already excluded, by mechanism rather than evidence:** `min_gain` sat at 0.7 for part
of that era and is 0.4 now, but it is confined to `decide()`, which `HOLD` never calls, so it cannot
touch the column being compared.

## The bisect is tractable, and here is the obstacle it will hit

The range is `c261f32..HEAD`, **514 commits** — and usefully, the recorded table and the `want("BLEND")`
block were **born in the same commit**, with identical arms (8/12/16/24), so the sweep can be run at
any point in the range. At ~7 minutes a run on four seasons, `log₂(513) ≈ 9` runs is about an hour.

⚠️ **Three obstacles, and the one first named here was wrong.**

1. **The cache does not refetch — it reuses period-correct v7 files that already exist.** The cache
   is repo-relative and version-keyed (`backtest-v8-` today, `backtest-v7-` at `c261f32`), and
   `.cache/fpl/` **already holds `backtest-v7-` for all four grid seasons**, written 2026-08-10. So
   an old checkout reads pre-repair data — better than a refetch, and *silent*: check which file was
   read before quoting a level. **Do not rename a v8 file to v7**; the version exists because the
   parsed schema changed.
2. ⚠️ **The real obstacle: the old harness discards its config error.** `c261f32`'s
   `runPolicySweep` calls `config.Load("/Users/bbowman/Projects/fpl/config.json")` with `_` for the
   error — a macOS path that does not exist here — so `config.Load` returns `Default()` and every
   pre-2026-08-10 arm measures the *default* config while the current end measures the repo's. That
   is a confound the bisect **introduces**, and it would look like a clean step.
3. **2025-26 is complete in the archive** (38 gameweeks, 380 fixtures) and the window is after the
   season closed, so the earlier "was in-season, upstream may have drifted" framing was wrong.

## What this buys

- The note's retraction gets its **primary candidate eliminated**: the data state does not explain
  it, and "cause unattributed" is now a narrower and better-supported statement.
- CLAUDE.md's reproduction rule gains the caveat that makes it correct.
- The bisect is scoped, priced (~1 hour) and its one obstacle is named in advance.
