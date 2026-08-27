#!/usr/bin/env Rscript

# Is the four-arm gate block ADDITIVE? mu_X + mu_R - mu_P, per cell.
#
# Usage:
#   Rscript stats/gate_additivity.R <cells.csv>
#
# # Why this exists as a committed script
#
# `analysis.XPoints` is defined as Points minus XPointsResidual, so the three
# oracle CRITERIA are an exact additive decomposition:
#
#     criterion_P = X + R - 4h      criterion_X = X - 4h      criterion_R = R - 4h
#
# The GAINS are not thereby additive, and the amount by which they are not is a
# quantity the record quotes. It was previously produced by an adaptation that was
# never checked in -- `gate_recovered_fraction.py`'s own docstring says so -- which
# is why a figure of +43.6 a season sat in the record with no reproducible source.
#
# ⚠️ This is NOT a share and must never be converted into one. "N% of the points
# arm is luck" does not follow from a decomposition of the criteria: a gate is a
# threshold on a sum rather than a sum, the arms hold different squads from week
# one, and each component gate charges a full four points for a hit the composite
# charges once. Those three are why the contrast is non-zero at all.
#
# The contrast is formed PER CELL and pushed through cells_common.R's own CR2
# helper. It carries no copy of the estimator -- a diagnostic must never carry its
# own copy of the thing it is checking.

args <- commandArgs(trailingOnly = TRUE)
if (length(args) != 1) {
  stop("usage: gate_additivity.R <cells.csv>", call. = FALSE)
}

source(file.path(dirname(sub("^--file=", "", grep("^--file=", commandArgs(FALSE),
                                                  value = TRUE)[1])),
                 "cells_common.R"))

# ⚠️ The code state, before anything is differenced.
#
# This script forms a CROSS-ARM contrast — mu_X + mu_R - mu_P — and until
# 2026-08-27 it did so without ever asking whether the arms shared a code state.
# That is the gap this call closes, and it is narrower than sweep_inference.R's:
# this script takes exactly ONE file, so the fatal across-files case cannot
# arise here. What can, and what this now reports, is the WITHIN-file mixed
# state — blocks banked at different commits, or with a fingerprinted switch set
# for some and not others, then read together as if they were one sweep.
#
# ⚠️ No --vary opt-out is offered, and the omission is deliberate rather than an
# oversight. The three arms of an additivity check are three CRITERIA over one
# sweep, not three separately-configured runs: there is no fingerprinted switch
# this comparison is entitled to vary, so a flag permitting one would only ever
# be used to silence a real finding. If that ever stops being true, add the flag
# rather than deleting the call.
note("Checking the arms share a code state...")
check_shared_code_state(args[1], character(0))

d <- read_cells(args[1])

# Arms are matched on their CRITERION, not on their index.
#
# ⚠️ This block used to read `want <- c(P = 1, X = 2, R = 3)` and then check only
# that a row EXISTED at each index, under a comment claiming it would "fail loudly
# on a re-ordered sweep". It would not: `c(P = 1, ...)` names the R vector element,
# not the arm. Demonstrated in review by swapping variant_index 1 and 2 in a copy
# of the bank — the script reported +3.5073 pts/gw (+133.3 a season, t 7.10,
# p 0.0009), a confidently significant non-additivity that is pure mislabelling,
# with no complaint. The estimand mu_X + mu_R - mu_P is ASYMMETRIC in P, so a
# P/X swap is not benign.
#
# The identity is in the file already: every row carries `oracle`, and the
# provenance sidecar carries `declared_arm` per index. Match on that.
want <- list(P = "decision:transfergate",
             X = "decision:transfergatexp",
             R = "decision:transfergateres")
idx <- list()
for (nm in names(want)) {
  hit <- unique(d$variant_index[!d$is_baseline & d$oracle == want[[nm]]])
  if (length(hit) != 1) {
    fail("expected exactly one non-baseline arm with oracle '", want[[nm]],
         "' (the ", nm, " criterion), found ", length(hit),
         ". Arms present: ", paste(unique(d$oracle), collapse = ", "))
  }
  idx[[nm]] <- hit
}
if (length(unique(unlist(idx))) != 3) fail("two criteria resolved to one arm")

for (metric in c("policy", "policy_xpoints")) {
  col <- paste0(metric, "_per_gw")
  if (!(col %in% names(d))) fail("no such column: ", col)

  base <- d[d$is_baseline, c("cell", "season", "start_gw", col)]
  names(base)[4] <- "base"

  arm_diff <- function(idx) {
    a <- d[d$variant_index == idx, c("cell", col)]
    names(a)[2] <- "arm"
    j <- merge(base, a, by = "cell")
    j$d <- j$arm - j$base
    j[, c("cell", "season", "start_gw", "d")]
  }

  P <- arm_diff(idx$P); X <- arm_diff(idx$X); R <- arm_diff(idx$R)
  j <- merge(merge(X, R, by = c("cell", "season", "start_gw"),
                   suffixes = c(".X", ".R")),
             P, by = c("cell", "season", "start_gw"))
  names(j)[names(j) == "d"] <- "d.P"
  j <- j[!is.na(j$d.X) & !is.na(j$d.R) & !is.na(j$d.P), ]

  # The estimand: how far the two component gates together over- or under-shoot
  # the composite, per cell.
  j$diff <- j$d.X + j$d.R - j$d.P

  cr <- se_cr2(j)
  crs <- se_cr2_start(j)
  w <- wild_cluster_p_season(j)
  m <- mean(j$diff)

  cat("\n", metric, " — mu_X + mu_R - mu_P, ", nrow(j), " cells\n", sep = "")
  cat(sprintf("  mean %+0.4f pts/gw  (%+0.1f a season)   positive in %d of %d cells\n",
              m, m * 38, sum(j$diff > 0), nrow(j)))
  cat(sprintf("  CR2 season   SE %0.4f  df %0.1f  t %+0.2f  p %0.4f   <- PRIMARY\n",
              cr$se, cr$df, cr$t, cr$p))
  # ⚠️ NOT "start-fixed". This is se_cr2_start -- CR2 clustered on the ENTRY
  # POINT, which cells_common.R calls "a robustness check rather than a rival
  # estimate to prefer on its own". The record's "start-fixed" / `t fixd` is
  # se_fixed in sweep_inference.R, a start-block FIXED-EFFECTS estimator on
  # different df, and it is NOT computed here -- it needs season_share, which
  # that script keeps local on purpose and which must not be copied a third time.
  # Printing this one under the other's name is how an estimator swap reads as a
  # data change, which has now happened three times in this record.
  cat(sprintf("  CR2 entrypt  SE %0.4f  df %0.1f  t %+0.2f  p %0.4f   (robustness only;\n",
              crs$se, crs$df, crs$t, crs$p))
  cat("               NOT the record's `start-fixed`, which is se_fixed and is not computed here)\n")
  cat(sprintf("  wild p %s\n", wcb_label(w)))
  if (!is.na(cr$df)) {
    cat(sprintf("  this contrast's own threshold: %0.1f a season "
                , qt(0.975, cr$df) * cr$se * 38))
    cat(sprintf("(t_crit %0.3f at df %0.1f)\n", qt(0.975, cr$df), cr$df))
  }
}
cat("\n⚠️ Not a share. See the header before quoting any of this.\n")
