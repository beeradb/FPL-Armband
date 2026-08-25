# Triple captain timed on OPPONENT QUALITY, six-season extended grid, shipped config

`TestDiagTripleCaptainMatchup`, two arms, 36 cells each. Control places the chip
at a fixed offset from entry; the anchored arm places it in the gameweek whose
best available candidate projects highest, from an engine built by `EngineAt` at
the entry cutoff with `Horizon` forced to 1. POLICY. `WeeklyXI` pinned true in
both arms — see the test's own note on why the comparison cannot work without it.

Clean tree, `dirty=false`, at `5b970338`.

## Why these are banked

The figures this comparison first published (+4.6 a season, threshold 11.3) were
taken on a tree carrying uncommitted `MinutesWeight` and `minutesExponent`
changes and were read on the default `per_gw` scale. Nothing was banked, so they
could not be re-derived — only re-measured. These close that.

## Read them with the right estimand

⚠️ `Rscript stats/sweep_inference.R --scale=per_path`, NOT the default. A chip is
an **event count**: the cell total is the whole effect, and dividing by weeks
manufactures a per-gameweek rate that does not exist.

⚠️ `--scale=per_path` writes `stats/out/inference-per_path.csv`, a DIFFERENT file
from the default's `stats/out/inference.csv`.

## What they say

**+2.25 points a season-path, CR2 t 1.38 at df 5, against a threshold of 4.19.
It does not resolve.** Positive in four of six seasons (+7.17, −3.17, +1.83,
+5.33, −1.50, +3.83) and resolving in **0 of 6** leave-one-season-out subsets.
The chip's week differs between arms in 35 of 36 cells, so this is a real timing
contrast and not a set of ties.

⚠️ **It is nonetheless a sign flip from the doubles-only rule**, which reads
−0.75 a season-path on the same grid (`2026-08-25-f7d2be1b/decomposition.csv`).
Both sit below their own thresholds, so the gap between them is not a measured
difference — but the doubles-only rule is bounded to roughly ±2 and this one is
not bounded below the effect its own inputs predict.

⚠️ **The threshold is the number that changed most in the rescale, and it cuts
against the earlier reading.** On the `per_gw` scale this comparison was reported
as resolving ~11.3 a season, which exceeded the ~6.8 the projections implied — so
a null was argued to be a fact about the instrument. **On the correct scale the
threshold is 4.19**, which is *below* what the per-decision instrument measures
for the same rule (+7.95 realised per chip). The replay could have resolved an
effect that size here and it read +2.25. See the captain-week skill note.
