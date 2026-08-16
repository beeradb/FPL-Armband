# The engine's scoring rules: pinned per season, and an unpriced position refused

Branch `engine-season-rules-and-unknown-position-guard`, off `origin/main` at `b452d44`, with
`xpoints-season-rules-and-unknown-position-guard` merged in. `origin/main..HEAD` is two commits:
`69b3b13` (the merge) and `f8ce35c` (this change), plus a second commit carrying the review
findings.

⚠️ **The sibling branch had NOT landed on `origin/main`.** The brief said it had. It exists as a
local branch one commit ahead of `origin/main`, and merging it was the only way to satisfy "do not
add a second per-season rules table" — so this branch's diff against `origin/main` contains the
sibling's 2,192 lines as well as this change. Stated here rather than buried, because a reviewer
diffing against `main` will otherwise attribute the instrument's change to this one. The sibling's
own record is `reviews/2026-08-16-xpoints-season-rules-and-unknown-position/`.

## What was reviewed

The engine-side half of two defects whose instrument half the sibling closed. **Neither result
transports**: the instrument writes a column nothing scores, and `baseXP90` writes `Score`.

1. **`metrics.go` read `goalPoints[pos]` as a bare map index at five sites** — `baseXP90` (1),
   `setPieceXP90` (3), `fixtureSensitiveAt` (1). Go returns the zero value for a missing key and
   cannot distinguish it from a stored zero, and a stored zero is a real FPL rule in this table's
   siblings (a forward's clean sheet pays 0), so **a value test cannot work**. The fix is key
   presence: `ScoringRules.Prices`.
2. **The engine's scoring rules were not season-pinned**, where `BankLimitFor` and
   `DefconScoredIn` are. `TestScoringConstantsMatchFPL` asserts `goalPoints` against FPL's
   published `game_config`, which is what keeps it honest and is also the mechanism that would
   force the next rule change backwards over the whole archive.

`cleanSheetPoints[pos]`, `concedeBlock[pos]` and `assistPoints` were routed onto the same table at
the same time. Pinning one channel of four and leaving three unpinned is a half-fix that reads as
a whole one.

## The design decision, and why it is not the sibling's

The sibling attached a `ScoringRules` to each `Player`. The engine cannot copy that: it has six
construction sites in `simulate.go` alone, plus two **derived**-engine builders in
`internal/analysis` (`WeekEngine`, `engineAt`) that rebuild from `e.Boot` and that
`TestEveryScoringEngineGetsRecency`'s scan cannot see at all. An `Engine.Rules` field would have
needed eight assignments, any one of which a patch could miss — the shape of this project's most
expensive recorded wiring bug, and worse here because a missed pin is byte-identical.

So the season rides on **`fpl.Bootstrap.Season`** (`json:"-"`, absent from FPL's payload) and
`NewEngineFull` derives `Engine.rules` from it. There is no assignment to miss, the derived
builders inherit for free, and the exposure collapses onto the **two** places a replay bootstrap is
constructed. `TestNoReplayBootstrapIsBuiltWithoutASeason` parses for a third.

⚠️ **`ScoringRulesFor("")` had to be given an explicit meaning, and getting it wrong would have
been a shipped scoring bug rather than a replay one.** FPL's payload carries no season, so a live
bootstrap is `""` — and `""` is the one string that really does sort below every archived season
under Go's byte comparison. Without the clause, pinning the replay would silently have priced every
**live** goalkeeper's goal at the pre-2024-25 6.

## Confinement and liveness

Four arms of `TestDiagEngineSeasonRules`, six-season grid, `sweepConfig` at shipped weights.
Two escape hatches, deliberately separate so an arm that moved both would be uninterpretable:
`FPL_NO_SEASON_SCORING_RULES` and `FPL_NO_UNPRICED_POSITION_GUARD`.

**Liveness — both halves must move, and do.**

| arm | pin moves `BaseXP90` | pin moves `Score` | guard refuses |
|---|---|---|---|
| shipped | **552 player-cutoffs** (2020-21: 66, 2021-22: 8, 2022-23: 100, 2023-24: 378; 2024-25 and 2025-26 zero, correctly — both are at or after the boundary) | **155**, max \|ΔScore\| **0.0151** (2022-23 Meslier @GW26) | 40 |
| `FPL_NO_SEASON_SCORING_RULES=1` | **0** | **0** | 40 |
| `FPL_NO_UNPRICED_POSITION_GUARD=1` | 552 | 155 | 40, **each scoring `BaseXP90` exactly 2.0** |

⚠️ **Quote both columns.** `BaseXP90` is the channel the rule enters; `Score` is what the optimiser
orders on, and a keeper with no minutes moves the first while the second stays identically zero. 552
is an **upper bound** on the mediating population; 155 is the population. The first version of this
arm reported only `BaseXP90`, which would have made the confinement a confinement of a quantity
nothing had measured.

The third row is the guard's liveness measured rather than asserted: with the hatch open the
diagnostic's own assertion fails on all 40, at exactly `appearancePoints`, which is the defect
reproduced. All 40 are 2024-25 at `through` 26 and 37 — 20 managers, two cutoffs.

**Confinement — `policy_points`, `hold_points`, opening fifteen. 72 cells: 6 seasons × 6 entry
points, each replayed at both `WeeklyXI` settings.**

| comparison | `policy_points` | `hold_points` | `squad_hash` |
|---|---|---|---|
| shipped against shipped (determinism control, 36 cells) | 0 of 36 | 0 of 36 | 0 of 36 |
| shipped against **pin off** | **8 of 72** | 0 of 72 | 0 of 72 |
| shipped against **guard off** | 0 of 72 | 0 of 72 | 0 of 72 |
| **pin on at the EARLY boundary** against pin off | **0 of 72** | 0 of 72 | 0 of 72 |

⚠️ **Read the last row before the second one.** It is the whole answer, and it took a fourth arm to
get: with `keeperGoalRuleChangeSeason` set to the early end of its own unresolved range, the pinned
engine reproduces the **unpinned** result **byte for byte in 72 of 72 cells**.

**So the pin is confined and the constant is not.** Pinning the engine's rules per season — the
mechanism fix, the thing this change is for — moves no replayed point at all. Every one of the eight
moved cells is produced by the constant taking the *late* end of a range this repository cannot
resolve, and all eight are in seasons inside that range.

⚠️ **This is a materially worse result than the 36-cell version reported, and the difference is
entirely the `WeeklyXI=false` half that `fpl-stats-review` insisted on.** The first pass ran
`WeeklyXI=true` only, found **1 of 36**, and would have shipped a confinement claim that missed
seven of the eight moved cells — including the only pattern in them.

| season | entry points moved | at | Δ `policy_points` |
|---|---|---|---|
| **2022-23** | **all six** (1, 6, 11, 16, 21, 26) | `WeeklyXI=false` only | **−7 in every one** |
| 2023-24 | GW11 only | both settings | −2 |

Everything else is byte-identical, in both directions and at both settings. The **uniform −7 across
all six 2022-23 entry points** is a signature rather than six results: every cell runs to GW38, so
they share a tail, and one decision inside that shared tail moves all six. It is one event, counted
six times — **do not read it as six independent cells**, and do not sum it.

⚠️ **`WeeklyXI=false` is the setting `runPolicySweep` uses**, which is the setting the banked
`POLICY` record was measured at. So the half that moves is the half the bank was taken on.

The unpriced-position guard remains a **pure tripwire**: real on the engine's own output column
(40 player-cutoffs, `BaseXP90` 2.0 → 0) and byte-identical on all 72 replayed cells.

**`hold_points` and the opening fifteen never move.** So nothing entered through squad selection or
through the weekly XI; all of it entered through the **weekly transfer decision**, which the traced
cell below confirms directly.

### A third instrument, and the contrast with the sibling is the point

The accuracy snapshot regenerated at this commit, `stats/snapshots/2026-08-16-258ad5c`, moves
**13 model figures of 555** against `2026-08-16-db82ab0` — small ones, in the calibration-drift,
prediction-calibration, prediction-candidates and prediction-ordering families, none larger than
0.0032.

⚠️ **The sibling's snapshot moved exactly one figure, `stamp.commit`.** That was the correct result
for a change to an instrumentation column, and its record says so. This one moves thirteen, because
those diagnostics score players *through the engine* and the engine now reads a per-season table.
Two changes closing the same defect on two paths, and the snapshot tells them apart without being
asked to — which is worth more than either number on its own, and is the cheapest available check
that "the engine writes `Score` and the instrument does not" is a real distinction rather than a
claim in a comment.

`stamp.dirty` is `false`: the snapshot was taken on a clean tree after the change committed, so its
figures are attributable to a commit that exists. A first render had `dirty: true` and was discarded
for that reason.

## Mutation checks

Each mutation applied, the named test run, then reverted. `/tmp/mutcheck.sh`; the tree is clean
afterwards. **All nine were caught.**

| mutation | test that failed |
|---|---|
| delete the door in `Engine.Metrics` | `TestTheEngineRefusesToScoreAPositionItHasNoRulesFor` |
| move the door BELOW `m.BaseXP90 = e.baseXP90(...)` | same — placement, not merely existence |
| make `CleanSheetPoints` a VALUE test (`v == 0`) instead of key presence | same, via its positive control: a forward's clean sheet is legitimately 0 |
| delete the season amendment in `ScoringRulesFor` | `TestTheEngineScoresEachSeasonUnderItsOwnRules` |
| delete the `season == ""` clause | `TestTheLiveEngineScoresUnderTodaysRules` |
| drop `Season: cur.Name` from `PointInTimeWith` | `TestEveryReplayBootstrapCarriesItsSeason` **and** `TestNoReplayBootstrapIsBuiltWithoutASeason` |
| make `WeekEngine` rebuild from a season-less bootstrap | `TestDerivedEnginesInheritTheSeasonsRules` |
| leave a refused row's `AvailabilityFactor` at zero | `TestTheEngineRefusesToScoreAPositionItHasNoRulesFor` |
| make `Engine.ScoringRules()` alias the engine's maps instead of cloning | `TestTheReportedRulesCannotReachTheEngine` |

⚠️ One mutation that does **not** discriminate, and is worth naming: making `Prices` a value test
(`r.Goal[pos] > 0`) still refuses element_type 5, because FPL pays every playing position something
for a goal. The key-presence argument is carried by `CleanSheetPoints`, where a stored zero is a
real rule, and by the sibling's `TestTheUnpricedPositionTestIsKeyPresenceAndNotValue`. A reader who
mutates only `Prices` will conclude the distinction is untested. It is not — it is tested one
channel over, which is where it bites.

## The one moved cell, traced

`TestDiagTheMovedCell` in `internal/backtest/enginerulesmovedcell_diag_test.go`. Three questions,
each answered by a measurement rather than by reading the code.

⚠️ **`-count=1` is load-bearing and its absence produced a wrong answer once.** Both arms are the
same package at the same commit, differing only in an environment variable read at package
initialisation, so Go's test cache served the pinned run's output for the unpinned one. The two arms
printed **identical move lists and identical points** — which reads exactly like "the hatch is
inert", and is the strongest possible wrong answer, because it is what a clean confinement result
looks like. `staleness_test.go` records the same trap for the snapshot recipe.

**Which rule differed.** `Goal[1]` — a goalkeeper's goal — **6 pinned against 10 unpinned**. It is
the only difference: all four positions' goal, clean-sheet and concede values and the assist are
enumerated and match otherwise. 2023-24 sits inside the amended span, because
`keeperGoalRuleChangeSeason` is `"2024-25"` and the amendment is `season < that`.

⚠️ **"There are zero keeper goals in the archive, so it cannot be that" is the trap**, and it is the
one the instrument's own author fell into and had to retract. 2023-24 really does hold **0**
goalkeeper goals — and **12 of the 85** goalkeepers in the GW11 pool are still re-priced on `Score`,
because `baseXP90` prices `XG90 × scale × Goal[pos]`. The mediator is the **expected** half. A
goalkeeper who never shoots still carries a blended, shrunk rate.

**What changed in the season: timing, not selection.** Both arms make the same **27 transfers** and
take the same **1 hit**. The move lists differ in exactly one way — GW29 and GW30 swap contents:

| | GW29 | GW30 |
|---|---|---|
| pinned (1730) | Raya (GKP) → Pickford (GKP), Bowen → Ødegaard | Saliba → Gabriel |
| unpinned (1732) | Saliba → Gabriel | Raya → Pickford, Bowen → Ødegaard |

The player whose price moved is the goalkeeper, and it is the goalkeeper's move whose week changed.
So **the rule did act** — this is not a bare path-divergence artefact, the mediator is identifiable
and it is the right position. But the two points measure **when three transfers were made**, which
is a draw from the transfer path's own recorded 303-point spread, not the value of the rule.

**Why 8 of 72, and why the movements are so small.** The mediator is not rare — 155 player-cutoffs move
`Score` across the grid — but the *magnitude* is: max |ΔScore| anywhere is **0.0151**. For that to
change anything, two candidates must sit within 0.015 of each other at the transfer margin, which
is the argmax boundary. Rarity is a property of the decision boundary, not of the rule.

### ⚠️ And the boundary, not the pin, is what moves it

`keeperGoalRuleChangeSeason` is **bounded, not measured**: the change happened somewhere in
2021-22..2024-25, and the constant takes the **latest** end the evidence permits — which applies the
pre-modern 6 to 2021-22, 2022-23 and 2023-24 on no direct evidence at all.

Re-run with the constant temporarily set to the **early** end, `"2021-22"`, and restored immediately
afterwards. First on the traced cell:

```
2023-24 element_type 1: goal 10/10 — 0 of 85 goalkeepers re-priced
CELL 2023-24 start=11 policy_points=1732        <- the pre-pin value
```

Then over the whole confinement grid, which is what settles it: at the early boundary the pinned
engine reproduces the **unpinned** arm in **72 of 72 cells, byte for byte** — all six 2022-23 cells
and the 2023-24 one included.

**So the pin moves nothing and the constant moves everything.** Pinning the engine's scoring rules
per season — the mechanism fix, and the whole point of the change — is byte-identical on replayed
points. All eight moved cells are produced by `keeperGoalRuleChangeSeason` taking the late end of a
range this repository cannot resolve, in seasons that sit inside that range.

⚠️ **This constant was instrumentation-only until now, and it is points-load-bearing from here.**
`fpl-stats-review` predicted that the pin would raise the bar on it; this is the measurement that
shows by how much. Whichever end it takes, it should take deliberately — and neither end is
evidence-backed for 2022-23 or 2023-24.

## Bug fix, or contamination event? — the question the brief asked to be answered first

**Both, and the second half is a decision for the user rather than something to absorb.**

- **In kind it is a bug fix, and not a close call.** Before it, `TestScoringConstantsMatchFPL` —
  the test that keeps `goalPoints` honest against FPL's published table — was also the mechanism
  that would carry the *next* rule change backwards over the whole archive. That is exactly what
  `BankLimitFor` and `DefconScoredIn` exist to prevent one layer away, and nothing argues for the
  old behaviour. **This is the part the change stands on**, and it is prospective: it is worth
  having on the day FPL moves a value, which has not happened since 2024-25.
- ⚠️ **In the cells that actually moved it is neither, and the pin is not what moved them.** All
  eight are in 2022-23 and 2023-24, both inside `keeperGoalRuleChangeSeason`'s unresolved span.
  Before the pin those goalkeepers were priced at today's **10** — today's rule applied to seasons it
  may not have been in force for. After, at **6** — decoded from a single 2020-21 row and
  extrapolated forward on no direct evidence. **Neither value is established.** And the fourth
  confinement arm shows the pin at the *early* boundary reproducing the unpinned result in 72 of 72,
  so the movement belongs to the constant's unresolved half and not to pinning.
- It **moves banked quantities, and the affected cells are greppable in the bank.** "Banked figures
  are unaffected" is **false**, and this is the part to bring to the user rather than absorb:

  | season | cells moved | delta | pre-change value banked in |
  |---|---|---|---|
  | 2022-23 | all six entry points, `WeeklyXI=false` | **-7 each** | **19 rows across 12 cells files** (`H#1`, `J#1`, `TestDiagMinutesOracle#1`, `GATERES#1`, `GATEXP2#1`, `BLEND#1`, `BLENDLO#1`, `reach2#1`, `anti-residual-gate#1`), checked on the GW1 cell alone |
  | 2023-24 | GW11, both `WeeklyXI` settings | -2 | 67 rows across 42 cells files (`TestDiagBaseline#1`, `XPPILOT#1`, `XGC6AGG#1`, `BENCH#1`, `MINW#1`) |

  Against this grid's own thresholds — roughly 33 on `HOLD`, 70 on `POLICY` — neither is close to
  resolvable, so no recorded verdict can turn on either. But that is an argument about **size**, not
  an invariance, and the six 2022-23 cells are **one event counted six times**, not six results.
- ⚠️ **72 shipped-config cells BOUND rather than settle it.** The banked arms above are not at
  shipped config, so whether each moves by the same amount is unknown — the pin changes a price, and
  whether a decision follows is arm-dependent. What *is* settled is that the affected cells are in
  the bank at their pre-change values, which the grep confirms rather than assumes.
- The 552 are concentrated in **2022-23 and 2023-24 (478 of them)**, which are *inside* the
  unresolved span of `keeperGoalRuleChangeSeason` — the boundary is bounded to 2021-22..2024-25 and
  not measured. So this change makes an unmeasured constant reach `Score` for the first time.
  Recorded rather than resolved; settling it needs FPL's published rule history, not a run.

**No points claim is made.** The change is justified on mechanism.

## Reviewers

| reviewer | why |
|---|---|
| **fpl-code-review** | the diff touches `internal/analysis` (scoring), `internal/backtest` and `internal/fpl` |
| **fpl-stats-review** | it makes a confinement claim and a liveness claim, and retracts a figure |
| **fpl-findings-audit** | it falsifies two claims in `CLAUDE.md` that I am instructed not to edit |
| **fpl-security-review** | it touches `internal/fpl` and adds two panics on a path fed by an untrusted payload |

Skipped: **fpl-run-review** (no live run), **fpl-season-maintenance** (none of the four
hand-maintained lists moves), **fpl-docs-accuracy / fpl-docs-review** (`docs/` greps clean for
`goalPoints`, `XPointsRules` and `element_type`; the only prose owed is in `CLAUDE.md`, which
`fpl-findings-audit` covers).

## Retractions made during this change

1. **The liveness figure was 14 and is 552.** The first arm built its engines with a bare
   `analysis.NewEngineFull` and no `Priors`, `Recent` or `TeamForm`.
   `TestEveryScoringEngineGetsRecency` caught it. The cause is that `Engine.Priors` supplies a
   prior season's expected-goals rate through `blendRates`, so a goalkeeper with zero
   season-to-date xG still carries a non-zero blended rate — a **40x** understatement of the
   mediating population, in the direction that flattered the confinement result. The confinement
   arm was never affected, because it goes through `Simulate`.
2. **"Assistant managers enter the replay's bootstrap at `through` 37" is wrong; it is 26.** The
   first probe sampled 1, 5, 10, 20 and 37 and skipped the entry point in between. 26 is a
   gameweek the shipped grid really enters at, so the guard's population is reached at a deadline
   the replay picks a squad from — a stronger reachability claim than the one it replaces.

Both are marked in place at the code sites, with the cause.

## Findings

All four reviewers returned findings, and **all four found something that mattered**. Each was
verified independently before being applied — a reviewer's report is a set of proposals. Ranked by
how misleading the pre-review state was.

### 1. `f8ce35c` shipped a RED test. Applied.

`TestEnvSwitchListIsComplete` failed on the commit: neither `FPL_NO_SEASON_SCORING_RULES` nor
`FPL_NO_UNPRICED_POSITION_GUARD` was in `snapshot.envSwitches`. Found by **fpl-security-review** and
independently by **fpl-code-review**, both of whom ran it rather than reasoned about it.

The consequence is worse than a red gate, and it is the exact failure the fingerprint exists to
prevent: a sweep run with either hatch set would be **fingerprinted as though the shipped defaults
ran**. The hatch-on arms of this change's own confinement run were taken in that state. Both are
registered now, with the note their neighbours carry — one changes the *rules* rather than a
constant, the other changes *what gets scored at all*.

### 2. A refused row asserted something FALSE about the player. Applied.

Found by **fpl-code-review**, after **fpl-security-review** had flagged the same area as
theoretical. The door returned with `AvailabilityFactor`, `Congestion` and `RoleFactor` at Go's
zero. All three are **multipliers whose neutral value is 1**, and a zero is not "unset" to what
reads them: `present/card.go` renders `availability ×0.00` through a branch whose own comment says a
zero there "must never be filtered out as empty", and `tools.go` passes `avail_factor` to the model
through a *pointer* so a ruled-out player's 0.0 survives. An assistant manager would have been
reported to a human and to the agent as **unavailable** — a different and wronger claim than
"unpriced". All three need no points table, so all three are now computed before the return, and
`TestTheEngineRefusesToScoreAPositionItHasNoRulesFor` asserts them at 1.

### 3. The liveness statistic was one column upstream of the confinement. Applied.

Found by **fpl-stats-review**. `BaseXP90` is where the rule enters; `Score` is what the optimiser
orders on, and a keeper whose minutes reliability is ~0 has the first move while the second stays
identically zero. So 552 was an **upper bound on the mediating population**, and the confinement was
a confinement of a quantity the liveness arm never measured. The arm now reports both, plus the
largest movement anywhere: **155 player-cutoffs move `Score`, max |ΔScore| 0.0151** (2022-23
Meslier at GW26). Free — no replay, the loop already held both engines.

### 4. The documented hatch command line failed by construction. Applied.

Found by **fpl-stats-review**. Both liveness assertions are *guaranteed* to fire in the hatch
process — finding nothing IS the pre-change behaviour — so the arm whose only job is to produce the
confinement's other half exited non-zero, and `docs/replay.md` sells an exit status you can trust.
The liveness arm now asserts in the shipped process only and **logs** the pre-change values
otherwise, which is also what turns "Score was already zero for this population by another route"
from something reached by reading code into a measurement.

### 5. The confinement ran at one `WeeklyXI` setting, and the banked POLICY record is at the other.
Applied, with a run.

Found by **fpl-stats-review**. `runPolicySweep` — which produced the `POLICY` cells under
`stats/snapshots/` — calls `sweepConfig(cfg, start, false)`; the arm ran `true`. A null at one is a
**simple-effect null**. Rather than add a third environment switch (which would have owed its own
`envSwitches` registration) the confinement now runs **both settings in one process**, doubling the
cell count to 72 per arm.

### 6. The instrument's 3-row bound does not transport to the engine. Applied.

Found by **fpl-stats-review** and **fpl-findings-audit** independently. `scoringrules.go` sized the
unresolved boundary at "3 archive rows and 1.00 xPoints, on the instrument columns only". That is
computed on `(Goals − XG·scale)·Goal[pos]` over *realised* rows. The engine's channel is
`XG90 · scale · Goal[pos]` with `XG90` blended and shrunk, so the population is every keeper-cutoff
with a rate — and **486 of the 552 sit inside the unresolved span** (2021-22 8, 2022-23 100,
2023-24 378). The header and the constant both now say so. ⚠️ This is the *third* time the "check
the mediator" rule has been broken inside the comment that states it.

### 7. `keeperGoalRuleChangeSeason`'s bound is one-sided, and it now reaches `Score`. Applied.

Found by **fpl-stats-review**. "2024-25 is the only end this repository can defend" over-claims:
ignorance about 2021-22..2023-24 is *symmetric*, and taking the late end applies the pre-modern 6 to
three unevidenced seasons — the choice that intervenes more, not the conservative one. Recorded on
the constant, together with the consequence that **the direction of the one moved cell carries no
claim**: 2023-24 is inside the span, so neither 1730 nor 1732 is known to be what FPL would have
paid.

### 8. "Assistant managers enter at `through` 26" is still wrong; the onset is 23. Applied.

Found by **fpl-findings-audit**, which read the archive rather than a probe grid: their first
`merged_gw.csv` rows are **GW23**, and `registeredBy` admits a player from his first row. 26 is the
earliest point *the shipped grid samples*, which is the reachability claim that matters — but it is
not the onset, and two versions of that comment have now been wrong in different ways. Corrected at
all four sites, with the distinction spelled out.

### 9. A retracted figure survived inside its own correction. Applied.

Found by **fpl-code-review** and **fpl-findings-audit**. `metrics.go` said `through` 26 in one
paragraph and GW37 eleven lines later. Also applied: the retracted liveness figure (**14**) and the
corrected one (**552**) now sit together at the code site, rather than the cause alone.

### 10. The bootstrap source scan reached one package. Applied.

Found by **fpl-code-review**. `Season` lives on `fpl.Bootstrap`, so the exposure is repo-wide even
though today's two literals are not — `cmd/priorblend` and `cmd/flagfit` build engines over archive
data, and `internal/capture`/`internal/backfill` hold whole archived payloads. The scan now walks
from the repository root, skipping `.git` and `.claude` for the reason its neighbours record.

### 11. `Engine.ScoringRules()` handed out the engine's own maps. Applied.

Found by **fpl-code-review**. A comment asked callers to treat the result as immutable; a write
would have re-priced every later player *and* raced, since the tool runner reads one engine from
several goroutines. It clones now, and `TestTheReportedRulesCannotReachTheEngine` is the assertion
rather than the paragraph.

### 12. One switch, two readers, two behaviours. Applied as a comment.

Found by **fpl-code-review**. `FPL_NO_UNPRICED_POSITION_GUARD` restores the engine's bare index
exactly but does **not** reach `XPointsResidual`, whose refusal is unconditional. Stated on the
hatch. Deliberately **not** made symmetric: gating the instrument's panic would re-open the silent
zero its own guard closed, and the instrument has no live user to keep running.

### 13. Two unit and arithmetic corrections. Applied.

- **478 → 486.** The unresolved span is 2021-22..2024-25, so 2021-22's 8 belong in it
  (**fpl-findings-audit**).
- **"552 player-cutoffs against 13 archive rows" is not a ratio.** Different units: 552 is one
  goalkeeper counted once per cutoff over 6 seasons × 8 cutoffs, of order 70 distinct
  keeper-seasons (**fpl-findings-audit**).

## Declined

- **Gating `XPointsResidual`'s panic on `FPL_NO_UNPRICED_POSITION_GUARD`.** Proposed for symmetry by
  nobody and explicitly advised against by **fpl-code-review**; declined for its reason. See
  finding 12.
- **Amending `f8ce35c`'s commit message.** **fpl-findings-audit** found four over-claims in it, all
  real: "a stored zero is a real FPL rule *here*" (it is a rule in this table's *siblings*, not in
  `goalPoints`); "came back at exactly 2.0 … a plausible number with a channel deleted" (2.0 is
  `appearancePoints`, and the real population has no underlying either way, so the 2.0 shows the row
  *surviving* rather than the channel dying — the deletion is shown on a fixture); "eight
  assignments" (true on the replay path, but three more construction sites exist outside it); and
  the GW26/GW37 wording. Declined **as an amendment** and recorded here instead: the commit is what
  was handed to four reviewers, and rewriting it would falsify the audit trail. The corrections are
  applied at the code sites a reader lands on.
- **Reordering `Metrics` so every descriptive field sits above the door.** Only the three
  multipliers had a consumer that mis-reads a zero; `Fixtures`, `AvgDifficulty`, `RestRisk` and
  `FixtureLoad` are all guarded by `!= 0` or `> 0` at their call sites. Moving four more assignments
  through a function that scores every player on every call, for a case that cannot occur, is a
  worse trade than the three that were moved.
- **Re-running the banked snapshot families at both pins.** Proposed as "the run that would settle
  it"; it is an order of magnitude more expensive than everything else here and would still only be
  a simple effect. The bound is stated instead.

## What is owed and was not done

⚠️ **The resident record was renamed on `main` while this branch was in flight**, and `CLAUDE.md`
is now a 424-byte import pointing at it. This branch still carries the old file, so the text below is
quoted from that copy — apply it to whichever file carries the resident record when this lands, and
check the line has not moved. (The new name is deliberately not written here: it is not a path that
resolves in this checkout, and `TestNoTrackedMarkdownCitesAMissingFile` is right to refuse it.)

### The resident record: two false claims and a dead identifier, and I was instructed not to edit it

Around line 724, in *What has been measured → What a player is worth*, the bullet
**"A goalkeeper's goal paid 6 before the modern 10"** ends:

> The xPoints instrument pins the rule per season (`analysis.XPointsRulesFor`, its `BankLimitFor`),
> because `TestScoringConstantsMatchFPL` would otherwise carry the *next* rule change backwards over
> the whole archive. ⚠️ **The engine's own scoring path is NOT pinned** — `baseXP90` and its family
> still read `goalPoints` — so that exposure, which moves squad selection rather than a column, is
> open.

All three of those are now wrong: the exposure is closed, the identifier is `ScoringRulesFor`, and
"the instrument pins" is half the truth. **fpl-findings-audit** drafted the replacement; this is it,
trimmed to the figures this run actually produced. Apply from `The xPoints instrument pins…` to the
end of that ⚠️ sentence, leaving the paragraphs either side alone:

> Both the instrument and the engine pin the rule per season through **one** table
> (`analysis.ScoringRulesFor`, the `BankLimitFor` of the scoring rules), because
> `TestScoringConstantsMatchFPL` would otherwise carry the *next* rule change backwards over the
> whole archive. ⚠️ **This bullet said "The engine's own scoring path is NOT pinned", and that
> exposure closed at 2026-08-16.** The five bare `goalPoints[pos]` reads — one in `baseXP90`, three
> in `setPieceXP90`, one in `fixtureSensitiveAt` — go through the season's table, which
> `NewEngineFull` derives from `fpl.Bootstrap.Season` rather than from an `Engine` field, so the two
> derived engines inherit it with no assignment to miss. ⚠️ **The type was `XPointsRules` until that
> commit**; renamed because a name saying "xPoints" would claim the instrument reaches `Score`. The
> escape hatches are `FPL_NO_SEASON_SCORING_RULES` and `FPL_NO_UNPRICED_POSITION_GUARD`; the first
> reaches both readers, the second the engine only.
> The **engine** half is not confined the way the instrument's was. Liveness: the pin moves
> `BaseXP90` at **552 player-cutoffs** over 6 seasons × 8 cutoffs and `Score` — the mediator the
> optimiser orders on — at **155**, max |ΔScore| 0.0151. Read those in *player-cutoffs*: one
> goalkeeper counts once per cutoff, so of order 70 distinct keeper-seasons. Confinement over 72
> shipped-config cells (6 × 6, each at both `WeeklyXI` settings): see the review record for the
> counts. ⚠️ **"Banked figures are unaffected" is false** — one cell moved by 2 points, and its
> pre-change value is banked under `stats/snapshots/`, several rows labelled shipped baselines.
> **Quote no t**: one of six season-means non-zero pins the season-clustered t at 1.00 by
> construction; the pre-change value is banked in 67 rows across 42 cells files. ⚠️ **486 of the
> 552 sit inside 2021-22..2023-24, where the boundary is bounded and
> not measured**, and so does the one cell that moved — so the single banked quantity this change
> moves is produced entirely by the half of the constant this repository cannot establish. Before
> the pin those seasons were priced at today's 10, which is equally unestablished there. It ships on
> **mechanism**; no points gain is claimed. → `reviews/2026-08-16-the-engine-scoring-rules-pin/`

**fpl-stats-review** additionally proposed a *Things that have already bitten* entry for the bare
map index. I would **decline** that one on the section's own contract — it is a list of *shipped
bugs*, and this defect never produced a wrong score, because the population records no minutes. The
sibling branch's record declined the identical proposal for the identical reason. It belongs where
the bullet above is.
- **Four scoring constants are still unpinned**: `appearancePoints`, `defConPoints`, `savesBlock`
  and the card deductions. They are read straight and forced forward by
  `TestScoringConstantsMatchFPL` exactly as the four channels here were. Declared in `Prices`'s doc
  comment; widening the table to them is a separate change with its own confinement run to do.
- **`CleanSheetTermFor`** in `teamstrength.go` still reads the package table directly. It is a
  calibration accessor with no production callers and it wants today's rules on purpose. Declared.

## What could not be checked on this harness

- **Which banked figures move, arm by arm.** ⚠️ **This was filed as "could not be checked" and half
  of it can be, by grep** — found by **fpl-findings-audit**. The affected cell *is* banked.
  ⚠️ **Its figure is corrected here**: the report said 48 rows; re-run independently,
  `grep -rh ",2023-24,2022-23,11,28,5,false,1732," stats/snapshots/*/cells/*.csv` returns **67 rows
  across 42 cells files**. Labels include shipped baselines (`TestDiagBaseline#1`, `XGC6AGG#1`,
  `XPPILOT#1`) and non-shipped arms (`BENCH#1`, `MINW#1`, `A4XGC#1`, `BOTH6#1`).
  That the unpinned arm reproduced **1732** exactly is itself the check that this diagnostic's
  configuration and those banked arms agree on that cell. What is *not* settled is whether the
  non-shipped arms move by the same 2, or at all — the pin changes a price, not a decision, and
  whether a decision follows is arm-dependent. That needs a run, and it is not worth one: see
  "Declined".
- **The early end of `keeperGoalRuleChangeSeason`.** Nothing before 2024-25 GW16 publishes scoring
  and no goalkeeper scores in 2021-22..2023-24. It needs FPL's published rule history.
- **Any points consequence of the change.** One cell of 36 moving by 2 points is not a measurement
  of anything; it is a confinement result with one non-zero entry.
