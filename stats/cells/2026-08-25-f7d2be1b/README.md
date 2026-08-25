# ANCHORED, six-season extended grid, at the shipped config

`TestDiagAnchoredChips`, 36 cells per arm, five arms (control at fixed offsets;
anchored at full sight; anchored at 2, 4 and 6 gameweeks of sight). POLICY.

## Why these are banked and the earlier ones were not

The record carried a standing gap — no anchored-chip cells were banked anywhere,
so nothing that used one could be re-derived, only re-measured. These close it
for this comparison.

They also replace a contaminated run. Every chip sweep taken earlier on
2026-08-25 was measured against a tree carrying an uncommitted change to
`MinutesWeight` (1.25 -> 1.0) and to `minutesExponent`, while its sidecar stamped
a commit that shipped 1.25. The sidecar recorded `dirty=true`, which is precisely
the flag meaning it is NOT a complete record of what was measured: it enumerates
config and cannot see a changed function body. **This run's sidecar records
`dirty=false`.** It is the first chip measurement in that sequence where the
provenance is complete.

## Read them with the right estimand

⚠️ `Rscript stats/sweep_inference.R --scale=per_path`, NOT the default. A chip is
an **event count**: the cell total is the whole effect, and dividing by weeks
manufactures a per-gameweek rate that does not exist. The default `per_gw` scale
inflates these arms by roughly 1.7x, and that convention error has already
produced one retracted figure in this project's history — and then produced
another on 2026-08-25 before review caught it.

⚠️ `--scale=per_path` writes `stats/out/inference-per_path.csv`, a DIFFERENT file
from the default's `stats/out/inference.csv`. Reading the wrong one silently
returns the previous run's numbers.

## What they say

At 4 gameweeks of sight — the shortest lookahead the test's own bar accepts as
strategy rather than hindsight — anchoring reads **+20.6 points a season-path,
CR2 t 3.63 against a threshold of 14.5**, season-clustered at df 5. Positive in
six of six seasons (+0 to +36), and resolving in all six leave-one-season-out
subsets (weakest t 2.86). Full sight reads +24.0; 6gw +20.6; **2gw does not
resolve** (+12.9, t 1.89, threshold 17.5).

⚠️ These are NOT comparable with any figure banked before 2026-08-25: the shipped
`MinutesWeight` moved 1.25 -> 1.0 in the same branch, which reprices every
midfielder in every cell. An estimator swap reads as a data change.

---

## Also banked here: the per-chip decomposition and the wildcard-value arm

Same run conditions — clean tree (`dirty=false`), shipped config, six-season
extended grid, POLICY, read with `--scale=per_path`.

`decomposition.csv` — `TestDiagAnchoredChipDecomposition`, four two-arm blocks,
one chip isolated per block at 4 gameweeks of sight:

| chip alone | pts/season-path | CR2 t | threshold | |
|---|---:|---:|---:|---|
| free hit | +14.5 | 2.44 | 15.3 | **does not resolve** |
| bench boost | +5.6 | 7.74 | 1.9 | resolves |
| wildcard | +6.4 | 0.61 | 27.0 | does not resolve |
| triple captain | −0.8 | −0.98 | 2.0 | does not resolve |

⚠️ **Only bench boost resolves individually, and it is the smallest of the three
that contribute.** Free hit is the largest single component and misses its own
threshold (14.5 against 15.3, t 2.44 against t_crit 2.571). The three bundled
chips sum to +19.4 against the bundled arm's +20.6, so the effect is additive —
but "free hit carries it" is a statement about a point estimate, not a resolved
one, and must not be quoted as measured.

`wildcardvalue.csv` — `TestDiagWildcardValue`, one wildcard at entry+8 against no
chip at all: **−7.6 a season-path, t −1.07, threshold 18.2.** A tie. Four of six
seasons negative, two clearly positive, LOSO 0 of 6. The standing claim that this
policy has nothing for a wildcard to undo reproduces.
