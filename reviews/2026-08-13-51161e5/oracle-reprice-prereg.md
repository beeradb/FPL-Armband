# Pre-registration: re-pricing the two oracles on harvested starts

**Written and committed before the run.** The handoff from the independent review is that
`OracleLineups` (≈73 points a season held) and `OracleMinutes` (≈47) were produced against
**rank-reconstructed** starting elevens, which the harvest replaces — so they sit on changed inputs
and are **unmeasured rather than unchanged**.

## Design

Two runs of `TestDiagMinutesOracle` on the shipped six-season grid, differing **only** in
`FPL_NO_STARTS_REPAIR`. Same code, same commit, same cells; one data state changed. Five arms each
— baseline, `availability`, `lineups`, `minutes`, `seasonwindow` — so 180 cell-replays per state.

Paired on `(season, start_gw)`. The quantity of interest is the **paired difference in each
oracle arm's gain over its own baseline**, between the two starts states — not the level, which is
confounded by every other change since the ≈73 and ≈47 were recorded.

## What is predicted, and what would falsify it

1. **The baseline arm must be byte-identical between the two states in all 36 cells.** It does not
   read `Starts`; this is the same invariance already measured on `TestDiagBaseline`. A violation
   means the harness is wrong, not that starts matter — it does not become a finding about starts.
2. **All movement must be confined to the 24 live cells.** `repairdata/` stops at 2022-23, so the
   12 cells replaying 2024-25 and 2025-26 carry no harvested row in either their own season or
   their prior and **cannot move in any arm**. If one does, the harvest is reaching a season
   `TestTheStartsHarvestCannotReachARecordedSeason` says it must not.
3. **The `recon` column of the conditional-prices table is the transmission check.** With the
   harvest on it should fall to ≈0 for 2020-21 through 2022-23; with it off it should be high. If
   `recon` does not move, the arm did not arrive and the points columns mean nothing — the exact
   failure the `fetch`-placement bug produced once already.

**No direction is predicted for the oracle prices, and that is deliberate.** The reconstruction's
error is biased by role — it flatters the player whose start share is least certain — so correcting
it could raise or lower what perfect selection is worth. Predicting a sign here would be inventing
one.

## What this run cannot settle

The **levels** ≈73 and ≈47 are not re-established by it. Those were recorded on an older code and
data state, and this design deliberately measures a paired difference instead, because a level
comparison across three days and a merge is the "name the data state" error this branch was
already caught by once.
