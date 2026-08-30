d <- read.csv("/work/drop/breakout-2026-08-30/candidates.csv")
tcrit <- qt(0.975, df=5)
cat(sprintf("candidates=%d  seasons=%d  t_crit(df=5)=%.3f\n\n", nrow(d), length(unique(d$season)), tcrit))

cat("=== VOID CHECKS ===\n")
tb <- table(d$season); print(tb)
cat(sprintf("  outcome out_points_10: mean %.1f  sd %.1f  min %d  max %d\n",
            mean(d$out_points_10), sd(d$out_points_10), min(d$out_points_10), max(d$out_points_10)))
cat(sprintf("  distinct players: %d\n\n", length(unique(paste(d$season,d$player_id)))))

preds <- c("engine_score","hot_xgxa_per90","hot_start_share","minutes_trend",
           "hot_points","hot_pts_minus_xgi","price_tenths","hot_minutes")

cat("=== SPEARMAN of each predictor against out_points_10, season-clustered df 5 ===\n")
res <- list()
for (p in preds) {
  per <- sapply(unique(d$season), function(s) {
    x <- d[d$season==s,]
    suppressWarnings(cor(x[[p]], x$out_points_10, method="spearman"))
  })
  m <- mean(per); se <- sd(per)/sqrt(length(per)); thr <- tcrit*se
  pv <- t.test(per)$p.value
  res[[p]] <- c(m=m, se=se, p=pv)
  cat(sprintf("  %-18s rho %+6.3f  SE %5.3f  thr +/-%5.3f  %s  seasons +ve %d/6\n",
              p, m, se, thr, ifelse(abs(m)>thr,"*** RESOLVES","not detected"), sum(per>0)))
}

cat("\n=== Does anything beat engine_score? (paired per season) ===\n")
base <- sapply(unique(d$season), function(s){x<-d[d$season==s,]; cor(x$engine_score,x$out_points_10,method="spearman")})
for (p in setdiff(preds,"engine_score")) {
  per <- sapply(unique(d$season), function(s){x<-d[d$season==s,]; suppressWarnings(cor(x[[p]],x$out_points_10,method="spearman"))})
  dd <- per-base; m<-mean(dd); se<-sd(dd)/sqrt(length(dd)); thr<-tcrit*se
  cat(sprintf("  %-18s minus engine_score %+6.3f  SE %5.3f  thr +/-%5.3f  %s\n",
              p, m, se, thr, ifelse(abs(m)>thr,"*** RESOLVES","not detected")))
}

ps <- sapply(preds, function(p) res[[p]]["p"])
h <- p.adjust(ps, method="holm")
cat("\n=== HOLM across the eight predictors ===\n")
for (i in seq_along(preds)) cat(sprintf("  %-18s p=%.4f  Holm=%.4f  %s\n", preds[i], ps[i], h[i],
                                        ifelse(h[i]<0.05,"*** survives","does not survive")))

cat("\n=== TOP vs BOTTOM THIRD of out_points_10, within season ===\n")
d$tercile <- ave(d$out_points_10, d$season, FUN=function(v) as.integer(cut(rank(v,ties.method="first"), 3, labels=FALSE)))
for (p in preds) {
  hi <- mean(d[[p]][d$tercile==3]); lo <- mean(d[[p]][d$tercile==1])
  cat(sprintf("  %-18s top third %8.3f   bottom third %8.3f   ratio %5.2f\n", p, hi, lo,
              ifelse(lo!=0, hi/lo, NA)))
}
cat(sprintf("\n  out_points_10  top third %.1f  bottom third %.1f\n",
            mean(d$out_points_10[d$tercile==3]), mean(d$out_points_10[d$tercile==1])))

cat("\n=== FACE VALIDITY: the named players (pre-declared, NOT evidence) ===\n")
nm <- c("Palmer","Semenyo","Rogers","Wilson","Anderson","Anthony")
sub <- d[grepl(paste(nm,collapse="|"), d$web_name, ignore.case=TRUE),]
sub <- sub[order(-sub$out_points_10),]
cat(sprintf("  %-8s %-4s %-14s %5s %5s %7s %8s %8s %7s\n",
            "season","gw","name","price","hot","xgxa90","startsh","engScore","OUT10"))
for (i in 1:nrow(sub)) with(sub[i,],
  cat(sprintf("  %-8s %-4d %-14s %5.1f %5d %7.3f %8.2f %8.3f %7d\n",
              season, gw, web_name, price_tenths/10, hot_points, hot_xgxa_per90,
              hot_start_share, engine_score, out_points_10)))
cat(sprintf("\n  within-season percentile of engine_score, for the named rows:\n"))
for (i in 1:nrow(sub)) { r<-sub[i,]; pool<-d$engine_score[d$season==r$season]
  cat(sprintf("    %-8s gw%-3d %-14s engine_score pct %5.1f%%   OUT10 pct %5.1f%%\n",
      r$season, r$gw, r$web_name, 100*mean(pool<r$engine_score),
      100*mean(d$out_points_10[d$season==r$season] < r$out_points_10))) }
