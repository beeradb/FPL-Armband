#!/usr/bin/env Rscript

# Inference for the variance-frontier diagnostic: does trading points for
# weekly SD (ownership, price or haul-weighted picks) find a genuine frontier,
# or does it just cost points nobody can see it costing at this sample size?
#
# Usage:
#   Rscript stats/variance_frontier.R <weeks.csv> <seasons.csv>
#
# Produce the input with:
#   DIAG=1 go test ./internal/backtest -run TestDiagVarianceFrontier -v -timeout 60m
#
# That test writes weeks.csv (one row per season/arm/gameweek, gross and net
# points) and seasons.csv (one row per season/arm, season points and the
# void-check slot count) to /work/drop/variance-frontier-2026-08-30/ as of
# this writing -- pass whatever paths the run actually used.
#
# ⚠️ This does NOT read the sweep cells contract -- both inputs are the
# variance-frontier diagnostic's own shape, not one row per (sweep, season,
# start point). It sources cells_common.R for read_sidecar, the sanctioned
# reader for a pipeline CSV that is not a cells file, and for nothing else.
# The guard in internal/snapshot (TestTheSharedCellQuantitiesHaveOneImplementation)
# forbids a raw `read.csv(` anywhere else under stats/, so this script does
# not carry one either, even though neither file has anything to do with
# cells.
#
# R is a developer tool here, not a dependency: `go build`, `go vet` and
# `go test` all pass with no R installed, and nothing in the Go test suite
# invokes this script.
# ---------------------------------------------------------------------------

args <- commandArgs(trailingOnly = TRUE)
if (length(args) != 2) {
  stop("usage: variance_frontier.R <weeks.csv> <seasons.csv>")
}

# `fail`, `note`, `hr` and `read_sidecar` are in cells_common.R, sourced from
# beside this script the same way prediction_inference.R does it, so this
# works whether it is invoked as `Rscript stats/variance_frontier.R ...` or
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

w <- read_sidecar(args[1])
s <- read_sidecar(args[2])
w$gross <- as.numeric(w$gross)
w$gw <- as.numeric(w$gw)
s$points <- as.numeric(s$points)
s$slots_vs_baseline <- as.numeric(s$slots_vs_baseline)

tcrit <- qt(0.975, df = 5)
seasons <- unique(s$season)
arms <- unique(s$arm)
cat(sprintf("arms=%d seasons=%d t_crit(df=5)=%.3f\n\n", length(arms), length(seasons), tcrit))

cat("=== 1. VOID CHECK: did each arm bite? ===\n")
for (a in arms) {
  if (a != "baseline") {
    cat(sprintf("  %-16s slots %s (mean %.1f)\n", a,
                paste(s$slots_vs_baseline[s$arm == a], collapse = " "),
                mean(s$slots_vs_baseline[s$arm == a])))
  }
}

sdw <- function(a) sapply(seasons, function(x) sd(w$gross[w$season == x & w$arm == a]))
pts <- function(a) s$points[s$arm == a][order(s$season[s$arm == a])]
b_sd <- sdw("baseline")
b_pt <- pts("baseline")

cat(sprintf("\n=== 2. THE (mean, SD) CLOUD.  baseline: points %.0f, weekly SD %.2f ===\n",
            mean(b_pt), mean(b_sd)))
cat(sprintf("  %-16s %9s %9s | %9s %9s\n", "arm", "d_points", "thr", "d_SD", "thr"))
res <- list()
for (a in arms) {
  if (a == "baseline") next
  dp <- pts(a) - b_pt
  ds <- sdw(a) - b_sd
  sep <- sd(dp) / sqrt(6)
  ses <- sd(ds) / sqrt(6)
  cat(sprintf("  %-16s %+9.1f %9.1f | %+9.3f %9.3f  %s%s\n", a, mean(dp), tcrit * sep,
              mean(ds), tcrit * ses,
              ifelse(abs(mean(dp)) > tcrit * sep, "PTS-MOVED ", ""),
              ifelse(abs(mean(ds)) > tcrit * ses, "*** SD RESOLVES", "")))
  res[[a]] <- list(dp = mean(dp), ds = mean(ds), evneutral = abs(mean(dp)) <= tcrit * sep)
}

cat("\n=== 3. THE FRONTIER: SD bought per point of expected points given up ===\n")
ev <- names(res)[sapply(res, function(r) r$evneutral)]
cat(sprintf("  arms whose POINTS are indistinguishable from baseline: %d of %d\n", length(ev), length(res)))
if (length(ev) > 0) {
  sds <- sapply(ev, function(a) res[[a]]$ds)
  cat(sprintf("  their d_SD range: %+.3f to %+.3f  (spread %.3f pts of weekly SD)\n",
              min(sds), max(sds), max(sds) - min(sds)))
  cat(sprintf("  baseline SD %.2f, so the spread is %.1f%% of it\n",
              mean(b_sd), 100 * (max(sds) - min(sds)) / mean(b_sd)))
}

cat("\n=== 4. Does a TARGET pick a different arm? P(season pts > T), normal approx ===\n")
cat("  ⚠️ normal approximation declared in the prereg as a convenience, not a probability to quote\n")
allm <- c(baseline = mean(b_pt))
alls <- c(baseline = sd(b_pt))
for (a in names(res)) {
  allm[a] <- mean(pts(a))
  alls[a] <- sd(pts(a))
}
for (q in c(0.50, 0.75, 0.90, 0.95)) {
  T <- quantile(b_pt, q)
  p <- pnorm(T, mean = allm, sd = pmax(alls, 1e-9), lower.tail = FALSE)
  best <- names(which.max(p))
  cat(sprintf("  T=baseline p%02.0f (%4.0f pts): best arm %-16s P=%.3f   (baseline P=%.3f)\n",
              100 * q, T, best, max(p), p["baseline"]))
}
