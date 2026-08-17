# Project context for Claude Code

A Go CLI that scores Fantasy Premier League players with a quantitative model, with a Claude
agent on top to reason over the output. Read [docs/architecture.md](docs/architecture.md) before
changing code and [docs/model.md](docs/model.md) before changing the scoring.
[docs/README.md](docs/README.md) maps the rest.

Everything in `docs/` is **reference** — what the system *is*. Design proposals and research
notes do not belong there; `docs/README.md` says so and says where the evidence lives instead.

This file holds the **verdicts**: one line per conclusion that has been paid for, plus the rules
that keep the next measurement honest. The evidence behind each conclusion is not in this
repository, so a verdict here cannot be re-checked from a checkout. Three things in the tree are the
exception: `stats/findings/` holds a narrative and a pre-registration per run, `stats/cells/` holds
the banked cells two R screens read as input, and `stats/snapshots/` holds the accuracy series.

## Build and test

```bash
go build ./... && go vet ./... && go test ./...
```

Tests hit the live FPL API and skip when it is unreachable. They assert invariants, not exact
values — the underlying data changes weekly, so a test pinned to a specific player or score rots
within days.

**Before merging anything to `main`, invoke the `merge-gate` skill and satisfy every line of it.**
Twelve conditions, all mechanical, each of which has caught something real. `review-gate` establishes
that a review happened; `merge-gate` establishes that everything else did. **This binds on every
agent that can merge**, the default one included. The three most often skipped: **`0 behind`
`origin/main` re-checked immediately before merging** (many sessions run concurrently and `main` has
moved five times in an afternoon); the leak scan across **three** channels, the branch name included;
and **merging the paired research-store branch in the same sitting**, because they are one unit of
work. ⚠️ A green gate is not a correct change — every condition in it is a process fact.

### Replay sweeps run through `scripts/replay`, and they run in parallel

```bash
EXP=G FPL_CELLS=/tmp/g.csv scripts/replay -run TestDiagTransferPolicy -v -timeout 2h
```

`go test` flag spellings are accepted and translated for the compiled binary — they do not pass
through to `go test`, which is not run, so a flag the binary does not know is rejected rather than
honoured. `DIAG=1` is set for you.

The wrapper compiles once under a build lock and runs the binary in a child process it waits for —
deliberately not an `exec`, because it has to outlive the run to report the exit status and the peak
RSS. One block measured both ways
on 2026-08-11 cost 97 MB resident instead of the 1031 MB the `go test` driver holds, at the same
speed, which is what makes parallel sweeps affordable. Banked sweep runs since span 89-142 MB;
budget from [docs/replay.md](docs/replay.md), not from the 97. It adds three guard rails:

- `FPL_REPLAY_SLOTS` (default 3) — extra runs *queue* rather than race.
- A per-run memory cap set above the measured peak, so it binds only on a run that has gone wrong.
  It exists only where a user systemd manager does; elsewhere the wrapper says so and runs uncapped.
- An exit status you can trust. A killed sweep leaves a partial cells file that reads downstream
  like a complete sweep with fewer arms.

It prints each run's peak RSS (peak resident memory). That is not decoration — the memory figures
above hold only until some arm makes them false. Details in [docs/replay.md](docs/replay.md).

## Conventions

- **A correction here REPLACES the claim; it does not narrate the replacement.** This file carries
  what is true now — one verdict per finding. Delete what a re-measurement falsified rather than
  annotating it as withdrawn; the history of a retraction, and the run that caused it, belong with
  the evidence, which is not in this repository. ⚠️ **Marking a withdrawal in place is correct where
  the evidence lives and wrong here, and the two being opposites is deliberate** — do not "fix" it.
  A resident file is read for the current state, and the narrative of how it got there crowds that
  out. ⚠️ **A deleted figure still owes its referent**:
  if a number survives the cut, whatever made it meaningful must survive with it, or the cut leaves
  the withdrawn reading as the only available inference.
- **Deterministic analysis never calls the LLM.** `internal/analysis` is pure computation. That
  is why five of the commands cost nothing to run.
- **Tool output is replayed on every later API call.** Verbose fields are paid for repeatedly —
  keep tool JSON compact, and put full detail behind single-player lookups.
- **Every scoring term is a separate, reported multiplier.** If you add one, expose it on
  `PlayerMetrics` and `playerRow` so the agent can explain a number rather than assert it.
- **New config fields need a backfill in `config.Load`**, so existing config files stay valid.
  One narrow exception: a hand-maintained list whose membership can legitimately be empty — the
  campaign maps and `tournament_absences`. There an empty list is a *statement* and only a missing
  key is an omission, so backfilling would conflate the two and silently undo a deliberate
  deletion. `analysis.CampaignMap` replaces on unmarshal instead. Fixed-arity lists such as
  `Review.Rules` and `MinutesWeightByPosition` still get backfills, because empty is meaningless
  for them.
- **Comments explain *why*,** especially where a value was calibrated or a bug was fixed. Several
  carry the data that justified a constant — don't strip them.
- **Gloss a Go identifier the first time a section uses it**, in four to eight words: "`XIValue`
  (what the transfer search scores a squad by: the best eleven plus the captain counted again)".
  Not every occurrence, just the first per section. This file is read as reference rather than
  front to back, so a section that opens on three bare identifiers is unreadable to anyone who has
  not just been in that code. The same applies to any label or abbreviation invented for a table.

## How to read the measurements

Six terms account for most of this file.

| term | what it means here |
|---|---|
| **`HOLD`** | buy the opening fifteen and never transfer, but re-pick the eleven and the captain every week, with substitutions applied. **Use this for anything about scoring**, because it excludes the noisy transfer decisions |
| **`POLICY`** | the same, plus the weekly transfer decision. **Only for settings that are themselves about transfers** |
| **cell** | one replayed season entered at one deadline. Six seasons × six entry gameweeks = **36 cells** per setting. Many figures here predate the widening and say **24 cells**, which is correct as a four-season figure; **12 cells** is three seasons by four entry points. Take the cell count from the figure, never from this row |
| **paired difference** | one setting minus the shipped one *within the same cell* — same football, same opening conditions, one thing changed. Always **per gameweek played**, because entering at GW1 banks 38 gameweeks and at GW26 only 13 |
| **pts/gw** | points per gameweek. **Multiply by 38** for the season-scale figures quoted here |
| **detectable** | a detection threshold belongs to a *comparison*, not to the harness. Median **39 points a season on the four-season grid, season-clustered estimator** — the same 23 comparisons read 32 start-fixed, and the six-season arithmetic is roughly 26. The quoted **3.9 to 232** span is pooled across both estimators; season-clustered alone runs 7.6 to 232 |

A few statistical terms recur:

- **CR2** — a cluster-robust standard error that treats each season as one group, so seasons that
  disagree widen the error bar rather than being averaged away.
- **Holm** — a correction applied when you test several things at once, so that testing more
  arms does not manufacture a winner.
- **MDE** — minimum detectable effect: the smallest true effect this comparison would find at
  80% power.
- **argmax** — see the box at the end of the standing rules. It is the most load-bearing idea here.

Where a constant cannot be resolved on points, this record decides on **mechanism** (does the
objective say what the game actually pays?) or on **shape** (a plateau with a cliff, or
monotonic movement across several settings) rather than on which single value scored highest.
Picking the highest of several noisy estimates manufactures effects.

## What the harness can resolve

**A detection threshold belongs to a comparison, not to the harness.** Across 23 real
comparisons on four seasons × six entry gameweeks, the season-clustered median is **39 points a
season** (start-fixed: 32). By metric the medians are **33 on `HOLD`** and **70 on `POLICY`**. A
change whose mechanism is certain — one that lands almost identically in every cell, like the
vice-captain fallback — resolves **at a threshold of 12.7**, on an effect of 0.436 pts/gw (16.6 a
season, `POLICY`). A setting whose effect swings by season needs **232**.

**Compute a threshold as `t_crit(df) × SE × 38`, not `2 × SE × 38`.** That is 2.571 at a CR2
degrees-of-freedom count of 5, which is what six seasons give — but **take the df from the
comparison**, since it is resolved per contrast and is often lower. `stats/variance_components.R`
prints the p = 0.05 effect and the 80%-power MDE per arm. Quote the range between the clustered
and start-fixed estimators rather than picking an end. Clustering is not always conservative:
where the start-point main effect is large, it makes the standard error *smaller*.

**The six-season grid is worth roughly 20-26 points a season on `HOLD`**, taking the
season-clustered degrees of freedom from 3 to 5. Read that as an ordering, not a point estimate.
Sweep transfer settings on six seasons too: widening helps on 10 of 11 arms, median ratio 0.62.

**Three switches disable the archive backfills**, for reproducing a figure measured before they
existed. `FPL_NO_XG_REPAIR=1` turns off the xG backfill *and* the xGC reconstruction;
`FPL_NO_XGC_REPAIR=1` turns off the xGC reconstruction alone, the narrowest of the three; and
`FPL_NO_XG_AGGREGATE` governs the xGC aggregate. `FPL_SWEEP_SEASONS=default` narrows the grid
back to four seasons. **Name the data state, or do not quote a level.** The
switches do *not* govern the archive defect repairs (phantom matches, duplicate rows) — those
are ungated, and `season.go` reads no environment variable at all. The four shipped seasons are
byte-identical inside the six, so grid width alone changes no cell. And on the four-season grid
the switches reach almost nothing: `FPL_NO_XG_REPAIR` moved 24 of 96 cells, all 2022-23, so a
failure to reproduce there is three seasons that could not respond, not an eliminated cause.

**Several different numbers for "the smallest effect this harness can see" appear across this
record**, written by different methods, each true of the thing it measured. Prefer the canonical
39 above. Common traps: **42/147** is pooled over one sweep's arms, **~150** belongs to the era
when totals rather than paired differences were compared, and **~10-12**
(`TestDiagNoiseFloor`) is a *floor* rather than a threshold.

Two things that are easy to get backwards:

- **A threshold is not a verdict.** Nearly every constant here is worth 11 to 34 points a season,
  so "unresolved" is the *expected* reading for a real effect of that size, and is not evidence
  against one.
- **The way to resolve something is to make the comparison sharper, not to run more cells.** The
  vice-captain fix resolves at a threshold of 12.7 points on the metric this file calls noisy,
  because its mechanism is certain.

**Absolute point totals are not comparable across eras of this codebase.** Fixed bugs moved them
by up to 115 points a season, and a bug that costs points unevenly across seasons does not merely
add noise — it invents shapes. Paired differences within a cell usually survive such a fix,
because both arms lost the same fixtures. Absolute totals across tables do not. Three specifics,
because surviving bullets rest on them:

- The doubles fix — `loadGameweeks` assigning where it should accumulate — is worth **+115
  `POLICY`, +106 `HOLD`**, so every absolute total before it *understates* the season by about
  that, and it dwarfs every constant this record argues over.
- Two later fixes cut the other way, making every earlier total an **over-statement**:
  substitutions FPL would
  never make (7-14 a season) and sales at the market price rather than half of any rise
  (~31 a season).
- One bug cost points unevenly enough to **invent structure** — an unset congestion penalty
  multiplied as 0 rather than skipped, worth 28 on 2023-24 and **113 on 2024-25**. It
  manufactured a monotone attacking-fixture ladder that does not exist, and reversed the
  flat-bench-weight verdict.
- Making defcon visible to the replay moved 2025-26 by **−95**, but it was one of four changes in
  that block. Do not attribute the movement to visibility alone.

## What each season can and cannot run

**A byte-identical season under an intervention is not a tie — it is a season where the
intervention could not run.**

| season | cannot run | note |
|---|---|---|
| 2018-19 and earlier | xG, xA, xGC natively | `tackled` and the full bonus-points component set are present 2016-19, which is what let the pre-2024-25 bonus schedule be decoded |
| 2019-20 | **`POLICY`** — FPL granted unlimited free transfers before the GW30+ deadline and froze prices for three months. Also xG and `starts`, both backfilled | scoring is fine, so it stays valid for `HOLD`. Rounds are numbered 1-29 then 39-47 |
| 2020-21, 2021-22 | xG natively; `starts` is recorded, from the Understat harvest | nothing on the **replay's** scoring path reads `Starts`: `reliabilityFrom` (the reliability blend over minutes share and start share) weights start share by exactly zero at the shipped `reliabilityMinutesShare` of 1.0, and `appearanceOdds` (P(appears) and its complement) reads it only in the non-shipped branch. So `FPL_NO_STARTS_REPAIR` is byte-identical at shipped config and is not a data state. That is a **simple-effect null**: under `FPL_NO_UNIFIED_APPEARANCE` or `FPL_RELIABILITY_SPLIT` it is untested, not inert. It *is* live on the agent path, where `tournamentAbsence` reads `Starts`, and in `OracleLineups` |
| 2022-23 | xG and `starts` for GW1-15 — one event, FPL adding fields mid-season | GW7 has no rows and GW8 is partial. That is real football, not a hole |
| every season but 2025-26 | defensive contribution ("defcon") | 6 live cells in 36, so widening the grid makes defcon *harder* to measure |
| all archived seasons | the full five-change 2026/27 bonus figure — no season carries both the modern saves baseline and a `tackled` column | the individual channels **are** measurable, and the joint CBI-plus-tackled arm is measurable on 2016-19 and unrun — it is the arm that would sign the keeper result. The four shipped seasons span three bonus regimes, so `Bonus90`'s *level* is not comparable across them, though paired comparisons are |

The earliest season that is recognisably the same game is **2013-14**. The boundary is the
introduction of the Bonus Points System — before it, bonus was awarded by human judgement in the
stand.

## Standing rules

- **Four classes take priority over anything else in the queue: security, performance, velocity,
  and a model or scoring fix** — in that order when they collide. The first two are unbounded and
  the second two compound. Security is currently **empty by construction**: there is no
  authenticated write path, and `TestTheClientHasNoAuthenticatedSurface` guards its absence.
  Performance changes what is affordable to run, which is the binding constraint on this whole
  enterprise. Velocity is the same argument one layer up: a recurring manual step or a duplicated
  implementation is paid every time. A model or scoring fix changes `Score`, therefore the
  ordering, therefore which footballers get bought — the only thing here that is *for* anything.
  This ordering is about **precedence, not worth**. It says which to do first when two are ready.
  It never says an unresolved constant is not worth measuring.
- **Convert per-gameweek figures by multiplying by 38. Never divide a pooled total by the cell
  count.** The six entry points give cells of 38/33/28/23/18/13 gameweeks, mean 25.5, so dividing
  by 36 understates by about a third.
- **A constant fitted against a proxy for its input is fitted to the proxy's noise too.**
- **Pre-register against a quantity that can actually move in the direction you would act on.** A
  raw standard deviation that contains its own floor cannot go down, so a threshold written
  against it buys nothing.
- **Review the plan, not just the output, for anything that will produce a number.** A brief can
  ask an arm to test a hypothesis it cannot discriminate — for instance when the acceptance
  criterion decomposes into the quantity being scored, so the arm gains by construction. That
  defect is invisible in a diff, and it is far cheaper to fix before the run.
- **An estimator swap reads as a data change.** A mean of per-club ratios is not a ratio of
  totals; gameweek-weighted is not equal-weighted. Name the estimator beside any figure.
- **A gap between two point estimates is not a result until it is divided by something.** A
  recorded estimate's standard error is usually recoverable: the old Go harness printed `mean` and
  `t`, so `SE = |mean/t|`.
- **An exactly identified moment fit cannot test the mechanism it is named after.** One free
  parameter matched to one moment reproduces that moment *by construction*, so agreement with an
  independently measured constant is a consistency check on one number, not a test — it passes for
  every rival mechanism reproducing that number. Quote the fit's own SE, and **name the season and
  the feed** of whatever it is compared against. Paid for 2026-08-15: a fit returning 1.2830 was read
  as confirming a measured 1.27 "to 1.0%", when it had zero degrees of freedom, SE 0.050, a family
  rejected on the same rows by the same output, and 1.27 was a different season on a different feed
  (season-matched: 1.3291).
- **Do not quote the parts of a decomposition that telescopes.** Where the intermediate point is
  *defined* by the identity being decomposed, the factors either side cancel and their sizes are a
  property of where the cut was made. Same rows, same product, two splits: −33.8%/+55.1% and
  +32.7%/−22.6%. Quote the product; never the wedges.
- **A recorded regime triple belongs to a ROW, and equal phases identify which** — they average
  exactly to the row mean, so the arithmetic names the metric before any comparison. `BlendRateK`'s
  +0.936/−0.611/−1.783 averages to −0.486, its **POLICY** row; it was compared against `HOLD` twice.
  ⚠️ **A compression pass may not delete a paid-for rule to fit** — that is what
  `TestTheResidentIndexStaysSmall`'s own comment forbids. Raise the budget instead.
- **Monotonicity that the construction forces is not evidence.** Removing a −1 line item can only
  move a rank toward whoever paid it. Read the *size*, not the direction.
- **Check which *file* a number came from**, not merely whether the archive has the field.
  `players_raw.csv` is the end-of-season snapshot behind `statusAt` (the replay's availability
  reconstruction); `data/captures/<season>/GW*/` is what the point-in-time oracles read.
  And **"the archive does not have X" is unverified until someone greps for X**, per season.
  Check every source the pipeline touches, not just the archive: **a field the code fetches and
  throws away is not a field the project lacks**. A column audit can only answer "can we
  reconstruct it"; it never asks whether the truth is recoverable somewhere else.
  The mirror also holds: **a constant having been swept does not mean its cells were banked.**
  Grep `stats/snapshots/*/cells/` for the sweep label before quoting one.
- **Check that a setting is read on the path you are about to score, before you score it.** "Read"
  means a consumer executes it on that path — not that a field of that name exists in the package
  you expected. A setting that never arrives returns a **byte-identical null**, which looks exactly
  like a null meaning the knob does nothing. Naming the consumer is the check; naming a package is
  not.
- **A byte-identical result is not a tie.** It may be a comparison that never ran. Check the
  mediator — the thing the setting is supposed to act through — before reading anything into it.
- **A null is a tie, not the refutation of one.** A non-resolving comparison shows two settings
  **cannot be separated by this instrument**. It does not show they are equal, and it cannot show
  they differ. What it *can* refute is a recorded **magnitude**. Reading "the point estimates moved
  apart" as "the tie is gone" retires a mechanism argument on noise.
- **A one-at-a-time null is a *simple-effect* null** — true of the shipped configuration and
  silent about any other. The chip-preparation transfer channel reads **+0.237 pts/gw (t 2.21)**
  with the rebuild lever blind, and **−0.003 (t −0.04)** averaged over both rebuild settings.
  Report the factorial main effect beside the simple one, and label which you are quoting.
  Invariance results are unaffected: byte-identical stays byte-identical at the configuration
  tested, and becomes *untested* elsewhere rather than false.
- **Two levers that a mechanism says touch the same decisions get a 2×2, not two sweeps.** The
  interaction is not the expensive contrast — a difference of differences *within one cell*
  cancels the path divergence that a single difference carries, measured here at a season-clustered
  SE of **0.216 against 0.599** for the noisier main effect, with no degrees-of-freedom penalty.
  The real cost is multiplicity: 2^k−1 contrasts against k, which is free at two levers and fatal
  at four. Cross **pairs a mechanism names**, never a lattice — an unpredicted interaction found by
  search is an argmax over 2^k−1 contrasts.
- **Holding a confounder constant is only safe when it is constant *with respect to* the thing
  being varied.** Pinning the wildcard to a common week put the bench boost immediately after the
  rebuild in 30 of 30 cells for one arm and 3-5 of 30 for the others.
- **A diagnostic must never carry its own copy of the thing it is checking.** A diagnostic is what
  everything else is checked against. Where a frozen baseline genuinely is wanted, pin it to the
  package with a test.
- **Size a canary with the omitted-variable coefficient, not with a raw variance share — and on
  the link the model actually fits.** A canary says "an effect this big would have to move the
  arm"; getting it wrong is how an arm passes a check it should have failed. The recorded miss,
  2026-08-16: the defensive-fixture hindsight gate sized its canary with the **OLS**
  omitted-variable formula `(b2−1)/q_w`, on a **log-link GLM**, without partialling the other
  regressor or the IWLS weights. It read 1.977; the quantity that sentence *named* is **1.685**,
  solved by refitting the misspecified model to expectations from the candidate data-generating
  process (a check with a built-in control — `b3 = 0` must return `b2 = 1.000000` exactly, and did).
  The threshold was 1.702, so the correct sizing **fires** where the erroneous one did not, and the
  error ran in the direction that **flatters the instrument** — which is the direction to expect,
  because the convenient formula is the one reached for. ⚠️ **Check the degrees of freedom the
  same way**: Satterthwaite gave **1.72**, not the assumed 2. And ⚠️ **a rank-deficient
  cluster-robust matrix can still be computable** — that gate's augmented CR2 was rank 3 of 4, so a
  declared numerical fallback could never have caught it.
- **One quantity, two implementations — this project's signature failure.**
  `TestTheSharedCellQuantitiesHaveOneImplementation` and
  `TestTheCopiedExpressionsHaveOneImplementation` scan for it. Extend an existing scan rather than
  adding a runtime equivalence test per copy: the second stops one divergence, the first stops the
  next copy. **A scan passing is not "there are no copies"** — both scans match an *idiom* keyed on
  one spelling of it, so they are tripwires rather than proofs.
- **A nonlinear transform of an archive-row field is not a statement about the model.** The engine
  prices the clean sheet on `m.XGC90` (expected goals conceded per 90: blended toward a prior
  season, shrunk, read point-in-time) through `cleanSheetProb`, inside a fixture and a defcon
  multiplier; a diagnostic fitting `math.Exp(-g.XGC)` prices it on realised single-match xGC. `exp`
  is convex, so the *gap between the two regressors* is construction rather than football. Take the
  regressor off `Metrics`, as `TestDiagCleanSheetRegressor` does.
  `TestNonlinearTransformsScoreTheModelsOwnRegressor` scans the backtest diagnostics for it. ⚠️ It
  catches a mismatched **regressor** only, never a mismatched **population**, and it is blind
  wherever the archive and `PlayerMetrics` (the struct the scoring path scores a footballer from)
  spell one field the same way — `Minutes`, `Starts`, `Goals`, `Assists`, `Bonus` and `TotalPoints`
  today.
- **"No truth value" and "a truth value we cannot resolve" are different, and only the second
  describes any constant here.** Every constant in this project has a right answer. What this
  harness lacks is the power to find it, which is a fact about the instrument and not about the
  constant. An unresolvable question is still owed an answer if the instrument improves; a
  meaningless one is not.
- **A better predictor can make a worse policy.** The transfer search is an **argmax**, so it
  lives in the tail of the estimate distribution rather than the middle. Removing a **bias** is
  safe; trading bias for **variance** is not. Ask which of the two a change does before making it.
- **A bias shared by every player in a position is not an ordering error; a within-position bias
  is.** The optimiser consumes an ordering and FPL forces five defenders regardless. "Shared" has
  to mean shared on `Score`, not on one of its components: a position-wide **multiplier on one
  additive term** reorders within the position whenever players differ in that term's share — the
  clean sheet is 26-45% of a defender's score and 0% of a forward's. The rule also needs a
  **quota** to bite, and it covers **ordering** only. `Optimize` is a knapsack against one budget,
  so a position-wide level can still change which defenders and which keeper are bought, and how
  many are fielded, even where the count owned is forced. Prefer the sibling rule, which is
  unqualified: **a measured bias does not imply a correction exists.**
- **Correcting a measured bias has lost points five times.**
- **An oracle that accepts on the sign of the quantity it is scored on raises that quantity by
  construction.** Check the **contrasts** as well as the levels, because a contrast inherits the
  construction of either leg.
- **A confinement check on a path that cannot carry the effect confirms nothing. Pair it with a
  liveness check that must move.** Confinement is usually a *code* fact, so re-running it can only
  fail. The check with power is the mirror: on the gate re-run, `hold_xpoints` had to move and did,
  in **144 of 144 cells**, against `hold_points` and `squad_hash` at 0 of 144 and the baseline's
  `policy_points` at 0 of **36** — mind the denominators. Sharper still, the
  two instrument-reading arms changed *decisions* in **20 and 27 of their 36 cells** where the
  points arm changed none — arrival on the scored path, not merely in a column. (`squad_hash` is
  the per-cell fingerprint of the fifteen, so a move in it means the squad itself changed.)
- **The wild cluster bootstrap may withdraw support from a CR2 rejection; it may never grant one.**
  Webb 6-point weights, enumerated exactly rather than sampled (6^6 = 46,656 draws costs 0.054 s,
  so there is no seed and no Monte-Carlo error). It swaps an unverifiable normality assumption on
  the season means for a **symmetry** assumption. On this balanced, intercept-only design CR2 *is*
  the equal-weighted t-test on season means, verified bit-for-bit, so the two cannot disagree by
  construction. It is a function of the season totals alone, so it cannot see cell-level
  concentration at all — that is `concentration_screen.R`'s job. **Quote `S_eff` (the number of
  seasons that actually contribute) and the floor `6/6^S_eff` beside every p**: 0.1667 at 2,
  0.0278 at 3, 0.00463 at 4, 0.000129 at 6. An arm whose floor exceeds 0.05 is *unmeasurable*, not
  null. For scale, the perfect armband — the largest effect in this record — enumerates to 0.0093
  on four seasons, which is why the four-season column is a season-agreement statistic rather than
  a magnitude.
- **A snapshot's figures are not guaranteed to have come from its own commit.** `FPL_MODEL_CSV`
  appends, its path is outside the repository, and the renderer keeps the newest row per figure —
  where "newest" means newest in wall-clock time. On a machine where several branches share a
  scratch path, a snapshot can carry another checkout's numbers under this one's name.
  `snapshot.ModelRunIDs` warns when the CSV holds more than one run.
- **A lock that looks necessary is evidence of a bug elsewhere.** Both recorded cases were money
  the pool would not release, not a valuation the optimiser got wrong. Correct the numbers and let
  everything downstream recompute — `lock` and `start` assert a conclusion the optimiser cannot
  decline. And **check the claim before you exclude on it.**

> **argmax** — "the option with the highest estimated value", and the single most load-bearing
> idea in this record. Take six noisy estimates and keep the biggest: the winner is usually not
> the best option, it is the option whose estimate got the most flattering noise. So the winner's
> value is **systematically over-stated**, and the more options you compare the worse it gets.
> That is why this record wants a *shape* rather than a winner, and why the transfer search — an
> argmax over players — reaches for whichever player the model most over-rates. Also called the
> winner's curse, or the optimiser's curse when a search does the choosing.

## Things that have already bitten

Shipped bugs, each now covered by a regression test. Re-introducing one is easy.

- **Never compare a replayed float for exact equality: a banked total is reproducible from a
  commit AND a machine, and only the commit is recorded.** Go's `math` is not bit-identical across
  machines — `Exp` has per-architecture assembly, `Log` has it on amd64 only, and amd64's `Exp`
  branches at run time on `cpu.X86.HasAVX && cpu.X86.HasFMA`, so two amd64 CPUs can disagree from
  one binary. `Log` is reached only through `Pow`. Live on `Score`: `cleanSheetProb`, the Poisson
  saves and concede blocks, the sixty-minute sigmoid, `defconCleanFactor`, `reliabilityFrom` **for
  midfielders only** (the other three positions ship an exponent of exactly 1), and the recency
  index. **Priors and team strength are not** — `BlendPriors` gets an integer exponent at
  `prior_half_life` 0 and `Pow` takes those exactly, and both team-strength sites ship off behind
  `FPL_MAGNITUDE`/`FPL_TEAM_FORM`. Measured: `math.Pow(0.5, 0.25)` is one ulp high on arm64,
  making a recency-weighted 90 minutes read **90 on arm64 and 90.00000000000001 on amd64**. Two
  tests compared exactly, so **CI was red on eight consecutive commits on those two assertions
  while green on every arm64 machine the work was done on** — the instrument was disabled by what
  it should have caught. (A snapshot-staleness failure is red alongside them and is unrelated.)
  Fixed with a tolerance (`sameMinutes`); **no test guards the spelling, so this is a convention
  rather than a guard.** Not fixed in production: swapping the transcendental would move every
  banked `xpoints` cell for no football, and the points columns by an unmeasured amount.
  ⚠️ **`hold_xpoints`/`policy_xpoints` are banked at full float64 and will not reproduce across
  machines**; the points columns are integers and `squad_hash` is a digest of the fifteen, so both
  reproduce unless a decision flips, **at a rate unmeasured, not unmeasurable — CI runs amd64, so
  one sweep there against a banked arm measures it.** The transfer search is an argmax and
  `cutByExpectedMinutes` is a cliff. Paired differences within a run share the machine, so the
  architecture channel cannot bias a comparison, though it does not make the difference's value
  portable. A byte-identity that is a *confinement* — a code fact that the path cannot carry the
  effect — is architecture-invariant; an empirical zero is not. Provenance records no `GOARCH`.
- **The doubles guard must key on `(element, fixture)`, never `(element, gameweek)`.** A real
  double gameweek has the identical shape to the archive's duplicate rows, so a gameweek-keyed
  guard would re-introduce the +115/season doubles bug while fixing the duplicates. `season.go`
  accumulates, never assigns. The archive holds 59 phantom matches in 2019-20 and 10 duplicate rows
  in 2025-26; both are dropped at load, counted, and pinned to those exact numbers with 0 in every
  other season.
- **Anything reading fixture results must be gated by gameweek.** `loadFixtures` originally dropped
  the scores entirely, so `teamBands` saw no completed matches and every band strength returned the
  baseline — measuring nothing while looking like a clean null. `playedFixtures` strips the
  scoreline and the `Finished` flag after the cutoff, and `TestPointInTimeHidesFutureResults` pins
  it. The archive holds every score for the whole season, so this is the one place a new feature
  can silently train on **results**. ⚠️ **It is not the only place it can train on the future** —
  the archive's `team_h_difficulty` is end-stamped and `playedFixtures` strips the scoreline but
  not the difficulty, and the `teams.csv` strength block is under the same suspicion. See "FPL
  revises team strength mid-season" under *The archive*.
  ⚠️ **And the PRE-SEASON path is unguarded, found 2026-08-16.** `PreSeasonWith` returns fixtures
  **unfiltered**, and `buildTeamRates` gates on a scoreline being non-nil rather than on `Finished`
  — under a comment asserting the opposite. `TestPointInTimeHidesFutureResults` has **never tested
  `through = 0`**, so the guard that pins the in-season path has never reached this one. Currently
  behind `FPL_MAGNITUDE`, so it is a latent leak rather than a live one. **Unfixed.**
- **FPL's aggregates reset at GW1, so the denominator must follow.** `minutes` and `starts` carry
  *last season's* totals until GW1 completes. Dividing by a fixed 38 reports an ever-present as
  2.4 min/gw after one gameweek, and nothing recovers until about GW29. Use `Engine.DataWindow()`,
  never the constant. The same applies to `MinMinutes`, which is written as a season total and must
  be scaled, or the pool is empty and the optimiser errors outright.
  `TestDataWindowTracksTheSeason` fails if either regresses.
- **Every per-90 rate must go through `blendFor`, counting stats included.** Bonus, saves and cards
  were read straight off the element while xG/xA/defcon were blended. Dividing a whole number by a
  fraction of a match made a 22-minute cameo with two bonus points read as 8.18 bonus per gameweek,
  and the replay's first transfers were chasing it. `TestCountingStatsGoThroughTheBlend` fails if a
  term goes back to raw.
- **A player with no prior is not a player with no uncertainty.** Promoted clubs and arrivals from
  abroad fall through `Priors.Get`, and scoring them on their own two appearances rebuilds the same
  explosion for exactly the players nobody has data on. `shrinkToLeague` pulls their rates toward
  the position's league-wide rates. Minutes are deliberately left alone.
- **`starts_per_90` is not a rotation signal.** It measures "when this player appears, does he
  start", and sits at ≈1.0 for nearly everyone. Using it rated a 26-min/week player identically to
  an ever-present. Use minutes and starts against the full 38-game season.
- **Single-swap local search stalls, and paired swaps are not enough either.** The optimiser needs
  a *paired* downgrade-and-upgrade move to fund a premium from cheap bench fodder; the symptom is
  identical XI scores across scenarios that should differ. Even then, asked whether a £15.5m
  striker fits, local search said 3.3 pts/GW while an exact search found a formation change fitting
  him for 0.26, by taking the money out of the goalkeeper. No sequence of swaps reaches that,
  because every step is downhill. `dpseed.go` solves each formation exactly by dynamic programming
  and seeds the local search — **do not "simplify" that away.**
  `TestOptimizerIsNeverWorseThanAnExactSeed` and `TestNoPremiumSquadBeatsTheOptimum` pin it.
- **The seed's bench reservation must take the *cheapest* players who could fill those slots.**
  `frontier` returns each position by ascending price; indexing from the far end reserved the most
  expensive possible bench, cut ~£20m off the eleven's budget, and left every seed too poor to buy
  a premium. Nothing failed — the DP silently stopped contributing.
  `TestSeedBudgetLeavesRoomForThePremiums` catches it. Locked players are pre-placed in the seeds
  rather than disabling seeding, so locking a player already in the optimum must reproduce it.
- **Never let the pair search choose greedily, and charge per move rather than per week.** The
  proxy that ranks candidates — the sum of the players' own score deltas — is exactly what misleads
  a greedy search, so it may filter but must never decide. Picking funding sales greedily scored
  2076 against 2158 for scoring the survivors properly; charging the week's decision once scored
  2110 against 2151. Two transfers really are twice the scarce resource.
- **A free transfer is not a costless transfer, and four points is the intuitive price and it is
  wrong.** Gated only on `min_gain_for_free_transfer`, the replay churned — twelve reversals across
  three seasons. Charged the full four-point hit value, it drops from 73 transfers to 39 and scores
  *below* charging nothing, because the gate stops filtering noise and starts refusing real
  improvements. `free_transfer_value` ships at 2.0 as a confidence threshold, not an opportunity
  cost, and **must not taper as the season ends**.
- **FPL banks 5 free transfers, not 2** — the rule changed for 2024-25. `backtest.BankLimitFor`
  keeps replays on the rule actually in force, or 2023-24 gets simulated saving transfers nobody
  could save.
- **Every engine that scores players needs the recency index.** `Simulate` builds three — one for
  transfers, one for the eleven, and one for `Hold` — and a patch once wired two and silently
  missed the transfer decision. Nothing failed: scores looked plausible, totals moved, and the
  whole gain was coming from better captaincy. `TestEveryScoringEngineGetsRecency` counts them.
- **Overrides are keyed by permanent player code**, because element ids are reassigned every summer
  and an override keyed on one comes back attached to a different footballer. Both solvers read
  `config.Roster` — an override the transfer search ignores is worse than none — and
  `TestExcludedPlayersAreNeverOffered` and `TestLockedPlayersAreNeverSold` fail if either stops.
  Per-call `lock_players`/`exclude_players` **add to** the standing set rather than replacing it.
  An indefinite override never lapses but is reported for review every run.
- **2019-20's rounds are labelled 1-29 then 39-47.** `loadGameweeks` drops anything outside 1..38,
  so before the fix its final nine rounds were discarded and the season scored as though it stopped
  in March — no error, a plausible-looking total. `renumberGW` maps them, and
  `hasRestartGameweeks` refuses a cache written before it existed.
- **`Optimize` is not run-to-run deterministic unless it is made so.** It ranged over a map to
  order each DP seed's bench, and returned two different fifteens from identical inputs on about
  one landscape in seventy-two. `TestSeedOrderIsDeterministic` pins it.

## Closed lines — do not rebuild these

Each was measured and lost, closed on mechanism, or withdrawn after re-measurement. A title alone
does not stop an idea being rebuilt, so each carries its **verdict**. The name in bold at the end is the note the evidence
sits in, not a file you can open here.

- **Do not build a custom fixture-difficulty rating, do not target the worst defences, do not band
  attack and defence separately, and do not move the fixture window.** The per-match effect is
  large and real — a defender facing one of the three bluntest attacks returns 21-41% above his own
  average — and every form of acting on it has lost points, because you never buy a fixture, you
  buy a run of them, and runs converge. Anything from two to eight gameweeks of lookahead performs
  the same. Read "lost points" as *never won measurably, at any setting tried* rather than as a
  measured loss: the whole apparatus is **unresolvable at current scoring**. Two of the four
  alternative defensive scales sit **above** shipped on the recorded ladder — 0 at **2172** and 1.5
  at **2178** against shipped **2152** — so zeroing the defensive response entirely *gains* 20
  points. That is inside-noise wobble and not a case for zeroing: the ladder has no shape either
  way, and the totals are 3 cells at one entry point with no threshold ever computed.
  → **fixtures-and-difficulty**
- **Do not extend recency to rates.** It predicts better out of sample and loses in the replay at
  every setting, because it buys accuracy on the average player and pays in noise at the top.
  → **recency-and-priors**
- **The clean-sheet over-prediction ships uncorrected.** `cleanSheetProb` scores `XGC90` (expected
  goals conceded per 90, blended toward a prior season, shrunk, point-in-time). On that regressor,
  predicted over actual is **1.052 on native-xGC rows** (n 1566, clustered SE 0.0081, df 2, so
  `t_crit` is 4.303 and the native ratio interval is [0.90, 1.20]) and **1.004 pooled**. The free fit is **b = 0.9922** (clustered SE 0.1516) and separates neither b = 1
  (t −0.05) nor b = 1.1731 (t −1.19); the 80%-power MDE on `|b−1|` is **0.424** against a candidate
  of 0.173. Two known biases both run the same way — the 90-minute population filter drops 14.2% of
  single-fixture club-gameweeks whose clean-sheet rate is 0.1992 against 0.2636, and the defcon
  coupling is omitted — and composing them moves the pooled over-prediction from ~0.4% to roughly
  **3.7%**. That is a composition of two separately-measured shifts, not a joint measurement.
  **The points question is closed as unmeasurable, by a canary.** The 2×2 ran on 36 cells,
  `HOLD`: factor **+1.9**, flat **+6.2**, both **+7.0** a season against thresholds of 23/16/20,
  Holm 1.000 (96% of the `both` arm is 2021-22). Then the canary — halving *every* clean sheet —
  costs **−21.6 against its own threshold of 28** and still does not resolve, so a candidate a
  quarter that size was about **4× below detection before the run started**. **Size a candidate
  against a canary before spending 180 cells.**
  ⚠️ **The mechanism is TWO things and neither replaces the other.** *Cross-match convexity* explains the gap between the two regressors and predicts it
  to **0.3%** — quote the exact `E[exp(−x)]/exp(−x̄)`, never `exp(σ²/2)`, which is good at `XGC90`'s
  dispersion and 8% high at the realised one. *A shot-level wedge* (`exp(−Σxᵢ)` against `Π(1−xᵢ)`)
  explains why `exp(−x)` over-predicts on realised x at all, and **its size is not established
  here** — the moment fit that appears to measure it is exactly identified and cannot test anything.
  ⚠️ **The near-calibration is a cancellation, not a structure, but the wedge sizes are an artefact
  of where the identity is cut and must not be quoted.** ⚠️ **And the fragility is in the MEAN, not
  the dispersion**: annihilating `XGC90`'s dispersion moves calibration 4.0% and its season range
  spans 2.5%, while a 10% shift in the mean moves it 4.1% — so the reconstructed-xGC seasons enter
  by the level channel. Sizes in `stats/findings/2026-08-15-clean-sheet-2x2.md`.
  → **scoring-model**
- **The clean-sheet factor `f` does not separate from 1; the defensive fixture ladder is what
  runs hot.**
  `fixtureSensitiveAt` scores `exp(−f·XGC90·def·cf)`, and because `ladder` is `1 + s(base−1)` the
  exponent is exactly `f·x + f·s·x·(def−1)` — linear in two columns, so the two channels are
  separately identified (they are orthogonal, r = 0.001). This is a **bound and cannot localise**:
  `def` is itself modelled — FPL's rank times this project's band adjustment — so the fit
  calibrates one part of the model against another rather than against truth. Fitted jointly
  (`stats/cs_calibration.R`): **`f` = 1.0476 (clustered SE 0.1612), t 0.30 — a failure to
  separate, not a calibration**, since the 80%-power MDE on `|b−1|` is 0.424 against a candidate
  of 0.173, so this fit could not have seen the candidate either way. Meanwhile the
  defensive fixture channel reads **1.5688 (SE 0.2253), t 2.53 native and 3.30 pooled**, above 1 in
  6 of 6 seasons, implying `s ≈ 1.50`. So the excess sits on `FPL_DEF_FIXTURE_SCALE`'s defensive
  half — which is points-null across a fourfold width change, making this **a calibration fact with
  no reachable points consequence**. This fit is post-hoc, and native season-clustering does not
  clear (4.14 on 3 clusters against a `t_crit(2)` of 4.303). `def` comes from the archive's
  end-of-season fixtures file and is ungated by the cutoff, **and it is end-stamped**: on the 308
  revised native rows `def` tracks the opponent's **end-of-season** strength at Spearman **0.872**
  against **0.421** for its value at the cutoff, and `def` is a per-club-venue constant all season
  in all six seasons. So the fixture channel is flattered.
  ⚠️ **The SIZE of the leak is unmeasurable, so this does not retract
  1.5688.** The hindsight gate puts the leak channel at **+0.846 (SE 0.396, t 2.14, df 2)** against
  a threshold of **1.702** — and a *full* artefact would require **1.685**, itself below that
  threshold, so the comparison cannot separate "entirely hindsight" from "none". Identified off
  **221 of 1566 rows**. Read this as *unmeasurable*, not as a refutation and not as a clearance.
  ⚠️ Whether FPL's **live** difficulty moved in-season remains a mechanism argument: the captures
  carry team strength but **no fixtures payload**. Adding fixtures to the weekly capture answers it
  by construction. **Do not open a points arm on this, and do not
  re-run at the refitted constants** (`f` 0.992, flat 0.939) — the factor arm is a no-op there.
  → **scoring-model**
- **Do not remove the bonus term for being circular.** It double-counts goals, assists and clean
  sheets, and shows exactly the bias signature that condemns a term — and removing it costs 67
  points a season, because it carries ordering signal the model never prices. `BonusWeight` ships
  at **1.5** against `BonusPriorWeight` **0.5**, with `bonusWeightFor` interpolating from the prior
  end toward the evidence end and approaching ~1.33 at 38 full matches, never 1.5. The older flat
  regime is reachable as `bonus_prior_weight: -1`.
  Two caveats on that 67. It is an **absolute total from a contaminated era**, and 66% of it is
  2024-25 alone — the season the zero-penalty bug was worth 113 — so read it as evidence that the
  ordering signal exists, not as a magnitude. And **leaning further in is worse too, but weakly**:
  the curve peaked at 1.0 on three GW1 `POLICY` cells, on absolute totals, as an argmax over five
  values, so 1.0 is better supported than 0.5 or 2.0 rather than established over them.
  → **scoring-model**
- **Do not penalise a squad for holding two players from the same club, and do not build a
  "talisman" rule.** Refuted on **arithmetic**, so no grid width reaches it. The 3/2/1 bonus pool
  makes teammates' bonus **dependent**, but dependence of any sign is a property of the pair's
  *variance*, never its mean: `E[B_i+B_j] = E[B_i]+E[B_j]` exactly. The claim substitutes the
  second moment for the first. Three supports, all checkable here: `Bonus90` (the blended per-90
  bonus rate a player is scored on) is a **realised marginal**, already earned while ten teammates
  competed for the same pool, so there is nothing to un-double-count; `xiValueShrunk` (the eleven's
  value on the transfer and replay path) sums **deterministic point predictions**, so there is no
  distribution for a covariance correction to enter; and ownership is not causal on the pitch —
  the pool is shared with the opposing eleven too, so the claim applied consistently would demand a
  penalty for owning one player from each side of a fixture. The **variance** reading is correct
  and is separately closed: acting on it means a risk-adjusted objective, which "do not chase a
  rank objective" closes for the expected-points manager it also argues is the right target, and
  leaves *unmeasurable* rather than open for a top-10k one. Note also that **measurability is not
  a reason to measure something the objective cannot consume** — the cheap archive-side falsifier
  (realised bonus covariance between same-club pairs, against zero) is scoped to **kill**:
  clearing it authorises nothing, because the objective still has no second moment to put it in. The "some clubs have talismans, so find them" reframing is a different
  claim, and its *level* channel is already priced — a talisman's own `Bonus90` is high precisely
  because he won the pool. → **scoring-model**
- **Do not port a correction across positions on the strength of an analogy.** Keepers do not need
  the defcon/clean-sheet coupling — the model already prices it through team xGC.
  → **scoring-model**
- **Stop sweeping the transfer gate: nothing swept in this family is recorded as having resolved.**
  That scope is narrower than "nothing in this family *can* resolve", and the ground is the bullet
  below rather than a ratio — **one invariance and two ties, not four supports.** The invariance is
  the strong row and is a fact about the code rather than an estimate: `min_gain` inert at or below
  0.4, byte-identical at 12 cells and again at 36. The two ties are the floor at horizon 8 and the
  horizon arm, which carry thresholds of their own and **fail to reject**, so under *a null is a
  tie* they support the closure only by not refuting it. And the ladder swept above 0.4 is a **gap,
  not a null**: 24 cells on four seasons, no threshold recorded and its cells never banked, on the
  grid this file calls wrong for a transfer constant. ⚠️ **So gate constants are MORE resolvable
  than the withdrawn reason claimed, not less.** That reason charged a constant the **perfect**
  arm's threshold of 94, and a perfect gate replaces the squad far harder than a `min_gain` nudge —
  which is exactly what drives a paired SE — so 94 was never a constant's to clear; the two arms
  here that carry thresholds of their own carry **34 and 21.7**. Read that as **the edge of this
  instrument, not out of its reach**. ⚠️ **Re-grounded, not reopened** — an oracle is not a
  constant. → **transfer-policy**
- **`min_gain` ships at 0.4 and is inert at or below it.** At the shipped horizon the charge clause
  already demands `charge/horizon` = 0.4, so 0.0 and 0.4 are byte-identical. Above 0.4 the floor
  binds and has been swept there: 0.4/0.7/0.95/1.30 read 0, −0.535, −0.589, −0.866 pts/gw —
  monotone harmful, at 24 cells on `POLICY`. **A floor below 0.4 is unmeasurable at the shipped
  horizon.** The horizon arm is the other half of the same threshold — at horizon 8 the floor
  binds and reads −15.8 against its own threshold of 34, with one season carrying 68% of it, and
  the horizon itself reads −8.4 against 21.7. **Do not compose the two into
  one ladder**: they are different arms. → **transfer-policy**, **constants-and-sweeps**
- **The minutes floor's "argmax protection" does not reproduce, and re-measured at −40 the
  direction reverses.** Downward it is close to inert but **not** byte-identical: lifting
  `MinExpectedMinutes` moves the fifteen in **2 of 36 cells**. On points this is **unmeasurable
  rather than unresolved** — with 2 of 6 seasons non-zero, the clustered |t| is capped at 1.58 by
  construction against a `t_crit` of 2.571, so quote no p, no confidence interval and no
  threshold. The bar is four non-zero seasons. ⚠️ **One cell carries almost all of the arm**:
  dropping it takes the mean from −4.7 to −0.9 a season, and in that cell five players change with
  **none removed by the floor** — a search artefact rather than the floor acting.
- **No projection constant re-tuned at 24 cells is "confirmed".** None resolved. The surface is
  rough and the argmax is not resolvable. `MinutesWeight` in particular is unresolved, and its
  ordering is not data-state-free. → **constants-and-sweeps**
- **Twelve cells could not resolve 37 points a season**, so re-judge anything decided at twelve
  cells or fewer. → **constants-and-sweeps**
- **Do not unify the transfer searches.** Favoured in direction, unresolved on points, and it ships
  bespoke on mechanism: a correct search exploits a mis-specified objective harder than a broken
  one can. `HOLD` is byte-identical across every arm. → **transfer-policy**
- **Do not build a state trigger for the wildcard, and do not read a wildcard replay as a
  valuation.** Every trigger loses to a fixed early wildcard: the literal reading of "cannot fix it
  with free transfers" measures transfer scarcity rather than squad quality, so it fires at GW2
  when the model has least data. The replay reads the wildcard negative because this policy has
  nothing for one to undo — unmeasurable, not refuted. → **chips**
- **Do not add a lock.** Both recorded locks became no-ops once the underlying bug was fixed.
  → **optimiser-and-squad**
- **Do not use the olbauday CSV mirror** for a live weekly signal or for priors. It publishes only
  the current season, so a multi-season blend silently degrades with no error, and stale minutes
  are *worse than none* — they report a dropped player as still starting, which is the exact
  failure the recency index exists to fix. → **recency-and-priors**
- **Do not build a squad for rotation.** Depth from GW1, bench credit in the transfer objective and
  a weekly imminent-fixture XI together produce real rotation and no points — a hedge that costs
  the ceiling. → **optimiser-and-squad**
- **Do not chase a rank objective for "maximise my percentile".** Percentile against a roughly
  normal field is an S-curve, locally *linear* at the median, so for a manager finishing near the
  middle, maximising expected points is not merely close to optimal — it is right. Above the median
  a percentile objective is variance-**averse**, so the deviation it implies runs toward *less*
  risk, not into differentials. For a top-10k target it is unmeasurable here rather than
  unresolved: a tail-probability payoff against roughly five independent season draws.
  → **harness-and-inference**
- **Tuning constants on accumulated expected points ("xPoints") is closed; the columns stay as
  instrumentation.** On `HOLD` the proxy cuts standard errors 20-25% **and attenuates the means
  with them**, so |t| goes *down* on five of six contrasts — the third instance here of *removing
  variance removes signal*. Never quote a `hold_xpoints` threshold as if power improved. On
  `POLICY` it works: 30-60% SE cuts with means preserved, so `policy_xpoints` is a second
  POLICY-side instrument — **beside points, never instead of them**. → **xppilot**
- **xPoints prices xG and xA through a per-season, per-position conversion scale**
  (`internal/analysis/xpoints.go`), and ships on mechanism. It is fitted **in sample**, so the
  position-mean attacking residual is zero by construction for DEF/MID/FWD but not GKP. The
  instrument therefore sees **within-position** deviation only, and cross-season levels are
  recentred and carry a data state. Paired differences stay one metric but are **not numerically
  unchanged**. What remains unmeasured on points is the **scale itself**.
  ⚠️ **THERE ARE TWO CONVERSION SCALES BUILT THE SAME WAY, AND THE OTHER IS LIVE ON `Score`.**
  "Instrumentation, cannot move replayed points" is true of `Player.Conversion` only — read at
  exactly one non-test site, `xPointsOf`. `Engine.calibrateExpectedStats` builds `e.xScale` through
  the **same `CalibrationRatio`**, same 20.0 sample floor and same [0.5, 3.0] clamp, and
  `e.scaleFor(pos)` is read by **`baseXP90`** and **`fixtureSensitiveAt`** — `baseXP90` *is*
  `Score`. It applies **no coverage gate and no exposure gate**, so a realised assist whose
  `expected_assists` is zero enters the numerator with nothing behind it in the denominator.
  **Its exposure is sized and bounded — no repair, no arm**, and it **could not have been a
  repair whatever the size**: the counterfactual conditions on the outcome, so it is a
  decomposition rather than a bias, and the term exists to price exactly the assists xA does not
  count. Bounded by the thin-sample floor, not by the clamp, which never binds.
  ⚠️ **`Simulate` rebuilds this engine every gameweek**, so a figure quoted over the six entry
  deadlines is the wrong denominator — they govern the opening fifteen alone.
  → **scoring-model**
- **The underlying criterion recovers 0.64 of a perfect points gate, Fieller upper limit 0.813 — an
  information statement about the criterion, and not a bar any constant must clear.** The Fieller
  interval (a confidence interval for a ratio) is **[0.325, 0.813]**, rejecting **0.89** (t −3.96)
  and **1.00** (t −5.87) and rejecting neither 0.50 (t +1.42) nor the six-season 0.414 (t +2.051).
  Both rejections stand as facts about the fraction. All three biases run the optimistic way
  — the bonus leak inflates the fraction, and the in-sample scale gives the criterion a
  season-global fit no deployable one has — so the **shortfall** has a floor even though the
  fraction has only a ceiling: the criterion sees roughly two-thirds of what the points criterion
  does, and a biased-up estimate makes that gap harder to escape, not softer. ⚠️ **A
  `sig_season/perfect` ratio — 0.89 is the four-season one, 94/106 — is a property of the comparison
  it was computed on and transports to nothing**: it is built from a threshold carrying `t_crit` of
  the season-cluster df, so it moves with the grid whatever the football does, and it moves with the
  run besides. ⚠️ **The bonus leak is SIZED, and it is not small: about a quarter of an attacker's
  stripped luck comes back.** BPS pays goals, assists and clean sheets, so the conversion residual
  the instrument strips returns through the realised bonus column — regressing bonus on the removed
  residual over three native seasons gives **corr 0.606, slope 0.252 for attackers** (n 12,104) and
  **0.479 / 0.156** for defenders, found independently by two audits using different estimators and
  reproduced in `stats/xpoints_channel_audit.py`. It biases an xPoints arm **toward** a
  realised-points arm, so part of the fraction above is leakage rather than information in the
  underlying. **Quote the leak beside the fraction.** ⚠️ **It is not fixable here and the channel
  must not be replaced**: modelling expected bonus is a closed line, because the bonus term
  double-counts goals, assists and clean sheets by construction.
  Settled 2026-08-15, arithmetic off banked cells with no sweep; the three bars and the
  derivation are in `stats/findings/2026-08-15-gatescaled.md`. Banked `POLICY` figures:
  underlying **+2.2294 pts/gw (+84.7 a season)** against that arm's own threshold of 57.7; residual
  arm **+2.2546** on points and **−0.828 (−31.5)** on the expected-points discriminator. (54.7, the
  six-season bar's numerator, is the *points* arm's threshold while 57.7 is the *underlying* arm's —
  different arms, different standard errors.) Do not re-run to tighten the interval: six season
  clusters cannot deliver the ~45% SE cut it would take. → **transfer-policy**
- **A gate on the residual alone buys realised points, but its underlying gain is consistently
  negative — `suggestive`, not established.** On `policy_xpoints` the residual arm reads **−0.828
  pts/gw (−31.5 a season), CR2 t −2.04, p 0.0971, wild bootstrap 0.0598** against its own threshold
  of 39.7, negative in 5 of 6 season means. The RESIDUAL−UNDERLYING contrast (−2.770 pts/gw,
  t −11.38) does **not** carry this: it is exactly `level_R − level_X`, about 70% of it is the
  underlying arm's positive control, and a contrast whose null the pre-registration already
  declares false cannot discriminate. What does discriminate is that the residual *level* came out
  negative where the pre-registered confound expected it positive — and even that has an unexcluded
  alternative. ✅ **The anti-residual arm that was to settle it is RUN, 2026-08-16, and could not
  discriminate — so the SIGN reading is `unresolved`, with a measured failure to corroborate rather
  than an outstanding test.** `ANTI − RES` on `policy_xpoints` is **−0.229 pts/gw (−8.7 a season),
  CR2 t −0.38, 18 of 36 positive, wild 0.7035** against its own threshold of 58.1, and
  leave-one-season-out **changes sign**. ⚠️ **Its null is not zero and is not identified**: the two
  accept sets partition the free-transfer stream, so accept-mass asymmetry enters additively as
  `T(1−2p)`, and `T` reads **−2.066 (t −2.75)** net of the hit charge and **+4.361 (t +5.91)** gross
  of it — both resolve, in opposite directions, off the same cells. Tested as one per-cell contrast
  `Z = D − (1−2p)T`, no candidate null rejects, but the closest is **t −2.51 against 2.5706**: read
  this as *the test could not discriminate*, never as *the null survived*. ⚠️ **The realised-points
  half is untouched and still resolves** (+2.255, t 4.64, wild 0.0111), and the Holm family moving
  3 → 5 is bookkeeping — this arm's raw 0.0971 and wild 0.0598 are unchanged.
  `stats/snapshots/2026-08-16-anti-residual-gate/`.
  The negative reading is **not** the hit charge: the arm makes 2.67 fewer moves and 0.39 fewer
  hits a cell than shipped, so the hit channel is +0.06 pts/gw and *helps*. Effectively all of it is
  squad composition. Leave-one-season-out spans −18.4 to −38.9 a season — but that sign stability
  is **arithmetic, not evidence**, since each subset shares five of six seasons. And the level is
  data-state-dependent: pre-scale it reads −0.583 (CR2 t −1.06, p 0.3378).
  → **transfer-policy**
- **Accepting every offered swap costs 82.4 a season on realised `POLICY`** — CR2 t −6.05 against
  its own threshold of 35.0, wild 0.0051, 6 of 6 season means negative, LOSO −77.1 to −92.9 — **and
  gains 40.9 gross of the transfer charge** (t 3.47, threshold 30.4, 6 of 6 positive, LOSO +31.9 to
  +46.1). ⚠️ **Not a lower bound on the gate family**: the no-gate arm moves three levers at once —
  no value criterion, `moveLimit` saturating at 821 hits over 918 cell-gameweeks against shipped's
  73, and `acceptTransfer` bypassing `shippedAccept` so `min_gain` and `free_transfer_value` are
  off. It bounds *no gate at the shipped move and hit budget*. ⚠️ **Do not pair it with the perfect
  gate's 106 to form a span** — that figure is four-season and this is six. ⚠️ And the +40.9 is a
  decomposition of that arm's own outcome, **not** of the gate's value, so it does not license "the
  gate is mostly hit avoidance"; separating them needs a `MaxHits: 0` arm, unrun.
  → `stats/snapshots/2026-08-16-anti-residual-gate/`
- **An antisymmetric pair of criteria cancels the common level but not the accept-mass asymmetry**,
  so such a contrast's null is `T(1−2p)` rather than zero, and identifying it needs an
  accept-everything arm whose own budget constraints do not contaminate the level it defines. Here
  they did — measured, not asserted.
- **The gate oracles are a veto on one candidate, not a selector, and they replace the value bar
  rather than adding to it.** `bestSwap` returns `swaps[0]` and `decide` **returns** on rejection
  instead of trying the next, so an oracle arm's only channels are refusing the model's single best
  pair and the path divergence that follows. `acceptTransfer` also bypasses `shippedAccept`
  entirely, so `min_gain` and `free_transfer_value` do not apply inside an oracle arm. **Any
  statistic over a candidate population describes a different operator.**

## What has been measured

The evidence behind these is not in this repository. Read them as conclusions that were paid for.
A verdict here **cannot be checked from this checkout** — so never write "I verified" when you
mean "I re-ran". Re-measuring is still how a verdict falls; when it happens, the new number sits
*beside* the recorded one, so say which you have. And **a claim that is absent here is not
thereby unmeasured** — nothing checks that this file stays complete, so absence is weak evidence.
The `→ **name**` at the end of a line is the note the evidence sits in, not a file you can open.

### What a player is worth: the scoring terms

- **P(appears) ships as one implementation, fitted against measured minutes**; the second
  estimator is retained behind `FPL_NO_UNIFIED_APPEARANCE` and does not ship. Start share adds nothing
  (r = 0.9934 with mean minutes), and refitting against `ExpectedMinutes` recovers the shipped
  `playsAtAll` constants to within 2%, which is their best support.
- **A club's expected goals sum to the club on average but not club by club, and the static anchor
  is closed on mechanism** — a club's ratio correlates with its own next season at −0.232, and a
  static offset must persist to be removable. The **short-clock version is open and unresolved**
  (blend +13.1% pooled, t = 0.82); the mechanism there is lag, not bias. `FPL_TEAM_FORM` is built
  and **ships off**, at +24 against a standard error of 34 — and that +24 is two cells of one
  season on a pre-repair bank, reversing when both are dropped. Never a magnitude.
- **Club "form" is about half the fixtures it was earned against, and that half is anti-signal** —
  ease in one window correlates with the next at −0.519, because the schedule is a fixed budget.
- **The 2026/27 bonus rules are measurable on old football**, because bonus is a ranking within a
  match rather than a rate. Keepers gain +15.4% assumption-free, and there is a 16-point spread
  *inside* the defenders. Nothing ships changed; `BonusPriorWeight` at 0.5 already does that job.
- **The fifth rule change moves the keeper result, not the midfield one** — priced on 2016-19,
  GKP −14.5% to −17.5%. Read the two arms as **unsigned, not cancelling**: different seasons,
  different schedules, and the channels do not add.
- **The pre-2024-25 bonus-points schedule is decoded, not read off an announcement.** Every
  coefficient was solved from the archive and required to reproduce recorded `bps` on **31,402 of
  31,402 rows, max deviation 0** (`bps` is the archive's bonus-points-system column). `pen_saved
  = 15`, which settles the pre-2024-25 value only: the later legs carry an unresolved
  contradiction in the record, and settling it needs FPL's published tables rather than a run.
  **The gate must be "residual identically zero", never "a good fit"** — mis-coding one sibling
  feature returns plausible coefficients at an 88%-exact fit.
- **A goalkeeper's goal paid 6 before the modern 10, decoded from the archive's only one.**
  Alisson 2020-21 GW36 reconstructs to exactly 6 with every other channel accounted for, and it is
  robust to the two divisors FPL does not publish. The 10 is published from **2024-25 GW16**, when
  FPL added `game_config` to the bootstrap — so the change falls in **2021-22..2024-25** and the
  early end is unestablishable here. ⚠️ **Do not check "the archive's only goalkeeper goal" with a
  `position == 'GK'` filter**: `merged_gw.csv` carries no `position` column before 2020-21 and
  spells 101 of 2021-22's keeper rows `GKP`, so the filter returns a clean zero on five seasons for
  reasons that are not football. Join `players_raw.csv`'s `element_type` instead.
  Both the instrument and the engine pin the rule per season through **one** table
  (`analysis.ScoringRulesFor`, the `BankLimitFor` of the scoring rules), because
  `TestScoringConstantsMatchFPL` would otherwise carry the *next* rule change backwards over the
  whole archive. ⚠️ **This bullet said "the engine's own scoring path is NOT pinned"; that exposure
  closed 2026-08-16.** The five bare `goalPoints[pos]` reads go through the season's table, which
  `NewEngineFull` derives from `fpl.Bootstrap.Season` rather than from an `Engine` field, so the two
  derived engines inherit it with no assignment to miss. ⚠️ **The type was `XPointsRules` until that
  commit** — renamed because a name saying "xPoints" would claim the instrument reaches `Score`.
  ⚠️ **The engine half is NOT confined the way the instrument's was**: the pin moves `BaseXP90` at
  **552 player-cutoffs** and `Score` at **155**, max |ΔScore| 0.0151, and **"banked figures are
  unaffected" is false** — 8 of 72 cells move, the pre-change values banked in 19 rows across 12
  cells files and 67 across 42. **Quote no t.** ⚠️ **But the pin itself moves no replayed point**:
  at the early end of `keeperGoalRuleChangeSeason`'s own unresolved range it is byte-identical in
  72 of 72, so every moved cell belongs to that boundary taking the late end in the two seasons
  inside it — the half of the constant this repository cannot establish. It ships on **mechanism**;
  no points gain is claimed. The pin's whole cost is **~6.6 xPoints across ten seasons**, and it is *not* inert: the goals
  channel is `(Goals − XG·scale)·Goal[pos]`, so a keeper with no goal and non-zero xG is re-priced
  too, at 13 such rows. On 30 shipped-config cells, `hold_points`, `policy_points` and the opening
  fifteen move in **0 of 30** while `policy_xpoints` moves in **6 of 30** and `hold_xpoints` in
  **2 of 30**, all by exactly −0.44 — one archive row, and 1% of this grid's own `HOLD` threshold.
- **The defcon term is about half redundant for defenders — quote 50%.** That is one estimate with
  no uncertainty attached, over 87 defenders at one cutoff, in the only season that has the
  category. **Never quote the midfielder figure**: its denominator collapses when the prior is
  wired. The cause — defensive actions and clean sheets are negatively correlated while the model
  adds them independently — is untouched, and `DefConCleanCoupling` stands on mechanism at 0.3.
- **No archived prior season carries defcon**, so every defcon figure here was produced under a
  rate blended toward zero. Unmeasured, not unmeasurable.
- **Four findings that change nothing shipped**: defcon has a small opponent effect, not
  implemented; saves are fixture-sensitive and the model prices only the losing half; team strength
  wants a far heavier prior than player rates (k=70/35 against 8/5); and the model is
  **over-confident mid-season**, so it is least trustworthy when most active.
- **Fixture multipliers are applied per fixture, not averaged** — the clean sheet is convex.
  Forwards are provably untouched, which is the invariance check.
- **The captain had no vice-captain and the replay forfeited the bonus.** Shipped. It cleared
  significance on both standard errors at the pre-repair four-season grid — though the clustered t
  is the retired estimator's output, so it is re-derivable rather than current — and since the xGC
  repair moves 18 of that grid's 36 cells, **the six-season value is unmeasured**. Recorded:
  **+0.4590 pts/gw held**, +0.4313 on `POLICY`. Because +0.4590 rounds to the recorded figure, a
  fresh default-grid run looks like a perfect reproduction while hiding the move on the four
  seasons. The channel is selection, not belief.
- **The minutes model under-reacts to the onset of an absence** — one unflagged blank multiplies
  the vanish risk ninefold. `blankRunFactor` ships at 0.75 for the agent-facing number; the replay
  cannot resolve it, because `MinExpectedMinutes` cliffs the affected players out of the pool.
- **The bonus term should not be a constant.** A flat weight is monotone *harmful* from GW1 and
  monotone *helpful* from GW11, so averaging reads as "no structure". The 0.5/1.5 prior/evidence
  schedule ships and beats flat at every start point.
- **Real team news is measured and too small, rather than unmeasurable** — six seasons of FPL's own
  pre-deadline payloads give **+15 a season held** against a threshold of 51. This is not hindsight:
  `availabilityFactor` already reads these flags live, so it was the *replay* that was blind. **The
  contrast is the sharp finding** — granular repricing is ±3 held and **−18 on `POLICY`**, another
  arrival of a better predictor making a worse policy. Suggestive, not established. Nothing ships
  changed.
- **Perfect minutes is the largest information bound on the *scoring* side** — the armband bounds
  below are larger, but they bound a decision over a fixed squad — **it is two capabilities, and
  none of it resolves.** `OracleLineups` grants selection and `OracleMinutes` the realised
  quantity: **≈93** and **≈47** a season held. Quote ≈93 with its data state: it moved with the
  starts harvest, because an oracle that names the wrong lineup in 2.36% of starter slots is a
  degraded oracle, and the increase is not established — one season carries most of it.
  `OracleMinutes` is untouched by the harvest and needs no caveat, because it writes `StartShare`,
  which nothing shipped reads. **A season average of future minutes is worth more than the truth, for
  a squad** — an argmax fed a five-week sample optimises the first five weeks of a thirty-week
  hold. What a judgement layer should buy is **who is picked**.

### Recency and priors: what to weight by time, and what never to

- **Minutes need recency; rates do not.** Minutes are a statement about the present and reward
  sharp recency. Rates are a statement about quality and punish it — a three-game window is 19%
  *worse* than the season average on both points and underlying stats, because it chases finishing
  variance.
- **`MinutesHalfLife` ships at 4, and the reason is asymmetry, not the points**, which are flat
  from 3 to 8. A short half-life is right about a player *losing* his place and wrong about one
  *gaining* it, so the sell-side and buy-side errors move in opposite directions. 4 is the shortest
  setting that beat flat in all three seasons while keeping the buy-side error small. The
  minutes-prediction error favours 2 because it cannot see the asymmetry.
- **Multi-season priors come from FPL's `history_past`, not the CSV mirror**, and it returns them
  **oldest first** — a naive walk weights the oldest season hardest. Priors must load regardless of
  season state: pre-season is exactly when there is no current data to fall back on.
- **A thin prior season is a fallback trigger, not a smoothing opportunity.** Blending
  unconditionally dilutes genuine improvement, which is most players most of the time; gated on
  `ThinSeason` it costs ~7 points. **Off by default** — that −7 measures the cost on replayed
  points, not the benefit, and the case the feature exists for occurs ~40 times a season, ~16 of
  them squad-relevant.
- **`prior_half_life` stays off — unresolved, not favourable.** Measured on ordering, both the
  **information** channel (rank correlation within the treated population) and the **level**
  channel (visible only whole-field, since there is no quota on thin-season players) are non-zero,
  and the **affirmative** arms do not survive Holm (best raw p 0.022 → 0.154). The stable-signed arm is the **negative** one: a minutes-only blend worsens whole-field
  ordering in six of six seasons, replicated under the unrepaired archive, so p = 0.031 on six
  independent seasons — and it clears Holm at 0.0385 on the `popEveryone` population. Read that as
  the arm least supported for shipping rather than a refutation: its shape is the fingerprint of a
  **rescaling**, and the precedent it argues against was set on replayed points, which an ordering
  statistic cannot overturn either way.
- **One family resolves the other way, and a verdict of "no affirmative case" must not omit it.**
  The signed error over the twenty highest-predicted players *inside* the treated population is
  monotone, unanimous from dose 0.25 up, and Holm-clearing. But it is a **level** statistic, so it
  is exactly what a rescaling would produce. The points question is unmeasured and expected to stay
  unresolved.

### Fixtures: measured hard, priced about right, and every attempt to lean harder loses

- **Fixtures matter hugely per match and barely per horizon.** A single fixture swings returns a
  long way and the model under-reacts everywhere — but between the 10th and 90th percentile team,
  one gameweek spans 35% on goals and five gameweeks only 13%. The model's response over a
  five-game horizon is about right. It is only wrong about a single match, which is not the unit a
  transfer buys.
- **The model is already position-dependent by construction**, which is easy to miss: the clean
  sheet moves with `defenceMultiplier` and goals with `attackMultiplier`, so a defender's fixture
  response is larger than a forward's without any per-position weight. Scaling on top of that is
  what fails.
- **Neither fixture ladder has a shape**, and zeroing the defensive response entirely **gains** 20
  points — inside noise — even though the clean sheet is 26-45% of a defender's score. The recorded
  defensive column is 2172/2158/**2152**/2178/2117 and the attacking 2127/2129/**2152**/2164/2122.
  The verdict rests on **non-monotonicity**: shipped is third of five and the column peaks at 1.5
  with both endpoints below it.
  ⚠️ **The defensive column spans 61 against this file's ±50, and that is not a contradiction:
  ±50 is a HALF-WIDTH**, derived by halving an observed 105-point gap, so the band is ~100 wide and
  61 is inside it. The expected max−min of five draws at that dispersion is ≈58, so 61 is what
  noise produces across five arms. Comparing the span against the half-width is the natural
  misreading. Add **±50** to the list of eyeballed figures in *What the harness can resolve*.
  ⚠️ **Data state and cell count.** Three seasons at one entry point — **3 cells**, absolute
  totals, no threshold ever computed, below this file's own twelve-cell bar and against its own
  rule to judge on paired differences. Run at a commit post-doubles, post-selling-price and
  post-zero-penalty but **predating the defcon-visibility change worth −95 on 2025-26, one of its
  three seasons.** Neither knob has banked cells (`stats/snapshots/*/cells/`) and sweep provenance
  stamping post-dates both runs, so these totals **cannot be re-derived — only re-measured**. So
  "no shape" is *unmeasured on the current grid* rather than a measured null: nothing on disk
  distinguishes it from too small a sweep to see one.
- **The model is not timing fixtures in practice, and there is little to time.** Across 63 replayed
  transfers the incoming player's fixtures got *harder* 63% of the time, and the correlation
  between the swing and what the move returned is −0.103.
- **`fixture_weight` is clamped to [0,1]** — setting 1.4 is silently identical to 1.0.

### The archive: what it carries, what it gets wrong

- **The replay was buying players who had already left**, and **players who had not arrived yet** —
  18-26% of the GW1 pool. Both fixed; the first stands on correctness rather than on points.
- **Extending the archive backwards buys two seasons, and a third on `HOLD` only** — not "eight
  seasons". Mixing xG providers is a real cost for xA.
- **Expected goals conceded is reconstructed for the four seasons that carry none**, and the chain
  needed no harvest and no calibration: a club's xGC in a match *is* its opponents' xG. Coverage
  goes 0% → ~100%, and `xgcScale` ships at **1 on mechanism**. The validation is FPL-fed and does
  **not** transport to the Understat-fed xG the chain actually runs on: ever-present error goes
  3.0-5.2% → 16.0-20.2%, so quote 1.0088/0.9853 as the FPL-fed control only. Its confinement is
  exact — **18 of 36 cells** move and 18 are byte-identical — and its price does not resolve on any
  grid. Never quote its −34 alone: nothing rejects on any estimator, and 45% of it is captaincy.
  **That is not a verdict on keeping the reconstruction** — switched off, `baseXP90` skips the
  clean sheet *and* the concede deduction. Nor is it evidence for "a better predictor makes a
  worse policy": that arm was pre-registered as unpredicted.
- **`FPL_NO_XG_REPAIR=1` also disables the xGC reconstruction**, and `FPL_NO_XG_AGGREGATE` governs
  the xGC aggregate, so a 2×2 over those switches has only **three** live corners and a zero
  interaction by construction.
- **FPL revises team strength mid-season, in waves, and the revisions are outcome-driven — so
  any coefficient fitted on `def` **plausibly** carries post-cutoff information. That last step is
  a mechanism argument, not a measurement.** The archived captures
  (`data/captures/<season>/GW*/bootstrap-static.json.gz`) hold point-in-time strength for all six
  seasons. The coarse 1-5 `strength` moves for **6 to 11 clubs of 20 in every season** and the fine
  `strength_*` fields for **20 of 20**. The waves carry `LIV 5→4` in a collapsed title defence,
  `ARS 4→5` inside a title race, `MCI 5→4` as they fell away — the season's own result, applied
  retroactively to every fixture that club appeared in. `fixtures.csv` records **one**
  `team_h_difficulty` per fixture, the end-of-season value, and `playedFixtures` strips the
  scoreline and the `Finished` flag but **not** the difficulty. So this is the second place a
  feature can train on the future, beside the one the fixtures bullet already names, and the bias
  runs **upward on a fixture-interaction term** specifically.
  ⚠️ **This does NOT retract `b2 = 1.5688`.** The leak's *size* is unmeasurable on that comparison
  — the hindsight gate puts the channel at **+0.846 (SE 0.396)** against a threshold of **1.702**,
  and a *full* artefact would need **1.685**, itself under the threshold — so "entirely hindsight"
  and "none" cannot be separated. Unmeasurable, neither refuted nor cleared.
  ⚠️ **A THIRD channel is suspected and is worse, because it is live on the scoring path rather
  than fit-side: the archive's `teams.csv` strength block.** `season.go` justifies treating it as
  point-in-time-safe on the ground that *"played and points are zero — so it is a pre-season
  snapshot"*. **That inference is invalid, verified 2026-08-16**: FPL's `teams` payload carries
  `played: 0, points: 0` in the **GW38** capture too, so those columns never carried the
  information the argument needed, and Arsenal's 2023-24 strength moves 4 → 5
  (`strength_overall_home` 1230 → 1350) *within* the season. The path is live —
  `PointInTimeWith` hands `cur.Teams` to the bootstrap ungated by `through`, and `priorFromStrength`
  reads the **fine** fields, the ones that move for every club in every season, into the club prior
  every replayed cell scores against, weighted heaviest where `played` is smallest.
  ⚠️ **What is NOT verified here is that `teams.csv` equals the last capture** — it is not in the
  checkout, so that half is reported rather than reproduced, and **the size of what this is worth
  is unmeasured**. Do not quote a magnitude. Three code comments still state the opposite.
  Reproducible in seconds: `python3 stats/team_strength_revisions.py`.
  ⚠️ **A reading of "0 of 380 difficulties changed" does not license ignoring this** — that window
  is pre-season and three days long, and reaches no archived season. ⚠️ **What is measured is team
  strength, not difficulty**: the captures carry no fixtures payload, so whether
  `team_h_difficulty` itself moved is unresolved from this archive, and the step from one to the
  other is mechanism rather than measurement. ⚠️ A subsample selected on the coarse field alone is
  **not** clean, because the fine fields moved for every club in every season.
- **The payloads carry a great deal the code ignores** — `can_select`, the CBI/tackle components,
  team form, ownership, and the bonus-points coefficient schedule.
- **The xG harvest left no coverage hole on goals, and left the assist channel about twice as
  exposed as a natively-fed season.** `XPointsResidual` gates the clean sheet on `XGC > 0` and
  applies no equivalent gate to goals or assists, so a realised return with zero underlying is a
  row whose xG or xA is *absent* rather than worthless. **Goals: closed** — 0 such rows in all
  seven windows post-repair, against 868/924/327 rows and 4747/5043/1763 points under
  `FPL_NO_XG_REPAIR=1`. **Assists: not closed** — conditional on an assist, 193/1981 repaired
  against 168/3306 native, Fisher OR **2.02 [1.62, 2.51]**, and on the within-2022-23 contrast (the
  only one holding the football constant) every channel sits near 2x. All 46 blank rows belong to
  elements the Understat crosswalk mapped, so *"the harvest never saw this player"* is eliminated
  rather than assumed. ⚠️ **These are counts of archive rows and carry NO points claim** — nothing
  was replayed, so no detection threshold applies and none of it sizes a gate arm. The likeliest
  cause is that `covered` changes *instrument* between arms — Understat's key-pass xA against FPL's
  `expected_assists`, which also pays for won penalties and deflections — **a hypothesis the count
  is consistent with and does not test**. ⚠️ **Do not read the ~0.3% mass share as a bound on
  decision leverage**: every exposed row carries a realised return, which is exactly what a
  positive-residual gate fires on, so mass share is a *lower* bound there and not an upper one.
  **A naive `xg+xa > 0` gate would be wrong** — most of the exposure is legitimate, and the right
  shape if one is ever wanted is a season/gameweek capability gate like `DefconScoredIn`.
  ⚠️ **"Goals: closed" rests on the ungated population and is narrower than it reads.** The zero
  count is post-repair on the rows the existing coverage gate admits; it is not a statement that the
  goals channel cannot be exposed.
- **The exposed-return leak into the conversion fit is sized, and dropping those rows is refused** —
  it breaks the in-sample identity in every fitted cell.
  ⚠️ **`XA == 0` is a two-decimal DISPLAY threshold, not a real zero**, reaching only about a sixth
  of near-zero-expectation assists in FPL-fed seasons. **Any population defined on `== 0` is a
  minority of its own phenomenon** — that is general, and it is what makes such a repair the wrong
  shape rather than merely the wrong size. → **scoring-model**
- **The weekly capture yields nothing this season**, and unblocks four questions recorded as
  unmeasurable by next spring. That is the cost of not having started earlier.

### The harness: noise, inference, and what it can resolve

- **The replay's noise is sensitivity, not randomness.** Score scoring constants on `HOLD`, and
  average over start points. **Budget jitter is not an averaging axis** — about 60% of nudges change
  the squad, but the squads are correlated draws.
- **Go for the engine, R for the inference, CSV as the contract.** Go prints no standard error, no
  t and no verdict word.
- **The prediction benchmark is a second instrument** answering a different question — the model
  orders players 28% better than a five-game average. It cannot replace the replay.
- **The noise splits differently on the two metrics.** On `POLICY`, 78% is genuine season-to-season
  heterogeneity, so adding more replay paths buys nothing there. On `HOLD` it is 100% path noise as
  a point estimate, so entry points are the one remedy that works. 48 entry points is structurally
  impossible.
- **Captaincy is 45% of `HOLD`'s residual variance, and removing it is still worse** — it removes
  47% of the signal too. The cheapest way to reduce variance is to stop measuring, so any variance
  reduction must be checked against a known effect.
- **A perfect armband is worth 210 points a season, and it is the largest resolvable thing here.**
  Its t of 20.4 is mechanical and not comparable with any other t here, because both arms replay
  the same squad through the same football. **The decomposition is more useful than the 210**:
  perfect hindsight ~465 over doubling nobody, the model's own weekly captain ~255, a captain
  pinned in week one ~228. The model already captures ~55% of the premium and captains the right
  player 22% of weeks, so **the entire observed span of captaincy *rules* is about 28 points a
  season**. That is what a real captaincy change competes for, not 210.
- **An ordering is cheaper to establish than a gap.** The predicted order must be committed to in
  advance.
- **Paired standard errors are optimistic in one direction and pessimistic in the other** —
  clustering strengthens the evidence where seasons agree and weakens it where they disagree.
- **Judge a sweep on paired differences, not on totals.** Every sweep cell runs a five-transfer bank
  regardless of season, so absolute totals from `runPolicySweep` are not comparable with a real
  replayed season.
- **The season-to-season spread lives in weekly re-picking**, not in squad selection. **Start points
  are three information regimes and the early one is the quietest**: weak evidence and a noisy
  measurement are different properties. A sweep writes its own provenance before its first cell.
- **The archive carries what a field average needs** — `selected` gives per-player ownership, and an
  average is linear, so marginals suffice. It is an entry count that **grows within and across
  seasons**, so quote it with its season and gameweek or not at all, and only on rounds where all
  twenty teams play. `selected_N` at gameweek N is honest; reading `selected_{N+1}` is a leak. What
  is absent is other managers' *squads*, so a reconstructed field carries a known bias: everyone
  owns the template, so an independent field overstates dispersion.

### The weekly transfer decision

- **The transfer charge is a volume brake, not an anti-churn device.** Raising it cuts moves and
  round-trips, but the *proportion* that are round-trips barely shifts — so a device that worked as
  intended would be destroying value, since round-trips are solidly positive. Stays at 2.0.
  **Rotating for blanks and doubles pays**: those are the best moves the policy makes.
- **Team value compounds, and the half-of-any-rise selling rule taxes 62% of it.** Affordability
  still rises, because the squad converges on the best players. **You cannot sell at the market
  price**, and modelling it properly costs 31 points a season.
- **Perfect price timing is too small for this harness to see** — **+15 a season** on the corrected
  arm, t = 0.95 against a threshold near 50. A bound, not a measurement. It caps the entire
  automation-by-speed argument, price forecasting and value-chasing at once.
- **The premium-acquisition over-valuation was never measured** — the captain-doubling *mechanism*
  survives, the size does not, and the resolvable evidence is in the under-£6.0m bucket.
- **The policy never banks a transfer, and banking is not the fix** — money binds, not transfer
  count. `BankLookahead` changes nothing and ships off. **The late-season quiet is a converged
  squad, not the price**: nothing clears the gain threshold at any price after GW28.
  ⚠️ **That null is a simple-effect null** (the standing rule, above) **and it was not taken at
  shipped config**: both the `BANK` sweep and the reach map set `WeeklyXI = true`, which
  `runPolicySweep` does not. `shouldBank` prices both arms on **today's board**, so `MinGain`,
  `FreeCost`, `BankUpTo` and the horizon are its whole bar. ✅ **The zero is now CHECKED, and it is
  not degenerate.** The one banked `BankLookahead` arm — `stats/snapshots/2026-08-13-reach/`, 4
  seasons × 2 entry points — replays under the current tree to all 8 `policy_points` **byte-exact**,
  and its mediator reads **236 consulted weeks, 169 weighed, 0 banked**. So in 72% of consulted
  weeks the rule had a real choice and preferred to act every time; without `weighed_weeks` that is
  indistinguishable from "nothing ever cleared the gain floor", which licenses the opposite
  conclusion. ⚠️ Read it as a **confinement, not a null**: `banked = 0` means the branch never ran,
  so the points columns are byte-identical **by construction** and a threshold, a p or "unresolved"
  would be a category error. ⚠️ `WeeklyXI = true` is not shipped config, `BankUpTo` is pinned at 5
  rather than `BankLimitFor(season)` — which runs *in favour* of banking, so the zero is
  conservative — and the original `BANK` sweep's cells were never banked, so that arm stays
  unverifiable. ⚠️ **"Never banks" is a claim about `Week.Free`, what survived each decision;
  `free_at_decision` is what the search ran with, and on 2025-26 from GW1 they read 0.55 and 1.46.**
  ⚠️ **The banked branch used to grant a second free transfer on top of the weekly accrual.** Fixed
  on correctness — FPL grants one — and **inert on every banked cell**, since the arm banks in 0 of
  8 so the branch never executed. Reachable from user config as `bank_transfers_lookahead`, shipped
  off. **Hypothesis, unmeasured:** the rule may be near-unreachable *by construction* at shipped
  config, because `MoveLimit` is `free + hits` so the hit allowance already grants the extra move
  while waiting costs a flat 1/horizon — setting `MaxHits: 0` takes banked weeks from 0 to 5 on one
  season. No cross against team news is banked here.
- **Reach is not the problem: 97.6% of worth-taking two-move packages are already reachable**, which
  closes the unified-search line on mechanism. The lever is the **valuation**, not the gate.
- **The sell side is calibrated; its error is entirely availability** — −0.100 per gameweek for a
  sold player who keeps playing, against −2.223 for the 13% who stop.
- **The transfer path's noise, measured cleanly, is 303 points of spread** with `HOLD` provably
  byte-identical. That is the floor for any transfer-policy experiment.

### Constants: what survived re-tuning

- **The flat prior ships at k=8.** Two attempts to fix the mid-season over-confidence were both
  refuted, and what they leave standing is k=8. The calibration stands; the unresolved direction is
  *upward*.
- **`LeagueShrinkK` is split out from `BlendRateK` and ships at 8.** Out of sample the league anchor
  wants K=2-4 and beats the shared 8 in three of four seasons. Wired into the replay, `HOLD` is a
  flat null (+0.0095 pts/gw, t 0.03) and `POLICY` reads −0.843 pts/gw, t −1.94 — **unresolved, not
  a measured loss**, since |t| 1.94 clears no two-sided 0.05 bar this arm could carry. It ships at 8
  because a predictive win does not discharge the burden to move a constant, not because a loss was
  measured. The two anchors are kept separate because they answer different questions:
  `shrinkToLeague` governs players with **no prior**, `BlendRateK` governs everyone else.
- **`BandStrength` now has banked cells and still does not resolve — and the arm that DECIDED it is
  still unrun.** `stats/snapshots/2026-08-16-band-strength/`, 36 cells, one process, consumer
  `SimConfig.Weights.BandStrength` set by `policyVariant.apply`. `HOLD`, s=1 against 0:
  **+0.357 pts/gw (+13.6 a season)**, CR2 SE 0.184, df 5, t 1.94, against a threshold of **18** and
  an MDE of **24**. **Unresolved, below its own MDE** — and *not* unmeasurable: the s=2 canary reads
  −10.6 against 27, came back **smaller and opposite**, so it sizes nothing. Unresolved twice.
  ⚠️ **This did not re-test the recorded verdict.** The deciding arm was **s = 0.25**, which is
  **unrun**; the original was *"the best of six swept values against a ±20 noise floor"* that also
  *"loses 12 out of sample on 2022-23"* — an argmax whose winner sits inside its own stated noise.
  ⚠️ **The original refutation was `POLICY`, not `HOLD`**, so a `HOLD` figure does not speak to it.
  That is now **established rather than inferred**: at the originating commit `FPL_BAND_STRENGTH`
  reached only the `base` SimConfig handed to the transfer policies, while the hold row was built
  from a fresh `SimConfig` that never saw it — **so the hold baseline was byte-identical across that
  whole sweep by construction**, and only `POLICY` rows could move. A textbook instance of the trap:
  a byte-identical result that was a comparison which never ran.
  ⚠️ `FPL_NO_XGC_REPAIR` bears on **18 of 36 cells**, not the 6 an earlier note claimed.
- **`teamBands` was not run-to-run deterministic, and is now pinned.** `bands.go` ranged a map
  into a slice and sorted it with the non-stable `sort.Slice`, so a tie at a band boundary
  resolved by map order. Fixed by building the slice in club-id order and breaking both sort
  ties on club id — `sort.SliceStable` alone would **not** have worked, because stability
  preserves an input order that was already the random one. `TestBandAssignmentIsDeterministic`
  pins it against a constructed boundary tie, so the test carries its own positive control
  rather than depending on whichever ties a season happens to hold;
  `TestBandTiesBreakTowardTheLowerClubID` pins which total order was chosen.
  ⚠️ **Every `BandStrength` figure recorded before the fix carries that jitter and cannot be
  re-derived from its own cells. Its size is known and small: it moved the s=1 arm's mean from
  +0.339 to +0.357 pts/gw — 0.7 points a season, about a tenth of that contrast's own CR2
  standard error — and concentrated in the GW1 and GW6 entries rather than the GW26 column that
  carries most of the estimate. So it widens an interval and cannot overturn a null**; had the arm
  resolved, the defect would have been disqualifying. One repeat is a single draw, not a variance
  estimate. Two banked runs at one commit
  and one constants digest differed, at `band_strength 1`, in **3 of 36 `hold_points` cells, 12 of
  36 `policy_points`, 13 `policy_xpoints`, 6 `hold_xpoints`, and 7 each on `moves` and `hits`** —
  decisions, not only scores. **`squad_hash` moved in 1 cell against `hold_points`'s 3**, so two
  cells re-scored an unchanged fifteen: squad-hash identity is weaker evidence than points
  identity, which matters wherever this file leans on it. **No per-season magnitude is
  available** — one repeat gives an occurrence count, not an effect size. It was **inert at the
  shipped `band_strength` of 0 as a code fact** and reachable by a user through
  `FPL_WEIGHT=band=1`, which is why it was fixed rather than left latent. `Optimize` had the
  identical defect and is pinned by `TestSeedOrderIsDeterministic`.
- **`BlendRateK` is banked and nothing resolves.** The ladder is **non-monotone** over 3/5/12/16/24
  — −4.1, +1.8, −11.6, +11.6, +12.6 a season, Holm 1.000 — and **8 ships unchanged**. The low side
  is flat: k=3/5 are the smallest effects. Two seasons carry the swing, and dropping both reverses
  its sign.
- **Calibrate against data, not intuition — and check what a multiplier multiplies before
  calibrating it.** Two terms were guessed at the same size, one right on minutes and one wrong on
  `Score`.
- **No scoring constant with banked `HOLD` cells is measurably a schedule** (that is, varying with
  the point in the season) — `stats/schedule_screen.R`, 7 ladders and 31 arm contrasts, Holm 1.000
  on both. **This is a tie the design guaranteed**: the interaction is formed *between* cells, so
  its p = 0.05 threshold is 152-349 points a season per ladder, against this grid's own median of
  39 (roughly 26 on six seasons). Transfer constants are not screened at all, because they are
  byte-identical on `HOLD`. And the screen **cannot test its own motivating example**: it refuses
  `BONUS` on a coincidence of label text, since `0.5 / 1.5` and `0.5 / 2.0` both parse as a
  ladder. So the one constant this record does hold to be a schedule — the prior/evidence bonus
  weight, which beats flat at every start point — sits outside the 7 ladders. Read this bullet as
  "of the constants the screen accepted", not "of all of them".

### Chips: what the replay can and cannot say about them

- **All four chips are modelled, and the replay cannot value a wildcard** — it replaces all fifteen,
  so the within-season spread swamps it. Bench boost is the one chip this harness can value. Swept
  over its own week, the wildcard has exactly one reliable week: **GW4**, positive in all four
  seasons, because the opening fifteen is built on the season's weakest information. Below the
  detection threshold even so.
- **Anchoring the chips on the calendar is a clean null as a measurement, and "anchoring is worth
  nothing" is not established** — MDE 34-37 per season-path, with the sign resting entirely on the
  GW1 column. `fullSight` is the realistic arm, because the biggest double is known ahead of time.
- **The scoring-chip timing `+0.000` is a declared invariance, not a result.**
  `mustNotMoveForAxis(AxisChipWeek)` returns all eight of `cellMetricColumns` — the eight columns
  every sweep collects a comparable series for, which is **not** every column in the cells file,
  since the ten chip-reading columns, the five banking-mediator columns, the five fixture-run
  mediator columns and `oracle_kind` are
  required to differ by arm — and the harness checks
  them cell by cell on every run. The axis reads a finished season's per-week gains and plays no
  chip, so a byte-identical `POLICY` is what it is *required* to produce. It says the argmax never
  reached the simulation, and nothing whatever about timing.
  The **levels** are a different quantity and are unbanked. `reportChipCells` prints
  `oracle − threshold` and `oracle − median` **per chip** with no combined figure, so each level is
  the sum of the two scoring chips: timing **+8.3** a season, and the **threshold rule**'s own row
  **+21.9**. Never halve either. "Pooled" means pooled over entry points, not over chips, which is
  why the timing arm's **13.3 at GW1** is quoted apart — the argmax ranges over 38 weeks there and 13 at GW26. Both levels are
  functions of `chipBarBenchBoost` 16 and `chipBarTripleCaptain` 12, which are **asserted, not
  measured**, and the output is oracle minus the first week clearing the bar — so a bar set too low
  flatters timing and one set too high flatters the threshold rule. **There is no threshold of its
  own**, and both differences are ≥ 0 in every cell by construction, so a t against zero is
  mechanical. An interval on a bound is the only legitimate reading. Nothing has been re-measured
  under the banked schema; a re-sweep is owed.
- **Only two of the four chips are *preparation* problems.** A free hit fields a temporary fifteen
  and a wildcard *is* the rebuild, so what those want was already wired. `ChipCredit` adds the other
  two, off by default. **The bench channel is mechanism-real and points-unresolved**: the chip's own
  week is a paired **+7.28 points** (CR2 t +2.91, p 0.033, 27 of 36 cells positive) — **suggestive,
  not established**, since Holm over the two channels reads ≈0.066 and no leave-one-season-out
  subset stays under 0.05. The bench channel's season figure is **+13.3** — a different quantity
  from the timing arm's GW1 13.3 above — on the `per_gw × 38` estimand, where `per_path` reads
  +9.0, a 32% disagreement. Read it against **this comparison's own threshold of 17.7-24.5**,
  never the global `POLICY` median of 70. 73% of it is 2025-26 alone, which is also the season the
  defcon-visibility change moved by −95, so name the era before reading the shape.
- **The two preparation channels overlap by about a third on the chip week and fight over a
  season.** A 2×2 with the wildcard held common: interaction **−4.2** on the boost week (t −2.38)
  and **−18.3** a season (t −2.23), neither clearing 0.05. **Do not restrict to the non-zero cells**
  — 13 of 36 are exactly zero and only one is a cell where the intervention could not run; the rest
  are measured additivity, and dropping them drops 2023-24 entirely.
- **Truncating the transfer horizon at a planned wildcard is bounded, and closed on mechanism.**
  185 moves in the wildcard's shadow return **+15.8** a season-path with the losers at **−14.0**,
  against the transfer path's own **303-point** noise floor — a ceiling of order 14 cannot be
  measured against it. Note that truncation does not only refuse moves: a shorter horizon lowers the
  funded pair's bar and substitutes packages rather than declining them.
- **Triple-captain preparation changed no decision, which is not a measured zero.** Only **23 of 36
  cells place the chip**, and the estimator is degenerate: it bounds the flip *rate* near 8% and
  says nothing about a flip's value. Quote no p.
- **Do not project the two-set chip rule backwards to buy chip observations.** The first half of the
  season holds **15 of 189** doubling club-gameweeks — and **11 of the 15 are one COVID-rescheduled
  2020-21 round** — while 2025-26, the only two-set season, holds none. So a first-half arm is
  collinear with "a chip on a plain week". There are no extra degrees
  of freedom either: two halves share one squad. Two sets *are* expressible for 2025-26 onward, off
  by default, and unmeasurable at 6 cells.

### The optimiser, the squad, and the money

- **The optimiser was garbage-collection-bound, not compute-bound** — 6.2× faster and bit-exact
  against the old path. That changes what is affordable to run, which is the binding constraint on
  this enterprise.
- **The optimiser's move set** needs N downgrades funding one upgrade, and the ranking proxy may
  filter but must never decide. Fixing the search made realised points *worse*, because a correct
  search exploits a mis-specified objective harder. **An injured premium needs no special case** —
  correct the minutes and let everything downstream recompute.
- **Money is worth what it can still be turned into** — a £100m squad holds only about £36m of
  discretionary spending. Valuing freed money inside the transfer gate does not work.
- **The bench is a hedge and its slots are not interchangeable.** A flat weight models the wrong
  thing, because the multiplier is P(this slot is needed), not P(he plays). The tie between shapes
  survives re-measurement: four arms × 36 cells on `HOLD`, spanning ~12 points a season against
  per-arm thresholds of **17 to 40**, so the derived weights keep their `ViceCaptainWeight`
  justification. The derived arm is not shape-only — the fixed tuples renormalise to sum 4 and it
  does not, so it varies effective `BenchWeight` too. → **optimiser-and-squad**
- **Two elevens exist on purpose** — `Plan.XI` is picked on the imminent fixture, while
  `Plan.GainPerGW` is measured on the horizon eleven. Presenting a recommendation beside an eleven
  nobody would field is wrong; changing what the *decision* optimises is a different act, and is
  worth nothing.

## Season maintenance

Four things are not in the FPL API and go stale the moment the season turns over. They ship as
dated 2026/27 defaults (`DefaultEuropeanCampaigns`, `DefaultDomesticCups`, `DefaultNewCoachClubs`,
`DefaultRestPlayers`) and must be re-derived every summer:

- **Competition windows** per club, with start *and* end dates. `armband congestion` reports what
  is set and how stale it is.
- **Managerial changes.** The test is not "is the manager new" but "was last season's data produced
  under him" — which is why Tottenham is on the list and Manchester United is not.
- **Post-tournament rest.** Names must match the FPL spelling exactly, accents included.
- **Nationality code lists** for travel load — `armband nations` maps the opaque codes.

Two regression tests fail loudly if a hand-maintained name stops resolving, because that failure is
otherwise silent.

**Three of the four lists are display-only, and one is live on the scoring path.**

- All eight congestion penalties ship at 1.00, so the competition windows and the nationality lists
  can only mis-inform a human, not mis-score a player. `TestTheShippedCongestionBlockIsInert` makes
  re-enabling any of them deliberate; if one is revisited, the channel is minutes, not `Score`.
  `DefaultNewCoachClubs` is display-only too, but through `NewCoachPenalty` in `rolerisk.go` — a
  ninth penalty, outside the congestion block.
- **`DefaultRestPlayers` is live.** `blendFor` applies `restFactor`, multiplying `MinutesPerMatch`
  and `StartShare` by the post-tournament factor. That factor is a `Weights` field, **not** one of
  the eight `Congestion` penalties, so a wrong name there mis-scores a player today. It is live at
  **GW1 and GW2 only** (`restFactor` returns 1 once the next gameweek is past `rest_gameweeks`,
  which is 2) — so it bites in exactly the two gameweeks after the summer maintenance that was
  supposed to have checked it.
  ⚠️ **The applied multiplier is `rest_minutes_factor` (0.83) prorated across the horizon, not
  0.83 itself.** What it feeds is averaged over the next `Horizon` gameweeks, so at a 5-gameweek
  horizon and a 2-gameweek window it is (2×0.83 + 3×1.00)/5 = **0.93 at GW1**, 0.97 at GW2 and
  1.00 from GW3. Do not quote 0.83 as the applied figure: the unprorated version is a bug this
  code has already paid for.

Two unrelated mechanisms answer to the word "rest": `ShortRestPenalty` and `VeryShortRest` are
congestion and are inert, while `rest_players` is the post-tournament teamsheet and is not.
`restFactor` has two further call sites in `metrics.go` — one un-applies it to build
`SettledMinutes` for the pool filters, and one is reporting-only and labelled as such, which reads
misleadingly like confirmation that nothing is scored.

**The rest list is a teamsheet, not a squad list, and it looks incomplete because it is supposed
to.** Read the Go comment on `DefaultRestPlayers` before editing `config.json`'s copy, and see
[configuration.md](docs/configuration.md) for the rule and the worked list.
