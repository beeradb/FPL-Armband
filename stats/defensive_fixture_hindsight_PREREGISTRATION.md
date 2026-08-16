# Pre-registration: is the defensive fixture coefficient `b2 = 1.5688` an artefact of hindsight?

Written and committed **before any fit was run**. The commit that adds this file
adds no fitting code; `stats/defensive_fixture_hindsight.R` arrives in a later
commit. The previous pass on this line was post-hoc and `AGENTS.md` records it as
such — this file exists so that this one is not.

What had been computed before this file was written: the join and its **design**
quantities only (row counts, the `def` distribution, the `revised_opp` share,
`q_w`). No coefficient, standard error or t has been looked at.

---

## 1. The claim under test

`stats/cs_calibration.R` fits, on the fixture-path regressor dump,

```
-ln P(clean sheet) = a + b1*w1 + b2*w2      w1 = XGC90,  w2 = XGC90*(def-1)
```

and banks, on the **native** stratum (2023-24, 2024-25, 2025-26; n = 1566):

| | estimate | clustered SE (season × team) | t vs 1 |
|---|---|---|---|
| `b1` (the clean-sheet factor `f`) | +1.0476 | 0.1612 | +0.30 |
| `b2` (`f` × the defensive half of `FPL_DEF_FIXTURE_SCALE`) | **+1.5688** | 0.2253 | +2.53 |

`AGENTS.md` records the same `b2` **season-clustered** at t = 4.14 on 3 clusters,
against a `t_crit(2)` of 4.303 — so the anchor channel already fails to clear on
its own native stratum. That is the starting point, not something this run
discovers.

`def` is `defenceMultiplier(fixture.Difficulty) * defenceBandAdj(...)`. Shipped
`band_strength` is 0, so `defenceBandAdj` is exactly 1 and **`def` is a pure
function of the archive's `team_h_difficulty`/`team_a_difficulty`** — confirmed
by the banked column taking exactly the five values 0.70/0.85/1.00/1.20/1.40.
`internal/backtest/season.go` reads one such difficulty per fixture, the
end-of-season value, and `playedFixtures` (`internal/backtest/simulate.go`
:242-260) strips the scoreline and the `Finished` flag but **not** the
difficulty.

**Hypothesis under test.** FPL revises team ratings mid-season in outcome-shaped
waves (`stats/team_strength_revisions.py`: coarse `strength` moves for 6-11 clubs
of 20 in every archived season). If the end-of-season difficulty embeds those
revisions, `def` is partly a *result* rather than a *forecast*, and the bias runs
**upward on the fixture-interaction term**, which is `b2` and not `b1`.

## 2. What is measured, and the one thing that is asserted

**Measured**: the opponent's coarse `strength` at the row's own cutoff, from
`data/captures/<season>/GW*/bootstrap-static.json.gz`, against its end-of-season
value from the archive's `teams.csv` (the same scrape that produced the
`fixtures.csv` supplying `def`). A banked row's `gw` is the gameweek predicted
and the cutoff is `gw - 1`; capture `GW{gw}` is taken before gameweek `gw`'s
deadline, so it is the state the engine had at that cutoff.

**Asserted, and it is a mechanism argument rather than a measurement**: that a
revised `strength` implies a revised `team_h_difficulty`. ⚠️ **The captures hold
`bootstrap-static.json.gz` only and no fixtures payload, so this cannot be
verified from this archive.** Every reading below inherits that. If FPL's
difficulty rank were in fact frozen at season start, `revised_opp` would be a
proxy for "this club's form surprised FPL" and nothing more — which is an
alternative explanation this design cannot exclude.

⚠️ **`revised_opp == 0` is not a clean row.** The fine `strength_*` fields moved
for 20 of 20 clubs in every season. The never-revised subsample below is a
robustness arm and is **not** called clean anywhere.

## 3. Estimator, clustering, df

Binomial GLM with a **log link** on `clean_sheet`, coefficients negated so they
read on `-ln p`, exactly as `stats/cs_calibration.R` does — same family, same
link, same sign convention, so the augmented fit nests the banked one.

**Augmented model (primary):**

```
-ln P(clean sheet) = a + b1*w1 + b2*w2 + b3*w3
    w1 = XGC90
    w2 = XGC90*(def-1)
    w3 = revised_opp * XGC90*(def-1)
```

`b2` is then the defensive fixture channel on rows whose opponent's rating did
**not** move, and `b3` is the extra carried by rows whose did.

**Clustering axis: season.** Native stratum G = 3 → **df = 2**, `t_crit(2) =
4.303`. Pooled stratum G = 6 → df = 5, `t_crit(5) = 2.571`.

Variance: `clubSandwich::vcovCR(..., type = "CR2")`. If CR2 is not computable at
G = 3 with 4 parameters, the run falls back to `sandwich::vcovCL` (CR0) with the
same df and **says so in the output** — it does not silently switch.

The season × team CR2 SE (the banked estimator, 60 clusters) is printed **as
context only**. The verdict is taken on the season-clustered SE at df 2.
⚠️ The record forbids leaning on the pooled six-season stratum for a rejection
while disowning it for a null: **the pooled stratum is reported for both or
neither, and carries no verdict either way.**

**Detection threshold.** This is an archive-side calibration fit and spends no
replay cells, so there is no per-gameweek quantity and the `× 38` conversion in
`AGENTS.md`'s threshold formula does not apply. The analogue in coefficient units
is `t_crit(df) × SE`, and it is reported for every coefficient that carries a
verdict. **No points figure will be quoted from this run.**

## 4. Confirmatory family, and the Holm correction

**Family size m = 2**, both on the native stratum, season-clustered, df 2:

- **H1 (primary)** — `b3 = 0` in the augmented model. Alternative: `b3 > 0`
  (hindsight inflates the fixture interaction), tested two-sided.
- **H2 (robustness)** — `b2 = 1` on the subsample where **neither** club's coarse
  `strength` moved at any point in the season (`either_revised_season == 0`).
  This costs power exactly where power is shortest, which is why it is secondary.

Holm at α = 0.05 with m = 2: the smaller of the two p-values must clear
**0.025**, which at df 2 is `|t| > 6.205`; the larger must then clear 0.05, i.e.
`|t| > 4.303`. Both thresholds are printed beside the estimates.

**Outside the family, declared in advance and reported with uncorrected t:**

- The **signed** specification, `w3s = revised_delta * w1 * (def-1)` in place of
  `w3`. Pre-declared as a sensitivity, not a second bite: it is the same
  mechanism with a directional regressor.
- The augmented model's `b2` (the unrevised rows' channel) as a **descriptive
  decomposition**. It is algebraically tied to `b3` and the banked `b2`, so it is
  not an independent test and gets no Holm slot.
- Everything on the pooled six-season stratum.

## 5. Required control — the run is void without it

The 3-parameter banked model, refitted here and clustered on season, must
reproduce `b2 = 1.5688` with a season-clustered `t(b2 = 1)` of **4.14** to
rounding, and must reproduce the season × team clustered SE of 0.2253. If it does
not, the estimator or the population is wrong and **no result from this run may
be quoted**.

## 6. Liveness — measured before this file was written, and it fired

A guard that cannot fire is not a passed check, so these are stated with the
numbers that were actually obtained:

| check | native (n = 1566) | pooled (n = 2974) |
|---|---|---|
| rows joined | 1566 / 1566 | **2974 / 2974**, none dropped |
| `def` distribution | 0.70:79, 0.85:410, 1.00:675, 1.20:263, 1.40:139 | **0.70:79, 0.85:1059, 1.00:1040, 1.20:593, 1.40:203** |
| `revised_opp` | 308 revised, 1258 not | 588 revised, 2386 not |
| `def` varies within `revised_opp = 1` | all five levels present | all five levels present |
| `def` varies within `revised_opp = 0` | all five levels present | all five levels present |
| `either_revised_season == 0` | 502 rows | 841 rows |
| GW38 capture vs `teams.csv` | 0/20 clubs disagree, all six seasons | — |

The pooled `def` row reproduces the banked distribution **exactly**, which is the
stated liveness bar. `revised_opp` takes both values in every season, with a
per-season revised share of 0.175 to 0.237. The join is therefore live and the
interaction is identified rather than collinear.

`revised_delta` is non-degenerate too: pooled −2:5, −1:365, 0:2386, +1:218.

## 7. Sizing, fixed before the fit — the canary

`q_w`, the share of `sum(w2^2)` carried by revised rows, is a pure design
quantity and was computed with the liveness pass: **0.2876 native**, 0.2372
pooled.

If hindsight explains the *whole* banked excess, then the unrevised rows sit at
`b2 = 1` and the revised rows carry all of it, which requires

```
needed_b3  ≈  (1.5688 - 1) / q_w  =  0.5688 / 0.2876  =  1.98    (native)
```

against `b1` rather than 1, the requirement is `(1.5688 - 1.0476) / 0.2876 =
1.81`.

**Declared now**: if `t_crit(2) × SE(b3) = 4.303 × SE(b3)` exceeds **1.98**, this
comparison could not have seen a full leak even had one been there, and the
verdict is **UNMEASURABLE** rather than null — the record's own "size a candidate
against a canary" rule. This is written down before `SE(b3)` is known.

## 8. Decision rule — every outcome named

Evaluated on the native stratum, season-clustered, df 2, Holm over m = 2.

- **A — HINDSIGHT CARRIES THE EXCESS.** H1 rejects Holm-corrected (`|t(b3)| >
  6.205`) with `b3 > 0`, **and** the augmented `b2` point estimate falls to
  within one of its own standard errors of 1. Verdict: `b2 = 1.5688` is an
  artefact of post-cutoff information and the defensive fixture calibration line
  closes.
- **B — HINDSIGHT DOES NOT CARRY THE EXCESS.** H1 does not reject, **and** the
  augmented `b2` stays within 0.10 of 1.5688 (the leak channel absorbs under ~18%
  of the 0.5688 excess), **and** the §7 canary says a full leak was detectable.
  Verdict: the leak is real but too small to explain `b2`; the line stays open
  and the recorded caveat stands as a caveat rather than a cause.
- **C — UNMEASURABLE.** The §7 canary fires: `4.303 × SE(b3) > 1.98`. Verdict:
  this instrument cannot separate a full leak from none, and no reading of `b3`
  may be quoted as evidence in either direction. Reported as a fact about the
  instrument, not about the constant.
- **D — UNRESOLVED.** Anything else — in particular H1 failing to reject while
  the augmented `b2` moves materially toward 1, which is consistent with both a
  partial leak and none. Verdict: the gate does not close, and the honest report
  is that the question is open.

**A and C are both plausible before the fit and D is the modal expectation**,
given that the anchor coefficient itself reads t 4.14 against a 4.303 bar on this
very stratum. An honest **C** or **D** is a good outcome of this run and will be
reported as such. No verdict will be upgraded by switching stratum, estimator or
clustering axis after the fact.

## 9. What this run does not do

- It runs **no replay sweep** and spends **no cells**. `HOLD` and `POLICY` are
  untouched and no points figure is produced.
- It does not decide whether `FPL_DEF_FIXTURE_SCALE` should move. `AGENTS.md`
  already records the defensive ladder as points-null across a fourfold width
  change, so nothing here reaches the scored path.
- It does not establish that difficulty moved. See §2.
