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

**1. The recorded 2x doubling does not reproduce, in either direction.** The record
has calendar anchoring tighter on the reconstruction (t 5.33) than on the measured
source (t 2.32). Here at full sight the **source** is the tighter arm, 4.86 against
5.65; at four gameweeks of sight the reconstruction is 2.6x *wider*. No consistent
ordering.

**2. The SE is not estimable at this df, and the proof is inside one arm.** The
`native` input's own CR2 SE moves **15.23 -> 4.96** between two adjacent lookaheads
with the input held fixed. Ranking three SEs against each other reads noise that a
single arm generates on its own.

**3. The mechanism — the per-cell effect is uncorrelated across inputs while the
totals are nearly identical.** This was not anticipated and is the useful result:

| quantity | native~external | native~recon | external~recon |
|---|---:|---:|---:|
| raw `policy_points` per cell | **0.9981** | **0.9980** | **0.9988** |
| the paired DIFFERENCE per cell | -0.359 | 0.059 | 0.025 |

The chip effect is a small residual on the season — cell SD **36.1** against a raw
cell SD of **537.6**, about 7% — and a 13-14% perturbation of one input
re-randomises that residual completely. All three correlations sit within ~1.4 SE
of zero at n 18. The arms share a mean and little else.

**4. The mean survives; only the ranking dies.** +31.39 / +26.06 / +28.56 at four
gameweeks of sight — a spread of 5.3 points, inside every standard error above.
**The effect is robust to the xGC input. Its interval is not.**

## What it does NOT say

⚠️ **It does not show the reconstruction manufactures precision, and that was
tested.** Its within-season spread is tighter — mean absolute deviation 23.56
against 27.59 (source) and 28.93 (native) — but paired over the same 18 cells that
is **t -0.62, p 0.54** against the source and **t -0.80, p 0.43** against native.
It does not resolve. A claim adjacent to this was published and retracted the same
day on 2026-08-25; this is why it is stated as not resolving rather than as a
finding.

⚠️ **It does not show the six-season doubling was noise.** That was measured at
df 5. This is df 2 and cannot speak to it directly.

⚠️ **The 4gw and 6gw columns are ONE arm printed twice.** They produce identical
plans under every input.

⚠️ **`native` means FPL's published xGC, which is Opta — and so is the measured
source's.** Two Opta-derived measurements and one reconstruction, not three
independent providers.

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

`spread.R` prints the effects, clustered SEs and within-season spreads.
`levene.R` prints the paired variance test and the correlation table, which has no
other generator. Both read the CSVs beside them by repo-relative path; run them from
the repository root.
