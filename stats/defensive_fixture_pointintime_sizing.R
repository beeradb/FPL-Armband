#!/usr/bin/env Rscript
#
# Size the canary for the point-in-time refit, BEFORE any standard error exists.
#
#   Rscript stats/defensive_fixture_pointintime_sizing.R \
#     stats/defensive_fixture_pointintime/joined_rows.csv
#
# ---------------------------------------------------------------------------
# Why this file exists at all, and why it runs before the fit
#
# The predecessor arm (`defensive-fixture-coefficient-hindsight-gate`) sized its
# canary with `(b2 - 1) / q_w`, an **OLS omitted-variable formula applied to a
# log-link GLM** with no partialling and no IWLS weights. It read 1.977 where the
# exact answer was 1.685 against a threshold of 1.702 — so the approximation
# said "the canary does not fire" and the exact quantity said it does. The error
# ran in the direction that flatters the instrument, and it decided the verdict.
#
# Nothing here is approximated. Every pole is obtained by writing down a complete
# data-generating process, taking its expectations row by row, and **refitting
# the working model to those expectations by the same IWLS the real fit uses**.
# That is the population projection of the DGP onto the working model, which is
# exactly what the estimator converges to, and it is computed rather than
# derived.
#
# ---------------------------------------------------------------------------
# The two poles, and why the obvious sizing is wrong
#
# The estimand is `b2_pit`, the coefficient on `w2_pit = XGC90 * (def_pit - 1)`
# in `-ln P(clean sheet) = a + b1*w1 + b2*w2_pit`. The banked figure it is
# compared against is `b2_end = 1.5688`.
#
# The tempting sizing is "a full artefact means b2_pit = 1, so the effect to be
# detected is 1.5688 - 1 = 0.5688". **That is wrong, and it is wrong in the same
# direction as the predecessor's error.** Under the NO-artefact hypothesis
# `b2_pit` is NOT 1.5688: if the season-long column is the truth, the cutoff
# column mismeasures it, and Model P is misspecified, so `b2_pit` is attenuated
# below 1.5688 by an amount that depends on how much the reconstruction moves.
# The separation the instrument actually has to resolve is between the two POLES,
# not between one pole and the banked number.
#
#   pole N -- NO artefact. The banked model is the truth:
#             mu = exp(-(a + b1*w1 + 1.5688*w2_end)).
#             Reproduces b2_end = 1.5688 by construction. What it yields is the
#             value Model P returns when the ONLY thing wrong with the cutoff
#             column is that it mismeasures the season-long one.
#
#   pole H -- FULL artefact. Must satisfy TWO constraints at once: Model P
#             returns exactly 1 (the hypothesis), AND Model E returns exactly
#             1.5688 (the observed banked fit). An unconstrained
#             "mu = exp(-(a + b1*w1 + 1*w2_pit))" DGP satisfies the first and
#             badly fails the second, so it is not a candidate for what happened.
#             The honest version is a 2x2 solve over (c_pit, c_rev) on
#             w2_pit and w2_rev = w2_end - w2_pit, and it is solved here to a
#             printed residual rather than assumed solvable.
#
# The self-consistency checks that make the construction auditable are printed:
# pole H must return b2_pit = 1.000000 and b2_end = 1.568809 simultaneously, and
# pole N must return a Model D contrast of exactly 0.
#
# ⚠️ **What was looked at to produce this file**, stated so the pre-registration
# can be audited: the CONTROL fit's `a`, `b1` and `b2_end` on the native stratum
# — the fit that the pre-registration separately REQUIRES to reproduce the banked
# figures, so it carries no information the record does not already hold. The
# estimand `b2_pit` was not fitted to any outcome here, and no standard error of
# any kind was computed. Model P appears only against synthetic expectations.

args <- commandArgs(trailingOnly = TRUE)
path <- if (length(args) >= 1) args[1] else
  "stats/defensive_fixture_pointintime/joined_rows.csv"
NATIVE <- c("2023-24", "2024-25", "2025-26")
BANKED_B2 <- 1.5688

# `read_sidecar` is in cells_common.R. This input is NOT a cells file — it is one
# row per team-match — so it goes through the sanctioned sidecar reader. The guard
# in TestTheSharedCellQuantitiesHaveOneImplementation refuses a raw `read.csv(`
# anywhere in stats/*.R outside that file.
local({
  a <- commandArgs(trailingOnly = FALSE)
  f <- sub("^--file=", "", a[grep("^--file=", a)])
  dd <- if (length(f) > 0) dirname(normalizePath(f[1])) else "stats"
  p <- file.path(dd, "cells_common.R")
  if (!file.exists(p)) {
    stop("cannot find cells_common.R beside this script (looked in ", dd, ")")
  }
  source(p, local = FALSE)
})

d <- read_sidecar(path)
d <- d[d$season %in% NATIVE, ]
d$w1 <- d$xgc90
d$w2e <- d$xgc90 * (d$def - 1)
d$w2p <- d$xgc90 * (d$def_pit - 1)
d$w2r <- d$w2e - d$w2p

cat("------------------------------------------------------------------\n")
cat("CANARY SIZING for the point-in-time refit -- native stratum\n")
cat("------------------------------------------------------------------\n")
cat(sprintf("n = %d   seasons: %s\n", nrow(d),
            paste(sort(unique(d$season)), collapse = ", ")))

# --- design quantities: no outcome enters any of these ---------------------
cat("\nDESIGN (no outcome enters):\n")
cat(sprintf("  rows the reconstruction moves            %d / %d = %.4f\n",
            sum(d$def_moved), nrow(d), mean(d$def_moved)))
cat(sprintf("  rows informing w2_rev (w2_rev != 0)      %d\n", sum(d$w2r != 0)))
cat(sprintf("  rows where w2_pit == 0 (def_pit == 1)    %d\n", sum(d$w2p == 0)))
cat(sprintf("  cor(w2_pit, w2_rev) = %+.3f   cor(w1, w2_rev) = %+.3f\n",
            cor(d$w2p, d$w2r), cor(d$w1, d$w2r)))
cat(sprintf("  cor(w2_end, w2_pit) = %+.3f\n", cor(d$w2e, d$w2p)))
cat(sprintf("  share of sum(w2_end^2) on moved rows = %.4f\n",
            sum(d$w2e[d$def_moved == 1]^2) / sum(d$w2e^2)))

checked <- function(f, label) {
  if (!f$converged || isTRUE(f$boundary)) {
    stop(label, ": IWLS did not converge cleanly")
  }
  f
}

# The CONTROL, on real outcomes. This is the ONLY outcome fit in this file, and
# the pre-registration independently requires it to reproduce the banked figures.
ctrl <- checked(glm(clean_sheet ~ w1 + w2e, data = d,
                    family = binomial(link = "log"), start = c(-0.1, -1, -1)),
                "control")
co <- -coef(ctrl)
a <- co[[1]]; b1 <- co[[2]]; b2 <- co[[3]]
cat(sprintf("\nCONTROL (the banked model, refit here on real outcomes):\n"))
cat(sprintf("  a = %+.6f   b1 = %+.6f   b2_end = %+.6f\n", a, b1, b2))
if (abs(b2 - BANKED_B2) > 5e-5 || abs(b1 - 1.0476) > 5e-5) {
  stop(sprintf("CONTROL FAILED: b2_end = %.6f b1 = %.6f against the banked %.4f / 1.0476",
               b2, b1, BANKED_B2))
}
cat("  CONTROL PASSES: reproduces the banked b2 = 1.5688 and b1 = 1.0476.\n")

# The population projection of a DGP onto a working model, by IWLS on the DGP's
# own expectations. Non-integer responses warn under binomial(); the IWLS
# solution is still the quasi-likelihood projection, which is the object wanted.
project <- function(mu, form, start) {
  f <- suppressWarnings(glm(form, data = transform(d, y = mu),
                            family = binomial(link = "log"), start = start))
  checked(f, "projection")
  -coef(f)
}
mu2 <- function(cp, cr, aa = a) exp(-(aa + b1 * d$w1 + cp * d$w2p + cr * d$w2r))
P3 <- c(-0.1, -1, -1)
P4 <- c(-0.1, -1, -1, -1)

# --- pole N ---------------------------------------------------------------
muN <- mu2(b2, b2)
poleN_pit <- project(muN, y ~ w1 + w2p, P3)[[3]]
poleN_end <- project(muN, y ~ w1 + w2e, P3)[[3]]
dN <- project(muN, y ~ w1 + w2p + w2r, P4)
cat("\nPOLE N -- no artefact; the banked model IS the truth\n")
cat(sprintf("  Model P -> b2_pit = %.6f      Model E -> b2_end = %.6f\n",
            poleN_pit, poleN_end))
cat(sprintf("  Model D -> b_pit = %.6f  b_rev = %.6f  contrast = %.6f\n",
            dN[[3]], dN[[4]], dN[[4]] - dN[[3]]))
cat("  self-consistency: Model E returns the banked b2 and the Model D contrast\n")
cat("  is exactly 0, both by construction.\n")

# --- pole H, constrained --------------------------------------------------
obj <- function(p) {
  mu <- mu2(p[1], p[2])
  c(project(mu, y ~ w1 + w2p, P3)[[3]] - 1, project(mu, y ~ w1 + w2e, P3)[[3]] - b2)
}
p <- c(1, 2)
for (it in 1:60) {
  f0 <- obj(p)
  if (max(abs(f0)) < 1e-10) break
  J <- matrix(0, 2, 2)
  for (j in 1:2) {
    q <- p; q[j] <- q[j] + 1e-5
    J[, j] <- (obj(q) - f0) / 1e-5
  }
  p <- p - solve(J, f0)
}
resid <- max(abs(obj(p)))
if (resid > 1e-8) stop("pole H did not solve: residual ", resid)
muH <- mu2(p[1], p[2])
dH <- project(muH, y ~ w1 + w2p + w2r, P4)
cat("\nPOLE H -- full artefact, CONSTRAINED to reproduce the banked fit\n")
cat(sprintf("  solved c_pit = %.6f   c_rev = %.6f   residual %.2e\n",
            p[1], p[2], resid))
cat(sprintf("  Model P -> b2_pit = %.6f      Model E -> b2_end = %.6f\n",
            project(muH, y ~ w1 + w2p, P3)[[3]], project(muH, y ~ w1 + w2e, P3)[[3]]))
cat(sprintf("  Model D -> b_pit = %.6f  b_rev = %.6f  contrast = %.6f\n",
            dH[[3]], dH[[4]], dH[[4]] - dH[[3]]))
cat("  For the whole excess to be hindsight, the REVISION component has to carry\n")
cat(sprintf("  a slope of %.3f against the point-in-time component's %.3f.\n",
            p[2], p[1]))

# What an UNCONSTRAINED full-artefact DGP would have produced, as a declared
# diagnostic: it says how much of the banked 1.5688 cannot be manufactured by
# mismeasurement and must come from def_end tracking outcomes.
muU <- mu2(1, 0)   # c_pit = 1, c_rev = 0: the truth is 1 * w2_pit and nothing else
cat(sprintf("\nDIAGNOSTIC: an UNCONSTRAINED 'truth = 1 * w2_pit' DGP returns\n"))
cat(sprintf("  Model E b2_end = %.6f, not %.4f -- so mismeasurement alone cannot\n",
            project(muU, y ~ w1 + w2e, P3)[[3]], BANKED_B2))
cat("  manufacture the banked coefficient, and that is why pole H is constrained.\n")

# --- the separations ------------------------------------------------------
sep_level <- abs(poleN_pit - 1)
sep_D <- abs((dH[[4]] - dH[[3]]) - (dN[[4]] - dN[[3]]))
cat("\n------------------------------------------------------------------\n")
cat("SEPARATIONS -- what the instrument has to resolve\n")
cat("------------------------------------------------------------------\n")
cat(sprintf("  on b2_pit (Model P level):      |%.6f - %.6f| = %.6f\n",
            poleN_pit, 1.0, sep_level))
cat(sprintf("  on the Model D contrast:        |%.6f - %.6f| = %.6f\n",
            dH[[4]] - dH[[3]], dN[[4]] - dN[[3]], sep_D))
cat(sprintf("  the NAIVE sizing, which is WRONG: %.6f -- it over-states the\n",
            b2 - 1))
cat(sprintf("  separation by %.1f%%, in the direction that flatters the instrument.\n",
            100 * ((b2 - 1) / sep_level - 1)))
cat("\nCANARY, declared before any standard error exists:\n")
cat(sprintf("  H2 (b2_pit level) is UNMEASURABLE if 4.303 * SE(b2_pit) > %.6f,\n",
            sep_level))
cat(sprintf("     i.e. unless SE(b2_pit) < %.6f\n", sep_level / 4.303))
cat(sprintf("  H1 (Model D contrast) is UNMEASURABLE if 4.303 * SE > %.6f,\n", sep_D))
cat(sprintf("     i.e. unless SE(contrast) < %.6f\n", sep_D / 4.303))

cat("\nSENSITIVITY of pole N to the DGP intercept (it is nearly flat):\n")
for (da in c(-0.10, -0.05, 0, 0.05, 0.10)) {
  cat(sprintf("  a %+0.2f -> pole N b2_pit = %.6f\n", da,
              project(mu2(b2, b2, aa = a + da), y ~ w1 + w2p, P3)[[3]]))
}
