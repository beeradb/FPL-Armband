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
# this script reports the sidecar side). Descriptive: the workedOut table
# (hitNet - waitedNet >= 4, matched against the no-hits arm's later free
# purchase of the same in-player), the wildcard-week-after split, and the
# availability-adjusted rates beside the raw.
#
# The package is the unit: a funded pair is one row, its legs summed, its hit
# charged once — the unit the gate judged.

args <- commandArgs(trailingOnly = TRUE)
if (length(args) < 1) stop("usage: hittune_verdicts.R <hits.csv>")

h <- read.csv(args[1], stringsAsFactors = FALSE)
h$arm <- sub(", flat.*|, floored.*| \\(wait\\)", "", h$arm)
h$net3 <- h$hit_net < 3
h$net0 <- h$hit_net < 0
h$adj <- h$out_played

hits <- h[grepl("^mgh", h$arm), ]
baseline <- hits[hits$arm == "mgh3", ]

cat("=== the shipped arm's hits, against the gate's own bar ===\n")
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

cat("=== loss rates by arm (net < 3), paired per cell ===\n")
cell <- aggregate(net3 ~ arm + season + start_gw, hits, mean)
arms <- sort(unique(cell$arm))
wide <- reshape(cell, idvar = c("season", "start_gw"), timevar = "arm",
                direction = "wide")
names(wide) <- sub("^net3\\.", "", names(wide))
for (a in c("mgh4", "mgh5", "mgh6")) {
  d <- wide[[a]] - wide[["mgh3"]]
  cat(sprintf("  %s - mgh3: mean %+.3f per-cell rate, %d cells, %d negative\n",
              a, mean(d, na.rm = TRUE), sum(!is.na(d)), sum(d < 0, na.rm = TRUE)))
}
fl <- hits[hits$arm == "mgh3floored", ]
cat(sprintf("  floored machine (own arm): net<3 %.1f%% of %d packages; net<0 %.1f%%\n",
            100*mean(fl$net3), nrow(fl), 100*mean(fl$net0)))

cat("\n=== workedOut: hit packages against the no-hits arm's later free purchase ===\n")
wait <- h[grepl("^no hits", h$arm), ]
matchNet <- function(season, start, gw, inIds) {
  ids <- unlist(strsplit(inIds, "|", fixed = TRUE))
  w <- wait[wait$season == season & wait$start_gw == start &
            wait$gw >= gw & wait$gw <= gw + 4, ]
  if (nrow(w) == 0) return(NA)
  for (i in seq_len(nrow(w))) {
    wids <- unlist(strsplit(w$in_ids[i], "|", fixed = TRUE))
    if (any(wids %in% ids)) return(w$hit_net[i])
  }
  NA
}
worked <- rep(NA_real_, nrow(baseline))
for (i in seq_len(nrow(baseline))) {
  b <- baseline[i, ]
  worked[i] <- matchNet(b$season, b$start_gw, b$gw, b$in_ids)
}
matched <- !is.na(worked)
wo <- (baseline$hit_net - worked) >= 4
cat(sprintf("hit packages: %d; matched to a later free purchase: %d (%.0f%%)\n",
            nrow(baseline), sum(matched), 100*mean(matched)))
cat(sprintf("  workedOut (>= +4 vs waiting): %d of %d matched (%.0f%%)\n",
            sum(wo[matched]), sum(matched), 100*mean(wo[matched])))
cat(sprintf("  mean hitNet %+.1f vs mean waitedNet %+.1f over the matched pairs\n",
            mean(baseline$hit_net[matched]), mean(worked[matched])))

cat("\n=== the wildcard-week-after split (floored arm, descriptive) ===\n")
if (any(fl$wildcard_after)) {
  wa <- fl[fl$wildcard_after, ]
  nw <- fl[!fl$wildcard_after, ]
  cat(sprintf("  after-wildcard packages: %d, net<3 %.0f%%;  other weeks: %d, net<3 %.0f%%\n",
              nrow(wa), 100*mean(wa$net3), nrow(nw), 100*mean(nw$net3)))
} else {
  cat("  no wildcard-after packages recorded\n")
}
