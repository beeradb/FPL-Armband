# Review — the median consolidation, the xGC walk, and the chip re-measurement

**Commit range**: `b397c93..d55c7ac` (branch `worktree-prior-half-life-on-repaired-xgc`,
rebased onto `origin/main` at `936897e`, fast-forward).

**What the range does.** Consolidates "the middle value" from eleven expressions in nine
files across four packages into one `stats.Median`; extracts `weeklyTotal` in
`xgrepair.go` and uses it in `rebuildXGCAggregates`; re-measures `docs/notes/chips.md`'s
chip table; adds a repo-wide duplication audit to TODO.md.

## The invariant came first, and it is what actually confined the change

Per this skill's opening section, the question before dispatching anyone was *what must
this change not move*. Answer: the replay's scoring. That was checked by running
`TestDiagChipWeekOracle` **twice on one tree**, once under each median convention — the
replay came back byte-identical (53412 `POLICY`, 47105 `HOLD`, 891 moves, `+0.000`
invariance in all 36 cells). The code reviewer independently confirmed the bit-identity of
the `weeklyTotal` extraction over **500,000 random players, 0 mismatches**, which is
stronger evidence than any reading of the diff and is the form this skill asks for.

Two guards were added: `TestTheMiddleValueHasOneImplementation` (source scan, matches the
*idiom* `[len(x)/2]` rather than the word) and
`TestRepairedXGCAggregateMatchesTheWeeklyRows`.

## Reviewers dispatched

| reviewer | why | outcome |
|---|---|---|
| **fpl-code-review** | `internal/analysis` touched; a refactor asserting byte-identical output | 6 findings, 5 applied |
| **fpl-stats-review** | `internal/backtest` + a re-measured recorded figure | 4 claims judged, 1 **refuted**, 2 corrected |
| **fpl-findings-audit** | `CLAUDE.md`, `docs/`, TODO.md | 15 findings, 9 applied |
| fpl-security-review | **skipped** — no `internal/agent`, `internal/fpl`, or config-persistence change. `cmd/fplagent/backtest.go` is a display-only edit | — |
| fpl-run-review | **skipped** — no live run, nothing wrote config | — |
| fpl-season-maintenance | **skipped** — the four lists are *discussed* in TODO.md B5 but not edited | — |

**The gate was not theatre**: every reviewer returned findings, two of them refuted claims
I had committed, and one found a test that did not test what it claimed.

## Findings, ranked by how misleading the state was

### 1. The chip re-measurement's causal story was wrong — REFUTED, applied

I wrote that the grid widening 24→36 explained "four times" more of the movement than the
estimator. I had flagged it as inference. It is worse: **the 36-cell grid contains the
four-season grid**, so the decomposition was available at zero cost and I did not take it.
Verified independently by restricting my own run to the four default seasons:

| | total | tree/data | estimator | grid |
|---|---:|---:|---:|---:|
| timing | −1.594 | **−1.317** | 0.000 | −0.278 |
| playing it at all | −0.364 | −0.392 | +0.271 | −0.243 |
| the rule | +1.131 | **+0.825** | +0.271 | +0.035 |

Grid is 17% of the timing move and 3% of the rule move. The mover is the **data state** —
the recorded table is from `407fac6`, which predates the xGC repair (`7cb769e`). Applied to
`chips.md` as a measured table replacing the asserted sentence.

### 2. `TestWeeklyTotalSumsXGCInGameweekOrder` did not test what it claimed — applied

It exercised `weeklyTotal` in isolation and never ran the rebuild, so the regression it
named (re-inlining `rebuildXGCAggregates` over a map) would not have failed it. The
reviewer also measured its detection rate at ~57% of random orderings. **The real gap was
the opposite of what A5 recorded**: there was no xGC aggregate-vs-rows test at all, where
the xG side has `TestRepairedAggregateMatchesTheWeeklyRows`. Replaced by
`TestRepairedXGCAggregateMatchesTheWeeklyRows`, which runs the rebuild.

### 3. B5's central factual claim was false — applied

I wrote that `config.json` does not carry the domestic-cups list. It does, at line 177, as
`domestic_cup_campaigns`; **I grepped for `domestic_cups`, the Go field name**. That is this
record's own standing rule failing inside an audit that cites it. The replacement finding is
sharper: `[]string` fields are *replaced* by the JSON copy while `map` fields are *merged*
into it, so a Go-side re-derivation silently does nothing for two lists and a deletion from
`config.json` silently does nothing for the other two.

### 4. "42-59" is a retracted threshold — applied

`harness-and-inference.md` retracts it as "a pooled figure and the wrong unit for one arm",
and both `CLAUDE.md` and `chips.md` quoted it for the chip comparison, which has **no
threshold of its own** (six means, no paired difference, no SE).

### 5. "Inside the noise" was the wrong phrase — applied

Timing is **+8.3 pooled, season-clustered SE 1.25, t 6.6** — precisely measured, not
indistinguishable from zero. The defensible claim is that an implemented policy's effect is
too small to validate against a `POLICY` arm MDE. Also: pooling entry points mixes bounds of
different tightness (the oracle argmaxes over 38 weeks at GW1 and 13 at GW26), so **the
season figure is the GW1 column at 13.3**.

### 6. "Three quarters" vs "two thirds" — applied

Verified: the ratio is 0.725 with a season-clustered 95% CI of **[0.637, 0.813]**, which
contains *both* labels. Per entry point it runs 0.616 at GW1 to 0.793 at GW26 — the GW1
column is nearer the label I retired. The fraction is now quoted with its CI, not swapped.

### 7. Counting and naming errors — applied

"Ten expressions across six packages" → **eleven expressions in nine files across four
packages** (six was the *file* count the guard failed on). "An accident at all ten" → nine of
ten. `backfill/run.go` is the guard's exemption, not a migrated site. A stale `UpperMedian`
reference, the renamed test name in three places, and the withdrawn two-estimator design
still closing the audit section as its forward-facing lesson.

### 8. The guard skipped any directory *named* `stats` — applied

Which made its own `internal/stats` check dead code and would have silently exempted a future
package of that name anywhere in the tree — the one place a second median is most likely to
appear. Now matched on the path.

### 9. `median.go` under-reported the accounting — applied

It said one figure moved. Three `model.transfer_error.*.median` rows moved, and the useful
part — that they were *expected* not to and parity was assumed rather than checked — was
absent from the code comment while present in TODO.md.

## Declined, with reasons

- **Re-running the 24-cell arm at `FPL_NO_XGC_REPAIR=1`** to separate the xGC repair from
  the rest of the tree state. Correct and unrun. The decomposition already measured is enough
  to fix the claim, and `chips.md` now says the residual is "a data-state change of
  unattributed composition" rather than naming a cause. Queued rather than done.
- **Computing the chip comparison's own SE** from banked cells. Declined because the cells
  *do not exist* — see the process finding below.
- **Widening `bpsschedule_test.go`'s `%10.0f` median verb.** Real (an even fixture count now
  prints a half-BPS rounded), but the column is quoted nowhere: `docs/notes/scoring-model.md`
  cites that function's *mean* and within-1 percentage. Left, noted here.
- **`CLAUDE.md:197`'s pointer to a completed TODO item.** Pre-existing, outside this range.
- **The `~int64` arm of `Number` being unexercised.** True; nothing approaches 2^53.
- **Several `chips.md` cross-references to the old 24-cell table** (lines 283, 371, 684,
  975). Real staleness, but they belong to sections this range did not otherwise touch and
  fixing them well needs the same care as the table itself. Recorded, not done.

## What could not be checked on this harness

- **Whether the chip table's levels resolve.** The comparison has no SE of its own, and
  building one is blocked by the process finding below rather than by compute.
- **Whether `docs/accuracy.md`'s transfer-error figures (−0.61 / −0.20 / −0.05 / −2.47) are
  on a population this change touches.** They match no value in any snapshot, before or
  after, so they are from an earlier era and their provenance is unestablished. Flagged by
  the code reviewer, not resolved.

## Process finding worth more than any of the above

**The chip table is structurally unreproducible from anything committed.** The
`ORACLECHIP` run banks no `cells/` directory, and the six columns the table is built from
(oracle/median/threshold × 2 chips) **are not in the cells CSV schema at all** — they exist
only on stdout. The only reason this review could check the arithmetic is that two run logs
survived in `/tmp`. CLAUDE.md's own rule fires ("a constant having been *swept* does not mean
its cells were *banked*"), and this is worse than a missing file. → queued in TODO.md.
