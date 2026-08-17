# Leaning the transfer policy into fixture runs

Reviewed: the working tree of `lean-the-transfer-policy-into-fixture-runs` against `b4e2b61`
(the banking merge on `development`), which is this branch's base.

## What was reviewed

Three things, which are one piece of work:

1. **A determinism fix in `teamBands`.** The 3/14/3 band assignment was built by ranging a map
   into a slice and ordering it with the non-stable `sort.Slice`, so a tie on a band boundary
   resolved by Go's randomised map order. Fixed by building the slice in club-id order and
   breaking both sort ties on club id.
2. **A fixture-run mediator**, five new cells-CSV columns beside the transfer-banking funnel:
   `band_ready_weeks`, `band_moves`, `band_run_moves`, `band_worse_moves`, `band_exposure`.
3. **The tandem made expressible and provable** — banking, chip preparation and the fixture
   bands in one arm, each checked at its own mediator rather than at its config field.

**No points claim is made anywhere in this change, and no default moves.** `band_strength` still
ships at 0.

## Reviewers

| reviewer | ran | why |
|---|---|---|
| **fpl-stats-review** | yes | the change touches `internal/analysis` (scoring) and `internal/backtest` (harness), and its whole output is an instrument whose columns will be read by someone who was not here |
| **fpl-code-review** | **yes, on a second pass** | Skipped on the first pass on the argument that invariants cover the failure modes it exists to catch. **That was the wrong call and the record says so**: it ran over `4cf601b` and found a live inversion the invariants could not see (Finding 1 below), plus three documentation defects. The invariants argument holds for the bugs already *known* to this codebase; it says nothing about a bypass path nobody had connected to the mediator |
| fpl-findings-audit | **skipped** | triage lists it for `AGENTS.md` edits. The `AGENTS.md` change is a single verdict correction (`teamBands` "Unfixed" → fixed) plus one stale enumeration, both of which the statistics reviewer checked directly and corrected. No new finding was recorded and no figure was added |
| fpl-security-review | not applicable | no change to `internal/agent`, `internal/fpl`, or config persistence |

## Findings from the code review, applied

Ranked by how misleading the state would have been. All four were reproduced before being acted
on.

0. **`FPL_MAGNITUDE` made `band_strength` inert while the mediator still reported the bands as
   live** — the exact inversion the block exists to prevent, delivered by the block itself.
   `fixtureMultipliersFor` returns the magnitude multipliers and **returns before `attackBandAdj`
   or `defenceBandAdj` is ever called**, so `BandStrength` reaches nothing at any value; but the
   bands still compute from finished fixtures, so all five columns populated with plausible
   counts. A tandem sweep run with that variable set would have shown "33 of 37 weeks ready, 31
   moves, 11 better / 10 worse" and been read as *the policy had every opportunity and declined*.
   **Applied**: the mediator now gates on `analysis.BandChannelLive`, which requires both that the
   ratings exist and that the scoring path consults them, and `FixtureRunFor` reports not-ready
   under the same condition. `TestTheBandChannelIsNotLiveUnderMagnitudeDifficulty` pins it.

   What the code review saw and the invariants could not: nothing in this repository connected
   `FPL_MAGNITUDE` to the bands, so no existing guard was watching that seam.

1. **`band_run_moves` had no null to be read against.** With only the favourable count,
   `band_moves - band_run_moves` pooled "traded the run away" with "the bands had nothing to say
   about this move", and those license opposite conclusions. **Applied: added
   `band_worse_moves`.** The data vindicates the finding outright — on one 2025-26 cell the arm
   reads **11 better, 10 worse, 10 tied**, so the original single count would have been reported
   as "a third of transfers improve the run" when the honest reading is a coin flip.
2. **Nothing warned that a between-arm difference in these columns is a `POLICY` difference.**
   The first thing a tandem sweep invites is subtracting them across arms, which carries the full
   transfer-path divergence with no pairing and ~31 observations a cell. Applied, in the struct
   comment and in `stats/README.md`, with the recorded 303-point transfer-path floor named.
3. **`stats/README.md` was not updated at all**, and it is where an analyst actually looks —
   it already carries the "counts, not rates" warning for the banking block. Applied: a full
   section, including that `band_moves` is **not** the `moves` column (36 against 31 on one cell)
   and that `band_ready_weeks` is close to a calendar fact rather than an arm property.
4. **`FixtureRun.Fixtures`' doc comment was factually wrong** — it said blanks and doubles change
   the count. They do not: `TeamFixtures` takes the next *n fixtures*, so a double changes which
   gameweeks the window spans, not how many matches are in it. Wrong reason, and it is the reason
   a reader would use to decide whether doubles inflate `band_exposure`. Applied.
5. **"That split is the same one `attackBandAdj` and `defenceBandAdj` make" overstated.** True of
   *which* band each position reads, false of *how much* it is worth: the model's attacking
   coefficients are deliberately asymmetric (0.23 target against 0.15 avoid) and the clean sheet
   enters through `exp(-x)`. `Net` weighs them equally, so it is a count and never a proxy for
   the adjustment's size. Applied.
6. **`TestTheFixtureRunFunnelNests` asserted a property four of six sweep start points
   legitimately violate** — from about GW7 every decision week is band-ready. It passed only
   because the fixture pins `StartGW: 1`. Applied: gated on an early start point, with the gate's
   reason stated, since the sibling comment in the same file warns this is how a test gets deleted
   instead of read.
7. **`FixtureRunMediator.Moves`' comment and the code disagreed** about moves whose players do not
   resolve. Applied: the comment now says "resolvable".
8. **The `144` reachable-assignments figure could not be checked from the text.** The number is
   right, but the quoted tie sizes multiply to 96 — it is a product of `C(tie, places inside)`.
   Applied: the derivation is now carried in full in `banddeterminism_test.go` and the shortened
   copy in `bands.go` points at it rather than inviting wrong arithmetic.
9. **The `AGENTS.md` jitter warning was over-stated by omission.** "Cannot be re-derived from its
   own cells" standing alone reads as an indictment of the `BandStrength` result two bullets
   above. The size is known and small — the arm's mean moved +0.339 → +0.357 pts/gw, about a tenth
   of that contrast's own CR2 standard error, concentrated away from the cells carrying the
   estimate. Applied, with the referent the file's own rule requires.
10. **`docs/configuration.md` used `0.25` in a worked example with no explanation**, which reads
    as a recommendation. It is precisely the arm the record calls deciding and unrun. Applied.
11. **A stale enumeration in `AGENTS.md`** — the `mustNotMoveForAxis` bullet listed the column
    families required to differ by arm and did not include the new block. Applied.
    (`cellMetricColumns` is unchanged at eight, so no invariance falsely covers the new columns.)
12. **`FixtureRunFor` reads one of the two band channels a defender is scored on, and only the
    midfielder half was documented.** `fixtureAdjustedXP90` passes BOTH multipliers to
    `fixtureSensitiveAt` for every position, so a defender's goals and assists are re-priced by
    the opponent's defence band and this counter cannot see it — and that omitted channel is
    larger than the midfielder one the comment did discuss. **Applied as documentation**, since
    the block is a count of fixtures and never a proxy for their worth, plus a correspondence pin
    (`TestFixtureRunReadsTheSameBandSideTheEngineDoes`) so the half that *is* modelled cannot
    drift out of step — the desynchronised-mirror shape this repository has paid for twice on
    `baseXP90`/`fixtureSensitivePart`.
13. **Five comments said the block was four columns; it is five** — residue from adding
    `band_worse_moves`, and not cosmetic in `cellcsv_regression_test.go`, whose own header warns
    about a synthesised predecessor whose width does not match the build it names. Applied.
    (`cellcsv_test.go:573` was checked and was about the *banking* block being extended, so it was
    corrected to five for a different reason; its neighbouring "the last four of these count
    MOVES" is correct as written and was left.)
14. **The `AGENTS.md` jitter figure read as pre-fix → post-fix and both numbers are pre-fix.**
    +0.339 and +0.357 are two repeats of one sweep at one commit, and +0.357 is the headline quoted
    two bullets above. Applied: reworded as a spread between two pre-fix draws, with the post-fix
    value stated as unmeasured.

## What was declined

**Nothing from either review was declined.** One thing was applied as documentation rather than as
code — Finding 12, the defender's unobserved attacking channel — because closing it properly means
the mediator carrying two signed counts per position, and the block is explicitly a count of
fixtures rather than a proxy for their worth. The omission is now stated in the place a reader of
the column will meet it, and the modelled half is pinned.

Two things the statistics reviewer suggested for *later* are recorded here rather than done:

- **A canary arm** — a `band_strength` large enough that the mediator must move — before any
  tandem sweep is banked, so a flat result can be told from a mediator that cannot respond. Agreed
  and correct; it belongs to whoever runs the sweep, not to this change, which runs none.
- **A per-move "band component of the chosen pair's score delta"** was considered and refused on
  mechanism, not cost: it describes the pair the argmax already chose, so it inherits that
  selection and would read as attribution while being a property of the winner. The refusal is
  written into `stats/README.md` so it is not built later.

## What could not be checked on this harness

- **Whether the tandem is worth anything.** Nothing here measures it, and nothing here claims to.
  The change makes the arm expressible and its null readable; the sweep is a separate act.
- **The magnitude the determinism defect cost, per season.** One repeat gives an occurrence count,
  not a variance estimate. The recorded 0.7 points a season is a single draw and is quoted as one.
- **Attribution — did the bands cause a move.** Unavailable at any affordable cost, not deferred:
  it needs a second full transfer search every week. This is a property of the instrument and is
  stated as such in both the code and `stats/README.md`.

## Proofs run, rather than reasoned about

Two silent-no-op holes were closed and both were demonstrated, because a guard that has never been
seen to fire is not evidence:

- **The determinism fix**: with `bands.go` reverted, `TestBandAssignmentIsDeterministic` reports
  **3 distinct band assignments from 16 byte-identical calls** and fails. With the fix, 1 and
  passes. The test constructs its own boundary tie, so it carries its own positive control rather
  than depending on whichever ties a season happens to hold.
- **The mediator join**: deleting `row.FixtureRuns = fixtureRunsOf(res)` from `runPolicySweep`
  was verified to leave **every other test in the package passing** — exactly the hole a review
  found in the banking block one commit earlier. `TestTheSweepAssignsBothMediators` fails on it,
  and watches the older banking join too, which had the same gap.

## Two tests that were written vacuous, and were caught by running the control

Both were found by deliberately breaking the thing the test watches and confirming it went red.
Recorded because in each case the test *passed* when first written, which is the failure mode that
does not announce itself.

- **The correspondence pin passed with the band map inverted, twice, for two different reasons.**
  First, its fixtures coupled the two ratings — every club scored a constant rate, so goals
  conceded was a strictly decreasing function of goals scored and both band sides gave the same
  answer. `bandSplitFixtures` decouples them (club *i* scores `i + 2j` against club *j*), with a
  guard asserting the decoupling so the construction cannot silently regress. Second, even then it
  looked the band up in `bands` using **its own copy** of `FixtureRunFor`'s position rule — a
  diagnostic carrying a copy of the thing it checks, which this project's standing rules forbid
  outright. It now reads `FixtureRunFor`'s own `Target`/`Avoid` through a one-fixture window. With
  the map inverted it fails on all four cases.
- The mediator join test, as noted below, asserts a composition rather than the sweep's line; the
  source scan is what closes that, and it was verified by deleting the line.

## Build and test

`go build ./...` ok. `go vet ./...` ok. `go test ./...` — **every package ok except
`internal/snapshot`, where `TestSnapshotCoversTheCurrentCode` fails.**

⚠️ An earlier version of this record claimed all packages passed. That was false, and the code
review caught it. The failure is expected and is not this branch's to fix: the accuracy snapshot is
regenerated by the repository owner before merging, and this project's standing practice is never
to commit a snapshot directory to satisfy that test. The base commit `b4e2b61` passes it, so this
branch did open the gap — it is a snapshot that has not been regenerated, not a broken test.
