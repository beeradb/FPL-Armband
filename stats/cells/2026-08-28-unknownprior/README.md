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
it. **The price tilt is closed on MECHANISM, and this sweep is consistent with that
rather than establishing it.** The closure rests on the standing argmax rule plus
the ordering evidence failing Holm; the points reading here is
measured-and-unresolved, on an instrument the next section shows is far weaker
than 36 cells implies. "Closed on points" would be a stronger word than this
design earns, and is not vocabulary this record uses.

## ⚠️ Far fewer live cells than thirty-six, and the two knobs die differently

⚠️ **An earlier version of this section said "both knobs are applied only on the
pre-season path". That is true of one of them and false of the other**, and the
error was already contradicted by a live check on the same day: setting
`price_minutes_prior` to 0.5 at GW2 of the running season visibly changed
`armband transfers`. Corrected here after review.

The two arms die for different reasons and at different times:

- **`UnknownPriorShare`** is read only in `unknownPriorRates`, reached only from
  `blendRatesCode`'s `!SeasonHasStarted()` branch. Once any ball is kicked it is
  unreachable. Pre-season only, as claimed.
- **`PriceMinutesPrior`** is read in `priceMinutesTilt`, called from
  `shrinkToLeague` — which `blendRatesCode` calls from **two** places, the
  pre-season branch *and* the in-season no-prior branch. It is gated on
  `GameweeksPlayed() < priceTiltFadesByGW` (11), not on `SeasonHasStarted()`, so
  it keeps firing after kickoff and fades out around GW11.

Checked against the banked CSV on `squad_hash`, not on the log's rounded print:

| entry gw | seasons where any arm differs from baseline | which arms |
|---|---|---|
| 1 | 6/6 | all three — both knobs live |
| 6 | **2/6** | the two tilt arms only; `prior fix` is byte-identical in all six |
| 11, 16, 21, 26 | 0/6 | none |

So the live-cell count is **six guaranteed, plus up to six partial and
knob-specific** — not a clean twelve, and not thirty-six. The middle and late
strata print `+0.000` exactly, which is the record's byte-identical signature:
not a tie, a comparison that never ran.

Both consequences cut against the headline numbers:

- **The pooled mean is diluted several-fold**, because most cells are exact zeros
  by construction.
- **The pooled SE is not honest.** It is computed across cells that could not
  move, so it understates the spread of the few that could.

The real comparison is nearer six to eight cells, which on this harness is below
the recorded "twelve cells could not resolve 37 points a
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
