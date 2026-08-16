# Review record — the prior-blend prediction benchmark

**Commit reviewed:** `38a0129`, "Run the prior-blend benchmark, and split the population that
decides it", on `prior-blend-benchmark`. It adds `cmd/priorblend`, extends
`stats/prediction_inference.R`, and adds two regression tests.

## Reviewer dispatched

`fpl-stats-review`, on the measurement and on the inference change. It is the right and only
reviewer here: the diff adds a research command and a statistic, changes no scoring path, and
the risk is entirely in whether the claims are supported.

The review was run against an **earlier state of the branch** — a five-season, two-grid version
with three arms. Everything it found was applied, and the applied fixes changed the headline.
Its findings and their disposition are below.

## What the reviewer found, and what happened to each

**1. The grid was split on an era boundary the loader had already removed. Applied, and it was
the most valuable finding in the review.** The tool ran two grids, one "xG era" and one
"pre-xG era", on the premise that the archive carries no expected goals before 2022-23 — and
excluded 2023-24 entirely because its prior and its older season sat on opposite sides of that
line. The premise was verified against the **cached JSON**, which `backtest.Load` writes
*before* applying `xgrepair.go`. The repair backfills 2018-19 through 2021-22 from Understat,
including the season **totals**, which are exactly what `newPriorIndexMulti` reads to build a
prior. So there was no boundary, the "the rate channel cancels exactly" sentence was false, and
a season was discarded for nothing. Now one grid, six current seasons, a sixth season cluster
recovered, and the mistake recorded in place at the declaration.

One half of the reviewer's algebra is worth keeping even so, and is now in the code: with the
repair *off*, a zero prior rate would cancel exactly for players who have a prior in both arms
and would **not** cancel for the absent population, because shipped sends those to
`shrinkToLeague`. That is the population carrying the only negative result, so the old design
was weakest exactly where it mattered.

**2. The headline pooled two files the tool's own comment forbade pooling. Moot, applied.**
With one grid there is one file. The contradiction is gone rather than argued away.

**3. "The entire ordering loss is the absent players" was not a claim a Spearman could support.
Applied, and the proper experiment agrees with the improper one.** The old evidence was that
dropping those players from the field flipped the sign. The reviewer is right that this is not
the counterfactual — the version you would ship keeps them in the field, scored by shipped —
and right that rank correlation is not decomposable over a partition. A fourth arm now does the
real experiment: same field, same players, one gate changed, built by overwriting
`Engine.Priors` after `EngineAt`. Whole-field paired Δρ goes from −0.00225 (season t −3.70) for
half-life 1 to **−0.00020 (season t −0.52)** for half-life 1 restricted to injury cases. The
attribution survives the correct test.

The reviewer also pointed out the injury-only gate *could* be built from outside
`internal/backtest` — `Engine.Priors` is an exported interface — where the code claimed it
could not. Correct, and that is how the arm is built.

**4. "Spread held" understated the spread, and it was untested. Applied.** The R script now
carries `spread2`, the squared residual scatter within a gameweek, as a paired statistic
alongside `bias`, so the two halves of mse are both tested rather than one tested and one read
off a descriptive table. On six seasons the spread does not merely hold — it **falls**, at
season t = −4.44 / −2.40 / −1.41. The comment claiming "the spread does not move (0.2 per
cent)" is gone. `bias` now reports its own standard error, which the reviewer noted was missing.

The reviewer's deeper point is **not** resolved and is recorded here rather than answered: this
is not the same shape as the two precedents CLAUDE.md cites for "bias reduction is safe". Those
are a mis-stated quantity the model reads directly (minutes recency) and a bias shared by a
whole position (the clean sheet). This raises one identifiable subgroup relative to the rest of
the field, which is a within-field ordering change by construction. The ordering measurement
above is the honest answer to it, and it is why the injury-only arm is the one worth carrying
forward — not the "safe for an argmax" label, which is not claimed anywhere any more.

**5. Both clusterings on the paired scalars. Applied.** The block reported only the gameweek
clustering. It now reports both, as `compare()` does.

**6. `tail_signed_err` was in the file and not reported. Applied.** Shipped +0.2206, injury-only
+0.2016, every arm's paired difference around −0.02 at |t| below 1.8 on gameweeks and below 0.8
on seasons. It moves slightly *toward* zero and does not resolve, so nothing here says the top
of the predicted distribution got more over-rated.

**7. `popPremium` was classified, emitted and never reported. Applied.** 6,017 observations,
older season worth 4.0+ points per 90: bias −0.2971 → −0.1360 at half-life 1, mse −0.107 at
season t = −4.47, spread² −0.036 at season t = −2.41. The largest effect of any population, as
the mechanism predicts.

**8. Holm. Applied, in the direction the reviewer argued.** Adding `bias` to the existing family
would have silently moved adjusted p-values already recorded from this script, for a reason that
is not a measurement. `bias` and `spread2` are now excluded from `holm()` with a comment saying
why: a quantity and its own decomposition are not several questions, since mse ≡ bias² + spread².
The family still spans arms, which is where argmax inflation actually lives.

**9. The unwired-replay claim. Verified by the reviewer independently, by grep.**
`FPL_WEIGHT=prior_half_life=v` sets `cfg.Weights.PriorHalfLife`; only `internal/recent`'s live
path reads it; the replay reads `SimConfig.PriorHalfLife`; `SimConfig.OlderPriors` is populated
in exactly three places, none of them a CLI path. A replay sweep of this knob is a silent no-op.
The reviewer's wording correction is taken: the arm is **unwired**, not unmeasurable, and
declining the replay stands as sequencing rather than as a verdict.

**10. Provenance.** The reviewer noted the figures carried a date and nothing tying them to a
tree. Every figure in the commit and in the package comment is from the re-run on this tree at
this commit.

## What the reviewer praised, and what it should not be read as endorsing

`internal/analysis/thinprior_test.go` — the invariant that a thin prior is *believed*, not
shrunk. It refutes a premise stated twice in writing, by reading the shipped gate. Note the
reviewer saw this test in its final form; it is unchanged since.

The reviewer explicitly did **not** endorse the bias figures or the ordering result at the time
of review, because both were re-run afterwards. Nobody has reviewed the six-season numbers. They
are reported in the commit message and the package comment as measurements with their standard
errors, no verdict word is attached to the setting, and `prior_half_life` still ships at 0.

## What is not tested and should be

The four-arm run is not reproducible from a test — it is a command someone has to invoke. The
control-population invariance (every arm exactly 0 against shipped on players the setting cannot
reach) is the check that matters and it currently lives in the R output rather than in Go. That
is the cheapest remaining hardening and it is not done here.
