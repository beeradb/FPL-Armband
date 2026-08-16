# Review: chip preparation — `ChipCredit` and the two replay switches

**Commit range reviewed:** `3decb95..a05f7e0` (five commits: `9bb27c3`, `ca6540b`, `17260be`,
`2ebe6dd`, `a05f7e0`). Fixes applied at `5b8dd2f`; the write-up corrections at `594dcfc` and the
re-taken snapshot at `a6682de` follow from the findings below, so this record is committed last and
covers them.

**What the change is.** The transfer objective could not express two of the four chips.
`analysis.XIValue` took a squad and nothing else, so a bench worth boosting priced at zero and the
triple captain had no expression anywhere; `SimConfig.anticipate` already called `ApplyChipPlan` and
discarded the bench weight it returned. `analysis.ChipCredit` on `SquadState.Chip` closes it, behind
`SimConfig.PrepareBenchBoost` / `PrepareTripleCaptain`, both off by default.
`TestDiagChipPreparation` measured it on 36 cells.

## Reviewers run, and the triage

| reviewer | why |
|---|---|
| **fpl-code-review** | the diff touches `internal/analysis` (scoring) and `internal/backtest` (harness) |
| **fpl-stats-review** | a new points claim on the replay grid |
| **fpl-findings-audit** | `CLAUDE.md`, `docs/notes/chips.md` and `TODO.md` all gained claims |

Skipped, with reasons: **fpl-security-review** — no `internal/agent`, `internal/fpl` or config
persistence in the range; **fpl-run-review** — no live run wrote config; **fpl-season-maintenance** —
none of the four hand-maintained lists is touched.

**Invariants first, as the gate demands.** The quantity this change must not move is `HOLD`: the
credit lives on `SquadState`, which only the transfer searches read. That was tested three ways
before any reviewer was dispatched — `TestTheCreditsDoNotReachTheSquadBuilder` at the unit level,
the sweep's own HOLD column across all three captaincy rungs in 36 cells, and an accuracy snapshot
that moved no model figure at all. All three held. The reviewers were then asked for what invariants
cannot catch, and between them they found one wrong constant-of-mechanism, one latent bug, one
vacuous test and a borrowed threshold, so the dispatch paid for itself.

## Findings, ranked by how misleading the state was

### 1. The bench credit priced bench *quality*, not the *double* — APPLIED

`PlayerMetrics.FixtureLoad` is matches per gameweek **averaged over the horizon**, so on a
five-week horizon a club playing twice in the boost week arrives at 6/5 = 1.2 rather than 2. The
credit multiplied `Score` directly and therefore priced the double at a fifth of its strength. The
arm bought a better bench, not a doubling one — which is not the mechanism a bench boost pays for,
and is not what "build toward the double" means.

Verified against `internal/analysis/metrics.go` before applying. Fixed by `ChipCredit.WeekLoad`,
carrying the chip week's own per-club fixture counts from the new `Engine.FixtureCountsIn`, with the
bench term stripping the horizon average back out. **Every points figure from the first sweep is
superseded by this and the sweep is re-running.** Found by fpl-findings-audit; it is the highest-value
finding of the pass because no amount of re-measuring the arm would have exposed it.

### 2. The credit reached past an intervening wildcard — APPLIED

`playWildcard` replaces all fifteen, so a credit reaching past one spends free transfers on a squad
about to be torn up. `analysis.EffectiveHorizon` already stops at a wildcard on the opening-squad
path, so this was one quantity computed two ways. Unreachable in the sweep as run — no arm plays a
wildcard — but it is precisely the next arm, which makes it worth fixing before rather than after.
`TestAWildcardEndsThePreparationWindow` pins it, including that a free hit is deliberately *not* a
barrier. Found by fpl-code-review.

### 3. The prune exemption was unnecessary, and its test was vacuous — APPLIED

`RankSwaps` skips candidates scoring no more than the man they replace. The first version exempted
that prune when the bench credit was on, arguing the monotonicity might fail. The objective is in
fact monotone in every player's score for any credit, so the prune is exact — established
analytically and by 200,000 randomised same-position downgrades across six credit settings, none of
which raised the objective. The exemption also left the two searches inconsistent: the same prune in
`RankPairs` was never exempted.

Worse, the test guarding it **passed with the prune fully restored** — confirmed by mutation. Its
fixture lowered a bench forward and offered a midfielder, so the prune never fired on the slot that
produced the gain, and the case it existed for was never constructed. Replaced with
`TestACheaperArrivalCannotRaiseTheObjective`, which asserts the property directly over every slot,
five drop sizes and six credits. Found by fpl-code-review.

### 4. The threshold was borrowed, and the file retracts that exact move elsewhere — APPLIED

The write-up quoted "a POLICY threshold near 70", which is the four-season median across 23 *other*
comparisons. `CLAUDE.md`'s own headline rule is that a detection threshold belongs to a *comparison*,
and `docs/notes/chips.md` three sections down retracts a borrowed threshold that was four to six
times too high. This comparison's own threshold, from its own cells: **13.3 season-clustered, 16.8
fixed block**. So +9.7 falls short by about 1.5×, not by a factor of seven — the difference between
"closed" and "one grid widening from resolvable". Verified by re-running `variance_components.R`.
Raised independently by both fpl-stats-review and fpl-findings-audit.

### 5. The season-versus-chip-week decomposition was a unit error — APPLIED

"+9.7 a season is more than the +2.8 the boost week gained, so most of it is path divergence"
compares a per-gameweek mean × 38 against a single week's raw points. Converted properly the boost
week is 0.125 pts/gw, **a third to a half** of the measured 0.255. The remainder is not
distinguishable from zero and the design cannot attribute it. Found by fpl-stats-review.

### 6. "Every chip figure is a floor by about 30%" — APPLIED

Refuted by the run's own triple-captain row, which moved its chip's week by zero, and transported
across grids: the table it corrected is 24 cells on four seasons at three other placements. Narrowed
to a bench-boost direction claim with the size confined to its own grid and placement. Found by both
fpl-stats-review and fpl-findings-audit.

### 7. The triple-captain null's denominator and its p — APPLIED

Quoted as "all 36 cells"; only **23** place the chip, so 13 are cells where the intervention could
not run rather than cells that voted — the record's own "a byte-identical season under an
intervention is not a tie". And the `p = 1.000` printed for it is a convention, not an inference: the
estimator is degenerate, so 36 cells bound the *flip rate* (rule of three, ~8%) and say nothing about
the value of a flip. Both corrected; the p is no longer quoted. Found by fpl-stats-review, with the
denominator independently found by fpl-findings-audit.

### 8. Smaller applied items

- **No guard on `Unified` plus a preparation switch.** `unifiedDecide` values squads through
  `Optimize` and only gates on `XIValue`, so the credit never reaches its objective — and
  `FPL_UNIFIED_TRANSFERS` left exported from another sweep would silently turn both arms into
  byte-identical nulls reading as "preparation does nothing". Now a returned error, matching the
  refusals `Simulate` already makes for oracles and chip plans.
- **The "shrunk armband" comment claimed a dormant protection.** `captainShrink` ships at 1.0, so the
  credit is one copy of the *raw* armband and is exactly `Captain × max(XI score)`. Comment corrected;
  the mechanism it reveals is now recorded beside the null, since it explains it.
- **`WildcardIgnoresBoost`** added as the per-cell form of `FPL_WC_IGNORES_BOOST`: a sweep arm cannot
  vary a package-level global without leaving it set for every arm after it.
- **The liveness witness hard-coded `horizon = 5`** — a diagnostic carrying its own copy of the thing
  it checks. Now derived from the cell's own config.
- **Section placement.** The new results section had been inserted mid-way through the
  `AnticipateChips` post-mortem, silently re-filing that retraction's tables under it. Moved.
- **Entry-point and per-season breakdowns, and the `per_path` scale**, all now reported: this file
  requires the entry-point column of a chip measurement after a retraction caused by exactly its
  absence.
- **Holm multiplicity.** The repo printed 0.363 (family of three, containing a byte-identical
  duplicate arm) and 0.242 (family of two). 0.242 is now the quoted figure and the discrepancy is
  named.

## Declined, with reasons

- **"The two negative seasons are negative because 2020-21 and 2021-22 carry no clean-sheet term."**
  fpl-findings-audit built this on a comment in `harness_test.go` that **predates the xGC backfill**.
  The repair covers exactly those two seasons, is on by default, and was not disabled for this run,
  so the premise is stale. The *observation* is kept — they are the two seasons whose xGC is
  reconstructed rather than native, and the repair's own size is an open TODO item — but recorded as
  a flag rather than as the audit's mechanism. This is the pass's one instance of the standing rule
  that a reviewer's report is a proposal, not a finding.
- **Committing the cells file into the repository.** Correct in principle and deferred: the run it
  would preserve is superseded by finding 1, so the cells to commit are the re-run's. Carried into
  the re-run rather than dropped.
- **Re-stamping the provenance.** Same reason. The first sweep ran from a dirty tree at the base
  commit, which is recorded in the note as a limitation rather than repaired, because the artefact
  is being replaced.

## What could not be checked on this harness

- **Whether the 30% floor survives a different placement rule.** The chip-oracle table is a different
  grid, bank and placement; reconciling them needs a run neither reviewer could start.
- **The size of the corrected credit.** Finding 1 changes what the arm buys, so the re-run is the
  only way to know whether the +2.8 mechanism grows. Expected to grow, unmeasured.
- **Whether the season effect is real.** +9.7 against a 13-17 threshold is unresolved by design, and
  the design projection on these cells says 8 seasons × 6 starts would resolve 20 a season. That is
  a wider archive, not a better analysis — unmeasured rather than unmeasurable.
- **The wildcard-into-boost sequence.** Measured only by `TestDiagChipSequence`, which compares
  season totals down one path — the noisiest method this record carries.
  `TestDiagChipSequencePaired` is the paired-grid version and was running when this record was
  written.
