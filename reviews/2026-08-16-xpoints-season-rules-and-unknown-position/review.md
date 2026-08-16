# xPoints: an unknown position must be loud, and the rules must be pinned per season

Branch `xpoints-season-rules-and-unknown-position-guard`, off `origin/main` at `cf4379a`.
The change and this record ride in one commit.

## What was reviewed

Two silent-failure tripwires in the xPoints instrument. Neither had ever shipped a wrong number;
both are the class this record calls its signature failure — a null that reads as a result.

1. **`XPointsResidual` read `goalPoints[g.Position]` as a bare map index.** Go returns the zero
   value for a missing key, so an `element_type` the table has never heard of priced its goals
   channel at *nothing* and the row still returned a plausible number, because appearance, bonus,
   saves and cards are all left realised. The population is real: FPL ran assistant managers as
   `element_type` 5 for 2024-25, and `Season.Players` loads 20 of them — 322 archive rows,
   accumulating to 312 player-gameweeks and carrying 1,861 points, every one at zero minutes.
   Now `XPointsRules.Prices(pos)` is a key-presence test and an unpriced position panics, **before**
   the `Minutes <= 0` return, because a guard behind it could never fire on that population.
2. **xPoints pinned no per-season rules**, where `BankLimitFor` and `DefconScoredIn` do.
   `TestScoringConstantsMatchFPL` asserts `goalPoints` against FPL's *current* published
   `game_config`, which is what keeps it honest and is also the mechanism that would have carried
   the next rule change backwards over the whole archive. `analysis.XPointsRulesFor(season)` now
   derives a per-season table from the current constants and applies named amendments backwards;
   `repaired()` resolves it onto `Player.Rules` beside the conversion scale, and `xPointsOf` reads it.

Files: `internal/analysis/xpoints.go`, `internal/analysis/xpointsrules.go` (new),
`internal/backtest/season.go`, `internal/backtest/simulate.go`, six test files (two new),
`stats/xpoints_common.py`, `CLAUDE.md`.

## Which reviewers ran

| reviewer | why |
|---|---|
| **fpl-code-review** | the diff touches `internal/analysis` and `internal/backtest` |
| **fpl-stats-review** | the change asserts a decoded FPL constant and a confinement result |
| **fpl-findings-audit** | it retracts a claim already written into two files, and raises whether `CLAUDE.md` owes an entry |

Skipped, with reasons: **fpl-security-review** — no credential, cache, agent or config-persistence
surface is touched. **fpl-run-review** — no live run. **fpl-season-maintenance** — none of the four
hand-maintained lists moves. **fpl-docs-accuracy / fpl-docs-review** — `docs/` greps clean for
`xPoints`, `XPointsResidual` and `element_type`; the only prose change is `CLAUDE.md`, which
`fpl-findings-audit` covers.

## Findings, ranked by how misleading the pre-review state was

Each was **verified independently before being applied** — the skill's own rule that a report is a
set of proposals, not a finding. Two reviewer claims disagreed with each other on the row set, and
the measurement below settles it.

### 1. "The choice is inert" was false. Applied.

`xpointsrules.go` argued that leaving the boundary unestablished was safe because the archive's one
goalkeeper *goal* sits outside the unresolved span. That is the mediator argument failing on the
author of the mediator argument: the goals channel is `(Goals − XG·scale)·Goal[pos]`, so `Goal[1]`
prices the **expected** half too, and every goalkeeper row with non-zero xG is re-priced.

Measured over the repaired archive, GKP goals scale exactly 1.0 in every season (keeper xG never
reaches `minCalibrationSample`, so `CalibrationRatio` returns 1):

| season | rows | ΔxPoints | | season | rows | ΔxPoints |
|---|---|---|---|---|---|---|
| 2016-17 | 0 | 0 | | 2021-22 | 0 | 0 |
| 2017-18 | 0 | 0 | | 2022-23 | 2 | −0.920 |
| 2018-19 | 3 | −0.574 | | 2023-24 | 1 | −0.080 |
| 2019-20 | 3 | −1.460 | | 2024-25 | 2 | −0.280 |
| 2020-21 | 1 | +3.604 | | 2025-26 | 1 | −0.640 |

13 rows, ~6.6 xPoints of absolute movement across ten seasons. Replaced with the row list and an
explicit **bound, not identity**. The residual unresolved span is now 3 rows and 1.00 xPoints.

### 2. The boundary constant was wrong by a season. Applied.

The header claimed no capture before 2025-26 carries a `game_config` block. That was checked on GW1
of each season only, and it is false. Verified over all captured 2021-22..2024-25 bootstraps:
2021-22, 2022-23 and 2023-24 carry none, and **23 of 2024-25's 38 do**, from
`GW16-2024-12-14T1200Z` onward, publishing `goals_scored.GKP = 10`. That capture is a genuine
point-in-time harvest — `current_event` 15, and its `scoring` block has no `defensive_contribution`
key, which 2025-26's does.

So `season < "2025-26"` was pricing a 2024-25 keeper goal at 6 against published evidence of 10.
`keeperGoalRuleChangeSeason` is now **`"2024-25"`**, the latest season the evidence permits, and the
unresolved span narrows from five seasons to three (2021-22..2023-24). The constant's label moves
from *asserted* to **bounded, not measured**. No figure moves either way — no goalkeeper scored in
2024-25 — but inertness is why an unestablished boundary is tolerable, not a licence to leave a
value the captures contradict.

### 3. The season-ordering comment was backwards. Applied.

`xpointsrules.go` and `replay_test.go` both said a hand-built fixture name "sorts below every real
season and therefore reads as the oldest rules". Go compares bytes: `"test" < "2024-25"` is
**false** ('t' 0x74 against '2' 0x32), so every letter-initial fixture gets the **modern** table.
Inert on the fixtures in this tree — none gives a keeper a goal or an xG — and the comment was the
whole of what a reader had. Corrected in both places.

### 4. "The only goalkeeper goal in the archive" was right, by an unsound method. Applied.

My scan filtered `position == 'GK'` on `merged_gw.csv`. That column **does not exist** before
2020-21 (checked: 2016-17 through 2019-20), and 2021-22 spells 101 keeper rows `GKP` rather than
`GK` — so the filter returns a clean zero on five seasons for reasons that are not football, which
is exactly the byte-identical null this record keeps being caught by. The conclusion survives on the
sound method (join `players_raw.csv`'s `element_type` onto `merged_gw.csv`'s `element`, which is
what this package's own loader does and what `Player.Type` therefore is), independently confirmed by
scanning the parsed cache for all ten seasons. Both traps are now written into the comment and into
`CLAUDE.md`, because that is the part worth keeping.

### 5. The confinement claim was over-scoped, and the liveness half was missing. Applied.

The first run was 18 cells over 2020-21 / 2024-25 / 2025-26 and came back byte-identical on all four
metric columns. Read narrowly that is nearly worthless: 2025-26 is a no-op by construction at either
pin, and `policy_points`/`hold_points` cannot move at shipped config at all, since `Player.Rules`
has exactly one non-test reader and it is inside `xPointsOf`.

Re-run on 30 cells including **2022-23 and 2023-24**, the seasons that actually carry mediating rows,
pin on against pin off:

| column | moved |
|---|---|
| `policy_points` | **0 of 30** |
| `hold_points` | **0 of 30** |
| opening fifteen | **0 of 30** |
| `policy_xpoints` | **6 of 30** — all six 2022-23 cells, each exactly −0.44 |
| `hold_xpoints` | **2 of 30** — 2022-23 at entry GW21 and GW26, each exactly −0.44 |

That is the pair the record asks for: confinement on the columns that must not move, and a liveness
arm that **must** move and does, by exactly one archive row (Raya 2022-23 GW27, xG 0.11 × 4 = 0.44).
0.44 on a season total is about 1% of this grid's own `HOLD` threshold of ~33.

A third, independent confinement datum: the accuracy snapshot `2026-08-16-db82ab0` regenerated at
this commit moves **one figure of 555, `stamp.commit`**, against `2026-08-16-b93179d`. The
calibration, prediction-benchmark, defcon-bias, team-blend and transfer-error diagnostics are
byte-identical, which is what "xPoints is instrumentation and does not reach the scoring path"
predicts and is worth having as a check rather than an assumption.

### 6. A retracted claim was repeated in two more files. Applied.

`stats/xpoints_common.py` and `xpointstable_test.go` recorded that the keeper goal's 6 "was the
script's own invention rather than an older rule it had been left behind by". The first clause is
retracted — 6 is a value FPL really paid. The retraction is now marked **immediately under** the
sentence it retracts in both files, not two paragraphs later.
`internal/backtest/conversionscale_test.go` and `internal/backtest/bpsschedule_test.go` carried the
same framing at one remove and are amended in place.

⚠️ **Deliberately weakened from what the first amendment said.** It claimed "the two values are two
eras". That over-claims: what is established is that 6 was a real FPL value, *not* that this
script's 6 descended from it. Provenance is unrecoverable and the correction does not need it.

### 7. The declared Python divergence was sized at zero, and is not zero. Applied.

`stats/xpoints_common.py` carries one table for every season while the Go instrument is now
season-aware, declared rather than fixed on the ground that it is exercised on zero rows. False, for
the same reason as finding 1: `GOAL[1]` multiplies xg. At the default `NATIVE_XG_SEASONS`, and with
the corrected boundary, 2024-25 and 2025-26 are priced correctly and only **2023-24** is in the
unresolved span, holding one affected row worth **0.08 residual points**. The declaration now
carries that number and the fact that `appearances(seasons=…)` reaches much larger ones.

### 8. Two smaller corrections. Applied.

- "322 gameweek rows" is a fixture count. `loadGameweeks` accumulates, so it is **322 archive rows
  and 312 player-gameweeks** — the doubles distinction this package is built around, in the one
  place it is live. Corrected at all four sites.
- "Nothing stops a sixth position arriving, and that arrival would be silent" is overstated.
  `TestScoringConstantsMatchFPL` iterates FPL's *published* element types and errors on one the
  model has no value for, so a live arrival is loud. It skips outright on a payload with no
  `game_config`, which is how element_type 5 arrived in the **archive** unnoticed. Rescoped.

### 9. `Prices` closes one channel of three. Applied as a test, not a code change.

`Prices` reads `Goal` alone, while `CleanSheet[pos]` and `ConcedeBlock[pos]` are still bare map
reads. A position present in `Goal` and absent from `CleanSheet` would pass the guard and silently
lose a channel — the bug being fixed, one channel over. Inert today (all four positions are in
both). `TestTheUnpricedPositionTestIsKeyPresenceAndNotValue` now asserts
`keys(Goal) ⊆ keys(CleanSheet)`, which is the tripwire that survives the next amendment.

### 10. A liveness test skipped where its sibling fatalled. Applied.

`TestTheArchiveHoldsAPositionTheInstrumentCannotPrice` skipped when 2024-25 loaded no unpriced
element_type. That population is the *entire* justification for placing the guard before the minutes
check, so a skip would take the evidence away while the suite went green — which the xPoints script
guard in this same diff condemns in as many words. Now a `t.Fatal`. The dropped error from
`PointInTime` is also checked.

## What was declined

- **Pinning the engine's own scoring path per season.** `metrics.go` reads `goalPoints[pos]`,
  `cleanSheetPoints[pos]`, `assistPoints` and `concedeBlock[pos]` directly at five sites in
  `baseXP90`, `fixtureSensitiveAt` and the bonus estimate, and none of them is season-pinned. That
  is strictly the *larger* exposure of the two, because it moves squad selection and therefore
  `hold_points`/`policy_points` rather than an instrumentation column. It is out of this change's
  scope and it is a scoring change, not an instrument one. **Declined but named**, in `Prices`'s doc
  comment and in `CLAUDE.md`, so nobody reads this guard as closing the class.
- **Making `stats/xpoints_common.py` season-aware.** It needs the season threaded into
  `unscaled_residuals`, which changes five scripts and supersedes their banked figures for a
  0.08-point defect at the default seasons. Declared and sized instead; recorded as owed.
- **Adding a hard refusal to `appearances()`.** The proposed guard keys on a goalkeeper *goal*,
  which would not fire on the xG rows that are the actual divergence, and it changes the behaviour
  of scripts this branch does not otherwise touch. The sized declaration is what was applied.
- **A `Things that have already bitten` entry in `CLAUDE.md`.** That section's contract is *shipped
  bugs*. Neither tripwire shipped a wrong number: the unknown-position zero has an empty realised
  population, and the per-season pin is prospective by construction. Putting a hazard in a list of
  bugs is how the list stops being trustworthy. One bullet went into *What a player is worth*
  instead, beside the decoded `pen_saved = 15`, which is its nearest sibling.
- **Amending `reviews/2026-08-15-xpoints-conversion-scale/review.md`,** which repeats the retracted
  framing. A review record is a dated statement of what was concluded that day, and re-keying it
  would break the digest it was committed under. The corrections live at the code sites a reader
  lands on.

## What could not be checked on this harness

- **The early end of the keeper-goal boundary.** Nothing before 2024-25 GW16 publishes scoring, and
  no goalkeeper scores in 2021-22..2023-24. It needs FPL's own published rule history, not a run.
  The same applies to the amendment's *early* extent — the 6 is applied back to 2016-17 on the
  evidence of one 2020-21 row, and that is now stated in the header rather than left implicit.
- **Whether any banked `hold_xpoints` / `policy_xpoints` figure moves. It does, and by how much on
  which arms is UNVERIFIED.** 2020-21 and 2022-23 are banked in the four xPoints-bearing snapshots
  (`2026-08-15-{xppilot,gateresidual,gatescaled,clean-sheet-2x2}`), and the 30-cell run above shows
  2022-23 moving in 6 of 6 cells at shipped config. The banked arms are *not* shipped config, so
  their ownership differs and the per-arm figure is unknown. **Nothing here claims banked figures are
  unaffected.** The bound is 0.44 per affected cell, roughly 1% of the `HOLD` threshold and about
  0.6% of the `POLICY` one, so no recorded interval, null or verdict can turn on it — but a paired
  difference banked earlier is not byte-identical, and a re-derivation would need the 2020-21 and
  2022-23 cells of those four snapshots re-run at both pins.
- **Whether the movement can flip a gate-oracle decision.** `perfectGateXPoints` and
  `perfectGateResidual` call `xPointsOver` on the *candidates* of a proposed move, so the mediator
  there is transfer candidacy rather than squad ownership, which is strictly wider than what the
  30-cell run tests. At shipped config nothing flipped — squads and `policy_points` are identical in
  30 of 30 — but the oracle arms are unchecked.

## Verification run on this branch

`go build ./... && go vet ./... && go test ./...` all pass.

Four mutations were checked against the new tests, each caught: deleting the position guard; moving
it below the `Minutes <= 0` return; deleting the season amendment; aliasing the package maps instead
of copying them. A fifth — moving the boundary to 2021-22 — correctly does *not* fail, which is the
honest statement that the early end is unresolvable from data this repository holds.
