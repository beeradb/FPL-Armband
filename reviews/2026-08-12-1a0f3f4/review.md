# Review record — pricing perfect knowledge of who plays

**Commits reviewed:** `888a2e6..1a0f3f4` on `prior-blend-gate`, which includes the merge of
`oracle-team-news2` (itself reviewed under
[`2026-08-12-e335012`](../2026-08-12-e335012/review.md), and merged to `main` separately).
The new work is `eccf254`, `6c39bae`, the merge `3c78a6a`, and the corrections this record
covers. Named for the commit that carries it rather than for the snapshot it sits
beside, which is the convention `TestReviewCoversTheCurrentCode` enforces. Previous record: [`2026-08-12-888a2e6`](../2026-08-12-888a2e6/review.md).

**What the change is.** Two instruments and one finding.

The instruments: a `lineupCoverage` hook that restricts `OracleLineups` to a named
subpopulation, and a new information oracle `OracleFeatures` that rewrites
`Element.Status` from whether the player actually featured in the gameweek about to be
played, restricted by `FeatureScope` to one side of the model's own availability flag.

The finding, which is what the user asked for: **what is perfect lineup knowledge worth to
the weekly eleven, given the squad you already own?**

## Before the reviewers: what must this change NOT move?

Invariants first, per the skill's own instruction that they have caught more real defects
here than reviewers have and are free.

| invariant | outcome |
|---|---|
| every model figure in the accuracy snapshot is byte-identical, twice | passes — only `stamp.commit` moved, on both regenerations |
| `PointInTime` (no oracles) is unchanged by the new `gw` parameter on `statusAt` | passes; both new branches are gated on bits that are zero in `Oracles{}` |
| Tier 1: `OracleFeatures` perturbs `Status` and nothing else, across the whole grid | passes |
| the two restricted arms sum to the unrestricted arm | passes — `lineups: unflagged` reproduces `lineups: all` digit for digit in all 918 gameweeks |
| no environment seed can default the new oracle on | passes; `OraclesFromEnv` still seeds only availability and prices |
| `Validate` refuses features+teamnews, features+availability, and a scope with no oracle | passes |
| `availabilityFactor` returns **exactly** 0 for `i`/`u`/`s`/`n` | **now pinned** by a new unit test; it was load-bearing and untested |
| a restriction without its oracle is refused rather than dropped | **now pinned**; it was neither |

**Two of the repository's own guards caught the new oracle before it ever ran** —
`TestEveryOracleSaysWhatItBounds` and `TestEveryInformationOracleHasAnInputDiff` both
failed on a bit with no declared bound and no input-diff case. That is the third time in
this branch's history that a guard has been cheaper than a reviewer.

## What the reviewers found

Two agents: a code review of the diff and the merge, and a statistical review of the
measurement and its stated conclusion. Fourteen findings, six of them real code defects.
All six fixed; the headline retracted.

### Code defects

| # | defect | why it mattered |
|---|---|---|
| 1 | `OracleFeatures` had **no liveness declaration**, so `oracleLivenessViolations` checked nothing on it | `statusAt` short-circuits on `FPL_NO_AVAILABILITY` **before** any oracle branch, so a sweep with that variable exported turns the arm into a total no-op while every cell still runs and still stamps itself. Reachable, not theoretical — `TestDiagAvailabilityImpact` sets it per call |
| 2 | `Stamp()` dropped `FeatureScope` | three different oracle states wrote `info:features` to the cells file. The stamp exists precisely so it "cannot disagree with what ran", and `Validate` refuses the mirror case by name |
| 3 | `lineupCovers` was honoured on one branch of `recentIndex` and dropped in silence elsewhere | a `"minutes: flagged"` arm written by copying the lineups one would run the **unrestricted** oracle and reproduce it exactly — which this branch would read as the structural inertness the flagged lineups arm genuinely has |
| 4 | `HoldCaptaincyWeekly` never called `cfg.Oracles.Validate()` | it is the entry point every oracle diagnostic here uses, so every refusal was advisory on the metric this project scores constants with. `applyTeamNews` overwrites `Status` after `statusAt`, so `OracleTeamNews` silently won over `OracleFeatures` |
| 5 | the `FeatureScope` doc named `OracleTeamNews` throughout | after the merge that is a live and *different* oracle, so a reader following the comment gets a `Validate` error contradicting the comment two lines above the field |
| 6 | the lineups constructor's doc block was orphaned by an inserted type | godoc attached "newOracleLineups builds…" to `lineupCoverage`, and the load-bearing sentence about both arms classifying the same window was detached from the function |

Plus one performance note taken: `gameweekStart` scans every fixture and was being called
per player inside the coverage loop. Hoisted.

### Claims of mine that were overstated

**"Every term `ExpectedMinutes` feeds is behind that zero."** True of `Score`, false of the
model. `SettledMinutes` is written from the same blended minutes and gates the squad pool
*outside* `availabilityFactor`; what keeps a flagged player out of the squad path is a
separate `Status` filter. Corrected in place. This is the file's own signature failure —
a sentence about one quantity inherited as a sentence about the system.

**"A player who returns in February stays at a hard zero until May."** Refuted by the
reconstruction's own rule. `statusAt` reads the **end-of-season** status and the **last**
news item, so a player who returns and finishes the season fit reads available all year and
is never flagged at all. That sentence was the entire reason the flagged arm looked worth
measuring.

**The pool denominator.** Held slots legitimately sum over cells — six entry points buy six
different fifteens. Pool player-gameweeks do not, and summing them over six starts reported
a league six times its real size. Counted once per season, the figure is 12 of 32,501.

**One defect I caught myself, by arithmetic rather than by review**, and it is worth
recording because it is this project's catalogued failure mode committed inside a
diagnostic: a scripted edit silently no-opped, so the pool counter was declared, summed and
never incremented. It printed "0 of 150,051 played" beside a held column showing 3 — and a
subset cannot exceed its superset. The contradiction is what surfaced it; nothing else
would have.

## The measurement, and what survives of it

Six arms over the same held fifteen, six seasons by six entry points, 918 gameweeks. Only
the weekly re-pick differs, so the squad channel is closed by construction.

| arm | weeks changed | pts/firing | clean seasons | reconstructed |
|---|---:|---:|---:|---:|
| `lineups: all` | 43.9% | −1.087 | +1.8 | −38.1 |
| `lineups: flagged` | 0.0% | — | 0.0 | 0.0 |
| `lineups: unflagged` | 43.9% | −1.087 | +1.8 | −38.1 |
| `features: all` | 1.3% | +7.000 | +4.5 | +2.5 |
| `features: flagged` | 0.0% | — | 0.0 | 0.0 |
| `features: unflagged` | 1.3% | +7.000 | +4.5 | +2.5 |

"Clean seasons" is 2023-24 onward, where `starts` is a real column.

### What is established

**A flagged player is unreachable by any oracle travelling through minutes, and that is a
proof rather than a measurement.** `Score` ends in `* availabilityFactor(el)`, which
returns exactly 0 for `i`/`u`/`s`/`n`, and the weekly re-pick consumes nothing but `Score`
and position — formation, bench order, captaincy and the autosub loop all key on one or the
other. The reviewer traced all four and found no escape; the bench-order tie is broken by a
*stable* sort on the input order, which is identical in both arms. So `lineups: flagged`
returning exactly 0.0% is structural.

**The exactness has a second load-bearing dependency nobody had written down**: the
coverage predicate is evaluated at the same gameweek as the bootstrap status. Test it at
any other week of the window and the arms stop partitioning. Now commented at the site.

**The replay's availability flag is right 99.96% of the time**, so no flagged-only arm can
be informative on this archive. The reconstruction fires only on terminal absences.

### What is retracted

**"Perfect lineup knowledge makes the weekly XI worse" is not a finding, and the −18.1 a
season behind it must not be quoted.** Clustered by season it is −18.1 ± 10.8, **t =
−1.68** against a df-5 threshold of ~28 — unresolved on its own numbers. The sign comes
entirely from 2020-21 and 2021-22, where three things are wrong at once: `starts` is
reconstructed so the oracle's classification *and* its conditional prices are both
inferred; `xgc` is identically zero so the clean-sheet channel the oracle is documented to
act through multiplies nothing; and 2020-21's prior is COVID 2019-20.

On the three real-starts seasons the same arm reads **+1.8 ± 8.5, t = 0.22** against a
threshold of ~36. **Cannot tell — emphatically not "nothing there."**

**And the arm does not grant what its title says.** `oracleWindow()` returns the decision
horizon, 5, so `OracleLineups` grants a five-gameweek forward *average* of selection while
the metric pays this week. That alone can produce the negative sign with no claim about
lineup knowledge at all — it is the record's own "a season average of future minutes beats
the truth, for a squad" family arriving in reverse. `OracleFeatures` is window-1 by
construction and does not have this problem, which is part of why its figures are the
cleaner pair.

**The recorded +1.932 pts/gw is not contradicted.** It is a four-season figure that also
lets the oracle change which fifteen is bought, on a cell-equal-weighted estimator. Three
things differ and only one is the channel. Subtracting the two would attribute ~76 a season
to squad selection, and that is **not recorded**: it is a difference of two unresolved
numbers on different grids with different estimators. The honest version is a paired
diff-in-diff in one process, which is queued rather than done.

### One limit newly recorded, because it makes the arm a dirty bound

`"u"` **removes** a player from the optimiser's pool rather than zeroing his score, so a
one-gameweek fact acquires permanent force on the opening squad. At a GW1 entry on 2025-26,
391 of 690 registered players record no minutes in GW1 and are barred for the season —
twelve of whom go on to play 2,000+ minutes, Gravenberch (2,991 minutes, 144 points) and
Foden (2,078, 131) among them. The diagnostic closes that channel by holding the fifteen
fixed and un-oracled; **a sweep arm would inherit it and must not be presented as a bound.**

## The practical reading

Perfect knowledge of who plays, applied to the weekly eleven with the squad held fixed, is
worth about **+4.5 points a season** on the seasons whose data supports the question — and
that comparison cannot resolve anything under about 36. So the honest answer to "what is
perfect team news worth to the XI?" is **not much, and we cannot measure the not-much
precisely.**

The mechanism is the interesting part, and it is not a harness artefact: **FPL's autosub
rule already captures most of it.** A manager who fields a player who does not appear gets
his bench player's points automatically. The intuition that eighteen missed starters a
season is eighteen chances to gain is right about the *event count* and wrong about the
*counterfactual* — the game already substitutes for you.

That relocates the value of team news to **squad building**, where the recorded +1.932
lives, and to **transfers**, neither of which this measurement touches.

## What is queued rather than done

- Re-run `TestDiagLineupEventValue` at `OracleWindow: 1`, so the information matches the
  decision being scored, with per-cell diffs to CSV and CR2 inference in R. Warn the
  operator: seven arms over 36 cells of weekly engine rebuilds, on the order of an hour.
  Run it under `scripts/replay`.
- The squad-versus-XI split as a paired diff-in-diff in one process, if that decomposition
  is wanted.
- `FeatureScope` on `OracleTeamNews`, which is where the flagged/unflagged split actually
  belongs: the crawled bootstraps carry `d` and carry every absence that later resolved,
  which is the population the reconstruction cannot see. Not implemented.
- The diagnostic reports a gameweek-weighted mean; every other figure in the record is
  cell-equal-weighted. The record already carries a retraction about exactly that estimator
  swap. Worth aligning before any figure here is quoted beside a sweep figure.
