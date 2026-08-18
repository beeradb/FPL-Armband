#!/usr/bin/env Rscript

# The three registered contrasts of the prior-reactivity 2x2
# (TestDiagPriorReactivityUnderExitLevers), computed per cell and read through
# the same SE machinery as sweep_inference.R: season-clustered CR2 with
# Satterthwaite df, start-fixed as the robustness rival, and the wild cluster
# bootstrap. Holm over the three.
#
# Usage:  Rscript stats/priorx_contrasts.R /tmp/priorx.csv
#
# Cells carry policy_per_gw per (variant, season, start_gw). The four arms are
# pivoted wide on (season, start_gw), then:
#   A      (k24-k8) averaged over levers off and on
#   B      (on-off) averaged over k=8 and k=24
#   AxB    (k24-k8)|on  minus  (k24-k8)|off

args <- commandArgs(trailingOnly = TRUE)
if (length(args) < 1) stop("usage: priorx_contrasts.R <cells.csv>")
source("stats/cells_common.R")

# Duplicated from sweep_inference.R rather than sourced, for the same reason
# given there: this one-off must run standalone against a single cells file.
season_share <- function(d) {
  n_dup <- sum(duplicated(paste(d$season, d$start_gw)))
  if (n_dup > 0) return(NULL)
  tab <- tapply(d$diff, list(d$season, d$start_gw), function(x) x[1])
  if (is.null(dim(tab)) || any(is.na(tab))) return(NULL)
  S <- nrow(tab); G <- ncol(tab)
  if (S < 2 || G < 2) return(NULL)
  grand <- mean(tab)
  a <- rowMeans(tab) - grand
  b <- colMeans(tab) - grand
  e <- tab - outer(rowMeans(tab), colMeans(tab), "+") + grand
  ms_s <- G * sum(a^2) / (S - 1)
  ms_g <- S * sum(b^2) / (G - 1)
  ms_e <- sum(e^2) / ((S - 1) * (G - 1))
  v_s <- (ms_s - ms_e) / G
  v_g <- (ms_g - ms_e) / S
  tot <- max(v_s, 0) + max(v_g, 0) + ms_e
  if (!is.finite(tot) || tot <= 0) return(NULL)
  list(S = S, G = G,
       v_season = v_s, v_start = v_g, v_resid = ms_e,
       share_season = max(v_s, 0) / tot,
       share_start = max(v_g, 0) / tot,
       share_resid = ms_e / tot)
}

se_fixed <- function(d) {
  v <- season_share(d)
  if (is.null(v)) return(list(se = NA, df = NA, t = NA, p = NA))
  se <- sqrt(v$v_resid / (v$S * v$G))
  if (!is.finite(se) || se <= 0) return(list(se = 0, df = NA, t = NA, p = 1))
  df <- (v$S - 1) * (v$G - 1)
  tt <- mean(d$diff) / se
  list(se = se, df = df, t = tt, p = 2 * pt(-abs(tt), df))
}

cells <- read_cells(args[1])
need <- c("k8, levers off", "k24, levers off", "k8, levers on", "k24, levers on")
stopifnot(all(need %in% cells$variant))

w <- reshape(cells[, c("variant", "season", "start_gw", "policy_per_gw")],
             idvar = c("season", "start_gw"), timevar = "variant",
             direction = "wide")
names(w) <- sub("^policy_per_gw\\.", "", names(w))

off8  <- w[["k8, levers off"]]
off24 <- w[["k24, levers off"]]
on8   <- w[["k8, levers on"]]
on24  <- w[["k24, levers on"]]

contr <- data.frame(
  season   = w$season,
  start_gw = w$start_gw,
  A   = ((off24 - off8) + (on24 - on8)) / 2,
  B   = ((on8 - off8) + (on24 - off24)) / 2,
  AxB = (on24 - on8) - (off24 - off8)
)

fmt <- function(name, d, holm_p) {
  cr <- se_cr2(d)
  fx <- se_fixed(d)
  wb <- tryCatch(wild_cluster_p_season(d),
                 error = function(e) list(p = NA, S_eff = NA, floor = NA))
  cat(sprintf("\n%-12s mean %+6.1f a season  (%+.4f pts/gw)\n",
              name, mean(d$diff) * 38, mean(d$diff)))
  cat(sprintf("  CR2    SE %6.2f  df %4.1f  t %+5.2f  p %6.4f  thr %5.1f%s\n",
              cr$se * 38, cr$df, cr$t, cr$p,
              if (!is.na(cr$p)) qt(0.975, cr$df) * cr$se * 38 else NA,
              if (!is.na(holm_p)) sprintf("   Holm %6.4f", holm_p) else ""))
  cat(sprintf("  start  SE %6.2f  df %4.0f  t %+5.2f  p %6.4f  thr %5.1f\n",
              fx$se * 38, fx$df, fx$t, fx$p,
              if (!is.na(fx$p)) qt(0.975, fx$df) * fx$se * 38 else NA))
  cat(sprintf("  wild   p %s   S_eff %s   floor %s\n",
              ifelse(is.na(wb$p), "  NA  ", sprintf("%6.4f", wb$p)),
              ifelse(is.na(wb$S_eff), "NA", as.character(wb$S_eff)),
              ifelse(is.na(wb$floor), "  NA  ", sprintf("%6.4f", wb$floor))))
}

rows <- list(
  A   = data.frame(season = contr$season, start_gw = contr$start_gw, diff = contr$A),
  B   = data.frame(season = contr$season, start_gw = contr$start_gw, diff = contr$B),
  AxB = data.frame(season = contr$season, start_gw = contr$start_gw, diff = contr$AxB)
)

ps <- sapply(rows, function(d) se_cr2(d)$p)
holm <- p.adjust(ps, method = "holm")

cat("=== registered contrasts of the prior-reactivity 2x2 (POLICY) ===\n")
cat(sprintf("n = %d cells per contrast; Holm over the three CR2 p's\n", nrow(contr)))
for (nm in c("A", "B", "AxB")) fmt(nm, rows[[nm]], holm[[nm]])

cat("\n--- simple effects of k, reported beside the factorial ones ---\n")
fmt("k24-k8 | off",
    data.frame(season = contr$season, start_gw = contr$start_gw, diff = off24 - off8), NA)
fmt("k24-k8 | on",
    data.frame(season = contr$season, start_gw = contr$start_gw, diff = on24 - on8), NA)
