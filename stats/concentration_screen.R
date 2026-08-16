#!/usr/bin/env Rscript
#
# Which banked results are a handful of cells wearing a mean?
#
#   Rscript stats/concentration_screen.R --committed
#   Rscript stats/concentration_screen.R [--metric=<col>] <cells.csv> [<cells.csv> ...]
#
# # Why this exists
#
# On HOLD, `BlendRateK=16 minus BlendRateK=12` reads +23.3 points a season at a
# season-clustered t of 3.94, above this grid's t_crit of 2.571. It is not a
# result: it is positive in only 17 of 36 cells, and dropping its three largest
# cells takes it to +0.2. Three cells of thirty-six carry 99% of it.
#
# (!) A first version of this header blamed leave-one-season-out for missing it,
# and that was wrong twice. LOSO was never a concentration test -- the record
# already says "a sign count cannot see magnitude concentration" -- so calling a
# known limitation a discovery dressed it up. And the mechanism is not the drop
# rule, it is the TAILS: pooled excess kurtosis over 13 banked HOLD arms is
# +3.46, individual arms to +9.3, with up to 17 of 36 cells at |d| < 0.05. The
# cells are spike-and-slab, which is this record's "the noise is sensitivity,
# not randomness" made quantitative -- a squad flip either happens or it does
# not.
#
# So the small clustered SE (0.156 against a naive 0.419) is a chi-square fluke
# on five degrees of freedom, SELECTED as the largest of six pairwise contrasts.
# A season-level wild cluster bootstrap over all 64 sign patterns gives
# p = 0.0625, against a nominal 0.011. It does not clear 0.05.
#
# (!) The general consequence outranks this screen: every t in this record is
# computed on 36 heavy-tailed cells and the CLT is slow here, so |t| >= t_crit
# wants a permutation or wild-cluster check beside it.
#
# # What it screens
#
#   pos/n     how many cells share the mean's sign. Near half, with a large
#             mean, is the signature. sweep_inference.R already prints this
#   top-3     the mean recomputed with the three largest |cell| dropped. If
#             almost nothing survives, the mean was those cells
#
# (!) A concentrated mean is NOT evidence of an artefact, and reading it that way
# would be a worse error than the one this screen catches. A setting whose
# mechanism only fires sometimes -- a wildcard landing better, a chip week, a
# squad flip that avoids one bad buy -- OUGHT to be lumpy. A flag says only "this
# mean is a few events". The response is to find the right denominator and name a
# mechanism that predicts those cells, which is the move the chip work already
# makes with "the denominator is the cells that played the chip, not the grid".
#
# # (!) `seas` is NOT a discriminator, and the argument first written on it is withdrawn
#
# The column counts how many distinct seasons the three carriers sit in. The
# first version read seas=3 as evidence of a real intermittent mechanism and
# seas=1 as one event counted twice. The second half is right; the first is
# backwards. Drawing three carriers at random from a 6x6 grid puts all three in
# distinct seasons with P = 0.605 -- seas=3 is the MODAL OUTCOME OF PURE NOISE.
# Only seas=1 carries information (P ~ 0.015).
#
# Worse, carrier selection is length-biased by construction: the top-3 carriers
# land at entry gameweeks 26 and 21 -- the two shortest cells, 13 and 18
# gameweeks -- 24 times in 39, because a per-gameweek difference over 13 weeks
# carries ~1.7x the sd of one over 38. This screen ranks on an unweighted
# per-gameweek difference while the cells differ threefold in precision, and so
# does the record's headline estimator. The column is kept for seas=1 and for
# that warning; do not read seas=3 as anything.
#
# # It screens EVERY pairwise contrast
#
# Not only arm-against-baseline. The first version screened only against the
# baseline and therefore could not compute the contrast in its own opening
# paragraph -- the exact defect recorded against schedule_screen.R, reproduced
# inside the file written to complain about it.
#
# # And on any banked metric, not only HOLD -- `--metric=<col>`
#
# The default is `hold_per_gw`, because a scoring constant belongs on HOLD and
# almost everything in the bank is one. It was the only option until 2026-08-15,
# and that made an entire family INVISIBLE to this screen rather than merely
# unscreened: a transfer knob is confined to `decide()`, which HOLD never calls,
# so every oracle-on-transfers arm is byte-identical on HOLD BY THEOREM. The
# screen would drop all of them at the `all(diff == 0)` line and exit
# `no screenable arms found` -- indistinguishable, in the output, from a bank
# with nothing worth flagging. `2026-08-14-gatexpoints` is the worked case: both
# gate oracles read `0.000` on HOLD under sweep_inference's "every difference is
# exactly zero" banner, and the arm had to be screened for concentration by hand.
#
# Two guards come with the option, and both exist because the failure they
# prevent looks like a clean result:
#
#   - the column must END IN `_per_gw`. This screen multiplies the paired mean by
#     38 and prints it under "a season", and MIN_SEASON is stated in season
#     points, so a `_points` column would be silently rescaled by a factor of 38.
#     sweep_inference.R's `--scale` makes the same distinction and for the same
#     reason: per_gw and per_path are different estimands, not two units.
#   - the column must be PRESENT, and a metric no file carries is an error naming
#     what the files do carry -- never an empty table. "A screen that silently
#     screens nothing" is the exact failure this whole file exists to catch.
#
# (!) An unknown `--flag` is now an ERROR. The parser was a bare loop with one
# flag and no `--` case, so `--metric=policy_per_gw` was appended to `paths` and
# read as a FILENAME; the run then died on `!! missing:` or, with `--committed`,
# printed a full and entirely HOLD-framed report while the reader believed they
# had asked for POLICY. Same shape as sweep_inference.R:74-91, deliberately.
#
# # (!) `--committed` SKIPS a banked file that lacks the metric, and says so
#
# The bank is shared verbatim with schedule_screen.R and spans several column
# eras, so on any metric newer than the oldest file some files cannot answer.
# Erroring there would make `--metric` unusable for exactly the arms it was added
# for -- the answer would be to hand-list the files, which forks the shared bank,
# and the file-list comment below says two screens disagreeing about which files
# are the record is worse than either being wrong.
#
# So a missing metric column skips THAT FILE, names it, names the metric and
# names the `_per_gw` columns the file does have. The risk skipping carries is
# NARROWING -- a report headed "the record" that is really nine files of thirteen
# -- and narrowing is defeated by COUNTING, not by aborting: the skip list prints
# before the table, the coverage line prints after it, and if every file was
# skipped the run stops rather than printing an empty screen. The precedent is
# already in this file, at the MIN_CELLS `dropped` report.
#
# An explicitly NAMED path is the same rule, and deliberately not a stricter one:
# a second code path here would be a second answer to one question, and the
# coverage line makes the narrowing loud either way.

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

args <- commandArgs(trailingOnly = TRUE)
opt_committed <- FALSE
# The banked column the paired difference is taken on. HOLD by default -- a
# scoring constant belongs there and nearly the whole bank is one -- but see the
# header: HOLD-only made every transfer-side arm byte-identical by theorem and so
# invisible to this screen rather than merely unscreened.
opt_metric <- "hold_per_gw"
paths <- character(0)

# The parser shape is sweep_inference.R:74-91's, not a second convention. The
# `^--` arm is the load-bearing one: without it this loop swallowed any flag it
# did not recognise into `paths` and read it as a filename.
for (a in args) {
  if (identical(a, "--committed")) {
    opt_committed <- TRUE
  } else if (grepl("^--metric=", a)) {
    opt_metric <- sub("^--metric=", "", a)
  } else if (grepl("^--", a)) {
    stop("unknown option: ", a)
  } else {
    paths <- c(paths, a)
  }
}
# Per-gameweek only. `season = mean * 38` and MIN_SEASON below are both stated in
# season points, so a `_points` column would be multiplied by 38 a second time
# and every threshold in the report would be off by that factor -- silently, and
# in the direction that flags everything.
if (!grepl("_per_gw$", opt_metric)) {
  stop("--metric must name a per-gameweek column ending in _per_gw, not: ",
       opt_metric,
       " — this screen reports `mean * 38` as points a season, so a totals ",
       "column would be rescaled by 38 with no error.")
}

# The same curated bank schedule_screen.R reads, and deliberately the same list:
# two screens disagreeing about which files are the record is worse than either
# being wrong.
COMMITTED <- c(
  "stats/cells/2026-08-14-blendlo/blendlo.csv",
  "stats/cells/2026-08-14-blend/blend.csv",
  "stats/cells/2026-08-13-aa95f75/6s-minhl.csv",
  "stats/cells/2026-08-13-aa95f75/6s-fixw.csv",
  "stats/cells/2026-08-13-aa95f75/h-six.csv",
  "stats/cells/2026-08-13-benchshape/benchshape.csv",
  "stats/cells/2026-08-13-4d61058/runD-bonus-shipped.csv",
  "stats/cells/2026-08-13-4d61058/runD-mink-shipped.csv",
  "stats/cells/2026-08-13-4d61058/runD-minw-shipped.csv",
  "stats/cells/2026-08-13-4d61058/runD-bench-shipped.csv",
  "stats/cells/2026-08-12-4d61058/hits.csv",
  "stats/cells/2026-08-12-4d61058/teamform.csv",
  "stats/cells/2026-08-12-4d61058/anchored.csv"
)
if (opt_committed) paths <- c(paths, COMMITTED)
if (length(paths) == 0) {
  stop("give --committed or at least one cells CSV; see the header of this file")
}

# Flag thresholds. A mean below MIN_SEASON is not worth screening — a tiny mean
# is allowed to be lumpy, since nobody is quoting it as an effect.
#
# MIN_CELLS is a THIRD minimum-cell threshold in this pipeline — `diffs_for`'s
# callers pass 2 (grid_width, sweep_inference, schedule_screen) and
# variance_components passes 4, deliberately and with its reason stated. Two
# halves of the complaint this comment used to make are now closed upstream:
# `min_cells` has no default, so every call site states its own, and `diffs_for`
# NAMES an arm it drops rather than letting it vanish. 6 is this screen's own
# floor because the top-3-drop statistic is undefined below it, and the count
# dropped is reported at the end for the same reason cells_common reports it.
MIN_CELLS  <- 6
MIN_SEASON <- 8      # points a season; below this the arm claims nothing
SURVIVE    <- 0.35   # fraction of the mean surviving the top-3 drop
POS_NEAR   <- 0.60   # pos/n below this is "most cells disagree"

rows <- list()
dropped <- character(0)   # arms below MIN_CELLS, named at the end rather than vanishing
identical_arms <- character(0)  # byte-identical contrasts: unscreenable, but counted
# Coverage bookkeeping. A file that cannot answer on the requested metric is
# skipped rather than fatal (see the header), so the count of files that DID
# answer has to be carried to the report -- otherwise a narrowed run and a full
# one print the same table.
n_read <- 0
n_screened <- 0
skipped <- character(0)
seen_metrics <- character(0)   # every _per_gw column any file offered, for the stop message
for (p in paths) {
  if (!file.exists(p)) {
    cat(sprintf("!! missing: %s\n", p))
    next
  }
  # read_cells does the coercion, the keying and the contract checks. This file
  # used to do all three itself, including its own copy of the one-baseline-per-
  # block check -- the "one quantity, two implementations" failure, in the script
  # written to screen the bank for exactly that class of defect.
  #
  # Two behaviour changes come with it, both deliberate:
  #   - a file lacking a contract column now ABORTS rather than being skipped with
  #     a message. The skip was a silent narrowing: the screen would report on
  #     fewer files than its own COMMITTED list names and say so only in passing.
  #   - `cell` is now the WIDE key (run_id, sweep, season, start_gw). Pairing here
  #     is within a single block, so the extra fields are constant and the join is
  #     unchanged -- see the carriers line below for the one place it mattered.
  d <- read_cells(p)
  n_read <- n_read + 1
  have <- grep("_per_gw$", names(d), value = TRUE)
  seen_metrics <- union(seen_metrics, have)
  if (!opt_metric %in% names(d)) {
    # No metric column is in cellsRequired, by design: a cells file written
    # before a column existed must stay readable. So this is a file that cannot
    # answer THIS question, not a malformed file -- skipped, named, and counted
    # into the coverage line so the narrowing cannot pass for a full run.
    skipped <- c(skipped, p)
    cat(sprintf("!! %s has no %s; skipped. It carries: %s\n",
                p, opt_metric, paste(have, collapse = ", ")))
    next
  }
  n_screened <- n_screened + 1

  for (b in unique(d$block)) {
    q <- d[d$block == b, ]
    # read_cells has already refused any block without exactly one baseline arm, so
    # `bl` has rows here by construction. The `if (nrow(bl) == 0) next` this replaced
    # was dead code, and dead code that used to SKIP is worth naming rather than
    # deleting quietly: before the migration a baseline-less block was skipped and the
    # rest of the file screened; now one aborts the whole --committed run over all
    # thirteen banked files. That is a THIRD behaviour change from the migration, and
    # the two the comment above names were argued for while this one arrived silently.
    # It is the right behaviour -- a block with no baseline has no paired difference to
    # take, so screening the rest of the file reports on a bank that is not the one
    # named -- but it is louder than what it replaced.
    bl <- q[q$is_baseline, ]

    # EVERY pairwise contrast, not only against the baseline.
    #
    # (!) The first version of this screen looped over
    # `setdiff(variants, baseline)` only, so `BlendRateK=16 minus BlendRateK=12`
    # -- the contrast its own header is built on, and the one that motivated the
    # whole file -- could not appear in its output. It quoted "+23.3 a season,
    # 17 of 36 cells, three cells carry 99%" and could compute none of them.
    #
    # That is precisely the defect this project recorded against
    # schedule_screen.R ("it cannot test its own motivating example"),
    # reproduced inside the file written to complain about it.
    # sweep_inference.R already emits between-arm contrasts; a screen reading
    # the same bank must too.
    base <- bl$variant[1]
    others <- setdiff(unique(q$variant), base)
    pairs <- list()
    for (a in others) pairs[[length(pairs) + 1]] <- c(a, base, a)
    if (length(others) > 1) {
      for (i in 1:(length(others) - 1)) {
        for (j in (i + 1):length(others)) {
          pairs[[length(pairs) + 1]] <-
            c(paste(others[j], "minus", others[i]), others[i], others[j])
        }
      }
    }

    for (pr in pairs) {
      a <- pr[1]
      lo <- q[q$variant == pr[2] & !q$infeasible, ]
      hi <- q[q$variant == pr[3] & !q$infeasible, ]
      lov <- setNames(lo[[opt_metric]], lo$cell)
      arm <- hi[hi$cell %in% names(lov), ]
      if (nrow(arm) < MIN_CELLS) { dropped <- c(dropped, a); next }
      diff <- arm[[opt_metric]] - lov[arm$cell]
      # NA guard: a missing or infeasible cell on either side makes
      # all(diff == 0) return NA and throw. diffs_for filters these; so must this.
      ok <- !is.na(diff)
      diff <- diff[ok]
      arm <- arm[ok, ]
      if (length(diff) < MIN_CELLS) { dropped <- c(dropped, a); next }
      # Byte-identical: a theorem, not a mean — so it is not screenable. But it is
      # COUNTED, because a contrast that vanishes without a word is the failure this
      # whole file is written against.
      #
      # ⚠️ Added 2026-08-15, and it changes the default output by one line. On
      # `--committed` at the default HOLD metric, **26 contrasts were being dropped
      # here in silence** and appearing in no tally anywhere — so a reader saw
      # "53 of 85 arms flagged" for what was really 85 of 111 contrasts considered.
      # They are exactly the transfer families (`H#1` min_gain, `HITS#1`,
      # `ANCHORED#1`), which are byte-identical on HOLD **by theorem**, so the count
      # is also the pointer to `--metric=policy_per_gw`.
      #
      # Gating this on "the metric is not the default" was considered and rejected:
      # a diagnostic that goes quiet exactly when it has something to say is the
      # failure the MIN_CELLS report at the foot of this file already rails against.
      if (all(diff == 0)) { identical_arms <- c(identical_arms, a); next }
      m <- mean(diff)
      pos <- sum(sign(diff) == sign(m))
      o <- order(abs(diff), decreasing = TRUE)
      m3 <- mean(diff[o[-(1:3)]])
      surv <- if (m == 0) NA_real_ else m3 / m
      # How many DISTINCT seasons the three carriers sit in. 1 means they are
      # nested — overlapping football at different entry points, so one event
      # counted more than once. 3 means three independent occurrences, which is
      # what a real intermittent mechanism looks like.
      #
      # (!) Read the season COLUMN, never parse it out of the key. This line was
      # `sub("@.*", "", arm$cell[...])`, which worked only while `cell` was
      # `season@start_gw`. read_cells keys on `(run_id, sweep, season, start_gw)`
      # separated by spaces, so that regex matches nothing, returns the key
      # unchanged, and `carriers` reads 3 for EVERY arm -- silently, and 3 is
      # exactly the value this record warns is the modal outcome of noise. A
      # derived column that survives a key change by returning a plausible
      # constant is the worst version of this failure.
      carriers <- length(unique(arm$season[o[1:3]]))
      rows[[length(rows) + 1]] <- data.frame(
        block = sub(".* / ", "", b), arm = a, n = nrow(arm),
        season = m * 38, pos = pos, top3 = m3 * 38, surv = surv,
        carriers = carriers, stringsAsFactors = FALSE)
    }
  }
}
# Named, not silently absent — the same reason cells_common's diffs_for names an
# arm it drops. A contrast below MIN_CELLS is one this screen cannot judge, which
# is different from one it judged and did not flag, and a reader counting rows
# cannot tell those apart unless the difference is printed.
#
# (!) This must come BEFORE the "no screenable arms" stop, not after it. Written
# after, the one case where the report is most needed -- every contrast dropped --
# died on `Error: no screenable arms found` with the reason it had just computed
# sitting unprinted. A diagnostic that goes quiet exactly when it has something to
# say is the failure this file exists to screen for.
#
# `unique` on both the count and the list: the same arm name can drop in two blocks,
# and a count that disagrees with the list it is printed beside invites the reader to
# assume the list is truncated.
if (length(dropped) > 0) {
  u <- unique(dropped)
  cat(sprintf("!! %d contrast(s) below MIN_CELLS=%d, not screened: %s\n\n",
              length(u), MIN_CELLS, paste(u, collapse = ", ")))
}
# The byte-identical tally, on the same footing and for the same reason. It is a
# COUNT and not a list: on the default metric it runs to dozens of transfer-family
# arms, and a list that long buries the MIN_CELLS report above it. The count is
# enough to tell a reader that the denominator they are reading is not the whole
# bank — which is the thing they cannot otherwise know.
if (length(identical_arms) > 0) {
  u <- unique(identical_arms)
  cat(sprintf("!! %d contrast(s) byte-identical on `%s`, not screened.\n",
              length(u), opt_metric))
  cat("   Byte-identical is a THEOREM about the arm, not a null result — and on a\n")
  cat("   transfer-side arm it is what HOLD is expected to say. Try",
      "--metric=policy_per_gw.\n\n")
}
# (!) The metric check is a STOP, not an empty table, and it names what the files
# actually carry. `no screenable arms found` on its own is the same string for
# three different situations -- a mistyped metric, a bank that predates the
# column, and a family that is byte-identical by theorem -- and only the third is
# a result. That ambiguity is what sent the gate arm to a manual pass.
if (n_screened == 0) {
  stop("no file carried `", opt_metric, "`. ", n_read, " file(s) read, all ",
       "skipped and named above. Between them they carry: ",
       paste(sort(seen_metrics), collapse = ", "))
}
if (length(rows) == 0) {
  stop("no screenable arms found on `", opt_metric, "` in ", n_screened,
       " file(s). Every contrast was byte-identical, or below MIN_CELLS=",
       MIN_CELLS, ". A transfer-side arm is byte-identical on HOLD BY THEOREM ",
       "-- try --metric=policy_per_gw before reading this as a null.")
}
r <- do.call(rbind, rows)

r$flag <- ifelse(abs(r$season) >= MIN_SEASON &
                   !is.na(r$surv) & r$surv < SURVIVE, "CONCENTRATED", "")
r$flag <- ifelse(r$flag == "" & abs(r$season) >= MIN_SEASON &
                   (r$pos / r$n) < POS_NEAR, "MOST CELLS DISAGREE", r$flag)

r <- r[order(-abs(r$season)), ]
# The banner names the metric, because every number under it is on that metric
# and a table headed HOLD over POLICY differences is the label drift this record
# flags. The map exists so the two banked metrics print their record-wide names
# rather than a column name -- an unmapped column prints as itself.
METRIC_LABELS <- c(hold_per_gw = "HOLD", policy_per_gw = "POLICY",
                   hold_xpoints_per_gw = "HOLD xPoints",
                   policy_xpoints_per_gw = "POLICY xPoints")
metric_label <- if (opt_metric %in% names(METRIC_LABELS))
  unname(METRIC_LABELS[opt_metric]) else opt_metric

cat("\nConcentration screen — is a banked mean a handful of cells?\n")
cat(sprintf("%s. EVERY pairwise contrast, not only against the baseline. No replay.\n\n",
            metric_label))
cat(sprintf("%-13s %-29s %4s %9s %7s %9s %6s %5s  %s\n",
            "sweep", "arm", "n", "a season", "pos/n", "drop top3", "surv",
            "seas", "flag"))
for (i in seq_len(nrow(r))) {
  cat(sprintf("%-13s %-29s %4d %9.1f %3d/%-3d %9.1f %6s %5d  %s\n",
              substr(r$block[i], 1, 13), substr(r$arm[i], 1, 29), r$n[i],
              r$season[i], r$pos[i], r$n[i], r$top3[i],
              if (is.na(r$surv[i])) "-" else sprintf("%.2f", r$surv[i]),
              r$carriers[i], r$flag[i]))
}

n_flag <- sum(r$flag != "")
cat(sprintf("\n%d of %d arms flagged.\n", n_flag, nrow(r)))
# Printed only when the run WAS narrowed, so a full run's output is unchanged. A
# reader who scrolls to the flag count and stops must still be told the table is
# a subset of the files they asked for -- the skip lines are above the table and
# a long table hides them.
if (length(skipped) > 0) {
  cat(sprintf("(!) COVERAGE: %d of %d file(s) carry `%s`; %d skipped: %s\n",
              n_screened, n_read, opt_metric, length(skipped),
              paste(skipped, collapse = ", ")))
}
cat("CONCENTRATED  = |mean| >= ", MIN_SEASON, " a season and under ",
    round(SURVIVE * 100), "% survives dropping the three largest cells.\n", sep = "")
cat("MOST CELLS DISAGREE = |mean| >= ", MIN_SEASON,
    " a season and under ", round(POS_NEAR * 100),
    "% of cells share its sign.\n", sep = "")
cat("`seas` = how many DISTINCT seasons the three carrier cells sit in.\n")
cat("\n(!) Neither flag is a verdict, and a lumpy effect is not a false one -- a\n")
cat("setting that only acts through a wildcard or a chip week SHOULD be lumpy,\n")
cat("and averaging it over n cells is the wrong thing to do to it. A flag says\n")
cat("the mean is a few events, so the follow-up is to find the right\n")
cat("denominator and name a mechanism that predicts those cells.\n")
cat("\n(!) Do NOT read seas=3 as evidence of a real mechanism. Three carriers in\n")
cat("three distinct seasons is the MODAL outcome of pure noise (P = 0.605 on a\n")
cat("6x6 grid). Only seas=1 is informative (P ~ 0.015). Carrier selection is\n")
cat("also length-biased toward the short cells at GW21/GW26, whose per-gameweek\n")
cat("differences carry ~1.7x the sd of a GW1 cell. See this file's header.\n")
cat("\n(!) Counts above are CONTRASTS, not distinct arms: the BLEND and BLENDLO\n")
cat("banks share byte-identical arms, so an arm can appear twice.\n")
