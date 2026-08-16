#!/usr/bin/env Rscript

# Inference for the out-of-sample prediction benchmark.
#
# Usage:
#   Rscript stats/prediction_inference.R /tmp/prediction.csv [more.csv ...]
#   Rscript stats/prediction_inference.R --out=stats/out --target=points \
#       --population="60+ minutes" /tmp/prediction.csv
#   Rscript stats/prediction_inference.R --selftest
#
# Produce the input with:
#   FPL_PREDICTION_CSV=/tmp/prediction.csv DIAG=1 go test ./internal/backtest \
#       -run TestDiagPredictionBenchmark -v -timeout 60m
#
# ---------------------------------------------------------------------------
# What this answers, and what it deliberately does not
#
# The Go benchmark prints error, calibration and ordering figures and no standard
# error, exactly as the sweep harness prints means and no t: inference lives in
# one place. This script is that place for the prediction half.
#
# It answers one question. Two settings of the model, or a setting against a
# naive baseline, are compared on the *same* player-gameweeks: is the difference
# in error larger than the noise?
#
# It does NOT answer "is the change worth points". Nothing here can: the
# project's hardest-won result is that a better predictor can make a worse
# policy, because a transfer policy is an argmax that lives in the tail of the
# estimate distribution, and out-of-sample accuracy bought on the average player
# is paid for with noise exactly where the search looks. Recency on rates
# predicts about 2 per cent better and cost about 49 points a season. So a
# candidate that wins here is a candidate worth spending replay time on, and the
# replay decides.
#
# ---------------------------------------------------------------------------
# Why the cluster is a gameweek
#
# Every player in a gameweek is exposed to the same football: the same referees,
# the same weather, the same league-wide scoring level. Their errors are
# therefore correlated, and a standard error that treats 40,000 player-gameweeks
# as 40,000 independent draws will be far too small. The unit of replication is a
# gameweek, of which there are about 130 across four seasons.
#
# Two cluster levels are reported for the same reason the sweep script reports
# three standard errors. Clustering by GAMEWEEK gives about 130 clusters and is
# the primary reading. Clustering by SEASON gives four, which is the level the
# replay harness is forced to use and is reported so the two instruments can be
# compared honestly — four clusters means three degrees of freedom and a critical
# value of 3.18 rather than the familiar 2, which is precisely why the prediction
# instrument exists.
#
# The CSV carries per-gameweek sufficient statistics rather than one row per
# player-gameweek, and that is not a shortcut. A paired difference between two
# arms is the difference of their per-cluster sums, because both arms score the
# identical observations — so summing then differencing and differencing then
# summing are the same arithmetic. Per-observation rows would only permit
# clustering below the cluster.
#
# R is a developer tool here, not a dependency: `go build`, `go vet` and
# `go test` all pass with no R installed, and nothing in the Go test suite
# invokes this script.
# ---------------------------------------------------------------------------

args <- commandArgs(trailingOnly = TRUE)
opt_out <- "stats/out"
opt_target <- "points"
opt_population <- "60+ minutes"
opt_selftest <- FALSE
paths <- character(0)

for (a in args) {
  if (identical(a, "--selftest")) {
    opt_selftest <- TRUE
  } else if (grepl("^--out=", a)) {
    opt_out <- sub("^--out=", "", a)
  } else if (grepl("^--target=", a)) {
    opt_target <- sub("^--target=", "", a)
  } else if (grepl("^--population=", a)) {
    opt_population <- sub("^--population=", "", a)
  } else if (grepl("^--", a)) {
    stop("unknown option: ", a)
  } else {
    paths <- c(paths, a)
  }
}

# `fail`, `note`, `hr` and `read_sidecar` are in cells_common.R.
#
# ⚠️ This script does NOT read the cells contract — its input is the prediction
# benchmark's own CSV, with its own required columns and its own validation below.
# It sources the shared file for the helpers and for `read_sidecar`, the sanctioned
# reader for a pipeline CSV that is not a cells file. The guard forbids `read.csv(`
# elsewhere in `stats/*.R`, and a script with a different contract is still not the
# place a raw read should hide.
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

# --- the arithmetic, in one place ------------------------------------------

# cluster_test is a paired mean with a cluster-robust standard error.
#
# `diff` is one paired difference per cluster, already weighted by how many
# observations the cluster holds via `w`. The estimate is the weighted mean, and
# the standard error is the ordinary cluster-robust one for a weighted mean: the
# spread of the per-cluster contributions to the estimate.
#
# Reported with Satterthwaite-free df = clusters - 1, stated rather than assumed,
# and a two-sided t p-value. There is no small-sample CR2 correction here and
# none is needed for the primary reading: at ~130 clusters the correction is
# negligible, and it is named as absent rather than implied.
cluster_test <- function(diff, w) {
  keep <- is.finite(diff) & is.finite(w) & w > 0
  diff <- diff[keep]
  w <- w[keep]
  g <- length(diff)
  if (g < 2) {
    return(list(est = NA_real_, se = NA_real_, t = NA_real_, p = NA_real_,
                df = NA_real_, clusters = g))
  }
  est <- sum(w * diff) / sum(w)
  # Each cluster's influence on the weighted mean.
  infl <- w * (diff - est) / sum(w)
  se <- sqrt(g / (g - 1) * sum(infl^2))
  df <- g - 1
  tv <- if (is.finite(se) && se > 0) est / se else NA_real_
  p <- if (is.finite(tv)) 2 * stats::pt(-abs(tv), df = df) else NA_real_
  list(est = est, se = se, t = tv, p = p, df = df, clusters = g)
}

# holm adjusts a family of raw p-values for having asked several questions.
#
# Holm rather than Bonferroni because it controls the family-wise error rate
# without assuming independence, and these comparisons share every observation.
holm <- function(p) stats::p.adjust(p, method = "holm")

# --- self-test -------------------------------------------------------------
#
# Arithmetic invariants only: no replay, no CSV, no packages. It exists because
# the one thing that must not be wrong here is the standard error, and a wrong
# standard error still prints a confident number.

if (opt_selftest) {
  ok <- TRUE
  check <- function(label, cond, got = NULL) {
    if (!isTRUE(cond)) {
      ok <<- FALSE
      cat("FAIL: ", label, if (!is.null(got)) paste0(" (got ", got, ")"), "\n", sep = "")
    } else {
      cat("ok:   ", label, "\n", sep = "")
    }
  }

  # An identical pair of arms differs by exactly zero in every cluster, so the
  # estimate, the standard error and t are all zero rather than NaN.
  z <- cluster_test(rep(0, 20), rep(300, 20))
  check("identical arms give an estimate of exactly 0", isTRUE(all.equal(z$est, 0)), z$est)
  check("identical arms give a standard error of exactly 0", isTRUE(all.equal(z$se, 0)), z$se)

  # Equal weights must reproduce the ordinary paired t-test exactly. This is the
  # cross-check that matters: the cluster-robust formula is derived completely
  # differently from the textbook one and must agree when the weights are flat.
  set.seed(11)
  d <- rnorm(40, mean = 0.3, sd = 1)
  got <- cluster_test(d, rep(1, 40))
  want <- stats::t.test(d)
  check("with equal weights the estimate is the plain mean",
        isTRUE(all.equal(got$est, mean(d))), got$est)
  check("with equal weights the standard error matches a paired t-test",
        isTRUE(all.equal(got$se, unname(want$stderr), tolerance = 1e-12)),
        paste(got$se, unname(want$stderr)))
  check("with equal weights the p-value matches a paired t-test",
        isTRUE(all.equal(got$p, unname(want$p.value), tolerance = 1e-10)),
        paste(got$p, unname(want$p.value)))
  check("df is clusters minus one, reported not assumed", got$df == 39, got$df)

  # Weighting must actually weight: a cluster carrying ten times the
  # observations must move the estimate ten times as far.
  w <- c(1, 10)
  wd <- c(0, 1)
  check("weights move the estimate in proportion to the observations",
        isTRUE(all.equal(cluster_test(wd, w)$est, 10 / 11)),
        cluster_test(wd, w)$est)

  # Too few clusters is reported as unmeasurable rather than as zero noise.
  check("one cluster reports NA rather than certainty",
        is.na(cluster_test(c(0.5), c(100))$se))

  # Holm must be at least as large as raw, and must not reject more often.
  praw <- c(0.01, 0.04, 0.2)
  check("Holm never lowers a p-value", all(holm(praw) >= praw))

  cat("\n", if (ok) "SELFTEST PASSED" else "SELFTEST FAILED", "\n", sep = "")
  quit(status = if (ok) 0 else 1)
}

if (length(paths) == 0) {
  stop("give at least one prediction CSV, or --selftest; see the header of this file")
}

# --- load ------------------------------------------------------------------

required <- c("run_id", "variant", "is_baseline", "season", "gw",
              "population", "target", "predictor", "category",
              "n", "sum_abs_err", "sum_sq_err", "sum_pred", "sum_act")

read_one <- function(p) {
  if (!file.exists(p)) fail(p, " does not exist")
  d <- read_sidecar(p)
  miss <- setdiff(required, names(d))
  if (length(miss)) {
    fail(p, " is missing columns: ", paste(miss, collapse = ", "),
         "\nThis is not a prediction-benchmark CSV. Produce one with ",
         "FPL_PREDICTION_CSV=<path> on TestDiagPredictionBenchmark.")
  }
  d$source <- basename(p)
  d
}

d <- do.call(rbind, lapply(paths, read_one))
note("loaded ", nrow(d), " rows from ", length(paths), " file(s)")

# Resolve the two selectors against what is actually in the file, so a typo says
# what was available rather than silently analysing nothing.
pick <- function(value, column, label) {
  present <- sort(unique(d[[column]]))
  hit <- present[grepl(value, present, fixed = TRUE)]
  if (length(hit) != 1) {
    fail("--", label, "=", value, " matched ", length(hit), " of the ",
         length(present), " values present:\n  ",
         paste(present, collapse = "\n  "))
  }
  hit
}
target <- pick(opt_target, "target", "target")
population <- pick(opt_population, "population", "population")
note("target:     ", target)
note("population: ", population)

sel <- d[d$target == target & d$population == population, ]
if (!nrow(sel)) fail("no rows survived the target and population filters")

baseline <- unique(sel$variant[sel$is_baseline == "true"])
if (length(baseline) != 1) {
  fail("expected exactly one baseline arm, found ", length(baseline),
       ": ", paste(baseline, collapse = ", "),
       "\nEverything is paired against the baseline and R cannot pick it by guessing.")
}
note("baseline:   ", baseline)

# --- descriptive: the levels, so the differences below have a scale ---------

hr()
note("ERROR BY PREDICTOR AND REALISED CATEGORY")
note("Mean absolute error and root-mean-square error, LOWER IS BETTER for both.")
note("Bias is predicted minus actual, so positive means over-prediction.")
note("Categories are defined by what the player ACTUALLY scored, which conditions")
note("on the outcome: that rewards a NOISIER predictor in the extreme buckets, so")
note("read Haulers beside the ordering table and never on its own.")
hr()

levels_of <- function(x) {
  n <- sum(x$n)
  data.frame(
    n = n,
    mae = sum(x$sum_abs_err) / n,
    rmse = sqrt(sum(x$sum_sq_err) / n),
    bias = (sum(x$sum_pred) - sum(x$sum_act)) / n,
    stringsAsFactors = FALSE
  )
}

lv <- do.call(rbind, lapply(
  split(sel, list(sel$variant, sel$predictor, sel$category), drop = TRUE),
  function(x) cbind(
    variant = x$variant[1], predictor = x$predictor[1], category = x$category[1],
    levels_of(x)
  )
))
lv <- lv[order(lv$variant, lv$predictor, lv$category), ]
print(lv, row.names = FALSE, digits = 4)

# --- paired: model against each naive baseline, within the baseline arm -----
#
# The comparison is between PREDICTORS at a fixed setting of the model. Its
# cluster is a gameweek and its pairing is exact: both predictors are scored on
# the identical player-gameweeks.

paired_by_gw <- function(a, b, stat) {
  key <- function(x) paste(x$season, x$gw, sep = "|")
  a$k <- key(a); b$k <- key(b)
  m <- merge(a, b, by = "k", suffixes = c(".a", ".b"))
  if (!nrow(m)) return(NULL)
  if (any(m$n.a != m$n.b)) {
    fail("two arms disagree about how many observations a gameweek holds. ",
         "They are supposed to score the identical population, so the pairing ",
         "is broken and every difference below would be meaningless.")
  }
  val <- switch(stat,
    mse = (m$sum_sq_err.a - m$sum_sq_err.b) / m$n.a,
    mae = (m$sum_abs_err.a - m$sum_abs_err.b) / m$n.a,
    # bias is the SYSTEMATIC half of the error, signed, and it is a separate
    # question from mse rather than a decomposition of the same answer. This
    # project's standing rule is that removing a bias is safe for a search that
    # picks the highest-scoring player while trading bias for variance is not,
    # and five recorded corrections of measured biases have lost points — so a
    # candidate has to be judged on which of the two it does. mse cannot say:
    # mse = bias^2 + spread^2, and a fall in either lowers it.
    #
    # Written out in full rather than as a difference of predictions. The two are
    # identical whenever the pairing holds, since both arms score the same
    # observations and sum_act cancels, and the n check above already enforces
    # that — but the full form stays correct if it ever stops holding, and costs
    # nothing.
    bias = ((m$sum_pred.a - m$sum_act.a) - (m$sum_pred.b - m$sum_act.b)) / m$n.a,
    # spread2 is the OTHER half, so the two together account for mse exactly and
    # neither half has to be read off a descriptive table with no standard error.
    #
    # It is the squared spread WITHIN the gameweek — mean squared error minus that
    # gameweek's own squared bias — differenced between the arms. That is not the
    # pooled spread reported in the levels table, because the pooled bias is not
    # the mean of the per-gameweek biases; it is the quantity a paired test can
    # actually carry, and it answers the question that matters: did the arm's
    # residual scatter change, or only its level?
    spread2 = (m$sum_sq_err.a / m$n.a - ((m$sum_pred.a - m$sum_act.a) / m$n.a)^2) -
              (m$sum_sq_err.b / m$n.b - ((m$sum_pred.b - m$sum_act.b) / m$n.b)^2),
    fail("unknown statistic: ", stat))
  list(diff = val, w = m$n.a, season = m$season.a)
}

report <- function(title, rows, note_lines = character(0)) {
  hr()
  note(title)
  for (l in note_lines) note(l)
  hr()
  if (!length(rows)) {
    note("nothing to compare")
    return(invisible(NULL))
  }
  out <- do.call(rbind, rows)
  # Holm covers the ERROR statistics only, and deliberately not the decomposition.
  #
  # Holm controls the family-wise error rate over hypotheses one might reject.
  # `bias` and `spread2` are not rejected on: they are the two halves of mse,
  # since mse is exactly bias squared plus spread squared, and they exist to say
  # WHICH of the two a change moved. Putting a quantity and its own decomposition
  # in one family inflates the correction on the quantity for having also looked
  # at its parts — which would silently move every adjusted p-value already
  # recorded from this script, for a reason that is not a measurement. They are
  # reported with raw p-values and read beside mse, never instead of it.
  err <- out$statistic %in% c("mse", "mae")
  out$p_holm <- NA_real_
  out$p_holm[err] <- holm(out$p_gw[err])
  print(out, row.names = FALSE, digits = 4)
  invisible(out)
}

compare <- function(label, a, b, stat) {
  by_gw <- paired_by_gw(a, b, stat)
  if (is.null(by_gw)) return(NULL)
  gw <- cluster_test(by_gw$diff, by_gw$w)
  # Season-level: aggregate each season's clusters first, then cluster on four.
  bys <- split(seq_along(by_gw$diff), by_gw$season)
  sdiff <- vapply(bys, function(i)
    sum(by_gw$w[i] * by_gw$diff[i]) / sum(by_gw$w[i]), numeric(1))
  sw <- vapply(bys, function(i) sum(by_gw$w[i]), numeric(1))
  se <- cluster_test(sdiff, sw)
  data.frame(
    comparison = label, statistic = stat,
    estimate = gw$est,
    se_gw = gw$se, t_gw = gw$t, df_gw = gw$df, gws = gw$clusters, p_gw = gw$p,
    se_season = se$se, t_season = se$t, df_season = se$df,
    stringsAsFactors = FALSE
  )
}

base <- sel[sel$variant == baseline & sel$category == "all categories", ]
model_rows <- base[base$predictor == "model", ]
others <- setdiff(unique(base$predictor), "model")

rows <- list()
for (o in others) {
  for (stat in c("mse", "mae")) {
    rows[[length(rows) + 1]] <- compare(paste0("model minus [", o, "]"),
                                        model_rows, base[base$predictor == o, ], stat)
  }
}
report("THE MODEL AGAINST EACH NAIVE BASELINE",
       rows,
       c("estimate is the model's error minus the baseline's, per observation, so",
         "NEGATIVE MEANS THE MODEL IS BETTER. mse is mean squared error (the square",
         "of root-mean-square error) and is the statistic to test on, because it is",
         "linear in the per-observation squared errors and therefore pairs exactly.",
         "",
         "Two clusterings of the same estimate. The GAMEWEEK columns are primary:",
         "about 130 clusters, so the t distribution behaves. The SEASON columns use",
         "four clusters and three degrees of freedom, which is the level the replay",
         "harness is forced to work at — reported so the two instruments can be",
         "compared honestly, and the reason this one exists.",
         "",
         "p_holm adjusts for having asked about several baselines and statistics at",
         "once."))

# --- paired: each candidate arm against the shipped baseline ---------------

arms <- setdiff(unique(sel$variant), baseline)
rows <- list()
for (a in arms) {
  aa <- sel[sel$variant == a & sel$category == "all categories" & sel$predictor == "model", ]
  for (stat in c("mse", "mae", "bias", "spread2")) {
    rows[[length(rows) + 1]] <- compare(paste0("[", a, "] minus [", baseline, "]"),
                                        aa, model_rows, stat)
  }
}
report("EACH ARM AGAINST THE SHIPPED BASELINE",
       rows,
       c("estimate is the arm's error minus the shipped config's, per observation,",
         "so NEGATIVE MEANS THE ARM IS BETTER on mse and mae.",
         "",
         "bias is the exception and is SIGNED, not an error: it is the arm's",
         "over-prediction minus the shipped config's, so a negative estimate means",
         "the arm predicts LOWER, which is only an improvement where the shipped",
         "model was over-predicting. Read it beside the bias column of the levels",
         "table above, which gives each arm's bias its sign and scale. It is here",
         "because mse cannot separate the two halves of the error and this project",
         "treats them completely differently: removing a bias is safe for an argmax,",
         "trading bias for variance is not.",
         "",
         "spread2 is the other half — the squared residual scatter within a gameweek,",
         "differenced. NEGATIVE is better. Read bias and spread2 together: they",
         "account for mse exactly, and which of the two moved is the whole question.",
         "Neither is in the Holm family, because a quantity and its own decomposition",
         "are not several questions; their p-values are raw.",
         "",
         "An arm whose estimate and standard error are both exactly 0 is an",
         "INVARIANCE CHECK PASSING, not a failed measurement — the vice-captain",
         "fallback arm should read exactly zero here, because it changes how a",
         "played-out gameweek is scored and nothing about what the model predicts.",
         "",
         "A significant improvement here is NOT a reason to change a constant. It is",
         "a reason to spend replay time on one. Read the Go benchmark's",
         "bias-versus-variance verdict and its tail figure first: a candidate that",
         "lowers error while pushing the signed error over the highest-predicted",
         "players away from zero has the recorded better-predictor-worse-policy",
         "shape."))

# --- ordering and the tail, which are gameweek-level scalars ---------------

hr()
note("ORDERING AND THE TAIL")
note("These are per-gameweek scalars rather than sums, so they are averaged over")
note("gameweeks with a cluster-robust standard error on the gameweek.")
note("")
note("rank correlation: +1 is a perfect ordering, HIGHER IS BETTER. It matters")
note("because the optimiser consumes an ordering and never a level.")
note("tail signed error: the mean of predicted minus actual over the highest-")
note("predicted players in a gameweek. POSITIVE means the top of the predicted")
note("distribution is over-rated; CLOSER TO ZERO IS BETTER.")
hr()

scal <- sel[sel$category == "all categories" & !is.na(sel$rank_corr), ]
if (nrow(scal)) {
  ord <- do.call(rbind, lapply(
    split(scal, list(scal$variant, scal$predictor), drop = TRUE),
    function(x) {
      r <- cluster_test(x$rank_corr, rep(1, nrow(x)))
      tl <- cluster_test(x$tail_signed_err, rep(1, nrow(x)))
      data.frame(variant = x$variant[1], predictor = x$predictor[1],
                 gws = r$clusters,
                 rank_corr = r$est, rank_se = r$se,
                 tail = tl$est, tail_se = tl$se,
                 stringsAsFactors = FALSE)
    }))
  ord <- ord[order(ord$variant, ord$predictor), ]
  print(ord, row.names = FALSE, digits = 4)

  # The same two scalars, PAIRED against the baseline arm.
  #
  # The table above is unpaired: its standard error is the spread of the
  # statistic across gameweeks, which is dominated by how much easier some
  # gameweeks are to order than others — a quantity both arms share exactly. That
  # makes it about thirty times too large to answer the question anybody actually
  # asks of two arms, which is whether one orders better than the other on the
  # SAME gameweeks. Differencing within a gameweek removes the shared part, and
  # is the same arithmetic every other comparison here already uses.
  #
  # It matters most for rank correlation, because the optimiser consumes an
  # ordering and never a level, so an arm that lowers error while quietly
  # degrading the ordering has the recorded better-predictor-worse-policy shape.
  #
  # Both clusterings, for the reason compare() reports both: the season level is
  # what the replay is forced to work at, it has a handful of clusters rather
  # than a couple of hundred, and which of the two standard errors is larger is
  # not knowable in advance — clustering strengthens the evidence where seasons
  # agree and weakens it where they disagree.
  pair_scalar <- function(a, b, col) {
    key <- function(x) paste(x$season, x$gw, sep = "|")
    a$k <- key(a); b$k <- key(b)
    m <- merge(a, b, by = "k", suffixes = c(".a", ".b"))
    d <- m[[paste0(col, ".a")]] - m[[paste0(col, ".b")]]
    gw <- cluster_test(d, rep(1, length(d)))
    bys <- split(seq_along(d), m$season.a)
    sdiff <- vapply(bys, function(i) mean(d[i]), numeric(1))
    se <- cluster_test(sdiff, vapply(bys, length, integer(1)))
    list(gw = gw, season = se)
  }
  base_scal <- scal[scal$variant == baseline & scal$predictor == "model", ]
  prows <- list()
  for (a in setdiff(unique(scal$variant), baseline)) {
    aa <- scal[scal$variant == a & scal$predictor == "model", ]
    if (!nrow(aa) || !nrow(base_scal)) next
    for (col in c("rank_corr", "tail_signed_err")) {
      r <- pair_scalar(aa, base_scal, col)
      prows[[length(prows) + 1]] <- data.frame(
        comparison = paste0("[", a, "] minus [", baseline, "]"),
        statistic = col, estimate = r$gw$est, se_gw = r$gw$se, t_gw = r$gw$t,
        df_gw = r$gw$df, gws = r$gw$clusters, p_gw = r$gw$p,
        se_season = r$season$se, t_season = r$season$t, df_season = r$season$df,
        stringsAsFactors = FALSE)
    }
  }
  if (length(prows)) {
    hr()
    note("THE SAME TWO SCALARS, PAIRED WITHIN THE GAMEWEEK")
    note("rank_corr: POSITIVE means the arm orders players BETTER than shipped.")
    note("tail_signed_err: signed, not an error — it is the arm's over-rating of the")
    note("top of its own distribution minus the shipped config's, so read it beside")
    note("the unpaired table above, where each arm's figure has its own sign.")
    hr()
    pout <- do.call(rbind, prows)
    pout$p_holm <- holm(pout$p_gw)
    print(pout, row.names = FALSE, digits = 4)
  }
} else {
  note("the file carries no per-gameweek rank correlation; it predates that column")
}

# --- write ------------------------------------------------------------------

dir.create(opt_out, showWarnings = FALSE, recursive = TRUE)
utils::write.csv(lv, file.path(opt_out, "prediction_levels.csv"), row.names = FALSE)
if (exists("ord")) {
  utils::write.csv(ord, file.path(opt_out, "prediction_ordering.csv"), row.names = FALSE)
}
hr()
note("wrote prediction_levels.csv", if (exists("ord")) " and prediction_ordering.csv" else "",
     " to ", opt_out)
note("")
note("Reminder, because it is the most expensive mistake available here: this")
note("script ranks predictors and cannot price them. The replay decides points,")
note("and a better predictor can make a worse policy.")
