d <- read.csv("internal/backtest/testdata/haulchannel/pairs.csv")
tcrit <- qt(0.975, df=5)
cat(sprintf("pairs=%d  seasons=%d  t_crit(df=5)=%.3f\n", nrow(d), length(unique(d$season)), tcrit))
cat("\n=== VOID CHECKS ===\n")
tb <- table(d$season)
cat("  pairs per season: ", paste(sprintf("%s=%d", names(tb), tb), collapse="  "), "\n")
cat(sprintf("  max season share: %.1f%% (void if one season dominates)\n", 100*max(tb)/sum(tb)))
cat(sprintf("  mean |d_points| = %.2f\n", mean(abs(d$d_points))))

sig <- function(col, label) {
  x <- d[[col]]
  keep <- x != 0
  # orient on THIS signal: does the higher-signal player outscore the lower?
  y <- sign(x[keep]) * d$d_points[keep]
  ss <- d$season[keep]
  per <- tapply(y, ss, mean)
  m <- mean(per); se <- sd(per)/sqrt(length(per)); thr <- tcrit*se
  win <- 100*mean(ifelse(y>0,1,ifelse(y<0,0,0.5)))
  p <- t.test(per)$p.value
  cat(sprintf("  %-16s n=%6d  mean %+6.3f pts  SE %5.3f  thr +/-%5.3f  win %5.2f%%  p=%.4f  %s\n",
              label, sum(keep), m, se, thr, win, p,
              ifelse(abs(m)>thr, "*** RESOLVES", "not detected")))
  c(p=p, m=m)
}
cat("\n=== CHANNEL: does the higher-signal player outscore the lower, next GW? ===\n")
r1 <- sig("d_haulrate","haul rate")
r2 <- sig("d_expmins","expected minutes")
r3 <- sig("d_own","ownership")
r4 <- sig("d_price","price")

ps <- c(r1["p"],r2["p"],r3["p"],r4["p"])
h <- p.adjust(ps, method="holm")
cat("\n=== HOLM across the four pre-registered signals ===\n")
nm <- c("haul rate","expected minutes","ownership","price")
for (i in 1:4) cat(sprintf("  %-16s p=%.4f  Holm=%.4f  %s\n", nm[i], ps[i], h[i],
                           ifelse(h[i]<0.05,"*** survives","does not survive")))

cat("\n=== per-season detail for the headline signal (spread, not just the mean) ===\n")
x <- d$d_haulrate; keep <- x!=0
y <- sign(x[keep])*d$d_points[keep]
per <- tapply(y, d$season[keep], mean)
for (n in names(per)) cat(sprintf("  %s  %+6.3f\n", n, per[n]))
