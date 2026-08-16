# Review record — repairing and re-running the anchored-chip measurement

**Commit range reviewed:** `74cc360..8bdac0f`, seven commits. The work applies a code review and a
statistical review of the chip measurement whose `+10.8` headline was retracted at `74cc360`, fixes
the defects that produced it, and re-runs.

**The branch was not green at the base commit.** `TestReviewCoversTheCurrentCode` was already
failing at `74cc360` because `docs/notes/chips.md` had moved since the previous record. That is
what this record clears, on top of its own range.

## Reviewers dispatched

| reviewer | why | outcome |
|---|---|---|
| **fpl-code-review** | `internal/analysis` and `internal/backtest` both changed | 11 findings; 8 applied, 2 recorded and declined, 1 pre-existing and out of scope |
| **fpl-stats-review** | `stats/*.R` and two new inference conventions entering the record | 2 claims judged, both established with corrections; 6 defects in the implementation, all applied |
| **fpl-findings-audit** | `CLAUDE.md` and `docs/` changed | 14 findings; 13 applied, 1 declined |

**Not dispatched:** `fpl-security-review` (nothing touches `internal/fpl`, `internal/agent` or
config persistence), `fpl-run-review` (no live run), `fpl-season-maintenance` (no hand-maintained
list moved).

Both dispatched reviewers were told not to run Go tests or replays, because a sweep was running and
a parallel replay gets killed on this machine — which produces a silently partial result rather than
an error.

## Verified before applying

Every finding below was reproduced independently before it was acted on. Two of the incoming
briefs' claims were checked and one of my own was found wrong.

**The four original defects, reproduced against the archive:**

- `laggedPlan(38)` returned three chips against `anchoredPlan`'s four in **5 of 24 cells** (2024-25,
  every entry but GW26) and a different triple-captain week in several more (2025-26: TC36 against
  TC26). So the lag arms and the ceiling arm compared different chip *sets*, which is a mechanical
  explanation for full sight underperforming a four-gameweek lag — the one number in the retracted
  table that should have looked suspicious.
- `controlPlan` dropped the triple captain in exactly **3 cells**, all at start 26, as reported.
- The control wildcarded at **GW4** from a GW1 entry against the anchored arm's GW28-33, as reported.
- Neither `WeekEngine` nor `engineAt` copied `TeamForm`; five budget fields were absent from both.

**The per-gameweek inflation arithmetic**, re-derived rather than taken: `38 × mean(1/w)` over
{38,33,28,23,18,13} is 1.6992 and over {18,13} is 2.5171. Matches.

**A defect I introduced, found by review and confirmed by my own reconstruction.** Holding the
wildcard at a common week — the obvious fix for the GW4 confound — makes the anchored arm's bench
boost land the gameweek *after* the rebuild by construction, since its wildcard is `bigDouble − 1`
and its boost is `bigDouble`. Counted over five archived seasons by six entry points: **30 of 30**
cells for full sight against 3, 4 and 5 of 30 for the lag arms and 5 of 30 for the control. This
record already establishes that a boost on a wildcard-rebuilt squad is worth materially more, so the
only arm boosting a prepared squad would have been the arm the measurement argues for.

**The lag arms had no minimum feature size.** At 2023-24 from a GW1 entry every lag arm played its
bench boost on a **two-club** double in GW7 and its free hit on a two-club blank in GW2, against
full sight's seven-club double in GW34 and twelve-club blank in GW29.

**The clustering-axis claim, on the archived `MinutesHalfLife` cells.** The raw between-season
variance component comes back negative in every HOLD arm (−1.402, −0.336, −0.278, −1.038) while the
printed share reads 0.0% for all of them; the season-clustered t is −2.41 where the fixed-start
estimator on 15 df reads −0.88, and −4.80 where it reads −2.68. The start-clustered CR2 prints
**−17.54** on POLICY/flat. All reproduced by hand before the script was changed.

**And one of my own claims was wrong.** The new weighting sentence said `per_gw` weights a GW1 entry
three times as heavily. For an event count it is the opposite — the GW26 cell contributes 2.92× the
GW1 cell, which is the mechanism of the 1.70× inflation — and the sentence contradicted the standing
convention fifty lines below it. Corrected.

## Applied

**The measurement.** One placement rule at a parameterised sight, with `anchoredPlan` *being* that
function at full sight rather than a second implementation. `firstUnbeaten`'s lookahead excludes
taken weeks. Control offsets place backwards from GW38 rather than dropping. `matchedChips`
intersects placed-ness across every arm and masks all of them, so the matched-set guarantee is no
longer one-directional. No arm plays a wildcard. `minAnchorClubs = 4`, asserted and labelled as
asserted. Three invariant tests replace the one that compared a single chip at a single entry point.

**The wiring.** `TeamForm` and the five budget fields copied into both derived engines, guarded by
`TestDerivedEnginesCarryEverySource`, which derives the required set by reflection over `Engine`
minus the fields `NewEngineFull`'s composite literal sets, read by AST — so a new field fails the
test until it is wired, with no hand-maintained exemption list to go stale. Verified it fails on both
its assertions when the copy is removed.

**The gate.** `AnticipateGate` takes `min(decisionHorizon, engine horizon)` and only when
`AnticipateChips` is set, instead of assigning the engine horizon outright and re-merging the two
jobs `DecisionHorizon` exists to separate. `TestOnlyTheTransferEngineAnticipatesChips` counts the
single call site — counting *down* rather than up, because that is what makes the arm's
byte-identical `HOLD` a construction guarantee.

**The inference.** `--scale=per_gw|per_path` on both R scripts; the fixed-start estimator, the raw
between-season component, the season sign agreement and the SE ratio printed beside both clusterings;
the `1/√weeks` null suppressed rather than relabelled on `per_path`; output filenames carrying the
scale; `season_share` refusing duplicated cells.

**Comments that had gone wrong.** The `WeekEngine` note attributed the `TeamForm` miss to a replay
sweep, when `internal/backtest` never calls it and the exposure was live — `Plan.GainPerGW` carrying
club form while `Plan.XI` did not. `SimConfig.ChipPlanner` still called a calendar planner flatly
"hindsight" where the test header retracts that. The derived-engine guard's out-of-scope note named
the skip set but not `priceForecasts`.

## Declined, with reasons

**Adding an `events_placed` column to the cells CSV.** The stats review is right that `per_path` is
a season-scale figure only if every path carries the same number of events, and that the per-cell
matched set does not guarantee it. On this grid the effect is small and countable: 12 cells play
three chips and 12 play two, split cleanly by season, because 2024-25 and 2025-26 have no second
double large enough for a triple captain. That is recorded in the section and in the harness note
instead of being fixed. The CSV schema has its own regression tests and the change generalises past
this measurement, so it belongs in its own commit rather than inside a re-run.

**Replacing the start-clustered CR2 rather than supplementing it.** The stats review recommends
dropping it, on the sound argument that the six entry gameweeks are a replicated fixed block so
CR2-by-start answers a question nobody asked, and on the demonstrated ground that it prints −17.54.
It is kept because the retraction cites its value (t = 1.00) and a reader must be able to reproduce
that. It now carries df and a caption saying exactly what it is, and the fixed-start estimator — the
analogue the review argues for — is printed beside it.

**`e.Cong` read without `congMu` in both derived-engine builders.** Real, pre-existing, and of
exactly the class this record warns about: `SetCompetitionWindows` writes under the lock and both
builders are reachable from concurrent tool handlers. Not fixed here because it is a change to the
lock discipline of a shared field rather than to anything in this range, and burying it inside a chip
re-run is how it would go unnoticed. Recorded in the guard's own comment so the next reader finds it.

**The `MinutesHalfLife` verdict.** Checked and deliberately not moved. Its recorded t values are
naive ones, and on those cells the naive figure sits within a rounding error of the correct
fixed-start estimator, so nobody quoted the inflated clustered column. It is overstated for a
different reason — half-life 2's Holm-adjusted p is 0.069 and 8 and 20 are indistinguishable from 4 —
and that is now recorded as a rider rather than as a re-run.

## What could not be checked here

**Whether calendar anchoring is worth anything smaller than the design can see.** The corrected
result is −5.4 points per season-path with a minimum detectable effect of 14 to 34. So "null" means
unresolved, not refuted, and an effect the size of the retracted +10.8 was never resolvable on this
grid in the first place.

**Chip preparation**, which is the half the measurement had to remove to be honest. The
wildcard-into-boost sequence is unmeasurable here for the reason the record already gives — a
wildcard is the largest perturbation available to a chaotic path — and it is where the value would
be if it is anywhere, on the 59%-more-bench-quality figure.

**Whether `minAnchorClubs = 4` is right.** It is asserted, the lag columns are sensitive to it, and
nothing here measures it. Stated at the constant and in the section.

## Provenance of the re-run

`/tmp/anchored2/cells.provenance.csv` stamps `commit efb68d7`, `dirty true`, constants digest
`67455aaa8835`. The uncommitted changes at run time are exactly the `anchored_diag_test.go` edits
that became `19a52aa`; everything added to that commit afterwards is comments and R, verified by
diffing `efb68d7..19a52aa` for non-comment Go changes and finding none. **So the run corresponds to
`19a52aa`'s Go behaviour**, not to `efb68d7`'s.

Every cell is stamped `oracle: -`, `oracle_kind: none`, which is consistent with the position taken
in both `SimConfig.ChipPlanner` and the test header: a planner reading only the published fixture
calendar is not hindsight, and one reading realised outcomes would belong in `Oracles`.

## The findings audit, and the two things it caught that the code reviews could not

Fourteen findings on the record itself, all reproduced before applying. Two changed a conclusion.

**The null's sign is one column of four cells.** Dropping the GW1 entries turns every arm positive —
full sight −5.4 → +3.2, two gameweeks −8.8 → +1.2, four and six −5.7 → +3.6, on both scales. That
column supplies 111-153% of each arm's mean and carries sd 48.9 against 16-21 at every other entry.
**This is the same defect the retraction identified in the old result** — four cells supplying 52% of
the mean — recurring at a larger share and in the opposite direction, in the measurement built to
remove it. The section now says "near zero at five of six entries and large and very noisy at the
sixth" rather than "every arm is negative".

**`minAnchorClubs` reaches full sight, and the section claimed it did not.** `firstUnbeaten` applies
the bar at every sight and `matchedChips` intersects across all five arms. Full sight's bench boost
is the season maximum and safe; its triple captain takes the *second*-biggest double, which is 6, 6,
**2** and **2** clubs across the four seasons. So the triple captain's absence from 2024-25 and
2025-26 is the asserted constant's doing, not the calendar's — the opposite of what was written —
and `TestAnchoredPlanSitsOnTheCalendarMaxima` checks `bigDouble` and `bigBlank` and never
`secondDouble`, so nothing in the suite would have caught it.

Also applied: the MDE was quoted as 14-34, pairing the best of two columns, and is 34-37 clustered
against 14-19 start-fixed — with the start-fixed end not defensible here, because the anchored plan
is *identical at all six entries* in three seasons of four, so the six entry points do not replicate
the treatment. The clustering precondition was anchored on the `MinutesWeight` table, which has since
been re-swept with every sign flipped, making it an example of the artifact rather than of the good
case. My "nobody quoted the clustered column" was false — `constants-and-sweeps.md` headlines −2.41
and −4.80 and explains them as seasons agreeing, when the between-season component is negative in
every arm; interpretation retracted, arithmetic kept. `agree` fails on its own worked example, being
a sign count. "Every constant in this record is a rate" is false, and the exception — the fixture
load term and the doubles fix — is in the same file. Plus the provenance note, the third-category
self-classification, the 19.8 oracle figure, the restored calendar premise, the "30 of 30" scope, the
no-anticipation note, and the `shape_inference.R` warning.

**Declined:** dropping the start-clustered CR2 column, for the reason given above — the retraction
cites its value and a reader must be able to reproduce it.

## Outstanding

**The corrected cells are provenance-dirty and want a re-stamp.** They were run from a working tree
at `efb68d7` with the two repairs applied but uncommitted, and `efb68d7` itself still plays a
wildcard and carries no size bar. The Go behaviour is `19a52aa`'s, verified by diffing for
non-comment changes, but the stamp is not sufficient to reproduce them. Re-run and re-stamp at
`19a52aa` or later, and commit the `mde.csv` — the snapshot records "no `mde.csv` was found", so the
34-37 threshold has no committed artefact. Recorded in `chips.md` beside the table.

**The fixture-load and doubles-counting magnitudes are owed a `--scale=per_path` re-read**, now that
the convention they were reported under has a name and an exception. Their invariances are
unaffected.
