# The recorded ladder swept two anchors. That is a real channel, and it is not the whole gap.

**Run 2026-08-14**, `EXP=BLEND2 FPL_SWEEP_SEASONS=default FPL_NO_XG_REPAIR=1`, 4 arms × 24 cells,
96 cells, 7m09s. Cells in `cells/blend-both.csv`.

## The hypothesis, stated before the run

At `c261f32` — the commit that both records the disputed table and introduces `want("BLEND")` —
`shrinkToLeague` read the same constant the sweep was setting:

```
blend.go:407   k := e.Weights.BlendRateK       # c261f32
blend.go:407   k := e.Weights.LeagueShrinkK    # today, since c509255 (2026-08-10)
```

So the recorded ladder moved **two** anchors together — the personal prior's strength, and the pull
of a priorless player's rates toward his position's league-wide rates — while today's `BLEND` block
moves one. `BLEND2` sets both to `k`, reproducing the old arm exactly.

## The result: a real channel, not the cause

Shipped `LeagueShrinkK` is 8, so the two runs have an **identical baseline** and the second anchor
isolates as a per-cell paired contrast:

| arm | one anchor | two anchors | **second-anchor effect** | a season | t (CR2, 3 df) |
|---|---:|---:|---:|---:|---:|
| k=12 | −0.326 | −0.096 | **+0.230** | +9 | +0.45 |
| k=16 | −0.052 | +0.212 | **+0.265** | +10 | +0.59 |
| k=24 | +0.351 | −0.490 | **−0.841** | **−32** | −0.97 |

Against the recorded row:

| state | k=12 | k=16 | k=24 |
|---|---:|---:|---:|
| **recorded**, pre-2026-08-10 | −0.632 | −0.740 | −1.509 |
| old data state, one anchor | −0.326 | −0.052 | +0.351 |
| **old data state, two anchors** | **−0.096** | **+0.212** | **−0.490** |

**Verdict: contributor at `k=24`, refuted as the sole cause.** It closes 0.84 of the 1.86 gap at
`k=24` — about **45%** — and it moves `k=12` and `k=16` **away** from the recorded values. A single
mechanism that explains one arm and worsens two is not the explanation.

⚠️ **And it does not resolve on its own terms**: |t| ≤ 0.97 on 3 df at every arm. The 32 points a
season at `k=24` is a point estimate, not a measured effect.

## What the whole arc now says, which is much narrower than it started

Three corrections compound, and together they shrink the original finding a long way:

1. **Only `HOLD` `k=24` is arguably discrepant at all.** Recovering the recorded SEs from the
   recorded `t` (`SE = |mean/t|` → 0.290 / 0.471 / 0.665), the gaps are 0.31 / 0.69 / 1.86 pts/gw
   against conservative independent-run SEs of 0.69 / 0.84 / 1.01 — **t 0.44, 0.82, 1.84**. Nothing
   separates at `k=12` or `k=16`.
2. **Even `k=24` is inside the current run's CR2 interval.** This record says to prefer the
   clustered figure; the recorded −1.509 sits inside `[−1.55, +2.25]`.
3. **Of the `k=24` point gap, ~45% is this estimand change** — a known, dated, documented code
   change, not drift.

**So "the table does not replicate" was over-stated.** What survives is: one of six recorded numbers
moved by more than its own noise would comfortably allow, and about half of that movement is
attributable to the arms having stopped being the same intervention.

⚠️ **The `POLICY` row replicates at both data states** and its recorded monotone decline is intact,
so the retraction was always a `HOLD` retraction and is now a `HOLD` `k=24` retraction.

## What is eliminated, and what a bisect would still be for

**Eliminated or downgraded:**

- The xG/xGC backfills — the switches were set and only 24 of 96 cells could respond, all 2022-23.
- The estimator swap, for the *mean*: `c261f32`'s harness already banked
  `res.Points / len(res.Weeks)` per cell and called `reportPairedDifferences`, the same quantity
  `sweep_inference.R` reproduces at max abs diff 0.
- The `LeagueShrinkK` split — measured here, ~45% of one arm.

**Still open**, and worth at most one more run rather than nine: the vice-captain rule
(`c5aa13d`, 2026-08-09, one day after the recorded table, on the failing metric, and
`FPL_NO_VICE_CAPTAIN=1` is a real switch), and the illegal-substitution fix (`0febe8e`).

⚠️ **But weigh that against finding 2 above.** The residual after the second anchor is ~1.02 pts/gw
at one arm, which is inside the noise of the comparison. **Chasing it further is chasing a
difference the instrument cannot see**, and the honest close is to record the arc and stop rather
than to spend a bisect on it.

## Method note, because it is the transferable part

Two habits, not twenty slips, produced every over-claim in this arc:

- **A gap between two point estimates is not a result until it is divided by something.** The
  recorded SEs were recoverable from the recorded `t` all along.
- **A recorded figure belongs to a row, and equal-sized phase means identify which.** Three phase
  means over 8 cells each average exactly to the row mean; the recorded regime triple
  +0.936 / −0.611 / −1.783 averages to −0.486, which is the **POLICY** `k=12` row. It was compared
  against `HOLD` columns twice before anyone checked.
