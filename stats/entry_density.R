#!/usr/bin/env Rscript

# Does densifying the entry-gameweek grid buy real resolution, or correlated cells?
#
# Usage:
#   Rscript stats/entry_density.R /tmp/density/cells.csv
#   Rscript stats/entry_density.R --metric=hold --out=stats/out/density cells.csv
#   Rscript stats/entry_density.R --selftest
#
# ---------------------------------------------------------------------------
# The claim under test
#
# AGENTS.md ("The start-point axis is exhausted on POLICY and wide open on HOLD")
# derives the value of extra entry points from
#
#   Var(mean) = (sigma2_season + sigma2_resid / G) / S
#
# with G entry points per season and S = 4 seasons, and publishes a specific
# prediction for the season-clustered SE on HOLD: 0.515 at the shipped G = 6,
# falling to 0.364, 0.257 and 0.182 at G = 12, 24 and 48.
#
# The `sigma2_resid / G` term assumes the G within-season residuals are
# **independent**. They are not obviously so. `SimConfig` carries `StartGW` and no
# `EndGW`, so every window runs to GW38 and two entry points are strictly nested:
# an entry at GW2 shares 37 of its 38 gameweeks with an entry at GW1. At the
# shipped spacing of 5 that correlation is evidently tolerable, since the shipped
# grid resolves what it resolves. At spacing 1 or 2 it may not be, and a grid of
# near-duplicates shrinks the reported SE without adding information — which is the
# trap the budget-jitter axis was rejected for.
#
# ---------------------------------------------------------------------------
# The reframing that came out of building this, which narrows the question
#
# Write the paired difference in cell (season s, entry g) as
#
#   d[s,g] = mu + a[s] + b[g] + e[s,g]
#
# — the same two-way model `variance_components.R` uses — and let the within-season
# covariance between two entry points be c(h) at gap h, with c(0) = v. On a
# balanced S x G table, method of moments gives
#
#   E[MS_season]/G = Var(season mean) = cbar + (v - cbar)/G
#   E[MS_resid]                      = v - cbar
#
# where cbar is the **average off-diagonal covariance** over the grid's pairs. So
# `sigma2_season` as that script reports it — (MS_season - MS_resid)/G — is
# estimating **cbar itself**, not a separate physical quantity. Three consequences:
#
#  1. An exchangeable within-season correlation and a genuine season effect are the
#     *same model*. They are not separable at any sample size, because both do
#     exactly one thing: make the four season means more variable. AGENTS.md's
#     formula therefore already accommodates correlated entry points, and its
#     measured sd_season = 0 on HOLD is already the statement that the average
#     within-season covariance is zero.
#  2. The measured clustered SE, sd(season means)/sqrt(S), is **unbiased whatever
#     the correlation is**. Every measured SE below is honest by construction; only
#     projections to grids that were never run can be wrong.
#  3. What is *not* accommodated, and is the only thing at risk, is **gap
#     dependence** — nearby entry points more correlated than distant ones, which
#     would make cbar grow as a grid is densified. That is separable from a season
#     effect, because it varies with the gap and a season effect does not.
#
# So the question is not "are the cells correlated" but "does the correlation depend
# on the gap", which is a strictly smaller claim and a far better-conditioned
# measurement.
#
# ---------------------------------------------------------------------------
# The two benchmarks worth having in mind
#
# rho = 0 is the formula's assumption: two entry points produce independent paths
# through the same football.
#
# The other end is not rho = 1 but a computable ceiling. If two nested windows
# scored the *same* football with the same squad, the paired difference per gameweek
# would be an average of the same weekly contributions over w1 and w2 < w1 weeks,
# and the correlation would be sqrt(w2 / w1) — 0.982 for GW1 against GW2. The reason
# it is not that high is that each entry point buys a different opening fifteen and
# then diverges. The ceiling is printed beside the measurement, because "the
# correlation is 0.09" means one thing against a ceiling of 0.98 and another against
# a ceiling of 0.10.
#
# ---------------------------------------------------------------------------
# The estimator: a variogram, read three ways
#
# The naive thing is to correlate two columns across the four seasons, and it is
# biased by a[s]: two columns share the season effect, so their correlation is
# (sigma2_a + c) / (sigma2_a + v). The *difference* of two columns kills both
# nuisance terms exactly. D[s] = d[s,g1] - d[s,g2] has the season effect cancelled
# (same season) and the start-point effect absorbed into a constant (b[g1] - b[g2]
# does not vary with s), so
#
#   gamma(h) = Var_s(D)/2 = (v1 + v2)/2 - c(h)
#
# with no assumption that v1 = v2. Three readings, and the third is the one to read:
#
#   rho        = 1 - gamma/v,  v = MS_resid + sigma2_season.
#                The absolute correlation, and noisy: sigma2_season rests on three
#                degrees of freedom and can come out negative.
#
#   excess     = 1 - gamma/MS_resid.
#                Correlation in excess of the grid's average pair. Zero on average
#                by construction, immune to sigma2_season, and precise — but it
#                assumes every column has the same variance.
#
#   exc_het    = ((v1 + v2)/2 - gamma) / sqrt(v1 v2), per-column variances.
#                The same excess without that assumption, and it matters: the
#                columns' residual variances span a factor of three or four across
#                the season, and only a *wide* pair can span the extremes, so
#                pooling one variance over every pair manufactures a gap dependence
#                exactly where the fear lies.
#
# Both excess measures are relative by construction. The two-way residual already
# has the average correlation removed, so neither can report the *level* — that
# comes from cbar/v and from the naive column correlation, both printed. The level
# and the gap dependence are different questions and only the second is well
# conditioned here.
#
# Four seasons give each pair 3 df. No single pair is worth reading; every table
# pools over the pairs at a gap.

args <- commandArgs(trailingOnly = TRUE)
opt_metric <- "hold"
opt_out <- NA_character_
opt_selftest <- FALSE
paths <- character(0)
for (a in args) {
  if (grepl("^--metric=", a)) {
    opt_metric <- sub("^--metric=", "", a)
  } else if (grepl("^--out=", a)) {
    opt_out <- sub("^--out=", "", a)
  } else if (identical(a, "--selftest")) {
    opt_selftest <- TRUE
  } else if (grepl("^--", a)) {
    stop("unknown option: ", a)
  } else {
    paths <- c(paths, a)
  }
}

# `fail`, `as_flag`, `read_cells` and the contract checks are in cells_common.R.
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
fmt <- function(x, d = 3) formatC(x, format = "f", digits = d)
SEASON_GWS <- 38

# One place that builds a row of entry_density.csv, so every row has the same shape
# whatever produced it. `kind` says which table it came from, because a projected SE
# and a measured one must never be read as the same kind of number.
emit_row <- function(block, arm, kind, label, G, spacing, rho_bar, g_eff, se,
                     se_ratio) {
  data.frame(block = block, arm = arm, metric = opt_metric, kind = kind,
             label = label, G = G, spacing = spacing, rho_bar = rho_bar,
             g_effective = g_eff, se = se, se_ratio = se_ratio,
             stringsAsFactors = FALSE)
}

# --- the arithmetic, in one place -------------------------------------------

# se_clustered is sd(season means)/sqrt(S), which is algebraically
# sqrt(MS_season/(G*S)) and is clubSandwich's CR2 on these cells —
# variance_components.R pins that identity to machine precision, so it is not
# re-derived here.
se_clustered <- function(tab) {
  m <- rowMeans(tab)
  sd(m) / sqrt(length(m))
}

two_way <- function(tab) {
  S <- nrow(tab)
  G <- ncol(tab)
  grand <- mean(tab)
  a <- rowMeans(tab) - grand
  e <- tab - outer(rowMeans(tab), colMeans(tab), "+") + grand
  ms_s <- G * sum(a^2) / (S - 1)
  ms_e <- if (G > 1) sum(e^2) / ((S - 1) * (G - 1)) else NA_real_
  list(S = S, G = G, mean = grand, resid = e,
       ms_season = ms_s, ms_resid = ms_e,
       v_season = (ms_s - ms_e) / G, v_resid = ms_e,
       f_season = ms_s / ms_e,
       p_season = pf(ms_s / ms_e, S - 1, (S - 1) * (G - 1), lower.tail = FALSE))
}

# Per-column variance and gap dependence, both from the variogram, fitted jointly.
#
# The obvious per-column estimator — the two-way residual's own column sums of
# squares — is contaminated when the columns have unequal variances, because the row
# mean it subtracts averages over every column. A quiet column borrows variance from
# a loud one, the estimates are pulled toward their mean, and the leftover
# heterogeneity reappears as a spacing effect. Verified in the self-test: on data
# with a threefold variance spread and no gap dependence at all, that route invents
# an excess spanning 0.19, which is larger than anything real this measures.
#
# The fix is to model the variogram directly. Under
#
#   gamma_ij = (v_i + v_j)/2 - c(h_ij),      c(h) = cbar + delta(h),
#
# adding a constant to every v_i and to cbar leaves every gamma unchanged, so only
# u_i = v_i - cbar is identified — the same non-identifiability as everywhere else
# here, and it is why the level cannot be read off this. Dropping the intercept and
# fitting gamma_ij = (u_i + u_j)/2 by least squares over the G(G-1)/2 pairs gives the
# u_i, and the residuals are -delta(h): the gap dependence, with the heterogeneity
# accounted for rather than assumed away.
fit_variogram <- function(tab) {
  gws <- as.numeric(colnames(tab))
  G <- length(gws)
  idx <- which(upper.tri(matrix(0, G, G)), arr.ind = TRUE)
  gamma <- apply(idx, 1, function(r) var(tab[, r[1]] - tab[, r[2]]) / 2)
  X <- matrix(0, nrow(idx), G)
  rows <- seq_len(nrow(idx))
  X[cbind(rows, idx[, 1])] <- 0.5
  X[cbind(rows, idx[, 2])] <- X[cbind(rows, idx[, 2])] + 0.5
  fit <- lm.fit(X, gamma)
  u <- fit$coefficients
  u[!is.finite(u)] <- NA_real_
  list(gws = gws, i = idx[, 1], j = idx[, 2], gamma = gamma, u = u,
       delta = -fit$residuals)
}

# The SE a grid would have, given the total per-cell variance v and the average
# off-diagonal covariance cbar its pairs imply:
#
#   Var(season mean) = cbar + (v - cbar)/G,    SE = sqrt(that / S).
#
# At cbar = 0 this is AGENTS.md's formula with sigma2_season = 0. The floor as G
# grows is sqrt(cbar/S), which is AGENTS.md's own floor — so the framework agrees
# with the record and only the gap dependence of cbar is new.
se_from_cov <- function(v, cbar, S, G) {
  sqrt(max(cbar + (v - cbar) / G, 0) / S)
}

# The effective number of independent entry points: the G that would give the same
# variance on the mean if the cells were independent.
g_effective <- function(G, rho_bar) G / (1 + (G - 1) * rho_bar)

# The shared-football ceiling on rho for a nested pair, from weeks alone.
ceiling_rho <- function(w1, w2) sqrt(min(w1, w2) / max(w1, w2))

pair_rhos <- function(weeks, v_resid, v_total, vg) {
  i <- vg$i
  j <- vg$j
  data.frame(g1 = vg$gws[i], g2 = vg$gws[j], gap = vg$gws[j] - vg$gws[i],
             w1 = weeks[i], w2 = weeks[j], gamma = vg$gamma,
             # The excess, heteroscedasticity-aware: delta(h) normalised by the
             # geometric mean of the two columns' fitted variances.
             exc_het = vg$delta / sqrt(pmax(vg$u[i], 0) * pmax(vg$u[j], 0)),
             rho = 1 - vg$gamma / v_total,
             excess = 1 - vg$gamma / v_resid,
             rho_ceiling = mapply(ceiling_rho, weeks[i], weeks[j]),
             stringsAsFactors = FALSE)
}

# The naive column correlation, kept as a cross-check on the *level*. It is biased
# upward by the shared season effect, by (sigma2_a + c)/(sigma2_a + v) against c/v,
# so it is informative mainly where the season component is small — which on HOLD is
# what AGENTS.md measures and what the F test here reports.
pair_corrs <- function(tab) {
  gws <- as.numeric(colnames(tab))
  out <- NULL
  for (i in seq_along(gws)) {
    for (j in seq_along(gws)) {
      if (j <= i) next
      out <- rbind(out, data.frame(gap = gws[j] - gws[i],
                                   r = cor(tab[, i], tab[, j])))
    }
  }
  out
}

# The nominal spacings a densified grid would use. A twelve-point grid has around
# twenty distinct gaps and most carry a single pair, so a row here is "what a grid at
# this spacing would see".
bucket_of <- function(h) {
  as.numeric(as.character(cut(h, c(0, 1.5, 2.5, 4, 7.5, 12.5, 17.5, 22.5, Inf),
                              labels = c(1, 2, 3, 5, 10, 15, 20, 25))))
}

# Excess correlation fitted against the gap as an exponential to a floor,
#
#   f(h) = floor + (f0 - floor) * exp(-h / L),
#
# by profiling L on a grid and solving the two linear coefficients at each L,
# weighted by the number of pairs behind each gap. An exponential is the obvious
# first form for a decaying autocorrelation and nothing claims it is the true one;
# the fit's own weighted rms is printed so a reader can see how much to trust the
# extrapolation, which is the only thing it is used for.
#
# It falls back to a flat model, loudly, when the exponential degenerates. Fitted to
# points with no decay in them the length scale runs to its lower bound and the
# gap-zero intercept goes anywhere at all, and extrapolating from a runaway intercept
# would be the most confident-looking wrong answer this script could produce.
fit_decay <- function(by_gap, col) {
  d <- by_gap[is.finite(by_gap[[col]]), ]
  if (nrow(d) < 3) return(NULL)
  d$y <- d[[col]]
  flat <- sum(d$pairs * d$y) / sum(d$pairs)
  flat_rms <- sqrt(sum(d$pairs * (d$y - flat)^2) / sum(d$pairs))

  best <- NULL
  # L on [1, 200]: gaps are whole gameweeks, so a length scale below one describes
  # nothing the data can distinguish and is exactly where the intercept runs away.
  for (L in exp(seq(log(1), log(200), length.out = 400))) {
    x <- exp(-d$gap / L)
    f <- lm(y ~ x, data = data.frame(y = d$y, x = x), weights = d$pairs)
    rss <- sum(d$pairs * residuals(f)^2)
    if (is.null(best) || rss < best$rss) {
      best <- list(L = L, coef = coef(f), rss = rss)
    }
  }
  floorv <- best$coef[[1]]
  f0 <- best$coef[[1]] + best$coef[[2]]
  # The bound is 2 rather than 1 so sampling noise pushing a fitted gap-zero
  # correlation to 1.001 is not called degenerate; the self-test's gap-dependent case
  # does exactly that and recovers the true length scale to within 3%.
  degenerate <- !is.finite(f0) || abs(f0) > 2 || !is.finite(floorv) ||
    abs(floorv) > 2 || best$L <= 1.001
  if (degenerate) {
    return(list(L = NA_real_, floor = flat, f0 = flat, flat = flat, rms = flat_rms,
                flat_rms = flat_rms, degenerate = TRUE,
                predict = function(h) rep(flat, length(h))))
  }
  list(L = best$L, floor = floorv, f0 = f0, flat = flat,
       rms = sqrt(best$rss / sum(d$pairs)), flat_rms = flat_rms,
       degenerate = FALSE,
       # Clamped, because an exponential extrapolated below the smallest observed gap
       # can leave [-1, 1] and a correlation cannot.
       predict = function(h) {
         pmin(1, pmax(-1, floorv + (f0 - floorv) * exp(-h / best$L)))
       })
}

# The average fitted excess over the pairs of an evenly spread grid of G entry points
# across [lo, hi]. This is what turns a curve into an SE(G).
exc_bar_even <- function(G, lo, hi, f) {
  gws <- seq(lo, hi, length.out = G)
  gaps <- outer(gws, gws, function(x, y) abs(x - y))
  mean(f(gaps[upper.tri(gaps)]))
}

# --- self-test --------------------------------------------------------------
#
# No cells file, no packages, no replay. Synthetic tables whose answers are exact by
# construction, because the estimator *is* the result: an arithmetic slip in the
# variogram would produce a plausible number and a wrong verdict, and there is no
# second implementation to disagree with it.

if (opt_selftest) {
  failures <- 0
  check <- function(label, got, want, tol) {
    # Logicals are accepted so a boolean invariant reads as 1 vs 1 rather than
    # needing an as.numeric at every call site.
    got <- as.numeric(got)
    want <- as.numeric(want)
    ok <- is.finite(got) && abs(got - want) <= tol
    note(sprintf("%-62s %9s vs %9s  %s", label, fmt(got, 4), fmt(want, 4),
                 if (ok) "ok" else "FAIL"))
    if (!ok) failures <<- failures + 1
  }

  rule("an exchangeable correlation IS the season component, at every level")
  # The claim the whole framing rests on. If sigma2_season did not estimate cbar, the
  # verdict would be about the wrong quantity.
  set.seed(11)
  S <- 400
  gwsA <- c(1, 2, 3, 4, 6, 11, 16, 21)
  G <- length(gwsA)
  for (rho in c(0, 0.3, 0.75)) {
    shared <- matrix(rnorm(S), S, G)
    indep <- matrix(rnorm(S * G), S, G)
    tab <- sqrt(rho) * shared + sqrt(1 - rho) * indep
    dimnames(tab) <- list(seq_len(S), gwsA)
    tw <- two_way(tab)
    vg <- fit_variogram(tab)
    check(sprintf("exchangeable rho=%.2f: sigma2_season estimates cbar", rho),
          tw$v_season, rho, 0.05)
    check(sprintf("exchangeable rho=%.2f: MS_resid is (1-rho)*v", rho),
          tw$v_resid, 1 - rho, 0.05)
    check(sprintf("exchangeable rho=%.2f: fitted u_i is v - cbar = (1-rho)*v", rho),
          mean(vg$u), 1 - rho, 0.06)
    v_total <- tw$v_resid + tw$v_season
    pr <- pair_rhos(rep(38, G), tw$v_resid, v_total, vg)
    check(sprintf("exchangeable rho=%.2f: absolute rho recovers the level", rho),
          mean(pr$rho), rho, 0.05)
    check(sprintf("exchangeable rho=%.2f: excess is flat at 0", rho),
          mean(pr$excess), 0, 0.05)
    check(sprintf("exchangeable rho=%.2f: exc_het is flat at 0", rho),
          mean(pr$exc_het), 0, 0.05)
    check(sprintf("exchangeable rho=%.2f: column correlation recovers the level",
                  rho), mean(pair_corrs(tab)$r), rho, 0.05)
    check(sprintf("exchangeable rho=%.2f: projected SE = measured SE", rho),
          se_from_cov(v_total, tw$v_season, S, G), se_clustered(tab), 0.01)
  }

  rule("unequal column variances: why exc_het is fitted rather than pooled")
  # The failure mode this design is exposed to, and the reason for fit_variogram.
  # Column variance rises across the season, as it measurably does, and there is NO
  # gap dependence in the correlations at all. Both the pooled excess and the
  # naive per-column route must invent one; the fitted excess must not.
  set.seed(13)
  S <- 3000
  gwsB <- c(1, 2, 3, 4, 6, 11, 16, 21, 26)
  G <- length(gwsB)
  sdv <- seq(1, 3, length.out = G)
  tab <- matrix(rnorm(S * G), S, G) %*% diag(sdv)
  dimnames(tab) <- list(seq_len(S), gwsB)
  tw <- two_way(tab)
  vg <- fit_variogram(tab)
  check("unequal variances: the fitted u_i recover the true column variances",
        max(abs(vg$u - sdv^2) / sdv^2), 0, 0.10)
  pr <- pair_rhos(rep(38, G), tw$v_resid, tw$v_resid, vg)
  pr$bucket <- bucket_of(pr$gap)
  span <- function(col) {
    v <- tapply(pr[[col]], pr$bucket, mean)
    max(v) - min(v)
  }
  check("unequal variances, no gap dependence: pooled excess spans a spurious",
        span("excess") > 0.2, TRUE, 0)
  check("unequal variances, no gap dependence: fitted exc_het spans",
        span("exc_het"), 0, 0.05)

  rule("recovering a gap-dependent correlation, which is what the tables report")
  set.seed(12)
  S <- 4000
  gwsC <- c(0, 1, 2, 4, 8, 16)
  G <- length(gwsC)
  L_true <- 5
  Sig <- exp(-abs(outer(gwsC, gwsC, "-")) / L_true)
  tab <- matrix(rnorm(S * G), S, G) %*% chol(Sig)
  dimnames(tab) <- list(seq_len(S), gwsC)
  tw <- two_way(tab)
  vg <- fit_variogram(tab)
  pr <- pair_rhos(rep(38, G), tw$v_resid, 1, vg)
  check("gap-dependent: absolute rho, max error over 15 pairs, v known",
        max(abs(pr$rho - exp(-pr$gap / L_true))), 0, 0.05)
  # exc_het is the absolute curve minus the grid's mean correlation, so its shape and
  # therefore its length scale survive; only the level is removed.
  pr$bucket <- bucket_of(pr$gap)
  by_gap <- aggregate(cbind(rho, exc_het) ~ bucket, data = pr, FUN = mean)
  names(by_gap)[1] <- "gap"
  by_gap$pairs <- as.numeric(table(pr$bucket)[as.character(by_gap$gap)])
  f <- fit_decay(by_gap, "rho")
  check("gap-dependent: fitted length scale", f$L, L_true, 1.5)
  check("gap-dependent: fitted correlation at gap 0", f$f0, 1, 0.10)
  check("gap-dependent: exponential beats the flat model",
        f$rms < f$flat_rms, TRUE, 0)
  # The mechanism the whole test is about: with a decaying correlation, a denser grid
  # over the same span really does carry more average covariance.
  check("gap dependence means a denser grid has higher mean correlation",
        exc_bar_even(24, 1, 26, f$predict) > exc_bar_even(6, 1, 26, f$predict),
        TRUE, 0)

  rule("the arithmetic the verdict rests on")
  check("se_from_cov at cbar=0 is the published formula",
        se_from_cov(4, 0, 4, 12), sqrt((4 / 12) / 4), 1e-12)
  check("se_from_cov at cbar=v does not shrink with G at all",
        se_from_cov(4, 4, 4, 24), sqrt(4 / 4), 1e-12)
  check("se_from_cov floors at sqrt(cbar/S)", se_from_cov(4, 1, 4, 1e9),
        sqrt(1 / 4), 1e-6)
  check("G_eff at rho=0 is G", g_effective(12, 0), 12, 1e-12)
  check("G_eff at rho=1 is 1", g_effective(12, 1), 1, 1e-12)
  check("shared-football ceiling, GW1 against GW2", ceiling_rho(38, 37),
        sqrt(37 / 38), 1e-12)
  check("se_clustered is sd(season means)/sqrt(S)",
        se_clustered(matrix(c(1, 2, 3, 4, 1, 2, 3, 4), 4, 2)),
        sd(c(1, 2, 3, 4)) / 2, 1e-12)
  check("a degenerate fit falls back to the flat model",
        fit_decay(data.frame(gap = c(1, 5, 10, 20), y = c(0, 0, 0, 0),
                             pairs = c(1, 1, 1, 1)), "y")$degenerate, TRUE, 0)

  note("")
  if (failures > 0) {
    note(sprintf("%d check(s) FAILED", failures))
    quit(status = 1)
  }
  note("all checks passed")
  quit(status = 0)
}

# --- read the cells ---------------------------------------------------------

if (length(paths) == 0) stop("give a cells CSV (or --selftest)")
# `read_cells_all` is in cells_common.R. This block used to read the file itself,
# with no `na.strings` and an inline `%in% c(TRUE, "true")` flag test — the only
# reader in the family that defined nothing, and therefore the one the name-scanning
# guard could never have protected.
cells <- read_cells_all(paths)

flagged <- sum(cells$infeasible)
if (flagged > 0) {
  note(sprintf("NOTE: dropping %d infeasible cell(s). A variant that could not ",
               flagged),
       "field a legal fifteen is a result about the variant, not a missing row.")
  cells <- cells[!cells$infeasible, ]
}

col <- paste0(opt_metric, "_per_gw")
if (!col %in% names(cells)) stop("no column ", col, " in the cells file")

blocks <- split(cells, list(cells$sweep, cells$run_id), drop = TRUE)

# What was found, before anything is computed. A sweep on this machine gets killed
# under load routinely — AGENTS.md records one block dying at 1, 3, 3 and 4 arms of 6
# — and the failure that costs time is a partial run analysed as a complete one.
rule("What is in the cells file")
usable <- 0
for (bn in names(blocks)) {
  d <- blocks[[bn]]
  nb <- length(unique(d$variant[!d$is_baseline]))
  per <- sort(unique(as.numeric(table(d$variant))))
  note(sprintf("  %s: %d arms (%d non-baseline), %d cells, %s per arm", bn,
               length(unique(d$variant)), nb, nrow(d), paste(per, collapse = "/")))
  if (nb > 0) usable <- usable + 1
}
if (usable == 0) {
  note("")
  note("NOTHING TO ANALYSE: no block has a non-baseline arm. This is what a sweep")
  note("looks like part-way through its first arm, or after being killed during it.")
  note("It is not a null result — re-run or resume. Exiting non-zero so a script")
  note("cannot mistake this for a completed analysis.")
  quit(status = 2)
}

emit <- list()

for (bn in names(blocks)) {
  d <- blocks[[bn]]
  base <- d[d$is_baseline, ]
  arms <- setdiff(unique(d$variant), unique(base$variant))
  if (nrow(base) == 0 || length(arms) == 0) next

  for (v in arms) {
    arm <- d[d$variant == v, ]
    j <- merge(base[, c("season", "start_gw", "weeks", col)],
               arm[, c("season", "start_gw", col)],
               by = c("season", "start_gw"), suffixes = c(".base", ".arm"))
    j$diff <- j[[paste0(col, ".arm")]] - j[[paste0(col, ".base")]]
    j <- j[is.finite(j$diff), ]

    tab <- tapply(j$diff, list(j$season, j$start_gw), function(x) x[1])
    if (is.null(tab) || any(is.na(tab)) || ncol(tab) < 4) {
      note("")
      note(sprintf("SKIPPING %s / %s: the table is %s with %d hole(s). Every table ",
                   bn, v,
                   if (is.null(tab)) "empty" else paste(dim(tab), collapse = " x "),
                   if (is.null(tab)) 0 else sum(is.na(tab))),
           "here assumes balance, so a partial arm is not analysed at all rather ",
           "than analysed on fewer cells — which would read as a weaker version of ",
           "the same comparison.")
      next
    }
    # tapply orders columns by the *string* form of start_gw, so "11" sorts before
    # "2". Reorder numerically or every gap in every table below is wrong — and wrong
    # in a way that still prints a plausible decaying curve.
    tab <- tab[, order(as.numeric(colnames(tab))), drop = FALSE]
    gws <- as.numeric(colnames(tab))
    weeks <- as.numeric(tapply(j$weeks, j$start_gw,
                               function(x) x[1])[colnames(tab)])
    S <- nrow(tab)
    G <- ncol(tab)

    note("")
    note(strrep("=", 78))
    note(sprintf("%s   arm: %s   metric: %s", bn, v, opt_metric))
    note(sprintf("%d seasons x %d entry points = %d cells; mean paired difference ",
                 S, G, S * G),
         sprintf("%+.3f pts/gw (%+.1f a season)", mean(tab),
                 mean(tab) * SEASON_GWS))
    note(strrep("=", 78))

    tw <- two_way(tab)
    vg <- fit_variogram(tab)
    v_col <- pmax(vg$u, 1e-9)
    v_total <- tw$v_resid + tw$v_season

    rule("1. Variance components, and whether seasons disagree")
    note(sprintf("  sd_season %8s   sd_resid %8s   F_season %6s (p %5s)",
                 fmt(sign(tw$v_season) * sqrt(abs(tw$v_season))),
                 fmt(sqrt(max(tw$v_resid, 0))), fmt(tw$f_season, 2),
                 fmt(tw$p_season, 3)))
    note(sprintf("  cbar (= sigma2_season) %s   v (= MS_resid + cbar) %s   ",
                 fmt(tw$v_season), fmt(v_total)),
         sprintf("mean rho implied %s", fmt(tw$v_season / v_total)))
    if (tw$v_season < 0) {
      note("  The method-of-moments season component is NEGATIVE, which is how a")
      note("  component says it is below the residual — and since that component IS")
      note("  cbar, it says the average within-season covariance between two entry")
      note("  points is indistinguishable from zero or slightly below. v is floored")
      note("  at MS_resid for the projections below: a negative total variance is not")
      note("  a thing, and the honest reading of a negative cbar is zero.")
      v_total <- tw$v_resid
    }

    rule("1b. Per-entry-point variance, which is not flat and matters twice")
    note("  From the variogram fit, so it is not contaminated by the row mean. It")
    note("  matters because a WIDE sub-grid necessarily reaches both ends of the")
    note("  season, so any heterogeneity across entry points reads as a spacing")
    note("  effect; and because a pooled variogram assumes it away.")
    note("")
    note(sprintf("  %-8s %s", "entry", paste(sprintf("%6d", gws), collapse = "")))
    note(sprintf("  %-8s %s", "weeks",
                 paste(sprintf("%6d", as.integer(weeks)), collapse = "")))
    note(sprintf("  %-8s %s", "sd", paste(sprintf("%6s", fmt(sqrt(v_col), 2)),
                                          collapse = "")))
    wk_exp <- coef(lm(log(v_col) ~ log(weeks)))[[2]]
    note("")
    note(sprintf("  variance goes as weeks^%s, fitted on these %d columns; spread ",
                 fmt(wk_exp, 2), G),
         sprintf("%sx.", fmt(max(v_col) / min(v_col), 1)))
    note("  AGENTS.md's candidates for this exponent are -1 (averaging over more")
    note("  gameweeks) and pessimistically -1.56. A value near or above zero means")
    note("  the dominant term is not averaging at all: it is that the effect under")
    note("  test is a different size at different entry points, so the entry points")
    note("  where it is largest are also the noisiest. Nothing downstream assumes a")
    note("  weeks law — the variogram fit uses these variances directly.")

    pr <- pair_rhos(weeks, tw$v_resid, v_total, vg)
    cr <- pair_corrs(tab)
    pr$bucket <- bucket_of(pr$gap)
    cr$bucket <- bucket_of(cr$gap)

    by_bucket <- do.call(rbind, lapply(split(seq_len(nrow(pr)), pr$bucket),
                                       function(ix) {
      b <- pr$bucket[ix[1]]
      data.frame(bucket = b, pairs = length(ix), df = length(ix) * (S - 1),
                 gap = mean(pr$gap[ix]), exc_het = mean(pr$exc_het[ix]),
                 rho = mean(pr$rho[ix]), excess = mean(pr$excess[ix]),
                 r_column = mean(cr$r[cr$bucket == b]),
                 ceiling = mean(pr$rho_ceiling[ix]), stringsAsFactors = FALSE)
    }))
    by_bucket <- by_bucket[order(by_bucket$bucket), ]

    rule("2. Correlation against the gap between two entry points")
    note("  From the variogram gamma(h) = Var_s(d[g1]-d[g2])/2, pooled over the")
    note("  pairs in each spacing bucket. Read `exc_het` first: it is the excess")
    note("  correlation over the grid's average pair, computed against per-column")
    note("  variances, so neither the season component nor the heterogeneity in 1b")
    note("  can put a slope in it. Flat in the gap means no gap dependence and the")
    note("  1/G arithmetic stands. `rho` and `r(col)` speak to the LEVEL, not the")
    note("  slope, and both are noisy. `ceiling` is sqrt(w_short/w_long): what two")
    note("  entry points would correlate at if they scored the same football with")
    note("  the same squad.")
    note("")
    note(sprintf("  %6s %5s %6s %5s %9s %8s %8s %8s %8s", "bucket", "gap", "pairs",
                 "df", "exc_het", "rho", "excess", "r(col)", "ceiling"))
    for (i in seq_len(nrow(by_bucket))) {
      note(sprintf("  %6d %5s %6d %5d %9s %8s %8s %8s %8s", by_bucket$bucket[i],
                   fmt(by_bucket$gap[i], 1), by_bucket$pairs[i], by_bucket$df[i],
                   fmt(by_bucket$exc_het[i]), fmt(by_bucket$rho[i]),
                   fmt(by_bucket$excess[i]), fmt(by_bucket$r_column[i]),
                   fmt(by_bucket$ceiling[i])))
    }
    note("")
    note(sprintf("  over all %d pairs: exc_het %s, rho %s, r(col) %s, ceiling %s",
                 nrow(pr), fmt(mean(pr$exc_het)), fmt(mean(pr$rho)),
                 fmt(mean(cr$r)), fmt(mean(pr$rho_ceiling))))
    # Does the excess depend on the gap at all? A weighted regression on log(gap) is
    # the cheapest test. Its t is read against the number of buckets, not the number
    # of pairs, since the pairs within a bucket share columns.
    if (nrow(by_bucket) >= 4) {
      cs <- summary(lm(exc_het ~ log(gap), data = by_bucket,
                       weights = by_bucket$pairs))$coefficients
      note(sprintf("  exc_het against log(gap): slope %+.4f (SE %s, t %s, %d buckets)",
                   cs[2, 1], fmt(cs[2, 2], 4), fmt(cs[2, 3], 2), nrow(by_bucket)))
      note("  A slope indistinguishable from zero is the null this test looks for,")
      note("  and it is the good outcome for densification. A NEGATIVE slope is the")
      note("  feared case: near neighbours more correlated than distant ones.")
    }

    f <- fit_decay(by_bucket, "exc_het")
    if (!is.null(f)) {
      rule("3. exc_het(gap) fitted, so a grid that was not run can be priced")
      if (f$degenerate) {
        note(sprintf("  exc_het(h) = %s, flat.", fmt(f$flat)))
        note("  The exponential form is DEGENERATE here — no decay in the gaps, so")
        note("  the length scale sits on its lower bound and the gap-zero intercept")
        note("  runs away. The flat model is used instead, which is the null and is")
        note("  what 'no gap dependence' looks like once fitted.")
      } else {
        note(sprintf("  exc_het(h) = %s + (%s - %s) * exp(-h / %s)", fmt(f$floor),
                     fmt(f$f0), fmt(f$floor), fmt(f$L, 1)))
        note(sprintf("  weighted rms %s against %s for the flat model exc = %s",
                     fmt(f$rms), fmt(f$flat_rms), fmt(f$flat)))
      }
      note("  It prices grids that were not run and nothing else, and the rms above")
      note("  is how much to trust it.")
    }

    rule("4. Measured clustered SE on sub-grids of this run")
    note("  Assumption-free: each row re-computes sd(season means)/sqrt(S) on a")
    note("  subset of the columns actually replayed. Every one rests on four season")
    note("  means and three degrees of freedom, so read the pattern and not a row.")
    note("")
    subgrids <- list(
      "this grid, whatever it is" = gws,
      "shipped six" = c(1, 6, 11, 16, 21, 26),
      "tight four, early (1,2,3,4)" = c(1, 2, 3, 4),
      "spread four, early (1,6,11,16)" = c(1, 6, 11, 16),
      "tight four, mid (16,17,18,19)" = c(16, 17, 18, 19),
      "spread four, late (11,16,21,26)" = c(11, 16, 21, 26),
      "tight eight (both clusters)" = c(1, 2, 3, 4, 16, 17, 18, 19),
      "spread three (1,11,21)" = c(1, 11, 21))
    note(sprintf("  %-34s %3s %9s %9s %9s %8s", "sub-grid", "G", "mean/gw",
                 "SE_clust", "SE_indep", "G_eff"))
    for (nm in names(subgrids)) {
      keep <- as.character(subgrids[[nm]])
      # All or nothing. Silently dropping a missing column would print a G=1 row
      # under a label saying "tight four" and invite exactly the comparison this
      # section exists to make, on data that cannot make it.
      if (!all(keep %in% colnames(tab))) {
        note(sprintf("  %-34s  not in this grid (missing %s)", nm,
                     paste(setdiff(keep, colnames(tab)), collapse = ",")))
        next
      }
      st <- tab[, keep, drop = FALSE]
      g <- length(keep)
      se <- se_clustered(st)
      se_indep <- se_from_cov(v_total, 0, S, g)
      # G_eff backed out of the measured SE: the G that would give this SE if the
      # cells were independent. Above G means the sub-grid came out luckier than
      # independence, which four seasons routinely do.
      geff <- if (se > 0) v_total / (S * se^2) else NA_real_
      note(sprintf("  %-34s %3d %9s %9s %9s %8s", nm, g,
                   sprintf("%+.3f", mean(st)), fmt(se), fmt(se_indep),
                   if (is.finite(geff)) fmt(geff, 1) else "n/a"))
      emit[[length(emit) + 1]] <- emit_row(bn, v, "measured", nm, g, NA_real_,
                                           NA_real_, geff, se, NA_real_)
    }

    if (G >= 8) {
      rule("4b. SE against how spread out a grid is, over ALL sub-grids of each size")
      note("  Every choice of k entry points from this run is scored and grouped into")
      note("  thirds by mean pairwise gap. Twice: raw, and with each column divided")
      note("  by its own residual sd from 1b, which equalises the columns exactly")
      note("  rather than through an assumed weeks law. Read the standardised row —")
      note("  in the raw one a wide grid is penalised simply for reaching the")
      note("  noisiest entry points.")
      note("")
      note("  Sub-grids overlap heavily, so this is a descriptive contrast. No")
      note("  standard error is quoted and none would mean anything.")
      note("")
      tab_z <- sweep(tab, 2, sqrt(v_col), "/")
      colnames(tab_z) <- colnames(tab)
      note(sprintf("  %2s %7s %13s %9s %8s %9s %8s %11s", "k", "grids", "table",
                   "gap tight", "gap wide", "SE tight", "SE wide", "tight/wide"))
      for (k in 3:min(6, G - 2)) {
        idx <- combn(G, k)
        gap_k <- apply(idx, 2, function(ix) {
          d2 <- outer(gws[ix], gws[ix], function(x, y) abs(x - y))
          mean(d2[upper.tri(d2)])
        })
        cut3 <- quantile(gap_k, c(1 / 3, 2 / 3))
        third <- cut(gap_k, c(-Inf, cut3, Inf), labels = c("tight", "mid", "wide"))
        mg <- tapply(gap_k, third, mean)
        for (which in c("raw", "standardised")) {
          tt <- if (which == "raw") tab else tab_z
          se_k <- apply(idx, 2, function(ix) se_clustered(tt[, ix, drop = FALSE]))
          ms <- tapply(se_k, third, mean)
          ratio <- as.numeric(ms[["tight"]] / ms[["wide"]])
          note(sprintf("  %2d %7d %13s %9s %8s %9s %8s %11s", k, ncol(idx), which,
                       fmt(mg[["tight"]], 1), fmt(mg[["wide"]], 1),
                       fmt(ms[["tight"]]), fmt(ms[["wide"]]), fmt(ratio, 2)))
          if (which == "standardised") {
            emit[[length(emit) + 1]] <- emit_row(
              bn, v, "subgrid_thirds", "tightest third, column-standardised", k,
              as.numeric(mg[["tight"]]), NA_real_, NA_real_,
              as.numeric(ms[["tight"]]), ratio)
          }
        }
      }
      note("")
      note("  A ratio near 1.00 is the null: how spread out the grid is does not")
      note("  change what it resolves, so extra entry points are worth their nominal")
      note("  1/G however they are placed. ABOVE 1.00 is the feared case, tight grids")
      note("  noisier because their cells duplicate each other. BELOW 1.00 says tight")
      note("  grids are quieter, which no correlation structure explains and which")
      note("  means something is still confounded — the likely candidate is that a")
      note("  wide grid must span information regimes in which the effect under test")
      note("  is genuinely a different size.")
    }

    if (!is.null(f)) {
      rule("5. Extrapolation to an EVENLY SPREAD grid, against the prediction")
      note("  This grid's own SE is not the answer for G=12: it over-samples short")
      note("  gaps deliberately, so an evenly spread twelve would contain fewer")
      note("  near-duplicates and do better. The fitted curve prices that instead.")
      note("  Span 1..26 throughout, the shipped grid's own span, so the only thing")
      note("  varying down the table is how closely packed the entry points are.")
      note("")
      note("  The model gives the SHAPE and the level is pinned to the measured")
      note("  shipped-six SE. The published figures come from a different sweep")
      note("  pooled over four arms, so their level is not this comparison's to")
      note("  reproduce; and the model's own G=6 value need not equal the")
      note("  measurement, since flooring a negative cbar at zero raises it. The")
      note("  ratio column separates 'does densification shrink the SE' from 'is the")
      note("  level right', and only the first is being asked.")
      note("")
      se6 <- se_clustered(tab[, as.character(c(1, 6, 11, 16, 21, 26)), drop = FALSE])
      published <- c(`6` = 0.515, `12` = 0.364, `24` = 0.257, `48` = 0.182)
      mean_rho <- max(tw$v_season, 0) / v_total
      gs <- c(6, 12, 24, 26)
      rb <- vapply(gs, function(g) mean_rho + exc_bar_even(g, 1, 26, f$predict),
                   numeric(1))
      se_model <- vapply(seq_along(gs), function(i) {
        se_from_cov(v_total, rb[i] * v_total, S, gs[i])
      }, numeric(1))
      se_model6 <- se_model[gs == 6]
      note(sprintf("  %4s %8s %8s %7s %10s %8s %11s %10s %9s", "G", "spacing",
                   "rho_bar", "G_eff", "SE(model)", "vs G=6", "SE(calib.)",
                   "1/sqrt(G)", "published"))
      for (i in seq_along(gs)) {
        g <- gs[i]
        ratio <- se_model[i] / se_model6
        note(sprintf("  %4d %8s %8s %7s %10s %8s %11s %10s %9s", g,
                     fmt(25 / (g - 1), 2), fmt(rb[i]),
                     fmt(g_effective(g, rb[i]), 1), fmt(se_model[i]),
                     fmt(ratio, 2), fmt(se6 * ratio), fmt(sqrt(6 / g), 2),
                     if (as.character(g) %in% names(published))
                       fmt(published[[as.character(g)]]) else "-"))
        emit[[length(emit) + 1]] <- emit_row(bn, v, "extrapolated",
                                             "evenly spread over 1..26", g,
                                             25 / (g - 1), rb[i],
                                             g_effective(g, rb[i]), se6 * ratio,
                                             ratio)
      }
      note(sprintf("  %4d %8s %8s %7s %10s %8s %11s %10s %9s", 48, "<1", "-", "-",
                   "impossible", "-", "-", fmt(sqrt(6 / 48), 2),
                   fmt(published[["48"]])))
      note("")
      note(sprintf("  measured SE at the shipped six on these cells: %s, which is ",
                   fmt(se6)), "what the calibrated column is anchored on.")
      note(sprintf("  the floor as G grows, sqrt(cbar/S): %s pts/gw, %s a season.",
                   fmt(sqrt(max(tw$v_season, 0) / S)),
                   fmt(sqrt(max(tw$v_season, 0) / S) * SEASON_GWS, 1)))
      note("  G=48 needs 48 distinct deadlines inside GW1-26 and there are 26, so")
      note("  that row of the published table is impossible rather than expensive.")
      note("  Reaching G=48 at all means entering after GW26, where a window is under")
      note("  13 gameweeks — see 1b for whether that is cheap or not.")
      note("  AGENTS.md's 0.515 is the MinutesHalfLife sweep POOLED over four arms,")
      note("  not this comparison. The like-for-like check is the ratio column, plus")
      note("  the recorded CR2 SE for this arm: -0.709/-5.95 = 0.119.")
    }
  }
}

if (length(emit) > 0 && !is.na(opt_out)) {
  dir.create(opt_out, recursive = TRUE, showWarnings = FALSE)
  write.csv(do.call(rbind, emit), file.path(opt_out, "entry_density.csv"),
            row.names = FALSE)
  note("")
  note("wrote ", file.path(opt_out, "entry_density.csv"))
}
