#!/usr/bin/env Rscript

# Inference for the Go replay's sweep cells.
#
# Usage:
#   Rscript stats/sweep_inference.R /tmp/cells.csv [more.csv ...]
#   Rscript stats/sweep_inference.R --out=stats/out --no-plots /tmp/cells.csv
#   Rscript stats/sweep_inference.R --scale=per_path /tmp/cells.csv
#   Rscript stats/sweep_inference.R --vary=FPL_XGC_EXTERNAL_DIR a.csv b.csv
#
# ⚠️ Reading two cell files together means DIFFERENCING them, so they must have
# been measured at one code state. This refuses when they were not, and
# `--vary=<field>[,<field>]` is how a comparison declares what it is varying on
# purpose. See check_shared_code_state.
#
# Produce the input with:
#   FPL_CELLS=/tmp/cells.csv DIAG=1 EXP=A go test ./internal/backtest \
#       -run TestDiagTransferPolicy -v -timeout 90m
#
# ---------------------------------------------------------------------------
# Why this is not in Go
#
# The Go harness owns the engine and nothing else. It replays the grid, writes
# one row per (sweep, variant, season, start point), and prints the mean paired
# difference. Every standard error, degree of freedom, p-value and plot is
# computed here, in one place.
#
# That split exists because three specific defects had accumulated in the
# hand-rolled Go statistics:
#
#   1. The season-clustered SE averaged each of four seasons to one number and
#      took their spread. No small-sample correction, no principled df, and
#      AGENTS.md conceded it was "a noisy estimate of noise".
#   2. The variance decomposition computed each layer's marginal SE as the
#      root-sum-square of two adjacent cumulative SEs, which assumes
#      independence between layers that differ by one mechanism on the *same*
#      cells and the same weeks. The test documented that this was invalid and
#      reported it anyway.
#   3. Nothing controlled for multiplicity, while roughly twenty constants times
#      four to six alternatives each were judged at |t| >= 2. At alpha = 0.05
#      that expects about four spurious "confirmed" verdicts, and at least three
#      rows labelled "confirmed" in AGENTS.md turned out to have no t at all.
#
# The engine stays in Go because the hot path is discrete combinatorial search —
# the exact per-formation DP, the knapsack, the paired-move local search — which
# is branchy scalar work. A single sweep arm is already about fifteen minutes.
#
# R is a developer tool here, not a dependency: `go build`, `go vet` and
# `go test` must all pass on a machine with no R installed, and nothing in the
# Go test suite invokes this script.
# ---------------------------------------------------------------------------

# --- options ---------------------------------------------------------------

args <- commandArgs(trailingOnly = TRUE)
opt_out <- "stats/out"
opt_plots <- TRUE
# Relative tolerance for the two reproduction checks. Both compare arithmetic
# rather than formatting — the CSV carries full round-tripping float64 precision
# — so this only has to absorb summation order.
opt_tol <- 1e-9
# The scale the paired difference is taken on.
#
#   per_gw    <metric>_per_gw — the cell total divided by gameweeks played. Right
#             for a RATE, where the cell total is delta x weeks and dividing
#             recovers delta.
#   per_path  <metric>_points — the cell total itself, one number per
#             season-path. Right for an EVENT COUNT, where the cell total is the
#             whole effect and dividing by weeks manufactures a per-gameweek rate
#             that does not exist.
#
# These are different estimands and not two units for one number: paths have
# unequal length, so per_path weights a GW1 entry's 38 gameweeks the same as a
# GW26 entry's 13 rather than three times as heavily. That is the correct
# weighting when what is being measured happens a fixed number of times per path.
# See the harness-and-inference note.
opt_scale <- "per_gw"
# The provenance fields this comparison DECLARES it is varying. Everything else
# must agree across the inputs, or they are not two arms of one comparison — see
# check_shared_code_state.
opt_vary <- character(0)
paths <- character(0)

for (a in args) {
  if (identical(a, "--no-plots")) {
    opt_plots <- FALSE
  } else if (grepl("^--out=", a)) {
    opt_out <- sub("^--out=", "", a)
  } else if (grepl("^--tol=", a)) {
    opt_tol <- as.numeric(sub("^--tol=", "", a))
  } else if (grepl("^--scale=", a)) {
    opt_scale <- sub("^--scale=", "", a)
    if (!opt_scale %in% c("per_gw", "per_path")) {
      stop("--scale must be per_gw or per_path, not: ", opt_scale)
    }
  } else if (grepl("^--vary=", a)) {
    opt_vary <- strsplit(sub("^--vary=", "", a), ",")[[1]]
    opt_vary <- trimws(opt_vary[nzchar(trimws(opt_vary))])
  } else if (grepl("^--", a)) {
    stop("unknown option: ", a)
  } else {
    paths <- c(paths, a)
  }
}
# The suffix every metric column is read through, and the words the report uses
# for it. One definition, because a table headed "per gameweek" over totals is
# the failure this option exists to make impossible.
SCALE_SUFFIX <- if (opt_scale == "per_path") "_points" else "_per_gw"
SCALE_UNITS <- if (opt_scale == "per_path")
  "points per season-path (the cell total; no division by weeks)" else
  "points per gameweek played; multiply by ~38 for a season"
# The scale is in the filename on anything but the default, because the README
# tells the reader to run both and nothing else in the output distinguishes them.
# Without this a per_path run silently overwrites the per_gw run's CSV and every
# plot, leaving files that look right and carry the other estimand.
scale_tag <- if (opt_scale == "per_gw") "" else paste0("-", opt_scale)
if (length(paths) == 0) {
  stop("give at least one cells CSV; see the header of this file for usage")
}

# The paired difference, the cluster-robust SEs and the regime split live in
# cells_common.R, because schedule_screen.R computes the same quantities per
# entry gameweek and "one quantity, two implementations" is this project's
# signature failure. The I/O below is deliberately NOT shared: its failure mode
# is loud, and the arithmetic's is silent.
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

# --- load ------------------------------------------------------------------

# `read_cells_all` is in cells_common.R, and so are the contract checks that used
# to sit here: the required-column list, the flag coercion, the cell key and the
# block key. It also adds two this script did not have — exactly one baseline arm
# per block, and `is_baseline` agreeing with `variant_index == 0` — both verified
# against the whole bank before promotion, with zero failures.

note("Checking the inputs share a code state...")
check_shared_code_state(paths, opt_vary)

cells <- read_cells_all(paths)

# This script needs more than the shared contract: it reports POLICY as well as
# HOLD, so the policy columns are required here rather than in cells_common.R,
# where a HOLD-only reader would be refused a file it can use.
required <- c("bank_up_to", "policy_points", "hold_points", "moves",
              "policy_per_gw", "hold_per_gw")
missing_cols <- setdiff(required, names(cells))
if (length(missing_cols) > 0) {
  fail("cells CSV is missing columns: ", paste(missing_cols, collapse = ", "))
}

# Every per-cell score column the CSV can carry, whether or not a given sweep
# measured it. Two groups, gated independently in Go and blank when unmeasured —
# "not measured" and "measured as nothing" are different facts, and a zero here
# would be averaged into a mean:
#
#   policy, hold                      every sweep
#   frozen, frozen_captain, weekly    the variance decomposition's ladder, in
#                                     which the eleven is frozen at the day-one
#                                     pick and mechanisms are added back one at a
#                                     time
#   hold_fixedcap, hold_nocap         the captaincy rungs: HOLD with the armband
#                                     pinned to the day-one pick, and HOLD with
#                                     nobody doubled. Diagnostic instruments for
#                                     asking how much of HOLD's noise is the
#                                     armband — neither is what FPL pays, so
#                                     neither substitutes for HOLD.
#   hold_xpoints, policy_xpoints      the accumulated-xPoints instrument: the
#                                     same season, the same eleven, the same
#                                     autosubs and armband, with the four
#                                     conversion channels analysis.XPointsResidual
#                                     replaces taken off each player-gameweek.
#                                     Present only in files written since the
#                                     column existed
METRIC_COLS <- c("frozen", "frozen_captain", "weekly", "hold", "policy",
                 "hold_fixedcap", "hold_nocap",
                 "hold_xpoints", "policy_xpoints")

# The xPoints totals are named `hold_xpoints`, not `hold_xpoints_points`, so the
# `<metric>_points` / `<metric>_per_gw` convention every estimator here addresses
# columns by does not reach them. Aliased once, here, rather than teaching the
# shared machinery in cells_common.R a second naming rule — a second rule is a
# second place for the two to disagree, and the per-gameweek half already has the
# conventional name.
#
# Written only when the source column exists, so a banked file that predates the
# instrument stays readable and simply has no xPoints metric.
for (m in c("hold_xpoints", "policy_xpoints")) {
  if (!is.null(cells[[m]])) cells[[paste0(m, "_points")]] <- cells[[m]]
}

# Two further columns, `oracle` and `oracle_kind`, say what hindsight each cell was
# granted: "-"/"none" for an ordinary row, otherwise a canonical stamp such as
# "info:availability" and a kind of info/decision/composite. They are deliberately
# NOT in `required` — a cells file written before they existed stays readable — and
# nothing here branches on them yet.
#
# What they are for: an oracled arm is a hindsight **upper bound**, not a score, and
# its totals are not comparable with any figure in AGENTS.md. Go refuses a sweep
# whose baseline arm is oracled and checks each oracle's declared invariance
# immediately after the grid, so a violation fails the run rather than waiting to be
# noticed here. See the oracle-design document.
for (col in c("variant_index", "start_gw", "weeks", "bank_up_to",
              paste0(METRIC_COLS, "_points"), paste0(METRIC_COLS, "_per_gw"), "moves")) {
  if (!is.null(cells[[col]])) {
    cells[[col]] <- suppressWarnings(as.numeric(cells[[col]]))
  }
}

# --- the one season POLICY may not use --------------------------------------
#
# FPL granted unlimited free transfers before 2019-20's GW30+ deadline after the
# COVID restart, and froze prices for three months. So that season's transfer
# path and its wallet are not samples of the same process as any other season's,
# while its *scoring* is fine — which is the split the HOLD-only seven-season
# grid exists to exploit.
#
# It reaches a cells file only through FPL_SWEEP_SEASONS=scoring, so this is
# inert for every file written before that grid existed. It is here anyway
# because the Go side's refusal to print POLICY is a *printout*, and the cells
# file outlives it: a pooled POLICY mean over this season would be a plausible
# number of the right magnitude with nothing about it to say which population it
# came from. Blanked rather than dropped, so the HOLD rows of the same cells stay
# — "not usable on this metric" and "not run" are different facts.
POLICY_EXCLUDED_SEASON <- "2019-20"
excluded <- cells$season == POLICY_EXCLUDED_SEASON
if (any(excluded)) {
  note("")
  note("  !!  ", sum(excluded), " cell(s) from ", POLICY_EXCLUDED_SEASON,
       " are excluded from POLICY and kept for HOLD.")
  note("      That season had unlimited free transfers before the GW30+ deadline",
       " and three months")
  note("      of frozen prices, so its transfer path is not a sample of the same",
       " process.")
  cells$policy_per_gw[excluded] <- NA
  cells$policy_points[excluded] <- NA
  # And the same season on the same transfer path scored the other way. The
  # refusal is about the PATH — unlimited free transfers and three frozen months —
  # not about how its points were counted, so a metric swap does not make the
  # season comparable again. Missing this would leave one POLICY reading refused
  # and its instrument twin quietly reporting the excluded cells.
  for (col in c("policy_xpoints", "policy_xpoints_points", "policy_xpoints_per_gw")) {
    if (!is.null(cells[[col]])) cells[[col]][excluded] <- NA
  }
  # The chip block is the THIRD instrument on this path, and the list has now been
  # extended twice for the same reason. Every column below is a reading of
  # Week.BenchBoostGain or Week.TripleCaptainGain, which simulate.go computes from
  # the eleven and bench the TRANSFER POLICY held that week — so an unlimited-free-
  # transfer season's squad path is upstream of every one of them, exactly as it is
  # of policy_points.
  #
  # The trap it closes is that the chip-oracle block is gated on HasChipOracle, so
  # the natural reader filter is "rows where the oracle column is non-NA" — and a
  # 2019-20 row passes that with nothing on it saying which population it came from.
  # Reaching it needs a non-default season set AND the chip axis, so it is latent
  # rather than live, and this is the cheapest moment to write the line.
  #
  # It is inert on every tracked cells file, and the reason is the `season` column
  # rather than anything about the chip columns. Counted over all tracked CSVs: 15
  # carry `bench_boost_pts` and `triple_captain_pts`, which every sweep emits, and
  # NONE of the 15 has a single row with `season == "2019-20"` — so `excluded` is
  # all FALSE and the loop never fires. No tracked file carries the six
  # `*_oracle_*`/`*_median_pts`/`*_threshold_pts` columns at all.
  #
  # ⚠️ Two wrong versions of that sentence were written before anyone counted, and
  # both are failures this record already carries. "Ten of those hold 2019-20 rows"
  # came from a `grep 2019-20`, which matches **`prior_season`**: 12 files have
  # 2019-20 as a PRIOR and 0 have it as the season played, and only the latter is
  # what `excluded` keys on — right file, wrong COLUMN. And "thirteen" counted
  # `stats/snapshots/` only; the two it dropped, `stats/cells/2026-08-13-chipprep3.csv`
  # and `chipseq2.csv`, are exactly the two with NON-ZERO chip values, so one
  # sentence got the population and its one interesting property wrong together.
  #
  # The trap is one sweep away rather than hypothetical: two banked files DO carry a
  # played 2019-20 — `stats/snapshots/2026-08-13-aa95f75/cells/xgc7-{on,off}.csv`,
  # the seven-season HOLD grid, six such rows each — and neither carries a chip
  # column. Crossing those two properties in one sweep is all it takes, which is why
  # this is written now rather than after it bites.
  #
  # `*_bar_pts` is deliberately NOT here: `chipBarBenchBoost` and
  # `chipBarTripleCaptain` are Go consts, so the column is invariant to the season
  # and travels with the row only so a file can decode its own `threshold >= bar`
  # mixture. Blanking it would remove that decoding without removing a poolable
  # POLICY quantity.
  #
  # `bench_boost_gw` and `triple_captain_gw` are also out, and for a different
  # reason than the bars: they record the week the CHIP PLAN played the chip, set
  # from `cfg.plays(...)`, so they are config and not a measurement.
  # `*_oracle_gw` is IN, and the earlier "the week numbers identify a week rather
  # than measuring one" was wrong about it — the oracle week is the argmax over the
  # same per-week gains as the blanked readings, and cellcsv_test.go calls it the
  # block's mediator and warns that a distribution gets built from it. A
  # distribution over oracle weeks is exactly the pooled POLICY reading this block
  # refuses.
  for (col in c("bench_boost_pts", "triple_captain_pts",
                "bench_boost_oracle_gw", "bench_boost_oracle_pts",
                "bench_boost_median_pts", "bench_boost_threshold_pts",
                "triple_captain_oracle_gw", "triple_captain_oracle_pts",
                "triple_captain_median_pts",
                "triple_captain_threshold_pts")) {
    if (!is.null(cells[[col]])) cells[[col]][excluded] <- NA
  }
}

# Derived metrics: each layer minus the one before it.
#
# This is the fix for the marginal SE, and the point is that there is nothing to
# approximate. The Go version computed a marginal's SE as the root-sum-square of
# two adjacent cumulative SEs, which assumes independence between layers that
# differ by one mechanism on the same cells and the same weeks. But a marginal is
# itself a per-cell quantity — one layer minus the next, within a cell — so it is
# just another metric, and its SE comes out of the same machinery as any other
# with no independence assumption anywhere.
MARGINALS <- list(
  m_captain   = c("frozen_captain", "frozen"),
  m_weekly_xi = c("weekly", "frozen_captain"),
  m_autosub   = c("hold", "weekly"),
  m_transfers = c("policy", "hold")
)
# Gated per row on the decomposition having actually run. `m_transfers` is policy
# minus hold, which is computable for *any* sweep — but reporting it there would
# put a fourth near-duplicate of the POLICY row in front of the reader for every
# ordinary sweep, and it is only a "marginal" as part of the ladder. Per row
# rather than per file, so one CSV can hold both kinds of sweep.
decomposed <- if (is.null(cells$frozen_per_gw)) {
  rep(FALSE, nrow(cells))
} else {
  !is.na(cells$frozen_per_gw)
}

# The captaincy rungs' own marginals, gated on their own columns rather than on
# the ladder's, because the two blocks are measured by different sweeps.
#
#   m_armband     HOLD minus HOLD-with-nobody-doubled: what the armband
#                 contributes to the *effect being measured*, which is not the
#                 same question as what it contributes to a season's points.
#   m_capweekly   HOLD minus HOLD-with-the-armband-pinned-on-day-one: what
#                 re-picking the captain each week contributes.
CAPTAIN_MARGINALS <- list(
  m_armband   = c("hold", "hold_nocap"),
  m_capweekly = c("hold", "hold_fixedcap")
)
rungs_scored <- if (is.null(cells$hold_nocap_per_gw)) {
  rep(FALSE, nrow(cells))
} else {
  !is.na(cells$hold_nocap_per_gw)
}
ALL_MARGINALS <- c(MARGINALS, CAPTAIN_MARGINALS)
MARGINAL_GATE <- c(
  setNames(rep("ladder", length(MARGINALS)), names(MARGINALS)),
  setNames(rep("rungs", length(CAPTAIN_MARGINALS)), names(CAPTAIN_MARGINALS))
)
# Derived on both scales, because --scale=per_path reads the totals and a
# marginal that existed only per gameweek would silently vanish from that run —
# an arm disappearing from a report is the failure mode this file's `diffs_for`
# already names.
for (nm in names(ALL_MARGINALS)) {
  for (suffix in c("_per_gw", "_points")) {
    hi <- cells[[paste0(ALL_MARGINALS[[nm]][1], suffix)]]
    lo <- cells[[paste0(ALL_MARGINALS[[nm]][2], suffix)]]
    gate <- if (MARGINAL_GATE[[nm]] == "ladder") decomposed else rungs_scored
    cells[[paste0(nm, suffix)]] <- if (is.null(hi) || is.null(lo)) {
      NA
    } else {
      ifelse(gate, hi - lo, NA)
    }
  }
}

# A sweep is identified by the label *and* the process that produced it. Two
# runs of the same EXP block appended to one file are two samples, not one
# bigger one, and pooling them would shrink the SE without adding information.
# `block` and `cell` are set by read_cells, and `cell` now carries `(run_id,
# sweep)` too — so a caller that forgets to block cannot cross-pair, which is the
# defect grid_width.R had.

note("")
note("Read ", nrow(cells), " cells from ", length(paths), " file(s): ",
     length(unique(cells$block)), " sweep(s).")

# --- contract checks -------------------------------------------------------
#
# These are loud on purpose. Every one of them is a way for the pipeline to
# produce a plausible number from the wrong cells.

hr()
note("Contract checks")

# 1. is_baseline must agree with variant_index == 0.
bad <- cells[cells$is_baseline != (cells$variant_index == 0), ]
if (nrow(bad) > 0) {
  fail("is_baseline disagrees with variant_index on ", nrow(bad), " row(s)")
}

# 2. Exactly one baseline variant per sweep. Go pairs everything against
#    variants[0]; with two flagged, the sign of every difference below would be
#    unexplained rather than obviously wrong.
for (b in unique(cells$block)) {
  d <- cells[cells$block == b, ]
  bl <- unique(d$variant[d$is_baseline])
  if (length(bl) != 1) {
    fail("sweep ", b, " has ", length(bl), " baseline variants (want 1): ",
         paste(bl, collapse = " | "))
  }
}
note("  ok  exactly one baseline arm in each of ", length(unique(cells$block)), " sweep(s)")

# 3. The per-gameweek columns must be re-derivable from points and weeks. This
#    is the check the `weeks` column exists for: the harness divides before
#    anything downstream sees the number, and a wrong denominator silently
#    reweights the start points.
feas <- cells[!cells$infeasible, ]
if (nrow(feas) == 0) fail("every cell is flagged infeasible")
if (any(is.na(feas$weeks)) || any(feas$weeks <= 0)) {
  fail("a feasible cell has no usable `weeks` denominator")
}
checked <- character(0)
for (m in METRIC_COLS) {
  pc <- paste0(m, "_points")
  gc <- paste0(m, "_per_gw")
  if (is.null(feas[[pc]]) || all(is.na(feas[[pc]]))) next
  # (!) The total present without its per-gw twin used to fall through to a
  # vacuous comparison over zero rows -- and the metric was then APPENDED TO
  # `checked`, so the banner announced arithmetic that never ran. read_cells now
  # refuses a half-pair at load, and this is the same refusal one layer up, kept
  # because this script also reads sidecar means files that do not go through
  # read_cells.
  if (is.null(feas[[gc]])) {
    fail(m, "_points is present without ", m, "_per_gw — the re-derivation ",
         "check cannot run, and skipping it silently would print a false ok.")
  }
  ok <- !is.na(feas[[pc]]) & !is.na(feas[[gc]])
  want <- feas[[pc]][ok] / feas$weeks[ok]
  got <- feas[[gc]][ok]
  off <- which(abs(got - want) > opt_tol * pmax(1, abs(want)))
  if (length(off) > 0) {
    fail(m, "_per_gw does not equal ", m, "_points / weeks in ",
         length(off), " row(s); first: ", got[off[1]], " vs ", want[off[1]])
  }
  checked <- c(checked, m)
}
note("  ok  re-derives from points / weeks: ", paste(checked, collapse = ", "))

# 4. Infeasible cells are reported, never silently dropped. A variant that
#    cannot field a legal fifteen is a result about the variant.
inf <- cells[cells$infeasible, ]
if (nrow(inf) > 0) {
  note("  !!  ", nrow(inf), " infeasible cell(s) — excluded from inference, listed:")
  agg <- table(inf$block, inf$variant)
  for (i in seq_len(nrow(agg))) {
    for (j in seq_len(ncol(agg))) {
      if (agg[i, j] > 0) {
        note("        ", rownames(agg)[i], "  ", colnames(agg)[j], ": ",
             agg[i, j], " cell(s)")
      }
    }
  }
} else {
  note("  ok  no infeasible cells")
}

# 5. bank_up_to travels with the row for a reason. sweepBankLimit pins every
#    cell at 5 regardless of season, which is historically wrong for 2022-23 and
#    2023-24 (FPL banked 2 before 2024-25). Absolute season totals produced this
#    way are not comparable with figures measured under BankLimitFor; only the
#    paired differences are.
banks <- sort(unique(feas$bank_up_to))
note("  ..  bank_up_to = ", paste(banks, collapse = ", "),
     if (length(banks) == 1 && banks[1] == 5)
       "  (pinned modern rule; totals are not comparable across seasons)"
     else "")

# --- paired differences ----------------------------------------------------
#
# One difference per (season, start point): the same football, the same opening
# conditions, one setting changed. Per gameweek *played*, because a GW1 entry
# banks 38 gameweeks and a GW26 entry 13, and pooling raw totals weights the
# earliest regime roughly twice as heavily.

# `diffs_for` is in cells_common.R. This script always asks it for the scale the
# --scale option resolved, and every call below passes SCALE_SUFFIX explicitly
# rather than letting the shared function default to one.
diffs_for_scale <- function(d, metric, quiet = FALSE) {
  # min_cells 2: the floor below which sd() does not exist. It is passed rather
  # than defaulted because variance_components.R legitimately needs 4 — mom()
  # divides by S-1 and G-1 — and a shared default would silently give one of them
  # the other's floor.
  diffs_for(d, metric, SCALE_SUFFIX, min_cells = 2, quiet = quiet)
}

# --- the three standard errors ---------------------------------------------
#
# `have`, `has_cs`, `has_lmer`, `degenerate`, `se_cr`, `se_cr2` and
# `se_cr2_start` are in cells_common.R.

if (!has_cs) {
  note("")
  note("  !!  clubSandwich not installed — no CR2 cluster-robust SE.")
  note("      install.packages(c('clubSandwich'))")
}
if (!has_lmer) {
  note("  !!  lme4/lmerTest not installed — no mixed-model SE.")
  note("      install.packages(c('lme4','lmerTest'))")
}

# naive: every cell independent. This is what every t value currently written
# into AGENTS.md is, so it is reported for continuity, not because it is right —
# the six entry points inside a season replay the same football.
se_naive <- function(x) {
  n <- length(x)
  if (n < 2) return(list(se = NA, df = NA, t = NA, p = NA))
  s <- sd(x)
  if (!is.finite(s) || s == 0) {
    # Byte-identical arms are common and expected here: a transfer knob must
    # leave HOLD untouched, and that invariance is the check, not a degeneracy
    # to hide. t.test() errors on it.
    return(list(se = 0, df = n - 1, t = NA, p = 1))
  }
  se <- s / sqrt(n)
  tt <- mean(x) / se
  list(se = se, df = n - 1, t = tt, p = 2 * pt(-abs(tt), n - 1))
}

# CR2 — `degenerate`, `se_cr` and `se_cr2` are in cells_common.R: cluster-robust
# at the season level with the bias-reduced small-sample correction and
# Satterthwaite df. With four seasons the df is small and CR2 is what makes that
# honest rather than merely pessimistic — and the df it resolves is *reported*
# rather than asserted to be 3.

# `se_cr2_start` is the same estimator clustered on the ENTRY POINT rather than
# the season, and is also in cells_common.R.
#
# It exists because "trust the clustered figure when they disagree" is a rule
# written on the MinutesWeight precedent, where the season-clustered SE fell
# 0.348 -> 0.088 because the seasons genuinely agreed. A clustered SE can also
# fall because the clustering axis carries **no variance**, and then it is
# anticonservative rather than conservative — the same arithmetic, the opposite
# meaning. The anchored-chip result is the case: season carried 1% of the
# variance in those differences against start point's 10%, the season-clustered t
# read 2.39 and the start-clustered t read 1.00, and leave-one-season-out moved
# the season-clustered figure between 1.53 and 4.19.
#
# Emitting both makes the disagreement visible instead of a choice made silently
# by which axis the script happened to cluster on. Read it with `season_share`
# beside it, and note the caveat variance_components.R sets out: the same six
# entry gameweeks are replayed in every season on purpose, so a start-point main
# effect is *fixed* and cancels from a paired contrast. This is therefore a
# robustness check on the season clustering rather than a rival estimate to
# prefer on its own.

# The estimator that is the honest rival when the season axis carries nothing:
# start point treated as a FIXED block, no season effect, so the SE is
# `sqrt(MS_resid/(S*G))` on `(S-1)(G-1)` df.
#
# This is `variance_components.R`'s EST_FIXED and it is what the design argues
# for — the same six entry gameweeks are replayed in every season on purpose, so
# their main effect is fixed rather than sampled. Where sigma2_season is
# indistinguishable from zero, CR2-on-season is **targeting this same quantity**
# and estimating it on 3 degrees of freedom instead of 15. That is the whole
# defect: not that the clustered figure is biased, but that it swings. On
# MinutesHalfLife/HOLD it reads -4.80 where this reads -2.68, and -2.41 where
# this reads -0.88.
#
# It is not licensed when the season F test finds something. Read it with
# `share_season` and the season-mean signs beside it.
se_fixed <- function(d) {
  v <- season_share(d)
  if (is.null(v)) return(list(se = NA, df = NA, t = NA, p = NA))
  se <- sqrt(v$v_resid / (v$S * v$G))
  if (!is.finite(se) || se <= 0) return(list(se = 0, df = NA, t = NA, p = 1))
  df <- (v$S - 1) * (v$G - 1)
  tt <- mean(d$diff) / se
  list(se = se, df = df, t = tt, p = 2 * pt(-abs(tt), df))
}

# The share of the variance in these differences that sits between seasons.
#
# Method of moments on the balanced season-by-start table, which is the same
# arithmetic as variance_components.R's `mom()` and is duplicated here rather
# than sourced for one reason: this script must keep working when run on its own
# against a single cells file. It is deliberately the *share* only — the full
# decomposition, the chi-squared bound on sigma2_season and the minimum
# detectable effect all stay in variance_components.R, which is where anyone
# acting on this number should go next.
#
# Components can come out negative, which is the honest way for one to say it is
# indistinguishable from zero. The *share* clamps at zero, so a printed 0.0%
# cannot distinguish "+0.001" from "-1.40 on three degrees of freedom" — the raw
# `v_season` is therefore returned and printed beside it, which the first version
# of this claimed to do and did not. NULL on an unbalanced grid, because the
# arithmetic assumes balance.
#
# `n_dup` guards the other way `tapply` can lie here: `x[1]` silently keeps the
# first of any duplicated (season, start) pair, where the CR2 fit uses all of
# them. Harmless on one sweep and silently divergent the moment two runs of a
# block are pooled, which the cells format explicitly allows.
season_share <- function(d) {
  n_dup <- sum(duplicated(paste(d$season, d$start_gw)))
  if (n_dup > 0) return(NULL)
  tab <- tapply(d$diff, list(d$season, d$start_gw), function(x) x[1])
  if (is.null(dim(tab)) || any(is.na(tab))) return(NULL)
  S <- nrow(tab); G <- ncol(tab)
  if (S < 2 || G < 2) return(NULL)
  grand <- mean(tab)
  a <- rowMeans(tab) - grand
  b <- colMeans(tab) - grand
  e <- tab - outer(rowMeans(tab), colMeans(tab), "+") + grand
  ms_s <- G * sum(a^2) / (S - 1)
  ms_g <- S * sum(b^2) / (G - 1)
  ms_e <- sum(e^2) / ((S - 1) * (G - 1))
  v_s <- (ms_s - ms_e) / G
  v_g <- (ms_g - ms_e) / S
  tot <- max(v_s, 0) + max(v_g, 0) + ms_e
  if (!is.finite(tot) || tot <= 0) return(NULL)
  list(S = S, G = G,
       v_season = v_s, v_start = v_g, v_resid = ms_e,
       share_season = max(v_s, 0) / tot,
       share_start = max(v_g, 0) / tot,
       share_resid = ms_e / tot)
}

# Mixed model: a random intercept per season.
#
# Note this is `(1|season)` and NOT `(1|season/start_gw)`. There is exactly one
# observation per (season, start point), so a start-point random effect is
# perfectly confounded with the residual and lme4 cannot identify both — the
# nested form either fails to converge or silently splits one variance
# arbitrarily between two terms. `(1|season)` *is* the nested model here: the
# residual is the within-season, between-start level.
se_lmer <- function(d) {
  if (!has_lmer) return(list(se = NA, df = NA, t = NA, p = NA))
  if (length(unique(d$season)) < 3 || degenerate(d)) {
    return(list(se = NA, df = NA, t = NA, p = NA))
  }
  tryCatch({
    fit <- lmerTest::lmer(diff ~ 1 + (1 | season), data = d,
                          control = lme4::lmerControl(
                            check.conv.singular = "ignore"))
    co <- summary(fit)$coefficients
    list(se = co[1, "Std. Error"], df = co[1, "df"],
         t = co[1, "t value"], p = co[1, "Pr(>|t|)"])
  }, error = function(e) list(se = NA, df = NA, t = NA, p = NA))
}

# --- run -------------------------------------------------------------------

results <- list()
# Contrasts between two non-baseline arms, keyed the same way as `results`.
#
# The baseline-paired table cannot express "how much more is arm B worth than arm
# A", and for a decomposition that difference IS the deliverable: perfect lineups
# bounds what better team news could buy, and perfect-minutes-minus-perfect-lineups
# is the residual nobody could have bought even in principle. Reporting one number
# for such a pair invites reading all of it as headroom.
#
# It needs no new estimator and makes no new assumption. Both arms sit on the same
# cells against the same baseline, so the baseline cancels and B − A is a paired
# difference of exactly the kind everything else here already is; the same
# se_naive / se_cr2 / se_lmer run on it unchanged. It was previously described in
# the Go harness as a contrast that "carries no SE", which was a tooling gap stated
# as a statistical fact.
#
# Not folded into the Holm family above: that family is "alternatives against the
# shipped setting", and a contrast asks a different question. Its p is raw and says
# so.
contrasts <- list()

for (b in unique(cells$block)) {
  d_all <- cells[cells$block == b & !cells$infeasible, ]
  if (nrow(d_all) == 0) next
  baseline <- unique(d_all$variant[d_all$is_baseline])

  # Only the metrics this sweep actually measured. An ordinary sweep has policy,
  # hold and the two captaincy rungs with their marginals; the variance
  # decomposition instead has the three frozen-eleven layers and the four
  # marginals derived from them.
  candidates <- c(METRIC_COLS, names(ALL_MARGINALS))
  present <- candidates[vapply(candidates, function(m) {
    col <- d_all[[paste0(m, SCALE_SUFFIX)]]
    !is.null(col) && any(!is.na(col))
  }, logical(1))]

  for (metric in present) {
    armsets <- diffs_for_scale(d_all, metric)
    if (length(armsets) == 0) next

    rows <- list()
    for (v in names(armsets)) {
      d <- armsets[[v]]
      n <- nrow(d)
      m <- mean(d$diff)
      a <- se_naive(d$diff); b2 <- se_cr2(d); c2 <- se_lmer(d)
      s2 <- se_cr2_start(d); fx <- se_fixed(d)
      vc <- season_share(d)
      # The wild cluster bootstrap — Webb 6-point weights on the season, null
      # imposed. `wild_cluster_p_season` and `wcb_label` are in cells_common.R;
      # read the long note there before quoting one of these, in particular that
      # it is a robustness diagnostic and NOT a gate.
      wb <- wild_cluster_p_season(d)
      # How many seasons agree in sign with the pooled mean. A share near 100%
      # cannot distinguish "the seasons agree" from "one season is the whole
      # effect", and this can — it is the `pos` column the contrast table already
      # carries, at the level that matters for a season-clustered SE.
      sm <- tapply(d$diff, d$season, mean)
      rows[[v]] <- data.frame(
        block = b, metric = metric, baseline = baseline, variant = v,
        variant_index = d$variant_index[1], n = n, mean = m,
        scale = opt_scale,
        seasons = length(unique(d$season)),
        seasons_agreeing = sum(sign(sm) == sign(m) & sm != 0),
        se_naive = a$se, t_naive = a$t, p_naive = a$p,
        se_cr2 = b2$se, df_cr2 = b2$df, t_cr2 = b2$t, p_cr2 = b2$p,
        se_cr2_start = s2$se, df_cr2_start = s2$df, t_cr2_start = s2$t,
        p_cr2_start = s2$p,
        se_fixed = fx$se, df_fixed = fx$df, t_fixed = fx$t, p_fixed = fx$p,
        v_season = if (is.null(vc)) NA else vc$v_season,
        share_season = if (is.null(vc)) NA else vc$share_season,
        share_start = if (is.null(vc)) NA else vc$share_start,
        share_resid = if (is.null(vc)) NA else vc$share_resid,
        se_lmer = c2$se, df_lmer = c2$df, t_lmer = c2$t, p_lmer = c2$p,
        p_wild = wb$p, wild_seasons = wb$S,
        wild_seasons_movable = wb$S_eff, wild_floor = wb$floor,
        wild_inert = wb$inert, wild_why = wb$why, wild_label = wcb_label(wb),
        stringsAsFactors = FALSE
      )
    }
    tab <- do.call(rbind, rows)

    # Multiplicity, across the family of alternatives *within* this sweep and
    # metric. Holm rather than Bonferroni: it controls the family-wise error
    # rate without assuming independence, which matters because the arms share
    # every cell. Raw p is kept alongside — every verdict already in AGENTS.md
    # is a raw one, so the adjusted figures have to be additive rather than a
    # silent replacement.
    prim <- ifelse(is.na(tab$p_cr2), tab$p_naive, tab$p_cr2)
    tab$p_primary <- prim
    tab$p_holm <- p.adjust(prim, method = "holm")
    tab$p_bh <- p.adjust(prim, method = "BH")
    tab$family <- nrow(tab)

    results[[paste(b, metric)]] <- tab

    # Pairwise contrasts between the non-baseline arms, in variant-index order so
    # the sign of "B minus A" is predictable from the sweep's own arm ordering.
    if (length(armsets) >= 2) {
      idx <- order(vapply(armsets, function(a) a$variant_index[1], numeric(1)))
      names_ord <- names(armsets)[idx]
      crows <- list()
      for (i in seq_along(names_ord)) {
        for (j in seq_along(names_ord)) {
          if (j <= i) next
          lo <- armsets[[names_ord[i]]]
          hi <- armsets[[names_ord[j]]]
          k <- merge(lo[, c("season", "start_gw", "diff")],
                     hi[, c("season", "start_gw", "diff")],
                     by = c("season", "start_gw"), suffixes = c(".lo", ".hi"))
          k$diff <- k$diff.hi - k$diff.lo
          k <- k[!is.na(k$diff), ]
          if (nrow(k) < 2) next
          a <- se_naive(k$diff); b2 <- se_cr2(k); c2 <- se_lmer(k)
          # Contrasts get the bootstrap too, because that is where the sharpest
          # disagreements with the CR2 t sit — `BlendRateK=16 minus BlendRateK=12`
          # reads CR2 t 3.94 on HOLD while being positive in only 17 of 36 cells.
          # ⚠️ That particular contrast is already reported by
          # `concentration_screen.R`, so its p here is corroboration of a figure
          # the record has, not a new discovery.
          wb <- wild_cluster_p_season(k)
          crows[[length(crows) + 1]] <- data.frame(
            block = b, metric = metric,
            contrast = paste(names_ord[j], "minus", names_ord[i]),
            n = nrow(k), mean = mean(k$diff),
            se_naive = a$se, t_naive = a$t,
            se_cr2 = b2$se, df_cr2 = b2$df, t_cr2 = b2$t, p_cr2 = b2$p,
            se_lmer = c2$se, t_lmer = c2$t,
            p_wild = wb$p, wild_label = wcb_label(wb),
            wild_seasons_movable = wb$S_eff, wild_floor = wb$floor,
            wild_inert = wb$inert, wild_why = wb$why,
            positive = sum(k$diff > 0),
            stringsAsFactors = FALSE
          )
        }
      }
      if (length(crows) > 0) {
        contrasts[[paste(b, metric)]] <- do.call(rbind, crows)
      }
    }
  }
}

if (length(results) == 0) stop("no comparable arms found")
all_res <- do.call(rbind, results)

# --- report ----------------------------------------------------------------

# num formats a possibly-NA number to a fixed width, printing "." rather than a
# zero for a missing value. A zero where a statistic could not be computed is the
# kind of thing that gets read as a result.
METRIC_LABEL <- c(
  policy = "POLICY (the weekly transfer decision)",
  hold = "HOLD (the opening fifteen; scoring constants belong here)",
  frozen = "layer: frozen XI, no captain",
  frozen_captain = "layer: + captain, XI still frozen",
  weekly = "layer: + weekly XI & captaincy",
  m_captain = "marginal: captaincy alone",
  m_weekly_xi = "marginal: weekly XI repick alone",
  m_autosub = "marginal: autosubs alone",
  m_transfers = "marginal: transfers alone",
  hold_fixedcap = "instrument: HOLD, armband pinned to the day-one pick",
  hold_nocap = "instrument: HOLD with nobody doubled (not what FPL pays)",
  hold_xpoints = "instrument: HOLD on accumulated xPoints (read beside HOLD)",
  policy_xpoints = "instrument: POLICY on accumulated xPoints (read beside POLICY)",
  m_armband = "marginal: the armband's share of this effect",
  m_capweekly = "marginal: re-picking the captain weekly"
)
label_of <- function(m) if (is.na(METRIC_LABEL[m])) toupper(m) else METRIC_LABEL[[m]]

num <- function(x, dp = 3, w = 8) {
  if (length(x) == 0 || is.na(x)) return(formatC(".", width = w))
  formatC(x, format = "f", digits = dp, width = w)
}

# The wild cluster bootstrap block: the p, the number of clusters that can carry
# it, the smallest p those clusters could ever produce, and — for every arm with
# no p — the named reason.
#
# The floor and the reason are the load-bearing halves. An arm with one movable
# season returns p = 1.0000 by construction, and a reader who sees that without
# `movable` beside it reads a resounding null where the truth is that the
# measurement cannot be made. That is the byte-identical trap in a new costume.
wcb_report <- function(tab, ord) {
  live <- !is.na(tab$p_wild)
  note("")
  note("  wild cluster bootstrap — Webb 6-point weights on SEASON, null imposed,")
  note("  enumerated exactly over all 6^movable draws (no sampling, so no seed):")
  if (any(live)) {
    cat(sprintf("    %-26s %9s | %8s %8s %10s\n",
                "variant", "p wild", "seasons", "movable", "floor"))
    for (i in ord) {
      r <- tab[i, ]
      if (is.na(r$p_wild)) next
      cat(sprintf("    %-26s %9s | %8d %8d %10s\n",
                  substr(r$variant, 1, 26), num(r$p_wild, 4, 9),
                  r$wild_seasons, r$wild_seasons_movable,
                  num(r$wild_floor, 5, 10)))
    }
  }
  for (i in ord) {
    r <- tab[i, ]
    if (is.na(r$wild_inert)) next
    note("    ", substr(r$variant, 1, 26), ": ", toupper(r$wild_inert), " — ",
         r$wild_why)
  }
  if (!any(live)) {
    note("    No arm in this table can be bootstrapped. That is a statement about")
    note("    the arms, not a null: see each reason above.")
  }
  # ⚠️ Printed even when every arm is inert. An earlier draft returned early in
  # that case, so the contrasts note below pointed at "the block above" for an
  # explanation that was not on the page — and the all-inert table is exactly
  # where a reader most needs to be told what `identical` means.
  note("    `movable` is the number of seasons carrying a non-zero difference. A")
  note("    season of exact zeros is unchanged by any weight, so it sets no part")
  note("    of the reference distribution and the FLOOR IS SET BY `movable`, not")
  note("    by `seasons`. The smallest attainable p is 6/6^movable — SIX, because")
  note("    the CR2 t is homogeneous in y, so every draw giving all movable")
  note("    seasons the same weight ties |t|, and Webb has six weight values:")
  note("      movable  2 -> 0.1667   3 -> 0.02778   4 -> 0.00463   6 -> 0.000129")
  note("    At four seasons the largest effect in this record — the perfect")
  note("    armband, CR2 t 20.4 — reaches only 0.0093, so read this as a SEASON-")
  note("    AGREEMENT statistic with a few usable gradations, not a magnitude.")
  note("    Three inert words, because they are three different facts: `identical`")
  note("    is the invariance check passing, `pinned` is one movable season and")
  note("    measures nothing, `underpowered` is two and cannot reach 0.05.")
  note("    It is a function of the SEASON TOTALS ALONE, hence exactly invariant")
  note("    to how a season's effect is spread across its cells. It therefore")
  note("    cannot see cell-level kurtosis at all, and cannot tell one big cell")
  note("    from six equal ones — that is what `pos` is for. Read them together.")
  note("    ASYMMETRIC BY RULE: it may WITHDRAW support from a CR2 rejection, and")
  note("    it may NOT grant one. Do not Holm-adjust it, and derive no MDE or")
  note("    points-per-season from it — a bootstrap p carries no SE.")
  invisible(NULL)
}

for (key in names(results)) {
  tab <- results[[key]]
  hr()
  note("Sweep: ", tab$block[1])
  note("Metric: ", label_of(tab$metric[1]))
  note("Paired against: ", tab$baseline[1])
  if (grepl("^m_", tab$metric[1])) {
    note("This marginal is differenced per cell, so its SE needs no assumption")
    note("about independence between adjacent layers.")
  }
  note("mean is ", SCALE_UNITS, ".")
  note("")
  cat(sprintf("%-26s %8s %5s | %8s %7s | %8s %6s %7s | %8s %8s %8s\n",
              "variant", "mean", "n", "SE nai", "t nai", "SE CR2", "df", "t CR2",
              "p raw", "p holm", "p wild"))
  ord <- order(tab$variant_index)
  for (i in ord) {
    r <- tab[i, ]
    cat(sprintf("%-26s %s %5d | %s %s | %s %s %s | %s %s %s\n",
                substr(r$variant, 1, 26), num(r$mean, 3, 8), r$n,
                num(r$se_naive, 3, 8), num(r$t_naive, 2, 7),
                num(r$se_cr2, 3, 8), num(r$df_cr2, 1, 6), num(r$t_cr2, 2, 7),
                num(r$p_primary, 4, 8), num(r$p_holm, 4, 8),
                formatC(r$wild_label, width = 8)))
  }

  # The wild cluster bootstrap, and the floor it is unreadable without.
  #
  # `p wild` sits at the END of the row, after `p holm`, so every line this table
  # printed before the bootstrap existed is still a strict prefix of the line it
  # prints now. It is a DIFFERENT estimator from `p raw` beside it — see the block
  # below, which carries the floor without which the number is unreadable.
  wcb_report(tab, ord)

  # The clustering axis, checked rather than chosen.
  #
  # A season-clustered SE falls either because the seasons agree or because the
  # season axis carries no variance, and the arithmetic is identical. Where it
  # carries nothing, CR2 is targeting the same quantity as the fixed-start
  # estimator and spending 3 degrees of freedom on it instead of 15 — so it
  # swings. Both tails are printed because the share alone catches only one:
  # a share near 100% can equally mean one season IS the effect, which is what
  # `agree` is for.
  if (any(!is.na(tab$share_season)) || any(!is.na(tab$t_fixed))) {
    note("")
    note("  clustering axis check — is the season clustering doing anything?")
    cat(sprintf("    %-24s %7s %7s %7s | %8s %6s | %7s %7s %7s %6s\n",
                "variant", "t seas", "t fixd", "t entry", "v_season", "agree",
                "%seas", "%entry", "%resid", "ratio"))
    for (i in ord) {
      r <- tab[i, ]
      pc <- function(x) if (is.na(x)) formatC(".", width = 7) else
        formatC(100 * x, format = "f", digits = 1, width = 7)
      # How far the clustered SE has drifted from what it would be if the season
      # component were exactly zero. Near 1 means the clustering is inert; well
      # below 1 means it is buying its significance from a low draw on 3 df.
      ratio <- if (is.na(r$se_cr2) || is.na(r$se_fixed) || r$se_fixed == 0) NA else
        r$se_cr2 / r$se_fixed
      cat(sprintf("    %-24s %s %s %s | %s %s | %s %s %s %s\n",
                  substr(r$variant, 1, 24),
                  num(r$t_cr2, 2, 7), num(r$t_fixed, 2, 7), num(r$t_cr2_start, 2, 7),
                  num(r$v_season, 3, 8),
                  sprintf("%3d/%-2d", r$seasons_agreeing, r$seasons),
                  pc(r$share_season), pc(r$share_start), pc(r$share_resid),
                  num(ratio, 2, 6)))
    }
    note("    t seas  = CR2 clustered on season, df ~", num(tab$df_cr2[ord[1]], 1, 1),
         " — the record's primary column")
    note("    t fixd  = start point as a FIXED block, no season effect, df ",
         num(tab$df_fixed[ord[1]], 0, 1),
         " — the SAME estimand as t seas when v_season is ~0, on five times the df")
    note("    t entry = CR2 clustered on entry point. A ROBUSTNESS CHECK, not a rival:")
    note("              the same six entry gameweeks are replayed every season on")
    note("              purpose, so their main effect is fixed and cancels from a")
    note("              paired contrast. It can print absurd values on six tight")
    note("              clusters; read it only as 'does the axis matter'.")
    note("    Prefer t fixd where %seas is near zero, t seas where it is not — and")
    note("    treat a high %seas with `agree` well below the season count as one")
    note("    season carrying the whole effect rather than as seasons agreeing.")
    note("    v_season is the RAW component and may be negative, which is how a")
    note("    method-of-moments estimate says 'indistinguishable from zero' on 3 df;")
    note("    the %seas column clamps it and cannot show that.")
    note("    Run variance_components.R on the same cells before acting on any of it.")
  }
  if (has_lmer && any(!is.na(tab$se_lmer))) {
    note("")
    note("  mixed model, diff ~ 1 + (1|season) — residual is the within-season level:")
    for (i in ord) {
      r <- tab[i, ]
      cat(sprintf("    %-26s SE %s  df %s  t %s  p %s\n",
                  substr(r$variant, 1, 26), num(r$se_lmer, 3, 8),
                  num(r$df_lmer, 1, 6), num(r$t_lmer, 2, 7),
                  num(r$p_lmer, 4, 8)))
    }
  }
  if (all(is.na(tab$t_naive))) {
    note("")
    note("  every difference is exactly zero: this arm is byte-identical to the")
    note("  baseline. For a transfer knob measured on HOLD that is the invariance")
    note("  check passing, not a failed measurement.")
  }
  note("")
  note("  Family size for the Holm adjustment: ", tab$family[1],
       " alternative(s) in this sweep and metric.")
  note("  A raw p that clears 0.05 and an adjusted p that does not is exactly the")
  note("  case the old |t| >= 2 rule counted as 'confirmed'.")

  ct <- contrasts[[key]]
  if (!is.null(ct)) {
    note("")
    note("  Contrasts BETWEEN arms — the baseline cancels, so each is an ordinary")
    note("  paired difference. Read with the same t_crit as everything above, and")
    note("  note the p is raw: this is a different question from the Holm family.")
    # `p wild` is APPENDED after `pos` rather than slotted in beside the other p
    # columns, so that every line this table printed before the bootstrap existed
    # is still a strict prefix of the line it prints now. Inserting it mid-row
    # would move `pos` two columns right — no value would change, but a reader
    # diffing two vintages of this transcript would see every contrast row as
    # modified, which is the noise that hides a real movement.
    cat(sprintf("    %-44s %8s %5s | %8s %7s | %8s %7s | %5s %8s\n",
                "contrast", "mean", "n", "SE nai", "t nai", "SE CR2", "t CR2",
                "pos", "p wild"))
    for (i in seq_len(nrow(ct))) {
      r <- ct[i, ]
      cat(sprintf("    %-44s %s %5d | %s %s | %s %s | %5d %s\n",
                  substr(r$contrast, 1, 44), num(r$mean, 3, 8), r$n,
                  num(r$se_naive, 3, 8), num(r$t_naive, 2, 7),
                  num(r$se_cr2, 3, 8), num(r$t_cr2, 2, 7), r$positive,
                  formatC(r$wild_label, width = 8)))
    }
    for (i in seq_len(nrow(ct))) {
      r <- ct[i, ]
      if (is.na(r$wild_inert)) next
      note("    ", substr(r$contrast, 1, 44), ": ", toupper(r$wild_inert), " — ",
           r$wild_why)
    }
    note("  `pos` is how many of the cells the contrast is positive in. A mean that")
    note("  rests on a handful of cells is a one-cell result whatever its t says.")
    note("  `p wild` is the Webb wild cluster bootstrap described in the block")
    note("  above — a different estimator from the CR2 t beside it, and a")
    note("  robustness diagnostic rather than a gate. It asks whether the contrast")
    note("  survives reweighting whole SEASONS, so read it with `pos`, which is")
    note("  about cells: the two answer different questions and a contrast can")
    note("  fail one and pass the other.")
  }
}

# --- noise by information regime -------------------------------------------
#
# Replaces reportRegimeNoise, which was the third hand-rolled variance estimator
# in the Go package and was dead code besides.
#
# The blend weight is n/(n+k), so entry gameweek *is* the information regime: a
# GW1 entrant decides on the prior alone, a late one almost entirely on this
# season. The confound is that a late entry plays fewer gameweeks, so its
# per-gameweek figure averages less football and is noisier for purely arithmetic
# reasons. The null is therefore not "equal noise" but noise scaling as
# 1/sqrt(weeks), and the last column states it: above the prediction means the
# regime is noisier than its shorter horizon explains.

# `regime_of` is in cells_common.R, so this table and schedule_screen.R's
# early/late columns cannot drift apart on where the boundaries sit.

for (metric in c("policy", "hold")) {
  sub <- all_res[all_res$metric == metric, ]
  if (nrow(sub) == 0) next
  # Pool every comparison's cells, since one comparison per regime is too few
  # cells for a variance estimate worth reading.
  pooled <- list()
  for (b in unique(sub$block)) {
    d_all <- cells[cells$block == b & !cells$infeasible, ]
    for (arm in diffs_for_scale(d_all, metric, quiet = TRUE)) {
      arm$regime <- regime_of(arm$start_gw)
      arm$weeks_mean <- 38 - arm$start_gw + 1
      pooled[[length(pooled) + 1]] <- arm
    }
  }
  if (length(pooled) == 0) next
  p <- do.call(rbind, pooled)
  p <- p[!is.na(p$regime), ]
  if (nrow(p) == 0) next

  # Every difference exactly zero means the ratios are 0/0. That is the
  # invariance check passing, not a regime with no noise, so say so rather than
  # printing a column of NaN.
  if (isTRUE(all.equal(sd(p$diff), 0, tolerance = 0)) || sd(p$diff) == 0) {
    hr()
    note("Noise by information regime — ", toupper(metric),
         ": every difference is exactly zero, so there is no noise to attribute.")
    next
  }

  hr()
  note("Noise by information regime — ", toupper(metric),
       ", pooled over every comparison")
  # The `if length` column is the 1/sqrt(weeks) null, and it is derived above
  # from the per-gameweek figure averaging less football on a shorter path. It
  # does NOT carry over to totals: a rate-shaped total has SD proportional to
  # sqrt(weeks), which inverts the prediction, and an event-count total is
  # roughly flat in path length, which flattens it. Relabelling the header while
  # leaving the column would print a fabricated benchmark, so it is suppressed
  # rather than converted — there is no single right null on that scale, since
  # which one applies is the very thing --scale is asking the reader to decide.
  show_null <- opt_scale == "per_gw"
  if (show_null) {
    cat(sprintf("%-20s %7s %10s %9s %11s %7s\n",
                "regime", "weeks", "SE per gw", "vs early", "if length", "cells"))
  } else {
    note("  (the 1/sqrt(weeks) null is a per-gameweek statement and is omitted here)")
    cat(sprintf("%-20s %7s %10s %9s %7s\n",
                "regime", "weeks", paste0("SE ", opt_scale), "vs early", "cells"))
  }
  base_se <- NA; base_w <- NA
  for (rg in c("early (prior-led)", "middle", "late (season-led)")) {
    q <- p[p$regime == rg, ]
    if (nrow(q) < 3) next
    # SE within each comparison, then averaged — the same quantity the Go
    # version reported, so the recorded finding stays comparable.
    ses <- tapply(q$diff, q$variant, function(x) {
      if (length(x) < 2) return(NA)
      sd(x) / sqrt(length(x))
    })
    mse <- mean(ses, na.rm = TRUE)
    w <- mean(q$weeks_mean)
    if (is.na(base_se)) { base_se <- mse; base_w <- w }
    if (show_null) {
      cat(sprintf("%-20s %7.0f %10.3f %9.2f %11.2f %7d\n",
                  rg, w, mse, mse / base_se, sqrt(base_w) / sqrt(w), nrow(q)))
    } else {
      cat(sprintf("%-20s %7.0f %10.3f %9.2f %7d\n",
                  rg, w, mse, mse / base_se, nrow(q)))
    }
  }
}

# --- reproduce Go's means --------------------------------------------------
#
# The mean is the one quantity computed in both Go and R on purpose, and this is
# what makes that defensible: an unchecked duplicate is the bug class this
# project has shipped twice, a checked one is a pipeline test. If these disagree
# the CSV, the join, or the normalisation is wrong, and nothing below it can be
# trusted.

hr()
note("Reproducing Go's paired means")
# Only on the per-gameweek scale. Go writes `mean_per_gw` and nothing else, so on
# `--scale=per_path` there is no Go figure for these means to be checked against
# — and comparing a total to a rate would fail every row for the wrong reason.
# Said out loud rather than skipped quietly: this is the pipeline's only
# end-to-end test, and a run without it is a run with one fewer guard.
if (opt_scale != "per_gw") {
  note("  ..  skipped: --scale=", opt_scale, " and Go writes only mean_per_gw.")
  note("      Re-run without --scale to exercise the end-to-end check; the")
  note("      difference between the two scales is the divisor, not the cells.")
}
# This must reproduce Go's rule *exactly*: strings.TrimSuffix(path, ".csv") plus
# ".means.csv". `sub("\\.csv$", ".means.csv", p)` is not that rule — for a path
# with no extension it returns the path unchanged, which is the **cells file
# itself**, and `file.exists` is then true. The consequence is nastier than a
# crash: every table above has already printed, and the reproduction check — the
# pipeline's only end-to-end test, and the entire justification for computing the
# mean twice — silently does not run.
means_path_for <- function(p) paste0(sub("\\.csv$", "", p), ".means.csv")
means_paths <- unique(means_path_for(paths))
if (any(means_paths %in% paths)) {
  fail("the derived means path is one of the cells files (",
       paste(intersect(means_paths, paths), collapse = ", "),
       ") — this build's derivation disagrees with Go's")
}
means_paths <- means_paths[file.exists(means_paths)]
# Emptied on any other scale, so the branch below reports the skip once and does
# not then try to read a file it has just declared irrelevant.
if (opt_scale != "per_gw") means_paths <- character(0)
gm <- NULL
if (length(means_paths) == 0) {
  if (opt_scale == "per_gw") {
    note("  !!  no .means.csv beside the cells file, so Go's means cannot be checked.")
    note("      This is the pipeline's only end-to-end test. Re-run the sweep with a")
    note("      current build, which writes it automatically.")
  }
} else {
  gm <- do.call(rbind, lapply(means_paths, read_sidecar))
  # Validated before use, so a file that is not a means file says so instead of
  # dying inside a data.frame assignment with a message that names nothing.
  need <- c("sweep", "run_id", "metric", "variant", "mean_per_gw", "n_cells")
  if (!all(need %in% names(gm))) {
    fail(paste(means_paths, collapse = ", "), " is not a means file: missing ",
         paste(setdiff(need, names(gm)), collapse = ", "))
  }
  # A header-only means file is the normal state of a sweep that is still running
  # or was killed: Go creates both files when the sink opens and writes the means
  # only after the last arm finishes, while cells are flushed per row so a partial
  # run keeps its work. Failing here would make the script unusable on exactly the
  # case the per-row flush exists for — and this fired for real on a live sweep.
  if (nrow(gm) == 0) {
    note("  ..  the means file is empty, so Go has not finished a sweep yet.")
    note("      Cells are flushed per row and means only at the end, so this is a")
    note("      partial or killed run. Everything above is computed from the cells")
    note("      that exist; the cross-check against Go resumes once a sweep completes.")
    gm <- NULL
  }
}
if (!is.null(gm)) {
  gm$block <- paste(gm$run_id, gm$sweep, sep = " / ")
  gm$mean_per_gw <- suppressWarnings(as.numeric(gm$mean_per_gw))
  gm$n_cells <- suppressWarnings(as.numeric(gm$n_cells))

  j <- merge(
    all_res[, c("block", "metric", "variant", "mean", "n")],
    gm[, c("block", "metric", "variant", "mean_per_gw", "n_cells")],
    by = c("block", "metric", "variant")
  )
  if (nrow(j) == 0) {
    fail("no Go mean matched an R mean — the means file and the cells file do ",
         "not describe the same sweeps")
  }
  j$d_mean <- abs(j$mean - j$mean_per_gw)
  j$d_n <- abs(j$n - j$n_cells)
  bad <- j[j$d_mean > opt_tol * pmax(1, abs(j$mean_per_gw)) | j$d_n > 0, ]
  if (nrow(bad) > 0) {
    for (i in seq_len(nrow(bad))) {
      note("  MISMATCH ", bad$block[i], " ", bad$metric[i], " ", bad$variant[i],
           ": R ", bad$mean[i], " (n=", bad$n[i], ") vs Go ",
           bad$mean_per_gw[i], " (n=", bad$n_cells[i], ")")
    }
    fail(nrow(bad), " of ", nrow(j), " comparisons disagree with Go. Nothing ",
         "above this line can be trusted until that is explained.")
  }
  note("  ok  all ", nrow(j), " comparison(s) reproduce Go's mean and cell count",
       " (max abs diff ", format(max(j$d_mean), scientific = TRUE, digits = 2), ")")
  key_r <- paste(all_res$block, all_res$metric, all_res$variant)
  key_g <- paste(gm$block, gm$metric, gm$variant)
  only_r <- setdiff(key_r, key_g)
  only_g <- setdiff(key_g, key_r)
  if (length(only_r) > 0) {
    # Not an error. R derives a metric wherever the columns allow it, and the
    # emitting test may not have written a mean for every one — the layer
    # marginals are the usual case. It is listed so an *unexpected* gap is
    # visible rather than absorbed into a count.
    note("  ..  ", length(only_r), " comparison(s) computed here with no Go mean ",
         "to check against:")
    for (k in utils::head(unique(sub("^\\S+ \\S+ ", "", only_r)), 12)) {
      note("        ", k)
    }
  }
  if (length(only_g) > 0) {
    # A hard failure, unlike the other direction. Go writing a mean for a
    # comparison the cells cannot support means the two files describe different
    # runs — the same class as the two checks above, which both stop.
    for (k in utils::head(only_g, 12)) note("  MISSING  ", k)
    fail(length(only_g), " Go mean(s) have no matching comparison in the cells. ",
         "The two files describe different runs; nothing above can be trusted.")
  }
}

# --- plots -----------------------------------------------------------------
#
# The project has none, and several of its conclusions are *shape* claims —
# "monotone from 3", "a plateau with a cliff", "non-monotone, so the noise floor
# is about +/-20". A shape claim should be looked at.
#
# Design notes: one measure per panel and one y-axis, a single hue for the
# estimate, recessive grid, direct labels rather than a legend where a single
# series is plotted (the title names it). The two hues in the per-cell plot are
# the documented diverging pair (blue/red) used for the one thing it is for —
# polarity — with position carrying the same information, so colour is never the
# only encoding.

if (opt_plots) {
  hr()
  if (!have("ggplot2")) {
    note("ggplot2 not installed — skipping plots. install.packages('ggplot2')")
  } else {
    library(ggplot2)
    dir.create(opt_out, showWarnings = FALSE, recursive = TRUE)

    INK <- "#0b0b0b"; INK2 <- "#52514e"; GRID <- "#e6e5e1"
    BLUE <- "#2a78d6"; RED <- "#e34948"

    theme_sweep <- theme_minimal(base_size = 11) +
      theme(
        panel.grid.minor = element_blank(),
        panel.grid.major = element_line(colour = GRID, linewidth = 0.3),
        axis.title = element_text(colour = INK2),
        axis.text = element_text(colour = INK2),
        plot.title = element_text(colour = INK, face = "bold"),
        plot.subtitle = element_text(colour = INK2),
        plot.caption = element_text(colour = INK2, hjust = 0),
        strip.text = element_text(colour = INK)
      )

    safe <- function(s) gsub("[^A-Za-z0-9._-]+", "_", s)

    # A numeric x where the variant labels carry one — "min_gain=0.40 (ships)"
    # is a point on a ladder and should be plotted as one. Where they do not, or
    # where two arms would land on the same x, fall back to the sweep's own
    # ordering, because a fabricated axis is worse than a categorical one.
    numeric_x <- function(v) {
      pat <- "-?[0-9]+(\\.[0-9]+)?"
      m <- regexpr(pat, v)
      if (any(m < 0)) return(NULL)
      # A label carrying more than one number is a *tuple*, not a point on a
      # ladder — the bonus schedule's "0.5 / 1.5" or a bench-slot shape. Taking
      # the first number would plot those on an axis that means nothing, and the
      # whole purpose of this panel is to make a shape claim inspectable, so a
      # fabricated axis is worse than a categorical one.
      if (any(vapply(gregexpr(pat, v), function(g) sum(g > 0), numeric(1)) > 1)) {
        return(NULL)
      }
      x <- suppressWarnings(as.numeric(regmatches(v, m)))
      if (length(x) != length(v) || any(is.na(x)) || anyDuplicated(x) > 0) {
        return(NULL)
      }
      x
    }

    for (key in names(results)) {
      tab <- results[[key]]
      # The baseline is the reference and sits at exactly zero by construction,
      # with no interval. Plotting it makes the ladder readable.
      shape <- rbind(
        data.frame(variant = tab$baseline[1], variant_index = 0, mean = 0,
                   lo = NA, hi = NA, is_base = TRUE, stringsAsFactors = FALSE),
        data.frame(variant = tab$variant, variant_index = tab$variant_index,
                   mean = tab$mean,
                   lo = tab$mean - qt(0.975, ifelse(is.na(tab$df_cr2),
                                                    tab$n - 1, tab$df_cr2)) *
                     ifelse(is.na(tab$se_cr2), tab$se_naive, tab$se_cr2),
                   hi = tab$mean + qt(0.975, ifelse(is.na(tab$df_cr2),
                                                    tab$n - 1, tab$df_cr2)) *
                     ifelse(is.na(tab$se_cr2), tab$se_naive, tab$se_cr2),
                   is_base = FALSE, stringsAsFactors = FALSE)
      )
      shape <- shape[order(shape$variant_index), ]
      nx <- numeric_x(shape$variant)
      if (is.null(nx)) {
        shape$x <- seq_len(nrow(shape))
        xscale <- scale_x_continuous(breaks = shape$x, labels = shape$variant)
        xlab_ <- "variant"
      } else {
        shape$x <- nx
        xscale <- scale_x_continuous(breaks = nx)
        xlab_ <- "swept value"
      }

      which_se <- if (any(!is.na(tab$se_cr2))) "CR2 cluster-robust, Satterthwaite df" else "naive"
      p1 <- ggplot(shape, aes(x = x, y = mean)) +
        geom_hline(yintercept = 0, colour = INK2, linetype = "dashed",
                   linewidth = 0.3) +
        geom_line(colour = BLUE, linewidth = 0.5, alpha = 0.5) +
        geom_errorbar(aes(ymin = lo, ymax = hi), width = 0, colour = BLUE,
                      linewidth = 0.7, na.rm = TRUE) +
        geom_point(aes(shape = is_base), colour = BLUE, size = 2.6,
                   fill = "white", stroke = 0.8) +
        scale_shape_manual(values = c(`FALSE` = 16, `TRUE` = 21), guide = "none") +
        xscale +
        labs(
          title = paste0(toupper(tab$metric[1]), ": ", tab$block[1]),
          subtitle = paste0("paired difference against ", tab$baseline[1],
                            "; 95% CI (", which_se, ")"),
          x = xlab_, y = paste0("points ", sub("_", " ", opt_scale)),
          caption = paste0(
            "Hollow point is the baseline, fixed at zero by construction.\n",
            "Read the shape, not one point: a plateau with a cliff or a monotone run is ",
            "evidence; a single argmax is not.")
        ) + theme_sweep
      f1 <- file.path(opt_out, paste0(safe(paste0(tab$block[1], "-", tab$metric[1])), scale_tag,
                                      "-shape.png"))
      ggsave(f1, p1, width = 8, height = 4.6, dpi = 150, bg = "white")
      note("wrote ", f1)

      # Per-cell differences. The question this answers is whether the cells
      # agree or cancel — twelve cells at +40 is a real effect, +900 and -860 is
      # not, and a mean cannot tell them apart.
      armsets <- diffs_for_scale(
        cells[cells$block == tab$block[1] & !cells$infeasible, ], tab$metric[1],
        quiet = TRUE)
      per <- if (length(armsets) > 0) do.call(rbind, armsets) else NULL
      if (!is.null(per) && nrow(per) > 0) {
        per$sign <- ifelse(per$diff >= 0, "better", "worse")
        p2 <- ggplot(per, aes(x = factor(start_gw), y = diff, colour = sign)) +
          geom_hline(yintercept = 0, colour = INK2, linetype = "dashed",
                     linewidth = 0.3) +
          geom_point(size = 2.2) +
          scale_colour_manual(values = c(better = BLUE, worse = RED), name = NULL) +
          facet_grid(season ~ variant) +
          labs(
            title = paste0(toupper(tab$metric[1]), " per cell: ", tab$block[1]),
            subtitle = "each point is one (season, entry gameweek) paired difference",
            x = "entry gameweek", y = paste0("points ", sub("_", " ", opt_scale)),
            caption = paste0(
              "Cells that agree in sign across seasons and entry points are ",
              "evidence; cells that cancel are not.\nPosition already carries the ",
              "sign, so colour is redundant encoding rather than the only cue.")
          ) + theme_sweep + theme(legend.position = "top")
        f2 <- file.path(opt_out, paste0(safe(paste0(tab$block[1], "-", tab$metric[1])), scale_tag,
                                        "-cells.png"))
        ggsave(f2, p2, width = 10, height = 6, dpi = 150, bg = "white")
        note("wrote ", f2)
      }
    }
  }
}

# --- machine-readable output ----------------------------------------------

dir.create(opt_out, showWarnings = FALSE, recursive = TRUE)
out_csv <- file.path(opt_out, paste0("inference", scale_tag, ".csv"))
write.csv(all_res, out_csv, row.names = FALSE)
hr()
note("wrote ", out_csv)
note("")
note("Reading this: prefer the CR2 column, and read its df rather than assuming")
note("one. Treat a raw p under 0.05 whose Holm-adjusted p is not as the case the")
note("old |t| >= 2 rule mislabelled 'confirmed'. And weigh the shape plot above")
note("any single row: monotonicity, a plateau with a cliff, or the held-out")
note("season agreeing is what this project accepts as evidence.")
note("")
note("`p_wild` is a SECOND estimator, not a correction to the first: it swaps an")
note("unverifiable normality assumption on the season means for a symmetry one.")
note("It is a function of the SEASON TOTALS ALONE, so it says nothing whatever")
note("about the shape of the cells inside a season. Read it with")
note("`wild_seasons_movable` and `wild_floor` — both are populated on inert rows")
note("too, so `wild_floor > 0.05` is a working filter for 'could not be measured'")
note("and `wild_inert` names which of the three reasons applies. ASYMMETRIC BY")
note("RULE: it may withdraw support from a CR2 rejection, never grant one. Do not")
note("Holm-adjust it, and derive no MDE from it.")
