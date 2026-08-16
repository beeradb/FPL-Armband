#!/usr/bin/env Rscript
#
# Which family does the clean-sheet miscalibration belong to?
#
# Usage — the population is chosen by the DATA STATE of the dump, not by a flag here:
#
#   # every row the diagnostic sees, xGC repair on
#   FPL_CS_ROWS=/tmp/all.csv DIAG=1 \
#     go test ./internal/backtest -run TestDiagCleanSheetPoisson -count=1
#   Rscript stats/cs_calibration.R /tmp/all.csv
#
#   # native xGC only -- the arm that carries the verdict
#   FPL_NO_XGC_REPAIR=1 FPL_CS_ROWS=/tmp/native.csv DIAG=1 \
#     go test ./internal/backtest -run TestDiagCleanSheetPoisson -count=1
#   Rscript stats/cs_calibration.R /tmp/native.csv
#
# ---------------------------------------------------------------------------
# The question
#
# `baseXP90` prices the clean sheet as `exp(-f * xGC)` with `f =
# cleanSheetXGCFactor`, shipped at 1.0 and exposed as `FPL_CS_XGC_FACTOR`. The
# term over-predicts. The question is not "by how much" but **which shape**,
# because the shipped knob can only express one of the two candidates:
#
#   a pure SCALE   -ln(p) = b * x        -- what FPL_CS_XGC_FACTOR does
#   a pure OFFSET  -ln(p) = a + x        -- a flat rescale of every P(CS),
#                                           which the knob CANNOT express
#
# Fit `-ln(p) = a + b*x` and the two families are nested inside it: a scale is
# `a = 0`, an offset is `b = 1`. Under a binomial GLM with a **log link** that is
# the model exactly, so both are likelihood-ratio tests rather than eyeballed
# distances. The alternatives were considered and are worse: quasi-binomial is
# unusable because a *Bernoulli* dispersion parameter is not identifiable, nls
# loses the LRT and is heteroscedastic on a binary response, and fitting the
# complement models `exp(a+bx)`, which is a different model and unbounded above 1.
#
# ---------------------------------------------------------------------------
# Why this is not fitted on the diagnostic's buckets
#
# The Go diagnostic prints six xGC buckets and a fit over their means. That fit
# is **biased toward the offset**, and by about the size of the intercept it is
# used to establish. `exp` is convex, so the mean of `exp(-x)` is not
# `exp(-mean(x))`; backing an effective x out of a bucket mean inherits the gap,
# and the gap grows with within-bucket spread, which is largest in the open-ended
# top bucket. The bucket-mean estimator is reproduced at the bottom of this script
# on the same rows, so the gap is measured rather than asserted -- on native rows
# it is about a +0.10, b -0.09, and on all rows about half that.
#
# A family verdict was once written off that estimator, and it came out backwards.
# The buckets are kept for display and for reading a shape; the inference is here.
#
# ---------------------------------------------------------------------------
# Errors-in-variables: read the DIRECTION of any offset with suspicion
#
# The regressor is *realised per-match* xGC, which this record already describes
# as regressive match to match. With `x = lambda + e`, `E[lambda|x] = mu(1-rho) +
# rho*x` -- so measurement error in the regressor produces `a > 0` and `b < 1`
# **mechanically**, whether or not the model is mis-specified. An offset-shaped
# fit here is therefore the null, not a finding, and this is the record's own
# "a constant fitted against a proxy for its input is fitted to the proxy's
# noise too".
#
# This is also why the native arm is the defensible one rather than a pick: the
# reconstructed rows are a NOISIER regressor, and removing them moves the fit
# AWAY from the offset (a 0.144 -> 0.100, b 1.120 -> 1.173), which is the
# direction errors-in-variables predicts.
#
# The smoother regressor wants LESS slope, not more, and this has been measured
# rather than argued. TestDiagCleanSheetRegressor dumps the model's own XGC90
# against a clean sheet it had not seen, and THIS script fits it at
# a = +0.0625 (clustered SE 0.2077), b = 0.9922 (0.1516) on native rows, with
# neither restriction rejected (LRT p 0.7572 scale, 0.9587 offset).
# Predicted/actual falls from 1.281 to 1.052 native and 1.004 pooled.
#
# CROSS-MATCH CONVEXITY says why: exp() is convex, so the aggregate
# E[exp(-x)]/exp(-mean x) is larger the more dispersed x is, and realised match
# xGC (sd 0.848) is far more dispersed than XGC90 (0.204-0.301). ⚠️ Quote that
# exact ratio -- 1.3265 realised against 1.0386 on XGC90, predicting the observed
# 1.2799 to 0.3% -- and never `exp(sigma^2/2)` at "sigma ~0.70": sigma there is
# the sd of the DEVIATION rather than of x, and the approximation is 8% high at
# the realised dispersion though excellent at XGC90's.
#
# ⚠️ **This script's own fit is NOT withdrawn** -- it is the fit of a DIFFERENT
# REGRESSOR, and both dumps share this input schema, so name which dump produced
# any figure. stats/snapshots/2026-08-15-clean-sheet-2x2/.
# ⚠️ Neither-rejected there is non-separation, NOT acceptance: at SE 0.152 that
# fit cannot tell b = 1 from b = 1.1731 either (t -1.19).

args <- commandArgs(trailingOnly = TRUE)
path <- if (length(args) >= 1) args[1] else Sys.getenv("FPL_CS_ROWS")
if (path == "") {
  stop("need the per-observation rows: pass a path or set FPL_CS_ROWS")
}

# `fail`, `note`, `hr` and `read_sidecar` are in cells_common.R. This input is NOT
# a cells file -- it is one row per team-match, not per replayed cell -- so it goes
# through `read_sidecar`, the sanctioned reader for a pipeline CSV with its own
# contract. The guard forbids a raw `read.csv(` anywhere in `stats/*.R` outside
# that file, and a script with a different contract is still not the place a raw
# read should hide.
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

d <- read_sidecar(path)
need <- c("season", "gw", "team", "xgc", "clean_sheet")
missing <- setdiff(need, names(d))
if (length(missing) > 0) {
  fail("the rows file is missing ", paste(missing, collapse = ", "),
       " -- regenerate it with FPL_CS_ROWS set on TestDiagCleanSheetPoisson")
}

# ⚠️ There is deliberately NO population flag here, and no 2022-23 gameweek
# boundary. `xgrepair.go` owns that boundary (`"2022-23": {FirstGW: 1, LastGW:
# 15}`) and restating it in R would have made a FOURTH copy of one constant --
# this record's signature failure. It is also unnecessary: the native subset of a
# repaired dump is **byte-identical** to a dump taken under `FPL_NO_XGC_REPAIR=1`
# (2598 of 2870 rows, all six columns, max |delta| exactly 0, measured
# 2026-08-14). So the data state is chosen by the switch, which already has a
# fingerprint entry, and this script fits whatever it is handed.
cat("Clean-sheet calibration: which family?\n")
cat("-ln P(clean sheet) = a + b * xGC, binomial GLM with a log link.\n")
cat("A pure SCALE is a = 0 (this is what FPL_CS_XGC_FACTOR can express).\n")
cat("A pure OFFSET is b = 1 (a flat rescale of every clean-sheet probability).\n")
cat(sprintf("\nrows: %d   seasons: %s\n", nrow(d),
            paste(sort(unique(d$season)), collapse = ", ")))
cat("⚠️  Name the DATA STATE beside any figure from this run: an unswitched dump\n")
cat("    carries reconstructed 2022-23 xGC, which is a noisier regressor and\n")
cat("    biases the fit toward the offset. FPL_NO_XGC_REPAIR=1 gives native only.\n")

# A glm that fails to converge returns with a WARNING and coefficients from
# wherever IWLS stopped -- and Rscript defers warnings to the end of the run,
# after every table has printed. That is the silent-failure shape exactly, so
# this is `fail` and not `note`: a non-converged fit that still prints a
# coefficient is worse than no output at all.
checked <- function(f, label) {
  if (!f$converged || isTRUE(f$boundary) || max(fitted(f)) > 0.999) {
    fail(label, ": glm did not converge cleanly (converged=", f$converged,
         " boundary=", isTRUE(f$boundary),
         " max fitted p=", signif(max(fitted(f)), 4), ")")
  }
  f
}

fit_one <- function(rows, label) {
  x <- rows$xgc
  y <- rows$clean_sheet
  full <- checked(glm(y ~ x, family = binomial(link = "log"),
                      start = c(-0.2, -0.9)), paste(label, "free"))
  a <- -coef(full)[1]
  b <- -coef(full)[2]

  # Cluster-robust variance, because 2598 team-matches are not 2598 independent
  # draws on their face -- a club's matches share a defence and a gameweek's
  # matches share a round. Clustering barely moves this fit and moves `b` the
  # RIGHT way, but that is a fact to print rather than assume.
  cl <- interaction(rows$season, rows$team, drop = TRUE)
  V <- sandwich::vcovCL(full, cluster = cl)
  se_cl <- sqrt(diag(V))
  se <- summary(full)$coefficients[, 2]

  scale_only <- checked(glm(y ~ x - 1, family = binomial(link = "log"),
                            start = c(-0.9)), paste(label, "scale-only"))
  f_scale <- -coef(scale_only)[1]
  p_scale <- anova(scale_only, full, test = "LRT")[2, "Pr(>Chi)"]

  offset_only <- checked(glm(y ~ 1, family = binomial(link = "log"),
                             offset = -x, start = c(-0.2)),
                         paste(label, "offset-only"))
  a_offset <- -coef(offset_only)[1]
  p_offset <- anova(offset_only, full, test = "LRT")[2, "Pr(>Chi)"]

  # Wald t for each restriction, clustered. Printed beside the LRT because the
  # two can disagree at the boundary: on native rows a = 0 gives |t| 1.99, which
  # this record's own |t| < 2 convention does NOT reject, while the LRT does.
  # Which family "survives" must not depend on which test happens to be printed.
  t_a <- a / se_cl[1]
  t_b <- (b - 1) / se_cl[2]

  cat(sprintf("\n%-22s n = %d, clusters (season x team) = %d\n",
              label, nrow(rows), nlevels(cl)))
  cat(sprintf("  free fit     a = %+.4f (naive %.4f, clustered %.4f)\n",
              a, se[1], se_cl[1]))
  cat(sprintf("               b = %+.4f (naive %.4f, clustered %.4f)\n",
              b, se[2], se_cl[2]))
  cat(sprintf("  restriction Wald, clustered:  t(a = 0) = %.2f    t(b = 1) = %.2f\n",
              t_a, t_b))
  cat(sprintf("  pure SCALE   a = 0 forced,  f = %.4f     LRT p = %.4g\n",
              f_scale, p_scale))
  cat(sprintf("  pure OFFSET  b = 1 forced,  a = %+.4f    LRT p = %.4g\n",
              a_offset, p_offset))
  if (p_scale < 0.05 && p_offset < 0.05) {
    cat("  ⚠️  BOTH one-parameter families are rejected. The free fit is the only\n")
    cat("      unrejected model, so neither is adequate and the less-rejected one\n")
    cat(sprintf("      (%s) is NOT thereby established.\n",
                if (p_scale > p_offset) "SCALE" else "OFFSET"))
  } else {
    cat(sprintf("  the less-rejected one-parameter family: %s\n",
                if (p_scale > p_offset) "SCALE" else "OFFSET"))
  }

  # The orthogonal 2x2 levels come from THIS fit, never from the two constrained
  # ones: `f_scale` has already absorbed the level into the slope, so crossing it
  # with a flat arm would double-count. With these two, the both-on corner
  # reproduces the free fit exactly, which is what makes a 2x2 interpretable.
  cat(sprintf("  → 2x2 arms from the JOINT fit: f = %.3f, flat P(CS) x %.3f\n",
              b, exp(-a)))
  invisible(list(a = a, b = b, p_scale = p_scale, p_offset = p_offset))
}

fit_one(d, "all rows in file")

# --- the Jensen scale this dump's regressor implies -------------------------
#
# One number, and it is the sharpest way to say WHICH regressor a dump carries.
#
# The model computes `exp(-sum x_i)` for a clean sheet. The true probability is
# `prod(1 - x_i)`, which is `exp(-c * sum x_i)` with c = -E[ln(1-x)]/E[x] > 1 --
# the SHOT-LEVEL Jensen gap. Solving `mean(exp(-c*x)) = observed rate` for c
# summarises that gap from OUTCOMES rather than from shot distributions.
#
# ⚠️ It cannot TEST the gap. The fit is exactly identified -- one free parameter
# matched to one moment -- so it reproduces that moment by construction and
# returns the same value for every rival mechanism reproducing it. Its SIZE is
# not established here. stats/xg_provider_scale.py measures a c of 1.27, but that
# is a different season on a different feed (2015/16, on 175 of 380 shared
# fixtures, while FPL uses Opta which that script says it does not identify);
# season-matched on the realised-xGC dump the comparable figure is 1.3291, this
# fit's clustered interval is ~[1.19, 1.39], and the pure-scale family that gives
# c its meaning is rejected on those rows by the LRT above (p 0.00075). Read the
# order of magnitude, never the third decimal.
#
# On a realised-match-xGC dump this returns ~1.28, on the model's own smoothed
# XGC90 ~1.00. ⚠️ That contrast is NOT "the smooth regressor is better
# calibrated" -- it is a cancellation of two opposing wedges that net to ~1.026
# on the realised-xGC dump, where both are visible. ⚠️ Quote that product and
# never the wedges themselves: c is fitted so `mean(exp(-c*x))` equals the
# observed rate, so the decomposition TELESCOPES and the parts are an artefact of
# where the identity is cut -- the same rows split elsewhere give +32.7%/-22.6%
# against -33.8%/+55.1%, for an identical product. Sizes and their annotation
# live in stats/findings/2026-08-15-clean-sheet-2x2.md.
#
# ⚠️ And the fragility is in the MEAN, not the dispersion. The whole dispersion
# channel is 1.0410, so annihilating XGC90's dispersion moves calibration 4.0%
# and its season range spans 2.5%, while calibration goes as exp((c-1)*x_bar) and
# a 10% shift in the MEAN moves it 4.1%. The reconstructed-xGC seasons enter by
# that level channel. ⚠️ Arithmetic off the banked rows, not a measurement: it
# assumes c is population-independent, which nobody has checked.
local({
  target <- mean(d$clean_sheet)
  # f is NON-INCREASING in cc for x >= 0 and strictly decreasing whenever any
  # x > 0, so the root is unique and bisection is legitimate. Stated because
  # `uniroot` does NOT require monotonicity -- it will happily return one of
  # several roots -- so this property is the whole reason the call is safe.
  f <- function(cc) mean(exp(-cc * d$xgc)) - target
  if (is.na(f(0.01)) || is.na(f(5)) || f(0.01) * f(5) >= 0) {
    # Given monotonicity, no root in the bracket means a data pathology, so this
    # fails rather than printing a note and continuing -- the same standard
    # `checked()` applies above: a non-converged fit that still prints a
    # coefficient is worse than no output at all.
    fail("c_implied: no root in [0.01, 5], or NA in xgc/clean_sheet")
  }
  cc <- uniroot(f, c(0.01, 5), tol = 1e-10)$root
  cat(sprintf("\nc_implied = %.3f   (n = %d, observed rate %.4f)\n",
              cc, nrow(d), target))
  cat("  Solves mean(exp(-c*x)) = observed rate. ⚠️ EXACTLY IDENTIFIED -- one\n")
  cat("  parameter, one moment -- so it reproduces that moment by construction\n")
  cat("  and CANNOT TEST any mechanism. It is a moment summary of the aggregate\n")
  cat("  over-prediction in exponent units, and it returns the same value for\n")
  cat("  every rival story reproducing that one number. Clustered SE ~0.05 on the\n")
  cat("  realised-xGC dump, so the third decimal is noise; and the pure-scale\n")
  cat("  family it assumes may be REJECTED on the same rows -- read the LRT above.\n")
  if (!("def" %in% names(d))) {
    # The gloss is a regressor discriminator and only means anything where `xgc`
    # is a single quantity. On the fixture-path dump that column is the PRODUCT
    # XGC90 x def, so c mixes the two knobs and has no shot-level reading.
    cat("  As a DISCRIMINATOR: ~1.28 says this dump's regressor is realised match\n")
    cat("  xGC, ~1.00 says it is the model's smoothed XGC90. ⚠️ The ~1.00 end is a\n")
    cat("  cancellation on this population, not an identification.\n")
  }
})

# --- the two-channel decomposition, when the dump carries `def` ------------
#
# ⚠️ Runs ONLY on a dump whose `xgc` column is the PRODUCT `XGC90 x def` and which
# carries `def` and `xgc90` beside it. On the neutral dump there is no `def` and
# this block is skipped -- which is correct, not a silent failure: with def == 1
# the second regressor is identically zero and the decomposition does not exist.
#
# Why it exists. `ladder(base, scale)` in internal/analysis/sweep.go is
# `1 + scale*(base-1)`, so the engine's exponent under the two shipped knobs is
#
#     f * x * (1 + s*(def-1))  =  f*x  +  f*s*x*(def-1)
#
# -- LINEAR in two regressors this dump already carries separately. So
# `b1 = f` (FPL_CS_XGC_FACTOR) and `b2 = f*s` (f times FPL_DEF_FIXTURE_SCALE's
# defensive half), and the engine's shipped position is b1 = b2 = 1. A mis-set
# clean-sheet factor moves BOTH; a mis-scaled defensive ladder moves only b2.
#
# That is the discrimination a single-regressor fit on the product cannot make,
# and the reason it was briefly recorded here as impossible. It is not: `def`
# varies over five levels within every season and is very nearly orthogonal to
# `x` in this population, so the two channels are separately identified.
if (all(c("def", "xgc90") %in% names(d))) {
  d$w1 <- d$xgc90
  d$w2 <- d$xgc90 * (d$def - 1)
  cat("\ntwo-channel decomposition  -ln p = a + b1*x + b2*x*(def-1)\n")
  cat("  b1 is FPL_CS_XGC_FACTOR; b2 is that times FPL_DEF_FIXTURE_SCALE's\n")
  cat("  defensive half. The shipped position is b1 = b2 = 1.\n")
  fit2 <- try(glm(clean_sheet ~ w1 + w2, data = d,
                  family = binomial(link = "log"),
                  start = c(-0.1, -1, -1)), silent = TRUE)
  if (inherits(fit2, "try-error")) {
    cat("  the joint fit did not converge; reported as a failure, not omitted\n")
  } else {
    # Same clustering axis as fit_one above -- season x team -- so the two
    # blocks' standard errors are comparable rather than two conventions.
    cl2 <- interaction(d$season, d$team, drop = TRUE)
    V <- clubSandwich::vcovCR(fit2, cluster = cl2, type = "CR2")
    co <- -coef(fit2)          # -ln p, so signs flip
    se <- sqrt(diag(V))
    b1 <- co[["w1"]]; b2 <- co[["w2"]]
    s1 <- se[["w1"]]; s2 <- se[["w2"]]
    # The difference is the discriminating contrast and its SE needs the
    # covariance -- the two coefficients are fitted on the same rows.
    sd12 <- sqrt(s1^2 + s2^2 - 2 * V["w1", "w2"])
    cat(sprintf("  b1 = %+.4f (clustered %.4f)   t(b1 = 1) = %+.2f\n", b1, s1, (b1 - 1) / s1))
    cat(sprintf("  b2 = %+.4f (clustered %.4f)   t(b2 = 1) = %+.2f\n", b2, s2, (b2 - 1) / s2))
    cat(sprintf("  t(b2 - b1) = %+.2f          implied s = b2/b1 = %.3f\n", (b2 - b1) / sd12, b2 / b1))
    cat(sprintf("  cor(w1, w2) = %.3f  — near zero means the two channels are\n",
                cor(d$w1, d$w2)))
    cat("  separately identified rather than trading off against each other.\n")
    cat("  ⚠️ POST-HOC: this decomposition was written after seeing a product fit.\n")
    cat("  ⚠️ `def` is read from the archive's END-OF-SEASON fixtures file and is\n")
    cat("     not gated by the replay cutoff. This caveat used to add that two live\n")
    cat("     captures three days apart show 0 of 380 difficulties changing, so\n")
    cat("     revision looks rare. WITHDRAWN 2026-08-16: 0 of 20 clubs changed any\n")
    cat("     team field between those same two captures, so the antecedent never\n")
    cat("     fired and the observation cannot distinguish `difficulty does not\n")
    cat("     track strength' from `nothing moved'. It was also pre-season and\n")
    cat("     reached no archived season.\n")
    cat("     What replaces it is a measurement: FPL's difficulty is an exact step\n")
    cat("     function of the opponent's fine strength_overall_* rating, and the\n")
    cat("     archive's column is reproduced by the END-of-season strength on\n")
    cat("     4560/4560 fixture-sides against 2755/4560 by the season-start value.\n")
    cat("     The column IS end-stamped. Pricing that leak on this fit is\n")
    cat("     UNMEASURABLE at three season clusters --\n")
    cat("     stats/defensive_fixture_pointintime/fit.txt.\n")
    cat("\n  per season:\n")
    for (sn in sort(unique(d$season))) {
      rs <- d[d$season == sn, ]
      f2 <- try(glm(clean_sheet ~ w1 + w2, data = rs,
                    family = binomial(link = "log"),
                    start = c(-0.1, -1, -1)), silent = TRUE)
      if (inherits(f2, "try-error")) {
        cat(sprintf("    %-10s %6d   did not converge\n", sn, nrow(rs)))
      } else {
        c2 <- -coef(f2)
        cat(sprintf("    %-10s %6d   b1 %+.4f   b2 %+.4f\n", sn, nrow(rs),
                    c2[["w1"]], c2[["w2"]]))
      }
    }
  }
}

# Per season, because the record's standard for a shape is that it holds in every
# season rather than pooled. A slope on the same side of 1 in every season is the
# held-out-season standard; a pooled figure alone is not.
cat("\nper season, free fit:\n")
cat(sprintf("  %-10s %8s %8s %8s\n", "season", "n", "a", "b"))
for (sn in sort(unique(d$season))) {
  rows <- d[d$season == sn, ]
  # A count threshold would be arbitrary. What actually has to hold is that both
  # outcomes occur and the fit converges -- and a season dropped is SAID, because
  # silent truncation reads downstream as "this season had no signal".
  if (length(unique(rows$clean_sheet)) < 2) {
    note("  !!  ", sn, " dropped: only one outcome class in ", nrow(rows), " rows")
    next
  }
  f <- try(glm(clean_sheet ~ xgc, data = rows,
               family = binomial(link = "log"), start = c(-0.2, -0.9)),
           silent = TRUE)
  if (inherits(f, "try-error") || !f$converged || isTRUE(f$boundary)) {
    note("  !!  ", sn, " dropped: fit did not converge on ", nrow(rows), " rows")
    next
  }
  cat(sprintf("  %-10s %8d %+8.4f %+8.4f\n",
              sn, nrow(rows), -coef(f)[1], -coef(f)[2]))
}

# The bucket-mean estimator the Go diagnostic prints, reproduced here on the SAME
# rows, so the gap between the two is measured rather than asserted.
cat("\nthe Go diagnostic's bucket-mean estimator, on these same rows:\n")
brk <- c(0, 0.7, 1.0, 1.3, 1.6, 2.0, Inf)
d$bucket <- cut(d$xgc, brk, right = FALSE)
agg <- aggregate(cbind(pred = exp(-d$xgc), act = d$clean_sheet),
                 by = list(bucket = d$bucket), FUN = mean)
# ⚠️ `aggregate` drops empty factor levels and `table` does not, so indexing the
# counts positionally silently mismatches them the moment a bucket is empty --
# wrong weights, no error. Match on the level instead.
cnt <- as.numeric(table(d$bucket))[match(agg$bucket, levels(d$bucket))]
ok <- agg$pred > 0 & agg$pred < 1 & agg$act > 0 & agg$act < 1
w <- cnt[ok]
bx <- -log(agg$pred[ok])
by <- -log(agg$act[ok])
bb <- sum(w * (bx - weighted.mean(bx, w)) * (by - weighted.mean(by, w))) /
      sum(w * (bx - weighted.mean(bx, w))^2)
ba <- weighted.mean(by, w) - bb * weighted.mean(bx, w)
cat(sprintf("  bucket-mean fit   a = %+.4f   b = %+.4f   (%d of %d buckets used)\n",
            ba, bb, sum(ok), length(ok)))
cat("  (the per-observation MLE above is the one that carries a verdict)\n")
