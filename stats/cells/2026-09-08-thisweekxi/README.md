# This-week XI on HOLD — difficulty isolated from load

Banked 2026-09-08. Commit `d3c81e93`, dirty=false. Six seasons × six entry
gameweeks = 36 cells an arm, HOLD. `FPL_XGC_EXTERNAL_DIR` was set (measured xGC).

Reproduce:

    DIAG=1 EXP=FIXH FPL_SWEEP_SEASONS=extended FPL_CELLS=/tmp/fixh.csv \
      scripts/replay -run TestDiagThisWeekXIOnHold -v -timeout 2h
    Rscript stats/sweep_inference.R /tmp/fixh.csv

Inference is `stats/sweep_inference.R` (CR2, Holm over A1 and A2 vs A0, wild
cluster bootstrap). Do not quote the POLICY columns: they were filled with HOLD
totals so the required CSV contract is met.

## Arms

| arm | fielding |
|---|---|
| A0 | then-shipped `HoldCaptaincyWeekly` (horizon 5). After the ship this is `HoldFielding{Horizon: 5}` |
| A1 | this-GW FDR only: skip other gameweeks, Score=0 on blanks, both this-GW double legs averaged (horizon 5; load already off Score, `DisableLoad` not set) |
| A2 | horizon 1, load on — now shipped `HoldCaptaincyWeekly` / `WeekEngine` fielding |

## Result — HOLD, per gameweek × 38, t_crit(5)=2.571

| arm | pts/gw | a season | SE CR2 | t | threshold | Holm p | wild p | seasons |
|---|---:|---:|---:|---:|---:|---:|---:|---|
| A1 difficulty | 0.289 | +11.0 | 0.161 | 1.80 | 15.7 | 0.132 | 0.163 | 4/6 |
| A2 load+week | 0.653 | +24.8 | 0.151 | 4.31 | 14.7 | 0.015 | 0.014 | 6/6 |

A1 does not clear. A2 clears CR2 and Holm; wild does not withdraw. The A2−A1
wedge is the double half of load (blanks match on A1 and A2). No change to
`fixture_weight`, ladders, or `BandStrength`.
