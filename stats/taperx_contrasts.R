#!/usr/bin/env Rscript

# The three registered contrasts of the option-decay 2x2
# (TestDiagOptionDecayUnderExitLevers), computed per cell and read through the
# same SE machinery as sweep_inference.R: season-clustered CR2 with
# Satterthwaite df, start-fixed as the robustness rival, and the wild cluster
# bootstrap. Holm over the three.
#
# Usage:  Rscript stats/taperx_contrasts.R /tmp/taperx.csv
#
# Cells carry policy_per_gw per (variant, season, start_gw). The four arms are
# pivoted wide on (season, start_gw), then:
#   ON simple    (taper - flat) | levers on   — PRIMARY
#   OFF simple   (taper - flat) | levers off
#   Interaction  ON simple - OFF simple
# The factorial main (mean over both corners) is printed beside the simples,
# labelled as which it is, outside the Holm family.
#
# The mediator tables at the foot are part of the pre-registered liveness
# floors (the doc comment of the diagnostic), not post-hoc exploration:
# moves-differ census by entry role, ftv_flips by entry role, ftv_mean_charge
# by entry point, and banked_weeks maxima.

args <- commandArgs(trailingOnly = TRUE)
if (length(args) < 1) stop("usage: taperx_contrasts.R <cells.csv>")
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
need <- c("flat, levers off", "taper, levers off", "flat, levers on", "taper, levers on")
stopifnot(all(need %in% cells$variant))

w <- reshape(cells[, c("variant", "season", "start_gw", "policy_per_gw",
                       "policy_xpoints_per_gw", "moves", "hits",
                       "ftv_flips", "ftv_gate_calls", "ftv_mean_charge",
                       "banked_weeks")],
             idvar = c("season", "start_gw"), timevar = "variant",
             direction = "wide")
names(w) <- sub("^policy_per_gw\\.", "", names(w))

offFlat  <- w[["flat, levers off"]]
offTaper <- w[["taper, levers off"]]
onFlat   <- w[["flat, levers on"]]
onTaper  <- w[["taper, levers on"]]

contr <- data.frame(
  season   = w$season,
  start_gw = w$start_gw,
  ON  = onTaper - onFlat,
  OFF = offTaper - offFlat,
  AxB = (onTaper - onFlat) - (offTaper - offFlat),
  # The factorial main, printed beside the simples and outside the Holm family.
  MAIN = ((onTaper - onFlat) + (offTaper - offFlat)) / 2,
  # The second POLICY-side instrument on the primary contrast.
  ON_xpoints = w[["policy_xpoints_per_gw.taper, levers on"]] -
               w[["policy_xpoints_per_gw.flat, levers on"]]
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
  ON  = data.frame(season = contr$season, start_gw = contr$start_gw, diff = contr$ON),
  OFF = data.frame(season = contr$season, start_gw = contr$start_gw, diff = contr$OFF),
  AxB = data.frame(season = contr$season, start_gw = contr$start_gw, diff = contr$AxB)
)

ps <- sapply(rows, function(d) se_cr2(d)$p)
holm <- p.adjust(ps, method = "holm")

cat("=== registered contrasts of the option-decay 2x2 (POLICY) ===\n")
cat(sprintf("n = %d cells per contrast; Holm over the three CR2 p's\n", nrow(contr)))
for (nm in c("ON", "OFF", "AxB")) fmt(nm, rows[[nm]], holm[[nm]])

cat("\n--- the factorial main, beside the simples (outside the Holm family) ---\n")
fmt("taper mean",
    data.frame(season = contr$season, start_gw = contr$start_gw, diff = contr$MAIN), NA)

cat("\n--- the primary contrast on the second POLICY-side instrument ---\n")
fmt("ON xpoints",
    data.frame(season = contr$season, start_gw = contr$start_gw, diff = contr$ON_xpoints), NA)

# Entry-point role decomposition of the primary contrast: GW1/GW6 are the
# shape-clean columns, GW11/GW16 transitional, GW21/GW26 level-cut.
cat("\n--- ON simple by entry point (roles: 1/6 shape-clean, 11/16 transitional, 21/26 level-cut) ---\n")
for (g in sort(unique(contr$start_gw))) {
  d <- contr[contr$start_gw == g, ]
  cat(sprintf("  GW%-3d mean %+6.1f a season (%+0.4f pts/gw), %d cells, %d positive\n",
              g, mean(d$ON) * 38, mean(d$ON), nrow(d), sum(d$ON > 0)))
}

# --- Mediator tables: the pre-registered liveness floors ---

cat("\n--- moves-differ census: taper vs flat, cells where moves differ, by entry ---\n")
for (corner in c("levers off", "levers on")) {
  mf <- w[[paste0("moves.flat, ", corner)]]
  mt <- w[[paste0("moves.taper, ", corner)]]
  d  <- data.frame(start_gw = w$start_gw, differ = mt != mf)
  by <- tapply(d$differ, d$start_gw, function(x) sum(x))
  cat(sprintf("  %-10s %s\n", corner,
              paste(sprintf("GW%d:%d/%d", as.integer(names(by)), by, table(d$start_gw)),
                    collapse = " ")))
}

cat("\n--- ftv_flips: seasons with at least one flip in the taper arms, by entry role ---\n")
for (corner in c("levers off", "levers on")) {
  fl <- w[[paste0("ftv_flips.taper, ", corner)]]
  d  <- data.frame(season = w$season, start_gw = w$start_gw, flips = fl)
  cat(sprintf("  %-10s ", corner))
  for (role in list(shape = c(1, 6), levelcut = c(21, 26))) {
    dd <- d[d$start_gw %in% role, ]
    n_seasons <- sum(tapply(dd$flips, dd$season, sum) > 0)
    cat(sprintf("GW%s: %d of %d seasons with flips   ",
                paste(role, collapse = "/"), n_seasons, length(unique(dd$season))))
  }
  cat("\n")
}

cat("\n--- ftv_mean_charge in the taper arms, by entry point (expected: GW1 ~2.0-2.1, GW26 ~1.3-1.5) ---\n")
for (corner in c("levers off", "levers on")) {
  mc <- w[[paste0("ftv_mean_charge.taper, ", corner)]]
  cat(sprintf("  %-10s %s\n", corner,
              paste(sprintf("GW%d:%.2f", sort(unique(w$start_gw)),
                            tapply(mc, w$start_gw, mean)),
                    collapse = " ")))
}

cat("\n--- banked_weeks maxima per arm (expected 0 everywhere: banking inert) ---\n")
for (v in need) {
  bw <- suppressWarnings(as.numeric(cells$banked_weeks[cells$variant == v]))
  bw <- bw[!is.na(bw)]
  cat(sprintf("  %-16s max %g\n", v, if (length(bw)) max(bw) else 0))
}

cat("\n--- reproduction of the OFF corner's flat rung against the ladder's 2.0 baseline ---\n")
cat(sprintf("  flat, levers off: mean policy_per_gw %+0.4f over %d cells\n",
            mean(offFlat), length(offFlat)))
