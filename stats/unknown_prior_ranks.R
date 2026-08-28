#!/usr/bin/env Rscript
#
# Does anything rank the players the prior is silent on?
#
# Usage:
#   FPL_RANKS_CSV=/tmp/ranks.csv FPL_LEVELS_CSV=/tmp/levels.csv DIAG=1 \
#     go test ./internal/backtest -run TestDiagOwnershipPredictsMinutes -count=1
#   Rscript stats/unknown_prior_ranks.R --levels=/tmp/levels.csv /tmp/ranks.csv
#
# ---------------------------------------------------------------------------
# The question
#
# A player with no prior-season minutes used to leave `blendRatesCode` at zero
# expected minutes. The fix gives him the position's league rates instead, and a
# price tilt then orders the players inside that fallback. Two claims are on
# trial and they are easy to run together:
#
#   the FIX    every unknown gets a plausible LEVEL rather than zero
#   the TILT   the unknowns are ORDERED within that level, by price
#
# The Go diagnostic prints one Spearman per season per predictor, against the
# minutes actually played in the window the opening squad has to survive. It
# prints no standard error and no verdict word. This script is the inference.
#
# ---------------------------------------------------------------------------
# ⚠️ Why POOLED and WITHIN POSITION are both carried, and why the pair decides
#
# The league fallback is FLAT inside a position and differs BETWEEN positions —
# a goalkeeper's league-average minutes sit far above a rotating forward's. So a
# pooled rank over unknowns is very largely a rank by POSITION, and it can come
# out negative for a reason that says nothing about the ordering the optimiser
# actually consumes: `Optimize` fills a positional quota, so it compares
# goalkeepers with goalkeepers.
#
# That is not a hypothetical correction applied after seeing an unwelcome
# number. It is checkable directly, and the check is the `rankable` column: if
# the fix's predictor is CONSTANT inside every position, no within-position rho
# exists at all, and the entire pooled figure — of either sign — is
# between-position level and nothing else.
#
# ⚠️ **`rankable = FALSE` and `rho = 0` are opposite findings.** "Cannot rank
# these players" is a stronger statement than "ranked them at zero", and a bare
# rho column conflates them. This script refuses to average a non-rankable arm
# and refuses to difference against one.
#
# ---------------------------------------------------------------------------
# Why the season is the cluster, and what that costs
#
# Every player in a season meets the same football, the same promoted clubs and
# the same transfer market, so their rhos are one draw and not hundreds. Six
# seasons is six clusters, df 5, and the two-sided critical value is 2.571
# rather than the familiar 2. The detection threshold reported beside each
# contrast is `t_crit(5) * SE`: a difference smaller than that is unresolved,
# which on this design is the expected reading for a real effect.
#
# ⚠️ This is an ORDERING instrument and it does not price anything in points. A
# better predictor can make a worse policy here — the transfer search is an
# argmax and lives in the tail of the estimate distribution — so an arm that
# wins here has earned replay time and nothing else.

args <- commandArgs(trailingOnly = TRUE)
# ⚠️ The levels file is a FLAG, not a positional argument. The rank files are
# rbind-ed, and a second schema silently joined to the first is exactly the
# mistake this separation exists to prevent.
lv <- sub("^--levels=", "", grep("^--levels=", args, value = TRUE))
args <- grep("^--levels=", args, value = TRUE, invert = TRUE)
if (length(args) < 1) {
  stop("usage: unknown_prior_ranks.R [--levels=levels.csv] ranks.csv [more.csv ...]")
}

# Neither file is a cells file, so both read through the shared SIDECAR reader
# rather than a raw read.csv. That reader is the sanctioned home — a raw read
# here trips the one-implementation guard, and the guard is right: reading the
# file is where this family's divergences lived.
local({
  a <- commandArgs(trailingOnly = FALSE)
  f <- sub("^--file=", "", a[grep("^--file=", a)])
  d <- if (length(f) > 0) dirname(normalizePath(f[1])) else "stats"
  p <- file.path(d, "cells_common.R")
  if (!file.exists(p)) {
    stop("cannot find cells_common.R beside this script (looked in ", d, ")")
  }
  source(p, local = FALSE)
})

d <- do.call(rbind, lapply(args, read_sidecar))
need <- c("season", "stratum", "n", "scope", "predictor", "rho", "rankable")
miss <- setdiff(need, names(d))
if (length(miss)) stop("missing columns: ", paste(miss, collapse = ", "))
d$rankable <- as.logical(d$rankable)

t_crit <- function(df) qt(0.975, df)

# ---------------------------------------------------------------------------
# ⚠️ Every test this script reports lands in ONE family, and the family is
# Holm-corrected at the end.
#
# The first version printed a bare "RESOLVES" per contrast at the nominal 0.05.
# It reported five of them from a single run — one ordering contrast and four
# position excesses — and the closest cleared its own threshold by 3.8% with one
# of six seasons pointing the other way. Five nominal passes at df 5 is exactly
# the situation `AGENTS.md` names Holm for, and an uncorrected pass read as
# established is how an argmax over contrasts gets mistaken for a finding.
#
# ⚠️ **A test enters the family once.** Two rows that are algebraically the same
# comparison must not both be counted: doing so both inflates the family and
# double-reports the evidence. `model_on - own` and `price - own` are one test,
# because within a position the tilt induces price's own ordering exactly.
FAMILY <- list()
record <- function(label, mean, se, df, alt_of = 1) {
  FAMILY[[length(FAMILY) + 1]] <<- list(
    label = label, mean = mean, se = se, df = df,
    p = 2 * pt(-abs((mean - alt_of) / se), df))
}

# level reports one arm on its own: is this predictor's mean rho distinguishable
# from no ordering at all?
level <- function(stratum, scope, predictor) {
  rows <- d[d$stratum == stratum & d$scope == scope & d$predictor == predictor, ]
  if (!nrow(rows)) return(invisible(NULL))
  lab <- sprintf("%-11s %-11s %-9s", stratum, scope, predictor)
  if (!all(rows$rankable)) {
    # ⚠️ Not averaged, and not reported as zero. A predictor constant inside
    # every position has no ordering to estimate, and printing 0.000 here is
    # exactly the confusion this script exists to prevent.
    cat(sprintf("  %s  NOT RANKABLE in %d of %d seasons -- constant within every\n",
                lab, sum(!rows$rankable), nrow(rows)))
    cat("                                        position, so no ordering exists to test\n")
    return(invisible(NULL))
  }
  m <- mean(rows$rho)
  se <- sd(rows$rho) / sqrt(nrow(rows))
  df <- nrow(rows) - 1
  cat(sprintf("  %s  rho %+.3f  SE %.3f  t %+6.2f  thr %.3f  %d/%d seasons > 0\n",
              lab, m, se, m / se, t_crit(df) * se, sum(rows$rho > 0), nrow(rows)))
}

# contrast pairs two predictors WITHIN each season and differences them there,
# so the season's own difficulty — how many unknowns, how settled the market —
# cancels before anything is averaged.
contrast <- function(stratum, scope, a, b, family = TRUE) {
  ra <- d[d$stratum == stratum & d$scope == scope & d$predictor == a, ]
  rb <- d[d$stratum == stratum & d$scope == scope & d$predictor == b, ]
  key <- intersect(ra$season, rb$season)
  ra <- ra[match(key, ra$season), ]
  rb <- rb[match(key, rb$season), ]
  lab <- sprintf("%-11s %-11s %-9s - %-9s", stratum, scope, a, b)
  if (!all(ra$rankable) || !all(rb$rankable)) {
    cat(sprintf("  %s  REFUSED: an arm is not rankable in every season\n", lab))
    return(invisible(NULL))
  }
  diff <- ra$rho - rb$rho
  m <- mean(diff)
  se <- sd(diff) / sqrt(length(diff))
  df <- length(diff) - 1
  if (se == 0) {
    # ⚠️ An exact zero in every season is an INVARIANCE, not a null. It means
    # the two predictors induce the same ordering by construction, and a t of
    # 0/0 would be a division by nothing.
    cat(sprintf("  %s  IDENTICAL in all %d seasons -- same ordering by construction\n",
                lab, length(diff)))
    return(invisible(NULL))
  }
  if (family) record(sprintf("%s %s: %s - %s", stratum, scope, a, b), m, se, df, 0)
  cat(sprintf("  %s  %+.3f  SE %.3f  t %+6.2f  thr %.3f  %d/%d seasons > 0  %s\n",
              lab, m, se, m / se, t_crit(df) * se,
              sum(diff > 0), length(diff),
              if (abs(m) > t_crit(df) * se) "nominal pass" else "unresolved"))
}

cat("\nSpearman of each predictor against minutes played, one rho per season.\n")
cat("Season-clustered; df", length(unique(d$season)) - 1,
    "and t_crit", sprintf("%.3f", t_crit(length(unique(d$season)) - 1)), "\n\n")

cat("LEVELS -- can this predictor rank these players at all?\n")
for (st in c("NO history", "has history")) {
  for (sc in c("pooled", "within_pos")) {
    for (pd in c("own", "price", "model_off", "model_on")) level(st, sc, pd)
  }
}

cat("\nCONTRASTS -- paired within season, so the season's own difficulty cancels.\n")
cat("⚠️ 'nominal pass' is uncorrected. The family verdict is at the bottom.\n")
# Does the fix+tilt beat doing nothing, on the players it was built for?
contrast("NO history", "pooled", "model_on", "model_off")
contrast("NO history", "within_pos", "model_on", "model_off")
# Against the two market signals, on the stratum where they are not an echo.
#
# ⚠️ `model_on - own` and `price - own` are ONE test, not two. Within a position
# the tilt is a strictly monotone function of price percentile applied to a
# constant base, so it induces price's ordering exactly — the two rows are
# algebraically identical, which the run confirms to the last digit. The first
# is printed for continuity with the table above and kept OUT of the family;
# counting it twice would both inflate the correction and double-report one
# piece of evidence.
contrast("NO history", "within_pos", "model_on", "own", family = FALSE)
contrast("NO history", "within_pos", "model_on", "price")
contrast("NO history", "within_pos", "price", "own")
# ⚠️ The confinement check. The tilt must not touch a player the prior can
# already describe, and this is where that claim is tested rather than asserted.
contrast("has history", "pooled", "model_on", "model_off")
contrast("has history", "within_pos", "model_on", "model_off")

cat("\n⚠️ Ordering only. Nothing here prices a change in points, and a better\n")
cat("predictor can make a worse policy: the transfer search is an argmax.\n")
cat("⚠️ A contrast against a NOT RANKABLE arm is refused rather than scored.\n")

# ---------------------------------------------------------------------------
# The LEVEL, which no rho above can see
#
# Spearman is invariant to any monotone rescaling, so every figure above would
# be unchanged if the fallback handed each unknown twice the minutes he plays.
# The level is the fix's other half and it has its own failure mode: an unknown
# goalkeeper is usually a BACKUP, because a first-choice keeper generally has
# history — so the position most likely to be over-stated is the one whose
# league average is highest.
#
# ⚠️ Reported as a RATIO of predicted to actual per-match minutes. Predicted is
# per match; actual is a window total, so it is divided by the window before the
# two are compared. A ratio above 1 is over-statement.
if (length(lv) == 1 && file.exists(lv)) {
  L <- read_sidecar(lv)
  L$ratio <- L$pred_off / (L$actual / L$window)
  cat("\nLEVEL -- predicted per-match minutes over actual, by position.\n")
  cat("Above 1 is over-statement. The tilt does not enter here: it reorders\n")
  cat("within a position and the position's MEAN is what this reads.\n\n")
  cat(sprintf("  %-11s %-4s %6s %9s %8s %8s %s\n",
              "stratum", "pos", "n/szn", "ratio", "SE", "thr", "seasons > 1"))
  for (st in unique(L$stratum)) {
    for (ps in c("GK", "DEF", "MID", "FWD")) {
      rows <- L[L$stratum == st & L$pos == ps, ]
      if (nrow(rows) < 2) next
      m <- mean(rows$ratio)
      se <- sd(rows$ratio) / sqrt(nrow(rows))
      cat(sprintf("  %-11s %-4s %6.0f %9.3f %8.3f %8.3f %d/%d\n",
                  st, ps, mean(rows$n), m, se, t_crit(nrow(rows) - 1) * se,
                  sum(rows$ratio > 1), nrow(rows)))
    }
  }
  cat("\n⚠️ A ratio far above 1 on the NO-history stratum is the fix's own risk:\n")
  cat("it says the fallback level flatters players nobody has seen, which is a\n")
  cat("LEVEL defect the ordering figures above are blind to by construction.\n")

  # ⚠️ **The absolute ratio above is not the number to act on.** It depends on
  # what `ExpectedMinutes` is a rate OF and on how blank gameweeks land in the
  # window average, and both conventions cancel in the EXCESS below: the
  # unknown stratum's ratio over the KNOWN stratum's, in the same position and
  # the same season, measured the same way. The known stratum is the control
  # that says the instrument reads roughly 1 when the model is calibrated.
  #
  # The reciprocal is printed beside it because that is the quantity the shipped
  # lever expresses: `Weights.UnknownPriorShare` scales the league fallback, so
  # a stratum reading 1.6x too high implies a share near 1/1.6.
  cat("\nEXCESS -- the unknown stratum's ratio over the KNOWN stratum's, paired\n")
  cat("within season and position. Conventions cancel; 1.000 is calibrated.\n\n")
  cat(sprintf("  %-4s %8s %8s %8s %8s %10s %8s %8s %s\n",
              "pos", "excess", "SE", "thr", "t", "1/excess",
              "min szn", "max szn", "seasons > 1"))
  for (ps in c("GK", "DEF", "MID", "FWD")) {
    u <- L[L$stratum == "NO history" & L$pos == ps, ]
    k <- L[L$stratum == "has history" & L$pos == ps, ]
    key <- intersect(u$season, k$season)
    if (length(key) < 2) next
    e <- u$ratio[match(key, u$season)] / k$ratio[match(key, k$season)]
    m <- mean(e)
    se <- sd(e) / sqrt(length(e))
    record(sprintf("level excess %s", ps), m, se, length(e) - 1, 1)
    cat(sprintf("  %-4s %8.3f %8.3f %8.3f %8.2f %10.3f %8.2f %8.2f %d/%d\n",
                ps, m, se, t_crit(length(e) - 1) * se, (m - 1) / se, 1 / m,
                min(e), max(e), sum(e > 1), length(e)))
    # ⚠️ Every season printed, in season order, because the summary above hides
    # a shape. These series RISE across the six seasons in all four positions,
    # which a mean and an SE describe as noise. It is printed rather than
    # tested: an unpredicted trend found by looking is an argmax over shapes,
    # and adding it to the family would spend the correction on a fishing trip.
    # What it does mean is that a single flat constant fitted to the six-season
    # mean would under-correct the most recent seasons — the ones that matter.
    cat(sprintf("       by season: %s\n",
                paste(sprintf("%s %.2f", substr(key, 3, 7),
                              e), collapse = "  ")))
  }
  cat("\n⚠️ The t column tests the excess against 1, not against 0.\n")
  cat("⚠️ **Read the min and max season columns before quoting any excess as a\n")
  cat("LOCATION.** This is a ratio of means and its denominator is small where the\n")
  cat("unknowns barely play, which is leverage rather than symmetric noise -- one\n")
  cat("season can carry the mean. Where min and max span a factor of several, the\n")
  cat("DIRECTION is what six seasons agree on and the magnitude is not.\n")
  cat("⚠️ The known stratum reads about 1.1, not 1.0 -- a GW1 forecast loses to a\n")
  cat("ten-week window through injury and rotation. It is near-uniform across the\n")
  cat("four positions, which is why it cancels in the ratio; that near-uniformity\n")
  cat("is the assumption, and it is visible in the LEVEL table above.\n")
  cat("⚠️ 1/excess is the implied share only for a LINEAR pipeline. The fallback\n")
  cat("passes through clamps and a power-mean before it reaches ExpectedMinutes,\n")
  cat("so treat the reciprocal as a starting point for a sweep, not a setting.\n")
  cat("⚠️ This sizes a MISCALIBRATION. It does not say a correction pays: this\n")
  cat("project has lost points five times correcting a measured bias, and only\n")
  cat("the replay can settle it.\n")
} else {
  cat("\n(no levels file given -- pass --levels=levels.csv to also read the LEVEL\n")
  cat("half, which every rho above is blind to. Produce it with\n")
  cat("FPL_LEVELS_CSV=/tmp/levels.csv on the same Go run.)\n")
}

# ---------------------------------------------------------------------------
# THE FAMILY VERDICT
#
# Everything above prints "nominal pass" at the uncorrected 0.05. This is where
# a verdict is actually reached, because the run offers several tests at once
# and keeping whichever clears is an argmax over contrasts — the failure this
# record names as its most load-bearing idea.
#
# Holm is step-down: sort the p-values ascending and compare the i-th against
# alpha/(k-i+1), stopping at the first failure. It controls the chance of ANY
# false positive in the family, assumes nothing about independence between the
# tests, and is uniformly more powerful than Bonferroni.
#
# ⚠️ The family is what this script reports in one run. Running it twice on
# different populations and quoting the better outcome puts the argmax back.
if (length(FAMILY)) {
  ps <- sapply(FAMILY, function(f) f$p)
  ord <- order(ps)
  k <- length(ps)
  holm <- rep(NA_real_, k)
  running <- 0
  for (i in seq_len(k)) {
    # The step-down running maximum is what makes the adjusted p-values
    # monotone; without it a later test can appear to beat an earlier one.
    running <- max(running, min(1, ps[ord[i]] * (k - i + 1)))
    holm[ord[i]] <- running
  }
  cat("\n\nFAMILY VERDICT -- Holm across all", k, "tests reported above.\n")
  cat("This supersedes every 'nominal pass' printed earlier.\n\n")
  cat(sprintf("  %-44s %9s %9s  %s\n", "test", "p", "Holm p", "verdict"))
  for (i in ord) {
    f <- FAMILY[[i]]
    cat(sprintf("  %-44s %9.5f %9.5f  %s\n", f$label, f$p, holm[i],
                if (holm[i] < 0.05) "RESOLVES" else "unresolved"))
  }
  cat("\nA test that survives Holm is still an ORDERING or CALIBRATION result and\n")
  cat("prices nothing. One that does not survive is unresolved, which on six\n")
  cat("clusters is the expected reading for a real effect.\n")
}
