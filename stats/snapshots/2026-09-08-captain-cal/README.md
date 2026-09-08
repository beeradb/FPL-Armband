# Captain calibration A0 / C / F — 2026-09-08

HOLD overlay, 36 cells (6 seasons × starts 1/6/11/16/21/26). Chips off. `WeeklyXI` false.

Primary data state: **legacy/Opta xGC**, `FPL_XGC_EXTERNAL_DIR` unset, `FPL_SWEEP_SEASONS=extended`, commit `4f5aa59c`. Cells and inference in `legacy/`.

A first pass ran with `FPL_XGC_EXTERNAL_DIR` set (FotMob cache) because that env is on in this shell. That is a **separate comparison**, not a robustness check on the primary. Banked in `measured-xgc/`. Do not difference the two tables without `--vary=FPL_XGC_EXTERNAL_DIR`.

Diagnostic: `internal/backtest/captaincalibration_diag_test.go`.
Inference: `stats/captain_rule.R` (per_path; C−A0 and F−A0 are sizing, not a Holm family).

A0 = `HoldCaptaincyWeekly.Full`. F = `FixedCaptain` (same pass). C = `bestArmband` on that week's XI.

Estimand is HOLD points per season-path. Do not divide by weeks. Do not multiply by 38.
