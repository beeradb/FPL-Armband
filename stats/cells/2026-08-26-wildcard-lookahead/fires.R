# The fire counts, median fired gameweek and hits per cell behind this README.
#
#   Rscript stats/cells/2026-08-26-wildcard-lookahead/fires.R
#
# Run from the repository root.
#
# ⚠️ **This exists because those three columns had no generator.** The README's
# effects and standard errors come from `stats/sweep_inference.R`, which is
# committed — but the fire counts were derived once with an ad-hoc script that was
# never committed, so a reader could not reproduce the numbers the README leans on
# hardest. The same failure had just been caught one directory over, where a
# 20,000-draw figure was quoted from a deleted scratch test.
#
# The fire counts are not decoration here. Three of the eight arms fire in 4 of 36
# cells and read as small tidy positive effects; without the counts they look like
# safe, mildly helpful rules rather than arms that are mostly the control. **An
# arm that never fires is inert, not neutral.**
source("stats/cells_common.R")

d <- read_cells("stats/cells/2026-08-26-wildcard-lookahead/lookahead.csv")
d <- d[!d$infeasible, ]

cat(sprintf("\n%-52s%8s%7s%9s%10s\n", "arm", "fired", "medGW", "weighed", "hits/cell"))
for (v in unique(d$variant)) {
  a <- d[d$variant == v, ]
  gws <- a$wc_trig_gw[!is.na(a$wc_trig_gw) & a$wc_trig_gw > 0]
  cat(sprintf("%-52s%4d/%-3d%7s%9.1f%10.2f\n",
              substr(v, 1, 50), length(gws), nrow(a),
              if (length(gws)) format(median(gws)) else "-",
              mean(a$wc_trig_weighed, na.rm = TRUE),
              mean(a$hits, na.rm = TRUE)))
}
cat("\n⚠️ `wc_trig_gw` is the FIRST firing, not the only one — chips come in two\n")
cat("sets from 2025-26. This sweep is confined to GW1-19, one set, so first and\n")
cat("only coincide here; they do not in general.\n")
cat("\n⚠️ Read the fire count before reading any null in the README's table.\n")
