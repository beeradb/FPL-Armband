#!/usr/bin/env Rscript

# The shared cells reader, exercised on the inputs the BANK CANNOT PROVIDE.
#
#   Rscript stats/cells_reader_selftest.R
#
# # Why this exists, and why the banked cells are not enough
#
# A2 consolidated seven readers onto `cells_common.R`, and the acceptance test
# offered for that work was "run every script on committed cells before and after
# and require identical output". That check was run, it passes, and **it has no
# power over the thing the consolidation fixes**. Measured across the whole bank:
#
#   72 files, 7,320 rows, 82 blocks
#   0 rows flagged infeasible
#   0 empty fields in any character column
#   is_baseline typed character in all 72 files
#   no multi-block file ever passed to the one reader that pooled
#
# So the banked inputs exercise none of the paths on which the readers differed.
# A before/after on them answers "did I break a reader that works" — a real and
# necessary question, and a different one. This file answers the other.
#
# It is the same trap this record already paid for once, on the defcon prior:
# an invariance that holds *for the same reason the fix was needed* is an
# acceptance test for collateral damage, not evidence about the fix.
#
# # The five dark paths
#
#   1. A multi-block file with OVERLAPPING cell labels. This is the live one:
#      `grid_width.R` keyed cells on `season@start_gw` and never blocked, so every
#      arm was differenced against every block's baseline.
#   2. Flag spellings the writer does not currently emit — `TRUE`/`FALSE`, which R
#      types as logical, and `1`/`0`, which it types as integer.
#   3. Empty fields in a CHARACTER column, the one case where `na.strings` changes
#      anything at all.
#   4. Infeasible rows, which no banked file has.
#   5. An arm whose observed t is EXACTLY zero, which made the wild bootstrap's
#      tie guard fire and took the entire run's output with it.

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

td <- file.path("stats", "testdata")
failures <- 0
check <- function(label, got, want) {
  ok <- isTRUE(all.equal(got, want))
  if (!ok) failures <<- failures + 1
  note(sprintf("  %-4s %-62s got %s want %s",
               if (ok) "ok" else "FAIL", label,
               paste(format(got), collapse = "/"),
               paste(format(want), collapse = "/")))
}

hr()
note("The shared cells reader, on inputs the bank cannot provide")
hr()

# --- 1. the multi-block file, which is the defect ---------------------------
#
# Two blocks, each with its own baseline, sharing both cell labels. Arm one is
# +1 per gameweek against its own baseline; arm two is -1 against its own.
#
# ⚠️ **The two baselines are deliberately at different levels — 100 and 98 — and
# the first draft of this fixture had them equal.** With equal baselines,
# cross-pairing doubles n and leaves every mean untouched, so the fixture would
# have asserted half the defect while looking complete.
#
# That is not a hypothetical shape. On the banked `MINHL` file, `MINHL#1` and
# `MINHL#2` carry a BYTE-IDENTICAL baseline and only `MINHL#3` differs — so the
# entire measured 0.1434 shift comes from one of the three blocks, and a file whose
# blocks agree on a baseline shows n tripling with nothing moving. The level
# difference is the half that moves a number, and it is the half a fixture is most
# likely to omit.
#
# Under the OLD narrow key (`season@start_gw`) an unblocked `diffs_for` matched
# each arm against both baselines: n doubled AND each mean became the average of
# the real difference and a difference against a foreign experiment — +1 and +3
# averaging to +2 for arm one. Under the shared key it cannot join across blocks,
# so an unblocked call is now correct.
note("")
note("1. a multi-block file, read WITHOUT blocking — the grid_width.R defect")
mb <- read_cells(file.path(td, "cells_two_blocks.csv"))
check("blocks present", length(unique(mb$block)), 2)
check("cell labels overlap between blocks", length(unique(mb$label)), 2)
check("cell keys do not", length(unique(mb$cell)), 4)

arms <- diffs_for(mb, "hold", "_per_gw", min_cells = 2, quiet = TRUE)
check("arms found", length(arms), 2)
check("arm one: cells", nrow(arms[["arm one"]]), 2)
check("arm one: mean", mean(arms[["arm one"]]$diff), 1)
check("arm two: cells", nrow(arms[["arm two"]]), 2)
check("arm two: mean", mean(arms[["arm two"]]$diff), -1)

# The explicit statement of what the old key did, so the failure this guards is
# legible rather than implied: 4 cells instead of 2, and a mean pulled toward the
# foreign baseline — +2 where the truth is +1.
narrow <- mb
narrow$cell <- narrow$label
wrong <- diffs_for(narrow, "hold", "_per_gw", min_cells = 2, quiet = TRUE)
check("under the OLD key, arm one would read", nrow(wrong[["arm one"]]), 4)
check("under the OLD key, its mean would read", mean(wrong[["arm one"]]$diff), 2)
check("under the OLD key, arm two's mean would read",
      mean(wrong[["arm two"]]$diff), -2)

# --- 2. flag spellings ------------------------------------------------------
note("")
note("2. flag spellings the writer does not emit today")
up <- read_cells(file.path(td, "cells_flags_upper.csv"))
check("TRUE/FALSE: baselines found", sum(up$is_baseline), 2)
check("TRUE/FALSE: infeasible found", sum(up$infeasible), 0)
num <- read_cells(file.path(td, "cells_flags_numeric.csv"))
check("1/0: baselines found", sum(num$is_baseline), 2)
check("1/0: infeasible found", sum(num$infeasible), 1)

# The idiom two readers used before A2, shown failing on the same input. It is
# not a hypothetical: `rank_robustness.R` tested `is_baseline == "true"` and
# `entry_density.R` `%in% c(TRUE, "true")`, and both were correct only while R
# happened to type the column as character.
raw_upper <- read_sidecar(file.path(td, "cells_flags_upper.csv"))
check("the retired `== \"true\"` idiom would find",
      sum(raw_upper$is_baseline == "true", na.rm = TRUE), 0)

# --- 3. empty character fields ---------------------------------------------
#
# The only case where `na.strings` changes anything. R reads an empty NUMERIC
# field as NA regardless; a character one stays "" without it.
note("")
note("3. an empty field in a character column")
check("empty oracle reads as NA", sum(is.na(up$oracle)), 3)
check("and not as the empty string", sum(up$oracle == "", na.rm = TRUE), 0)

# --- 4. infeasible rows -----------------------------------------------------
note("")
note("4. infeasible rows, of which the bank has none")
check("infeasible flagged", sum(num$infeasible), 1)
check("and it is the row it should be",
      num$variant[num$infeasible], "arm")

# --- 5. an arm whose observed t is exactly zero -----------------------------
#
# The wild bootstrap's tie guard asserts that six enumerated draws must reproduce
# |t| exactly: the CR2 t is homogeneous of degree 0 in the cluster weights, and six
# draws give every movable cluster the same Webb weight. That argument is sound.
# The tolerance testing it was `1e-9 * abs(chk)` — purely RELATIVE — which collapses
# to 0 when the arm's mean is 0. "Tied" then means "bit-exactly zero", and floating
# point delivered only 2 of the 6.
#
# ⚠️ Not a hypothetical. On 2026-08-24 a plain MINHL run over the six-season
# `extended` grid aborted with NO OUTPUT AT ALL — every metric, every arm — because
# one arm (`hold_nocap_points` / `half-life 8`) cancelled to a mean of 0. An
# exactly-inert arm is an ordinary thing for a sweep to contain, and its p is 1.
#
# ⚠️ **The fixture is the REAL 36 cells, and a synthetic one did not reproduce.**
# The first draft here built six clusters from small integers whose means cancelled
# in pairs — and THE TEST PASSED AGAINST THE UNFIXED CODE, asserting nothing.
#
# The discriminator is not the observed t. Both fixtures have `chk` exactly 0, so
# both give the old relative tolerance `1e-9 * abs(chk)` a value of exactly 0. What
# differs is whether the six DRAWS the guard counts on cancel exactly. Those six are
# the six constant Webb weights, and only ±1 is exact in binary floating point. On
# the real cells:
#
#   w = ±1.2247448713915889 (±sqrt(1.5))  ->  t* = ∓5.972e-18   not zero
#   w = ±1                                ->  t* =  0.000e+00   zero
#   w = ±0.70710678118654757 (±sqrt(0.5)) ->  t* = ∓7.844e-17   not zero
#
# so 2 of 6 tied under a zero tolerance and the guard fired. The synthetic integers
# cancelled under every weight, all six tied, and the guard was satisfied. Real
# cells leave residue; round numbers do not.
note("")
note("5. a near-zero-t arm returns p = 1 rather than aborting the run")
zero_arm <- read_sidecar(file.path(td, "cells_zero_t_arm.csv"))
check("the fixture has its 36 cells", nrow(zero_arm), 36)
check("which cancel to a zero mean", mean(zero_arm$diff), 0)
z <- tryCatch(wild_cluster_p_season(zero_arm),
              error = function(e) list(p = paste("ABORTED:", conditionMessage(e))))
check("p is 1, and the guard did not fire", z$p, 1)

hr()
if (failures > 0) {
  note(failures, " self-test check(s) FAILED")
  quit(status = 1)
}
note("all self-test checks passed")
