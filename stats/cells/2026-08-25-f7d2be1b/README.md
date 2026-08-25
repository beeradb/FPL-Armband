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
