# A reversal that shrank to one arm, half of it a changed estimand

Covers `95982db..d6a61e8` — the data-state check, the two-anchor run, and the corrections two
reviews produced. Includes a merge of `origin/main` (block J's minutes-floor work), reviewed on its
own branch and not re-reviewed here.

## What was being asked

Banking `BlendRateK` reversed a recorded 24-cell table. Two runs, both pre-registered:

1. **The documented data state.** `FPL_SWEEP_SEASONS=default FPL_NO_XG_REPAIR=1`, the recipe
   CLAUDE.md gives for pre-2026-08-10 figures. Result: −0.326 / −0.052 / +0.351 against a recorded
   −0.632 / −0.740 / −1.509.
2. **The pre-split arm.** `shrinkToLeague` read `BlendRateK` until `c509255` split out
   `LeagueShrinkK`, so the recorded ladder moved two anchors and today's moves one. `BLEND2` sets
   both. Result: the second anchor is worth **−0.841 pts/gw at `k=24`**, ~45% of that arm's gap,
   while moving `k=12` and `k=16` the other way.

## Reviewers dispatched

`stats/*.R` was untouched; the change is `docs/`, `CLAUDE.md`, `TODO.md`, a Go test block, and two
snapshots, and it is an attribution claim about a scoring constant. Union: **fpl-stats-review** and
**fpl-findings-audit**, run concurrently on committed state.

**Not owed:** code-review (the only code is a new sweep block that adds an arm and changes no
shipped path), security, run-review, season-maintenance.

## The finding neither I nor the first round caught

**The arms stopped being the same intervention**, and it was invisible for two rounds because the
arm *literals* are identical (8/12/16/24). fpl-findings-audit found it by reading `blend.go` at both
ends of the range. Verified before acting:

```
blend.go:407   k := e.Weights.BlendRateK       # c261f32, the recording commit
blend.go:407   k := e.Weights.LeagueShrinkK    # today, since c509255 (2026-08-10)
```

That is this record's own *"folding two levers into one arm measures their sum and neither"*,
pointed at the recorded row — and it qualifies a resident claim: "splitting `LeagueShrinkK` out
changed nothing" is a **simple-effect null taken at 8/8**, while every pre-split arm above 8 moved
both anchors.

Testing it was one run, not a nine-run bisect, because shipped `LeagueShrinkK` is 8 — so the two
runs share an identical baseline and the second anchor isolates as a per-cell paired contrast.

## Two of my claims were outright errors

### 1. The regime comparison was a category error

I wrote that the recorded "helps early, hurts mid and late" fails to reproduce. The recorded triple
**+0.936 / −0.611 / −1.783 is the `POLICY` `k=12` row**, not `HOLD`. The proof is arithmetic and I
verified it: equal-sized phases average exactly to the row mean, and those three average to
**−0.486**, which is that row's recorded mean. I compared a `POLICY` triple against `HOLD` columns —
twice, in two documents. On the metric it was actually claimed on, "helps early, hurts middle"
**reproduces**.

### 2. "These magnitudes are refuted" ignored uncertainty that was in hand

The old harness printed `mean` and `t`, so the recorded SEs were recoverable as `SE = |mean/t|` →
**0.290 / 0.471 / 0.665**. Against conservative independent-run SEs the gaps are **t 0.44, 0.82,
1.84**, and the recorded −1.509 sits **inside** the re-run's CR2 interval `[−1.55, +2.25]` — the
estimator this record says to prefer. Only `k=24` is arguably discrepant, and the **`POLICY` row
replicates at both data states**.

So the retraction went from *"THIS TABLE DOES NOT REPLICATE"* to one number, about half of which is
a dated code change, with a residual inside the instrument's noise.

## Smaller corrections applied

- **The CLAUDE.md caveat's absolute form is refuted by CLAUDE.md.** I wrote "anything recorded before
  2026-08-12 is not reproducible by switches alone"; the vice-captain 08-11 snapshot reproduces
  bit-exactly across those repairs. Narrowed to a reachability caveat.
- **"The data state is eliminated" was wrong, and my own document said so two sections later.** Only
  **24 of 96 cells differ, all 2022-23** — `applyXGRepair` returns early for seasons with no
  `repairdata/` spec, so three of four seasons could not run the intervention.
- **The bisect obstacle I named was the wrong one.** Verified: `.cache/fpl/` already holds
  `backtest-v7-` for all four grid seasons, so an old checkout **reuses** period-correct data rather
  than refetching. The real confound is that `c261f32`'s harness calls
  `config.Load("/Users/bbowman/Projects/fpl/config.json")` **with the error discarded**, so every
  pre-2026-08-10 arm would measure `config.Default()`.
- **The estimator-swap hedge is narrowed**: `c261f32` already banked `res.Points / len(res.Weeks)`
  per cell and called `reportPairedDifferences` — the same quantity R now computes at max abs
  diff 0. A swap can move a `t`, not the mean that moved.
- Counts: **nine** `season.go` changes, **514** commits.

## Root cause, not instances

The ~20 findings across two rounds are two habits, and both are now standing rules in CLAUDE.md:

- **A gap between two point estimates is not a result until it is divided by something** — and a
  recorded estimate's SE is usually recoverable from its recorded `t`.
- **A recorded regime triple belongs to a row, and equal phases identify which.**

Fixing the four documents would have been the symptom.

## Declined

- **Running the vice-captain suspect.** It is the best-named remaining candidate (`c5aa13d`, a day
  after the table, on the failing metric, with a real switch), but the residual it would explain is
  ~1.0 pts/gw at one arm — inside the comparison's noise. Recorded in TODO as *"chasing a difference
  the instrument cannot see"*, with the switch named so the next person can decide otherwise.
- **The bisect.** Retitled and downgraded rather than deleted; its real obstacle is now recorded.
- **Importing the reviewers' proposed prose wholesale.** Each correction is the minimum edit that
  makes the claim true; CLAUDE.md sat 26 bytes over budget after the additions and was brought back
  under by cutting my own now-wrong text, not by trimming a hedge.

## One reviewer figure did not survive recomputation

fpl-findings-audit reported that `POLICY` was dismissed on the flattering estimator, quoting
start-fixed t of **−2.17** and **−2.32**. The actual values in `inference.csv` are **−0.71, −0.62,
−1.57**. The recommendation (quote both ends) was right and is applied; the figures were not, and
the inference drawn from them does not hold. Recorded because "verify before applying" is the rule
and this is the third reviewer figure in two rounds to fail it.

## What could not be checked on this harness

- **The residual `k=24` gap**, ~1.0 pts/gw after the second anchor — inside the noise of the
  comparison. Unresolvable rather than unattributed.
- **Which single change moved it**, if any single one did. Two contributors are measured; the rest
  is not separable at this power.
- **The pre-2026-08-12 archive state**, which no switch restores.
