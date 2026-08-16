# Review: the gate 2x2, and a composed ladder that had to be withdrawn

**Commit range reviewed:** `735743f..48f44ea`. Fixes applied at `bce7341`, which this record is
named for.

**What changed.** A new diagnostic (`TestDiagGateThreshold`) crossing the gate horizon with the
`min_gain` floor, its results written into `docs/notes/transfer-policy.md` and `CLAUDE.md`, and a
TODO item queueing a per-setting metric-reachability map.

## Reviewers run, and the triage

| reviewer | why |
|---|---|
| **fpl-stats-review** | a new points claim, and a composed shape used as evidence |
| **fpl-findings-audit** | `CLAUDE.md`, `docs/notes/transfer-policy.md` and `TODO.md` all gained claims |

Skipped: **fpl-code-review** — the only source change is a test file, and the diagnostic's
arithmetic was checked by the statistics reviewer against the cells.
**fpl-security-review**, **fpl-run-review**, **fpl-season-maintenance** — nothing in scope.

**Invariants first.** `HOLD`, both captaincy rungs and both captain marginals are exactly zero in
all 36 cells of all three non-baseline arms, and both mechanism-certain predictions held —
including the float band where `gain >= 0.4` and `gain*5 >= 2` could have disagreed. None of that
caught what the reviewers caught.

## Findings, ranked

### 1. The composed ladder spliced two baselines at a shared zero — WITHDRAWN

The headline claim was an "interior optimum at the shipped 0.4", composing four recorded rungs at
horizon 5 with this run's 0.25 point at horizon 8. Three reasons it cannot be constructed, any one
fatal:

- **The zero column is two different arms.** Recorded points are `(floor f, h5) − (floor 0.4, h5)`;
  the new one is `(floor 0.0, h8) − (floor 0.4, h8)`. Those baselines differ by −0.221 pts/gw, so
  anchored on one reference the 0.25 rung is **−24.2**, confounded with the horizon. Splicing two
  floor-effect curves at a shared zero is how a V appears where the levels differ by 8.4.
- **"Effective floor" is not one axis.** Verified from the cells: free moves are **818 at h5 and
  815 at h8** — essentially unchanged — so all 99 extra moves at h8 are **hits**, which carry no
  gain bar (`asHit` sets `noGainBar`). The arm labelled "the horizon makes the floor binding"
  leaves the floor's own population flat and adds moves it cannot reach.
- **It violated the diagnostic's own pre-registration**, which declared a favourable h8 result "not
  a case for anything". Promoting an *unfavourable* one into affirmative support for the shipped
  value is the same inferential step with the sign flipped — and it was the version that reached
  CLAUDE.md.

Three further mismatches even if the axis were one: the recorded rungs are 24 cells on four seasons
and **predate both data repairs**; they are re-baselined from a table computed vs 0.70, so point
estimates subtract but standard errors do not, leaving two rungs with **no uncertainty at all**;
and their source names a `BlankRunPenalty` confound that did not travel.

**A floor below 0.4 is unmeasurable on this harness as the code stands** — below it the binding
clause has both parameters in the value function — which is now recorded as the finding.

### 2. The horizon replication was mischaracterised — APPLIED

Called "a larger drift than the ±30% for four-season figures". Restricted to the *same four
seasons* the recorded −0.503 was measured on, this run reads −0.276 pts/gw (−10.5), so the gap is
not grid width. The xG wiring, the goals-anchor fix and the three data repairs all land after that
figure and touch seasons inside the four. **Stale, not unreplicated** — and a better story than
the one I wrote.

### 3. The wrong standard error was quoted — APPLIED

The h8/0.4 arm has a *negative* season variance component and `share_season` 0%, so the fixed block
is licensed and CR2 is anticonservative there. Correct figures: **t −0.80 against a threshold of
21.7**, not −1.34 against 16.1. The floor contrast keeps CR2 (`share_season` 17.1%, season F
p 0.081). Two estimators in one write-up for good reasons, now with the reasons stated.

### 4. "Refuted" over-claimed — APPLIED

At t −1.19 with 10 of 36 cells exactly zero, the pre-registered direction **did not appear**;
nothing is refuted either way. And the deeper reading is better than mine: none of the three
supports *could* bear on a looser floor — the ladder's left endpoint at 0.4 is forced by
construction rather than by sampling, the over-charged-gate mechanism is about a different knob at
the shipped horizon, and the perfect gate's extra volume is a mediator.

### 5. The reachability TODO was wrong on two of its four knobs — APPLIED

It listed `min_gain`, `DecisionHorizon`, `FreeCost` and `BankUpTo` as reaching only `decide()`.
True for the first and third. `BankUpTo` is read in `Simulate` itself; and **`DecisionHorizon`
reaches `HOLD`** through `oracleWindow()` → `recentIndex` → `HoldCaptaincyWeekly` whenever a
minutes or lineups oracle is wired. So the theorem/measurement split is **per arm, not per knob** —
a better statement of the item's own thesis, arrived at by committing the exact failure the item
proposes to fix, inside the item proposing it.

### 6. Smaller applied items

- The gate was written without its money term and without the `budgetWeight == 0` condition that
  makes "exactly 0.4" true — two statements of one predicate, eleven lines apart.
- The move analysis understated itself: **51 moves differ in absolute terms across 19 cells**, and
  **8 cells make identical moves and still differ by up to 38 points**. Path divergence, confirmed
  harder than claimed.
- `CLAUDE.md`'s "the gate's constants are a no-op" is scoped to *at or below 0.4* rather than left
  unqualified.
- The diagnostic's opening sentence still stated the refuted premise while the block below
  retracted it.
- The sweep ran POLICY transfer settings on the six-season grid against a standing instruction to
  use `default` until more POLICY arms have run on both. Now marked as a deliberate departure.

## Declined, with reasons

- **Re-running the recorded 0.7/0.95/1.30 rungs under the repairs.** Correct in principle and not
  done: the composition is withdrawn, so nothing now depends on their magnitudes, and re-running
  them would be re-opening a closed line for a figure no live claim rests on.
- **Adding a cells-file citation to the new section.** `transfer-policy.md` carries no such
  citations anywhere, so one here would be a convention change rather than a fix. Noted rather than
  repaired.

## What could not be checked on this harness

- **Whether a floor below 0.4 helps at the shipped horizon.** Structurally unmeasurable as the code
  stands — it needs a config field decoupling the gate threshold from the package value function.
- **Whether the recorded rungs survive the data repairs.** Unmeasured; they predate both.
- **Whether the six-season POLICY grid is the right one for transfer settings.** This run is one of
  the arms the standing instruction says are needed to decide that.
