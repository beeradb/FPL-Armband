#!/usr/bin/env Rscript

# The registered contrasts of the gate-floor 2x2
# (TestDiagGateFloorUnderExitLevers), computed per cell and read through the
# same SE machinery as sweep_inference.R: season-clustered CR2 with
# Satterthwaite df, start-fixed as the robustness rival, and the wild cluster
# bootstrap. Holm over the FOUR ON-family contrasts — S = A + B exactly, so it
# belongs in the family rather than beside it.
#
# Usage:  Rscript stats/floorx_contrasts.R /tmp/floorx.csv
#
# Arms: a1={2.0,0.4} a2={1.0,0.4} a3={2.0,0.2} a4={1.0,0.2} all levers on,
# off1={2.0,0.4} off4={1.0,0.2} levers off. Contrasts:
#   A    charge main   ((a2-a1)+(a4-a3))/2
#   B    MinGain main  ((a3-a1)+(a4-a2))/2
#   AxB  interaction   (a4-a2)-(a3-a1)
#   S    floor-drop    a4 - a1  (identity: S = A + B; checked, not assumed)
#   OFF  floor-drop at shipped config: off4 - off1, its own threshold.
#
# The mediator tables at the foot are the pre-registered liveness floors:
# floor_flips split at GW28 (the canary discriminator), the moves-differ
# census, and free_at_decision.

args <- commandArgs(trailingOnly = TRUE)
if (length(args) < 1) stop("usage: floorx_contrasts.R <cells.csv>")
source("stats/cells_common.R")

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
need <- c("{2.0,0.4} levers on (baseline)", "{1.0,0.4} levers on",
          "{2.0,0.2} levers on", "{1.0,0.2} levers on",
          "{2.0,0.4} levers off (shipped)", "{1.0,0.2} levers off")
stopifnot(all(need %in% cells$variant))

w <- reshape(cells[, c("variant", "season", "start_gw", "policy_per_gw",
                       "policy_xpoints_per_gw", "moves", "hits",
                       "floor_flips_le28", "floor_flips_gt28",
                       "free_at_decision", "banked_weeks")],
             idvar = c("season", "start_gw"), timevar = "variant",
             direction = "wide")
names(w) <- sub("^policy_per_gw\\.", "", names(w))

a1 <- w[["{2.0,0.4} levers on (baseline)"]]
a2 <- w[["{1.0,0.4} levers on"]]
a3 <- w[["{2.0,0.2} levers on"]]
a4 <- w[["{1.0,0.2} levers on"]]
off1 <- w[["{2.0,0.4} levers off (shipped)"]]
off4 <- w[["{1.0,0.2} levers off"]]

contr <- data.frame(
  season   = w$season,
  start_gw = w$start_gw,
  A   = ((a2 - a1) + (a4 - a3)) / 2,
  B   = ((a3 - a1) + (a4 - a2)) / 2,
  AxB = (a4 - a2) - (a3 - a1),
  S   = a4 - a1,
  OFF = off4 - off1
)
stopifnot(all(abs(contr$S - (contr$A + contr$B)) < 1e-9))

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
  AxB = data.frame(season = contr$season, start_gw = contr$start_gw, diff = contr$AxB),
  S   = data.frame(season = contr$season, start_gw = contr$start_gw, diff = contr$S)
)

ps <- sapply(rows, function(d) se_cr2(d)$p)
holm <- p.adjust(ps, method = "holm")

cat("=== registered contrasts of the gate-floor 2x2 (POLICY) ===\n")
cat(sprintf("n = %d cells per contrast; Holm over the four (S = A + B by identity)\n",
            nrow(contr)))
for (nm in c("A", "B", "AxB", "S")) fmt(nm, rows[[nm]], holm[[nm]])

cat("\n--- the OFF floor-drop simple, its own threshold, outside the family ---\n")
fmt("OFF", data.frame(season = contr$season, start_gw = contr$start_gw,
                      diff = contr$OFF), NA)

cat("\n--- S on the second POLICY-side instrument ---\n")
fmt("S xpoints",
    data.frame(season = contr$season, start_gw = contr$start_gw,
               diff = w[["policy_xpoints_per_gw.{1.0,0.2} levers on"]] -
                      w[["policy_xpoints_per_gw.{2.0,0.4} levers on (baseline)"]]), NA)

# Entry-point decomposition of S by the early-exposure roles (GW1/GW6/GW11
# contain gameweeks 1-10; GW16 transitional; GW21/GW26 late).
cat("\n--- S by entry point (roles: 1/6/11 early-exposure, 16 transitional, 21/26 late) ---\n")
for (g in sort(unique(contr$start_gw))) {
  d <- contr[contr$start_gw == g, ]
  cat(sprintf("  GW%-3d mean %+6.1f a season (%+0.4f pts/gw), %d cells, %d positive\n",
              g, mean(d$S) * 38, mean(d$S), nrow(d), sum(d$S > 0)))
}

# --- Mediator tables: the pre-registered liveness floors ---

flipCol <- function(name) {
  data.frame(season = w$season, start_gw = w$start_gw,
             le = w[[paste0("floor_flips_le28.", name)]],
             gt = w[[paste0("floor_flips_gt28.", name)]])
}

cat("\n--- floor_flips: seasons with at least one flip, split at GW28 (the canary) ---\n")
for (nm in need) {
  f <- flipCol(nm)
  n_le <- sum(tapply(f$le, f$season, sum) > 0)
  n_gt <- sum(tapply(f$gt, f$season, sum) > 0)
  cat(sprintf("  %-28s le28: %d of %d seasons   gt28: %d of %d seasons\n",
              nm, n_le, length(unique(f$season)), n_gt, length(unique(f$season))))
}

cat("\n--- moves-differ census: floor arms vs baseline, by entry ---\n")
for (arm in c("{1.0,0.4} levers on", "{2.0,0.2} levers on",
              "{1.0,0.2} levers on", "{1.0,0.2} levers off")) {
  m0 <- w[[paste0("moves.{2.0,0.4} levers on (baseline)")]]
  m1 <- w[[paste0("moves.", arm)]]
  if (arm == "{1.0,0.2} levers off") {
    m0 <- w[["moves.{2.0,0.4} levers off (shipped)"]]
  }
  d <- data.frame(start_gw = w$start_gw, differ = m1 != m0)
  by <- tapply(d$differ, d$start_gw, function(x) sum(x))
  cat(sprintf("  %-24s %s  (total %d/36)\n", arm,
              paste(sprintf("GW%d:%d", as.integer(names(by)), by), collapse = " "),
              sum(by)))
}

cat("\n--- free_at_decision means (the spending dose: lower in the floor-drop arm) ---\n")
for (nm in need) {
  f <- cells$free_at_decision[cells$variant == nm]
  f <- suppressWarnings(as.numeric(f))
  cat(sprintf("  %-28s mean %6.4f\n", nm, mean(f, na.rm = TRUE)))
}

cat("\n--- banked_weeks maxima per arm (banking reads both levers) ---\n")
for (nm in need) {
  bw <- suppressWarnings(as.numeric(cells$banked_weeks[cells$variant == nm]))
  bw <- bw[!is.na(bw)]
  cat(sprintf("  %-28s max %g\n", nm, if (length(bw)) max(bw) else 0))
}
