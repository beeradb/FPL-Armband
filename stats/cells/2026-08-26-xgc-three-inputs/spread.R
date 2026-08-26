# The variance comparison the tier-0 question actually turns on.
#
# The season-clustered SE over three seasons has df 2 and is a weak instrument.
# The WITHIN-season spread across the six entry points is estimated from 18 cells
# and is what "borrowing strength" — correlation between the xGC estimate and the
# scored path — would compress first. Both are printed; neither is a significance
# test and the script does not compute one.
args <- commandArgs(trailingOnly = TRUE)
for (p in args) {
  d <- read.csv(p, stringsAsFactors = FALSE)
  d <- d[tolower(as.character(d$infeasible)) != "true", ]
  d$is_baseline <- tolower(as.character(d$is_baseline))
  base <- d[d$is_baseline == "true", ]
  if (!nrow(base)) { cat(p, ": no baseline rows\n"); next }
  cat("\n=== ", basename(p), "\n", sep = "")
  for (v in setdiff(unique(d$variant), unique(base$variant))) {
    a <- d[d$variant == v, ]
    m <- merge(a, base, by = c("season", "start_gw"), suffixes = c(".a", ".b"))
    if (!nrow(m)) next
    m$diff <- m$policy_points.a - m$policy_points.b
    per <- tapply(m$diff, m$season, mean)
    within <- tapply(m$diff, m$season, sd)
    se_clu <- sd(per) / sqrt(length(per))
    cat(sprintf("  %-46s  eff %+7.1f  clustered SE %6.2f (df %d)  mean within-season SD %6.2f  n %d\n",
                substr(v, 1, 46), mean(per), se_clu, length(per) - 1L,
                mean(within, na.rm = TRUE), nrow(m)))
  }
}
