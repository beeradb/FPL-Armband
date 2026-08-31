#!/usr/bin/env Rscript

# Inference for the trajectory-channel diagnostic: does a player's DIRECTION
# across the two prior seasons separate same-position, same-band decisions the
# way TestDiagHaulChannel asked whether SHAPE does?
#
# Usage:
#   Rscript stats/trajectory_channel.R <pairs.csv>
#
# Produce the input with:
#   DIAG=1 go test ./internal/backtest -run TestDiagTrajectoryChannel -v -timeout 60m
#
# That test writes one row per same-position pair inside MinSeparableGain,
# oriented hi-minus-lo on engine Score, to /work/drop/trajectory-2026-08-30/pairs.csv
# as of this writing -- pass whatever path the run actually used.
#
# ⚠️ This does NOT read the sweep cells contract -- its input is one row per
# player pair, not one row per (sweep, season, start point). It sources
# cells_common.R for read_sidecar, the sanctioned reader for a pipeline CSV
# that is not a cells file, and for nothing else. The guard in
# internal/snapshot (TestTheSharedCellQuantitiesHaveOneImplementation) forbids
# a raw `read.csv(` anywhere else under stats/, so this script does not carry
# one either, even though its file has nothing to do with cells.
#
# R is a developer tool here, not a dependency: `go build`, `go vet` and
# `go test` all pass with no R installed, and nothing in the Go test suite
# invokes this script.
# ---------------------------------------------------------------------------

args <- commandArgs(trailingOnly = TRUE)
if (length(args) != 1) {
  stop("usage: trajectory_channel.R <pairs.csv>")
}

# `fail`, `note`, `hr` and `read_sidecar` are in cells_common.R, sourced from
# beside this script the same way prediction_inference.R does it, so this
# works whether it is invoked as `Rscript stats/trajectory_channel.R ...` or
# from some other working directory entirely.
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

d <- read_sidecar(args[1])
d$d_score <- as.numeric(d$d_score)
d$d_pts90_trend <- suppressWarnings(as.numeric(d$d_pts90_trend))
d$d_xgi90_trend <- suppressWarnings(as.numeric(d$d_xgi90_trend))
d$d_mins_trend <- as.numeric(d$d_mins_trend)
d$d_mins_level <- as.numeric(d$d_mins_level)
d$d_points <- as.numeric(d$d_points)
d$price_hi_tenths <- as.numeric(d$price_hi_tenths)
d$rate_gate_ok <- as.numeric(d$rate_gate_ok)

tcrit <- qt(0.975, df = 3)
seasons <- unique(d$season)
cat(sprintf("pairs=%d  seasons=%d  t_crit(df=3)=%.3f\n", nrow(d), length(seasons), tcrit))
cat("=== VOID CHECKS ===\n")
tb <- table(d$season)
print(tb)
cat(sprintf("  max season share %.1f%%   rate gate passes %d (%.1f%%)\n",
            100 * max(tb) / sum(tb), sum(d$rate_gate_ok), 100 * mean(d$rate_gate_ok)))
cat(sprintf("  mean |d_points| %.2f\n\n", mean(abs(d$d_points))))

sig <- function(col, lab, sub = d) {
  x <- sub[[col]]
  keep <- !is.na(x) & x != 0
  if (sum(keep) < 200) {
    cat(sprintf("  %-18s n=%5d  (too few)\n", lab, sum(keep)))
    return(NA)
  }
  y <- sign(x[keep]) * sub$d_points[keep]
  ss <- sub$season[keep]
  per <- tapply(y, ss, mean)
  per <- per[!is.na(per)]
  m <- mean(per)
  se <- sd(per) / sqrt(length(per))
  thr <- tcrit * se
  cat(sprintf("  %-18s n=%6d  mean %+6.3f  SE %5.3f  thr +/-%5.3f  %s\n",
              lab, sum(keep), m, se, thr,
              ifelse(abs(m) > thr, "*** RESOLVES", "not detected")))
  t.test(per)$p.value
}

cat("=== does the higher-trajectory player outscore the lower, next GW? ===\n")
ps <- c()
nm <- c()
for (v in list(c("d_score", "engine Score"), c("d_mins_trend", "minutes trend"),
               c("d_mins_level", "minutes LEVEL (control)"),
               c("d_pts90_trend", "pts/90 trend"), c("d_xgi90_trend", "xGI/90 trend"))) {
  ps <- c(ps, sig(v[1], v[2]))
  nm <- c(nm, v[2])
}

cat("\n=== STRATIFIED BY PRICE (the amended prediction: pays cheap, not dear) ===\n")
bands <- list(c(0, 50, "<=5.0m"), c(51, 60, "5.1-6.0m"), c(61, 80, "6.1-8.0m"), c(81, 999, ">8.0m"))
for (b in bands) {
  lo <- as.numeric(b[1])
  hi <- as.numeric(b[2])
  sub <- d[d$price_hi_tenths >= lo & d$price_hi_tenths <= hi, ]
  cat(sprintf("\n  -- %s  (n=%d)\n", b[3], nrow(sub)))
  for (v in list(c("d_mins_trend", "minutes trend"), c("d_pts90_trend", "pts/90 trend"))) {
    ps <- c(ps, sig(v[1], v[2], sub))
    nm <- c(nm, paste0(v[2], " ", b[3]))
  }
}
ok <- !is.na(ps)
h <- p.adjust(ps[ok], method = "holm")
cat("\n=== HOLM across every comparison ===\n")
for (i in seq_along(h)) {
  cat(sprintf("  %-28s p=%.4f  Holm=%.4f  %s\n", nm[ok][i], ps[ok][i], h[i],
              ifelse(h[i] < 0.05, "*** survives", "does not survive")))
}
