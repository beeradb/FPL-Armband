# The blank-run de-levelling is not position-wide

Reviewed `76a57bf..1a6e933` on `measure-blank-run-bias-by-position`, off `origin/main` at `cf4379a`.

**What the branch does.** `analysis.blankRunFactor` de-levels its calibration against the run-0
row and justified that as removing "the model's general, and harmlessly position-wide, tendency to
over-predict minutes". The table it rests on carries no position split, so "position-wide" was
asserted at that site and never measured. This re-runs the same calibration cut by position —
inherited population filters, window, cutoffs and estimator — and the claim is false.

**Measure only.** `blankRunFactor` stays at 0.75 and the de-levelling is untouched, by explicit
instruction. No shipped constant, config default or scoring expression changed; the only edit to a
non-test file is the doc comment on `blankRunFactor`. No points figure is claimed: every quantity
here is a minutes calibration.

## Which reviewers ran

| reviewer | why | outcome |
|---|---|---|
| **fpl-code-review** | the diff touches `internal/backtest` and `internal/analysis` | 8 findings, 7 applied, 1 declined |
| **fpl-stats-review** | it produces a number and a verdict | 6 questions answered, 6 findings, all applied |
| **fpl-findings-audit** | `internal/analysis` and `stats/*.R` moved, and a recorded claim is being retracted | 12 findings, 11 applied, 1 declined |
| fpl-security-review | skipped: no `internal/fpl`, no `internal/agent`, no config persistence, no credential or cache path | — |
| fpl-run-review | skipped: no live run, nothing wrote config | — |
| fpl-season-maintenance | skipped: none of the four hand-maintained lists is touched | — |
| fpl-docs-review / fpl-docs-accuracy | skipped: no `docs/` change. `CLAUDE.md` is deliberately untouched — the task forbade editing it — and the entries the audit proposes are carried in this record instead | — |

**Invariants first, per this skill's own opening.** The quantity this change must not move is the
model, and the guard is that `blend.go`'s diff is comment-only — every changed line begins with
`//`, checked. Two new invariants were added rather than relying on a reviewer's eye, and one of
them is the reason a whole class of wrong answer is now unreachable:

- `TestExpectedMinutesCarriesTheBlankRunDiscount` pins that `ExpectedMinutes` carries
  `blankRunFactor`. That coupling is what makes this calibration circular at shipped config, and
  it is what the `FPL_NO_BLANK_RUN` guards depend on being true.
- Both `TestDiagAvailability` and `TestDiagAvailabilityByPosition` now **`t.Fatal`** without
  `FPL_NO_BLANK_RUN=1`. A skip would have left a complete-looking table measuring the residual
  after the term.

## Findings applied

**1. Season clustering is the wrong cluster here, and my first reading over-claimed.**
(fpl-stats-review, verified.) Players are *crossed* with seasons, not nested: 8,849 run-0 rows are
805 footballers, 98 of them forwards. One cohort enters all six season means, so `sd/sqrt(6)` is
too small. Added a player-code bootstrap (B 2000, seeded) and a within-season sampling floor, which
settles it arithmetically — MID−FWD's season-means SE of 0.0052 is below its own floor of 0.0140,
and three of six pairs sit below theirs. All three GKP pairs clear on both estimators' Holm bars;
DEF−FWD and MID−FWD clear neither. The first commit's claim that the outfield pairs were supported
is **withdrawn**.

**2. The headline is now the omnibus, not a pair.** (fpl-stats-review.) F(3,15) = 15.85,
p = 6.4e-05 with season blocked; Friedman p = 0.0051. It tests the actual question, chooses no
pair, and rejects on the four-season grid too (F(3,9) = 7.01, p = 0.0099) — which the pairwise
route only managed by switching which pair carried it, i.e. an argmax across grids. Added the
ordering as the most robust statement: GKP highest of four in 5 of 6 seasons, FWD lowest in 5 of 6.

**3. Three quotable factual errors in the comment.** (fpl-findings-audit, all three verified
against the banked output before applying.)

- "DEF−FWD and MID−FWD clear Holm on the season-means SE" — false, they read 0.0869 each. The
  pairs that clear are GKP−DEF and GKP−FWD, which the same comment said correctly nine lines later.
- "This table does not re-run" — too broad. `n`, `expected` and `vanished` all reproduce
  (2.0/18.3/24.9/27.2/35.2% against 2/18/25/28/35), so **CLAUDE.md's ninefold vanish-risk figure is
  corroborated rather than contaminated**. Only `actual` and `bias` moved.
- The superseded 7% survived unmarked in the two files that motivate the run.

**4. The cause of the data-state move is separable from the checkout, so the disjunction
collapsed.** (fpl-findings-audit, verified: `66e2a18` 2026-08-08 08:02 is not a descendant of
`89fa973` 17:54.) `newRecentIndexWith` divides by the weighted *fixture* count, so `expected` is
per-match and invariant to the doubles fix while `actual` and the established-player filter are
per-gameweek — predicting expected flat, actual up, n up, all three observed. The sibling guess is
**refuted**: `66e2a18`'s hard-coded pairs are today's four. Attribution stays a hypothesis, with
the 2026-08-12 duplicate-row drop named as unexcluded.

**5. Two over-claims narrowed.** "The ordering, which no clustering argument touches" — within-season
ranking removes *within-season* dependence only; Binomial(6, ¼) still assumes independent season
draws, which 62 keepers at 2.55 seasons each violate. And "an SE below its own floor is
arithmetically impossible, therefore proof" — at df 5 that is a ~2% event per pair; what carries it
is three of six.

**6. A double standard in my own verdict block.** The bootstrap column was screened at a bare
|t| ≥ 2 while the column beside it was Holm-corrected, and on four seasons that declared three
pairs "surviving" at t ≈ 2.93 against the same file's printed `t_crit` of 3.182. Both columns now
get Holm on their own reference, and the verdict prints the **intersection**. On six seasons that
is GKP−DEF and GKP−FWD; on four it is honestly **none**, with the omnibus still rejecting.

**7. The exemption citation was on the wrong sentence.** (fpl-findings-audit.) A bias constant
within each position is literally the exemption's own case, since FPL forces 2/5/5/3. The grounds
that bite are CLAUDE.md's qualifications — `Optimize` is a knapsack against one budget, and
`blankRunFactor` applies to a *subset within* each position rather than to the position, so an
error in its level is a within-position weighting error by construction — plus the unqualified
sibling rule, **a measured bias does not imply a correction exists**. Also added: `xgcrepair.go`'s
rule made the precedent uncitable for a multiplicative removal *before* anything was measured, so
the measurement is a second and independent ground; and `MinExpectedMinutes` already cliffs this
population out of the pool, so none of it reaches replayed points.

**8. Provenance was false.** The R header claimed the rule was pre-registered. The pooled table,
already showing GKP at 0.999, was read first. Relabelled **post-hoc**, with what genuinely was
fixed in advance (inherited population and estimator; Holm over six fixed before any p-value)
spelled out separately.

**9. A designed-in fallacy removed.** The header said a differing Holm count between the additive
and multiplicative tables "IS the conversion defect, visible". That is the
significant/non-significant fallacy written into the design, and the report then correctly refused
to honour it. Both are gone; the question is only whether the falsification survives additively,
and it does (GKP−FWD, Holm 0.049).

**10. Go-side hygiene.** (fpl-code-review.) `TestDiagAvailability` got the guard its new sibling
had; the exact-ratio assertion is labelled as holding only with `Priors` nil, since
`collectAvailabilityObs` sets them; the row dump opens *after* the observation-count skip so a
skipped run cannot truncate a complete file; it dumps the raw run beside the capped bucket; the
console records `minutes_half_life`/`blend_minutes_k`/`horizon`; and the new test skips when
`FPL_BLANK_RUN_PENALTY`/`MAX` make the term inert.

**11. Two inline copies of the MDE product survived the consolidation** in
`variance_components.R`'s `seasons_needed`/`starts_needed` — it moved the copy that had a name and
left the two that did not, which the shared-quantity scan cannot see. Both now call `sig_and_mde`.
Verified `seasons_needed` still renders identically on banked cells.

**12. The plateau differs by position too**, and was banked but not reported at the site: GKP
0.556, 95% CI [0.402, 0.735], excluding the shipped 0.75; GKP's and FWD's `4 or more` rows at 0.775
and 0.854 rather than above 1. No contrast in that table clears Holm and it is badly powered (111
GKP rows), so it is recorded as **not a second falsification** — but "the shape survives" must not
be read as "position does not reach the shape".

## Declined

**Consolidating `season_t` into `cells_common.R`.** (fpl-code-review, finding 4.) It is
numerically identical to `cr2_t_fast` at one row per cluster — verified to twelve decimals — so it
is a fourth spelling of the season-clustered t. Declined because every shared implementation takes
a *cells* contract this input does not have: `se_cr` needs a `diff` column and `clubSandwich`,
`cr2_t_fast` needs a matrix of draws, and both key on the cell, while this input is a
per-observation frame whose cluster statistic is a **ratio**. Recorded as sanctioned with that
argument in the source rather than left silent. Revisit if a second ratio-clustering caller appears
— then it is a shared quantity and the argument expires.

**Whether the omnibus should still lead**, given it inherits the same cohort blind spot and has no
player-clustered counterpart. The caveat is applied in both the comment and the printed output.
Adjudicating the choice of headline is a statistics call, not a claims call, and the intersection
line means nothing rests on the omnibus alone.

**Editing `CLAUDE.md`.** Forbidden by the task. The audit proposed four entries — a Standing rules
entry on crossed players and the sampling floor, an amendment to the doubles-fix contamination
bullet, an amendment to the existing minutes entry, and no standalone entry for the 3-4% figure.
They are carried here and in the commit messages for whoever holds the pen. The crossed-players
entry needs its scoping sentence: it does **not** impugn any banked cell-level CR2 figure, where
the unit is a cell and the fifteen are not the sampling unit.

## Could not be checked on this harness

- **Whether the ordering statistic survives player resampling.** Binomial(6, ¼) assumes independent
  seasons and the cohort recurs. The rows are banked and no replay is needed, so this is
  **unmeasured, not unmeasurable**.
- **What the constant should be.** Deliberately unanswered. Beyond the instruction, the replay
  cannot resolve this term at all: `MinExpectedMinutes` is a cliff at 55, not a discount, and it
  removes most of the affected population from the optimiser's pool. A `HOLD` arm is expected to
  return a tie rather than a verdict — which is a fact about the instrument, not about the constant.
- **Whether the doubles fix is the *whole* +2.0 min/gw.** The ordering and the mechanism are
  established; the magnitude attribution is not, and the duplicate-row drop is an unexcluded
  co-mover.
- **The `bootHolm` reference.** A normal reference over 600-plus player clusters, not a bootstrap-t
  percentile. Adequate at this cluster count and stated rather than assumed.

One standing caution, quoted because it applies to this branch's own output: a null here is a tie,
not a refutation. DEF−MID reads 0.0034 against its own MDE of 0.0182 — it was never measurable, and
that is silence rather than evidence that defenders and midfielders agree.
