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
| **fpl-code-review** | yes | the diff touches the backtest harness and, later, the agent-facing command layer |
| **fpl-stats-review** | yes | `internal/backtest` — the triage table's harness row |
| **fpl-findings-audit** | yes | the change edits `AGENTS.md`'s transfer bullet |
| **fpl-docs-review** | yes | the change adds a schema section to `stats/README.md` |
| fpl-security-review | no | no credential, cache, config-persistence or network surface is touched. The two new config fields are read-only booleans on an existing struct |
| fpl-run-review | no | no live run was made and nothing wrote config |
| fpl-season-maintenance | no | none of the four hand-maintained lists is touched |

⚠️ **The four reviews cover the mediator commit, not the capability commit.** The scope change
landed after they were dispatched, so the analysis extraction, the config fields, the live
wiring and the accrual fix are **unreviewed by an agent**. They are covered by tests named
below, and the accrual fix is itself a reviewer finding acted on. This is stated rather than
papered over.

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

It is worse than generous, it is **self-defeating**: `shouldBank` refuses once the allowance is
at `BankUpTo`, so an arm that manufactured allowance reached the ceiling at double speed and
could then never bank again. Any future banking arm would have measured a rule sabotaging
itself.

**Applied.** The second increment is gone. `TestABankedWeekAccruesExactlyOneTransfer` pins it as
an invariant over the replay: the allowance may never rise by more than one across a gameweek
that spent nothing. Nothing pinned this arithmetic before.

⚠️ **This changes replay behaviour for `BankLookahead` arms only.** That setting shipped off and
no banked sweep carries it, so no recorded figure moves — verified by grepping
`stats/snapshots/*/cells/`. The consequence for the record is that the arm the recorded null was
measured on no longer exists, which is now stated in `AGENTS.md`.

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

## What could not be checked on this harness

- **Whether banking is worth points.** No sweep was run. The columns exist so that one can be
  read; nothing here claims a figure, and `AGENTS.md` says no banked sweep carries them.
- **Whether the accrual fix changes any replayed number.** It cannot on shipped config —
  `BankLookahead` is off — and there is no banked banking sweep to compare against. So the fix is
  correct-by-construction rather than measured, and that is the strongest available claim.
- **The chip-credit path on the live command against a real entry.** The unit tests cover the
  switch and the window; exercising it end-to-end needs a season with a chip planned and an entry
  with picks. `TestChipPreparationIsOffUntilAsked` covers the off arm, which is the shipped one.

## Tests added

| test | what it refuses |
|---|---|
| `TestTheSweepWritesTheBankingBlock` | the join nothing covered — proven green after deleting the wiring line |
| `TestTheBankingFunnelNests` | a counter incremented in the wrong branch, which would silently restore the ambiguous count |
| `TestABankedWeekAccruesExactlyOneTransfer` | the double grant coming back |
| `TestTheBankingMediatorIsCountedOnEveryDecisionWeek` | the rule going unconsulted in a banking arm; deliberately does **not** assert that banking ever fires |
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
`config.json` before this branch existed — verified by stashing the change and re-running. The
snapshot series is published by CI and a snapshot directory must not be committed to satisfy the
test on a branch.
