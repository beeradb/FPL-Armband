#!/usr/bin/env Rscript

# The hit-verdict half of the transfer-hit tuning measurement: per-package
# verdicts from the sidecar (FPL_HITS_CSV), read against the sweep's cells.
#
# Usage:  Rscript stats/hittune_verdicts.R /tmp/hittune-hits.csv
#
# Registered: (a) the shipped arm's per-hit loss rate against the gate's OWN
# bar (a calibrated gate gives ~50% by truncation at net < 3, so a rate
# clearly above it is the "not tuned well" signal); (b) the season-level
# shipped-vs-no-hits contrast (computed by sweep_inference.R on the cells —
# this script reports the sidecar side); (H) the holding-window clearance —
# % of hit packages accruing +4 net before their in-players are sold or a
# wildcard lands, split forced vs preference; (H') the preference-hit share by
# MinGainHit rung. The earlier workedOut wait-match is superseded by the
# holding criterion (the user's ruling, 2026-08-18). Descriptive: the
# wildcard-week-after split and the availability-adjusted rates beside the
# raw.
#
# The package is the unit: a funded pair is one row, its legs summed, its hit
# charged once — the unit the gate judged.

args <- commandArgs(trailingOnly = TRUE)
if (length(args) < 1) stop("usage: hittune_verdicts.R <hits.csv>")

# The sidecar is not a cells file, so it reads through the shared sidecar
# reader rather than a raw read.csv — that reader is the sanctioned home, and
# a raw read here trips the one-implementation guard in the test suite.
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

h <- read_sidecar(args[1])
need <- c("arm", "season", "start_gw", "gw", "n_moves", "hit_net", "out_played",
          "wildcard_after", "in_ids", "hit", "hold_net", "hold_out_played",
          "hold_weeks", "out_was_captain")
miss <- setdiff(need, names(h))
if (length(miss) > 0) {
  stop("sidecar is missing columns: ", paste(miss, collapse = ", "),
       " — an 11-column file is the pre-holding-criterion bank and cannot ",
       "answer the holding questions")
}
h$out_played <- as.logical(h$out_played)
h$wildcard_after <- as.logical(h$wildcard_after)
h$hit <- as.logical(h$hit)
h$hold_out_played <- as.logical(h$hold_out_played)
h$out_was_captain <- as.logical(h$out_was_captain)
# One arm per short name, and the floored and full-plan machines are their OWN
# arms — the sub must not fold them into the flat baseline.
h$arm <- sub(", flat( \\(shipped\\))?", "", h$arm)
h$arm[h$arm == "mgh3, floored machine"] <- "mgh3floored"
h$arm[h$arm == "mgh3, full plan (shipped)"] <- "mgh3fullplan"
h$arm[h$arm == "no hits (wait)"] <- "no hits"
h$net3 <- h$hit_net < 3
h$net0 <- h$hit_net < 0
h$adj <- h$out_played

# The registered rates are PER HIT — the hit flag carries which packages paid
# the -4, so the free packages (gated at MinGain 0.4, not MinGainHit 3.0) are
# excluded before any loss rate is computed. The no-hits arm's packages stay
# unused: the season-level wait contrast comes from the cells, and the
# holding criterion needs no counterfactual match.
hits <- h[grepl("^mgh", h$arm) & h$hit, ]
baseline <- hits[hits$arm == "mgh3", ]

cat("=== the flat mgh3 machine's hits, against the gate's own bar ===\n")
n <- nrow(baseline)
cat(sprintf("packages: %d;  net < 0: %d (%.1f%%);  net < 3: %d (%.1f%%);  ",
            n, sum(baseline$net0), 100*mean(baseline$net0),
            sum(baseline$net3), 100*mean(baseline$net3)))
adj <- baseline[baseline$adj, ]
cat(sprintf("availability-adjusted net < 3: %.1f%% (n %d)\n",
            100*mean(adj$net3), nrow(adj)))
cat(sprintf("mean hit_net %+.1f, median %+.1f, spread [%+d, %+d]\n\n",
            mean(baseline$hit_net), median(baseline$hit_net),
            min(baseline$hit_net), max(baseline$hit_net)))

cat("=== loss rates by arm (availability-adjusted net < 3), paired per cell ===\n")
# The pairing keeps every cell: a cell where an arm took no availability-
# adjusted hits is NA, not dropped — the dropped cells are exactly the ones
# where the rung's bar rejected everything, and an inner join would read the
# surviving, easier cells as the whole contrast.
mk <- function(arm) {
  a <- hits[hits$arm == arm & hits$out_played, ]
  agg <- aggregate(net3 ~ season + start_gw, a, mean)
  names(agg)[3] <- "rate"
  grid <- expand.grid(season = unique(hits$season), start_gw = unique(hits$start_gw))
  merge(grid, agg, by = c("season", "start_gw"), all.x = TRUE)
}
hitsPerCell <- function(arm) {
  a <- hits[hits$arm == arm & hits$out_played, ]
  as.data.frame(table(a$season, a$start_gw))$Freq
}
for (a in c("mgh4", "mgh5", "mgh6")) {
  m <- merge(mk("mgh3"), mk(a), by = c("season", "start_gw"), suffixes = c(".3", ".a"))
  ok <- !is.na(m$rate.3) & !is.na(m$rate.a)
  d <- m$rate.a[ok] - m$rate.3[ok]
  cat(sprintf("  %s - mgh3: mean %+.3f per-cell rate over %d compared cells (%d dropped for zero adjusted hits); %d negative; naive paired t %.2f\n",
              a, mean(d), sum(ok), sum(!ok), sum(d < 0),
              mean(d) / (sd(d) / sqrt(length(d)))))
  cat(sprintf("    adjusted hits per cell: mgh3 mean %.1f, %s mean %.1f\n",
              mean(hitsPerCell("mgh3")), a, mean(hitsPerCell(a))))
}
fl <- hits[hits$arm == "mgh3floored", ]
cat(sprintf("  floored machine (own arm): net<3 %.1f%% of %d packages; net<0 %.1f%%\n",
            100*mean(fl$net3), nrow(fl), 100*mean(fl$net0)))

cat("\n=== the holding-window criterion: +4 net before sale, wildcard or season's end ===\n")
# The user's ruling replaces the workedOut wait-match: a hit worked iff the
# package accrued +4 net during the span the squad actually held the incoming
# players — each leg's Week.Contrib (autosubs, armband, bench boost inside it;
# a free-hit week contributing nothing) minus the sold player's raw points.
# The split: forced (the replaced player stopped appearing AFTER the transfer
# week — the squad was fielding a blank without the move) vs preference (he
# kept playing — the real bet). The tuned-bets population is the preference
# one. The arms run three versions of the criterion: flat ladder = no chips
# (no cut binds); floored machine = bench boost + free hit + triple captain
# (wildcard cut absent); full plan = all four (every cut live).
hits$worked4 <- hits$hold_net >= 4
hits$workedH <- hits$hit_net >= 4  # the horizon clearance, for the H' gap
for (arm in c("mgh3", "mgh3floored", "mgh3fullplan")) {
  a <- hits[hits$arm == arm, ]
  pref <- a[a$hold_out_played, ]
  forced <- a[!a$hold_out_played, ]
  cuts <- if (arm == "mgh3") "no chips — no cut binds" else if (arm == "mgh3floored")
    "BB+FH+TC live, wildcard absent" else "all four chips — every cut live"
  cat(sprintf("%s: %d hits, %d (%.0f%%) clear +4 in the hold; mean %+.1f, median %+.1f, spread [%+d, %+d]\n",
              arm, nrow(a), sum(a$worked4), 100*mean(a$worked4),
              mean(a$hold_net), median(a$hold_net),
              min(a$hold_net), max(a$hold_net)))
  cat(sprintf("  cuts: %s\n", cuts))
  cat(sprintf("  preference (sold player kept playing): %d hits, %.0f%% clear +4, mean %+.1f\n",
              nrow(pref), 100*mean(pref$worked4), mean(pref$hold_net)))
  cat(sprintf("  forced (replaced player stopped):      %d hits, %.0f%% clear +4, mean %+.1f\n",
              nrow(forced), 100*mean(forced$worked4), mean(forced$hold_net)))
  cat(sprintf("  hold length (weeks): mean %.1f, median %d, max %d\n",
              mean(a$hold_weeks), median(a$hold_weeks), max(a$hold_weeks)))
  cap <- a$out_was_captain
  if (any(cap)) {
    cat(sprintf("  out-leg was last week's captain in %d packages (out-side raw understates)\n", sum(cap)))
  }
}

cat("\n  rung pattern, preference hits only (registered H' — the HOLDING minus HORIZON clearance gap):\n")
for (a in c("mgh3", "mgh4", "mgh5", "mgh6")) {
  r <- hits[hits$arm == a & hits$hold_out_played, ]
  cat(sprintf("  %s: hold %.0f%%, horizon %.0f%%, gap %+.0fpp (%d preference hits)\n",
              a, 100*mean(r$worked4), 100*mean(r$workedH),
              100*mean(r$worked4) - 100*mean(r$workedH), nrow(r)))
}

cat("\n  free packages, holding bar 0 (descriptive):\n")
frees <- h[grepl("^mgh", h$arm) & !h$hit, ]
for (arm in c("mgh3", "mgh3floored", "mgh3fullplan")) {
  a <- frees[frees$arm == arm, ]
  cat(sprintf("  %s: %d free packages, %.0f%% non-negative hold, mean %+.1f\n",
              arm, nrow(a), 100*mean(a$hold_net >= 0), mean(a$hold_net)))
}

cat("\n=== the wildcard-week-after split (descriptive) ===\n")
for (arm in c("mgh3floored", "mgh3fullplan")) {
  a <- hits[hits$arm == arm, ]
  if (any(a$wildcard_after)) {
    wa <- a[a$wildcard_after, ]
    nw <- a[!a$wildcard_after, ]
    cat(sprintf("  %s: after-wildcard hits %d, %.0f%% clear +4 in the hold;  other weeks %d, %.0f%%\n",
                arm, nrow(wa), 100*mean(wa$worked4), nrow(nw), 100*mean(nw$worked4)))
  } else {
    cat(sprintf("  %s: no wildcard-after packages recorded (its plan plays no wildcard)\n", arm))
  }
}
