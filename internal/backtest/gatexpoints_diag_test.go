package backtest

// The one arm that decides whether scoring decisions on underlying is worth
// building.
//
// # The question
//
// Is expected-points-from-realised-underlying a sufficient statistic for whether a
// transfer decision was good?
//
// SIX arms, all SCORED on realised POLICY points. Only what the gate KNOWS
// changes:
//
//	shipped          the real gate
//	transfergate     a gate with perfect hindsight on realised POINTS      ~106/season
//	transfergatexp   a gate with perfect hindsight on realised UNDERLYING  the 2026-08-14 arm
//	transfergateres  a gate with perfect hindsight on the RESIDUAL alone   added 2026-08-15
//	transfergateanti the same criterion NEGATED: accept iff −ΔR − 4h > 0   added 2026-08-16
//	transfergateall  no gate at all: accept every proposal                 added 2026-08-16
//
// ⚠️ **The last line is not an oracle**, and it is the one arm here that could be
// run live. It reads no hindsight; it is the NO-GATE policy, and this project has
// never measured one. It is in this family because it replaces the same predicate
// through the same hook — and because of what it identifies, below.
//
// # The fifth and sixth arms, 2026-08-16: the anti-residual pair
//
// The residual arm's `policy_xpoints` level came out NEGATIVE, on the far side of
// its own pre-registered confound, and the open question it left is whether that
// sign is information or arithmetic. `AGENTS.md` states the question exactly: the
// sign is informative only if an X-uninformative criterion would read zero, and
// nobody has measured that.
//
// The **anti-residual** arm is the test. Two criteria that are exact negations of
// each other pay the same veto cost against the baseline, so their contrast cancels
// it and leaves the antisymmetric part — the only part that could be information
// about ΔX. It needs no rate calibration, which is why it replaces the
// cross-sectional `corr(X, R)` check that was refuted for measuring the
// instrument's attenuation slope rather than football.
//
// ⚠️ **The contrast's null is NOT zero, and this is the defect a plan review caught
// before a line was written.** The accept sets `{ΔR > 4h}` and `{ΔR < −4h}` are
// disjoint with a dead band of width 8h, and h is zero for the large majority of
// packages — so over most of the stream they *partition* it. Per offered package,
// with μ the mean underlying gain and p the residual arm's accept mass:
//
//	ANTI − RES = −cov(ΔX, sign ΔR) + μ·(1 − 2p)
//
// The design wants the first term. The second vanishes only at p = ½ or μ = 0, and
// on this record's own priors both are unlikely: the search proposes what it rates
// highest, so μ > 0 is expected, and realised-minus-expected differences are
// right-skewed, so p < ½ is expected. **A large antisymmetric term is therefore
// manufacturable under the mechanism's own hypothesis**, with `cov` exactly zero —
// which is a configuration where both pre-registered readings fit the data.
//
// Three things follow, and only the first is obvious. Reporting `moves` does not
// repair it, because nothing said what to do when the counts differ. The
// concentration screen cannot catch it, because a level bias shows as a *high* `pos`
// and reads as reassurance. And the wild bootstrap cannot either, being a function
// of the season totals alone.
//
// **The accept-everything arm is the fix**, and it identifies both nuisances in the
// run's own units with no algebra: its level is `C + T` while `ANTI + RES ≈ 2C + T`,
// so `C = (ANTI + RES) − ACCEPTALL`, `T = ACCEPTALL − C`, and
// `p̂ = moves(RES) / moves(ACCEPTALL)`. The contrast is tested against `T̂·(1 − 2p̂)`.
// The per-package log measures the same two terms directly, per package, before the
// sweep's means are read at all.
//
// ⚠️ **On realised points the anti arm is a NEGATIVE control by construction** — the
// exact mirror of the residual arm's positive one — and `ANTI − RES` there is
// *doubly* constructed. Liveness only, never evidence.
//
// ⚠️ **`ANTI` is ANTI-informative, not X-uninformative.** `−ΔR` is exactly as
// informative about ΔX as `ΔR` is, so this arm does not on its own discharge the
// open question. Only the pair does, and only with the accept-everything reference.
// Say so, or this run will be read as having closed a line it bounded.
//
// ⚠️ **Expect the contrast's SE at or ABOVE the banked between-arm figures.** A
// paired SE is predicted by path DIVERGENCE rather than by activity, and these two
// arms have *disjoint* accept sets — the most divergence of any pair in the bank.
// Take the threshold from the run's own row; an SE on 5 df carries ~32% sampling
// spread, so do not compare it to another run's.
//
// ⚠️ **Adding arms takes the Holm family from 3 to 5 on every metric**, so the
// banked `p_holm` values are not comparable to this run's. Say which family a quoted
// adjusted p belongs to.
//
// # The fourth arm is a POSITIVE CONTROL, and the primary reading is 4 x 2
//
// It was very nearly added with the opposite reading attached, so the correction
// is stated before the arm is. `analysis.XPoints` is *defined* as Points minus
// XPointsResidual, so the criteria are an exact additive decomposition:
//
//	criterion_P = X + R − 4h
//	criterion_X = X     − 4h
//	criterion_R =     R − 4h
//
// Every arm is scored on realised points, and realised points *are* X + R. An
// oracle that accepts on the sign of an additive component of the metric it is
// scored on raises that metric **by construction** — E[X+R | R>0] > E[X] with no
// decision quality involved. So a gain on the residual arm is the expected outcome
// under the luck-harvesting hypothesis AND under its negation. It cannot
// discriminate, and it must not be reported as evidence of luck-harvesting.
//
// What discriminates is the SECOND metric, which is banked at no extra cost:
//
//	arm                     realised POLICY points        policy_xpoints
//	perfect on POINTS       +132 recorded                 —
//	perfect on UNDERLYING   +85.3 recorded                —
//	perfect on the RESIDUAL large BY CONSTRUCTION         this is the finding
//
// A residual gate that improved the squad's *underlying* did something real. One
// that moves realised points and leaves `policy_xpoints` flat harvested the
// unforecastable component and nothing else.
//
// **Realised POLICY points is PRIMARY and `policy_xpoints` is the secondary
// discriminator.** Declared here, before the run, because reading two metrics
// without saying which is which doubles the tests with no adjustment.
//
// ⚠️ **Pre-registered confound.** xPoints retains realised *bonus*, and bonus is
// awarded largely for goals and assists — the channels the residual replaces. So
// the residual arm's `policy_xpoints` gain is expected to be positive and
// materially smaller than the xPoints arm's. Read it as a **ratio against the
// xPoints arm**, never against zero.
//
// ⚠️ The GAINS do not decompose even though the criteria do, so
// `G_R / G_P` is not "the share of +132 that is luck" and must not be quoted as
// one: the arms hold different squads from week one, and each component gate
// charges a whole four points for a hit the composite charges once.
//
// ⚠️ **No Fieller recovered fraction for this arm.** The ratio is mechanically
// computable and semantically wrong — the residual arm is a *component*, not a
// criterion approximating another — and a reader will add the two fractions and
// get nonsense. Fieller stays where it is, on the xPoints arm alone. What this arm
// reports instead is three linear contrasts with CR2 standard errors: μ_R = 0 on
// realised points (expect rejection, positive control), μ_R = 0 on
// `policy_xpoints` (the finding), and μ_X + μ_R − μ_P = 0, which asks whether the
// gains are additive at all and is interesting either way.
//
// ⚠️ **Do not borrow the threshold of 46.** That is the xPoints arm's own row
// (CR2 SE 0.471 x 2.571 x 38); the points arm's is 54.7. Take this arm's from its
// own row and budget 55-80 — it accepts on a near-coin-flip criterion and diverges
// paths at least as hard as either sibling. Quote the clustered and start-fixed
// estimators both.
//
// ⚠️ **The xG coverage table gates the whole reading.** Where a row carries no xG,
// XPointsResidual degenerates to Goals x goalPoints — "did he score", the
// strongest hindsight available — which would make this arm a near-copy of the
// points arm on exactly the backfilled seasons, and look precisely like the
// artefact it is meant to rule out. TestDiagResidualXGCoverage prints the count;
// if it is non-trivial the pooled figure is not readable and only the FPL-fed
// seasons are.
//
// ⚠️ **The Holm family is now 3, not 2, and the recorded adjusted p moves.** On the
// same raw p's the step-down puts the xPoints arm at **0.0101**, not the **0.0050**
// recorded on 2026-08-14 — measured, not predicted: `stats/sweep_inference.R`
// prints 0.0047 / 0.0101 / 0.0109 for points / underlying / residual. There is a
// real argument for declaring the residual arm outside the family, since a positive
// control is not a test of the research hypothesis and is conventionally excluded,
// and at family 2 the recorded 0.0050 is reproduced exactly. But the R script
// computes the family from the arms present and cannot know that, so **0.0101 is
// what the printed table says** and it is the figure a reader meets first. Both are
// stated rather than one chosen; this file has been caught carrying a stale figure
// once already.
//
// ⚠️ **The 0.0050 is NOT in AGENTS.md** — checked rather than assumed, after a first
// draft of this paragraph asserted it was. It lives here and in two dated files
// under `reviews/`, which are a record of what was concluded when and are corrected
// in place by later entries rather than rewritten.
//
// ⚠️ **The three figures above — `+85.3` in the arm table, the threshold of 46 with
// its `CR2 SE 0.471`, and the Holm triple `0.0047 / 0.0101 / 0.0109` — are PRE-SCALE
// and superseded by the re-run block below. On the scaled bank they are `+84.7`,
// `57.7` with `SE 0.591`, and `0.0047 / 0.0130 / 0.0113`.** The ARGUMENTS they carry
// are unaffected, including the family-3-versus-family-2 dispute. Kept as written.
// The Holm triple was missing from the supersession list and was found by audit.
//
// # Pre-registered, before the run
//
// Recorded gate oracle on points: **+2.784 pts/gw, CR2 SE 0.775, df 3, t 3.59,
// p 0.037, ~106 a season** against this comparison's own threshold of 94.
//
// **The xPoints arm must recover at least 50% of the points arm's gain over the
// shipped baseline** for underlying to be usable as a per-decision criterion.
//
// The 50% is argued rather than picked. The whole gate-constant family is worth
// 11-34 a season against a threshold of 94, so a criterion capturing less than half
// of perfect hindsight cannot separate anything in that family even in principle —
// it would need to capture ~89% to resolve a constant, and below half it is not
// close enough to be worth a build. Above half it is worth continuing, without
// being sufficient on its own.
//
// ⚠️ **The pre-registration stands as written; the ~89% inside it is demoted,
// 2026-08-15.** It is `sig_season/perfect` on the four-season bank at
// `stats/snapshots/2026-08-12-4d61058/cells/oraclegate.csv` (commit `0102d0d`,
// **dirty**; the directory is dated by when the cells were banked, not when they
// ran), and re-derives at 0.696 on those same four seasons out of the later bank
// (`82fc8e0`, clean) — a different run, not a data-state effect; the channels are
// not separated — and at 0.414 on six. Do not carry it to any other comparison.
// ⚠️ **And the `94` it is divided by is the PERFECT arm's own threshold, charged to a
// hypothetical constant** — the cross-arm substitution `gate.go`'s block now names,
// and the reason the demotion runs AGAINST the closure rather than for it: a
// constant's own threshold is lower, so gate constants are more resolvable than this
// paragraph's arithmetic implies. The pre-registration's words are kept as written
// because they are what was pre-registered; this marker is where the correction goes.
// This changes nothing about what was pre-registered here, which is the 50% bar.
//
// ⚠️ **EVERYTHING BELOW THAT READS THE INSTRUMENT IS SUPERSEDED, 2026-08-15
// (`xpoints-position-scale`), and NO RE-RUN HAS BEEN DONE.**
// — ✅ **DISCHARGED 2026-08-15: the re-run landed and its block begins below. This
// marker is kept as written because the figures it names are still the ones being
// superseded.**
//
// `internal/analysis/xpoints.go` now prices xG and xA through a per-season,
// per-position ConversionScale. Three of the four arms read it — `transfergatexp`
// and `transfergateres` through their criteria, and `policy_xpoints` through the
// metric itself — so the superseded set is wider than the 2026-08-14 pair:
// **+85.4 and its 46.1, the 0.645 recovered fraction and its Fieller CI, +90.5, all
// three policy_xpoints figures (+80.3 / +72.7 / −22.1), the −94.8 RESIDUAL-minus-
// UNDERLYING contrast and the +43.6 non-additivity contrast.**
//
// **Unchanged, and therefore the confinement check: `perfect on POINTS`, +3.482 /
// +132.3 on realised points.** It calls pointsOver and never touches the instrument,
// so the re-run must return it byte-identical — exactly how the 2026-08-14
// corrections were shown to be confined to the xPoints path.
//
// ⚠️ **The reading "decision quality dominates" is a HYPOTHESIS until the re-run
// lands.** It rests on the sign of policy_xpoints for the residual arm, which is the
// metric that moved. The pre-registration above stands as written — it is not edited
// after the fact — but no verdict may be quoted off it today.
// ✅ **The re-run has since landed; the verdict is quotable off the block below, not
// off this paragraph.**
//
// ✅ **THE RE-RUN LANDED, 2026-08-15 (`xpoints-scaled-gate-rerun`, off `main`
// `82fc8e0`). Everything from here to the 2026-08-15 RESULT block below is the
// scaled instrument; that block and the 2026-08-14 one beneath it are SUPERSEDED and
// are kept as written.** 36 cells, one run_id, banked at
// `stats/snapshots/2026-08-15-gatescaled/`.
//
//	arm                     realised POLICY      threshold   policy_xpoints    threshold
//	perfect on POINTS       +3.4821 (+132.3)     54.7        +2.204  (+83.8)   39.0
//	perfect on UNDERLYING   +2.2294 (+84.7)      57.7        +1.942  (+73.8)   33.9
//	perfect on the RESIDUAL +2.2546 (+85.7)      47.5        −0.828  (−31.5)   39.7
//
// Shipped data state — no repair or reproduction switch set — on the default grid,
// 6 seasons × 6 starts. Every threshold is that arm's OWN row on this run —
// `t_crit(df) × SE_CR2 × 38`, df 5.0 throughout. **Do not reuse 46.0/46.1 or 59.0**:
// they belong to the superseded run and are the two arms that moved. ⚠️ **54.8 is
// NOT a third — it is a mis-rounding of 54.7.** The points arm's `SE_CR2` is 0.560
// in both runs, so its threshold never moved; the record simply spelled one number
// two ways, and listing it beside two that did move invites the conclusion that the
// points arm moved — which would contradict the provenance check three lines down.
// `S_eff` (movable seasons) is **6 of 6** on every arm and contrast below, so the
// wild-bootstrap floor is `6/6^6` = 0.000129 and no p quoted here is floor-bound.
//
// **The pre-registered reading survives as `suggestive`, and the statistic that
// carries it is NOT the one the first write-up chose.**
//
// The residual arm's own discriminator level is **−0.828 pts/gw (−31.5 a season),
// CR2 t −2.04, p 0.0971, wild 0.0598**, against its own threshold of 39.7, negative
// in 5 of 6 season means. **It does not resolve.** What makes it the informative
// statistic anyway is its SIGN: the pre-registered bonus confound at lines 51-55
// expected this figure POSITIVE under both hypotheses, and it came out negative, so
// the result sits on the far side of its own confound.
//
// ⚠️ **RESIDUAL minus UNDERLYING is −2.770 pts/gw (−105.3 a season), CR2 t −11.38,
// wild p 0.0033, positive in 1 of 36 — and it DISCRIMINATES NOTHING.** It is exactly
// `level_R − level_X`; roughly 70% of its magnitude is the X leg, which is the
// UNDERLYING arm's positive control on this very metric; and the pre-registration
// declares its null false in advance ("expected to be positive and materially
// smaller… read it as a ratio against the xPoints arm, **never against zero**").
// A t against a null nobody held is the positive-control defect one level up, and
// the first write-up of this re-run made exactly that error while correcting the
// same error at the level below. **Quote the contrast as predicted-and-confirmed —
// the instrument behaving as designed — never as the finding.**
//
// On realised points the same two arms remain indistinguishable: **+0.025 pts/gw,
// CR2 t 0.15, wild p 0.8888, positive in 18 of 36**, against that contrast's own
// threshold of **16.0 a season** — which is what makes it an informative tie rather
// than an underpowered one.
//
// ⚠️ **`start-fixed −2.79` is quoted above for the residual level and it REJECTS
// (df 25).** It is not licensed here: the season component is non-zero (`v_season`
// 0.463, 12.8% of variance, `agree` 5/6), and `sweep_inference.R` says the fixed
// estimator "is not licensed when the season F test finds something". Quote the
// range, and prefer the clustered end.
//
// ⚠️ **Two things the prior write-up got wrong, both caught by a plan review BEFORE
// this run, and neither visible in a diff.**
//
// **1. The confinement check confirms nothing.** Confinement is a CODE fact —
// `acceptTransfer` → `perfectGate` → `pointsOver`, and nothing in
// `weekScoreWithChip` branches on any xPoints quantity — so byte-identity has no
// power to confirm it and can only fail, with at least four candidate causes among
// which the scale does not appear: a different data state, a stale season cache, the
// commits between the two runs, and the banked tree itself, whose provenance records
// `commit a359de4…, dirty true` against a diff that ADDS `perfectGateResidual`.
// It is a PROVENANCE check. It passed: +3.4821 both runs, delta 0.0000.
//
// **The check with actual power is the opposite one, and it was missing: per cell,
// `hold_xpoints` MUST MOVE and `hold_points`, `squad_hash` and the baseline's
// `policy_points` MUST NOT.** Measured on all 144 cells: `hold_xpoints` moved in
// 144 of 144, the other three in 0 of 144. Without the first, a re-run that changed
// nothing would look identical to a re-run that worked — the byte-identical-null
// trap in a new dress.
//
// **2. `policy_xpoints` is a POSITIVE CONTROL for the UNDERLYING arm**, exactly as
// realised points is for the RESIDUAL arm, and the symmetry runs both ways: an
// oracle accepting on the sign of the quantity it is scored on raises that quantity
// by construction. So **"the only arm that improves both metrics" is half mechanical
// and is NOT corroboration** — the sentence below saying so is superseded. What is
// non-constructive is exactly two things: the underlying arm raising REALISED POINTS
// (its criterion sees only X; it is scored on X+R), and the residual arm FAILING to
// raise xPoints.
//
// ⚠️ **And that second one has an alternative explanation nobody has excluded.**
// Selecting in-players on high realised residual and out-players on low residual
// depresses accumulated xPoints whenever X and R are negatively correlated across
// the candidate population over the window — plausible, since the high-xG
// under-converter carries large X and negative R. Under that mechanism the negative
// figure is partly selection arithmetic rather than "conversion carries no ordering
// information". **Measurable off the archive with no sweep, and unrun.** Until it is
// run, the reading is `established` on the contrast and `suggestive` on the
// mechanism.
//
// ⚠️ **The 50% bar is UNRESOLVED for the SECOND time, and that is now the finding
// about the bar.** The recovered fraction is **0.6402, Fieller 95% CI
// [0.325, 0.813]** — it rejects 0.89 (t −3.96) and 1.00 (t −5.87) and **cannot
// reject 0.50** (t +1.42). The pre-registered decision statistic is the Fieller LOWER
// LIMIT against 0.50, and it straddles. ⚠️ **The CI got WIDER, not narrower**
// (half-width 0.244 against 0.205), so this is not "nearly resolved": the design is
// unchanged and nothing here can reject 0.5 that could not before. A bar that has
// failed twice to be a decision rule is not a bar.
//
// ⚠️ **The fraction has THREE unsized biases, not two, and the third is new.** The
// bonus leak inflates it; the points arm optimising the scored quantity deflates it;
// and **the scale is fitted IN SAMPLE**, so the underlying criterion enjoys a
// season-global fit no deployable criterion has. **The fraction is therefore
// OPTIMISTIC for a deployable criterion**, and the LOSO alternative is open and unrun.
//
// ⚠️ **Old and new fractions are not comparable as a test of the scale** — paired
// differences stay one metric but are not numerically unchanged. The only quantity
// comparable across the two runs is realised policy points.
//
// **Non-additivity, re-measured: unresolved on the primary estimator in BOTH runs.**
// μ_X + μ_R − μ_P is **+1.0018 pts/gw (+38.1 a season), CR2 season t 1.77 (p 0.1365),
// positive in 24 of 36**, against this contrast's own threshold of 55.2. On the same
// estimator the superseded run read t 1.92. The mean's movement, +43.6 → +38.1, is
// −5.5 a season against a threshold of 55.2 and is noise.
//
// ⚠️ **A first write-up of this said "now unresolved on BOTH estimators, where +43.6
// rejected on start-fixed" — and that transition never happened. It was an ESTIMATOR
// SWAP, the third instance in this record.** `stats/gate_additivity.R` prints
// `se_cr2_start`, which is CR2 clustered on the ENTRY POINT and which
// `cells_common.R` calls "a robustness check rather than a rival estimate"; the
// record's "start-fixed" / `t fixd` is `se_fixed` in `sweep_inference.R`, a
// start-block fixed-effects estimator on different df. Two estimators under one
// name, in a file that uses the name correctly forty lines above. The script now
// prints it as `CR2 entrypt` and says outright that it is not the record's
// start-fixed; `se_fixed` is NOT computed there, because it needs `season_share`,
// which `sweep_inference.R` keeps local on purpose and which must not become a third
// copy. **So no like-for-like fixed-effects comparison across the two runs is
// recorded here, and none should be quoted until one is computed.**
//
// Produced by `stats/gate_additivity.R`, which is committed — the +43.6 came from an
// adaptation that never was. ⚠️ The script also prints the same contrast on
// `policy_xpoints` (**−1.0897, −41.4 a season, CR2 t −2.64, p 0.0460**), which
// rejects; it is largely mechanical, since X is a positive control on that metric and
// R is its complement. Reported here because a figure the committed script prints and
// the write-up omits is selective the moment anyone else runs it.
//
// **The degeneracy screen was re-run and still comes back empty**: zero rows with a
// goal and no xG in any of the six seasons. Its residual-MASS columns read the
// instrument through `residualOf` → `xPointsOf` and were missing from the supersession
// list above; re-measured, `%degen` is **0.90% to 2.06%** against the 0.92-2.08%
// recorded below. The row counts are unaffected, as they must be.
//
// **The fitted scale is now banked** by `TestDiagConversionScales`, because it is
// neither a config constant nor reconstructible from the tree — it is fitted, in
// sample, and moves with the data state. It was the one part of this run that would
// have been unrecoverable later.
//
// ⚠️ **SUPERSEDED by the re-run above (2026-08-15, `xpoints-scaled-gate-rerun`) —
// every figure in this block that reads the instrument was measured on the
// PRE-SCALE instrument. Kept as written, not edited.** The verdict it reaches
// survives; the numbers under it do not, and one of its arguments is withdrawn.
//
// ⚠️ **RESULT, 2026-08-15, the four-arm run: the positive control fires on realised
// points and REVERSES on the discriminator.** 36 cells, one run_id, banked at
// `stats/snapshots/2026-08-15-gateresidual/`. The three shared arms reproduce
// 2026-08-14 to four decimal places, which is the provenance check.
//
//	arm                     realised POLICY      threshold   policy_xpoints    threshold
//	perfect on POINTS       +3.482  (+132.3)     54.8        +2.112  (+80.3)   33.1
//	perfect on UNDERLYING   +2.246  (+85.4)      46.1        +1.913  (+72.7)   22.3
//	perfect on the RESIDUAL +2.383  (+90.5)      59.0        −0.583  (−22.1)   53.7
//
// The residual arm clears on realised points (CR2 t 3.95, start-fixed 6.98) exactly
// as the construction requires, so the positive control passes and the arm is
// wired. On `policy_xpoints` it is **negative**: −22.1 a season, CR2 t −1.06,
// start-fixed −2.67, positive in **10 of 36 cells** against 34 of 36 for the
// underlying arm. RESIDUAL minus UNDERLYING on that metric is **−2.496 pts/gw
// (−94.8 a season), CR2 t −6.96, wild-bootstrap p 0.0071, positive in 3 of 36** —
// the two arms are separated decisively on the discriminator while being
// indistinguishable on realised points (+0.136, t 0.56, p 0.58).
//
// **Reading: "decision quality dominates", the first of the three pre-registered
// outcomes.** A gate that sees only conversion buys realised points and makes the
// squad's underlying no better, and the pre-registered bonus confound predicted a
// positive-but-smaller figure there rather than a negative one, so the result is on
// the far side of its own confound. The +132 does contain a large component no
// criterion can reach — but that component is unforecastable by construction, so
// the reachable bound is the underlying arm's +85.4 and it is corroborated by being
// the only arm that improves both metrics.
//
// ⚠️ **That last clause is WITHDRAWN — see the re-run above.** `policy_xpoints` is a
// positive control for the underlying arm in the same way realised points is one for
// the residual arm, so "improves both metrics" is half mechanical and corroborates
// nothing. The reachable-bound reading survives on the other two legs; this sentence
// is not one of them.
//
// ⚠️ **Do not convert +90.5 / +132.3 into "68% of the points arm is luck".** The
// gains are not additive even though the criteria are: μ_X + μ_R − μ_P is **+43.6 a
// season** (CR2 t 1.92, unresolved; start-fixed t 2.52, which rejects), so the two
// component gates together over-shoot the composite. Each charges four points for a
// hit the composite charges once, and the arms hold different squads from week one.
//
// ⚠️ **The degeneracy screen came back empty, and that is a real result rather than
// a formality.** `TestDiagResidualXGCoverage`: **zero** rows in any of the six
// seasons carry a goal with no xG, so the goal channel never collapses to "did he
// score". About 52% of appearances carry no xG in every season alike — that is a
// player who took no shot, a true zero, and the flatness across backfilled and
// FPL-fed seasons is what rules the backfill out. The assist channel is the only
// live one, at 42-86 rows a season worth **0.92% to 2.08%** of total residual mass,
// mildly worse in the two backfilled seasons and far too small to carry the arm.
// So the pooled residual figure is readable.
//
// ⚠️ **RESULT, 2026-08-14, re-run on the corrected scorer: the level resolves.**
// Underlying is **+2.246 pts/gw (85.3 a season, t 4.76, Holm 0.0050 at family 2,
// 0.0101 at family 3)** against this
// comparison's own threshold of **46.0**, which is the finding. The recovered
// fraction is **64.5%, Fieller 95% CI [0.426, 0.835]** — it **rejects equivalence**
// (t −4.18) and rejects 0.89, and **cannot reject** the 50% bar below, so **the bar
// this file pre-registers is still not a decision rule it can apply.**
//
// ⚠️ These figures were stale here for two commits — the pre-correction run read
// +2.250 / t 4.27 / CI [0.359, 0.895], and the documents were updated while this
// comment was not. Found by a rulebook audit, and it is the desynchronised-mirror
// class: the number a reader meets first is the one in the code.
//
// ⚠️ **Read the per-season column before building on 85.3.** The recovered fraction
// runs **0.34 to 0.86** across the six seasons, and **2 of the 6 feed all four
// replaced channels from a borrowed-offset Understat backfill** (2020-21 and 2021-22
// in full, 2022-23 for GW1-15), where the record already prices the xGC chain at a
// 16-20% ever-present error against 3.0-5.2% FPL-fed. `stats/gate_recovered_fraction.py`
// prints the column; the cells are banked, so it needs no re-run.
// Kept as written because a pre-registration edited after the run is worthless, and
// because the failure is instructive: the bar was undefended, and an undefended bar
// that the data cannot resolve either way decides nothing.
//
// ⚠️ **The fraction is NOT a lower bound**, which this comment first claimed. Two
// biases run opposite ways and neither is sized: the points arm optimises the
// quantity both arms are scored on (pushes it down), and xPoints retains realised
// minutes, bonus, saves, cards and defcon (pushes it up).
//
// ⚠️ **The disagreement rate promised below was NOT delivered** — transfer counts
// were reported instead, and counts are a weak proxy since equal counts do not imply
// the same packages. They are enough to rule out "the arms are the same arm" and
// "an arm collapsed onto the baseline", and nothing finer. Emitting the real
// statistic needs a per-package log this diagnostic does not keep.
//
// ⚠️ Both oracle arms see the future. Neither is a policy anyone could run; they
// bound what a criterion is worth, nothing more.

import (
	"fmt"
	"sort"
	"testing"
)

func TestDiagGateOracleOnXPoints(t *testing.T) {
	requireDiag(t)
	starts := sweepStarts()

	fmt.Printf("\n=== does a gate perfect on UNDERLYING recover a gate perfect on POINTS?\n")
	fmt.Printf("SIX arms, all scored on realised POLICY points. Only the gate's own\n")
	fmt.Printf("criterion changes.\n")
	fmt.Printf("The 50%% recovery bar this banner used to announce as PRE-REGISTERED\n")
	fmt.Printf("is RETIRED as a decision rule: it failed to resolve twice, and the\n")
	fmt.Printf("interval got wider rather than narrower the second time. It is kept\n")
	fmt.Printf("in the file comment as what was pre-registered, and it decides\n")
	fmt.Printf("nothing here. The recovered fraction is still reported.\n")
	fmt.Printf("The RESIDUAL arm is a POSITIVE CONTROL on realised points: points =\n")
	fmt.Printf("xPoints + residual identically, so an oracle on the residual's sign\n")
	fmt.Printf("raises realised points BY CONSTRUCTION. A gain there is evidence of\n")
	fmt.Printf("nothing; a small one is a wiring fault.\n")
	fmt.Printf("The ANTI-RESIDUAL arm is the same construction NEGATED, so on\n")
	fmt.Printf("realised points it is a NEGATIVE control and liveness only. Its\n")
	fmt.Printf("purpose is the CONTRAST against the residual arm on policy_xpoints,\n")
	fmt.Printf("which cancels the veto cost the two share and leaves the\n")
	fmt.Printf("antisymmetric term.\n")
	fmt.Printf("*** That contrast's null is NOT zero. *** The two accept sets\n")
	fmt.Printf("partition the free-transfer stream, so accept-mass asymmetry enters\n")
	fmt.Printf("additively as mu*(1-2p). ACCEPTALL identifies it: C = (ANTI + RES) -\n")
	fmt.Printf("ACCEPTALL, T = ACCEPTALL - C, p = moves(RES)/moves(ACCEPTALL), and\n")
	fmt.Printf("the contrast is tested against T*(1-2p). ACCEPTALL is also the\n")
	fmt.Printf("NO-GATE policy, which nothing in this project has measured.\n")
	fmt.Printf("PRIMARY metric: realised POLICY points. SECONDARY: policy_xpoints.\n\n")

	var base, onPoints, onXP, onRes, onAnti, onAll []gateCell
	collect := func(into *[]gateCell) func(seasonPair, int, *SimResult) {
		return func(pair seasonPair, start int, res *SimResult) {
			*into = append(*into, gateCell{
				season: pair.Name, start: start,
				moves: res.Transfers, hits: res.Hits, weeks: len(res.Weeks),
			})
		}
	}
	// The per-package log, one accumulator per arm. It is what supplies p, mu and
	// cov(dX, sign dR) — the three quantities the ANTI-minus-RES contrast's null is
	// built from, none of which is recoverable from a transfer count. This
	// diagnostic has recorded owing exactly this statistic since its first re-run,
	// when counts were reported in its place.
	// One list, built where the arms are declared. A map plus a separate print-order
	// slice was the first version, and a name changed on one side would have dropped
	// a row silently from the one output that supplies the contrast's offset.
	var streams []namedStream
	logging := func(name string) func(*SimConfig) {
		st := &gateStream{}
		streams = append(streams, namedStream{name: name, stream: st})
		return func(sc *SimConfig) { sc.gateLog = st.add }
	}
	streamNamed := func(name string) *gateStream {
		for _, ns := range streams {
			if ns.name == name {
				return ns.stream
			}
		}
		t.Fatalf("no per-package stream named %q; the arm list and the reader have "+
			"drifted, which is the failure one list exists to prevent", name)
		return nil
	}

	shipped := policyVariant{label: "real (ships)", apply: logging("shipped")}
	shipped.observe = collect(&base)

	pts := oracleVariant(Oracles{Decision: AxisTransferGate},
		"perfect on POINTS", logging("on pts"))
	pts.observe = collect(&onPoints)

	xp := oracleVariant(Oracles{Decision: AxisTransferGateXPoints},
		"perfect on UNDERLYING", logging("on xP"))
	xp.observe = collect(&onXP)

	res := oracleVariant(Oracles{Decision: AxisTransferGateResidual},
		"perfect on the RESIDUAL", logging("on resid"))
	res.observe = collect(&onRes)

	// The fifth arm: the same criterion negated, and the whole point of the run.
	anti := oracleVariant(Oracles{Decision: AxisTransferGateAntiResidual},
		"ANTI-residual", logging("anti"))
	anti.observe = collect(&onAnti)

	// The sixth, and it is not optional garnish: without it the contrast between
	// the two above is tested against a null the design already has reason to
	// believe false, which is the discrimination failure this project has paid for
	// once at exactly this spot.
	all := oracleVariant(Oracles{Decision: AxisTransferGateAcceptAll},
		"NO GATE (accepts all)", logging("accept all"))
	all.observe = collect(&onAll)

	runPolicySweep(t, []policyVariant{shipped, pts, xp, res, anti, all}, starts)

	// ⚠️ The three guards reportGateMediator has, which a first version of this
	// diagnostic dropped and replaced with printed prose for a human to notice.
	//
	// The length check is not defensive padding: runPolicySweep `continue`s past an
	// infeasible cell BEFORE calling observe, so one arm can legitimately observe
	// fewer cells than another, and the positional indexing below would then compare
	// different cells to each other or panic.
	//
	// ⚠️ Both halves of the alignment have to grow with the arms, and missing
	// either is silent: an unextended length check lets an arm with fewer cells
	// through, and an unextended sort loop leaves one arm in run order while the
	// rest are keyed — so the table would compare different cells to each other and
	// still print.
	if len(base) == 0 || len(base) != len(onPoints) || len(base) != len(onXP) ||
		len(base) != len(onRes) || len(base) != len(onAnti) || len(base) != len(onAll) {
		t.Fatalf("observed %d baseline cells, %d on points, %d on underlying, %d on "+
			"the residual, %d anti-residual and %d with no gate; the mediator zips "+
			"them positionally and cannot align unequal sets",
			len(base), len(onPoints), len(onXP), len(onRes), len(onAnti), len(onAll))
	}
	key := func(c gateCell) string { return fmt.Sprintf("%s@%d", c.season, c.start) }
	for _, s := range []*[]gateCell{&base, &onPoints, &onXP, &onRes, &onAnti, &onAll} {
		sort.Slice(*s, func(i, j int) bool { return key((*s)[i]) < key((*s)[j]) })
	}

	fmt.Printf("\nMEDIATOR — transfers accepted under each criterion.\n")
	fmt.Printf("%-9s %6s %9s %9s %9s %9s %9s %9s\n",
		"season", "start", "shipped", "on pts", "on xP", "on resid", "anti", "no gate")
	var bm, pm, xm, rm, am, allm int
	// Hits alongside, because the no-gate arm's hit charge is a level bias on T and
	// the hit channel (4 x hits / weeks) is what sizes it.
	var bh, rh, ah, allh int
	for i := range base {
		fmt.Printf("%-9s %6d %9d %9d %9d %9d %9d %9d\n", base[i].season, base[i].start,
			base[i].moves, onPoints[i].moves, onXP[i].moves, onRes[i].moves,
			onAnti[i].moves, onAll[i].moves)
		bm += base[i].moves
		pm += onPoints[i].moves
		xm += onXP[i].moves
		rm += onRes[i].moves
		am += onAnti[i].moves
		allm += onAll[i].moves
		bh += base[i].hits
		rh += onRes[i].hits
		ah += onAnti[i].hits
		allh += onAll[i].hits
	}

	// Wired-and-inert is an assertion, not a sentence. An oracle that accepted
	// exactly what the shipped gate does reports a clean null indistinguishable from
	// "the gate is already perfect", and this run's own liveness must not depend on
	// somebody reading the prose below.
	if xm == bm {
		t.Error("the xPoints oracle accepted exactly as many transfers as the " +
			"shipped gate in total: wired and inert, or not wired at all")
	}
	if xm == pm {
		t.Error("the two oracles accepted identical transfer counts: they may be " +
			"the same arm, and the recovered fraction would be meaningless")
	}
	// The same two liveness assertions for the residual arm, and it needs them more
	// than either sibling: its interesting result is a LARGE gain, so the reading
	// that would be quietly wrong is a small one, and "inert" and "small" are the
	// same number in the output.
	if rm == bm {
		t.Error("the residual oracle accepted exactly as many transfers as the " +
			"shipped gate in total: wired and inert, or not wired at all")
	}
	if rm == xm || rm == pm {
		t.Error("the residual oracle's transfer count matches one of the other two " +
			"criteria exactly: it may be running the sibling's hook, which three " +
			"near-copies behind one switch makes easy and silent")
	}
	// The anti-residual arm needs the same two, and it needs one more than its
	// siblings: matching the arm it NEGATES is the specific failure a fourth
	// near-copy invites, and the sign-flip unit test cannot see a switch that routes
	// two axes to one predicate at the sweep's own configuration.
	if am == bm {
		t.Error("the anti-residual oracle accepted exactly as many transfers as the " +
			"shipped gate in total: wired and inert, or not wired at all")
	}
	if am == rm {
		t.Error("the anti-residual oracle's transfer count matches the residual " +
			"oracle's exactly: the two are the same arm, and their contrast — which " +
			"is what this run is for — would be a difference of an arm with itself")
	}
	// ⚠️ And against the other two, which a first version omitted. A code review
	// showed that on the unit fixture perfectGateXPoints answers identically to
	// perfectGateAntiResidual on both of the probes that version used, so a
	// mis-route of this axis to the UNDERLYING hook passed every check in the
	// change — and would print a table whose "anti" column is a byte-copy of
	// "on xP", making the pre-registered contrast silently a different contrast.
	if am == xm || am == pm {
		t.Error("the anti-residual oracle's transfer count matches the underlying or " +
			"the points criterion exactly: it may be running a sibling's hook, which " +
			"five near-copies behind one switch makes easy and silent")
	}
	// The no-gate arm must take MORE transfers than every gate, which is the one
	// liveness statement available for an arm with no criterion to assert against.
	// It is a bound rather than an inequality of totals: every gate is a refusal
	// applied to the same proposal stream, so a gate that accepted more than
	// accepting everything would be a harness fault rather than a small effect.
	if allm <= bm {
		t.Errorf("the no-gate arm made %d transfers against the shipped gate's %d: "+
			"an arm that refuses nothing cannot make fewer moves than one that "+
			"refuses something, so it is not wired", allm, bm)
	}
	if allm < rm || allm < am || allm < xm || allm < pm {
		t.Errorf("the no-gate arm made %d transfers and some gated arm made more "+
			"(pts %d, xP %d, resid %d, anti %d): a gate cannot accept more than "+
			"accepting everything", allm, pm, xm, rm, am)
	}
	// ⚠️ The inequalities above are bounds and a mis-route can satisfy them. This arm
	// needs the equality check MORE than any other, because it is not a reported arm
	// in its own right: `C = (ANTI + RES) − ACCEPTALL` and `p̂` are both read off it,
	// so a duplicated no-gate arm produces a pre-registered null computed from an arm
	// that is not the no-gate policy, with nothing in the output saying so.
	if allm == bm || allm == pm || allm == xm || allm == rm || allm == am {
		t.Errorf("the no-gate arm's transfer count (%d) matches another arm's exactly "+
			"(shipped %d, pts %d, xP %d, resid %d, anti %d): it has no criterion to "+
			"be checked against, so this is the only assertion that can catch it "+
			"running a sibling's hook", allm, bm, pm, xm, rm, am)
	}
	fmt.Printf("%-9s %6s %9d %9d %9d %9d %9d %9d   <- totals\n", "", "",
		bm, pm, xm, rm, am, allm)

	// p-hat, two ways, and the PACKAGE one is the one the null is built from.
	//
	// ⚠️ The mediator's ratio was quoted alone in a first version of this block and
	// it is a MOVE-weighted proxy: `p` enters the decomposition as `μ·(1 − 2p)` with
	// `p = P(ΔR > 4h)` per offered PACKAGE, while `moves` counts legs — a funded pair
	// is one package and two to five moves. Two further distortions push the same
	// way: `decide`'s singles loop returns on its first rejection, so a refusing arm
	// forfeits the rest of the week's moves for a reason unrelated to its accept
	// mass, and the no-gate arm is not offered the same stream at all. Both are
	// printed, and the package figure is the one the sentence below uses.
	movesRatio := 0.0
	if allm > 0 {
		movesRatio = float64(rm) / float64(allm)
	}
	pHat := streamNamed("on resid").packageMass()
	pShipped := streamNamed("shipped").packageMass()
	fmt.Printf("\np-hat, PACKAGE mass on the residual arm's own stream: %.4f\n", pHat)
	fmt.Printf("       the same on the shipped stream (the reference):  %.4f\n", pShipped)
	fmt.Printf("       MOVE-weighted proxy, moves(RESID)/moves(NOGATE): %d / %d = %.4f\n",
		rm, allm, movesRatio)
	fmt.Printf("The contrast ANTI - RESID on policy_xpoints is tested against\n")
	fmt.Printf("T*(1-2p) = %+.4f * T, NOT against zero, taking p as the package mass.\n",
		1-2*pHat)
	fmt.Printf("T comes from the sweep's own means: C = (ANTI + RESID) - NOGATE and\n")
	fmt.Printf("T = NOGATE - C, all three read as paired differences against shipped.\n")
	fmt.Printf("*** CORRECT T FOR THE NO-GATE ARM'S OWN HIT CHARGE BEFORE USING IT.\n")
	fmt.Printf("*** moveLimit bounds a week at free+1 moves, so an arm that refuses\n")
	fmt.Printf("nothing exhausts the week every week and takes a -4 nearly every week\n")
	fmt.Printf("— a charge no gated arm pays. The hit channel is 4*hits/weeks and the\n")
	fmt.Printf("hits column above gives it: shipped %d, resid %d, anti %d, no gate %d.\n",
		bh, rh, ah, allh)

	reportGateStreams(streams)
	fmt.Printf("\nIf `on xP` equals `on pts` in every cell the two criteria agree on\n")
	fmt.Printf("every package the model proposed, and the arms are the same arm.\n")
	fmt.Printf("If `on xP` equals `shipped`, the oracle changed nothing and the\n")
	fmt.Printf("comparison is a byte-identical null rather than a result.\n")
	fmt.Printf("The same two readings apply to `on resid`, and its counts say\n")
	fmt.Printf("nothing about its VALUE: equal counts are not the same packages.\n")
	fmt.Printf("\nThe points figures are in the sweep table above; take the recovered\n")
	fmt.Printf("fraction from the POLICY per-gameweek means, not from these counts.\n")
	fmt.Printf("Report policy_xpoints_per_gw beside policy_per_gw — the POLICY-side\n")
	// ⚠️ This printed "cuts SEs 30-60%% with the means preserved" on every run. That
	// is an xppilot figure measured on the PRE-SCALE instrument, superseded on
	// 2026-08-15 and not re-measured by this or any run — printed live, with nothing
	// in the output saying so. Exactly the failure gate_recovered_fraction.py was
	// repaired for in the same commit, one file away.
	fmt.Printf("The xPoints instrument goes BESIDE realised points, never instead of\n")
	fmt.Printf("them. Its SE-cut figures are pre-scale and superseded: take them from\n")
	fmt.Printf("a re-run, not from here.\n")
}
