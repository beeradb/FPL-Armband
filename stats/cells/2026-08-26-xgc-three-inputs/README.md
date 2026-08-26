# Three xGC inputs on the same football — can the arms be ranked by standard error?

**No, and the reason is worth more than the answer.**

`TestDiagAnchoredChips`, three separate processes at ONE commit (`6eca3167`),
`dirty=false` in all three, differing in exactly one declared variable:
`FPL_XGC_FORCE`. `FPL_SWEEP_SEASONS=native`, 18 cells per arm (3 season pairs x 6
entry points), POLICY, `--scale=per_path`.

The native grid is the only place all three inputs describe the same football. The
measured source turns out to carry all 380 matches of each of 2023-24, 2024-25 and
2025-26 — it had only ever been used for the three repaired seasons — which is what
makes the comparison possible at all.

⚠️ **Three seasons is df 2, `t_crit` 4.303.** Nothing below resolves and nothing is
offered as resolving. The quantity was meant to be the SE ratio between arms.

## Effect, points per season-path, against `chips at fixed offsets from entry`

| input | full sight | 2 gw sight | 4 gw / 6 gw sight |
|---|---:|---:|---:|
| `native` — FPL's own published xGC | +27.11 | +24.67 | +31.39 |
| `external` — the measured per-match source | +19.06 | +18.00 | +26.06 |
| `reconstruction` — opponents' xG shared by minutes | +24.28 | +15.06 | +28.56 |

## CR2 season-clustered standard error, same cells

| input | full sight | 2 gw sight | 4 gw / 6 gw sight |
|---|---:|---:|---:|
| `native` | 9.08 | **15.23** | **4.96** |
| `external` | 4.86 | 4.95 | 3.73 |
| `reconstruction` | 5.65 | 5.21 | 9.80 |

## What it says

**1. This design CANNOT rank the inputs by clustered standard error, and that is
a fact about df 2 rather than about the arms.** The 95% sampling band on a ratio
of two df-2 SD estimates is **F(2,2) = [0.16, 6.24]**. A true 2x ratio is
invisible inside that band and a true 1x ratio produces ratios well past 2 by
chance. So the recorded doubling is **not testable here** — not "not reproduced".
The observed spread (source tighter at full sight, reconstruction 2.6x wider at
4gw) is what a band that wide looks like.

**2. What IS estimable is the WITHIN-season spread, and there the three agree.**
Those are df 15 each, band [0.59, 1.69], and the SDs are **36.16 / 36.07 / 31.09**
— a max ratio of **1.16**. **A 2x difference in within-season spread between these
inputs is excluded on this football.** This is the one ratio in the experiment
with power and it is the honest headline.

**3. The per-cell effect is predominantly not a property of the cell.**

| quantity | native~external | native~recon | external~recon |
|---|---:|---:|---:|
| the paired DIFFERENCE per cell | -0.359 | 0.059 | 0.025 |

⚠️ **Read the null before reading the number.** If the input changed nothing but
added a small independent perturbation, these would correlate near **+1**, not 0:
differencing removes the shared season path, and what is left is the cell-level
structure of the effect. Near zero says that structure mostly is not there.
`SE(r)` is ~0.26 at n 18, so this **excludes large shared structure and cannot
distinguish 0 from ~0.3** — "predominantly", not "completely". In points, the same
fact: the arm-to-arm difference SD is **59.1** against a within-arm cell spread of
**36.1**, which is the arithmetic consequence of the near-zero correlation rather
than a second finding.

**4. The arm means are INDISTINGUISHABLE, down to a floor of 35-61 points.** Not
"robust" — a null is only a result if it clears its own threshold, and these do
not come close to clearing theirs:

| contrast | mean | clustered SE | t | detection floor |
|---|---:|---:|---:|---:|
| native - external | +5.33 | 8.56 | +0.62 | 36.8 |
| native - reconstruction | +2.83 | 14.25 | +0.20 | 61.3 |
| external - reconstruction | -2.50 | 8.04 | -0.31 | 34.6 |

The data is consistent with the input moving the mean by up to 35-61 points a
season-path. What can be said positively is weaker and does not need the SEs:
**three arms whose per-cell residuals barely correlate are all positive at every
sight setting**, which is sign-consistency evidence that the effect is above zero.

⚠️ **Do NOT read per-arm t values off the table below.** At 4gw sight they are
6.33 (native) and 6.99 (external) against `t_crit` 4.303, which would "resolve" —
but point 1 says those SEs are not usable, and a table cannot have it both ways.
Either the df-2 SEs rank and the effect resolves, or they do not and it does not.
This README takes the second position throughout, and the SE table is printed to
show the spread, not to be tested against.

## What it does NOT say

⚠️ **It does not show the reconstruction manufactures precision, and that was
tested twice.** Its within-season spread is tighter — mean absolute deviation
23.56 against 27.59 (source) and 28.93 (native) — but over the same 18 cells,
paired: **Levene t -0.80 p 0.43 / t -0.62 p 0.54**, and **Pitman-Morgan**, the
correct parametric test for two correlated variances and the more powerful one,
**t -0.74 p 0.47 / t -0.72 p 0.48**. Neither resolves. A claim adjacent to this
was published and retracted the same day on 2026-08-25; the more powerful test
was run precisely so that declining it is a measurement rather than a reflex.

⚠️ **It does not show the six-season doubling was noise.** That was measured at
df 5. This is df 2 and by point 1 cannot speak to it.

⚠️ **The 4gw and 6gw columns are ONE arm printed twice** — verified byte-identical
`policy_points` AND `squad_hash` on all 18 cells in all three files. Nothing pools
across variants.

⚠️ **`native` means the values the data provider published, and the measured
source is derived from the same underlying provider.** Two measurements of one
provider and one reconstruction, not three independent providers.

## Provenance

| file | `FPL_XGC_FORCE` | run id |
|---|---|---|
| `native.csv` | `native` — the archive untouched | `1787764127-1803155` |
| `external.csv` | `external` | `1787763469-1800106` |
| `reconstruction.csv` | `reconstruction` | `1787763740-1802039` |

⚠️ **The 2022-23 prior is NOT forced in any arm.** `applyForcedXGC` only touches a
season carrying a native truth, so the pair's prior takes the shipped path in all
three and is constant across them. Without that gate the arms would have differed in
their priors as well as in the season being scored, and would not have been one
comparison.

## Reproducing

```bash
DIAG=1 EXP=ANCHORED FPL_SWEEP_SEASONS=native \
  FPL_XGC_EXTERNAL_DIR=<the measured source> FPL_XGC_FORCE=<input> \
  FPL_CELLS=stats/cells/2026-08-26-xgc-three-inputs/<input>.csv \
  go test ./internal/backtest -run '^TestDiagAnchoredChips$' -count=1 -v -timeout 90m
```

⚠️ **`-count=1` is not optional and its absence cost a run here.** A cells file is a
side effect the Go test cache cannot see: the second run of an arm at the same commit
printed every arm's totals, exited 0 and wrote nothing at all. Guarded since by
`TestDocumentedCellsCommandsDisableTheTestCache`.

⚠️ **`FPL_CELLS` APPENDS.** An arm re-run into an existing path leaves both blocks in
one file under one block label. `native.csv` was contaminated exactly this way before
the run banked here — delete the file, never overwrite it.

`arms.R` regenerates EVERY figure above — the bands, the difference-of-differences
with its floors, the correlations and both variance tests. Run it from the
repository root; it reads the cells through `stats/cells_common.R`, which is the
only reader for this family.

⚠️ **The three `constants_digest` values DIFFER across the sidecars, by design.**
`internal/snapshot/provenance.go` hashes the config subtrees **plus the env
switches**, and `FPL_XGC_FORCE` is fingerprinted — so declaring it as the varied
variable necessarily moves the digest. A reviewer diffing the sidecars would
otherwise conclude the shipped constants moved. The per-`constant` rows are what
actually compare, and they agree.
