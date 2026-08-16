#!/usr/bin/env Rscript
#
# Screen the shipped constants for schedules in disguise.
#
# Usage:
#   Rscript stats/schedule_screen.R --committed
#   Rscript stats/schedule_screen.R path/to/cells.csv [more.csv ...]
#   Rscript stats/schedule_screen.R --committed --out=stats/out/screen.csv
#
# ---------------------------------------------------------------------------
# The question
#
# `BonusWeight` looked structureless in aggregate because two opposite trends
# averaged out — harmful from GW1, helpful from GW11 — and the answer was **a
# schedule, not a constant**. A flat value cannot express a term that wants one
# setting early and another later, and averaging over entry points hides
# exactly that shape.
#
# So: for every sweep whose cells are committed under stats/cells/, print each
# arm's mean paired difference **per entry gameweek** on **HOLD**, and flag any
# constant whose *ordering across settings* reverses between the early and late
# columns.
#
# No replay. This reads cells that already exist.
#
# ---------------------------------------------------------------------------
# Why HOLD only
#
# On HOLD the intervention acts once, at the opening build, so the entry
# gameweek *is* the information regime — a GW1 entrant decides on the prior
# alone, a GW26 entrant on most of a season. On POLICY a term acting every week
# has its within-season reversal averaged inside each column, and one short
# cell can impersonate a trend. POLICY is therefore not screened here, and a
# constant whose HOLD column is byte-identical is reported as a THEOREM rather
# than as a null — see below.
#
# ---------------------------------------------------------------------------
# What a flag does and does not mean
#
# Three limitations, all of which constrain the reading rather than the method,
# and none of which are visible in the table unless stated here.
#
# 1. **The entry columns are nested suffixes of one season, not disjoint
#    regimes.** A GW1 cell replays GW1-38 and a GW26 cell replays GW26-38, so
#    the late column's football is *contained in* the early column's.
#
#    ⚠️ **This argument is CONDITIONAL on the calendar reading, and the first
#    version of this comment stated it unconditionally.** The decomposition
#    D(g) = mean over weeks [g,38] of a per-week effect e(w) requires a SINGLE
#    e(w) — that is, an effect indexed by calendar week and not by which squad
#    was built. Under that reading the shrink is exact and computable:
#    D(26) − D(1) = (25/38) × (mean over [26,38] − mean over [1,25]), a factor
#    of **0.66**, sign-preserving and proportional, so the size IS readable
#    after dividing by it. Under an EVIDENCE reading — the one limitation 2
#    describes and the one the bench-shape mechanism below assumes — e is
#    indexed by entry point, the squads differ, and the nesting of *weeks*
#    implies nothing about the nesting of *effects*: there is no attenuation to
#    correct. The screen cannot tell which it has, so: attenuated by ~0.66 under
#    a calendar effect, un-attenuated under an evidence effect.
#
#    ⚠️ And non-flatness is **necessary, not sufficient**. A first version said
#    the contrast is non-zero "exactly when" e is not flat; any e whose mean
#    over [26,38] equals its mean over [1,38] gives exactly zero while being
#    wildly non-flat. Read it as "only if".
#
# 2. **Entry point confounds evidence-at-entry with the window scored.** A late
#    entrant both knows more at the build and is scored only on the run-in. A
#    flag therefore says "this constant's best value depends on entry point",
#    which is consistent with an evidence schedule (n/(n+k)-shaped) and with a
#    calendar one (the run-in scores differently). The screen cannot separate
#    them. For a blend weight the two readings coincide; for a fixture term
#    they do not.
#
# 3. **This is a screen, so it is an argmax over many contrasts.** Every arm of
#    every sweep contributes one, and picking the largest is exactly the
#    winner's curse this record warns about. Holm over the whole screened
#    family is printed for that reason, and nothing here is a verdict: a flag
#    buys a *pre-registered* re-run of that one constant, nothing else.
#
# 4. **Power, which is the thing to read before the p values.** The schedule
#    test's p=0.05 detection threshold is 150-350 season points per ladder, and
#    its 80%-power MDE is 207-457, against AGENTS.md's own median detectable of
#    39 — so a null here was close to guaranteed by the design. ⚠️ Do **not** import this record's prior that an interaction is the
#    *cheap* contrast (CR2 SE 0.216 against 0.599): that holds for a difference
#    of differences WITHIN one cell, which cancels path divergence. This one is
#    formed BETWEEN cells, across entry points that are different squads
#    scoring different football, so none of that cancellation applies. It is
#    the expensive kind.
#
# The controls are what make the screen readable rather than suggestive. BONUS
# is a **positive control** — it is the prior/evidence schedule sweep, the
# constant whose entry-point reversal founded this whole hypothesis, so the
# screen ought to re-find it. ⚠️ It cannot: the schedule test refuses BONUS, and
# see `ladder_of` for why that refusal is a coincidence of label text rather
# than a principled test of dimensionality.
#
# The transfer-gate sweeps are **structural negatives**: `min_gain` and the hit
# threshold are confined to `decide()`, which HOLD never calls, so their HOLD
# columns are byte-identical by theorem and reporting them as "no schedule
# found" would be reporting that the compiler works. ⚠️ **The chip-calendar
# sweep is a theorem for a DIFFERENT reason** — `HoldCaptaincyWeekly`
# (`internal/backtest/simulate.go:2609`) never touches `Chips`, `Chips2` or
# `ChipPlanner`, so HOLD plays no chips at all; it is not about `decide()`.
# Naming the consumer is the check, and one wrong consumer was named here.
#
# ⚠️ A consequence of screening HOLD only: **no transfer constant is screened at
# all.** For those the schedule question is unasked, not answered — it is only
# askable on POLICY, which this screen refuses by design and for the reason in
# "Why HOLD only" above.
# ---------------------------------------------------------------------------

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

# --- options ---------------------------------------------------------------

args <- commandArgs(trailingOnly = TRUE)
opt_out <- NULL
opt_committed <- FALSE
paths <- character(0)
for (a in args) {
  if (a == "--committed") {
    opt_committed <- TRUE
  } else if (grepl("^--out=", a)) {
    opt_out <- sub("^--out=", "", a)
  } else if (grepl("^--", a)) {
    fail("unknown option: ", a)
  } else {
    paths <- c(paths, a)
  }
}

# The curated set, and the reason for each choice. This list IS the provenance
# of anything the screen reports, so it is here rather than in a shell command
# somebody has to remember: several sweeps were banked in three or five data
# states and at two grid widths, and picking silently would put a level in the
# record with no data state attached — which this project's standing rule
# forbids.
#
# The rule applied, in order: prefer the SIX-season grid where one was banked,
# then the SHIPPED data state over the repair-disabled ones. Sweeps banked only
# at four seasons are taken at their shipped state and marked.
COMMITTED <- c(
  # Six-season grids, shipped data state.
  "stats/cells/2026-08-14-blend/blend.csv",          # BLEND, BlendRateK — banked 2026-08-14 BECAUSE this screen could not reach it
  "stats/cells/2026-08-13-aa95f75/6s-minhl.csv",     # MINHL, minutes half-life
  "stats/cells/2026-08-13-aa95f75/6s-fixw.csv",      # FIXW, fixture weight
  "stats/cells/2026-08-13-aa95f75/h-six.csv",        # H, min_gain — structural negative
  "stats/cells/2026-08-13-benchshape/benchshape.csv",# bench slot shape
  # Four-season grids, shipped data state (no six-season bank exists).
  "stats/cells/2026-08-13-4d61058/runD-bonus-shipped.csv", # BONUS — positive control
  "stats/cells/2026-08-13-4d61058/runD-mink-shipped.csv",  # MINK, minutes prior strength
  "stats/cells/2026-08-13-4d61058/runD-minw-shipped.csv",  # MINW, minutes convexity
  "stats/cells/2026-08-13-4d61058/runD-bench-shipped.csv", # BENCH, bench weight level
  "stats/cells/2026-08-12-4d61058/hits.csv",         # HITS — structural negative
  "stats/cells/2026-08-12-4d61058/teamform.csv",     # TEAMFORM
  "stats/cells/2026-08-12-4d61058/anchored.csv"      # ANCHORED, chip calendar
)

if (opt_committed) paths <- c(paths, COMMITTED)
if (length(paths) == 0) {
  stop("give --committed or at least one cells CSV; see the header of this file")
}

# --- load ------------------------------------------------------------------

METRIC <- "hold"
SUFFIX <- "_per_gw"

read_one <- function(p) {
  # `read_cells` is in cells_common.R, and the contract checks below used to live
  # here — this script had the strictest set in the family, so promoting them made
  # them everyone's. Verified against the whole bank before the promotion: 72 files,
  # 7,320 rows, 82 blocks, zero failures.
  d <- read_cells(p)

  # This screen additionally needs the HOLD columns and the arm's declared setting.
  miss <- setdiff(c("hold_points", "hold_per_gw"), names(d))
  if (length(miss) > 0) {
    fail(p, " is missing columns: ", paste(miss, collapse = ", "))
  }
  # `setting` is OPTIONAL on purpose: every cells file banked before the column
  # existed lacks it, and refusing those would make this screen's own recorded
  # result — 7 ladders, 31 arm contrasts — impossible to reproduce from the bank it
  # was measured on. Absent reads as all-NA, which `ladder_of` treats as a distinct,
  # named provenance rather than as a gap to paper over. It is not in
  # `cellsRequired` for the same reason.
  if (!("setting" %in% names(d))) d$setting <- NA_real_
  d$setting <- suppressWarnings(as.numeric(d$setting))

  # Keep exactly the columns the screen reads. Snapshots banked weeks apart
  # carry different optional columns — the captaincy rungs and the variance
  # ladder arrived later — and rbind on the union would fail on a schema
  # difference that has nothing to do with this question.
  keep <- c(cellsRequired, "hold_points", "hold_per_gw",
            "setting", "cell", "label", "block", "src")
  d[, keep]
}

# The data state, read off the sibling provenance file rather than inferred
# from the path. "Name the data state or do not quote a recorded level."
#
# ⚠️ It must be looked up PER SWEEP BLOCK, not per file. The provenance schema is
# `sweep,run_id,key,value` and a cells file can hold several blocks with
# DIFFERENT commits — `runA-minhl-shipped.csv` holds three, at 59f0830, c6190c3
# and 9daefda. A first version filtered on `key` alone and took `commit[1]`,
# which printed MINHL#1's commit under MINHL#2's numbers and repeated the env
# string once per block. It quoted the wrong data state while citing the rule
# that says not to, and nothing errored. The committed set is all single-block,
# so the banked run was correct by luck rather than by construction.
state_of <- function(p, sweep = NULL, run_id = NULL) {
  pv <- sub("\\.csv$", ".provenance.csv", p)
  if (!file.exists(pv)) return("provenance absent")
  d <- read_sidecar(pv)
  if (!all(c("key", "value") %in% names(d))) return("provenance unreadable")
  if (!is.null(sweep) && "sweep" %in% names(d)) d <- d[d$sweep == sweep, ]
  if (!is.null(run_id) && "run_id" %in% names(d)) d <- d[d$run_id == run_id, ]
  if (nrow(d) == 0) return("no provenance row for this sweep block")
  one <- function(k) {
    v <- unique(d$value[d$key == k])
    if (length(v) == 0) NULL else v
  }
  bits <- character(0)
  commit <- one("commit")
  if (!is.null(commit)) bits <- c(bits, paste0("commit ", substr(commit, 1, 7)))
  # A run made from a dirty tree is not identified by its commit, and every
  # committed provenance file here records dirty=true. Saying so is the whole
  # point of naming a data state.
  dirty <- one("dirty")
  if (!is.null(dirty) && any(tolower(dirty) %in% c("true", "t", "1"))) {
    bits <- c(bits, "DIRTY TREE (commit does not identify the code)")
  }
  env <- one("env")
  if (!is.null(env)) bits <- c(bits, gsub("[\t ]+", "=", env))
  if (length(bits) == 0) "no env recorded" else paste(bits, collapse = "; ")
}

cells <- do.call(rbind, lapply(paths, read_one))
cells <- cells[!cells$infeasible, ]
if (nrow(cells) == 0) fail("every cell is flagged infeasible")

note("Schedule screen — does a constant's best value depend on entry point?")
note("Metric: HOLD, per gameweek. ", length(unique(cells$block)), " sweep(s) over ",
     length(paths), " committed cells file(s). No replay.")
note("")

# --- the screen ------------------------------------------------------------

ENTRIES <- sort(unique(cells$start_gw))
rows <- list()
blocks <- list()

for (b in unique(cells$block)) {
  d <- cells[cells$block == b, ]
  baseline <- unique(d$variant[d$is_baseline])
  src <- unique(d$src)[1]
  # min_cells 2, passed rather than defaulted — see the note in cells_common.R.
  arms <- diffs_for(d, METRIC, SUFFIX, min_cells = 2, quiet = TRUE)

  # The declared setting per arm, collapsed from the cells. It is written per
  # cell but is a property of the variant, so disagreement inside one arm means
  # the sweep varied it by cell — which would make every ladder statistic below
  # a slope in a setting half the cells did not have. Fail rather than take the
  # first: this is the same rule as the oracle stamp, whose per-cell equality
  # runPolicySweep asserts in Go for the same reason.
  settings <- vapply(split(d$setting, d$variant), function(v) {
    u <- unique(v)
    if (length(u) != 1) NA_real_ else as.numeric(u)
  }, numeric(1))
  for (nm in names(settings)) {
    v <- unique(d$setting[d$variant == nm])
    if (length(v) > 1) {
      fail(src, ": block ", b, " arm '", nm, "' declares ", length(v),
           " different settings (", paste(v, collapse = ", "),
           "); a ladder rung must not depend on the cell.")
    }
  }

  # ⚠️ A block with no usable arm must be REPORTED, not skipped. A first version
  # did `if (length(arms) == 0) next` before writing anything, so a sweep whose
  # arms all lacked usable HOLD cells vanished from the report entirely while
  # the header above still counted it among the sweeps read — a curated entry
  # could be silently absent and the count would say otherwise. That is the
  # exact shape this record calls "measures nothing, looks like a clean null".
  if (length(arms) == 0) {
    blocks[[b]] <- list(
      block = b, baseline = baseline, src = src, theorem = FALSE,
      unusable = TRUE, seasons = length(unique(d$season)),
      state = state_of(src, d$sweep[1], d$run_id[1]), arms = arms,
      settings = settings)
    next
  }

  # A sweep every one of whose arms is exactly zero on HOLD is a knob confined
  # to the transfer path. That is a theorem about which functions read the
  # field, not a measurement of a schedule, and it must not consume a slot in
  # the multiplicity family.
  all_zero <- all(vapply(arms, function(a) degenerate(a) && all(a$diff == 0),
                         logical(1)))

  blocks[[b]] <- list(
    block = b, baseline = baseline, src = src, theorem = all_zero,
    unusable = FALSE, seasons = length(unique(d$season)),
    state = state_of(src, d$sweep[1], d$run_id[1]), arms = arms,
    settings = settings)

  if (all_zero) next

  for (nm in names(arms)) {
    a <- arms[[nm]]
    a$regime <- regime_of(a$start_gw)

    # Per-entry means across seasons: the six numbers the item asks for.
    per_entry <- tapply(a$diff, a$start_gw, mean)

    # The contrast, formed WITHIN each season and then averaged over seasons.
    # Seasons are the independent replicates here; the six entry points inside
    # one season replay overlapping football and are not six observations.
    cs <- list()
    for (s in unique(a$season)) {
      q <- a[a$season == s, ]
      e <- mean(q$diff[q$regime == "early (prior-led)"])
      l <- mean(q$diff[q$regime == "late (season-led)"])
      if (is.finite(e) && is.finite(l)) {
        cs[[length(cs) + 1]] <- data.frame(season = s, diff = l - e)
      }
    }
    con <- if (length(cs) > 0) do.call(rbind, cs) else NULL
    st <- if (!is.null(con) && nrow(con) > 1) se_cr(con, con$season) else
      list(se = NA, df = NA, t = NA, p = NA)

    # GW1-against-the-rest, formed the same way and reported beside the 2/2/2
    # contrast rather than instead of it.
    #
    # It exists because the 2/2/2 split this file criticises for the BONUS case
    # is the split the late-minus-early column uses, and a step at GW1 — which
    # is the shape the strongest candidate here actually has — is diluted by
    # pooling GW1 with GW6. A linear or two-bucket statistic has poor power
    # against a step, so a screen that only prints those will under-report
    # exactly the shape the founding case has.
    #
    # The GW1 column also sits in the QUIET group rather than the noisy one, so a
    # candidate resting on it escapes the late-column caveat that discounts most
    # of the late-minus-early ranking. Cell-level SD of the paired differences by
    # entry gameweek, over all screened arms (132 cells per column):
    #
    #   GW1 1.548   GW6 1.525   GW11 1.537   GW16 2.218   GW21 2.573   GW26 3.357
    #
    # ⚠️ GW1 is NOT the minimum — the first three are effectively tied and GW6 is
    # marginally lowest. The step is at GW16. Read the group, not the rank.
    fs <- list()
    for (s in unique(a$season)) {
      q <- a[a$season == s, ]
      one <- mean(q$diff[q$start_gw == min(ENTRIES)])
      rest <- mean(q$diff[q$start_gw != min(ENTRIES)])
      if (is.finite(one) && is.finite(rest)) {
        fs[[length(fs) + 1]] <- data.frame(season = s, diff = one - rest)
      }
    }
    fcon <- if (length(fs) > 0) do.call(rbind, fs) else NULL
    fst <- if (!is.null(fcon) && nrow(fcon) > 1) se_cr(fcon, fcon$season) else
      list(se = NA, df = NA, t = NA, p = NA)

    rows[[length(rows) + 1]] <- data.frame(
      block = b, variant = nm, variant_index = a$variant_index[1],
      mean_all = mean(a$diff),
      early = mean(a$diff[a$regime == "early (prior-led)"]),
      late = mean(a$diff[a$regime == "late (season-led)"]),
      contrast = if (is.null(con)) NA else mean(con$diff),
      se = st$se, df = st$df, t = st$t, p = st$p,
      gw1_vs_rest = if (is.null(fcon)) NA else mean(fcon$diff),
      gw1_t = fst$t, gw1_p = fst$p, gw1_df = fst$df,
      n_seasons = length(unique(a$season)),
      byte_identical = degenerate(a) && all(a$diff == 0),
      stringsAsFactors = FALSE)
    for (g in ENTRIES) {
      rows[[length(rows)]][[paste0("gw", g)]] <-
        if (as.character(g) %in% names(per_entry)) per_entry[[as.character(g)]] else NA
    }
  }
}

if (length(rows) == 0) stop("no screenable arms found")
res <- do.call(rbind, rows)

# Multiplicity over the whole screened family — every arm of every measurable
# sweep, which is what an "any constant reverses?" question actually searches.
res$p_holm <- p.adjust(res$p, method = "holm")

# --- report ----------------------------------------------------------------

num <- function(x, dp = 3, w = 7) {
  if (length(x) == 0 || is.na(x)) return(formatC(".", width = w))
  formatC(x, format = "f", digits = dp, width = w)
}

# --- the ladder slope, per entry column -------------------------------------
#
# "Ordering across settings reverses between the early and late columns" is a
# statement about the ladder's SLOPE, and the two-bucket contrast above cannot
# see it: the repo's early/middle/late split is 2/2/2, so the "early" bucket is
# exactly {GW1, GW6} — and the founding `BonusWeight` case turns between GW1 and
# GW6, i.e. *inside* that bucket. ⚠️ A first version wrote "between GW1 and
# GW11, inside that first bucket", which is self-inconsistent: GW11 is in the
# MIDDLE bucket. The argument survives either way — the early bucket's own two
# columns straddle the turn — but the parenthetical was wrong and comments here
# carry the justification. Pooling averages the reversal away, which is the same
# failure ("averaging reads as no structure") the schedule hypothesis names.
#
# So the slope is computed per entry column: Spearman rho between an arm's
# numeric setting and its mean paired difference, the baseline included at
# exactly 0. A sign change across the six columns is the screen's real flag.
#
# ⚠️ This rho and the schedule test's slope are DIFFERENT ESTIMANDS printed
# under adjacent labels: this is a rank correlation, `schedule_test` regresses
# on the raw setting scale. They can disagree in sign — `MINHL`'s OLS slope is
# dominated by the `half-life 20` arm and the rank correlation is not. The
# DESCRIPTIVE flag below uses this rho; the inferential test uses OLS.
#
# A sweep is only a ladder if its DESIGN says so, which since 2026-08-14 means
# the `setting` column: each arm supplies a getter that runs on the config the
# cell was simulated under, so the number in the file is the number that ran and
# an arm which declares nothing is positively saying it varies no single scalar.
# The sweep is refused unless every arm declares one and they are distinct.
#
# ⚠️ Read the refusals with their source, which the screen now prints. BONUS was
# refused because `0.5 / 1.5` and `0.5 / 2.0` both *parse* to 0.5 — a
# coincidence of text that happened to give the right answer for a genuinely
# two-dimensional family, and that meant the screen could not test its own
# motivating example. Declared settings refuse it for the actual reason. The
# label parse survives for pre-column banks only, including the one this
# screen's recorded 7-ladder result was measured on; those runs print a warning
# saying the slope is licensed by the labels rather than by the design.
# Either way: a slope through a family that is not ordered is a number with no
# referent.
# --- the schedule test ------------------------------------------------------
#
# The sign-change flag above is DESCRIPTIVE and must not be read as a test.
# Under the null of no schedule, each column's rank correlation is a noisy draw
# about the constant's true slope, so with four arms and six columns a sign
# change is close to certain whenever that true slope is near zero — the flag
# fires hardest exactly where there is no effect at all. It conflates "this
# constant does nothing" with "this constant wants to be a schedule", which are
# the two hypotheses the screen exists to separate.
#
# The test that does separate them asks whether the ladder's slope TRENDS with
# entry point:
#
#   b(season, entry) = OLS slope of paired difference on setting, across the
#                      ladder's arms, within that one cell
#   c(season)        = OLS slope of b on entry index, within that one season
#   the statistic    = mean of c over seasons, season-clustered
#
# Seasons are the replicates. The six entry points inside one season replay
# overlapping football — a GW26 cell's weeks are a subset of a GW1 cell's — so
# they are not six independent observations, and forming c() within a season
# before averaging is what keeps that honest. The df is therefore seasons - 1:
# three on a four-season bank, five on a six-season one.
#
# ⚠️ **It routes through se_cr, but call it a one-sample t over seasons, which
# is what it is.** With one observation per cluster the CR2 adjustment is
# (1 - 1/G)^(-1/2) for every cluster, the sandwich collapses to s^2/G, and the
# Satterthwaite df is exactly G-1 — verified bit-identical to `t.test` at G=4
# and G=6. Calling it CR2 would be misleading in THIS record specifically:
# cells_common.R's virtue for CR2 is that "the df it resolves is *reported*
# rather than asserted", and here the df is asserted by the design. A reader
# comparing "df 5" here with "df 5" from a genuine CR2 fit elsewhere would
# think comparable information had been resolved. se_cr is used anyway so there
# is one implementation, not two.
#
# Two properties in the statistic's favour, neither obvious:
#
#   - **The ladder slope is invariant to the baseline's own noise.** Every point
#     is (arm - baseline) including the baseline's own 0, so the common baseline
#     is an additive shift and cancels out of a slope entirely; b() is the slope
#     of the *absolute* HOLD per-gw on the setting. That is a real reason to
#     prefer it over the 28 per-arm contrasts, which all share the baseline's
#     noise and are therefore strongly positively correlated with each other.
#   - **The unweighted second stage costs almost nothing.** lm(b ~ g) puts
#     maximal leverage on GW1 and GW26 and GW26 has ~4.7x GW1's variance, but
#     inverse-variance weighting buys only ~12% on the SE, because the weights
#     that down-weight GW26 also destroy design leverage. Do not "fix" this
#     expecting a factor.
#
# The real specification limit is that a LINEAR second stage has poor power
# against a STEP — which is the shape both the founding case and the strongest
# candidate here actually have. That is what the GW1-against-rest contrast in
# the per-arm table is for.
#
# `baseline_setting` is NA when the baseline's own label carries no parseable
# setting — TEAMFORM's "shipped, no club-form blend" is the case. The test then
# runs on the arms alone, which is correct but weaker: it loses the one point
# on the ladder whose difference is exactly zero by construction.
schedule_test <- function(arms, baseline_setting, ld, entries) {
  per <- list()
  for (nm in names(arms)) per[[nm]] <- arms[[nm]]
  seasons <- unique(unlist(lapply(per, function(a) a$season)))
  cs <- list()
  for (s in seasons) {
    bs <- list()
    for (g in entries) {
      x <- numeric(0); y <- numeric(0)
      if (!is.na(baseline_setting)) { x <- baseline_setting; y <- 0 }
      for (nm in names(per)) {
        a <- per[[nm]]
        v <- a$diff[a$season == s & a$start_gw == g]
        st <- ld$vals[[nm]]
        if (length(v) == 1 && !is.na(st) && isTRUE(ld$keep[[nm]])) {
          x <- c(x, st); y <- c(y, v)
        }
      }
      if (length(x) >= 3 && length(unique(x)) >= 2) {
        bs[[length(bs) + 1]] <- data.frame(g = g, b = unname(coef(lm(y ~ x))[2]))
      }
    }
    if (length(bs) >= 3) {
      bb <- do.call(rbind, bs)
      if (length(unique(bb$g)) >= 2) {
        cs[[length(cs) + 1]] <-
          data.frame(season = s, diff = unname(coef(lm(b ~ g, data = bb))[2]))
      }
    }
  }
  if (length(cs) < 2) return(NULL)
  con <- do.call(rbind, cs)
  st <- se_cr(con, con$season)

  # Two thresholds on the scale the record quotes: the GW1-against-GW26 swing in
  # the ladder's best value, over the ladder's own span, times a season.
  #
  # ⚠️ **`thr05` is a p = 0.05 DETECTION THRESHOLD, not an MDE**, and a first
  # version of this script called it MDE in the column header, in `FINDINGS.md`
  # and in the constants-and-sweeps note. AGENTS.md distinguishes them and
  # points at `stats/variance_components.R`, which prints "the p = 0.05 effect
  # **and** the 80%-power MDE" as two columns. `mde80` is the second one, on that
  # script's own convention `(t_crit + qt(0.8, df)) * se` — about 1.36x `thr05`
  # at df 5. Quoting the smaller number under the larger name understates how
  # blunt this screen is by a third.
  #
  # ⚠️ And **`swing / thr05` is identically `t / t_crit`** — the span, the range
  # and the 38 all cancel. So the two columns are not two quantifications of the
  # result; the ratio between them is the t already in the row. Report them for
  # the SCALE they put the effect on, never as corroboration of each other.
  #
  # Without a threshold column "Holm 1.000" is unreadable: a null means nothing
  # until the reader knows what it excludes, and this instrument is several times
  # blunter than an ordinary sweep verdict, so the tie was close to guaranteed by
  # the design rather than discovered by the data.
  #
  # ⚠️ The estimate is a slope in (pts/gw) per unit-of-setting per gameweek-of-
  # entry, so the raw column is SIX DIFFERENT UNITS and must never be read
  # across ladders. Multiplying by each ladder's own setting range and by the
  # entry span is what makes the rows comparable.
  span <- diff(range(entries))
  rng <- diff(range(ld$vals[ld$keep]))
  scale <- span * rng * 38
  ok <- !is.na(st$se) && !is.na(st$df)
  list(estimate = mean(con$diff), t = st$t, p = st$p, df = st$df,
       n_seasons = nrow(con), se = st$se,
       gap = mean(con$diff) * scale,
       thr05 = if (ok) qt(0.975, st$df) * st$se * scale else NA,
       mde80 = if (ok) (qt(0.975, st$df) + qt(0.80, st$df)) * st$se * scale else NA)
}

# ladder_of decides whether a family of arms is an ordered one-dimensional
# ladder, and if so at what settings.
#
# # The declared column is authoritative and the label parse is a fallback for
# # ONE case: a bank written before the column existed
#
# The sweep now emits `setting` per cell, read off the config the cell actually
# ran under by a getter the variant supplies (see cellRow.HasSetting). That is a
# statement about the *design*, and it fixes three separate defects the label
# parse had:
#
#   - it refused BONUS on a coincidence — `0.5 / 1.5` and `0.5 / 2.0` both parse
#     to 0.5 — so the screen could not test its own motivating example;
#   - it dropped any arm whose label carries no number, which is how TEAMFORM's
#     baseline and MINHL's "flat" arm left their ladders, in TEAMFORM's case
#     removing the on/off step its recorded result is about;
#   - and worst, it would have SILENTLY ACCEPTED a two-dimensional family whose
#     first coordinates happened to differ (`0.0/2.0, 0.5/1.5, 1.0/1.0`) and
#     reported a slope with no referent.
#
# An arm that declares no setting is now a positive statement that the family
# varies no single scalar, so refusing it is principled rather than incidental.
#
# The label parse survives only for a file with NO setting column at all, which
# is every bank committed before 2026-08-14 — including the one this screen's
# recorded result was measured on. Deleting it would make that result
# irreproducible from its own bank, which this record weighs heavily. A MIXED
# block is refused outright: it means some arms declared and others did not,
# which no single provenance describes.
ladder_of <- function(labels, settings) {
  # ⚠️ REQUIRED, with no default, and that is the fix for a bug this function
  # shipped for about an hour. With `settings = NULL` a caller that forgot to
  # plumb the column through fell silently into the label path AND printed
  # "this bank predates the `setting` column" — a false statement about the
  # data, produced by the guard written to make the provenance honest. The
  # first BLENDLO run hit exactly that: the block list carried `settings` in
  # its unusable branch and not in its main one.
  #
  # A missing argument is now an R error. Absence must be a property of the
  # FILE, which reads as all-NA below, never of whether a call site remembered.
  if (missing(settings) || is.null(settings)) {
    stop("ladder_of: settings not supplied. Absence of a declared setting is a ",
         "property of the cells file (all-NA), not of the call site.")
  }
  if (!all(labels %in% names(settings))) {
    stop("ladder_of: no setting entry for ",
         paste(setdiff(labels, names(settings)), collapse = ", "),
         " — the settings vector must cover every arm including the baseline.")
  }
  {
    vals <- as.numeric(settings[labels])
    n_dec <- sum(!is.na(vals))
    if (n_dec > 0 && n_dec < length(labels)) {
      return(list(ok = FALSE, source = "mixed", why = paste0(
        n_dec, " of ", length(labels), " arms declare a setting; a family is ",
        "either an ordered ladder or it is not, and a partly-declared block ",
        "has no single provenance")))
    }
    if (n_dec == length(labels)) {
      keep <- rep(TRUE, length(labels))
      names(vals) <- labels
      names(keep) <- labels
      if (length(labels) < 3) {
        return(list(ok = FALSE, source = "column", why = paste0(
          "only ", length(labels), " arm(s) declare a setting; a ladder needs 3")))
      }
      # Still refused, but now on the design rather than on the text: two arms
      # at one setting means the family is not ordered by this number.
      if (anyDuplicated(vals)) {
        return(list(ok = FALSE, source = "column", why = paste0(
          "two arms declare the same setting (",
          paste(unique(vals[duplicated(vals)]), collapse = ", "),
          "), so this number does not order the family")))
      }
      return(list(ok = TRUE, source = "column", keep = keep, vals = vals))
    }
    # n_dec == 0 falls through to the label parse below, which is the
    # pre-column bank.
  }
  first_num <- function(s) {
    m <- regmatches(s, regexpr("[0-9]+(\\.[0-9]+)?", s))
    if (length(m) == 0) NA_real_ else as.numeric(m)
  }
  prefix <- trimws(sub("[0-9].*$", "", labels))
  vals <- vapply(labels, first_num, numeric(1))
  tab <- table(prefix[!is.na(vals)])
  if (length(tab) == 0) {
    return(list(ok = FALSE, source = "labels",
                why = "no arm declares a setting and no label carries a number"))
  }
  modal <- names(tab)[which.max(tab)]
  keep <- !is.na(vals) & prefix == modal
  if (sum(keep) < 3) {
    return(list(ok = FALSE, source = "labels", why = paste0("only ", sum(keep),
      " arm(s) share the modal label prefix '", modal, "'; a ladder needs 3")))
  }
  if (anyDuplicated(vals[keep])) {
    return(list(ok = FALSE, source = "labels", why = paste0(
      "two arms parse to the same setting (",
      paste(vals[keep][duplicated(vals[keep])], collapse = ", "),
      "), so the labels do not order the family — often a 2-D sweep")))
  }
  # Named by label, because schedule_test() looks each arm's setting up by the
  # variant string rather than by position.
  names(vals) <- labels
  names(keep) <- labels
  list(ok = TRUE, source = "labels", keep = keep, vals = vals)
}

sched <- list()

for (b in names(blocks)) {
  bi <- blocks[[b]]
  hr()
  note(b)
  note("  baseline: ", bi$baseline)
  note("  ", bi$seasons, " seasons; ", bi$state)
  if (isTRUE(bi$unusable)) {
    note("  NO USABLE ARM — every arm had fewer than 2 pairable cells on HOLD.")
    note("  Reported rather than skipped: a block that vanishes silently while")
    note("  the header still counts it is indistinguishable from one that ran.")
    next
  }
  if (bi$theorem) {
    note("  THEOREM — every arm is byte-identical on HOLD, so this knob does not")
    note("  reach the metric at all. Not screened, and not counted in the family.")
    next
  }
  q <- res[res$block == b, ]
  q <- q[order(q$variant_index), ]
  cat(sprintf("  %-34s %s %9s %9s %7s %7s %9s %7s %7s\n", "arm",
              paste(sprintf("%8s", paste0("GW", ENTRIES)), collapse = ""),
              "early", "late", "late-early", "p", "GW1-rest", "t", "p"))
  for (i in seq_len(nrow(q))) {
    cat(sprintf("  %-34s %s %9s %9s %7s %7s %9s %7s %7s\n",
                substr(q$variant[i], 1, 34),
                paste(vapply(ENTRIES, function(g) num(q[[paste0("gw", g)]][i], 3, 8),
                             character(1)), collapse = ""),
                num(q$early[i], 3, 9), num(q$late[i], 3, 9),
                num(q$contrast[i], 3, 7), num(q$p[i], 3, 7),
                num(q$gw1_vs_rest[i], 3, 9), num(q$gw1_t[i], 2, 7),
                num(q$gw1_p[i], 3, 7)))
  }

  # The ordering statistic. The baseline sits at exactly 0 in both columns by
  # construction and is included, because "the shipped value is best early and
  # third late" is precisely the shape being screened for.
  e <- c(0, q$early); l <- c(0, q$late)
  rho <- suppressWarnings(cor(e, l, method = "spearman"))
  lab <- c(bi$baseline, q$variant)
  best_e <- lab[which.max(e)]; best_l <- lab[which.max(l)]

  # The ladder slope per entry column, which is the statistic the item's own
  # words describe. The baseline enters at exactly 0 with its own setting.
  ld <- ladder_of(lab, bi$settings)
  slopes <- rep(NA_real_, length(ENTRIES))
  if (isTRUE(ld$ok)) {
    for (k in seq_along(ENTRIES)) {
      y <- c(0, q[[paste0("gw", ENTRIES[k])]])
      slopes[k] <- suppressWarnings(
        cor(ld$vals[ld$keep], y[ld$keep], method = "spearman"))
    }
    cat(sprintf("  %-38s %s\n", "ladder slope (rho: setting vs gain)",
                paste(vapply(slopes, function(z) num(z, 3, 8), character(1)),
                      collapse = "")))
    note("    over ", sum(ld$keep), " arms at settings ",
         paste(format(ld$vals[ld$keep]), collapse = ", "))
    # Which licensed the slope. A reader cannot otherwise tell a design-declared
    # ladder from one inferred off label text, and the whole point of the column
    # is that those are different claims.
    if (identical(ld$source, "column")) {
      note("    settings DECLARED by the sweep — the ladder is licensed by the design")
    } else {
      note("    ⚠️ settings PARSED FROM LABELS — this bank predates the `setting` ",
           "column, so read the slope as licensed by the labels rather than by ",
           "the design")
    }
    bset <- if (isTRUE(ld$keep[[bi$baseline]])) ld$vals[[bi$baseline]] else NA_real_
    stt <- schedule_test(bi$arms, bset, ld, ENTRIES)
    if (is.null(stt)) {
      note("  schedule test: not estimable on this bank")
    } else {
      note("  SCHEDULE TEST — does the ladder slope trend with entry point?")
      note("    d(slope)/d(entry gw) = ", num(stt$estimate, 4, 1),
           "   t = ", num(stt$t, 2, 1), "   p = ", num(stt$p, 3, 1),
           "   df = ", num(stt$df, 1, 1), " (", stt$n_seasons, " seasons)")
      note("    GW1-to-GW26 swing over the ladder's span: ", num(stt$gap, 1, 1),
           " a season; p=0.05 threshold ", num(stt$thr05, 1, 1),
           ", 80%-power MDE ", num(stt$mde80, 1, 1))
      sched[[length(sched) + 1]] <- data.frame(
        sweep = sub(".* / ", "", b), estimate = stt$estimate, t = stt$t,
        p = stt$p, df = stt$df, n_seasons = stt$n_seasons,
        gap = stt$gap, thr05 = stt$thr05, mde80 = stt$mde80,
        stringsAsFactors = FALSE)
    }
  } else {
    note("  ladder slope: n/a — ", ld$why)
    note("  so neither the slope nor the schedule test has a referent here.")
  }

  note("  ordering across settings: Spearman rho(early, late) = ", num(rho, 3, 1))
  note("  best arm early: ", best_e)
  note("  best arm late:  ", best_l)
  flags <- character(0)
  sgn <- sign(slopes[!is.na(slopes) & slopes != 0])
  if (length(sgn) > 1 && length(unique(sgn)) > 1) {
    flags <- c(flags, paste0("ladder slope changes sign (", sum(sgn > 0),
                             " positive, ", sum(sgn < 0),
                             " negative) — DESCRIPTIVE, see the schedule test"))
  }
  if (!is.na(rho) && rho < 0) flags <- c(flags, "pooled early/late ordering reverses (rho < 0)")
  if (!identical(best_e, best_l)) flags <- c(flags, "argmax moves between columns")
  if (any(!is.na(q$p) & q$p < 0.05)) flags <- c(flags, "an arm's late-early contrast has raw p < 0.05")
  if (length(flags) > 0) {
    note("  >> FLAG: ", paste(flags, collapse = "; "))
  } else {
    note("  no flag")
  }
}

hr()
if (length(sched) > 0) {
  sc <- do.call(rbind, sched)
  sc$p_holm <- p.adjust(sc$p, method = "holm")
  note("SCHEDULE TEST, every ordered ladder in the bank.")
  note("The estimate is the change in the ladder's slope per gameweek of entry:")
  note("non-zero means the constant's best value depends on when you enter.")
  note("Holm is over the ", nrow(sc), " ladders tested.")
  note("")
  note("⚠️ The raw estimate is SIX DIFFERENT UNITS and must not be read across")
  note("   rows: BENCH's ladder spans 0.18 and MINK's spans 11. `swing` and")
  note("   `MDE` put every row on one scale — season points, over that ladder's")
  note("   own range, GW1 entry against GW26.")
  cat(sprintf("%-14s %14s %7s %7s %6s %9s %8s %8s %8s\n",
              "sweep", "d(slope)/d(gw)", "t", "p", "df", "p_holm",
              "swing", "thr05", "mde80"))
  o <- sc[order(sc$p), ]
  for (i in seq_len(nrow(o))) {
    cat(sprintf("%-14s %14s %7s %7s %6s %9s %8s %8s %8s\n",
                substr(o$sweep[i], 1, 14), num(o$estimate[i], 4, 14),
                num(o$t[i], 2, 7), num(o$p[i], 3, 7), num(o$df[i], 1, 6),
                num(o$p_holm[i], 3, 9), num(o$gap[i], 1, 8),
                num(o$thr05[i], 1, 8), num(o$mde80[i], 1, 8)))
  }
  note("")
  note("The threshold columns are the finding. Against AGENTS.md's own median")
  note("detectable of 39 points a season, this screen is 4-9x blunter at p=0.05")
  note("and 5-12x at 80% power, so 'nothing resolves' was close to guaranteed by")
  note("the design. ⚠️ swing/thr05 is IDENTICALLY t/t_crit, so the two are one")
  note("number in two costumes -- read them for scale, never as corroboration.")
  note("⚠️ And do NOT import the recorded")
  note("prior that an interaction is the CHEAP contrast: that holds for a")
  note("difference of differences WITHIN one cell, which cancels path divergence.")
  note("This one is formed BETWEEN cells, across entry points that are different")
  note("squads scoring different football, so none of that cancellation applies.")
  hr()
}
note("Ranked by the late-minus-early contrast, largest first.")
# The family size R actually corrected over, not the row count. p.adjust drops
# NA p-values before applying Holm — a byte-identical arm inside an otherwise
# live sweep yields one — so printing nrow(res) would overstate the family
# whenever that happens. No committed sweep has such an arm today, which makes
# this untested rather than wrong, which is the reason to compute it rather than
# assume the two agree.
note("Holm is over the ", sum(!is.na(res$p)), " arm contrasts with a p-value",
     if (any(is.na(res$p))) paste0(" (", sum(is.na(res$p)),
       " byte-identical arm(s) carry none and are not in the family)") else "",
     ".")
cat(sprintf("%-26s %-30s %10s %7s %7s %8s\n",
            "sweep", "arm", "late-early", "t", "p", "p_holm"))
o <- res[order(-abs(res$contrast)), ]
for (i in seq_len(nrow(o))) {
  cat(sprintf("%-26s %-30s %10s %7s %7s %8s\n",
              substr(sub(".* / ", "", o$block[i]), 1, 26),
              substr(o$variant[i], 1, 30),
              num(o$contrast[i], 3, 10), num(o$t[i], 2, 7),
              num(o$p[i], 3, 7), num(o$p_holm[i], 3, 8)))
}

hr()
note("Read this as a screen. A flag buys a pre-registered re-run of that one")
note("constant; it is not a verdict, and the contrast's SIZE is attenuated by")
note("the entry columns being nested suffixes of one season. See this file's")
note("header for the three limitations that constrain the reading.")

if (!is.null(opt_out)) {
  dir.create(dirname(opt_out), showWarnings = FALSE, recursive = TRUE)
  write.csv(res, opt_out, row.names = FALSE)
  note("")
  note("wrote ", opt_out)
}
