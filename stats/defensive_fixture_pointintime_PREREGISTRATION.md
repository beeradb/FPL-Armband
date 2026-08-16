# Pre-registration: refit the defensive fixture coefficient on the difficulty that was actually available at each cutoff

Written and committed **before any code that fits the estimand exists**. The
commit that adds this file adds the reconstruction
(`stats/defensive_fixture_pointintime_join.py`) and the canary sizing
(`stats/defensive_fixture_pointintime_sizing.R`) and nothing else;
`stats/defensive_fixture_pointintime.R`, which fits `b2_pit`, arrives in a later
commit.

**Exactly what had been computed before this file was written**, so that the
disclosure is auditable rather than a claim:

- the reconstruction and every design quantity in `join_liveness.txt` — row
  counts, the `def_pit` distribution, the liveness fraction, the map checks. No
  outcome enters any of them; the join reads `clean_sheet` only to copy it.
- the **control** fit `a`, `b1` and `b2_end` on the native stratum
  (`sizing.txt`). §5 independently *requires* this fit to reproduce the banked
  `b2 = 1.5688` and `b1 = 1.0476`, which it does to six decimals, so it carries
  no information this record does not already hold. It is needed here because a
  canary sized after its own standard error is not a canary.
- the two canary poles in §7, which are projections of candidate
  data-generating processes and touch no outcome beyond the control's three
  coefficients.

**Not computed**: `b2_pit` on real outcomes, the Model D contrast on real
outcomes, and any standard error, t, p or threshold whatsoever.

---

## 1. The claim under test

`stats/cs_calibration.R` fits, on the fixture-path regressor dump
(`stats/snapshots/2026-08-15-clean-sheet-2x2/cs_regressor_fixture_path_rows.csv`),

```
-ln P(clean sheet) = a + b1*w1 + b2*w2      w1 = XGC90,  w2 = XGC90*(def-1)
```

and banks, on the **native** stratum (2023-24, 2024-25, 2025-26; n = 1566):

| | estimate | clustered SE (season × team) | t vs 1 |
|---|---|---|---|
| `b1` (the clean-sheet factor `f`) | +1.0476 | 0.1612 | +0.30 |
| `b2` (`f` × the defensive half of `FPL_DEF_FIXTURE_SCALE`) | **+1.5688** | 0.2253 | +2.53 |

`def` is `defenceMultiplier(fixture.Difficulty) * defenceBandAdj(...)`, and with
the shipped `band_strength: 0` the band adjustment is exactly 1, so `def` is a
pure function of the archive's difficulty column. `internal/backtest/season.go`
reads one difficulty per fixture — the end-of-season value — and `playedFixtures`
strips the scoreline and the `Finished` flag but **not** the difficulty. The
archive's column is measurably end-stamped: on rows where the opponent's rating
moved, `def` tracks the opponent's end-of-season strength at Spearman 0.872
against 0.421 for its value at the cutoff, and `def` is a per-club-venue constant
all season in all six seasons.

**Hypothesis under test.** If the end-stamped difficulty carries post-cutoff
information, `b2 = 1.5688` is inflated: part of the fit is `def` describing what
happened rather than forecasting it. Rebuilding `def` from the opponent's
strength **as it stood at each row's own cutoff** removes that channel, and the
refitted coefficient says how much of the 0.5688 excess it was carrying.

**Why this arm and not the predecessor's.** The predecessor
(`defensive-fixture-coefficient-hindsight-gate`) tested an interaction with a
binary "the opponent's coarse strength moved at some point this season"
indicator, identified off 221 of 1566 rows, and returned UNMEASURABLE. This is
not that test. It is a point-in-time reconstruction: every one of the 1566 rows
gets a difficulty, the regressor is continuous and signed rather than an
indicator, and the estimand is the same coefficient the record already banks.

## 2. What is measured, what is decoded, and the one thing that is assumed

**Decoded and checked, not assumed.** FPL's fixture difficulty is an exact,
global, monotone step function of the opponent's **fine** `strength_overall_*`
rating, keyed by venue:

```
difficulty faced AT HOME = step(opponent's strength_overall_home)
difficulty faced AWAY    = step(opponent's strength_overall_away)
step(s) = 1 if s < 1000, 2 if s < 1105, 3 if s < 1220, 4 if s < 1350, else 5
```

This reproduces **4560 of 4560** archived fixture-sides across all six seasons,
760 per season, exactly. The gate is *residual identically zero*, never *a good
fit*: `defensive_fixture_pointintime_join.py` aborts on a single mismatch.

⚠️ **Which field, and why there was no choice.** The coarse 1-5 `strength` — the
field the predecessor arm keyed on — is neither deterministic nor monotone at
either venue, and neither is any of `strength_attack_*` or `strength_defence_*`.
The full screen prints in `join_liveness.txt`. Only `strength_overall_home` (at
home) and `strength_overall_away` (away) qualify, and each only at its own venue.
So the field is forced by the data rather than selected, and the question "does
the choice change the answer" has no other arm to compare against: no other field
yields a map at all.

**Three checks on the mechanism step, all of which fired before this file:**

1. the map is exact on every archived fixture-side, 4560/4560;
2. the **venue pairing is verified on a live payload**. `data/captures/2026-*`
   are the only captures in this project carrying `fixtures.json.gz` beside
   `bootstrap-static.json.gz`. In 2026/27 FPL publishes `strength_overall_*`
   already on the 1-5 scale, so the map degenerates to the identity and the
   thing left testable is precisely the structural claim: **760/760 fixture-sides
   match venue-matched, against 456/760 venue-swapped**. FPL derives difficulty
   from `strength_overall_*` and from nothing else. ⚠️ The decoded *thresholds*
   do not transport to those payloads — the units changed — so this is a check on
   structure only;
3. the GW38 capture agrees with the archive's own `teams.csv` on both fine fields
   for **20 of 20 clubs in all six seasons**, so the reconstruction is continuous
   into the banked column at season end rather than a differently-built column.

**Assumed, and not checkable from this archive**: that FPL re-published the
difficulty when it revised a strength mid-season, rather than computing the
difficulty column once at season start. The per-gameweek captures hold no
fixtures payload, so no archived capture can show a difficulty moving. If FPL
froze the column, `def_pit` is not what the live engine saw and this arm
measures something else. Nothing below may claim to have recovered the true
point-in-time difficulty.

⚠️ **The one live observation the record quotes against this hypothesis carries
no information.** `cs_calibration.R` records "two live captures three days apart
show 0 of 380 difficulties changing". Between those same two captures **0 of 20
clubs changed any strength field**, so the antecedent never fired: that
observation cannot distinguish "difficulty does not track strength" from
"nothing moved". It is neither evidence for nor against, and it is not quoted
below as either.

**Threshold placement is not a free parameter here.** The decoding leaves four
empty intervals — (975, 1000), (1100, 1105), (1215, 1220), (1340, 1350) — inside
which a threshold could sit anywhere. **Zero** of the 1566 native rows (and zero
of the 2974 pooled) have a cutoff strength landing in one, so no placement
changes a single reconstructed difficulty. That is a measured sensitivity of
exactly zero, not an argument.

## 3. Estimator, clustering, df, and the threshold

Binomial GLM with a **log link**, coefficients negated to read on `-ln p`,
exactly as `stats/cs_calibration.R` does — same family, same link, same sign
convention, so every fit here nests or reproduces the banked one.

```
Model E (control)  -ln p = a + b1*w1 + b2*w2_end
Model P (primary)  -ln p = a + b1*w1 + b2*w2_pit
Model D (H1)       -ln p = a + b1*w1 + b_pit*w2_pit + b_rev*w2_rev

  w1     = XGC90
  w2_end = XGC90*(def_end - 1)        the banked regressor
  w2_pit = XGC90*(def_pit - 1)        rebuilt at each row's own cutoff
  w2_rev = w2_end - w2_pit            the revision the end-stamp adds
```

Model D nests both: `b_pit = b_rev` collapses it to Model E, and `b_rev = 0`
collapses it to Model P.

**Clustering axis: season.** Native stratum G = 3 → **df = 2**, `t_crit(2) =
4.303`. Pooled stratum G = 6 → df = 5, `t_crit(5) = 2.571`. Variance from
`clubSandwich::vcovCR(..., type = "CR2")`. Model D has 4 parameters against 3
clusters, so **the CR2 meat will be rank-deficient on the native stratum** — this
is declared now rather than discovered: the rank is printed, clubSandwich's
Satterthwaite df is printed as context, and the verdict is taken at the
pre-registered `G - 1 = 2` regardless. If CR2 is not computable the run falls
back to `sandwich::vcovCL` (CR0) at the same df and **says so in the output**; it
does not silently switch.

The season × team CR2 SE — the banked estimator, 60 clusters — is printed
**as context only** beside every coefficient. The verdict is taken on the
season-clustered SE at df 2. Changing the axis after seeing which one is
favourable is exactly what this record forbids, so it is fixed here.

**Detection threshold.** This is an archive-side calibration fit. It spends **no
replay cells**, there is no per-gameweek quantity, and `AGENTS.md`'s `× 38`
season conversion **does not apply**. The analogue in coefficient units is

```
threshold = t_crit(df) * SE      =  4.303 * SE   on the native stratum
```

and it is printed beside every coefficient that carries a verdict. **No points
figure will be quoted from this run.**

## 4. Confirmatory family, and the Holm correction

**Family size m = 2**, both on the native stratum, season-clustered, df 2. In
both, the null is **pole N — no artefact** (§7), so the burden sits on the
hindsight claim rather than on its denial.

- **H1 (primary)** — the Model D contrast `b_rev - b_pit = 0`. Under pole N this
  null holds **exactly**, by construction and to printed precision, because a
  DGP driven by `w2_end` puts the same slope on both components. Under pole H it
  equals **+1.202761**. Two-sided. A *negative* contrast would say the cutoff
  column tracks outcomes better than the end-stamped one, which no mechanism
  here predicts and which would indicate the framing is wrong rather than
  reversed.
- **H2 (secondary)** — `b2_pit = 1.434919`, pole N's exact value for the Model P
  level. Under pole H it is **1.000000**. Two-sided. This is the arm the task
  asks for directly and it uses all 1566 rows, but it is secondary because its
  separation is 2.8× narrower than H1's.

Holm at α = 0.05 with m = 2: the smaller p must clear **0.025**, which at df 2 is
`|t| > 6.205`; the larger must then clear 0.05, `|t| > 4.303`. Both thresholds
print beside the estimates. An arm that turns out unestimable does **not** shrink
the family — it was declared, and the cost it imposes on the other stands.

**Outside the family, declared now, reported with uncorrected t and carrying no
verdict:**

- **S1 — a staler cutoff.** `def_pit` rebuilt from capture `GW{gw-1}` instead of
  `GW{gw}`. If the reading is knife-edge on the cutoff convention it is not a
  reading.
- **S2 — the pooled six-season stratum.** Reported for both arms or neither, per
  the record's rule against leaning on a stratum for a rejection while disowning
  it for a null. Three of those six seasons carry reconstructed xGC, which makes
  `w1` a different construct.
- **S3 — the per-season table**, `b2_end` against `b2_pit` in each of the three
  seasons. Shape, not a test: it carries no standard errors, and a gap between
  two point estimates is not a result until it is divided by something.
- **S4 — `b1` confinement.** `w1` is untouched by the reconstruction, so `b1`
  should barely move between Model E and Model P. A large move means the fit
  broke, not that the fixture channel did. Diagnostic, not a test — and a
  confinement check alone confirms nothing, which is why §6's liveness is the
  check with power.

## 5. Required control — the run is void without it

Two, both of which `fail()` rather than print a caution:

1. **The map.** `defensive_fixture_pointintime_join.py` aborts unless the decoded
   step function reproduces every archived difficulty, 4560/4560. It also aborts
   unless the banked `def` equals `defenceMultiplier(end-of-season difficulty)`
   on 2974 of 2974 rows, which pins `band_strength = 0` and `defFixtureScale = 1`
   and so guarantees `def_pit` and `def_end` sit on the same ladder.
2. **The refit.** Model E refitted here must reproduce `b2_end = 1.5688` and
   `b1 = 1.0476`, and the season × team CR2 SE of **0.2253**, on the native
   stratum; and `b2 = 1.5654` with SE 0.1712 pooled. If it does not, the
   estimator or the population is wrong and **no result from this run may be
   quoted**. The fitting script must demonstrate that this control *can* fire.

## 6. Liveness — the bar the predecessor's could not clear, stated with the numbers obtained

The predecessor's stated liveness bar was tautological: its join copied `def`
through unchanged, so conditional on the row count the distribution was
bit-identical by construction. This one can fail. **If the reconstruction returns
the banked column, the same column has been rebuilt and the arm is void.** It
does not:

| check | native (n = 1566) | pooled (n = 2974) |
|---|---|---|
| **`def_pit != def_end`** | **438 / 1566 = 0.2797** | **756 / 2974 = 0.2542** |
| moved, by season | 2023-24 86/513, 2024-25 218/527, 2025-26 134/526 | 0.087 to 0.414 |
| `difficulty_pit - difficulty_end` | −2:7, −1:253, 0:1128, +1:172, +2:6 | −2:7, −1:384, 0:2218, +1:348, +2:17 |
| `def_pit` distribution | 0.70:16, 0.85:559, 1.00:613, 1.20:275, 1.40:103 | 0.70:16, 0.85:1146, 1.00:1054, 1.20:572, 1.40:186 |
| `def_pit` varies within `def_moved` = 0 and = 1 | all five / four levels | all five / four levels |
| **club-venue-season cells with more than one `def`** | `def_end` **0/120**, `def_pit` **59/120** | `def_end` 0/240, `def_pit` 106/240 |
| rows whose cutoff strength lands in a decoded gap | 0 | 0 |
| unjoined rows | 0 | 0 |

The last row of the table is the sharpest one. The archive's `def` is a single
value per club-venue for the whole season in **every** cell — which is what an
end-stamped column looks like, and is why no gateing of the archive could have
fixed this. The reconstruction breaks that constancy in **59 of 120** cells. The
movement runs in both directions, 253 down against 172 up, so it is not a
rescaling.

Design quantities for the identification, also fixed before the fit:
`cor(w2_pit, w2_rev) = −0.145` and `cor(w1, w2_rev) = +0.041`, so Model D's two
fixture channels are close to orthogonal rather than trading off; 438 rows inform
`w2_rev`, carrying 0.3638 of `sum(w2_end^2)`; 613 rows have `def_pit = 1` where
`w2_pit` vanishes.

## 7. The canary, sized exactly, before any standard error exists

⚠️ **This is the section the predecessor got wrong, and its error decided the
verdict.** It sized a full artefact as `(b2 - 1)/q_w`, an OLS omitted-variable
formula applied to a log-link GLM with no partialling and no IWLS weights,
reading 1.977 where the quantity that sentence named was 1.685 against a
threshold of 1.702. Nothing here is approximated. Every pole is a complete
data-generating process, evaluated row by row and **refitted by the same IWLS the
real fit uses**, which is the population projection the estimator actually
converges to. `stats/defensive_fixture_pointintime_sizing.R` computes it;
`sizing.txt` is its output.

**The obvious sizing is wrong, and wrong in the flattering direction.** It is
tempting to say a full artefact means `b2_pit = 1`, so the effect to detect is
`1.5688 − 1 = 0.5688`. That assumes the *other* pole sits at 1.5688, and it does
not: if the season-long column is the truth, then the cutoff column mismeasures
it, Model P is misspecified, and `b2_pit` is attenuated below 1.5688 with no
hindsight involved at all. The instrument has to separate the two **poles**, not
one pole from the banked number.

**Pole N — no artefact.** The banked model is the truth,
`mu = exp(-(a + b1*w1 + 1.5688*w2_end))`. It reproduces `b2_end = 1.568809` by
construction. Model P returns

```
  b2_pit = 1.434919          Model D contrast = 0.000000 (exact)
```

**Pole H — full artefact, and it must be CONSTRAINED.** An unconstrained
"the truth is `1 * w2_pit`" DGP satisfies the hypothesis but returns
`b2_end = 0.739052`, not 1.5688 — so it is not a candidate for what actually
happened, and **mismeasurement alone cannot manufacture the banked
coefficient**. The honest version solves for `(c_pit, c_rev)` on
`mu = exp(-(a + b1*w1 + c_pit*w2_pit + c_rev*w2_rev))` such that Model P returns
exactly 1 **and** Model E returns exactly 1.5688. It solves, to a residual of
2.30e-12:

```
  c_pit = 1.238857   c_rev = 2.441619
  Model P -> b2_pit = 1.000000     Model E -> b2_end = 1.568809
  Model D -> b_pit = 1.238857  b_rev = 2.441619  contrast = 1.202761
```

For the whole excess to be hindsight, the revision component has to carry a slope
of 2.44 against the point-in-time component's 1.24.

**The separations, which are what the canary is written against:**

| statistic | pole N | pole H | separation |
|---|---|---|---|
| Model D contrast `b_rev - b_pit` (**H1**) | 0.000000 | +1.202761 | **1.202761** |
| Model P level `b2_pit` (**H2**) | 1.434919 | 1.000000 | **0.434919** |
| *the naive sizing* | *1.568809* | *1.000000* | *0.568809* |

The naive sizing over-states H2's separation by **30.8%**, in the direction that
flatters the instrument — the same class of error as the predecessor's 17%.

**Declared now, before `SE` is known:**

- **H2 is UNMEASURABLE if `4.303 × SE(b2_pit) > 0.434919`**, i.e. unless
  `SE(b2_pit) < 0.101073`. For scale, the banked `b2_end`'s own season-clustered
  SE is 0.1375, so H2 firing is the expected outcome and would not be a surprise.
- **H1 is UNMEASURABLE if `4.303 × SE(b_rev - b_pit) > 1.202761`**, i.e. unless
  `SE(contrast) < 0.279517`.
- If **both** fire, the verdict is C and no reading of either may be quoted in
  either direction.

Pole N is nearly flat in the DGP intercept — 1.4361 at `a − 0.10` and 1.4340 at
`a + 0.10` — so the sizing does not turn on the one control coefficient the
record does not separately publish.

## 8. Decision rule — every outcome named

Evaluated on the native stratum, season-clustered, df 2, Holm over m = 2.

- **A — HINDSIGHT CARRIES THE EXCESS.** H1 rejects Holm-corrected
  (`|t| > 6.205`) with the contrast **positive**, and `b2_pit` sits closer to
  1.000 than to 1.435. Verdict: the banked `b2 = 1.5688` is inflated by
  post-cutoff information, and no reading of the defensive fixture channel's
  calibration may be quoted from the end-stamped column.
- **B — HINDSIGHT DOES NOT CARRY THE EXCESS.** Neither arm rejects, `b2_pit`
  stays within 0.10 of pole N's 1.434919, **and** the §7 canary says the
  separation was detectable. Verdict: the end-stamp is real and does not explain
  the excess. ⚠️ Even here this is a *tie that pole N survived*, not a
  confirmation of pole N.
- **C — UNMEASURABLE.** Both canaries fire. Verdict: this instrument cannot
  separate a full artefact from none, and no reading of `b2_pit` or the contrast
  may be quoted as evidence in either direction. A fact about the instrument, not
  about the constant.
- **D — UNRESOLVED.** Anything else — in particular one canary firing and the
  other not, or a `b2_pit` that lands between the poles without either arm
  rejecting, which is consistent with a partial leak and with none.

**C and D are both plausible before the fit and D is the modal expectation**,
given that the anchor coefficient itself reads t 4.14 against a 4.303 bar on this
very stratum. An honest C or D is a good outcome of this run and will be reported
as such. **No verdict will be upgraded by switching stratum, estimator or
clustering axis after the fact**, and no verdict will be taken from S1-S4.

If a correction to this file is needed after the fit, it goes in the fit output
under a dated post-fit heading and **this file is not edited** — a
pre-registration edited after its fit is not one. A post-fit correction may move
a verdict toward quoting *less*, never toward quoting more.

## 9. What this run does not do

- It runs **no replay sweep** and spends **no cells**. `HOLD` and `POLICY` are
  untouched, no Go code changes, and no points figure is produced anywhere.
- It changes **no shipped constant**. `FPL_DEF_FIXTURE_SCALE` and
  `FPL_CS_XGC_FACTOR` stay where they are. The record already holds the
  defensive ladder to be points-null across a fourfold width change, so a
  calibration reading here has no reachable points consequence and licenses no
  move.
- It does **not** establish that FPL's live difficulty moved during an archived
  season. See §2.
- It does not touch `AGENTS.md`.
