#!/usr/bin/env Rscript

# Where this harness's noise comes from, and what each remedy is worth.
#
# Usage:
#   Rscript stats/variance_components.R /tmp/cells.csv [more.csv ...]
#   Rscript stats/variance_components.R --out=stats/out/cells --power=0.8 cells.csv
#   Rscript stats/variance_components.R --selftest
#
# With no --out the tables go to stats/out/<name of the cells file>, never to
# stats/out itself. See `resolve_out` below for why that rule exists.
#
# ---------------------------------------------------------------------------
# The question
#
# `sweep_inference.R` reports a season-clustered SE and leaves it there. At four
# seasons that SE is roughly 0.75 points per gameweek, and backing the variance
# out of it implies the season-to-season spread of the *season mean effect* is
# about 1.50 — some 57 points over a season, which is larger than every constant
# in AGENTS.md. That number decides what to do next, and it has two very
# different explanations:
#
#   (a) genuine season heterogeneity — the effect really is different in
#       2024-25 than in 2023-24. Only more seasons help, and there are four.
#   (b) within-season path noise — six entry points per season disagree with
#       each other, and their disagreement leaks into the four season means.
#       More paths, or better-chosen ones, help.
#
# A season-clustered SE cannot tell them apart: it measures
# Var(season mean) = sigma2_season + sigma2_resid/G_starts and reports the sum.
# A crossed random-effects fit separates them.
#
# ---------------------------------------------------------------------------
# Why crossed and not nested
#
# `sweep_inference.R` fits `diff ~ 1 + (1|season)` and documents why it is not
# `(1|season/start_gw)`: there is one observation per (season, start point), so a
# start point nested inside a season is perfectly confounded with the residual.
# That is true and it is a different model from this one.
#
# `(1|season) + (1|start_gw)` is **crossed**, and it is identified on exactly the
# same cells: every start point appears in all four seasons and every season
# appears at all six start points, so a start-point effect common to the seasons
# is separable from a season effect common to the start points. What the residual
# absorbs is the season-by-start-point interaction, which is where the confounding
# with a nested effect actually lies. Do not read the nested caveat as saying
# start-point variance cannot be recovered — it can, and it turns out to matter.
#
# ---------------------------------------------------------------------------
# What it reports
#
# 1. The three variance components per comparison, two ways: method of moments on
#    the balanced season-by-start table, which is transparent arithmetic and may
#    go negative, and REML via lme4, which truncates at zero.
# 2. The measured season-mean spread, replacing the inferred 1.50, and the split
#    of it into (a) and (b).
# 3. Why the CR2 and mixed-model SEs disagree, in components.
# 4. The minimum detectable effect **per comparison**, in points per gameweek and
#    points per season, so "unresolved" stops being read as "refuted".
# 5. What disjoint evaluation windows would actually buy.
# 6. The Rademacher wild-cluster-bootstrap p-value floor at four clusters, by
#    enumeration rather than assertion.
#
# ---------------------------------------------------------------------------
# Why the detectable effect is reported per arm, and bracketed
#
# It used to be reported once per metric, from variance components averaged over
# the sweep's arms. That average is dominated by whichever arm disagrees most
# between seasons, and the resulting figure then describes no arm in particular.
# On the minutes-half-life sweep the `flat (no recency)` arm — the one that turns
# the feature off — contributes 72% of the pooled season variance on POLICY, and
# the pooled 147 points a season is essentially that one arm's 232 applied to
# three arms whose own figures are 80, 90 and 133. Judged per arm the same grid
# spans a thirtyfold range, and the vice-captain fallback resolves 13.
#
# So every arm gets its own row, and each is bracketed by the two defensible
# estimators rather than reduced to one:
#
#   season-clustered   Var(mean) = (sigma2_season + sigma2_resid/G) / S, on S-1 df.
#                      Correct whenever the effect genuinely differs by season.
#                      This is exactly clubSandwich's CR2 on these cells, which
#                      the cross-check below pins to machine precision.
#   start fixed        Var(mean) = sigma2_resid / (S*G), on (S-1)(G-1) df. The
#                      same six entry gameweeks are replayed in every season on
#                      purpose, so an offset between them is a fixed device that
#                      cancels from every paired comparison and should not be paid
#                      for. Valid only where sigma2_season is genuinely zero.
#
# The bracket is deliberately not collapsed by a pre-test. At four seasons the
# season F test has about 22% power against the case that would change the answer
# — a season component just large enough to double the clustered variance — so
# "the F test found nothing, therefore treat start points as fixed" is
# anti-conservative four times out of five. The power is computed rather than
# asserted, in `f_power`, and printed beside the bracket.
#
# A pooled row per metric is still emitted and labelled `scope = "pooled"`,
# because every threshold recorded in AGENTS.md was computed that way and removing
# it would orphan the record. It is no longer the only row.

suppressWarnings(suppressMessages({
  has_lmer <- requireNamespace("lme4", quietly = TRUE)
  has_cs <- requireNamespace("clubSandwich", quietly = TRUE)
}))

args <- commandArgs(trailingOnly = TRUE)
OUT_ROOT <- "stats/out"
opt_out <- NA_character_          # NA means "derive it from the cells file"
opt_power <- 0.8
opt_alpha <- 0.05
opt_force <- FALSE
opt_selftest <- FALSE
paths <- character(0)
# The scale the paired difference is taken on; see sweep_inference.R's option of
# the same name and "Per gameweek is right for a rate and wrong for an event" in
# the harness-and-inference note. It matters here as well as there: the
# minimum detectable effect is reported in season-scale points, and on an event
# count the per-gameweek figure would otherwise be multiplied by 38 a second time.
opt_scale <- "per_gw"

for (a in args) {
  if (grepl("^--out=", a)) {
    opt_out <- sub("^--out=", "", a)
  } else if (grepl("^--power=", a)) {
    opt_power <- as.numeric(sub("^--power=", "", a))
  } else if (grepl("^--alpha=", a)) {
    opt_alpha <- as.numeric(sub("^--alpha=", "", a))
  } else if (identical(a, "--force")) {
    opt_force <- TRUE
  } else if (identical(a, "--selftest")) {
    opt_selftest <- TRUE
  } else if (grepl("^--scale=", a)) {
    opt_scale <- sub("^--scale=", "", a)
    if (!opt_scale %in% c("per_gw", "per_path")) {
      stop("--scale must be per_gw or per_path, not: ", opt_scale)
    }
  } else if (grepl("^--", a)) {
    stop("unknown option: ", a)
  } else {
    paths <- c(paths, a)
  }
}

# `fail`, `as_flag`, `read_cells`, `read_sidecar`, `degenerate` and the paired
# difference are in cells_common.R. The wrapper below is named for the
# estimator whose floor it carries, not for the shared function it forwards to.
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
note <- function(...) cat(..., "\n", sep = "")
rule <- function(s) {
  note("")
  note(s)
  note(strrep("-", nchar(s)))
}

# A season is 38 gameweeks. Every per-gameweek figure in AGENTS.md is quoted
# times 38 for a season-scale number, so keep the same conversion here.
SEASON_GWS <- 38

# The suffix every metric column is read through, and the season conversion that
# goes with it. On per_path a cell total is ALREADY a season-path figure, so the
# multiplier is 1 — multiplying by 38 there is exactly the error the option
# exists to prevent.
SCALE_SUFFIX <- if (opt_scale == "per_path") "_points" else "_per_gw"
SCALE_TO_SEASON <- if (opt_scale == "per_path") 1 else SEASON_GWS

fmt <- function(x, d = 3) formatC(x, format = "f", digits = d)
sgn <- function(x, d = 3) sprintf("%+.*f", d, x)

# `as_flag` is in cells_common.R.

# --- where the tables go ----------------------------------------------------
#
# `stats/out/mde.csv` was overwritten by a twelve-cell demo run whose figures —
# a mean paired difference of 6.1 points a gameweek at t = 68 — are not a
# replayed season at all, and the accuracy snapshot read them as current for
# weeks. The default output path was the whole cause: any run without --out
# clobbered the last one, and nothing in the file said which cells it came from.
#
# So a bare run now writes to stats/out/<cells file name> and never to
# stats/out itself, and every table carries the block it was computed from.
#
# `slug` is a private default rule, not a mirror of Go's `sanitise` in
# cmd/fplagent/snapshot.go: that caller always passes an explicit --out, so the
# two derivations are never applied to the same run and cannot desynchronise the
# way the .means.csv path rule once did. They agree on the names that exist today
# because both drop the extension and keep [A-Za-z0-9_-].
slug <- function(path) {
  s <- sub("\\.csv$", "", basename(path))
  s <- gsub("[^A-Za-z0-9_-]", "_", s)
  if (nchar(s) == 0) "cells" else s
}

resolve_out <- function(out, paths) {
  if (!is.na(out)) return(out)
  file.path(OUT_ROOT, slug(paths[1]))
}

# Which sweeps an existing mde.csv was computed from, so a run into an explicit
# --out can refuse to overwrite somebody else's numbers.
blocks_in_mde <- function(dir) {
  f <- file.path(dir, "mde.csv")
  if (!file.exists(f)) return(character(0))
  old <- try(read_sidecar(f), silent = TRUE)
  if (inherits(old, "try-error") || is.null(old$block)) return(character(0))
  unique(as.character(old$block))
}

# The two metrics every sweep carries, plus the two captaincy rungs when a sweep
# scored them.
#
#   policy         HOLD plus the weekly transfer decision
#   hold           the opening fifteen held all season, eleven and captain
#                  re-picked weekly, autosubs applied — what a scoring constant
#                  is judged on, and what FPL actually pays
#   hold_fixedcap  the same, with the armband pinned to whoever would have been
#                  captained in the week the squad was bought
#   hold_nocap     the same, with nobody doubled at all
#
# The last two are candidate lower-noise *instruments*, and this script is where
# the case for one is made or refused: it reports what size of effect each could
# resolve. A smaller minimum detectable effect is necessary and not sufficient —
# an instrument that responds less to everything, signal included, has a smaller
# MDE and is worse. Read it beside the t ratios sweep_inference.R gives for the
# same arms on a change known to be real.
OPTIONAL_METRICS <- c("hold_fixedcap", "hold_nocap")

# --- paired differences, one per (season, start point) ----------------------

# `diffs_for` is in cells_common.R. This wrapper fixes the two things that are
# THIS estimator's, not the family's.
#
# ⚠️ **min_cells is 4 here and 2 everywhere else, and that is deliberate rather
# than a typo — the record wondered which.** `mom` below divides by `S - 1` and
# `G - 1`, so a two-way decomposition needs at least two seasons and two start
# points; fewer than four cells cannot produce one at all. `cells_common.R`'s 2 is
# the minimum for `sd()` to exist. Different questions, both right, which is why
# the shared function takes the floor as an argument with **no default** — a
# caller that has not decided must not inherit the other estimator's.
diffs_for_mom <- function(d, metric) {
  diffs_for(d, metric, SCALE_SUFFIX, min_cells = 4, quiet = TRUE)
}

# --- method of moments on the season-by-start table -------------------------
#
# For a balanced S x G table with one observation per cell,
#
#   d[s,g] = mu + a[s] + b[g] + e[s,g]
#
# the two-way sums of squares give unbiased estimates directly:
#
#   MS_season = G * sum(a^2) / (S-1)      E[MS_season] = sigma2_e + G*sigma2_s
#   MS_start  = S * sum(b^2) / (G-1)      E[MS_start]  = sigma2_e + S*sigma2_g
#   MS_resid  = sum(e^2) / ((S-1)(G-1))   E[MS_resid]  = sigma2_e
#
# so sigma2_s = (MS_season - MS_resid)/G and sigma2_g = (MS_start - MS_resid)/S.
# These can come out negative, which is the honest way for a component to say it
# is indistinguishable from zero. REML would report 0.000 and hide the margin.

mom <- function(df) {
  tab <- tapply(df$diff, list(df$season, df$start_gw), function(x) x[1])
  if (any(is.na(tab))) return(NULL)   # unbalanced; MoM below assumes balance
  S <- nrow(tab); G <- ncol(tab)
  grand <- mean(tab)
  a <- rowMeans(tab) - grand
  b <- colMeans(tab) - grand
  e <- tab - outer(rowMeans(tab), colMeans(tab), "+") + grand
  ms_s <- G * sum(a^2) / (S - 1)
  ms_g <- S * sum(b^2) / (G - 1)
  ms_e <- sum(e^2) / ((S - 1) * (G - 1))
  list(S = S, G = G, grand = grand,
       ms_season = ms_s, ms_start = ms_g, ms_resid = ms_e,
       v_season = (ms_s - ms_e) / G,
       v_start = (ms_g - ms_e) / S,
       v_resid = ms_e,
       # The spread the clustered SE actually sees. Var(season mean) across
       # seasons is MS_season/G exactly, and it equals sigma2_s + sigma2_e/G.
       var_season_mean = ms_s / G,
       # Is there any season heterogeneity at all? Under sigma2_s = 0 the ratio
       # MS_season / MS_resid is F on (S-1, (S-1)(G-1)). This is the test the
       # whole (a)-versus-(b) question reduces to.
       f_season = ms_s / ms_e,
       p_season = pf(ms_s / ms_e, S - 1, (S - 1) * (G - 1), lower.tail = FALSE),
       f_start = ms_g / ms_e,
       p_start = pf(ms_g / ms_e, G - 1, (S - 1) * (G - 1), lower.tail = FALSE),
       season_means = rowMeans(tab),
       resid = e)
}

# A one-sided bound on sigma2_season, because a point estimate of zero at four
# seasons is not the same as zero. theta = sigma2_s + sigma2_e/G is estimated by
# MS_season/G on S-1 df, so a chi-squared interval on theta carries straight over
# to sigma2_s once sigma2_e is subtracted. sigma2_e has (S-1)(G-1) df, five times
# as many, so treating it as known is a small approximation and is stated.
season_bound <- function(var_season_mean, v_resid, S, G, level = 0.95) {
  theta_hi <- var_season_mean * (S - 1) / qchisq(1 - level, S - 1)
  theta_lo <- var_season_mean * (S - 1) / qchisq(level, S - 1)
  c(lo = max(theta_lo - v_resid / G, 0), hi = max(theta_hi - v_resid / G, 0))
}

reml <- function(df) {
  if (!has_lmer) return(NULL)
  fit <- try(lme4::lmer(diff ~ 1 + (1 | season) + (1 | start_gw), data = df,
                        REML = TRUE,
                        control = lme4::lmerControl(check.conv.singular = "ignore")),
             silent = TRUE)
  if (inherits(fit, "try-error")) return(NULL)
  vc <- as.data.frame(lme4::VarCorr(fit))
  pick <- function(g) {
    r <- vc$vcov[vc$grp == g]
    if (length(r) == 0) 0 else r[1]
  }
  list(v_season = pick("season"), v_start = pick("start_gw"),
       v_resid = pick("Residual"))
}

# --- what an estimator can see ---------------------------------------------
#
# `sig` is the effect that would land exactly at p = alpha: anything smaller
# cannot be called significant however cleanly it was measured. `mde` is larger,
# and is the effect the design would actually *find* `power` of the time. The gap
# between them is the difference between "would clear the bar if we saw it" and
# "would reliably see it".

# The arithmetic is `sig_and_mde` in cells_common.R, shared since
# 2026-08-16 with stats/blank_run_position.R. This wrapper supplies the cells
# family's own two things — the script's alpha and power options, and the x38
# season scale — which a caller measuring a unitless ratio must not inherit. A
# wrapper that forwards is not a second implementation.
mde_row <- function(se, df, label) {
  sm <- sig_and_mde(se, df, opt_alpha, opt_power)
  tcrit <- sm[["t_crit"]]
  thresh <- sm[["sig"]]
  mdet <- sm[["mde"]]
  data.frame(estimator = label, se = se, df = df, t_crit = tcrit,
             sig_gw = thresh, sig_season = thresh * SCALE_TO_SEASON,
             mde_gw = mdet, mde_season = mdet * SCALE_TO_SEASON,
             stringsAsFactors = FALSE)
}

# The power of the season F test against the alternative that actually matters:
# a season component just large enough to multiply the clustered variance by
# `ratio`. At sigma2_s = (ratio-1) * sigma2_e / G the ratio MS_season / MS_resid
# is distributed as `ratio` times a central F, so the power is exact and needs no
# simulation.
#
# This is why the bracket is not collapsed by a pre-test. A test with 22% power
# fails to reject four times in five when the thing it is testing for is present
# and large enough to double the variance, so "it did not reject" is not licence
# to take the narrower estimator.
f_power <- function(S, G, alpha = opt_alpha, ratio = 2) {
  df1 <- S - 1
  df2 <- (S - 1) * (G - 1)
  pf(qf(1 - alpha, df1, df2) / ratio, df1, df2, lower.tail = FALSE)
}

# --- self-test --------------------------------------------------------------
#
# Run as: Rscript stats/variance_components.R --selftest
#
# No cells file, no R packages, no replay. Two kinds of check, following
# shape_inference.R: arithmetic invariants that pin the formulas on inputs whose
# answers are exact by construction, and one end-to-end worked example on the
# frozen cells file under stats/testdata, which pins the per-arm thresholds to
# numbers verified by hand. The frozen file is a real sweep's output kept only so
# the example stays reproducible — nothing should be re-derived from it.

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

  rule("Self-test: the output-directory rule")
  check_true("a bare run derives a subdirectory from the cells file",
             identical(resolve_out(NA_character_, "/tmp/cells.csv"),
                       file.path(OUT_ROOT, "cells")))
  check_true("an explicit --out is used literally",
             identical(resolve_out("stats/out/mine", "/tmp/cells.csv"),
                       "stats/out/mine"))
  check_true("the slug drops the extension and keeps the stem",
             identical(slug("stats/testdata/minutes_half_life_cells.csv"),
                       "minutes_half_life_cells"))
  check_true("punctuation is mapped, so no path can escape the root",
             identical(slug("/tmp/a b.2.csv"), "a_b_2"))
  check_true("a nameless path still lands somewhere",
             identical(slug("/tmp/.csv"), "cells"))

  rule("Self-test: the variance decomposition")
  # A table built as mu + a[s] + b[g] + e[s,g] with a and b summing to zero and e
  # a product of two zero-sum vectors, so e has zero row and column means and the
  # two-way decomposition must attribute each part exactly.
  #   sum(a^2) = 4, G = 6  ->  MS_season = 6*4/3 = 8
  #   sum(b^2) = 6, S = 4  ->  MS_start  = 4*6/5 = 4.8
  #   sum(e^2) = 4*6 = 24, 15 df  ->  MS_resid = 1.6
  a <- c(1, -1, 1, -1)
  b <- c(1, 1, 1, -1, -1, -1)
  e <- outer(a, c(1, -1, 1, -1, 1, -1))
  tab <- 7 + outer(a, b, "+") + e
  synth <- data.frame(
    season = rep(paste0("s", seq_along(a)), times = length(b)),
    start_gw = rep(seq_along(b), each = length(a)),
    diff = as.vector(tab), stringsAsFactors = FALSE)
  m <- mom(synth)
  check("grand mean", m$grand, 7)
  check("mean squares 8 / 4.8 / 1.6",
        c(m$ms_season, m$ms_start, m$ms_resid), c(8, 4.8, 1.6))
  check("components 1.0667 / 0.8 / 1.6",
        c(m$v_season, m$v_start, m$v_resid), c(6.4 / 6, 0.8, 1.6))
  check("F_season 5, F_start 3", c(m$f_season, m$f_start), c(5, 3))
  check("Var(season mean) is MS_season/G", m$var_season_mean, 8 / 6)
  check("the season-clustered SE is sqrt(MS_season/(G*S))",
        sqrt(m$var_season_mean / m$S), sqrt(8 / 6 / 4))
  # A component with nothing in it must come out negative rather than clamped:
  # that is how method of moments says "smaller than the noise around it", and
  # clamping here would hide the margin REML's 0.000 already hides.
  flat_season <- synth
  flat_season$diff <- as.vector(7 + outer(rep(0, 4), b, "+") + e)
  check_true("an absent season component reads negative, not zero",
             mom(flat_season)$v_season < 0)

  rule("Self-test: thresholds and the F test's power")
  one <- mde_row(1, 3, "x")
  check("at 3 df the critical multiple is 3.1824", one$t_crit, 3.18244631)
  check("and the 80% detectable multiple is 4.1609", one$mde_gw, 4.16091862)
  check("a season is 38 gameweeks", one$sig_season, 3.18244631 * 38, tol = 1e-5)
  check("at 15 df the critical multiple is 2.1314",
        mde_row(1, 15, "x")$t_crit, 2.13144955)
  check("the season F test has 22% power at 4 seasons and 6 start points",
        f_power(4, 6), 0.22153483, tol = 1e-6)
  # It is the season count that starves the test, and only forty seasons would
  # make it a test worth pre-testing with. There are four.
  check("and 64% at twenty seasons, 88% at forty",
        c(f_power(20, 6), f_power(40, 6)), c(0.6449684, 0.8773236), tol = 1e-6)
  check_true("more start points barely help it", f_power(4, 24) < 0.3)

  rule("Self-test: the worked example, end to end")
  td <- file.path("stats", "testdata", "minutes_half_life_cells.csv")
  if (!file.exists(td)) {
    note("  FAIL  ", td, " is missing — run from the repository root")
    failures <- failures + 1
  } else {
    # Through the shared reader, so the self-test cannot diverge from
    # production in how it reads its own fixture.
    d <- read_cells(td)
    d <- d[!d$infeasible, ]
    arms <- diffs_for_mom(d, "policy")
    ms <- lapply(arms, mom)
    names(ms) <- names(arms)
    ord <- c("flat (no recency)", "half-life 2", "half-life 8", "half-life 20")
    check_true("the four POLICY alternatives are in variant-index order",
               identical(names(ms), ord))

    # The defect this file was changed to fix: one arm dominates the average that
    # used to be applied to all four.
    vs <- vapply(ms, function(x) x$v_season, numeric(1))
    check("pooled season variance 4.566, as the old single row used",
          mean(vs), 4.56571702, tol = 1e-6)
    check("the flat arm alone is 72.2% of it", 100 * vs[[1]] / sum(vs),
          72.19426, tol = 1e-4)

    clustered <- vapply(ms, function(x) {
      mde_row(sqrt(x$var_season_mean / x$S), x$S - 1, "c")$sig_season
    }, numeric(1))
    fixed <- vapply(ms, function(x) {
      mde_row(sqrt(x$v_resid / (x$S * x$G)), (x$S - 1) * (x$G - 1),
              "f")$sig_season
    }, numeric(1))
    check("per-arm season-clustered thresholds 232 / 80 / 90 / 133",
          unname(clustered), c(231.92062, 79.98565, 89.86544, 132.95750),
          tol = 1e-4)
    check("per-arm start-fixed thresholds 50 / 43 / 42 / 50",
          unname(fixed), c(50.03536, 42.87491, 41.84465, 50.04369), tol = 1e-4)
    # The defect in one line: the pooled figure is 147 and describes none of the
    # four arms above.
    pooled <- mde_row(sqrt((mean(vs) + mean(vapply(ms, function(x) x$v_resid,
                                                   numeric(1))) / 6) / 4),
                      3, "c")$sig_season
    check("the pooled figure they used to share is 147", pooled, 146.57624,
          tol = 1e-4)
    check("only the flat arm's season F test rejects",
          vapply(ms, function(x) x$p_season < opt_alpha, logical(1)),
          c(TRUE, FALSE, FALSE, FALSE))
  }

  rule("")
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
  stop("usage: variance_components.R [--out=DIR] cells.csv ... | --selftest")
}
opt_out <- resolve_out(opt_out, paths)

# `read_cells_all` is in cells_common.R, with the flag coercion, the contract
# checks and the keys this block used to build for itself.
cells <- read_cells_all(paths)
for (col in c("variant_index", "start_gw", "weeks", "policy_per_gw", "hold_per_gw",
              paste0(OPTIONAL_METRICS, "_per_gw"))) {
  if (!is.null(cells[[col]])) {
    cells[[col]] <- suppressWarnings(as.numeric(cells[[col]]))
  }
}
# `block` and `cell` come from read_cells. ⚠️ `cell` gained `(run_id, sweep)`, and
# the separator changed from a space to the shared one — neither is visible here,
# because this script blocks before every use, but a script that did not was
# cross-pairing arms against other sweeps' baselines. See cells_common.R.
cells <- cells[!cells$infeasible, ]

METRICS <- c("policy", "hold",
             OPTIONAL_METRICS[vapply(OPTIONAL_METRICS, function(m) {
               col <- cells[[paste0(m, "_per_gw")]]
               !is.null(col) && any(!is.na(col))
             }, logical(1))])

# A run into an explicit --out must not silently replace a different sweep's
# tables. Overlapping blocks are a re-run and overwrite; disjoint ones are the
# demo-clobbers-the-record failure this guard exists for.
existing <- blocks_in_mde(opt_out)
if (length(existing) > 0 && !opt_force &&
    length(intersect(existing, unique(cells$block))) == 0) {
  stop(opt_out, "/mde.csv holds a different sweep (", paste(existing, collapse = ", "),
       ") than these cells (", paste(unique(cells$block), collapse = ", "),
       "). Write somewhere else, or pass --force to replace it.")
}

# --- per-comparison decomposition ------------------------------------------

rows <- list()
resids <- list()
for (b in unique(cells$block)) {
  d_all <- cells[cells$block == b, ]
  for (metric in METRICS) {
    if (all(is.na(d_all[[paste0(metric, "_per_gw")]]))) next
    for (arm in diffs_for_mom(d_all, metric)) {
      m <- mom(arm)
      if (is.null(m)) next
      r <- reml(arm)
      rows[[length(rows) + 1]] <- data.frame(
        block = b, metric = metric, variant = arm$variant[1],
        variant_index = arm$variant_index[1],
        n = nrow(arm), S = m$S, G = m$G, mean = m$grand,
        mom_season = m$v_season, mom_start = m$v_start, mom_resid = m$v_resid,
        reml_season = if (is.null(r)) NA_real_ else r$v_season,
        reml_start = if (is.null(r)) NA_real_ else r$v_start,
        reml_resid = if (is.null(r)) NA_real_ else r$v_resid,
        f_season = m$f_season, p_season = m$p_season,
        f_start = m$f_start, p_start = m$p_start,
        sd_season_mean = sqrt(m$var_season_mean),
        se_cluster = sqrt(m$var_season_mean / m$S),
        se_naive = sd(arm$diff) / sqrt(nrow(arm)),
        stringsAsFactors = FALSE)
      rr <- data.frame(metric = metric, variant = arm$variant[1],
                       start_gw = rep(as.numeric(colnames(m$resid)),
                                      each = nrow(m$resid)),
                       resid = as.vector(m$resid), stringsAsFactors = FALSE)
      resids[[length(resids) + 1]] <- rr
    }
  }
}
res <- do.call(rbind, rows)
resid_all <- do.call(rbind, resids)
if (is.null(res)) stop("no usable comparisons found")

rule("Variance components per comparison (points per gameweek, squared)")
note("Method of moments on the ", res$S[1], " x ", res$G[1],
     " season-by-start table; REML from lme4 in brackets.")
note("MoM may be negative — that is a component saying it is indistinguishable ",
     "from zero,")
note("which REML's 0.000 hides.")
note("")
note(sprintf("%-14s %-22s %10s %10s %10s", "metric", "variant",
             "s2_season", "s2_start", "s2_resid"))
for (i in seq_len(nrow(res))) {
  note(sprintf("%-14s %-22s %10s %10s %10s", res$metric[i],
               substr(res$variant[i], 1, 22),
               sgn(res$mom_season[i]), sgn(res$mom_start[i]),
               fmt(res$mom_resid[i])))
  note(sprintf("%-14s %-22s %10s %10s %10s", "", "  REML",
               fmt(res$reml_season[i]), fmt(res$reml_start[i]),
               fmt(res$reml_resid[i])))
}

# --- is there any season heterogeneity at all? -----------------------------
#
# This is the whole question, and it is a two-way F test. A point estimate of
# zero at four seasons does not mean zero, so the bound is reported beside it.

rule("Is the season component real? F tests and a bound")
note("F_season tests sigma2_season = 0 on (", res$S[1] - 1, ", ",
     (res$S[1] - 1) * (res$G[1] - 1), ") df; F_start tests sigma2_start = 0 on (",
     res$G[1] - 1, ", ", (res$S[1] - 1) * (res$G[1] - 1), ").")
note("The bound is a one-sided 95% interval on sd_season, in points per gameweek.")
note("")
note(sprintf("%-14s %-22s %7s %8s %7s %8s %16s", "metric", "variant",
             "F_seas", "p", "F_strt", "p", "sd_season 95% CI"))
for (i in seq_len(nrow(res))) {
  bd <- season_bound(res$sd_season_mean[i]^2, res$mom_resid[i], res$S[i], res$G[i])
  note(sprintf("%-14s %-22s %7s %8s %7s %8s %16s", res$metric[i],
               substr(res$variant[i], 1, 22),
               fmt(res$f_season[i], 2), fmt(res$p_season[i]),
               fmt(res$f_start[i], 2), fmt(res$p_start[i]),
               paste0("[", fmt(sqrt(bd["lo"]), 2), ", ",
                      fmt(sqrt(bd["hi"]), 2), "]")))
}
note("")
note("An upper bound of this width is the four-season sample speaking, not the")
note("harness: with 3 df on the season mean, a component estimated at zero and a")
note("component as large as the total are not distinguishable.")

# --- cross-check against the shipped instrument ----------------------------
#
# sqrt(MS_season/(G*S)) is the between-season SE of the grand mean. If that is
# what clubSandwich's CR2 computes on the same cells, then this decomposition and
# `sweep_inference.R`'s primary column are the same estimator taken apart, and
# neither is a second implementation of the other's quantity.

if (has_cs) {
  rule("Cross-check: does the decomposition reproduce CR2?")
  note(sprintf("%-14s %-22s %10s %10s %9s", "metric", "variant",
               "se_cluster", "se_CR2", "rel.diff"))
  worst <- 0
  for (b in unique(cells$block)) {
    d_all <- cells[cells$block == b, ]
    for (metric in METRICS) {
      if (all(is.na(d_all[[paste0(metric, "_per_gw")]]))) next
      for (arm in diffs_for_mom(d_all, metric)) {
        mine <- res$se_cluster[res$metric == metric &
                               res$variant == arm$variant[1]]
        # Only balanced arms are in `res`, and CR2 only reduces to
        # sqrt(MS_season/(G*S)) when the clusters are equal sized. An unbalanced
        # arm is skipped rather than compared against a formula that does not
        # apply to it.
        if (length(mine) == 0) next
        mine <- mine[1]
        fit <- lm(diff ~ 1, data = arm)
        ct <- clubSandwich::coef_test(fit, vcov = "CR2", cluster = arm$season,
                                     test = "Satterthwaite")
        # A metric whose paired differences are all *exactly* zero has no
        # standard error to compare — CR2 returns 0 and the relative difference
        # is 0/0. That is not a degenerate edge case to be tolerated, it is a
        # legitimate and informative state: a transfer knob measured on HOLD
        # produces it, and so does `hold_nocap` under the vice-captain fix, which
        # cannot touch a metric that doubles nobody. Reported as an exact
        # invariance rather than folded into `worst`, which would otherwise carry
        # an NaN into the comparison below and abort the whole script after the
        # replay had already been paid for.
        if (ct$SE[1] == 0 && mine == 0) {
          note(sprintf("%-14s %-22s %10s %10s %9s", metric,
                       substr(arm$variant[1], 1, 22), fmt(0, 4), fmt(0, 4),
                       "exact 0"))
          next
        }
        rel <- abs(mine - ct$SE[1]) / ct$SE[1]
        worst <- max(worst, rel)
        note(sprintf("%-14s %-22s %10s %10s %9s", metric,
                     substr(arm$variant[1], 1, 22), fmt(mine, 4),
                     fmt(ct$SE[1], 4), formatC(rel, format = "e", digits = 1)))
      }
    }
  }
  note("")
  note("worst relative difference: ", formatC(worst, format = "e", digits = 1))
  if (worst > 1e-6) {
    # A checked duplicate is a pipeline test; an unchecked one is the bug this
    # project has shipped repeatedly. So this stops rather than warning — the
    # same rule the .means.csv reproduction check follows.
    stop("se_cluster and CR2 should agree to machine precision on a balanced ",
         "design; worst relative difference ", worst, ". Either the ",
         "decomposition is wrong or the cells are not balanced.")
  } else {
    note("  ok  the season-clustered SE in sweep_inference.R is exactly")
    note("      sqrt(MS_season / (G*S)), so the components above take it apart")
    note("      rather than re-deriving it a second way.")
  }
}

# --- pooled components per metric ------------------------------------------
#
# Four alternatives in a sweep share every cell and the same baseline, so their
# components are correlated and averaging them is not four independent
# estimates. It is still a materially more stable estimate than any one arm, and
# MoM is unbiased so the average is unbiased.

pool <- do.call(rbind, lapply(METRICS, function(m) {
  s <- res[res$metric == m, ]
  if (nrow(s) == 0) return(NULL)
  data.frame(metric = m, arms = nrow(s),
             v_season = mean(s$mom_season), v_start = mean(s$mom_start),
             v_resid = mean(s$mom_resid), S = s$S[1], G = s$G[1],
             stringsAsFactors = FALSE)
}))
# A pooled variance is used to size a design, and a negative one would size it
# optimistically. Floor at zero for the projections and report both figures.
pool$v_season_used <- pmax(pool$v_season, 0)
pool$v_start_used <- pmax(pool$v_start, 0)

rule("Pooled over the sweep's alternatives, and what the 1.50 really is")
note("Var(season mean) = sigma2_season + sigma2_resid / G_starts, and the")
note("season-clustered SE reports its square root over sqrt(S). Splitting it:")
note("")
note(sprintf("%-8s %9s %9s %9s %9s %9s %9s", "metric", "sd_season",
             "sd_start", "sd_resid", "sd(smean)", "from (a)", "from (b)"))
for (i in seq_len(nrow(pool))) {
  vs <- pool$v_season_used[i]; ve <- pool$v_resid[i]; G <- pool$G[i]
  tot <- vs + ve / G
  note(sprintf("%-8s %9s %9s %9s %9s %8s%% %8s%%", pool$metric[i],
               fmt(sqrt(vs)), fmt(sqrt(pool$v_start_used[i])), fmt(sqrt(ve)),
               fmt(sqrt(tot)),
               fmt(100 * vs / tot, 1), fmt(100 * (ve / G) / tot, 1)))
}
note("")
note("sd(smean) is the measured season-to-season spread of the effect that the")
note("clustered SE implies. (a) is genuine season heterogeneity; (b) is",
     " within-season")
note("path noise leaking into the four season means.")

# --- the empirical check on 1/sqrt(weeks) ----------------------------------
#
# The design projections below need to know how residual noise scales with the
# window length. AGENTS.md records SE per gameweek rising from 0.456 at 36 weeks
# to 0.691 at 16 weeks, which is close to sqrt(36/16) = 1.50. Check it on these
# cells rather than importing it: regress log(residual^2) on log(weeks) across
# the start points, pooling the arms.

weeks_of <- unique(cells[, c("start_gw", "weeks")])
resid_all <- merge(resid_all, weeks_of, by = "start_gw")
by_gw <- do.call(rbind, lapply(split(resid_all, resid_all$start_gw), function(d) {
  data.frame(start_gw = d$start_gw[1], weeks = d$weeks[1], n = nrow(d),
             rms = sqrt(mean(d$resid^2)))
}))
by_gw <- by_gw[order(-by_gw$weeks), ]
lg <- lm(log(rms^2) ~ log(weeks), data = by_gw)
slope <- unname(coef(lg)[2])

rule("Does residual noise scale as 1 / weeks played?")
note("Residuals are the season-by-start interaction terms, pooled over arms and")
note("metrics. The null the design projections assume is a slope of -1 on")
note("log(variance) against log(weeks), i.e. SE proportional to 1/sqrt(weeks).")
note("")
note(sprintf("%9s %7s %5s %9s", "start_gw", "weeks", "n", "rms"))
for (i in seq_len(nrow(by_gw))) {
  note(sprintf("%9d %7d %5d %9s", by_gw$start_gw[i], by_gw$weeks[i],
               by_gw$n[i], fmt(by_gw$rms[i])))
}
note("")
note("fitted slope on log(variance) ~ log(weeks): ", fmt(slope, 2),
     "   (assumed -1)")
note("")
note("Read that slope as an upper bound on the length effect, not a measurement of")
note("it. Window length and information regime are the same variable in this")
note("design — a 13-gameweek window is also a GW26 entry — and AGENTS.md already")
note("records the middle regime as noisier than its length explains. The rms")
note("column is not monotone in weeks either (33 gameweeks reads above 38), so")
note("treat p = 1 as the defensible assumption and ", fmt(-slope, 2),
     " as the pessimistic one.")

# --- unequal cell lengths ---------------------------------------------------
#
# Every path runs to GW38 — `SimConfig` has `StartGW` and no `EndGW` — so a GW1
# entry contributes 38 gameweeks and a GW26 entry 13. Expressing each difference
# per gameweek played fixes the *mean*, which is the error AGENTS.md records
# correcting. It does not fix the *weighting*: each cell then counts equally
# despite a 13-gameweek cell being genuinely noisier per gameweek.
#
# If residual variance goes as 1/weeks, the efficient weight is weeks itself, and
# it is free to adopt. Both weightings are computed here so the question is
# whether it moves a verdict rather than whether it is more efficient in theory.

wls_decomp <- function(df, weighted) {
  w <- if (weighted) df$weeks else rep(1, nrow(df))
  S <- length(unique(df$season)); G <- length(unique(df$start_gw))
  mu <- sum(w * df$diff) / sum(w)
  # Season means under the same weighting, which is what a season-clustered SE
  # averages. With weights constant within a start point every season gets the
  # same weight vector, so the between-season spread is directly comparable.
  ms <- sapply(split(df, df$season), function(d) {
    ww <- if (weighted) d$weeks else rep(1, nrow(d))
    sum(ww * d$diff) / sum(ww)
  })
  fit <- lm(diff ~ factor(season) + factor(start_gw), data = df, weights = w)
  df_e <- (S - 1) * (G - 1)
  v0 <- sum(w * residuals(fit)^2) / df_e
  # Under Var(e) = sigma2_0 / w, Var(weighted season mean) = sigma2_s +
  # sigma2_0 / sum(w) over that season's cells.
  wsum <- sum(w) / S
  list(mean = mu, se_cluster = sd(ms) / sqrt(S),
       v_resid0 = v0, v_season = max(var(ms) - v0 / wsum, 0),
       v_season_raw = var(ms) - v0 / wsum)
}

rule("Unequal cell lengths: equal weighting against inverse-variance weighting")
note("Weights proportional to gameweeks played, which is the efficient weight if")
note("residual variance goes as 1/weeks. Both are season-clustered at ",
     res$S[1] - 1, " df.")
note("")
note(sprintf("%-14s %-20s %8s %8s %7s | %8s %8s %7s", "metric", "variant",
             "mean_eq", "SE_eq", "p_eq", "mean_iv", "SE_iv", "p_iv"))
wrows <- list()
for (b in unique(cells$block)) {
  d_all <- cells[cells$block == b, ]
  for (metric in METRICS) {
    if (all(is.na(d_all[[paste0(metric, "_per_gw")]]))) next
    for (arm in diffs_for_mom(d_all, metric)) {
      eq <- wls_decomp(arm, FALSE)
      iv <- wls_decomp(arm, TRUE)
      S <- length(unique(arm$season))
      pe <- 2 * pt(-abs(eq$mean / eq$se_cluster), S - 1)
      pi <- 2 * pt(-abs(iv$mean / iv$se_cluster), S - 1)
      note(sprintf("%-14s %-20s %8s %8s %7s | %8s %8s %7s", metric,
                   substr(arm$variant[1], 1, 20), sgn(eq$mean), fmt(eq$se_cluster),
                   fmt(pe), sgn(iv$mean), fmt(iv$se_cluster), fmt(pi)))
      wrows[[length(wrows) + 1]] <- data.frame(
        metric = metric, variant = arm$variant[1],
        mean_eq = eq$mean, se_eq = eq$se_cluster, p_eq = pe,
        mean_iv = iv$mean, se_iv = iv$se_cluster, p_iv = pi,
        v_season_eq = eq$v_season, v_season_iv = iv$v_season,
        v_resid0_iv = iv$v_resid0, stringsAsFactors = FALSE)
    }
  }
}
wtab <- do.call(rbind, wrows)
for (metric in unique(wtab$metric)) {
  s <- wtab[wtab$metric == metric, ]
  hq <- p.adjust(s$p_eq, "holm"); hi <- p.adjust(s$p_iv, "holm")
  note("")
  note("  Holm-adjusted, ", toupper(metric), ": equal ",
       paste(fmt(hq), collapse = " "), " | inverse-variance ",
       paste(fmt(hi), collapse = " "))
  note("  verdicts at 0.05 change: ",
       sum((hq < opt_alpha) != (hi < opt_alpha)), " of ", nrow(s))
}
note("")
note("Mean SE ratio, inverse-variance over equal: ",
     fmt(mean(wtab$se_iv / wtab$se_eq), 3))
note("Mean |mean| ratio:                          ",
     fmt(mean(abs(wtab$mean_iv) / abs(wtab$mean_eq)), 3))
note("")
note("Read those two lines together. The SE falls by ",
     fmt(100 * (1 - mean(wtab$se_iv / wtab$se_eq)), 0),
     "%, which looks like a free")
note("efficiency gain, and the point estimate moves by nearly as much — so the")
note("two weightings are not two estimates of one quantity. The effect itself")
note("varies with the entry point, and re-weighting changes which entry points")
note("the answer is about:")
note("")
for (b in unique(cells$block)) {
  d_all <- cells[cells$block == b, ]
  for (metric in METRICS) {
    if (all(is.na(d_all[[paste0(metric, "_per_gw")]]))) next
    arms <- diffs_for_mom(d_all, metric)
    if (length(arms) == 0) next
    gws <- sort(unique(arms[[1]]$start_gw))
    note("  ", toupper(metric), " mean difference by entry gameweek:")
    note(sprintf("    %-20s %s", "variant",
                 paste(sprintf("%8s", paste0("GW", gws)), collapse = "")))
    for (arm in arms) {
      cm <- sapply(gws, function(g) mean(arm$diff[arm$start_gw == g]))
      note(sprintf("    %-20s %s", substr(arm$variant[1], 1, 20),
                   paste(sprintf("%8s", sgn(cm, 2)), collapse = "")))
    }
    note("")
  }
}
note("So inverse-variance weighting is the efficient estimator of *the effect over")
note("a full season from GW1*, which is arguably the estimand that matters, since")
note("that is the season a manager plays. It is not a free improvement on the")
note("estimand already in use, and its effect on the verdicts is mixed rather than")
note("uniformly better — read the p columns above, where two HOLD arms get worse.")

if (has_lmer) {
  note("")
  note("Crossed REML with the same weights, to check sigma2_resid was not just an")
  note("average over cells of different precision:")
  note("")
  note(sprintf("  %-14s %-20s %9s %9s %9s", "metric", "variant", "s2_season",
               "s2_start", "s2_resid0"))
  for (b in unique(cells$block)) {
    d_all <- cells[cells$block == b, ]
    for (metric in METRICS) {
      if (all(is.na(d_all[[paste0(metric, "_per_gw")]]))) next
      for (arm in diffs_for_mom(d_all, metric)) {
        fit <- try(lme4::lmer(diff ~ 1 + (1 | season) + (1 | start_gw),
                              data = arm, weights = arm$weeks, REML = TRUE,
                              control = lme4::lmerControl(
                                check.conv.singular = "ignore")), silent = TRUE)
        if (inherits(fit, "try-error")) next
        vc <- as.data.frame(lme4::VarCorr(fit))
        g <- function(n) {
          r <- vc$vcov[vc$grp == n]; if (length(r) == 0) 0 else r[1]
        }
        note(sprintf("  %-14s %-20s %9s %9s %9s", metric,
                     substr(arm$variant[1], 1, 20), fmt(g("season")),
                     fmt(g("start_gw")), fmt(g("Residual"))))
      }
    }
  }
  note("")
  note("s2_resid0 is on the weighted scale: divide by a cell's gameweeks for that")
  note("cell's residual variance. At the mean window of ",
       fmt(mean(weeks_of$weeks), 1), " gameweeks it is")
  note("comparable with the unweighted s2_resid above. The season and start")
  note("components are unaffected in substance, which is the point of checking.")
}

# --- reconciling CR2 with the mixed model ----------------------------------
#
# These are not two estimates of one quantity. They target different variances,
# and the crossed components say which.
#
#   naive:            (sigma2_s + sigma2_g + sigma2_e) / (S*G), df S*G-1
#   season-clustered: (sigma2_s + sigma2_e/G) / S,              df S-1
#   diff ~ 1+(1|season): the same as naive whenever sigma2_s goes to zero,
#                        because that model has no start-point term and charges
#                        sigma2_g into the residual, then divides by S*G.
#
# The start points are not a random sample. The same six are replayed in every
# season on purpose, so a start-point main effect is a *fixed* offset that is
# identical in both arms of every comparison and cancels from the contrast. A
# variance estimator that charges for it is answering a question nobody asked:
# "what would this effect be at a randomly drawn entry gameweek".

rule("Why CR2 and the mixed model disagree")
note(sprintf("%-14s %-22s %9s %9s %9s %9s", "metric", "variant",
             "se_naive", "se_clust", "s2_start", "share"))
for (i in seq_len(nrow(res))) {
  tot <- max(res$mom_season[i], 0) + max(res$mom_start[i], 0) + res$mom_resid[i]
  note(sprintf("%-14s %-22s %9s %9s %9s %8s%%", res$metric[i],
               substr(res$variant[i], 1, 22),
               fmt(res$se_naive[i]), fmt(res$se_cluster[i]),
               sgn(res$mom_start[i]), fmt(100 * max(res$mom_start[i], 0) / tot, 1)))
}
note("")
note("'share' is the start-point main effect as a share of total cell variance.")
note("The naive SE pays for it and the season-clustered SE does not, so a large")
note("share is exactly when clustering makes the SE *smaller*.")

# --- minimum detectable effect, per comparison -----------------------------
#
# Every row here is written to mde.csv as well as printed, because the accuracy
# snapshot leads with these figures: they are what make an "unresolved" verdict
# readable as *below what this instrument can detect* rather than as *shown to
# have no effect*, a distinction this project has got wrong in both directions.
# The snapshot renderer must therefore read them rather than recompute them —
# inference lives in this script and nowhere else, and a second implementation is
# the bug class behind the two bench-weight defaults, where the measured value
# turned out not to be the one that ran.
#
# `scope` says what a row is about, and it is the column to read first:
#
#   arm     one comparison — one alternative against the baseline, on one metric.
#           This is the honest unit. Two rows per comparison, one per estimator,
#           and no `is_primary` on either: see the header for why a 22%-power
#           pre-test must not collapse the bracket.
#   pooled  the whole sweep's arms averaged, which is how every threshold in
#           AGENTS.md was computed. Kept so that record stays interpretable, and
#           labelled so it stops being mistaken for a per-comparison figure.

EST_CLUST <- "season-clustered (primary)"
EST_FIXED <- "start fixed, no season effect"
EST_NAIVE <- "naive, all cells independent"

# The bracket. `se_clust` is passed in rather than rebuilt because for an arm it
# is sqrt(MS_season/(G*S)) — clubSandwich's CR2 exactly, as the cross-check above
# pins — while for the pooled row it is built from floored components.
bracket <- function(se_clust, v_resid, S, G) {
  rbind(mde_row(se_clust, S - 1, EST_CLUST),
        mde_row(sqrt(v_resid / (S * G)), (S - 1) * (G - 1), EST_FIXED))
}

# One place that fills in the non-estimator columns, so every row in mde.csv has
# the same shape whatever its scope. A blank is a blank: NA for a quantity that
# does not apply to a scope, never a zero, since a zero p-value reads as an
# overwhelmingly significant season component.
stamp <- function(tab, scope, block, metric, variant, variant_index,
                  f_season, p_season, arms, primary, p_pooled, S, G) {
  tab$scope <- scope
  tab$block <- block
  tab$metric <- metric
  tab$variant <- variant
  tab$variant_index <- variant_index
  tab$f_season <- f_season
  tab$p_season <- p_season
  tab$f_power <- f_power(S, G)
  tab$arms <- arms
  tab$is_primary <- if (is.na(primary)) FALSE else tab$estimator == primary
  tab$p_season_pooled <- p_pooled
  tab$seasons <- S
  tab$start_gws <- G
  tab$power <- opt_power
  tab$alpha <- opt_alpha
  tab$season_gws <- SCALE_TO_SEASON
  tab$scale <- opt_scale
  tab
}

mde_rows <- list()

rule("What each comparison can detect, per arm")
note("'significant' is the effect that would land exactly at p = ", opt_alpha,
     "; 'MDE' is the effect")
note("this design would detect ", fmt(100 * opt_power, 0),
     "% of the time. Points per season is x", SCALE_TO_SEASON,
     " on the ", opt_scale, " scale.")
note("")
note("Two estimators per comparison, and the answer is the range between them:")
note("  clustered  Var = (s2_season + s2_resid/G)/S on ", res$S[1] - 1,
     " df — correct if the effect")
note("             genuinely differs by season. Conservative.")
note("  start-fix  Var = s2_resid/(S*G) on ", (res$S[1] - 1) * (res$G[1] - 1),
     " df — the same six entry gameweeks")
note("             are replayed every season on purpose, so an offset between")
note("             them cancels from the contrast. Valid only if s2_season = 0.")
note("")
note(sprintf("%-14s %-22s %9s %9s %9s %9s %7s %7s", "metric", "variant",
             "clu sig", "clu MDE", "fix sig", "fix MDE", "F_seas", "p_seas"))
for (i in seq_len(nrow(res))) {
  S <- res$S[i]; G <- res$G[i]
  tab <- bracket(res$se_cluster[i], res$mom_resid[i], S, G)
  tab <- stamp(tab, "arm", res$block[i], res$metric[i], res$variant[i],
               res$variant_index[i], res$f_season[i], res$p_season[i],
               NA_integer_, NA_character_, NA_real_, S, G)
  mde_rows[[length(mde_rows) + 1]] <- tab
  pv <- if (is.finite(res$p_season[i])) fmt(res$p_season[i]) else "—"
  fv <- if (is.finite(res$f_season[i])) fmt(res$f_season[i], 2) else "—"
  note(sprintf("%-14s %-22s %9s %9s %9s %9s %7s %7s", res$metric[i],
               substr(res$variant[i], 1, 22),
               fmt(tab$sig_season[1], 0), fmt(tab$mde_season[1], 0),
               fmt(tab$sig_season[2], 0), fmt(tab$mde_season[2], 0), fv, pv))
}
note("")
note("F_seas tests whether the season component exists at all, so a small p is")
note("the licence to read the left-hand pair and a large one is *not* licence to")
note("read the right-hand pair: at ", res$S[1], " seasons and ", res$G[1],
     " start points that test has only")
note(fmt(100 * f_power(res$S[1], res$G[1]), 0),
     "% power against a season component large enough to double the clustered")
note("variance. So it fails to reject four times in five when the thing it looks")
note("for is there. Report the range and the evidence; do not pick an end.")

# The spread across arms, which is the point of reporting them separately. These
# are order statistics of the rows just printed, not new estimates.
note("")
note(sprintf("%-14s %5s %28s %28s", "metric", "arms",
             "clustered sig/season", "start-fixed sig/season"))
for (m in unique(res$metric)) {
  s <- do.call(rbind, Filter(function(t) t$metric[1] == m && t$scope[1] == "arm",
                             mde_rows))
  cl <- s$sig_season[s$estimator == EST_CLUST]
  fx <- s$sig_season[s$estimator == EST_FIXED]
  rng <- function(v) sprintf("%7s  [%6s, %6s]", fmt(median(v), 0),
                             fmt(min(v), 0), fmt(max(v), 0))
  note(sprintf("%-14s %5d %28s %28s", m, length(cl), rng(cl), rng(fx)))
}
note("")
note("median [min, max] over the sweep's arms. A wide range is the finding: the")
note("threshold belongs to the comparison, not to the harness, and quoting one")
note("arm's figure as the harness's resolution is what this section was changed")
note("to stop.")

rule("The same thing pooled over the sweep's arms, for continuity")
note("Every threshold recorded in AGENTS.md was computed this way — variance")
note("components averaged over the arms, then one row per metric. Kept so that")
note("record stays interpretable, and *not* the figure to quote for a single")
note("comparison: the average is dominated by whichever arm disagrees most")
note("between seasons, so it can describe no arm in the sweep.")
for (i in seq_len(nrow(pool))) {
  m <- pool$metric[i]
  S <- pool$S[i]; G <- pool$G[i]
  vs <- pool$v_season_used[i]; vg <- pool$v_start_used[i]; ve <- pool$v_resid[i]
  tab <- rbind(bracket(sqrt((vs + ve / G) / S), ve, S, G),
               mde_row(sqrt((vs + vg + ve) / (S * G)), S * G - 1, EST_NAIVE))
  # is_primary is retained on the pooled rows alone, because the accuracy
  # snapshot and stats/README.md both key off it and dropping it would orphan
  # them. It is the *pooled* pre-test's answer and carries that test's 22% power,
  # which the header and the per-arm section above both say plainly.
  #
  # The strictest arm decides, not the average of the arms. Averaging p-values is
  # not a test of anything, and here it fails in the expensive direction: on
  # POLICY one arm's season F test gives p = 0.001 and the other three do not
  # reject, so the mean is 0.11 and would license treating start points as fixed
  # on a metric whose season component is demonstrably real. One arm showing
  # season heterogeneity is enough to withdraw that licence, because the arms
  # share every cell.
  fp <- suppressWarnings(min(res$p_season[res$metric == m], na.rm = TRUE))
  if (!is.finite(fp)) fp <- NA_real_
  primary <- if (!is.na(fp) && fp >= opt_alpha) EST_FIXED else EST_CLUST
  tab <- stamp(tab, "pooled", paste(unique(res$block[res$metric == m]),
                                    collapse = " + "),
               m, NA_character_, NA_real_, NA_real_, NA_real_,
               pool$arms[i], primary, fp, S, G)
  mde_rows[[length(mde_rows) + 1]] <- tab
  note("")
  note(toupper(m), " pooled over ", pool$arms[i], " arms:")
  note(sprintf("  %-30s %7s %4s %7s %9s %9s %9s %9s", "estimator", "SE", "df",
               "t_crit", "sig/gw", "sig/season", "MDE/gw", "MDE/season"))
  for (j in seq_len(nrow(tab))) {
    note(sprintf("  %-30s %7s %4d %7s %9s %9s %9s %9s", tab$estimator[j],
                 fmt(tab$se[j]), as.integer(tab$df[j]), fmt(tab$t_crit[j], 2),
                 fmt(tab$sig_gw[j]), fmt(tab$sig_season[j], 0),
                 fmt(tab$mde_gw[j]), fmt(tab$mde_season[j], 0)))
  }
}

# --- what would disjoint windows buy? -------------------------------------
#
# A disjoint design chops each season into W windows of 38/W gameweeks and
# evaluates each once. Two things follow from the components:
#
#   * Clustering by season, W makes almost no difference. Averaging W disjoint
#     windows of length 38/W is averaging the same season's football either way,
#     and under 1/weeks scaling sigma2_e(38/W)/W is constant in W. So a disjoint
#     design's season-clustered SE is the SE of one full-season window.
#   * Treating the S*W segments as independent clusters is the whole prize: it
#     takes df from S-1 to S*W-1. It is only legitimate if sigma2_season is
#     zero, because otherwise segments inside a season share a[s] and are
#     correlated by construction.

rule("What disjoint evaluation windows would buy")
L_ref <- mean(weeks_of$weeks)
note("Residual variance taken as proportional to weeks^-p, calibrated at the")
note("current design's mean window of ", fmt(L_ref, 1), " gameweeks. Two values")
note("of p: the 1/weeks the length argument predicts, and the ", fmt(-slope, 2),
     " measured above.")
note("")
note("The key arithmetic: a disjoint design evaluates each season's football")
note("exactly once, where the current nested design evaluates ",
     fmt(sum(weeks_of$weeks) / SEASON_GWS, 1), " seasons' worth")
note("per season by overlapping. At p = 1 the number of disjoint windows drops")
note("out entirely — averaging W windows of 38/W gameweeks is averaging the same")
note("season either way — so W only ever changes the cluster count.")
note("")
note("How much overlap there is, in shared gameweeks between entry points:")
ws <- weeks_of[order(-weeks_of$weeks), ]
note(sprintf("  %8s %s", "", paste(sprintf("%5d", ws$start_gw), collapse = "")))
for (i in seq_len(nrow(ws))) {
  sh <- sapply(seq_len(nrow(ws)), function(j) {
    SEASON_GWS - max(ws$start_gw[i], ws$start_gw[j]) + 1
  })
  note(sprintf("  GW%-6d %s", ws$start_gw[i],
               paste(sprintf("%5d", sh), collapse = "")))
}
note("")
note("Every window is a suffix of the season, so they are strictly nested: the")
note("GW1 and GW6 entries share 33 of 38 gameweeks and the GW26 window sits")
note("inside all five others. Building disjoint windows needs an `EndGW` that")
note("`SimConfig` does not have — it carries `StartGW` only, and every path runs")
note("to GW38.")
proj <- list()
for (i in seq_len(nrow(pool))) {
  m <- pool$metric[i]
  S <- pool$S[i]; G <- pool$G[i]
  vs <- pool$v_season_used[i]; ve <- pool$v_resid[i]
  for (p in c(1, -slope)) {
    plab <- paste0("p=", fmt(p, 2))
    proj[[length(proj) + 1]] <- cbind(
      data.frame(metric = m, p = p,
                 design = paste0("current: ", G, " nested starts"),
                 clusters = S, stringsAsFactors = FALSE),
      mde_row(sqrt((vs + ve / G) / S), S - 1, "season-clustered"))
    for (W in c(2, 3)) {
      L <- SEASON_GWS / W
      ve_L <- ve * (L_ref / L)^p
      proj[[length(proj) + 1]] <- cbind(
        data.frame(metric = m, p = p,
                   design = paste0("disjoint ", W, " x ", round(L),
                                   "gw, clustered by season, ", plab),
                   clusters = S, stringsAsFactors = FALSE),
        mde_row(sqrt((vs + ve_L / W) / S), S - 1, "season-clustered"))
      proj[[length(proj) + 1]] <- cbind(
        data.frame(metric = m, p = p,
                   design = paste0("disjoint ", W, " x ", round(L),
                                   "gw as ", S * W, " clusters, ", plab),
                   clusters = S * W, stringsAsFactors = FALSE),
        mde_row(sqrt(vs / S + ve_L / (S * W)), S * W - 1, "segment-clustered"))
    }
  }
}
proj <- do.call(rbind, proj)
proj <- proj[!duplicated(proj[, c("metric", "design")]), ]
for (m in unique(proj$metric)) {
  s <- proj[proj$metric == m, ]
  note("")
  note(toupper(m), ":")
  note(sprintf("  %-52s %6s %4s %8s %10s", "design", "SE", "df", "sig/gw",
               "sig/season"))
  for (j in seq_len(nrow(s))) {
    note(sprintf("  %-52s %6s %4d %8s %10s", s$design[j], fmt(s$se[j]),
                 as.integer(s$df[j]), fmt(s$sig_gw[j]),
                 fmt(s$sig_season[j], 0)))
  }
}
note("")
note("The segment-clustered rows are only valid where sigma2_season is zero.")
note("Where it is not, segments inside a season remain correlated and the honest")
note("cluster count stays at ", pool$S[1], ".")

# --- how many seasons would it take? ---------------------------------------
#
# The other side of the same arithmetic. Holding the design fixed and varying
# only the number of seasons, what is resolvable?

rule("Seasons needed to resolve an effect, at 80% power")
note("Solved for the smallest S with (t_crit + t_power) * SE(S) <= the target.")
note("Two axes: seasons at the current six start points, and — where the season")
note("component is zero — start points at four seasons.")
note("")
# ⚠️ Both solvers call `sig_and_mde` in cells_common.R rather than writing the
# `(t_crit + t_power) * se` product out. They used to write it out, and they were
# the two copies the 2026-08-16 consolidation MISSED: it moved the one that had a
# name (`mde_row`) and left the two that did not. The shared-quantity scan keys
# on `^name <- function`, so it could not see either, and the scan passing was
# not evidence the quantity had one implementation.
mde_of <- function(se, df) sig_and_mde(se, df, opt_alpha, opt_power)[["mde"]]
seasons_needed <- function(target, vs, ve, G, max_S = 5000) {
  for (S in 2:max_S) {
    if (mde_of(sqrt((vs + ve / G) / S), S - 1) <= target) return(S)
  }
  NA_integer_
}
starts_needed <- function(target, ve, S, max_G = 100000) {
  for (G in 2:max_G) {
    if (mde_of(sqrt(ve / (S * G)), (S - 1) * (G - 1)) <= target) return(G)
  }
  NA_integer_
}
TARGETS <- c(11, 20, 34, 57)
note(sprintf("%-8s %14s %14s %16s", "metric", "target/season", "seasons needed",
             "start points (*)"))
for (i in seq_len(nrow(pool))) {
  m <- pool$metric[i]
  vs <- pool$v_season_used[i]; ve <- pool$v_resid[i]
  S <- pool$S[i]; G <- pool$G[i]
  for (tg in TARGETS) {
    per_gw <- tg / SCALE_TO_SEASON
    ns <- seasons_needed(per_gw, vs, ve, G)
    ng <- if (vs > 0) NA_integer_ else starts_needed(per_gw, ve, S)
    note(sprintf("%-8s %14d %14s %16s", m, tg,
                 ifelse(is.na(ns), ">5000", as.character(ns)),
                 ifelse(is.na(ng), "n/a", as.character(ng))))
  }
}
note("")
note("(*) start points is only a legitimate axis where sigma2_season is zero, and")
note("it saturates the moment it is not: no number of paths beats sqrt(sigma2_s/S).")
note("Read it against the upper bound on sd_season reported above, not against the")
note("point estimate. Adding start points also widens the estimand, since each new")
note("entry gameweek is a different information regime — see the by-entry-gameweek")
note("table above, where one column carries most of HOLD's effect.")

# --- is the path axis exhausted? -------------------------------------------
#
# The season-clustered variance is (sigma2_s + sigma2_e/G)/S, so G — the number
# of start points — is in it. More paths shrink the clustered SE as well as the
# naive one; what they cannot move is the df. But the shrinkage has a floor at
# sqrt(sigma2_s/S), and how close the current design already sits to that floor
# is the whole question of whether more paths are worth buying.

rule("Is the start-point axis exhausted?")
note("Season-clustered SE against the number of start points, at ", pool$S[1],
     " seasons. The floor")
note("is sqrt(sigma2_season / S) and is reached only asymptotically.")
note("")
note(sprintf("%-8s %8s %8s %8s %8s %8s %10s", "metric", "G=6", "G=12", "G=24",
             "G=48", "floor", "% of floor"))
for (i in seq_len(nrow(pool))) {
  S <- pool$S[i]; vs <- pool$v_season_used[i]; ve <- pool$v_resid[i]
  se <- function(G) sqrt((vs + ve / G) / S)
  floor_se <- sqrt(vs / S)
  note(sprintf("%-8s %8s %8s %8s %8s %8s %9s%%", pool$metric[i],
               fmt(se(6)), fmt(se(12)), fmt(se(24)), fmt(se(48)),
               fmt(floor_se),
               fmt(100 * floor_se / se(pool$G[i]), 0)))
}
note("")
note("A '% of floor' near 100 says the paths already bought everything paths can")
note("buy and only seasons remain. Near 0 says the axis is wide open.")

# --- the Rademacher floor, by enumeration ---------------------------------

rule("Wild cluster bootstrap: the Rademacher floor at four clusters")
S <- pool$S[1]
combos <- 2^S
note("With ", S, " clusters there are 2^", S, " = ", combos,
     " Rademacher sign vectors, and the")
note("statistic is symmetric under a global flip, so the bootstrap distribution")
note("has at most ", combos / 2, " distinct absolute values. The original sample ",
     "is one of them,")
note("so the smallest attainable two-sided p is 2/", combos, " = ",
     fmt(2 / combos, 3), ".")
note("")
note("That is above ", opt_alpha, ", so a Rademacher wild cluster bootstrap at ",
     S, " clusters can")
note("never reject at the ", fmt(100 * opt_alpha, 0),
     "% level however large the effect. It must not arbitrate anything here.")

# --- write the tables -----------------------------------------------------

dir.create(opt_out, showWarnings = FALSE, recursive = TRUE)
write.csv(res, file.path(opt_out, "variance_components.csv"), row.names = FALSE)
write.csv(pool, file.path(opt_out, "variance_pooled.csv"), row.names = FALSE)
write.csv(proj, file.path(opt_out, "design_projection.csv"), row.names = FALSE)
write.csv(do.call(rbind, mde_rows), file.path(opt_out, "mde.csv"),
          row.names = FALSE)
note("")
note("wrote ", file.path(opt_out, "variance_components.csv"), ", ",
     "variance_pooled.csv, design_projection.csv and mde.csv")
note("mde.csv carries one row per (scope, metric, variant, estimator); read the")
note("`scope` column first — `arm` is a comparison, `pooled` is the whole sweep.")
