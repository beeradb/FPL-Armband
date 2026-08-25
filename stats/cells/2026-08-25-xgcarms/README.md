# The xGC source, controlled: reconstruction against measured, one variable

`TestDiagAnchoredChipDecomposition`, four two-arm blocks, one chip isolated per
block at 4 gameweeks of sight, six-season extended grid, 36 cells per arm, POLICY.

**Both runs are at the same code state and both are clean.**

| | commit | `dirty` | `FPL_XGC_EXTERNAL_DIR` |
|---|---|---|---|
| `reconstruction.csv` | `9f2553f1` | false | *absent* |
| `measured-xgc.csv` | `f30e1cea` | false | set |

⚠️ `f30e1cea..9f2553f1` is **comment-only** — every changed line begins with `//`,
verified by `git diff -U0`. A deterministic replay cannot differ across it. The
two sidecars therefore differ in exactly one field, and that field is the
declared variable.

## Read them with the right estimand

⚠️ `Rscript stats/sweep_inference.R --scale=per_path`, NOT the default. A chip is
an event count; the per-gameweek scale inflates these arms ~1.7x.

## ⚠️ Why these exist rather than a difference against `2026-08-25-f7d2be1b`

Because that difference would not have been about xGC. Those cells were banked
inside PR #83 (`74d7fe1f`), and **PR #82 (`5b970338`) merged after it** and
rewrote `blend.go`, `dpseed.go` and `funding.go` — squad construction on the
scored path. Differencing a new arm against them carries #82 as well.

**The tell generalises**: the uncontrolled comparison moved 2024-25 and 2025-26,
which the measured source cannot touch and whose priors it cannot touch either.
**A deterministic replay cannot move a season whose inputs did not change.**

## What they say

### 1. The overlay does exactly what it declares, and nothing else

Over all 288 matched cells, the maximum absolute per-cell POLICY difference is:

| 2020-21 | 2021-22 | 2022-23 | 2023-24 | 2024-25 | 2025-26 |
|---:|---:|---:|---:|---:|---:|
| 113 | 176 | 120 | **0** | **0** | **0** |

The three declared seasons move; the three undeclared ones are identical in every
cell. That is the coverage claim verified at cell level rather than asserted.

### 2. The source does not move the effect — it moves the VARIANCE

| chip alone | reconstruction | measured xGC |
|---|---|---|
| free hit | **+21.0, SE 5.83, t 3.60, thr 15.0, 6/6 LOSO** | +20.7, SE **11.81**, t 1.75, thr 30.3, 1/6 |
| bench boost | +4.4, SE 0.94, t 4.65, thr 2.4, 6/6 | +4.5, SE 0.83, t 5.41, thr 2.1, 6/6 |
| wildcard | −1.4, SE 11.04, t −0.13 | −5.4, SE 13.39, t −0.40 |
| triple captain | +1.4, SE 1.72, t 0.79 | +1.7, SE 1.63, t 1.04 |

**Free hit's point estimate is unchanged to within 0.3 points and its SE
doubles**, so it resolves on the reconstruction and does not on the measured
source. Per-season, 2020-21 goes +32.0 → −26.8 and 2021-22 goes +20.7 → +58.2:
large swings in opposite directions, which is what inflates a season-clustered SE.

⚠️ **This is specific to free hit, not a verdict on the source.** Bench boost and
triple captain are essentially untouched, and bench boost's SE actually falls.
Free hit swaps the whole fifteen for one week, so its value routes through squad
selection, which is where xGC acts.

⚠️ **It cuts both ways and neither way is established.** Either the transport step
smooths a variance that is really there, or the measured source injects one that
is not. Nothing here separates those, and
`2026-08-24-fotmob-is-opta-...` is explicit that the source is shown only to be
"not worse", never more accurate.

### 3. ⚠️ PR #82 changed free hit more than the data source did

`reconstruction.csv` is the same arm as `2026-08-25-f7d2be1b/decomposition.csv`'s
free-hit block, at a later commit:

| free hit, reconstruction | 20-21 | 21-22 | 22-23 | 23-24 | 24-25 | 25-26 | mean | t | LOSO |
|---|---:|---:|---:|---:|---:|---:|---:|---:|---|
| pre-`5b970338` | −4.7 | −3.0 | +18.0 | +25.2 | +28.0 | +23.7 | +14.5 | 2.44 | 2/6 |
| post-`5b970338` | +32.0 | +20.7 | +1.3 | +6.2 | +30.3 | +35.3 | **+21.0** | **3.60** | **6/6** |

**On today's `main` free hit RESOLVES**, all six seasons positive, no negative
season to drop. The "two negative seasons, LOSO 2 of 6" reading is a property of
the pre-#82 codebase and must not be quoted as current.
