#!/usr/bin/env Rscript
#
# Does the xGC proration's player-season cancellation survive the input change?
#
# Usage:
#
#   mkdir -p stats/out && cp stats/cells/xgc-transport/transport-*.csv stats/out/
#   DIAG=1 FPL_XGC_TERCILE_CSV=/tmp/xgcterc \
#     go test ./internal/backtest -run TestDiagXGCTransport -v -count=1
#   Rscript stats/xgc_tercile_transport.R /tmp/xgcterc
#
# ---------------------------------------------------------------------------
# The question
#
# `xgcrepair.go` records that the minutes-proration's two errors -- over-crediting a
# withdrawn starter, under-crediting a late substitute -- "largely cancel at player-season
# level", measured as season XGC90 reconstructed over actual running **0.983-1.014 across
# substitution terciles**, within +/-1.7%. That figure is FPL-fed. The chain only ever runs
# on Understat-fed seasons, and the transport run measured dispersion and ordering rather
# than the tercile structure, so the claim covers a population its evidence does not reach.
#
# `scoreXGCArm` now emits both arms' per-player rows with the tercile assignment already
# made, so the CUT has one implementation and lives in Go. This script does the inference
# only: Go prints no standard error, no t and no verdict word.
#
# ---------------------------------------------------------------------------
# Why the raw ratio is the wrong thing to read, and what replaces it
#
# The Understat arm carries a whole-season LEVEL that has nothing to do with terciles: the
# leave-one-out borrowed provider offset misses each season's own in-season ratio by
# +7.7 / -0.2 / -4.3 / -2.1 percent, and reconstructed xGC is linear in that offset. So the
# UST arm's raw tercile ratios sit near 0.93 in 2022-23 and near 1.05 in 2024-25, and none
# of that is a cancellation failure. The record's own instruction on this run is to read
# arm B **over arm A**, not against 1.0.
#
# The structural statistic is therefore the tercile ratio divided by the same season-arm's
# whole-population ratio -- call it RECENTRED. Cancellation holding means the recentred
# ratios sit flat across exposure; failing means the high-exposure bucket departs.
# Both are reported, raw first, because the mandate asks for the recorded figure's own
# form beside the transported one.
#
# ---------------------------------------------------------------------------
# Estimators, named, because this record has twice mistaken an estimator swap for a
# data change
#
#   RATIO OF TOTALS   sum(rec90) / sum(act90) over the bucket's players. Every player
#                     weighted equally in RATE space -- not minutes-weighted, which would
#                     be a third quantity again. This is the headline.
#   MEAN OF RATIOS    mean(rec90 / act90). Biased upward when the denominator is small and
#                     variable. Reported so the recorded 0.983-1.014, which does not say
#                     which it was, can be read against either.
#
# Two uncertainties, because they answer different questions:
#
#   CLUB-CLUSTERED BOOTSTRAP, WITHIN SEASON. Pairs bootstrap resampling CLUBS with
#     replacement inside each (season, arm, tercile), 2000 draws, fixed seed. Teammates
#     share every club-match error in this chain -- the chain is linear in the opponent
#     club's xG, so a club-match error is common to all of that club's players -- which is
#     why the player is not the independent unit. This SE describes ONE season's bucket
#     and does not include season-to-season heterogeneity.
#   SEASON-CLUSTERED. The season is the cluster, as everywhere else in this record.
#     Used for every pooled figure and every contrast, with t_crit(S-1).
#
# ---------------------------------------------------------------------------
# The detection threshold
#
# Every contrast prints `thresh`, which is t_crit(df) * SE on the contrast's own scale.
# That is the smallest |effect| this comparison could call non-zero at p = 0.05, and a
# result inside it is not a result. With four seasons df is 3 and t_crit is 3.182; with
# three it is 2 and 4.303. Both are printed, because 2022-23 is scored on GW16-38 only.

suppressWarnings(suppressMessages({
  args <- commandArgs(trailingOnly = TRUE)
}))
if (length(args) < 1) stop("usage: xgc_tercile_transport.R <dir written by FPL_XGC_TERCILE_CSV>")

dir <- args[1]
files <- list.files(dir, pattern = "^xgc-tercile-[0-9-]+\\.csv$", full.names = TRUE)
if (length(files) == 0) stop(sprintf("no xgc-tercile-<season>.csv under %s", dir))
d <- do.call(rbind, lapply(files, read.csv, stringsAsFactors = FALSE))

# The bucket ratios Go computed, so the estimator this script re-forms can be checked
# against the one the diagnostic printed rather than eyeballed across two console files.
# R has to re-form it to bootstrap it, so the second implementation is unavoidable; two
# implementations disagreeing SILENTLY is what is avoidable.
bfiles <- list.files(dir, pattern = "^xgc-tercile-.*-buckets\\.csv$", full.names = TRUE)
if (length(bfiles) != length(files)) {
  stop(sprintf("%d row files but %d bucket sidecars under %s -- regenerate both",
               length(files), length(bfiles), dir))
}
gob <- do.call(rbind, lapply(bfiles, read.csv, stringsAsFactors = FALSE))

stopifnot(all(d$tercile %in% 0:2), all(d$act90 > 0), all(d$minutes >= 900))
labs <- c("low", "mid", "high")
d$lab <- factor(labs[d$tercile + 1], levels = labs)
seasons <- sort(unique(d$season))

cat(sprintf("rows %d, seasons %s\n", nrow(d), paste(seasons, collapse = " ")))

# LIVENESS. Two of them, and the second is the one this contrast actually needs.
#
# The first is raw movement: did the transported reconstruction reach the chain at all.
# It is necessary and it is nowhere near sufficient, because **a pure per-season SCALAR
# rescale of arm A would pass it at 100% while forcing every recentred contrast below to
# be exactly zero by construction** -- the recentring divides by the same season-arm's
# whole-population ratio, and a scalar cancels. So a guard on raw movement cannot tell a
# real null from a degenerate one, which is this record's signature failure wearing the
# clothes of a confirmation.
#
# The second is the guard with power: the WITHIN-SEASON dispersion of the per-player arm
# ratio. Under a pure rescale it is 0 by construction. Anything materially above 0 says
# the two arms disagree player by player, which is the only condition under which the
# tercile agreement below is evidence of anything.
moved <- abs(d$rec90_ust - d$rec90_fpl) > 1e-9 * abs(d$rec90_fpl)
cat(sprintf("liveness 1, raw movement: %d of %d player-seasons move = %.4f\n",
            sum(moved), nrow(d), mean(moved)))
big <- abs(d$rec90_ust - d$rec90_fpl) > 0.01 * abs(d$rec90_fpl)
cat(sprintf("                          %d of %d move by more than 1%% = %.4f\n",
            sum(big), nrow(d), mean(big)))
if (mean(moved) < 0.90) stop("the arms are the same quantity; nothing below measures anything")

d$armratio <- d$rec90_ust / d$rec90_fpl
cv <- sapply(seasons, function(s) {
  x <- d$armratio[d$season == s]; sd(x) / mean(x)
})
cat("liveness 2, NOT a scalar rescale -- within-season CV of rec90_ust/rec90_fpl:\n")
cat(sprintf("                          %s\n",
            paste(sprintf("%s %.4f", seasons, cv), collapse = "   ")))
if (max(cv) < 0.005) {
  stop("arm B is within half a percent of a scalar multiple of arm A. The recentred ",
       "contrasts below are then zero BY CONSTRUCTION and measure nothing")
}

# The recentring's own constraint, checked rather than assumed. The whole-population
# ratio is the act90-weighted combination of the three bucket ratios, so the three
# recentred values satisfy an act90-weighted mean of exactly 1. That makes the three
# buckets TWO degrees of freedom, not three: Holm over six rows in Table 3 is
# conservative rather than wrong, and the single-df summary to read is the signed
# high-low. Verified numerically below so nobody has to take it on trust.
zs <- sapply(seasons, function(s) sapply(c("FPL", "UST"), function(arm) {
  ds <- d[d$season == s, ]
  rec <- if (arm == "FPL") ds$rec90_fpl else ds$rec90_ust
  whole <- sum(rec) / sum(ds$act90)
  w <- tapply(ds$act90, ds$lab, sum); w <- w / sum(w)
  r <- tapply(rec, ds$lab, sum) / tapply(ds$act90, ds$lab, sum) / whole
  sum(w * r) - 1
}))
cat(sprintf("recentring identity: max |act90-weighted mean of recentred - 1| = %.2e\n",
            max(abs(zs))))
stopifnot(max(abs(zs)) < 1e-9)

ratio_of_totals <- function(rec, act) sum(rec) / sum(act)
mean_of_ratios  <- function(rec, act) mean(rec / act)

# Club-clustered pairs bootstrap for a ratio of totals.
boot_se <- function(rec, act, club, B = 2000, seed = 20260816) {
  cl <- split(seq_along(rec), club)
  if (length(cl) < 2) return(NA_real_)
  set.seed(seed)
  reps <- replicate(B, {
    take <- unlist(cl[sample.int(length(cl), length(cl), replace = TRUE)], use.names = FALSE)
    sum(rec[take]) / sum(act[take])
  })
  sd(reps)
}

# ---------------------------------------------------------------------------
# Table 1: raw tercile ratios, both arms, per season. Reproduces the Go log exactly --
# that equality is the check that this script and the diagnostic have not drifted.
cat("\n=== Table 1. Raw tercile ratios (reproduces the Go log)\n")
cat(sprintf("%-9s %-8s %-5s %4s  %-15s %-15s %8s\n",
            "season", "arm", "terc", "n", "ratio-of-totals", "mean-of-ratios", "boot SE"))
cells <- list()
for (s in seasons) {
  for (arm in c("FPL", "UST")) {
    ds <- d[d$season == s, ]
    rec_all <- if (arm == "FPL") ds$rec90_fpl else ds$rec90_ust
    whole <- ratio_of_totals(rec_all, ds$act90)
    for (b in labs) {
      k <- ds$lab == b
      rec <- rec_all[k]; act <- ds$act90[k]
      rt <- ratio_of_totals(rec, act)
      mr <- mean_of_ratios(rec, act)
      se <- boot_se(rec, act, ds$club[k])
      cat(sprintf("%-9s %-8s %-5s %4d  %15.4f %15.4f %8.4f\n",
                  s, arm, b, sum(k), rt, mr, se))
      cells[[length(cells) + 1]] <- data.frame(
        season = s, arm = arm, lab = b, n = sum(k),
        ratio = rt, meanratio = mr, se = se,
        whole = whole, recentred = rt / whole, stringsAsFactors = FALSE)
    }
  }
}
cells <- do.call(rbind, cells)

# The equality check the comment above used to assert and nothing performed.
gob$lab <- factor(labs[gob$tercile + 1], levels = labs)
m <- merge(cells, gob, by = c("season", "arm", "lab"), suffixes = c("", ".go"))
if (nrow(m) != nrow(cells)) {
  stop(sprintf("%d bucket cells here against %d in Go's sidecars -- the two are not the ",
               nrow(cells), nrow(m)), "same population")
}
dmax <- max(abs(m$ratio - m$ratio.go), abs(m$meanratio - m$mean_ratio), abs(m$n - m$n.go))
cat(sprintf("Go/R estimator agreement: max |difference| over %d bucket cells = %.2e\n",
            nrow(m), dmax))
if (dmax > 1e-8) {
  stop("this script and scoreXGCArm disagree about the bucket estimators. One quantity, ",
       "two implementations -- fix before reading anything below")
}

cat("\nwhole-population ratio per season-arm (the level the recentring removes):\n")
w <- unique(cells[, c("season", "arm", "whole")])
for (i in seq_len(nrow(w))) cat(sprintf("  %-9s %-4s %.4f\n", w$season[i], w$arm[i], w$whole[i]))

# ---------------------------------------------------------------------------
# Table 2: recentred ratios -- the structural statistic.
cat("\n=== Table 2. Recentred tercile ratios (tercile ratio / this season-arm's whole-population ratio)\n")
cat(sprintf("%-9s %-8s %8s %8s %8s   %10s\n", "season", "arm", "low", "mid", "high", "high-low"))
hl <- list()
for (s in seasons) for (arm in c("FPL", "UST")) {
  r <- cells[cells$season == s & cells$arm == arm, ]
  v <- setNames(r$recentred, r$lab)
  cat(sprintf("%-9s %-8s %8.4f %8.4f %8.4f   %+10.4f\n",
              s, arm, v["low"], v["mid"], v["high"], v["high"] - v["low"]))
  hl[[length(hl) + 1]] <- data.frame(season = s, arm = arm,
                                     hl = unname(v["high"] - v["low"]),
                                     spread = max(v) - min(v), stringsAsFactors = FALSE)
}
hl <- do.call(rbind, hl)

# ---------------------------------------------------------------------------
# Season-clustered summaries and contrasts.
clustered <- function(x, label, scale = 1) {
  S <- length(x); df <- S - 1
  m <- mean(x); se <- sd(x) / sqrt(S)
  tc <- qt(0.975, df)
  # MDE: the smallest TRUE effect this contrast would find at 80% power, which is the
  # question "what could it have seen" as distinct from "what did it see". `thresh` is
  # the p = 0.05 boundary and is always the smaller of the two.
  mde <- (tc + qt(0.80, df)) * se
  data.frame(what = label, S = S, mean = m * scale, se = se * scale,
             t = m / se, p = 2 * pt(-abs(m / se), df), thresh = tc * se * scale,
             mde = mde * scale,
             lo = (m - tc * se) * scale, hi = (m + tc * se) * scale,
             stringsAsFactors = FALSE)
}

# Holm within each table, because every table below tests several buckets at once and
# reading the smallest of six raw p-values is an argmax over six noisy estimates.
report <- function(rows, title) {
  rows$holm <- p.adjust(rows$p, method = "holm")
  cat(sprintf("\n=== %s\n", title))
  cat(sprintf("%-34s %2s %9s %9s %7s %7s %7s %9s %8s %19s\n",
              "quantity", "S", "mean", "SE", "t", "p", "Holm", "thresh", "MDE80", "95% CI"))
  for (i in seq_len(nrow(rows))) {
    r <- rows[i, ]
    cat(sprintf("%-34s %2d %+9.4f %9.4f %+7.2f %7.4f %7.4f %9.4f %8.4f  [%+8.4f,%+8.4f]\n",
                r$what, r$S, r$mean, r$se, r$t, r$p, r$holm, r$thresh, r$mde, r$lo, r$hi))
  }
}

# Labels are DERIVED, not literals. A directory holding three of the four seasons used to
# print "[four seasons]" over three and then "[three full seasons (2022-23 dropped)]" for
# a season that was never there -- and these labels get pasted into Go comments verbatim.
setlabel <- function(ss, note = "") {
  s <- sprintf("%d season%s: %s", length(ss), ifelse(length(ss) == 1, "", "s"),
               paste(ss, collapse = " "))
  if (note != "") s <- paste0(s, " -- ", note)
  s
}
full <- setdiff(seasons, "2022-23")
sets <- list(list(nm = setlabel(seasons), ss = seasons))
if (length(full) >= 2 && length(full) < length(seasons)) {
  sets[[2]] <- list(nm = setlabel(full, "2022-23 dropped, GW16-38 only"), ss = full)
}
for (set in sets) {
  ss <- set$ss
  if (length(ss) < 2) next
  cc <- cells[cells$season %in% ss, ]
  hh <- hl[hl$season %in% ss, ]

  rows <- list()
  # Per-arm per-tercile recentred level, against 1. Departure from 1 IS the
  # cancellation failing, since recentring already removed the shared level.
  for (arm in c("FPL", "UST")) for (b in labs) {
    x <- cc$recentred[cc$arm == arm & cc$lab == b] - 1
    rows[[length(rows) + 1]] <- clustered(x, sprintf("%s recentred %s minus 1", arm, b))
  }
  report(do.call(rbind, rows), sprintf("Table 3 [%s]. Is each bucket flat?", set$nm))

  rows <- list()
  # The PAIRED contrast, which is the mandate: UST minus FPL within season.
  for (b in labs) {
    x <- cc$recentred[cc$arm == "UST" & cc$lab == b] -
         cc$recentred[cc$arm == "FPL" & cc$lab == b]
    rows[[length(rows) + 1]] <- clustered(x, sprintf("UST-FPL recentred %s", b))
  }
  x <- hh$hl[hh$arm == "UST"] - hh$hl[hh$arm == "FPL"]
  rows[[length(rows) + 1]] <- clustered(x, "UST-FPL (high-low)")
  x <- hh$spread[hh$arm == "UST"] - hh$spread[hh$arm == "FPL"]
  rows[[length(rows) + 1]] <- clustered(x, "UST-FPL spread (max-min)")
  report(do.call(rbind, rows),
         sprintf("Table 4 [%s]. Paired UST minus FPL -- the transport contrast", set$nm))

  # The structure statistics per arm, so the +/-1.7% band has something to be read
  # against. WARNING on the second of the two: `spread` is max minus min, which is
  # bounded below by zero and inflated by noise, so a t against zero is MECHANICAL --
  # the same defect this record records against the chip-timing differences, which are
  # >= 0 in every cell by construction. Read the signed `high-low` row for a shape and
  # the spread row only as a magnitude.
  #
  # The PAIRED spread contrast in Table 4 is biased TOWARD finding a transport failure and
  # does not find one: the UST arm's bootstrap SEs run larger than the FPL arm's (median
  # 1.69x over the twelve buckets, spanning 0.88 to 2.35, below 1 in one), so under a null
  # of identical true structure E[spread_UST] exceeds E[spread_FPL]. ⚠️ "The bias is
  # common to both arms and differences it" was the wrong defence and is WITHDRAWN -- it
  # needs the two arms' noise to be equal, and it is not. See the matching note in
  # xgcrepair.go. Recorded here because the retraction guard does not scan stats/*.R, so
  # nothing but this comment stops the withdrawn argument being read as live.
  rows <- list()
  for (arm in c("FPL", "UST")) {
    rows[[length(rows) + 1]] <- clustered(hh$hl[hh$arm == arm], sprintf("%s (high-low)", arm))
    rows[[length(rows) + 1]] <- clustered(hh$spread[hh$arm == arm], sprintf("%s spread (max-min)", arm))
  }
  report(do.call(rbind, rows), sprintf("Table 5 [%s]. Tercile structure per arm", set$nm))
}

# ---------------------------------------------------------------------------
# The CONTINUOUS form of the same contrast, which is sharper than the tercile cut and is
# the reading the verdict should carry.
#
# Terciles discard the within-bucket ordering on exposure. Regress the per-player log arm
# ratio on exposure directly -- log because the arms differ by a level and a log turns
# that level into an intercept the slope is orthogonal to -- and the slope IS "does the
# transported reconstruction credit high-exposure players differently". Fitted per season,
# then the four season slopes are combined season-clustered, which is the same estimator
# every pooled figure above uses.
#
# This must AGREE with the tercile contrast, or one of the two is wrong: the slope times
# the observed high-minus-low mean exposure gap should reproduce the high-low tercile
# contrast. That reconciliation is printed.
cat("\n=== Table 6. The continuous contrast: slope of log(rec90_ust/rec90_fpl) on exposure\n")
cat(sprintf("%-9s %9s %9s %6s   %s\n", "season", "slope", "SE(club)", "n", "clubs"))
slopes <- c(); gaps <- c()
for (s in seasons) {
  ds <- d[d$season == s, ]
  fit <- lm(log(armratio) ~ exposure, data = ds)
  b <- unname(coef(fit)["exposure"])
  # Club-clustered SE within season, CR0 sandwich by hand -- no sandwich package here.
  X <- model.matrix(fit); u <- residuals(fit)
  bread <- solve(crossprod(X))
  meat <- Reduce(`+`, lapply(split(seq_len(nrow(X)), ds$club), function(i) {
    xu <- crossprod(X[i, , drop = FALSE], u[i]); tcrossprod(xu)
  }))
  se <- sqrt((bread %*% meat %*% bread)[2, 2])
  cat(sprintf("%-9s %+9.4f %9.4f %6d   %d\n", s, b, se, nrow(ds), length(unique(ds$club))))
  slopes <- c(slopes, b)
  gaps <- c(gaps, mean(ds$exposure[ds$lab == "high"]) - mean(ds$exposure[ds$lab == "low"]))
}
slopesets <- list(list(nm = setlabel(seasons), k = seq_along(seasons)))
if (length(full) >= 2 && length(full) < length(seasons)) {
  slopesets[[2]] <- list(nm = setlabel(full, "2022-23 dropped"),
                         k = which(seasons != "2022-23"))
}
for (set in slopesets) {
  r <- clustered(slopes[set$k], sprintf("exposure slope [%s]", set$nm))
  r$holm <- r$p
  cat(sprintf("\npooled [%s]: slope %+.4f, SE %.4f, t %+.2f, p %.4f, thresh %.4f, MDE80 %.4f\n",
              set$nm, r$mean, r$se, r$t, r$p, r$thresh, r$mde))
  cat(sprintf("  positive in %d of %d seasons; implied high-low gap = slope x %.3f = %+.4f\n",
              sum(slopes[set$k] > 0), length(set$k), mean(gaps[set$k]),
              r$mean * mean(gaps[set$k])))
}

# ---------------------------------------------------------------------------
# The two estimators' disagreement, which is not noise-shaped and is an argument for the
# headline rather than a footnote: mean-of-ratios exceeds ratio-of-totals by more in the
# high-exposure bucket than the low one, so it MANUFACTURES tercile structure.
cat("\n=== Table 7. mean-of-ratios minus ratio-of-totals, by tercile (percentage points)\n")
cat(sprintf("%-9s %-5s %8s %8s %8s   %s\n", "season", "arm", "low", "mid", "high", "monotone?"))
mono <- 0
for (s in seasons) for (arm in c("FPL", "UST")) {
  r <- cells[cells$season == s & cells$arm == arm, ]
  g <- setNames(100 * (r$meanratio - r$ratio), r$lab)
  up <- g["low"] <= g["mid"] && g["mid"] <= g["high"]
  mono <- mono + as.integer(up)
  cat(sprintf("%-9s %-5s %+8.3f %+8.3f %+8.3f   %s\n",
              s, arm, g["low"], g["mid"], g["high"], ifelse(up, "yes", "no")))
}
cat(sprintf("increasing in exposure in %d of %d season-arms; overall range %+.3f to %+.3f pp\n",
            mono, 2 * length(seasons),
            100 * min(cells$meanratio - cells$ratio), 100 * max(cells$meanratio - cells$ratio)))

cat("\nEstimator: ratio of totals over per-player season XGC90, players weighted equally\n")
cat("in rate space; recentred on the same season-arm's whole-population ratio.\n")
cat("Uncertainty: club-clustered pairs bootstrap within season (Table 1),\n")
cat("season-clustered elsewhere. thresh = t_crit(S-1) * SE.\n")
