d <- read.csv("/work/drop/enabler-2026-08-30/candidates.csv")
tcrit <- qt(0.975, df=5); seasons <- unique(d$season)
cat(sprintf("n=%d seasons=%d\n\n", nrow(d), length(seasons)))
cat("=== 1. DO THEY APPRECIATE? raw rise in tenths ===\n")
q <- quantile(d$raw_rise_tenths, c(.1,.25,.5,.75,.9,.99))
cat(sprintf("  mean %+.2f  p10 %+.0f  p25 %+.0f  med %+.0f  p75 %+.0f  p90 %+.0f  p99 %+.0f  max %+.0f\n",
  mean(d$raw_rise_tenths), q[1],q[2],q[3],q[4],q[5],q[6], max(d$raw_rise_tenths)))
cat(sprintf("  share rising %.1f%%  flat %.1f%%  falling %.1f%%\n",
  100*mean(d$raw_rise_tenths>0), 100*mean(d$raw_rise_tenths==0), 100*mean(d$raw_rise_tenths<0)))
cat("\n=== 2. SELL-SIDE (half of a rise, all of a fall) ===\n")
qs <- quantile(d$sell_side_tenths, c(.1,.5,.9,.99))
cat(sprintf("  mean %+.2f  p10 %+.0f  med %+.0f  p90 %+.0f  p99 %+.0f  max %+.0f\n",
  mean(d$sell_side_tenths), qs[1],qs[2],qs[3],qs[4], max(d$sell_side_tenths)))
cat("\n=== 3. THE HEADLINE: enabler points vs own points ===\n")
cat(sprintf("  mean enabler %+.3f pts   mean own %.2f pts   ratio %+.4f\n",
  mean(d$enabler_points), mean(d$own_points_10), mean(d$enabler_points)/mean(d$own_points_10)))
cat("\n  THE TAIL -- top decile of raw rise, which is the owner's claim:\n")
top <- d[d$raw_rise_tenths >= quantile(d$raw_rise_tenths,0.9),]
cat(sprintf("  n=%d  mean rise %+.1f tenths  mean enabler %+.2f pts  mean own %.1f pts  ratio %.3f\n",
  nrow(top), mean(top$raw_rise_tenths), mean(top$enabler_points), mean(top$own_points_10),
  mean(top$enabler_points)/mean(top$own_points_10)))
best <- d[order(-d$raw_rise_tenths),][1:5,]
cat("\n  the five biggest risers:\n")
for(i in 1:5) with(best[i,], cat(sprintf("   %-8s gw%-3d %-14s +%2.0f tenths -> enabler %+.2f pts, own %3.0f pts\n",
  season,gw,web_name,raw_rise_tenths,enabler_points,own_points_10)))
cat("\n=== 4. season-clustered: is the enabler value distinguishable from zero? ===\n")
per <- sapply(seasons, function(s) mean(d$enabler_points[d$season==s]))
m<-mean(per); se<-sd(per)/sqrt(6)
cat(sprintf("  enabler pts %+.4f  SE %.4f  thr +/-%.4f  %s\n", m,se,tcrit*se,
  ifelse(abs(m)>tcrit*se,"*** RESOLVES","not detected")))
perr <- sapply(seasons, function(s) mean(d$enabler_points[d$season==s]/pmax(d$own_points_10[d$season==s],1)))
m2<-mean(perr); se2<-sd(perr)/sqrt(6)
cat(sprintf("  per-player ratio %+.4f  SE %.4f  thr +/-%.4f  (pre-registered bar: 0.15)\n", m2,se2,tcrit*se2))
