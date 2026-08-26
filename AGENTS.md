# Project context for Claude Code

A Go CLI that scores Fantasy Premier League players with a quantitative model, with a Claude
agent on top to reason over the output. Read [docs/architecture.md](docs/architecture.md) before
changing code and [docs/model.md](docs/model.md) before changing the scoring.
[docs/README.md](docs/README.md) maps the rest.

Everything in `docs/` is **reference** — what the system *is*, written for a reader of the
repository. Design proposals and research notes do not belong there.

This file holds the **rules and pointers**: build and test commands, code conventions, the
standing rules that keep the next measurement honest, and pointers to everything else. The
verdicts themselves — the findings, the evidence, the numbers — live in the **research
vault**, the private Obsidian store reached through `~/.claude/bin/research-worktree`; the
`→ **name**` at the end of an entry names the note that carries it (`notes/<name>.md` unless it
says otherwise). "Closed lines" and "What has been measured" are therefore **named lists** —
titles and pointers, deliberately not verdicts restated: every session starts at the store's
index.md, so a title plus a pointer routes a reader to the verdict, which a repo with no second
store could never do. The user-facing docs never reference the vault; this file and the other
agent-facing surfaces may. Three things in the tree are also evidence: `stats/findings/` holds a
narrative and a pre-registration per run, `stats/cells/` holds the banked cells two R screens
read as input, and `stats/snapshots/` holds one-off sweep evidence for closed findings.

⚠️ **The ACCURACY series is no longer committed here, as of 2026-08-22.** It publishes as a
GitHub Release asset on every push to `main` (`.github/workflows/snapshot.yml`), never as a
tracked directory — see that workflow's own comment for why, and
`internal/snapshot`'s `TestSnapshotCoversTheCurrentCode` for the guard that used to require the
commit. **The published series is not a citable record**: nothing in this repository may point
at it, for the same reason `stats/snapshots/` itself could never be cited directly — see the
retired-location guard. A figure a comment needs stated has to be inlined at the citing site,
not pointed at a release. What remains under `stats/snapshots/` is the older, pre-2026-08-22
evidence directories that predate this split and the odd one-off sweep since — check a
directory's own files before assuming it is either kind.

⚠️ **The 2026-08-17 compaction moved derivation narratives out of this file, and the 2026-08-19
cut moved the verdicts after them — titles and pointers stay resident, derivations and verdict
bodies do not.** If an entry seems too short to act on, read the note it points at before
rebuilding anything.

## Build and test

```bash
go build ./... && go vet ./... && go test ./<package you touched>/...
```

Build and vet run over the whole module — they are fast and catch a break anywhere. **Test locally
scoped to the package or two you actually touched; the full suite is CI's job**, on every push and
every pull request (`.github/workflows/ci.yml`).

**Do not build a tool that derives test scope from the import graph.** That was built, measured
and deleted on 2026-08-19: Go's own test cache already re-runs the packages a change reaches — the
changed package, everything importing it, and anything whose tests *open* a changed file within
the module, which is `go help test`'s own boundary — and that reaches cross-package source scans
an import graph cannot see, which is exactly where the cross-cutting guards in `internal/snapshot`
live. **This is a practice, not automation**: run the scoped command above while you work, and let
CI be the full answer.

⚠️ **A suite that re-runs everything every time is the symptom of a FULL DISK, and so are browser
tests that fail LOCALLY on a screenshot write.** Go declines to cache a result it cannot write,
silently. Observed 2026-08-19: several sessions running the suite at once took `/` to **100%, 61 MB
free of 58 GB** — 27 GB of it in `$(go env GOCACHE)` — and in that state a run died on `no space
left on device` writing `testlog.txt`, `internal/webui` reported eleven `TestLayout` failures that
were the browser unable to write a screenshot, and `internal/backtest` alternated cached and not
with nothing changed. It drained to 26 GB free within the hour, so **it is contention, not a
standing state**: check `df -h /` before believing a red run, and `go clean -cache` if it is not
draining.

⚠️ **CI's `TestLayout` redness on `main` (six subtests, machine-dependent goldens, worst channel
delta 2 of 255, since 2026-08-19) is fixed as of `97c941c`** — it is skipped in CI now, not green
by repair; `FPL_LAYOUT_GOLDENS=1` forces it back on for anyone who wants to look, and see the
standing exception below for when you owe it a local run instead. **Do not read this paragraph as
"CI is clean," and do not trust its own claim without checking `gh run list` first — this project's
CI state has already gone stale under one written description of it inside a single day.** As of
`97c941c` `main` is red on a different, narrower thing: `TestEnvSwitchListIsComplete` fails because
the same change that fixed `TestLayout` added `FPL_LAYOUT_GOLDENS` without registering it in
`envSwitches` — a one-line fix, already known, not yours to chase on an unrelated branch. If your
own branch inherits exactly that failure and nothing else is red, that is expected; anything else
red is worth investigating.

⚠️ **Standing exception, until the goldens defect above is fixed: run the layout goldens locally
yourself, because CI cannot see them.** A companion change skips `TestLayout` in CI (detecting
`GITHUB_ACTIONS`) so the red check above stops being the default state a reviewer has to explain
away on every PR; locally the goldens still render and compare, which is where they have actually
caught regressions. `FPL_LAYOUT_GOLDENS=1` forces them back on in CI, for anyone who wants to look.
So: **a change touching `internal/webui`, or anything that feeds what it renders —
`internal/viewmodel` and `internal/present` both do — must be run locally with
`go test ./internal/webui/ -count=1` before it ships**, because "test the package you touched"
above is not sufficient on its own: the package that changed and the package that owns the goldens
are not always the same one. **This exception expires with the CI skip that motivates it** — when
the goldens defect is fixed and the skip is deleted from `internal/webui/visual_test.go`, delete
this paragraph with it, not before.

Tests hit the live FPL API and skip when it is unreachable. They assert invariants, not exact
values — the underlying data changes weekly, so a test pinned to a specific player or score rots
within days.

**Changes land through a pull request against `main`, checked by CI.** This retired, 2026-08-20,
the twelve-condition `merge-gate` skill, its review-record counterpart `review-gate`, the
`armband reviewkey` command and `TestReviewCoversTheCurrentCode` — `armband` is a product an end
user runs, and a command that existed only to serve this project's own review ritual did not
belong in it. Some of what `merge-gate` condition 1 did by hand — a green run of the whole suite,
keyed to the commit, that a reader who was not there can pull by id rather than take on trust — a
pull request now gives mechanically: CI runs on every push to a branch and every pull request, and
the check is attached to the PR itself. Reviewers are still dispatched (self-review is forbidden
on this project) and their findings still belong in the PR description; there is no longer a
required `reviews/` record — the 171 records already committed stay as history. **Current
practice: measurement work lands on `development`; other changes go through a pull request to
`main`.**

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

- **Go for the engine, R for the inference, CSV as the contract.** Go prints no standard error,
  no t, no verdict word.
- **The prediction benchmark is a second instrument** answering a different question (the model
  orders players 28% better than a five-game average). It cannot replace the replay.
- **An ordering is cheaper to establish than a gap.** The predicted order must be committed to
  in advance.
- **Paired standard errors are optimistic in one direction and pessimistic in the other.**
- **Judge a sweep on paired differences, not on totals.** Every sweep cell runs a five-transfer
  bank regardless of season, so absolute totals from `runPolicySweep` are not comparable with a
  real replayed season.
- **Start points are three information regimes and the early one is the quietest.** A sweep
  writes its own provenance before its first cell.

## What each season can and cannot run

**A byte-identical season under an intervention is not a tie — it is a season where the
intervention could not run.** The season capability table lives in [docs/replay.md](docs/replay.md) —
it is replay documentation, needed when running a sweep, not every run. → **archive-and-data**

## Standing rules

- **Four classes take priority over anything else in the queue: security, performance, velocity,
  and a model or scoring fix** — in that order when they collide. This is **precedence, not
  worth**. Security is currently empty by construction against the FPL API (no
  authenticated surface there; `TestTheClientHasNoAuthenticatedSurface` guards its absence
  and is scoped to `internal/fpl` — it says nothing about `armband serve`'s own inbound
  listener, which is token-gated, writes config under `-persist`, and now also accepts a
  gate POST that records an email address to Postgres — the only personal data this
  project COLLECTS, as against the published FPL payloads it archives. Performance
  changes what is
  affordable to run, the binding constraint on this enterprise; velocity is the same argument one
  layer up; a scoring fix changes `Score`, therefore the ordering, therefore which footballers
  get bought.
- **Verify on staging against real live data before promoting to production, not just `go
  test`, for anything touching account-specific or live-API-dependent paths.** A staging
  environment — a second copy of the app in its own namespace, in the separate ops/deployment
  repo — went live 2026-08-22 and was used that same day to check a live-API fix this way before
  it reached production. That class of bug had passed every test and still broken
  `fplarmband.com` within minutes of a deploy earlier the same day.
  ⚠️ **Verify through the SURFACE the reader uses.** Running the binary and curling the
  endpoint are not proxies for the page. On 2026-08-23 the CLI was right, `/api/transfers`
  was right, and the page rendered `plans[0]` of five — found by a person clicking the
  button, on a candidate two earlier staging deploys had each "verified".
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
  *idiom*, so they are tripwires rather than proofs. ⚠️ **Both scans are Go-only, so a copy in
  the served client is invisible to them**, which is how the transfer gate came to be decided
  twice with two different roundings. Where deleting the second copy is not available, pin the
  two languages against one table — `TestTheGateIsDecidedTheSameWayInBothLanguages` — and put
  the SEPARATING cases in it, or the pin fixes the shipped constant rather than the rule.
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
in the vault; the lesson and the pinning test here — the test is the guard. →
**harness-and-inference**, **optimiser-and-squad**, **archive-and-data**

- **A minutes floor written as a season total must be scaled before it is compared, at EVERY
  call site.** → **transfer-policy**. `buildTransferBoard` filtered on a bare
  `c.Minutes < 600`; against fresh-season aggregates 0 of 609 players cleared it, so the pool
  was empty and every transfer surface answered "nothing would improve this squad" for about
  seven gameweeks. `ScaledMinMinutesFor` already existed for exactly this and had been applied
  to `Optimize` alone. Pinned by `TestTheTransferPoolScalesItsMinutesFloor` (source scan).
  ⚠️ **There were THREE copies.** The first fix took one; `internal/agent/tools.go`'s own pool
  was still unscaled afterwards, while the same file scaled the floor correctly twenty lines
  above. The guard now reads every file that builds a pool, because one scoped to the file
  whose bug prompted it reads as coverage and is not.

- **The hit ceiling is a knob, not a clamp, and both expressions of it must move together.** →
  **transfer-policy**. Pinned by `TestTheHitCeilingIsReadByTheFundedPairBranch` (source scan)
  and `TestTheHitCeilingIsReachableAndDefaultsToOne`.
- **A differential corpus on a dyadic score grid cannot see a re-associated sum, and the
  optimiser's was entirely dyadic.** Sums of eleven exact multiples of 0.25 are exactly
  representable, so any regrouping of one is bit-identical *by construction* — a comparison green
  because it never ran. It shipped a "bit-identical" claim for `bestFormation`'s prefix fold that
  was false. `optimizerdiff_test.go` now carries a continuous arm, and records why a DECIMAL grid
  must not be added: tenths are both non-dyadic and coarse enough for two formations to tie
  exactly, which is the one region where the fold really does pick differently. →
  **optimiser-and-squad**
- **A per-request mutation of a shared engine outlives the request.** `serve` holds one engine
  for every reader, and `ApplyChipPlan` shortens `Weights.Horizon` for a planned wildcard —
  correct for that build, permanent unless it is put back. Save and restore whatever a request
  mutates. **A CLI that exits cannot find this class, and most of this code was written as
  one.** Pinned by `TestAPlannedWildcardDoesNotShortenTheNextReadersHorizon`.
- **`runPolicySweep` builds cells at `WeeklyXI: false`, and several diagnostics run at `true`.**
  Pin the setting in `apply` and stamp it.
- **Never compare a replayed float for exact equality against a BANKED total: a banked total is
  reproducible from a commit AND a machine, and only the commit is recorded.** Two fresh arms run
  back to back on one machine hold the machine fixed, and there exact equality is the correct and
  **sharpest** test of a rewrite claiming to preserve answers — it has no detection threshold,
  because the prediction is exactness, so one differing cell refutes it at power 1. Do not quote
  the first clause to block the second. → **harness-and-inference**
- **The doubles guard must key on `(element, fixture)`, never `(element, gameweek)`.**
  `season.go` accumulates, never assigns; the phantom/duplicate counts are pinned at load.
- **Anything reading fixture results must be gated by gameweek.** →
  **archive-and-data**. Pinned by `TestPointInTimeHidesFutureResults`. ⚠️ **The PRE-SEASON
  path is unguarded** (`PreSeasonWith` unfiltered; behind `FPL_MAGNITUDE`, latent not live).
  **Unfixed.**
- **FPL's aggregates reset at GW1, so the denominator must follow.** Use `Engine.DataWindow()`,
  never the constant 38. `TestDataWindowTracksTheSeason`. ⚠️ **`DataWindow` alone is not enough
  during the live GW1 gap** — `SeasonHasStarted()` true, `GameweeksPlayed()` still 0, a span of
  days — where it answers a pre-season 38 that is false for every club: FPL zeroes the whole
  league at the first kickoff, and within the gap some clubs have a completed match and some have
  not begun. `GameweeksPlayed()>0` and `SeasonHasStarted()` are two implementations of one
  question and agree all season except this gap — six call sites across five PRs came from
  using the former where the latter was meant: #39, #42, the squad pool's minutes floor,
  `bonusEvidence` and `AssemblyBudget` (both #45), `squadPriceGameweek`. (#40 shipped the same
  day in the same window but is mechanistically distinct — a blend-weight dilution, not a
  swapped predicate — and is not a seventh instance of this specific pattern.) Use the shared
  `inLiveGameweekGap()` predicate, whose doc comment routes
  every call site to one of three classes: `matchesAvailable` for a rate DENOMINATOR (per-club
  `TeamMatchesStarted`, only when `>0`), `minutesFloorWindow`/the minutes-evidence mix for a
  sample-size THRESHOLD or evidence COUNT (per-club `TeamMatchesFinished`, always including 0 —
  never `TeamMatchesStarted` there, a live match's minutes are still climbing), and
  `bonusEvidence`/`AssemblyBudget` for aggregate PROVENANCE (`SeasonHasStarted()` alone, no
  per-club signal — the quantity itself, e.g. `el.Minutes`, already carries per-player evidence).
  ⚠️ **Two of these bugs cancelled each other out, and fixing one exposed the other as a live
  production incident.** `AssemblyBudget`'s gate and `run()`'s squad-price-fetch gate both used
  `GameweeksPlayed()==0` for the same "has this manager bought anything" question; fixing the
  first alone (correctly) turned the second's dormant nil `Bank`/`SquadValue` into a hard 500 for
  the real production entry, deployed and rolled back within minutes. **A fix in this family is
  only proven by exercising the live gap end to end, not by `go test` on the changed function
  alone** — grep every other reader of the two raw predicates before shipping one fix among them.
  Pinned by `TestTheMinutesFloorScalesOnTheClubsOwnMatches`,
  `TestTheBonusScheduleReadsAPlayedMatchDuringTheGap`,
  `TestSquadPriceGameweekTracksEveryGameweeksOwnGap`. →
  **the-squad-pools-minutes-floor-was-the-fourth-datawindow-gap-bug-and-the-only-one-users-saw**
- **Every per-90 rate must go through `blendFor`, counting stats included.**
  `TestCountingStatsGoThroughTheBlend`.
- **A player with no prior is not a player with no uncertainty.** `shrinkToLeague` pulls rates
  toward the position's league rates; minutes are deliberately left alone.
- **`starts_per_90` is not a rotation signal.** Use minutes and starts against the full
  38-game season.
- **Single-swap local search stalls, and paired swaps are not enough either.** `dpseed.go`
  solves each formation exactly and seeds the local search — **do not "simplify" that away.**
  `TestOptimizerIsNeverWorseThanAnExactSeed`, `TestNoPremiumSquadBeatsTheOptimum`.
- **The seed's bench reservation must take the *cheapest* players who could fill those slots.**
  `TestSeedBudgetLeavesRoomForThePremiums`.
- **Never let the pair search choose greedily, and charge per move rather than per week.** The
  ranking proxy may filter but must never decide.
- **A free transfer is not a costless transfer, and four points is the intuitive price and it is
  wrong.** ⚠️ 0.0 is not a rung — the recorded four-against-nothing comparison is still owed.
  → **transfer-policy**. Pinned by
  `TestTheFreeTransferChargeIsInertOnSinglesBelowTheKink` and
  `TestTheSinglesProposalCarriesNoAlternativeOrStrictFlag`.
- **FPL banks 5 free transfers, not 2** — `backtest.BankLimitFor` keeps replays on the rule
  actually in force.
- **Every engine that scores players needs the recency index.** `TestEveryScoringEngineGetsRecency`
  counts them.
- **Overrides are keyed by permanent player code** — element ids are reassigned every summer.
  `TestExcludedPlayersAreNeverOffered`, `TestLockedPlayersAreNeverSold`.
- **2019-20's rounds are labelled 1-29 then 39-47.** `renumberGW` maps them;
  `hasRestartGameweeks` refuses a stale cache.
- **A fixture window must be anchored on the calendar's next GAMEWEEK, never on the club's next
  FIXTURE.** → **fixtures-and-difficulty**. Five pinning tests:
  `TestFreeHitNeverFieldsABlankingClub`, `TestWeekViewsPriceEachWeeksOwnFixtures`,
  `TestAWildcardIsNotBuiltOnOneWeeksBlanks`, `TestATotalBlankIsWorthNothingToTheTransferSearch`,
  `TestFixtureLoadMatchesTheArchiveOnOneSeason`. ⚠️ **The sharp trap**: a rebuilt wildcard must
  NOT be built on the horizon-1 week engine — a wildcard planned for a heavy blank returns a
  free-hit squad that is then *kept*.
- **An anchored-chip arm silently lost every 2025-26 cell** — a plan went into the FIRST set
  wholesale; repaired by `backtest.SplitChipSets`. ✅ **Anchored-chip cells are banked**, at
  `stats/cells/2026-08-25-f7d2be1b/` (bundled, per-chip, wildcard value) and
  `2026-08-25-tcmatchup/` (triple captain on matchup). ⚠️ **Read with `--scale=per_path`** —
  a chip is an event count, and per-gameweek inflates these ~1.7x. → **chips**
- **BOTH xGC ESTIMATORS ARE EQUALLY ACCURATE where a truth exists.** On the same 32,477
  player-gameweeks in the three native seasons: source **13.1% MAE, −0.4% bias, r 0.945**;
  reconstruction **14.0%, −0.1%, r 0.937** — a dead heat, and the reconstruction is the *less*
  biased. They agree to **5.5%** when both eat the same xG provider. ⚠️ **The 17.4% gap in the
  repaired seasons is the xG PROVIDER (Understat, borrowed offset — see `xgcrepair.go:177-186`),
  not the estimator.** ⚠️ **So NOTHING explains the SE doubling on the chip arms, and nothing
  distinguishes "the reconstruction borrows strength" from "the source injects variance" — that
  question is OPEN.** → **archive-and-data**
- **xGC has two sources; the DEFAULT IS THE RECONSTRUCTION.** `XGCExternalDir` /
  `FPL_XGC_EXTERNAL_DIR` selects measured per-match xGC for 2020-21, 2021-22, 2022-23
  GW1-15; empty (the shipped config) selects the reconstruction. ⚠️ An unresolvable
  directory is a hard ERROR, never a fall back; the switch is fingerprinted, so a cell
  states its arm. ⚠️ **The two arms are not comparable.** → **archive-and-data**
- **`Optimize` is not run-to-run deterministic unless it is made so.**
  `TestSeedOrderIsDeterministic`, `TestBandAssignmentIsDeterministic`,
  `TestBandTiesBreakTowardTheLowerClubID`. ⚠️ Every pre-fix `BandStrength` figure carries that
  jitter; the post-fix value is UNMEASURED. → **constants-and-sweeps**

## Closed lines — do not rebuild these

Each was measured and lost, closed on mechanism, or withdrawn after re-measurement. **This is a
NAMED LIST — titles and pointers — deliberately, not a verdict restated**: a bare title in a repo
with no second store dead-ended, and this repo has one — every session starts at the store's
index.md, and the bold name at the end of each entry is the note the evidence sits in. A title
alone does not stop an idea being rebuilt; the pointer does not either, but a title plus a
pointer routes a reader to the verdict, which a bare repo never could. Do not "finish the job"
by deleting the list, and do not re-derive a verdict from a title alone.

- **Do not build a custom fixture-difficulty rating, do not target the worst defences, do not
  band attack and defence separately, and do not move the fixture window.** → **fixtures-and-difficulty**
- **Do not extend recency to rates.** → **recency-and-priors**
- **The clean-sheet over-prediction ships uncorrected.** → **scoring-model**
- **Neither the clean-sheet factor `f` nor the defensive fixture ladder separates from 1 on the
  stratum that carries the verdict, and the ladder is the higher of the two point estimates rather
  than the established location of the excess.** ⚠️ **No pooled figure may be quoted, except in
  this sentence, which disowns it.** ⚠️ The leak's SIZE is unmeasurable and this does NOT retract
  the ladder's 1.5688. → **scoring-model**
- **Do not remove the bonus term for being circular.** → **scoring-model**
- **Do not penalise a squad for holding two players from the same club, and do not build a
  "talisman" rule.** → **scoring-model**
- **Do not port a correction across positions on the strength of an analogy.** →
  **scoring-model**
- **Stop sweeping the transfer gate: nothing swept in this family is recorded as having
  resolved.** → **transfer-policy**
- **`min_gain` ships at 0.4 and is inert at or below it.** → **transfer-policy**,
  **constants-and-sweeps**
- **The minutes floor's "argmax protection" does not reproduce, and re-measured at −40 the
  direction reverses** — unmeasurable rather than unresolved; quote no p, no interval, no
  threshold. → **constants-and-sweeps**
- **No projection constant re-tuned at 24 cells is "confirmed".** → **constants-and-sweeps**
- **Twelve cells could not resolve 37 points a season.** → **constants-and-sweeps**
- **Do not unify the transfer searches.** → **transfer-policy**
- **Do not ship a state trigger for the wildcard, and do not read a wildcard replay as a
  valuation.** ⚠️ **Both halves have now been re-opened and BOTH survived**, so this line rests
  entirely on measurement. The valuation half, 2026-08-25: a wildcard-against-no-wildcard arm
  reproduces the null it predicts. **The trigger half, 2026-08-26 — it previously said
  "unre-examined" and that was already false when written.** Three readings were built and swept
  against a control that plays no trigger at all, 36 cells, one commit, `dirty=false`:
  **the shipped cost rule reads −3.53** a season-path (CR2 SE 3.66, threshold 9.4) — it is
  NEGATIVE and it takes MORE hits than the control, 2.06 against 1.94; single-week XI drift
  +3.50 (threshold 11.2); the fixture-aware lookahead +1.67 (threshold 11.1). **Nothing
  resolves and the best arm is a third of its own threshold.** ⚠️ Higher lookahead bars fire in
  4 of 36 cells and are inert rather than good — read the fire count before any null here.
  `2026-08-26-wildcard-lookahead/`, `2026-08-26-wildcard-attribution/`. → **chips**
- **⚠️ Do not expect a fixture-aware reading of squad drift to pick different weeks — measured,
  and it does not.** `WildcardDriftBar` reads the horizon-5 engine where `FixtureLoadInScore()`
  is FALSE, so it is a five-week average blind to the doubles inside its own window;
  `WildcardLookaheadBar` re-reads one gameweek at a time at horizon 1, where fixture load IS
  scored. At matched fire rate — 14 and 15 of 36 — the two pick the **same median gameweek
  (15)**, take the **same hits (1.69/cell)**, and differ by +1.67 against +3.50 with thresholds
  above 11. The readings correlate **0.884**. ⚠️ This says the two RANK weeks alike; it is not
  evidence that fixture awareness is worthless elsewhere. → **chips**
- **Do not add a lock.** → **optimiser-and-squad**
- **Do not scope the local test run to the packages a change touches.** Built and measured 2026-08-19: the Go test cache already does it, and better — it tracks the cross-package source scans an import graph cannot see, so a hand-derived scope skips exactly the guards this record pins its shipped bugs with. → **work/ruled-out/scope-the-test-run-and-move-the-suite-to-ci**
- **Do not memoise `blankRate`.** Answer-exact and measured no faster —
  `playsAtAll` is cheaper than the cache lookup that would replace it. →
  **optimiser-and-squad**
- **Do not use the olbauday CSV mirror** for a live weekly signal or for priors. →
  **recency-and-priors**
- **Do not build a squad for rotation.** → **optimiser-and-squad**
- **Do not chase a rank objective for "maximise my percentile".** → **harness-and-inference**
- **Tuning constants on accumulated expected points ("xPoints") is closed; the columns stay as
  instrumentation.** → **xppilot**
- **xPoints prices xG and xA through a per-season, per-position conversion scale**, fitted
  in sample, and ships on mechanism. → **xppilot**
- **The underlying criterion recovers 0.64 of a perfect points gate, Fieller upper limit 0.813.**
  → **xppilot**
- **A gate on the residual alone buys realised points, but its underlying gain is consistently
  negative — `suggestive`, not established.** → **transfer-policy**
- **Accepting every offered swap costs 82.4 a season on realised `POLICY`** and gains 40.9 gross
  of the transfer charge. → **transfer-policy**
- **An antisymmetric pair of criteria cancels the common level but not the accept-mass
  asymmetry.** → **xppilot**
- **The gate oracles are a veto on one candidate, not a selector, and they replace the value bar
  rather than adding to it.** → **xppilot**

## What has been measured

**A named list — titles and pointers.** The evidence and the numbers live in the vault notes the
bold names point at; a verdict here cannot be checked from this checkout (never write
"I verified" when you mean "I re-ran"), and re-measuring is still how a verdict falls — when it
happens, the new number sits *beside* the recorded one, so say which you have. Absence
from this list is weak evidence of absence — nothing checks it stays complete.

### What a player is worth: the scoring terms

- **P(appears) ships as one implementation.** → **scoring-model**
- **A club's expected goals sum to the club on average but not club by club.** The short-clock
  `FPL_TEAM_FORM` is open and unresolved, built and ships off. → **scoring-model**
- **The 2026/27 bonus rules are measurable on old football.** → **scoring-model**
- **The pre-2024-25 bonus-points schedule is decoded, not read off an announcement.** ⚠️ Do not
  check "the archive's only goalkeeper goal" with a `position == 'GK'` filter — join
  `players_raw.csv`'s `element_type` instead. → **scoring-model**
- **The defcon term is about half redundant for defenders — quote 50%.** → **scoring-model**
- **Four findings that change nothing shipped.** → **scoring-model**
- **Fixture multipliers are applied per fixture, not averaged.** → **scoring-model**
- **The captain had no vice-captain and the replay forfeited the bonus.** →
  **harness-and-inference**
- **The minutes model under-reacts to the onset of an absence.** → **scoring-model**
- **The bonus term should not be a constant.** → **scoring-model**
- **Real team news is measured and too small, rather than unmeasurable.** →
  **harness-and-inference**
- **Perfect minutes is the largest information bound on the *scoring* side — two capabilities,
  none of it resolves.** ⚠️ Quote ≈93 with its data state — it moved with the starts harvest. →
  **harness-and-inference**

### Recency and priors

- **Minutes need recency; rates do not.** → **recency-and-priors**
- **`MinutesHalfLife` ships at 4, and the reason is asymmetry, not the points.** →
  **recency-and-priors**
- **Multi-season priors come from FPL's `history_past`, which returns them oldest first.** →
  **recency-and-priors**
- **A thin prior season is a fallback trigger, not a smoothing opportunity.** →
  **recency-and-priors**
- **`prior_half_life` stays off — unresolved, not favourable.** → **recency-and-priors**

### Fixtures

- **Fixture *count* is a different term from fixture *difficulty*, and it is the one thing in
  the fixture family that pays.** → **fixtures-and-difficulty**
- **Fixtures matter hugely per match and barely per horizon.** → **fixtures-and-difficulty**
- **The model is already position-dependent by construction.** → **fixtures-and-difficulty**
- **Neither fixture ladder has a shape** — 3 cells, absolute totals, pre-defcon-visibility,
  *unmeasured on the current grid* rather than a measured null. → **fixtures-and-difficulty**
- **The model is not timing fixtures in practice, and there is little to time.** →
  **fixtures-and-difficulty**
- **`fixture_weight` is clamped to [0,1]** — setting 1.4 is silently identical to 1.0.

### The archive

- **The replay was buying players who had already left, and players who had not arrived yet.** →
  **archive-and-data**
- **Extending the archive backwards buys two seasons, and a third on `HOLD` only.** →
  **archive-and-data**
- **Expected goals conceded is reconstructed for the four seasons that carry none.** →
  **archive-and-data**
- **`FPL_NO_XG_REPAIR=1` also disables the xGC reconstruction.** → **archive-and-data**
- **FPL revises team strength mid-season, in waves, and the revisions are outcome-driven.** ⚠️
  Mechanism, not measurement — this does NOT retract b2 = 1.5688, and "0 of 380 difficulties
  changed" licenses nothing. → **archive-and-data**
- **The payloads carry a great deal the code ignores.** → **archive-and-data**
- **The xG harvest left no coverage hole on goals, and left the assist channel about twice as
  exposed as a natively-fed season.** ⚠️ "Goals: closed" rests on the ungated population and is
  narrower than it reads; a naive `xg+xa > 0` gate would be wrong. → **archive-and-data**
- **The exposed-return leak into the conversion fit is sized, and dropping those rows is
  refused.** → **scoring-model**
- **The weekly capture yields nothing this season.** → **archive-and-data**

### The harness

- **The replay's noise is sensitivity, not randomness.** → **harness-and-inference**
- **The noise splits differently on the two metrics.** → **harness-and-inference**
- **Captaincy is 45% of `HOLD`'s residual variance, and removing it is still worse.** →
  **harness-and-inference**
- **A perfect armband is worth 210 points a season, and it is the largest resolvable thing
  here.** → **harness-and-inference**
- **The archive carries what a field average needs.** → **harness-and-inference**

### The weekly transfer decision

- **The transfer charge is a volume brake, not an anti-churn device.** → **transfer-policy**
- **Team value compounds, and the half-of-any-rise selling rule taxes 62% of it.** →
  **transfer-policy**
- **Perfect price timing is too small for this harness to see.** → **transfer-policy**
- **The premium-acquisition over-valuation was never measured.** → **transfer-policy**
- **The policy never banks a transfer at shipped config, and the zero is checked, not
  degenerate.** → **transfer-policy**
- **Reach is not the problem: 97.6% of worth-taking two-move packages are already reachable.** →
  **transfer-policy**
- **The sell side is calibrated; its error is entirely availability.** → **transfer-policy**
- **The transfer path's noise, measured cleanly, is 303 points of spread.** → **transfer-policy**
- **`MinGainHit` 3.0 stands, the hits mostly pay, and nothing ships.** →
  **transfer-policy**
- **The flat `free_transfer_value` ladder resolves nothing — 2.0 ships unchanged,
  measured-and-unresolved rather than untested.** → **transfer-policy**

### Constants

- **The flat prior ships at k=8.** → **constants-and-sweeps**
- **`LeagueShrinkK` is split out from `BlendRateK` and ships at 8** — the replay reading is
  **unresolved, not a measured loss** (POLICY −0.843, t −1.94; HOLD a flat null). →
  **constants-and-sweeps**
- **`BandStrength` has banked cells and still does not resolve — and the arm that DECIDED it is
  still unrun.** → **constants-and-sweeps**
- **The fixture mediator's canary is `band_strength` 2, and `band_ready_weeks` is not a canary
  at all.** → **constants-and-sweeps**
- **`BlendRateK` is banked and nothing resolves.** → **constants-and-sweeps**
- **Calibrate against data, not intuition — and check what a multiplier multiplies before
  calibrating it.** → **constants-and-sweeps**
- **No scoring constant with banked `HOLD` cells is measurably a schedule.** →
  **constants-and-sweeps**
- **`MinutesWeight` ships at 1.0 from 2026-08-25: a judgement to ship neutral, not a
  measurement — this harness cannot locate an optimum on it.** ⚠️ At 1.0
  `MinutesWeightByPosition` is inert. → **constants-and-sweeps**

### Chips

- **All four chips are modelled, and the replay cannot value a wildcard.** → **chips**
- **SPENDING BOTH CHIP SETS AS A MANAGER DOES IS THE LARGEST CHIP EFFECT HERE**: first set spent
  before it expires, wildcard/free hit/bench boost bundled on the second-half calendar, triple
  captain last. **+38.1 a season-path (t 4.51, thr 21.7) on legacy and +38.4 (t 4.43) on measured
  xGC, 6/6 seasons each**, resolving on both. ⚠️ **NOT the only result that agrees across sources
  — bench boost does too (+4.4/+4.5) and resolves on both at a HIGHER t.** Largest, not most
  robust. ⚠️ **How much of it is the first set does NOT resolve**: +11.9 legacy (t 1.91, thr 16.1)
  / +15.6 measured (t 2.22, thr 18.0), positive in 15 of 36 cells — a point estimate, and the
  arithmetic that an expired set is lost is a separate argument. Replays every season under
  today's two-set rules on purpose. `2026-08-25-tworegime/`. → **chips**
- **⚠️ EVERY OTHER CHIP FIGURE DEPENDS ON THE COMMIT AND THE xGC SOURCE — quote none without its
  arm.** Calendar anchoring: +27.0 at 4gw sight (t 5.33) on the legacy reconstruction, RESOLVING;
  the pre-#82 +20.6/t 3.63 is superseded. +26.4 on measured with the SE doubled 5.06→11.41,
  t 2.32, NOT resolving — only the full-sight hindsight arm survives there. ⚠️ **Which arm to
  believe CANNOT be decided by ranking standard errors, and the three-season design that would
  have tried is refuted by its own band.** Run 2026-08-26 in the three seasons where all three
  inputs describe the same football, one commit, one declared variable, 18 cells an arm. **df 2
  puts the 95% band on a ratio of two SD estimates at F(2,2) = [0.16, 6.24]**, so a 2× ratio is
  invisible there and the recorded doubling is **not testable** on that grid — NOT "not
  reproduced". ✅ **What IS estimable is the WITHIN-season spread** (df 15, band [0.59, 1.69]):
  36.16 / 36.07 / 31.09, max ratio **1.16** — **a 2× within-season difference between the inputs
  is EXCLUDED**. ✅ **The per-cell effect is predominantly not a property of the cell**: paired
  differences correlate −0.36 / 0.06 / 0.03 across inputs where the null predicts **+1** (
  differencing removes the shared season path), so **an SE computed on one input does not transfer
  to another**. SE(r) ~0.26 at n 18 — excludes large shared structure, cannot separate 0 from 0.3.
  ⚠️ **The arm means are INDISTINGUISHABLE, not "robust"**: difference-of-differences +5.33/+2.83/
  −2.50 against floors of **36.8 / 61.3 / 34.6**, so the input could be worth up to ~60 a
  season-path and this would not see it. What survives positively is sign-consistency — three arms
  with barely-correlated residuals, all positive at every sight setting. ⚠️ **Do not read per-arm
  t off that grid** (6.33/6.99 at 4gw would "resolve"): either the df-2 SEs rank or they do not,
  and this entry takes the second position. ⚠️ **The reconstruction manufacturing precision stays
  UNPROVEN** — its tighter within-season spread gives paired Levene p 0.43/0.54 and Pitman-Morgan
  (more powerful, correct for correlated variances) p 0.47/0.48. `2026-08-26-xgc-three-inputs/`,
  regenerate with its `arms.R`.
  Free hit: +14.5/t 2.44 pre-`5b970338`, +21.0/t 3.60 after it, +20.7 with SE
  11.81 on measured. **Bench boost is the only component resolving on every arm and commit**
  (+4.4-5.6, LOSO 6/6). ⚠️ `per_path` always — `per_gw` is wrong for an event count and has
  produced two retracted figures. `2026-08-25-anchored-xgcarms/`, `-xgcarms/`. → **chips**
- **The two triple-captain instruments disagree and the replay is no longer the blind one.** Per
  decision the rule delivers +7.95 realised a chip (t 2.92, threshold 7.0); the season-path
  replay of the same rule reads +2.25 against a threshold of 4.19, so it could have resolved that
  and did not. Ownership is the leading and untested explanation. ⚠️ The old "the replay's
  threshold exceeds the effect" defence was a `per_gw` artefact and is withdrawn. → **chips**
- **The wildcard's own value stays unmeasurable here — a wildcard against no wildcard is a tie
  (−7.6, threshold 18.2) because this policy has nothing for it to undo.** → **chips**
- **A bench-boost PLACEMENT contrast is measurable at a threshold of 2.65 — the comparison, not
  the effect.** → **chips**
- **`OptionPricing.CongestionSensitivity = 0` means the DEFAULT of 1.0, not off.** → **chips**
- **The scoring-chip timing `+0.000` is a declared invariance, not a result.** → **chips**
- **Only two of the four chips are *preparation* problems.** → **chips**
- **The two preparation channels overlap by about a third on the chip week and fight over a
  season.** → **chips**
- **Truncating the transfer horizon at a planned wildcard is bounded, and closed on mechanism.** →
  **chips**
- **Triple-captain preparation changed no decision, which is not a measured zero.** → **chips**
- **Do not project the two-set chip rule backwards to buy chip observations.** → **chips**

### The optimiser, the squad, and the money

- **The optimiser was garbage-collection-bound, not compute-bound.** → **optimiser-and-squad**
- **The optimiser's move set** needs N downgrades funding one upgrade; the ranking proxy may
  filter but must never decide. → **optimiser-and-squad**
- **Money is worth what it can still be turned into.** → **optimiser-and-squad**
- **The bench is a hedge and its slots are not interchangeable.** → **optimiser-and-squad**
- **Two elevens exist on purpose.** → **optimiser-and-squad**

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
