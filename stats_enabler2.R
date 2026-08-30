d <- read.csv("/work/drop/enabler-2026-08-30/candidates.csv")
tcrit <- qt(0.975, df=5); seasons <- unique(d$season)
# ⚠️ enabler_points in the CSV is a PER-GAMEWEEK rate (PointsPerTenth's own unit).
d$enab_per_gw <- d$enabler_points
d$enab_10gw   <- d$enab_per_gw * 10
d$rest        <- pmax(38 - d$gw, 0)
d$enab_rest   <- d$enab_per_gw * d$rest

cat("=== corrected: enabler value is a RATE, so it must be multiplied by a horizon ===\n")
cat(sprintf("  pooled per-gameweek %+.4f pts/gw\n", mean(d$enab_per_gw)))
cat(sprintf("  over 10 GW  %+.3f pts   against own_points_10 %.2f   ratio %+.4f\n",
  mean(d$enab_10gw), mean(d$own_points_10), mean(d$enab_10gw)/mean(d$own_points_10)))
cat(sprintf("  to season end (mean %.1f GW remaining) %+.3f pts\n",
  mean(d$rest), mean(d$enab_rest)))

cat("\n=== THE TAIL: top decile of raw rise ===\n")
top <- d[d$raw_rise_tenths >= quantile(d$raw_rise_tenths,0.9),]
cat(sprintf("  n=%d  mean rise %+.1f tenths\n", nrow(top), mean(top$raw_rise_tenths)))
cat(sprintf("  over 10 GW  %+.2f pts vs own %.1f  -> %.1f%% of his own return\n",
  mean(top$enab_10gw), mean(top$own_points_10), 100*mean(top$enab_10gw)/mean(top$own_points_10)))
cat(sprintf("  to season end %+.2f pts (mean %.1f GW left)\n", mean(top$enab_rest), mean(top$rest)))

cat("\n=== the five biggest risers, to season end ===\n")
b <- d[order(-d$raw_rise_tenths),][1:5,]
for(i in 1:5) with(b[i,], cat(sprintf("  %-8s gw%-3d %-13s +%2.0f tenths | %4.1f pts over 10GW | %5.1f pts to season end | own %3.0f\n",
  season,gw,web_name,raw_rise_tenths,enab_10gw,enab_rest,own_points_10)))

cat("\n=== season-clustered, ratio against the pre-registered 15% bar ===\n")
for (lab in c("10gw","rest")) {
  col <- if (lab=="10gw") d$enab_10gw else d$enab_rest
  per <- sapply(seasons, function(s) mean(col[d$season==s]/pmax(d$own_points_10[d$season==s],1)))
  m<-mean(per); se<-sd(per)/sqrt(6)
  cat(sprintf("  %-5s ratio %+.4f  SE %.4f  thr +/-%.4f  %s vs bar 0.15\n", lab,m,se,tcrit*se,
    ifelse(abs(m)>tcrit*se,"resolves","not detected")))
}
