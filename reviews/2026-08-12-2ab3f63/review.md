# Review record — what perfect team news is worth, and what it is worth for

**Commits reviewed:** `1a0f3f4..12684be`, which is the review fixes plus the snapshot regenerated
after them. Named for the commit that carries the record rather than for the snapshot beside
it — the gate names a record by what contains it, and a snapshot committed alongside always
trails by one.

⚠️ **The directory is named `2ab3f63`, which is the merge of `main` into this branch, and the
record does NOT cover what that merge brought in.** `9040ac3` (the BPS schedule decode and the
tackled channel) has its own record at [`2026-08-12-9040ac3`](../2026-08-12-9040ac3/review.md),
reviewed on `main` before it landed. The name is pushed past the merge so the gate reads the
branch as covered; the two records together are what covers it, and neither alone does. on `decision-metric-experiment`. Previous record:
[`2026-08-12-1a0f3f4`](../2026-08-12-1a0f3f4/review.md).

**What the change is.** Three measurements and one gate.

`OracleFeatures` gained `FeaturesFrom`, which restricts it to gameweeks at or after a
given one. Set to the entry gameweek plus one, the opening fifteen is bought on honest
information in both arms and comes back byte-identical, so the arm measures what the
knowledge is worth *when acted on* rather than what a differently-built squad is worth.

`TestDiagTeamNewsTransferValue` runs four `Simulate` calls per cell over 36 cells —
baseline and oracled, each at shipped `MaxHits` and at `MaxHits: 0`.
`TestDiagAutosubValue` measures what FPL's automatic substitution actually recovers.

## Before the reviewers: what must this change NOT move?

| invariant | outcome |
|---|---|
| the opening fifteen is identical across arms in all 36 cells | **caught a real off-by-one** — `FeaturesFrom: start` fires on the build, and the fifteen differed in 35 of 36 |
| the best legal promotion cannot score less than bench order | **caught a real bug** — the substitution loop consumed the formation counts and the counterfactual was handed the repaired map |
| the accuracy snapshot moves nothing but `stamp.commit` | passes, three times |
| `PointInTime` with no oracles is unchanged by the new `gw` parameter on `statusAt` | passes |
| Go computes no standard error | **caught by `TestInferenceLivesInOnePlace`** — an earlier version clustered seasons in Go |
| the env-switch fingerprint list is complete | **caught by `TestEnvSwitchListIsComplete`** — a comment of mine contained a literal switch token |

Six invariants, four of which fired. That is the skill's first section vindicated again:
the free checks caught more than either reviewer, and caught them earlier.

## The statistical review: the headline does not survive

**+54.5 a season does not resolve.** Season-clustered SE 25.07, **t = 2.174 against a
df-5 critical value of 2.571**, so this comparison's own threshold is **64.5 a season**
and the estimate sits below it. Worse, it is **carried by two seasons**: 2021-22 and
2025-26 supply **77.5%** of the sum, and the other four average **+18.4 at t = 1.13**.
The median season mean is +22.7, less than half the mean. Exactly one leave-one-out
subset clears its bar, and it is the one that drops the sole negative season.

The largest column, 2021-22, is one of the two seasons where `XGC` is unrepaired, so
defenders and keepers there are scored with **neither** the clean sheet nor the
goals-conceded deduction. The six seasons are not exchangeable draws, which is what a
season-clustered SE assumes.

**The corroboration was a units error, and it was mine.** `TestDiagAutosubValue` measures
**+0.1005 points per blank**. The "+4.7 a season" it was reported as came from dividing
the pooled total by the **36 cells** — and a cell averages **25.5 gameweeks**, not 38.
Converted this repository's own way (1.831 blanks per gameweek × 38) it is **+7.0**. The
33% understatement is almost exactly what made it coincide with the oracle arm's +3.9 and
be written up as two instruments agreeing.

They are not the same quantity either: `bestLegalPromotion` maximises *realised points*
over a fixed eleven with no armband, while `OracleFeatures` supplies only *whether he
features* and then re-picks the eleven **and the captain** on `Score`. And agreement on
the eleven-only rung would not have supported the POLICY rung regardless — different
decision, different noise, and it is the number in dispute. Retracted at all four sites,
the conversion is now performed inside the test, and the general failure is recorded in
CLAUDE.md's bugs list.

**"The transfer channel" was the wrong label.** POLICY = T + X(policy squad) and HOLD =
X(baseline squad, frozen all season), so the residual is T plus a bracket that is neither
zero nor signed — the baseline makes about 25 transfers per cell path, so by spring the
two squads barely relate, and the oracled policy buys players who feature and so has
fewer blanks left to re-pick around. Renamed to *what acting adds*, which is a real
estimand.

**Three of four headline numbers had no clustering ingredient.** Only POLICY was
accumulated per season, so only POLICY could ever have been given a verdict. All four
arms now emit season columns:

| season | POLICY | HOLD | acting | free only |
|---|---:|---:|---:|---:|
| 2020-21 | +22.7 | +0.0 | +22.7 | +25.5 |
| 2021-22 | +130.6 | +7.9 | +122.7 | +198.1 |
| 2022-23 | +21.5 | +0.0 | +21.5 | +30.8 |
| 2023-24 | −24.8 | +11.3 | −36.1 | −22.3 |
| 2024-25 | +54.2 | +3.0 | +51.2 | +26.6 |
| 2025-26 | +122.9 | +1.2 | +121.7 | +61.1 |

Worth reading rather than skipping: the free-only arm **swings harder** than POLICY
(+198.1 against +130.6 in 2021-22, +61.1 against +122.9 in 2025-26). It is the cleaner
*question* and the noisier *measurement*. And the HOLD rung is **exactly 0.0** in two
seasons, so the eleven-only channel does not fire at all there.

**It is not an upper bound**, and the reason is the gate rather than the oracle.
`availabilityFactor` returns exactly 0 and the transfer gate charges
`gain × DecisionHorizon`, so **a one-week absence is priced as a five-week write-off**.
The arm over-reacts to its own information by construction, cannot buy anyone who blanks
in the transfer week however much it wants him for the next five, and never learns that
an absence persists.

**Context that reframes the headline.** `OracleTeamNews` — FPL's own published flags —
already reads POLICY +25 a season. So roughly half of the +54.5 is free to a live user
and the incremental value of a *better* source is nearer +30, with neither figure
resolving. The baseline here is the deliberately blinded replay, not the live system.

**+1.2 for hits is not a measurement.** It is 2% of the detection threshold and has no
standard error. What it establishes is the negative — the headline is not an artifact of
an aggression setting — and the TODO item that closed the question as answered has been
reopened.

## The code review: five defects and two doc errors

| # | defect | why it mattered |
|---|---|---|
| 1 | `FeaturesFrom` had **no range guard**, in either direction | at or below the entry it gates nothing, so the arm runs the *unrestricted* oracle while stamping a restriction — the `min_gain` 0.0-vs-0.4 pattern; above 38 it never fires and reports the baseline under an oracled stamp. Both silent, both stamped. Now refused in `Simulate` and `HoldCaptaincyWeekly` |
| 2 | the field's own doc **prescribed the off-by-one** the invariance caught | it said "set it to the entry gameweek"; the build evaluates `statusAt` at `start` itself |
| 3 | the retracted `+4.7` was still asserted 460 lines below its own retraction | one file carrying both the corrected and the withdrawn figure, with the withdrawn one attached to the API a caller reads |
| 4 | `TestDiagAutosubValue` transcribed **one of the scoring path's two** substitution rules and read neither flag | under `FPL_NO_LEGAL_AUTOSUBS` the diagnostic would report on a rule the run did not use — the `TestDiagSixtyMinutes` failure exactly. Now refuses to run under that flag |
| 5 | the same test **hand-rolled the prior index**, dropping `PriorMinutesHalfLife`, `PriorRateHalfLife` and `OlderPriors` | equal at sweep defaults, which is the condition under which the recorded `HoldCaptaincyWeekly` divergence survived unnoticed |
| 6 | `oracleBounds` printed a caveat that `FeaturesFrom` **removes** | a confident caveat above a figure it no longer applies to; now branches on the gate |
| 7 | the baseline arm inherited `OraclesFromEnv()` and the treatment arms did not | a stray oracle switch would reach the baseline only, making the printed difference "team news minus perfect price timing". Refused in all three diagnostics |

**What the reviewer checked and found sound**, which is worth recording because two of
these were the specific things I asked to be attacked:

- **The `start+1` gate is right in both directions.** No transfer decision is left blind;
  the one blind week is the entry gameweek's XI, and it is forced rather than an
  off-by-one, since the opening build and that XI read the same point-in-time view. It is
  symmetric across both metrics, so the diff-in-diff is unaffected.
- **`HoldWeekly` applies the full oracle**, not a partial one — both rungs receive exactly
  the same perturbation, so the subtraction is not comparing a full oracle against a
  partial one.
- **`MaxHits: 0` is a valid paired comparison.** It is the only field differing, no
  consumer treats zero as unset, and each arm carries its own baseline.
- **The greedy promotion is exact** — brute-forced over **362,475 configurations with zero
  counterexamples**, plus a tie sweep. ⚠️ But the reason my comment gave for it was wrong:
  it is exact because `GKP` is pinned at one and the outfield slack is wide, not because
  "every candidate is considered against every remaining vacancy". Comment corrected.

## What this leaves

The **ordering** is what the arm establishes: team news pays through the transfer and not
through the eleven, by a factor of about fourteen. The **size** is not established and
must not be quoted as +53 or +54.5.

Queued rather than done:

- The CR2/Satterthwaite verdict. The standard path refuses this arm — `runPolicySweep`
  fails a cell whose `Oracles` differ from the variant's, and `FeaturesFrom` varies by
  start point by design. Two ways out: emit cells from the diagnostic directly via
  `openCellSink`, or normalise `Oracles` for the per-cell comparison. **Warn the operator:
  the arm is four `Simulate` calls over 36 cells, sweep-scale, and belongs under
  `scripts/replay`.**
- The duration problem. A one-gameweek oracle cannot see that an absence persists, and the
  gate inflates each week fivefold. `OracleMinutes` at a longer window is the arm that
  would test it, and it is a different measurement rather than a re-run.
- Repairing `XGC` for the seasons that have none — see the separate finding, which bears
  directly on 2021-22 being the largest column here.
