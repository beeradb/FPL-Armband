# Review record — the oracle harness and catalogue

**Commit range reviewed:** `3095fe5..b1af338` (27 commits, branch `worktree-oracle-harness`).

Implements `docs/oracle-design.md`, which was a written design with nothing built. Adds the shared
harness, six oracles, and the corrections that came out of measuring them.

## Reviewers dispatched

Triage was against the review-gate table, on a diff of 29 files in `internal/backtest`, the record,
`stats/`, and one line in `cmd/`.

| reviewer | why | outcome |
|---|---|---|
| **fpl-stats-review** | `internal/backtest` and `stats/*.R` changed; six new measured figures | 7 findings, 2 of them corrections to headline numbers |
| **fpl-findings-audit** | the record changed, and several load-bearing figures moved | 14 findings, mostly stale citations of superseded numbers |
| **fpl-code-review** | three refactors asserting byte-identical output | 9 findings, 1 provenance defect, 2 missing guards |

A second round (stats + findings audit) ran against the lineups/minutes split before it landed.

**Skipped, with reasons.** `fpl-security-review` — nothing in `internal/agent`, `internal/fpl` or
config persistence; `wallet.go` is replay-only bookkeeping. `fpl-run-review` — no live run, no config
written. `fpl-season-maintenance` — the four hand-maintained lists are untouched.

## Invariants first, per the gate's own instruction

The gate says to ask what quantity a change must *not* move before dispatching anyone. That is most
of why this branch is trustworthy, and the invariances did more work than the reviewers:

- **Chip week**: every collected metric byte-identical to the baseline in all 24 cells. The design
  chose it to go first precisely because its invariance is total, and it held exactly.
- **Armband**: `hold_nocap`, `hold_fixedcap`, transfer count and hits identical in all 24 cells.
  This later turned out to be the *correct* defence of its t = 20.43, in place of the mediator
  argument the commit had offered.
- **Price and gate**: all three held rungs exactly `+0.000`; baseline arm byte-identical at
  POLICY 35871 / HOLD 31987 / 595 moves / 57 hits — the control that proved the sell-side fix stayed
  inside the oracle branch.
- **Minutes**: Tier 1 field diff over four season pairs × 39 cutoffs, declaring *no* bootstrap
  fields, which is the strong claim rather than a shrug.

## Findings applied, ranked by how misleading the state was

1. **The vice-captain accounting was inverted.** `viceCaptainFallback` defaults on, so both arms bank
   the bonus and the armband's 210 is *net* of the vice rule, not inclusive. Four places said
   otherwise. Corrected; the operational conclusion (it bounds captain and vice jointly, so a vice
   refinement must not be bounded separately) is kept.
2. **`OracleMinutes` was the wrong estimand** — a whole-remainder-of-season average, unable to
   express a trajectory. Split into `OracleLineups` (selection) and `OracleMinutes` (quantity given
   selection). **The ≈183 is retracted**: measured per gameweek it is ≈73 and ≈47, neither resolving.
   The `HOLD`/`POLICY` reversal previously reported is withdrawn (diff-in-diff t = 0.21).
3. **`TestDiagVarianceDecomposition` stamped its means rows with a hardcoded inert oracle**, so an
   oracled run wrote cells saying `info:prices` and means saying `-`, bypassing every oracle guard.
   Fixed, plus `TestEveryMeansRowIsStampedFromWhatRan` so it cannot recur.
4. **The price oracle bought low and sold low.** The search was quoted `bestSellPrice`, the wallet
   credited `bestBuyPrice`. Invisible from the un-oracled path, so no sweep could have caught it.
   Every price figure ever recorded came from that arm. Corrected to a genuine upper bound.
5. **The +16/+5.6 discrepancy was a change of estimator, not contamination.** The record blamed the
   doubles-counting fix; the earlier estimator still returns +13.6 on current code. That explanation
   is retracted — a correction withdrawing a correction, recorded as such.
6. **The gate bound is a floor, not a ceiling** (raw-points proxy, first-rejection termination,
   inherited horizon), and its "two thirds" was untraceable arithmetic. Restated at 89%, with the
   closure argued from the ratio so it survives at any reading of the bound.
7. **Stale citations across seven files**, including a `+322` in a summary table I had missed while
   correcting the prose above it, one printed at runtime by a diagnostic, and one justifying a whole
   capture programme in `internal/capture`.
8. **Missing guards added**: a counting assertion for `.Priors` (the third information channel, which
   had already silently diverged once); a `MustMove` liveness tier; a source-scan test for the
   `internal/analysis` boundary; a `go/parser` check that `recentIndex`'s body reaches the builder.
9. **Archive defect**: rows are omitted entirely when a player is out of the squad — ~3,000 of 30,000
   club-gameweeks a season, disproportionately the injured. The oracle divided by the player's own
   rows rather than his club's fixtures. Now counted from the club calendar.

## Declined, and why

- **"Clears by 6, not about fifty"** (stats review, on the minutes threshold). Partly rejected: the
  inline figure is the *significance* threshold and reproduces `mde.csv` exactly, which is the
  convention every other threshold in the record uses. The 6-point margin is the 80%-power MDE, a
  different quantity. Both are now stated rather than one replacing the other. The same conflation
  was found and fixed in the gate section.
- **A blanket "every oracle must move something" rule.** It would fail `AxisChipWeek`, whose total
  invariance is the point. `MustMove` is a declared tier instead, and the price oracle's mediator is
  `moves` rather than its headline — proving an arm live from the number it exists to report is
  circular.
- **A guard firing on "oracle" in `internal/analysis` test files.** `optimizerdiff_test.go` uses the
  word correctly in the golden-reference sense, and a guard that fires on that gets deleted.

## Could not be checked on this harness

- **The armband and gate cells files live in `/tmp`**, so those two figures exist in the repository
  as prose in commit messages. Giving them a durable artifact needs a re-run. Deferred, not fixed.
- **Whether the information oracles are truly small or merely invisible.** Four seasons give three
  degrees of freedom; the entry-point axis adds cells but no df, budget jitter was checked and
  rejected as too correlated, and the six-season grid scales the bar by only 0.66. Unresolvable
  rather than unresolved.
- **The reachable half of team news.** The replay has no judgement layer and never will, so the gap
  the oracles size is the one component this instrument structurally cannot measure.

## Queued

The substitute-return-by-position diagnostic, with its four traps written down in `TODO.md`. And a
second armband arm picking the eleven's highest season-mean scorer, which would split "knowing who is
good" from "knowing which week" — the second being unreachable by any rule.
