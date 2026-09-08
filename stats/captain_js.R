#!/usr/bin/env Rscript

# James–Stein linear shrink captain overlay — 2026-09-08 prereg
# (JS25 / JS50 / JS75 against A0 = λ=0).
#
# Usage:
#   Rscript stats/captain_js.R /tmp/captain-js/cells.csv
#
# Produce the input with:
#   DIAG=1 EXP=JS FPL_CELLS=/tmp/captain-js/cells.csv FPL_SWEEP_SEASONS=extended \
#     scripts/replay -run '^TestDiagCaptainJamesStein$' -v -timeout 3h
#
# Estimand: HOLD points per season-path (the cell total). No ×38, no /weeks.
# Family size 3 (JS25−A0, JS50−A0, JS75−A0); Holm at family-wise α = 0.05.
# CR2 / MDE / Holm use cells_common.R (one quantity, one implementation).
#
# An arm with weeks_captain_differs below 5% of weeks_total across the 36 cells
# is VOID — the shrink never bit — not a null.

args <- commandArgs(trailingOnly = TRUE)
if (length(args) != 1) {
  stop("usage: captain_js.R <cells.csv>", call. = FALSE)
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
need <- c("season", "start", "horizon_weeks", "lambda",
          "hold_points", "weeks_total", "weeks_captain_differs", "squad_hash")
missing <- setdiff(need, names(d))
if (length(missing) > 0) {
  fail("cells CSV is missing columns: ", paste(missing, collapse = ", "))
}

d$start <- as.numeric(d$start)
d$horizon_weeks <- as.numeric(d$horizon_weeks)
d$lambda <- as.numeric(d$lambda)
d$hold_points <- as.numeric(d$hold_points)
d$weeks_total <- as.numeric(d$weeks_total)
d$weeks_captain_differs <- as.numeric(d$weeks_captain_differs)
d$start_gw <- d$start
d$cell <- paste(d$season, d$start)

note("")
hr()
note("captain James–Stein linear shrink — HOLD points per season-path")
note("arms: λ ∈ {0, 0.25, 0.50, 0.75}; family size 3 against A0 (λ=0)")
note("cells per arm: ", length(unique(d$cell[d$lambda == 0])),
     "   seasons: ", length(unique(d$season)))
note("UNITS: points per season-path. Do not divide by weeks; do not multiply by 38.")
hr()

a0 <- d[d$lambda == 0, ]
if (nrow(a0) == 0) fail("no λ=0 (A0) rows in the cells file")

arms <- c(0.25, 0.50, 0.75)
arm_names <- c("JS25", "JS50", "JS75")
names(arm_names) <- as.character(arms)

# VOID check pooled across the 36 cells per arm.
for (lam in arms) {
  arm <- d[abs(d$lambda - lam) < 1e-9, ]
  share <- sum(arm$weeks_captain_differs) / sum(arm$weeks_total)
  note(sprintf("VOID check %s (λ=%.2f): weeks_captain_differs share %.4f  (%d / %d)%s",
               arm_names[as.character(lam)], lam, share,
               sum(arm$weeks_captain_differs), sum(arm$weeks_total),
               if (share < 0.05) "  -> VOID (shrink never bit)" else ""))
}

if (!has_cs) {
  note("")
  note("!! clubSandwich is not installed — no CR2 SE, no threshold, no Holm.")
  quit(status = 1)
}

# Build paired per-path differences against A0.
rows <- list()
ps <- c()
for (lam in arms) {
  arm <- d[abs(d$lambda - lam) < 1e-9, ]
  j <- merge(
    a0[, c("cell", "season", "start_gw", "hold_points", "weeks_total")],
    arm[, c("cell", "hold_points", "weeks_captain_differs")],
    by = "cell", suffixes = c(".a0", ".arm")
  )
  j$diff <- j$hold_points.arm - j$hold_points.a0
  nm <- arm_names[as.character(lam)]
  rows[[nm]] <- data.frame(
    season = j$season, start_gw = j$start_gw, diff = j$diff,
    weeks_captain_differs = j$weeks_captain_differs,
    weeks_total = j$weeks_total,
    stringsAsFactors = FALSE
  )
  cr <- se_cr2(rows[[nm]])
  ps <- c(ps, cr$p)
}
names(ps) <- arm_names

holm <- p.adjust(ps, method = "holm")

note("")
note("--- paired differences (arm − A0), season-clustered CR2 ---")
note(sprintf("%-6s %8s %7s %6s %8s %6s %8s %8s %8s",
             "arm", "mean", "SE", "df", "thr", "|m|>thr", "Holm p", "MDE80",
             "differs%"))

for (nm in arm_names) {
  fit <- rows[[nm]]
  m <- mean(fit$diff)
  cr <- se_cr2(fit)
  sf <- se_cr2_start(fit)
  if (is.na(cr$se)) {
    note(sprintf("%-6s degenerate (byte-identical to A0 — VOID, not a null)", nm))
    next
  }
  smde <- sig_and_mde(cr$se, cr$df)
  thr <- unname(smde["sig"])
  mde80 <- unname(smde["mde"])
  clears <- abs(m) > thr
  holm_ok <- !is.na(holm[[nm]]) && holm[[nm]] < 0.05
  # Mechanical outcome of the prereg decision rule — not a shipping claim.
  resolves <- clears && holm_ok
  share <- sum(fit$weeks_captain_differs) / sum(fit$weeks_total)
  note(sprintf("%-6s %+8.2f %7.2f %6.1f %8.2f %6s %8.4f %8.2f %7.1f%%",
               nm, m, cr$se, cr$df, thr,
               if (clears) "yes" else "no",
               holm[[nm]], mde80, 100 * share))
  if (!is.na(sf$se)) {
    note(sprintf("       start-fixed SE %6.2f df %4.1f", sf$se, sf$df))
  }
  note(sprintf("       resolves ( |mean|>thr AND Holm<0.05 ): %s",
               if (resolves) "yes" else "no"))
  if (share < 0.05) {
    note("       !! VOID: differs share < 5% — comparison never ran; not a null.")
  }
}

note("")
hr()
note("Family: JS25−A0, JS50−A0, JS75−A0. Holm at family-wise α = 0.05.")
note("A non-resolving arm with a live VOID check may be written")
note("\"ruled out for shipping\" only after the A0/C/F sibling showed the")
note("instrument can see a ~28-point armband difference; otherwise power failure.")
note("Six clusters cannot establish a null.")
hr()
