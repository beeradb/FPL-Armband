#!/usr/bin/env Rscript

# Judging a sweep by the ORDER of its settings instead of the size of a gap.
#
# Usage:
#   Rscript stats/shape_inference.R --order=numeric /tmp/cells.csv
#   Rscript stats/shape_inference.R --order="flat,hl 2,hl 4,hl 8,hl 20" cells.csv
#   Rscript stats/shape_inference.R --order=numeric --mediator=moves cells.csv
#   Rscript stats/shape_inference.R --order=numeric --invariant=hold cells.csv
#   Rscript stats/shape_inference.R --selftest
#
# ---------------------------------------------------------------------------
# The problem this exists for
#
# A "cell" is one replayed season entered at one starting gameweek. The harness
# runs four seasons at six entry points, so 24 cells — but the six entry points
# inside a season replay the same football, so they are not 24 independent
# measurements. Uncertainty is therefore assessed by averaging each season to one
# number and asking how much the four seasons disagree, which leaves **four
# independent units and three degrees of freedom**.
#
# Three degrees of freedom is a hard wall. The cutoff for calling a result
# statistically significant at the conventional 5% level is 3.18 standard errors
# rather than the familiar 2, and the smallest effect the design can detect is
# roughly 42 points a season on HOLD (the metric that buys an opening fifteen and
# never transfers, but re-picks the eleven and captain every week with autosubs)
# and about 147 on POLICY (that, plus the weekly transfer decision). Every
# constant this project argues over is worth 11 to 34 points a season. So almost
# nothing is resolvable by comparing two settings head to head, and there will
# never be more than a handful of seasons — the archive carries expected goals
# from 2022-23 only.
#
# ---------------------------------------------------------------------------
# The way out, and its limits
#
# A sweep has a second axis nobody pools over: **the settings themselves.** Five
# settings do not just give five numbers, they give an *ordering*, and an ordering
# is much cheaper to establish than a magnitude. The mechanism is that seasons
# disagree wildly about how big an effect is and much less about which setting is
# better — a season that scales every effect up or down does not reorder them.
#
# Three things are computed here, all of them about order and none about size:
#
#   1. A trend test. Within each cell the settings are ranked worst to best; each
#      setting's ranks are added up across cells; those sums are then weighted by
#      the setting's position in a *predicted* order and totalled into a single
#      "trend score". A large score means the cells keep putting the settings in
#      the predicted order. (This is Page's test for ordered alternatives; the
#      name is not the definition, the three steps above are.)
#
#   2. Where the peak is, rather than how high it is. The winning setting is
#      reported per cell and the distribution over cells printed, so "the peak is
#      at 4 in 9 of 24 cells, at 20 in 6 and at flat in 4" replaces "4 wins".
#      This project's own standing complaint is that a single argmax out of five
#      swept values is not evidence, and this is what saying so looks like.
#
#   3. How many individual seasons reproduce the claimed order. This is the brake
#      on the first item and it must be read beside it.
#
# **The limit, stated before any number is printed.** The trend test computed
# over 24 cells as though they were independent gave p = 0.0001 for the minutes
# half-life ladder. That is optimistic and must never be quoted bare: on POLICY
# the cells demonstrably are not independent, which is exactly what a real
# season-to-season difference means. So this script reports the trend test twice,
# at 24 cells and again over the four season means, and the second figure is the
# one to believe. Shape adds an axis *inside* the four-season wall; it does not
# escape it. It helps most where the cells are nearly independent, which is HOLD
# — and HOLD is where the minutes half-life shape signal happens to be weak.
#
# Two further limits, both structural:
#
#   * **The predicted order must be committed to in advance.** Looking at the
#     data, noticing an order and then testing that order makes the resulting
#     p-value meaningless. This script therefore refuses to run the trend test
#     unless an order is passed on the command line, and prints the order it used
#     into its own output so the record shows what was predicted.
#   * **An order being right does not endorse any particular setting in it.** For
#     the minutes half-life the order that scored p = 0.0001 was "a longer
#     half-life is better", and the longest setting tried (20) has the highest
#     rank sum while the shipped setting (4) is second. What that establishes is
#     that the two shortest settings are worse than the other three. It is silent
#     on 4 against 20, which is exactly what the peak distribution says too.
#
# ---------------------------------------------------------------------------
# Two cheaper checks, in the same file because they answer the same question
#
#   * **A mediator the knob controls directly.** A transfer gate's first-order
#     effect is not points, it is the number of transfers made, and that is
#     counted almost without noise. A knob that moves the count monotonically
#     while leaving points flat is evidence the points really are flat, rather
#     than evidence that the measurement failed. It separates "no effect" from
#     "cannot see it", which four seasons otherwise cannot.
#   * **Invariance, i.e. falsifying instead of confirming.** Name a quantity the
#     change must *not* move and check it. A transfer-only knob must leave HOLD
#     byte-identical, and a violation shows up in a single cell — where
#     confirming an effect needs 147 points a season. Cheap, and it fails loudly.
#
# ---------------------------------------------------------------------------
# Relationship to the other two scripts
#
#   sweep_inference.R     is this arm distinguishable from the baseline
#   variance_components.R what size of effect could this design see at all
#   shape_inference.R     is the ORDER of the settings reproducible
#
# All three read the same cells CSV, described in stats/README.md. This one needs
# no baseline arm and does not difference anything: it reads levels, because an
# ordering is a statement about levels.
#
# R is a developer tool here, not a dependency. `go build`, `go vet` and
# `go test` all pass with no R installed, and nothing in the Go test suite
# invokes this. Its own regression test is `--selftest`, below.

# --- options ---------------------------------------------------------------

args <- commandArgs(trailingOnly = TRUE)
opt_out <- "stats/out"
opt_order <- NULL
opt_metrics <- NULL
opt_mediator <- NULL
opt_mediator_order <- NULL
opt_invariant <- NULL
opt_selftest <- FALSE
paths <- character(0)

for (a in args) {
  if (identical(a, "--selftest")) {
    opt_selftest <- TRUE
  } else if (grepl("^--out=", a)) {
    opt_out <- sub("^--out=", "", a)
  } else if (grepl("^--order=", a)) {
    opt_order <- sub("^--order=", "", a)
  } else if (grepl("^--metric=", a)) {
    opt_metrics <- strsplit(sub("^--metric=", "", a), ",")[[1]]
  } else if (grepl("^--mediator=", a)) {
    opt_mediator <- sub("^--mediator=", "", a)
  } else if (grepl("^--mediator-order=", a)) {
    opt_mediator_order <- sub("^--mediator-order=", "", a)
  } else if (grepl("^--invariant=", a)) {
    opt_invariant <- strsplit(sub("^--invariant=", "", a), ",")[[1]]
  } else if (grepl("^--", a)) {
    stop("unknown option: ", a)
  } else {
    paths <- c(paths, a)
  }
}

# `fail`, `note`, `hr`, `as_flag`, `read_cells` and the contract checks are in
# cells_common.R. This script carried its own copies of the first three and its own
# reader; the reader is where the family's defects lived.
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
fmt <- function(x, d = 3) formatC(x, format = "f", digits = d)

# A season is 38 gameweeks; every per-gameweek figure in AGENTS.md is quoted
# times 38 for a season-scale number, so keep the same conversion.
SEASON_GWS <- 38

# ===========================================================================
# The statistics, as plain functions so the self-test can pin them
# ===========================================================================

# rank_within_cells turns a table of scores into a table of ranks.
#
# One row per cell, one column per setting, columns already in the predicted
# order. Within each row the worst setting gets rank 1 and the best gets rank k.
# Settings that tie share the average of the ranks they span, which is what keeps
# each row's ranks summing to k(k+1)/2 whether there are ties or not.
#
# Ties are common and are not a defect: two settings can produce a
# byte-identical season, and on a metric a knob cannot touch every setting does.
rank_within_cells <- function(scores) {
  t(apply(scores, 1, rank))
}

# trend_score implements the three steps in the header.
#
#   1. rank the settings inside each cell, worst = 1;
#   2. add each setting's ranks down the cells, giving one rank sum per setting;
#   3. multiply each rank sum by that setting's position in the predicted order
#      (1 for the predicted-worst, k for the predicted-best) and total it.
#
# Step 3 is the whole trick. If the cells keep agreeing with the predicted order
# then the big rank sums sit against the big weights and the total is large. If
# the settings are all equally good the ranks are a coin toss within each cell and
# the total lands near its average.
#
# Under "every setting is equally good" the ranks in a cell are a uniformly random
# permutation, independently in each cell, which fixes the average and spread of
# the total exactly:
#
#   average = n * k * (k+1)^2 / 4
#   variance = n * k^2 * (k+1) * (k^2 - 1) / 144
#
# Only the upper tail is evidence: a large total means the predicted order keeps
# recurring. A *negative* z means the cells prefer the reverse order, which
# refutes the prediction rather than supporting it, so the one-sided p-value is
# deliberately the upper tail and not |z|.
#
# Ties make the true spread smaller than the formula above, so a z computed with
# the untied formula is too small and the test is conservative. That is the safe
# direction and it is reported rather than corrected.
trend_score <- function(scores) {
  n <- nrow(scores)
  k <- ncol(scores)
  if (n < 2 || k < 3) {
    fail("a trend test needs at least 2 cells and 3 settings; got ", n, " and ", k)
  }
  ranks <- rank_within_cells(scores)
  rank_sums <- colSums(ranks)
  # Every row's ranks sum to k(k+1)/2, so the rank sums must total n*k*(k+1)/2.
  # A mismatch means the ranking is broken — a missing cell, a stray NA — and
  # every number after it would be quietly wrong.
  want_total <- n * k * (k + 1) / 2
  if (abs(sum(rank_sums) - want_total) > 1e-9) {
    fail("rank sums total ", sum(rank_sums), " but must total ", want_total,
         " (", n, " cells x ", k, " settings)")
  }
  total <- sum(seq_len(k) * rank_sums)
  avg <- n * k * (k + 1)^2 / 4
  variance <- n * k^2 * (k + 1) * (k^2 - 1) / 144
  z <- (total - avg) / sqrt(variance)
  tied <- any(apply(scores, 1, function(r) anyDuplicated(r) > 0))
  list(n = n, k = k, ranks = ranks, rank_sums = rank_sums, total = total,
       expected = avg, sd = sqrt(variance), z = z,
       p_normal = pnorm(z, lower.tail = FALSE), tied = tied)
}

# permutations_of enumerates every ordering of 1..k. Only used for the exact
# null distribution below, and k here is the number of swept settings — five or
# six, never large.
permutations_of <- function(k) {
  if (k == 1) return(matrix(1L, 1, 1))
  smaller <- permutations_of(k - 1)
  out <- NULL
  for (first in seq_len(k)) {
    out <- rbind(out, cbind(first, ifelse(smaller >= first, smaller + 1L, smaller)))
  }
  out
}

# exact_trend_null gives the complete null distribution of the trend score, not
# an approximation to it.
#
# Under "every setting is equally good" each cell contributes the weighted sum of
# one random ordering, so the distribution of one cell's contribution is obtained
# by listing all k! orderings. The whole total is a sum of n independent copies of
# that, so its distribution is n-1 convolutions of a small discrete
# distribution — exact, and cheap at any n.
#
# This matters most exactly where it is needed. At four blocks the bell-curve
# approximation is poor, and four blocks is what clustering to seasons leaves.
exact_trend_null <- function(n, k) {
  weighted <- as.vector(permutations_of(k) %*% seq_len(k))
  tab <- table(weighted)
  vals <- as.numeric(names(tab))
  prob <- as.numeric(tab) / length(weighted)
  cur_v <- vals
  cur_p <- prob
  for (i in seq_len(n - 1)) {
    sums <- outer(cur_v, vals, "+")
    joint <- outer(cur_p, prob, "*")
    agg <- tapply(as.vector(joint), as.vector(sums), sum)
    cur_v <- as.numeric(names(agg))
    cur_p <- as.numeric(agg)
  }
  list(v = cur_v, p = cur_p)
}

# exact_trend_p is the share of the null distribution at or above the observed
# total. Valid only with no ties, because the enumeration above is over strict
# orderings; with ties the caller falls back to the bell-curve figure and says so.
exact_trend_p <- function(total, n, k) {
  d <- exact_trend_null(n, k)
  sum(d$p[d$v >= total - 1e-9])
}

# concordant_pairs counts how much one cell's own ordering agrees with the
# predicted one, pair by pair: of the k(k-1)/2 pairs of settings, how many does
# this cell put in the predicted relative order. All of them means perfect
# agreement, half means a coin toss, none means the exact reverse. Ties count as
# half a pair, which is the only neutral treatment.
concordant_pairs <- function(scores_row) {
  k <- length(scores_row)
  pairs <- 0
  conc <- 0
  for (i in seq_len(k - 1)) {
    for (j in (i + 1):k) {
      pairs <- pairs + 1
      if (scores_row[j] > scores_row[i]) {
        conc <- conc + 1
      } else if (isTRUE(all.equal(scores_row[j], scores_row[i]))) {
        conc <- conc + 0.5
      }
    }
  }
  list(concordant = conc, pairs = pairs)
}

# peak_counts says which setting wins in each cell and how often. `chance` is
# 1/k, and the exact binomial p-value asks whether any setting wins more often
# than a coin toss over k options would produce — Holm-adjusted across the k
# settings, because asking about all of them and reporting the best is k tests.
#
# **A tie has no winner, and this is where that matters most.** `which.max`
# returns the *first* index on a tie, so a metric the setting cannot touch — a
# transfer knob measured on HOLD, where every arm is byte-identical — would credit
# whichever column happened to sort first with every cell, and the binomial test
# would report it at p < 0.0001. That is a plausible number out of a degenerate
# state, which is this project's signature failure. Tied cells are therefore
# counted as undecided and excluded from both the counts and the denominator, and
# the number of them is reported.
peak_counts <- function(scores, which = c("max", "min")) {
  which <- match.arg(which)
  best <- apply(scores, 1, function(r) if (which == "max") max(r) else min(r))
  n_at_best <- vapply(seq_len(nrow(scores)), function(i) {
    sum(scores[i, ] == best[i])
  }, numeric(1))
  decided <- n_at_best == 1
  winners <- rep(NA_character_, nrow(scores))
  if (any(decided)) {
    winners[decided] <- colnames(scores)[
      apply(scores[decided, , drop = FALSE], 1,
            if (which == "max") which.max else which.min)]
  }
  counts <- table(factor(winners[decided], levels = colnames(scores)))
  n <- sum(decided)
  k <- ncol(scores)
  raw <- if (n == 0) {
    rep(NA_real_, k)
  } else {
    vapply(as.integer(counts), function(x) {
      stats::binom.test(x, n, 1 / k, alternative = "greater")$p.value
    }, numeric(1))
  }
  list(winners = winners, counts = counts, n = n, n_total = nrow(scores),
       ties = sum(!decided), k = k, chance = n / k,
       p_raw = raw,
       p_holm = if (n == 0) raw else p.adjust(raw, "holm"))
}

# strictly_extreme answers the narrow claim "setting number `pos` is the best (or
# worst) one here", counting a tie as *not* satisfying it. `which.min == pos` gets
# this wrong for the same reason peak_counts did.
strictly_extreme <- function(scores, pos, which = c("max", "min")) {
  which <- match.arg(which)
  hits <- vapply(seq_len(nrow(scores)), function(i) {
    r <- as.numeric(scores[i, ])
    others <- r[-pos]
    if (which == "max") all(r[pos] > others) else all(r[pos] < others)
  }, logical(1))
  sum(hits)
}

# ===========================================================================
# Self-test
# ===========================================================================
#
# Run as: Rscript stats/shape_inference.R --selftest
#
# Two kinds of check. Arithmetic invariants, which hold for any input and pin the
# formulas; and one end-to-end worked example on a frozen cells file committed
# under stats/testdata, which pins the whole pipeline to numbers that were
# verified by hand. The frozen file is a real sweep's output and is kept only so
# the worked example stays reproducible — it is not a source of truth about any
# constant, and nothing should be re-derived from it.

if (opt_selftest) {
  failures <- 0
  check <- function(label, got, want, tol = 1e-6) {
    ok <- length(got) == length(want) && all(abs(got - want) <= tol)
    note(if (ok) "  ok  " else "  FAIL", " ", label)
    if (!ok) {
      note("        got  ", paste(format(got), collapse = " "))
      note("        want ", paste(format(want), collapse = " "))
      failures <<- failures + 1
    }
  }
  check_true <- function(label, cond) {
    note(if (isTRUE(cond)) "  ok  " else "  FAIL", " ", label)
    if (!isTRUE(cond)) failures <<- failures + 1
  }

  hr()
  note("Self-test: arithmetic invariants")

  # The null average and spread of the trend score, for the two shapes this
  # harness actually produces: 24 cells and 4 season means, five settings each.
  s24 <- trend_score(matrix(rnorm(24 * 5), 24, 5,
                            dimnames = list(NULL, paste0("s", 1:5))))
  check("24 cells x 5 settings: null average is 1080", s24$expected, 1080)
  check("24 cells x 5 settings: null spread is 24.4949", s24$sd, 24.494897)
  s4 <- trend_score(matrix(rnorm(4 * 5), 4, 5,
                           dimnames = list(NULL, paste0("s", 1:5))))
  check("4 seasons x 5 settings: null average is 180", s4$expected, 180)
  check("4 seasons x 5 settings: null spread is 10", s4$sd, 10)

  # The exact null distribution is derived a completely different way — by
  # listing every ordering and convolving — so its average and spread
  # reproducing the closed-form ones is a real cross-check rather than a
  # restatement. Same discipline as variance_components.R checking its
  # decomposition against the shipped cluster-robust standard error.
  for (nk in list(c(4, 5), c(24, 5), c(6, 6))) {
    d <- exact_trend_null(nk[1], nk[2])
    mu <- sum(d$v * d$p)
    sdv <- sqrt(sum(d$v^2 * d$p) - mu^2)
    check(paste0("exact null reproduces the closed form at n=", nk[1],
                 ", k=", nk[2]),
          c(mu, sdv),
          c(nk[1] * nk[2] * (nk[2] + 1)^2 / 4,
            sqrt(nk[1] * nk[2]^2 * (nk[2] + 1) * (nk[2]^2 - 1) / 144)))
  }

  # A perfectly monotone set of cells hits the largest total the statistic can
  # take, and a perfectly reversed one hits the smallest. 3 cells, 4 settings:
  # largest is 3*(1+4+9+16) = 90, smallest is 3*(4+6+6+4) = 60.
  mono <- matrix(rep(c(1, 2, 3, 4), each = 1), nrow = 3, ncol = 4, byrow = TRUE,
                 dimnames = list(NULL, paste0("s", 1:4)))
  check("a perfectly monotone sweep hits the maximum total", trend_score(mono)$total, 90)
  rev4 <- mono[, 4:1]
  colnames(rev4) <- paste0("s", 1:4)
  check("a perfectly reversed sweep hits the minimum total", trend_score(rev4)$total, 60)
  check_true("a monotone sweep has a positive z", trend_score(mono)$z > 0)
  check_true("a reversed sweep has a negative z", trend_score(rev4)$z < 0)
  check_true("a reversed sweep is NOT significant in the predicted direction",
             trend_score(rev4)$p_normal > 0.5)

  # An all-tied cell contributes exactly the null average and nothing else, so a
  # metric a knob cannot touch produces z = 0 rather than an error or an NaN.
  flat <- matrix(1, 6, 5, dimnames = list(NULL, paste0("s", 1:5)))
  fs <- trend_score(flat)
  check("every setting identical: rank sums are all equal", as.numeric(fs$rank_sums),
        rep(6 * 3, 5))
  check("every setting identical: the total is exactly the null average",
        fs$total, fs$expected)
  check_true("every setting identical: the tie flag is set", fs$tied)

  # Ranks inside a cell always total k(k+1)/2, ties or not. This is the check
  # trend_score itself enforces, verified here on a deliberately tied cell.
  part <- matrix(c(1, 1, 2, 3, 3, 0, 5, 5, 5, 9), nrow = 2, byrow = TRUE,
                 dimnames = list(NULL, paste0("s", 1:5)))
  check("ranks total 15 per cell even with ties",
        as.numeric(rowSums(rank_within_cells(part))), c(15, 15))

  # Pair agreement: a monotone cell agrees on all pairs, a reversed one on none,
  # an all-tied one on exactly half.
  check("pair agreement, monotone cell", concordant_pairs(c(1, 2, 3, 4, 5))$concordant, 10)
  check("pair agreement, reversed cell", concordant_pairs(c(5, 4, 3, 2, 1))$concordant, 0)
  check("pair agreement, all tied", concordant_pairs(rep(1, 5))$concordant, 5)

  # A tie has no winner. This is the bug this block exists for: `which.max`
  # returns the *first* index on a tie, so a metric the setting cannot move —
  # every arm byte-identical, which is what a transfer knob does to HOLD —
  # credited whichever column sorted first with every single cell and reported it
  # at p < 0.0001. Caught on the real DecisionHorizon sweep, where HOLD is
  # byte-identical by design.
  pk_flat <- peak_counts(flat)
  check("all settings identical: no cell has a winner", pk_flat$n, 0)
  check("all settings identical: every cell counted as tied", pk_flat$ties, 6)
  check("all settings identical: no setting is credited a win",
        sum(as.integer(pk_flat$counts)), 0)
  check_true("all settings identical: no p-value is produced",
             all(is.na(pk_flat$p_raw)))
  check("all settings identical: nothing is strictly worst",
        strictly_extreme(flat, 1, "min"), 0)
  check("all settings identical: nothing is strictly best",
        strictly_extreme(flat, 5, "max"), 0)

  # A partial tie for top loses only its own cell, not the whole table.
  half_tied <- rbind(c(1, 2, 3, 4, 5), c(5, 1, 1, 1, 5), c(1, 1, 1, 1, 9))
  colnames(half_tied) <- paste0("s", 1:5)
  pk_half <- peak_counts(half_tied)
  check("one tied cell of three: two cells decided, one tied",
        c(pk_half$n, pk_half$ties), c(2, 1))
  check("one tied cell of three: the fifth setting wins both decided cells",
        as.integer(pk_half$counts), c(0, 0, 0, 0, 2))

  hr()
  note("Self-test: the worked example, end to end")
  td <- file.path("stats", "testdata", "minutes_half_life_cells.csv")
  if (!file.exists(td)) {
    note("  FAIL  ", td, " is missing — run from the repository root")
    failures <- failures + 1
  } else {
    # read_cells rather than a local read: the self-test's fixture is a cells
    # file, so it must go through the same reader and the same contract checks as
    # a real one. A self-test reading its input differently from production is
    # this record's "a diagnostic must never carry its own copy".
    d <- read_cells(td)
    d <- d[!d$infeasible, ]
    ord <- c("flat (no recency)", "half-life 2", "half-life 4 (ships)",
             "half-life 8", "half-life 20")
    cellwise <- function(metric) {
      m <- tapply(d[[paste0(metric, "_per_gw")]],
                  list(paste(d$season, d$start_gw), d$variant), mean)
      m[, ord, drop = FALSE]
    }
    seasonwise <- function(metric) {
      m <- tapply(d[[paste0(metric, "_per_gw")]], list(d$season, d$variant), mean)
      m[, ord, drop = FALSE]
    }

    pc <- cellwise("policy")
    tp <- trend_score(pc)
    check("POLICY, 24 cells: 24 cells and 5 settings", c(tp$n, tp$k), c(24, 5))
    check("POLICY, 24 cells: rank sums 53 / 55 / 84.5 / 79.5 / 88",
          as.numeric(tp$rank_sums), c(53, 55, 84.5, 79.5, 88))
    check("POLICY, 24 cells: rank sums total 360", sum(tp$rank_sums), 360)
    check("POLICY, 24 cells: trend score 1174.5 against an average of 1080",
          c(tp$total, tp$expected), c(1174.5, 1080))
    check("POLICY, 24 cells: spread 24.4949, z = 3.858",
          c(tp$sd, tp$z), c(24.4948974, 3.8579463), tol = 1e-6)

    pk <- peak_counts(pc)
    check("POLICY, 24 cells: the peak sits at 4 in 9 cells, at 20 in 6, at flat in 4",
          as.integer(pk$counts), c(4, 2, 9, 3, 6))
    check_true("POLICY, 24 cells: no setting's win count clears chance after Holm",
               all(pk$p_holm > 0.05))

    lo <- peak_counts(pc, "min")
    check("POLICY, 24 cells: flat is the worst setting in 12 of 24 cells",
          as.integer(lo$counts)[1], 12)

    ps <- seasonwise("policy")
    worst_by_season <- apply(ps, 1, function(r) which.min(r) == 1)
    check("POLICY, seasons: flat is the worst setting in only 2 of the 4 seasons",
          sum(worst_by_season), 2)
    tps <- trend_score(ps)
    check("POLICY, seasons: trend score 208 against an average of 180",
          c(tps$total, tps$expected), c(208, 180))
    check("POLICY, seasons: exact p-value 0.00171",
          exact_trend_p(tps$total, tps$n, tps$k), 0.001712, tol = 1e-5)

    hc <- cellwise("hold")
    th <- trend_score(hc)
    check("HOLD, 24 cells: rank sums 75.5 / 59 / 74 / 71 / 80.5",
          as.numeric(th$rank_sums), c(75.5, 59, 74, 71, 80.5))
    check("HOLD, 24 cells: z = 0.898, so the same shape is weak here",
          th$z, 0.8981462, tol = 1e-6)
  }

  hr()
  if (failures == 0) {
    note("all self-test checks passed")
    quit(status = 0)
  }
  note(failures, " self-test check(s) FAILED")
  quit(status = 1)
}

# ===========================================================================
# Main
# ===========================================================================

if (length(paths) == 0) {
  stop("give at least one cells CSV, or --selftest; see the header of this file")
}

# `read_cells_all` is in cells_common.R, and so are `as_flag`, the contract checks
# and the block and cell keys this script used to build for itself.
cells <- read_cells_all(paths)

METRIC_COLS <- c("policy", "hold", "hold_fixedcap", "hold_nocap",
                 "frozen", "frozen_captain", "weekly")
for (col in c("variant_index", "start_gw", "weeks", "moves", "hits",
              paste0(METRIC_COLS, "_per_gw"))) {
  if (!is.null(cells[[col]])) {
    cells[[col]] <- suppressWarnings(as.numeric(cells[[col]]))
  }
}
# `block` and `cell` come from read_cells. ⚠️ `cell` is now keyed on
# `(run_id, sweep, season, start_gw)` rather than `season@start_gw`, which changes
# the infeasible drop below on a MULTI-BLOCK file and changes it in the right
# direction: an infeasible cell in one sweep no longer drops the same season and
# entry point from a different sweep, which is a different comparison. `label` is
# the readable form and is what the note prints.

note("")
note("Read ", nrow(cells), " cells from ", length(paths), " file(s): ",
     length(unique(cells$block)), " sweep(s).")

# An infeasible cell is a variant that could not field a legal fifteen. Dropping
# it silently would read downstream as a comparison on fewer cells rather than as
# a variant that failed — and for a ranking it is worse than that, because the
# remaining settings would be ranked over a different set of cells than each
# other. So the *cell* is dropped from every setting, not just from the one that
# failed, and how many is said out loud.
if (any(cells$infeasible)) {
  bad_cells <- unique(cells$cell[cells$infeasible])
  # Listed as labels, counted as labels. Counting `cell` while listing `label`
  # made the two disagree on a multi-block file, where one label can name several
  # cells — a count that does not match its own list.
  bad_labels <- unique(cells$label[cells$infeasible])
  note("  !!  ", sum(cells$infeasible), " infeasible row(s) in ",
       length(bad_cells), " cell(s) (", length(bad_labels),
       " season/entry point(s)). A ranking needs every setting present in ",
       "every cell,")
  note("      so those whole cells are dropped rather than just the failing arm: ",
       paste(utils::head(bad_labels, 8), collapse = ", "))
  cells <- cells[!(cells$cell %in% bad_cells), ]
}

# --- resolving the predicted order -----------------------------------------
#
# This is where pre-specification is enforced. Without an order there is no trend
# test, and the script says so rather than inventing one from the data.

resolve_order <- function(spec, labels, d, what = "--order") {
  labels <- unique(labels)
  if (identical(spec, "index")) {
    # Read from this sweep's own rows, not from the whole file: two sweeps in one
    # file can use the same arm label at different positions, and picking up the
    # other sweep's index would silently reorder the ladder.
    idx <- vapply(labels, function(l) d$variant_index[match(l, d$variant)][1],
                  numeric(1))
    return(labels[order(idx)])
  }
  if (identical(spec, "numeric") || identical(spec, "numeric-desc")) {
    pat <- "-?[0-9]+(\\.[0-9]+)?"
    hits <- gregexpr(pat, labels)
    counts <- vapply(hits, function(g) sum(g > 0), numeric(1))
    if (any(counts != 1)) {
      fail(what, "=numeric needs exactly one number in every arm label; ",
           "these have ", paste(unique(counts), collapse = "/"), ": ",
           paste(labels[counts != 1], collapse = " | "),
           ". Pass the order explicitly instead.")
    }
    x <- as.numeric(vapply(regmatches(labels, regexpr(pat, labels)), identity,
                           character(1)))
    if (anyDuplicated(x) > 0) {
      fail(what, "=numeric found duplicate numbers across arms: ",
           paste(labels, collapse = " | "))
    }
    return(labels[order(x, decreasing = identical(spec, "numeric-desc"))])
  }
  wanted <- trimws(strsplit(spec, ",")[[1]])
  resolved <- vapply(wanted, function(w) {
    exact <- labels[labels == w]
    if (length(exact) == 1) return(exact)
    part <- labels[grepl(w, labels, fixed = TRUE)]
    if (length(part) == 1) return(part)
    if (length(part) == 0) {
      fail(what, ": no arm matches '", w, "'. Arms present: ",
           paste(labels, collapse = " | "))
    }
    fail(what, ": '", w, "' matches ", length(part), " arms (",
         paste(part, collapse = " | "), "). Give more of the label.")
  }, character(1))
  if (anyDuplicated(resolved) > 0) {
    fail(what, ": two entries resolved to the same arm: ",
         paste(resolved[duplicated(resolved)], collapse = " | "))
  }
  unname(resolved)
}

# --- the per-cell and per-season tables ------------------------------------
#
# One value per (cell, setting) and one per (season, setting). Both are levels,
# not differences: an ordering is a statement about levels, and there is no
# baseline arm in it. `tapply` with mean over a single row is that row's value;
# the mean is there so a duplicated row would be averaged rather than silently
# taken as whichever came last.

score_table <- function(d, metric, by) {
  col <- paste0(metric, "_per_gw")
  if (is.null(d[[col]])) return(NULL)
  key <- if (by == "cell") d$cell else d$season
  m <- tapply(d[[col]], list(key, d$variant), mean)
  if (is.null(m)) return(NULL)
  m
}

order_table <- function(m, ord) {
  if (is.null(m)) return(NULL)
  if (!all(ord %in% colnames(m))) return(NULL)
  m <- m[, ord, drop = FALSE]
  keep <- apply(m, 1, function(r) all(!is.na(r)))
  if (!all(keep)) {
    note("  !!  dropping ", sum(!keep), " row(s) with a missing setting")
  }
  m[keep, , drop = FALSE]
}

report_trend <- function(m, ord, unit_label, exact = TRUE) {
  # Too few units is a fact about the sweep, not a programmer error, so it is
  # reported and skipped rather than aborting a report that has already printed
  # half its tables. A killed sweep is the usual cause.
  if (is.null(m) || nrow(m) < 2 || ncol(m) < 3) {
    note("")
    note("  ", unit_label, ": only ", if (is.null(m)) 0 else nrow(m),
         " usable unit(s) and ", if (is.null(m)) 0 else ncol(m),
         " setting(s) — a trend test needs at least 2 and 3. Skipped.")
    return(NULL)
  }
  ts <- trend_score(m)
  note("")
  note("  ", unit_label, ": ", ts$n, " units x ", ts$k, " settings")
  note("    rank sum per setting, in the predicted order (worst first):")
  for (i in seq_len(ts$k)) {
    note(sprintf("      %-28s %8s", ord[i], fmt(ts$rank_sums[i], 1)))
  }
  note("    a setting that were always the best would score ", ts$n * ts$k,
       "; always the worst, ", ts$n)
  note(sprintf("    trend score %s   null average %s   null spread %s   z = %+.3f",
               fmt(ts$total, 1), fmt(ts$expected, 1), fmt(ts$sd, 2), ts$z))
  p_exact <- NA_real_
  if (exact && !ts$tied) {
    p_exact <- exact_trend_p(ts$total, ts$n, ts$k)
    note("    p-value, exact by enumerating every ordering: ", fmt(p_exact, 5))
    note("    p-value, bell-curve approximation:            ", fmt(ts$p_normal, 5))
  } else if (ts$tied) {
    note("    p-value, bell-curve approximation: ", fmt(ts$p_normal, 5))
    note("    (some cells contain ties, so the exact enumeration does not apply.")
    note("     Ties make the true spread smaller than the formula, so this ",
         "p-value is")
    note("     conservative — the effect is at least this strong, not at most.)")
  }
  note("    Only a large positive z is evidence for the predicted order. A ",
       "negative z")
  note("    says the units prefer the reverse, which refutes the prediction.")
  invisible(list(ts = ts, p_exact = p_exact))
}

dir.create(opt_out, showWarnings = FALSE, recursive = TRUE)
out_rows <- list()

for (b in unique(cells$block)) {
  d <- cells[cells$block == b, ]
  labels <- unique(d$variant)
  present <- METRIC_COLS[vapply(METRIC_COLS, function(m) {
    col <- d[[paste0(m, "_per_gw")]]
    !is.null(col) && any(!is.na(col))
  }, logical(1))]
  if (!is.null(opt_metrics)) present <- intersect(opt_metrics, present)
  if (length(present) == 0) next

  hr()
  note("Sweep: ", b)
  note("Settings: ", paste(labels, collapse = " | "))

  # How many cells each setting has. A sweep that was killed part-way — or one
  # still running — leaves later arms short or absent entirely, and an ordering
  # computed over whichever arms finished is a different experiment from the one
  # that was launched. Nothing else in this script can notice an arm that never
  # started, because it simply is not in the data.
  per_arm <- table(d$variant)
  if (length(unique(as.integer(per_arm))) > 1) {
    note("")
    note("  !!  the arms do not have the same number of cells, so this sweep is")
    note("      partial or was killed. Cells per setting:")
    for (i in seq_along(per_arm)) {
      note("        ", sprintf("%-28s %d", names(per_arm)[i],
                               as.integer(per_arm)[i]))
    }
    note("      Everything below is computed on the cells that exist. A ladder")
    note("      missing an arm is NOT the ladder that was launched — check the")
    note("      sweep log before quoting any of it.")
  } else {
    note("Cells per setting: ", as.integer(per_arm)[1])
  }

  if (is.null(opt_order)) {
    note("")
    note("No --order given, so the trend test is NOT run. That is deliberate: a")
    note("predicted order arrived at by looking at these numbers makes the p-value")
    note("meaningless, so the order has to be committed to on the command line.")
    note("Pass --order=numeric to order the arms by the number in each label,")
    note("--order=index for the order the sweep declared them in, or a")
    note("comma-separated list of labels from worst-predicted to best-predicted.")
    note("The peak distribution below needs no predicted order and is still shown.")
    ord <- NULL
  } else {
    ord <- resolve_order(opt_order, labels, d)
    off <- setdiff(labels, ord)
    note("")
    note("Predicted order, worst to best (committed in advance, on the command line):")
    for (i in seq_along(ord)) note("  ", i, ". ", ord[i])
    if (length(off) > 0) {
      note("")
      note("  !!  ", length(off), " arm(s) are not in the predicted order and are")
      note("      excluded from the trend test: ", paste(off, collapse = " | "))
      note("      Dropping an arm *after* seeing the data invalidates the p-value.")
      note("      If this sweep mixes a ladder with off-ladder arms, emit them as")
      note("      separate sweeps instead.")
    }
  }

  for (metric in present) {
    note("")
    note("=== ", toupper(metric),
         switch(metric,
                policy = "  (the weekly transfer decision)",
                hold = "  (the opening fifteen held all season; scoring constants belong here)",
                ""))

    cell_m <- score_table(d, metric, "cell")
    seas_m <- score_table(d, metric, "season")

    # A metric that is identical across every setting in every cell has no
    # ordering to test, and printing four tables of zeroes and half-agreements
    # invites someone to read one of them as a result. Said once instead. This is
    # the normal state for a transfer-only knob measured on HOLD, and it is the
    # invariance check passing.
    all_equal <- !is.null(cell_m) && ncol(cell_m) >= 2 &&
      all(apply(cell_m, 1, function(r) {
        all(is.na(r)) || max(r, na.rm = TRUE) == min(r, na.rm = TRUE)
      }))
    if (all_equal) {
      note("")
      note("  Every setting scores exactly the same in every cell, so there is no")
      note("  ordering here to test and nothing below would mean anything. For a")
      note("  knob that only touches the transfer decision this is the expected")
      note("  result on HOLD — the opening fifteen makes no transfers — and it is")
      note("  the invariance holding rather than a failed measurement.")
      next
    }

    if (!is.null(ord)) {
      cm <- order_table(cell_m, ord)
      sm <- order_table(seas_m, ord)
      note("")
      note("  THE TREND TEST — do the units keep putting the settings in the ",
           "predicted order?")
      note("  Read the season row and not the cell row. The six entry points ",
           "inside a season")
      note("  replay the same football, so 24 cells are not 24 independent ",
           "measurements, and")
      note("  the cell figure is optimistic by however much the seasons really ",
           "differ.")
      r_cells <- report_trend(cm, ord, "all cells, treated as independent (OPTIMISTIC)")
      r_seas <- report_trend(sm, ord, "season means, the honest unit")

      for (lv in list(list("cells", r_cells), list("seasons", r_seas))) {
        r <- lv[[2]]
        if (is.null(r)) next
        out_rows[[length(out_rows) + 1]] <- data.frame(
          block = b, metric = metric, level = lv[[1]],
          n = r$ts$n, k = r$ts$k, total = r$ts$total,
          expected = r$ts$expected, sd = r$ts$sd, z = r$ts$z,
          p_normal = r$ts$p_normal, p_exact = r$p_exact,
          predicted_order = paste(ord, collapse = " < "),
          stringsAsFactors = FALSE)
      }

      # --- per-season ordering consistency: the brake on the trend test ------
      #
      # Guarded, because a partial or killed sweep can leave one usable season and
      # every table below it would then be a statement about one number. Skipping
      # is reported by the trend test above; nothing here needs to repeat it.
      if (!is.null(sm) && nrow(sm) >= 2 && ncol(sm) >= 3) {
        note("")
        note("  DOES EACH SEASON SEPARATELY REPRODUCE THE ORDER?")
        note("  Of the ", ncol(sm) * (ncol(sm) - 1) / 2,
             " pairs of settings, how many does one season put in the predicted")
        note("  relative order. All of them is perfect agreement; half is a coin ",
             "toss.")
        note("")
        note(sprintf("    %-10s %14s   %-24s %-24s", "season", "pairs agreeing",
                     "its own best", "its own worst"))
        agree <- numeric(0)
        npairs <- ncol(sm) * (ncol(sm) - 1) / 2
        # "tied" rather than a name wherever the extreme is shared, for the reason
        # peak_counts explains: naming the first column would invent a winner.
        name_extreme <- function(r, which) {
          v <- if (which == "max") max(r) else min(r)
          at <- which(r == v)
          if (length(at) > 1) paste0("tied (", length(at), " settings)")
          else colnames(sm)[at]
        }
        for (s in rownames(sm)) {
          cp <- concordant_pairs(as.numeric(sm[s, ]))
          agree <- c(agree, cp$concordant)
          note(sprintf("    %-10s %14s   %-24s %-24s", s,
                       paste0(fmt(cp$concordant, 1), " of ", cp$pairs),
                       name_extreme(sm[s, ], "max"),
                       name_extreme(sm[s, ], "min")))
        }
        half <- npairs / 2
        # How often one season would beat half the pairs by luck alone. Computed
        # by listing every ordering of the settings rather than assumed to be 50%:
        # a single ordering can land exactly on half, and those cases belong to
        # neither tail.
        conc <- apply(permutations_of(ncol(sm)), 1,
                      function(r) concordant_pairs(r)$concordant)
        chance_beat <- mean(conc > half)
        nbeat <- sum(agree > half)
        note("")
        note("    seasons agreeing on more than half the pairs: ", nbeat, " of ",
             nrow(sm))
        note("    by luck alone one season beats half the pairs ",
             fmt(100 * chance_beat, 1), "% of the time, so ", nbeat, " of ",
             nrow(sm), " is p = ",
             fmt(stats::binom.test(nbeat, nrow(sm), chance_beat,
                                   "greater")$p.value, 4))
        note("    A trend test can look strong while the individual seasons ",
             "disagree, because")
        note("    it pools them. If this column is 2 of 4, the order is not ",
             "established.")

        # --- the specific extremum claims -----------------------------------
        #
        # "The worst setting really is the worst" is the claim most sweep
        # write-ups actually make, and it is much narrower than the whole order.
        # Counting it is one binomial test, so it is worth doing separately.
        note("")
        note("  THE NARROW CLAIMS: is the predicted-worst really worst, and the ",
             "predicted-best really best?")
        for (lvl in c("cells", "seasons")) {
          mm <- if (lvl == "cells") cm else sm
          if (is.null(mm) || nrow(mm) < 2) next
          n <- nrow(mm)
          k <- ncol(mm)
          # Strictly worst and strictly best: a shared extreme does not count.
          w <- strictly_extreme(mm, 1, "min")
          bb <- strictly_extreme(mm, k, "max")
          pw <- stats::binom.test(w, n, 1 / k, "greater")$p.value
          pb <- stats::binom.test(bb, n, 1 / k, "greater")$p.value
          note(sprintf("    %-8s %-30s %2d of %2d  (chance %s)  p = %s", lvl,
                       paste0("'", substr(ord[1], 1, 22), "' is worst"), w, n,
                       fmt(n / k, 1), fmt(pw, 4)))
          note(sprintf("    %-8s %-30s %2d of %2d  (chance %s)  p = %s", "",
                       paste0("'", substr(ord[k], 1, 22), "' is best"), bb, n,
                       fmt(n / k, 1), fmt(pb, 4)))
        }
        note("    These are two more tests on the same data and are not adjusted")
        note("    against each other or against the trend test. A narrow claim ",
             "that clears")
        note("    0.05 here while the trend test does not is one of four p-values ",
             "in this")
        note("    block, so read it as suggestive.")
      }
    }

    # --- where the peak is, not how high ---------------------------------
    #
    # Needs no predicted order, so it runs either way. This is the direct answer
    # to "a single argmax out of five swept values is not evidence": report the
    # distribution and let it say whether the peak is located at all.
    cm_free <- cell_m
    if (!is.null(cm_free)) {
      keep <- apply(cm_free, 1, function(r) all(!is.na(r)))
      cm_free <- cm_free[keep, , drop = FALSE]
    }
    if (!is.null(cm_free) && nrow(cm_free) >= 2 && ncol(cm_free) >= 2) {
      pk <- peak_counts(cm_free)
      note("")
      note("  WHERE THE PEAK IS, cell by cell")
      if (pk$n == 0) {
        # Every cell tied. For a transfer knob measured on HOLD this is the
        # expected and desirable state, and it must not be dressed up as a
        # winner — see peak_counts.
        note("  No cell has a single winner: every setting scores exactly the same ",
             "in all ", pk$n_total)
        note("  cells. This metric does not respond to the setting at all. For a ",
             "transfer knob")
        note("  measured on HOLD that is the invariance holding, not a failed ",
             "measurement.")
      } else {
      note("  The winning setting in each of the ", pk$n,
           " cells that have one. If one setting really is")
      note("  best it should win most of them; a scatter across settings means the ",
           "peak is")
      note("  unresolved, and saying so is more honest than naming the setting with ",
           "the")
      note("  highest average.")
      if (pk$ties > 0) {
        note("  ", pk$ties, " of ", pk$n_total, " cells are excluded because two or ",
             "more settings share the top")
        note("  score there, so those cells have no winner to attribute.")
      }
      note("")
      note(sprintf("    %-28s %7s %10s %9s %9s", "setting", "wins", "of",
                   "p raw", "p holm"))
      for (i in seq_along(pk$counts)) {
        note(sprintf("    %-28s %7d %10d %9s %9s", names(pk$counts)[i],
                     as.integer(pk$counts)[i], pk$n, fmt(pk$p_raw[i], 4),
                     fmt(pk$p_holm[i], 4)))
      }
      note("    chance, if every setting were equally good: ", fmt(pk$chance, 1),
           " of ", pk$n)
      top <- names(pk$counts)[which.max(as.integer(pk$counts))]
      if (all(pk$p_holm > 0.05)) {
        note("    VERDICT: no setting wins more cells than chance would give. The ",
             "peak is")
        note("    unresolved — '", top, "' has the most wins and that is not ",
             "evidence.")
      } else {
        note("    VERDICT: '", top, "' wins more cells than chance would give ",
             "(after adjusting")
        note("    for having asked about all ", pk$k, " settings).")
      }

      # The same, per season, plus the grid — because a peak that moves between
      # seasons and a peak that moves between entry points within a season are
      # different problems, and only the grid shows which.
      name_peak <- function(m, key) {
        r <- m[key, ]
        at <- which(r == max(r))
        if (length(at) > 1) "tied" else colnames(m)[at]
      }
      if (!is.null(seas_m)) {
        sk <- vapply(rownames(seas_m), function(s) name_peak(seas_m, s),
                     character(1))
        note("")
        note("    peak per season: ",
             paste(paste0(names(sk), " -> ", sk), collapse = ";  "))
      }
      note("")
      note("    winner in every cell (rows are seasons, columns entry gameweeks):")
      grid_rows <- unique(d$season)
      grid_cols <- sort(unique(d$start_gw))
      note(sprintf("      %-10s %s", "",
                   paste(sprintf("%-22s", paste0("GW", grid_cols)), collapse = "")))
      # The row key is resolved THROUGH THE DATA rather than rebuilt from the
      # season and gameweek. `cell` is keyed on `(run_id, sweep, season,
      # start_gw)`, so reconstructing `season@start_gw` here silently matched
      # nothing and printed a grid of dashes — which is what it did until
      # 2026-08-14, the moment the shared reader widened the key.
      #
      # On a file with one block the map is one-to-one. On a multi-block file a
      # label can name several cells and this grid has no column for the
      # difference, so the ambiguity is said out loud rather than resolved
      # arbitrarily.
      cell_of <- split(unique(d$cell[order(d$cell)]),
                       unique(d$label[order(d$cell)]))
      for (s in grid_rows) {
        vals <- vapply(grid_cols, function(g) {
          keys <- cell_of[[paste(s, g, sep = "@")]]
          if (is.null(keys)) return("-")
          if (length(keys) > 1) return("(ambiguous)")
          if (!(keys %in% rownames(cm_free))) return("-")
          substr(name_peak(cm_free, keys), 1, 21)
        }, character(1))
        note(sprintf("      %-10s %s", s, paste(sprintf("%-22s", vals), collapse = "")))
      }
      }
    }
  }

  # --- the mediator ---------------------------------------------------------
  #
  # A knob's first-order effect is often something other than points. A transfer
  # gate changes how many transfers get made, and that count is measured almost
  # without noise — it is an integer the harness observes directly rather than a
  # points total filtered through a season of football.
  #
  # The value of checking it is that it tells two very different situations
  # apart. If the count moves monotonically with the setting and points do not
  # move at all, the knob is working and the points really are flat. If neither
  # moves, the knob may not be doing anything and the experiment is broken. Four
  # seasons of points cannot distinguish those; the count can.
  #
  # The test is the same trend test, on the mediator instead of the points, and
  # the p-value is two-sided here: for points the prediction carries a direction,
  # while for the mediator the claim is only "it moves in step", so a strong
  # monotone *decrease* counts just as much as an increase.
  if (!is.null(opt_mediator)) {
    hr()
    note("MEDIATOR: ", opt_mediator, " — a quantity the knob controls directly")
    if (is.null(cells[[opt_mediator]])) {
      note("  !!  no column '", opt_mediator, "' in the cells file. Available: ",
           paste(names(cells), collapse = ", "))
    } else {
      mord <- if (!is.null(opt_mediator_order)) {
        resolve_order(opt_mediator_order, labels, d, "--mediator-order")
      } else ord
      if (is.null(mord)) {
        note("  !!  needs --order (or --mediator-order) to know what sequence to ",
             "test against.")
      } else {
        mm <- tapply(d[[opt_mediator]], list(d$cell, d$variant), mean)[, mord,
                                                                      drop = FALSE]
        keep <- apply(mm, 1, function(r) all(!is.na(r)))
        mm <- mm[keep, , drop = FALSE]
        ts <- trend_score(mm)
        note("")
        note("  mean ", opt_mediator, " per setting, in the predicted order:")
        for (i in seq_len(ncol(mm))) {
          note(sprintf("    %-28s %10s   rank sum %8s", mord[i],
                       fmt(mean(mm[, i]), 2), fmt(ts$rank_sums[i], 1)))
        }
        p_two <- 2 * min(pnorm(ts$z), pnorm(ts$z, lower.tail = FALSE))
        note("")
        note(sprintf("  trend score %s against a null average of %s, z = %+.3f, ",
                     fmt(ts$total, 1), fmt(ts$expected, 1), ts$z),
             "two-sided p = ", fmt(p_two, 5))
        note("  Two-sided on purpose: the claim about a mediator is only that it ",
             "moves in step")
        note("  with the setting, so a monotone fall counts as much as a monotone ",
             "rise.")
        note("")
        if (abs(ts$z) >= 3 && !is.null(ord)) {
          note("  The mediator moves monotonically. If the points columns above are ",
               "flat, that")
          note("  is evidence the points really ARE flat — the knob demonstrably ",
               "did something,")
          note("  and what it did was not worth points. It is not a failed ",
               "measurement.")
        } else {
          note("  The mediator does not move monotonically, so a flat points ",
               "column above is")
          note("  ambiguous: it may be a knob with no effect, or a knob that is ",
               "not being")
          note("  applied. Check the wiring before reading anything into the points.")
        }
        out_rows[[length(out_rows) + 1]] <- data.frame(
          block = b, metric = paste0("mediator:", opt_mediator), level = "cells",
          n = ts$n, k = ts$k, total = ts$total, expected = ts$expected,
          sd = ts$sd, z = ts$z, p_normal = p_two, p_exact = NA_real_,
          predicted_order = paste(mord, collapse = " < "),
          stringsAsFactors = FALSE)
      }
    }
  }

  # --- invariance ----------------------------------------------------------
  #
  # Falsification rather than confirmation, and much cheaper than either. Naming
  # a quantity the change must NOT move gives a check that fails on a single
  # cell, where confirming an effect on POLICY needs about 147 points a season.
  #
  # The canonical case: HOLD holds the opening fifteen all season and makes no
  # transfers, so a knob that only touches the transfer decision must leave it
  # byte-identical. If HOLD moves, the knob is leaking into scoring and the
  # experiment is measuring two things at once — which is precisely the bug that
  # made the original fixture-window sweep uninterpretable.
  if (!is.null(opt_invariant)) {
    hr()
    note("INVARIANCE: quantities this sweep must NOT move")
    note("A single cell is enough to fail. Confirming an effect on POLICY needs ",
         "about 147")
    note("points a season; refuting an invariance needs one number to differ by ",
         "anything.")
    for (metric in opt_invariant) {
      col <- paste0(metric, "_per_gw")
      if (is.null(d[[col]]) || all(is.na(d[[col]]))) {
        note("")
        note("  !!  ", metric, ": not measured in this sweep, so nothing to check.")
        next
      }
      m <- score_table(d, metric, "cell")
      keep <- apply(m, 1, function(r) all(!is.na(r)))
      m <- m[keep, , drop = FALSE]
      # Compared against the sweep's declared baseline arm, not against whichever
      # column the table happened to put first — `tapply` orders columns by label,
      # so the first column is alphabetical and naming it as the reference would
      # make the offender list depend on how the arms were spelled.
      base_label <- unique(d$variant[d$variant_index == 0])
      if (length(base_label) != 1 || !(base_label %in% colnames(m))) {
        base_label <- colnames(m)[1]
        note("")
        note("  ..  no single baseline arm found; comparing against '", base_label,
             "'.")
      }
      dev <- abs(m - m[, base_label])
      worst <- max(dev)
      note("")
      if (worst == 0) {
        note("  ok  ", toupper(metric), " is byte-identical across all ", ncol(m),
             " settings in all ", nrow(m), " cells.")
        note("      Every difference is exactly zero. This is the invariance ",
             "holding, not a")
        note("      failed measurement.")
      } else {
        bad <- which(dev > 0, arr.ind = TRUE)
        note("  FAIL ", toupper(metric), " moved. It must not.")
        note("      ", nrow(bad), " of ", nrow(m) * (ncol(m) - 1),
             " (cell, setting) pairs differ from '", base_label, "';")
        note("      largest difference ", fmt(worst, 6), " points per gameweek (",
             fmt(worst * SEASON_GWS, 2), " a season).")
        show <- bad[order(-dev[bad]), , drop = FALSE]
        # The rownames are `cell`, which carries `(run_id, sweep)`. Shown as the
        # readable label, because this listing is reached exactly when an
        # invariance FAILS — the moment someone most needs to read it — and
        # "1786593779-502178 MINHL#2 2022-23 1" is not a cell anyone recognises.
        # The block is already named above.
        row_label <- function(k) {
          i <- match(k, d$cell)
          if (is.na(i)) k else d$label[i]
        }
        for (i in seq_len(min(6, nrow(show)))) {
          note("        ", row_label(rownames(m)[show[i, 1]]), "  ",
               colnames(m)[show[i, 2]], "  ",
               fmt(dev[show[i, 1], show[i, 2]], 6))
        }
        note("      Either the knob leaks into scoring, or the two arms are not ",
             "the same")
        note("      experiment. Nothing above this line means what it claims ",
             "until that is")
        note("      explained.")
      }
      out_rows[[length(out_rows) + 1]] <- data.frame(
        block = b, metric = paste0("invariant:", metric), level = "cells",
        n = nrow(m), k = ncol(m), total = NA_real_, expected = 0,
        sd = NA_real_, z = NA_real_, p_normal = NA_real_, p_exact = NA_real_,
        predicted_order = paste0("max abs deviation = ", format(worst)),
        stringsAsFactors = FALSE)
    }
  }
}

# --- machine-readable output ----------------------------------------------

hr()
if (length(out_rows) == 0) {
  note("nothing to write: no trend test ran. Pass --order.")
} else {
  out <- do.call(rbind, out_rows)
  f <- file.path(opt_out, "shape.csv")
  write.csv(out, f, row.names = FALSE)
  note("wrote ", f)
}
note("")
note("Reading this output:")
note("  * Believe the season row of the trend test, not the cell row.")
note("  * A strong order does not endorse a setting. 'Longer is better' can hold")
note("    while the longest and second-longest settings remain indistinguishable —")
note("    check the peak distribution before moving a default.")
note("  * The predicted order had to be chosen before looking. If it was not, the")
note("    p-values above are decoration.")
note("  * A flat points column beside a monotone mediator means the points are")
note("    flat. A flat points column beside a flat mediator means nothing yet.")
