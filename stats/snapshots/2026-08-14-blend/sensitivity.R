#!/usr/bin/env Rscript
#
# The sensitivity analyses quoted in FINDINGS.md, banked so they are
# reproducible from this snapshot rather than from one session's scrollback.
#
#   Rscript stats/snapshots/2026-08-14-blend/sensitivity.R
#
# Everything here is the schedule test's own two-stage statistic, reusing
# stats/cells_common.R so it cannot drift from schedule_screen.R: within each
# (season, entry) cell fit the ladder slope b on the setting, then within each
# season fit b's trend across entry points, then average over seasons. What this
# adds is the leave-one-out and drop-a-column variants that decide how much of
# the headline is carried by how little.

here <- dirname(normalizePath(sub("^--file=", "",
  commandArgs(trailingOnly = FALSE)[grep("^--file=", commandArgs(trailingOnly = FALSE))][1])))
source(file.path(here, "..", "..", "cells_common.R"))

# ⚠️ Through the shared reader, from 2026-08-14. This script is INSIDE a snapshot
# directory and is still a caller of the shared library rather than an archive of
# one: when `diffs_for` gained a required `min_cells` it aborted outright, and a
# reproduction script that cannot run is not a reproduction. It also carried its
# own flag coercion and the narrow `season@start_gw` key, which is the defect A2
# removed from `grid_width.R`.
#
# `blend.csv` is single-block, so the wide key changes nothing here — this is
# consistency rather than a repair.
d <- read_cells(file.path(here, "cells", "blend.csv"))
d <- d[!d$infeasible, ]

arms <- diffs_for(d, "hold", "_per_gw", min_cells = 2, quiet = TRUE)
SET <- c("BlendRateK=12" = 12, "BlendRateK=16" = 16, "BlendRateK=24" = 24)
BASE <- 8
SEASONS <- sort(unique(d$season))
ENTRIES <- c(1, 6, 11, 16, 21, 26)

# The schedule statistic, over a chosen subset of seasons, entries and arms.
sched <- function(seasons = SEASONS, entries = ENTRIES, use = names(arms)) {
  cs <- c()
  for (s in seasons) {
    bs <- c()
    for (g in entries) {
      x <- BASE; y <- 0
      for (nm in use) {
        v <- arms[[nm]]$diff[arms[[nm]]$season == s & arms[[nm]]$start_gw == g]
        if (length(v) == 1) { x <- c(x, SET[[nm]]); y <- c(y, v) }
      }
      if (length(x) >= 3) bs <- c(bs, coef(lm(y ~ x))[2])
    }
    if (length(bs) >= 3) cs[s] <- coef(lm(bs ~ entries[seq_along(bs)]))[2]
  }
  q <- data.frame(season = names(cs), diff = as.numeric(cs))
  st <- se_cr(q, q$season)
  list(mean = mean(q$diff), t = st$t, p = st$p, per_season = cs)
}

line <- function(lab, r) {
  cat(sprintf("%-34s mean %+.5f   t %+.2f   p %.3f\n", lab, r$mean, r$t, r$p))
}

hr(); note("The schedule statistic, and what carries it")
full <- sched()
line("all 6 seasons, all 6 entries", full)
cat("\nper-season contrast:\n"); print(round(full$per_season, 5))

hr(); note("Leave one season out")
for (s in SEASONS) line(paste("drop", s), sched(seasons = setdiff(SEASONS, s)))

hr(); note("The two carrier seasons, dropped together — the sign reverses")
line("drop 2021-22 AND 2023-24",
     sched(seasons = setdiff(SEASONS, c("2021-22", "2023-24"))))

hr(); note("Leave one entry column out")
for (g in ENTRIES) line(paste("drop GW", g), sched(entries = setdiff(ENTRIES, g)))

hr(); note("Leave one arm out — is the statistic the k=24 arm alone?")
for (nm in names(arms)) line(paste("drop", nm), sched(use = setdiff(names(arms), nm)))

hr(); note("Degeneracy and tails")
p <- do.call(rbind, arms)
cat(sprintf("arm-cells %d, exactly zero %d (%.0f%%)\n",
            nrow(p), sum(p$diff == 0), 100 * mean(p$diff == 0)))
cat("zeros by entry gameweek:\n"); print(tapply(p$diff, p$start_gw, function(x) sum(x == 0)))
cat("per-entry SD of the paired differences:\n")
print(round(tapply(p$diff, p$start_gw, sd), 3))
for (nm in names(arms)) {
  x <- arms[[nm]]$diff
  cat(sprintf("  %-16s max|d| %.2f = %.0f%% of sum|d|\n",
              nm, max(abs(x)), 100 * max(abs(x)) / sum(abs(x))))
}

hr(); note("The baseline-free ladder slope, per entry column")
note("This is the estimand the post-hoc calendar-vs-evidence paragraph rests on:")
note("the common baseline is an additive shift and cancels from a slope, so this")
note("cannot be an artifact of the k=8 column's own draw — which the per-arm")
note("differences CAN be, since all three share it.")
for (g in ENTRIES) {
  bs <- c()
  for (s in SEASONS) {
    x <- BASE; y <- 0
    for (nm in names(arms)) {
      v <- arms[[nm]]$diff[arms[[nm]]$season == s & arms[[nm]]$start_gw == g]
      if (length(v) == 1) { x <- c(x, SET[[nm]]); y <- c(y, v) }
    }
    bs[s] <- coef(lm(y ~ x))[2]
  }
  q <- data.frame(season = SEASONS, diff = as.numeric(bs))
  st <- se_cr(q, q$season)
  cat(sprintf("  GW%-3d slope %+.4f   t %+.2f\n", g, mean(q$diff), st$t))
}

hr(); note("The calendar reading's own prediction, for the same columns")
note("share of the scored window [g,38] falling in the GW16-28 band:")
cat("  ", paste(sprintf("GW%d %.3f", ENTRIES,
      vapply(ENTRIES, function(g) length(intersect(16:28, g:38)) / (39 - g), 0)),
      collapse = "   "), "\n")
note("It peaks at GW16 and is LOWEST at GW26 — so 'GW11 contains all of the band'")
note("is not the criterion, and the calendar reading predicts GW26 smallest.")

hr(); note("Restricted to the four seasons of the recorded 24-cell table")
four <- c("2022-23", "2023-24", "2024-25", "2025-26")
dd <- d[d$season %in% four, ]
for (nm in names(diffs_for(dd, "hold", "_per_gw", min_cells = 2, quiet = TRUE))) {
  x <- diffs_for(dd, "hold", "_per_gw", min_cells = 2, quiet = TRUE)[[nm]]
  st <- se_cr(x, x$season)
  cat(sprintf("  %-16s mean %+.3f  t %+.2f  (recorded table: -0.632 / -0.740 / -1.509)\n",
              nm, mean(x$diff), st$t))
}
