# Sensitivity of the BLENDLO ladder, reproduced from this snapshot's own cells.
#
# ⚠️ Reads through cells_common.R from 2026-08-14. It previously read the file raw
# — no coercion, no contract check, and the narrow `season@start_gw` key — which
# made it the ninth independent reader of the cells contract and the second one
# living under `stats/snapshots/`, where the guard's non-recursive glob could not
# see it. `blendlo.csv` is single-block, so nothing it prints moves.
local({
  a <- commandArgs(trailingOnly = FALSE)
  f <- sub("^--file=", "", a[grep("^--file=", a)])
  d <- if (length(f) > 0) dirname(normalizePath(f[1])) else "."
  p <- file.path(d, "..", "..", "cells_common.R")
  if (!file.exists(p)) {
    stop("CONTRACT VIOLATION: cells_common.R not found: ", p)
  }
  source(p, local = FALSE)
})
d <- read_cells("stats/snapshots/2026-08-14-blendlo/cells/blendlo.csv")
d$cell <- paste(d$season, d$start_gw, sep = "@")
base <- d[grepl("ships", d$variant), ]
arms <- setdiff(unique(d$variant), base$variant)
bh <- setNames(base$hold_per_gw, base$cell)

cat("Leave-one-season-out, HOLD, points a season (x38)\n")
cat(sprintf("%-14s %8s", "arm", "all"))
seasons <- sort(unique(d$season))
for (s in seasons) cat(sprintf(" %9s", paste0("-", substr(s, 3, 7))))
cat("   sign flips\n")
for (a in arms) {
  q <- d[d$variant == a, ]
  diff <- q$hold_per_gw - bh[q$cell]
  all_m <- mean(diff) * 38
  outs <- numeric(0)
  for (s in seasons) outs <- c(outs, mean(diff[q$season != s]) * 38)
  flips <- sum(sign(outs) != sign(all_m))
  cat(sprintf("%-14s %8.1f", a, all_m))
  for (v in outs) cat(sprintf(" %9.1f", v))
  cat(sprintf("   %d of 6\n", flips))
}

cat("\nThe one contrast with |t CR2| above t_crit: k=16 minus k=12\n")
h12 <- setNames(d$hold_per_gw[d$variant == "BlendRateK=12"], d$cell[d$variant == "BlendRateK=12"])
h16 <- setNames(d$hold_per_gw[d$variant == "BlendRateK=16"], d$cell[d$variant == "BlendRateK=16"])
cells <- names(h12)
con <- h16[cells] - h12[cells]
seas <- sub("@.*", "", cells)
cat(sprintf("  overall %.3f pts/gw = %.1f a season, positive in %d of %d cells\n",
            mean(con), mean(con) * 38, sum(con > 0), length(con)))
for (s in seasons) {
  v <- con[seas == s]
  cat(sprintf("    %-9s %7.3f pts/gw   positive in %d of %d\n", s, mean(v), sum(v > 0), length(v)))
}
cat("  leave-one-season-out:\n")
for (s in seasons) {
  cat(sprintf("    drop %-9s %7.1f a season\n", s, mean(con[seas != s]) * 38))
}
top <- sort(abs(con), decreasing = TRUE)[1:3]
cat(sprintf("  three largest |cell| contributions: %s\n",
            paste(sprintf("%s %.2f", names(top), con[names(top)]), collapse = "; ")))
cat(sprintf("  dropping just those three: %.1f a season (from %.1f)\n",
            mean(con[!(names(con) %in% names(top))]) * 38, mean(con) * 38))
