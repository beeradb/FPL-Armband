#!/usr/bin/env Rscript

# Does widening the replay grid from four played seasons to six buy what theory
# says it should?
#
# Usage:
#   Rscript grid_width.R --pos6=vice6.csv --pos4=vice4.csv \
#                        --neg6=noise6.csv --neg4=noise4.csv
#
# Reports against the pre-registration in internal/backtest/gridwidth_test.go,
# which is the authority on what P1..P5 mean. Nothing here invents a condition.
#
# Inference is CR2 — cluster-robust at the season level, bias-reduced small-sample
# correction, Satterthwaite degrees of freedom — the same three lines of
# clubSandwich that sweep_inference.R uses, so the thresholds here are on the same
# estimator the rest of the record is judged by.

args <- commandArgs(trailingOnly = TRUE)
opt <- list(pos6 = "", pos4 = "", neg6 = "", neg4 = "")
for (a in args) {
  for (k in names(opt)) {
    if (grepl(paste0("^--", k, "="), a)) opt[[k]] <- sub(paste0("^--", k, "="), "", a)
  }
}
if (any(vapply(opt, function(x) !nzchar(x), logical(1)))) {
  stop("need --pos6= --pos4= --neg6= --neg4=")
}

# `note`, `hr`, `fail`, `as_flag`, `read_cells`, `diffs_for`, `degenerate` and
# `se_cr` are all in cells_common.R. This script carried its own copy of every one
# of them, and one of those copies held the defect this migration exists to fix —
# see the note on `read_cells` there.
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

if (!requireNamespace("clubSandwich", quietly = TRUE)) {
  fail("clubSandwich is not installed and there is no honest fallback: the naive ",
       "SE treats all cells as independent, which is the assumption this pass ",
       "exists to interrogate.")
}

SEASON_GWS <- 38
ALPHA <- 0.05

# The seasons each grid PLAYS. Literals, so a file missing a season is a loud
# failure rather than a quietly narrower grid.
GRID_4 <- c("2022-23", "2023-24", "2024-25", "2025-26")
GRID_6 <- c("2020-21", "2021-22", GRID_4)
# The two the extension adds. They carry no FPL expected-goals column at all, so
# xgRepairs backfills them from Understat on an offset BORROWED from seasons that
# have one. 2022-23's repair is fitted in-season and it is inside the shipped four
# anyway, so it sits on the native side of this split.
ADDED_2 <- c("2020-21", "2021-22")


# `diffs_for`, `degenerate` and `se_cr2` are in cells_common.R.
#
# ⚠️ Two behaviour changes come with that, and both are the point rather than
# collateral. This script's own `diffs_for` keyed cells on `season@start_gw` and
# never blocked on `(run_id, sweep)`, so a file holding several runs — which the
# cells file is opened for append to allow — cross-paired every arm against every
# block's baseline: n tripled on the banked three-block MINHL files, each arm's mean
# moved about 0.14 pts/gw and the naive SE shrank by root three, silently. The
# shared key is `(run_id, sweep, season, start_gw)`, so that join no longer happens.
#
# And this copy dropped an under-powered arm with a bare `next`, where the shared
# one names it. So this script will now REPORT arms it used to swallow. That is the
# defect the comment above cells_common.R's `diffs_for` says it exists to prevent,
# and it was live here.

# The effect that would land exactly at p = alpha. The bar, not the finding.
sig_season <- function(se, df) {
  if (is.na(se) || is.na(df)) return(NA_real_)
  qt(1 - ALPHA / 2, df) * se * SEASON_GWS
}

summarise <- function(cells, metric, seasons, label) {
  # Pre-filter, because the shared `diffs_for` does not. This script's own copy
  # filtered `!infeasible` inside the function; cells_common.R leaves it to the
  # caller, as sweep_inference.R and variance_components.R already do. Dropping
  # this line would silently admit infeasible rows, and the bank has none, so no
  # before/after run could catch it.
  d <- cells[cells$season %in% seasons & !cells$infeasible, ]
  # min_cells 2 and the suffix are passed explicitly: neither has a default, so a
  # caller cannot inherit another estimator's floor or the wrong scale.
  # NOT quiet: the comment above says this script now reports arms it used to
  # swallow, and passing quiet = TRUE would have made that false. grid_width.R's
  # whole job is a pre-registered pass/fail on arm counts, so an arm silently
  # missing turns a condition into "CANNOT TELL" with no named cause.
  arms <- diffs_for(d, metric, "_per_gw", min_cells = 2)
  if (length(arms) == 0) return(NULL)
  rows <- lapply(names(arms), function(v) {
    a <- arms[[v]]
    r <- se_cr2(a)
    data.frame(grid = label, metric = metric, variant = v, n = nrow(a),
               seasons = length(unique(a$season)), mean = mean(a$diff),
               se = r$se, df = r$df, t = r$t, p = r$p,
               sig_season = sig_season(r$se, r$df), stringsAsFactors = FALSE)
  })
  tab <- do.call(rbind, rows)
  # Holm across the family of arms within this grid and metric, matching
  # sweep_inference.R. A degenerate arm has no p and is left out of the family
  # rather than counted as a test that passed.
  tab$p_holm <- NA_real_
  ok <- !is.na(tab$p)
  if (any(ok)) tab$p_holm[ok] <- p.adjust(tab$p[ok], method = "holm")
  rownames(tab) <- NULL
  tab
}

# as.numeric first: a degenerate arm — one with exactly zero spread, which is what
# the determinism control must be — carries a logical NA rather than a numeric one,
# and formatC refuses it. That arm is the single most important row in the table,
# so it must print rather than abort the report.
num <- function(x, dp = 3, w = 8) {
  formatC(as.numeric(x), format = "f", digits = dp, width = w, flag = " ")
}
fmt_tab <- function(tt) {
  note(sprintf("%-30s %4s %4s %9s %8s %6s %7s %8s %10s",
               "arm", "n", "seas", "mean/gw", "SE CR2", "df", "t", "p Holm", "sig/season"))
  for (i in seq_len(nrow(tt))) {
    r <- tt[i, ]
    note(sprintf("%-30s %4d %4d %9s %8s %6s %7s %8s %10s",
                 substr(r$variant, 1, 30), r$n, r$seasons, num(r$mean, 4, 9),
                 num(r$se, 4, 8), num(r$df, 1, 6), num(r$t, 2, 7),
                 num(r$p_holm, 3, 8), num(r$sig_season, 1, 10)))
  }
}

pos6 <- read_cells(opt$pos6); pos4 <- read_cells(opt$pos4)
neg6 <- read_cells(opt$neg6); neg4 <- read_cells(opt$neg4)

for (nm in c("pos6", "neg6")) {
  d <- get(nm)
  miss <- setdiff(GRID_6, unique(d$season))
  if (length(miss) > 0) fail(nm, " is missing seasons: ", paste(miss, collapse = ", "))
}

POS <- unique(pos6$variant[!pos6$is_baseline])
note("")
hr()
note("GRID WIDTH: four seasons against six")
hr()
note("positive control : ", POS, "   (baseline: ",
     unique(pos6$variant[pos6$is_baseline]), ")")
note("negative controls: ", paste(setdiff(unique(neg6$variant),
     unique(neg6$variant[neg6$is_baseline])), collapse = ", "))
note("start gameweeks  : ", paste(sort(unique(pos6$start_gw)), collapse = ", "))

tabs <- list()
for (m in c("hold", "policy")) {
  tabs[[paste("pos4", m)]] <- summarise(pos4, m, GRID_4, "4 seasons (shipped)")
  tabs[[paste("pos6", m)]] <- summarise(pos6, m, GRID_6, "6 seasons (extended)")
  tabs[[paste("neg4", m)]] <- summarise(neg4, m, GRID_4, "4 seasons (shipped)")
  tabs[[paste("neg6", m)]] <- summarise(neg6, m, GRID_6, "6 seasons (extended)")
}

for (nm in names(tabs)) {
  tt <- tabs[[nm]]
  if (is.null(tt)) next
  note(""); hr()
  note(toupper(sub(" .*", "", nm)), " -- ", tt$grid[1], " -- ",
       toupper(tt$metric[1]))
  hr()
  fmt_tab(tt)
}

get1 <- function(key, variant) {
  tt <- tabs[[key]]
  if (is.null(tt)) return(NULL)
  r <- tt[tt$variant == variant, ]
  if (nrow(r) != 1) return(NULL)
  r
}

note(""); hr()
note("THE PRE-REGISTERED CONDITIONS")
hr()
verdict <- list()

# --- P1: the positive control reproduces ------------------------------------
p1 <- NA
for (m in c("hold", "policy")) {
  a <- get1(paste("pos4", m), POS); b <- get1(paste("pos6", m), POS)
  if (is.null(a) || is.null(b)) next
  lo <- a$mean - qt(1 - ALPHA/2, a$df) * a$se
  hi <- a$mean + qt(1 - ALPHA/2, a$df) * a$se
  inside <- b$mean >= lo && b$mean <= hi
  same <- sign(a$mean) == sign(b$mean)
  if (m == "hold") p1 <- inside && same
  note("")
  note("  P1 on ", toupper(m), ":")
  note("    4 seasons: ", num(a$mean, 4, 0), "/gw = ", num(a$mean * SEASON_GWS, 1, 0),
       " a season   95% CI [", num(lo, 4, 0), ", ", num(hi, 4, 0), "]")
  note("    6 seasons: ", num(b$mean, 4, 0), "/gw = ", num(b$mean * SEASON_GWS, 1, 0),
       " a season   -> ", if (inside) "inside" else "OUTSIDE",
       " the 4-season interval, sign ", if (same) "kept" else "CHANGED")
}
verdict$P1 <- p1
note("")
note("  P1: ", if (isTRUE(p1)) "PASS" else "FAIL",
     " -- judged on HOLD, the metric a scoring correction belongs on.")

# --- P2: the negative controls stay null ------------------------------------
n6 <- tabs[["neg6 hold"]]
ctrl_rows <- rbind(tabs[["neg6 hold"]], tabs[["neg4 hold"]])
ctrl_rows <- ctrl_rows[grepl("identical", ctrl_rows$variant), ]
nulls <- n6[!grepl("identical", n6$variant), ]
resolved <- nulls$variant[!is.na(nulls$p_holm) & nulls$p_holm < ALPHA]
ctrl_zero <- nrow(ctrl_rows) > 0 && all(abs(ctrl_rows$mean) < 1e-12)
p2 <- length(resolved) == 0 && ctrl_zero
verdict$P2 <- p2
note("")
note("  P2: ", if (isTRUE(p2)) "PASS" else "FAIL")
if (length(resolved) == 0) {
  note("    no near-null arm resolves on six seasons; smallest Holm p = ",
       num(suppressWarnings(min(nulls$p_holm, na.rm = TRUE)), 3, 0))
} else {
  note("    RESOLVED on six seasons: ", paste(resolved, collapse = ", "),
       " -- the wider grid is manufacturing effects.")
}
note("    determinism control ",
     if (ctrl_zero) "is exactly zero in every cell."
     else "IS NOT ZERO -- the replay is nondeterministic.")

# --- P3: the threshold improves, by less than theory says -------------------
theo <- (qt(1-ALPHA/2, 5)/sqrt(6)) / (qt(1-ALPHA/2, 3)/sqrt(4))
note("")
note("  P3: realised detection threshold, points a season")
note(sprintf("    %-30s %11s %11s %8s", "arm", "4 seasons", "6 seasons", "ratio"))
ratios <- c()
rows <- list(c("pos", POS))
for (v in setdiff(unique(neg6$variant[!neg6$is_baseline]), NA)) rows[[length(rows)+1]] <- c("neg", v)
for (rw in rows) {
  a <- get1(paste0(rw[1], "4 hold"), rw[2]); b <- get1(paste0(rw[1], "6 hold"), rw[2])
  if (is.null(a) || is.null(b) || is.na(a$sig_season) || is.na(b$sig_season)) next
  r <- b$sig_season / a$sig_season
  ratios <- c(ratios, r)
  note(sprintf("    %-30s %11s %11s %8s", substr(rw[2], 1, 30),
               num(a$sig_season, 1, 11), num(b$sig_season, 1, 11), num(r, 3, 8)))
}
med <- if (length(ratios)) median(ratios) else NA
p3 <- !is.na(med) && med < 1
verdict$P3 <- p3
note("    theory says ", num(theo, 3, 0), " (assumes the added cells are as quiet as the shipped ones)")
note("    median realised ratio ", num(med, 3, 0))
note("  P3: ", if (isTRUE(p3)) "PASS" else "FAIL", " -- threshold ",
     if (isTRUE(p3)) "improves" else "does NOT improve",
     if (!is.na(med) && med > theo) "; short of theory, and the gap is the price of the borrowed offset."
     else "; at or better than theory.")

# --- P4: attenuation --------------------------------------------------------
note("")
note("  P4: the positive control split by whether the season's xG is observed or reconstructed")
a6 <- diffs_for(pos6[pos6$season %in% GRID_6 & !pos6$infeasible, ],
                "hold", "_per_gw", min_cells = 2, quiet = TRUE)[[POS]]
p4 <- NA
if (!is.null(a6)) {
  nat <- a6$diff[!(a6$season %in% ADDED_2)]
  bak <- a6$diff[a6$season %in% ADDED_2]
  mn <- mean(nat); mb <- mean(bak)
  note(sprintf("    %-36s %4s %10s %11s", "cells", "n", "mean/gw", "a season"))
  note(sprintf("    %-36s %4d %10s %11s", "native xG (the shipped four)", length(nat),
               num(mn, 4, 10), num(mn * SEASON_GWS, 1, 11)))
  note(sprintf("    %-36s %4d %10s %11s", "backfilled, borrowed offset (added)", length(bak),
               num(mb, 4, 10), num(mb * SEASON_GWS, 1, 11)))
  ratio <- if (mn == 0) NA else mb / mn
  note("    ratio backfilled/native: ", num(ratio, 3, 0),
       "  (pre-declared: below 0.50 is material)")
  note("")
  note(sprintf("    %-13s %4s %10s %11s %10s", "season", "n", "mean/gw", "a season", "sd/gw"))
  for (s in GRID_6) {
    x <- a6$diff[a6$season == s]
    if (!length(x)) next
    note(sprintf("    %-13s %4d %10s %11s %10s",
                 paste0(s, if (s %in% ADDED_2) " *" else "  "), length(x),
                 num(mean(x), 4, 10), num(mean(x)*SEASON_GWS, 1, 11), num(sd(x), 4, 10)))
  }
  note("    * backfilled on a borrowed provider offset")
  p4 <- !is.na(ratio) && ratio >= 0.50
}
verdict$P4 <- p4
note("  P4: ", if (isTRUE(p4)) "PASS" else "FAIL")

# --- P5: the degrees of freedom actually rise -------------------------------
note("")
note("  P5: realised Satterthwaite df (nominal S-1 would be 3 then 5)")
note(sprintf("    %-30s %12s %12s", "arm", "4 seasons", "6 seasons"))
rose <- c()
for (rw in rows) {
  a <- get1(paste0(rw[1], "4 hold"), rw[2]); b <- get1(paste0(rw[1], "6 hold"), rw[2])
  if (is.null(a) || is.null(b) || is.na(a$df) || is.na(b$df)) next
  rose <- c(rose, b$df > a$df)
  note(sprintf("    %-30s %12s %12s", substr(rw[2], 1, 30), num(a$df, 2, 12), num(b$df, 2, 12)))
}
p5 <- length(rose) > 0 && all(rose)
verdict$P5 <- p5
note("  P5: ", if (isTRUE(p5)) "PASS" else "FAIL")

# --- the adoption rule ------------------------------------------------------
note(""); hr()
note("THE ADOPTION RULE, as committed before the run")
hr()
V <- function(k) if (is.null(verdict[[k]]) || is.na(verdict[[k]])) NA else verdict[[k]]
if (isFALSE(V("P2"))) {
  note("VERDICT: REFUSE -- a near-null arm resolved, or the determinism control moved.")
} else if (isFALSE(V("P1"))) {
  note("VERDICT: REFUSE -- the positive control does not reproduce.")
} else if (isFALSE(V("P3")) || isFALSE(V("P4")) || isFALSE(V("P5"))) {
  note("VERDICT: SIX FOR CONFIRMATION ONLY, not as the default grid.")
  note("  P3 ", V("P3"), "   P4 ", V("P4"), "   P5 ", V("P5"))
} else if (all(vapply(c("P1","P2","P3","P4","P5"), function(k) isTRUE(V(k)), logical(1)))) {
  note("VERDICT: ADOPT SIX AS DEFAULT -- and say in the same breath what happens to")
  note("  the four-season record.")
} else {
  note("VERDICT: CANNOT TELL -- a condition is undecided; see above.")
}

# --- nesting check ----------------------------------------------------------
note(""); hr()
note("NESTING CHECK: the six-season run restricted to the shipped four, against")
note("the independently-run four-season sweep")
hr()
for (pair in list(c("pos6","pos4"), c("neg6","neg4"))) {
  w <- get(pair[1]); n <- get(pair[2])
  sub <- w[w$season %in% GRID_4, ]
  # ⚠️ Joined on `label`, NOT on `cell`, and this is the one place in the family
  # where that is right. `cell` embeds `(run_id, sweep)` so that rows from two
  # blocks of ONE file cannot cross-pair — but this comparison is between two
  # SEPARATE files, produced by two separate runs, whose run_id is
  # `unix-seconds-pid` and therefore never matches. Joining on `cell` here makes
  # the merge empty, and an empty merge takes the `nrow(bad) == 0` branch and
  # prints "byte-identical": **the failure printing as the pass.**
  #
  # That was live for the length of one commit during the A2 migration, on this
  # script's only documented invocation. `label` is the display key the migration
  # separated out precisely so a cross-run comparison had something to join on.
  m <- merge(sub[, c("variant","label","hold_points","policy_points","moves")],
             n[, c("variant","label","hold_points","policy_points","moves")],
             by = c("variant","label"), suffixes = c(".wide", ".narrow"))
  # An empty join is never a pass. Without this the check reports success on any
  # pair of files that share no cell at all — a mistyped path, a renamed arm, or
  # the wrong key.
  if (nrow(m) == 0) {
    fail(pair[1], " and ", pair[2], " share no (variant, season@start_gw) in ",
         "common, so the nesting check compared nothing. An empty comparison is ",
         "not a byte-identical one.")
  }
  bad <- m[m$hold_points.wide != m$hold_points.narrow |
             m$policy_points.wide != m$policy_points.narrow |
             m$moves.wide != m$moves.narrow, ]
  note("  ", pair[1], " vs ", pair[2], ": ", nrow(m), " cells compared, ",
       nrow(bad), " disagree",
       if (nrow(bad) == 0) " -- byte-identical." else " -- NOT NESTED.")
  if (nrow(bad) > 0) print(utils::head(bad, 6))
}

# --- supplementary, descriptive only ----------------------------------------
note(""); hr()
note("SUPPLEMENTARY (descriptive; no condition rides on it)")
hr()
note("")
note("Baseline scoring level by season -- is this the same football?")
note(sprintf("  %-13s %5s %10s %11s %9s", "season", "n", "HOLD/gw", "POLICY/gw", "moves"))
b <- pos6[pos6$is_baseline & !pos6$infeasible, ]
for (s in GRID_6) {
  x <- b[b$season == s, ]
  if (!nrow(x)) next
  note(sprintf("  %-13s %5d %10s %11s %9s",
               paste0(s, if (s %in% ADDED_2) " *" else "  "), nrow(x),
               num(mean(x$hold_per_gw), 3, 10), num(mean(x$policy_per_gw), 3, 11),
               num(mean(x$moves), 1, 9)))
}
note("  * backfilled xG on a borrowed offset")
note("")
note("Between-season spread of the positive control (what CR2 clusters on)")
note(sprintf("  %-22s %6s %13s %13s", "grid", "seas", "sd between", "sd within"))
for (g in list(list(GRID_4, "4 seasons"), list(GRID_6, "6 seasons"))) {
  a <- diffs_for(pos6[pos6$season %in% g[[1]] & !pos6$infeasible, ],
                 "hold", "_per_gw", min_cells = 2, quiet = TRUE)[[POS]]
  if (is.null(a)) next
  ms <- tapply(a$diff, a$season, mean); wi <- tapply(a$diff, a$season, sd)
  note(sprintf("  %-22s %6d %13s %13s", g[[2]], length(ms),
               num(sd(ms), 4, 13), num(mean(wi, na.rm = TRUE), 4, 13)))
}
note("  'sd between' is the spread of the per-season means -- the quantity a")
note("  season-clustered SE divides by sqrt(S). If it grows faster than sqrt(S)")
note("  grows, adding seasons makes the threshold WORSE rather than better.")

note("")
note("Reading this: mean/gw is points per gameweek played; multiply by 38 for the")
note("season-scale figures the record quotes. sig/season is the effect that would")
note("land exactly at p = 0.05 -- the bar, not the finding.")
