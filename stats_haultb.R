w <- read.csv("/work/drop/haultiebreak-2026-08-30/weeks.csv")
s <- read.csv("/work/drop/haultiebreak-2026-08-30/seasons.csv")
tcrit <- qt(0.975, df=5); seasons <- unique(w$season)
cat(sprintf("t_crit(df=5)=%.3f\n\n", tcrit))
cat("=== 4. VOID CHECK (first) ===\n")
for (a in c("haul","price")) cat(sprintf("  %-6s slots changed: %s (mean %.1f/15)\n", a,
  paste(s$opening_slots_vs_baseline[s$arm==a],collapse=" "), mean(s$opening_slots_vs_baseline[s$arm==a])))

pair <- function(v,b,lab,unit,pred) {
  d<-v-b; m<-mean(d); se<-sd(d)/sqrt(length(d)); thr<-tcrit*se
  cat(sprintf("  %-22s %+8.3f %s SE %6.3f thr +/-%6.3f  %s   (predicted %s)\n",
    lab,m,unit,se,thr, ifelse(abs(m)>thr,"*** RESOLVES","not detected"), pred))
  t.test(d)$p.value }

cat("\n=== 1. PRIMARY -- UPSIDE (predicted HIGHER) ===\n")
p90 <- function(a) sapply(seasons,function(ss) quantile(w$gross[w$season==ss&w$arm==a],0.90))
b90 <- p90("baseline"); ps<-c()
cat(sprintf("  baseline p90 by season: %s (mean %.1f)\n", paste(round(b90),collapse=" "), mean(b90)))
for (a in c("haul","price")) ps<-c(ps,pair(p90(a),b90,paste0("p90 weekly, ",a),"pts","higher"))

big <- function(a) sapply(seasons,function(ss){
  bar <- quantile(w$gross[w$season==ss&w$arm=="baseline"],0.75)
  100*mean(w$gross[w$season==ss&w$arm==a] > bar)})
bb <- big("baseline")
cat(sprintf("  baseline big-week rate: %.1f%%\n", mean(bb)))
for (a in c("haul","price")) ps<-c(ps,pair(big(a),bb,paste0("big-week rate, ",a),"pp ","higher"))

cat("\n=== 2. PRIMARY -- THE COST CHECK (points must not FALL) ===\n")
bp <- s$points[s$arm=="baseline"][order(s$season[s$arm=="baseline"])]
for (a in c("haul","price")) {
  v <- s$points[s$arm==a][order(s$season[s$arm==a])]
  pair(v,bp,paste0("season points, ",a),"pts","flat") }
cat(sprintf("\n  per-season haul-minus-baseline: %s\n",
  paste(sprintf("%+d", s$points[s$arm=="haul"][order(s$season[s$arm=="haul"])]-bp), collapse=" ")))
cat("  ceiling by construction 0.41 x 38 = 15.6; canonical resolvable effect 39\n")

h<-p.adjust(ps,method="holm")
cat("\n=== HOLM across the four PRIMARY comparisons ===\n")
nm<-c("p90 haul","p90 price","bigweek haul","bigweek price")
for(i in 1:4) cat(sprintf("  %-16s p=%.4f Holm=%.4f %s\n",nm[i],ps[i],h[i],
  ifelse(h[i]<0.05,"*** survives","does not survive")))
