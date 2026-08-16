# Re-measuring `BandStrength`, and banking its cells

**Range** `9f7ce6a..HEAD` on `rebank-band-strength-zero-on-repaired-archive`.

**What was reviewed.** A re-measurement of `analysis.Weights.BandStrength` — the scale on the 3/14/3
attack/defence band adjustment — which ships at 0 and is **not changed here**. The diff is three new
test files under `internal/backtest` and a banked snapshot at
`stats/snapshots/2026-08-16-band-strength/`. No production code is modified.

The measurement was owed because the shipped zero rested on a pair of absolute season totals in
`bands.go` ("2195 at full strength against 2208") that had never been banked, so they could not be
re-derived — only re-measured — and because the archive backfills moved the data underneath them.

## Reviewers

| reviewer | why | outcome |
|---|---|---|
| **fpl-stats-review** (plan) | the change produces a number; the record's rule is to review the plan, not only the output | ran **before** the sweep; changed the design |
| **fpl-stats-review** (result) | same | ran; changed the verdict |
| **fpl-code-review** | `internal/backtest`, and a defect claim about `internal/analysis` | ran; found a real leak and one vacuous test |
| **fpl-findings-audit** | triage row for `internal/backtest` / `stats` | ran; found the two most serious errors in the branch (findings 0a and 0b) |
| fpl-security-review | no `internal/agent`, `internal/fpl` or config-persistence change | skipped |
| fpl-run-review | no live run wrote config | skipped |
| fpl-season-maintenance | none of the four hand-maintained lists touched | skipped |
| fpl-docs-review / fpl-docs-accuracy | no `docs/` change made — though see the declined item, which is a `docs/` claim | skipped |

**Invariants first, per this skill's opening section.** The quantity this change must not move is the
shipped scoring path, and that is now tested rather than argued:
`TestBandStrengthIsDeterministicAtTheShippedSetting` compares the full `Score` vector across 20
engines. It was **mutation-tested** — forcing `attackBandAdj`'s strength to 1 makes it fail with 8
distinct score vectors — because the first version of it could not fail at all (finding 2).

## Findings

Ranked by how misleading the state was before the fix.

### 0a. The measurement did not re-run the arm that decided the constant — APPLIED as documentation

`bands.go` quotes only the full-strength arm, "2195 against 2208". The commit that created the file,
`bad797c`, records what actually decided it:

> Quarter strength gains +18 on the three tuned seasons but that is the best of six swept values
> against a ±20 noise floor, and out of sample on 2022-23 it loses 12.

So the deciding arm was **s = 0.25**, chosen as an **argmax over six values whose winner sits inside
its own stated ±20 noise floor** — the winner's curse, self-documented at the time and dropped from
the code comment since. This sweep ran 0 / 1 / 2. **s = 0.25 is still unrun**, so the branch
re-measured a neighbouring arm and not the deciding one. Now stated in the diagnostic's header,
including a warning against reading this run as "the recorded verdict was re-tested".

Verified by `git log -1 --format=%B bad797c`.

### 0b. The "2022-23 reverses sign" claim is metric-crossed and wrong — APPLIED

I reported that 2022-23, the season of the original held-out refutation, "reverses sign to +29.4".
The original refutation was a **POLICY** figure (finding 3). Recomputed from the banked cells:

| season | HOLD s=1 | POLICY s=1 | POLICY s=2 |
|---|---|---|---|
| 2022-23 | **+29.4** | **−16.3** | −2.9 |

On the metric the held-out check actually used, 2022-23 keeps the **same sign as the refutation** and
does not reverse. The reversal exists only on a metric that did not exist when the refutation was
written. This was the most quotable wrong claim available from the run and it was in my own commit
message and handover. P4 now carries the correction and names the failure mode.

### 0c. The cell-sign count was wrong — APPLIED

I reported "15 of 36 cells disagree with the mean's sign". Recomputed: **15 positive, 10 negative,
11 exactly zero**. Fifteen cells *agree*. The concentration screen's `pos/n 15/36` and its
`MOST CELLS DISAGREE` flag count the eleven zeros in the denominator, which is the screen's
convention but is not a statement that 21 cells disagree — among the 25 cells that moved, 60% share
the mean's sign. Seven cells are byte-identical on the entire `HOLD` path (`hold_points`,
`hold_xpoints` and `squad_hash` all unchanged), four of them GW1 entries where the fifteen cannot
move by construction. Per the record's own rule, a byte-identical cell is not a vote.

### 1. A pre-season point-in-time leak, found by the code review — APPLIED as documentation, NOT fixed

`PreSeasonWith` (`internal/backtest/replay.go:125`) returns `cur.Fixtures` unfiltered:
`playedFixtures` is never called when `through <= 0`. Every archived season carries
`"finished": false` on all 380 fixtures **and** a scoreline on all 380. `teamBands` is safe because it
tests `f.Finished`; `buildTeamRates` (`internal/analysis/teamstrength.go:112`) tests only that the
scores are non-nil, **under a comment asserting `playedFixtures` has already stripped them**. So at
cutoff 0 that function holds the whole season's results.

Verified by the reviewer: with `FPL_MAGNITUDE=1`, `FixtureMultipliersFor` at cutoff 0 is bit-identical
to cutoff 38. Reachable only through `magnitudeAttack`/`magnitudeDefence`, both behind
`FPL_MAGNITUDE`, so it is opt-in rather than shipped — but **any `FPL_MAGNITUDE` figure that included
GW1 entry cells is contaminated**.

This is the recorded "anything reading fixture results must be gated by gameweek" bug in a second
function. It survived because `TestPointInTimeHidesFutureResults` sweeps `through` over
{1, 5, 12, 20, 38} and **has never tested 0**.

**Not fixed here** — it is outside a measurement branch, and the fix has to decide whether
`PreSeasonWith` should filter or `buildTeamRates` should gate. Recorded in
`bandstrength_test.go`'s header, including an explicit "do not read this test as covering it".

### 2. `TestBandStrengthIsDeterministicAtTheShippedSetting` was vacuous — APPLIED

The reviewer deleted both `strength <= 0` short circuits from `bands.go` and the test **still passed**.
At strength 0, `1 + target*0` and `1 - avoid*0` are exactly 1 whatever band a club is in, so the guard
is unreachable by that route. Worse, the compared quantity was `attackMultiplier(3) * 1` — a pure
function of two constants, invariant to the engine, the archive and the cutoff, and passing against an
empty archive.

This is precisely the failure mode the branch exists to warn about, committed by the branch.
Rewritten to compare every player's `Score` across 20 engines, with an empty-pool guard, and
mutation-tested as above.

### 3. "It was a POLICY figure" was under-claimed as an inference — APPLIED

The diagnostic hedged this as "an inference from the shared baseline, not something recovered from a
run". The audit showed it is **established by the code that produced the figure**. At `bad797c`,
`cmd/fplagent/backtest.go:103` applies `FPL_BAND_STRENGTH` to `base`, the `SimConfig` handed to the
three transfer policies, while line 141 builds the hold row from a fresh
`backtest.SimConfig{Weights: cfg.Weights}` that never sees `base`. The hold row was therefore
**byte-identical across that whole sweep by construction** — the byte-identical null the standing
rules warn about — so only the POLICY rows could move. Verified by reading both lines at that commit.

The shared-2208 argument still holds but is the weakest of the three supports and should not be the
one quoted. Also confirmed: the "twelve points on 2022-23" figure is cited nowhere as settled — it
appears once, in `stats/snapshots/2026-08-13-aa95f75/FINDINGS.md`, already marked void.

### 4. The pre-registration quoted a figure that does not reproduce — APPLIED

P0 recorded "moves 469 of 503 scored players". Six runs gave 469, 494, 494, 469, 469, 494 — the
arrival check is itself subject to the defect this branch documents, because at cutoff 19 the
third-best attack is a two-way tie. The assertion (`moved > 0`) was always safe; the number was not.
Both the pre-registration and the `t.Logf` now say the count varies and why.

### 5. Sampled counts written down as properties — APPLIED

The cutoff profile table quoted "GW5 10, GW6 34" as facts; the reviewer's run gave 11 and 30. Those
two are draws from a large space. Replaced with the **reachable** count, which is deterministic: the
product of the boundary tie multiplicities, 144 at cutoff 6.

### 6. The tie sentence named the wrong band — APPLIED

Said "four clubs tie at the third-worst defence and three at the third-worst attack". Measured: the
third-worst attack is a 3-way tie, the third-worst **defence** a 2-way tie, and the 4-way ties are at
the *best* ends. Corrected, and the explanation of *why only boundary ties matter* added, since that
is what makes cutoff 7 perfectly stable despite having interior ties.

### 7. "Nothing shipped is affected" was too strong — APPLIED

`applyWeightOverrides` runs in `cmd/fplagent/main.go` before command dispatch, so `FPL_WEIGHT=band=1`
makes a **live** `fplagent review` non-deterministic, and `FPL_BAND_STRENGTH` does the same for
`fplagent backtest`. The correct statement is "no **default** configuration reaches it". Reworded.

### 8. "Unmeasurable on this harness" was an over-claim — APPLIED

The stats review refuted the canary's sizing argument on its own arithmetic. The clean-sheet precedent
licenses "unmeasurable" because its canary was a ~4x strict amplification with a predicted sign. Here
the dose ratio is 2x, so a canary failing a threshold of 27 bounds the candidate only at
|e| < 13.5 a season — which lands *on* the observed +13.6 and excludes nothing. And linearity is
refuted by the run itself: the canary came back **smaller and opposite**, not larger. A non-resolving
canary of that kind buys **"unresolved twice"**, not "unmeasurable". The over-claim and the reason it
fails are now written into the diagnostic's header so it is not made again.

### 9. Denominators on the reproduction failure — APPLIED

"14 of 36 cells" is a union over all columns and is carried by the POLICY pair. On `hold`, the
deciding column, it is **3 of 36**, worth 0.7 points a season — about a tenth of that contrast's own
CR2 standard error. The per-column table is now in the file, with the point that unmodelled jitter can
only widen an interval and therefore **cannot overturn a null**.

## What was declined

- **Fixing the `teamBands` tie-break.** Correct and known — break ties on club id; `sort.SliceStable`
  alone does not help because the input order is already random. Declined because fixing it changes
  what the swept arms compute and would orphan the cells this branch exists to bank. It is the
  **third** instance of the map-ordering class here (after `Optimize` and `newTeamFormIndex`) and the
  first left unpinned, which is the cost of the deferral and is stated rather than hidden.
- **Fixing the pre-season leak (finding 1).** Same reason, plus it needs a design decision about which
  of the two functions is the right place.
- **Marking the retraction in `internal/analysis/bands.go` and `docs/configuration.md:110`.** Both
  still present the 2195/2208 comparison as the live verdict, and `bands.go` additionally omits the
  argmax and the ±20 self-caveat that the originating commit recorded. An edit to `bands.go`'s comment
  was written and then reverted outside this session; it has not been re-applied. **This is the
  highest-value remaining item**, because the record's convention is to mark a correction where a
  reader meets it, and because finding 0a means the comment currently understates how weak the
  original verdict was. `internal/analysis/squad.go`'s unstable-sort inventory *was* corrected here,
  since it actively misled by listing `funding.go` and `swaps.go` as the whole set.
- **Adding the new cells to `COMMITTED` in `concentration_screen.R` / `schedule_screen.R`.** The two
  screens deliberately share one curated list. `schedule_screen.R` would read arms 0/1/2 as a numeric
  ladder and test a question this run did not ask, adding multiplicity. Left for the owner of that
  list.
- **Editing `CLAUDE.md`.** Explicitly out of scope for this branch. The stats review supplied exact
  proposed text for three entries — an amendment to the fixture-family closed line, a new
  "already bitten" bullet for `teamBands`, and a general rule that a canary sizes an instrument only
  if it scales — and those are handed to whoever owns that file rather than applied.
- **Writing `FINDINGS.md` into the snapshot.** Both reviewers asked for it and every comparable
  snapshot has one. The session harness refuses to write report files; the prose exists in the commit
  body and the handover instead. **This is a real gap in the bank**, not a decision that it is
  unnecessary.

## What could not be checked on this harness

- **Whether `BandStrength` has a true effect.** +13.6 a season sits below this comparison's own MDE of
  24, so "unresolved" is the expected reading for a real effect of that size and is not evidence
  against one. The record's own rule applies: a null is a tie, not a refutation.
- **Whether the recorded −13 was ever right.** It has no standard error, its level comes from an era
  four contamination events moved by up to 115 points a season, and its metric is not recoverable
  because the cells were never banked. The new 95% interval [−4.4, +31.5] formally excludes −13, but a
  point estimate with no denominator cannot be rejected — so the old figure is retired **on
  provenance, not on points**.
- **Whether the GW26 concentration is mechanism or artefact.** It survives on raw points (65% from
  8.5% of gameweeks), so it is not merely a `per_gw` length-bias artefact; but the entry-point profile
  is non-monotone, with GW21 the most negative, which is not what the "later entry knows more"
  mechanism predicts. Unresolved either way.
- **The size of the jitter the tie-break defect adds.** One repeat is a single draw, not a variance
  estimate.

## Verification note

Per this skill's warning that a reviewer's report is a set of proposals: findings 1, 2, 3, 4 and 5
were each re-checked directly before being applied — the leak by reading `replay.go:125` and
`teamstrength.go:112` and the guard's cutoff list; the vacuity by mutation-testing the rewritten
guard to a genuine failure; the irreproducible count by re-running the arrival test, which printed
494 where the file had said 469.
