# The clean sheet: the recorded 30% over-prediction is the calibration's regressor, not the model

**Branch** `clean-sheet-calibration-refit`.
⚠️ **Provenance: this run is `commit 6021401` with `dirty = true`** (`cells/csxgc.provenance.csv`),
so the code state that produced it **cannot be named by a sha**. What makes it readable at all: the
baseline arm reproduces `stats/snapshots/2026-08-15-gatescaled/` **bit-exactly on 36 of 36 cells**,
on both `hold_points` and `squad_hash`. So the dirty tree was scoring-identical at
`f = 1.0, flat = 1.0`. **Quote that, never a HEAD the run did not use** — an earlier version of this
file named `182141f` and was wrong.

**Data state**: shipped, no `FPL_NO_*` switch. Six seasons × six entry points, 36 cells per arm.
**Metric**: `HOLD`.

---

## What is ESTABLISHED, and it is a LEVEL claim rather than a slope claim

**The recorded ~30% clean-sheet over-prediction is a property of the regressor the CALIBRATION
chose, not of the model.** `stats/cs_calibration.R` fits `-ln p` against **realised single-match
xGC**. `cleanSheetProb` scores **`XGC90`** — a per-player per-90 rate, blended toward a prior season
and shrunk. Different variances, convex exponent, so the aggregate over-prediction differs between
them by construction.

Refit against `XGC90` (`internal/backtest/csregressor_test.go`): one row per team-gameweek, the
model's `XGC90` from an engine built at the **previous** gameweek against the clean sheet actually
realised in **this** one; `Fixtures == 1`, representative played 90, `XGC90 > 0`, cutoff ≥ 6.

| stratum | n | predicted | actual | ratio | season-clustered SE of (pred−act) | t vs 1 | t vs the recorded 1.281 |
|---|---|---|---|---|---|---|---|
| native (2023-24…2025-26) | 1566 | 0.2486 | 0.2363 | **1.052** | 0.0081, df 2 | +1.51 | **−6.7** |
| pooled (six seasons) | 2974 | 0.2650 | 0.2640 | **1.004** | 0.0077, df 5 | +0.13 | **−9.5** |

⚠️ **Read that table's columns carefully; an earlier version mislabelled one.** The estimate column
is a **ratio**; the SE column is the SE of the **difference** `pred − act` across season clusters;
the t columns are on the **ratio** scale, via a delta-method SE of 0.0343 native and 0.0291 pooled.
Taken at face value "1.052 ± 0.0081" would give t = 130, which is not what the row says. The null is
**1**, not 0.
⚠️ **And two clustering axes appear in this document.** This table clusters on **season** (df 2 and
5, `t_crit` 4.303 and 2.571). The slope figures below cluster on **season × team** (60 clusters,
`t_crit` 2.00) — so the **`MDE 0.424` inherits the 60-cluster SE and is not on this record's primary
axis.** On season clustering it would be larger, which makes "cannot separate" *more* true and the
quoted number wrong for the axis a reader will assume.

The recorded figure is 30.5% predicted against 23.8% actual — **ratio 1.281 — and both strata reject
it by a wide margin.** In the units of the code comment at `internal/analysis/sweep.go`, the recorded
bias is *"+0.27 points a match to every defender and keeper"*; on the scored regressor the same
arithmetic gives **+0.049 native, +0.004 pooled**. That comment needs correcting.

**Mechanism — ⚠️ TWO are present and neither replaces the other. This paragraph has now been wrong
twice, in opposite directions, and both errors are kept.**

**Draft 1** said cross-match Jensen alone: `E[exp(−x)] ≈ E[exp(−λ)]·exp(σ²/2)`, `σ ≈ 0.70`, "the
magnitudes line up". **Draft 2 withdrew that entirely and named the shot-level gap as its
replacement. Draft 2's withdrawal is itself retracted** — it over-corrected.

*What draft 2 got right:* "σ ≈ 0.70" is simply the wrong number — `sd(x)` is **0.848**, and σ in that
formula is the sd of the **deviation** `x − λ`, not of `x`. And `exp(σ²/2)` is a poor approximation
at the realised dispersion (it gives 1.4327 against an exact 1.3265, 8% high), though excellent at
`XGC90`'s (1.0385 against 1.0386). **So quote the exact `E[exp(−x)]/exp(−x̄)` and never `exp(σ²/2)`.**

*What draft 2 got wrong, and it is the larger half:* cross-match convexity **does** explain the thing
it was invoked for — the gap **between the two regressors** — and it does so to **0.3%**. On
season-matched populations (both dumps restricted to 2022-23…2025-26): the `XGC90` dump's cross-match
factor is **1.0397** and the sibling's **1.3265**, so the mechanism predicts a ratio of **1.2759**
against an observed **1.2799**. Draft 2's two grounds for withdrawal both fail — the "not a proxy"
objection is a category error (errors-in-variables *slope attenuation* needs a proxy; this is a
statement about the **levels** of two aggregates and needs only that `x` is more dispersed at the
same mean), and the parameter objection compared a residual-sd formula against a total sd, and its
output against a different ratio than the one it targets.

**So: cross-match convexity explains the gap between the regressors. A shot-level wedge explains why
`exp(−x)` over-predicts on realised x at all.** Both are real; the decomposition below contains both.

**The shot-level half was already recorded in this repository, in a different file.** `xpoints.go`
and `stats/xg_provider_scale.py` name the **shot-level Jensen gap**: the model computes
`exp(−Σxᵢ)` but the true probability is `Π(1−xᵢ) ≈ exp(−c·Σxᵢ)` with `c = −E[ln(1−x)]/E[x] > 1`,
measured at **c = 1.27 on two independent providers over the same 380 matches**.

**Fitted here from the clean-sheet rate directly** — solve `mean(exp(−c·x)) = observed rate` on the
2870 sibling rows — gives **c = 1.2830**, against an independently measured 1.27 from shot
distributions rather than outcomes.

⚠️ **Consistent, and the point agreement is NOT the evidence.** An earlier version of this paragraph
said "agreement to 1.0%" and "that is the number". Both are withdrawn, on four counts:

- **The fit is exactly identified** — one free parameter matched to one moment — so it reproduces
  that moment by construction and **cannot test any mechanism.** It returns the same `c` for every
  rival story that reproduces the same aggregate ratio, including "the regressor is simply 28% high".
- **The interval is wide**: clustered 95% ≈ **[1.19, 1.39]**, SE ≈ 0.050. Quoting four decimals
  implies ±0.0001 where the standard error is ±0.05.
- **A second estimate of the same parameter sits twenty lines above it in the same output** —
  `cs_calibration.R`'s pure-SCALE fit, **f = 1.2556**, 2.2% the *other* side of 1.27. Quoting the
  nearer one is an argmax over two, and it was not disclosed.
- **The family that gives `c` its meaning is REJECTED on these very rows by this very script**:
  `a = +0.1438` (clustered t 2.94), LRT `p = 0.00075` against the pure scale, and the output prints
  "BOTH one-parameter families are rejected".

⚠️ **And 1.27 is the wrong season.** It is 2015/16, on 175 of 380 shared fixtures, with penalties.
On this dump's own seasons the same functional is **1.3291**, so 1.2830 is **−3.5%**, not +1.0%. The
source script's own conclusion is *"season variation beats provider variation ~6:1: name the season,
not the feed"* — and FPL's feed is **Opta**, which that script says it does not identify at all.

So `c` is a **regressor discriminator and a moment summary**, not a parameter this population
supports. The shot-level wedge is real, the right sign and order, and **suggestive rather than
established**.

### ⚠️ Why this changes the RISK even though it changes no level

Decomposing on the sibling population, where both wedges are visible:

| | value | factor |
|---|---|---|
| `exp(−x̄)` — what a smooth-regressor model computes | 0.2323 | — |
| `exp(−c·x̄)` — the shot wedge, at the mean | 0.1537 | **−33.8%** |
| `E[exp(−c·x)]` — cross-match convexity of the TRUE curve | 0.2383 | **+55.1%** |
| product of the two | | **1.0260** |
| observed rate ÷ `exp(−x̄)` | | **1.0261** |

⚠️ **The last two rows are ONE quantity, not two, and this table corroborates nothing.** `c` was
fitted so that `E[exp(−c·x)]` equals the observed rate, so the intermediate `exp(−c·x̄)` cancels and
the product telescopes into the final row — the 1.0260/1.0261 difference is rounding. Presenting
them adjacent and bolded reads as confirmation, which is structurally the same move ("the magnitudes
line up") being retracted three paragraphs above.

⚠️ **And the individual wedge sizes are an artefact of where the identity is cut.** Splitting through
`E[exp(−x)]` instead gives **+32.7%** and **−22.6%** on the same rows — half the size, signs swapped,
identical product. **Quote the product, which is measured. Never the wedges.**

**The realised clean-sheet rate sits within 2.6% of `exp(−mean x)` — which is what a model scoring a
smooth regressor computes.** So the near-calibration this document reports is a **cancellation of two
opposing wedges**, not a structural property of using a smoother input — a product of **1.026**, per
the paragraph above. Say "the two wedges" and quote the product; the sizes in the table are an
artefact of where the identity was cut.

⚠️ **The fragility is in the MEAN, not the dispersion — an earlier version of this paragraph named
the wrong lever, and the banked rows size both.** On `cs_regressor_rows.csv` the *entire* cross-match
convexity of `XGC90` is **1.0410** (native, n 1566, sd 0.283), and that factor is the only channel by
which dispersion reaches calibration. So **annihilating** the dispersion moves calibration 4.0%, and
its recorded season range 0.204-0.301 spans **2.5%**.

Calibration goes as `exp((c−1)·x̄)`, so the **mean is the larger lever**: a **10% shift in mean
`XGC90` moves calibration 4.1%**, more than removing all of its dispersion. And of the examples the
withdrawn sentence listed, the one that matters most — the reconstructed-xGC seasons — is a **level**
problem (ever-present error 3.0-5.2% → 16.0-20.2%), entering by exactly the channel it did not name.

**Watch level-moving interventions.** The `f`-versus-flat 2x2 has run and does not resolve — the
canary puts the family about four times below detection, so `CLAUDE.md` closes the points question
and forbids a re-run at the refitted constants. Any *future* arm on this term must be designed off
the level channel.
⚠️ Arithmetic off the banked rows, not a measurement — and it assumes the shot scale `c` is
population-independent, **which nobody has checked**.

⚠️ **A sentence withdrawn earlier is re-issued here against a DIFFERENT pair, and the withdrawal
still stands for the old one.** This file once read "the near-zero is two opposing biases roughly
cancelling" and was withdrawn because a reader would map it onto the two *selection* biases, which
both run the same way — **that reason is unchanged, and both pairs are still in this document.** The
phrase is correct of the two *wedges*, which the withdrawn version did not name. It comes back
because it was withdrawn for being **unreferenced, not for being false**, so nothing measured had to
be overturned to re-issue it. **Say "the two wedges", never "two opposing biases".** ⚠️ An earlier
version of this note said the sentence was "right all along, for the wrong reason", which implies
the withdrawal was a mistake. It was not.

`cs_calibration.R`'s header anticipated this and settled it **by assertion**, in the direction
favouring its own fit — *"a slope above 1 measured here is if anything an understatement of what the
model's own regressor wants. That is a mechanism argument, not a measurement."* This is the
measurement, and it goes the other way.

---

## What is NOT established — read this before quoting anything above

**The model is NOT shown to be correctly calibrated.** ⚠️ **An earlier version of this file opened
with "correctly calibrated, and both one-parameter families accepted". That is withdrawn.** Two
LRTs failing to reject is not acceptance. The free fit's native interval is **b ∈ [0.695, 1.289]**
and **ratio ∈ [0.90, 1.20]** — a 20% over-prediction sits inside it.

**The SLOPE carries nothing.** Free fit `b = 0.9922` (clustered SE 0.1516):

- `t(b = 1) = −0.05` — not rejected
- `t(b = 1.1731) = −1.19` — **also not rejected**, against `t_crit` 2.00
- 80%-power MDE on `|b−1|` is **0.424**; the candidate correction is **0.173**, under half. Power at
  b = 1.1731 is roughly **18%**.

So b = 1 and b = 1.1731 are **not separable by this fit**.

⚠️ **"The refit refutes b = 1.1731" is FALSE on the headline stratum.** Only the POOLED stratum
rejects it (t −2.44) — and pooled is the stratum this file labels context, whose
errors-in-variables bias runs in exactly that direction. **A stratum cannot be disowned for the null
and borrowed for the rejection.** What is refuted is the recorded **1.281 ratio**, on the level, on
both strata.

### Population limits, each of which flatters the result

- **The representative selection is not neutral.** ⚠️ **An earlier version of this file said the
  representative is "usually the ever-present keeper". That is WRONG on the headline stratum** —
  joining the banked dump to the archive by element id gives **765 GKP against 801 DEF** on native
  (1577/1397 pooled), so it is a defender more often than a keeper. What survives is the property
  the argument actually needs: it is the **most-played** defender or keeper at each club (≈1.85
  distinct representatives per season-team; 46 of 120 have exactly one all season), hence the
  **most** current-season evidence — least prior-blended, least shrunk: the model's best case. The
  optimiser buys from the whole pool, including rotated and newly-arrived defenders whose `XGC90` is
  prior-driven. 2022-23's `b = 0.298` is what a degraded regressor looks like here.
- **The refit omits `defconCleanFactor`, a FIFTH copy of the expression. ✅ SIZED 2026-08-15.**
  `csregressor_test.go` computes `pred += exp(-x)` where the engine evaluates
  `cleanSheetProb(xgc, def, cf)`. The coupling moves **214 of 2974 pooled rows (7.2%)**; applying it
  takes predicted from 0.2650 to 0.2669 and the pooled ratio from **1.0038 to 1.0112**. So it too
  runs toward this document's conclusion, and is now a number rather than a caveat.
  ⚠️ **The mechanism I first gave for it was wrong.** I wrote that the coupling "can only lower a
  predicted clean sheet"; the aggregate rises, because the factor sits **below** 1 for most
  defenders it touches and `exp(-x·cf)` then increases. The direction of the bias is as claimed; the
  reason is the opposite of the one asserted. Read the direction off the numbers.

**Together the two sized biases move the pooled over-prediction from ~0.4% to roughly 3.7%**
— `1.0038 + 0.0254 + 0.0074`; multiplicative composition gives `1.0368`, so the two agree. That
leaves the refutation of 1.281 completely intact and rules out any reading of "calibrated to within
half a percent" — which is the reading the first version of this document came close to.

⚠️ **It is a COMPOSITION of two separately-measured shifts, not a joint measurement.** The coupling
was sized on kept rows only and the selection with the coupling omitted, so an interaction of order
0.1pp is unmeasured; the two also use slightly different `actual` denominators (fixture-derived
0.2636 against player-level 0.2640). ⚠️ **And this read "roughly 3%" until it was checked** — the
selection figure alone, carried into five files under the word "together". A smaller number is
closer to "calibrated", so **the error ran the flattering way**, which is the direction this record
warns about.
- **The 90-minute guard conditions on a post-cutoff outcome. ✅ SIZED 2026-08-15.**
  ⚠️ **An earlier version of this file said "22% of team-matches dropped", taken from an audit
  estimate and not counted. It is 14.2%** — 492 dropped against 2974 kept, single-fixture
  club-gameweeks. Repeating an unverified figure into four files is the failure this record keeps
  paying for, and it is recorded rather than quietly corrected.
  **The selection is real and runs the feared way.** Clean-sheet rate is **0.1992 on dropped against
  0.2636 on kept**, measured from the FIXTURE (the opponent's score) rather than the player's own
  `CleanSheets` — which FPL awards only at sixty minutes, so on precisely the dropped rows the
  player-level field is unusable and would have manufactured the comparison it is meant to test.
  Carrying `pred` on the dropped rows too, the pooled ratio is **1.0051 kept-only against 1.0305
  unselected**. ⚠️ That still is not the population the model scores: it conditions on a club having
  a most-played DEF/GKP with positive `XGC90` at all, and prices the club by that one player. It
  removes the 90-minute selection and nothing else.
- **It measures the NEUTRAL path only.** `fixtureAdjustedXP90` scores `exp(−f·XGC90·def)` per
  fixture; this fixes `def = 1, cf = 1`. Convexity in `def` means the scored path over-predicts
  relative to the neutral one. Second-order (~1% at plausible spreads) but unmeasured.
- **The comparison leg cannot be checked from this repository.** The recorded `a = 0.1003,
  b = 1.1731` came from a dump of 2598 native rows on a four-season grid including 2022-23 GW16+;
  this is 1566 rows over three seasons. Not "the same population minus 2022-23" — the grid differs
  too, and that dump is banked nowhere here. Direction is conservative (2022-23's b = 0.298 would
  pull the refit further from 1.1731), but it is unverifiable.

---

## The fixture path (`XGC90 × def`) — RUN 2026-08-15, and it does not resolve

The neutral refit fixed `def = 1`. `fixtureSensitiveAt` evaluates `exp(−f · XGC90 · def · cf)`, so
the fixture path was the one place `FPL_CS_XGC_FACTOR` could still be live. Same diagnostic, second
dump (`cs_regressor_fixture_path_rows.csv`), `xgc` holding the product so `cs_calibration.R` fits it
unchanged. Every one of the 2974 rows had a matching fixture; mean `def` is **1.0058**.

| stratum | n | a (clustered SE) | b (clustered SE) | t(a=0) | t(b=1) |
|---|---|---|---|---|---|
| native | 1566 | −0.2012 (0.1811) | **+1.2022 (0.1331)** | −1.11 | **+1.52** |
| pooled | 2974 | −0.1290 (0.1235) | +1.1229 (0.0941) | −1.04 | +1.31 |

Per-season `b` on native: 1.095 / 1.098 / **1.393** — **above 1 in 3 of 3**. Predicted/actual is
1.0207 against the neutral arm's 1.0038 on the same rows.

**Neither restriction is rejected on either stratum** (LRT p 0.20 scale / 0.098 offset native), and
`t(b = 1) = 1.52` against a `t_crit` of 2.00. **Nothing resolves.**

### The two channels DO separate, and the factor is dead on this path too

⚠️ **An earlier version of this section said "this design cannot separate them" and let P1
arbitrate. That is RETRACTED — it is false, and it buried the finding.**

`ladder(base, scale)` is `1 + scale·(base−1)`, so the engine's exponent under the two shipped knobs
is exactly

```
f · x · (1 + s·(def−1))  =  f·x  +  f·s·x·(def−1)
```

— **linear in two regressors this dump already carries as separate columns.** So `b1 = f`
(`FPL_CS_XGC_FACTOR`) and `b2 = f·s` (`f` times `FPL_DEF_FIXTURE_SCALE`'s defensive half), and the
shipped position is `b1 = b2 = 1`. A mis-set clean-sheet factor moves **both**; a mis-scaled
defensive ladder moves **only `b2`**. That is a nested test, not a tiebreak. It is now banked in
`stats/cs_calibration.R` rather than living in a review.

| stratum | b1 (clean-sheet factor) | b2 (defensive ladder) | t(b2−b1) | cor(w1,w2) |
|---|---|---|---|---|
| native | **+1.0476 (0.1612), t 0.30** | **+1.5688 (0.2253), t 2.53** | +2.11 | 0.001 |
| pooled | +0.9568 (0.1097), t −0.39 | +1.5654 (0.1712), t **3.30** | +3.25 | 0.012 |

Per season, native: `b1` 0.968 / 0.905 / 1.158 — straddles 1. `b2` 1.481 / 1.439 / 1.887 — above 1
in 3 of 3, and **6 of 6 pooled**. Implied `s` ≈ **1.50** native, 1.64 pooled.

**So the fixture path does not revive `FPL_CS_XGC_FACTOR` — it refutes it a second time, on a second
path, and relocates the excess onto the defensive fixture ladder.** The near-zero correlation means
the two channels are separately identified rather than trading off, which is why the product fit's
`b = 1.2022` is a blend of a null and a real effect.

**And the relocation lands on an already-closed line.** *"Neither fixture ladder has a shape. Both
span less than the noise across a fourfold change in width, and zeroing the defensive response
entirely costs 20 points."* An `s` of 1.5 sits well inside a width range already measured as
points-null. **So this is a calibration fact with no reachable points consequence** — which is a
stronger and more useful statement than "neither revived nor dead".

⚠️ **POST-HOC.** The decomposition was written after seeing the product fit. It is not an argmax over
a lattice — it is the unique two-parameter nesting of the engine's own `ladder`, and P2/P3 named the
discrimination in advance — but it was found after the fact and wants a pre-registered re-emission
before anything is built on it.
⚠️ **`def` is read from the archive's end-of-season fixtures file and is not gated by the cutoff.**
Two live captures three days apart show **0 of 380** difficulties changing, so revision looks rare —
but that window is pre-season and reaches no archived season. The direction of any leak flatters
`b2`.
⚠️ **Corrected in place 2026-08-16, not rewritten — the "0 of 380" sentence is WITHDRAWN.** Between
those same two captures **0 of 20 clubs changed any team field**, so the antecedent never fired: the
observation cannot distinguish "difficulty does not track strength" from "nothing moved". What
replaces it is a measurement, not a caveat. FPL's difficulty is an exact step function of the
opponent's fine `strength_overall_*` rating, and the archive's column is reproduced by the
**end**-of-season strength on **4560/4560** fixture-sides against **2755/4560** by the season-start
value — so the column is measurably end-stamped and a season-start-frozen column is refuted.
Pricing that leak by refitting `b2` on a point-in-time reconstruction is **C — UNMEASURABLE** at
three season clusters, both canaries firing:
`stats/defensive_fixture_pointintime/fit.txt`. The rest of this paragraph stands.
⚠️ **Native season-clustering does not clear**: 4.14 on 3 clusters against `t_crit(2)` 4.303. The
season×team figures above are the ones quoted.

### The pre-registration collided, and that is still a defect

- **P3 fired**: `b > 1` on the product with `b ≈ 1` on the neutral was named as the reading that
  would reopen the factor.
- **P2 fired too**: a large move from 0.9922 was named as implicating the difficulty ladder.

**P2 won, on a nested test rather than an arbitration.** Writing two readings without a tiebreak was
the mistake, and the fix was a better estimator rather than a judgement call. ⚠️ **P1's premise was
also materially wrong**: at shipped config `band_strength` is 0 and the magnitude path is off, so
`def` is `defenceMultiplier(Difficulty)` and nothing else — five values (0.7/0.85/1.0/1.2/1.4). The
only "modelled" content in it is this project's choice of that table, which is exactly what `b2`
measures.

⚠️ **P4 never fired and could not**: all 2974 rows had a matching fixture, because the upstream
`Fixtures != 1` guard already forces exactly one match. That is a byte-identical null on P4 — a
guard that could not run — not evidence a population survived a filter.

**On differencing the two slopes**: the earlier refusal was right in outcome and wrong in reason.
`b_product − b_neutral` is ill-posed because the two coefficients sit on differently scaled
regressors, not because no standard error exists. The well-posed contrast is `t(b2 − b1)` **inside
the joint fit**, above, where the cluster-robust covariance supplies the correlation.

---

## The 2x2 — unmeasurable at 36 cells, and the CANARY is the number that says so

The sweep was in flight when the refit landed, so its arms carry `f = 1.1731, flat = 0.9046` — the
values the refit voids. `HOLD`, paired against shipped:

| arm | pts/gw | ×38 | CR2 SE | t | p raw | p wild | threshold (clu / fixed) |
|---|---|---|---|---|---|---|---|
| factor only | +0.051 | +1.9 | 0.240 | 0.21 | 0.8408 | 0.8071 | 23 / 11 |
| flat only | +0.164 | +6.2 | 0.159 | 1.03 | 0.3512 | 0.3395 | 16 / 15 |
| both | +0.184 | +7.0 | 0.206 | 0.89 | 0.4136 | 0.4691 | 20 / 9 |
| **CANARY** flat=0.5 | −0.569 | −21.6 | 0.285 | −2.00 | 0.1024 | 0.1239 | 28 / 37 |

df 5, `t_crit` 2.571; thresholds are `2.571 × SE_CR2 × 38`. Holm family 4; every candidate 1.0000.
All arms `movable` 6/6, bootstrap floor 0.000129.

**The canary is the design finding.** Halving *every* clean-sheet probability — a gross, deliberate
miscalibration — costs only **−21.6 a season against its own threshold of 28** and **does not
resolve**. The proposed flat arm is a 9.5% cut, about a fifth of the canary, so its expected
magnitude is ~4 points a season against a threshold of 16. **This 2x2 was underpowered by a factor
of about four before it started.** The lesson is procedural: **size the candidate against the canary
before spending 180 cells, not after.**

### P4 and P5 discharged — the interaction, and the factorial main effects

⚠️ **An earlier version of this file omitted these entirely**, which left the pre-registration's P4
undischarged: it commits to reporting the interaction "with the two main effects whatever it does".
Computed from the banked cells, season-clustered, df 5:

| contrast | pts/gw | ×38 | CR2 SE | t |
|---|---|---|---|---|
| factor main, **simple** | +0.0508 | +1.9 | 0.2403 | +0.21 |
| flat main, **simple** | +0.1637 | +6.2 | 0.1593 | +1.03 |
| factor main, **factorial** | +0.0355 | +1.4 | 0.1951 | +0.18 |
| flat main, **factorial** | +0.1484 | +5.6 | **0.0604** | **+2.46** |
| **INTERACTION** | −0.0306 | −1.2 | 0.2252 | −0.14 |

Two things follow, and the second is a failed prediction.

**The factorial flat main effect is the tightest estimate in the run** — SE 0.0604 against the
simple effect's 0.1593, and t **2.46 against a `t_crit` of 2.571**. It is the only quantity here
that comes near its bar. ⚠️ **It still does not clear it, it is one of three pre-registered
contrasts so Holm applies, and it estimates the effect of a constant the refit voids.** Averaging
over both factor settings is what buys the precision; that is the factorial design working exactly
as the record says it should.

⚠️ **P4's diagnostic prediction FAILED and is recorded as failed.** It predicted the interaction's
clustered SE would come back *smaller* than the main effects', because a difference of differences
within one cell cancels the path divergence a single difference carries — and said that if it came
back larger, "something is wrong with the pairing, not with the football". At 0.2252 it is larger
than both factorial main effects (0.1951, 0.0604) and than the simple flat effect (0.1593).
**Unexplained.** The honest reading is that the two arms' squad paths diverge in ways that do not
cancel, which is a fact about this comparison rather than about the football — but nothing here
tests that, and the recorded 0.216-against-0.599 precedent does not transport.

**P5's family was pre-registered as three contrasts; the banked Holm family is 4** — four
arm-vs-baseline levels including the canary, which is not a hypothesis test. Everything reads
1.0000 so nothing turns on it, but the run did not use the family it declared.

### P1, the mediator, read before the points columns

| arm | cells | 2020-21 | 2021-22 | 2022-23 | 2023-24 | 2024-25 | 2025-26 |
|---|---|---|---|---|---|---|---|
| factor only | 17/36 | 5 | 5 | 4 | 0 | 2 | 1 |
| flat only | 17/36 | 4 | 6 | 4 | 0 | 2 | 1 |
| both | 20/36 | 5 | 6 | 3 | 2 | 3 | 1 |
| **CANARY** | **35/36** | 6 | 6 | 6 | 6 | 5 | 6 |

**The canary passes at 35/36, so every arm reaches the scored path and no null here is a
byte-identical null.**

⚠️ **The concentration of the candidate arms in the reconstructed-xGC seasons is UNEXPLAINED.** An
earlier version of this file attributed it to "the factor amplifying regressor error in the
exponent". **That is contradicted by this table**: `flat only` also moves 4/6/4 there, and a flat
scale amplifies no regressor error at all.

### The +7.0 is one season

Leave-one-season-out, points per season:

| arm | full | −20-21 | **−21-22** | −22-23 | −23-24 | −24-25 | −25-26 |
|---|---|---|---|---|---|---|---|
| factor only | +1.9 | +4.3 | **−5.8** | +7.8 | +2.6 | +1.2 | +1.5 |
| flat only | +6.2 | +2.9 | +3.1 | +9.8 | +7.0 | +5.4 | +9.2 |
| both | +7.0 | +10.6 | **+0.3** | +10.1 | +9.0 | +5.5 | +6.5 |

**Dropping 2021-22 takes `both` from +7.0 to +0.3 — 96% of the effect is one season** — and
`factor only` changes sign. Same shape as the bench-slot weights and the residual gate arm.

**2021-22's own refit slope is `b = 0.855`, `a = +0.172` — but read it weakly.** ⚠️ **An earlier
version of this file called that a "kill shot", and it was the move this same document forbids
eleven lines above.** 2021-22 is a reconstructed-xGC season, so it sits only in the **pooled**
stratum this file disowns, and that stratum reads 1.41 in 2020-21 and 0.30 in 2022-23 — regressor
degradation, not football. A stratum cannot be disowned for the null and borrowed for the kill shot
either.

The defensible statement is weaker and stratum-free: **`b < 1.1731` in 5 of 6 seasons**
(1.409 / 0.855 / 0.298 / 0.872 / 0.902 / 1.179), and 2021-22's 0.855 sits between the two native
seasons' 0.872 and 0.902 — unremarkable. So nothing in the refit supports the installed `f` in the
season carrying the gain, or in any other. Whatever produced +40 points there, the refit gives no
reason to call it better calibration.

**`flat only` is the arm with shape**, and it is the one worth naming: sign-stable under all six
LOSO drops, span +2.9 to +9.8, full +6.2 against a threshold of 15-16. Still unresolved, still not a
result.

---

## What must NOT be claimed

That the clean sheet is correctly calibrated. That `b = 1` was accepted. That the refit refutes
`b = 1.1731` (it does not, on its own headline stratum). That the 2x2's +7.0 is evidence of
anything. **That the flat scale is ordering-inert** — it multiplies one additive component of
`Score`, so defenders with different clean-sheet shares reorder; `xgcrepair.go`'s own warning is
that the position-wide precedent is about an *additive* bias and must not be cited for a
multiplicative scaling.

**Contamination events.** No absolute total is quoted; every points figure is a paired difference
within a cell, so the five events cancel through the shared baseline. The refit is archive-side
calibration and none of the five reach it. The history this supersedes is exposed to **doubles
half-counted** — the 85 doubled rows behind the withdrawn "wrong family" refutation.

---

## What is left, in priority order

1. ✅ **DONE 2026-08-15 — see "The fixture path" above.** It does not resolve, and it is a bound
   rather than a calibration exactly as the caveat predicted. What replaces it: **an arm that varies
   `def` independently of the clean sheet**, which is the only way to separate "the model
   under-reacts to fixture difficulty inside the clean sheet" from "the difficulty ladder is
   mis-scaled". That is a different experiment from anything run here.
2. ✅ **DONE 2026-08-15 — see "Population limits" above.** Both biases are sized; the level claim
   now carries ~3.7% rather than a caveat. ⚠️ **What is still owed from it**: the banked
   `cs_regressor_rows.csv` holds the 2974 kept rows only, with no `kept` flag and no `cf` column, so
   **neither sizing can be recomputed from the bank** — both need the Go diagnostic re-run. That is
   the same defect this document complains about for the comparison leg two items below, now true of
   its own headline correction. Emit all 3466 rows with `kept` and `cf`.
3. ✅ **DONE 2026-08-15** — `cs_sibling_realised_xgc_rows.csv`, 2870 rows. The comparison leg is now
   readable from the repository, and banking it is what made the mechanism re-derivation possible:
   the fitted `c = 1.2830` comes from those rows. `stats/cs_calibration.R` now prints the implied
   Jensen scale for whatever dump it is given — **~1.28 identifies a realised-match-xGC regressor,
   ~1.00 the model's smoothed one** — so the two can no longer be confused by a reader who only has
   one file.
4. **Do NOT re-run the 2x2** with refitted constants (`f = 0.992, flat = 0.939`). The canary says
   the family is out of reach in points by a factor of four, and a factor arm that is a no-op has
   nothing to install.
