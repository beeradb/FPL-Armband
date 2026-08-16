#!/usr/bin/env Rscript

# Would scoring the replay on RANK rather than on points change any verdict?
#
# Usage:
#   Rscript stats/rank_robustness.R /tmp/cells.csv [more.csv ...]
#
# ---------------------------------------------------------------------------
# The question, and why it needs no field model
#
# "Rank" here means our percentile against the field of ~11M other managers.
# The tempting objection to every sweep in this project is that it maximises
# points while the game pays rank, so the verdicts might be measuring the
# wrong thing.
#
# The field does not react to our policy. So both arms of a paired comparison
# — same season, same entry gameweek, one setting changed — meet the *same*
# field distribution F. Percentile is F(x), and F is monotone increasing.
# Therefore
#
#     sign(percentile_A - percentile_B) = sign(x_A - x_B)
#
# exactly, in every cell. Re-scoring on rank cannot flip a single cell.
#
# It CAN flip a verdict, though, because a verdict is the *mean* over cells and
# the cells disagree. Percentile difference is approximately f_i * d_i, where
# f_i is the field's density at our score in cell i, so rank-scoring is exactly
# a reweighting of the cells by f_i > 0. If the cells that favour an arm get
# down-weighted, the mean can change sign even though no cell did.
#
# This script asks how fragile each arm's mean is to that reweighting, without
# ever having to model the field. Two statistics, and the second is the useful
# one:
#
#   R_crit    the ADVERSARIAL bound. The smallest weight ratio max(w)/min(w)
#             that could flip the sign, if the weights were chosen by an
#             opponent to conspire with the cell-level sign pattern. Closed
#             form: with P the sum of positive differences and N the absolute
#             sum of negative ones, R_crit = P/N (inverted for a negative
#             mean). Small R_crit means the mean is a knife-edge between
#             cells that disagree.
#
#   P(flip)   the RANDOM-REWEIGHTING figure. Weights are drawn log-uniform on
#             [1, R] and we report the fraction of draws that flip the sign.
#
#             ⚠️ NOT the realistic figure, which is what this header called it
#             until review. Two corrections, both in the harness note's scope
#             block. Weights COMPUTED from ownership marginals run 17x to
#             2,343x, one to three orders of magnitude past R = 5. And the
#             claim that "there is no mechanism by which either correlates
#             with which arm won a given cell" is measurably false: a cell's
#             paired difference correlates with our own baseline score at
#             −0.15 to −0.70, the SAME SIGN in all eleven arms. Any weighting
#             that falls as our score rises shrinks these arms toward zero.
#
#             Note also that under data-independent weights P(flip) has the
#             closed form Phi(−|t|/s_e), so a Spearman between it and |t| is
#             an algebraic identity, not a measurement. That mistake reached
#             the record once.
#
# R_crit alone badly overstates the risk, which is why both are printed. The
# one component we can bound WITHOUT a field model — cell length, since a cell
# entered at GW26 banks 13 gameweeks against 38 for GW1, and a field total's
# spread scales as sqrt(weeks) — flips nothing at all. "Exactly" would be too
# strong: sqrt() assumes independent gameweeks, and the note brackets the real
# exponent at p >= 0.62 because a squad persists all season. The conclusion is
# unaffected — no arm changes sign anywhere in the family weeks^(1-p).
#
# What a plausible R is: sqrt(38/13) = 1.71 from cell length alone, times
# perhaps 1.6-3 for variation in our standing against the field, so roughly
# 3 to 5. R = 5 is therefore the pessimistic column.

suppressWarnings(suppressMessages({ ok <- require(stats, quietly = TRUE) }))

args <- commandArgs(trailingOnly = TRUE)
if (length(args) == 0) stop("usage: rank_robustness.R cells.csv [more.csv ...]")

# `read_cells` and the contract checks are in cells_common.R. This script read the
# file itself and tested `is_baseline == "true"`, which is correct only while R
# types that column as character — see the note on `as_flag` there.
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

set.seed(42)
B <- 20000

# rcrit is the adversarial weight ratio that flips the sign of mean(dd).
# Weights live in [1, R]; the sign of a weighted mean is the sign of the
# weighted sum, which is minimised by putting R on every negative difference
# and 1 on every positive one. Solving R*sum(neg) + sum(pos) <= 0 gives P/N.
rcrit <- function(dd) {
  P <- sum(dd[dd > 0]); N <- -sum(dd[dd < 0])
  if (N == 0 || P == 0) return(Inf)
  r <- P / N
  if (mean(dd) < 0) r <- 1 / r
  r
}

# flipprob draws B random reweightings, log-uniform on [1, R], and reports how
# often the sign of the mean changes. Log-uniform rather than uniform because a
# density ratio is a multiplicative quantity.
flipprob <- function(dd, R) {
  n <- length(dd)
  w <- matrix(exp(runif(n * B, 0, log(R))), nrow = B, byrow = TRUE)
  mean(sign((w %*% dd) / rowSums(w)) != sign(mean(dd)))
}

metrics <- c(policy = "policy_per_gw", hold = "hold_per_gw")

for (path in args) {
  d <- read_cells(path)

  # Block on (run_id, sweep) and key cells by run as well, exactly as every other
  # script in this directory does — sweep_inference.R:248, shape_inference.R:572,
  # variance_components.R:527, entry_density.R:501 — and as internal/snapshot/cells.go
  # documents ("keyed on (sweep label, run_id) exactly as R keys a comparison").
  #
  # This is not defensive tidiness. The cells file is opened for **append**, so one
  # file can legitimately hold two runs of the same sweep label. Blocking on the
  # label alone silently pools them: each arm reads twice the rows, n doubles, the
  # standard error shrinks by sqrt(2) and every t inflates by the same factor — with
  # cells paired against the wrong run's baseline, since bm[a$cell] takes the first
  # match. No error and no warning, just a plausible table that is over-confident by
  # 41%. That is the exact failure internal/backtest/cellcsv_regression_test.go was
  # written to prevent, and this script was the one consumer not honouring it.
  # `block` and `cell` come from read_cells, which keys the cell on the sweep as
  # well as the run — this script keyed on the run alone, which was correct only
  # because it blocked before every use.

  for (sw in unique(d$block)) {
    ds <- subset(d, block == sw)
    base <- subset(ds, is_baseline)
    if (nrow(base) == 0) next

    for (mn in names(metrics)) {
      mcol <- metrics[[mn]]
      if (is.null(ds[[mcol]])) next
      bm <- setNames(base[[mcol]], base$cell)

      rows <- list()
      for (v in setdiff(unique(ds$variant), base$variant[1])) {
        a <- subset(ds, variant == v)
        dd <- a[[mcol]] - bm[a$cell]
        # A byte-identical arm is an invariance check passing, not a
        # measurement. Say so rather than dividing by zero.
        if (all(abs(dd) < 1e-12)) next
        se <- sd(dd) / sqrt(length(dd))
        rows[[length(rows) + 1]] <- data.frame(
          variant = v, mean = mean(dd), t = if (se > 0) mean(dd) / se else NA,
          rcrit = rcrit(dd),
          sqrtwk = sum(sqrt(a$weeks) * dd) / sum(sqrt(a$weeks)),
          f2 = 100 * flipprob(dd, 2), f5 = 100 * flipprob(dd, 5),
          stringsAsFactors = FALSE)
      }
      if (length(rows) == 0) {
        cat(sprintf("\n%s / %s — every arm byte-identical; nothing to reweight.\n",
                    sw, toupper(mn)))
        next
      }
      r <- do.call(rbind, rows)

      cat(sprintf("\n%s / %s   (mean is pts/gw; x38 for a season)\n", sw, toupper(mn)))
      cat("variant                    mean  t nai  R_crit  sqrt-wk | P(flip) R=2   R=5\n")
      for (i in seq_len(nrow(r))) {
        cat(sprintf("%-22s %+7.3f %6.2f  %6.2f  %+7.3f |  %6.1f%% %6.1f%%%s\n",
            substr(r$variant[i], 1, 22), r$mean[i], r$t[i], r$rcrit[i], r$sqrtwk[i],
            r$f2[i], r$f5[i],
            if (sign(r$sqrtwk[i]) != sign(r$mean[i])) "  ** sqrt-wk FLIPS **" else ""))
      }
    }
  }
}

cat("\nReading this: an arm with P(flip) at 0.0% cannot have its direction",
    "\nchanged by any RANDOM reweighting drawn to R. That is NOT the same as",
    "\nany realistic one: weights COMPUTED from ownership marginals run 17x to",
    "\n2,343x, one to three orders of magnitude past R = 5. The verdict does",
    "\nsurvive computing them, but on different evidence and with a different",
    "\narm ordering — read the SCOPE block under 'A rank metric reorders only",
    "\nwhat was already unresolved' in the harness-and-inference note",
    "\nbefore quoting the 0.0%.",
    "\n",
    "\nNote also that P(flip) is Monte Carlo (20,000 draws, ~0.1pp of wobble)",
    "\nand this script seeds ONCE and shares one stream across every file and",
    "\narm, so the last printed digit depends on which files you pass and in",
    "\nwhat order. The 0.0% cells and the ordering are stable; the tenths are",
    "\nnot. And do NOT cite the |t| correlation as evidence: for weights drawn",
    "\nindependently of the data, P(flip) has a closed form in |t| alone, so",
    "\nthat correlation is an identity this simulation recovers.\n")
