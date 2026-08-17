# Banking a transfer, and preparing the squad for a chip

## What was reviewed

Three commits on `bank-transfers-and-prepare-for-chips`, against `origin/main` at `c1f211b`
(merged in, so the branch is 0 behind).

The work began as one thing and became two. The original scope was a **mediator**: per-cell
columns recording whether the transfer-banking rule ever fired, so that the recorded verdict
"the policy never banks a transfer, and banking is not the fix" could be told apart from a
comparison that never ran. Mid-flight the scope widened to building the **capability** itself —
banking as a lever the transfer policy can actually act with, reachable from user config, with
the chip-credit path genuinely exercised and the decision explained in the user-facing output.

Both are in. Concretely:

- `internal/analysis/banking.go` — new. The banking rule, the package valuation, the move
  limit and the chip-credit window, as pure computation. The replay now delegates to all four
  rather than owning them, because the live command needs the same answers and a second
  implementation is this project's signature failure.
- `internal/backtest/simulate.go` — `BankingMediator` on `SimResult`, counted on every arm;
  `decide` returns a per-week contribution; `shouldBank` reports whether it had anything to
  weigh; the double-accrual bug fixed; `Week.Free`'s field comment corrected.
- `internal/backtest/cellcsv_test.go` — five columns, `decision_weeks / consulted_weeks /
  weighed_weeks / banked_weeks / free_at_decision`, as a nesting funnel.
- `internal/config` — `bank_transfers_lookahead` and `prepare_squad_for_chips`.
- `cmd/armband` — the live path: one shared `buildTransferBoard`, the banking decision computed
  and printed, the chip credit applied to the plans, both settings stated in the brief.
- `stats/README.md`, `docs/configuration.md`, `AGENTS.md` — the reader's half.

## Which reviewers ran

Dispatched concurrently against the mediator commit, before the scope widened:

| reviewer | ran | why |
|---|---|---|
| **fpl-code-review** | yes, **twice** | once over the mediator, once over the capability half — the second round returned the ten findings below |
| **fpl-stats-review** | yes | `internal/backtest` — the triage table's harness row |
| **fpl-findings-audit** | yes | the change edits `AGENTS.md`'s transfer bullet |
| **fpl-docs-review** | yes | the change adds a schema section to `stats/README.md` |
| fpl-security-review | no | no credential, cache, config-persistence or network surface is touched. The two new config fields are read-only booleans on an existing struct |
| fpl-run-review | no | no live run was made and nothing wrote config |
| fpl-season-maintenance | no | none of the four hand-maintained lists is touched |

✅ **The capability half is now reviewed too.** The first version of this record flagged it as
unreviewed by an agent; `fpl-code-review` and `fpl-stats-review` then ran over it and returned
sixteen findings between them, every one of which is dispositioned under "Second round" below.
The branch was not mergeable as it stood, and the two most serious findings — an unpinned fix and
deletable live wiring — were each proved by experiment rather than argued.

## Findings, ranked by how misleading the current state was

### 1. The one line joining the mediator to the CSV was untested — proven, not argued

`fpl-code-review` copied the tree, deleted `row.BankingMediator, row.HasBanking = bankingOf(res)`
from `runPolicySweep`, and ran the whole package: `ok`. Both columns would have been blank in
every cell of every sweep for ever, and the sweep would still have printed and banked normally.
That is precisely the silent no-op the block exists to detect, arriving in the block's own
wiring.

**Applied.** `TestTheSweepWritesTheBankingBlock` puts a real `SimResult` through `bankingOf` and
the sink and fails on a blank in any of the five columns.

### 2. The banked branch granted a free transfer FPL does not grant

Found independently by `fpl-code-review` and `fpl-docs-review`, and by reading before either
returned — three readings agreeing. `decide` accrued at the top and the banked branch accrued
again, so a banked week ended two transfers up.

**Applied.** The second increment is gone. Fixed on correctness alone: FPL grants one free
transfer a gameweek and the code granted two.

⚠️ **Two claims made in the first version of this section were FALSE and are withdrawn.**

**"No banked sweep carries `BankLookahead`" — false.** `fpl-stats-review` found
`stats/snapshots/2026-08-13-reach/cells/reach.csv`, variant `bank_lookahead on`, 8 cells. It is
the only such arm in the corpus, and `grep -rn bank_lookahead stats/` returns it immediately —
so the grep behind the original claim was either not run or run for the wrong string. **Verified
by replaying those exact eight cells under the current tree: all eight `policy_points` reproduce
byte-exactly.** The correct statement is therefore stronger than the false one: the arm exists,
it banks in **0 of 8 cells**, and the fix is provably inert on it.

**"It climbed to the ceiling at double speed and could then never bank again" — false, and it
was a hypothesis stated as a mechanism.** The arm never banks, so the buggy branch never
executed and nothing climbed. Withdrawn from `AGENTS.md`, from this record and from the
narrative; the fix needs no sabotage story. The erroneous version is also in commit `86095b3`'s
message, which cannot be corrected after the fact — it is named here instead.

### 2a. The accrual fix was UNPINNED, and every banking test was inert

`fpl-code-review` restored the exact deleted lines and ran the three guards named in the first
version of this record: **all three passed with the bug back.** Verified independently here —
restored the lines, ran them, all green; restored the fix, all green.

The cause is that at shipped config the rule is consulted on all 37 decision weeks of the
replayed season and banks **zero** times, so no test in the repository executed the banked
branch, the early return, or the `BankedWeeks` increment.
`TestABankedWeekAccruesExactlyOneTransfer` iterated a loop whose condition was unreachable.
**The first version of this record claimed that test was the pin. That claim was false and is
withdrawn.**

**Applied.** `bankingArm` sets `MaxHits: 0` — one lever, and the mechanism rather than a fudge:
`MoveLimit` is `free + hits`, so at the shipped one hit the now-arm already reaches two moves and
the later arm three, and only with no hit allowance does the extra free transfer buy the funded
pair the rule exists to reach. Measured: **0 banked weeks at shipped config, 5 with
`MaxHits: 0`**. `TestTheBankingRuleActuallyFires` asserts the branch is reachable and that firing
changes the season; the accrual test now runs on that arm and refuses to run at all if it banks
nothing.

**Verified both directions:** with the bug restored, `TestABankedWeekAccruesExactlyOneTransfer`
now fails with "GW7 made no transfer and the allowance rose from 0 to 2", and
`TestTheBankingRuleActuallyFires` fails too. With the fix, both pass.

One of the reviewer's proposals was **wrong and was corrected rather than applied**: that a
firing rule must make fewer transfers than the greedy arm. It does not — a banked week defers a
move and often spends it on a two-move package next week, measured at 34 against 34. The
assertion is now that the *season the policy played* differs, which is the record's own liveness
idiom.

### 3. `banked_weeks` had no denominator, and a zero was a mixture

Two reviewers, two distinct holes in the briefed two-column design.

`fpl-stats-review`: `shouldBank` has **three** false exits, not two, and the third includes the
degenerate case where nothing cleared the gain floor in either week — so a zero pooled "weighed a
real choice and preferred now" with "there was nothing to weigh". This record already holds that
nothing clears the gain threshold at any price after GW28, so the degenerate case is common.

Both reviewers, separately: the counts scale with cell length (37 decision weeks at a GW1 entry,
12 at GW26) and `decision_weeks` is **not** recoverable as `weeks - 1` on any arm that plays a
wildcard or free hit. Pooling would silently weight the earliest regime three times as heavily.

`fpl-code-review` added a third: with `consulted_weeks` unwritten, "banking on but never
consulted" aliased to "banking off" in the file — defeating the reason `Consulted` is counted
rather than read off config.

**Applied, going beyond the brief.** The block is five columns forming a funnel, `decision >=
consulted >= weighed >= banked`, each step removing one explanation for a zero, plus the mean.
`TestTheBankingFunnelNests` pins the nesting. Each addition is one integer and each was argued to
cost a full sweep to recover later.

**Declined:** `first_banked_gw` and `limit_binding_weeks`, both suggested by `fpl-stats-review`
as third-tier. The first is recoverable in spirit from a per-week dump if ever wanted; the second
belongs to the bullet's other clause ("money binds, not transfer count") and is a different
question. Recorded here so they are not re-proposed blind.

### 4. "A comparison that never ran" is the wrong term of art

`fpl-stats-review` and `fpl-docs-review` independently: the phrase means, in this record, a
setting that never reached its consumer — the `BandStrength` case — and a non-blank `0` refutes
exactly that. The pre-registered sentence therefore contradicted the table two lines above it.

**Applied, without deleting the pre-registration.** The sentence as handed over is kept verbatim
— it is the brief's, not mine to silently rewrite — and the sharper claim is stated immediately
after: a zero everywhere means the arm's points columns are byte-identical to the greedy arm **by
construction**, a confinement rather than a null. `AGENTS.md` carries the corrected form.

`fpl-stats-review` also asked for a dose bar: an arm firing in fewer than four of six seasons is
*unmeasurable* rather than null, since the season-clustered t is capped by construction. Applied
to `stats/README.md` and the writer's comment.

### 5. `TestDiagBanking` measures the wrong quantity under the right label

Both `fpl-code-review` and `fpl-docs-review`. `Week.Free` is assigned after `decide` has spent,
so the histogram labelled "free transfers in hand when each weekly decision was made" is the
post-spend residue. Measured: 0.55 against 1.46 for the quantity the label named, on 2025-26 from
GW1 — a factor of 2.6, and it folds in the opening week, which makes no decision at all. The
`BANK` sweep's own justification cites the wrong figure.

**Applied.** The label now says what it measures, the field comment on `Week.Free` is corrected
and carries the two numbers, and the diagnostic prints the mediator's figure beside the
histogram. Both are kept because both are real and they answer different questions.

### 6. Smaller corrections, all applied

- `withoutChipOracleBlock`'s comment still claimed to be the predecessor header; the banking
  block displaced it. Corrected, matching the demotion note the xPoints helper already carries.
- `AGENTS.md`'s chip-timing bullet enumerated the columns outside `cellMetricColumns` and was
  short by the banking block. Corrected.
- `bankingOf`'s doc justified its gate by naming the variance decomposition, which actually
  builds its rows from a real `Simulate` result. Corrected.
- The blank/0 gloss was attached to both columns and is true of one. Corrected.
- The infeasible-row rule in `stats/README.md` named "the integer columns" as zeroed; the chip
  and banking blocks go blank. Corrected to name the set that really is zeroed.
- `free_at_decision` is **post-treatment** in an arm that banks, and bounds the ceiling guard in
  one direction only (Markov on `free - 1`). Both now stated.

### 7. Declined

- **Making `stats/README.md` the single statement and pointing the two code comments at it**
  (`fpl-docs-review`, prose triplication). Partly applied: `bankingOf` now points at the README
  for the reader's half. The writer's comment keeps its own copy, because a comment at the point
  a column is produced is where the next author will look, and the brief required the
  pre-registered sentence to sit there.
- **Extending the reach map's `outcomeColumns`** (`fpl-code-review`, finding 8). Real gap, and
  the naive fix does not work — the baseline's `banked_weeks` is blank in every cell, so the
  live/dead filter would classify the arm dead. Needs its own design; out of scope here.
- **`floatOrBlank` never returns blank.** Pre-existing naming, no behaviour attached, and the
  gating is done by the surrounding condition. Not touched.

## What was applied without a reviewer asking

The capability half, which no reviewer saw. The judgement calls the coordinator delegated:

- **Both settings default OFF.** The precedent is explicit (`FPL_TEAM_FORM` "built and ships
  off", `ChipCredit` off by default), the recorded verdict still says the policy never banks, and
  no sweep has run under the corrected accrual. Shipping a default on an unmeasured claim is what
  this project has a rule against. Building it and shipping it off satisfies "build it no matter
  what" without asserting a points claim.
- **`shouldBank` needed wiring and one repair, not a rewrite.** Its logic is correct. What it
  lacked was reachability, a reason it could report, and the accrual fix. That is the honest
  finding and it is stated rather than dressed up as more work.
- **The rule moved to `internal/analysis` rather than being copied.** `PreferWaiting`,
  `PackageValue`/`BestPackageValue`, `MoveLimit` and `ChipCreditAt` are now shared, and the
  replay delegates. The enumeration of candidate packages stays per-caller — the replay prices
  sales through a wallet that knows purchase prices, the live command through ranked plans — and
  that split is deliberate: the callers legitimately find candidates by different routes, and
  what they must not disagree about is what a candidate is worth once found.
- **The live command shares one board with the page.** `buildTransferBoard` replaces two
  near-identical inline blocks. Adding banking to only one would have produced a page that
  recommends acting and a command that recommends waiting, from the same config and squad.

## Second round: the capability half, reviewed

The first version of this record flagged the capability half as agent-unreviewed. Two reviews
then ran over commit `86095b3` — `fpl-code-review` returning ten findings, `fpl-stats-review`
six. **Every proposal below was verified before being applied**, by experiment where it was an
empirical claim; two were found wrong as stated and are marked so.

Both reviewers independently confirmed things this record does not repeat: the
`internal/analysis` extraction is behaviour-preserving for the replay, the funnel nests on every
path through `decide`, wildcard and free-hit weeks are correctly excluded from both numerator and
denominator, the config backfill and its justifying comment are right, there is no point-in-time
leakage, the column design is correct and would not be changed, and the placement outside
`cellMetricColumns` is right on two independent arguments.

### Applied

| # | finding | what was done |
|---|---|---|
| 1 | the accrual fix was unpinned and every banking test inert | §2a above — `bankingArm`, `TestTheBankingRuleActuallyFires`, both directions verified |
| 2 | the live wiring could be deleted with the suite green; only the *negative* arm of the switch was pinned | `planFn` seam added so the rule runs without an engine — `TestTheLiveBankingRuleDecidesBothWays` pins both directions. `TestTheTransferBoardWiresTheBankingDecision` scans the join, and was verified to fail when the wiring line is deleted |
| 3 | the command and the page gave opposite recommendations off one board | `transferBoard.outcome()` is now the single decision both renderers switch on. `TestTheCommandAndThePageAgreeOnABankedWeek` pins it. Sharing the board was not sharing the decision |
| 4 | the later arm's chip credit was amortised over the wrong horizon, biasing against banking | `chipCreditNow` takes the horizon as a parameter; the later arm passes `horizon-1`, so `1/(h-1) × (h-1)` cancels as it does in the replay. The window bound moves with it |
| 5 | `AdviseBank` was a second implementation of guards `shouldBank` still carried inline | `analysis.BankGuardFor` is now the one implementation, called by both. `shouldBank` returns `a.Bank, a.Weighed()` |
| 6 | no end-of-season horizon clamp live, so `BankGuardHorizon` was dead code | `liveHorizonFor` clamps to gameweeks remaining. `TestTheLiveHorizonRunsOutWithTheSeason` pins both the guard and its reachability |
| 7 | the now-arm searched a different package space from the recommendation printed beside it | `liveMoveLimit` is used for both. This also stops the command offering plans the allowance cannot execute |
| 8 | neither config field reached the replay | `cmd/armband/backtest.go` now carries `BankLookahead` and both prepare flags |
| 9 | a transient `History` failure silently disabled banking | the reason is captured and printed as a note, as the comment had promised |
| 10 | `BuildPlans` applies neither gate while `BestPackageValue` applies both; `bank_transfers_up_to: 8` made the live ceiling guard unreachable | the "nothing weighed" wording is now scoped to the waiting comparison and says why a plan may still appear below it; `liveBankCeiling` clamps to `fpl.MaxBankedTransfers` |
| A | "no banked sweep carries `BankLookahead`" was false, in three places | §2 above. Replaced with the measured statement, verified by replaying the eight cells |
| B | the "climbed at double speed" narrative was a hypothesis contradicted by the only measurement of it | §2 above. Withdrawn from `AGENTS.md`, this record and the narrative |
| C | record the eight-cell result | `AGENTS.md` and `stats/README.md`: 236 consulted, **169 weighed**, 0 banked, with all four caveats — confinement not null, `WeeklyXI` not shipped, `bank_up_to` pinned at 5 running in favour of banking, original `BANK` cells never banked |
| D | "each step removes one explanation" over-reads — `weighed` removes three at once | stated, with the arithmetic that partly closes the gap |
| E | "the two transfer-banking columns" — it is five | corrected |
| F | mechanism hypothesis: the rule may be near-unreachable by construction | recorded as a hypothesis in both files, **not acted on**. Strengthened by an incidental measurement: `MaxHits: 0` takes banked weeks 0 → 5, which is what the hypothesis predicts, since that is the setting under which the extra transfer is a real capability |

### Verified and found wrong as stated

- **"A firing rule must make fewer transfers than the greedy arm"** (code-review 1, implied).
  Measured 34 against 34. A banked week defers rather than cancels. The assertion was rewritten
  to test that the played season differs.
- **"The arm the null was measured on no longer exists"** (my own prior claim, caught by
  `fpl-stats-review`). It exists and reproduces byte-exactly.

### Declined

- **A `first_banked_gw` column and a `limit_binding_weeks` column** (`fpl-stats-review`,
  third-tier). The first is recoverable from a per-week dump if ever wanted; the second belongs
  to the bullet's other clause, "money binds, not transfer count", and is a different question.
- **Redesigning the rule around finding F.** Explicitly out of scope per the coordinator, and
  right: the hypothesis is checkable off eight banked cells far more cheaply than it is
  guessable, and a redesign before that measurement would be an argmax over specifications.
- **Sweeping the sweep path for finding 8.** `armband backtest` is the user-facing command and is
  fixed. `runPolicySweep` deliberately builds its own `SimConfig` from `sweepConfig` so an arm
  varies one lever against a fixed baseline; wiring user config into it would make every sweep
  depend on the operator's `config.json`, which is the opposite of what a sweep is for.

## What could not be checked on this harness

- **Whether banking is worth points.** No sweep was run, and none can be read off the one banked
  arm: it banks in 0 of 8 cells, so its points columns are byte-identical to the greedy arm by
  construction. That is a confinement, not a null, and nothing here claims a figure.
- **Whether the accrual fix changes any replayed number.** ✅ **Measured, not assumed.** The one
  banked `BankLookahead` arm banks 0 times, so the buggy branch never executed there, and all
  eight of its `policy_points` reproduce byte-exactly under the fix. The fix is inert on every
  banked cell in the corpus. What remains unmeasured is any arm that *does* bank — which today
  means an arm nobody has run.
- **Whether banking is reachable at all at shipped config.** Finding F's hypothesis says it may
  not be, by construction. Recorded, unmeasured, and the named next measurement.
- **The chip-credit path on the live command against a real entry.** The unit tests cover the
  switch and the window; exercising it end-to-end needs a season with a chip planned and an entry
  with picks. `TestChipPreparationIsOffUntilAsked` covers the off arm, which is the shipped one.

## Tests added

| test | what it refuses |
|---|---|
| `TestTheSweepWritesTheBankingBlock` | the join nothing covered — proven green after deleting the wiring line |
| `TestTheBankingFunnelNests` | a counter incremented in the wrong branch, which would silently restore the ambiguous count |
| `TestABankedWeekAccruesExactlyOneTransfer` | the double grant coming back. ⚠️ **Was vacuous** at shipped config and now runs on `bankingArm`; verified to fail with the bug restored |
| `TestTheBankingRuleActuallyFires` | the banked branch never executing — the hole that made every other banking test inert |
| `TestTheBankingMediatorIsCountedOnEveryDecisionWeek` | the rule going unconsulted in a banking arm; deliberately does **not** assert that banking ever fires |
| `TestTheLiveBankingRuleDecidesBothWays` | the live rule pinned only on its refusals, so nothing pinned the switch turning it ON |
| `TestTheCommandAndThePageAgreeOnABankedWeek` | the two renderers reaching opposite recommendations from one board |
| `TestTheTransferBoardWiresTheBankingDecision` | the live wiring being deleted with the suite green; verified to fail when it is |
| `TestTheLiveHorizonRunsOutWithTheSeason` | advising a manager at GW38 to hold a transfer into a gameweek that does not exist |
| `TestAChippedWeekIsNotADecisionWeek` | a wildcard or free-hit week entering the denominator |
| `TestTheBankingColumnsSeparateOffFromNeverFired` | all five columns' gating and the mean's denominator |
| `TestTheBankingBlockIsBeforeTheChipBlockAndCounted` | a column dropped between two counted blocks |
| `TestTheBankingSwitchReachesTheRule` | the config knob not arriving at its consumer |
| `TestChipPreparationIsOffUntilAsked` | the chip credit firing on the chip plan alone |
| `TestTheBankingSettingsSurviveAnOlderConfigFile` | an older config file changing behaviour, and the defaults moving without a `hasKey` migration |
| `TestPackageValueFloorsAtDoingNothing`, `TestBankingActsOnATie`, `TestBankAdviceSeparatesItsThreeRefusals`, `TestTheChipWindowIsWalledByAWildcardAndNotByAFreeHit`, `TestMoveLimitClampsToOneHit` | the shared rule's arithmetic, including which way a tie goes |

## Gate state

`go build ./... && go vet ./... && go test ./...` passes except
`TestSnapshotCoversTheCurrentCode`, which was **already failing on `origin/main`** for
`config.json` before this branch existed — verified by stashing the change and re-running.

⚠️ **Corrected 2026-08-17 by the merging session, and the sentence this replaces was false.** It
read: *"The snapshot series is published by CI and a snapshot directory must not be committed to
satisfy the test on a branch."* Both halves mislead. `.github/workflows/snapshot.yml` does run on
push to `main` and publishes with `gh release create`, but it **never commits** — while
`TestSnapshotCoversTheCurrentCode` reads the newest **committed** snapshot, and keyed snapshots are
still committed, so `NewestKey` finds one and `FPL_SNAPSHOTS_EXTERNAL=1` does not skip. **The
committed series therefore never advances on its own, and the guard fails permanently until someone
commits a new snapshot.** Believing otherwise makes merge-gate condition 4 unsatisfiable by
construction and leaves CI red.

**So a snapshot was regenerated and committed** at `2026-08-17-0d34fa0`, which is the correct action
and is what the test's own failure message instructs. The watched trees really had moved —
`internal/analysis`, `internal/backtest` and `internal/config` — so the guard was firing on real
content, not on a rebase. It was generated from a **private** model CSV verified to hold exactly one
`run_id` before rendering, because `FPL_MODEL_CSV` appends to a path outside the repository and
several sessions run concurrently on this machine; that is the recorded failure where a snapshot
carries another checkout's numbers under this one's name. `model.present` is `true` — a cached
re-run banks `model.present,false` and still exits 0.

The review key was then re-cut with `armband reviewkey -rev HEAD`, because committing the snapshot
moved `stats` and left this record's own key stale.

**Gate state at merge:** build, vet and the whole suite pass; `gofmt -l ./internal ./cmd` empty; all
three leak channels clean including the branch name; working tree clean; **0 behind** both
`origin/main` and `origin/development`, re-checked immediately before merging.
