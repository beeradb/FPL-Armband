#!/usr/bin/env Rscript

# Bench-boost PLACEMENT: the canary's ceiling, and the state rule against the
# fixed-offset control.
#
# Usage:
#   Rscript stats/bench_boost_placement.R <cells.csv>
#
# Produce the input with:
#   DIAG=1 EXP=BBCEILING FPL_CELLS=/tmp/bbceiling/cells.csv \
#       FPL_SWEEP_SEASONS=extended scripts/replay \
#       -run '^TestDiagBenchBoostPlacement$' -v -timeout 3h
#
# The pre-registration is
# stats/findings/2026-08-17-bench-boost-placement-PREREGISTRATION.md and this
# script computes exactly what it declares, in the order it declares it.
#
# ---------------------------------------------------------------------------
# Why this is not sweep_inference.R
#
# `sweep_inference.R` differences a METRIC_COLS entry between an arm and the
# BASELINE arm, on one column. Neither half of that fits here:
#
#   - the quantity is a CHIP EVENT column, not one of the eight metrics. A chip
#     pays in one gameweek, so the cell total is the whole effect and the
#     per-gameweek scale does not apply to it at all;
#   - the ceiling is a CROSS-ARM read on TWO DIFFERENT columns —
#     `bench_boost_oracle_pts` on the oracle arm against `bench_boost_pts` on the
#     control arm — and neither of them is the baseline arm. The baseline arm
#     exists in the file to carry the path-invariance identity, not to be
#     differenced against.
#
# What it does NOT do is carry its own estimator. The CR2 fit, the
# `t_crit(df) x SE` threshold, the 80%-power MDE, the wild cluster bootstrap and
# the cells reader all come from `cells_common.R`, because a diagnostic must never
# carry its own copy of the thing it is checking.
#
# ⚠️ Every figure printed here is POINTS PER SEASON-PATH. Do not divide by weeks
# played and do not multiply by 38.
# ---------------------------------------------------------------------------

args <- commandArgs(trailingOnly = TRUE)
if (length(args) != 1) {
  stop("usage: bench_boost_placement.R <cells.csv>", call. = FALSE)
}

source(file.path(dirname(sub("^--file=", "", grep("^--file=", commandArgs(FALSE),
                                                  value = TRUE)[1])),
                 "cells_common.R"))

d <- read_cells(args[1])

if (length(unique(d$block)) != 1) {
  fail("this file carries ", length(unique(d$block)), " blocks; run one block ",
       "per file. Pooling two runs of one design into one sample is what the ",
       "(run_id, sweep) key exists to prevent.")
}
if (any(d$infeasible)) {
  note("!! ", sum(d$infeasible), " infeasible row(s) — a cell that could not run ",
       "is dropped below and is NOT a zero.")
}
d <- d[!d$infeasible, ]

for (col in c("bench_boost_gw", "bench_boost_pts", "policy_points")) {
  if (is.null(d[[col]])) fail("this file has no ", col, " column")
  d[[col]] <- as.numeric(d[[col]])
}

# --- the arms, matched on what they installed rather than on an index ---------
#
# Arm 0 is the no-chip reference: it is the baseline, plays no chip, and exists
# only to carry the path-invariance identity. Arm 1 is the control. Arm 2 is
# whichever placement rule this block ran.
#
# ⚠️ Matching on `variant_index` alone is what `gate_additivity.R` records being
# bitten by — a re-ordered sweep reported a confidently significant effect that was
# pure mislabelling. So each arm is checked against a property only it can have.
arm <- function(i) d[!is.na(d$variant_index) & d$variant_index == i, ]
a0 <- arm(0)
a1 <- arm(1)
a2 <- arm(2)
if (nrow(a0) == 0 || nrow(a1) == 0 || nrow(a2) == 0) {
  fail("expected three arms at variant_index 0/1/2; got ",
       nrow(a0), "/", nrow(a1), "/", nrow(a2), " rows")
}
if (!all(a0$is_baseline)) fail("variant_index 0 is not the baseline arm")
if (any(a0$bench_boost_gw != 0)) {
  fail("the baseline arm played a bench boost in ", sum(a0$bench_boost_gw != 0),
       " cell(s). Arm 0 must be the un-chipped reference or every identity below ",
       "is taken against the wrong season.")
}
if (any(a1$bench_boost_gw == 0)) {
  fail("the control arm placed no bench boost in ", sum(a1$bench_boost_gw == 0),
       " cell(s) — those are comparisons that could not run and must be counted, ",
       "not differenced.")
}

has_oracle <- !is.null(d$bench_boost_oracle_pts) &&
  any(!is.na(a2$bench_boost_oracle_pts))
mode <- if (has_oracle) "CEILING" else "RULE"

note("")
hr()
note("bench-boost placement — block: ", unique(d$block))
note("mode: ", mode, "   arms: ", paste(unique(d$variant[order(d$variant_index)]),
                                        collapse = " | "))
note("seasons: ", length(unique(d$season)), "   entry points: ",
     length(unique(d$start_gw)), "   cells: ", nrow(a1))
note("UNITS: points per season-path. The cell total IS the effect — a chip pays")
note("in one gameweek. Do not divide by weeks and do not multiply by 38.")
hr()

# --- check 1: the path invariance the whole design rests on -------------------
#
# The Go diagnostic checks the gain VECTOR, which the CSV cannot carry. What the
# CSV can carry is the identity that follows from it, and it is exact in integers:
# `weekPointsWithChip` is `weekScoreWithChip(...).Points` and `BenchBoostGain` is
# the difference of exactly those two calls, so a season that scored identically in
# every other week satisfies this and one that did not cannot.
inv <- merge(a0[, c("cell", "label", "policy_points", "squad_hash", "moves", "hits")],
             a1[, c("cell", "policy_points", "bench_boost_pts", "squad_hash",
                    "moves", "hits")],
             by = "cell", suffixes = c(".base", ".arm"))
inv2 <- merge(a0[, c("cell", "policy_points", "squad_hash", "moves", "hits")],
              a2[, c("cell", "policy_points", "bench_boost_pts", "squad_hash",
                     "moves", "hits")],
              by = "cell", suffixes = c(".base", ".arm"))
check_invariance <- function(j, who) {
  bad_id <- j$policy_points.arm - j$policy_points.base != j$bench_boost_pts
  bad_sq <- j$squad_hash.arm != j$squad_hash.base |
    j$moves.arm != j$moves.base | j$hits.arm != j$hits.base
  note("  ", who, ": squad/moves/hits identical in ",
       sum(!bad_sq), " of ", nrow(j), " cells; the exact points identity holds in ",
       sum(!bad_id), " of ", nrow(j))
  if (any(bad_sq) || any(bad_id)) {
    fail(who, " is not path-invariant against the no-chip baseline, so the ",
         "oracle's argmax and this arm's pick are readings of two different ",
         "seasons and nothing below is a ceiling or a paired difference.")
  }
}
note("PATH INVARIANCE (against the no-chip baseline arm):")
check_invariance(inv, "control")
check_invariance(inv2, "arm 2")

# --- the per-cell difference --------------------------------------------------
#
# `diffs_for` differences one column between an arm and the baseline. Neither the
# column nor the baseline is right here — see the header — so the join is written
# out. It is the same (run_id, sweep, season, start_gw) key `read_cells` builds,
# which is the half of that function that actually matters.
key <- c("cell", "label", "season", "start_gw", "weeks")
if (mode == "CEILING") {
  a2$reading <- as.numeric(a2$bench_boost_oracle_pts)
  a2$reading_gw <- as.numeric(a2$bench_boost_oracle_gw)
} else {
  a2$reading <- a2$bench_boost_pts
  a2$reading_gw <- a2$bench_boost_gw
}
j <- merge(a1[, c(key, "bench_boost_pts", "bench_boost_gw")],
           a2[, c("cell", "reading", "reading_gw")], by = "cell")
j$diff <- j$reading - j$bench_boost_pts
j <- j[order(j$season, j$start_gw), ]

note("")
note("--- per cell")
note(sprintf("%-9s %6s %6s | %7s %7s | %7s %7s | %8s",
             "season", "entry", "weeks", "ctl gw", "ctl pts", "arm gw", "arm pts",
             "diff"))
for (i in seq_len(nrow(j))) {
  note(sprintf("%-9s %6d %6d | %7d %7d | %7d %7d | %8d",
               j$season[i], j$start_gw[i], j$weeks[i],
               j$bench_boost_gw[i], j$bench_boost_pts[i],
               j$reading_gw[i], j$reading[i], j$diff[i]))
}

# --- placement liveness -------------------------------------------------------
#
# A cell where the two arms play in the same week is a cell where the comparison
# could not run, and it must be counted rather than entered as a zero. In CEILING
# mode a tie means the fixed offset happened to catch the best week; in RULE mode
# it means the rule agreed with the control, and if that is every cell the
# deliverable IS the count.
moved <- sum(j$reading_gw != j$bench_boost_gw)
note("")
note("LIVENESS — cells whose bench-boost gameweek differs from the control: ",
     moved, " of ", nrow(j))
note("distinct weeks arm 2 chose: ", length(unique(j$reading_gw)))
if (moved == 0) {
  note("")
  note("!! Arm 2 never moved the chip's week. That count IS the deliverable:")
  note("!! the arm did not act, so there is nothing for a paired difference to")
  note("!! be about, and the numbers below describe a comparison that never ran.")
}

# --- inference ----------------------------------------------------------------

if (!has_cs) {
  note("")
  note("!! clubSandwich is not installed — no CR2 SE, no threshold, no bootstrap.")
  quit(status = 1)
}

fit <- data.frame(diff = j$diff, season = j$season, start_gw = j$start_gw,
                  stringsAsFactors = FALSE)
m <- mean(fit$diff)

report <- function(lbl, s) {
  if (is.na(s$se)) {
    note(sprintf("  %-18s degenerate — every cell identical, no SE", lbl))
    return(invisible(NULL))
  }
  q <- sig_and_mde(s$se, s$df)
  note(sprintf("  %-18s SE %7.4f  df %5.2f  t %7.3f  t_crit %6.3f  threshold %7.2f  MDE %7.2f",
               lbl, s$se, s$df, s$t, q[["t_crit"]], q[["sig"]], q[["mde"]]))
  invisible(q)
}

note("")
note("--- inference. All figures POINTS PER SEASON-PATH.")
note(sprintf("mean difference over %d cells: %+.3f", nrow(fit), m))
note("")
s_season <- se_cr(fit, fit$season)
s_start <- se_cr(fit, fit$start_gw)
q_season <- report("season-clustered", s_season)
q_start <- report("entry-clustered", s_start)

if (!is.na(s_season$se) && !is.na(s_start$se)) {
  note(sprintf("  threshold RANGE across the two estimators: %.2f to %.2f",
               min(q_season[["sig"]], q_start[["sig"]]),
               max(q_season[["sig"]], q_start[["sig"]])))
}

# Season means, so cell-level concentration is visible before any p is read.
sm <- tapply(fit$diff, fit$season, mean)
note("")
note("season means: ",
     paste(sprintf("%s %+.2f", names(sm), sm), collapse = "  "))
note("season means above zero: ", sum(sm > 0), " of ", length(sm),
     "   cells above zero: ", sum(fit$diff > 0), " of ", nrow(fit))

# Wild cluster bootstrap, quoted with S_eff and its floor, per the standing rule.
w <- wild_cluster_p_season(fit)
note("")
note("wild cluster bootstrap (Webb 6-point, enumerated): ", wcb_label(w))
note(sprintf("  S %s   S_eff %s   floor 6/6^S_eff = %s",
             format(w$S), format(w$S_eff),
             if (is.na(w$floor)) "NA" else formatC(w$floor, format = "g", digits = 4)))
if (!is.na(w$floor) && w$floor > 0.05) {
  note("  !! the floor exceeds 0.05: this arm is UNMEASURABLE on this estimator,")
  note("  !! not null. Seasons of exactly zero difference do not count toward S_eff.")
}

# Leave-one-season-out. Subsets share five of six seasons, so sign stability here
# is arithmetic rather than evidence — printed because a sign CHANGE is still
# informative and its absence is not.
note("")
note("leave-one-season-out means (each subset shares 5 of 6 seasons — a stable")
note("sign is arithmetic, a sign CHANGE is the informative outcome):")
for (s in sort(unique(fit$season))) {
  sub <- fit[fit$season != s, ]
  note(sprintf("  drop %-9s %+.3f", s, mean(sub$diff)))
}

# --- post-hoc, and labelled so it cannot be read as pre-registered -------------
#
# ⚠️ **Neither reading below is in the pre-registered family.** They are printed
# because a verdict that omitted them would be over-readable, and they carry no p
# for exactly the reason the family was declared in advance: a contrast chosen
# after the numbers are seen is not a contrast the multiplicity accounting covers.

note("")
note("--- POST-HOC. Not in the pre-registered family; no p is offered for either.")

# Leave-one-CELL-out, which is a different question from leave-one-season-out and
# is the one that answers "is one cell carrying this". Its own limitation is
# stated: 35 of 36 cells are shared, so the SPREAD is the reading and the level is
# not news.
loo <- vapply(seq_len(nrow(fit)), function(i) mean(fit$diff[-i]), numeric(1))
note(sprintf("leave-one-CELL-out mean spans %+.3f to %+.3f (full sample %+.3f).",
             min(loo), max(loo), m))
note("  A cell whose removal moves this materially is a cell carrying the arm.")

# The arm's own LEVEL against playing no chip at all. This is a different question
# from placement — it asks whether the lever should be on, not where it should
# fire — and the shipped default plays no bench boost, so it is the number a
# reader will reach for and must not mistake for the result above.
lev <- merge(a0[, c("cell", "policy_points")],
             a2[, c("cell", "policy_points", "season", "start_gw")],
             by = "cell", suffixes = c(".base", ".arm"))
lev$diff <- lev$policy_points.arm - lev$policy_points.base
note("")
note(sprintf("arm 2's LEVEL against playing no chip at all: %+.3f points per",
             mean(lev$diff)))
note("season-path, over ", nrow(lev), " cells. That is a DIFFERENT question —")
note("whether the lever should be on, not where it should fire — and it is not")
note("what either block was designed to answer. Quoted without a threshold.")

# --- the gate -----------------------------------------------------------------

if (mode == "CEILING") {
  note("")
  hr()
  note("THE GATE, as pre-registered")
  note("")
  note("The ceiling is >= 0 in EVERY cell by construction: the oracle's argmax")
  note("ranges over the slice the control's pick is drawn from. So a t against")
  note("zero is MECHANICAL and its p-value is meaningless — the same status this")
  note("record assigns the perfect armband's t of 20.4. What follows is a SIZING")
  note("criterion, not a hypothesis test.")
  note("")
  if (any(j$diff < 0)) {
    fail("the ceiling is negative in ", sum(j$diff < 0), " cell(s), which the ",
         "construction forbids — the arms did not score the same season.")
  }
  if (is.na(s_season$se)) {
    note("The season-clustered SE is degenerate, so the gate cannot be evaluated.")
  } else {
    thr <- q_season[["sig"]]
    lo <- m - qt(0.95, s_season$df) * s_season$se
    note(sprintf("mean ceiling            %+.2f", m))
    note(sprintf("season-clustered thresh %7.2f  (t_crit(%.2f) x SE)", thr, s_season$df))
    note(sprintf("one-sided 95%% lower bnd %+.2f  <- the legitimate reading of a bound", lo))
    note(sprintf("80%%-power MDE           %7.2f", q_season[["mde"]]))
    note("")
    if (m > thr) {
      note(">>> GATE OPEN: the ceiling exceeds this comparison's own threshold.")
      note(">>> The rule arm is worth running. It is bounded cellwise by this")
      note(">>> ceiling, so it competes for a fraction of what just cleared.")
    } else {
      note(">>> GATE CLOSED: the ceiling does NOT exceed this comparison's own")
      note(">>> threshold. A placement rule's difference is bounded cellwise by")
      note(">>> the ceiling, so nothing in this family can resolve on this")
      note(">>> instrument. Do not run the rule arm.")
    }
  }
  hr()
} else {
  note("")
  hr()
  note("This is the RULE arm. Its difference is bounded cellwise by the ceiling")
  note("measured in the BBCEILING block; read the two together.")
  hr()
}
