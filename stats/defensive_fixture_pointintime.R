#!/usr/bin/env Rscript
#
# Refit the defensive fixture coefficient on the difficulty that was actually
# available at each cutoff.
#
#   python3 stats/defensive_fixture_pointintime_join.py \
#     stats/snapshots/2026-08-15-clean-sheet-2x2/cs_regressor_fixture_path_rows.csv \
#     stats/defensive_fixture_pointintime/joined_rows.csv
#   Rscript stats/defensive_fixture_pointintime_sizing.R \
#     stats/defensive_fixture_pointintime/joined_rows.csv
#   Rscript stats/defensive_fixture_pointintime.R \
#     stats/defensive_fixture_pointintime/joined_rows.csv
#
# Pre-registration: stats/defensive_fixture_pointintime_PREREGISTRATION.md,
# committed before this file existed. Read it first; this script implements it
# and does not re-argue it. The canary poles below are NOT recomputed here — they
# are the frozen numbers from the sizing run, so that a change to the sizing code
# cannot silently move a bar this script is judged against.

args <- commandArgs(trailingOnly = TRUE)
path <- if (length(args) >= 1) args[1] else
  "stats/defensive_fixture_pointintime/joined_rows.csv"

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

NATIVE <- c("2023-24", "2024-25", "2025-26")

# --- frozen from the pre-registration, §5 and §7 ---------------------------
BANKED <- list(native = list(b2 = 1.5688, b1 = 1.0476, se_st = 0.2253),
               pooled = list(b2 = 1.5654, b1 = 0.9568, se_st = 0.1712))
POLE_N_LEVEL <- 1.434919      # b2_pit under "no artefact"
POLE_H_LEVEL <- 1.000000      # b2_pit under "full artefact"
POLE_N_CONTRAST <- 0.000000   # Model D contrast under "no artefact"
POLE_H_CONTRAST <- 1.202761   # Model D contrast under "full artefact"
SEP_LEVEL <- abs(POLE_N_LEVEL - POLE_H_LEVEL)          # 0.434919
SEP_CONTRAST <- abs(POLE_H_CONTRAST - POLE_N_CONTRAST) # 1.202761
# Quoted from sizing.txt, not recomputed from the rounded BANKED$b2 above.
NAIVE_SEP <- 0.568809

hr <- function() cat(strrep("-", 78), "\n")

d <- read_sidecar(path)
for (c_ in c("season", "gw", "team", "clean_sheet", "def", "def_pit", "xgc90")) {
  if (!(c_ %in% names(d))) fail("the joined rows are missing ", c_)
}
d$w1 <- d$xgc90
d$w2e <- d$xgc90 * (d$def - 1)
d$w2p <- d$xgc90 * (d$def_pit - 1)
d$w2r <- d$w2e - d$w2p

# A glm that fails to converge returns with a WARNING and coefficients from
# wherever IWLS stopped, and Rscript defers warnings to the end of the run —
# after every table has printed. That is the silent-failure shape exactly.
checked <- function(f, label) {
  if (!f$converged || isTRUE(f$boundary) || max(fitted(f)) > 0.999) {
    fail(label, ": glm did not converge cleanly (converged=", f$converged,
         " boundary=", isTRUE(f$boundary),
         " max fitted p=", signif(max(fitted(f)), 4), ")")
  }
  f
}

# One fit, its two clusterings, and everything printed about it. `terms` is the
# named vector of nulls each coefficient is tested against.
fit_block <- function(rows, form, label, nulls, tcrit, df, start) {
  f <- checked(glm(form, data = rows, family = binomial(link = "log"),
                   start = start), label)
  cl_s <- factor(rows$season)
  cl_st <- interaction(rows$season, rows$team, drop = TRUE)
  V <- try(clubSandwich::vcovCR(f, cluster = cl_s, type = "CR2"), silent = TRUE)
  se_type <- "CR2"
  if (inherits(V, "try-error")) {
    V <- sandwich::vcovCL(f, cluster = cl_s)
    se_type <- "CR0 (clubSandwich CR2 not computable — SAID, not silently switched)"
  }
  Vst <- clubSandwich::vcovCR(f, cluster = cl_st, type = "CR2")
  co <- -coef(f)
  se <- sqrt(diag(V))
  se_st <- sqrt(diag(Vst))

  cat(sprintf("\n%s\n  n = %d, season clusters = %d, df = %d, t_crit = %.3f, SE type %s\n",
              label, nrow(rows), nlevels(cl_s), df, tcrit, se_type))
  # ⚠️ Correction 7. The rank printed is CR2's, after its adjustment. The
  # underlying CR0 cluster meat is rank G-1 in EVERY model here, because the
  # cluster scores sum to zero — so a warning that prints only where CR2 also
  # comes up short makes the other models look better founded than they are.
  # `df = 2` already encodes the real constraint.
  cat(sprintf("  rank(vcov) = %d of %d parameters (the CR0 cluster meat is rank %d in\n",
              qr(V)$rank, ncol(V), nlevels(cl_s) - 1))
  cat(sprintf("  every model here, G-1, whatever CR2's adjustment reports)%s\n",
              if (qr(V)$rank < ncol(V))
                "\n  ⚠️ CR2 IS ALSO RANK-DEFICIENT HERE: G clusters cannot span p parameters"
              else ""))
  sw <- try(clubSandwich::coef_test(f, vcov = Vst, test = "Satterthwaite"),
            silent = TRUE)
  if (!inherits(sw, "try-error")) {
    cat("  Satterthwaite df on the season x team clustering (context; the verdict\n")
    cat("  uses the pre-registered G-1):",
        paste(sprintf("%s %.1f", rownames(sw), sw$df_Satt), collapse = "  "), "\n")
  }
  cat(sprintf("  %-6s %10s %10s %8s %10s %10s %11s\n",
              "term", "estimate", "SE(season)", "t vs H0", "H0", "threshold",
              "SE(sxteam)"))
  for (nm in names(nulls)) {
    h0 <- nulls[[nm]]
    cat(sprintf("  %-6s %+10.4f %10.4f %+8.2f %10.4f %10.4f %11.4f\n",
                nm, co[[nm]], se[[nm]], (co[[nm]] - h0) / se[[nm]], h0,
                tcrit * se[[nm]], se_st[[nm]]))
  }
  list(fit = f, co = co, se = se, se_st = se_st, V = V, Vst = Vst)
}

# A linear contrast of the NEGATED coefficients, with the clustered variance.
contrast_of <- function(b, e) {
  # b$co is -coef, so a contrast on the negated scale is the negated contrast on
  # the raw scale; the variance is identical either way.
  est <- sum(e * b$co[names(e)])
  ev <- rep(0, length(b$co)); names(ev) <- names(b$co); ev[names(e)] <- e
  se <- sqrt(as.numeric(t(ev) %*% b$V %*% ev))
  se_st <- sqrt(as.numeric(t(ev) %*% b$Vst %*% ev))
  list(est = est, se = se, se_st = se_st)
}

stratum <- function(rows, label, banked, tcrit, df, verdict) {
  hr(); cat(toupper(label), "\n"); hr()

  cat("\nCONTROL: the banked model refitted here. It must reproduce b2_end,\n")
  cat("b1 and the season x team CR2 SE, or nothing from this run may be quoted.\n")
  E <- fit_block(rows, clean_sheet ~ w1 + w2e, "Model E (banked, end-stamped def)",
                 c(w1 = 1, w2e = 1), tcrit, df, c(-0.1, -1, -1))
  ok <- abs(E$co[["w2e"]] - banked$b2) < 5e-4 &&
        abs(E$co[["w1"]] - banked$b1) < 5e-4 &&
        abs(E$se_st[["w2e"]] - banked$se_st) < 5e-4
  if (!ok) {
    fail(sprintf("CONTROL FAILED: b2_end = %.4f (want %.4f), b1 = %.4f (want %.4f), ",
                 E$co[["w2e"]], banked$b2, E$co[["w1"]], banked$b1),
         sprintf("season x team SE = %.4f (want %.4f). ",
                 E$se_st[["w2e"]], banked$se_st),
         "The estimator or the population is wrong; no result may be quoted.")
  }
  cat("  CONTROL PASSES.\n")

  cat("\nMODEL P -- the PRIMARY estimand. `def` rebuilt at each row's own cutoff,\n")
  cat("all rows, not an interaction on a subset. The two poles it is read against\n")
  cat(sprintf("are pole N %.6f (no artefact) and pole H %.6f (full artefact).\n",
              POLE_N_LEVEL, POLE_H_LEVEL))
  P <- fit_block(rows, clean_sheet ~ w1 + w2p, "Model P (point-in-time def)",
                 c(w1 = 1, w2p = POLE_N_LEVEL), tcrit, df, c(-0.1, -1, -1))
  b2p <- P$co[["w2p"]]; se2p <- P$se[["w2p"]]
  cat(sprintf("  b2_pit = %+.4f. Against the other reference points, same SE %.4f\n",
              b2p, se2p))
  refs <- c("the banked b2_end" = banked$b2,
            "pole H (full artefact)" = POLE_H_LEVEL,
            "the shipped position 1" = 1)
  for (i in seq_along(refs)) {
    nm <- refs[[i]]
    cat(sprintf("    vs %-24s %+8.4f   t %+6.2f   threshold %.4f   %s\n",
                names(refs)[i], nm, (b2p - nm) / se2p, tcrit * se2p,
                if (abs(b2p - nm) > tcrit * se2p) "SEPARATES" else "does not separate"))
  }

  cat("\nMODEL D -- H1. The end-stamped column split into the part available at\n")
  cat("the cutoff and the revision the end-stamp adds. Under pole N the contrast\n")
  cat("b_rev - b_pit is EXACTLY 0; under pole H it is +1.202761.\n")
  cat(sprintf("  cor(w2_pit, w2_rev) = %+.3f   cor(w1, w2_rev) = %+.3f   rows with w2_rev != 0: %d\n",
              cor(rows$w2p, rows$w2r), cor(rows$w1, rows$w2r), sum(rows$w2r != 0)))
  D <- fit_block(rows, clean_sheet ~ w1 + w2p + w2r,
                 "Model D (cutoff channel + revision channel)",
                 c(w1 = 1, w2p = 1, w2r = 1), tcrit, df, c(-0.1, -1, -1, -1))
  ct <- contrast_of(D, c(w2r = 1, w2p = -1))
  cat(sprintf("  CONTRAST b_rev - b_pit = %+.4f   SE(season) %.4f   t vs 0 %+.2f\n",
              ct$est, ct$se, ct$est / ct$se))
  cat(sprintf("                          threshold %.4f   SE(sxteam) %.4f\n",
              tcrit * ct$se, ct$se_st))

  # --- S4: b1 confinement -------------------------------------------------
  cat(sprintf("\nS4 (diagnostic, not a test): b1 is %+.4f under Model E and %+.4f\n",
              E$co[["w1"]], P$co[["w1"]]))
  cat("  under Model P. w1 is untouched by the reconstruction, so a large move\n")
  cat("  would mean the fit broke rather than the fixture channel.\n")

  # --- S3: per-season shape ------------------------------------------------
  cat("\nS3 (shape, not a test — no standard errors, so a sign flip in one\n")
  cat("season is not a result):\n")
  cat(sprintf("  %-10s %6s %10s %10s %10s\n", "season", "n", "b2_end", "b2_pit",
              "b_rev-b_pit"))
  for (sn in sort(unique(rows$season))) {
    rs <- rows[rows$season == sn, ]
    fe <- try(glm(clean_sheet ~ w1 + w2e, data = rs, family = binomial(link = "log"),
                  start = c(-0.1, -1, -1)), silent = TRUE)
    fp <- try(glm(clean_sheet ~ w1 + w2p, data = rs, family = binomial(link = "log"),
                  start = c(-0.1, -1, -1)), silent = TRUE)
    fd <- try(glm(clean_sheet ~ w1 + w2p + w2r, data = rs,
                  family = binomial(link = "log"), start = c(-0.1, -1, -1, -1)),
              silent = TRUE)
    ge <- if (inherits(fe, "try-error")) NA else -coef(fe)[["w2e"]]
    gp <- if (inherits(fp, "try-error")) NA else -coef(fp)[["w2p"]]
    gd <- if (inherits(fd, "try-error")) NA else
      -coef(fd)[["w2r"]] + coef(fd)[["w2p"]]
    cat(sprintf("  %-10s %6d %10.4f %10.4f %10.4f\n", sn, nrow(rs), ge, gp, gd))
  }

  if (!verdict) {
    cat("\n⚠️ THE H0 QUOTED IN THIS BLOCK'S MODEL P TABLE (1.4349) IS POLE N SIZED\n")
    cat("   ON THE NATIVE STRATUM — an IWLS projection over those 1566 rows with\n")
    cat("   those control coefficients. The POOLED poles were never sized, and\n")
    cat("   sizing them now would be a post-hoc arm. So NO CANARY IS EVALUATED\n")
    cat("   HERE, and none may be inferred by dividing a pooled threshold into a\n")
    cat("   native separation — including the inference that would read as `the\n")
    cat("   pooled contrast arm was detectable and came out near pole N'. That\n")
    cat("   reading is available from the numbers above and is not licensed.\n")
    cat("\n⚠️ THIS STRATUM CARRIES NO VERDICT IN EITHER DIRECTION, per the\n")
    cat("   pre-registration and the record's rule against leaning on a stratum\n")
    cat("   for a rejection while disowning it for a null. Three of these six\n")
    cat("   seasons carry reconstructed xGC, which makes w1 a different construct.\n")
    return(invisible(b2p))
  }

  # --- the canary ----------------------------------------------------------
  hr(); cat("THE CANARY -- sized before the fit, evaluated now\n"); hr()
  th_ct <- tcrit * ct$se
  th_lv <- tcrit * se2p
  cat(sprintf("  H1  separation %.6f   threshold %.6f   %s\n",
              SEP_CONTRAST, th_ct,
              if (th_ct > SEP_CONTRAST) "CANARY FIRES" else "detectable"))
  cat(sprintf("  H2  separation %.6f   threshold %.6f   %s\n",
              SEP_LEVEL, th_lv,
              if (th_lv > SEP_LEVEL) "CANARY FIRES" else "detectable"))
  # ⚠️ Quoted from the sizing run rather than recomputed from the rounded frozen
  # BANKED$b2, which returned 0.568800 against sizing.txt's 0.568809. One
  # quantity, two implementations, in miniature.
  cat(sprintf("  (the NAIVE H2 sizing would have been %.6f, which is %.1f%% larger\n",
              NAIVE_SEP, 100 * (NAIVE_SEP / SEP_LEVEL - 1)))
  cat(sprintf("   and would have read '%s' — the predecessor's error class, caught\n",
              if (th_lv > NAIVE_SEP) "fires" else "detectable"))
  cat("   here before the fit. It changed the MARGIN, not the answer.)\n")
  cat("  How far from resolving, so the sizing method can itself be judged:\n")
  cat(sprintf("    H2 would have been detectable only if pole N projected above %.3f;\n",
              POLE_H_LEVEL + th_lv))
  cat(sprintf("    H1 only if pole H's contrast were %.4f rather than %.4f.\n",
              th_ct, POLE_H_CONTRAST))
  cat("    The verdict therefore survives the sizing method by roughly a factor of\n")
  cat("    three, which also bounds every sensitivity of the poles to the control\n")
  cat("    coefficients they inherit — `a` is swept in sizing.txt and is flat, and\n")
  cat("    `b1` is not swept because cor(w1, w2_rev) = 0.041 makes the channel\n")
  cat("    nearly orthogonal to it.\n")
  fired <- c(H1 = th_ct > SEP_CONTRAST, H2 = th_lv > SEP_LEVEL)

  # --- the paired estimator, post-hoc and declared as such -----------------
  #
  # Correction 3. Reported because it ALSO fires: an estimator computed after
  # the fact can only be quoted when it cannot have been chosen for its answer,
  # and one that reaches the same verdict as the pre-registered arms cannot
  # have been. Leave-one-season-out jackknife, because at G = 3 a stacked CR0
  # sandwich understates and there is no CR2 for a stacked GLM system.
  seasons <- sort(unique(rows$season))
  pair_of <- function(sub) {
    e <- -coef(glm(clean_sheet ~ w1 + w2e, data = sub,
                   family = binomial(link = "log"), start = c(-0.1, -1, -1)))[["w2e"]]
    p <- -coef(glm(clean_sheet ~ w1 + w2p, data = sub,
                   family = binomial(link = "log"), start = c(-0.1, -1, -1)))[["w2p"]]
    e - p
  }
  G <- length(seasons)
  loo <- sapply(seasons, function(s) pair_of(rows[rows$season != s, ]))
  se_pair <- sqrt((G - 1) / G * sum((loo - mean(loo))^2))
  cat(sprintf("\n  PAIRED (post-hoc, correction 3): b2_end - b2_pit = %+.4f,\n",
              pair_of(rows)))
  cat(sprintf("    leave-one-season-out jackknife SE %.4f, threshold %.4f x %.3f = %.4f\n",
              se_pair, se_pair, tcrit, tcrit * se_pair))
  cat(sprintf("    against the same separation %.6f -> %s\n", SEP_LEVEL,
              if (tcrit * se_pair > SEP_LEVEL) "CANARY FIRES HERE TOO"
              else "detectable — which would CONTRADICT the pre-registered arms"))

  # --- Holm ---------------------------------------------------------------
  hr(); cat("DECISION (pre-registration section 8)\n"); hr()
  t1 <- ct$est / ct$se
  t2 <- (b2p - POLE_N_LEVEL) / se2p
  ps <- c(H1 = 2 * pt(-abs(t1), df), H2 = 2 * pt(-abs(t2), df))
  # p.adjust over the WHOLE declared family. An unestimable arm does not shrink
  # it; that was declared, and the cost it imposes stands.
  holm <- p.adjust(ps, method = "holm")
  cat(sprintf("  H1  b_rev - b_pit = %+.4f vs 0          t %+.2f  raw p %.4f  Holm %.4f\n",
              ct$est, t1, ps[["H1"]], holm[["H1"]]))
  cat(sprintf("  H2  b2_pit        = %+.4f vs %.6f  t %+.2f  raw p %.4f  Holm %.4f\n",
              b2p, POLE_N_LEVEL, t2, ps[["H2"]], holm[["H2"]]))
  cat(sprintf("  Holm thresholds at df %d: |t| > %.3f for the smaller p, %.3f for the larger\n",
              df, abs(qt(0.025 / 2, df)), tcrit))

  near_H <- abs(b2p - POLE_H_LEVEL) < abs(b2p - POLE_N_LEVEL)
  rejects <- holm < 0.05
  verdictv <-
    if (all(fired)) "C -- UNMEASURABLE" else
    if (rejects[["H1"]] && ct$est > 0 && near_H) "A -- HINDSIGHT CARRIES THE EXCESS" else
    if (!any(rejects) && abs(b2p - POLE_N_LEVEL) < 0.10 && !any(fired))
      "B -- HINDSIGHT DOES NOT CARRY THE EXCESS" else
    "D -- UNRESOLVED"
  cat(sprintf("\n  branch conditions: canary fired H1=%s H2=%s; Holm rejects H1=%s H2=%s;\n",
              fired[["H1"]], fired[["H2"]], rejects[["H1"]], rejects[["H2"]]))
  if (all(fired)) {
    # Correction 6. `verdictv` short-circuits on C before the A/B branches read
    # `near_H`, so printing which pole the estimate sits nearer would be a
    # directional statement about the estimand -- exactly what section 8 forbids
    # -- dressed as an audit line.
    cat("                     proximity to a pole: NOT EVALUATED — C short-circuits\n")
    cat("                     before the A/B branches read it, and under C it would\n")
    cat("                     be a reading rather than a branch condition.\n")
  } else {
    cat(sprintf("                     b2_pit is nearer pole %s; |b2_pit - pole N| = %.4f\n",
                if (near_H) "H" else "N", abs(b2p - POLE_N_LEVEL)))
  }
  cat(sprintf("\n  VERDICT: %s\n", verdictv))
  if (grepl("^C", verdictv)) {
    cat("  ⚠️ UNDER C THE PRE-REGISTRATION FORBIDS QUOTING ANY READING OF EITHER\n")
    cat("     ARM IN EITHER DIRECTION. The lines above are the decision rule's own\n")
    cat("     branch conditions, printed so the verdict can be audited — they are\n")
    cat("     NOT results. C is a fact about this INSTRUMENT, not about the\n")
    cat("     constant: it does not say b2 = 1.5688 is sound and it does not say\n")
    cat("     it is an artefact.\n")
  }
  if (grepl("^C", verdictv)) {
    # Under C the arms carry nothing, but the INSTRUMENT does, and saying why it
    # failed is the whole content of a C. These are standard errors and design
    # facts, not readings of the estimand.
    cat("\n  WHY IT DOES NOT RESOLVE -- a fact about the instrument, which is what\n")
    cat("  a C verdict is:\n")
    cat(sprintf("    SE(b2_end)  %.4f      SE(b2_pit)  %.4f   ratio %.2fx\n",
                E$se[["w2e"]], se2p, se2p / E$se[["w2e"]]))
    cat("    The reconstruction does not merely move the coefficient, it more than\n")
    cat("    doubles the season-clustered standard error, because the point-in-time\n")
    cat("    column's slope DISAGREES ACROSS SEASONS far more than the end-stamped\n")
    cat("    one's does. Season clustering is built to widen on exactly that, so\n")
    cat("    the arm loses power at the same time as it removes the leak.\n")
    cat(sprintf("    Which of the two it is, without reading S3 at all: the season x\n"))
    cat(sprintf("    team CR2 SE goes %.4f -> %.4f (%+.1f%%) against %+.1f%% on the\n",
                E$se_st[["w2e"]], P$se_st[["w2p"]],
                100 * (P$se_st[["w2p"]] / E$se_st[["w2e"]] - 1),
                100 * (se2p / E$se[["w2e"]] - 1)))
    cat("    season-clustered one. The regressor did not get weaker; the seasons\n")
    cat("    stopped agreeing. That is a restatement of what a season-clustered SE\n")
    cat("    IS, not a second reading of S3.\n")
    cat("    ⚠️ DO NOT carry `de-contaminating a regressor is not free' forward as\n")
    cat("       a standing claim. It is n = 1; the ratio is between two variance\n")
    cat("       estimates each carrying 2 df from the SAME three clusters, so it is\n")
    cat("       not itself a resolved quantity; and the same ratio on the pooled\n")
    cat("       stratum is only about 1.5. Record the three SE columns, not a law.\n")
    cat("    ⚠️ Under C, S3's per-season table above carries nothing either. It\n")
    cat("       is printed for audit, not for reading a shape off.\n")
  }
  if (grepl("^D", verdictv)) {
    cat("  ⚠️ D IS A TIE, NOT THE REFUTATION OF ONE. A non-resolving comparison\n")
    cat("     shows two settings cannot be separated by this instrument. What it\n")
    cat("     CAN refute is a recorded magnitude, and what it cannot do is\n")
    cat("     establish that the end-stamp is harmless.\n")
  }
  invisible(b2p)
}

hr()
cat("DEFENSIVE FIXTURE COEFFICIENT, REFIT AT THE CUTOFF\n")
hr()
cat("Pre-registration: stats/defensive_fixture_pointintime_PREREGISTRATION.md\n")
cat("⚠️ `def_pit` is RECONSTRUCTED. The map from the opponent's fine\n")
cat("   strength_overall_* rating to difficulty is decoded and exact on 4560 of\n")
cat("   4560 archived fixture-sides, and the venue pairing is verified on a live\n")
cat("   payload that carries fixtures, 760/760. What is ASSUMED is that FPL\n")
cat("   re-published the difficulty when it revised a strength mid-season. The\n")
cat("   per-gameweek captures hold no fixtures payload, so no archived capture\n")
cat("   can show a difficulty moving, and this arm cannot claim to have recovered\n")
cat("   the true point-in-time difficulty.\n")
cat("⚠️ No replay cells are spent and the `x 38` season conversion does NOT\n")
cat("   apply. Thresholds below are t_crit(df) x SE, in coefficient units.\n")

hr()
cat("CORRECTIONS TO THE PRE-REGISTRATION (2026-08-16, POST-FIT)\n")
hr()
cat("A pre-registration edited after its fit is no longer one, so nothing below is
applied in place. These are review findings, each reproduced independently before
being written down. None of them changes the verdict; two of them weaken claims
the pre-registration made, and that is the only direction a post-fit correction
may run.

1. THE VERDICT LETTER IS POLE-SPECIFICATION DEPENDENT, and the pole chosen was
   the conservative one. Section 7 defines pole H on the STATISTIC (`Model P
   returns exactly 1`). Defining it on the TRUTH instead -- the true
   point-in-time slope is the shipped 1, with c_rev solved so Model E still
   reproduces 1.568809 -- gives c_rev = 3.139961, Model P -> 0.661538 and a
   Model D contrast of +2.139961. Separations become 0.773381 and 2.139961.
   H2 still fires against its realised threshold of 1.2990; H1 does NOT
   (1.9806 < 2.1400), and the decision rule would fall through to D. So:
   the pre-registered pole is the harder alternative on both statistics, C is
   the more cautious of the two labels, and both C and D forbid quoting -- but
   `C' should be read as `C or D', not as a resolved property of the instrument.

2. THERE IS A THIRD POLE AND SECTION 4 DENIES IT EXISTS. Section 4 says `a
   negative contrast ... no mechanism here predicts'. Pole P -- FPL's
   contemporaneous rating tracks the opponent's strength in THAT match better
   than a season-end aggregate does -- is exactly that mechanism. Solved
   (c_pit = 2.305783, Model E reproduces 1.568809), its Model D contrast is
   -2.305783. So H1's null is not `no artefact'; it is `no artefact AND def_end
   is the true regressor', with two zero-hindsight poles at 0 and -2.31. The
   CANARY IS UNAFFECTED: the binding pairwise separation on the level is still
   N-H at 0.4349 (N-P is 0.8709, P-H is 1.3058), so naming pole P would not
   have flattered the instrument.

3. THE PAIRED ESTIMATOR WAS REJECTED WITHOUT BEING COMPUTED. It is computed
   below. b2_end - b2_pit has EXACTLY H2's pole separation, because both poles
   pin Model E at 1.568809, and the two coefficients are strongly correlated
   across season clusters, so it is much better determined than either level.
   It also fires. `Rejected' was the wrong word; `computed, and it fires too'
   is the right one, and it is the estimator to use if this is ever revisited.

4. SECTION 6's `SHARPEST ROW' CANNOT FAIL. `def_end constant in 120 of 120
   club-venue-season cells' is an arithmetic consequence of the map check that
   runs earlier in the join and is required to pass. Monotonicity the
   construction forces is not evidence. The liveness that can fail is the
   438/1566 move rate and `def_pit' varying in 59 of 120.

5. SECTION 2 UNDERSTATES ITS OWN CONTROL. It calls `FPL re-published the
   difficulty when it revised a strength' unchecked. The map check substantially
   checks it: a column frozen at season start would have to be reproduced by the
   FIRST capture's strength, and that reproduces 2755 of 4560 against the
   end-of-season strength's 4560 of 4560. The frozen-column alternative is
   REFUTED, not merely unlikely. What survives is narrower -- whether FPL
   PUBLISHED the intermediate values -- and if it did not, the correct
   point-in-time column is the SEASON-START one, a third column this arm does
   not fit and S1's lag-1 arm does not reach.

6. THE DISCLOSURE SENTENCE IS FALSE AS WRITTEN. The preamble says `Not computed:
   ... any standard error, t, p or threshold whatsoever', and section 7 then
   quotes SE(b2_end) = 0.1375. That figure was not computed for this arm -- it
   is the predecessor arm's banked control SE -- but it was KNOWN when the
   canary was sized, which is why section 7 says H2 firing was the expected
   outcome. Read the disclosure as `no standard error was computed HERE'.

7. THE RANK WARNING IS SELECTIVE. It prints on Model D alone, which makes
   Models E and P look better founded. With G = 3 the cluster scores sum to
   zero, so the CR0 meat is rank 2 in ALL THREE models; CR2's adjustment lifts
   E and P to nominal full rank. `df = 2' already encodes the real constraint.

8. THE DENOMINATORS ARE INFLATED, in the join's report rather than here: 4560
   fixture-sides are 93 distinct constraints over 120 club-seasons, and the
   live venue check discriminates on 304 of 760 sides from 8 of 20 clubs, on
   ONE observation printed twice. Both are corrected in join_liveness.txt.
")

nat <- d[d$season %in% NATIVE, ]
if (nrow(nat) == 0) fail("no native-stratum rows in ", path)
b2p_native <- stratum(nat, "native stratum -- carries the verdict",
                      BANKED$native, 4.303, 2, TRUE)
stratum(d, "pooled stratum -- context only, no verdict", BANKED$pooled, 2.571, 5,
        FALSE)

# --- S1: a staler cutoff --------------------------------------------------
#
# Declared in the pre-registration, outside the Holm family, carrying no
# verdict. If the reading turns on the exact capture chosen it is not a reading.
if (length(args) >= 2) {
  hr(); cat("S1 -- A STALER CUTOFF (declared sensitivity, no verdict)\n"); hr()
  cat("`def_pit` rebuilt from capture GW{gw-1} instead of GW{gw}, clamped at GW1\n")
  cat("so the population does not change. Native stratum, season-clustered.\n")
  L <- read_sidecar(args[2])
  L <- L[L$season %in% NATIVE, ]
  L$w1 <- L$xgc90
  L$w2p <- L$xgc90 * (L$def_pit - 1)
  cat(sprintf("  rows the lagged reconstruction moves: %d / %d = %.4f\n",
              sum(L$def_moved), nrow(L), mean(L$def_moved)))
  S <- fit_block(L, clean_sheet ~ w1 + w2p, "Model P, lag 1 (point-in-time def)",
                 c(w1 = 1, w2p = POLE_N_LEVEL), 4.303, 2, c(-0.1, -1, -1))
  cat(sprintf("  b2_pit at lag 1 = %+.4f against %+.4f at lag 0. ⚠️ These are two\n",
              S$co[["w2p"]], b2p_native))
  cat("  fits of overlapping data, so the gap has no standard error of its own\n")
  cat("  and 'a gap between two point estimates is not a result until it is\n")
  cat("  divided by something'. It is reported to show the reading is not\n")
  cat("  knife-edge on the cutoff convention, and for nothing else.\n")
}
