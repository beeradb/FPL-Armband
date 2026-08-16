# The four-arm gate oracle, re-run on the scaled xPoints instrument

**Run** `TestDiagGateOracleOnXPoints`, 4 arms x 36 cells = 144 cells, one `run_id`
(`1786814218-1409335`), **commit `82fc8e0`, `dirty false`** — reproducible from a commit, which
its predecessor `2026-08-15-gateresidual` is not (`a359de4`, `dirty true`, against a diff that
adds `perfectGateResidual`).

**Why** `internal/analysis/xpoints.go` began pricing xG and xA through a per-(season, position)
`ConversionScale` at `7cbe87f`, merged `82fc8e0`. Three of the four arms read the instrument —
`transfergatexp` and `transfergateres` through their criteria, `policy_xpoints` through the metric
itself — so every figure downstream of it was superseded. This is the re-run that was owed.

The pre-registration was written before the numbers were read and is reproduced in the review
record at `reviews/2026-08-15-gate-rerun/review.md`. The plan was reviewed **before the run**, per
the standing rule, and that review changed the plan in two load-bearing ways recorded below.

## What this file is, and what it is not

**This is this run's own write-up, verdict included**, in the same form as the twelve sibling
`FINDINGS.md` files. It is in the repository for two reasons: `CLAUDE.md` quotes these figures and a
repo claim may not rest on evidence unreachable from a checkout; and a pre-registration must never
be separated from its own findings.

**What it deliberately does not carry is the CROSS-RUN record** — how this changes a closed line,
and the general rules it establishes — which lives in the research record, not in this repository.
This run's numbers, provenance, checks and verdict here; what it means for everything else, there.

⚠️ **An earlier version of this section said "it is NOT the research record" and then carried the
verdict and the reasoning seventeen lines later**, while a near-verbatim copy of the same paragraphs
sat in the other store. The split is **this-run versus across-runs**, not numbers-versus-verdict —
that first cut was the wrong axis and produced the drift it was trying to avoid.

## The table

Thresholds are each arm's OWN row on this run: `t_crit(df) x SE_CR2 x 38`, df 5.0 throughout.
**Do not reuse 46.1 / 54.8 / 59.0** — they belong to the superseded run and two of the three arms
moved. ⚠️ **"df 5.0 throughout" is this table only.** The four-season rows added below — the
`sig_season/perfect` bars and the four-season reading of the underlying arm — are **df 3**, and
`t_crit` is 3.182 there rather than 2.571; a threshold from this table is not comparable with one
from those rows.

| arm | realised POLICY | threshold | `policy_xpoints` | threshold |
|---|---:|---:|---:|---:|
| perfect on POINTS | **+3.4821** (+132.3) | 54.7 | +2.204 (+83.8) | 39.0 |
| perfect on UNDERLYING | **+2.2294** (+84.7) | 57.7 | +1.942 (+73.8) | 33.9 |
| perfect on the RESIDUAL | **+2.2546** (+85.7) | 47.5 | **−0.828** (−31.5) | 39.7 |

`S_eff` (movable seasons) is **6 of 6** on every arm and contrast, so the wild-bootstrap floor is
`6/6^6` = 0.000129 and nothing quoted here is floor-bound.

## The finding survives on the scaled instrument — `suggestive`

**The informative statistic is the residual arm's own level, and it does not resolve.** On
`policy_xpoints` it is **−0.828 pts/gw (−31.5 a season), CR2 t −2.04, p 0.0971, wild 0.0598**,
against its own threshold of 39.7, negative in 5 of 6 season means. What makes it informative
despite not resolving is its **sign**: the pre-registered bonus confound expected this figure
*positive* under both hypotheses, and it came out negative — the far side of its own confound.

⚠️ **RESIDUAL minus UNDERLYING is −2.770 pts/gw (−105.3 a season), CR2 t −11.38, wild p 0.0033,
positive in 1 of 36 — and it discriminates NOTHING.** It is exactly `level_R − level_X`; about 70%
of its magnitude is the X leg, which is the underlying arm's **positive control** on this very
metric; and the pre-registration declares its null false in advance — *"expected to be positive and
materially smaller… read it as a ratio against the xPoints arm, never against zero"*
(`gatexpoints_diag_test.go:51-55`). A t against a null nobody held is the positive-control defect
one level up. **A first version of this document led with it, while correcting the same error one
level below.** Quote it as the instrument behaving as designed, never as the finding.

On realised points the two arms stay indistinguishable: **+0.025 pts/gw, CR2 t 0.15, wild p 0.8888,
positive in 18 of 36**, against that contrast's own threshold of **16.0 a season** — which is what
makes it an *informative* tie rather than an underpowered one.

⚠️ **Neither the −2.496 → −2.770 movement nor +0.136 → +0.025 is a strengthening.** Those are the
superseded instrument's numbers on `policy_xpoints`, the metric this document says two sections
below is not comparable across runs — and the arms are not the same arms, having moved decisions in
20 and 27 of their 36 cells. Read the two runs as two readings.

⚠️ **`start-fixed −2.79` rejects (df 25) and is NOT licensed here**: the season component is
non-zero (`v_season` 0.463, 12.8%, `agree` 5/6) and `sweep_inference.R` says the fixed estimator is
not licensed when the season F test finds something. Quote the range; prefer the clustered end.

## Two corrections the plan review made before the run, neither visible in a diff

**1. The confinement check confirms nothing.** Confinement is a *code* fact — `acceptTransfer` ->
`perfectGate` -> `pointsOver`, and nothing in `weekScoreWithChip` branches on any xPoints
quantity — so re-running the points arm can only fail, and its candidate causes are data state,
stale cache, intervening commits and the dirty banked tree, none of them the change under test. It
is a **provenance** check. It passed: `+3.4821` both runs, delta exactly `0.0000`.

**The check with power is the mirror, and it was missing from the plan.** Per cell, on all 144:

| quantity | requirement | result |
|---|---|---|
| `hold_xpoints` | **MUST MOVE** | moved 144/144 |
| `hold_points` | must not move | 0/144 |
| `squad_hash` | must not move | 0/144 |
| `policy_points` (baseline) | must not move | 0/36 |

Without the first, a re-run that changed nothing looks identical to one that worked.

**2. `policy_xpoints` is a POSITIVE CONTROL for the UNDERLYING arm**, exactly as realised points is
for the RESIDUAL arm — an oracle accepting on the sign of the quantity it is scored on raises that
quantity by construction, and the symmetry runs both ways. So the prior write-up's *"corroborated
by being the only arm that improves both metrics"* is **half mechanical and is withdrawn**. What is
non-constructive is exactly two things: the underlying arm raising **realised points** (its
criterion sees only X; it is scored on X+R), and the residual arm **failing** to raise xPoints.

⚠️ **An alternative for the second is unexcluded and cheap to settle.** Selecting in-players on
high realised residual and out-players on low residual depresses accumulated xPoints whenever X and
R are negatively correlated across the candidate population over the window — plausible, since the
high-xG under-converter carries large X and negative R. Under that mechanism the negative figure is
partly selection arithmetic rather than "conversion carries no ordering information". **Measurable
off the archive with no sweep. Unrun.** The reading is therefore `established` on the contrast and
`suggestive` on the mechanism.

## The fraction is 0.64 with an upper limit of 0.813 — an information statement, not a bar

⚠️ **Retitled and narrowed in place, 2026-08-15.** This section was headed *"The fraction rejects
0.89 — and that, not the 50% straddle, is what it decides"*, which asserts the reading the body
below now withdraws: the Fieller rejections stand, but 0.89 as *a bar a constant must clear* does
not transport off the comparison it was computed on.

Recovered fraction **0.6402**, delta-method SE 0.0825, **Fieller 95% CI [0.325, 0.813]**. It
rejects 0.89 (t −3.96) and 1.00 (t −5.87) and **cannot reject 0.50** (t +1.42) — nor the six-season
bar of 0.414 (t **+2.051** against `t_crit` 2.571 at df 5). ⚠️ **That is UNRESOLVED, not
unmeasurable**: the test ran, produced a t, and failed to reject; `S_eff` is 6/6, so nothing here is
floor-bound. The fraction cannot be separated from 0.414 at this cluster count, which is a statement
about power and not about equality.

⚠️ **CHECKED 2026-08-15, and the suspension is lifted with the bar demoted: 0.89 is grid- AND
run-dependent, so it is a property of the comparison it was computed on and transfers to no other.**
The check is arithmetic off banked cells, no sweep. Three corrections to how the suspension put it:

- **Arm is probably not the confound — verified for the newer leg, inferred for the older.** Both
  legs of the bar — threshold and ceiling — are the *same* arm, `Oracles{Decision: AxisTransferGate}`,
  declared as `"perfect acceptance"` in `transfergateoracle_test.go` and `"perfect on POINTS"` in
  `gatexpoints_diag_test.go`. (**Cited by identifier and variant label, not by line**: the arm has
  already moved line twice, once inside this very change.) `transfergatexp` is the recovered
  fraction's numerator, not a leg of the bar. What differs between the two figures is the grid and
  the run. ⚠️ **The four-season leg's provenance is weaker than a sha suggests, and the arm claim
  there is inferred rather than verified**: the run stamps `0102d0d`, `dirty true`, and neither
  `AxisTransferGate` nor its test file exists in that tree — both first appear in `f9591b1`, and the
  run's own timestamp falls between the two commits (`0102d0d` 17:55:29, run 18:00:33, `f9591b1`
  18:07:17). So the older leg is not reproducible from any commit, which is the strongest single
  reason it cannot arbitrate a bar. What supports the inference is that **both predicates the arm
  runs on — `perfectGate` and `pointsOver` — are byte-identical from `f9591b1` to HEAD**, so the
  acceptance rule and the realised-points window it is judged over have not moved since; the
  adjacent commit is standing in for a tree that is gone.
- **Both channels move it.** `sig_season/perfect`, from `variance_components.R` over the committed
  cells — season-filtered for the middle row, since that script takes no season subset — and each
  arm's own `mean_per_gw`. The middle row is a legitimate four-season run at `82fc8e0` because the
  four shipped seasons are byte-identical inside the six and each (season, start) cell is replayed
  independently:

  | bank | grid | ceiling | `sig_season` | bar |
  |---|---|---:|---:|---:|
  | `2026-08-12-4d61058/cells/oraclegate.csv` (`0102d0d`, **dirty**, pre-2026-08-12 archive repairs) | 4 seasons | 105.79 | 93.76 | **0.886** |
  | `2026-08-15-gatescaled/cells/gatescaled.csv` (`82fc8e0`, clean), 2022-23..2025-26 subset | 4 seasons | 110.42 | 76.89 | **0.696** |
  | the same bank, all six | 6 seasons | 132.32 | 54.74 | **0.414** |

  ⚠️ **The two joining seasons (2020-21, 2021-22) are the two whose xG is reconstructed for a WHOLE
  season**, and the arm under test is an xG-derived criterion. ⚠️ **The four-season row is not a
  reconstruction-free control either** — 2022-23 is reconstructed for GW1-15 (`xgRepairs` in
  `internal/backtest/xgrepair.go`) and reads 2021-22 as its prior season, so **no subset of this
  archive holds the reconstruction out**, and the split is collinear with era. `stats/gate_recovered_fraction.py`
  prints all **three** backfilled seasons per-season and warns against reading the annotation as a
  contrast; an earlier draft of this paragraph said "the two whose xG is reconstructed" full stop,
  which reads as though the four-season row were clean.
  The joining pair carry the underlying arm's **largest and third-largest** season gains — 2021-22
  +158.9 and 2020-21 +101.7, with 2025-26's +134.8 second and already inside the four — and
  **removing both drops the level 27%**, 84.7 → 61.9 a season. Their share of the level is
  **51.3%**: of the sum of the six season means of the paired **realised `POLICY` per-gameweek**
  difference against `real (ships)` — all six positive, so the sum decomposes as a share — on the
  equal-weighted mean of season means, which on this balanced design is the same estimator as the
  pooled cell mean (both give 2.2294 pts/gw). The two figures are one fact, since leaving 48.7% of
  the sum over 4 seasons instead of 6 is 0.487 × 6/4 = 0.731 of the level.
  ⚠️ **Leave-one-season-out cannot answer this question and must not be offered as if it did.**
  Every LOSO subset is a *five*-season subset and so retains at least one of the two joining seasons;
  the leave-*two*-out case is the four-season row itself, which does not clear — **and could not
  have settled it if it had**, since it drops two *clusters* as well as two seasons and its threshold
  rises ×1.24 on `t_crit` alone before any SE change. The six LOSO subsets
  establish that no *single* season carries the arm, and establish nothing whatever about the pair.
  So this is a data-state caveat on the level that is **open**, not one the LOSO check discharges.

  **Grid ×0.59** holding arm, code and data state fixed, **run ×0.79** on the shared four seasons.
  Two cautions on reading those as channel magnitudes. Of the grid factor, `t_crit(df)`
  3.182 → 2.571 is ×0.81 **by construction** — `sig_season` carries `t_crit` of the season-cluster
  df, so any bar of this form moves with the grid whatever the football does — and the remaining
  ×0.73 is *which* two seasons joined as much as how many: 2021-22 (+196.7) is the largest
  perfect-gate gain of the six and 2020-21 (+155.6) the third — 2025-26 (+168.4) is second and was
  already in — so the pair's mean of 176 sits well above the four-season mean of 110, and count is
  not separable from composition at S = 4 against 6. The
  run factor is **not** a data-state effect alone — `gate.go` did not exist at the older leg and
  `oracle.go` and `season.go` both moved by hundreds of lines — and it runs almost entirely through
  a **3 df** SE estimate, which carries ~40% sampling spread. **Quote no magnitude for it.** The
  2×2's fourth corner, six seasons at the older state, is **unmeasurable**: it needs a replay of a
  tree that was dirty.
- **The decision rule cancels, which is why the bar decides nothing on its own.**
  `fraction ≥ sig_P/perfect` is identically `gain_X ≥ sig_P` **on the point estimates, and only
  where the fraction and the bar share the same `perfect`** — which the record's own use of 0.89 did
  not, having set a six-season fraction against a four-season bar — the
  underlying arm's gain against the **points** arm's threshold. (Only on the point estimates: the
  Fieller test treats a ratio with a random denominator and is not the same test.) The underlying arm
  has a threshold of its own — **+84.7 against 57.7** on six, CR2 t 3.77 — and a threshold belongs to
  its own comparison. ⚠️ **On the four-season subset that same arm reads +61.9 against 78.4 —
  unresolved there, not negative**, which is the honest companion figure: a document arguing that
  everything here is grid-dependent must not quote only the grid on which its arm resolves. Read it
  as a power loss and not a direction: the threshold rises 57.7 → 78.4 (×1.36) while the estimate
  falls 84.7 → 61.9 (×0.73), same sign at three-quarters the size, and **the six-season arm clears
  in all six leave-one-season-out subsets** (t 2.99, 3.38, 3.57, 4.09, 3.70, 3.03; estimates 69.9
  to 96.4) — which rules out any *single* season carrying it, and, per the caveat above, says
  nothing about the reconstructed-xG **pair**.

**So quote no fixed bar, and the paragraph below is narrowed rather than retracted**: the Fieller
rejections of 0.89 and 1.00 stand as facts about the fraction, while reading 0.89 as "the share a
constant would need" holds only for the four-season comparison it was derived on. ⚠️ **This is not a
reopening of anything** — an oracle is not a constant.

### What the closed line now rests on, and why that ground is weaker

The closed line *"stop sweeping the transfer gate"* was resting on this ratio and is re-grounded on
the family's swept members instead. **That ground is weaker than the ratio it replaces, and the
honest scope is correspondingly narrower — "nothing swept in this family is recorded as having
resolved", not "nothing in this family can resolve."** Four rows, of which exactly one is strong:

- **The invariance, and it is the strong row.** `min_gain` at or below 0.4 is **byte-identical**, at
  12 cells and again at 36. That is a fact about the code — the charge clause already demands
  `charge/horizon` = 0.4, so the floor cannot act — and not an estimate that failed to reject.
- **Two ties.** The floor at horizon 8 reads −15.8 against its own threshold of 34, with one season
  carrying 68%; the horizon arm reads −8.4 against 21.7. Both **fail to reject**, and their source
  bullet's own headline is *"crossing them resolved nothing"*. Under *a null is a tie* they support
  the closure only by not refuting it.
- **One ladder on the wrong grid, and it is a GAP rather than a null.** 0.7/0.95/1.30 monotone
  harmful is **24 cells on four seasons with no threshold recorded**, and its cells were never
  banked. Its bottom rung is −0.866 pts/gw = **−32.9 a season**, squarely at family scale — so that
  row's status is **unknown**, not non-resolving, which is why the scope above says *recorded as
  having resolved* rather than *has resolved*. The grid is also the one this record's own rule
  calls wrong for a transfer constant — *"sweep transfers on six too"*, where widening helped 10 of
  11 arms and **four of those ten refuting arms were `min_gain` itself**. Note also that "monotone
  harmful **above** 0.4" over-generalises: what was swept above 0.4 is those **three values**.

⚠️ **And the correction runs AGAINST the closure, which is the part that must not be dropped.**
Charging a hypothetical constant the *perfect* arm's threshold of 94 is a cross-arm substitution:
a perfect gate replaces the squad far harder than a `min_gain` nudge, and *footprint predicts a
paired SE through path divergence*, so 94 over-states what a constant's own comparison would carry.
**Gate constants are therefore MORE resolvable than the withdrawn reason claimed, not less** — the
family measures 11-34 a season and the two arms with thresholds of their own carry **34 and 21.7**.
Read that as *at the edge of this instrument*, not *out of reach of it*.

**What the rejection still carries, narrowed 2026-08-15.** The paragraph that stood here called the
0.89 rejection "the load-bearing result" and said it "closes *build a gate constant on underlying*
on this instrument". **Both readings are withdrawn**, because a bar this document has just declared
foreign to this comparison cannot be its load-bearing result. What survives is an **information**
statement, and it is the one the biases protect: the fraction is 0.64 with a Fieller upper limit of
0.813, and all three unsized biases argue it is **optimistic** — so *the underlying criterion loses
roughly a third of what the points criterion sees, and a biased-up estimate makes that floor harder
to escape rather than softer*. That is directional and bias-robust. It is not a resolvability
verdict about any constant.

### Where the withdrawn wording went, and why it is not in the resident file

⚠️ **`CLAUDE.md` carries the VERDICT ONLY from 2026-08-15**, with no retraction narrative and no
before/after accounting — a stale claim is deleted there rather than annotated. **That is why the
history is here**: this is a dated banked artefact, so in-place retraction markers are correct in
this file and wrong in that one, and a verdict-only resident file only works if the thing it stopped
carrying lands somewhere a checkout can still reach.

What the resident file carried until today, verbatim, and what is now deleted from it:

> A perfect gate is worth 106 points a season against this comparison's own significance threshold
> of 94, so a constant inside the family would have to capture 89% of perfect hindsight to resolve.
> The conclusion survives at the generous upper limit too.

and, from the interim state between the re-run and this check:

> ⚠️ **UNVERIFIED: 0.89 may be GRID-DEPENDENT, and this reading is suspended until it is checked.**

Both are gone from `CLAUDE.md`. What it keeps is the verdict — the closure at its narrowed scope,
the fraction as an information statement, the surviving Fieller rejections — plus the one fact that
settles the bar without a run (a `sig_season/perfect` ratio is built from a threshold carrying
`t_crit` of the season-cluster df, so it moves with the grid whatever the football does), and a
pointer to this file. The three banked bars with their shas, the `t_crit` 3.182 → 2.571 = ×0.81
arithmetic, the ×0.59 / ×0.79 traversal, the cancellation algebra and the LOSO t's are all above.
⚠️ **The ×0.59 / ×0.79 pair is deliberately absent from `CLAUDE.md`**: the 2×2's fourth corner needs
a dirty tree replayed, so they are path-ordered simple effects of one traversal and quoting them as
channel magnitudes would be the *simple-effect null* mistake in a decomposition's clothing.

**The 50% bar, by contrast, has failed twice and decides nothing.**

The pre-registered decision statistic is the Fieller **lower limit** against 0.50. It straddles.
⚠️ **The CI is WIDER than the one it replaces** (half-width 0.244 against 0.205), so this is not
"nearly resolved": the design is unchanged — same cells, same arms, same clustering — and nothing
here can reject 0.5 that could not before. **A bar that has failed twice to be a decision rule is
not a bar**, and re-running it a third time will not change that.

⚠️ **Three unsized biases, not two.** The bonus leak inflates it; the points arm optimising the
quantity both arms are scored on deflates it; and **the conversion scale is fitted IN SAMPLE**, so
the underlying criterion enjoys a season-global fit no deployable criterion has. **The fraction is
optimistic for anything shippable.** The leave-one-season-out alternative is open and unrun.

⚠️ **Old and new fractions are not comparable as a test of the scale** — paired differences stay
one metric but are not numerically unchanged. The only quantity comparable across the two runs is
realised policy points.

Per season the ratio runs **0.33 to 0.81** (2020-21 0.65, 2021-22 0.81, 2022-23 0.46, 2023-24 0.51,
2024-25 0.33, 2025-26 0.80). ⚠️ The backfilled seasons are the three oldest and the FPL-fed the
three newest, so that split is collinear with era and no sample size identifies the backfill from
it.

## Non-additivity: unresolved on the primary estimator in both runs

`mu_X + mu_R − mu_P` on realised points is **+1.0018 pts/gw (+38.1 a season), CR2 season t 1.77
(p 0.1365), positive in 24 of 36**, against this contrast's own threshold of 55.2. On the same
estimator the superseded run read **t 1.92**. The mean's movement (+43.6 → +38.1, −5.5 a season) is
noise against that threshold.

⚠️ **A first version of this said "now unresolved on BOTH estimators, where +43.6 rejected on
start-fixed". That transition never happened — it was an ESTIMATOR SWAP**, the third instance in
this record. `gate_additivity.R` prints `se_cr2_start` (CR2 clustered on the ENTRY POINT, which
`cells_common.R` calls a robustness check rather than a rival); the record's "start-fixed" is
`se_fixed` in `sweep_inference.R`, a fixed-effects estimator on different df. Two estimators, one
name. The script now labels it `CR2 entrypt` and says outright it is not the record's start-fixed.
`se_fixed` is not computed there because it needs `season_share`, which that script keeps local on
purpose and which must not become a third copy — **so no like-for-like fixed-effects comparison
across the two runs exists, and none should be quoted until one is computed.**

The script also prints the same contrast on `policy_xpoints`: **−1.0897 (−41.4 a season), CR2
t −2.64, p 0.0460**, which rejects. Largely mechanical — X is a positive control on that metric and
R is its complement — but recorded here because a figure the committed script prints and the
write-up omits is selective the moment anyone else runs it.

Produced by `stats/gate_additivity.R`, which is **committed**. The +43.6 came from an adaptation
that never was, which is why it had no reproducible source.

⚠️ **Not a share.** No "N% of the points arm is luck", no `G_R / G_P`, no Fieller on the residual
arm. A gate is a threshold on a sum rather than a sum; the arms hold different squads from week
one; each component gate charges four points for a hit the composite charges once.

## The degeneracy screen was re-run and still comes back empty

`TestDiagResidualXGCoverage`: **zero** rows with a goal and no xG in any of the six seasons, so the
goal channel never collapses to "did he score". ⚠️ **Its residual-MASS columns read the instrument**
through `residualOf` -> `xPointsOf` and were missing from the supersession list; re-measured,
`%degen` is **0.90% to 2.06%** against the recorded 0.92-2.08%. Row counts are unaffected, as they
must be. The pooled figure stays readable.

## The fitted scale is banked here

`conversion_scales.txt`, from `TestDiagConversionScales`. It is neither a config constant nor
reconstructible from the tree: it is fitted, in sample, season-global, and moves with the data
state — `FPL_NO_XG_REPAIR=1` changes every row of it. It was the one part of this run that would
have been unrecoverable later.
