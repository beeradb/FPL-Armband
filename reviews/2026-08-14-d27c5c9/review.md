# Review — A3 (the defcon prior projection), A4, and B1-B4

**Commit range**: `e1d7ca3..d27c5c9` on `worktree-prior-defcon-projection`, branched from
`main` and a fast-forward onto it.

**What the range does.** Closes six items of the duplication audit: the archive→`PriorPlayer`
projection (A3, two defects), `xiValue` (A4), and four mechanical duplications (B1-B4).

## The invariant came first, and it was measured twice

Per the skill's opening section: the quantity this must not move is the replay's output.
`TestDiagChipWeekOracle` was run on this branch and on `main`, and again after the review
fixes — **every line identical across all 36 cells**, both times. The accuracy snapshot moves
**nothing** across 560 model figures.

⚠️ **That invariance has no power over A3's fix and must not be read as validating it.** It
holds because no archived season pair has a defcon-carrying prior, which is the same reason
the fix was needed. It is an acceptance test for collateral damage.

The fix's own validation is different and stronger: `Player.DefCon` reconciles against
`players_raw.csv` — a file the derivation never reads — at **841 of 841** elements, residual
identically zero.

## Reviewers dispatched

| reviewer | why | outcome |
|---|---|---|
| **fpl-stats-review** | model change; ran on the **plan**, before any code | plan refuted and rewritten: 4 sites → 9, one defect → two, `NoExpected` → `NoXG`/`NoXGC` |
| **fpl-code-review** | `internal/analysis` + a refactor asserting bit-identity | 6 findings, **1 a live regression I introduced** |
| **fpl-findings-audit** | `CLAUDE.md`, `docs/`, TODO.md | 11 findings, 8 applied |
| fpl-security-review | **skipped** — no `internal/agent` behaviour, `internal/fpl`, or config-persistence change | — |
| fpl-run-review | **skipped** — no live run | — |
| fpl-season-maintenance | **skipped** — the four lists are untouched | — |

**Not theatre**: the plan review changed the design before code existed, and the code review
found a regression that the existing guard was concealing.

## Findings, ranked

### 1. B1 was a live regression, and its guard passed throughout — CONFIRMED, fixed

`cmd/fplagent`'s `seasonBefore` had a four-digit branch because `priorSeasonName` emits
`"2025-2026"`; `backtest.PriorSeasonName` always answers `"YYYY-YY"`. Delegating straight to it
made the walk return `"2025-2026"` then `"2024-25"` — verbatim the failure its own surviving
comment records: every older season a 404, the blend degrading silently.

**`TestSeasonBeforeFormatting` passed the whole time**, because it re-implemented the old
arithmetic locally under "the arithmetic is the part worth pinning, not the location". That is
the standing rule about a diagnostic carrying its own copy, and this change turned the copy
from redundant into concealing. The test moved to `cmd/fplagent`, where `seasonBefore` is
package-private and the only way to pin it is to call it, plus a walk test for the property
that broke.

### 2. A3 reached six of eight copies — CONFIRMED, fixed

The findings audit caught `cmd/priorblend`'s `seasonStats`: the replay and the experiment
binary disagreed about capability, **the defect A3 exists to close, re-created inside it**. The
code review then caught two more — `channelPriors`' two rescale arms dropped `DefCon` while
their control now carries it.

Both now go through `backtest.PriorStatsFrom`, which takes the `Capability` as a **required
argument**, so the omission is unspellable rather than merely fixed.

### 3. The recency arm left `DefCon` un-rebased — CONFIRMED, fixed

`newPriorIndexRecent` re-expresses every rate against the rewritten minutes; `DefCon` stayed at
the flat season total, so downstream it is divided by a smaller denominator — inflated by
`fullMinutes/recencyMinutes`, up to ~3× for a player who lost his place in the spring. In the
arm this record calls **actively swept**, so it would have been measured and attributed to the
half-life. ⚠️ **My own new test had recorded this as a design property**, which is the worse
half of the finding. `TestRecencyArmRebasesDefConWithTheOtherRates` now pins the correct
behaviour.

### 4. A recorded figure is now data-state-bound — CONFIRMED, recorded not hidden

Completing Defect 2 in `cmd/priorblend` moves the `FPL_NO_XG_REPAIR=1` arm of the
`prior_half_life` ordering replication: unrepaired seasons leave the xG denominator alone
instead of contributing measured zeros. **The recorded `p = 0.031` sign test is a figure from a
retired data state and must be re-run before it is quoted.** The repaired arm is untouched, and
no banked replay figure is exposed — every `weights.prior_half_life` in the snapshots reads 0.

Recorded in TODO.md and `recency-and-priors.md`. Fixing the code and marking the figure was
chosen over leaving a known divergence to preserve a number measured under it.

### 5. Three record claims described a mechanism that no longer exists — CONFIRMED, corrected

`CLAUDE.md`, `docs/notes/scoring-model.md` and `engineat.go` all said the replay's prior index
carries no defcon. ⚠️ **The subtle half**: the sections' *second* defect — that `blendRates` has
no capability gate — is **untouched and still live at shipped config**, so the danger was a
reader marking the whole section superseded. Each is amended in place to separate the fixed
mechanism from the unchanged consequence.

### 6. The point-in-time argument had no behavioural pin — CONFIRMED, added

`rebuildXGAggregates` makes the identical safety argument and backs it with a test;
`Player.DefCon` had none. `TestDefConDoesNotLeakIntoAPlayedSeason` asserts a GW5 view sees 25
of the season's 100.

## Declined, with reasons

- **`backfill.startYear` folded into `PriorSeasonName`.** Named in B1's body, still outstanding.
  It returns an `int` for a different purpose; sharing the scan is a separate small change.
- **Threading `DefconScoredIn` through the prior projection**, which the plan called for.
  `PriorFrom` reports the total and callers gate — pricing on `DefconScoredIn`, blending on
  `NoDefCon`. The derived total is already zero wherever the column is absent, so the gate
  would be redundant and would put a **rules** predicate on a **data** path. ⚠️ It rests on an
  unstated invariant: a prior is strictly earlier than the season played, and `DefconScoredIn`
  is monotone, so a non-zero prior `DefCon` implies the played season scores it. Unpinned.
- **A "three sources, three boundaries" line in CLAUDE.md.** It is a fact about sources; the
  season table is scoped to what the replay can run. It belongs in `archive-and-data.md`, not
  the resident file, and the budget had 14 bytes.
- **Extending `TestTheMiddleValueHasOneImplementation` to `xiValue` and `PriorSeasonName`.** C1
  asks for source scans; the four tests added here are runtime equivalence checks, which stop a
  divergence but not the next copy. C1 amended to say so and left open.

## What could not be checked on this harness

- **A3's magnitude.** No archived pair has a defcon-carrying prior; 2024-25 carries neither the
  stat nor the components to rebuild it. Harvesting would not rescue it — only the six cells
  playing 2025-26 could respond, so three season means are exactly zero and the clustered |t|
  is pinned at 1.00 by construction. **It ships on correctness, and that is stated rather than
  implied.**
- **Whether the flags change anything at `prior_half_life > 0`.** Unreachable at shipped config,
  so untested rather than verified — and the invariance claim is explicitly *false* there under
  `FPL_NO_XG_REPAIR=1`, which is why item 4 exists.
