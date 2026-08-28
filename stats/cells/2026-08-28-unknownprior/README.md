# Does giving an unknown player a prior earn points, and does price order it?

Banked 2026-08-28. Commit `1428eef1`, **`dirty=false`** — reproducible from that
commit. Six seasons x six entry gameweeks = 36 cells an arm, `HOLD`.

Reproduce:

    FPL_CELLS=/tmp/unknownprior.csv scripts/replay -run TestDiagUnknownPrior -timeout 4h
    Rscript stats/sweep_inference.R /tmp/unknownprior.csv

## The arms

| arm | what it sets |
|---|---|
| baseline | `UnknownPriorShare 0`, `PriceMinutesPrior 0` — reproduces the defect where a player with no prior read ZERO expected minutes |
| prior fix | the position's league rates, unordered |
| fix + price tilt 0.25 | plus a price-rank tilt inside the fallback |
| fix + price tilt 0.50 | the same, stronger |

## Result — nothing resolves

`HOLD`, per gameweek and multiplied by 38, against `t_crit(5) x SE x 38`:

| arm | a season | SE | t | threshold | seasons agreeing |
|---|---:|---:|---:|---:|---:|
| prior fix | **+0.8** | 0.259 | +0.08 | 25.3 | 3/6 |
| fix + tilt 0.25 | −4.0 | 0.086 | −1.22 | 8.4 | 4/6 |
| fix + tilt 0.50 | −0.5 | 0.061 | −0.23 | 5.9 | 3/6 |

Every arm sits well inside its own threshold, and the best of them is a third of
it. **The price tilt does not pay** — which is what the record's own rule
predicts, since its ordering win was never a points claim: a better predictor can
make a worse policy, because the transfer search is an argmax living in the tail.

## ⚠️ Read this as TWELVE cells, not thirty-six

The arms move `UnknownPriorShare` and `PriceMinutesPrior`, and **both are applied
only on the pre-season path** — `unknownPriorRates`, reached from
`blendRatesCode`'s `!SeasonHasStarted()` branch. An entry point after the season
has started cannot reach either knob.

So the middle and late start strata print **`+0.000` exactly**, in every arm.
That is the record's own byte-identical signature: not a tie, a comparison that
never ran. Twenty-four of the thirty-six cells are structurally inert.

Two consequences, and both cut against the numbers above:

- **The pooled mean is diluted by about three**, because two thirds of the cells
  are exact zeros by construction.
- **The pooled SE is not honest.** It is computed across cells that could not
  move, so it understates the spread of the twelve that could.

The early stratum is the real comparison and it is twelve cells, which on this
harness is close to the recorded "twelve cells could not resolve 37 points a
season".

## What this does NOT measure

⚠️ **It does not price the `minutesEvidence` change** — the 2026-08-28 fix that
made `shrinkToLeague` count a club's finished matches rather than the player's
own minutes. That is a different lever on a different path, it is live in every
arm here including the baseline, and it remains **unpriced**. A `HOLD` sweep for
it is owed and has not been run.

⚠️ Absolute totals here are not comparable with anything recorded before this
change: the baseline arm reproduces the old defect and the other three are a
different scoring era. Paired differences within a cell survive; totals do not.

## A hygiene note worth keeping

The first run of this sweep was written to a path that already held a block from
an unrelated earlier run (`UNKNOWNPRIOR#1`, six hours older). Nothing
cross-paired — `read_cells` in `stats/cells_common.R` keys on the sweep and run
id, which is exactly the divergence that guard exists to prevent, and it worked.
The banked file here is a single block written to a fresh path.

A second run was discarded for reading `dirty=true`: a new diagnostic file was
sitting uncommitted in the tree. Same numbers, not citable. The figures above are
from the third run, on a clean tree, and only that one is banked.
