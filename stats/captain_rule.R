#!/usr/bin/env Rscript

# Captain calibration A0 / C / F — sizing rungs of the 2026-09-01 prereg
# (ownership-censor arms A1–A4 are NOT in this file).
#
# Usage:
#   Rscript stats/captain_rule.R /tmp/captain-cal/cells.csv
#
# Produce the input with:
#   DIAG=1 EXP=CALIB FPL_CELLS=/tmp/captain-cal/cells.csv FPL_SWEEP_SEASONS=extended \
#     scripts/replay -run '^TestDiagCaptainCalibration$' -v -timeout 3h
#
# Estimand: HOLD points per season-path (the cell total). No ×38, no /weeks.
# C−A0 and F−A0 are SIZING, not a Holm family — reported descriptively.
# CR2 / MDE come from cells_common.R (one quantity, one implementation).

args <- commandArgs(trailingOnly = TRUE)
if (length(args) != 1) {
  stop("usage: captain_rule.R <cells.csv>", call. = FALSE)
}

local({
  a <- commandArgs(trailingOnly = FALSE)
  f <- sub("^--file=", "", a[grep("^--file=", a)])
  d <- if (length(f) > 0) dirname(normalizePath(f[1])) else "stats"
  p <- file.path(d, "cells_common.R")
  if (!file.exists(p)) {
    stop("CONTRACT VIOLATION: cells_common.R not found beside this script: ", p)
  }
  source(p, local = FALSE)
})

d <- read_sidecar(args[[1]])
need <- c("season", "start", "horizon_weeks",
          "hold_points", "hold_fixedcap_points", "hold_oracle_points",
          "weeks_total", "weeks_oracle_differs", "squad_hash")
missing <- setdiff(need, names(d))
if (length(missing) > 0) {
  fail("cells CSV is missing columns: ", paste(missing, collapse = ", "))
}

d$start <- as.numeric(d$start)
d$horizon_weeks <- as.numeric(d$horizon_weeks)
d$hold_points <- as.numeric(d$hold_points)
d$hold_fixedcap_points <- as.numeric(d$hold_fixedcap_points)
d$hold_oracle_points <- as.numeric(d$hold_oracle_points)
d$weeks_total <- as.numeric(d$weeks_total)
d$weeks_oracle_differs <- as.numeric(d$weeks_oracle_differs)
d$start_gw <- d$start

d$diff_f <- d$hold_fixedcap_points - d$hold_points
d$diff_c <- d$hold_oracle_points - d$hold_points

note("")
hr()
note("captain calibration A0/C/F — HOLD points per season-path")
note("cells: ", nrow(d), "   seasons: ", length(unique(d$season)),
     "   entry points: ", length(unique(d$start)))
note("C−A0 and F−A0 are SIZING (descriptive). Not in a Holm family.")
note("UNITS: points per season-path. Do not divide by weeks; do not multiply by 38.")
hr()

oracle_share <- sum(d$weeks_oracle_differs) / sum(d$weeks_total)
note(sprintf("weeks_oracle_differs share (C vs A0 captain): %.3f  (%d / %d)",
             oracle_share, sum(d$weeks_oracle_differs), sum(d$weeks_total)))
if (sum(d$weeks_oracle_differs) == 0) {
  note("!! VOID: C never differed from A0 in any cell — wiring dead, not a null.")
}

report_sizing <- function(label, diff) {
  fit <- data.frame(diff = diff, season = d$season, start_gw = d$start_gw,
                    stringsAsFactors = FALSE)
  m <- mean(fit$diff)
  note("")
  note("--- ", label)
  note(sprintf("  mean %+0.2f a season-path  (n=%d cells)", m, nrow(fit)))
  # Six season means of the cell differences (the descriptive secondary).
  sm <- tapply(fit$diff, fit$season, mean)
  note(sprintf("  season means: %s",
               paste(sprintf("%s=%+.2f", names(sm), sm), collapse = "  ")))
  if (!has_cs) {
    note("  !! clubSandwich not installed — no CR2 SE, no threshold, no MDE.")
    return(invisible(NULL))
  }
  cr <- se_cr2(fit)
  sf <- se_cr2_start(fit)
  if (is.na(cr$se)) {
    note("  CR2 degenerate (byte-identical or <2 clusters)")
    return(invisible(NULL))
  }
  smde <- sig_and_mde(cr$se, cr$df)
  thr <- unname(smde["sig"])
  mde80 <- unname(smde["mde"])
  clears <- abs(m) > thr
  note(sprintf("  CR2    SE %6.2f  df %4.1f  t_crit %5.3f  threshold %6.2f  |mean|>thr %s",
               cr$se, cr$df, unname(smde["t_crit"]), thr,
               if (clears) "yes" else "no"))
  note(sprintf("  MDE80  %6.2f", mde80))
  if (!is.na(sf$se)) {
    note(sprintf("  start  SE %6.2f  df %4.1f  (robustness rival, not the primary)",
                 sf$se, sf$df))
  }
  note(sprintf("  resolves_vs_threshold: %s  (sizing only — no Holm, no ship word)",
               if (clears) "yes" else "no"))
}

report_sizing("F − A0  (GW1-pinned minus shipped; the known ~28-point span)", d$diff_f)
report_sizing("C − A0  (perfect armband minus shipped; the ceiling)", d$diff_c)

note("")
hr()
note("Decision context (from the prereg, mechanical):")
note("  If F−A0 does not clear its own threshold, every JS null is uninformative.")
note("  If C never moves, the instrument cannot see armband effects at all.")
hr()
