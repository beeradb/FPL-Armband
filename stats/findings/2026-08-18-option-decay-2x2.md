# The option-decay 2×2: the taper is live, sharp, and worth nothing measurable

The second half of the user's 2026-08-17 hypothesis — *"the earlier they are, the more
valuable... our weighting may also be wrong on that"* — scored for the first time. The lever
(`TaperFreeTransferValue`) is built and ships off; this measures it. The twin run
(`stats/findings/2026-08-17-prior-reactivity-under-exit-levers.md`) measured the priors under
the same levers and found the levers resolve while the prior does not.

Pre-registered in the doc comment of `TestDiagOptionDecayUnderExitLevers`
(`internal/backtest/optiondecay_diag_test.go`), committed at `48ebc71` **before the first cell
ran**. Banked cells at `stats/snapshots/2026-08-18-option-decay-2x2/cells/`, provenance commit
`48ebc71`, `dirty=false`.

## What ran

Registered 2×2: factor A the option-decay taper OFF (the shipped flat `free_transfer_value`
2.0) vs ON (the default curve: `DefaultOptionHalfLife` 8, `CongestionSensitivity` 1.0, horizon
5 — all three asserted), factor B the exit levers OFF (shipped) vs ON (the override mode the
user directed: `anchoredPlan` chips set at the analysis layer + `AnticipateChips` +
`BankLookahead` + `WeeklyXI` true — the prior 2×2's ON corner, unchanged).

Grid: extended six seasons (2020-21 … 2025-26) × entry GW1/6/11/16/21/26 = **36 cells per arm,
144 cells**, `POLICY`. 13m41s, peak RSS 120 MB, exit 0, 144 of 144 cells feasible. Contrasts
computed by `stats/taperx_contrasts.R` (the same SE machinery as `sweep_inference.R`); MDEs and
variance shares by `stats/variance_components.R`; concentration by
`stats/concentration_screen.R`.

## The three registered contrasts (POLICY, paired per cell ×38)

| contrast | a season | CR2 SE | t (df 5) | p | threshold | Holm | start-fixed t (p) | wild p (S_eff 6) |
|---|---:|---:|---:|---:|---:|---:|---:|---:|
| **ON simple: taper−flat \| levers on** | **+2.3** | 4.18 | +0.55 | 0.608 | 10.7 | 1.000 | +0.47 (0.642) | 0.593 |
| OFF simple: taper−flat \| levers off | −0.4 | 6.49 | −0.06 | 0.951 | 16.7 | 1.000 | −0.05 (0.957) | 0.946 |
| AxB: interaction | +2.7 | 6.96 | +0.39 | 0.713 | 17.9 | 1.000 | +0.35 (0.731) | 0.762 |

The factorial main (taper mean over both corners) reads **+0.9 against 10.8**, and the primary
contrast on the second POLICY-side instrument (`policy_xpoints`) reads **+1.2 against 7.0**.
MDEs at 80% power on the clustered estimator: ≈14 (ON), ≈22 (OFF), ≈24 (AxB) a season.

**Nothing resolves, and these are the sharpest POLICY thresholds this record has produced.**
The taper perturbs the charge gently week to week, so the paired arms stay highly correlated
and the SEs collapse to 4-7 a season — far below the canonical POLICY median of 70. An effect
of 15-24 a season would have been seen. The reading is therefore *measured and absent at that
size*, not *unmeasurable*.

⚠️ **The interaction direction reverses the mechanism's prediction, at point-estimate size.**
The committed pre-registration carried a sign error — "Predicted >= 0" copied from the prior
2×2's interaction line — contradicted in the same paragraph by the mechanism it states: the
congestion tax on doubles weeks means the taper should be MORE harmful under ON, i.e. AxB ≤ 0
(the correction is marked in the doc comment, dated 2026-08-18, after the run). Measured AxB
is **+2.7**, ~15× below its threshold: the point estimate says the taper is slightly HELPFUL
under the levers (+2.3) and trivially harmful at shipped config (−0.4), where the congestion
mechanism predicted the opposite sign. A point-estimate reversal, not a refutation — and not a
clearance.

## Liveness: the taper arrived on the scored path, every check pre-registered

- **ftv_flips: 6 of 6 seasons with gate flips in every role** — both the shape-clean columns
  (GW1/GW6) and the level-cut columns (GW21/GW26), in both corners. The pre-registered floor
  (4 of 6) passes everywhere.
- **moves differ from baseline in 30 of 36 cells in each corner** (4-6 per entry point) — the
  census the ladder's 3.0 rung set as the comparable expectation.
- **ftv_mean_charge shows the pre-registered schedule**: GW1 2.00/2.03 → GW26 1.21 (both
  corners; the level-cut end is slightly below the pre-registered 1.3-1.5, the
  mean-preservation residual).
- **banked_weeks is 0 in all 144 cells** — banking stays inert, exactly as the prior 2×2 found.
- **HOLD is byte-identical** (SE 0 in variance_components for every arm) — the code fact stated
  in the pre-registration, not a result.
- Concentration screen: **0 of 6 arms flagged** — no lumpiness to caveat.

## The reproduction, and it is exact

The OFF corner's flat arm is **byte-identical in 36 of 36 cells** to the ladder's banked 2.0
rung (`stats/snapshots/2026-08-17-free-transfer-value-ladder/cells/freevalue.csv`, provenance
`c4de5ce`) on `policy_per_gw`. The pre-registration worried that the scored path had moved
since the ladder's commit; it has not, for this configuration. The in-process re-run and the
banked ladder agree perfectly.

## The live-cell moderator: vacuous again

**All 36 of 36 ON cells are live** — every entry window on this grid holds doubles and blanks,
and the plan's first chip lands at entry+7 or later in every cell. The inert half has zero
cells, which the pre-registered floor rule (below 4 of 6 → insufficient data) reads as
insufficient data, exactly as the prior 2×2's 12-cell grid did. The split buys nothing on
either grid; a genuinely chip-free window would need an entry past the last double of the
season, and no archived season provides one inside this grid's entry set.

## Entry-point decomposition of the primary contrast (committed roles)

GW1/GW6 shape-clean, GW11/GW16 transitional, GW21/GW26 level-cut:

| GW1 | GW6 | GW11 | GW16 | GW21 | GW26 |
|---:|---:|---:|---:|---:|---:|
| **+32.3** | −9.2 | −5.9 | −10.7 | −1.1 | +8.3 |

**No shape.** The GW1 column (+32.3, 5 of 6 cells positive) is the largest — and the record
already names GW1 the noisiest column on any grid (six season values across a 340-point spread
in the prior run). Nothing here supports reading it as a season-opening effect, and nothing
resolves per column either.

## A byproduct, labelled as one: the levers effect at 36 cells

Not a registered contrast of this run, reported because it corroborates the prior 2×2's
resolving B: flat-ON minus flat-OFF reads **+97.4 a season** (mean_per_gw +2.5628, CR2 SE
0.5429, threshold 53.0), against the prior 2×2's +73.0 (threshold 36.2) at 12 cells. Same
configuration, wider grid, same sign, both above their own thresholds. It does not re-open the
registered limitations on B (compound corner, full sight, wildcard absent, banking inert) —
those stand as recorded.

## What this settles, and what it does not

**Settles**: the taper at its default curve is a restraint that works mechanically and pays
nothing measurable — +2.3 (ON) / −0.4 (OFF) / +0.9 (factorial) a season against thresholds of
10.7-16.7, with the comparison sharp enough that an effect of ~15-24 a season would have been
seen. The flat `free_transfer_value` 2.0 stands; the taper stays off. The user's hypothesis
that the weighting is wrong is measured as: **the weighting is worth roughly zero at the
default curve**, under both configurations. The churn-noise reading (the taper filters argmax
noise early) gets point-estimate support in the ON corner only, at +2.3 against 10.7 — support
at the level of a sign, not a resolution.

**Does not settle**: the decay and congestion channels separately (one lever, two channels —
no arm separates them); the family's shape (a single half-life was run — the record's shape
rule refuses to generalise it, and the canary argument in the pre-registration shows why no
half-life moves far outside the ladder's measured envelope except in the sub-1.0 tail); the
sub-1.0 tail itself (the ladder's owed 0.0 rung, singles-inert by the kink, funded-pair-only);
and whether a different curve — state-dependent rather than calendar-dependent, per the
forced-move reading — would differ. The MinGain kink asymmetry carries forward: the taper can
only raise the singles bar early; its late cheapening is inert at the singles gate by
arithmetic, not by measurement.
