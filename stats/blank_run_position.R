#!/usr/bin/env Rscript
#
# Is the minutes over-prediction that `blankRunFactor` divides out position-wide?
#
# Usage — the data state is chosen by the RUN, not by a flag here:
#
#   FPL_NO_BLANK_RUN=1 FPL_BLANKRUN_ROWS=/tmp/blankrun.csv DIAG=1 \
#     go test ./internal/backtest -run TestDiagAvailabilityByPosition -count=1
#   Rscript stats/blank_run_position.R /tmp/blankrun.csv
#
# ---------------------------------------------------------------------------
# The question
#
# `analysis.blankRunFactor` derives the shipped 0.75 by dividing each
# trailing-blank row's actual/expected ratio by the run-0 row's, and justifies
# that de-levelling as stripping out "the model's general, and harmlessly
# position-wide, tendency to over-predict minutes". The table it rests on is
# pooled over positions, so "position-wide" is asserted at that site and was
# never measured.
#
# It is load-bearing for the LEVEL only. Undo the de-levelling and the constant
# moves by the run-0 ratio: ~0.75 -> ~0.70 on the table recorded at the site,
# which is pre-doubles-fix, and ~0.80 -> ~0.76 at today's data state. 7% then,
# 3-4% now — see the dated note in `analysis.blankRunFactor`. The plateau SHAPE
# survives either way and so does the case for shipping the term at all.
#
# The reason it matters is AGENTS.md's exemption — "a bias shared by every player
# in a position is not an ordering error; a within-position bias is" — which is
# what makes removing a level free. If the run-0 over-prediction differs by
# position, the exemption does not cover this de-levelling and the argument given
# for it does not hold. That is the whole test. This script decides nothing about
# what the constant should then be.
#
# ---------------------------------------------------------------------------
# The estimator, named, because swapping one reads as a data change
#
# The Go table's cell figure is mean(actual)/mean(expected), which on a common n
# is sum(actual)/sum(expected) — a RATIO OF MEANS, not a mean of ratios. Both are
# kept below and both are reported, because they are different quantities and the
# recorded 0.752/0.737/0.765 came from the first.
#
# ---------------------------------------------------------------------------
# ⚠️ Season clustering is NOT sufficient here, and the reason generalises
#
# This record's default is to cluster on season. On a *cells* file that is right,
# because a cell belongs to one season and to nothing else. Here it is not:
# **players are crossed with seasons, not nested in them.** The same established
# footballer appears at eight cutoffs in each of six seasons, so his idiosyncratic
# contribution is common to all six season means rather than spread across them,
# the season means are positively correlated, and `sd(v)/sqrt(S)` comes out too
# small. Table 1 prints the distinct player count beside n so the size of this is
# visible: 8,849 run-0 rows are **805 footballers**, and only **98** of them are
# forwards.
#
# The check with power is the sampling FLOOR. Resample players *within* each
# season and you get the irreducible sampling SE of that season's contrast; if the
# S seasons drew independent players, the SE of their mean could not fall below
# the root-mean-square of those over sqrt(S).
#
# ⚠️ A season-means SE below its own floor cannot happen under independent seasons
# EXCEPT through luck in the sd estimate — at df = 5 that is about a 2% event per
# pair, so "arithmetically impossible", which an earlier draft of this header
# said, is one word too strong at a single pair. What carries it is that THREE of
# the six pairs sit below theirs. Read the floor across the table, not at one row.
#
# So THREE estimators, quoted side by side rather than picked between:
#
#   SEASON-MEANS  the ratio within each season, then a one-sample t over the S
#                 values. What AGENTS.md says CR2 reduces to on a balanced
#                 intercept-only design. Absorbs all within-season dependence,
#                 including a player's own eight cutoffs. Absorbs NO cross-season
#                 recurrence. df = S - 1, so t_crit is 2.571 at six seasons.
#   SEASON-CR1    the linearised ratio variance, influence terms u_i = a_i/A -
#                 e_i/E summed by season. The weighted counterpart of the first,
#                 and it shares the first's blind spot exactly.
#   PLAYER-BOOT   resample distinct player CODES with replacement, whole player
#                 and all his rows in every season. This is the estimator that
#                 sees the recurrence. Fixed seed, so it is reproducible; B is
#                 printed.
#
# ⚠️ **Neither dominates, and the estimand is the reason.** Season clustering
# generalises over *football* — it lets whole seasons disagree — and a player
# bootstrap generalises over *footballers*, holding the six seasons fixed. A
# claim about positions needs the second, which is why it is printed; but a
# bootstrap t is not a licence to ignore that six seasons is all the football
# there is. Where they agree the contrast is safe, and that agreement, not either
# number alone, is what the verdict below is built on. Where PLAYER-BOOT is much
# wider, the season-means t was reading one cohort S times over.
#
# ---------------------------------------------------------------------------
# Additive or multiplicative — the second question, and it is not decoration
#
# `internal/backtest/xgcrepair.go` scopes the shared-bias precedent: it "is about
# an ADDITIVE bias and must not be cited for a multiplicative scaling". The site
# under test REPORTS the discrepancy additively (its `bias` column is
# actual - expected, -4.5 minutes) and REMOVES it multiplicatively (a ratio
# divisor). So the additive bias is reported here beside the ratio, and the
# question it answers is narrow: **does the falsification survive the
# parameterisation the precedent is actually about?**
#
# ⚠️ It is NOT answered by counting how many pairs clear Holm in each table. An
# earlier draft of this header said a differing count "IS the conversion defect,
# visible", and that is the "difference between significant and non-significant
# is not itself significant" fallacy written into the design. Two tables over the
# same rows, whose disagreement is one pair landing at Holm 0.039 and 0.065 on
# t values of 4.30 and 3.77, order nothing. What the comparison can say is
# whether the falsification holds in both, and it does.
#
# ---------------------------------------------------------------------------
# Provenance: this rule is POST-HOC, and saying otherwise would be worse than
# saying nothing
#
# An earlier draft of this header claimed the rule below was "pre-registered
# before reading any per-position number". **That was false.** The pooled Go
# table — which already showed GKP at 0.999 — was read before this script was
# written. The accurate label:
#
#   INHERITED, not chosen after the split. The population filters, the five-
#   gameweek window, the eight cutoffs, the run bucketing and the ratio-of-means
#   estimator all come from the recorded calibration this is re-running.
#   FIXED BEFORE ANY p-VALUE, and fixed conservatively: Holm over six pairs
#   rather than the largest gap uncorrected, which runs against the author's own
#   interest in falsifying.
#   POST-HOC in what matters: testing position at all, and where the bar sits,
#   were both decided knowing GKP sat apart.
#
# So discount this the way a post-hoc result is discounted. It is not dismissed
# by that — the effect is large, ordered, near-unanimous by season and survives
# player clustering — but a reader is owed the label rather than a claim to a
# blindness that did not happen.
#
# ---------------------------------------------------------------------------
# What would falsify "position-wide", and what would not
#
#   FALSIFIED   the position OMNIBUS rejects at 0.05 — a two-way F on the season
#               x position matrix, and Friedman as its rank sibling. This is the
#               test of the actual question ("is this bias common to all four?"),
#               it does not choose a pair, and it is the headline.
#   SUPPORTING  the pairwise contrasts, Holm over six. Any pair clearing Holm
#               also rejects the global null, so this route is sound and
#               conservative — but it is weaker, and WHICH pair carries it is not
#               stable across grids. Never read the identity of the winning pair
#               as a finding: that is an argmax over six contrasts.
#   NOT SHOWN   otherwise. A spread inside its own noise is a tie and NOT
#               evidence that the positions agree — this record's "a null is a
#               tie, not the refutation of one". The MDE column says what size of
#               difference this comparison would actually have found, and a
#               spread below it was never measurable.
#
# The most robust statement available is neither: it is the ORDERING. How often
# is GKP the highest of the four and FWD the lowest, season by season? That is a
# within-season rank statistic, so it is immune to every clustering argument
# above, and this record already holds that "an ordering is cheaper to establish
# than a gap".

args <- commandArgs(trailingOnly = TRUE)

# `fail`, `note`, `hr`, `as_flag`, `read_sidecar` and `sig_and_mde` are in
# cells_common.R. This input is NOT a cells file — it is one row per
# player-cutoff, not per replayed cell — so it goes through `read_sidecar`, the
# sanctioned reader for a pipeline CSV with its own contract. The guard forbids a
# raw `read.csv(` anywhere under stats/ outside that file, and a script with a
# different contract is still not the place a raw read should hide.
local({
  a <- commandArgs(trailingOnly = FALSE)
  f <- sub("^--file=", "", a[grep("^--file=", a)])
  dir <- if (length(f) > 0) dirname(normalizePath(f[1])) else "stats"
  p <- file.path(dir, "cells_common.R")
  if (!file.exists(p)) {
    stop("cannot find cells_common.R beside this script (looked in ", dir, ")")
  }
  source(p, local = FALSE)
})

ALPHA <- 0.05
POWER <- 0.80
POSITIONS <- c("GKP", "DEF", "MID", "FWD")

# The threshold pair, from cells_common.R. ⚠️ No x38: every quantity below is a
# unitless ratio or a count of minutes per gameweek, and the cells family's
# season scale would silently turn a ratio into a points figure.
sig_mde <- function(se, df) sig_and_mde(se, df, ALPHA, POWER)

if (length(args) < 1) fail("usage: blank_run_position.R <rows.csv>")
path <- args[[1]]

d <- read_sidecar(path)
need <- c("season", "cutoff", "code", "position", "run", "flagged",
          "expected", "actual")
missing <- setdiff(need, names(d))
if (length(missing)) fail("columns missing from ", path, ": ",
                          paste(missing, collapse = ", "))

flagged <- as_flag(d$flagged)
d$expected <- as.numeric(d$expected)
d$actual <- as.numeric(d$actual)
d$run <- as.integer(d$run)

# The calibration population. The recorded table's own header says "Restricted to
# players FPL had *not* flagged", and the flagged group is where
# availabilityFactor already has the channel, so including it would measure a
# different model.
d <- d[!flagged, ]
if (!nrow(d)) fail("no unflagged rows in ", path)
if (!all(d$position %in% POSITIONS)) {
  fail("unexpected position labels: ",
       paste(setdiff(unique(d$position), POSITIONS), collapse = ", "))
}

seasons <- sort(unique(d$season))
S <- length(seasons)
if (S < 3) fail("only ", S, " seasons in ", path,
                "; a season-clustered t needs df >= 2")
DF <- S - 1

note("")
hr()
note("blank-run de-levelling, cut by position")
hr()
note("rows          : ", path)
note("population    : unflagged, established players, ", nrow(d), " player-cutoffs")
note("seasons       : ", S, " (", paste(seasons, collapse = ", "), ")")
note("cutoffs       : ", paste(sort(unique(d$cutoff)), collapse = ", "))
note("clustering    : season, df = ", DF, ", t_crit = ",
     sprintf("%.3f", qt(1 - ALPHA / 2, DF)))
note("")
note("DATA STATE. The dump must have been written with FPL_NO_BLANK_RUN=1 —")
note("ExpectedMinutes carries blankRunFactor, so at shipped config this is a fit")
note("to the term's own output. The Go diagnostic refuses to run without it, so")
note("a file that exists was written with it. The xG/xGC backfills are NOT")
note("switched off here: FPL_NO_XG_REPAIR and FPL_NO_XGC_REPAIR govern the")
note("attacking and conceding rates, which no quantity below reads — minutes are")
note("FPL's own and are not backfilled. FPL_NO_STARTS_REPAIR is the one that")
note("could reach this, through StartShare; it is byte-identical at shipped")
note("config per AGENTS.md, which makes it untested here rather than inert.")

# --- the two cell quantities ------------------------------------------------
#
# ratio_of_means is the recorded estimator. mean_of_ratios is its sibling, kept
# because a gap between them is a fact about the population's spread rather than
# a coding error, and quoting one without the other invites the swap this record
# warns about.
ratio_of_means <- function(x) if (sum(x$expected) == 0) NA_real_ else
  sum(x$actual) / sum(x$expected)
mean_of_ratios <- function(x) {
  ok <- x$expected > 0
  if (!any(ok)) NA_real_ else mean(x$actual[ok] / x$expected[ok])
}
add_bias <- function(x) mean(x$actual - x$expected)

cut_rows <- function(pos, runs) {
  x <- d[d$run %in% runs, ]
  if (!is.null(pos)) x <- x[x$position == pos, ]
  x
}

# --- season-clustered inference on a per-observation statistic ---------------
#
# stat() is evaluated once per season and the S values are treated as the sample.
# Everything below routes through this, so there is one implementation of the
# clustering rather than one per table.
by_season <- function(rows, stat) {
  vapply(seasons, function(s) {
    x <- rows[rows$season == s, ]
    if (!nrow(x)) NA_real_ else stat(x)
  }, numeric(1))
}

# ⚠️ A fourth spelling of the season-clustered t, sanctioned with an argument
# rather than by omission. At one row per cluster `cr2_t_fast`'s leverage term
# collapses (1 - n_g/n with n_g = 1) and the two are algebraically identical —
# verified to twelve decimals. It is written out here because the shared
# implementations all take a *cells* contract this input does not have:
# `se_cr` wants a `diff` column and clubSandwich, `cr2_t_fast` wants a matrix of
# draws, and both key on the cell. The input here is a per-observation frame
# whose cluster statistic is a RATIO, which none of them computes.
season_t <- function(v, null = 0) {
  v <- v[is.finite(v)]
  n <- length(v)
  if (n < 3) return(list(n = n, mean = if (n) mean(v) else NA_real_,
                         se = NA_real_, t = NA_real_, p = NA_real_,
                         sig = NA_real_, mde = NA_real_, pos = NA_integer_))
  m <- mean(v); se <- sd(v) / sqrt(n)
  sm <- sig_mde(se, n - 1)
  tv <- (m - null) / se
  list(n = n, mean = m, se = se, t = tv,
       p = 2 * pt(-abs(tv), n - 1), sig = sm[["sig"]], mde = sm[["mde"]],
       pos = sum(v > null))
}

# --- the estimator that sees a player recurring across seasons ---------------
#
# Resample distinct player CODES with replacement — the whole footballer, every
# row he has, in every season — and recompute the statistic. Codes are permanent
# (element ids are reassigned each summer, which is why the dump carries the
# code), so a resampled player is the same player throughout.
#
# Seeded, because an unseeded bootstrap makes a figure that cannot be checked.
BOOT <- 2000
BOOT_SEED <- 20260816

codes <- sort(unique(d$code))
by_code <- split(seq_len(nrow(d)), factor(d$code, levels = codes))

# player_boot returns the bootstrap SE of stat(rows_a) - stat(rows_b), where the
# two row sets are given as index vectors into d. Passing indices rather than
# frames is what keeps this affordable: 2000 reps over 11k rows.
player_boot <- function(ia, ib, stat) {
  set.seed(BOOT_SEED)
  reps <- vapply(seq_len(BOOT), function(.) {
    draw <- sample.int(length(codes), length(codes), replace = TRUE)
    idx <- unlist(by_code[draw], use.names = FALSE)
    ra <- d[intersect_sorted(idx, ia), ]
    rb <- d[intersect_sorted(idx, ib), ]
    if (!nrow(ra) || !nrow(rb)) return(NA_real_)
    stat(ra) - stat(rb)
  }, numeric(1))
  reps <- reps[is.finite(reps)]
  if (length(reps) < BOOT / 2) return(NA_real_)
  sd(reps)
}

# A resampled player's rows must be kept once per time he was drawn, so this is a
# multiset filter and not R's `intersect`, which would deduplicate and silently
# turn the bootstrap into a subsample.
intersect_sorted <- function(idx, keep) idx[idx %in% keep]

# The sampling floor. Resample players WITHIN one season to get that season's own
# sampling SE; if the S seasons drew independent players, the SE of their mean
# could not be below rms(these)/sqrt(S). A season-means SE below it is impossible
# under independence, so the floor is a proof of shared cohort rather than a
# statement about the effect.
season_floor <- function(ia, ib, stat) {
  per <- vapply(seasons, function(s) {
    rows_s <- which(d$season == s)
    cs <- sort(unique(d$code[rows_s]))
    if (length(cs) < 5) return(NA_real_)
    bc <- split(rows_s, factor(d$code[rows_s], levels = cs))
    set.seed(BOOT_SEED)
    reps <- vapply(seq_len(BOOT %/% 4), function(.) {
      draw <- sample.int(length(cs), length(cs), replace = TRUE)
      idx <- unlist(bc[draw], use.names = FALSE)
      ra <- d[intersect_sorted(idx, ia), ]
      rb <- d[intersect_sorted(idx, ib), ]
      if (!nrow(ra) || !nrow(rb)) return(NA_real_)
      stat(ra) - stat(rb)
    }, numeric(1))
    reps <- reps[is.finite(reps)]
    if (length(reps) < BOOT %/% 8) NA_real_ else sd(reps)
  }, numeric(1))
  per <- per[is.finite(per)]
  if (!length(per)) return(NA_real_)
  sqrt(mean(per^2)) / sqrt(length(per))
}

# The linearised ratio variance, clustered by season. u_i = a_i/A - e_i/E is the
# influence of observation i on log(A/E); the cluster sums are what varies.
log_ratio_clustered <- function(rows) {
  A <- sum(rows$actual); E <- sum(rows$expected)
  if (A <= 0 || E <= 0) return(list(est = NA_real_, se = NA_real_))
  u <- rows$actual / A - rows$expected / E
  g <- vapply(seasons, function(s) sum(u[rows$season == s]), numeric(1))
  G <- sum(vapply(seasons, function(s) any(rows$season == s), logical(1)))
  # CR1's finite-cluster correction. CR2's leverage adjustment is not available
  # for a ratio with no design matrix, so this is named as CR1 and not as CR2.
  list(est = log(A / E), se = sqrt(sum(g^2) * G / max(G - 1, 1)))
}

# --- table 1: the run-0 row, which is the divisor -----------------------------

note("")
hr()
note("1. THE DIVISOR. actual/expected at zero trailing blanks, by position.")
hr()
note("This is the quantity the comment calls position-wide. `ratio` is the")
note("recorded estimator (ratio of means); `bias` is the same discrepancy in")
note("the additive parameterisation the site reports it in.")
note("")
cat(sprintf("%-6s %7s %7s %8s %10s %13s %9s %8s %9s %6s\n",
            "pos", "n", "players", "ratio", "se(seas)", "95% ci(seas)",
            "se(clus)", "bias", "se(bias)", "sgn"))
emit_run0 <- function(label, x) {
  st <- season_t(by_season(x, ratio_of_means))
  bst <- season_t(by_season(x, add_bias))
  cl <- log_ratio_clustered(x)
  r <- ratio_of_means(x)
  cat(sprintf("%-6s %7d %7d %8.4f %10.4f %13s %9.4f %8.2f %9.2f %5d/%d\n",
              label, nrow(x), length(unique(x$code)), r, st$se,
              sprintf("%.3f-%.3f", st$mean - st$sig, st$mean + st$sig),
              cl$se * r, add_bias(x), bst$se, st$pos, S))
}
for (p in POSITIONS) emit_run0(p, cut_rows(p, 0))
emit_run0("POOLED", cut_rows(NULL, 0))
note("")
note("⚠️ `n` counts player-cutoffs and `players` counts FOOTBALLERS. Read every t")
note("below against the second: a position's rows are one cohort seen up to ", S,
     " x ", length(unique(d$cutoff)), " times over,")
note("not n independent draws. 98 forwards carry 826 rows.")
note("")
note("`ratio` and `bias` are pooled over all rows; `se(seas)` and the interval are")
note("the season-means estimator, so the interval is around the SEASON MEAN and")
note("not around the pooled point. `se(clus)` is the linearised ratio variance")
note("with the influence terms summed by season (CR1 correction) — the weighted")
note("counterpart, printed beside rather than instead of, because an estimator")
note("swap reads as a data change. `sgn` counts seasons above 1, out of ", S, ".")

# --- table 2: the pairwise contrasts, Holm-corrected --------------------------

pairs <- combn(POSITIONS, 2, simplify = FALSE)

# boot = TRUE adds the player-clustered SE and the sampling floor. It costs a
# few seconds per table, so it is on for the two tables that carry a verdict and
# off for the de-levelled one, which is reported for completeness.
contrast_table <- function(title, runs, stat, null, fmt, boot = TRUE) {
  note("")
  hr()
  note(title)
  hr()
  note("`n(a)`/`n(b)` count ROWS; `pl` counts distinct footballers, which is the")
  note("number the t is really built on. For the de-levelled contrasts both")
  note("include the run-0 rows the divisor needs.")
  note("")
  rows <- list()
  for (pr in pairs) {
    ia <- which(d$run %in% runs & d$position == pr[1])
    ib <- which(d$run %in% runs & d$position == pr[2])
    a <- d[ia, ]; b <- d[ib, ]
    st <- season_t(by_season(a, stat) - by_season(b, stat), null)
    bse <- if (boot) player_boot(ia, ib, stat) else NA_real_
    flr <- if (boot) season_floor(ia, ib, stat) else NA_real_
    rows[[length(rows) + 1]] <- data.frame(
      pair = paste(pr[1], "-", pr[2]), n_a = nrow(a), n_b = nrow(b),
      pl = length(unique(c(a$code, b$code))),
      diff = st$mean, se = st$se, t = st$t, p = st$p,
      sig = st$sig, mde = st$mde, agree = st$pos,
      boot_se = bse, boot_t = st$mean / bse, floor = flr,
      stringsAsFactors = FALSE)
  }
  tab <- do.call(rbind, rows)
  tab$holm <- p.adjust(tab$p, method = "holm")
  # The bootstrap side gets the SAME correction as the season-means side. An
  # earlier version screened it at a bare |t| >= 2 while Holm-correcting the
  # column beside it, which invited a comparison between two different bars —
  # and on four seasons declared three pairs "surviving" at bootstrap t ~2.93,
  # below that file's own printed t_crit of 3.182. The normal reference is right
  # for the bootstrap and wrong for the season means: 600-plus player clusters
  # against S - 1 = 5 or 3.
  tab$boot_p <- 2 * pnorm(-abs(tab$boot_t))
  tab$boot_holm <- if (all(is.na(tab$boot_p))) tab$boot_p else
    p.adjust(tab$boot_p, method = "holm")
  cat(sprintf("%-12s %6s %6s %5s %9s %8s %6s %8s %8s %8s %7s %9s %7s %7s\n",
              "pair", "n(a)", "n(b)", "pl", "diff", "se", "t", "holm",
              "mde", "bootSE", "bootT", "bootHolm", "floor", "sgn+"))
  for (i in seq_len(nrow(tab))) {
    cat(sprintf("%-12s %6d %6d %5d %9s %8s %6s %8s %8s %8s %7s %9s %7s %5d/%d\n",
                tab$pair[i], tab$n_a[i], tab$n_b[i], tab$pl[i],
                fmt(tab$diff[i]), fmt(tab$se[i]),
                sprintf("%.2f", tab$t[i]),
                sprintf("%.4f", tab$holm[i]), fmt(tab$mde[i]),
                if (is.finite(tab$boot_se[i])) fmt(tab$boot_se[i]) else "-",
                if (is.finite(tab$boot_t[i])) sprintf("%.2f", tab$boot_t[i]) else "-",
                if (is.finite(tab$boot_holm[i])) sprintf("%.4f", tab$boot_holm[i]) else "-",
                if (is.finite(tab$floor[i])) fmt(tab$floor[i]) else "-",
                tab$agree[i], S))
  }
  if (boot) {
    note("")
    note("`bootSE`/`bootT`/`bootHolm` resample ", length(codes), " player codes,")
    note("B = ", BOOT, ", seed ", BOOT_SEED, " — the estimator that sees a")
    note("footballer recurring across seasons. `bootHolm` is a NORMAL reference")
    note("Holm-corrected over the same ", length(pairs), " pairs, so the two Holm")
    note("columns are held to one bar; the normal reference is right for 600+")
    note("player clusters and wrong for the S-1 df beside it.")
    note("")
    note("`floor` is the within-season player-sampling SE of the season mean. An")
    note("`se` below its own floor cannot happen under independent seasons except")
    note("through luck in the sd estimate (about 2% per pair at df 5) — so read it")
    note("across the table, not at one row. ⚠️ Neither estimator dominates: `se`")
    note("generalises over FOOTBALL, `bootSE` over FOOTBALLERS. Their AGREEMENT is")
    note("what the verdict rests on, not either column alone.")
  }
  invisible(tab)
}

f4 <- function(x) sprintf("%.4f", x)
f2 <- function(x) sprintf("%.2f", x)

# --- table 2: the omnibus and the ordering, which are the headline -----------
#
# The season x position matrix of run-0 ratios. Both tests below treat the S x 4
# cells as one observation each, so they inherit the player-overlap caveat above
# — the RANK statement that follows does not, which is why it is printed last and
# is the one to quote where a reader will not follow an argument about clusters.

mat <- sapply(POSITIONS, function(p) by_season(cut_rows(p, 0), ratio_of_means))
rownames(mat) <- seasons

note("")
hr()
note("2. THE OMNIBUS. Is the run-0 ratio common to all four positions?")
hr()
note("The season x position matrix of run-0 ratios, then a test of position with")
note("season blocked. This tests the actual question and chooses no pair.")
note("")
cat(sprintf("%-10s %9s %9s %9s %9s   %s\n",
            "season", POSITIONS[1], POSITIONS[2], POSITIONS[3], POSITIONS[4],
            "highest / lowest"))
for (i in seq_len(nrow(mat))) {
  v <- mat[i, ]
  cat(sprintf("%-10s %9.4f %9.4f %9.4f %9.4f   %s / %s\n", seasons[i],
              v[1], v[2], v[3], v[4],
              POSITIONS[which.max(v)], POSITIONS[which.min(v)]))
}
note("")

long <- data.frame(
  ratio = as.vector(mat),
  position = factor(rep(POSITIONS, each = nrow(mat)), levels = POSITIONS),
  season = factor(rep(seasons, times = length(POSITIONS)), levels = seasons))
aov_fit <- summary(aov(ratio ~ position + season, data = long))[[1]]
f_stat <- aov_fit[["F value"]][1]
f_df1 <- aov_fit[["Df"]][1]
f_df2 <- aov_fit[["Df"]][length(aov_fit[["Df"]])]
f_p <- aov_fit[["Pr(>F)"]][1]
fr <- friedman.test(mat)
note("two-way F on position, season blocked : F(", f_df1, ",", f_df2, ") = ",
     sprintf("%.2f", f_stat), ", p = ", format.pval(f_p, digits = 3))
note("Friedman (its rank sibling)           : chi2 = ", sprintf("%.2f", fr$statistic),
     ", df = ", fr$parameter, ", p = ", sprintf("%.4f", fr$p.value))
note("")
note("⚠️ Both treat the ", S, " x 4 cells as one observation each, so they INHERIT")
note("the player-overlap caveat and neither has a player-clustered form. What")
note("makes the omnibus safe to lead on is that the pairs carrying it survive one.")

# The ordering. Under a null of exchangeable positions within a season, each
# position is the maximum with probability 1/4 independently across seasons, so
# the count is Binomial(S, 1/4) and the one-sided tail is exact.
hi <- table(factor(POSITIONS[apply(mat, 1, which.max)], levels = POSITIONS))
lo <- table(factor(POSITIONS[apply(mat, 1, which.min)], levels = POSITIONS))
note("")
note("THE ORDERING, which no clustering argument touches. Within-season ranks,")
note("so a footballer recurring across seasons cannot inflate it.")
note("")
cat(sprintf("%-6s %10s %12s %10s %12s\n",
            "pos", "highest", "p(binom)", "lowest", "p(binom)"))
for (p in POSITIONS) {
  ph <- pbinom(hi[[p]] - 1, S, 0.25, lower.tail = FALSE)
  pl <- pbinom(lo[[p]] - 1, S, 0.25, lower.tail = FALSE)
  cat(sprintf("%-6s %6d/%-3d %12s %6d/%-3d %12s\n",
              p, hi[[p]], S, sprintf("%.4f", ph),
              lo[[p]], S, sprintf("%.4f", pl)))
}
note("")
note("These are one-sided tails on Binomial(", S, ", 1/4), uncorrected — they are")
note("descriptive of a shape, not eight more tests to Holm over. The floor at S = ",
     S, " is ", sprintf("%.4f", 0.25^S), ", and Bonferroni over the eight printed")
note("here would multiply by 8.")
note("")
note("⚠️ Within-season ranking removes WITHIN-season dependence and nothing else.")
note("Binomial(", S, ", 1/4) still assumes ", S, " independent season draws, which a")
note("recurring cohort is exactly what violates — ", length(unique(cut_rows("GKP", 0)$code)),
     " keepers carry the GKP")
note("column at a mean of ", sprintf("%.2f", nrow(cut_rows("GKP", 0)) /
     length(unique(cut_rows("GKP", 0)$code)) / length(unique(d$cutoff))),
     " seasons each. A player-resampled version is unrun.")

t_ratio <- contrast_table(
  paste0("3. THE PAIRS. Contrasts on the run-0 RATIO, Holm over ",
         length(pairs), ".\n",
         "   SUPPORTING, not the headline: any pair clearing Holm also rejects\n",
         "   the global null, but WHICH pair carries it is not stable and must\n",
         "   never be read as a finding of its own."),
  0, ratio_of_means, 0, f4)
note("")
note("`mde` is the difference this comparison would find 80% of the time. A")
note("`diff` below it was never measurable, so a large p there is silence and")
note("not agreement.")

t_bias <- contrast_table(
  paste0("4. THE SAME CONTRASTS IN THE ADDITIVE PARAMETERISATION (minutes/gw).\n",
         "   xgcrepair.go's rule: the shared-bias precedent is about an ADDITIVE\n",
         "   bias, and the site measures additively and removes multiplicatively.\n",
         "   The question is whether the falsification SURVIVES here — not which\n",
         "   table clears more pairs, which orders nothing."),
  0, add_bias, 0, f2)

# --- table 3: the de-levelled plateau ----------------------------------------

note("")
hr()
note("5. WHAT THE DE-LEVELLING DOES, by position. Plateau = runs 1-3 pooled.")
hr()
note("raw = plateau ratio; delevelled = raw / run-0 ratio. The shipped constant")
note("is one number for all four, so the spread here is what a pooled divisor")
note("costs whichever positions are unlike it.")
note("")
cat(sprintf("%-6s %7s %9s %9s %12s %10s %12s\n",
            "pos", "n(1-3)", "raw", "run-0", "delevelled", "se(delev)",
            "95% ci"))
delev <- function(x) {
  # Per season, so the ratio-of-ratios is formed inside the cluster and the
  # t below is over S of them rather than over one number with no spread.
  p <- x[x$run %in% 1:3, ]; z <- x[x$run == 0, ]
  r0 <- ratio_of_means(z)
  if (!is.finite(r0) || r0 == 0 || !nrow(p)) return(NA_real_)
  ratio_of_means(p) / r0
}
for (p in POSITIONS) {
  x <- d[d$position == p, ]
  pl <- x[x$run %in% 1:3, ]
  st <- season_t(by_season(x, delev))
  cat(sprintf("%-6s %7d %9.4f %9.4f %12.4f %10.4f %12s\n",
              p, nrow(pl), ratio_of_means(pl),
              ratio_of_means(x[x$run == 0, ]),
              delev(x), st$se,
              sprintf("%.3f-%.3f", st$mean - st$sig, st$mean + st$sig)))
}
stp <- season_t(by_season(d, delev))
cat(sprintf("%-6s %7d %9.4f %9.4f %12.4f %10.4f %12s\n",
            "POOLED", sum(d$run %in% 1:3), ratio_of_means(d[d$run %in% 1:3, ]),
            ratio_of_means(d[d$run == 0, ]), delev(d), stp$se,
            sprintf("%.3f-%.3f", stp$mean - stp$sig, stp$mean + stp$sig)))

t_delev <- contrast_table(
  paste0("6. Pairwise position contrasts on the DE-LEVELLED plateau.\n",
         "   Reported for completeness. It is not the falsifier: the claim is\n",
         "   about the divisor, and a plateau that differs by position is a\n",
         "   separate question this design is much worse powered for."),
  0:3, delev, 0, f4, boot = FALSE)

# --- verdict -----------------------------------------------------------------
#
# Printed mechanically from the rule stated at the top, so the reading cannot
# drift from the numbers. The rule leads on the omnibus; the pairs support it.

note("")
hr()
note("VERDICT on 'position-wide', by the rule stated at the top")
hr()
survivors <- t_ratio[is.finite(t_ratio$boot_holm) & t_ratio$boot_holm < ALPHA, ]
if (f_p < ALPHA) {
  note("FALSIFIED, on the omnibus: F(", f_df1, ",", f_df2, ") = ",
       sprintf("%.2f", f_stat), ", p = ", format.pval(f_p, digits = 3),
       "; Friedman p = ", sprintf("%.4f", fr$p.value), ".")
  note("The over-prediction the de-levelling removes is not common to all four")
  note("positions, so AGENTS.md's shared-bias exemption does not cover it and")
  note("the argument written at the site does not hold. What the constant should")
  note("then be is a separate question and is NOT answered here.")
} else {
  note("NOT SHOWN on the omnibus: F(", f_df1, ",", f_df2, ") = ",
       sprintf("%.2f", f_stat), ", p = ", format.pval(f_p, digits = 3), ".")
  note("That is a tie, not a demonstration that the positions agree.")
}
note("")
clearing <- t_ratio$pair[is.finite(t_ratio$holm) & t_ratio$holm < ALPHA]
note("Pairs clearing Holm, season-means SE : ",
     if (length(clearing)) paste(clearing, collapse = ", ") else "none")
note("Pairs clearing Holm, PLAYER-boot SE  : ",
     if (nrow(survivors)) paste(survivors$pair, collapse = ", ") else "none")
note("BOTH (this is what to quote)         : ",
     if (length(intersect(clearing, survivors$pair)))
       paste(intersect(clearing, survivors$pair), collapse = ", ") else "none")
note("")
note("Same bar on both columns, Holm over ", length(pairs), ". A pair on ONE list")
note("only is not a finding: the season-means column can read one cohort of")
note("footballers ", S, " times over, and the bootstrap column holds the ", S,
     " seasons")
note("fixed and so cannot speak to season-to-season disagreement at all.")
note("")
note("The falsification also holds in the ADDITIVE parameterisation the")
note("shared-bias precedent is actually about (table 4). Do NOT read an ordering")
note("into the two tables' Holm counts — ", sum(t_ratio$holm < ALPHA), " and ",
     sum(t_bias$holm < ALPHA), " of ", nrow(t_ratio), " — that is two sides of")
note("an arbitrary line on the same rows, and orders nothing.")
note("")
