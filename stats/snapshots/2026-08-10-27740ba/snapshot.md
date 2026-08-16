# Model and harness accuracy snapshot

Two halves, which must not be blurred together.

**Model accuracy** asks whether the scoring model is right about football. It is measured against outcomes — what players went on to score — and rests on thousands of observations.

**Harness accuracy** asks whether the replay can see anything at all. It rests on four seasons, which is all the archive holds and all it ever will: expected goals begin in 2022-23.

A model can be well calibrated while the harness cannot resolve any change to it. That is this project's actual situation, and reading one half as though it answered the other is how "the instrument could not see it" came to be recorded as "there is no effect" — repeatedly, and in both directions.

## The headline: what this harness can detect at all

Two quantities, both in points over a 38-gameweek season, and both smaller is better:

- **Significance threshold** — the effect that would land exactly at p = 0.05. Anything smaller cannot be called significant however cleanly it was measured.
- **Minimum detectable effect** — the effect this design would actually find most of the time. Larger than the threshold; the gap between them is the difference between "would clear the bar if we saw it" and "would reliably see it".

**The figure is per comparison, not per harness.** The variance behind it comes from the paired differences of one specific setting against the baseline, so a change whose effect is nearly identical in every cell resolves far more finely than one whose effect varies between seasons. That is why every arm gets its own row below, and why **no single row should be quoted as "the harness's resolution"**. One was, for weeks: a sweep's arms were averaged into a single figure per metric, and the average turned out to be dominated by the one arm that disagreed most between seasons.

Each row is a **range**, because two estimators are defensible and the test that would choose between them cannot. *Clustered* treats the four seasons as the independent units, which is right whenever the effect genuinely differs between them and leaves only three degrees of freedom — so the multiple of the standard error needed for p = 0.05 is 3.18, not the familiar 2. *Start-fixed* treats the entry gameweeks as the fixed device they are — the same ones are replayed in every season on purpose, so an offset between them cancels from a paired comparison — which buys five times the degrees of freedom and is valid only where the season component really is zero.

| source sweep | metric | comparison | clustered (3 df) | start-fixed (15 df) | season F | p |
|---|---|---|---:|---:|---:|---:|
| MINHL#1 | **POLICY** | flat (no recency) | 207–271 pts | 40–56 pts | 12.09 | 0.000 |
| MINHL#1 | **POLICY** | half-life 2 | 56–74 pts | 34–48 pts | 1.22 | 0.335 |
| MINHL#1 | **POLICY** | half-life 8 | 86–112 pts | 44–62 pts | 1.71 | 0.208 |
| MINHL#1 | **POLICY** | half-life 20 | 140–183 pts | 51–71 pts | 3.43 | 0.044 |
| MINHL#1 | POLICY | *pooled over the sweep's arms* | 135–177 pts, 3 df | — | — | 0.000 (strictest arm) |
| MINHL#1 | **HOLD** | flat (no recency) | 64–84 pts | 35–49 pts | 1.54 | 0.244 |
| MINHL#1 | **HOLD** | half-life 2 | 67–87 pts | 33–46 pts | 1.84 | 0.184 |
| MINHL#1 | **HOLD** | half-life 8 | 44–58 pts | 28–39 pts | 1.14 | 0.365 |
| MINHL#1 | **HOLD** | half-life 20 | 68–88 pts | 36–51 pts | 1.54 | 0.245 |
| MINHL#1 | HOLD | *pooled over the sweep's arms* | 33–47 pts, 15 df | — | — | 0.184 (strictest arm) |
| MINHL#1 | **armband pinned** | flat (no recency) | 68–89 pts | 38–54 pts | 1.42 | 0.276 |
| MINHL#1 | **armband pinned** | half-life 2 | 79–104 pts | 32–45 pts | 2.80 | 0.076 |
| MINHL#1 | **armband pinned** | half-life 8 | 33–44 pts | 26–37 pts | 0.71 | 0.559 |
| MINHL#1 | **armband pinned** | half-life 20 | 63–83 pts | 35–49 pts | 1.50 | 0.255 |
| MINHL#1 | armband pinned | *pooled over the sweep's arms* | 33–47 pts, 15 df | — | — | 0.076 (strictest arm) |
| MINHL#1 | **nobody doubled** | flat (no recency) | 74–97 pts | 38–53 pts | 1.73 | 0.204 |
| MINHL#1 | **nobody doubled** | half-life 2 | 74–97 pts | 30–43 pts | 2.66 | 0.086 |
| MINHL#1 | **nobody doubled** | half-life 8 | 33–44 pts | 26–37 pts | 0.71 | 0.559 |
| MINHL#1 | **nobody doubled** | half-life 20 | 62–81 pts | 35–49 pts | 1.42 | 0.276 |
| MINHL#1 | nobody doubled | *pooled over the sweep's arms* | 33–46 pts, 15 df | — | — | 0.086 (strictest arm) |

Each pair is *significance threshold–minimum detectable effect*: the first is the effect that would land exactly at p = 0.05, the second the effect the design would actually find most of the time. The metrics, in one clause each — the full definitions are with the components below:

- **POLICY** — buy the opening fifteen, then make the weekly transfer decision all season.
- **HOLD** — buy the opening fifteen and never transfer, re-picking the eleven and the captain every week with substitutes applied.
- **armband pinned** — HOLD with the captain fixed at whoever the model would have captained in the week the squad was bought.
- **nobody doubled** — HOLD with no captain at all.

**The season F test cannot be used to pick an end of the range.** It tests whether the effect differs between seasons at all, and a small p licenses the clustered figure. A large p does *not* license the other end, because at four seasons that test has only **22% power** against a season component large enough to double the clustered variance — it fails to reject four times in five when the thing it looks for is there. So both are reported, with the evidence beside them, and a reader who needs one number should take the conservative end.

For scale: nearly every constant argued over in this project is worth 11 to 34 points a season. Taking the conservative end of every range above, the comparisons run from **44 to 271 points**, so **every one of those constants sits below what any comparison here could detect**. A properly inferred, multiplicity-corrected re-judgement should be *expected* to return "unresolved" for most scoring constants, and that is compatible with the effects being real. The three legitimate responses are to decide on mechanism (does the objective say what the game pays?), to decide on shape (a plateau with a cliff, or monotonicity across several settings, pools information a single comparison cannot), or to buy resolution with more entry gameweeks where the components say that helps.

## Provenance

Every expensive failure in this project's history is a provenance failure rather than an arithmetic one. A whole body of evidence was measured with the transfer gate's minimum-gain threshold at 0.7, the value was retracted to 0.4 three commits later, nothing recorded the link, and a later audit cited the evidence as ground truth. Separately, a six-arm sweep was killed under load after three arms and the gap was invisible until somebody counted rows. So:

| | |
|---|---|
| snapshot taken | 2026-08-10 14:56 EDT |
| commit | `27740badc1f0` |
| branch | `integrate-chips-and-autosub` |
| cells file | `/Users/bbowman/.claude/jobs/0b40ba12/tmp/remeasure/cells.csv` |
| model file | `/Users/bbowman/.claude/jobs/0b40ba12/tmp/remeasure/model.csv` |
| inference for MINHL#1 | `/Users/bbowman/.claude/jobs/0b40ba12/tmp/remeasure/out/cells` |

### Sweep `MINHL#1`

| | |
|---|---|
| ran at commit | `27740badc1f0` |
| constants fingerprint | `ae6ec815e6a5` |
| seasons replayed | 2022-23, 2023-24, 2024-25, 2025-26 |
| entry gameweeks | 1, 6, 11, 16, 21, 26 |
| cells per arm | 4 seasons x 6 entry points = 24 |
| free-transfer bank | 5 for every cell — **historically wrong for 2022-23 and 2023-24**, which ran a two-transfer bank. Deliberate: a setting compared across cells governed by different transfer rules adds a nuisance factor that interacts with the very knobs being swept. It means absolute totals from this grid describe a hypothetical run under one rule set, and only the paired differences carry across. |
| captaincy rungs emitted | yes |

**Arms**

| setting | role | cells run | cells infeasible | status |
|---|---|---:|---:|---|
| half-life 4 (ships) | **baseline** | 24 | 0 | complete |
| flat (no recency) | alternative | 24 | 0 | complete |
| half-life 2 | alternative | 24 | 0 | complete |
| half-life 8 | alternative | 24 | 0 | complete |
| half-life 20 | alternative | 24 | 0 | complete |

Every declared arm ran every cell.

**Invariance checks**

A quantity the change under test must *not* move, and whether it moved. These are worth far more than they cost: a violation shows up in a single cell, where confirming an effect needs the whole grid and still usually fails. Falsification is cheap here and confirmation is not.

Note both directions are informative. For a knob that only touches the weekly transfer decision, an unmoved HOLD is the check passing. For a scoring knob, HOLD *should* move, and these rows are then a description rather than a failure — which of the two a sweep is testing is a judgement the cells file does not carry.

| quantity | arms compared | cells | cells that differ | verdict |
|---|---:|---:|---:|---|
| HOLD (hold the opening fifteen, never transfer) | 4 | 96 | 89 | moved in 89 cells (worst: 2025-26 entered at GW21, half-life 2: 851 against 950) |
| nobody doubled (HOLD with no captain at all) | 4 | 96 | 73 | moved in 73 cells (worst: 2022-23 entered at GW16, flat (no recency): 1144 against 1043) |

### Operator notes

Stamped in by hand at snapshot time. These are the caveats a reader would otherwise have to be told separately, which means eventually not be told.

- Re-measured after the optimiser determinism fix (9e5e1d1): seedFor and repairClubs no longer iterate a map, so Optimize returns one fifteen per input.

## Harness accuracy: where the noise comes from

Most of the replay's "noise" is sensitivity rather than randomness: a hair's-breadth score change flips one discrete transfer, and that transfer changes the squad for every remaining week. Splitting that spread decides what can be done about it.

**It is not ALL sensitivity, and this line used to claim it was.** `Optimize` returns two different fifteens from byte-identical inputs on one engine — measured at 48.643364 against 48.206244 in `XIScore`, about 17 points a season — so part of the spread below is genuine non-determinism that nobody has separated out. It also means a byte-identical invariance may have held by luck. See `TestDiagOptimizerIsNotDeterministic`.

- **Season-to-season disagreement** means the effect genuinely differs between seasons. Only more seasons help, and there are four.
- **Within-season path noise** means the effect is the same and the path through it differed. More entry gameweeks average that away, and entry gameweeks are cheap — linear runtime, no new football needed.

All figures are points per gameweek; multiply by 38 for a season. `season F test p` is the test of whether the season component exists at all, and a small value means it does.

| source sweep | metric | season-to-season | by entry gameweek | path noise | spread of the season means | of which season | of which path | season F test p |
|---|---|---:|---:|---:|---:|---:|---:|---:|
| MINHL#1 | **POLICY** | 1.971 | 0.576 | 2.572 | 2.233 | 78% | 22% | 0.000 |
| MINHL#1 | **HOLD** | 0.604 | 0.000 | 2.006 | 1.018 | 35% | 65% | 0.184 |
| MINHL#1 | **armband pinned** | 0.657 | 0.000 | 2.001 | 1.048 | 39% | 61% | 0.076 |
| MINHL#1 | **nobody doubled** | 0.662 | 0.000 | 1.971 | 1.042 | 40% | 60% | 0.086 |

A season component estimated at zero is not proof of zero — four seasons give it three degrees of freedom, and a component measured at zero and one as large as the whole spread are not distinguishable at that sample size. Read a zero as "the best available reading", not as a fact.

### What each metric is

- **POLICY** — buy the opening fifteen, then make the weekly transfer decision all season. The instrument for constants that are *about* transfers, where the transfer path is the thing being measured.
- **HOLD** — buy the opening fifteen and never transfer, re-picking the eleven and the captain every week with substitutes applied. The instrument for scoring and squad constants, because it drops the noisiest component without dropping anything FPL pays for.
- **armband pinned** — HOLD with the captain fixed at whoever the model would have captained in the week the squad was bought. A candidate quieter instrument, not a metric in its own right.
- **nobody doubled** — HOLD with no captain at all. Removes the armband's variance contribution outright — and, being blind to any rule about who gets doubled, is the reductio of choosing an instrument on variance alone.

## Model accuracy: is the scoring model right about football?

Measured against outcomes rather than against another setting of the model, so these figures do not carry the harness's standard errors: their unit is a player-cutoff or a team-match, not a replayed season, and several rest on thousands of observations where the harness has twenty-four. That makes this the more trustworthy half — and the half that cannot tell you whether acting on a bias would gain points, which is a separate and much harder question this project has answered "no" to five times.

### Does the model's confidence drift through a season?

For each cutoff the model is built from data through that gameweek and every player's predicted points per gameweek is compared with what he actually scored over the next five. Restricted to players the model would consider — 2.0+ predicted points on 45+ expected minutes — because whether it correctly rates a reserve is not the question.

*Population: 4 season pairs, next 5 gameweeks.*

| | n | predicted | actual | ratio | bias | mean absolute error |
|---|---:|---:|---:|---:|---:|---:|
| model built through GW4 | 653 | 2.934 | 2.747 | 0.936 | -0.187 | 1.31 |
| model built through GW8 | 671 | 2.952 | 2.941 | 0.996 | -0.011 | 1.303 |
| model built through GW12 | 677 | 2.976 | 2.95 | 0.992 | -0.025 | 1.38 |
| model built through GW16 | 714 | 3.4 | 2.877 | 0.846 | -0.524 | 1.57 |
| model built through GW20 | 693 | 3.275 | 2.989 | 0.913 | -0.286 | 1.451 |
| model built through GW24 | 699 | 3.242 | 2.864 | 0.883 | -0.379 | 1.398 |
| model built through GW28 | 715 | 3.18 | 2.833 | 0.891 | -0.347 | 1.429 |
| model built through GW32 | 692 | 3.147 | 3.102 | 0.986 | -0.045 | 1.43 |

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

*Population: 4 seasons, cutoffs GW5-33, next 5 gameweeks.*

| | n | mean absolute error | root-mean-square error | bias (predicted minus actual) |
|---|---:|---:|---:|---:|
| minutes — season to date (flat) | 36702 | 23.057 | 30.238 | -0.68 |
| minutes — last 3 gameweeks (flat) | 36702 | 22.811 | 30.845 | 5.509 |
| minutes — ewma, half-life 2 | 36702 | 21.191 | 27.918 | 4.515 |
| minutes — ewma, half-life 4 | 36702 | 21.417 | 27.94 | 2.873 |
| minutes — ewma, half-life 8 | 36702 | 21.949 | 28.633 | 1.293 |
| minutes — ewma, half-life 20 | 36702 | 22.537 | 29.473 | 0.117 |
| points — season to date (flat) | 36702 | 1.347 | 1.774 | -0.032 |
| points — last 3 gameweeks (flat) | 36702 | 1.659 | 2.23 | 0.207 |
| points — ewma, half-life 2 | 36702 | 1.452 | 1.922 | 0.171 |
| points — ewma, half-life 4 | 36702 | 1.366 | 1.788 | 0.11 |
| points — ewma, half-life 8 | 36702 | 1.339 | 1.754 | 0.048 |
| points — ewma, half-life 20 | 36702 | 1.338 | 1.758 | 0 |
| expected goals + assists — season to date (flat) | 36702 | 0.086 | 0.134 | -0.003 |
| expected goals + assists — last 3 gameweeks (flat) | 36702 | 0.107 | 0.17 | 0.01 |
| expected goals + assists — ewma, half-life 2 | 36702 | 0.093 | 0.146 | 0.008 |
| expected goals + assists — ewma, half-life 4 | 36702 | 0.087 | 0.136 | 0.005 |
| expected goals + assists — ewma, half-life 8 | 36702 | 0.086 | 0.133 | 0.001 |
| expected goals + assists — ewma, half-life 20 | 36702 | 0.085 | 0.133 | -0.002 |

**Reading it.** Mean absolute error in the target's own units, **lower is better**. The shape to look for, not the level: **minutes reward sharp recency and rates punish it.** Minutes are a statement about the present, so weighting recent gameweeks removes a bias — a player who lost his place six weeks ago is not a starter. Rates are a statement about quality, and a short window chases finishing variance, so the same weighting trades bias for variance. That is the evidence two shipped constants rest on, and the diagnostic fails if either half stops holding.

### How wrong is the model about one player in one gameweek?

Out-of-sample error predicting a single gameweek from a model built through the gameweek before, split by what the player ACTUALLY scored: Zeros recorded no minutes, Blanks played for two points or fewer, Tickers scored three or four, Haulers five or more. The categories are OpenFPL's (arXiv 2508.09992) so the figures sit beside published ones. Two naive baselines are shown for scale: the mean of the last five gameweeks, which is OpenFPL's own baseline, and the flat season average, which is what FPL's bootstrap publishes.

*Population: 4 seasons, gameweeks 6-38, one gameweek ahead.*

| | n | mean absolute error | root-mean-square error | bias (predicted minus actual) | error sd |
|---|---:|---:|---:|---:|---:|
| points — model — Zeros | 8890 | 1.532 | 1.876 | 1.532 | 1.083 |
| points — model — Blanks | 20321 | 1.245 | 1.643 | 1.078 | 1.24 |
| points — model — Tickers | 3635 | 1.192 | 1.574 | -0.455 | 1.507 |
| points — model — Haulers | 7765 | 4.779 | 5.597 | -4.701 | 3.038 |
| points — model — all categories | 40611 | 1.979 | 2.887 | -0.065 | 2.886 |
| points — naive: mean of last 5 gameweeks — Zeros | 8890 | 1.736 | 2.218 | 1.731 | 1.387 |
| points — naive: mean of last 5 gameweeks — Blanks | 20321 | 1.579 | 2.134 | 1.321 | 1.676 |
| points — naive: mean of last 5 gameweeks — Tickers | 3635 | 1.468 | 1.835 | -0.27 | 1.815 |
| points — naive: mean of last 5 gameweeks — Haulers | 7765 | 4.722 | 5.618 | -4.568 | 3.27 |
| points — naive: mean of last 5 gameweeks — all categories | 40611 | 2.204 | 3.113 | 0.143 | 3.11 |
| points — naive: mean of season to date — Zeros | 8890 | 1.734 | 2.108 | 1.733 | 1.2 |
| points — naive: mean of season to date — Blanks | 20321 | 1.302 | 1.708 | 0.997 | 1.387 |
| points — naive: mean of season to date — Tickers | 3635 | 1.249 | 1.56 | -0.651 | 1.417 |
| points — naive: mean of season to date — Haulers | 7765 | 4.972 | 5.805 | -4.927 | 3.07 |
| points — naive: mean of season to date — all categories | 40611 | 2.093 | 3.016 | -0.122 | 3.013 |
| minutes — model — Zeros | 8890 | 40.101 | 45.189 | 40.101 | 20.832 |
| minutes — model — Blanks | 20321 | 23.018 | 29.188 | -8.32 | 27.977 |
| minutes — model — Tickers | 3635 | 23.548 | 30.467 | -19.162 | 23.687 |
| minutes — model — Haulers | 7765 | 23.081 | 30.233 | -20.11 | 22.575 |
| minutes — model — all categories | 40611 | 26.817 | 33.633 | -0.945 | 33.619 |
| minutes — naive: mean of last 5 gameweeks — Zeros | 8890 | 45.092 | 50.849 | 45.092 | 23.501 |
| minutes — naive: mean of last 5 gameweeks — Blanks | 20321 | 21.989 | 30.126 | -2.936 | 29.982 |
| minutes — naive: mean of last 5 gameweeks — Tickers | 3635 | 23.999 | 35.957 | -17.448 | 31.44 |
| minutes — naive: mean of last 5 gameweeks — Haulers | 7765 | 23.044 | 35.189 | -17.792 | 30.359 |
| minutes — naive: mean of last 5 gameweeks — all categories | 40611 | 27.428 | 37.049 | 3.438 | 36.889 |
| minutes — naive: mean of season to date — Zeros | 8890 | 43.518 | 49.536 | 43.518 | 23.663 |
| minutes — naive: mean of season to date — Blanks | 20321 | 25.501 | 33.146 | -11.294 | 31.162 |
| minutes — naive: mean of season to date — Tickers | 3635 | 29.461 | 41.116 | -25.594 | 32.179 |
| minutes — naive: mean of season to date — Haulers | 7765 | 28.456 | 40.516 | -25.763 | 31.27 |
| minutes — naive: mean of season to date — all categories | 40611 | 30.365 | 39.396 | -3.342 | 39.254 |
| expected goals + assists — model — Zeros | 8890 | 0.089 | 0.136 | 0.089 | 0.102 |
| expected goals + assists — model — Blanks | 20321 | 0.114 | 0.176 | 0.026 | 0.174 |
| expected goals + assists — model — Tickers | 3635 | 0.131 | 0.205 | -0.013 | 0.205 |
| expected goals + assists — model — Haulers | 7765 | 0.232 | 0.38 | -0.163 | 0.344 |
| expected goals + assists — model — all categories | 40611 | 0.132 | 0.226 | 0 | 0.226 |
| expected goals + assists — naive: mean of last 5 gameweeks — Zeros | 8890 | 0.092 | 0.158 | 0.092 | 0.128 |
| expected goals + assists — naive: mean of last 5 gameweeks — Blanks | 20321 | 0.125 | 0.194 | 0.034 | 0.192 |
| expected goals + assists — naive: mean of last 5 gameweeks — Tickers | 3635 | 0.142 | 0.222 | -0.008 | 0.222 |
| expected goals + assists — naive: mean of last 5 gameweeks — Haulers | 7765 | 0.242 | 0.39 | -0.156 | 0.358 |
| expected goals + assists — naive: mean of last 5 gameweeks — all categories | 40611 | 0.141 | 0.241 | 0.006 | 0.241 |
| expected goals + assists — naive: mean of season to date — Zeros | 8890 | 0.092 | 0.15 | 0.092 | 0.118 |
| expected goals + assists — naive: mean of season to date — Blanks | 20321 | 0.112 | 0.176 | 0.015 | 0.176 |
| expected goals + assists — naive: mean of season to date — Tickers | 3635 | 0.131 | 0.211 | -0.031 | 0.208 |
| expected goals + assists — naive: mean of season to date — Haulers | 7765 | 0.24 | 0.393 | -0.177 | 0.351 |
| expected goals + assists — naive: mean of season to date — all categories | 40611 | 0.134 | 0.232 | -0.009 | 0.232 |

**Reading it.** Mean absolute error and root-mean-square error are both in the target's own units and **lower is better**. Bias is predicted minus actual, so positive means over-prediction, and error sd is the spread around that bias — root-mean-square error squared is exactly bias squared plus error sd squared. **The categories condition on the outcome, which rewards a noisier predictor in the extreme buckets**: a predictor that fires more high numbers will look better on Haulers while being worse calibrated at the top of its own distribution, so read the Haulers column beside the calibration and ordering tables and never on its own. This instrument ranks candidates and cannot price them — the replay does that, and a better predictor can make a worse policy.

### Do the players the model rates at 5.0 score 5.0?

The same one-gameweek-ahead predictions grouped by what was PREDICTED rather than by what happened, so the table reads at the level decisions are made at.

*Population: 4 seasons, gameweeks 6-38, one gameweek ahead.*

| | n | predicted | actual | ratio |
|---|---:|---:|---:|---:|
| predicted under 1.0 | 5517 | 0.634 | 0.862 | 1.359 |
| predicted 1.0 to 2.0 | 11873 | 1.513 | 1.73 | 1.143 |
| predicted 2.0 to 3.0 | 12136 | 2.486 | 2.604 | 1.048 |
| predicted 3.0 to 4.0 | 7261 | 3.412 | 3.391 | 0.994 |
| predicted 4.0 to 5.0 | 2207 | 4.41 | 3.99 | 0.905 |
| predicted 5.0 to 6.0 | 846 | 5.43 | 4.669 | 0.86 |
| predicted 6.0 and above | 771 | 7.593 | 6.412 | 0.845 |

**Reading it.** The ratio is actual divided by predicted, so **1.000 is perfect and below 1.000 means the band is over-predicted**. The top band is where a transfer search picks, so its ratio matters more than the aggregate: a bias shared by every player is invisible to an argmax, and this project has measured that correcting one costs points.

### Is a candidate change safe for an argmax, or a variance trade?

Each arm of the benchmark paired against the shipped config on the same observations. Two of the arms are controls rather than proposals: switching minutes recency off must make the minutes error worse, and switching the vice-captain fallback off must change nothing at all, since it alters how a played-out gameweek is scored and not what the model predicts.

*Population: 4 seasons, gameweeks 6-38, one gameweek ahead.*

| | n | change in mean absolute error | change in root-mean-square error | change in absolute bias | change in error sd | change in signed error over the top predicted | change in rank correlation |
|---|---:|---:|---:|---:|---:|---:|---:|
| CONTROL, directional: minutes recency off — points | 40611 | 0.038 | 0.074 | 0.12 | 0.069 | — | — |
| CONTROL, directional: minutes recency off — minutes | 40611 | 3.899 | 4.313 | 3.201 | 4.099 | — | — |
| CONTROL, directional: minutes recency off — expected goals + assists | 40611 | 0.001 | 0.005 | 0.007 | 0.005 | — | — |
| CONTROL, directional: minutes recency off — tail and ordering | 0 | — | — | — | — | 0.063 | -0.086 |
| CONTROL, invariance: vice-captain fallback off — points | 40611 | 0 | 0 | 0 | 0 | — | — |
| CONTROL, invariance: vice-captain fallback off — minutes | 40611 | 0 | 0 | 0 | 0 | — | — |
| CONTROL, invariance: vice-captain fallback off — expected goals + assists | 40611 | 0 | 0 | 0 | 0 | — | — |
| CONTROL, invariance: vice-captain fallback off — tail and ordering | 0 | — | — | — | — | 0 | 0 |
| CANDIDATE: two estimators of P(appears), as before the unification — points | 40611 | 0 | 0 | 0.001 | 0 | — | — |
| CANDIDATE: two estimators of P(appears), as before the unification — minutes | 40611 | 0 | 0 | 0 | 0 | — | — |
| CANDIDATE: two estimators of P(appears), as before the unification — expected goals + assists | 40611 | 0 | 0 | 0 | 0 | — | — |
| CANDIDATE: two estimators of P(appears), as before the unification — tail and ordering | 0 | — | — | — | — | -0.008 | 0 |
| CANDIDATE: rate recency, half-life 8 — points | 40611 | -0.012 | -0.011 | 0.025 | -0.012 | — | — |
| CANDIDATE: rate recency, half-life 8 — minutes | 40611 | 0 | 0 | 0 | 0 | — | — |
| CANDIDATE: rate recency, half-life 8 — expected goals + assists | 40611 | 0 | 0 | 0 | 0 | — | — |
| CANDIDATE: rate recency, half-life 8 — tail and ordering | 0 | — | — | — | — | -0.078 | 0.003 |

**Reading it.** These are differences, so **negative means better** for the error columns. The distinction that matters is whether an improvement came from shrinking the systematic part of the error (bias reduction, safe for an argmax, because removing a systematic error cannot reorder candidates by chance) or from shrinking the spread while the bias grew (a bias-for-variance trade, dangerous — the recorded reason recency on minutes gained points and recency on rates lost them). Read the tail and ordering rows beside the error rows: a candidate that lowers aggregate error while pushing the tail figure away from zero has the better-predictor-worse-policy shape.

### Did every season reach the prediction benchmark's sample?

How many gameweeks and how many player-gameweeks each season contributed to the benchmark's headline population — players who played sixty or more minutes in one of the previous five gameweeks their club played.

*Population: 4 seasons, gameweeks 6-38, one gameweek ahead.*

| | n | gameweeks contributing observations | observations in the headline population | observations per gameweek |
|---|---:|---:|---:|---:|
| 2022-23 | 9865 | 32 | 9865 | 308.281 |
| 2023-24 | 10205 | 33 | 10205 | 309.242 |
| 2024-25 | 10286 | 33 | 10286 | 311.697 |
| 2025-26 | 10255 | 33 | 10255 | 310.758 |

**Reading it.** **Higher is better and even across seasons is what matters** — the seasons should contribute roughly equally, and a season contributing nothing means the population filter is reading an archive column that season does not carry. That is not a hypothetical: the per-gameweek `starts` field is empty for all of 2021-22 and for 2022-23 before GW16, and a filter reading it silently made a four-season figure into a three-season one while every other table stayed plausible. One gameweek missing from 2022-23 is expected — its GW7 was postponed outright.

### Does the model rank players correctly, and is its top over-rated?

Spearman's rank correlation — the ordinary correlation computed on ranks rather than values — between predicted and actual points within each gameweek, averaged over gameweeks. Beside it, the signed error over the twenty highest-predicted players in each gameweek, which is roughly the set a transfer search chooses between.

*Population: 4 seasons, gameweeks 6-38, one gameweek ahead.*

| | n | mean within-gameweek rank correlation | signed error over the top 20 predicted |
|---|---:|---:|---:|
| model | 131 | 0.427 | 0.411 |
| naive: mean of last 5 gameweeks | 131 | 0.33 | 2.57 |
| naive: mean of season to date | 131 | 0.311 | 1.032 |

**Reading it.** For the rank correlation, +1 is a perfect ordering, 0 is no ordering information and **higher is better**. This axis exists because the optimiser consumes an ordering and never a level, which is why the bonus term is kept despite being badly calibrated. For the tail figure, **positive means the top of the predicted distribution is over-rated** and closer to zero is better — it is the winner's curse as a measured number rather than an inference.

### Are the terms FPL pays as a step scaled as a step?

Appearance points and the clean sheet are paid at sixty minutes, not prorated: a starter taken off at seventy banks both in full. The model scales them by an estimated probability of reaching sixty minutes, and this compares that estimate against how often players in each minutes band actually reached it.

*Population: 2934 player-seasons, 4 seasons, 20+ gameweeks.*

| | n | model credits | actually reached 60 minutes | error |
|---|---:|---:|---:|---:|
| players averaging 0-10 minutes a gameweek | 1323 | 0.006 | 0.007 | 0 |
| players averaging 10-20 minutes a gameweek | 214 | 0.103 | 0.123 | -0.02 |
| players averaging 20-30 minutes a gameweek | 230 | 0.202 | 0.227 | -0.025 |
| players averaging 30-40 minutes a gameweek | 210 | 0.306 | 0.335 | -0.029 |
| players averaging 40-50 minutes a gameweek | 217 | 0.423 | 0.461 | -0.039 |
| players averaging 50-60 minutes a gameweek | 193 | 0.543 | 0.576 | -0.033 |
| players averaging 60-70 minutes a gameweek | 169 | 0.667 | 0.7 | -0.033 |
| players averaging 70-80 minutes a gameweek | 185 | 0.796 | 0.809 | -0.013 |
| players averaging 80-91 minutes a gameweek | 156 | 0.935 | 0.921 | 0.013 |

**Reading it.** Error is what the model credits minus what happens, so **positive means the model over-credits**. Unlike the clean-sheet bias this one is *not* uniform across players, so it mis-ranks a part-timer against an ever-present — an ordering error, which the optimiser does see.

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

*Population: 321 moves judged, 4 seasons x 3 start points, min_gain 0.40.*

| | n | median | mean |
|---|---:|---:|---:|
| buy side (player bought) | 321 | -0.609 | -0.278 |
| sell side (player sold) | 321 | -0.201 | -0.216 |
| asymmetry (buy minus sell) | 321 | -0.408 | -0.062 |
| sell error: sold player played again | 278 | -0.052 | — |
| sell error: sold player never played again | 43 | -2.474 | — |

**Reading it.** **Negative means the player was over-rated.** The asymmetry row is the one that matters: a model that is simply badly calibrated moves both sides together, while a search that hunts the top of a noisy estimate distribution moves the buy side further. Note the move count in the population line — the transfer gate decides which moves exist to judge, so two runs at different gate settings measure different populations and are not a reproduction of each other.

### What is not measured here, and why

**No section here prices a change in points.** Every figure above is measured against outcomes — what the players went on to do — and the unit is a player-gameweek or a team-match rather than a replayed season. That is what makes these the more trustworthy half: several rest on tens of thousands of observations where the harness half has twenty-four cells. It is also what they cannot do. A predictor that wins here is a candidate worth spending replay time on, not a candidate already proved, and this project has a measured case where about 2 per cent lower out-of-sample error cost roughly 49 points a season: a transfer policy is an argmax living in the tail of the estimate distribution, so accuracy bought on the average player is paid for with noise where the search looks.

The predictor comparison for minutes, points and expected goals **is now runnable** and appears above as "How much should a predictor weight recent gameweeks?". An earlier edition of this snapshot had to record its absence: the figures were frozen prose in `internal/analysis` and `internal/recent` with no code that could recompute them, which is precisely the orphaned measurement this artefact exists to prevent. Read the relative columns rather than the levels when comparing against those comments — the population behind the original was never written down, and it predates the doubles-counting fix that changed what a gameweek means in this archive.

## Change since the previous snapshot

Compared against `2026-08-10-9af2315`.

### Figures that moved

| figure | previous | now | change |
|---|---:|---:|---:|
| `harness.mde_season.coarsest` | 273 | 271 | -2 |
| `harness.mde_season.finest` | 39 | 44 | +5 |
| `harness.minhl_1.mde_season.hold` | 46 | 47 | +1 |
| `harness.minhl_1.mde_season.hold.flat_no_recency.clustered` | 82 | 84 | +2 |
| `harness.minhl_1.mde_season.hold.flat_no_recency.startfixed` | 42 | 49 | +7 |
| `harness.minhl_1.mde_season.hold.half_life_2.clustered` | 92 | 87 | -5 |
| `harness.minhl_1.mde_season.hold.half_life_20.clustered` | 77 | 88 | +11 |
| `harness.minhl_1.mde_season.hold.half_life_20.startfixed` | 54 | 51 | -3 |
| `harness.minhl_1.mde_season.hold.half_life_8.clustered` | 54 | 58 | +4 |
| `harness.minhl_1.mde_season.hold_fixedcap` | 45 | 47 | +2 |
| `harness.minhl_1.mde_season.hold_fixedcap.flat_no_recency.clustered` | 87 | 89 | +2 |
| `harness.minhl_1.mde_season.hold_fixedcap.flat_no_recency.startfixed` | 48 | 54 | +6 |
| `harness.minhl_1.mde_season.hold_fixedcap.half_life_2.clustered` | 108 | 104 | -4 |
| `harness.minhl_1.mde_season.hold_fixedcap.half_life_2.startfixed` | 44 | 45 | +1 |
| `harness.minhl_1.mde_season.hold_fixedcap.half_life_20.clustered` | 71 | 83 | +12 |
| `harness.minhl_1.mde_season.hold_fixedcap.half_life_20.startfixed` | 50 | 49 | -1 |
| `harness.minhl_1.mde_season.hold_fixedcap.half_life_8.clustered` | 39 | 44 | +5 |
| `harness.minhl_1.mde_season.hold_nocap` | 44 | 46 | +2 |
| `harness.minhl_1.mde_season.hold_nocap.flat_no_recency.clustered` | 94 | 97 | +3 |
| `harness.minhl_1.mde_season.hold_nocap.flat_no_recency.startfixed` | 47 | 53 | +6 |
| `harness.minhl_1.mde_season.hold_nocap.half_life_2.clustered` | 101 | 97 | -4 |
| `harness.minhl_1.mde_season.hold_nocap.half_life_2.startfixed` | 42 | 43 | +1 |
| `harness.minhl_1.mde_season.hold_nocap.half_life_20.clustered` | 69 | 81 | +12 |
| `harness.minhl_1.mde_season.hold_nocap.half_life_20.startfixed` | 51 | 49 | -2 |
| `harness.minhl_1.mde_season.hold_nocap.half_life_8.clustered` | 39 | 44 | +5 |
| `harness.minhl_1.mde_season.policy` | 174 | 177 | +3 |
| `harness.minhl_1.mde_season.policy.flat_no_recency.clustered` | 273 | 271 | -2 |
| `harness.minhl_1.mde_season.policy.flat_no_recency.startfixed` | 54 | 56 | +2 |
| `harness.minhl_1.mde_season.policy.half_life_2.clustered` | 58 | 74 | +16 |
| `harness.minhl_1.mde_season.policy.half_life_2.startfixed` | 45 | 48 | +3 |
| `harness.minhl_1.mde_season.policy.half_life_20.clustered` | 177 | 183 | +6 |
| `harness.minhl_1.mde_season.policy.half_life_20.startfixed` | 70 | 71 | +1 |
| `harness.minhl_1.mde_season.policy.half_life_8.clustered` | 107 | 112 | +5 |
| `harness.minhl_1.mde_season.policy.half_life_8.startfixed` | 60 | 62 | +2 |
| `harness.minhl_1.sd_resid.hold` | 1.961 | 2.006 | +0.045 |
| `harness.minhl_1.sd_resid.hold_fixedcap` | 1.937 | 2.001 | +0.064 |
| `harness.minhl_1.sd_resid.hold_nocap` | 1.906 | 1.971 | +0.065 |
| `harness.minhl_1.sd_resid.policy` | 2.497 | 2.572 | +0.075 |
| `harness.minhl_1.sd_season.hold` | 0.564 | 0.604 | +0.04 |
| `harness.minhl_1.sd_season.hold_fixedcap` | 0.634 | 0.657 | +0.023 |
| `harness.minhl_1.sd_season.hold_nocap` | 0.641 | 0.662 | +0.021 |
| `harness.minhl_1.sd_season.policy` | 1.948 | 1.971 | +0.023 |
| `harness.minhl_1.share_season.hold` | 33 | 35 | +2 |
| `harness.minhl_1.share_season.policy` | 79 | 78 | -1 |
| `harness.minhl_1.sig_season.hold` | 32 | 33 | +1 |
| `harness.minhl_1.sig_season.hold_fixedcap` | 32 | 33 | +1 |
| `harness.minhl_1.sig_season.hold_nocap` | 32 | 33 | +1 |
| `harness.minhl_1.sig_season.policy` | 133 | 135 | +2 |
| `stamp.commit` | 9af23159141a | 27740badc1f0 | — |
| `stamp.dirty` | true | false | — |
| `sweep.MINHL#1.commit` | 9af23159141a | 27740badc1f0 | — |
| `sweep.MINHL#1.digest` | 88bbca18a1ee | ae6ec815e6a5 | — |
| `sweep.MINHL#1.invariance_differs.hold_hold_the_opening_fifteen_never_transfer` | 90 | 89 | -1 |
| `sweep.MINHL#1.invariance_differs.nobody_doubled_hold_with_no_captain_at_all` | 72 | 73 | +1 |

**Attributing a movement.** Check the constants fingerprint rows first. A figure that moved while the fingerprint held means the code changed and no setting did — a scoring fix, a harness fix, or a bug. A figure that moved *with* the fingerprint means a setting changed, and the constants diff below names which.

## Regenerating this

See `stats/README.md` for the full recipe and the runtimes. In short: run a sweep with `FPL_CELLS` set, which writes its own provenance; run the calibration diagnostics with `FPL_MODEL_CSV` set; then `fplagent snapshot`, which invokes the R inference and renders this file.

**Constants in force at the sweeps above** are recorded in the provenance sidecar beside the cells file, not inlined here — there are over a hundred of them and the fingerprint is what a reader needs. `fplagent snapshot --constants` prints the full list for a fingerprint.
