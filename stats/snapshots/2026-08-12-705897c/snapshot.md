# Model and harness accuracy snapshot

Two halves, which must not be blurred together.

**Model accuracy** asks whether the scoring model is right about football. It is measured against outcomes — what players went on to score — and rests on thousands of observations.

**Harness accuracy** asks whether the replay can see anything at all. It rests on four seasons, which is all the archive holds and all it ever will: expected goals begin in 2022-23.

A model can be well calibrated while the harness cannot resolve any change to it. That is this project's actual situation, and reading one half as though it answered the other is how "the instrument could not see it" came to be recorded as "there is no effect" — repeatedly, and in both directions.

## The headline: what this harness can detect at all

**Not measured in this snapshot.** No `mde.csv` was found, so the harness half is absent rather than empty. Nothing below should be read as a statement about resolution.


## Provenance

Every expensive failure in this project's history is a provenance failure rather than an arithmetic one. A whole body of evidence was measured with the transfer gate's minimum-gain threshold at 0.7, the value was retracted to 0.4 three commits later, nothing recorded the link, and a later audit cited the evidence as ground truth. Separately, a six-arm sweep was killed under load after three arms and the gap was invisible until somebody counted rows. So:

| | |
|---|---|
| snapshot taken | 2026-08-12 01:03 EDT |
| commit | `705897c2416b` |
| branch | `flag-refit` |
| cells file | `not supplied` |
| model file | `/home/bbowman.guest/.claude/jobs/5ac0c3b1/tmp/model.csv` |

**No sweep cells were supplied**, so there is no grid, no arm accounting and no constants fingerprint in this snapshot. The harness half above, if present, came from an inference directory whose provenance is therefore unverified.

## Model accuracy: is the scoring model right about football?

Measured against outcomes rather than against another setting of the model, so these figures do not carry the harness's standard errors: their unit is a player-cutoff or a team-match, not a replayed season, and several rest on thousands of observations where the harness has twenty-four. That makes this the more trustworthy half — and the half that cannot tell you whether acting on a bias would gain points, which is a separate and much harder question this project has answered "no" to five times.

### Does the model's confidence drift through a season?

For each cutoff the model is built from data through that gameweek and every player's predicted points per gameweek is compared with what he actually scored over the next five. Restricted to players the model would consider — 2.0+ predicted points on 45+ expected minutes — because whether it correctly rates a reserve is not the question.

*Population: 6 season pairs, next 5 gameweeks.*

| | n | predicted | actual | ratio | bias | mean absolute error |
|---|---:|---:|---:|---:|---:|---:|
| model built through GW4 | 948 | 2.928 | 2.797 | 0.955 | -0.131 | 1.338 |
| model built through GW8 | 971 | 2.925 | 2.925 | 1 | 0.001 | 1.319 |
| model built through GW12 | 967 | 2.954 | 2.904 | 0.983 | -0.051 | 1.379 |
| model built through GW16 | 991 | 3.258 | 2.806 | 0.861 | -0.451 | 1.549 |
| model built through GW20 | 968 | 3.155 | 3.006 | 0.953 | -0.149 | 1.467 |
| model built through GW24 | 993 | 3.125 | 2.996 | 0.959 | -0.129 | 1.423 |
| model built through GW28 | 1010 | 3.071 | 2.831 | 0.922 | -0.24 | 1.384 |
| model built through GW32 | 998 | 3.043 | 3.145 | 1.034 | 0.102 | 1.428 |

**Reading it.** The ratio is actual divided by predicted, so **1.000 is perfect calibration and below 1.000 means the model over-predicts**. Read the predicted and actual columns separately rather than only the ratio: if actual is flat while predicted rises, the model is not getting worse at football, it is getting more confident while reality stays where it was.

### Is the clean sheet priced correctly?

One row per team-match, not per player, since eleven team-mates share a clean sheet and counting them separately would multiply the same observation by eleven.

*Population: one row per team-match, 4 seasons with expected goals.*

| | n | predicted | actual | error | points per match for a defender |
|---|---:|---:|---:|---:|---:|
| clean sheet rate, all team-matches pooled | 2683 | 0.3 | 0.24 | 0.06 | 0.238 |
| expected against actual goals conceded | 2683 | 1.506 | 1.527 | -0.021 | — |

**Reading it.** Error is predicted minus actual, so **positive means the model over-predicts**. The two rows separate the two candidate causes: if expected and actual goals conceded agree, the bias is not in the expected-goals figure but in the Poisson applied to it. A bias shared by every player in a position is not an ordering error, and this project has measured that correcting one costs points.

### Is the defensive-contribution term already priced by something else?

Players grouped into thirds by how many defensive actions they record per 90 minutes, within position — because defensive contribution and position are nearly the same variable, so pooling positions would read the model's bias by position as a defcon effect. Reversing that distinction reverses the answer.

*Population: 2025-26, model through GW19, scored GW20-38.*

| | n | defcon actions per 90 | term | bias | bias plus term | bonus per 90 |
|---|---:|---:|---:|---:|---:|---:|
| defenders, lowest third by defcon rate | 29 | 5.669 | 0.153 | 0.234 | 0.388 | 0.207 |
| defenders, middle by defcon rate | 29 | 7.557 | 0.467 | 0.213 | 0.679 | 0.206 |
| defenders, highest third by defcon rate | 31 | 9.946 | 1.055 | -0.311 | 0.745 | 0.278 |
| midfielders, lowest third by defcon rate | 34 | 5.752 | 0.048 | 0.168 | 0.216 | 0.374 |
| midfielders, middle by defcon rate | 34 | 8.418 | 0.301 | 0.099 | 0.4 | 0.33 |
| midfielders, highest third by defcon rate | 35 | 11.307 | 0.908 | 0.061 | 0.969 | 0.287 |
| forwards, lowest third by defcon rate | 8 | 3.178 | 0 | 0.584 | 0.584 | 0.577 |
| forwards, middle by defcon rate | 8 | 4.54 | 0.006 | 1.02 | 1.026 | 0.592 |
| forwards, highest third by defcon rate | 8 | 5.76 | 0.038 | 1.236 | 1.274 | 0.612 |

**Reading it.** Bias is actual minus predicted points per 90, so **negative means over-prediction**. If the term were fully earned, bias would be flat across the three groups. If it were entirely redundant, bias would fall by exactly the term's growth and 'bias plus term' would be flat instead — which is the redundancy signature to look for.

### How much should a predictor weight recent gameweeks?

Six ways of summarising a player's record so far, each predicting his mean over the next five gameweeks. No model is built: every predictor is arithmetic on the archive, which makes this a clean test of the recency question rather than of the model that consumes the answer. A half-life of h means a gameweek h gameweeks back counts half as much as the most recent one, so smaller means sharper recency.

*Population: 6 seasons, cutoffs GW5-33, next 5 gameweeks.*

| | n | mean absolute error | root-mean-square error | bias (predicted minus actual) |
|---|---:|---:|---:|---:|
| minutes — season to date (flat) | 55610 | 23.714 | 30.904 | -1.433 |
| minutes — last 3 gameweeks (flat) | 55610 | 23.873 | 32.027 | 5.308 |
| minutes — ewma, half-life 2 | 55610 | 22.064 | 28.916 | 4.263 |
| minutes — ewma, half-life 4 | 55610 | 22.12 | 28.695 | 2.5 |
| minutes — ewma, half-life 8 | 55610 | 22.599 | 29.296 | 0.763 |
| minutes — ewma, half-life 20 | 55610 | 23.182 | 30.125 | -0.545 |
| points — season to date (flat) | 55610 | 1.362 | 1.799 | -0.054 |
| points — last 3 gameweeks (flat) | 55610 | 1.671 | 2.25 | 0.202 |
| points — ewma, half-life 2 | 55610 | 1.467 | 1.947 | 0.164 |
| points — ewma, half-life 4 | 55610 | 1.378 | 1.808 | 0.099 |
| points — ewma, half-life 8 | 55610 | 1.351 | 1.774 | 0.031 |
| points — ewma, half-life 20 | 55610 | 1.351 | 1.781 | -0.02 |
| expected goals + assists — season to date (flat) | 55610 | 0.086 | 0.135 | -0.004 |
| expected goals + assists — last 3 gameweeks (flat) | 55610 | 0.106 | 0.171 | 0.009 |
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
| points — model — Zeros | 14189 | 1.489 | 1.811 | 1.489 | 1.031 |
| points — model — Blanks | 29259 | 1.177 | 1.571 | 0.988 | 1.221 |
| points — model — Tickers | 5321 | 1.185 | 1.548 | -0.459 | 1.478 |
| points — model — Haulers | 11785 | 4.895 | 5.713 | -4.822 | 3.064 |
| points — model — all categories | 60554 | 1.975 | 2.919 | -0.153 | 2.915 |
| points — naive: mean of last 5 gameweeks — Zeros | 14189 | 1.738 | 2.241 | 1.734 | 1.42 |
| points — naive: mean of last 5 gameweeks — Blanks | 29259 | 1.6 | 2.161 | 1.328 | 1.705 |
| points — naive: mean of last 5 gameweeks — Tickers | 5321 | 1.462 | 1.844 | -0.237 | 1.829 |
| points — naive: mean of last 5 gameweeks — Haulers | 11785 | 4.724 | 5.664 | -4.566 | 3.351 |
| points — naive: mean of last 5 gameweeks — all categories | 60554 | 2.228 | 3.158 | 0.138 | 3.155 |
| points — naive: mean of season to date — Zeros | 14189 | 1.725 | 2.107 | 1.724 | 1.21 |
| points — naive: mean of season to date — Blanks | 29259 | 1.306 | 1.719 | 0.992 | 1.404 |
| points — naive: mean of season to date — Tickers | 5321 | 1.249 | 1.555 | -0.623 | 1.425 |
| points — naive: mean of season to date — Haulers | 11785 | 5 | 5.87 | -4.949 | 3.157 |
| points — naive: mean of season to date — all categories | 60554 | 2.118 | 3.064 | -0.134 | 3.061 |
| minutes — model — Zeros | 14189 | 40.356 | 45.837 | 40.356 | 21.735 |
| minutes — model — Blanks | 29259 | 23.636 | 30.104 | -9.201 | 28.664 |
| minutes — model — Tickers | 5321 | 24.379 | 31.868 | -19.883 | 24.904 |
| minutes — model — Haulers | 11785 | 24.067 | 31.729 | -21.133 | 23.667 |
| minutes — model — all categories | 60554 | 27.703 | 34.862 | -0.85 | 34.852 |
| minutes — naive: mean of last 5 gameweeks — Zeros | 14189 | 45.422 | 51.481 | 45.422 | 24.231 |
| minutes — naive: mean of last 5 gameweeks — Blanks | 29259 | 22.727 | 31.126 | -3.465 | 30.932 |
| minutes — naive: mean of last 5 gameweeks — Tickers | 5321 | 26.324 | 39.351 | -19.653 | 34.092 |
| minutes — naive: mean of last 5 gameweeks — Haulers | 11785 | 25.951 | 39.338 | -20.154 | 33.783 |
| minutes — naive: mean of last 5 gameweeks — all categories | 60554 | 28.988 | 39.069 | 3.319 | 38.928 |
| minutes — naive: mean of season to date — Zeros | 14189 | 43.396 | 49.481 | 43.396 | 23.773 |
| minutes — naive: mean of season to date — Blanks | 29259 | 25.947 | 33.827 | -12.231 | 31.538 |
| minutes — naive: mean of season to date — Tickers | 5321 | 31.559 | 44.473 | -27.99 | 34.561 |
| minutes — naive: mean of season to date — Haulers | 11785 | 31.291 | 44.763 | -28.876 | 34.204 |
| minutes — naive: mean of season to date — all categories | 60554 | 31.569 | 41.114 | -3.82 | 40.936 |
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
| predicted under 1.0 | 8738 | 0.646 | 0.861 | 1.333 |
| predicted 1.0 to 2.0 | 18751 | 1.518 | 1.777 | 1.17 |
| predicted 2.0 to 3.0 | 18393 | 2.469 | 2.708 | 1.097 |
| predicted 3.0 to 4.0 | 9318 | 3.412 | 3.452 | 1.012 |
| predicted 4.0 to 5.0 | 3028 | 4.428 | 4.178 | 0.943 |
| predicted 5.0 to 6.0 | 1242 | 5.417 | 4.948 | 0.913 |
| predicted 6.0 and above | 1084 | 7.522 | 6.678 | 0.888 |

**Reading it.** The ratio is actual divided by predicted, so **1.000 is perfect and below 1.000 means the band is over-predicted**. The top band is where a transfer search picks, so its ratio matters more than the aggregate: a bias shared by every player is invisible to an argmax, and this project has measured that correcting one costs points.

### Is a candidate change safe for an argmax, or a variance trade?

Each arm of the benchmark paired against the shipped config on the same observations. Two of the arms are controls rather than proposals: switching minutes recency off must make the minutes error worse, and switching the vice-captain fallback off must change nothing at all, since it alters how a played-out gameweek is scored and not what the model predicts.

*Population: 6 seasons, gameweeks 6-38, one gameweek ahead.*

| | n | change in mean absolute error | change in root-mean-square error | change in absolute bias | change in error sd | change in signed error over the top predicted | change in rank correlation |
|---|---:|---:|---:|---:|---:|---:|---:|
| CONTROL, directional: minutes recency off — points | 60554 | 0.041 | 0.075 | 0.113 | 0.067 | — | — |
| CONTROL, directional: minutes recency off — minutes | 60554 | 3.902 | 4.234 | 3.142 | 4.04 | — | — |
| CONTROL, directional: minutes recency off — expected goals + assists | 60554 | 0.001 | 0.005 | 0.005 | 0.004 | — | — |
| CONTROL, directional: minutes recency off — tail and ordering | 0 | — | — | — | — | 0.017 | -0.087 |
| CONTROL, invariance: vice-captain fallback off — points | 60554 | 0 | 0 | 0 | 0 | — | — |
| CONTROL, invariance: vice-captain fallback off — minutes | 60554 | 0 | 0 | 0 | 0 | — | — |
| CONTROL, invariance: vice-captain fallback off — expected goals + assists | 60554 | 0 | 0 | 0 | 0 | — | — |
| CONTROL, invariance: vice-captain fallback off — tail and ordering | 0 | — | — | — | — | 0 | 0 |
| CANDIDATE: two estimators of P(appears), as before the unification — points | 60554 | 0 | 0 | 0.001 | 0 | — | — |
| CANDIDATE: two estimators of P(appears), as before the unification — minutes | 60554 | 0 | 0 | 0 | 0 | — | — |
| CANDIDATE: two estimators of P(appears), as before the unification — expected goals + assists | 60554 | 0 | 0 | 0 | 0 | — | — |
| CANDIDATE: two estimators of P(appears), as before the unification — tail and ordering | 0 | — | — | — | — | -0.005 | 0 |
| CANDIDATE: appearance constants refit on the windowed population — points | 60554 | -0.015 | 0.004 | 0.072 | -0.001 | — | — |
| CANDIDATE: appearance constants refit on the windowed population — minutes | 60554 | 0 | 0 | 0 | 0 | — | — |
| CANDIDATE: appearance constants refit on the windowed population — expected goals + assists | 60554 | 0 | 0 | 0 | 0 | — | — |
| CANDIDATE: appearance constants refit on the windowed population — tail and ordering | 0 | — | — | — | — | -0.183 | -0.004 |
| CANDIDATE: appearance constants refit against ExpectedMinutes — points | 60554 | -0.007 | -0.001 | 0.025 | -0.003 | — | — |
| CANDIDATE: appearance constants refit against ExpectedMinutes — minutes | 60554 | 0 | 0 | 0 | 0 | — | — |
| CANDIDATE: appearance constants refit against ExpectedMinutes — expected goals + assists | 60554 | 0 | 0 | 0 | 0 | — | — |
| CANDIDATE: appearance constants refit against ExpectedMinutes — tail and ordering | 0 | — | — | — | — | -0.09 | -0.001 |
| CANDIDATE: the sixty-minute curve alone, refit against ExpectedMinutes — points | 60554 | -0.007 | -0.001 | 0.024 | -0.003 | — | — |
| CANDIDATE: the sixty-minute curve alone, refit against ExpectedMinutes — minutes | 60554 | 0 | 0 | 0 | 0 | — | — |
| CANDIDATE: the sixty-minute curve alone, refit against ExpectedMinutes — expected goals + assists | 60554 | 0 | 0 | 0 | 0 | — | — |
| CANDIDATE: the sixty-minute curve alone, refit against ExpectedMinutes — tail and ordering | 0 | — | — | — | — | -0.088 | -0.001 |
| CANDIDATE: rate recency, half-life 8 — points | 60554 | -0.008 | -0.008 | 0.017 | -0.009 | — | — |
| CANDIDATE: rate recency, half-life 8 — minutes | 60554 | 0 | 0 | 0 | 0 | — | — |
| CANDIDATE: rate recency, half-life 8 — expected goals + assists | 60554 | 0 | 0 | 0 | 0 | — | — |
| CANDIDATE: rate recency, half-life 8 — tail and ordering | 0 | — | — | — | — | -0.054 | 0.003 |

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
| model | 197 | 0.431 | 0.329 |
| naive: mean of last 5 gameweeks | 197 | 0.335 | 2.61 |
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
| players averaging 40-50 minutes a gameweek | 217 | 0.454 | 0.461 | -0.007 | 0.423 |
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

### What is not measured here, and why

**No section here prices a change in points.** Every figure above is measured against outcomes — what the players went on to do — and the unit is a player-gameweek or a team-match rather than a replayed season. That is what makes these the more trustworthy half: several rest on tens of thousands of observations where the harness half has twenty-four cells. It is also what they cannot do. A predictor that wins here is a candidate worth spending replay time on, not a candidate already proved, and this project has a measured case where about 2 per cent lower out-of-sample error cost roughly 49 points a season: a transfer policy is an argmax living in the tail of the estimate distribution, so accuracy bought on the average player is paid for with noise where the search looks.

The predictor comparison for minutes, points and expected goals **is now runnable** and appears above as "How much should a predictor weight recent gameweeks?". An earlier edition of this snapshot had to record its absence: the figures were frozen prose in `internal/analysis` and `internal/recent` with no code that could recompute them, which is precisely the orphaned measurement this artefact exists to prevent. Read the relative columns rather than the levels when comparing against those comments — the population behind the original was never written down, and it predates the doubles-counting fix that changed what a gameweek means in this archive.

## Change since the previous snapshot

Compared against `2026-08-11-b9c7d46`.

### Figures that moved

| figure | previous | now | change |
|---|---:|---:|---:|
| `model.calibration_drift.model_built_through_gw12.actual` | 2.9504 | 2.9038 | -0.0466 |
| `model.calibration_drift.model_built_through_gw12.bias` | -0.0251 | -0.0505 | -0.0254 |
| `model.calibration_drift.model_built_through_gw12.mean_absolute_error` | 1.3798 | 1.3787 | -0.0011 |
| `model.calibration_drift.model_built_through_gw12.predicted` | 2.9755 | 2.9543 | -0.0212 |
| `model.calibration_drift.model_built_through_gw12.ratio` | 0.9915 | 0.9829 | -0.0086 |
| `model.calibration_drift.model_built_through_gw16.actual` | 2.8768 | 2.8061 | -0.0707 |
| `model.calibration_drift.model_built_through_gw16.bias` | -0.5235 | -0.4515 | +0.072 |
| `model.calibration_drift.model_built_through_gw16.mean_absolute_error` | 1.5705 | 1.5491 | -0.0214 |
| `model.calibration_drift.model_built_through_gw16.predicted` | 3.4003 | 3.2575 | -0.1428 |
| `model.calibration_drift.model_built_through_gw16.ratio` | 0.8460 | 0.8614 | +0.0154 |
| `model.calibration_drift.model_built_through_gw20.actual` | 2.9887 | 3.0064 | +0.0177 |
| `model.calibration_drift.model_built_through_gw20.bias` | -0.2861 | -0.1491 | +0.137 |
| `model.calibration_drift.model_built_through_gw20.mean_absolute_error` | 1.4511 | 1.4665 | +0.0154 |
| `model.calibration_drift.model_built_through_gw20.predicted` | 3.2748 | 3.1555 | -0.1193 |
| `model.calibration_drift.model_built_through_gw20.ratio` | 0.9126 | 0.9528 | +0.0402 |
| `model.calibration_drift.model_built_through_gw24.actual` | 2.8638 | 2.9960 | +0.1322 |
| `model.calibration_drift.model_built_through_gw24.bias` | -0.3787 | -0.1291 | +0.2496 |
| `model.calibration_drift.model_built_through_gw24.mean_absolute_error` | 1.3983 | 1.4232 | +0.0249 |
| `model.calibration_drift.model_built_through_gw24.predicted` | 3.2425 | 3.1251 | -0.1174 |
| `model.calibration_drift.model_built_through_gw24.ratio` | 0.8832 | 0.9587 | +0.0755 |
| `model.calibration_drift.model_built_through_gw28.actual` | 2.8333 | 2.8315 | -0.0018 |
| `model.calibration_drift.model_built_through_gw28.bias` | -0.3471 | -0.2400 | +0.1071 |
| `model.calibration_drift.model_built_through_gw28.mean_absolute_error` | 1.4294 | 1.3840 | -0.0454 |
| `model.calibration_drift.model_built_through_gw28.predicted` | 3.1804 | 3.0715 | -0.1089 |
| `model.calibration_drift.model_built_through_gw28.ratio` | 0.8909 | 0.9219 | +0.031 |
| `model.calibration_drift.model_built_through_gw32.actual` | 3.1020 | 3.1451 | +0.0431 |
| `model.calibration_drift.model_built_through_gw32.bias` | -0.0447 | 0.1021 | +0.1468 |
| `model.calibration_drift.model_built_through_gw32.mean_absolute_error` | 1.4304 | 1.4282 | -0.0022 |
| `model.calibration_drift.model_built_through_gw32.predicted` | 3.1467 | 3.0430 | -0.1037 |
| `model.calibration_drift.model_built_through_gw32.ratio` | 0.9858 | 1.0335 | +0.0477 |
| `model.calibration_drift.model_built_through_gw4.actual` | 2.7473 | 2.7968 | +0.0495 |
| `model.calibration_drift.model_built_through_gw4.bias` | -0.1867 | -0.1308 | +0.0559 |
| `model.calibration_drift.model_built_through_gw4.mean_absolute_error` | 1.3097 | 1.3384 | +0.0287 |
| `model.calibration_drift.model_built_through_gw4.predicted` | 2.9340 | 2.9276 | -0.0064 |
| `model.calibration_drift.model_built_through_gw4.ratio` | 0.9364 | 0.9553 | +0.0189 |
| `model.calibration_drift.model_built_through_gw8.actual` | 2.9407 | 2.9254 | -0.0153 |
| `model.calibration_drift.model_built_through_gw8.bias` | -0.0110 | 0.0009 | +0.0119 |
| `model.calibration_drift.model_built_through_gw8.mean_absolute_error` | 1.3028 | 1.3187 | +0.0159 |
| `model.calibration_drift.model_built_through_gw8.predicted` | 2.9517 | 2.9246 | -0.0271 |
| `model.calibration_drift.model_built_through_gw8.ratio` | 0.9963 | 1.0003 | +0.004 |
| `model.next_five_predictors.expected_goals_assists_ewma_half_life_2.bias_predicted_minus_actual` | 0.0080 | 0.0071 | -0.0009 |
| `model.next_five_predictors.expected_goals_assists_ewma_half_life_2.mean_absolute_error` | 0.0931 | 0.0929 | -0.0002 |
| `model.next_five_predictors.expected_goals_assists_ewma_half_life_2.root_mean_square_error` | 0.1463 | 0.1470 | +0.0007 |
| `model.next_five_predictors.expected_goals_assists_ewma_half_life_20.bias_predicted_minus_actual` | -0.0017 | -0.0025 | -0.0008 |
| `model.next_five_predictors.expected_goals_assists_ewma_half_life_20.mean_absolute_error` | 0.0855 | 0.0849 | -0.0006 |
| `model.next_five_predictors.expected_goals_assists_ewma_half_life_20.root_mean_square_error` | 0.1332 | 0.1334 | +0.0002 |
| `model.next_five_predictors.expected_goals_assists_ewma_half_life_4.bias_predicted_minus_actual` | 0.0045 | 0.0037 | -0.0008 |
| `model.next_five_predictors.expected_goals_assists_ewma_half_life_4.mean_absolute_error` | 0.0874 | 0.0870 | -0.0004 |
| `model.next_five_predictors.expected_goals_assists_ewma_half_life_4.root_mean_square_error` | 0.1355 | 0.1359 | +0.0004 |
| `model.next_five_predictors.expected_goals_assists_ewma_half_life_8.bias_predicted_minus_actual` | 0.0010 | 0.0001 | -0.0009 |
| `model.next_five_predictors.expected_goals_assists_ewma_half_life_8.mean_absolute_error` | 0.0856 | 0.0851 | -0.0005 |
| `model.next_five_predictors.expected_goals_assists_ewma_half_life_8.root_mean_square_error` | 0.1328 | 0.1330 | +0.0002 |
| `model.next_five_predictors.expected_goals_assists_last_3_gameweeks_flat.bias_predicted_minus_actual` | 0.0100 | 0.0089 | -0.0011 |
| `model.next_five_predictors.expected_goals_assists_last_3_gameweeks_flat.mean_absolute_error` | 0.1065 | 0.1060 | -0.0005 |
| `model.next_five_predictors.expected_goals_assists_last_3_gameweeks_flat.root_mean_square_error` | 0.1704 | 0.1705 | +0.0001 |
| `model.next_five_predictors.expected_goals_assists_season_to_date_flat.bias_predicted_minus_actual` | -0.0035 | -0.0044 | -0.0009 |
| `model.next_five_predictors.expected_goals_assists_season_to_date_flat.mean_absolute_error` | 0.0861 | 0.0856 | -0.0005 |
| `model.next_five_predictors.expected_goals_assists_season_to_date_flat.root_mean_square_error` | 0.1345 | 0.1347 | +0.0002 |
| `model.next_five_predictors.minutes_ewma_half_life_2.bias_predicted_minus_actual` | 4.5149 | 4.2632 | -0.2517 |
| `model.next_five_predictors.minutes_ewma_half_life_2.mean_absolute_error` | 21.1910 | 22.0636 | +0.8726 |
| `model.next_five_predictors.minutes_ewma_half_life_2.root_mean_square_error` | 27.9178 | 28.9161 | +0.9983 |
| `model.next_five_predictors.minutes_ewma_half_life_20.bias_predicted_minus_actual` | 0.1175 | -0.5453 | -0.6628 |
| `model.next_five_predictors.minutes_ewma_half_life_20.mean_absolute_error` | 22.5373 | 23.1821 | +0.6448 |
| `model.next_five_predictors.minutes_ewma_half_life_20.root_mean_square_error` | 29.4729 | 30.1248 | +0.6519 |
| `model.next_five_predictors.minutes_ewma_half_life_4.bias_predicted_minus_actual` | 2.8725 | 2.5005 | -0.372 |
| `model.next_five_predictors.minutes_ewma_half_life_4.mean_absolute_error` | 21.4167 | 22.1203 | +0.7036 |
| `model.next_five_predictors.minutes_ewma_half_life_4.root_mean_square_error` | 27.9401 | 28.6950 | +0.7549 |
| `model.next_five_predictors.minutes_ewma_half_life_8.bias_predicted_minus_actual` | 1.2930 | 0.7627 | -0.5303 |
| `model.next_five_predictors.minutes_ewma_half_life_8.mean_absolute_error` | 21.9491 | 22.5992 | +0.6501 |
| `model.next_five_predictors.minutes_ewma_half_life_8.root_mean_square_error` | 28.6326 | 29.2957 | +0.6631 |
| `model.next_five_predictors.minutes_last_3_gameweeks_flat.bias_predicted_minus_actual` | 5.5085 | 5.3080 | -0.2005 |
| `model.next_five_predictors.minutes_last_3_gameweeks_flat.mean_absolute_error` | 22.8113 | 23.8729 | +1.0616 |
| `model.next_five_predictors.minutes_last_3_gameweeks_flat.root_mean_square_error` | 30.8452 | 32.0270 | +1.1818 |
| `model.next_five_predictors.minutes_season_to_date_flat.bias_predicted_minus_actual` | -0.6801 | -1.4325 | -0.7524 |
| `model.next_five_predictors.minutes_season_to_date_flat.mean_absolute_error` | 23.0572 | 23.7144 | +0.6572 |
| `model.next_five_predictors.minutes_season_to_date_flat.root_mean_square_error` | 30.2384 | 30.9043 | +0.6659 |
| `model.next_five_predictors.points_ewma_half_life_2.bias_predicted_minus_actual` | 0.1714 | 0.1643 | -0.0071 |
| `model.next_five_predictors.points_ewma_half_life_2.mean_absolute_error` | 1.4516 | 1.4666 | +0.015 |
| `model.next_five_predictors.points_ewma_half_life_2.root_mean_square_error` | 1.9219 | 1.9467 | +0.0248 |
| `model.next_five_predictors.points_ewma_half_life_20.bias_predicted_minus_actual` | 0.0005 | -0.0196 | -0.0201 |
| `model.next_five_predictors.points_ewma_half_life_20.mean_absolute_error` | 1.3378 | 1.3512 | +0.0134 |
| `model.next_five_predictors.points_ewma_half_life_20.root_mean_square_error` | 1.7582 | 1.7805 | +0.0223 |
| `model.next_five_predictors.points_ewma_half_life_4.bias_predicted_minus_actual` | 0.1096 | 0.0985 | -0.0111 |
| `model.next_five_predictors.points_ewma_half_life_4.mean_absolute_error` | 1.3659 | 1.3785 | +0.0126 |
| `model.next_five_predictors.points_ewma_half_life_4.root_mean_square_error` | 1.7880 | 1.8083 | +0.0203 |
| `model.next_five_predictors.points_ewma_half_life_8.bias_predicted_minus_actual` | 0.0476 | 0.0315 | -0.0161 |
| `model.next_five_predictors.points_ewma_half_life_8.mean_absolute_error` | 1.3385 | 1.3507 | +0.0122 |
| `model.next_five_predictors.points_ewma_half_life_8.root_mean_square_error` | 1.7538 | 1.7740 | +0.0202 |
| `model.next_five_predictors.points_last_3_gameweeks_flat.bias_predicted_minus_actual` | 0.2075 | 0.2022 | -0.0053 |
| `model.next_five_predictors.points_last_3_gameweeks_flat.mean_absolute_error` | 1.6589 | 1.6707 | +0.0118 |
| `model.next_five_predictors.points_last_3_gameweeks_flat.root_mean_square_error` | 2.2303 | 2.2497 | +0.0194 |
| `model.next_five_predictors.points_season_to_date_flat.bias_predicted_minus_actual` | -0.0319 | -0.0543 | -0.0224 |
| `model.next_five_predictors.points_season_to_date_flat.mean_absolute_error` | 1.3473 | 1.3622 | +0.0149 |
| `model.next_five_predictors.points_season_to_date_flat.root_mean_square_error` | 1.7745 | 1.7995 | +0.025 |
| `model.prediction_benchmark.expected_goals_assists_model_all_categories.bias_predicted_minus_actual` | 0.0002 | 0.0011 | +0.0009 |
| `model.prediction_benchmark.expected_goals_assists_model_all_categories.error_sd` | 0.2257 | 0.2278 | +0.0021 |
| `model.prediction_benchmark.expected_goals_assists_model_all_categories.root_mean_square_error` | 0.2257 | 0.2278 | +0.0021 |
| `model.prediction_benchmark.expected_goals_assists_model_blanks.bias_predicted_minus_actual` | 0.0263 | 0.0292 | +0.0029 |
| `model.prediction_benchmark.expected_goals_assists_model_blanks.error_sd` | 0.1737 | 0.1722 | -0.0015 |
| `model.prediction_benchmark.expected_goals_assists_model_blanks.mean_absolute_error` | 0.1139 | 0.1130 | -0.0009 |
| `model.prediction_benchmark.expected_goals_assists_model_blanks.root_mean_square_error` | 0.1757 | 0.1747 | -0.001 |
| `model.prediction_benchmark.expected_goals_assists_model_haulers.bias_predicted_minus_actual` | -0.1631 | -0.1662 | -0.0031 |
| `model.prediction_benchmark.expected_goals_assists_model_haulers.error_sd` | 0.3438 | 0.3492 | +0.0054 |
| `model.prediction_benchmark.expected_goals_assists_model_haulers.mean_absolute_error` | 0.2316 | 0.2351 | +0.0035 |
| `model.prediction_benchmark.expected_goals_assists_model_haulers.root_mean_square_error` | 0.3805 | 0.3867 | +0.0062 |
| `model.prediction_benchmark.expected_goals_assists_model_tickers.bias_predicted_minus_actual` | -0.0132 | -0.0107 | +0.0025 |
| `model.prediction_benchmark.expected_goals_assists_model_tickers.error_sd` | 0.2050 | 0.2105 | +0.0055 |
| `model.prediction_benchmark.expected_goals_assists_model_tickers.mean_absolute_error` | 0.1306 | 0.1339 | +0.0033 |
| `model.prediction_benchmark.expected_goals_assists_model_tickers.root_mean_square_error` | 0.2054 | 0.2108 | +0.0054 |
| `model.prediction_benchmark.expected_goals_assists_model_zeros.bias_predicted_minus_actual` | 0.0887 | 0.0863 | -0.0024 |
| `model.prediction_benchmark.expected_goals_assists_model_zeros.error_sd` | 0.1024 | 0.1016 | -0.0008 |
| `model.prediction_benchmark.expected_goals_assists_model_zeros.mean_absolute_error` | 0.0887 | 0.0863 | -0.0024 |
| `model.prediction_benchmark.expected_goals_assists_model_zeros.root_mean_square_error` | 0.1355 | 0.1333 | -0.0022 |
| `model.prediction_benchmark.expected_goals_assists_naive_mean_of_last_5_gameweeks_all_categories.bias_predicted_minus_actual` | 0.0064 | 0.0061 | -0.0003 |
| `model.prediction_benchmark.expected_goals_assists_naive_mean_of_last_5_gameweeks_all_categories.error_sd` | 0.2405 | 0.2438 | +0.0033 |
| `model.prediction_benchmark.expected_goals_assists_naive_mean_of_last_5_gameweeks_all_categories.mean_absolute_error` | 0.1414 | 0.1413 | -0.0001 |
| `model.prediction_benchmark.expected_goals_assists_naive_mean_of_last_5_gameweeks_all_categories.root_mean_square_error` | 0.2406 | 0.2439 | +0.0033 |
| `model.prediction_benchmark.expected_goals_assists_naive_mean_of_last_5_gameweeks_blanks.bias_predicted_minus_actual` | 0.0336 | 0.0366 | +0.003 |
| `model.prediction_benchmark.expected_goals_assists_naive_mean_of_last_5_gameweeks_blanks.error_sd` | 0.1915 | 0.1914 | -0.0001 |
| `model.prediction_benchmark.expected_goals_assists_naive_mean_of_last_5_gameweeks_blanks.mean_absolute_error` | 0.1246 | 0.1240 | -0.0006 |
| `model.prediction_benchmark.expected_goals_assists_naive_mean_of_last_5_gameweeks_blanks.root_mean_square_error` | 0.1945 | 0.1949 | +0.0004 |
| `model.prediction_benchmark.expected_goals_assists_naive_mean_of_last_5_gameweeks_haulers.bias_predicted_minus_actual` | -0.1556 | -0.1620 | -0.0064 |
| `model.prediction_benchmark.expected_goals_assists_naive_mean_of_last_5_gameweeks_haulers.error_sd` | 0.3579 | 0.3643 | +0.0064 |
| `model.prediction_benchmark.expected_goals_assists_naive_mean_of_last_5_gameweeks_haulers.mean_absolute_error` | 0.2420 | 0.2457 | +0.0037 |
| `model.prediction_benchmark.expected_goals_assists_naive_mean_of_last_5_gameweeks_haulers.root_mean_square_error` | 0.3903 | 0.3987 | +0.0084 |
| `model.prediction_benchmark.expected_goals_assists_naive_mean_of_last_5_gameweeks_tickers.bias_predicted_minus_actual` | -0.0085 | -0.0103 | -0.0018 |
| `model.prediction_benchmark.expected_goals_assists_naive_mean_of_last_5_gameweeks_tickers.error_sd` | 0.2219 | 0.2263 | +0.0044 |
| `model.prediction_benchmark.expected_goals_assists_naive_mean_of_last_5_gameweeks_tickers.mean_absolute_error` | 0.1421 | 0.1450 | +0.0029 |
| `model.prediction_benchmark.expected_goals_assists_naive_mean_of_last_5_gameweeks_tickers.root_mean_square_error` | 0.2221 | 0.2266 | +0.0045 |
| `model.prediction_benchmark.expected_goals_assists_naive_mean_of_last_5_gameweeks_zeros.bias_predicted_minus_actual` | 0.0919 | 0.0889 | -0.003 |
| `model.prediction_benchmark.expected_goals_assists_naive_mean_of_last_5_gameweeks_zeros.mean_absolute_error` | 0.0919 | 0.0889 | -0.003 |
| `model.prediction_benchmark.expected_goals_assists_naive_mean_of_last_5_gameweeks_zeros.root_mean_square_error` | 0.1577 | 0.1560 | -0.0017 |
| `model.prediction_benchmark.expected_goals_assists_naive_mean_of_season_to_date_all_categories.bias_predicted_minus_actual` | -0.0086 | -0.0087 | -0.0001 |
| `model.prediction_benchmark.expected_goals_assists_naive_mean_of_season_to_date_all_categories.error_sd` | 0.2321 | 0.2355 | +0.0034 |
| `model.prediction_benchmark.expected_goals_assists_naive_mean_of_season_to_date_all_categories.mean_absolute_error` | 0.1337 | 0.1335 | -0.0002 |
| `model.prediction_benchmark.expected_goals_assists_naive_mean_of_season_to_date_all_categories.root_mean_square_error` | 0.2323 | 0.2356 | +0.0033 |
| `model.prediction_benchmark.expected_goals_assists_naive_mean_of_season_to_date_blanks.bias_predicted_minus_actual` | 0.0155 | 0.0187 | +0.0032 |
| `model.prediction_benchmark.expected_goals_assists_naive_mean_of_season_to_date_blanks.error_sd` | 0.1756 | 0.1747 | -0.0009 |
| `model.prediction_benchmark.expected_goals_assists_naive_mean_of_season_to_date_blanks.mean_absolute_error` | 0.1116 | 0.1105 | -0.0011 |
| `model.prediction_benchmark.expected_goals_assists_naive_mean_of_season_to_date_blanks.root_mean_square_error` | 0.1763 | 0.1757 | -0.0006 |
| `model.prediction_benchmark.expected_goals_assists_naive_mean_of_season_to_date_haulers.bias_predicted_minus_actual` | -0.1768 | -0.1836 | -0.0068 |
| `model.prediction_benchmark.expected_goals_assists_naive_mean_of_season_to_date_haulers.error_sd` | 0.3508 | 0.3584 | +0.0076 |
| `model.prediction_benchmark.expected_goals_assists_naive_mean_of_season_to_date_haulers.mean_absolute_error` | 0.2403 | 0.2447 | +0.0044 |
| `model.prediction_benchmark.expected_goals_assists_naive_mean_of_season_to_date_haulers.root_mean_square_error` | 0.3929 | 0.4027 | +0.0098 |
| `model.prediction_benchmark.expected_goals_assists_naive_mean_of_season_to_date_tickers.bias_predicted_minus_actual` | -0.0310 | -0.0321 | -0.0011 |
| `model.prediction_benchmark.expected_goals_assists_naive_mean_of_season_to_date_tickers.error_sd` | 0.2084 | 0.2122 | +0.0038 |
| `model.prediction_benchmark.expected_goals_assists_naive_mean_of_season_to_date_tickers.mean_absolute_error` | 0.1307 | 0.1329 | +0.0022 |
| `model.prediction_benchmark.expected_goals_assists_naive_mean_of_season_to_date_tickers.root_mean_square_error` | 0.2107 | 0.2146 | +0.0039 |
| `model.prediction_benchmark.expected_goals_assists_naive_mean_of_season_to_date_zeros.bias_predicted_minus_actual` | 0.0925 | 0.0890 | -0.0035 |
| `model.prediction_benchmark.expected_goals_assists_naive_mean_of_season_to_date_zeros.error_sd` | 0.1181 | 0.1157 | -0.0024 |
| `model.prediction_benchmark.expected_goals_assists_naive_mean_of_season_to_date_zeros.mean_absolute_error` | 0.0925 | 0.0890 | -0.0035 |
| `model.prediction_benchmark.expected_goals_assists_naive_mean_of_season_to_date_zeros.root_mean_square_error` | 0.1500 | 0.1459 | -0.0041 |
| `model.prediction_benchmark.minutes_model_all_categories.bias_predicted_minus_actual` | -0.9453 | -0.8496 | +0.0957 |
| `model.prediction_benchmark.minutes_model_all_categories.error_sd` | 33.6194 | 34.8520 | +1.2326 |
| `model.prediction_benchmark.minutes_model_all_categories.mean_absolute_error` | 26.8172 | 27.7028 | +0.8856 |
| `model.prediction_benchmark.minutes_model_all_categories.root_mean_square_error` | 33.6326 | 34.8624 | +1.2298 |
| `model.prediction_benchmark.minutes_model_blanks.bias_predicted_minus_actual` | -8.3205 | -9.2010 | -0.8805 |
| `model.prediction_benchmark.minutes_model_blanks.error_sd` | 27.9766 | 28.6638 | +0.6872 |
| `model.prediction_benchmark.minutes_model_blanks.mean_absolute_error` | 23.0184 | 23.6355 | +0.6171 |
| `model.prediction_benchmark.minutes_model_blanks.root_mean_square_error` | 29.1877 | 30.1044 | +0.9167 |
| `model.prediction_benchmark.minutes_model_haulers.bias_predicted_minus_actual` | -20.1098 | -21.1326 | -1.0228 |
| `model.prediction_benchmark.minutes_model_haulers.error_sd` | 22.5746 | 23.6670 | +1.0924 |
| `model.prediction_benchmark.minutes_model_haulers.mean_absolute_error` | 23.0806 | 24.0670 | +0.9864 |
| `model.prediction_benchmark.minutes_model_haulers.root_mean_square_error` | 30.2327 | 31.7287 | +1.496 |
| `model.prediction_benchmark.minutes_model_tickers.bias_predicted_minus_actual` | -19.1620 | -19.8833 | -0.7213 |
| `model.prediction_benchmark.minutes_model_tickers.error_sd` | 23.6870 | 24.9045 | +1.2175 |
| `model.prediction_benchmark.minutes_model_tickers.mean_absolute_error` | 23.5477 | 24.3788 | +0.8311 |
| `model.prediction_benchmark.minutes_model_tickers.root_mean_square_error` | 30.4673 | 31.8682 | +1.4009 |
| `model.prediction_benchmark.minutes_model_zeros.bias_predicted_minus_actual` | 40.1011 | 40.3562 | +0.2551 |
| `model.prediction_benchmark.minutes_model_zeros.error_sd` | 20.8316 | 21.7354 | +0.9038 |
| `model.prediction_benchmark.minutes_model_zeros.mean_absolute_error` | 40.1011 | 40.3562 | +0.2551 |
| `model.prediction_benchmark.minutes_model_zeros.root_mean_square_error` | 45.1891 | 45.8372 | +0.6481 |
| `model.prediction_benchmark.minutes_naive_mean_of_last_5_gameweeks_all_categories.bias_predicted_minus_actual` | 3.4380 | 3.3194 | -0.1186 |
| `model.prediction_benchmark.minutes_naive_mean_of_last_5_gameweeks_all_categories.error_sd` | 36.8888 | 38.9275 | +2.0387 |
| `model.prediction_benchmark.minutes_naive_mean_of_last_5_gameweeks_all_categories.mean_absolute_error` | 27.4279 | 28.9883 | +1.5604 |
| `model.prediction_benchmark.minutes_naive_mean_of_last_5_gameweeks_all_categories.root_mean_square_error` | 37.0487 | 39.0688 | +2.0201 |
| `model.prediction_benchmark.minutes_naive_mean_of_last_5_gameweeks_blanks.bias_predicted_minus_actual` | -2.9363 | -3.4653 | -0.529 |
| `model.prediction_benchmark.minutes_naive_mean_of_last_5_gameweeks_blanks.error_sd` | 29.9821 | 30.9321 | +0.95 |
| `model.prediction_benchmark.minutes_naive_mean_of_last_5_gameweeks_blanks.mean_absolute_error` | 21.9887 | 22.7269 | +0.7382 |
| `model.prediction_benchmark.minutes_naive_mean_of_last_5_gameweeks_blanks.root_mean_square_error` | 30.1256 | 31.1256 | +1 |
| `model.prediction_benchmark.minutes_naive_mean_of_last_5_gameweeks_haulers.bias_predicted_minus_actual` | -17.7919 | -20.1541 | -2.3622 |
| `model.prediction_benchmark.minutes_naive_mean_of_last_5_gameweeks_haulers.error_sd` | 30.3595 | 33.7834 | +3.4239 |
| `model.prediction_benchmark.minutes_naive_mean_of_last_5_gameweeks_haulers.mean_absolute_error` | 23.0441 | 25.9509 | +2.9068 |
| `model.prediction_benchmark.minutes_naive_mean_of_last_5_gameweeks_haulers.root_mean_square_error` | 35.1887 | 39.3383 | +4.1496 |
| `model.prediction_benchmark.minutes_naive_mean_of_last_5_gameweeks_tickers.bias_predicted_minus_actual` | -17.4483 | -19.6534 | -2.2051 |
| `model.prediction_benchmark.minutes_naive_mean_of_last_5_gameweeks_tickers.error_sd` | 31.4403 | 34.0922 | +2.6519 |
| `model.prediction_benchmark.minutes_naive_mean_of_last_5_gameweeks_tickers.mean_absolute_error` | 23.9987 | 26.3241 | +2.3254 |
| `model.prediction_benchmark.minutes_naive_mean_of_last_5_gameweeks_tickers.root_mean_square_error` | 35.9574 | 39.3514 | +3.394 |
| `model.prediction_benchmark.minutes_naive_mean_of_last_5_gameweeks_zeros.bias_predicted_minus_actual` | 45.0920 | 45.4216 | +0.3296 |
| `model.prediction_benchmark.minutes_naive_mean_of_last_5_gameweeks_zeros.error_sd` | 23.5008 | 24.2310 | +0.7302 |
| `model.prediction_benchmark.minutes_naive_mean_of_last_5_gameweeks_zeros.mean_absolute_error` | 45.0920 | 45.4216 | +0.3296 |
| `model.prediction_benchmark.minutes_naive_mean_of_last_5_gameweeks_zeros.root_mean_square_error` | 50.8486 | 51.4807 | +0.6321 |
| `model.prediction_benchmark.minutes_naive_mean_of_season_to_date_all_categories.bias_predicted_minus_actual` | -3.3419 | -3.8205 | -0.4786 |
| `model.prediction_benchmark.minutes_naive_mean_of_season_to_date_all_categories.error_sd` | 39.2542 | 40.9361 | +1.6819 |
| `model.prediction_benchmark.minutes_naive_mean_of_season_to_date_all_categories.mean_absolute_error` | 30.3648 | 31.5689 | +1.2041 |
| `model.prediction_benchmark.minutes_naive_mean_of_season_to_date_all_categories.root_mean_square_error` | 39.3962 | 41.1140 | +1.7178 |
| `model.prediction_benchmark.minutes_naive_mean_of_season_to_date_blanks.bias_predicted_minus_actual` | -11.2943 | -12.2306 | -0.9363 |
| `model.prediction_benchmark.minutes_naive_mean_of_season_to_date_blanks.error_sd` | 31.1620 | 31.5383 | +0.3763 |
| `model.prediction_benchmark.minutes_naive_mean_of_season_to_date_blanks.mean_absolute_error` | 25.5015 | 25.9471 | +0.4456 |
| `model.prediction_benchmark.minutes_naive_mean_of_season_to_date_blanks.root_mean_square_error` | 33.1456 | 33.8268 | +0.6812 |
| `model.prediction_benchmark.minutes_naive_mean_of_season_to_date_haulers.bias_predicted_minus_actual` | -25.7630 | -28.8760 | -3.113 |
| `model.prediction_benchmark.minutes_naive_mean_of_season_to_date_haulers.error_sd` | 31.2696 | 34.2040 | +2.9344 |
| `model.prediction_benchmark.minutes_naive_mean_of_season_to_date_haulers.mean_absolute_error` | 28.4561 | 31.2908 | +2.8347 |
| `model.prediction_benchmark.minutes_naive_mean_of_season_to_date_haulers.root_mean_square_error` | 40.5157 | 44.7631 | +4.2474 |
| `model.prediction_benchmark.minutes_naive_mean_of_season_to_date_tickers.bias_predicted_minus_actual` | -25.5943 | -27.9899 | -2.3956 |
| `model.prediction_benchmark.minutes_naive_mean_of_season_to_date_tickers.error_sd` | 32.1791 | 34.5609 | +2.3818 |
| `model.prediction_benchmark.minutes_naive_mean_of_season_to_date_tickers.mean_absolute_error` | 29.4606 | 31.5593 | +2.0987 |
| `model.prediction_benchmark.minutes_naive_mean_of_season_to_date_tickers.root_mean_square_error` | 41.1164 | 44.4734 | +3.357 |
| `model.prediction_benchmark.minutes_naive_mean_of_season_to_date_zeros.bias_predicted_minus_actual` | 43.5183 | 43.3961 | -0.1222 |
| `model.prediction_benchmark.minutes_naive_mean_of_season_to_date_zeros.error_sd` | 23.6627 | 23.7729 | +0.1102 |
| `model.prediction_benchmark.minutes_naive_mean_of_season_to_date_zeros.mean_absolute_error` | 43.5183 | 43.3961 | -0.1222 |
| `model.prediction_benchmark.minutes_naive_mean_of_season_to_date_zeros.root_mean_square_error` | 49.5355 | 49.4811 | -0.0544 |
| `model.prediction_benchmark.points_model_all_categories.bias_predicted_minus_actual` | -0.0646 | -0.1526 | -0.088 |
| `model.prediction_benchmark.points_model_all_categories.error_sd` | 2.8860 | 2.9153 | +0.0293 |
| `model.prediction_benchmark.points_model_all_categories.mean_absolute_error` | 1.9786 | 1.9747 | -0.0039 |
| `model.prediction_benchmark.points_model_all_categories.root_mean_square_error` | 2.8867 | 2.9193 | +0.0326 |
| `model.prediction_benchmark.points_model_blanks.bias_predicted_minus_actual` | 1.0784 | 0.9877 | -0.0907 |
| `model.prediction_benchmark.points_model_blanks.error_sd` | 1.2397 | 1.2211 | -0.0186 |
| `model.prediction_benchmark.points_model_blanks.mean_absolute_error` | 1.2447 | 1.1773 | -0.0674 |
| `model.prediction_benchmark.points_model_blanks.root_mean_square_error` | 1.6431 | 1.5706 | -0.0725 |
| `model.prediction_benchmark.points_model_haulers.bias_predicted_minus_actual` | -4.7009 | -4.8216 | -0.1207 |
| `model.prediction_benchmark.points_model_haulers.error_sd` | 3.0377 | 3.0636 | +0.0259 |
| `model.prediction_benchmark.points_model_haulers.mean_absolute_error` | 4.7788 | 4.8955 | +0.1167 |
| `model.prediction_benchmark.points_model_haulers.root_mean_square_error` | 5.5970 | 5.7125 | +0.1155 |
| `model.prediction_benchmark.points_model_tickers.bias_predicted_minus_actual` | -0.4547 | -0.4586 | -0.0039 |
| `model.prediction_benchmark.points_model_tickers.error_sd` | 1.5069 | 1.4780 | -0.0289 |
| `model.prediction_benchmark.points_model_tickers.mean_absolute_error` | 1.1922 | 1.1855 | -0.0067 |
| `model.prediction_benchmark.points_model_tickers.root_mean_square_error` | 1.5740 | 1.5475 | -0.0265 |
| `model.prediction_benchmark.points_model_zeros.bias_predicted_minus_actual` | 1.5321 | 1.4889 | -0.0432 |
| `model.prediction_benchmark.points_model_zeros.error_sd` | 1.0832 | 1.0314 | -0.0518 |
| `model.prediction_benchmark.points_model_zeros.mean_absolute_error` | 1.5321 | 1.4889 | -0.0432 |
| `model.prediction_benchmark.points_model_zeros.root_mean_square_error` | 1.8763 | 1.8112 | -0.0651 |
| `model.prediction_benchmark.points_naive_mean_of_last_5_gameweeks_all_categories.bias_predicted_minus_actual` | 0.1425 | 0.1385 | -0.004 |
| `model.prediction_benchmark.points_naive_mean_of_last_5_gameweeks_all_categories.error_sd` | 3.1100 | 3.1552 | +0.0452 |
| `model.prediction_benchmark.points_naive_mean_of_last_5_gameweeks_all_categories.mean_absolute_error` | 2.2041 | 2.2283 | +0.0242 |
| `model.prediction_benchmark.points_naive_mean_of_last_5_gameweeks_all_categories.root_mean_square_error` | 3.1133 | 3.1582 | +0.0449 |
| `model.prediction_benchmark.points_naive_mean_of_last_5_gameweeks_blanks.bias_predicted_minus_actual` | 1.3215 | 1.3280 | +0.0065 |
| `model.prediction_benchmark.points_naive_mean_of_last_5_gameweeks_blanks.error_sd` | 1.6756 | 1.7045 | +0.0289 |
| `model.prediction_benchmark.points_naive_mean_of_last_5_gameweeks_blanks.mean_absolute_error` | 1.5785 | 1.5998 | +0.0213 |
| `model.prediction_benchmark.points_naive_mean_of_last_5_gameweeks_blanks.root_mean_square_error` | 2.1340 | 2.1608 | +0.0268 |
| `model.prediction_benchmark.points_naive_mean_of_last_5_gameweeks_haulers.bias_predicted_minus_actual` | -4.5685 | -4.5656 | +0.0029 |
| `model.prediction_benchmark.points_naive_mean_of_last_5_gameweeks_haulers.error_sd` | 3.2701 | 3.3513 | +0.0812 |
| `model.prediction_benchmark.points_naive_mean_of_last_5_gameweeks_haulers.mean_absolute_error` | 4.7222 | 4.7244 | +0.0022 |
| `model.prediction_benchmark.points_naive_mean_of_last_5_gameweeks_haulers.root_mean_square_error` | 5.6183 | 5.6636 | +0.0453 |
| `model.prediction_benchmark.points_naive_mean_of_last_5_gameweeks_tickers.bias_predicted_minus_actual` | -0.2696 | -0.2374 | +0.0322 |
| `model.prediction_benchmark.points_naive_mean_of_last_5_gameweeks_tickers.error_sd` | 1.8150 | 1.8286 | +0.0136 |
| `model.prediction_benchmark.points_naive_mean_of_last_5_gameweeks_tickers.mean_absolute_error` | 1.4685 | 1.4622 | -0.0063 |
| `model.prediction_benchmark.points_naive_mean_of_last_5_gameweeks_tickers.root_mean_square_error` | 1.8349 | 1.8439 | +0.009 |
| `model.prediction_benchmark.points_naive_mean_of_last_5_gameweeks_zeros.bias_predicted_minus_actual` | 1.7311 | 1.7338 | +0.0027 |
| `model.prediction_benchmark.points_naive_mean_of_last_5_gameweeks_zeros.error_sd` | 1.3869 | 1.4202 | +0.0333 |
| `model.prediction_benchmark.points_naive_mean_of_last_5_gameweeks_zeros.mean_absolute_error` | 1.7356 | 1.7385 | +0.0029 |
| `model.prediction_benchmark.points_naive_mean_of_last_5_gameweeks_zeros.root_mean_square_error` | 2.2181 | 2.2412 | +0.0231 |
| `model.prediction_benchmark.points_naive_mean_of_season_to_date_all_categories.bias_predicted_minus_actual` | -0.1219 | -0.1344 | -0.0125 |
| `model.prediction_benchmark.points_naive_mean_of_season_to_date_all_categories.error_sd` | 3.0133 | 3.0609 | +0.0476 |
| `model.prediction_benchmark.points_naive_mean_of_season_to_date_all_categories.mean_absolute_error` | 2.0935 | 2.1179 | +0.0244 |
| `model.prediction_benchmark.points_naive_mean_of_season_to_date_all_categories.root_mean_square_error` | 3.0157 | 3.0638 | +0.0481 |
| `model.prediction_benchmark.points_naive_mean_of_season_to_date_blanks.bias_predicted_minus_actual` | 0.9973 | 0.9923 | -0.005 |
| `model.prediction_benchmark.points_naive_mean_of_season_to_date_blanks.error_sd` | 1.3871 | 1.4042 | +0.0171 |
| `model.prediction_benchmark.points_naive_mean_of_season_to_date_blanks.mean_absolute_error` | 1.3018 | 1.3056 | +0.0038 |
| `model.prediction_benchmark.points_naive_mean_of_season_to_date_blanks.root_mean_square_error` | 1.7084 | 1.7194 | +0.011 |
| `model.prediction_benchmark.points_naive_mean_of_season_to_date_haulers.bias_predicted_minus_actual` | -4.9270 | -4.9489 | -0.0219 |
| `model.prediction_benchmark.points_naive_mean_of_season_to_date_haulers.error_sd` | 3.0701 | 3.1568 | +0.0867 |
| `model.prediction_benchmark.points_naive_mean_of_season_to_date_haulers.mean_absolute_error` | 4.9721 | 4.9997 | +0.0276 |
| `model.prediction_benchmark.points_naive_mean_of_season_to_date_haulers.root_mean_square_error` | 5.8052 | 5.8700 | +0.0648 |
| `model.prediction_benchmark.points_naive_mean_of_season_to_date_tickers.bias_predicted_minus_actual` | -0.6509 | -0.6232 | +0.0277 |
| `model.prediction_benchmark.points_naive_mean_of_season_to_date_tickers.error_sd` | 1.4174 | 1.4248 | +0.0074 |
| `model.prediction_benchmark.points_naive_mean_of_season_to_date_tickers.mean_absolute_error` | 1.2495 | 1.2487 | -0.0008 |
| `model.prediction_benchmark.points_naive_mean_of_season_to_date_tickers.root_mean_square_error` | 1.5598 | 1.5551 | -0.0047 |
| `model.prediction_benchmark.points_naive_mean_of_season_to_date_zeros.bias_predicted_minus_actual` | 1.7331 | 1.7244 | -0.0087 |
| `model.prediction_benchmark.points_naive_mean_of_season_to_date_zeros.error_sd` | 1.1997 | 1.2104 | +0.0107 |
| `model.prediction_benchmark.points_naive_mean_of_season_to_date_zeros.mean_absolute_error` | 1.7338 | 1.7253 | -0.0085 |
| `model.prediction_benchmark.points_naive_mean_of_season_to_date_zeros.root_mean_square_error` | 2.1078 | 2.1068 | -0.001 |
| `model.prediction_calibration.predicted_1_0_to_2_0.actual` | 1.7296 | 1.7769 | +0.0473 |
| `model.prediction_calibration.predicted_1_0_to_2_0.predicted` | 1.5133 | 1.5181 | +0.0048 |
| `model.prediction_calibration.predicted_1_0_to_2_0.ratio` | 1.1429 | 1.1705 | +0.0276 |
| `model.prediction_calibration.predicted_2_0_to_3_0.actual` | 2.6043 | 2.7077 | +0.1034 |
| `model.prediction_calibration.predicted_2_0_to_3_0.predicted` | 2.4859 | 2.4685 | -0.0174 |
| `model.prediction_calibration.predicted_2_0_to_3_0.ratio` | 1.0476 | 1.0969 | +0.0493 |
| `model.prediction_calibration.predicted_3_0_to_4_0.actual` | 3.3907 | 3.4517 | +0.061 |
| `model.prediction_calibration.predicted_3_0_to_4_0.ratio` | 0.9936 | 1.0115 | +0.0179 |
| `model.prediction_calibration.predicted_4_0_to_5_0.actual` | 3.9900 | 4.1780 | +0.188 |
| `model.prediction_calibration.predicted_4_0_to_5_0.predicted` | 4.4098 | 4.4285 | +0.0187 |
| `model.prediction_calibration.predicted_4_0_to_5_0.ratio` | 0.9048 | 0.9434 | +0.0386 |
| `model.prediction_calibration.predicted_5_0_to_6_0.actual` | 4.6690 | 4.9485 | +0.2795 |
| `model.prediction_calibration.predicted_5_0_to_6_0.predicted` | 5.4300 | 5.4175 | -0.0125 |
| `model.prediction_calibration.predicted_5_0_to_6_0.ratio` | 0.8599 | 0.9134 | +0.0535 |
| `model.prediction_calibration.predicted_6_0_and_above.actual` | 6.4125 | 6.6780 | +0.2655 |
| `model.prediction_calibration.predicted_6_0_and_above.predicted` | 7.5929 | 7.5222 | -0.0707 |
| `model.prediction_calibration.predicted_6_0_and_above.ratio` | 0.8445 | 0.8878 | +0.0433 |
| `model.prediction_calibration.predicted_under_1_0.actual` | 0.8621 | 0.8608 | -0.0013 |
| `model.prediction_calibration.predicted_under_1_0.predicted` | 0.6344 | 0.6460 | +0.0116 |
| `model.prediction_calibration.predicted_under_1_0.ratio` | 1.3588 | 1.3326 | -0.0262 |
| `model.prediction_candidates.candidate_appearance_constants_refit_against_expectedminutes_points.change_in_absolute_bias` | 0.0291 | 0.0252 | -0.0039 |
| `model.prediction_candidates.candidate_appearance_constants_refit_against_expectedminutes_points.change_in_error_sd` | -0.0044 | -0.0028 | +0.0016 |
| `model.prediction_candidates.candidate_appearance_constants_refit_against_expectedminutes_points.change_in_mean_absolute_error` | -0.0095 | -0.0072 | +0.0023 |
| `model.prediction_candidates.candidate_appearance_constants_refit_against_expectedminutes_points.change_in_root_mean_square_error` | -0.0036 | -0.0014 | +0.0022 |
| `model.prediction_candidates.candidate_appearance_constants_refit_against_expectedminutes_tail_and_ordering.change_in_rank_correlation` | -0.0006 | -0.0007 | -0.0001 |
| `model.prediction_candidates.candidate_appearance_constants_refit_against_expectedminutes_tail_and_ordering.change_in_signed_error_over_the_top_predicted` | -0.1040 | -0.0898 | +0.0142 |
| `model.prediction_candidates.candidate_appearance_constants_refit_on_the_windowed_population_points.change_in_absolute_bias` | 0.0808 | 0.0721 | -0.0087 |
| `model.prediction_candidates.candidate_appearance_constants_refit_on_the_windowed_population_points.change_in_error_sd` | -0.0038 | -0.0007 | +0.0031 |
| `model.prediction_candidates.candidate_appearance_constants_refit_on_the_windowed_population_points.change_in_mean_absolute_error` | -0.0217 | -0.0153 | +0.0064 |
| `model.prediction_candidates.candidate_appearance_constants_refit_on_the_windowed_population_points.change_in_root_mean_square_error` | -0.0009 | 0.0040 | +0.0049 |
| `model.prediction_candidates.candidate_appearance_constants_refit_on_the_windowed_population_tail_and_ordering.change_in_signed_error_over_the_top_predicted` | -0.1964 | -0.1831 | +0.0133 |
| `model.prediction_candidates.candidate_rate_recency_half_life_8_expected_goals_assists.change_in_absolute_bias` | 0.0003 | 0.0001 | -0.0002 |
| `model.prediction_candidates.candidate_rate_recency_half_life_8_expected_goals_assists.change_in_mean_absolute_error` | 0.0001 | -0.0000 | -0.0001 |
| `model.prediction_candidates.candidate_rate_recency_half_life_8_points.change_in_absolute_bias` | 0.0250 | 0.0174 | -0.0076 |
| `model.prediction_candidates.candidate_rate_recency_half_life_8_points.change_in_error_sd` | -0.0118 | -0.0088 | +0.003 |
| `model.prediction_candidates.candidate_rate_recency_half_life_8_points.change_in_mean_absolute_error` | -0.0119 | -0.0082 | +0.0037 |
| `model.prediction_candidates.candidate_rate_recency_half_life_8_points.change_in_root_mean_square_error` | -0.0111 | -0.0078 | +0.0033 |
| `model.prediction_candidates.candidate_rate_recency_half_life_8_tail_and_ordering.change_in_rank_correlation` | 0.0032 | 0.0026 | -0.0006 |
| `model.prediction_candidates.candidate_rate_recency_half_life_8_tail_and_ordering.change_in_signed_error_over_the_top_predicted` | -0.0776 | -0.0539 | +0.0237 |
| `model.prediction_candidates.candidate_the_sixty_minute_curve_alone_refit_against_expectedminutes_points.change_in_absolute_bias` | 0.0282 | 0.0242 | -0.004 |
| `model.prediction_candidates.candidate_the_sixty_minute_curve_alone_refit_against_expectedminutes_points.change_in_error_sd` | -0.0044 | -0.0029 | +0.0015 |
| `model.prediction_candidates.candidate_the_sixty_minute_curve_alone_refit_against_expectedminutes_points.change_in_mean_absolute_error` | -0.0092 | -0.0069 | +0.0023 |
| `model.prediction_candidates.candidate_the_sixty_minute_curve_alone_refit_against_expectedminutes_points.change_in_root_mean_square_error` | -0.0036 | -0.0015 | +0.0021 |
| `model.prediction_candidates.candidate_the_sixty_minute_curve_alone_refit_against_expectedminutes_tail_and_ordering.change_in_rank_correlation` | -0.0006 | -0.0008 | -0.0002 |
| `model.prediction_candidates.candidate_the_sixty_minute_curve_alone_refit_against_expectedminutes_tail_and_ordering.change_in_signed_error_over_the_top_predicted` | -0.0998 | -0.0878 | +0.012 |
| `model.prediction_candidates.candidate_two_estimators_of_p_appears_as_before_the_unification_points.change_in_absolute_bias` | 0.0011 | 0.0008 | -0.0003 |
| `model.prediction_candidates.candidate_two_estimators_of_p_appears_as_before_the_unification_points.change_in_mean_absolute_error` | -0.0004 | -0.0002 | +0.0002 |
| `model.prediction_candidates.candidate_two_estimators_of_p_appears_as_before_the_unification_tail_and_ordering.change_in_signed_error_over_the_top_predicted` | -0.0080 | -0.0053 | +0.0027 |
| `model.prediction_candidates.control_directional_minutes_recency_off_expected_goals_assists.change_in_absolute_bias` | 0.0069 | 0.0050 | -0.0019 |
| `model.prediction_candidates.control_directional_minutes_recency_off_expected_goals_assists.change_in_error_sd` | 0.0050 | 0.0045 | -0.0005 |
| `model.prediction_candidates.control_directional_minutes_recency_off_expected_goals_assists.change_in_mean_absolute_error` | 0.0010 | 0.0007 | -0.0003 |
| `model.prediction_candidates.control_directional_minutes_recency_off_expected_goals_assists.change_in_root_mean_square_error` | 0.0051 | 0.0045 | -0.0006 |
| `model.prediction_candidates.control_directional_minutes_recency_off_minutes.change_in_absolute_bias` | 3.2014 | 3.1418 | -0.0596 |
| `model.prediction_candidates.control_directional_minutes_recency_off_minutes.change_in_error_sd` | 4.0991 | 4.0397 | -0.0594 |
| `model.prediction_candidates.control_directional_minutes_recency_off_minutes.change_in_mean_absolute_error` | 3.8990 | 3.9022 | +0.0032 |
| `model.prediction_candidates.control_directional_minutes_recency_off_minutes.change_in_root_mean_square_error` | 4.3131 | 4.2337 | -0.0794 |
| `model.prediction_candidates.control_directional_minutes_recency_off_points.change_in_absolute_bias` | 0.1198 | 0.1134 | -0.0064 |
| `model.prediction_candidates.control_directional_minutes_recency_off_points.change_in_error_sd` | 0.0690 | 0.0672 | -0.0018 |
| `model.prediction_candidates.control_directional_minutes_recency_off_points.change_in_mean_absolute_error` | 0.0382 | 0.0406 | +0.0024 |
| `model.prediction_candidates.control_directional_minutes_recency_off_points.change_in_root_mean_square_error` | 0.0740 | 0.0750 | +0.001 |
| `model.prediction_candidates.control_directional_minutes_recency_off_tail_and_ordering.change_in_rank_correlation` | -0.0865 | -0.0872 | -0.0007 |
| `model.prediction_candidates.control_directional_minutes_recency_off_tail_and_ordering.change_in_signed_error_over_the_top_predicted` | 0.0626 | 0.0166 | -0.046 |
| `model.prediction_ordering.model.mean_within_gameweek_rank_correlation` | 0.4273 | 0.4306 | +0.0033 |
| `model.prediction_ordering.model.signed_error_over_the_top_20_predicted` | 0.4113 | 0.3292 | -0.0821 |
| `model.prediction_ordering.naive_mean_of_last_5_gameweeks.mean_within_gameweek_rank_correlation` | 0.3301 | 0.3347 | +0.0046 |
| `model.prediction_ordering.naive_mean_of_last_5_gameweeks.signed_error_over_the_top_20_predicted` | 2.5697 | 2.6099 | +0.0402 |
| `model.prediction_ordering.naive_mean_of_season_to_date.mean_within_gameweek_rank_correlation` | 0.3114 | 0.3139 | +0.0025 |
| `model.prediction_ordering.naive_mean_of_season_to_date.signed_error_over_the_top_20_predicted` | 1.0319 | 1.0818 | +0.0499 |
| `stamp.commit` | b9c7d462a404 | 705897c2416b | — |

**Attributing a movement.** Check the constants fingerprint rows first. A figure that moved while the fingerprint held means the code changed and no setting did — a scoring fix, a harness fix, or a bug. A figure that moved *with* the fingerprint means a setting changed, and the constants diff below names which.

### Newly measured

- `model.prediction_coverage.2020_21.gameweeks_contributing_observations` = 33.0000
- `model.prediction_coverage.2020_21.observations_in_the_headline_population` = 10096.0000
- `model.prediction_coverage.2020_21.observations_per_gameweek` = 305.9394
- `model.prediction_coverage.2021_22.gameweeks_contributing_observations` = 33.0000
- `model.prediction_coverage.2021_22.observations_in_the_headline_population` = 9847.0000
- `model.prediction_coverage.2021_22.observations_per_gameweek` = 298.3939

## Regenerating this

See `stats/README.md` for the full recipe and the runtimes. In short: run a sweep with `FPL_CELLS` set, which writes its own provenance; run the calibration diagnostics with `FPL_MODEL_CSV` set; then `fplagent snapshot`, which invokes the R inference and renders this file.

**Constants in force at the sweeps above** are recorded in the provenance sidecar beside the cells file, not inlined here — there are over a hundred of them and the fingerprint is what a reader needs. `fplagent snapshot --constants` prints the full list for a fingerprint.
