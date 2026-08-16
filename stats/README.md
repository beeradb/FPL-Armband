# stats — inference for the replay's sweeps and for the prediction benchmark

Go runs the engine. R does the statistics. A CSV is the contract between them.

Five scripts answer five different questions off the same **cells** file, which
is one row per replayed season-path:

| script | question |
|---|---|
| `sweep_inference.R` | is this setting distinguishable from the baseline? |
| `variance_components.R` | what size of effect could this design see at all? |
| `shape_inference.R` | is the *order* of the settings reproducible? |
| `entry_density.R` | would more entry gameweeks buy resolution, or duplicates? |
| `rank_robustness.R` | would scoring on **rank** rather than on points change the verdict? |

A sixth answers a different question off a different file — one row per replayed
**gameweek** rather than per season-path:

| script | question |
|---|---|
| `prediction_inference.R` | is the model's *prediction error* distinguishable? |

The distinction between those two groups is the important one and is set out
under "Two instruments, two questions" below. In short: the cells file measures
*points*, on 36 season-paths and at most five degrees of freedom; the prediction file
measures *accuracy*, on tens of thousands of player-gameweeks. Neither answers
the other's question, and reading one as though it did is how "the harness could
not see it" has repeatedly become "there is no effect".

A seventh thing reads across all of it: `fplagent snapshot` renders the
**model-and-harness accuracy snapshot** into `stats/snapshots/`. See "Snapshots"
below.

**An eighth is not a script at all.** `cells_common.R` is sourced by every one of
the above and holds the cells reader (`read_cells`, `read_cells_all`), the sidecar
reader (`read_sidecar`), the flag coercion, the cell key and the contract checks,
as well as the paired difference and the CR2 standard error. ⚠️ **It used to hold
only the arithmetic, and its header records why that reversed on 2026-08-14 —
read it before adding a reader.** `TestTheSharedCellQuantitiesHaveOneImplementation`
refuses a raw table read anywhere else under `stats/`, recursively, because the
two readers that were missed for longest lived in `stats/snapshots/`.

All six are **developer tools**, not dependencies. `go build ./... && go vet
./... && go test ./...` all pass on a machine with no R installed, and nothing in
the Go test suite invokes them. Five carry their own regression tests:
`Rscript stats/shape_inference.R --selftest`,
`Rscript stats/prediction_inference.R --selftest`,
`Rscript stats/variance_components.R --selftest`,
`Rscript stats/entry_density.R --selftest` and
`Rscript stats/cells_reader_selftest.R`, which is the reader's own and takes no
arguments. That last one runs on fixtures under `stats/testdata/` rather than on
banked cells, and deliberately: **the bank exercises none of the paths the readers
differed on** — 0 infeasible rows in 7,320, 0 empty character fields, `is_baseline`
character in all 72 files — so a before/after on committed cells is a regression
check and never evidence about a reader fix.

## Why the split exists

`internal/backtest`'s sweep harness used to run the grid *and* hand-roll the
statistics that decided what the grid meant. Three defects had accumulated in the
second half:

1. **The season-clustered SE was crude.** It averaged each season to one number
   and took the spread of four, with no small-sample correction and no principled
   df. AGENTS.md conceded it was "a noisy estimate of noise".
2. **One place knowingly did invalid inference.** The variance decomposition
   computed each layer's marginal SE as the root-sum-square of two adjacent
   cumulative SEs — assuming independence between layers that differ by one
   mechanism on the *same* cells and the same weeks. The test documented that it
   was wrong and reported it anyway.
3. **Nothing controlled for multiplicity.** Roughly twenty constants times four to
   six alternatives each, judged at `|t| >= 2`. At alpha = 0.05 that expects about
   four spurious "confirmed" verdicts.

The engine stays in Go because the hot path is discrete combinatorial search —
`dpseed.go`'s exact per-formation DP, the knapsack, the paired-move local search —
which is branchy scalar work that a vectorised language would be slower at, not
faster. **Budget from the per-cell rate rather than from a per-arm number**: a cell
is about **4.5 seconds**, so an arm on the shipped 36-cell grid is two and a half to
three minutes. The 85-seconds-an-arm figure this paragraph used to give was measured
2026-08-10 on the `MINHL` block, 5 arms over 24 cells, and it rotted the day the grid
widened; [replay.md](../docs/replay.md) carries the wrapper's own `elapsed=` evidence.
It was fifteen minutes an arm before the optimiser stopped being GC-bound.

**Inference now lives in exactly one place.** Go prints the mean paired difference
and the cell counts, which are cheap and useful for watching a sweep progress, and
prints no SE, no t and no verdict word. The mean is the single deliberate
duplicate: Go writes it to a `.means.csv` and this script asserts its own
recomputation against it, so the duplication is a pipeline test rather than the
two-defaults-for-one-quantity bug this project has shipped twice.

## Running it

```bash
# 1. Emit cells. Any sweep block works; EXP names it in the CSV.
FPL_CELLS=/tmp/cells.csv DIAG=1 EXP=A \
  go test ./internal/backtest -run TestDiagTransferPolicy -v -timeout 90m

# 2. Infer.
Rscript stats/sweep_inference.R /tmp/cells.csv

# 3. Ask what the design can resolve at all, from the same file.
Rscript stats/variance_components.R /tmp/cells.csv

# 4. Ask whether the ORDER of the settings is reproducible, which is a much
#    cheaper question than whether any one gap is real.
Rscript stats/shape_inference.R --order=numeric --mediator=moves \
  --invariant=hold /tmp/cells.csv
```

Options for `sweep_inference.R`: `--out=DIR` (default `stats/out`, gitignored),
`--no-plots`, `--tol=`, `--scale=per_gw|per_path`. `variance_components.R` takes
`--out=`, `--power=`, `--alpha=`, `--force`, `--selftest` and `--scale=`.
`shape_inference.R` takes neither `--tol=` nor `--scale=` and stops on an option
it does not know, which is deliberate — silence is not allowed to read as success
in any of these. ⚠️ **It also hardcodes the ×38 season conversion, so it must not be
pointed at an event-count sweep**; see the scale table below.

`--scale` picks what the paired difference is taken on, and the two are different
estimands rather than two units:

| scale | column | right for | over-weights |
|---|---|---|---|
| `per_gw` (default) | `<metric>_per_gw` | a **rate** — the cell total is `δ × weeks` | on an event count, the **short** paths: a GW26 entry counts ~2.9× a GW1 entry |
| `per_path` | `<metric>_points` | an **event count** — a fixed number of events per path | on a rate, the **long** paths: a GW1 entry counts ~2.9× a GW26 entry |

Each scale over-weights the opposite end, so the direction reverses depending on
which kind of quantity you have. Ask: *does the cell **total** grow in proportion
to weeks?* Yes is a rate. Dividing an event count by weeks and multiplying the
mean by 38 inflates the short paths by `38/weeks` — 1.70× overall on the
six-point grid and 2.52× in the late phase. See "Per gameweek is right for a rate
and wrong for an event" in the harness-and-inference note, which also has
the third category the binary does not cover: an effect anchored to a fixed
calendar week, which scales with neither.

Three things to know before using it. A `per_path` mean is **already** a
season-scale figure, so do not multiply it by 38 — and pass `--scale` to
`variance_components.R` too, or its `mde_season` column will. The reproduction
check against Go's own means runs on `per_gw` only, because Go writes
`mean_per_gw` and nothing else; the script says so rather than skipping quietly,
since that check is the pipeline's only end-to-end test. Run both scales — output
files carry the scale in their name, so the second run does not overwrite the
first.

Every comparison also reports the **clustering axis check**: `t seas`, `t fixd`
and `t entry` side by side with the raw between-season variance component, how
many seasons agree in sign with the pooled mean, and the three variance shares.
A small season-clustered SE means the clustering is conservative only when
`%seas` is non-trivial; where it is near zero, `t seas` and `t fixd` estimate the
same quantity and `t fixd` has five times the degrees of freedom. A `%seas` near
100 is not automatically the safe case either — read `agree` beside it.
`variance_components.R` is where to go next when they disagree.

Several sweeps in one session append to the same file safely; each row carries a
`sweep` label and a per-process `run_id`, so two runs of the same block stay
separate samples instead of pooling into one over-confident one.

R packages, installed once:

```r
install.packages(c("clubSandwich", "lme4", "lmerTest", "ggplot2"))
```

`lme4` needs `cmake` on macOS (`brew install cmake`). The script degrades rather
than failing if a package is absent — it says which and what it lost.

## The CSV schema

One row per (sweep, variant, season, start point).

| column | notes |
|---|---|
| `sweep`, `run_id` | sweep label plus process identity; together they key a comparison |
| `variant`, `variant_index`, `is_baseline` | `is_baseline` marks `variant_index == 0`, the arm everything is paired against |
| `season`, `prior_season`, `start_gw` | the cell's identity |
| `weeks` | `len(res.Weeks)` — gameweeks actually played, and the denominator |
| `bank_up_to` | pinned at 5 for every cell, which is historically wrong for 2022-23 and 2023-24 |
| `infeasible` | `true` when the variant could not field a legal fifteen |
| `policy_points`, `hold_points` | raw season totals |
| `policy_per_gw`, `hold_per_gw` | the same, divided by `weeks` |
| `moves`, `hits` | transfer counts, from the policy arm |
| `frozen_*`, `frozen_captain_*`, `weekly_*` | the variance decomposition's intermediate layers, in which the eleven is frozen at the day-one pick; blank for an ordinary sweep |
| `hold_fixedcap_*`, `hold_nocap_*` | the captaincy rungs (below); blank for the variance decomposition |
| `bench_boost_gw`, `bench_boost_pts`, `triple_captain_gw`, `triple_captain_pts` | what each scoring chip returned in the week the arm actually played it, and which week that was. Zero week means the arm did not play that chip in this cell. The standard sweep config plans no chips, so both pairs are `0` in an ordinary sweep; only a diagnostic that sets a chip plan populates them |
| `hold_xpoints`, `hold_xpoints_per_gw`, `policy_xpoints`, `policy_xpoints_per_gw` | accumulated **expected** points — realised points with four channels swapped for their expected value (`internal/analysis/xpoints.go`). **Blank, not zero,** for the variance decomposition, which builds its own row, and absent entirely from cells banked before 2026-08-15 |
| `setting`, `min_expected_minutes`, `squad_hash` | what the arm actually **did**, rather than what its label says: the swept value read back off the applied `SimConfig`, the resolved opening-squad pool floor, and a fingerprint of the opening fifteen as a set — so "the squad moved" is checkable off a banked file rather than asserted |
| `oracle`, `oracle_kind` | the hindsight granted to the cell, stamped from the same value the simulation consumed. `-` / `none` on an un-oracled arm |
| `bench_boost_oracle_*`, `bench_boost_median_pts`, `bench_boost_threshold_pts`, `bench_boost_bar_pts`, and the `triple_captain_` five | the chip-week oracle's three readings of each chip and the bar behind the third (below); blank on every arm but the oracled one, including the baseline arm of the chip-week oracle's own sweep |

Every `_pts` column in those last two groups is **one gameweek's points**, so none
has a `_per_gw` twin, none may be divided by `weeks`, and none may be multiplied by
38 either — they are already at season scale. The `_gw` columns are gameweek
numbers, not points. A chip pays once; normalising it by gameweeks played is an
inflation this record has paid for before.

### The chip-week oracle's readings

`TestDiagChipWeekOracle` places each scoring chip in its best week in hindsight,
plays no chip, and is required to leave every other collected metric
byte-identical — so it has no paired difference against its baseline to report and
its whole output is a table of levels. The two differences formed *within* a cell,
below, are differences between three readings of the same played season and not
against any baseline, which is what makes them bounded below rather than testable
against zero. Three readings of each chip, per cell:

- **`*_oracle_pts`** — the best gameweek's gain, and `*_oracle_gw` is which week
  that was. Unreachable, and the ceiling. The argmax takes the **first** week on a
  tie, so the week column is biased early by construction.
- **`*_median_pts`** — the median gameweek's gain. Read it as "a chip played
  without timing", **not** as the expected value of a randomly chosen week, which
  is the mean and which sits higher on a right-skewed series — and the series is
  right-skewed precisely because one big week is what the argmax is chasing. A
  float: the gains are integers, so an even number of weeks puts the median on a
  half-integer, and an int column would shift every such cell toward zero by up to
  half a point — a systematic per-cell shift that does not average away.
- **`*_threshold_pts`** — the first week clearing a fixed bar, falling back to the
  final week when none does. This is the shape any honest policy must take, since
  you cannot see the rest of the season. `*_bar_pts` is the bar it was run
  against, banked beside it: `threshold >= bar` is exactly "a week cleared" and
  `threshold < bar` exactly the forced end-of-season spend, so without the bar the
  column is a mixture whose proportions cannot be recovered.

Timing is `oracle − threshold`; `oracle − median` is the value of playing the chip
at all rather than wasting it, which is a different question and not what a timing
policy buys. It is **not** bounded above or below by the timing figure: when
nothing clears the bar the threshold rule takes the final week, which can sit below
the median.

**They are banked because the table alone could not be given a standard error.**
The readings were printed and written to no machine-readable file. The printed grid
does carry one line per cell, so this is not an aggregate that discarded its detail
— it is that nothing in `stats/` reads stdout, and this sweep's own banked cells
(`stats/snapshots/2026-08-12-4d61058/cells/oraclechip.csv`, 24 cells over four
seasons) carry none of the readings, because the schema had no columns for them.
The quotable form was therefore the summary: six means over the grid, no
dispersion. That is why the recorded chip-timing figures have no detection
threshold of their own. Per cell, keyed by `(run_id, sweep, season, start_gw)` and
read one `variant` at a time, either difference can be formed within a cell and
clustered on season.

⚠️ **What that standard error is for: an interval on a BOUND, not a test against
zero.** Every comparison here that hands a `diff` column to `se_cr2` is
arm-minus-baseline, where a mean of zero is a live hypothesis and a t against zero
answers something. **These two differences are not that, and cannot be.**
`*_oracle_pts` is `max(gains)`, `*_threshold_pts` is an *element* of the same
`gains` (`firstClearing` returns one, or the final week), and `*_median_pts` is a
median of it — so `oracle − threshold` and `oracle − median` are **≥ 0 in every
cell, always, by construction**. `reportChipCells` errors outright if
`oracle < median`, which is *half* that fact stated as a guard; the threshold half
is unguarded and rests on `firstClearing` returning an element of the same slice.
Hand such a difference to `se_cr2` and `lm(diff ~ 1)` returns a t against a null
the arithmetic already refuted, and one **not commensurable with any other t in
this record** — the status `AGENTS.md` assigns the perfect armband, whose "t of
20.4 is mechanical". So quote the mean **with its clustered interval, as a bound**:
"perfect timing is worth at most X **over the threshold rule**, ±" — naming the
comparator, since `oracle − median` bounds it over an untimed chip instead — and
never "significantly greater than zero".

⚠️ **The sign is guaranteed; the magnitude is not.** A small |t| here is not
evidence that the bound is small. The threshold rule catching the best week makes a
cell exactly 0, and a difference that is 0 in several cells caps the clustered |t|
**by construction** — the degeneracy `AGENTS.md` records for the minutes floor,
"with 2 of 6 seasons non-zero the clustered |t| is capped at 1.58". Zero in every
cell and `degenerate` in `cells_common.R` refuses the arm outright. Two more
consequences of the bound: the quantity is a maximum, so its season means are
right-skewed and a symmetric interval at 5 df **can extend below zero**, into a
region the arithmetic has excluded — report the lower end, but not as a live value.
And six start points is six intervals, so picking the largest is an argmax over six
noisy estimates, which is this record's signature failure rather than a reading.

**And per start point, not pooled — which is what makes the interval honest.**
`*_oracle_pts` is a maximum over the window the cell played — 38 candidate weeks
entering at GW1, 13 at GW26 — while `*_median_pts` is close to window-invariant. So
`oracle − median` rises with the window by construction and its pooled mean over
the grid mixes six estimands; `oracle − threshold` is distorted too and in an
ambiguous direction, since a longer window both raises the maximum and makes the
bar more likely to be cleared. The record already carries this on this exact
quantity — "Timing +8.3 pooled, 13.3 at GW1: read the column" — and `start_gw` and
`weeks` are both banked, so a reader can condition. It also means the six cells of
one season re-read **nested windows of the same football**, which is why a
cell-level standard error over the grid is invalid rather than merely optimistic:
cluster on season.

The two halves join, and they constrain the output together. Condition on
`start_gw` first, because the estimand changes with it; within one start point a
season contributes exactly one cell, so the frame handed to `se_cr2` in
`cells_common.R` is one row per season and the clustering is the equal-weighted
t-test on those. **The honest output is therefore six bounds with six intervals,
one per start point — not one pooled figure with one interval.** A pooled mean is
readable as a rough ceiling over the whole grid and nothing more; it has no single
estimand to put an interval around. Note the rows are seasons the chip readings
*survive* on: these are POLICY-path quantities, so `sweep_inference.R` blanks
2019-20 in them for the reason it blanks `policy_points`, and a start point on a
grid carrying that season has one row fewer than it appears to.

**No script reports them yet, and that is deliberate rather than an omission.** No
banked cells file carries these columns — every one predates them — so a reader
written now would have nothing to run on and no way to be wrong out loud. It is
owed the next time the chip-week oracle is swept, on the cells that sweep writes.
Note that no shared helper forms this contrast today: `diffs_for` builds
arm-minus-baseline on the cell key, and pointing it at `bench_boost_oracle_pts`
yields all-`NA` differences, because the baseline arm banks blanks by design.

⚠️ **Nothing here has been re-measured.** These columns make a standard error
*obtainable*; they do not supply one. No sweep has run under this schema and
`sweep_inference.R`'s metric list does not name them, so either difference is still
a run plus a reader away.

### The captaincy rungs

Two extra scorings of the same held fifteen, emitted by every ordinary sweep
because `HoldCaptaincyWeekly` produces all three in the one weekly pass HOLD
already pays for:

- **`hold`** — HOLD as every figure in AGENTS.md is measured. Eleven and captain
  both re-picked weekly, vice-captain takes over when the captain records no
  minutes, autosubs applied. This is what FPL pays.
- **`hold_fixedcap`** — the same, with the armband pinned to whoever the model
  would have captained in the week the squad was bought. Removes the weekly churn
  in *who* is captained; keeps the doubling.
- **`hold_nocap`** — the same, with nobody doubled at all. Removes the armband's
  variance contribution entirely.

Two derived marginals come with them, each differenced per cell like the ladder's:
`m_armband` is `hold − hold_nocap` and `m_capweekly` is `hold − hold_fixedcap`.

**Neither rung may replace `hold`.** FPL doubles a captain every week, so a metric
that does not is further from the game rather than closer to it. They exist to ask
whether a lower-noise *instrument* for tuning is available, and the test of an
instrument is signal-to-noise on an effect known to be real — not a smaller
standard error, which any metric that responds less to everything will show.
`variance_components.R` reports the minimum detectable effect for each; read it
beside the t ratios `sweep_inference.R` gives for the same arms.

Four properties matter and each has a Go regression test, because each fails
silently:

- **`weeks` is populated**, so the per-gameweek normalisation is reproducible
  downstream and not only pre-baked. Both the raw and the normalised figure are
  emitted; R re-derives one from the other and stops if they disagree.
- **An infeasible cell still emits a row**, flagged. Dropping it would read
  downstream as a comparison on fewer cells instead of a variant that failed. Its
  `*_per_gw` columns are blank because there is no denominator; the integer
  columns are zeroed, which is unambiguous only because `infeasible` is `true`
  beside them — R drops those rows before averaging anything.
- **Rows append and identify their run**, so several sweeps in one session do not
  overwrite each other.
- **Exactly one arm per sweep is the baseline.** Otherwise R has to guess the
  reference and the sign of every difference is unexplained.

Blank versus zero is load-bearing where a column could be averaged: an *unmeasured
layer* and a layer measured at zero are different facts, so the layer columns are
blank when a sweep did not measure them. It is not a blanket rule — an infeasible
row's integer columns are zero, and safe only because the `infeasible` flag beside
them means nothing reads them.

## What the script reports

- **Paired differences** per gameweek played, checked against Go's own means.
- **Three standard errors** side by side. `naive` treats all cells as independent
  and is what every t value currently in AGENTS.md is — reported for continuity,
  not because it is right. `CR2` is cluster-robust at the season level with the
  bias-reduced small-sample correction and **Satterthwaite df, reported rather
  than asserted**. `lmer` fits `diff ~ 1 + (1|season)`.

  That model is deliberately *not* `(1|season/start_gw)`: there is exactly one
  observation per (season, start point), so a start-point random effect is
  perfectly confounded with the residual. `(1|season)` **is** the nested model
  here — the residual is the within-season, between-start level.

- **Multiplicity.** Raw and Holm-adjusted p-values across the alternatives within
  a sweep and metric. Holm rather than Bonferroni because it controls FWER without
  assuming independence, and the arms share every cell. Raw is kept alongside
  because every verdict already written into AGENTS.md is a raw one.

- **Marginals for the variance decomposition**, differenced **per cell**. This is
  defect 2's real fix: a marginal is itself a per-cell quantity, so it goes through
  the same machinery as any other metric and there is nothing to approximate.

- **Noise by information regime**, replacing `reportRegimeNoise` — the third
  hand-rolled variance estimator in the Go package, and dead code besides. The
  null is not equal noise but noise scaling as `1/sqrt(weeks)`, since a late entry
  averages less football; the last column states that prediction so a regime
  exceeding it is visible.

- **Plots**, to `stats/out`. A shape plot — estimate and CI against the swept
  value — because several of this project's conclusions are shape claims
  ("monotone from 3", "a plateau with a cliff"), and a shape claim should be
  looked at. And a per-cell plot faceted by season and entry gameweek, because
  twelve cells at +40 is a real effect and +900 with −860 is not, and a mean
  cannot tell them apart.

## Reading the output

Prefer the CR2 column and read the df it resolved rather than assuming one. A raw
p under 0.05 whose Holm-adjusted p is not is exactly the case the old `|t| >= 2`
rule counted as "confirmed". And weigh the shape above any single row: this project
accepts monotonicity across several values, a plateau with a cliff, or the held-out
season agreeing — a single argmax is none of those.

A metric where every difference is exactly zero is reported as such. For a
transfer knob measured on HOLD that is the invariance check passing, not a failed
measurement.

## Two instruments, two questions

Everything above measures **points**. Its unit of replication is a replayed
season-path: buy a squad, play the season, count. The default grid is six seasons
times six entry gameweeks, so **36 cells**, and honest clustering leaves at most
**five degrees of freedom** — `S − 1`, and the figure that applies to a given
comparison is resolved from that comparison's own cells and is often lower. That
ceiling is the binding constraint: entry points add paths through the same
football and cannot move it at all. It grew once, from three, when
`understat_xg_backfill.py` made 2019-20 to 2021-22 playable, and after that it
grows by one season a year and by nothing else. `FPL_SWEEP_SEASONS=default`
returns the four pairs the record was built on, which is how a figure recorded
before 2026-08-11 is reproduced on its own grid.

The prediction benchmark measures **accuracy**. Its unit is a player-gameweek,
and the headline population carries roughly 10,000 of them and 33 gameweek clusters
**per season replayed** — so about 60,000 observations and 200 clusters on the six
that ship, against 40,000 and 130 on the four the record was built on. It follows the
replay grid rather than declaring its own, which is why the per-season rate is the
figure to hold.

They are not interchangeable, and the failure mode runs both ways:

| | replay cells | prediction benchmark |
|---|---|---|
| answers | is this setting worth points? | is the model right about football? |
| unit | one replayed season-path | one player-gameweek |
| sample | 36 cells, at most 5 degrees of freedom | ~60,000 observations, ~200 clusters |
| script | `sweep_inference.R` | `prediction_inference.R` |
| cannot answer | whether the model is right | whether a change earns points |

**A minimum detectable effect belongs to a comparison, not to a harness**, and the
range is sixtyfold. Judged per arm across 23 comparisons from three real sweeps —
all on the four-season grid, before the default widened — the significance threshold
runs from **3.9 to 232 points a season** pooled across both estimators, with a median
of 39 on the season-clustered one and 32 on the start-fixed one — both re-derived by
`stats/regenerate_mde.sh`, which prints them. The vice-captain
fallback fix — mechanism-certain, and therefore landing almost identically in every
cell — resolves **12.7** on the transfer metric, better than most held-metric
comparisons; the minutes half-life sweep's `flat` arm needs **232** on the same metric
and the same grid. Do not rescale these to six seasons: the `t_crit(S−1)/√S`
arithmetic below prices the widening as an ordering, and no comparison here has been
re-measured on the wider grid.

The figures long recorded as "42 on the held metric and 147 on the transfer metric"
are **pooled** over a sweep's arms, and the pooling is what made the transfer metric
look like the noisy one: on that sweep a single arm contributes 72% of the pooled
season variance. Read them as one sweep's average and not as the instrument's
resolution. `mde.csv` now carries a row per arm; see its schema below.

**The prediction instrument cannot replace the replay, and the reason is
load-bearing.** A better predictor can make a worse policy. Recency-weighted rates
improve out-of-sample error by about 2% and cost about 49 points a season, because
a transfer policy is an argmax living in the tail of the estimate distribution:
accuracy bought on the average player is paid for with noise exactly where the
search looks. So the benchmark ranks candidates and chooses structure; the replay
prices them.

## `prediction_inference.R` — is the prediction error distinguishable?

```bash
# 1. Emit per-gameweek cells. About six seconds — no season is replayed, so this
#    is orders of magnitude cheaper than a sweep arm.
FPL_PREDICTION_CSV=/tmp/prediction.csv FPL_MODEL_CSV=/tmp/model.csv DIAG=1 \
  go test ./internal/backtest -run TestDiagPredictionBenchmark -count=1 -v -timeout 60m

# 2. Re-derive the recency comparison two shipped constants rest on. Also seconds.
FPL_MODEL_CSV=/tmp/model.csv DIAG=1 \
  go test ./internal/backtest -run TestDiagNextFivePredictors -count=1 -v -timeout 30m

# 3. Infer.
Rscript stats/prediction_inference.R /tmp/prediction.csv

# 4. Its own regression test. No replay, no CSV, no R packages.
Rscript stats/prediction_inference.R --selftest
```

`-count=1` is not optional: Go caches test results, and a cached run replays its
stdout while writing no new CSV rows.

Options: `--out=DIR` (default `stats/out`), `--target=` (`points`, `minutes` or
`expected goals`), `--population=` (`60+ minutes` for the headline population, or
`every registered` for the whole game). Both selectors are substring matches
against what is actually in the file, and a miss lists what was available rather
than silently analysing nothing.

### What the Go side measures

Predicting **one gameweek ahead** from a model built through the gameweek before
it, on the same seasons the replay uses — `TestDiagPredictionBenchmark` reads
`sweepPairNames()` through `loadPairs`, so its population widens and narrows with the
grid rather than being declared separately. One gameweek ahead because that is
what the published comparison it sits beside measures, and because the model's
one-gameweek view is a real object: `Score` averages fixture difficulty over the
horizon, and the fixture-load term that makes a double gameweek worth two is
deliberately confined to horizon 1. So the benchmark builds the engine at horizon
1, which is the configuration `analysis.Engine.WeekEngine` produces.

Three axes, because error alone is not enough:

- **Error** — mean absolute error and root-mean-square error, on points, minutes
  and expected goals plus assists. Lower is better.
- **Calibration** — predicted against realised, grouped by *what was predicted*.
  This is how the mid-season over-confidence was found.
- **Ordering** — Spearman's rank correlation within each gameweek, plus the signed
  error over the twenty highest-predicted players. The optimiser consumes an
  ordering and never a level, which is why the bonus term is kept despite being
  badly calibrated.

Error is split by **what the player actually scored**, using OpenFPL's categories
(arXiv 2508.09992) so the figures sit beside published ones: *Zeros* recorded no
minutes, *Blanks* played for two points or fewer, *Tickers* scored three or four,
*Haulers* five or more. Aggregate error is dominated by thousands of easy
near-zero predictions and says nothing about the tail the transfer search hunts;
the Haulers row is the only direct measurement of that tail this project has.

### Three things about it that are easy to get wrong

**The population is players whose club had a fixture.** Without that restriction a
missing per-gameweek row is ambiguous between "he was dropped" and "his club did
not play", so the Zeros category fills with blank gameweeks rather than with the
dropped and injured players it is for. With it, a missing row is an unambiguous
zero.

**Squad relevance is measured in minutes, not starts, and this cost a run.** The
obvious filter is "started at least one of the previous five gameweeks". The
archive's per-gameweek `starts` column is **zero for all of 2021-22 and for
2022-23 up to GW15**, only populating from GW16 — verified directly against
`gws/merged_gw.csv`. A starts-based filter therefore admitted nobody in 2022-23
before GW20, silently made a four-season figure into a three-season one, and
printed an entirely plausible table for the other three. The filter now reads
minutes with a sixty-minute bar, which is where FPL itself pays appearance points
and the clean sheet, and a **coverage table is printed first and the run fails**
if a season contributes nothing or loses a third of its gameweeks.

**FPL's own published expected points is deliberately not a baseline.** The
archive carries it as `xP`, scraped from `ep_this` *after* each gameweek ends, and
the data dictionary warns it may reflect post-match information. It is a
contaminated reference, not a free one, and wiring it in would also cost a
cache-version bump plus a field check in `parsedByThisVersion` — a version bump
alone does not catch a stale file an experiment left behind. The clean baseline,
mean of the last five gameweeks, is OpenFPL's own, so the shapes stay comparable.

### The CSV schema

One row per (arm, season, gameweek, population, target, predictor, realised
category).

| column | notes |
|---|---|
| `run_id`, `variant`, `is_baseline` | process identity, the arm, and the one arm everything is paired against |
| `season`, `prior_season`, `gw` | the cluster's identity; `gw` is the gameweek predicted |
| `population` | the two model-independent filters |
| `target` | points, minutes, or expected goals plus assists |
| `predictor` | `model`, or one of the two naive baselines |
| `category` | the realised return category, plus an `all categories` row |
| `n` | observations in this cell — the weight |
| `sum_abs_err`, `sum_sq_err`, `sum_pred`, `sum_act` | per-cluster sums |
| `rank_corr`, `tail_signed_err` | gameweek-level scalars, on the `all categories` points row only, **blank** elsewhere |

**Sufficient statistics per gameweek, not one row per observation, and that is a
reading of the standing rule rather than an exception to it.** The unit of
replication is a gameweek — every player in one is exposed to the same football,
so their errors are correlated and no standard error may treat them as
independent. The four sums are *exactly sufficient* for mean absolute error, mean
squared error, bias and the error spread; and a **paired** difference between two
arms is the difference of their sums, because both arms score the identical
observations, so summing then differencing and differencing then summing are the
same arithmetic. Per-observation rows would buy only the ability to cluster below
the cluster, at ten times the file size, plus a way to get the pairing wrong.
`TestPredictionCellsSumToTheReportedTotals` pins the two granularities against
each other.

The two gameweek-level scalars are blank rather than zero on every other row,
because an unmeasured quantity and one measured at zero are different facts and
only one of them is a number R will average.
`TestPredictionCellsCarryTheGameweekScalarsExactlyOnce` pins that.

### What the R script reports

- **The levels**, so the differences have a scale: mean absolute error,
  root-mean-square error and bias per (arm, predictor, category).
- **The model against each naive baseline** on mean squared error and mean
  absolute error, paired by gameweek. Mean squared error is the statistic to test
  on: it is linear in the per-observation squared errors, so it pairs exactly,
  where root-mean-square error does not.
- **Each arm against the shipped baseline**, the same way.
- **Two clusterings of every estimate.** Clustering by **gameweek** gives about 33
  clusters per season replayed — roughly 200 on the shipped six — and is the primary
  reading. Clustering by **season** gives six,
  which is the level the replay is forced to work at, and is reported so the two
  instruments can be compared honestly. A candidate significant by gameweek and
  not by season is precisely the class of effect the replay cannot see.
- **Holm-adjusted p-values** beside the raw ones, for having asked about several
  baselines and statistics at once.
- **Ordering and the tail**, averaged over gameweeks with a cluster-robust
  standard error.

An arm whose estimate and standard error are both exactly zero is an **invariance
check passing**. The vice-captain fallback arm reads exactly zero by construction,
and that is the point of including it.

### Reading it

Two controls are built in and the Go side fails the run if either misbehaves,
because a control that misbehaves makes every other figure unsafe to read.

**The vice-captain fallback is an invariance control here** — a *negative* control,
even though it is a positive control for the replay. It changes how a played-out
gameweek is scored and nothing about what the model predicts, so every figure must
be identical with it on and off. If it moved, the instrument would be reading the
replay's scoring rather than the model's predictions.

**The minutes half-life is the directional control.** It was set out of sample on
8,374 predictions where sharpening recency cut minutes error by about 9%, so
switching it off must make the minutes error worse.

Then read in this order: the coverage table, so you know the sample is what it
claims; the ordering and calibration tables, because they describe the quantity
the optimiser consumes; and the conditional error table last, remembering that
**conditioning on the outcome rewards a noisier predictor in the extreme
buckets** — a predictor that fires more high numbers will look better on Haulers
while being worse calibrated at the top of its own distribution. That is not a
quibble about our data; it is a caveat on the published table too.

The verdicts this produced on the shipped model are written up in AGENTS.md under
"The prediction benchmark: what the model is right about, and where it is not".

## `understat_xg_backfill.py` — extending the season axis backwards

Seasons are the scarce axis and the binding constraint on everything else here: the
four that shipped before this script gave the season-clustered standard error **three
degrees of freedom**, so the 5% critical value was 3.18 rather than 2 and the canonical
median detection threshold was 39 points a season — against constants worth 11 to 34.
This script is what took the default to six (`0b994d5`), df to five and `t_crit` to
2.571, by backfilling the one thing the older archive lacks. What that is and is not
worth is under "What it buys" below.

```bash
# What the provider offset is, per season, and how far the crosswalk reaches.
# Reads the archive and Understat; writes nothing.
python3 stats/understat_xg_backfill.py --calibrate

# Regenerate one season, or all of them. Cached, so a re-run is nearly free.
python3 stats/understat_xg_backfill.py --season 2020-21
python3 stats/understat_xg_backfill.py --all

# Validate a repair that is already written.
python3 stats/understat_xg_backfill.py --season 2020-21 --check

# What the wider crosswalk would add to each season, without applying it.
python3 stats/understat_xg_backfill.py --compare-crosswalks

# Then check the loader agrees, without running a replay.
DIAG=1 go test ./internal/backtest -run TestDiagExtendedSeasons -v
```

Output goes to `internal/backtest/repairdata/<season>-xg.csv` plus a `.meta.json`
sidecar, both embedded into the `backtest` package. The sidecar records the provider
offset the season's xG was divided by and whether it was fitted in season or borrowed;
`readXGRepairMeta` refuses a repair whose sidecar disagrees with the loader's own window
and renumber, because that disagreement puts rows in the wrong weeks and every figure
downstream would still look plausible.

### Two different defects, and only one of them can be validated properly

| season | defect | offset | validated against |
|---|---|---|---|
| 2022-23 | weekly `expected_goals` is zero for GW1-15; the series starts at GW16 | fitted **in season** on GW16-38 | `players_raw.csv`'s complete season aggregate, which the repair never sees |
| 2021-22, 2020-21, 2019-20, 2018-19 | `expected_goals` is **absent as a column** from both files | **borrowed** from the four seasons that carry both | nothing equivalent exists — see below |

The four older seasons have no aggregate to check against and no overlap window to fit
on, because the quantity does not exist in the archive in any form. That is a weaker
claim than 2022-23's and the sidecar says so on every load.

### The season total is repaired too, and it is a separate half

The weekly rows are only half of what a season needs. `PointInTime` accumulates them, so a
**played** season sees the repair; `PreSeason` and the prior index read the season
*aggregate*, which is exactly zero in the four seasons with no `expected_goals` column. So
`rebuildXGAggregates` fills it from the repaired weeks, and before it existed three cells
built their opening fifteen and blended every later gameweek with no expected goals at
all — 2020-21, 2021-22 and **2022-23**, the last of which is in the shipped grid.

2022-23 itself is not rebuilt: its own aggregate is complete at 1097.3 against weekly rows
summing to 731.5, so adding the backfill to it would inflate the season by half again.
That is why the season table carries a `NoAggregate` flag rather than a rule about
"seasons with a repair". `FPL_NO_XG_AGGREGATE=1` isolates this half from the backfill as a
whole.

Summing a whole season is normally the point-in-time leak this project refuses, so the
difference is pinned rather than argued: the aggregate is read only for a **prior**, whose
season is wholly past relative to the one being played, and
`TestRepairedAggregateDoesNotLeakIntoAPlayedSeason` checks behaviourally that a view
through GW5 still sees five gameweeks and strictly less than the season total.

### 2018-19 is prior-only, and that is a loader change rather than a backfill one

`PreSeason` reads only season aggregates from the prior and takes `Teams` from the
*current* season, so `teams.csv` and `fixtures.csv` are needed to **play** a season and not
to **be** one. 2018-19 publishes `players_raw.csv`, `fixtures.csv` and
`gws/merged_gw.csv` and **no `teams.csv`** — checked by status code — and `Load` refused it
outright, which was the real blocker rather than any reconstruction.

`loadTeams` and `loadFixtures` now tolerate a **404 only**, record it in `Season.Absent`,
and the season loads marked prior-only. Every other failure — a 500, a timeout, a
truncated body — stays a hard error, because treating one as absence would let a transient
network fault quietly drop a season from a grid while every number still printed.
`PlayableAsCurrent` refuses to play such a season, `PreSeason`/`PointInTime` panic as the
backstop that covers `Hold` and friends, and `absentIsConsistent` checks the marker against
what was actually parsed in *both* directions, so it cannot be a field somebody forgot to
set.

What it buys is degrees of freedom, and the honest arithmetic is:

| grid | `FPL_SWEEP_SEASONS` | seasons played | df | t_crit | the canonical 39/season becomes |
|---|---|---|---|---|---|
| `extendedPairNames` (**ships**) | unset or `extended` | 6 | 5 | 2.571 | 26 |
| the four the record was built on | `default` | 4 | 3 | 3.182 | 39 |
| `scoringPairNames`, `HOLD` only | `scoring` | **7** | **6** | **2.447** | **23** |

and `HOLD`'s own 33 becomes about 22 on the six that ship and about 19 on the seven.
**POLICY gains nothing** from the seventh: the season 2018-19 unlocks
is 2019-20, which POLICY has to exclude anyway, so POLICY has six usable seasons either
way. And the 23 assumes the seventh cell is as quiet as the four that ship, which a
backfilled season is not — read it as the figure if the cells were equivalent.

### What is checked instead, and it is stronger than it sounds

The join, which is where a backfill actually goes wrong: a bad player id or a date mapped
to the wrong gameweek attaches real xG to the wrong footballer or the wrong week.
**Understat's goals against FPL's goals** over the joined cells catches exactly that, and
both sources count an exact integer, so the answer has to be 1.0000 rather than merely
close. It is, in every repaired season — 993/993, 955/955, 920/920, 981/981, 966/966 — and
`TestEveryRepairShipsItsOffset` fails outside a tenth of a per cent. Minutes agree to
1.003.

Two corrections to that check came out of adding 2018-19, and both make it sharper:

- **Own-goal cells are excluded**, because Understat credits an own goal to the attacker
  who forced it and FPL credits it to the defender — so the anchor was measuring a
  definitional difference between the sources rather than the join. That is what made
  2018-19 read **1.0019**, and it was verified against the archive's own `fixtures.csv`
  stats blob rather than assumed: Burnley's two goals against Fulham on 2019-01-12 are
  FPL own goals by Odoi and Bryan with two assists for Hendrick, and Understat gives
  Hendrick a goal. Excluded, 2018-19 reads 1.000000 with no disagreeing cell, and the
  other four stay where they were.
- **The disagreeing-cell count now travels with the ratio, and it is the number to read.**
  A ratio of exactly 1.0000 is *not* proof the join is right: 2019-20 reads 1.0000 with
  **two** cells disagreeing, because Understat gives Kevin De Bruyne a Manchester City goal
  in GW10 that FPL gives to David Silva. One cell over and one short cancel — which is
  precisely the signature of a mis-mapped date, the thing the anchor exists to catch. The
  guard bounds the count at the 2 this archive is known to carry.

The crosswalk is validated three ways for the same reason. It is built from FPL's
permanent `code` — one column from `ChrisMusson/FPL-ID-Map`, joined to element ids
re-derived from the archive's own `players_raw.csv` — and it agrees with the archive's
bundled `id_dict.csv` on **1,228 of 1,228** overlapping pairs, reaches **100% of players
with 900+ minutes** in every season from 2019-20 to 2025-26, and covers 0.9998 or more of
league minutes.

### The offset is not a constant, and the residual is the benign kind of error

Measured on every season carrying both sources, players with 900+ minutes:

| overlap season | xG understat/FPL | xA understat/FPL |
|---|---|---|
| 2022-23 (GW16-38) | 1.0310 | 1.2105 |
| 2023-24 | 1.0907 | 1.2430 |
| 2024-25 | 1.1281 | 1.2680 |
| 2025-26 | 1.1041 | 1.1722 |
| **applied to the borrowed seasons** | **1.0885** | **1.2234** |

An 8.9% spread on xG, so a borrowed offset leaves about ±4-5% of level error whichever
estimator is chosen. Three are defensible and they span 2.6%; the plain mean ships
because it hand-picks no season and fits no regime model. `--offset-xg` and `--offset-xa`
re-measure a different choice rather than editing the script.

What licenses leaving that residual is a finding this project already has from the clean
sheet and the bonus term: **a level error shared by every player in a season is invisible
to an argmax**, because the optimiser consumes an ordering. What is *not* benign is
per-player dispersion — two xG models disagree shot by shot, p90 of the per-player ratio
is 1.54, and no rescaling touches it. **A backfilled season is a noisier cell, not an
equivalent one**, which is why pooling one attenuates a paired difference as well as
sharpening it.

Rescaling at all is worth it for one reason: the prior for a replayed season is the season
before it, so an unrescaled 2021-22 feeding 2022-23 would put the seam between two scales
*inside* a cell rather than between cells.

### xGC is deliberately not repaired — ⚠️ RETRACTED, it is repaired

This section described the state before `internal/backtest/xgcrepair.go`. It argued that
Understat publishes team xGA per match rather than per player, that the per-player figure
carries a substitution channel a club rate would delete — worth +0.140/+0.067/+0.007 pts/gw
across substitution terciles — and that a prorated club rate would therefore be a different
quantity present only in the backfilled seasons.

**The premise is true and the conclusion did not follow.** The choice was never "per-player
figure against club rate", it was **reconstruction against nothing**: `baseXP90` gates both
the clean sheet and the goals-conceded deduction on `XGC90 > 0`, so leaving xGC alone did not
preserve the substitution channel — it switched off 26-45% of every defender's and keeper's
score in **18 of the 36 six-season cells**. The reconstruction runs repaired player xG → club
xG → the opponent's xGA → prorate by minutes, needs no new harvest, and flags every
reconstructed row for exactly the objection this section raised.

Swept 2026-08-13 (`stats/snapshots/2026-08-13-4d61058/`): the confinement is exactly 18 of 36
and the other 18 cells are byte-identical, while the points effect is **unresolved in any
direction** and structurally cannot resolve — a season with native xGC is a season where the
repair is inert, so no widening of the grid adds a cluster.

### 2019-20 needs a gameweek renumber and is HOLD-only

Its rounds are labelled 1-29 and then **39-47**: COVID stopped the season and FPL numbered
the restarted rounds afresh. `loadGameweeks` drops anything outside 1..38, so without
`renumberGW` the season parses as 29 gameweeks — a quarter of the football sitting in the
archive, reading as a season that stopped in March. The shift is exactly minus nine and
cannot collide, because events 30-38 are entirely absent from its `fixtures.csv`.

And its transfer path is not a sample of the same process as the other seasons': FPL
granted **unlimited free transfers** before the GW30+ deadline and froze prices for three
months, which no bank limit can express. `TransferPathComparable` says so in code rather
than in prose. Its *scoring* is fine, which is the split `HOLD` and `POLICY` already draw,
so it extends the season axis for scoring constants and not for transfer constants.

### The six-season grid ships; the seven-season one does not

`sweepPairNames()` in `harness_test.go` now returns `extendedPairNames()`, the
six-season grid, and has since `0b994d5` on 2026-08-11 — so an unset
`FPL_SWEEP_SEASONS` gives 36 cells, not 24. `FPL_SWEEP_SEASONS=default` returns
the four pairs the record was built on, `=scoring` returns `scoringPairNames()`,
the seven-season `HOLD`-only grid, and anything else panics rather than falling
through to a default its operator never chose.

**No *sweep* runs on the seven, but three diagnostics do**, and this is the half
worth knowing before you read their numbers beside a sweep's: `TestDiagExtendedSeasons`,
`TestDiagXGAggregate` and `TestDiagXGCPoints` call `scoringPairNames()` directly, so
their figures are seven-season `HOLD` figures and are not commensurable with a
36-cell sweep. Two more name it without replaying it — `TestSeasonNeedsReproduceTheNamedGrids`
and `TestTheGridsAreNested` — and both enumerate the named grids in order to compare
them against the capability model rather than to run one.

The standing objection — that widening the default would make the record
incomparable with itself at a stroke — was **checked and did not survive**. The
shipped four pairs are a strict subset of the extended six, and the cells they
produce inside a six-season run are byte-identical to an independently run
four-season sweep: 48 of 48 overlapping cells agreeing on every outcome column,
192 across all arms. No published figure was invalidated; each remains correct
**as a four-season figure**, which is why every one of them needs its grid named
beside it. `sweepStarts` keeps its six entry points for the other half of that
reason, and densifying it is opt-in through `FPL_SWEEP_STARTS`.

Wiring a grid in was still a decision that wanted its own measurement pass, and it
got one — `gridwidth_test.go` carries the pre-registration and
`stats/snapshots/2026-08-11-6acc5ad` the cells. `TestTheGridIsDeclaredOnce` exists
because a diagnostic measuring a different season population from the sweeps it is
quoted beside is a silent failure.

### Politeness

Understat is a free site with no stated licence. One request per player, cached to
`~/.cache/fplagent/understat`, with a delay, and a long backoff on a transport error
because a reset from a free site is most likely it asking for less traffic. A player's
response carries every season he played, so adding a season mostly re-reads the cache.
**Do not remove the delay.**

## `variance_components.R` — what the design can resolve

`sweep_inference.R` answers "is this arm distinguishable from the baseline".
`variance_components.R` answers the prior question: "what size of effect could
this design distinguish from anything". It reads the same cells file and needs no
replay.

```bash
Rscript stats/variance_components.R /tmp/cells.csv      # writes stats/out/cells/
```

It fits the **crossed** model `diff ~ 1 + (1|season) + (1|start_gw)` — which is
identified on these cells even though the *nested* `(1|season/start_gw)` is not,
because every start point appears in every season and only the interaction goes
to the residual. From the three components it reports:

- **which noise is which**: genuine season heterogeneity against within-season
  path noise, with an F test on the season component and a one-sided interval on
  it, because a point estimate of zero on a handful of seasons is not zero;
- **the minimum detectable effect** per metric, in points per gameweek and points
  per season, so "unresolved" is readable as a statement about the sample;
- **a cross-check that `sqrt(MS_season/(G*S))` reproduces CR2 exactly**, so the
  decomposition takes the shipped estimator apart rather than becoming a second
  implementation of it;
- **what more start points, or disjoint windows, would buy** — priced from the
  measured components rather than guessed;
- **the Rademacher wild-bootstrap p-value floor** at this cluster count, by
  enumeration. At four clusters it is 0.125, so that procedure cannot reject at
  5% however large the effect and must not arbitrate anything;
- **equal against inverse-variance weighting** of the unequal-length cells, with
  the per-entry-point means beside it, because the two weightings target
  different estimands whenever the effect varies by entry regime — and it does.

`--power=` and `--alpha=` move the MDE definition. Everything but the REML column
is base-R arithmetic, so it degrades rather than failing without `lme4`.

It writes four tables: `variance_components.csv` (per comparison),
`variance_pooled.csv` (pooled per metric), `design_projection.csv`, and
**`mde.csv`**. The last one exists so the accuracy snapshot can *read* the minimum
detectable effect rather than recompute it — see "Snapshots" above.

### `mde.csv`, and why `scope` is the column to read first

One row per (`scope`, `metric`, `variant`, `estimator`).

**`scope = "arm"`** is one comparison: one alternative against the baseline, on one
metric. This is the honest unit, and each comparison gets **two rows** — the two
defensible estimators — rather than one:

| estimator | variance | df | valid when |
|---|---|---:|---|
| `season-clustered (primary)` | (σ²season + σ²resid/G) / S | S−1 | always; conservative. Equals clubSandwich's CR2 on these cells, pinned to machine precision by the script's own cross-check |
| `start fixed, no season effect` | σ²resid / (S·G) | (S−1)(G−1) | only where σ²season really is zero. The same entry gameweeks are replayed in every season on purpose, so an offset between them cancels from a paired comparison and should not be paid for |

**The df column is a formula and not a number on purpose.** The script resolves `S` and
`G` from the cells it is handed, so the shipped 6×6 grid gives 5 and 25 while
`FPL_SWEEP_SEASONS=default` legitimately gives 3 and 15 — and a figure recorded under
one is not comparable with a figure recorded under the other. Read the df off the arm's
own row rather than substituting a remembered constant.

Beside them, that arm's **own** `f_season` and `p_season`, so a reader can see which
end of the bracket the data supports. **The bracket is deliberately not collapsed by
a pre-test.** `f_power` is the power of that F test against the case that would
change the answer — a season component just large enough to double the clustered
variance. It was **0.22** at four seasons and the record puts it near **0.30** on the
shipped 6×6 grid; either way "the F test did not reject, therefore treat entry
gameweeks as fixed" is anti-conservative most of the time, and no arm row carries
`is_primary`. **Read `f_power` off the arm's own row** rather than taking either
figure — the script prints it per arm, and these two are illustration.

**`scope = "pooled"`** is the whole sweep's arms averaged, which is how every
threshold recorded in AGENTS.md was computed. It is kept so that record stays
interpretable and it is **not** the figure to quote for a comparison: a plain mean of
the arms' variance components is dominated by whichever arm disagrees most between
seasons. On the minutes-half-life grid the `flat (no recency)` arm contributes 72% of
the pooled season variance on POLICY, and the pooled 147 points a season is
essentially that arm's own 232 applied to three arms whose figures are 80, 90 and
133.

`is_primary` survives on the pooled rows alone, because the snapshot and this file
key off it. It is decided by the **strictest** arm rather than the average of the
arms: averaging p-values is not a test of anything, and here it fails in the
expensive direction — on POLICY one arm's season F test gives p = 0.001 while three
do not reject, so the mean is 0.11 and would license treating entry gameweeks as
fixed on a metric whose season component is demonstrably real. It carries the same
low-power caveat as any other reading of that test.

### Where the tables go

With no `--out` they go to **`stats/out/<name of the cells file>`** — never to
`stats/out` itself. A twelve-cell demo run once wrote to the bare default and its
figures (a mean paired difference of 6.1 points a gameweek at t = 68, which is not a
replayed season) were read as current by the accuracy snapshot for weeks. Nothing
looked wrong because the output named no source.

With an explicit `--out` the path is used literally, and the script **refuses** to
overwrite an `mde.csv` written from a different sweep unless `--force` is passed.
`fplagent snapshot` always passes an explicit per-cells-file directory, so a run of
several sweeps keeps its tables apart.

### Its own regression test

```bash
Rscript stats/variance_components.R --selftest
```

No cells file, no R packages, no replay. It pins the decomposition on a synthetic
table whose components are exact by construction, the two threshold formulas, the F
test's power, and the per-arm thresholds of the frozen worked example — including the
pooled 147 that used to be the only figure, so the defect and the fix are both
regression-tested.

The verdicts it produced on the shipped grid are written up in AGENTS.md under
"The noise splits differently on the two metrics, and only one of them can be
fixed".

## Snapshots — regenerating the accuracy record

A **snapshot** is a dated, stamped record of two things that must not be blurred:
whether the scoring model is right about football, and whether the replay harness
can see anything at all. It lives in `stats/snapshots/<date>-<commit>/` as three
files — `snapshot.md` for a human, `figures.csv` so the *next* snapshot can diff
against it, and `constants.csv` listing every shipped setting in force.

**It is a side effect of running a sweep, not a discipline to remember.** That
matters because this project's hand-maintained records rot silently: the four
season lists go stale every summer, an override list outlived its situation and
kept applying, a cache version bump was defeated by a stale file. A convention
saying "also write down what you ran" would rot the same way, so nothing here
depends on one.

Two thirds of it happens without asking. A sweep with `FPL_CELLS` set writes a
`*.provenance.csv` sidecar beside its cells before the first cell is computed; a
diagnostic with `FPL_MODEL_CSV` set writes its own numbers alongside the table it
prints. The remaining third needs R, so it cannot live inside `go test`, and
`fplagent snapshot` does the remembering instead — it runs the inference itself,
defaults every path, finds the previous snapshot on its own, and is meant to be
invoked with no arguments.

### The three steps

```bash
# 1. Model accuracy. Eight diagnostics, each against outcomes rather than against
#    another setting of the model. These may be run in parallel through
#    scripts/replay, which compiles once and runs the binary in a child process
#    it waits for (not an exec) rather than paying
#    for a resident `go test` driver per run -- that driver, not the replay, is
#    what got runs killed under load. A killed run is still a silently partial
#    result, so check the exit status the wrapper reports.
#    Seven are seconds; TestDiagTransferError replays the grid x 3 start points,
#    which is 18 cells today, and takes two to three minutes.
export FPL_MODEL_CSV=/tmp/model.csv DIAG=1
go test ./internal/backtest -run TestDiagCalibrationDrift    -count=1 -v -timeout 60m
go test ./internal/backtest -run TestDiagTransferError       -count=1 -v -timeout 60m
go test ./internal/backtest -run TestDiagDefconBias          -count=1 -v
go test ./internal/backtest -run TestDiagCleanSheetPoisson   -count=1 -v
go test ./internal/backtest -run TestDiagSixtyMinutes        -count=1 -v
go test ./internal/backtest -run 'TestDiagTeamBlend$'        -count=1 -v
# The prediction benchmark's own per-gameweek cells go to a second file; the
# model CSV picks up its printed tables either way.
FPL_PREDICTION_CSV=/tmp/prediction.csv \
  go test ./internal/backtest -run TestDiagPredictionBenchmark -count=1 -v -timeout 60m
go test ./internal/backtest -run TestDiagNextFivePredictors  -count=1 -v -timeout 30m

# 2. Harness accuracy. Any sweep block works; it writes its own provenance.
#    EXP must name a block THIS diagnostic has — TestDiagProjection offers
#    MINHL, MINW, BONUS, DCC, BENCH, FIXW and MINK, while A-J belong to
#    TestDiagTransferPolicy and TestDiagRejudge. Naming another diagnostic's
#    block used to run nothing and pass in 0.00s; it now fails and lists the
#    blocks it does have.
FPL_CELLS=/tmp/cells.csv DIAG=1 EXP=MINHL \
  go test ./internal/backtest -run TestDiagProjection -v -timeout 180m

# 3. Render. Runs variance_components.R itself and diffs against the last snapshot.
#    Pass the paths explicitly whenever step 1 or 2 wrote somewhere other than
#    the defaults (/tmp/model.csv, /tmp/cells.csv) — the renderer does NOT read
#    FPL_CELLS or FPL_MODEL_CSV, and a leftover file at a default path will be
#    picked up in silence. It warns afterwards; -cells and -model avoid it.
fplagent snapshot -cells /tmp/cells.csv -model /tmp/model.csv
```

`-count=1` is not optional on step 1: Go caches test results, and a cached run
replays its stdout while writing no new CSV rows.

Useful flags: `-note "..."` stamps a caveat in (repeatable), `-no-r` reads whatever
is already in `stats/out` instead of re-running the inference, `-constants` prints
the full settings list for a fingerprint and exits, `-previous DIR` diffs against a
chosen snapshot rather than the latest.

### What every snapshot is stamped with

Provenance is the whole point. The costly failures in this project's history are
not wrong numbers but orphaned ones: a body of evidence was measured with the
transfer gate's minimum-gain threshold at 0.7, the value was retracted to 0.4
three commits later, nothing recorded the link, and a later audit cited the
evidence as ground truth. Separately, a six-arm sweep was killed under load after
three arms and the gap was invisible until somebody counted rows.

| stamped | why it has to be |
|---|---|
| commit SHA, and whether the tree was dirty | a dirty tree is recorded, not refused — refusing would only mean the measurement got taken with no stamp at all |
| a fingerprint of every shipped constant in force | 12 hex characters covering ~174 config values plus any `FPL_*` switch set. Generated by walking the config, never hand-listed, so a new field is covered the day it lands |
| the cell grid | which seasons, which entry gameweeks, and the resulting cells per arm |
| the free-transfer bank rule | sweeps pin the modern five-transfer bank for every cell, which is historically wrong for 2022-23 and 2023-24. That caveat governs roughly half of this project's evidence and used to live only in a code comment |
| which arms completed and which were killed | **the one thing a cells file cannot say by itself.** Arms are declared before the first cell, so a kill leaves the declaration behind and the gap becomes arithmetic |
| invariance checks | quantities the change must not move, and whether they moved |

`FPL_SESSION` is deliberately excluded from the fingerprint: it is a credential and
a snapshot is committed.

### Two properties worth knowing before trusting one

**Inference is read, never recomputed.** Every standard error, degree of freedom
and minimum detectable effect in a snapshot is `variance_components.R`'s
arithmetic, published to `stats/out/<label>/mde.csv` and carried across unchanged.
⚠️ **Corrected 2026-08-15: this said `stats/out/mde.csv`, a bare path nothing
writes.** `variance_components.R` writes one directory per sweep and never to bare
`stats/out` — which is the whole point of the warning in `regenerate_mde.sh`'s
header, where a twelve-cell demo run once wrote to the bare default and its figures
were read as current for weeks. A reader following the old path here would have
found nothing, or worse, found a stale file somebody had put there by hand.
`TestThisPackageDoesNotComputeInference` fails if `internal/snapshot` grows a copy —
the companion to `TestInferenceLivesInOnePlace`, which scans only
`internal/backtest` and so cannot see it. The temptation is specific: the MDE is two
lines given the components, and the renderer already reads the components. Yielding
would be the bug class behind `DefaultBenchWeight` against `Weights.BenchWeight`,
where one quantity had two implementations and the measured one was not the one that
ran.

**An absent section says it is absent.** A missing input is never fatal and never
silent, because a section that quietly omitted its numbers looks much like a section
that had nothing to say — and this project has already recorded a null result that
was really a measurement that never ran.

### A diagnostic that deduplicates must iterate in a fixed order

The diff is only worth having if a movement means something changed, so a diagnostic
returning different numbers from identical inputs does not merely add noise — it
disables the feature, because a reader who learns the comparison always shows
changes stops reading it.

Building the first snapshot found one. `TestDiagCleanSheetPoisson` keeps one
representative row per team-match and iterated `Season.Players`, which is a map, so
which team-mate represented each match varied per run and identical runs disagreed by
0.7%. **Accumulating over a map is safe** — addition commutes, give or take the last
bits of a float — **and selecting from one is not.** Use `sortedPlayerIDs` in any
diagnostic that picks one of several equivalent rows;
`TestModelDiagnosticsAreReproducible` (also behind `DIAG=1`) fails if the selected
sample stops being stable.

### Which cells files can be used

A cells file written before the provenance stamping existed is still perfectly
good for the variance components and the minimum detectable effect. What it cannot
support is arm accounting: the arms that emitted cells are not the arms that were
asked for, and a snapshot built from such a file says so in those words rather than
implying the grid was complete.

`stats/testdata/minutes_half_life_cells.csv` is one such file. It is a frozen
120-row fixture kept **only** so `shape_inference.R --selftest` stays reproducible,
it has no provenance sidecar, and it predates the captaincy rungs — its header is
23 columns ending at `weekly_per_gw`, and carries none of the blocks added since.
(That sentence used to name the current width too, and the very next block added
falsified it. The schema grows with every block, so the figure to quote is the
fixture's, which cannot move.) Do not re-derive a constant from it, and
do not use it as a snapshot's harness half except to demonstrate the mechanism.

## `shape_inference.R` — is the *order* of the settings reproducible?

The other two scripts both compare one setting against another and both run into
the same wall: a season is the independent unit and there are six of them, and the
smallest effect that design can see is typically 35 to 65 points a season — per
comparison, measured on the four-season grid, and as high as 232 for a
season-varying one — while
every constant this project argues over is worth 11 to 34. Head-to-head
comparison therefore returns "unresolved" almost always, and that will not change
much — the Understat backfill has already bought every season it can, since 2018-19
publishes no `teams.csv` and 2016-17 and 2017-18 no `fixtures.csv`, and a season that
cannot be played cannot be a cell. After that the axis grows by one season a year.

A sweep has a second axis nobody pools over: **the settings themselves**. Five
settings give an *ordering*, and an ordering is much cheaper to establish than a
magnitude, because seasons disagree wildly about how big an effect is and much
less about which setting is better. A season that scales every effect up does not
reorder them.

```bash
# The order must be given on the command line — see below for why.
Rscript stats/shape_inference.R --order=numeric /tmp/cells.csv

# With the two cheap extras: a quantity the knob controls directly, and a
# quantity it must leave alone.
Rscript stats/shape_inference.R --order=numeric --mediator=moves \
  --invariant=hold /tmp/cells.csv

# Its own regression test. No replay, no cells file, no R packages.
Rscript stats/shape_inference.R --selftest
```

It reports three things about order and nothing about size:

- **A trend test.** Within each cell the settings are ranked worst to best; each
  setting's ranks are added up across cells to give one rank sum per setting;
  those sums are multiplied by the setting's position in the predicted order (1
  for predicted-worst, 5 for predicted-best) and totalled into a single **trend
  score**. If the cells keep agreeing with the predicted order, the big rank sums
  land against the big weights and the score is large. Under "every setting is
  equally good" the score's average and spread are known exactly, which turns it
  into a z-score and a p-value. It is computed **twice** — once over all the cells
  and once over the season means — and the second figure is the one to
  believe. At a handful of blocks the p-value is exact, obtained by listing every
  ordering of the settings and convolving, because the bell-curve approximation
  is poor there and a handful is what honest clustering leaves. The script resolves
  the block count from the cells it is handed rather than assuming a grid.
- **Where the peak is, rather than how high it is.** The winning setting in each
  cell, with the distribution over cells and an exact binomial test against the
  1-in-*k* a coin toss would give, Holm-adjusted for having asked about every
  setting. "The peak sits at 4 in 9 of 24 cells, at 20 in 6 and at flat in 4"
  replaces "4 wins", and it is the direct answer to this project's own standing
  complaint that a single argmax out of five swept values is not evidence.
- **Per-season ordering consistency.** For each season on its own, how many of
  the pairs of settings it puts in the predicted relative order, beside its own
  best and worst setting. This is the brake on the trend test: a pooled statistic
  can look strong while the individual seasons disagree.

Plus two checks that cost nothing and answer the same question from the side:

- **`--mediator=COLUMN`.** A knob's first-order effect is often not points. A
  transfer gate's is the number of transfers made, which the harness counts
  directly and almost without noise. The same trend test is run on that column,
  two-sided, because the claim about a mediator is only that it moves in step. A
  mediator that moves monotonically while points stay flat is evidence the points
  really *are* flat — the knob demonstrably did something and it was not worth
  points. A mediator that does not move says the flat points column means nothing
  yet and the wiring should be checked. Four seasons of points cannot tell those
  apart.
- **`--invariant=METRIC`.** Name a quantity the change must **not** move and
  check it. A transfer-only knob must leave HOLD byte-identical, because HOLD
  makes no transfers. A violation shows up in a single cell, where *confirming*
  an effect on POLICY needs tens of points a season and can need 232, so
  falsification is enormously cheaper than confirmation here. Passing prints "byte-identical in
  all cells"; failing names the worst offending cells.

### The order must be committed to in advance, and the script enforces it

Looking at the data, noticing an order, and then testing that order makes the
p-value meaningless. So there is no default: without `--order` the trend test does
not run and the script says why. Three ways to give one, all of them a decision
made before reading the output:

| `--order=` | meaning |
|---|---|
| `numeric` | ascending by the single number in each arm label. `numeric-desc` for descending |
| `index` | the order the Go sweep declared the arms in |
| a comma-separated list | arm labels (exact, or any unambiguous substring), predicted-worst first |

The order used is printed and written into `stats/out/shape.csv`, so the record
shows what was predicted rather than what was found.

### What it cannot do

**Shape adds an axis inside the season wall. It does not escape it.** The
trend test over 24 cells as though they were independent gave p = 0.0001 for the
minutes half-life ladder; the cells are demonstrably not independent on POLICY,
which is exactly what a real season-to-season difference means. Clustered to four
seasons the same ladder gives an exact p of 0.0017 — still notable, and a much
weaker claim, because it is a statement about *order only*.

**An order holding does not endorse a setting in it.** For the minutes half-life
the order that scored p = 0.0001 was "a longer half-life is better", and the
longest setting tried has the highest rank sum while the shipped setting is
second. What that establishes is that the two shortest settings are worse than the
other three. It is silent on which of the remaining three is best — which is also
what the peak distribution says.

The method helps most where the cells are nearly independent, which is HOLD; and
HOLD is where this particular shape signal happens to be weak (z = 0.90).

### `stats/testdata` and the self-test

`--selftest` runs two kinds of check and needs no replay, no cells file and no R
packages. Arithmetic invariants pin the formulas: the null average and spread at
the two shapes this harness produces, the exact enumerated null distribution
reproducing the closed-form average and spread (a genuine cross-check, since the
two are derived completely differently), a perfectly monotone sweep hitting the
largest attainable trend score and a reversed one the smallest, and an all-tied
sweep landing exactly on the null average rather than erroring.

Then one worked example end to end, against
`stats/testdata/minutes_half_life_cells.csv` — a frozen 120-row cells file from
one real `MinutesHalfLife` sweep, committed **only** so that example stays
reproducible. Every number in it was verified by hand: rank sums 53 / 55 / 84.5 /
79.5 / 88 totalling 360, trend score 1174.5 against a null average of 1080 and a
spread of 24.49, so z = 3.858; the peak at half-life 4 in 9 of 24 cells; "flat is
the worst of five" in 12 of 24 cells but only 2 of 4 seasons.

**It is not a source of truth about any constant.** It is one sweep's output,
kept as a fixture. Do not re-derive a default from it; run a fresh sweep.

The verdicts it produced on the shipped grid are written up in AGENTS.md under
"An ordering is cheaper to establish than a gap, and it is a second axis".

## `entry_density.R` — would more entry gameweeks buy resolution, or duplicates?

AGENTS.md records the entry-gameweek axis as the one noise remedy its own
measurements support on the held metric, from

    Var(mean) = (sigma2_season + sigma2_resid / G) / S

with G entry points per season and S = 4 seasons, and publishes a prediction: the
season-clustered SE on HOLD should fall 0.515 at the shipped G = 6 to 0.364, 0.257
and 0.182 at G = 12, 24 and 48. The `sigma2_resid / G` term assumes the G
within-season residuals are **independent**, and entry points are strictly nested —
`SimConfig` carries `StartGW` and no `EndGW`, so every window runs to GW38 and an
entry at GW2 shares 37 of its 38 gameweeks with an entry at GW1. If adjacent cells
are near-duplicates the shrinkage is fictional, and densifying manufactures
confidence the way the budget-jitter axis would have.

```bash
# 1. Replay a two-arm positive control on a grid that over-samples short gaps.
#    ~1 hour. Run nothing else: this machine drops replays under load.
DIAG=1 EXP=DENSITY FPL_SWEEP_STARTS="1,2,3,4,6,11,16,17,18,19,21,26" \
  FPL_CELLS=/tmp/density/cells.csv \
  go test ./internal/backtest -run TestDiagEntryDensity -v -timeout 180m

# 2. Ask what the spacing did.
Rscript stats/entry_density.R /tmp/density/cells.csv

# 3. Its own regression test. No replay, no cells file, no R packages.
Rscript stats/entry_density.R --selftest
```

Options: `--metric=` (default `hold`), `--out=DIR` to write `entry_density.csv`.

### The reframing that came out of building it, which narrows the question

On a balanced S x G table with within-season covariance c(h) at gap h and c(0) = v,

    E[MS_season] / G  =  Var(season mean)  =  cbar + (v - cbar) / G
    E[MS_resid]       =  v - cbar

where `cbar` is the average off-diagonal covariance. So `sigma2_season` as
`variance_components.R` reports it — `(MS_season - MS_resid)/G` — **is** an estimate
of `cbar`. Three consequences:

- **An exchangeable within-season correlation and a genuine season effect are the
  same model.** They are not separable at any sample size, because both do exactly
  one thing: make the four season means more variable. AGENTS.md's formula therefore
  already accommodates correlated entry points, and its measured `sd_season = 0` on
  HOLD is already the statement that the average within-season covariance is zero.
- **The measured clustered SE is unbiased whatever the correlation is**, since it is
  just `sd(season means)/sqrt(S)`. Only projections to grids that were never run can
  be wrong.
- **What is not accommodated, and is the only thing at risk, is gap dependence** —
  nearby entry points more correlated than distant ones, which would make `cbar` grow
  as a grid is densified. That *is* separable from a season effect, because it varies
  with the gap and a season effect does not.

So the question is not "are the cells correlated" but "does the correlation depend on
the gap", which is a strictly smaller claim and a far better-conditioned measurement.

### What it reports

1. **The variance components on this grid**, with `cbar` and the implied mean rho.
2. **Correlation against the gap**, from the variogram
   `gamma(h) = Var_s(d[g1] - d[g2]) / 2`. Differencing two columns cancels the season
   effect exactly (same season) and absorbs the start-point effect into a constant,
   which a naive column correlation does not. Read two ways: `rho = 1 - gamma/v` is
   absolute and noisy, because `v` needs `sigma2_season` on `S − 1` degrees of freedom;
   `excess = 1 - gamma/MS_resid` is relative to the grid's average pair, immune to
   `sigma2_season`, and is the precise test of gap dependence. Beside them,
   `ceiling = sqrt(w_short/w_long)` — what two entry points would correlate at if they
   scored the same football with the same squad, which is the only sensible upper
   benchmark for a nested design.
3. **A fitted `rho(h)`**, exponential-to-a-floor, used only to price grids that were
   not run. It falls back to a flat model and says so when the exponential degenerates,
   which is what a no-decay dataset does to it.
4. **Measured clustered SEs on sub-grids of the run**, including matched tight and
   spread grids at the same G — and then the same question over **every** sub-grid of
   each size, grouped into thirds by mean pairwise gap. One hand-picked sub-grid rests
   on four season means; averaging over all of them is a much better use of the same
   cells. Sub-grids share cells, so it is a descriptive contrast, but it is
   assumption-free about the correlation structure, which the extrapolation is not.
5. **The extrapolation to an evenly spread grid**, against the published prediction.

### Three things it is easy to get wrong

**This grid's own SE is not "the SE at G = 12".** It over-samples short spacings on
purpose, so an evenly spread twelve would do better. The prediction is tested through
the fitted curve, and that is stated as a fit.

**The published 0.515 is not this comparison's SE.** It is the `MinutesHalfLife`
sweep's figure **pooled over four arms**. The like-for-like check is the *ratio*
column, plus the recorded CR2 SE for the arm actually replayed here — `MinutesWeight`
1.00 against 1.25 on HOLD, −0.709 pts/gw at CR2 t = −5.95, so SE ≈ 0.119.

**`tapply` orders columns by the string form of the gameweek**, so "11" sorts before
"2". The script reorders numerically; without that every gap in every table is wrong,
and wrong in a way that still prints a plausible decaying curve.

The verdict it produced is written up in AGENTS.md under "Densifying the entry-point
grid".

## `rank_robustness.R` — would scoring on rank change the verdict?

Every sweep here is judged on **points**, and FPL pays **rank** — your percentile among
the roughly eleven million entries. The standing worry is that the two disagree and the
verdicts are therefore measuring the wrong quantity. This script settles that **without
ever modelling the field**, off any cells file already on disk.

```bash
# Any number of cells files; each sweep and metric in them is reported separately.
Rscript stats/rank_robustness.R /tmp/hits-cells.csv stats/testdata/minutes_half_life_cells.csv
```

No options and no packages — it is base R, and it needs nothing the cells file does not
already carry.

The argument it rests on is that the field does not react to our policy, so both arms of
a paired comparison meet the same field distribution `F`. Percentile is `F(x)` with `F`
monotone increasing, so **no individual cell can change sign**. What rank-scoring *can*
do is reweight the cells — a percentile difference is about `f_i × d_i`, with `f_i` the
field's density at our score — and a mean over disagreeing cells can change sign though
no cell did. So the script asks how fragile each arm's mean is to that reweighting, and
prints two statistics per arm plus the exactly-computable one:

- **`R_crit`**, the *adversarial* bound: the smallest weight ratio that flips the sign if
  the weights conspire with the cell-level sign pattern. Closed form `P/N`, with `P` the
  sum of the positive paired differences and `N` the absolute sum of the negative ones.
  It badly overstates the risk and is printed because a reader will otherwise reach for
  it.
- **`P(flip)`**, the random-reweighting figure (⚠️ **not** the realistic one — computed
  weights run 17x to 2,343x; see the scope block in the harness note): the share of 20,000
  random reweightings, drawn
  log-uniform on `[1, R]`, that change the sign. Log-uniform because a density ratio is
  multiplicative. `R = 5` is the pessimistic column — `sqrt(38/13) = 1.71` from cell
  length alone, times 1.6 to 3 for how far our squad sits above the field mean.
- **the sqrt-wk mean**, the one reweighting that needs no field model: a cell entered at
  GW26 banks 13 gameweeks against 38 for GW1, a field total's spread scales as
  `sqrt(weeks)`, and the cell's total difference is `weeks × d` — so a per-gameweek
  difference carries weight `sqrt(weeks)`.

**The invariance to read first: no arm's sqrt-wk mean may differ in sign from its
equal-weight mean.** The derivation says a positive reweighting cannot flip a cell, so a
violation means the implementation is wrong rather than that rank-scoring is dangerous.
The script flags such a row `** sqrt-wk FLIPS **`.

An arm that is byte-identical to the baseline is reported as an invariance check passing
and not reweighted — a transfer knob on `HOLD` is the usual case, and dividing by zero
there would be nonsense.

It carries **no `--selftest`**, unlike the four scripts above. Its one exact quantity is
`R_crit = P/N`, derived in the script's own header, and its invariance check is printed
in the output rather than asserted.

The verdict it produced is written up in the harness-and-inference note under "A
rank metric reorders only what was already unresolved": no arm with `|t| >= 1.72` flips
under a random reweighting to 5x, against 10-32% for arms below `|t| = 0.6`, so
rank-scoring reorders only what the harness had already declined to resolve.

⚠️ **Do not derive that verdict from the Spearman −0.977 this paragraph used to quote.**
Under weights drawn independently of the data — which is what `P(flip)` simulates —
`P(flip)` has a closed form in `|t|` alone, reproducing every arm to 0.74 points and
correlating with `|t|` at exactly −1.00. The −0.977 is therefore that algebraic identity
recovered by Monte Carlo, a property of the simulation design rather than a discovered
fact, and it was quoted as evidence here until review caught it. The verdict survives on
the stronger test the note reports instead: **computing** the reweighting from ownership
marginals rather than drawing it, across 60 specifications, where the correlation with
the exact flip share is **−0.315**.

⚠️ **And read that box on `HOLD`, not on `|t|`.** "No arm above `|t| = 1` changes sign"
is the wording it first carried, here and in three other files, and the box contradicts
it: `POLICY` `half-life 2` is at `|t| = 2.90` and flips in **18.3%** of specifications.
What holds is that **no `HOLD` arm above `|t| = 1` flips** — the four `HOLD` arms come in
at 0.0%, 0.0%, 1.7% and 10.0% against 0-78% on `POLICY` — so the defence is structural in
the *metric*, which matters because `HOLD` is the metric this project judges scoring
constants on. Six arms flip at all, not the three the note's prose once named.
