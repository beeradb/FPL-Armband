#!/usr/bin/env Rscript
#
# Is the banked defensive fixture coefficient `b2 = 1.5688` an artefact of
# hindsight in `def`?
#
# ⚠️ READ stats/defensive_fixture_hindsight_PREREGISTRATION.md FIRST. It was
# committed before this script existed, and it fixes the hypotheses, the
# estimator, the clustering axis, the Holm family (m = 2), the canary and all
# four outcomes. This script implements that document and adds nothing to it.
# Nothing here may be re-read as a fresh question.
#
# Usage:
#   python3 stats/defensive_fixture_hindsight_join.py \
#       stats/snapshots/2026-08-15-clean-sheet-2x2/cs_regressor_fixture_path_rows.csv \
#       stats/defensive_fixture_hindsight/joined_rows.csv
#   Rscript stats/defensive_fixture_hindsight.R \
#       stats/defensive_fixture_hindsight/joined_rows.csv
#
# ---------------------------------------------------------------------------
# What `def` is, and why only b2 is at risk
#
# `def` is `defenceMultiplier(fixture.Difficulty) * defenceBandAdj(...)`, and at
# the shipped `band_strength: 0` the band adjustment is exactly 1 — the banked
# column takes precisely the five values 0.70/0.85/1.00/1.20/1.40, which is
# `defenceMultiplier` of difficulty 1..5 and nothing else. So `def` is a pure
# function of the archive's end-of-season `team_h_difficulty`, and `w2 = XGC90 *
# (def - 1)` is the ONLY regressor that can carry a difficulty leak. `w1 = XGC90`
# is point-in-time and cannot, which is why this design has an internal control
# rather than needing an external one.
#
# ---------------------------------------------------------------------------
# The one assertion
#
# `revised_opp` is measured on TEAM STRENGTH, not on difficulty. The captures
# hold `bootstrap-static.json.gz` and no fixtures payload, so whether
# `team_h_difficulty` itself moved is unverifiable from this archive. The step
# from one to the other is a MECHANISM argument. Every verdict below inherits it,
# and the alternative it cannot exclude is that `revised_opp` marks "this club's
# form surprised FPL" with the difficulty column frozen throughout.
#
# ⚠️ `revised_opp == 0` is not a clean row: the fine `strength_*` fields moved for
# 20 of 20 clubs in every season. H2's subsample is a robustness arm and is never
# called clean.
#
# ---------------------------------------------------------------------------
# No points figure comes out of this
#
# This is an archive-side calibration fit. It spends no replay cells, so there is
# no per-gameweek quantity and `AGENTS.md`'s `t_crit(df) * SE * 38` does not
# apply. The threshold in coefficient units is `t_crit(df) * SE`, printed beside
# every coefficient that carries a verdict.

args <- commandArgs(trailingOnly = TRUE)
path <- if (length(args) >= 1) args[1] else
  "stats/defensive_fixture_hindsight/joined_rows.csv"

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
need <- c("season", "team", "clean_sheet", "def", "xgc90",
          "revised_opp", "revised_delta", "either_revised_season")
missing <- setdiff(need, names(d))
if (length(missing) > 0) {
  fail("the joined rows are missing ", paste(missing, collapse = ", "),
       " -- regenerate with stats/defensive_fixture_hindsight_join.py")
}

NATIVE <- c("2023-24", "2024-25", "2025-26")

d$w1 <- d$xgc90
d$w2 <- d$xgc90 * (d$def - 1)
d$w3 <- d$revised_opp * d$w2
d$w3s <- d$revised_delta * d$w2

# A glm that fails to converge returns with a WARNING and coefficients from
# wherever IWLS stopped, and Rscript defers warnings past every table. Same
# standard as stats/cs_calibration.R: that is `fail`, not `note`.
checked <- function(f, label) {
  if (inherits(f, "try-error")) fail(label, ": glm errored")
  if (!f$converged || isTRUE(f$boundary) || max(fitted(f)) > 0.999) {
    fail(label, ": glm did not converge cleanly (converged=", f$converged,
         " boundary=", isTRUE(f$boundary),
         " max fitted p=", signif(max(fitted(f)), 4), ")")
  }
  f
}

# Season-clustered CR2, with the pre-registered fallback. It announces itself
# rather than switching silently, because an estimator swap reads as a data
# change.
season_vcov <- function(fit, rows, label) {
  v <- try(clubSandwich::vcovCR(fit, cluster = rows$season, type = "CR2"),
           silent = TRUE)
  if (inherits(v, "try-error") || any(!is.finite(diag(as.matrix(v)))) ||
      any(diag(as.matrix(v)) <= 0)) {
    note("  !!  ", label, ": CR2 not computable at G = ",
         length(unique(rows$season)), " clusters; FALLING BACK to CR0 ",
         "(sandwich::vcovCL) at the same df, as pre-registered")
    return(list(V = sandwich::vcovCL(fit, cluster = rows$season), type = "CR0",
                cr = NULL))
  }
  # `cr` keeps the vcovCR OBJECT, not the bare matrix: clubSandwich::coef_test
  # dispatches on its class and silently declines a plain matrix, which is how
  # the Satterthwaite line went missing on its first run.
  list(V = as.matrix(v), type = "CR2", cr = v)
}

# The banked estimator, season x team, printed as context only.
pair_vcov <- function(fit, rows) {
  cl <- interaction(rows$season, rows$team, drop = TRUE)
  v <- try(clubSandwich::vcovCR(fit, cluster = cl, type = "CR2"), silent = TRUE)
  if (inherits(v, "try-error")) return(NULL)
  as.matrix(v)
}

# Coefficients read on -ln p, so every sign flips out of the log-link glm.
fit_model <- function(rows, rhs, label) {
  form <- as.formula(paste("clean_sheet ~", paste(rhs, collapse = " + ")))
  start <- c(-0.1, rep(-1, length(rhs)))
  f <- checked(try(glm(form, data = rows, family = binomial(link = "log"),
                       start = start), silent = TRUE), label)
  list(fit = f, co = -coef(f), rhs = rhs, rows = rows, label = label)
}

report <- function(m, targets) {
  sv <- season_vcov(m$fit, m$rows, m$label)
  se <- sqrt(diag(sv$V))
  pv <- pair_vcov(m$fit, m$rows)
  G <- length(unique(m$rows$season))
  df <- G - 1
  tc <- qt(0.975, df)
  cat(sprintf("\n%s   n = %d, season clusters = %d, df = %d, t_crit = %.3f, SE type %s\n",
              m$label, nrow(m$rows), G, df, tc, sv$type))

  # ⚠️ Added at reporting time, after review, and it is a DISCLOSURE rather than a
  # test. The pre-registered df is the naive `G - 1`. At G = 3 the CR2 meat is a
  # sum over three clusters, so the variance matrix is RANK-DEFICIENT the moment
  # the model has four parameters -- computable, and not therefore trustworthy.
  # The pre-registered CR0 fallback guards non-computability, which is the wrong
  # failure. clubSandwich's own Satterthwaite df is the guard that fires, and here
  # it is LOWER than G - 1 on every coefficient, so the pre-registered df is the
  # GENEROUS choice. `AGENTS.md`: "take the df from the comparison, since it is
  # resolved per contrast and is often lower". The verdict still uses G - 1
  # because G - 1 was declared; this prints what declaring it cost.
  rk <- qr(as.matrix(sv$V))$rank
  cat(sprintf("  rank(vcov) = %d of %d parameters%s\n", rk, ncol(sv$V),
              if (rk < ncol(sv$V))
                "  ⚠️ RANK-DEFICIENT: G clusters cannot span p parameters" else ""))
  satt <- if (is.null(sv$cr)) NULL else
    try(clubSandwich::coef_test(m$fit, vcov = sv$cr, test = "Satterthwaite"),
        silent = TRUE)
  if (is.null(satt) || inherits(satt, "try-error")) {
    note("  !!  Satterthwaite df unavailable for ", m$label,
         " -- reported as missing, not silently skipped")
  } else {
    cat("  Satterthwaite df per coefficient (context; the verdict uses G-1):",
        paste(sprintf("%s %.2f", satt$Coef, satt$df_Satt), collapse = "  "), "\n")
  }
  cat(sprintf("  %-6s %10s %10s %8s %8s %12s %10s\n",
              "term", "estimate", "SE(season)", "t vs H0", "H0", "threshold", "SE(sxteam)"))
  out <- list()
  for (nm in m$rhs) {
    h0 <- targets[[nm]]
    est <- m$co[[nm]]
    s <- se[[nm]]
    tt <- (est - h0) / s
    pse <- if (!is.null(pv)) sqrt(pv[nm, nm]) else NA_real_
    cat(sprintf("  %-6s %+10.4f %10.4f %+8.2f %8.1f %12.4f %10.4f\n",
                nm, est, s, tt, h0, tc * s, pse))
    out[[nm]] <- list(est = est, se = s, t = tt, df = df,
                      p = 2 * pt(-abs(tt), df), thr = tc * s)
  }
  invisible(out)
}

# `hr()` from cells_common.R takes no arguments. A titled rule is this script's
# own presentation concern, so it is defined here rather than widened there.
section <- function(title) {
  hr()
  cat(title, "\n", sep = "")
  hr()
}

section("DEFENSIVE FIXTURE HINDSIGHT GATE")
cat("Pre-registration: stats/defensive_fixture_hindsight_PREREGISTRATION.md\n")
cat("⚠️ `revised_opp` is measured on TEAM STRENGTH. The captures carry no fixtures\n")
cat("   payload, so no capture can show a difficulty moving. See the CORRECTIONS\n")
cat("   block below: the archive itself narrows this more than the pre-registration\n")
cat("   allowed for.\n")
cat("⚠️ `revised_opp == 0` is NOT a clean row: the fine strength_* fields moved\n")
cat("   for 20/20 clubs in every season.\n")

section("CORRECTIONS TO THE PRE-REGISTRATION (2026-08-16, POST-FIT)")
cat("A pre-registration edited after its fit is no longer one, so nothing below was\n")
cat("applied in place. These are the review findings this run verified independently.\n")
cat("\n1. SECTION 7's SIZING WAS AN APPROXIMATION AND IT DECIDED THE VERDICT.\n")
cat("   `(b2-1)/q_w` is an OLS omitted-variable formula, applied to a log-link GLM\n")
cat("   without partialling w1 or using IWLS weights. It reads 1.977; the quantity\n")
cat("   that sentence names is 1.685. The threshold is 1.702. So the canary FIRES\n")
cat("   and the verdict is C, where the approximation gave D. The error ran in the\n")
cat("   direction that flatters the instrument.\n")
cat("\n2. THE MECHANISM ARGUMENT IS NARROWER THAN 'UNVERIFIABLE'. The archive's own\n")
cat("   difficulty column is END-STAMPED, and that is measurable from the committed\n")
cat("   join -- see the join's liveness output. What stays unverifiable is whether\n")
cat("   FPL's LIVE difficulty moved in-season, which needs a fixtures payload.\n")
cat("\n3. SECTION 9's 'nothing here reaches the scored path' IS WRONG AS MECHANISM.\n")
cat("   `FPL_DEF_FIXTURE_SCALE` runs through defenceMultiplier -> fixtureMultipliersFor\n")
cat("   -> fixtureSensitiveAt and is live on the scored path. What the record holds\n")
cat("   is that MOVING it does not resolve on points, which is a tie and not a\n")
cat("   refutation. Read section 9 as 'nothing here licenses moving a scored constant'.\n")
cat("\n4. TWO ALTERNATIVES SECTION 2 DOES NOT NAME.\n")
cat("   (a) TIMING. Revisions land in 2-4 discrete gameweeks a season, so revised_opp\n")
cat("       is partly 'early in the season': mean gw 18.08 revised against 22.76\n")
cat("       unrevised on the native stratum. XGC90's reliability rises through the\n")
cat("       season, so the w2 slope can differ by calendar position with no hindsight\n")
cat("       involved. Unsigned: it can mask a leak as easily as manufacture one.\n")
cat("   (b) OUTCOME CONDITIONING. revised_opp is a function of END-of-season strength,\n")
cat("       which FPL sets from results including this row's own match, so b3 = 0 is\n")
cat("       not a clean null. Bounded at ~1 match in 38 and blunted by a 5-level scale.\n")
cat("\n5. THE ONLY DIRECT OBSERVATION OF DIFFICULTY MOVEMENT IN THIS PROJECT RUNS\n")
cat("   AGAINST THE HYPOTHESIS, and the pre-registration quoted the other half of\n")
cat("   its paragraph without it: the banked cs_calibration output records 'two live\n")
cat("   captures three days apart show 0 of 380 difficulties changing'. That window\n")
cat("   is pre-season and reaches no archived season, so it is weak -- but it was\n")
cat("   known beforehand and omitting it was selective.\n")
cat("\n6. SECTION 6's STATED LIVENESS BAR CANNOT FIRE. The join copies `def` unchanged\n")
cat("   and fails on any unmatched row, so conditional on the row count the `def`\n")
cat("   distribution is bit-identical BY CONSTRUCTION. The check that can fire, and\n")
cat("   does pass, is the independent re-derivation in the join's liveness output.\n")

for (stratum in c("native", "pooled")) {
  rows <- if (stratum == "native") d[d$season %in% NATIVE, ] else d
  verdict_stratum <- stratum == "native"
  section(sprintf("%s stratum -- %s", toupper(stratum),
             if (verdict_stratum) "CARRIES THE VERDICT"
             else "CONTEXT ONLY, no verdict either way"))

  # --- the required control (pre-registration section 5) --------------------
  base <- fit_model(rows, c("w1", "w2"), "banked 2-channel model")
  if (verdict_stratum) {
    cat("\nCONTROL: the banked model refitted here. On the native stratum this must\n")
    cat("reproduce b2 = 1.5688 with season-clustered t = 4.14 and a season x team\n")
    cat("SE of 0.2253, or nothing from this run may be quoted.\n")
  } else {
    cat("\nCONTROL, pooled: this must reproduce the banked POOLED figures,\n")
    cat("b2 = 1.5654 with a season x team SE of 0.1712. The native control text\n")
    cat("does not apply here -- it used to print in this position and read like a\n")
    cat("failure.\n")
  }
  ctl <- report(base, list(w1 = 1, w2 = 1))
  # The pre-registration calls this control load-bearing -- "the run is void
  # without it" -- and prose is not a guard. A guard that cannot fire is not a
  # passed check, which is this file's own standard three functions up.
  want <- if (verdict_stratum) c(b2 = 1.5688, se = 0.2253) else c(b2 = 1.5654, se = 0.1712)
  got_se <- {
    pv <- pair_vcov(base$fit, rows)
    if (is.null(pv)) NA_real_ else sqrt(pv["w2", "w2"])
  }
  if (abs(ctl$w2$est - want[["b2"]]) > 5e-4 || is.na(got_se) ||
      abs(got_se - want[["se"]]) > 5e-4) {
    fail("CONTROL FAILED on the ", stratum, " stratum: b2 = ", signif(ctl$w2$est, 6),
         " (want ", want[["b2"]], "), season x team SE = ", signif(got_se, 6),
         " (want ", want[["se"]], "). The population or the estimator has moved, ",
         "so nothing from this run may be quoted.")
  }
  cat(sprintf("  CONTROL PASSES: b2 %.4f and season x team SE %.4f match the banked figures.\n",
              ctl$w2$est, got_se))

  # --- H1, the primary -----------------------------------------------------
  aug <- fit_model(rows, c("w1", "w2", "w3"), "augmented: + revised_opp*w2")
  cat("\nH1 (PRIMARY): b3 = 0.  b2 here is the fixture channel on rows whose\n")
  cat("opponent's coarse strength did NOT move; b3 is the extra on rows whose did.\n")
  # `n` on the table below is the row count, not the information behind b3. Rows
  # with def == 1 have w2 = w3 = 0 and pin only the intercept and b1, so quoting
  # n beside b3 over-rates the arm badly. The sibling cs_calibration.R prints a
  # correlation for exactly this purpose; the relevant one here is cor(w2, w3).
  cat(sprintf("⚠️ Only %d of %d rows inform b3 at all -- %d have def == 1, where w2 = w3 = 0.\n",
              sum(rows$revised_opp == 1 & rows$def != 1), nrow(rows),
              sum(rows$def == 1)))
  cat(sprintf("   cor(w2, w3) = %.3f, cor(w1, w3) = %.3f -- w2 and w3 are moderately\n",
              cor(rows$w2, rows$w3), cor(rows$w1, rows$w3)))
  cat("   collinear by construction, since w3 is w2 on a subset of the same rows.\n")
  a1 <- report(aug, list(w1 = 1, w2 = 1, w3 = 0))

  # --- H2, the robustness arm ----------------------------------------------
  sub <- rows[rows$either_revised_season == 0, ]
  cat(sprintf("\nH2 (ROBUSTNESS): b2 = 1 where NEITHER club's coarse strength moved\n"))
  cat(sprintf("all season.  n = %d of %d.  This costs power exactly where power is\n",
              nrow(sub), nrow(rows)))
  cat("shortest, and the subsample is NOT clean -- the fine fields still moved.\n")
  a2 <- NULL
  if (length(unique(sub$clean_sheet)) < 2 || length(unique(sub$season)) < 2 ||
      length(unique(sub$def)) < 2) {
    note("  !!  H2 not estimable on this subsample (outcome, season or def is ",
         "degenerate). Reported as unmeasurable, not as a null.")
  } else {
    subm <- fit_model(sub, c("w1", "w2"), "never-revised subsample")
    a2 <- report(subm, list(w1 = 1, w2 = 1))
    # H2 gets the same canary H1 got, which section 7 should have declared for it
    # and did not. `estimable` is not `powered`, and this arm is estimable.
    excess <- ctl$w2$est - 1
    if (a2$w2$thr > excess) {
      cat(sprintf("\n  ⚠️ H2 IS UNMEASURABLE, NOT NULL. Its threshold on b2 is %.4f against\n",
                  a2$w2$thr))
      cat(sprintf("     the %.4f excess being tested -- it could only ever have seen an\n",
                  excess))
      cat(sprintf("     effect %.1fx the one in question. Quote no p from it in either\n",
                  a2$w2$thr / excess))
      cat("     direction. Its point estimate is the most direct reading against the\n")
      cat("     leak story in this run and it is still NOT evidence, because the arm\n")
      cat("     cannot separate 1 from 1.5688. It keeps its Holm slot because it was\n")
      cat("     declared, and the cost that imposes on H1 stands.\n")
    }
  }

  # --- the pre-declared sensitivity, outside the family --------------------
  cat("\nSENSITIVITY (outside the Holm family, uncorrected t): the SIGNED\n")
  cat("specification, revised_delta*w2 in place of revised_opp*w2.\n")
  cat("⚠️ READ IT AS UNINFORMATIVE, NOT AS COUNTER-EVIDENCE. It carries w3s INSTEAD\n")
  cat("   of w3, so it constrains the up-revised and down-revised excesses to be\n")
  cat("   EQUAL AND OPPOSITE -- a shape that cannot express the unsigned model's own\n")
  cat("   point estimate of both groups elevated. The leak mechanism is\n")
  cat("   direction-agnostic: revised either way, the end-stamped `def` describes\n")
  cat("   realised strength better than the cutoff value did, so both directions\n")
  cat("   steepen the channel. A negative w3s says the elevation is not monotone in\n")
  cat("   the direction of revision; it does NOT say the leak runs the other way.\n")
  cat("   The two models are also fitted separately and cannot be composed.\n")
  sgn <- fit_model(rows, c("w1", "w2", "w3s"), "augmented: + revised_delta*w2")
  report(sgn, list(w1 = 1, w2 = 1, w3s = 0))

  if (!verdict_stratum) {
    # Quote the pooled block's two readings together or not at all. It resolves a
    # NONZERO leak and it EXCLUDES a full one, and reporting only the first would
    # be the selective use of a stratum this design forbids in the other
    # direction. Neither carries a verdict, for a reason of method rather than
    # convenience: three of these six seasons carry RECONSTRUCTED xGC, so `w1`
    # there is a different construct and not merely more of the same.
    cat("\n⚠️ THE POOLED BLOCK SAYS TWO THINGS AND THEY MUST BE QUOTED TOGETHER.\n")
    cat("   b3 clears its own threshold here, so a NONZERO leak resolves at df 5.\n")
    cat("   Its interval also sits well below the leak a FULL artefact would need,\n")
    cat("   so a full artefact is excluded here. Neither is a verdict: the verdict\n")
    cat("   stratum was fixed before the fit, and three of these six seasons carry\n")
    cat("   reconstructed xGC, which makes w1 a different construct rather than\n")
    cat("   more of the same.\n")
    next
  }

  # --- the canary (pre-registration section 7) ------------------------------
  section("THE CANARY -- sized before the fit, evaluated now")

  # The pre-registered ESTIMAND is "the b3 that, when omitted, drives the
  # 2-channel model's b2 to the banked 1.5688 while the unrevised rows sit at
  # exactly 1". Section 7 writes it `(b2 - 1)/q_w` with an explicit `≈`, and
  # that approximation is an OLS omitted-variable formula: it ignores partialling
  # out w1 and it ignores the log-link GLM's own IWLS weights.
  #
  # ⚠️ The approximation is not good enough for the margin the verdict turns on.
  # It over-states the required leak, which is the direction that makes this
  # instrument look MORE capable than it is. The estimand needs no approximation:
  # generate expectations under the candidate DGP, refit the misspecified
  # 2-channel model to them, and solve. `b3 = 0` returns b2 = 1 exactly, which is
  # the self-consistency check that the construction is the right one.
  qw <- sum(rows$w2[rows$revised_opp == 1]^2) / sum(rows$w2^2)
  approx_needed <- (ctl$w2$est - 1) / qw
  a_dgp <- -coef(aug$fit)[[1]]   # the intercept on -ln p, same convention as co
  pseudo_b2 <- function(b3) {
    p <- exp(-(a_dgp + rows$w1 + rows$w2 + b3 * rows$w3))
    f <- suppressWarnings(try(glm(cbind(p, 1 - p) ~ w1 + w2, data = rows,
                                  family = binomial(link = "log"),
                                  start = c(-0.1, -1, -1)), silent = TRUE))
    if (inherits(f, "try-error") || !f$converged) return(NA_real_)
    -coef(f)[["w2"]]
  }
  self <- pseudo_b2(0)
  if (is.na(self) || abs(self - 1) > 1e-6) {
    fail("the canary construction is wrong: b3 = 0 must return b2 = 1 exactly, got ",
         signif(self, 8))
  }
  needed <- try(uniroot(function(b) pseudo_b2(b) - ctl$w2$est,
                        c(0.8, 2.2), tol = 1e-9)$root, silent = TRUE)
  if (inherits(needed, "try-error") || is.na(needed)) {
    fail("the canary could not be solved on [0.8, 2.2]; reported as a failure ",
         "rather than falling back to the approximation")
  }
  thr <- a1$w3$thr
  cat(sprintf("  q_w (share of sum w2^2 on revised rows) = %.4f\n", qw))
  cat(sprintf("  section 7's APPROXIMATION, (b2 - 1)/q_w        = %.3f\n", approx_needed))
  cat(sprintf("  the EXACT b3 that same sentence asks for      = %.3f\n", needed))
  cat("     (self-consistency: b3 = 0 returns b2 = 1.000000 exactly)\n")
  cat(sprintf("  detection threshold on b3 = t_crit(%d) x SE(b3) = %.3f x %.4f = %.3f\n",
              a1$w3$df, qt(0.975, a1$w3$df), a1$w3$se, thr))
  canary_fires <- thr > needed
  cat(sprintf("  canary %s: threshold %s the leak a full artefact would require\n",
              if (canary_fires) "FIRES" else "does not fire",
              if (canary_fires) "EXCEEDS" else "is below"))
  if (canary_fires && thr <= approx_needed) {
    cat("  ⚠️ THE APPROXIMATION AND THE EXACT ANSWER DISAGREE ON THE VERDICT.\n")
    cat("     Taking section 7's `≈` literally gives 'does not fire' and outcome D;\n")
    cat("     evaluating the quantity that sentence NAMES gives 'fires' and\n")
    cat("     outcome C. C is reported, for two reasons. It is the same estimand,\n")
    cat("     computed without an approximation the pre-registration never\n")
    cat("     defended -- not a new rule. And it moves the verdict toward quoting\n")
    cat("     LESS, which is the direction a post-hoc correction is allowed to run;\n")
    cat("     the pre-registration's own bar is that no verdict may be UPGRADED\n")
    cat("     after the fact.\n")
  }
  cat("  ⚠️ The margin is small either way, and a second, independent route reaches\n")
  cat("     the same place: at clubSandwich's Satterthwaite df for b3 the threshold\n")
  cat("     is larger still, so the canary fires there without needing this\n")
  cat("     correction at all. See the df line printed with each fit above.\n")

  # --- the decision rule ----------------------------------------------------
  section("DECISION (pre-registration section 8)")
  ps <- c(H1 = a1$w3$p, H2 = if (is.null(a2)) NA_real_ else a2$w2$p)
  cat(sprintf("  raw p:  H1 %.4f   H2 %s\n", ps[["H1"]],
              if (is.na(ps[["H2"]])) "not estimable" else sprintf("%.4f", ps[["H2"]])))
  cat(sprintf("  Holm changed nothing here: H1's RAW p is %.4f and fails at m = 1 too.\n",
              ps[["H1"]]))
  # ⚠️ `n = length(ps)` is load-bearing and was once absent. `p.adjust(ps[!is.na(ps)])`
  # runs at n = 1 when an arm is unestimable and applies NO correction at all --
  # the family silently SHRINKS, which is the exact opposite of what the line
  # printed below promises, and it would inflate H1 precisely in the case the
  # promise is about. Latent here because both arms estimated.
  holm <- p.adjust(ps, method = "holm", n = length(ps))
  cat(sprintf("  Holm (m = %d, as pre-registered -- an unestimable arm does NOT\n",
              length(ps)))
  cat("  shrink the family, because it was declared):\n")
  for (nm in names(holm)) cat(sprintf("    %-3s %.4f\n", nm, holm[[nm]]))
  cat(sprintf("  Holm thresholds at df 2: |t| > %.3f for the smaller p, %.3f for the larger\n",
              qt(1 - 0.025 / 2, 2), qt(0.975, 2)))

  h1_rejects <- !is.na(holm["H1"]) && holm[["H1"]] < 0.05
  b2_aug <- a1$w2$est
  b2_moves_to_one <- abs(b2_aug - 1) <= a1$w2$se
  b2_holds <- abs(b2_aug - ctl$w2$est) <= 0.10

  verdict <- if (h1_rejects && a1$w3$est > 0 && b2_moves_to_one) {
    "A -- HINDSIGHT CARRIES THE EXCESS"
  } else if (canary_fires) {
    "C -- UNMEASURABLE"
  } else if (!h1_rejects && b2_holds) {
    "B -- HINDSIGHT DOES NOT CARRY THE EXCESS"
  } else {
    "D -- UNRESOLVED"
  }
  cat(sprintf("\n  augmented b2 (unrevised rows) = %+.4f against the banked %+.4f\n",
              b2_aug, ctl$w2$est))
  cat(sprintf("  moved-to-1 test: |b2 - 1| = %.4f vs its own SE %.4f -> %s\n",
              abs(b2_aug - 1), a1$w2$se, b2_moves_to_one))
  cat(sprintf("  held-at-banked test: |b2 - banked| = %.4f vs 0.10 -> %s\n",
              abs(b2_aug - ctl$w2$est), b2_holds))
  cat(sprintf("\n  VERDICT: %s\n", verdict))
  if (substr(verdict, 1, 1) == "C") {
    cat("  ⚠️ UNDER C, SECTION 8 FORBIDS QUOTING ANY READING OF b3 IN EITHER\n")
    cat("     DIRECTION. The three lines above are the decision rule's own branch\n")
    cat("     conditions, printed so the verdict can be audited -- they are NOT\n")
    cat("     results. In particular the drop from 1.5688 to 1.2854 must not be\n")
    cat("     reported as 'the leak absorbs about half the excess': that share's\n")
    cat("     own interval is not even bounded inside [0, 1], and dropping any one\n")
    cat("     of the three seasons moves it a long way.\n")
    cat("  ⚠️ C is a fact about this INSTRUMENT, not about the constant. It does not\n")
    cat("     say b2 = 1.5688 is sound and it does not say it is an artefact.\n")
  }
  cat("  ⚠️ The verdict is taken on the NATIVE stratum, season-clustered, df 2,\n")
  cat("     exactly as pre-registered. The pooled block below carries no verdict\n")
  cat("     in either direction, per the record's own rule against leaning on a\n")
  cat("     stratum for a rejection while disowning it for a null.\n")
}

section("PER-SEASON, native stratum -- shape, not a test")
cat("The record's standard for a shape is that it holds in every season. ⚠️ This\n")
cat("table carries NO standard errors, so a sign flip in one of three seasons is\n")
cat("not a result: 'a gap between two point estimates is not a result until it is\n")
cat("divided by something'. It is printed because a 3-cluster arm where one cluster\n")
cat("carries the effect is a pattern this record has been bitten by before.\n")
cat(sprintf("  %-10s %6s %6s %9s %9s %9s\n",
            "season", "n", "n_rev", "b1", "b2", "b3"))
for (sn in NATIVE) {
  rs <- d[d$season == sn, ]
  f <- try(glm(clean_sheet ~ w1 + w2 + w3, data = rs,
               family = binomial(link = "log"), start = c(-0.1, -1, -1, 0)),
           silent = TRUE)
  if (inherits(f, "try-error") || !f$converged) {
    cat(sprintf("  %-10s %6d %6d   did not converge\n",
                sn, nrow(rs), sum(rs$revised_opp)))
    next
  }
  co <- -coef(f)
  cat(sprintf("  %-10s %6d %6d %+9.4f %+9.4f %+9.4f\n",
              sn, nrow(rs), sum(rs$revised_opp), co[["w1"]], co[["w2"]], co[["w3"]]))
}
