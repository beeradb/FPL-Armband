# Does the reconstruction's tighter WITHIN-season spread resolve, or is it noise?
#
# The cells are the SAME 18 (season, start) pairs under all three inputs, so the
# test is PAIRED: for each cell take |difference - its season's mean difference|
# under each input, and compare those absolute deviations cell by cell. That is a
# paired Levene statistic and it uses the pairing, which an F-test on two pooled
# variances throws away.
arms <- c("native", "external", "reconstruction")
D <- list()
for (a in arms) {
  d <- read.csv(sprintf("stats/cells/2026-08-26-xgc-three-inputs/%s.csv", a),
                stringsAsFactors = FALSE)
  d$is_baseline <- tolower(as.character(d$is_baseline))
  b <- d[d$is_baseline == "true", ]
  v <- d[d$variant == "anchored, 4 gameweeks of sight", ]
  m <- merge(v, b, by = c("season", "start_gw"), suffixes = c(".a", ".b"))
  m$diff <- m$policy_points.a - m$policy_points.b
  m$absdev <- abs(m$diff - ave(m$diff, m$season))
  D[[a]] <- m[order(m$season, m$start_gw), c("season", "start_gw", "diff", "absdev")]
}
cat("arm             mean|dev|   within-SD\n")
for (a in arms) cat(sprintf("%-14s %8.2f %11.2f\n", a, mean(D[[a]]$absdev),
                            mean(tapply(D[[a]]$diff, D[[a]]$season, sd))))
cat("\npaired t on |deviation|, reconstruction against each other arm:\n")
for (a in c("native", "external")) {
  tt <- t.test(D[["reconstruction"]]$absdev, D[[a]]$absdev, paired = TRUE)
  cat(sprintf("  recon - %-10s  mean %+7.2f   t %+6.2f  df %d  p %.3f\n",
              a, mean(D[["reconstruction"]]$absdev - D[[a]]$absdev),
              tt$statistic, tt$parameter, tt$p.value))
}
cat("\ncorrelation of the per-cell differences between arms (are these the same cells moving?):\n")
cat(sprintf("  native~external      %.3f\n", cor(D$native$diff, D$external$diff)))
cat(sprintf("  native~reconstruction %.3f\n", cor(D$native$diff, D$reconstruction$diff)))
cat(sprintf("  external~reconstruction %.3f\n", cor(D$external$diff, D$reconstruction$diff)))

cat("\nAre the RAW cell totals correlated across inputs, while the DIFFERENCE is not?\n")
R <- list()
for (a in arms) {
  d <- read.csv(sprintf("stats/cells/2026-08-26-xgc-three-inputs/%s.csv", a),
                stringsAsFactors = FALSE)
  v <- d[d$variant == "anchored, 4 gameweeks of sight", ]
  R[[a]] <- v[order(v$season, v$start_gw), c("season", "start_gw", "policy_points")]
}
cat(sprintf("  raw policy_points  native~external       %.4f\n", cor(R$native$policy_points, R$external$policy_points)))
cat(sprintf("  raw policy_points  native~reconstruction %.4f\n", cor(R$native$policy_points, R$reconstruction$policy_points)))
cat(sprintf("  raw policy_points  external~reconstruction %.4f\n", cor(R$external$policy_points, R$reconstruction$policy_points)))
cat(sprintf("\n  sd of raw policy_points across cells: %.1f\n", sd(R$native$policy_points)))
cat(sprintf("  sd of the paired DIFFERENCE across cells: %.1f\n", sd(D$native$diff)))
cat("\neffect at 4 gameweeks of sight, all three inputs:\n")
for (a in arms) cat(sprintf("  %-14s %+7.2f\n", a, mean(D[[a]]$diff)))
