# Project context for Claude Code

A Go CLI that scores Fantasy Premier League players with a quantitative model, with a Claude
agent on top to reason over the output. Read [docs/architecture.md](docs/architecture.md) before
changing code and [docs/model.md](docs/model.md) before changing the scoring.
[docs/README.md](docs/README.md) maps the rest.

Everything in `docs/` is **reference** — what the system *is*, written for a reader of the
repository. Design proposals and research notes do not belong there.

This file holds the **verdicts**: one line per conclusion that has been paid for, plus the rules
that keep the next measurement honest. The evidence behind each verdict lives in the **research
vault** — the private Obsidian store reached through `~/.claude/bin/research-worktree` — and the
`→ **name**` at the end of an entry names the vault note that carries it (`notes/<name>.md` unless
it says otherwise). The user-facing docs never reference the vault; this file and the other
agent-facing surfaces may. Three things in the tree are also evidence: `stats/findings/` holds a
narrative and a pre-registration per run, `stats/cells/` holds the banked cells two R screens
read as input, and `stats/snapshots/` holds the accuracy series.

⚠️ **The 2026-08-17 compaction moved derivation narratives out of this file, not verdicts.** If
an entry seems too short to act on, read the note it points at before rebuilding anything.

## Build and test

```bash
go build ./... && go vet ./... && go test ./...
```

Tests hit the live FPL API and skip when it is unreachable. They assert invariants, not exact
values — the underlying data changes weekly, so a test pinned to a specific player or score rots
within days.

**Before merging anything to `main`, invoke the `merge-gate` skill and satisfy every line of it.**
Twelve conditions, all mechanical, each of which has caught something real. `review-gate`
establishes that a review happened; `merge-gate` establishes that everything else did. **This
binds on every agent that can merge**, the default one included. The three most often skipped:
**`0 behind` `origin/main` re-checked immediately before merging** (many sessions run
concurrently and `main` has moved five times in an afternoon); the leak scan across **three**
channels, the branch name included; and **merging the paired research-store branch in the same
sitting**, because they are one unit of work. ⚠️ A green gate is not a correct change — every
condition in it is a process fact. **Current practice: measurement work lands on `development`,
not `main`**; merge-gate binds whenever it is `main`.

### Replay sweeps run through `scripts/replay`, and they run in parallel

```bash
EXP=G FPL_CELLS=/tmp/g.csv scripts/replay -run TestDiagTransferPolicy -v -timeout 2h
```

`go test` flag spellings are accepted and translated for the compiled binary — they do not pass
through to `go test`, which is not run, so a flag the binary does not know is rejected rather than
honoured. `DIAG=1` is set for you.

The wrapper compiles once under a build lock and runs the binary in a child process it waits for —
deliberately not an `exec`, because it has to outlive the run to report the exit status and the
peak RSS. One block measured both ways on 2026-08-11 cost 97 MB resident instead of the 1031 MB
the `go test` driver holds, at the same speed, which is what makes parallel sweeps affordable.
Banked sweep runs since span 89-142 MB; budget from [docs/replay.md](docs/replay.md), not from
the 97. It adds three guard rails:

- `FPL_REPLAY_SLOTS` (default 3) — extra runs *queue* rather than race.
- A per-run memory cap set above the measured peak, so it binds only on a run that has gone wrong.
  It exists only where a user systemd manager does; elsewhere the wrapper says so and runs uncapped.
- An exit status you can trust. A killed sweep leaves a partial cells file that reads downstream
  like a complete sweep with fewer arms.

It prints each run's peak RSS. That is not decoration — the memory figures above hold only until
some arm makes them false. Details in [docs/replay.md](docs/replay.md).

## Conventions

- **A correction here REPLACES the claim; it does not narrate the replacement.** This file carries
  what is true now. The history of a retraction, and the run that caused it, belongs with the
  evidence in the vault. ⚠️ Marking a withdrawal in place is correct where the evidence lives and
  wrong here, and the two being opposites is deliberate — do not "fix" it. ⚠️ **A deleted figure
  still owes its referent**: if a number survives the cut, whatever made it meaningful must
  survive with it.
- **Deterministic analysis never calls the LLM.** `internal/analysis` is pure computation.
- **Tool output is replayed on every later API call.** Keep tool JSON compact; full detail behind
  single-player lookups.
- **Every scoring term is a separate, reported multiplier.** Expose it on `PlayerMetrics` and
  `playerRow` so the agent can explain a number rather than assert it.
- **New config fields need a backfill in `config.Load`**, so existing config files stay valid.
  One narrow exception: a hand-maintained list whose membership can legitimately be empty — the
  campaign maps and `tournament_absences`. There an empty list is a *statement* and only a
  missing key is an omission. Fixed-arity lists such as `Review.Rules` and
  `MinutesWeightByPosition` still get backfills, because empty is meaningless for them.
- **Comments explain *why*.** Several carry the data that justified a constant — don't strip them.
- **Gloss a Go identifier the first time a section uses it**, in four to eight words. Not every
  occurrence, just the first per section.
- **The resident size budget** is enforced by `TestTheResidentIndexStaysSmall`. This file is
  loaded into every session, so growth is paid for by every task. Since the 2026-08-17
  compaction the remedy for a genuinely needed entry is still RAISE THE BUDGET and name the claim
  — never drop a qualifier to fit, which is the failure the constant exists to prevent — but the
  *first* question is now whether the derivation belongs in the vault and only the verdict here.

## How to read the measurements

Six terms account for most of this file.

| term | what it means here |
|---|---|
| **`HOLD`** | buy the opening fifteen and never transfer, but re-pick the eleven and the captain every week, with substitutions applied. **Use this for anything about scoring**, because it excludes the noisy transfer decisions |
| **`POLICY`** | the same, plus the weekly transfer decision. **Only for settings that are themselves about transfers** |
| **cell** | one replayed season entered at one deadline. Six seasons × six entry gameweeks = **36 cells** per setting. Older figures may say **24 cells** (four seasons) or **12 cells** (three by four). Take the cell count from the figure, never from this row |
| **paired difference** | one setting minus the shipped one *within the same cell* — same football, same opening conditions, one thing changed. Always **per gameweek played** |
| **pts/gw** | points per gameweek. **Multiply by 38** for the season-scale figures quoted here |
| **detectable** | a detection threshold belongs to a *comparison*, not to the harness. Median **39 points a season on the four-season grid, season-clustered estimator** (start-fixed 32; six-season arithmetic roughly 26); the pooled span is 3.9 to 232 |

Recurring statistical terms: **CR2** (a cluster-robust standard error treating each season as one
group, so disagreeing seasons widen the bar rather than being averaged away); **Holm** (correction
for testing several arms at once); **MDE** (minimum detectable effect at 80% power); **argmax**
(see the box at the end of the standing rules — the most load-bearing idea here).

Where a constant cannot be resolved on points, this record decides on **mechanism** (does the
objective say what the game actually pays?) or on **shape** (a plateau with a cliff, or monotonic
movement across several settings) rather than on which single value scored highest. Picking the
highest of several noisy estimates manufactures effects.

## What the harness can resolve

**A detection threshold belongs to a comparison, not to the harness.** Compute it as
`t_crit(df) × SE × 38`, not `2 × SE × 38` — 2.571 at a CR2 df of 5 (six seasons), but **take the
df from the comparison**, since it is resolved per contrast and is often lower.
`stats/variance_components.R` prints the p = 0.05 effect and the 80%-power MDE per arm. Quote
the range between the clustered and start-fixed estimators rather than picking an end; clustering
is not uniformly conservative. A change whose mechanism is certain — one that lands almost
identically in every cell, like the vice-captain fallback — resolves at a threshold of **12.7**
on a metric this record calls noisy. → **harness-and-inference**

**The six-season grid is worth roughly 20-26 points a season on `HOLD`** (df 3 → 5) — read as an
ordering, not a point estimate. Widening helps on 10 of 11 transfer arms, median SE ratio 0.62.

**Three switches disable the archive backfills**, for reproducing an older figure:
`FPL_NO_XG_REPAIR=1` (xG backfill *and* xGC reconstruction), `FPL_NO_XGC_REPAIR=1` (xGC alone),
`FPL_NO_XG_AGGREGATE` (the xGC aggregate). `FPL_SWEEP_SEASONS=default` narrows the grid to four
seasons. **Name the data state, or do not quote a level.** The switches do *not* govern the
archive defect repairs (phantom matches, duplicate rows) — those are ungated. The four shipped
seasons are byte-identical inside the six, so grid width alone changes no cell. On the
four-season grid the switches reach almost nothing: `FPL_NO_XG_REPAIR` moved 24 of 96 cells, all
2022-23. → **archive-and-data**

**Several numbers for "the smallest effect this harness can see" exist**, each true of the thing
it measured; prefer the canonical 39. Traps: **42/147** is pooled over one sweep's arms, **~150**
belongs to the totals era, **~10-12** (`TestDiagNoiseFloor`) is a *floor* not a threshold. →
**harness-and-inference**

Two things easy to get backwards: **a threshold is not a verdict** — nearly every constant here
is worth 11 to 34 points a season, so "unresolved" is the *expected* reading for a real effect
of that size; and **the way to resolve something is to make the comparison sharper, not to run
more cells**.

**Absolute point totals are not comparable across eras of this codebase.** Fixed bugs moved them
by up to 115 points a season, and a bug that costs points unevenly across seasons does not merely
add noise — it **invents shapes**. Paired differences within a cell usually survive such a fix;
absolute totals across tables do not. Load-bearing instances: the doubles fix **+115 `POLICY` /
+106 `HOLD`** (every earlier total understates); the substitution and selling-price fixes the
other way (7-14 and ~31, over-statement); the unset congestion penalty that multiplied as 0
(28 on 2023-24, **113 on 2024-25**) which manufactured a monotone attacking-fixture ladder and
reversed the flat-bench-weight verdict; and defcon visibility (−95 on 2025-26, one of four
changes in that block — do not attribute alone). → **harness-and-inference**

## What each season can and cannot run

**A byte-identical season under an intervention is not a tie — it is a season where the
intervention could not run.**

| season | cannot run | note |
|---|---|---|
| 2018-19 and earlier | xG, xA, xGC natively | `tackled` and the full bonus-points component set are present 2016-19, which is what let the pre-2024-25 bonus schedule be decoded |
| 2019-20 | **`POLICY`** — FPL granted unlimited free transfers before the GW30+ deadline and froze prices for three months. Also xG and `starts`, both backfilled | scoring is fine, so it stays valid for `HOLD`. Rounds are numbered 1-29 then 39-47 |
| 2020-21, 2021-22 | xG natively; `starts` is recorded, from the Understat harvest | nothing on the **replay's** scoring path reads `Starts` (byte-identical at shipped config — a **simple-effect null**, untested under `FPL_NO_UNIFIED_APPEARANCE` or `FPL_RELIABILITY_SPLIT`). It *is* live on the agent path (`tournamentAbsence`) and in `OracleLineups` |
| 2022-23 | xG and `starts` for GW1-15 — FPL added fields mid-season | GW7 has no rows and GW8 is partial. Real football, not a hole |
| every season but 2025-26 | defensive contribution ("defcon") | 6 live cells in 36, so widening the grid makes defcon *harder* to measure |
| all archived seasons | the full five-change 2026/27 bonus figure — no season carries both the modern saves baseline and a `tackled` column | the individual channels **are** measurable; the joint CBI-plus-tackled arm is measurable on 2016-19 and unrun. The four shipped seasons span three bonus regimes, so `Bonus90`'s (the blended per-90 bonus rate a player is scored on) *level* is not comparable across them, though paired comparisons are |

The earliest season that is recognisably the same game is **2013-14** — the introduction of the
Bonus Points System. → **archive-and-data**

## Standing rules

- **Four classes take priority over anything else in the queue: security, performance, velocity,
  and a model or scoring fix** — in that order when they collide. This is **precedence, not
  worth**. Security is currently empty by construction (no authenticated write path;
  `TestTheClientHasNoAuthenticatedSurface` guards its absence). Performance changes what is
  affordable to run, the binding constraint on this enterprise; velocity is the same argument one
  layer up; a scoring fix changes `Score`, therefore the ordering, therefore which footballers
  get bought.
- **Convert per-gameweek figures by multiplying by 38. Never divide a pooled total by the cell
  count.** The six entry points give cells of 38/33/28/23/18/13 gameweeks (mean 25.5), so
  dividing by 36 understates by about a third.
- **A constant fitted against a proxy for its input is fitted to the proxy's noise too.**
- **Pre-register against a quantity that can actually move in the direction you would act on.**
- **Review the plan, not just the output, for anything that will produce a number.** A brief can
  ask an arm to test a hypothesis it cannot discriminate; that defect is invisible in a diff and
  far cheaper to fix before the run.
- **An estimator swap reads as a data change.** Name the estimator beside any figure.
- **A gap between two point estimates is not a result until it is divided by something.** A
  recorded estimate's SE is usually recoverable: the old harness printed `mean` and `t`, so
  `SE = |mean/t|`.
- **An exactly identified moment fit cannot test the mechanism it is named after.** One free
  parameter matched to one moment reproduces that moment *by construction*. Quote the fit's own
  SE, and name the season and feed of whatever it is compared against. → **scoring-model**
- **Do not quote the parts of a decomposition that telescopes.** Quote the product; never the
  wedges. → **scoring-model**
- **A recorded regime triple belongs to a ROW, and equal phases identify which.** ⚠️ A
  compression pass may not delete a paid-for rule to fit — raise the budget instead.
- **Monotonicity that the construction forces is not evidence.** Read the *size*, not the
  direction.
- **Check which *file* a number came from**, not merely whether the archive has the field.
  `players_raw.csv` is the end-of-season snapshot behind `statusAt`; `data/captures/<season>/GW*/`
  is what the point-in-time oracles read. **"The archive does not have X" is unverified until
  someone greps for X**, per season, across every source the pipeline touches — **a field the
  code fetches and throws away is not a field the project lacks**. The mirror: **a constant
  having been swept does not mean its cells were banked** — grep `stats/snapshots/*/cells/`.
- **Check that a setting is read on the path you are about to score, before you score it.** A
  setting that never arrives returns a **byte-identical null**, which looks exactly like a null
  meaning the knob does nothing. Naming the consumer is the check; naming a package is not.
- **A byte-identical result is not a tie.** It may be a comparison that never ran. Check the
  mediator — the thing the setting is supposed to act through — before reading anything into it.
- **A null is a tie, not the refutation of one.** A non-resolving comparison shows two settings
  **cannot be separated by this instrument**. What it *can* refute is a recorded **magnitude**.
- **A one-at-a-time null is a *simple-effect* null** — true of the shipped configuration and
  silent about any other. Report the factorial main effect beside the simple one, and label
  which you are quoting. Invariance results are unaffected: byte-identical stays byte-identical
  at the configuration tested, and becomes *untested* elsewhere rather than false.
- **Two levers that a mechanism says touch the same decisions get a 2×2, not two sweeps.** The
  interaction is not the expensive contrast — a difference of differences *within one cell*
  cancels the path divergence a single difference carries (season-clustered SE 0.216 against
  0.599 for the noisier main effect, no df penalty). The real cost is multiplicity: 2^k−1
  contrasts against k. Cross **pairs a mechanism names**, never a lattice — an unpredicted
  interaction found by search is an argmax over 2^k−1 contrasts.
- **Holding a confounder constant is only safe when it is constant *with respect to* the thing
  being varied.** Pinning the wildcard to a common week put the bench boost immediately after the
  rebuild in 30 of 30 cells for one arm and 3-5 of 30 for the others.
- **A diagnostic must never carry its own copy of the thing it is checking.** Where a frozen
  baseline genuinely is wanted, pin it to the package with a test.
- **Size a canary with the omitted-variable coefficient, not with a raw variance share — and on
  the link the model actually fits.** Getting it wrong is how an arm passes a check it should
  have failed; the convenient formula is the one reached for, and the error runs in the direction
  that **flatters the instrument**. Check the degrees of freedom the same way, and note that **a
  rank-deficient cluster-robust matrix can still be computable**. The worked miss is in the
  vault. → **harness-and-inference**
- **A canary is judged against the standard error of the ARM IT GATES**, which is unknown until
  the arm runs — so gating is a **necessary** condition, not a sufficient one. Where the gated
  arm does run, re-state the ceiling against the arm's own threshold. Measured: the bench-boost
  canary's 2.06 (SE 0.803) was a 22% loosening of the arm's own 2.65 (SE 1.032), in the
  flattering direction; it did not bite at a sixfold margin but decides a marginal one. →
  **chips**
- **One quantity, two implementations — this project's signature failure.**
  `TestTheSharedCellQuantitiesHaveOneImplementation` and
  `TestTheCopiedExpressionsHaveOneImplementation` scan for it. Extend an existing scan rather
  than adding a runtime equivalence test per copy: the second stops one divergence, the first
  stops the next copy. **A scan passing is not "there are no copies"** — the scans match an
  *idiom*, so they are tripwires rather than proofs.
- **A nonlinear transform of an archive-row field is not a statement about the model.** Take the
  regressor off `Metrics`, as `TestDiagCleanSheetRegressor` does;
  `TestNonlinearTransformsScoreTheModelsOwnRegressor` scans for it. ⚠️ It catches a mismatched
  **regressor** only, never a mismatched **population**, and is blind wherever the archive and
  `PlayerMetrics` (the struct the scoring path scores a footballer from) spell a field the same
  way. → **scoring-model**
- **"No truth value" and "a truth value we cannot resolve" are different, and only the second
  describes any constant here.** An unresolvable question is still owed an answer if the
  instrument improves; a meaningless one is not.
- **A better predictor can make a worse policy.** The transfer search is an **argmax**, so it
  lives in the tail of the estimate distribution. Removing a **bias** is safe; trading bias for
  **variance** is not.
- **A bias shared by every player in a position is not an ordering error; a within-position bias
  is.** "Shared" must mean shared on `Score`, not on one component: a position-wide multiplier
  on one additive term reorders within the position whenever players differ in that term's
  share. The rule also needs a **quota** to bite, covers **ordering** only, and `Optimize` is a
  knapsack against one budget, so a position-wide level can still change who is bought. Prefer
  the sibling rule, which is unqualified: **a measured bias does not imply a correction exists.**
- **Correcting a measured bias has lost points five times.**
- **An oracle that accepts on the sign of the quantity it is scored on raises that quantity by
  construction.** Check the **contrasts** as well as the levels.
- **A confinement check on a path that cannot carry the effect confirms nothing. Pair it with a
  liveness check that must move.** Confinement is usually a *code* fact, so re-running it can
  only fail; the check with power is the mirror that moves where it must.
- **The wild cluster bootstrap may withdraw support from a CR2 rejection; it may never grant
  one.** Webb 6-point weights on the season, enumerated exactly (6^6 = 46,656 draws, no seed, no
  Monte-Carlo error); it swaps a normality assumption for a **symmetry** one, and on this
  balanced intercept-only design CR2 *is* the equal-weighted t-test on season means, verified
  bit-for-bit. It is a function of season totals alone, so it cannot see cell-level
  concentration — that is `concentration_screen.R`'s job. **Quote `S_eff` and the floor
  `6/6^S_eff` beside every p**: 0.1667 at 2, 0.0278 at 3, 0.00463 at 4, 0.000129 at 6. An arm
  whose floor exceeds 0.05 is *unmeasurable*, not null. → **harness-and-inference**
- **A snapshot's figures are not guaranteed to have come from its own commit.** `FPL_MODEL_CSV`
  appends outside the repository and the renderer keeps the newest row per figure by wall-clock.
  `snapshot.ModelRunIDs` warns when the CSV holds more than one run.
- **A lock that looks necessary is evidence of a bug elsewhere.** Correct the numbers and let
  everything downstream recompute. And **check the claim before you exclude on it.**

> **argmax** — "the option with the highest estimated value", and the single most load-bearing
> idea in this record. Take six noisy estimates and keep the biggest: the winner is usually not
> the best option, it is the option whose estimate got the most flattering noise. The winner's
> value is **systematically over-stated**, and the more options you compare the worse it gets.
> That is why this record wants a *shape* rather than a winner, and why the transfer search — an
> argmax over players — reaches for whichever player the model most over-rates. Also called the
> winner's curse, or the optimiser's curse when a search does the choosing.

## Things that have already bitten

Shipped bugs, each now covered by a regression test. Re-introducing one is easy. Full narratives
in the vault; the lesson and the pinning test here. → **harness-and-inference**,
**optimiser-and-squad**, **archive-and-data**

- **The hit ceiling is a knob, not a clamp, and both expressions of it must move together.**
  `analysis.MoveLimit` clamped `maxHits` to 1 unconditionally and the funded-pair branch carried
  the same clamp as `hitsNeeded <= 1`, so `MaxHits: 2` was byte-identical to shipped and read as
  a null. Both now take `HitCeiling` (zero = `DefaultHitCeiling` = 1). Lifting one expression
  without the other is the trap — the limit widens while the pair refuses anything spending the
  extra move, which no points column can see. `TestTheHitCeilingIsReadByTheFundedPairBranch`
  (source scan) and `TestTheHitCeilingIsReachableAndDefaultsToOne`. ⚠️ The shipped 1 is measured
  at one setting on absolute totals — the reason, not a resolved constant. → **transfer-policy**
- **`runPolicySweep` builds cells at `WeeklyXI: false`, and several diagnostics run at `true`.**
  Fixture load reaches `Score` **only** through the horizon-1 engine built for the fielded eleven
  when `WeeklyXI` is set (or for a free-hit squad), so at sweep default a double is a 1/5-diluted
  bump in a five-week average, and **an arm testing doubles or blanks that leaves `WeeklyXI`
  false has switched off the fielding half of its own mechanism**. Pin the setting in `apply`
  and stamp it.
- **Never compare a replayed float for exact equality: a banked total is reproducible from a
  commit AND a machine, and only the commit is recorded.** Go's `math` is not bit-identical
  across machines (per-architecture assembly; amd64 branches at runtime on AVX/FMA), and the
  transcendental-dependent terms are live on `Score`. Two exact comparisons kept **CI red on
  eight consecutive commits** while green on every arm64 machine the work was done on. Fixed
  with a tolerance (`sameMinutes`); `hold_xpoints`/`policy_xpoints` are banked at full float64
  and will not reproduce across machines; points columns are integers and `squad_hash` a digest,
  so both reproduce unless a decision flips. A byte-identity that is a *confinement* is
  architecture-invariant; an empirical zero is not. → **harness-and-inference**
- **The doubles guard must key on `(element, fixture)`, never `(element, gameweek)`.** A real
  double gameweek has the identical shape to the archive's duplicate rows, so a gameweek-keyed
  guard would re-introduce the +115/season doubles bug while fixing the duplicates. `season.go`
  accumulates, never assigns. The archive holds 59 phantom matches in 2019-20 and 10 duplicate
  rows in 2025-26; dropped at load, counted, pinned to those exact numbers.
- **Anything reading fixture results must be gated by gameweek.** `playedFixtures` strips the
  scoreline and the `Finished` flag after the cutoff; `TestPointInTimeHidesFutureResults` pins
  it. ⚠️ Not the only place a feature can train on the future: the archive's `team_h_difficulty`
  is end-stamped and the `teams.csv` strength block is under the same suspicion — FPL revises
  team strength mid-season, in waves, outcome-driven. ⚠️ **The PRE-SEASON path is unguarded**:
  `PreSeasonWith` returns fixtures unfiltered and `buildTeamRates` gates on a scoreline being
  non-nil rather than on `Finished`; currently behind `FPL_MAGNITUDE`, latent not live.
  **Unfixed.** → **archive-and-data**
- **FPL's aggregates reset at GW1, so the denominator must follow.** Use `Engine.DataWindow()`,
  never the constant 38; the same applies to `MinMinutes`, or the pool is empty and the
  optimiser errors outright. `TestDataWindowTracksTheSeason` fails if either regresses.
- **Every per-90 rate must go through `blendFor`, counting stats included.** Dividing a whole
  number by a fraction of a match made a 22-minute cameo with two bonus points read as 8.18
  bonus per gameweek. `TestCountingStatsGoThroughTheBlend`.
- **A player with no prior is not a player with no uncertainty.** `shrinkToLeague` pulls their
  rates toward the position's league-wide rates. Minutes are deliberately left alone.
- **`starts_per_90` is not a rotation signal** — it sits at ≈1.0 for nearly everyone. Use
  minutes and starts against the full 38-game season.
- **Single-swap local search stalls, and paired swaps are not enough either.** The optimiser
  needs a *paired* downgrade-and-upgrade move to fund a premium from cheap bench fodder, and no
  sequence of swaps reaches a formation change that takes the money out of the goalkeeper because
  every step is downhill. `dpseed.go` solves each formation exactly by dynamic programming and
  seeds the local search — **do not "simplify" that away.**
  `TestOptimizerIsNeverWorseThanAnExactSeed` and `TestNoPremiumSquadBeatsTheOptimum`.
- **The seed's bench reservation must take the *cheapest* players who could fill those slots.**
  Indexing from the far end reserved the most expensive bench and silently starved every seed.
  `TestSeedBudgetLeavesRoomForThePremiums`. Locked players are pre-placed in the seeds rather
  than disabling seeding.
- **Never let the pair search choose greedily, and charge per move rather than per week.** The
  proxy that ranks candidates is exactly what misleads a greedy search, so it may filter but
  must never decide.
- **A free transfer is not a costless transfer, and four points is the intuitive price and it is
  wrong.** `free_transfer_value` ships at 2.0 as a confidence threshold, not an opportunity
  cost. The flat level is **swept and nothing resolves** (six-season, 36 cells, +8.8/+6.5/−23.0/
  −10.5 against thresholds 15-34; no shape), and **the ladder crosses a kink at its own
  baseline**: the effective singles bar is `max(min_gain, free_transfer_value/H)`, and the
  shipped charge at horizon 5 **is** `min_gain` 0.4 exactly — below 2.0 the singles bar cannot
  move, above it both channels act, so an interior optimum at 2.0 is confounded with
  `min_gain × DecisionHorizon`. The end-of-season exception starts at **GW35**. Pinned by
  `TestTheFreeTransferChargeIsInertOnSinglesBelowTheKink` and
  `TestTheSinglesProposalCarriesNoAlternativeOrStrictFlag`. **0.0 is not a rung**, so the
  recorded four-against-nothing comparison is still owed. ⚠️ "Must not taper" is a consequence
  of the *classification* (confidence threshold vs opportunity cost), not a measurement — the
  classification is disputed. → **transfer-policy**
- **FPL banks 5 free transfers, not 2** — the rule changed for 2024-25. `backtest.BankLimitFor`
  keeps replays on the rule actually in force.
- **Every engine that scores players needs the recency index.** `Simulate` builds three; a patch
  once wired two and silently missed the transfer decision, and the whole gain looked like
  better captaincy. `TestEveryScoringEngineGetsRecency` counts them.
- **Overrides are keyed by permanent player code** — element ids are reassigned every summer.
  Both solvers read `config.Roster`; per-call `lock_players`/`exclude_players` **add to** the
  standing set. An indefinite override never lapses but is reported for review every run.
  `TestExcludedPlayersAreNeverOffered`, `TestLockedPlayersAreNeverSold`.
- **2019-20's rounds are labelled 1-29 then 39-47.** `loadGameweeks` drops anything outside
  1..38, so before the fix the season scored as though it stopped in March — no error, a
  plausible-looking total. `renumberGW` maps them; `hasRestartGameweeks` refuses a stale cache.
- **A fixture window must be anchored on the calendar's next GAMEWEEK, never on the club's next
  FIXTURE.** Anchored on the fixture, a blank slides out of the window: `fixtureLoadFor` read
  ≥ 1 at horizon 1 always, missing 170 blanks. Three things shipped on top (the free-hit pool
  guard, `WeekViews` pricing each week's own fixtures, and `> 0` no longer meaning "computed"
  in `xiValueForTransfer`), and ⚠️ **the sharp trap**: a rebuilt **wildcard** must NOT be built
  on the horizon-1 week engine, because every blanking club is zeroed there and a wildcard
  planned for a heavy blank returns a free-hit squad that is then *kept*. Five pinning tests:
  `TestFreeHitNeverFieldsABlankingClub`, `TestWeekViewsPriceEachWeeksOwnFixtures`,
  `TestAWildcardIsNotBuiltOnOneWeeksBlanks`,
  `TestATotalBlankIsWorthNothingToTheTransferSearch`, and
  `TestFixtureLoadMatchesTheArchiveOnOneSeason` (the last chooses its gameweeks off the archive
  so it cannot stop exercising blanks when the grid moves). The fix ships on correctness; on
  points it does not resolve. → **fixtures-and-difficulty**
- **An anchored-chip arm silently lost every 2025-26 cell**: a chip planner's output went into
  the FIRST set wholesale, so in a two-set season a chip at or after `ChipResetGW` (the gameweek
  where chip sets reset for the second half) was refused,
  and `runPolicySweep` records a refusal as an **infeasible cell rather than fatalling**.
  Repaired by `backtest.SplitChipSets`; the census reads 0 of 24 refused. ⚠️ No anchored-chip
  cells are banked anywhere, so nothing that used one can be re-derived — only re-measured. →
  **chips**
- **`Optimize` is not run-to-run deterministic unless it is made so.** It ranged over a map to
  order each DP seed's bench. `TestSeedOrderIsDeterministic` pins it. `teamBands` had the same
  defect (map range plus non-stable sort; fixed by club-id order with club-id tie-breaks —
  `sort.SliceStable` alone would **not** have worked) and is pinned by
  `TestBandAssignmentIsDeterministic`; `TestBandTiesBreakTowardTheLowerClubID` pins which total
  order was chosen. ⚠️ Every `BandStrength` figure recorded pre-fix carries
  that jitter (~0.7 points a season spread across two draws — widens an interval, cannot
  overturn a null; had the arm resolved, the defect would have been disqualifying): two banked
  runs at one commit differed in 3 of 36 `hold_points` cells, 12 of 36 `policy_points`, and
  **`squad_hash` moved in 1 cell against `hold_points`'s 3** — two cells re-scored an unchanged
  fifteen, so squad-hash identity is weaker evidence than points identity, which matters
  wherever this record leans on it. The post-fix value is UNMEASURED. → **constants-and-sweeps**

## Closed lines — do not rebuild these

Each was measured and lost, closed on mechanism, or withdrawn after re-measurement. A title alone
does not stop an idea being rebuilt, so each carries its **verdict**; the bold name at the end is
the vault note the evidence sits in.

- **Do not build a custom fixture-difficulty rating, do not target the worst defences, do not
  band attack and defence separately, and do not move the fixture window.** Every form of acting
  on the per-match effect (real, 21-41%) has lost points, because you never buy a fixture, you
  buy a run of them. The convergence argument as stated is capacity-conditional and wrong — what
  binds is **throughput**: ~51 targeted matches against 418 fielded player-matches, at most ~12%
  re-pointable, and chips do not create transfers. The perfect-hindsight ceiling (~30-82 a
  season) straddles a `POLICY` threshold of 50-70 and falls below it once attenuated. The
  wildcard/free-hit/banked-transfer role is the **exit** — deleting the unwind cost — which is
  UNMEASURABLE on this policy (a code fact: `decide` never prices a future unwind in either
  arm), not refuted. The season's end is a free exit, and the late-season quiet collides with
  that — unresolved, do not assume the record's reading. → **fixtures-and-difficulty**
- **Do not extend recency to rates.** Predicts better out of sample, loses in the replay at
  every setting. → **recency-and-priors**
- **The clean-sheet over-prediction ships uncorrected.** Predicted over actual is 1.052 on
  native-xGC rows (interval [0.90, 1.20]) and 1.004 pooled; the free fit separates neither
  b = 1 nor b = 1.1731 (MDE 0.424 against a candidate of 0.173). Two same-way biases compose to
  ~3.7% (a composition, not a joint measurement). **The points question is closed as
  unmeasurable, by a canary** — the 2×2 arms (+1.9/+6.2/+7.0 against thresholds 23/16/20, Holm
  1.000) and then halving *every* clean sheet cost −21.6 against its own threshold of 28. **Size
  a candidate against a canary before spending 180 cells.** The mechanism is two things —
  cross-match convexity (predicts the regressor gap to 0.3%; quote the exact
  `E[exp(−x)]/exp(−x̄)`, never `exp(σ²/2)`) and a shot-level wedge whose size is not
  established (the moment fit is exactly identified and tests nothing). The near-calibration is
  a cancellation, not a structure; the fragility is in the MEAN, not the dispersion. →
  **scoring-model**
- **The clean-sheet factor `f` does not separate from 1; the defensive fixture ladder is what
  runs hot.** Joint fit: `f` = 1.0476 (SE 0.1612), t 0.30 — a failure to separate (MDE 0.424),
  while the defensive fixture channel reads 1.5688 (SE 0.2253), t 2.53 native / 3.30 pooled,
  above 1 in 6 of 6 seasons. The excess sits on `FPL_DEF_FIXTURE_SCALE`'s defensive half, which
  is points-null across a fourfold width change — a calibration fact with no reachable points
  consequence. ⚠️ `def` is end-stamped (Spearman 0.872 with end-of-season strength against 0.421
  at the cutoff), so the channel is flattered, and the leak's SIZE is unmeasurable (+0.846, SE
  0.396, against a threshold of 1.702 where a full artefact needs 1.685) — neither refuted nor
  cleared. **Do not open a points arm on this, and do not re-run at the refitted constants.** →
  **scoring-model**
- **Do not remove the bonus term for being circular.** Removing it costs 67 points a season
  (absolute total from a contaminated era; 66% of it the zero-penalty season — evidence the
  ordering signal exists, not a magnitude). `BonusWeight` ships at 1.5 against
  `BonusPriorWeight` 0.5, approaching ~1.33 at 38 full matches; the older flat regime is
  reachable as `bonus_prior_weight: -1`. Leaning further in is worse too, but weakly (an argmax
  over five values on three cells). → **scoring-model**
- **Do not penalise a squad for holding two players from the same club, and do not build a
  "talisman" rule.** Refuted on **arithmetic**: dependence of any sign is a property of the
  pair's *variance*, never its mean — `E[B_i+B_j] = E[B_i]+E[B_j]` exactly. `Bonus90` is a
  realised marginal; `xiValueShrunk` (the eleven's shrunk value on the transfer and replay path)
  sums deterministic point predictions; ownership is not
  causal on the pitch. The variance reading is separately closed (a risk-adjusted objective is
  closed for the expected-points manager, unmeasurable for a top-10k one). Measurability is not
  a reason to measure what the objective cannot consume. A talisman's level channel is already
  priced through his own `Bonus90`. → **scoring-model**
- **Do not port a correction across positions on the strength of an analogy.** Keepers do not
  need the defcon/clean-sheet coupling — the model already prices it through team xGC. →
  **scoring-model**
- **Stop sweeping the transfer gate: nothing swept in this family is recorded as having
  resolved.** The ground is the bullet below — **one invariance and six ties.** The four
  `free_transfer_value` ties of 2026-08-17 are the best-provenanced in the family (banked,
  six-season, per-arm thresholds). Gate constants are MORE resolvable than the withdrawn reason
  claimed — 94 was the **perfect** arm's threshold and never a constant's to clear; the two arms
  with their own thresholds carry 34 and 21.7. Re-grounded, not reopened — an oracle is not a
  constant. → **transfer-policy**
- **`min_gain` ships at 0.4 and is inert at or below it** — at the shipped horizon the charge
  clause already demands `charge/horizon` = 0.4, so 0.0 and 0.4 are byte-identical. Above 0.4
  the floor binds: 0.4/0.7/0.95/1.30 read 0, −0.535, −0.589, −0.866 pts/gw — monotone harmful,
  24 cells, `POLICY`. The horizon arm (−8.4 against 21.7) and the horizon-8 floor (−15.8
  against 34, one season carrying 68%) are different arms — **do not compose them into one
  ladder**. The same identity binds `free_transfer_value` from the other side — see its bullet
  under *Things that have already bitten*. ⚠️ **The floor 2×2 of 2026-08-18 measured the pair**:
  `free_transfer_value` 1.0 × `min_gain` 0.2 under the override-mode levers, 36 cells/arm —
  the floor-drop simple reads −2.5 a season against 32.3 (consistent with zero, bounded within
  ±32), the {2.0, 0.2} corner is byte-identical on the full grid, and the entry-point columns
  point the user's predicted way at point-estimate size (+22.7/+26.5 early, −8.4/−41.4 late,
  six cells each). The **scheduled floor** — {1.0, 0.2} through GW8, shipped after — ships ON
  by the user's ruling (configurable off via `review_policy.early_floor`), on mechanism:
  measured +1.8 pooled against ≈5.8, live early-entry columns +6.7/+4.0 (2 of 6 cells positive
  each), unresolved — the user's info-density reading, accepted rather than resolved. The
  replay's sweep baseline stays flat unless an arm sets the schedule.
  → **transfer-policy**, **constants-and-sweeps**
- **The minutes floor's "argmax protection" does not reproduce, and re-measured at −40 the
  direction reverses.** Unmeasurable rather than unresolved: 2 of 6 seasons non-zero caps the
  clustered |t| at 1.58 against `t_crit` 2.571 — quote no p, no interval, no threshold. One cell
  carries almost all of the arm, with none removed by the floor — a search artefact.
- **No projection constant re-tuned at 24 cells is "confirmed".** None resolved; the surface is
  rough and the argmax is not resolvable. `MinutesWeight` in particular is unresolved and its
  ordering is not data-state-free. → **constants-and-sweeps**
- **Twelve cells could not resolve 37 points a season**, so re-judge anything decided at twelve
  cells or fewer. → **constants-and-sweeps**
- **Do not unify the transfer searches.** Favoured in direction, unresolved on points, and it
  ships bespoke on mechanism: a correct search exploits a mis-specified objective harder than a
  broken one can. `HOLD` byte-identical across every arm. → **transfer-policy**
- **Do not build a state trigger for the wildcard, and do not read a wildcard replay as a
  valuation.** Every trigger loses to a fixed early wildcard; the replay cannot value a wildcard
  (it replaces all fifteen, so within-season spread swamps it) — unmeasurable, not refuted.
  ⚠️ **A repair cost priced in POINTS was built to escape that reason and does not.**
  `TestDiagWildcardTrigger`, four seasons at entry GW1 and GW16, decision counts only: at the
  shipped reservation the rule fires in the cell's **second week in 8 of 8 cells**, costs
  20/36/24/32 points at GW1 and 12/12/12/4 at GW16 (7/11/8/10 and 5/5/5/3 players at `free` =
  2), and only the reservation of 20 delays anything (GW16 cells only, one to three weeks). The
  entry-point contrast excludes information poverty for half the grid; the one twice-observed
  arm reads 12 → 16 and 12 → 24 — the level signature. ⚠️ **The series observer settled the
  mechanism question 2026-08-17**: `TestDiagRepairCostSeries` (8 cells, four seasons × entry
  GW1/GW16, observation-only, counts not points) — churn is **refused** (the frozen series rises
  in 8 of 8 cells) and the **standing gap dominates** (the evolving squad sits a flat 5.8-9.3
  players from a fresh optimum; the market-value arm exonerates the selling rule). The cost does
  not fall as data accumulates, so raising the reservation cannot fix the trigger — the
  *quantity* must change. ⚠️ A rising frozen series still does not separate a standing gap from
  injury accumulation. `stats/findings/2026-08-17-repair-cost-series.md`. The lever ships
  **off**. → **chips**
- **Do not add a lock.** Both recorded locks became no-ops once the underlying bug was fixed. →
  **optimiser-and-squad**
- **Do not use the olbauday CSV mirror** for a live weekly signal or for priors. It publishes
  only the current season, so a multi-season blend silently degrades; stale minutes are *worse
  than none*. → **recency-and-priors**
- **Do not build a squad for rotation.** Depth from GW1, bench credit in the transfer objective
  and a weekly imminent-fixture XI together produce real rotation and no points — a hedge that
  costs the ceiling. → **optimiser-and-squad**
- **Do not chase a rank objective for "maximise my percentile".** Percentile against a roughly
  normal field is an S-curve, locally *linear* at the median — for a manager near the middle,
  maximising expected points is right. Above the median a percentile objective is
  variance-**averse**. For a top-10k target it is unmeasurable here rather than unresolved. →
  **harness-and-inference**
- **Tuning constants on accumulated expected points ("xPoints") is closed; the columns stay as
  instrumentation.** On `HOLD` the proxy cuts SEs 20-25% **and attenuates the means with them**;
  on `POLICY` it works (30-60% SE cuts, means preserved) — `policy_xpoints` is a second
  POLICY-side instrument, **beside points, never instead of them**. → **xppilot**
- **xPoints prices xG and xA through a per-season, per-position conversion scale**
  (`internal/analysis/xpoints.go`), fitted **in sample** (position-mean attacking residual zero
  by construction for DEF/MID/FWD, not GKP), and ships on mechanism. ⚠️ **There are two
  conversion scales built the same way, and the other is live on `Score`**:
  `Engine.calibrateExpectedStats` builds `e.xScale` through the same `CalibrationRatio` (same
  20.0 floor, same [0.5, 3.0] clamp), read by `baseXP90` (*is* `Score`) and `fixtureSensitiveAt`,
  with **no coverage gate and no exposure gate**. Its exposure is sized and bounded — no repair,
  no arm — and **could not have been a repair whatever the size** (the counterfactual conditions
  on the outcome; the term exists to price exactly the assists xA does not count). Bounded by
  the thin-sample floor, not the clamp. ⚠️ `Simulate` rebuilds this engine every gameweek, so a
  figure quoted over the six entry deadlines is the wrong denominator. → **xppilot**
- **The underlying criterion recovers 0.64 of a perfect points gate, Fieller upper limit 0.813**
  — an information statement about the criterion, not a bar any constant must clear. Interval
  [0.325, 0.813]; rejects 0.89 (t −3.96) and 1.00 (t −5.87); rejects neither 0.50 nor the
  six-season 0.414. All three biases run the optimistic way, so the **shortfall** has a floor.
  ⚠️ A `sig_season/perfect` ratio (0.89 = four-season) is a property of its comparison and
  transports to nothing. ⚠️ **The bonus leak is sized and not small**: BPS pays goals, assists
  and clean sheets, so the stripped residual returns through realised bonus — corr 0.606, slope
  0.252 for attackers (n 12,104); 0.479/0.156 for defenders. Quote the leak beside the
  fraction. Not fixable here — modelling expected bonus is a closed line (double-counts by
  construction). Arithmetic off banked cells, no sweep. → **xppilot**
- **A gate on the residual alone buys realised points, but its underlying gain is consistently
  negative — `suggestive`, not established.** Residual arm −0.828 pts/gw (−31.5 a season), CR2
  t −2.04, p 0.0971, wild 0.0598, against its own threshold of 39.7; negative in 5 of 6 season
  means. The RESIDUAL−UNDERLYING contrast does **not** carry this (it is exactly the level
  difference, ~70% positive control). The anti-residual arm is RUN and could not discriminate
  (−0.229, t −0.38, wild 0.7035, LOSO changes sign; its null is `T(1−2p)`, not zero, and not
  identified) — read as *the test could not discriminate*, never *the null survived*. The
  realised-points half still resolves (+2.255, t 4.64, wild 0.0111). The negative reading is
  **not** the hit charge (the arm makes fewer hits, and the hit channel is +0.06 and helps);
  effectively all of it is squad composition. Data-state-dependent: pre-scale it reads −0.583. →
  **transfer-policy**
- **Accepting every offered swap costs 82.4 a season on realised `POLICY`** (t −6.05 against
  its own threshold of 35.0, wild 0.0051, 6 of 6 season means negative) **and gains 40.9 gross
  of the transfer charge** (t 3.47, threshold 30.4). ⚠️ Not a lower bound on the gate family —
  the no-gate arm moves three levers at once. Do not pair it with the perfect gate's 106
  (four-season vs six). The +40.9 is a decomposition of that arm's outcome, **not** of the
  gate's value; separating them needs a `MaxHits: 0` arm, unrun. → **transfer-policy**
- **An antisymmetric pair of criteria cancels the common level but not the accept-mass
  asymmetry**, so such a contrast's null is `T(1−2p)` rather than zero, and identifying it needs
  an accept-everything arm whose own budget constraints do not contaminate the level it defines.
  Here they did — measured, not asserted.
- **The gate oracles are a veto on one candidate, not a selector, and they replace the value bar
  rather than adding to it.** `bestSwap` returns `swaps[0]` and `decide` returns on rejection;
  `acceptTransfer` bypasses `shippedAccept`, so `min_gain` and `free_transfer_value` do not
  apply inside an oracle arm. **Any statistic over a candidate population describes a different
  operator.**

## What has been measured

Read these as conclusions that were paid for; the vault note carries the evidence. A verdict here
**cannot be checked from this checkout** — never write "I verified" when you mean "I re-ran".
Re-measuring is still how a verdict falls; when it happens, the new number sits *beside* the
recorded one, so say which you have. Absence from this file is weak evidence of absence —
nothing checks it stays complete.

### What a player is worth: the scoring terms

- **P(appears) ships as one implementation**, fitted against measured minutes; the second
  estimator is retained behind `FPL_NO_UNIFIED_APPEARANCE`. Start share adds nothing (r 0.9934
  with mean minutes); refitting against `ExpectedMinutes` recovers the shipped constants to
  within 2%. → **scoring-model**
- **A club's expected goals sum to the club on average but not club by club.** The static anchor
  is closed on mechanism (own-next-season correlation −0.232). The short-clock version
  (`FPL_TEAM_FORM`) is open and unresolved (+13.1% pooled, t 0.82; the +24 is two cells of one
  season on a pre-repair bank — never a magnitude), built and **ships off**. Club "form" is
  about half the fixtures it was earned against, and that half is anti-signal (−0.519). →
  **scoring-model**
- **The 2026/27 bonus rules are measurable on old football** (bonus is a ranking within a
  match, not a rate): keepers +15.4% assumption-free, a 16-point spread inside the defenders;
  the fifth rule change moves the keeper result, not the midfield one (GKP −14.5% to −17.5% on
  2016-19 — read the arms as **unsigned**, different seasons, channels do not add). Nothing
  ships changed; `BonusPriorWeight` 0.5 already does that job. → **scoring-model**
- **The pre-2024-25 bonus-points schedule is decoded, not read off an announcement** — every
  coefficient reproduces recorded `bps` on 31,402 of 31,402 rows, max deviation 0. `pen_saved
  = 15` settles the pre-2024-25 value only; the later legs carry an unresolved contradiction.
  **The gate must be "residual identically zero", never "a good fit".** A goalkeeper's goal paid
  6 before the modern 10 (Alisson 2020-21 GW36); the 10 is published from 2024-25 GW16, so the
  change falls in 2021-22..2024-25 and the early end is unestablishable here. ⚠️ Do not check
  "the archive's only goalkeeper goal" with a `position == 'GK'` filter — join
  `players_raw.csv`'s `element_type` instead. Both instrument and engine pin the rule per season
  through one table (`analysis.ScoringRulesFor`); the engine half moves `BaseXP90` at 552
  player-cutoffs and `Score` at 155 but **moves no replayed point** (byte-identical 72 of 72 at
  the early end; the moved cells belong to the boundary the repo cannot establish). Ships on
  mechanism; no points gain claimed. → **scoring-model**
- **The defcon term is about half redundant for defenders — quote 50%.** One estimate, no
  uncertainty attached, 87 defenders, one cutoff, the only season with the category. **Never
  quote the midfielder figure** (its denominator collapses when the prior is wired).
  `DefConCleanCoupling` (the defcon/clean-sheet coupling factor) stands on mechanism at 0.3.
  **No archived prior season carries defcon**,
  so every defcon figure was produced under a rate blended toward zero — unmeasured, not
  unmeasurable. → **scoring-model**
- **Four findings that change nothing shipped**: defcon has a small opponent effect
  (unimplemented); saves are fixture-sensitive and the model prices only the losing half; team
  strength wants a far heavier prior than player rates (k=70/35 against 8/5); the model is
  **over-confident mid-season**, least trustworthy when most active. → **scoring-model**
- **Fixture multipliers are applied per fixture, not averaged** — the clean sheet is convex.
  Forwards are provably untouched (the invariance check). → **scoring-model**
- **The captain had no vice-captain and the replay forfeited the bonus.** Shipped: **+0.4590
  pts/gw held**, +0.4313 on `POLICY`; the six-season value is unmeasured (the xGC repair moves
  18 of that grid's 36 cells). The channel is selection, not belief. → **harness-and-inference**
- **The minutes model under-reacts to the onset of an absence** — one unflagged blank multiplies
  the vanish risk ninefold. `blankRunFactor` ships at 0.75 for the agent-facing number; the
  replay cannot resolve it (`MinExpectedMinutes` cliffs the affected players out of the pool). →
  **scoring-model**
- **The bonus term should not be a constant.** Flat is monotone *harmful* from GW1 and monotone
  *helpful* from GW11; the 0.5/1.5 prior/evidence schedule ships and beats flat at every start
  point. → **scoring-model**
- **Real team news is measured and too small, rather than unmeasurable** — +15 a season held
  against a threshold of 51; the contrast is the sharp finding (granular repricing ±3 held,
  **−18 on `POLICY`** — another better-predictor-worse-policy). Suggestive, not established. →
  **harness-and-inference**
- **Perfect minutes is the largest information bound on the *scoring* side — two capabilities,
  none of it resolves.** `OracleLineups` ≈93 and `OracleMinutes` ≈47 a season held. Quote ≈93
  with its data state (it moved with the starts harvest; the increase is one season's).
  `OracleMinutes` writes `StartShare`, which nothing shipped reads. **A season average of future
  minutes is worth more than the truth, for a squad** — what a judgement layer should buy is
  *who is picked*. → **harness-and-inference**

### Recency and priors

- **Minutes need recency; rates do not.** A three-game window is 19% *worse* than the season
  average on both points and underlying stats. → **recency-and-priors**
- **`MinutesHalfLife` ships at 4, and the reason is asymmetry, not the points** (flat from 3 to
  8): short is right about *losing* a place and wrong about *gaining* one. → **recency-and-priors**
- **Multi-season priors come from FPL's `history_past`, which returns them oldest first** — a
  naive walk weights the oldest season hardest. Priors must load regardless of season state. →
  **recency-and-priors**
- **A thin prior season is a fallback trigger, not a smoothing opportunity.** Unconditional
  blending costs ~7 points; gated on `ThinSeason`. **Off by default.** → **recency-and-priors**
- **`prior_half_life` stays off — unresolved, not favourable.** Both information and level
  channels are non-zero; the affirmative arms do not survive Holm; the stable-signed arm is the
  **negative** one (minutes-only blend worsens ordering in 6 of 6 seasons, clears Holm at
  0.0385) — the arm least supported for shipping, its shape the fingerprint of a **rescaling**.
  One family resolves the other way: the signed error over the twenty highest-predicted players
  in the treated population is monotone and Holm-clearing — but it is a **level** statistic.
  The points question is unmeasured and expected to stay unresolved. → **recency-and-priors**

### Fixtures

- **Fixture *count* is a different term from fixture *difficulty*, and it is the one thing in
  the fixture family that pays** — see the fixture-load anchor fix under *Things that have
  already bitten*. Its doubles half was measured at `+33`; its blank half is unmeasured. →
  **fixtures-and-difficulty**
- **Fixtures matter hugely per match and barely per horizon** — one gameweek spans 35% on goals,
  five gameweeks only 13%. The model's five-game response is about right; only the single match
  is wrong, which is not the unit a transfer buys. → **fixtures-and-difficulty**
- **The model is already position-dependent by construction** (`defenceMultiplier` /
  `attackMultiplier`) — a defender's fixture response is larger than a forward's without any
  per-position weight. Scaling on top is what fails. → **fixtures-and-difficulty**
- **Neither fixture ladder has a shape** — the verdict rests on **non-monotonicity** (defensive
  column 2172/2158/**2152**/2178/2117, attacking 2127/2129/**2152**/2164/2122). ⚠️ ±50 is a
  HALF-WIDTH (the band is ~100 wide; 61 is inside it, and ≈58 is what noise produces across five
  arms). ⚠️ 3 cells, absolute totals, no threshold, pre-defcon-visibility — *unmeasured on the
  current grid* rather than a measured null. → **fixtures-and-difficulty**
- **The model is not timing fixtures in practice, and there is little to time** — across 63
  replayed transfers the incoming player's fixtures got *harder* 63% of the time; correlation
  with what the move returned −0.103. → **fixtures-and-difficulty**
- **`fixture_weight` is clamped to [0,1]** — setting 1.4 is silently identical to 1.0.

### The archive

- **The replay was buying players who had already left, and players who had not arrived yet** —
  18-26% of the GW1 pool. Both fixed; the first stands on correctness rather than points. →
  **archive-and-data**
- **Extending the archive backwards buys two seasons, and a third on `HOLD` only** — not "eight
  seasons". Mixing xG providers is a real cost for xA. → **archive-and-data**
- **Expected goals conceded is reconstructed for the four seasons that carry none** — a club's
  xGC in a match *is* its opponents' xG; coverage 0% → ~100%; `xgcScale` ships at **1 on
  mechanism**. The validation (1.0088/0.9853) is FPL-fed and does **not** transport to the
  Understat-fed chain (error 3.0-5.2% → 16.0-20.2%). Confinement exact (18 of 36 cells move);
  the price does not resolve on any grid (never quote −34 alone — nothing rejects, 45% is
  captaincy). Switched off, `baseXP90` skips the clean sheet *and* the concede deduction. →
  **archive-and-data**
- **`FPL_NO_XG_REPAIR=1` also disables the xGC reconstruction**, and `FPL_NO_XG_AGGREGATE`
  governs the xGC aggregate — a 2×2 over those switches has only **three** live corners and a
  zero interaction by construction. → **archive-and-data**
- **FPL revises team strength mid-season, in waves, and the revisions are outcome-driven** —
  coarse strength moves for 6-11 clubs of 20 in every season, fine fields for 20 of 20;
  fixtures.csv records one end-of-season `team_h_difficulty` per fixture, and `playedFixtures`
  strips the scoreline but not the difficulty. So any coefficient fitted on `def` **plausibly**
  carries post-cutoff information — mechanism, not measurement. ⚠️ This does NOT retract
  b2 = 1.5688 (the leak's size is unmeasurable on that comparison). ⚠️ **A third channel is
  suspected and is live on the scoring path**: the archive's `teams.csv` strength block — the
  "played and points are zero, so pre-season" inference is **invalid** (FPL's GW38 capture
  carries the same zeros; Arsenal's strength moves 4 → 5 *within* 2023-24). `PointInTimeWith`
  hands `cur.Teams` to the bootstrap ungated by `through`; `priorFromStrength` reads the fine
  fields, weighted heaviest where `played` is smallest. Whether `teams.csv` equals the last
  capture is reported rather than reproduced, and the size is unmeasured. Reproducible:
  `python3 stats/team_strength_revisions.py`. ⚠️ "0 of 380 difficulties changed" licenses
  nothing (three-day pre-season window). What is measured is team strength, not difficulty —
  the captures carry no fixtures payload. → **archive-and-data**
- **The payloads carry a great deal the code ignores** — `can_select`, the CBI/tackle
  components, team form, ownership, and the bonus-points coefficient schedule. →
  **archive-and-data**
- **The xG harvest left no coverage hole on goals, and left the assist channel about twice as
  exposed as a natively-fed season** — conditional on an assist, 193/1981 repaired against
  168/3306 native, Fisher OR 2.02 [1.62, 2.51]; the within-2022-23 contrast sits near 2× on
  every channel; "the harvest never saw this player" is eliminated. ⚠️ Counts of archive rows
  with NO points claim; the likeliest cause (a changed instrument between arms) is a hypothesis
  the count is consistent with and does not test. ⚠️ The ~0.3% mass share is a *lower* bound on
  decision leverage, not an upper one. **A naive `xg+xa > 0` gate would be wrong**; the right
  shape is a season/gameweek capability gate like `DefconScoredIn`. ⚠️ "Goals: closed" rests on
  the ungated population and is narrower than it reads. → **archive-and-data**
- **The exposed-return leak into the conversion fit is sized, and dropping those rows is
  refused** — it breaks the in-sample identity in every fitted cell. ⚠️ `XA == 0` is a
  two-decimal DISPLAY threshold, reaching only about a sixth of near-zero-expectation assists —
  **any population defined on `== 0` is a minority of its own phenomenon.** → **scoring-model**
- **The weekly capture yields nothing this season**, and unblocks four questions recorded as
  unmeasurable by next spring. → **archive-and-data**

### The harness

- **The replay's noise is sensitivity, not randomness.** Score scoring constants on `HOLD`,
  average over start points. **Budget jitter is not an averaging axis** (~60% of nudges change
  the squad, but the squads are correlated draws). → **harness-and-inference**
- **Go for the engine, R for the inference, CSV as the contract.** Go prints no standard error,
  no t, no verdict word.
- **The prediction benchmark is a second instrument** answering a different question (the model
  orders players 28% better than a five-game average). It cannot replace the replay.
- **The noise splits differently on the two metrics.** On `POLICY`, 78% is genuine
  season-to-season heterogeneity (more paths buy nothing); on `HOLD` it is 100% path noise, so
  entry points are the one remedy. 48 entry points is structurally impossible. →
  **harness-and-inference**
- **Captaincy is 45% of `HOLD`'s residual variance, and removing it is still worse** — it
  removes 47% of the signal too. Any variance reduction must be checked against a known effect.
  → **harness-and-inference**
- **A perfect armband is worth 210 points a season, and it is the largest resolvable thing
  here.** Its t of 20.4 is mechanical (same squad, same football, both arms). The decomposition
  is more useful: perfect hindsight ~465 over doubling nobody, the model's own weekly captain
  ~255, a captain pinned in week one ~228 — the model captures ~55% of the premium and captains
  the right player 22% of weeks, so **the entire observed span of captaincy *rules* is about 28
  points a season**. → **harness-and-inference**
- **An ordering is cheaper to establish than a gap.** The predicted order must be committed to
  in advance.
- **Paired standard errors are optimistic in one direction and pessimistic in the other.**
- **Judge a sweep on paired differences, not on totals.** Every sweep cell runs a five-transfer
  bank regardless of season, so absolute totals from `runPolicySweep` are not comparable with a
  real replayed season.
- **Start points are three information regimes and the early one is the quietest.** A sweep
  writes its own provenance before its first cell.
- **The archive carries what a field average needs** (`selected` gives ownership; an average is
  linear). It is an entry count that grows within and across seasons — quote it with season and
  gameweek or not at all, only on full rounds. `selected_N` at gameweek N is honest;
  `selected_{N+1}` is a leak. What is absent is other managers' *squads*: a reconstructed field
  overstates dispersion (everyone owns the template). → **harness-and-inference**

### The weekly transfer decision

- **The transfer charge is a volume brake, not an anti-churn device.** Raising it cuts moves and
  round-trips, but the *proportion* that are round-trips barely shifts. Stays at 2.0.
  **Rotating for blanks and doubles pays** — the best moves the policy makes. ⚠️ Rotating for
  DOUBLES pays; the blank half is unverified (those measurements ran while `fixtureLoadFor`
  could not express an imminent blank). → **transfer-policy**
- **Team value compounds, and the half-of-any-rise selling rule taxes 62% of it.** Affordability
  still rises (the squad converges on the best players). **You cannot sell at the market
  price**, and modelling it properly costs 31 points a season. → **transfer-policy**
- **Perfect price timing is too small for this harness to see** — +15 a season, t 0.95 against
  a threshold near 50. A bound, not a measurement; it caps the entire automation-by-speed
  argument. → **transfer-policy**
- **The premium-acquisition over-valuation was never measured** — the captain-doubling
  *mechanism* survives, the size does not; the resolvable evidence is in the under-£6.0m bucket.
  → **transfer-policy**
- **The policy never banks a transfer at shipped config, and the zero is checked, not
  degenerate.** 236 consulted weeks, 169 weighed, **0 banked** — in 72% of consulted weeks the
  rule had a real choice and preferred to act now; read as a **confinement, not a null**. The
  mechanism is identified and structural: identical candidate lists in 224 of 226 weeks
  (`RankPairs` builds multi-downgrade sets only for upgrades no single sale can reach), the
  horizon haircut ruled out by arithmetic, and at the shipped 2→3+ boundary 94 of 94 weeks with
  0 flips (318 of 320 across both arms including the `MaxHits: 0` control). The positive
  control banks 30 — but at the 1→2 boundary shipped can never reach. ⚠️ `HitCeiling` has
  removed the ground for "nowhere to vary `MaxHits` to": the inert boundary is 2→3+, and the
  boundary a two-hit arm reaches is **3→4, which no run has touched**; the preparation credit
  (`PrepareTripleCaptain`) is the other unrun channel. A tandem arm crossing banking at shipped
  `MaxHits` is a confinement, not a null. → **transfer-policy**
- **Reach is not the problem: 97.6% of worth-taking two-move packages are already reachable** —
  closes the unified-search line on mechanism. The lever is the **valuation**, not the gate. →
  **transfer-policy**
- **The sell side is calibrated; its error is entirely availability** — −0.100 per gameweek for
  a sold player who keeps playing, against −2.223 for the 13% who stop. → **transfer-policy**
- **The transfer path's noise, measured cleanly, is 303 points of spread** with `HOLD` provably
  byte-identical — the floor for any transfer-policy experiment. → **transfer-policy**
- **`MinGainHit` 3.0 stands and the hits mostly pay.** On the horizon criterion, 23.5% of hit
  packages come in below the gate's own bar (n 98; 26.9% availability-adjusted, n 78) against a
  ~50% truncation null, mean package +14.1; the MinGainHit ladder 3/4/5/6 resolves nothing
  (thresholds 13.5-18.8, no shape). On the **holding criterion** (the user's ruling: +4 net
  before the in-players are sold or a wildcard lands, squad contribution incl. captaincy and
  chips) **78-79% of hits clear, mean +35 to +45 after the −4**, holds ~10 gameweeks, on all
  three machines incl. the full user-facing plan; the forced (replaced player stopped) vs
  preference split does not separate (75-88% clear in both); no-hits costs −10 a season
  (unresolved). `stats/findings/2026-08-18-transfer-hit-tuning.md`. → **transfer-policy**

### Constants

- **The flat prior ships at k=8.** Two attempts to fix the mid-season over-confidence were both
  refuted; the unresolved direction is *upward*. → **constants-and-sweeps**
- **`LeagueShrinkK` is split out from `BlendRateK` and ships at 8.** Out of sample the league
  anchor wants K=2-4 (beats 8 in three of four seasons); wired into the replay, `HOLD` is a
  flat null and `POLICY` reads −0.843, t −1.94 — **unresolved, not a measured loss**. Ships at
  8 because a predictive win does not discharge the burden to move a constant. The two anchors
  answer different questions (`shrinkToLeague`: no prior; `BlendRateK`: everyone else). →
  **constants-and-sweeps**
- **`BandStrength` has banked cells and still does not resolve — and the arm that DECIDED it is
  still unrun.** s=1 against 0, `HOLD`: +0.357 pts/gw (+13.6), CR2 SE 0.184, t 1.94, against a
  threshold of 18 and an MDE of 24 — **unresolved, below its own MDE**, and not unmeasurable
  (the s=2 canary came back smaller and opposite, so it sizes nothing). The deciding arm was
  s = 0.25, **unrun**; the original was an argmax whose winner sits inside its own stated noise.
  ⚠️ The original refutation was `POLICY`, not `HOLD` — and at the originating commit
  `FPL_BAND_STRENGTH` reached only the transfer `SimConfig`, so the hold baseline was
  byte-identical across that whole sweep by construction. A textbook byte-identical-null trap.
  → **constants-and-sweeps**
- **The fixture mediator's canary is `band_strength` 2, and `band_ready_weeks` is not a canary
  at all** (220 at every dose — the funnel's first step can never respond). Mediator moves in 8
  of 8 cells from dose 1; the opening fifteen in 4 of 8 at 2. ⚠️ The choice of 2 over 1 is
  post-hoc; it is a readability threshold, NOT a recommended setting; the exposure ladder's
  direction is forced by construction and is not in fact monotone. The canary licenses a null
  at doses ≥ 2 only, is a four-season figure, and is simple-effect (banking off, no chips). →
  **constants-and-sweeps**
- **`BlendRateK` is banked and nothing resolves** — non-monotone over 3/5/12/16/24 (−4.1, +1.8,
  −11.6, +11.6, +12.6, Holm 1.000); **8 ships unchanged**. Two seasons carry the swing;
  dropping both reverses its sign. → **constants-and-sweeps**
- **Calibrate against data, not intuition — and check what a multiplier multiplies before
  calibrating it.** → **constants-and-sweeps**
- **No scoring constant with banked `HOLD` cells is measurably a schedule** — 7 ladders, 31 arm
  contrasts, Holm 1.000 on both. A tie the design guaranteed (interaction thresholds 152-349
  against this grid's own median of 39). Transfer constants are not screened at all
  (byte-identical on `HOLD`), and the screen cannot test its own motivating example (refuses
  `BONUS` on a label-text coincidence) — the prior/evidence bonus weight, which *is* a schedule,
  sits outside it. Read as "of the constants the screen accepted". → **constants-and-sweeps**

### Chips

- **All four chips are modelled, and the replay cannot value a wildcard.** Bench boost is the
  one chip this harness can value. Swept over its own week, the wildcard has exactly one
  reliable week: **GW4** (positive in all four seasons — the opening fifteen is built on the
  season's weakest information), below the detection threshold even so. → **chips**
- **Anchoring the chips on the calendar is a clean null as a measurement, and "anchoring is
  worth nothing" is not established** — MDE 34-37 per season-path, sign resting entirely on the
  GW1 column. `fullSight` is the realistic arm. → **chips**
- **A bench-boost PLACEMENT contrast is measurable at a threshold of 2.65 — the comparison, not
  the effect.** The chip is path-invariant (consult after pickXI; `BenchBoostGain` (what the
  chip would have been worth, per week) recorded
  against the unchipped week; confinement checked 36 of 36), so the paired SE contains only
  football. The decaying-option rule beats a fixed entry+6 by **+5.778** (t 5.60, wild 0.0096,
  6 of 6 seasons positive); perfect placement is +16.139 (mechanical t), recovered fraction
  0.358 Fieller [0.198, 0.518]. **Verdict: measured-neutral, lever stays off** — the control is
  a straw man on timing (rule fires mean GW 27.97 vs 19.5; lateness is designed into the rule;
  the anchored comparator is unrun and owed), the control is an average week not a bad one, the
  ceiling is a mixture of six argmax problems, the levels against no chip are ≥ 0 by
  construction, and nothing weighs the chip against its opportunity cost. Two bar-16 rules exist
  (`firstClearing` realised vs `BenchBoostTrigger` (the projection-based trigger) projection;
  levels 17.9 vs 9.5, agreeing 6 of
  36) and the bar is two literals with no reference between them. → **chips**
- **`OptionPricing.CongestionSensitivity = 0` means the DEFAULT of 1.0, not off** —
  `CongestionFactor` (what scales a chip's bar by fixture congestion) reads `if sensitivity <= 0
  { sensitivity = DefaultCongestionSensitivity }`, the struct's correct unset-means-default
  convention and a trap for an arm meaning to hold the channel still: the default is the
  *strongest* setting, so a zero reports a confounded contrast as a clean one. Say `1e-12`.
  `TestCongestionSensitivityZeroIsTheDefaultNotOff` pins both halves. → **chips**
- **The scoring-chip timing `+0.000` is a declared invariance, not a result.**
  `mustNotMoveForAxis(AxisChipWeek)` returns the eight `cellMetricColumns` and the harness
  checks them cell by cell; the axis reads a finished season's gains and plays no chip, so a
  byte-identical `POLICY` is what it is *required* to produce. The **levels** are a different
  quantity and are unbanked: timing **+8.3** a season, threshold rule **+21.9** (never halve
  either; each is the sum over the two scoring chips). Both are functions of the **asserted, not
  measured** bars 16 and 12; a t against zero is mechanical; an interval on a bound is the only
  legitimate reading. **Both halves are now banked under the current schema** and recompute to
  +6.4 timing and +30.2 threshold rule — read **beside**, not instead (different grid, data
  state; still ≥ 0 in every cell by construction). Nothing has been re-measured under the
  banked schema; a re-sweep is owed. → **chips**
- **Only two of the four chips are *preparation* problems** (free hit and wildcard were already
  wired for the preparation credit; the free hit's own *builder* needs its own blank guard — see
  *Things that have already bitten*). `ChipCredit` (the preparation-credit config for bench
  boost and triple captain) adds the other two, off by default. **The
  bench channel is mechanism-real and points-unresolved**: chip's own week +7.28 (t 2.91, p
  0.033, 27 of 36 positive) — **suggestive, not established** (Holm ≈0.066, no LOSO subset
  under 0.05). The season figure +13.3 (against this comparison's own threshold of 17.7-24.5,
  never the global 70) is 73% one season. → **chips**
- **The two preparation channels overlap by about a third on the chip week and fight over a
  season** — interaction −4.2 on the boost week (t −2.38), −18.3 a season (t −2.23), neither
  clearing 0.05. **Do not restrict to the non-zero cells** (dropping them drops 2023-24
  entirely). → **chips**
- **Truncating the transfer horizon at a planned wildcard is bounded, and closed on mechanism** —
  +15.8 a season-path, losers −14.0, against the 303-point noise floor. → **chips**
- **Triple-captain preparation changed no decision, which is not a measured zero.** Only 23 of
  36 cells place the chip; the estimator is degenerate (bounds the flip rate near 8%). Quote
  no p. → **chips**
- **Do not project the two-set chip rule backwards to buy chip observations.** The first half
  holds 15 of 189 doubling club-gameweeks, 11 of the 15 in one rescheduled 2020-21 round;
  2025-26 holds none. A first-half arm is collinear with "a chip on a plain week"; two halves
  share one squad. Two sets are expressible for 2025-26 onward, off by default, unmeasurable at
  6 cells. → **chips**

### The optimiser, the squad, and the money

- **The optimiser was garbage-collection-bound, not compute-bound** — 6.2× faster and bit-exact.
  That changes what is affordable to run. → **optimiser-and-squad**
- **The optimiser's move set** needs N downgrades funding one upgrade; the ranking proxy may
  filter but must never decide. Fixing the search made realised points *worse* (a correct search
  exploits a mis-specified objective harder). **An injured premium needs no special case** —
  correct the minutes and let everything downstream recompute. → **optimiser-and-squad**
- **Money is worth what it can still be turned into** — a £100m squad holds only about £36m of
  discretionary spending. Valuing freed money inside the transfer gate does not work. →
  **optimiser-and-squad**
- **The bench is a hedge and its slots are not interchangeable.** The multiplier is P(this slot
  is needed), not P(he plays). The tie between shapes survives re-measurement (four arms × 36
  cells, ~12 points span against thresholds 17-40); the derived weights keep their
  `ViceCaptainWeight` justification; the derived arm also varies effective `BenchWeight`. →
  **optimiser-and-squad**
- **Two elevens exist on purpose** — `Plan.XI` is picked on the imminent fixture, while
  `Plan.GainPerGW` is measured on the horizon eleven. Presenting a recommendation beside an
  eleven nobody would field is wrong; changing what the *decision* optimises is a different act,
  and is worth nothing. → **optimiser-and-squad**

## Season maintenance

Four things are not in the FPL API and go stale the moment the season turns over. They ship as
dated 2026/27 defaults (`DefaultEuropeanCampaigns`, `DefaultDomesticCups`, `DefaultNewCoachClubs`,
`DefaultRestPlayers`) and must be re-derived every summer:

- **Competition windows** per club, with start *and* end dates. `armband congestion` reports
  what is set and how stale it is.
- **Managerial changes.** The test is not "is the manager new" but "was last season's data
  produced under him" — which is why Tottenham is on the list and Manchester United is not.
- **Post-tournament rest.** Names must match the FPL spelling exactly, accents included.
- **Nationality code lists** for travel load — `armband nations` maps the opaque codes.

Two regression tests fail loudly if a hand-maintained name stops resolving, because that failure
is otherwise silent.

**Three of the four lists are display-only, and one is live on the scoring path.**

- All eight congestion penalties ship at 1.00, so the competition windows and the nationality
  lists can only mis-inform a human, not mis-score a player.
  `TestTheShippedCongestionBlockIsInert` makes re-enabling deliberate; the channel would be
  minutes, not `Score`. `DefaultNewCoachClubs` is display-only too, through `NewCoachPenalty`
  in `rolerisk.go` — a ninth penalty, outside the congestion block.
- **`DefaultRestPlayers` is live.** `blendFor` applies `restFactor`, multiplying
  `MinutesPerMatch` and `StartShare`. Live at **GW1 and GW2 only** (`restFactor` returns 1 once
  the next gameweek is past `rest_gameweeks`, which is 2) — so it bites in exactly the two
  gameweeks after the summer maintenance that was supposed to have checked it. ⚠️ **The applied
  multiplier is `rest_minutes_factor` (0.83) prorated across the horizon, not 0.83 itself** — at
  a 5-gameweek horizon and a 2-gameweek window it is (2×0.83 + 3×1.00)/5 = **0.93 at GW1**, 0.97
  at GW2, 1.00 from GW3. The unprorated version is a bug this code has already paid for.

Two unrelated mechanisms answer to the word "rest": `ShortRestPenalty` and `VeryShortRest` are
congestion and are inert, while `rest_players` is the post-tournament teamsheet and is not.
`restFactor` has two further call sites in `metrics.go` — one un-applies it to build
`SettledMinutes` for the pool filters, and one is reporting-only and labelled as such, which
reads misleadingly like confirmation that nothing is scored.

**The rest list is a teamsheet, not a squad list, and it looks incomplete because it is supposed
to.** Read the Go comment on `DefaultRestPlayers` before editing `config.json`'s copy, and see
[configuration.md](docs/configuration.md) for the rule and the worked list.
