# The gated prior blend, measured 2026-08-12

Run by hand after the agent building it was stopped. `go run ./cmd/priorblend`, six
seasons, gameweeks 1-38, one gameweek ahead. The 17 MB cells file is not committed;
regenerate with `go run ./cmd/priorblend -csv <path>` and read it with
`scripts/pbsummary.py` or `stats/prediction_inference.R`.

## The gate does what the by-population arm predicted, exactly

The tool's own wiring check: **1,346 of 1,346 priors identical** between the
by-population arm and `prior_half_life 1` as the model now builds it. The two are the
same index, so the gate wired into the model reproduces the hand-restricted arm
rather than approximating it.

| population | moved | mean Score diff |
|---|---|---|
| the case, injury shaped (played some of last season) | **83.1%** | **+0.120** |
| the case, absent (no minutes at all) | **0.0%** | **0.000** |

The absent half is now completely inert — which is the point. It falls through to
`shrinkToLeague`, the shipped answer for a player with no usable history.

## Error, on the population the setting exists for

| arm | bias | spread |
|---|---|---|
| shipped | −0.2244 | 2.1536 |
| gated, half-life 1 | **−0.0813** | 2.1498 |

**Bias** is being wrong in a consistent direction — here, under-rating. It falls 64%.
**Spread** is the scattered half, and it does not rise, so this is not the
accuracy-for-noise trade this file has been burned by five times.

## The measurement that decides it: ordering

The optimiser consumes an ordering and never a level, so this is closer to points than
any error figure. Higher is better; the field's level is 0.685.

| arm | rank correlation | vs shipped |
|---|---|---|
| shipped | 0.6854 | — |
| half-life 0.5 | 0.6857 | **+0.0003** |
| half-life 1 (gated) | 0.6852 | −0.0002 |
| half-life 2 | 0.6847 | −0.0007 |

**Ungated, the same comparison cost −0.0023 at a season-clustered t of −3.7.** Gated it
is −0.0002, and at half-life 0.5 the ordering is nominally *better* than shipped. The
gate removes essentially all of the damage, which is what the earlier by-population
arm predicted and is now shown on the model itself.

The tail figure moves the same way: over-rating of the highest-predicted players falls
from **0.2206 to 0.2016**. That matters more than it looks, because the top of the
predicted distribution is exactly where an argmax picks.

## What this does NOT establish

**Nothing here is a points figure.** A rank correlation has no conversion into a season
total, and no replay has been run — the record's own most-repeated finding is that a
better predictor can make a worse policy. ⚠️ And `prior_half_life` **cannot be swept
through the shipped CLI**: `FPL_WEIGHT=prior_half_life=v` sets `Weights.PriorHalfLife`,
which only the live path reads, while the replay needs `SimConfig.PriorHalfLife` *and*
`SimConfig.OlderPriors`. Wiring that is the next job; until it is done the recorded
−7 points a season has unrecoverable provenance.

**The recommendation stands at: do not ship yet.** The gate is the right shape and the
evidence for it is now on the model rather than on a hand-restricted population. What
is missing is a replay on `HOLD` of the gated arm.

---

## ⚠️ Four corrections, from the statistics and code reviews of `41e7038`

Marked in place rather than applied, per the convention that a later correction names
the commit that caused it. Every figure above reproduces; what changes is what they
support. The full re-derivation is in `cmd/priorblend`'s package comment.

**1. "1,346 of 1,346 priors identical" is an inflated denominator.** The wiring check
unioned the older seasons' player codes into it, and both indexes only ever emit a code
present in the *prior* season — so a code found only in an older season was "no prior"
on both sides and counted as agreement without either index having been asked anything.
Corrected the count is **666 to 865** across the six pairs, still with no disagreement.
The conclusion is untouched; the number was not the number of priors compared.

**2. "At half-life 0.5 the ordering is nominally better" is one season.** 99% of that
+0.0003 is 2022-23 — the season this instrument behaves worst in, with a top-twenty
signed error of +1.92 over GW16-38 against ±0.25 for every other season-half. Running
the two rungs below 0.5 changes the reading for the better and for a different reason:
anchored at exactly 0.000000 at half-life 0, the gated curve reads
**0 → +0.000142 → +0.000431 → +0.000332 → −0.000204 → −0.000752** at 0, 0.125, 0.25,
0.5, 1 and 2. That is a rise to an interior maximum near 0.25 and then a monotone
decline through zero — a shape, and it survives dropping 2022-23 at about half
strength. So the ladder is **not** monotone, and "the gate removes essentially all of
the damage" is the wrong shape of statement: the gate removes a **level**, +0.00204 to
within 0.5% across a fourfold dose change, and leaves the **slope** exactly as the
ungated run had it. More of this feature still orders the field worse.

**3. The tail sentence points the wrong way.** Pooled it moves toward zero, 0.2206 to
0.2016 — but **five of six seasons move away from it**, and 2022-23 alone supplies more
than the whole pooled change. Excluding it the paired difference is +0.0131 at a
season-clustered t of +3.06. Nothing here says the top of the distribution got less
over-rated, which is the direction that matters for an argmax.

**4. "The spread does not rise, so this is not the accuracy-for-noise trade" is too
strong.** The spread's own t fades with dose — −4.44, −2.40, −1.41 at half-lives 0.5, 1
and 2 — which is that trade arriving at the large end rather than being absent. It is a
reason to prefer a small half-life, not a reason to dismiss the concern.

**And one defect these figures were produced through.** The gate was applied on the two
archive paths and **silently defeated on the live one**: `recent.blendPast` drops
zero-minute rows before the gate can read them, so a player who sat out last season was
handed the last season he *played*, which carries minutes and therefore never reaches
`shrinkToLeague`. Fixed in `39c3e8a`. It does not touch anything above — the benchmark
runs through `newPriorIndexMulti` — but "the absent half falls through to
`shrinkToLeague`" was true of the measurement and false of `cmd/fplagent`.

**The recommendation is unchanged: do not ship yet.** It now rests on the mechanism
rather than on the ordering figure, since the gated run is in-sample with respect to the
gate's own construction — same six seasons, and the population split that motivated the
gate was introduced by the commit that first ran it.

---

## The setting is about ADAPTATION SPEED, and the decay confirms it

⚠️ **Everything above is pooled over a whole season, and that is the wrong cut.** The blend
does not change where the model ends up on a returning player — it changes **when he gets
there**. Without it his estimate starts from a wrecked season and is dragged up by this
season's minutes as they accumulate; with it he starts closer to the truth. So the value is
a **transient**, largest before any current-season football exists and decaying to nothing
as the prior is swamped.

Cut by gameweek — a proxy for how much current-season evidence exists — on the injury-shaped
population, `prior_half_life 1` against shipped. **Bias** is predicted minus actual, so
negative is under-rating; "bias gain" is how much of that under-rating the blend removes.

| gameweeks | n | bias, shipped | bias, blended | **bias gain** | mae gain |
|---|---:|---:|---:|---:|---:|
| 1-3 | 1,151 | −0.3256 | +0.0485 | **0.2771** | −0.1374 |
| 4-6 | 1,173 | −0.2648 | −0.0236 | **0.2412** | −0.0927 |
| 7-10 | 1,482 | −0.2154 | −0.0330 | **0.1824** | −0.0679 |
| 11-15 | 1,935 | −0.2523 | −0.1075 | **0.1449** | −0.0546 |
| 16-22 | 2,579 | −0.2073 | −0.0893 | **0.1180** | −0.0417 |
| 23-38 | 6,017 | −0.1978 | −0.1173 | **0.0805** | −0.0256 |

**Monotone across all six buckets, 0.277 falling to 0.081 — a factor of 3.4.** That is the
mechanism confirmed rather than assumed: the advantage is largest when the prior *is* the
estimate and shrinks as football accumulates. Reproduce with `scripts/pbdecay.py`.

**So the pooled figure understates the peak and overstates the tail at once.** 42% of the
observations sit in the final bucket, where the gain is smallest — a transient averaged
across a season looks like a small permanent effect, which is the shape that fails to
resolve.

⚠️ **And a new finding that complicates it: mean absolute error moves the OTHER way in every
bucket.** The blend reduces the systematic under-rating and *increases* average absolute
error, most in the first three gameweeks and least at the end. At GW1-3 it also overshoots,
+0.0485 from −0.3256. Bias down and absolute error up is the shape this file warns about —
bias traded for scatter — and it sits against the pooled spread, which fell slightly.
**Unresolved**: MAE and spread need not move together on a skewed error distribution, and
nobody has decomposed which is happening. Do that before quoting the bias gain as a clean win.

**This is a proxy, not the real cut.** Gameweek number stands in for weeks-since-return, and
players return at different times. The sharper measurement buckets each player by his own
appearances since returning, and needs a gameweek-varying population in `classify`. The
direction is unambiguous either way.

---

## The decomposition: the two columns are ONE column

⚠️ **The "bias down, absolute error up" puzzle above is resolved, and it is not two effects.**
The rise in mean absolute error *is* the fall in bias, multiplied by a constant near **0.36**
that is a property of the error distribution's shape and nothing else. Across all 21
bucket-by-dose cells the ratio sits in 0.30-0.42. They decay in step because only one thing
is decaying: the size of the shift.

**Why, proven rather than argued.** **57% of this population recorded no minutes at all** in
the gameweek being scored. For them the error is `predicted − 0 = predicted`, so it is
non-negative by construction and any upward shift raises absolute error one-for-one — checked
as an exact identity, mean absolute error over that category equalling mean prediction to
machine precision. So the median error is positive while the mean is negative, and absolute
error (which follows the median) and squared error (which follows the mean) are *obliged* to
disagree under a level shift. The pooled-spread-versus-MAE contradiction needed no
reconciling; it was two loss functions correctly describing one intervention.

⚠️ **So "bias traded for scatter" is REFUTED at the shipped candidate dose.** Under the
squared-error split both halves improve in every bucket, and the gain is 73% bias². The trade
is real but lives at **half-life 2**, where GW1-3 bias overshoots to +0.174 and variance
turns negative. The concern belongs on the *dose*, not on the effect.

## But the verdict hardens anyway, on a better argument

**82% of the bias improvement lands on players who returned nothing.** Decomposed by realised
outcome — a partition independent of the arm, so the split is exact:

| realised | n | share | contribution to the bias gain |
|---|---:|---:|---:|
| recorded no minutes | 8,121 | 56.6% | **+0.0635** |
| played, ≤2 points | 4,223 | 29.5% | **+0.0533** |
| 3-4 points | 642 | 4.5% | +0.0078 |
| 5+ points | 1,351 | 9.4% | +0.0186 |

Only 18% closes the gap on players who actually scored. **The mean moves toward zero because
two large opposite errors cancel better, not because either got smaller** — which is the
file's own standing rule arriving intact: *a measured bias does not imply a correction
exists*.

**And the correction is a fixed-size shift, not one sized to the error.** On expected
minutes, where there is ground truth, the blend adds a near-constant **+3.36 minutes a
gameweek** while the error it is correcting ranges from **+1.25 to −3.16** across seasons. In
2020-21 the model was already over-predicting and the blend made it worse; in 2025-26, the
one season with a real under-prediction, it lands almost exactly right. Shipped is
near-unbiased on minutes at −3.9%; blended overshoots to +8.0%.

**The blend also aims its extra points worse than a flat multiplier would**: the shift's
covariance with realised points is **+0.058**, against **+0.163** for simply scaling shipped
predictions by the same 14.1%.

**And it buys no ordering at the candidate dose** — the statistic the optimiser actually
consumes reads +0.0016 at a season-clustered t of **0.83**. Within this population ordering
peaks near **half-life 0.5** (t = +3.09) and is negative by 2. With the whole-field maximum
near 0.25, three independent readings agree **the candidate dose of 1 is past the useful
range.**

**What survives untouched:** the transient is real, the decay is monotone across six buckets,
and the squared-error gain at GW1-3 is three times the end-of-season figure and almost pure
bias². The feature does what it claims. The question is whether a level shift on a
57%-zeros population is the right instrument, and the evidence says it is a blunt one.

⚠️ **Not recommended despite being tempting:** gating additionally on the older season's
quality. That split was chosen in-sample, on these same six seasons, at a bar the tool
records as asserted rather than measured. Acting on it is an argmax over a gate the data has
already seen.

---

## The dose ladder: 0.5 is the rung, and it is a shape rather than an argmax

Re-run at **0.25 and 0.5** beside the old candidate of 1, after three independent readings
said 1 was past the useful range. Injury-shaped population, six seasons.

| | shipped | 0.25 | 0.5 | 1 |
|---|---:|---:|---:|---:|
| bias (under-rating) | −0.2244 | −0.2007 | **−0.1446** | −0.0813 |
| mean absolute error | 1.1223 | 1.1284 | 1.1476 | 1.1735 |
| spread | 2.1536 | 2.1514 | **2.1494** | 2.1498 |
| whole-field ordering | 0.6854 | **0.6858** | 0.6857 | 0.6852 |
| top-of-field over-rating | 0.2206 | 0.2062 | 0.2058 | **0.2016** |

**The efficiency frontier breaks — but only in the early window.** Cost in absolute error per unit
of under-rating removed:

| half-life | pooled | in gameweeks 1-3 |
|---|---:|---:|
| 0.25 | **0.257** | **0.288** |
| 0.5 | **0.317** | **0.314** |
| 1 | 0.358 | **0.496** |

⚠️ **Read the two columns separately; an earlier version of this paragraph did not, and
overstated the shape.** In **gameweeks 1-3** the exchange rate is near-constant from 0.25 to
0.5 (0.288 → 0.314, +9%) and then degrades **58%** at dose 1 — a genuine break, and it is
where the shift starts pushing observations past their own zero and the bias overshoots to
+0.0485. **Pooled there is no break at all**: 0.257 → 0.317 → 0.358 rises steadily and if
anything decelerates. So "the frontier is flat to 0.5 and then breaks" was a shape
generalised from the window where it is sharpest. The break is real *in the early window*,
which is the window the feature exists for — and that is a narrower claim than the one first
written here.

**Read against the other columns, 0.5 is the rung.** It captures **56%** of dose-1's bias
reduction for **49%** of its absolute-error cost, has the **best spread of any arm**, and
sits within 0.0001 of the best ordering. 0.25 is defensible but timid — it takes only 17% of
the available bias reduction.

⚠️ **Two honest limits.** The ordering differences across the whole ladder span 0.0006 on a
level of 0.685 and **none of them resolves** — 0.25 winning it is not evidence that 0.25 is
best, and the reason to prefer 0.5 is the cost ratio and the spread, not the ordering. And
the **top-of-field over-rating keeps improving with dose all the way to 1**, which points the
other way; it is the one column that prefers the rung the others reject, and nobody has
reconciled that.

**None of this changes the verdict.** The instrument is still a level shift landing 82% of
its correction on players who returned nothing, and it still buys no ordering that resolves.
What the ladder settles is which rung a replay should spend its time on if one is ever run:
**0.5, not 1** — and every figure recorded on this branch before today was measured at the
wrong dose.
