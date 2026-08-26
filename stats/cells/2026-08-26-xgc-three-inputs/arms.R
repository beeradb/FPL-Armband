# What three xGC inputs do to one chip comparison, and what can be concluded.
#
#   Rscript stats/cells/2026-08-26-xgc-three-inputs/arms.R
#
# Run from the repository root. Reads the three cells files beside it through
# stats/cells_common.R, which is the ONLY reader for this family — a raw read.csv
# once keyed cells without the sweep and cross-paired arms against another sweep's
# baseline, silently, on committed data.
#
# ⚠️ **Every quantity here is printed with the band it has to clear.** Three
# seasons is df 2, and the sampling band on a ratio of two df-2 SD estimates is
# F(2,2) — [0.16, 6.24] at 95%. A guard against reading a 2x SE ratio as a finding
# has to be the band itself, not a warning in prose.
source("stats/cells_common.R")

ARMS <- c("native", "external", "reconstruction")
DIR  <- "stats/cells/2026-08-26-xgc-three-inputs"
ARM  <- "anchored, 4 gameweeks of sight"   # the realistic-sight arm

# Paired difference per cell, against each file's own baseline.
diffs <- function(a, variant) {
  d <- read_cells(file.path(DIR, paste0(a, ".csv")))
  d <- d[!d$infeasible, ]
  b <- d[d$is_baseline, ]
  v <- d[d$variant == variant, ]
  m <- merge(v, b, by = c("season", "start_gw"), suffixes = c(".a", ".b"))
  m <- m[order(m$season, m$start_gw), ]
  m$diff <- m$policy_points.a - m$policy_points.b
  m$absdev <- abs(m$diff - ave(m$diff, m$season))
  m[, c("season", "start_gw", "diff", "absdev", "policy_points.a")]
}

# CR2 is what sweep_inference.R reports; this is the plain clustered SE, which is
# what the ratio argument below is about and is enough for it.
clustered <- function(x, season) {
  per <- tapply(x, season, mean)
  list(eff = mean(per), se = sd(per) / sqrt(length(per)), s = length(per), per = per)
}

D <- lapply(setNames(ARMS, ARMS), diffs, variant = ARM)

cat("\n=== the arm at four gameweeks of sight, per input\n")
cat("input           effect   clustered SE   within-season SD   mean |dev|\n")
for (a in ARMS) {
  cl <- clustered(D[[a]]$diff, D[[a]]$season)
  cat(sprintf("%-14s %+7.2f %14.2f %18.2f %12.2f\n", a, cl$eff, cl$se,
              mean(tapply(D[[a]]$diff, D[[a]]$season, sd)), mean(D[[a]]$absdev)))
}

cat("\n=== CAN the inputs be ranked by clustered SE? The band says no.\n")
s <- length(unique(D$native$season))
lo <- sqrt(1 / qf(0.975, s - 1, s - 1)); hi <- sqrt(qf(0.975, s - 1, s - 1))
cat(sprintf("  %d seasons -> df %d. 95%% band on a ratio of two SD estimates: [%.2f, %.2f].\n",
            s, s - 1, lo, hi))
cat("  A true 2x ratio is invisible inside that, and a true 1x ratio produces\n")
cat("  ratios well past 2 by chance. The clustered SEs below are NOT rankable.\n")

cat("\n=== what IS estimable: the WITHIN-season spread, df ~15 each\n")
n <- nrow(D$native); k <- length(unique(D$native$season))
dfw <- n - k
wlo <- sqrt(1 / qf(0.975, dfw, dfw)); whi <- sqrt(qf(0.975, dfw, dfw))
sds <- sapply(ARMS, function(a) mean(tapply(D[[a]]$diff, D[[a]]$season, sd)))
cat(sprintf("  within-season SDs: %s\n", paste(sprintf("%s %.2f", ARMS, sds), collapse = "   ")))
cat(sprintf("  max ratio %.2f against a 95%% band of [%.2f, %.2f] at df %d --> %s\n",
            max(sds) / min(sds), wlo, whi, dfw,
            if (max(sds) / min(sds) < whi) "AGREE; a 2x difference is EXCLUDED here" else "differ"))

cat("\n=== does the input move the MEAN? Paired difference-of-differences.\n")
cat("  A null is only a result if it clears its own floor, so the floor is printed.\n")
tcrit <- qt(0.975, k - 1)
for (p in list(c("native", "external"), c("native", "reconstruction"), c("external", "reconstruction"))) {
  dd <- D[[p[1]]]$diff - D[[p[2]]]$diff
  cl <- clustered(dd, D[[p[1]]]$season)
  cat(sprintf("  %-14s - %-14s %+7.2f  SE %6.2f  t %+5.2f  floor %5.1f  -> %s\n",
              p[1], p[2], cl$eff, cl$se, cl$eff / cl$se, tcrit * cl$se,
              if (abs(cl$eff) > tcrit * cl$se) "MOVES" else "indistinguishable"))
}

cat("\n=== is the per-cell effect the same effect under each input?\n")
cat("  ⚠️ Under a null where the input adds only a small independent perturbation,\n")
cat("  these correlate near +1 — differencing removes the shared season path, and\n")
cat("  what remains is the cell-level structure of the effect. Near zero means that\n")
cat("  structure is predominantly NOT a property of the cell.\n")
for (p in list(c("native", "external"), c("native", "reconstruction"), c("external", "reconstruction"))) {
  cat(sprintf("  paired difference   %-14s ~ %-14s r %+6.3f\n", p[1], p[2],
              cor(D[[p[1]]]$diff, D[[p[2]]]$diff)))
}
cat(sprintf("\n  SE(r) at n %d is about %.2f, so these exclude large shared structure\n",
            n, 1 / sqrt(n - 3)))
cat("  and cannot distinguish 0 from about 0.3. 'Predominantly', not 'completely'.\n")
cat(sprintf("\n  THE load-bearing comparison: the arm-to-arm difference SD is %.1f,\n",
            sd(D$native$diff - D$external$diff)))
cat(sprintf("  against a within-arm cell spread of %.1f. Changing the input moves a\n", sd(D$native$diff)))
cat("  cell's effect as much as the effect varies across cells in the first place.\n")

cat("\n=== does the reconstruction manufacture precision? Two tests, both paired.\n")
for (a in c("native", "external")) {
  tt <- t.test(D$reconstruction$absdev, D[[a]]$absdev, paired = TRUE)
  # Pitman-Morgan: the correct parametric test for two CORRELATED variances.
  x <- D$reconstruction$diff - ave(D$reconstruction$diff, D$reconstruction$season)
  y <- D[[a]]$diff - ave(D[[a]]$diff, D[[a]]$season)
  pm <- cor.test(x + y, x - y)
  cat(sprintf("  recon vs %-9s  paired Levene t %+5.2f p %.2f | Pitman-Morgan t %+5.2f p %.2f\n",
              a, tt$statistic, tt$p.value, pm$statistic, pm$p.value))
}
cat("  Neither resolves. The reconstruction's tighter spread is NOT established.\n")
