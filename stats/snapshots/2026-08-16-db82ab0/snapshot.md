# Model and harness accuracy snapshot

Two halves, which must not be blurred together.

**Model accuracy** asks whether the scoring model is right about football. It is measured against outcomes — what players went on to score — and rests on thousands of observations.

**Harness accuracy** asks whether the replay can see anything at all. Its grid is whatever `FPL_SWEEP_SEASONS` selected for the sweep this snapshot reads, and the per-arm thresholds below are the only authority on what it could resolve.

A model can be well calibrated while the harness cannot resolve any change to it. That is this project's actual situation, and reading one half as though it answered the other is how "the instrument could not see it" came to be recorded as "there is no effect" — repeatedly, and in both directions.

## The headline: what this harness can detect at all

**Not measured in this snapshot.** No `mde.csv` was found, so the harness half is absent rather than empty. Nothing below should be read as a statement about resolution.


## Provenance

Every expensive failure in this project's history is a provenance failure rather than an arithmetic one. A whole body of evidence was measured with the transfer gate's minimum-gain threshold at 0.7, the value was retracted to 0.4 three commits later, nothing recorded the link, and a later audit cited the evidence as ground truth. Separately, a six-arm sweep was killed under load after three arms and the gap was invisible until somebody counted rows. So:

| | |
|---|---|
| snapshot taken | 2026-08-16 02:22 EDT |
| commit | `db82ab0deb0c` |
| branch | `xpoints-season-rules-and-unknown-position-guard` |
| cells file | `not supplied` |
| model file | `/tmp/xpsnap-model.csv` |

### Operator notes

Stamped in by hand at snapshot time. These are the caveats a reader would otherwise have to be told separately, which means eventually not be told.

- no cells file at /tmp/cells.csv

**No sweep cells were supplied**, so there is no grid, no arm accounting and no constants fingerprint in this snapshot. The harness half above, if present, came from an inference directory whose provenance is therefore unverified.

## Model accuracy: is the scoring model right about football?

Measured against outcomes rather than against another setting of the model, so these figures do not carry the harness's standard errors: their unit is a player-cutoff or a team-match, not a replayed season, and several rest on thousands of observations where the harness has twenty-four. That makes this the more trustworthy half — and the half that cannot tell you whether acting on a bias would gain points, which is a separate and much harder question this project has answered "no" to five times.

### Does the model's confidence drift through a season?

For each cutoff the model is built from data through that gameweek and every player's predicted points per gameweek is compared with what he actually scored over the next five. Restricted to players the model would consider — 2.0+ predicted points on 45+ expected minutes — because whether it correctly rates a reserve is not the question.

*Population: 6 season pairs, next 5 gameweeks.*

| | n | predicted | actual | ratio | bias | mean absolute error |
|---|---:|---:|---:|---:|---:|---:|
| model built through GW4 | 1048 | 3.026 | 2.73 | 0.902 | -0.296 | 1.374 |
| model built through GW8 | 1072 | 3.04 | 2.866 | 0.943 | -0.174 | 1.31 |
| model built through GW12 | 1063 | 3.057 | 2.842 | 0.93 | -0.214 | 1.374 |
| model built through GW16 | 1053 | 3.052 | 2.75 | 0.901 | -0.302 | 1.451 |
| model built through GW20 | 984 | 3.067 | 3.013 | 0.982 | -0.054 | 1.379 |
| model built through GW24 | 1037 | 3.066 | 2.993 | 0.976 | -0.073 | 1.359 |
| model built through GW28 | 1053 | 3.045 | 2.787 | 0.915 | -0.258 | 1.352 |
| model built through GW32 | 1047 | 3.031 | 3.111 | 1.026 | 0.08 | 1.393 |

**Reading it.** The ratio is actual divided by predicted, so **1.000 is perfect calibration and below 1.000 means the model over-predicts**. Read the predicted and actual columns separately rather than only the ratio: if actual is flat while predicted rises, the model is not getting worse at football, it is getting more confident while reality stays where it was.

### Is the clean sheet priced correctly?

One row per team-match, not per player, since eleven team-mates share a clean sheet and counting them separately would multiply the same observation by eleven.

*Population: one row per team-match, 4 seasons with expected goals.*

| | n | predicted | actual | error | points per match for a defender |
|---|---:|---:|---:|---:|---:|
| clean sheet rate, all team-matches pooled | 2870 | 0.308 | 0.238 | 0.07 | 0.279 |
| expected against actual goals conceded | 2870 | 1.46 | 1.486 | -0.027 | — |

**Reading it.** Error is predicted minus actual, so **positive means the model over-predicts**. The two rows separate the two candidate causes: if expected and actual goals conceded agree, the bias is not in the expected-goals figure but in the Poisson applied to it. ⚠️ **Read the size here as a property of THIS regressor, not of the model.** These rows are cut on realised single-match xGC; the model scores on `XGC90`, a blended and shrunk per-90 rate, and `exp()` is convex, so the two disagree by construction. Refit against `XGC90` itself, predicted/actual is 1.052 native and 1.004 pooled against the ~1.28 here. ⚠️ And the old reading — that a bias shared by every player in a position is not an ordering error — **is contested**: the squad quota fixes how many defenders you own, not how many are fielded, and the optimiser is a knapsack against one budget rather than an ordering consumer — **mechanism, unmeasured on points**. Correcting a measured bias has still lost points five times.

### Is the defensive-contribution term already priced by something else?

Players grouped into thirds by how many defensive actions they record per 90 minutes, within position — because defensive contribution and position are nearly the same variable, so pooling positions would read the model's bias by position as a defcon effect. Reversing that distinction reverses the answer.

*Population: 2025-26, model through GW19, scored GW20-38.*

| | n | defcon actions per 90 | term | bias | bias plus term | bonus per 90 |
|---|---:|---:|---:|---:|---:|---:|
| defenders, lowest third by defcon rate | 29 | 3.667 | 0.015 | 0.334 | 0.35 | 0.198 |
| defenders, middle by defcon rate | 29 | 5.224 | 0.089 | 0.33 | 0.419 | 0.248 |
| defenders, highest third by defcon rate | 31 | 7.833 | 0.54 | 0.069 | 0.61 | 0.223 |
| midfielders, lowest third by defcon rate | 34 | 3.537 | 0.002 | -0.004 | -0.002 | 0.401 |
| midfielders, middle by defcon rate | 34 | 5.723 | 0.034 | 0.442 | 0.477 | 0.303 |
| midfielders, highest third by defcon rate | 35 | 8.288 | 0.299 | 0.742 | 1.042 | 0.301 |

**Reading it.** Bias is actual minus predicted points per 90, so **negative means over-prediction**. If the term were fully earned, bias would be flat across the three groups. If it were entirely redundant, bias would fall by exactly the term's growth and 'bias plus term' would be flat instead — which is the redundancy signature to look for.

### How much should a predictor weight recent gameweeks?

Six ways of summarising a player's record so far, each predicting his mean over the next five gameweeks. No model is built: every predictor is arithmetic on the archive, which makes this a clean test of the recency question rather than of the model that consumes the answer. A half-life of h means a gameweek h gameweeks back counts half as much as the most recent one, so smaller means sharper recency.

*Population: 6 seasons, cutoffs GW5-33, next 5 gameweeks.*

| | n | mean absolute error | root-mean-square error | bias (predicted minus actual) |
|---|---:|---:|---:|---:|
| minutes — season to date (flat) | 55610 | 23.716 | 30.906 | -1.436 |
| minutes — last 3 gameweeks (flat) | 55610 | 23.871 | 32.025 | 5.306 |
| minutes — ewma, half-life 2 | 55610 | 22.062 | 28.915 | 4.261 |
| minutes — ewma, half-life 4 | 55610 | 22.119 | 28.695 | 2.498 |
| minutes — ewma, half-life 8 | 55610 | 22.599 | 29.296 | 0.76 |
| minutes — ewma, half-life 20 | 55610 | 23.183 | 30.126 | -0.549 |
| points — season to date (flat) | 55610 | 1.362 | 1.799 | -0.055 |
| points — last 3 gameweeks (flat) | 55610 | 1.67 | 2.249 | 0.202 |
| points — ewma, half-life 2 | 55610 | 1.466 | 1.946 | 0.164 |
| points — ewma, half-life 4 | 55610 | 1.378 | 1.808 | 0.098 |
| points — ewma, half-life 8 | 55610 | 1.35 | 1.774 | 0.031 |
| points — ewma, half-life 20 | 55610 | 1.351 | 1.78 | -0.02 |
| expected goals + assists — season to date (flat) | 55610 | 0.086 | 0.135 | -0.004 |
| expected goals + assists — last 3 gameweeks (flat) | 55610 | 0.106 | 0.17 | 0.009 |
| expected goals + assists — ewma, half-life 2 | 55610 | 0.093 | 0.147 | 0.007 |
| expected goals + assists — ewma, half-life 4 | 55610 | 0.087 | 0.136 | 0.004 |
| expected goals + assists — ewma, half-life 8 | 55610 | 0.085 | 0.133 | 0 |
| expected goals + assists — ewma, half-life 20 | 55610 | 0.085 | 0.133 | -0.003 |

**Reading it.** Mean absolute error in the target's own units, **lower is better**. The shape to look for, not the level: **minutes reward sharp recency and rates punish it.** Minutes are a statement about the present, so weighting recent gameweeks removes a bias — a player who lost his place six weeks ago is not a starter. Rates are a statement about quality, and a short window chases finishing variance, so the same weighting trades bias for variance. That is the evidence two shipped constants rest on, and the diagnostic fails if either half stops holding.

### How wrong is the model about one player in one gameweek?

Out-of-sample error predicting a single gameweek from a model built through the gameweek before, split by what the player ACTUALLY scored: Zeros recorded no minutes, Blanks played for two points or fewer, Tickers scored three or four, Haulers five or more. The categories are OpenFPL's (arXiv 2508.09992) so the figures sit beside published ones. Two naive baselines are shown for scale: the mean of the last five gameweeks, which is OpenFPL's own baseline, and the flat season average, which is what FPL's bootstrap publishes.

*Population: 6 seasons, gameweeks 6-38, one gameweek ahead.*

| | n | mean absolute error | root-mean-square error | bias (predicted minus actual) | error sd |
|---|---:|---:|---:|---:|---:|
| points — model — Zeros | 14189 | 1.523 | 1.842 | 1.523 | 1.036 |
| points — model — Blanks | 29259 | 1.197 | 1.56 | 1.021 | 1.18 |
| points — model — Tickers | 5321 | 1.15 | 1.495 | -0.418 | 1.435 |
| points — model — Haulers | 11785 | 4.806 | 5.624 | -4.734 | 3.036 |
| points — model — all categories | 60554 | 1.972 | 2.885 | -0.108 | 2.883 |
| points — naive: mean of last 5 gameweeks — Zeros | 14189 | 1.738 | 2.241 | 1.734 | 1.42 |
| points — naive: mean of last 5 gameweeks — Blanks | 29259 | 1.599 | 2.159 | 1.327 | 1.703 |
| points — naive: mean of last 5 gameweeks — Tickers | 5321 | 1.462 | 1.844 | -0.237 | 1.829 |
| points — naive: mean of last 5 gameweeks — Haulers | 11785 | 4.724 | 5.663 | -4.565 | 3.351 |
| points — naive: mean of last 5 gameweeks — all categories | 60554 | 2.228 | 3.158 | 0.138 | 3.155 |
| points — naive: mean of season to date — Zeros | 14189 | 1.725 | 2.106 | 1.724 | 1.21 |
| points — naive: mean of season to date — Blanks | 29259 | 1.305 | 1.718 | 0.991 | 1.404 |
| points — naive: mean of season to date — Tickers | 5321 | 1.249 | 1.555 | -0.623 | 1.425 |
| points — naive: mean of season to date — Haulers | 11785 | 5 | 5.87 | -4.949 | 3.156 |
| points — naive: mean of season to date — all categories | 60554 | 2.117 | 3.064 | -0.135 | 3.061 |
| minutes — model — Zeros | 14189 | 40.356 | 45.837 | 40.356 | 21.735 |
| minutes — model — Blanks | 29259 | 23.636 | 30.104 | -9.2 | 28.664 |
| minutes — model — Tickers | 5321 | 24.379 | 31.868 | -19.883 | 24.904 |
| minutes — model — Haulers | 11785 | 24.061 | 31.713 | -21.126 | 23.652 |
| minutes — model — all categories | 60554 | 27.702 | 34.86 | -0.848 | 34.849 |
| minutes — naive: mean of last 5 gameweeks — Zeros | 14189 | 45.422 | 51.481 | 45.422 | 24.231 |
| minutes — naive: mean of last 5 gameweeks — Blanks | 29259 | 22.727 | 31.124 | -3.47 | 30.93 |
| minutes — naive: mean of last 5 gameweeks — Tickers | 5321 | 26.324 | 39.351 | -19.653 | 34.092 |
| minutes — naive: mean of last 5 gameweeks — Haulers | 11785 | 25.947 | 39.33 | -20.15 | 33.776 |
| minutes — naive: mean of last 5 gameweeks — all categories | 60554 | 28.988 | 39.066 | 3.318 | 38.925 |
| minutes — naive: mean of season to date — Zeros | 14189 | 43.395 | 49.48 | 43.395 | 23.774 |
| minutes — naive: mean of season to date — Blanks | 29259 | 25.949 | 33.83 | -12.236 | 31.54 |
| minutes — naive: mean of season to date — Tickers | 5321 | 31.559 | 44.473 | -27.99 | 34.561 |
| minutes — naive: mean of season to date — Haulers | 11785 | 31.287 | 44.756 | -28.875 | 34.195 |
| minutes — naive: mean of season to date — all categories | 60554 | 31.569 | 41.113 | -3.823 | 40.935 |
| expected goals + assists — model — Zeros | 14189 | 0.086 | 0.133 | 0.086 | 0.102 |
| expected goals + assists — model — Blanks | 29259 | 0.113 | 0.175 | 0.029 | 0.172 |
| expected goals + assists — model — Tickers | 5321 | 0.134 | 0.211 | -0.011 | 0.21 |
| expected goals + assists — model — Haulers | 11785 | 0.235 | 0.387 | -0.166 | 0.349 |
| expected goals + assists — model — all categories | 60554 | 0.132 | 0.228 | 0.001 | 0.228 |
| expected goals + assists — naive: mean of last 5 gameweeks — Zeros | 14189 | 0.089 | 0.156 | 0.089 | 0.128 |
| expected goals + assists — naive: mean of last 5 gameweeks — Blanks | 29259 | 0.124 | 0.195 | 0.037 | 0.191 |
| expected goals + assists — naive: mean of last 5 gameweeks — Tickers | 5321 | 0.145 | 0.227 | -0.01 | 0.226 |
| expected goals + assists — naive: mean of last 5 gameweeks — Haulers | 11785 | 0.246 | 0.399 | -0.162 | 0.364 |
| expected goals + assists — naive: mean of last 5 gameweeks — all categories | 60554 | 0.141 | 0.244 | 0.006 | 0.244 |
| expected goals + assists — naive: mean of season to date — Zeros | 14189 | 0.089 | 0.146 | 0.089 | 0.116 |
| expected goals + assists — naive: mean of season to date — Blanks | 29259 | 0.111 | 0.176 | 0.019 | 0.175 |
| expected goals + assists — naive: mean of season to date — Tickers | 5321 | 0.133 | 0.215 | -0.032 | 0.212 |
| expected goals + assists — naive: mean of season to date — Haulers | 11785 | 0.245 | 0.403 | -0.184 | 0.358 |
| expected goals + assists — naive: mean of season to date — all categories | 60554 | 0.134 | 0.236 | -0.009 | 0.235 |

**Reading it.** Mean absolute error and root-mean-square error are both in the target's own units and **lower is better**. Bias is predicted minus actual, so positive means over-prediction, and error sd is the spread around that bias — root-mean-square error squared is exactly bias squared plus error sd squared. **The categories condition on the outcome, which rewards a noisier predictor in the extreme buckets**: a predictor that fires more high numbers will look better on Haulers while being worse calibrated at the top of its own distribution, so read the Haulers column beside the calibration and ordering tables and never on its own. This instrument ranks candidates and cannot price them — the replay does that, and a better predictor can make a worse policy.

### Do the players the model rates at 5.0 score 5.0?

The same one-gameweek-ahead predictions grouped by what was PREDICTED rather than by what happened, so the table reads at the level decisions are made at.

*Population: 6 seasons, gameweeks 6-38, one gameweek ahead.*

| | n | predicted | actual | ratio |
|---|---:|---:|---:|---:|
| predicted under 1.0 | 8469 | 0.643 | 0.835 | 1.298 |
| predicted 1.0 to 2.0 | 17424 | 1.513 | 1.698 | 1.123 |
| predicted 2.0 to 3.0 | 18584 | 2.488 | 2.581 | 1.037 |
| predicted 3.0 to 4.0 | 11111 | 3.417 | 3.501 | 1.025 |
| predicted 4.0 to 5.0 | 2783 | 4.401 | 4.238 | 0.963 |
| predicted 5.0 to 6.0 | 1058 | 5.432 | 5.237 | 0.964 |
| predicted 6.0 and above | 1125 | 7.367 | 7.092 | 0.963 |

**Reading it.** The ratio is actual divided by predicted, so **1.000 is perfect and below 1.000 means the band is over-predicted**. The top band is where a transfer search picks, so its ratio matters more than the aggregate: a bias shared by every player is invisible to an argmax, and this project has measured that correcting one costs points.

### Is a candidate change safe for an argmax, or a variance trade?

Each arm of the benchmark paired against the shipped config on the same observations. Two of the arms are controls rather than proposals: switching minutes recency off must make the minutes error worse, and switching the vice-captain fallback off must change nothing at all, since it alters how a played-out gameweek is scored and not what the model predicts.

*Population: 6 seasons, gameweeks 6-38, one gameweek ahead.*

| | n | change in mean absolute error | change in root-mean-square error | change in absolute bias | change in error sd | change in signed error over the top predicted | change in rank correlation |
|---|---:|---:|---:|---:|---:|---:|---:|
| CONTROL, directional: minutes recency off — points | 60554 | 0.041 | 0.078 | 0.115 | 0.072 | — | — |
| CONTROL, directional: minutes recency off — minutes | 60554 | 3.903 | 4.236 | 3.146 | 4.042 | — | — |
| CONTROL, directional: minutes recency off — expected goals + assists | 60554 | 0.001 | 0.005 | 0.005 | 0.004 | — | — |
| CONTROL, directional: minutes recency off — tail and ordering | 0 | — | — | — | — | 0.04 | -0.088 |
| CONTROL, invariance: vice-captain fallback off — points | 60554 | 0 | 0 | 0 | 0 | — | — |
| CONTROL, invariance: vice-captain fallback off — minutes | 60554 | 0 | 0 | 0 | 0 | — | — |
| CONTROL, invariance: vice-captain fallback off — expected goals + assists | 60554 | 0 | 0 | 0 | 0 | — | — |
| CONTROL, invariance: vice-captain fallback off — tail and ordering | 0 | — | — | — | — | 0 | 0 |
| CANDIDATE: two estimators of P(appears), as before the unification — points | 60554 | 0 | 0 | 0.001 | 0 | — | — |
| CANDIDATE: two estimators of P(appears), as before the unification — minutes | 60554 | 0 | 0 | 0 | 0 | — | — |
| CANDIDATE: two estimators of P(appears), as before the unification — expected goals + assists | 60554 | 0 | 0 | 0 | 0 | — | — |
| CANDIDATE: two estimators of P(appears), as before the unification — tail and ordering | 0 | — | — | — | — | -0.003 | 0 |
| CANDIDATE: appearance constants refit on the windowed population — points | 60554 | -0.018 | 0.007 | 0.078 | 0.003 | — | — |
| CANDIDATE: appearance constants refit on the windowed population — minutes | 60554 | 0 | 0 | 0 | 0 | — | — |
| CANDIDATE: appearance constants refit on the windowed population — expected goals + assists | 60554 | 0 | 0 | 0 | 0 | — | — |
| CANDIDATE: appearance constants refit on the windowed population — tail and ordering | 0 | — | — | — | — | -0.164 | -0.003 |
| CANDIDATE: appearance constants refit against ExpectedMinutes — points | 60554 | -0.008 | 0 | 0.028 | -0.001 | — | — |
| CANDIDATE: appearance constants refit against ExpectedMinutes — minutes | 60554 | 0 | 0 | 0 | 0 | — | — |
| CANDIDATE: appearance constants refit against ExpectedMinutes — expected goals + assists | 60554 | 0 | 0 | 0 | 0 | — | — |
| CANDIDATE: appearance constants refit against ExpectedMinutes — tail and ordering | 0 | — | — | — | — | -0.084 | 0 |
| CANDIDATE: the sixty-minute curve alone, refit against ExpectedMinutes — points | 60554 | -0.008 | 0 | 0.027 | -0.001 | — | — |
| CANDIDATE: the sixty-minute curve alone, refit against ExpectedMinutes — minutes | 60554 | 0 | 0 | 0 | 0 | — | — |
| CANDIDATE: the sixty-minute curve alone, refit against ExpectedMinutes — expected goals + assists | 60554 | 0 | 0 | 0 | 0 | — | — |
| CANDIDATE: the sixty-minute curve alone, refit against ExpectedMinutes — tail and ordering | 0 | — | — | — | — | -0.081 | 0 |
| CANDIDATE: rate recency, half-life 8 — points | 60554 | 0 | -0.001 | -0.001 | -0.001 | — | — |
| CANDIDATE: rate recency, half-life 8 — minutes | 60554 | 0 | 0 | 0 | 0 | — | — |
| CANDIDATE: rate recency, half-life 8 — expected goals + assists | 60554 | 0 | 0 | 0 | 0 | — | — |
| CANDIDATE: rate recency, half-life 8 — tail and ordering | 0 | — | — | — | — | 0.038 | 0.001 |

**Reading it.** These are differences, so **negative means better** for the error columns. The distinction that matters is whether an improvement came from shrinking the systematic part of the error (bias reduction, safe for an argmax, because removing a systematic error cannot reorder candidates by chance) or from shrinking the spread while the bias grew (a bias-for-variance trade, dangerous — the recorded reason recency on minutes gained points and recency on rates lost them). Read the tail and ordering rows beside the error rows: a candidate that lowers aggregate error while pushing the tail figure away from zero has the better-predictor-worse-policy shape.

### Did every season reach the prediction benchmark's sample?

How many gameweeks and how many player-gameweeks each season contributed to the benchmark's headline population — players who played sixty or more minutes in one of the previous five gameweeks their club played.

*Population: 6 seasons, gameweeks 6-38, one gameweek ahead.*

| | n | gameweeks contributing observations | observations in the headline population | observations per gameweek |
|---|---:|---:|---:|---:|
| 2020-21 | 10096 | 33 | 10096 | 305.939 |
| 2021-22 | 9847 | 33 | 9847 | 298.394 |
| 2022-23 | 9865 | 32 | 9865 | 308.281 |
| 2023-24 | 10205 | 33 | 10205 | 309.242 |
| 2024-25 | 10286 | 33 | 10286 | 311.697 |
| 2025-26 | 10255 | 33 | 10255 | 310.758 |

**Reading it.** **Higher is better and even across seasons is what matters** — the seasons should contribute roughly equally, and a season contributing nothing means the population filter is reading an archive column that season does not carry. That is not a hypothetical: the per-gameweek `starts` field is empty for all of 2021-22 and for 2022-23 before GW16, and a filter reading it silently made a four-season figure into a three-season one while every other table stayed plausible. One gameweek missing from 2022-23 is expected — its GW7 was postponed outright.

### Does the model rank players correctly, and is its top over-rated?

Spearman's rank correlation — the ordinary correlation computed on ranks rather than values — between predicted and actual points within each gameweek, averaged over gameweeks. Beside it, the signed error over the twenty highest-predicted players in each gameweek, which is roughly the set a transfer search chooses between.

*Population: 6 seasons, gameweeks 6-38, one gameweek ahead.*

| | n | mean within-gameweek rank correlation | signed error over the top 20 predicted |
|---|---:|---:|---:|
| model | 197 | 0.438 | 0.238 |
| naive: mean of last 5 gameweeks | 197 | 0.335 | 2.608 |
| naive: mean of season to date | 197 | 0.314 | 1.082 |

**Reading it.** For the rank correlation, +1 is a perfect ordering, 0 is no ordering information and **higher is better**. This axis exists because the optimiser consumes an ordering and never a level, which is why the bonus term is kept despite being badly calibrated. For the tail figure, **positive means the top of the predicted distribution is over-rated** and closer to zero is better — it is the winner's curse as a measured number rather than an inference.

### Are the terms FPL pays as a step scaled as a step?

Appearance points and the clean sheet are paid at sixty minutes, not prorated: a starter taken off at seventy banks both in full. The model scales them by an estimated probability of reaching sixty minutes, and this compares that estimate against how often players in each minutes band actually reached it.

*Population: 2934 player-seasons, 4 seasons, 20+ gameweeks.*

| | n | model credits | actually reached 60 minutes | error | the superseded minutes-reliability proxy credited |
|---|---:|---:|---:|---:|---:|
| players averaging 0-10 minutes a gameweek | 1323 | 0.013 | 0.007 | 0.006 | 0.006 |
| players averaging 10-20 minutes a gameweek | 214 | 0.103 | 0.123 | -0.02 | 0.103 |
| players averaging 20-30 minutes a gameweek | 230 | 0.185 | 0.227 | -0.043 | 0.202 |
| players averaging 30-40 minutes a gameweek | 210 | 0.3 | 0.335 | -0.034 | 0.306 |
| players averaging 40-50 minutes a gameweek | 217 | 0.454 | 0.461 | -0.007 | 0.422 |
| players averaging 50-60 minutes a gameweek | 193 | 0.614 | 0.576 | 0.038 | 0.543 |
| players averaging 60-70 minutes a gameweek | 169 | 0.75 | 0.7 | 0.05 | 0.667 |
| players averaging 70-80 minutes a gameweek | 185 | 0.851 | 0.809 | 0.042 | 0.796 |
| players averaging 80-91 minutes a gameweek | 156 | 0.927 | 0.921 | 0.006 | 0.935 |

**Reading it.** Error is what the model credits minus what happens, so **positive means the model over-credits**. Unlike the clean-sheet bias this one is *not* uniform across players, so it mis-ranks a part-timer against an ever-present — an ordering error, which the optimiser does see. Read it as a **shape**: the error crosses zero near fifty minutes, under-crediting the fringe band and over-crediting rotation players, so it is the slope between those two groups that is wrong rather than the level of either. ⚠️ Figures for this section in snapshots before `2026-08-10-27740ba` and every earlier snapshot measured the minutes-reliability proxy `playsSixty` REPLACED, not the shipped curve — the diagnostic kept its own copy and labelled it "model now". Their error turns positive only in the top band, where the shipped curve's crosses near fifty, so the two disagree about WHERE the model over-credits and not merely by how much. Do not read a trend across that boundary.

### How much of this season should a team rating believe?

For each club and each cutoff, predict the *remainder* of the season's goals from the record so far, from last season, and from a blend of the two at each prior strength. Scored out of sample, which is the one predictor comparison this project can currently run.

*Population: 3 season pairs, predicting the rest of the season.*

| | n | this season only (error) | last season only (error) | best blend of the two (error) | prior strength that blend used, in matches |
|---|---:|---:|---:|---:|---:|
| goals conceded, after 3 matches played | 60 | 0.711 | 0.32 | 0.32 | 70 |
| goals conceded, after 5 matches played | 60 | 0.61 | 0.324 | 0.324 | 45 |
| goals conceded, after 8 matches played | 60 | 0.475 | 0.323 | 0.317 | 30 |
| goals conceded, after 11 matches played | 60 | 0.461 | 0.346 | 0.342 | 30 |
| goals conceded, after 15 matches played | 60 | 0.418 | 0.381 | 0.371 | 20 |
| goals conceded, after 20 matches played | 60 | 0.439 | 0.405 | 0.396 | 30 |
| goals conceded, after 25 matches played | 60 | 0.442 | 0.444 | 0.426 | 15 |
| goals scored, after 3 matches played | 60 | 0.62 | 0.36 | 0.354 | 20 |
| goals scored, after 5 matches played | 60 | 0.555 | 0.378 | 0.375 | 30 |
| goals scored, after 8 matches played | 60 | 0.514 | 0.388 | 0.382 | 30 |
| goals scored, after 11 matches played | 60 | 0.423 | 0.376 | 0.351 | 15 |
| goals scored, after 15 matches played | 60 | 0.406 | 0.397 | 0.367 | 15 |
| goals scored, after 20 matches played | 60 | 0.411 | 0.429 | 0.393 | 10 |
| goals scored, after 25 matches played | 60 | 0.445 | 0.485 | 0.441 | 5 |

**Reading it.** Root-mean-square error predicting the rest of the season, so **lower is better**. Compare within a row and never down a column: the absolute errors rise at later cutoffs because a shorter remainder is a noisier thing to predict, not because the predictor got worse. The prior strength is in matches — a strength of k gives this season a weight of n/(n+k) after n matches.

### Is the model more wrong about a player it buys than one it sells?

Every transfer the replay made, judged against what the two players went on to do over the window the decision was justified on. Error is realised minus modelled, per gameweek.

*Population: 480 moves judged, 6 seasons x 3 start points, min_gain 0.40.*

| | n | median | mean |
|---|---:|---:|---:|
| buy side (player bought) | 480 | -0.258 | -0.052 |
| sell side (player sold) | 480 | -0.305 | -0.244 |
| asymmetry (buy minus sell) | 480 | 0.047 | 0.191 |
| sell error: sold player played again | 413 | -0.153 | — |
| sell error: sold player never played again | 67 | -2.474 | — |

**Reading it.** **Negative means the player was over-rated.** The asymmetry row is the one that matters: a model that is simply badly calibrated moves both sides together, while a search that hunts the top of a noisy estimate distribution moves the buy side further. Note the move count in the population line — the transfer gate decides which moves exist to judge, so two runs at different gate settings measure different populations and are not a reproduction of each other.

### What is not measured here, and why

**No section here prices a change in points.** Every figure above is measured against outcomes — what the players went on to do — and the unit is a player-gameweek or a team-match rather than a replayed season. That is what makes these the more trustworthy half: several rest on tens of thousands of observations where the harness half has twenty-four cells. It is also what they cannot do. A predictor that wins here is a candidate worth spending replay time on, not a candidate already proved, and this project has a measured case where about 2 per cent lower out-of-sample error cost roughly 49 points a season: a transfer policy is an argmax living in the tail of the estimate distribution, so accuracy bought on the average player is paid for with noise where the search looks.

The predictor comparison for minutes, points and expected goals **is now runnable** and appears above as "How much should a predictor weight recent gameweeks?". An earlier edition of this snapshot had to record its absence: the figures were frozen prose in `internal/analysis` and `internal/recent` with no code that could recompute them, which is precisely the orphaned measurement this artefact exists to prevent. Read the relative columns rather than the levels when comparing against those comments — the population behind the original was never written down, and it predates the doubles-counting fix that changed what a gameweek means in this archive.

## Change since the previous snapshot

Compared against `2026-08-16-b93179d`.

### Figures that moved

| figure | previous | now | change |
|---|---:|---:|---:|
| `stamp.commit` | b93179d2770a | db82ab0deb0c | — |

**Attributing a movement.** Check the constants fingerprint rows first. A figure that moved while the fingerprint held means the code changed and no setting did — a scoring fix, a harness fix, or a bug. A figure that moved *with* the fingerprint means a setting changed, and the constants diff below names which.

## Regenerating this

See `stats/README.md` for the full recipe and the runtimes. In short: run a sweep with `FPL_CELLS` set, which writes its own provenance; run the calibration diagnostics with `FPL_MODEL_CSV` set; then `fplagent snapshot`, which invokes the R inference and renders this file.

**Constants in force at the sweeps above** are recorded in the provenance sidecar beside the cells file, not inlined here — there are over a hundred of them and the fingerprint is what a reader needs. `fplagent snapshot --constants` prints the full list for a fingerprint.
