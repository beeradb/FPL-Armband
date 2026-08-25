# Quantities shared by every script that reads the replay's cells CSV.
#
# Why this file exists
#
# AGENTS.md's signature failure is "one quantity, two implementations" — four
# recorded instances, each found only after the two copies had already
# disagreed in print. The paired difference and the cluster-robust standard
# error are exactly that kind of quantity: `sweep_inference.R` computes them
# for its per-arm verdicts and `schedule_screen.R` computes them per entry
# gameweek, and if the two ever diverged the screen's columns would not sum to
# the record's own numbers while looking entirely plausible.
#
# The *quantities* live here. ⚠️ **So does the I/O, from 2026-08-14** — this file
# said the opposite, and the reversal is recorded rather than silently made:
#
#   "So the *quantities* live here and the *I/O* does not. Each script keeps its
#    own reading and validation, which is a dozen lines of coercion whose failure
#    mode is loud; what is shared is the arithmetic whose failure mode is silent."
#
# Both halves were measured and neither holds.
#
# **"Loud" is falsified inside the family it describes.** For the single condition
# *no baseline arm found*, the seven readers did four different things:
# `grid_width.R` aborted, this file returned NULL, `variance_components.R` returned
# an empty list, and `rank_robustness.R` skipped the block and exited 0. A condition
# with four loudnesses, one of them silent success, is not a loud failure mode.
#
# **And the reading was hiding the worst defect of the lot.** `grid_width.R` keyed
# cells on `season@start_gw` and never blocked on `(run_id, sweep)` — so on a file
# holding several runs, which the cells file is opened for *append* to allow, every
# arm was paired against every block's baseline. Measured on the banked three-block
# `MINHL` files: n went 24 to 72, each arm's mean moved about 0.14 pts/gw, and the
# naive standard error shrank by root three. No error, no warning. That is the
# failure `rank_robustness.R`'s own comment calls "over-confident by 41%", surviving
# in a sibling because the reading was nine copies rather than one.
#
# ⚠️ **Corrected 2026-08-14, having first been written as "three different
# experiments".** It is three blocks, of which TWO CARRY A BYTE-IDENTICAL BASELINE and one does not.
# `MINHL#1` is a lone baseline arm and not an experiment at all, `MINHL#2` is the
# half-life ladder against the same baseline (max abs diff 0 across the same 24
# cells), and `MINHL#3` is the vice-captain pair against `vice off (pre-fix)`,
# 52.4022 against 52.8324. **The whole 0.1434 comes from the third block**, and the
# arithmetic is exact: the shift an arm takes is its own baseline's mean minus the
# pooled baselines' mean.
#
# ⚠️ Two things follow, and the second is why this was invisible. The shift is
# CONSTANT WITHIN A BLOCK, so the ladder's ordering and its arm-to-arm gaps
# survived intact — all four MINHL#2 arms moved by exactly +0.14338 — while every
# level, SE and t moved. And a file whose blocks SHARE a baseline shows n tripling
# with no mean moving at all. So "two-thirds of every difference is taken against
# the wrong baseline" describes the mechanism correctly and overstates the damage:
# the wrong baselines were two-thirds of them, and one-third OF THOSE was wrong.
#
# Nothing in the Go test suite invokes this, and `go build`, `go vet` and
# `go test` must all pass on a machine with no R installed.

fail <- function(...) stop(paste0("CONTRACT VIOLATION: ", ...), call. = FALSE)
note <- function(...) cat(paste0(..., "\n"))
hr <- function() cat(strrep("-", 78), "\n", sep = "")

# as_flag accepts every spelling of a boolean the writer could emit.
#
# ⚠️ The comment here used to read "read.csv turns 'true'/'false' into logical
# already, but be explicit". **That is false, and was measured false on 2026-08-14**:
# R's `type.convert` leaves lowercase `true`/`false` as CHARACTER, and turns
# `TRUE`/`FALSE` into logical and `1`/`0` into integer. The function was right and
# its stated reason was backwards.
#
# The correction matters because it inverts the risk. The family of readers agreed
# for years *because* the column arrives as character, so every `== "true"` in the
# tree happened to work — not because they were careful. `internal/backtest`'s
# writer emits `strconv.FormatBool`, always lowercase, so the logical column is
# currently unreachable from this pipeline; the day it is not, `== "true"` returns
# zero baselines and the script reports nothing rather than failing.
as_flag <- function(x) {
  if (is.logical(x)) return(x)
  tolower(as.character(x)) %in% c("true", "t", "1")
}

# --- reading a cells file ----------------------------------------------------

# cellsRequired is the contract every reader depends on. Anything outside it is
# optional by design: a cells file written before a column existed must stay
# readable, which is why `setting`, `oracle` and the captaincy rungs are absent
# from this list even though several scripts use them.
cellsRequired <- c("sweep", "run_id", "variant", "variant_index", "is_baseline",
                   "season", "start_gw", "weeks", "infeasible")

# read_cells reads one cells CSV, coerces it, keys it and checks its contract.
#
# # The key is the whole point, and it is not the separator
#
# `cell` is `(run_id, sweep, season, start_gw)`. The audit that led here recorded
# the readers as differing on the key's *separator* — `@` against a space — which is
# cosmetic. What bites is the *field list*: `grid_width.R` keyed on
# `season@start_gw` alone and never blocked, so on a multi-run file `merge` paired
# every arm against every block's baseline.
#
# Making the key wide fixes that **arithmetically, for a caller who forgets to
# block**, which a validation cannot: a check can be skipped by not calling it,
# whereas rows from two blocks simply no longer join. Callers that do block are
# unaffected, because within one block the extra fields are constant.
#
# `label` is the human-readable `season@start_gw` and is what belongs in output.
# Keeping them apart is deliberate — the merge key and the display name are
# different quantities, and conflating them is what made the wide key look like a
# formatting choice.
#
# # The checks are schedule_screen.R's, and they cost nothing
#
# Verified on the whole bank before promoting them: 72 files, 7,320 rows, 82 blocks,
# **zero failures**. They refuse a file with a missing required column, with other
# than exactly one baseline arm per block, with `is_baseline` disagreeing with
# `variant_index == 0`, or whose `hold_per_gw` is not `hold_points / weeks`. The
# last is the one that catches a file written under a different scale convention,
# which would otherwise read as a real effect.
read_cells <- function(p) {
  if (!file.exists(p)) fail("no such file: ", p)
  d <- read.csv(p, stringsAsFactors = FALSE, na.strings = c("", "NA"))

  miss <- setdiff(cellsRequired, names(d))
  if (length(miss) > 0) {
    fail(p, " is missing columns: ", paste(miss, collapse = ", "))
  }

  d$is_baseline <- as_flag(d$is_baseline)
  d$infeasible <- as_flag(d$infeasible)
  d$start_gw <- as.numeric(d$start_gw)
  d$variant_index <- as.numeric(d$variant_index)

  d$block <- paste(d$run_id, d$sweep, sep = " / ")
  d$cell <- paste(d$run_id, d$sweep, d$season, d$start_gw)
  d$label <- paste(d$season, d$start_gw, sep = "@")
  d$src <- p

  for (b in unique(d$block)) {
    q <- d[d$block == b, ]
    nb <- length(unique(q$variant[q$is_baseline]))
    if (nb != 1) {
      fail(p, ": block ", b, " has ", nb, " baseline arm(s), not 1. ",
           "Every paired difference is taken against the baseline, so this would ",
           "silently pair each arm against a doubled baseline set.")
    }
    bad <- q$is_baseline != (q$variant_index == 0)
    if (any(bad, na.rm = TRUE)) {
      fail(p, ": block ", b, " disagrees between is_baseline and ",
           "variant_index == 0 on ", sum(bad, na.rm = TRUE), " row(s).")
    }
  }

  # The per-gameweek column must be the cell total over gameweeks played, on
  # every pair the file happens to carry.
  #
  # ⚠️ **Checked pair by pair, and each one only WHEN PRESENT.** This checked
  # `hold` alone until the accumulated-xPoints columns arrived, and the temptation
  # then is to require the new pair — which would make every banked cells file
  # unreadable, since none of them has it. Optional-when-absent is the same rule
  # `cellsRequired` states for the captaincy rungs and the oracle stamp: a file
  # written before a column existed must stay readable.
  #
  # The names are spelled out rather than derived from a `<metric>_points`
  # convention because the xPoints totals do not follow one — the column is
  # `hold_xpoints`, not `hold_xpoints_points` — and inferring the pair from a
  # naming rule is how a column silently drops out of a check that still reports
  # "ok".
  per_gw_pairs <- list(
    c("hold_points", "hold_per_gw"),
    c("policy_points", "policy_per_gw"),
    c("hold_xpoints", "hold_xpoints_per_gw"),
    c("policy_xpoints", "policy_xpoints_per_gw")
  )
  for (pr in per_gw_pairs) {
    tot <- pr[1]
    per <- pr[2]
    # (!) A HALF-pair is a defect, not an optional column. Both absent means an
    # older bank and is fine; exactly one present means something dropped or
    # renamed a column, and letting it through here produced a false "ok" in
    # sweep_inference's own contract banner -- the check compared zero rows and
    # then announced the metric as verified. Found by code review of the pilot
    # build; the same hole predated it on the captaincy rungs.
    if (is.null(d[[tot]]) && is.null(d[[per]])) next
    if (is.null(d[[tot]]) || is.null(d[[per]])) {
      fail(p, ": carries ", if (is.null(d[[tot]])) per else tot,
           " without its pair ", if (is.null(d[[tot]])) tot else per,
           " — a half-pair means a column was dropped or renamed, and every ",
           "check downstream of it would pass vacuously.")
    }
    fs <- !d$infeasible & !is.na(d$weeks) & d$weeks > 0 & !is.na(d[[per]])
    if (!any(fs)) next
    lhs <- as.numeric(d[[per]][fs])
    rhs <- as.numeric(d[[tot]][fs]) / as.numeric(d$weeks[fs])
    off <- abs(lhs - rhs) > 1e-6 * pmax(1, abs(rhs))
    if (any(off, na.rm = TRUE)) {
      fail(p, ": ", per, " is not ", tot, "/weeks on ",
           sum(off, na.rm = TRUE), " row(s) — the scale quoted is not the one ",
           "in the file.")
    }
  }
  d
}

# read_sidecar reads a CSV from this pipeline that is NOT a cells file — the
# `.means.csv` Go writes beside a sweep, and the `.provenance.csv` that records its
# data state.
#
# It exists so those readers have a sanctioned home too. The guard that keeps this
# family together refuses `read.csv(` anywhere in `stats/*.R` outside this file, and
# a blanket ban with no alternative would just be routed around. Same coercion
# defaults as read_cells — `stringsAsFactors = FALSE` and `na.strings` — because the
# character-column case is where `na.strings` actually matters and a sidecar is
# mostly character.
read_sidecar <- function(p) {
  if (!file.exists(p)) fail("no such file: ", p)
  read.csv(p, stringsAsFactors = FALSE, na.strings = c("", "NA"))
}

# read_cells_all reads several files and stacks them, keeping only the columns
# they share. Snapshots banked weeks apart carry different optional columns — the
# captaincy rungs and the variance ladder arrived later — and rbind on the union
# fails on a schema difference that has nothing to do with the question asked.
read_cells_all <- function(paths) {
  parts <- lapply(paths, read_cells)
  keep <- Reduce(intersect, lapply(parts, names))
  # ⚠️ Said out loud. `rbind` on differing schemas used to ABORT, and taking the
  # intersection instead trades a loud failure for a silent narrowing: a
  # mixed-vintage run over several snapshots would quietly answer a narrower
  # question than the one asked — the captaincy rungs vanishing from BOTH files
  # because one of them predates the column. The intersection is still right; the
  # silence was not.
  dropped <- setdiff(Reduce(union, lapply(parts, names)), keep)
  if (length(dropped) > 0) {
    note("  !!  ", length(paths), " file(s) carry different columns, so ",
         length(dropped), " were dropped from ALL of them: ",
         paste(dropped, collapse = ", "))
    note("      A figure that needs one of those is now unmeasurable rather ",
         "than absent from one file.")
  }
  do.call(rbind, lapply(parts, function(d) d[, keep, drop = FALSE]))
}

# --- the paired difference --------------------------------------------------
#
# One arm minus the baseline arm *within the same cell* — same season, same
# entry gameweek, same football, one thing changed. `suffix` has no default on
# purpose: per_gw and per_path are different estimands rather than two units
# for one number, and a caller that has not decided which it wants should not
# be handed one silently.
diffs_for <- function(d, metric, suffix, min_cells, quiet = FALSE) {
  col <- paste0(metric, suffix)
  if (!(col %in% names(d))) fail("no such metric column: ", col)
  base <- d[d$is_baseline, ]
  if (nrow(base) == 0) return(NULL)
  out <- list()
  for (v in unique(d$variant[!d$is_baseline])) {
    arm <- d[d$variant == v, ]
    j <- merge(
      base[, c("cell", "season", "start_gw", "weeks", col)],
      arm[, c("cell", col)],
      by = "cell", suffixes = c(".base", ".arm")
    )
    j$diff <- j[[paste0(col, ".arm")]] - j[[paste0(col, ".base")]]
    j <- j[!is.na(j$diff), ]
    if (nrow(j) < min_cells) {
      # Named, not silently absent. Go's own table drops such an arm too, so
      # without this an arm that was mostly infeasible disappears from both
      # outputs and reads as an arm nobody ran. Suppressed on the repeat calls
      # that build plots and the regime table, so it is said once.
      if (!quiet) {
        note("  !!  ", v, ": only ", nrow(j), " usable cell(s), below this ",
             "estimator's floor of ", min_cells, " — no inference possible. ",
             "Check the infeasible listing above.")
      }
      next
    }
    out[[v]] <- data.frame(
      variant = v,
      variant_index = arm$variant_index[1],
      season = j$season, start_gw = j$start_gw,
      # `weeks` is carried because variance_components.R weights by it. It is
      # additive for every other caller, which is why it is unconditional rather
      # than another parameter.
      weeks = j$weeks, diff = j$diff,
      stringsAsFactors = FALSE
    )
  }
  # Ordered by the arm's index, which is the order the sweep declared them in and
  # the order every table in this repo prints. variance_components.R's selftest
  # asserts it; the others sort or index by name, so ordering here changes none of
  # their output — checked before making it unconditional.
  out <- out[order(vapply(out, function(x) x$variant_index[1], numeric(1)))]
  out
}

# --- the cluster-robust standard error --------------------------------------

have <- function(pkg) requireNamespace(pkg, quietly = TRUE)
has_cs <- have("clubSandwich")
has_lmer <- have("lmerTest") && have("lme4")

# A byte-identical arm is common and expected here — a transfer knob must leave
# HOLD untouched, and that invariance is the check rather than a degeneracy to
# hide. Every SE below returns NA on one instead of erroring.
degenerate <- function(d) {
  s <- suppressWarnings(sd(d$diff))
  !is.finite(s) || isTRUE(all.equal(s, 0, tolerance = 0))
}

# CR2: cluster-robust with the bias-reduced small-sample correction and
# Satterthwaite df. With four to six seasons the df is small, and CR2 is what
# makes that honest rather than merely pessimistic — the df it resolves is
# *reported* rather than asserted.
se_cr <- function(d, cluster) {
  if (!has_cs) return(list(se = NA, df = NA, t = NA, p = NA))
  if (length(unique(cluster)) < 2 || degenerate(d)) {
    return(list(se = NA, df = NA, t = NA, p = NA))
  }
  tryCatch({
    fit <- lm(diff ~ 1, data = d)
    ct <- clubSandwich::coef_test(fit, vcov = "CR2", cluster = cluster,
                                 test = "Satterthwaite")
    list(se = ct$SE[1], df = ct$df_Satt[1], t = ct$tstat[1], p = ct$p_Satt[1])
  }, error = function(e) list(se = NA, df = NA, t = NA, p = NA))
}

se_cr2 <- function(d) se_cr(d, d$season)

# The same estimator clustered on the ENTRY POINT rather than the season. It is
# a robustness check on the season clustering rather than a rival estimate to
# prefer on its own — see the long note at its call site in sweep_inference.R.
se_cr2_start <- function(d) se_cr(d, d$start_gw)

# --- what an estimator can see ----------------------------------------------
#
# `sig` is the effect that would land exactly at p = alpha: anything smaller
# cannot be called significant however cleanly it was measured. `mde` is larger,
# and is the effect the design would actually *find* `power` of the time. The gap
# between them is the difference between "would clear the bar if we saw it" and
# "would reliably see it".
#
# The t-quantile approximation to the non-central t is adequate here and is
# stated so it can be checked. `variance_components.R`'s `--selftest` pins the
# df = 3 multiple at 4.1609 and is the regression test for this function.
#
# ⚠️ **It returns a bare vector and no units.** `mde_row` wraps it for the cells
# family, where the scale is points per gameweek and the season figure is x38;
# a caller whose quantity is a unitless ratio — `blank_run_position.R` — must not
# inherit that scaling, and returning the raw numbers is what keeps the two from
# sharing a multiplier that is right for only one of them.
sig_and_mde <- function(se, df, alpha = 0.05, power = 0.80) {
  tcrit <- qt(1 - alpha / 2, df)
  c(t_crit = tcrit, sig = tcrit * se, mde = (tcrit + qt(power, df)) * se)
}

# --- the wild cluster bootstrap ----------------------------------------------
#
# # What this is, stated narrowly, because the first two drafts overstated it
#
# A replacement reference distribution for the test of `mean(diff) = 0`, built by
# reweighting whole SEASONS. It **swaps an unverifiable normality assumption on
# four-to-six season means for a symmetry assumption on them.** That is the whole
# claim. Three things it was drafted as and is not:
#
# ⚠️ **It does not repair a Satterthwaite approximation, because on this bank
# there is no approximation to repair.** For a balanced intercept-only design the
# CR2 season-clustered t **is identically the equal-weighted t-test on the season
# means** — same t, same df = S-1, same p, bit for bit. Verified on the worked
# case (both 3.9368849084 on df 5, p 0.0109941497) and on the armband (both
# 20.42726 on df 3). This grid is balanced six-per-season essentially everywhere.
# So do NOT write that CR2 "weights by cluster leverage" while the bootstrap
# "tracks the equal-weighted season means" — an earlier draft of this comment did,
# and it invites a reader to explain away a disagreement that cannot arise from
# that cause.
#
# ⚠️ **It does not correct an over-sized CR2 t.** That was the commissioning
# premise and it does not survive measurement. Under a true null, 400 reps at 6
# clusters, rejection at nominal 0.05 / 0.10:
#
#     cluster-level noise        CR2 t            this bootstrap
#     gaussian                   0.055 / 0.120    0.060 / 0.133
#     t2 (heavy)                 0.055 / 0.100    0.073 / 0.135
#     cauchy                     0.030 / 0.060    0.040 / 0.080
#     one outlier season         0.025 / 0.045    0.053 / 0.095
#
# CR2 is never over-sized here; under heavy cluster-level tails it goes
# CONSERVATIVE, and the bootstrap is the more liberal of the two. So this
# statistic **rescues more often than it retracts**. ⚠️ 400 reps puts the Monte
# Carlo SE near 0.011, so read that table as "no evidence either is mis-sized"
# rather than as an ordering, and note the design is balanced and homoskedastic.
#
# ⚠️ **It cannot see the cell-level heavy tails that motivated it — exactly, not
# approximately.** `t*` depends on the data only through the cluster totals `T_g`
# and sizes `n_g`: the mean is `(1/n) sum_g w_g T_g` and each cluster's CR2
# contribution is `w_g T_g - n_g m`. So the statistic AND its whole bootstrap
# distribution are functions of the season totals alone, and are **invariant to
# any redistribution of the differences within a season**. Measured: replacing
# each season's six cells with (season total, 0, 0, 0, 0, 0) — which wrecks the
# cell-level kurtosis and preserves the totals — moves neither the CR2 t nor the
# bootstrap p at all. Two corollaries, both limitations a reader will otherwise
# assume away: pooled excess kurtosis of +3.46 across cells is invisible here, and
# a season carrying its effect in one cell of six is indistinguishable from a
# season with six equal cells. That second one is precisely what `pos` catches, so
# `p wild` must be read WITH `pos` and neither substitutes for the other.
#
# # The asymmetric rule, which is what makes "diagnostic" operational
#
# **It may withdraw support from a CR2 rejection. It may NOT grant one.** Without
# a direction this is just a second p-value, and a second p-value read in either
# direction is p-shopping — which matters here because the one headline case where
# it crosses 0.05 upward (`flat (no recency)` on POLICY, CR2 0.0781 -> 0.0445) is
# also the case most tempting to quote as proof the tool works. Never Holm-adjust
# it beside `p raw`. And **no MDE, threshold or "points a season" can be derived
# from it** — this record's convention is `t_crit(df) x SE x 38` and a bootstrap p
# carries no SE.
#
# # Webb 6-point weights, not Rademacher
#
# Rademacher (+/-1 at 1/2) is wrong here, and `variance_components.R:1300` already
# says why in the section headed "the Rademacher floor at four clusters": support
# 2^S, and the statistic is symmetric under a global flip, so the smallest
# attainable two-sided p is 2/2^S. **That section is an argument AGAINST the
# statistic at four clusters, not a helper to lift.** At S = 5 the floor is 0.0625
# and at S = 4 it is 0.125, so on the four-season grid that is most of this bank
# nothing could ever reject — including the perfect armband at CR2 t 20.4.
#
# MacKinnon and Webb's six-point distribution puts mass 1/6 on each of
# +/-sqrt(1.5), +/-1, +/-sqrt(0.5). Mean 0 and variance 1, which is what the wild
# bootstrap requires. ⚠️ Its third moment is **0, not 1**: it is symmetric, so it
# deliberately forgoes Mammen's skewness match, and an earlier draft here said
# E[w^3] = 0 was "what the wild bootstrap needs", which is backwards. The
# Davidson-Flachaire argument is that symmetry is the better trade with few
# clusters, and that is the ground this ships on.
#
# # The floor is 6/6^S_eff, and the "2" was wrong
#
# The CR2 t is homogeneous of degree one in `y`, so ANY draw giving every movable
# cluster the SAME weight returns `|t*| = |t|`. Webb has six weight values, so six
# such draws exist — not two. Carrying Rademacher's 2 across was an error, found
# in review and confirmed by enumeration (exactly 6 draws tie, at every S tested).
#
#     S_eff = 2 -> 0.1667     cannot reject at 0.05
#     S_eff = 3 -> 0.02778    three usable gradations below 0.05
#     S_eff = 4 -> 0.00463
#     S_eff = 5 -> 0.000772
#     S_eff = 6 -> 0.0001286
#
# ⚠️ **Read the S = 4 line as a statement about dynamic range, not comfort.** The
# largest effect in this entire record — the perfect armband, CR2 t 20.4, +209.9 a
# season — enumerates to 12/1296 = 0.00926 on HOLD. So on the four-season grid
# this column is a **season-agreement statistic with a handful of usable
# gradations**, not a magnitude statistic, and Webb restored dynamic range rather
# than making the four-season grid fully resolvable.
#
# `S_eff` counts seasons carrying a non-zero difference, because a season of exact
# zeros is unchanged by any weight and sets no part of the reference distribution.
# An arm whose floor exceeds alpha is reported inert **with its own word** rather
# than given a p it could not have reached. Three different conditions hide under
# one heading and AGENTS.md separates them, so this does too:
#
#   identical    - byte-identical to the baseline. For a transfer knob on HOLD
#                  that is the invariance check PASSING, not a degeneracy.
#   pinned       - one movable season. Every draw returns |t*| = |t|, so p is 1
#                  by construction and measures nothing.
#   underpowered - two movable seasons. The p is genuinely variable over 36
#                  support points and does carry information about whether the
#                  two agree; it simply cannot reach 0.05.
#
# # Enumerated exactly, so there is no seed and no Monte Carlo error
#
# 6^S is 46,656 at six clusters, and through `cr2_t_fast` the whole support
# evaluates in **0.054 s** — 1.6x the cost of a 9,999-draw sample, and 0.13x at
# four clusters. So this enumerates. An earlier draft sampled at a fixed seed and
# carried `WCB_B`, an RNG save/restore, a Davidson-MacKinnon `(1+count)/(B+1)`
# correction and a Monte Carlo floor conflated with the exact one; enumeration
# deletes all five. The p is exact, `count / 6^S_eff`, with the observed statistic
# already among the counted draws. Above the cap this FAILS rather than quietly
# switching estimator.

# Webb's six-point distribution: mass 1/6 on each point, mean 0, variance 1,
# third moment 0 (symmetric; no Mammen skewness match, by choice).
WCB_WEBB <- c(-sqrt(1.5), -1, -sqrt(0.5), sqrt(0.5), 1, sqrt(1.5))
# The enumeration cap. 6^7 = 279,936 draws is about 0.4 s; the bank has never held
# more than six seasons. Above this the honest answer is that the sampled
# estimator is not built, not a silent switch to one.
WCB_MAX_CLUSTERS <- 7L
# Columns evaluated at once, to bound peak memory at roughly n x this x 8 bytes.
WCB_CHUNK <- 20000L
# The level a floor is compared against when deciding whether an arm can be
# measured at all.
WCB_ALPHA <- 0.05

# The CR2 t for `y ~ 1`, in closed form, vectorised over columns of `Y`.
#
# ⚠️ **This is a second implementation of a quantity clubSandwich already
# computes, which is this record's signature failure — so it is guarded rather
# than trusted.** `wild_cluster_p` calls `se_cr` (the shared, clubSandwich path)
# for the observed statistic and then requires THIS function to reproduce it on
# the same data before any enumerated draw is used. A divergence aborts.
#
# It exists for speed, measured rather than guessed: clubSandwich's `coef_test`
# costs 1.47 ms on a 36-row six-cluster fit, so enumerating 46,656 draws through
# it would take about 69 seconds per arm against 0.054 s here.
#
# The algebra is exact. For X = 1_n the hat block is H_gg = J_g/n, whose only
# non-zero eigenvalue is n_g/n on the constant direction, so
# (I - H_gg)^(-1/2) 1_g = (1 - n_g/n)^(-1/2) 1_g and each cluster's contribution
# collapses to its residual SUM rescaled:
#
#     SE^2 = (1/n^2) * sum_g  S_g^2 / (1 - n_g/n),   S_g = sum of (y - ybar) in g
#
# Checked against clubSandwich on 300 random UNBALANCED designs with S in 2..8:
# worst absolute difference 4.3e-13. Re-checked on 200 more with S in 2..14 and
# the same character-sorted key indexing `wild_cluster_p` uses, because that is
# where a `rowsum` group-ordering mismatch would hide and it cannot show up at
# S < 10: worst absolute difference 5.9e-14.
cr2_t_fast <- function(Y, cl_index, n_g) {
  n <- nrow(Y)
  m <- colMeans(Y)
  U <- Y - rep(m, each = n)
  # rowsum() sums by cluster; one row per cluster, one column per draw. The index
  # is integer, and rowsum orders integer groups NUMERICALLY, so its rows line up
  # with n_g's positions even above nine clusters.
  S <- rowsum(U, cl_index, reorder = TRUE)
  ss <- colSums(S^2 / as.numeric(1 - n_g / n))
  m / sqrt(ss / n^2)
}

wild_cluster_p <- function(d, cluster) {
  # ⚠️ `S`, `S_eff` and `floor` are carried out of EVERY return that knows them,
  # the inert ones included. An earlier draft returned NA for all three whenever
  # an arm was inert, which defeated the point of having them: the report tells a
  # reader to find unmeasurable arms with `wild_floor > 0.05`, and that filter
  # returned an EMPTY SET precisely because those rows were NA. The floor was
  # printed in English prose and nowhere a machine could read it — the same
  # defect, one layer along, as printing `p = 1` for a pinned arm.
  out <- function(word, reason, S = NA_integer_, S_eff = NA_integer_,
                  floor = NA_real_) list(
    p = NA_real_, S = S, S_eff = S_eff, floor = floor,
    t_obs = NA_real_, inert = word, why = reason
  )
  if (!has_cs) return(out("nopkg", "clubSandwich not installed — no CR2 t to bootstrap"))

  # Preconditions, checked rather than recycled around. A cluster vector of the
  # wrong length would be silently recycled by `==` below; an NA difference would
  # make the movable-cluster test neither TRUE nor FALSE.
  if (length(cluster) != nrow(d)) {
    fail("wild_cluster_p got ", length(cluster), " cluster labels for ", nrow(d),
         " rows; the clustering would be recycled rather than applied.")
  }
  if (any(is.na(d$diff))) {
    fail("wild_cluster_p got ", sum(is.na(d$diff)), " NA difference(s); drop them ",
         "before clustering, or the effective cluster count is undefined.")
  }

  cluster <- as.character(cluster)
  keys <- sort(unique(cluster))
  S <- length(keys)

  # Computed up front so every return below, inert ones included, can report it.
  # Six draws give every movable cluster the same weight and so return
  # |t*| = |t| exactly; that is the floor.
  movable <- keys[vapply(keys, function(k) any(d$diff[cluster == k] != 0),
                         logical(1))]
  S_eff <- length(movable)
  flr <- min(1, 6 / (6^S_eff))

  if (S < 2) {
    return(out("pinned", paste0("only ", S, " cluster — a cluster bootstrap needs 2"),
               S, S_eff, flr))
  }
  if (degenerate(d)) {
    return(out("identical", if (all(d$diff == 0))
      "every difference is exactly zero — byte-identical to the baseline"
    else
      "every difference is identical, so the residuals carry no variation",
      S, S_eff, flr))
  }

  obs <- se_cr(d, cluster)
  if (!is.finite(obs$t)) {
    return(out("nofit",
               "the CR2 fit did not return a t, so there is nothing to compare against",
               S, S_eff, flr))
  }

  if (S_eff < 2) {
    return(out("pinned", paste0("only ", S_eff, " of ", S, " cluster(s) carry a ",
                                "non-zero difference; every draw returns |t*| = ",
                                "|t|, so p is 1 by construction and measures nothing"),
               S, S_eff, flr))
  }
  if (flr >= WCB_ALPHA) {
    return(out("underpowered",
               paste0("only ", S_eff, " of ", S, " cluster(s) carry a non-zero ",
                      "difference, so the smallest attainable p is 6/6^", S_eff,
                      " = ", format(flr, digits = 3), ", above alpha = ", WCB_ALPHA,
                      " — the p would still rank the two seasons' agreement, but ",
                      "it cannot reject at any effect size"),
               S, S_eff, flr))
  }
  if (S_eff > WCB_MAX_CLUSTERS) {
    return(out("toowide",
               paste0(S_eff, " movable clusters exceeds the enumeration cap of ",
                      WCB_MAX_CLUSTERS, "; the sampled estimator is not built"),
               S, S_eff, flr))
  }

  cl_index <- match(cluster, keys)
  n_g <- as.numeric(table(factor(cluster, levels = keys)))
  y <- d$diff
  mv <- match(movable, keys)

  # The fast path must agree with the shared clubSandwich path on the observed
  # sample before it is trusted for the enumeration.
  chk <- cr2_t_fast(matrix(y, ncol = 1), cl_index, n_g)
  if (!is.finite(chk) || abs(chk - obs$t) > 1e-8 * max(1, abs(obs$t))) {
    fail("the closed-form CR2 t (", chk, ") disagrees with clubSandwich (",
         obs$t, "). The bootstrap would be referring the observed statistic to a ",
         "distribution generated by a different estimator.")
  }

  # WCR: the restricted residual under `diff ~ 1` with the mean set to zero is
  # `diff` itself, so a draw is the difference with each cluster's weight applied.
  # Nothing is re-centred — that is the whole point, and the unrestricted
  # alternative is measurably anti-conservative here (0.00103 against 0.02786 on
  # the worked case, a 27-fold shift).
  #
  # Non-movable clusters keep weight 1: their differences are all zero, so the
  # weight is arithmetically irrelevant and enumerating over them would multiply
  # the work by 6 per season for nothing.
  total <- 6^S_eff
  count <- 0
  ties <- 0
  # `chk`, not `obs$t`: both agree to within the tolerance asserted above, but the
  # draws come from the same estimator as `chk`, so a genuine tie cannot be lost
  # to a last-bit difference between two implementations — and the floor depends
  # on those six ties being counted.
  thr <- abs(chk) * (1 - 1e-9)
  # ⚠️ ABSOLUTE FLOOR, deliberately. The tie tolerance below was purely relative
  # (`1e-9 * abs(chk)`), which collapses to exactly 0 when the observed t is 0 — and
  # then "tied" degenerates into "bit-exactly zero", which floating point does not
  # oblige. Measured 2026-08-24 on MINHL/extended, arm `hold_nocap_points` /
  # `half-life 8`, whose mean is exactly 0: 2 of the 6 came back exact, the guard
  # fired, and the whole run aborted with no output. The homogeneity argument the
  # guard rests on is unaffected — it is the tolerance that was wrong, not the 6.
  #
  # Note what the run was aborting over: with `chk` 0, `thr` is 0 too, every draw
  # satisfies `abs(ts) >= thr`, and p is exactly 1. The guard was killing the run on
  # the one arm whose answer was never in question.
  #
  # For `abs(chk) >= 1` this is identical to the old expression. `ties` feeds the
  # guard and nothing else — `p` comes from `count` and `thr` — so this cannot move
  # a published number.
  tie_tol <- 1e-9 * max(abs(chk), 1)
  for (from in seq(1, total, by = WCB_CHUNK)) {
    to <- min(from + WCB_CHUNK - 1, total)
    idx <- from:to
    # Decode each draw index into its base-6 digits, one row per movable cluster.
    W <- matrix(1, nrow = length(keys), ncol = length(idx))
    q <- idx - 1
    for (j in seq_len(S_eff)) {
      W[mv[j], ] <- WCB_WEBB[(q %% 6) + 1]
      q <- q %/% 6
    }
    ts <- cr2_t_fast(y * W[cl_index, , drop = FALSE], cl_index, n_g)
    if (any(!is.finite(ts))) {
      # Not dropped and counted over the rest: a p over the survivors is a p over
      # a different, self-selected reference distribution, and the failure would
      # read as a slightly noisier number rather than as a failure.
      return(out("nofit", paste0(sum(!is.finite(ts)), " enumerated draw(s) in one ",
                                 "chunk produced no finite t (the CR2 variance is ",
                                 "degenerate under reweighting)"),
                 S, S_eff, flr))
    }
    count <- count + sum(abs(ts) >= thr)
    ties <- ties + sum(abs(abs(ts) - abs(chk)) <= tie_tol)
  }
  if (ties < 6) {
    fail("the enumeration found ", ties, " draws tying |t| where the ",
         "homogeneity of the CR2 t guarantees 6. The floor of 6/6^S_eff, and ",
         "every inert verdict resting on it, would be wrong.")
  }
  # Exact, so no Monte Carlo correction: the observed statistic is already among
  # the counted draws as the all-equal-weight case.
  list(p = count / total, S = as.integer(S), S_eff = as.integer(S_eff),
       floor = flr, t_obs = obs$t, inert = NA_character_, why = NA_character_)
}

# The season-clustered call, which is the axis this pipeline clusters on
# everywhere else.
wild_cluster_p_season <- function(d) wild_cluster_p(d, d$season)

# How a wild bootstrap p must be PRINTED: never as a bare number where an arm has
# no p. The word sends the reader to the named reason rather than letting a blank
# or a 1.0000 stand in for three different conditions.
wcb_label <- function(w) {
  if (!is.na(w$inert)) return(w$inert)
  formatC(w$p, format = "f", digits = 4)
}

# --- the information regime -------------------------------------------------
#
# The blend weight is n/(n+k), so entry gameweek *is* the information regime: a
# GW1 entrant decides on the prior alone, a late one almost entirely on this
# season. Six entry points split 2/2/2, which is the split every table in this
# repo that names a regime already uses.
regime_of <- function(start_gw) {
  qs <- unique(sort(start_gw))
  if (length(qs) < 3) return(rep(NA_character_, length(start_gw)))
  n <- length(qs) %/% 3
  early <- qs[seq_len(n)]
  late <- qs[(length(qs) - n + 1):length(qs)]
  ifelse(start_gw %in% early, "early (prior-led)",
         ifelse(start_gw %in% late, "late (season-led)", "middle"))
}

# Locate this file from a sibling script regardless of the working directory.
# Sourcing scripts use it before anything else, so it is defined here only for
# documentation — each caller needs its own copy to find this file at all.
common_dir_of_script <- function() {
  a <- commandArgs(trailingOnly = FALSE)
  f <- sub("^--file=", "", a[grep("^--file=", a)])
  if (length(f) == 0) return(".")
  dirname(normalizePath(f[1]))
}
