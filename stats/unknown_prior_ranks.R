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

d <- do.call(rbind, lapply(args, function(p) read.csv(p, comment.char = "#")))
need <- c("season", "stratum", "n", "scope", "predictor", "rho", "rankable")
miss <- setdiff(need, names(d))
if (length(miss)) stop("missing columns: ", paste(miss, collapse = ", "))
d$rankable <- as.logical(d$rankable)

t_crit <- function(df) qt(0.975, df)

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
contrast <- function(stratum, scope, a, b) {
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
  cat(sprintf("  %s  %+.3f  SE %.3f  t %+6.2f  thr %.3f  %d/%d seasons > 0  %s\n",
              lab, m, se, m / se, t_crit(df) * se,
              sum(diff > 0), length(diff),
              if (abs(m) > t_crit(df) * se) "RESOLVES" else "unresolved"))
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
# Does the fix+tilt beat doing nothing, on the players it was built for?
contrast("NO history", "pooled", "model_on", "model_off")
contrast("NO history", "within_pos", "model_on", "model_off")
# Against the two market signals, on the stratum where they are not an echo.
contrast("NO history", "within_pos", "model_on", "own")
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
  L <- read.csv(lv, comment.char = "#")
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
  cat(sprintf("  %-4s %8s %8s %8s %8s %10s %s\n",
              "pos", "excess", "SE", "thr", "t", "1/excess", "seasons > 1"))
  for (ps in c("GK", "DEF", "MID", "FWD")) {
    u <- L[L$stratum == "NO history" & L$pos == ps, ]
    k <- L[L$stratum == "has history" & L$pos == ps, ]
    key <- intersect(u$season, k$season)
    if (length(key) < 2) next
    e <- u$ratio[match(key, u$season)] / k$ratio[match(key, k$season)]
    m <- mean(e)
    se <- sd(e) / sqrt(length(e))
    cat(sprintf("  %-4s %8.3f %8.3f %8.3f %8.2f %10.3f %d/%d\n",
                ps, m, se, t_crit(length(e) - 1) * se, (m - 1) / se, 1 / m,
                sum(e > 1), length(e)))
  }
  cat("\n⚠️ The t column tests the excess against 1, not against 0.\n")
  cat("⚠️ This sizes a MISCALIBRATION. It does not say a correction pays: this\n")
  cat("project has lost points five times correcting a measured bias, and only\n")
  cat("the replay can settle it.\n")
} else {
  cat("\n(no levels file given -- pass --levels=levels.csv to also read the LEVEL\n")
  cat("half, which every rho above is blind to. Produce it with\n")
  cat("FPL_LEVELS_CSV=/tmp/levels.csv on the same Go run.)\n")
}
