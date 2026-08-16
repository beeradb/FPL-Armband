# Banking the chip-week oracle's per-cell readings

## What was reviewed

One change on `tier-0-queue-sweep`, staged against `8c05c69`: a ten-column block added to the
per-cell cells CSV carrying the chip-week oracle's three readings of each scoring chip — the
hindsight week and its gain, the median week's gain, the threshold rule's gain, and the bar that
rule ran against. Plus the derivation (`chipReadingsOf`), its wiring into `runPolicySweep`, two
regression tests, a rebuild of the stale-header chain, and the schema documentation in
`stats/README.md`.

The motivating defect: `TestDiagChipWeekOracle` printed those readings and wrote them to no
machine-readable file, so the quotable form was six means over the grid with no dispersion, and
the comparison could not be given a standard error. **This is plumbing. No sweep was run, no
figure produced, no constant moved.**

## Reviewers

| reviewer | why |
|---|---|
| **fpl-stats-review** | triage: the change touches `internal/backtest` and the inference contract |
| **fpl-code-review** | not required by triage, dispatched anyway: a schema change whose header and writer are positional mirrors is the shape of two bugs this package has already shipped |
| **fpl-findings-audit** | triage: `internal/backtest` and written record |

Skipped: **fpl-security-review** (no client, agent or config-persistence surface),
**fpl-run-review** (no live run), **fpl-season-maintenance** (no hand-maintained list),
**fpl-docs-accuracy** (`docs/` untouched; `stats/README.md` went to the findings auditor with the
rest of the prose).

Before dispatching: the invariant question first, per this skill. What must this change not move
is *every other sweep's output*, and that is structural rather than asserted — `res.ChipOracle` is
nil unless `AxisChipWeek` is on, so every other sweep writes ten blanks. The code reviewer
verified that independently rather than taking it from the brief.

## Findings, ranked by how misleading the state was

### 1. Three entries in the stale-header chain named builds that never existed. APPLIED.

`TestAppendingUnderAWrongSchemaIsRefused` synthesises older schemas by stripping blocks. Its own
comments record having twice carried mislabelled entries that still passed, because *every*
synthesised header differs from the current one and is refused whatever it contains. The first
version of this change asserted in prose that the chain was now honest, and it was honest for
exactly the two entries it touched.

Truncation is only a valid synthesis for a block appended after the current last block, and
nothing has been since the oracle pair landed. Verified against history rather than reasoned
about — `git show <commit>:internal/backtest/cellcsv_test.go` gives 40, 36, 33, 29, 27 and 23
columns, and only the last two end anywhere but `oracle_kind`. The synthesised 31-column header
ending at `triple_captain_pts` is one no commit ever wrote.

Fixed by removing each block from where it actually sits, and by pinning each entry's width and
final column to the historical fact — which is the check the chain has been missing through three
occurrences, since nothing else in the test can tell a right header from a wrong one.

### 2. A doc comment named `withChipOracle`, which exists nowhere. APPLIED.

Left behind by a rename during the change. The writer is `runPolicySweep`; the derivation is
`chipReadingsOf`. A reader following the pointer to check whether banking really happens would
have found nothing.

### 3. The threshold column was uninterpretable without its bar. APPLIED — two columns added.

`firstClearing` returns the first gain at or above the bar, else the final week's, so
`threshold >= bar` is exactly "a week cleared" and `threshold < bar` exactly the forced
end-of-season spend. Without the bar in the file that partition is unrecoverable and the column is
a mixture of two rules. The cells file is opened for append and stacked across runs, so a bar
edited between two runs would pool two quantities in one column with nothing marking it. A commit
in the provenance sidecar does not close this: it names a build, and this record already carries a
snapshot whose figures its own named commit does not produce.

Same rule the arm block states — the file must verify what produced its columns rather than a
reader verifying it by reading Go — and the same shape as `min_expected_minutes`, which is also
constant within a run.

### 4. The truncation-bias comment had its sign inverted, in the flattering direction. APPLIED.

The file defines PLAYING IT AT ALL as `oracle − median`. Truncating the median toward zero raises
that difference for a positive median; the comment said it lowered it. The act (float64) was right
and its stated reason was backwards — the same correction `as_flag` in `stats/cells_common.R`
already carries. An inherited "up to half a point, which is the whole size of the quantity that
row reports" went with it: no run has resolved either chip difference, so the comparison was an
assertion.

### 5. "Banking the run would not have helped" was too strong, and so was the queue item. APPLIED.

The printed grid has always carried one line per cell, and this repository banks sweep stdout —
26 `.log` files under `stats/snapshots/*/cells/`. So the dispersion was not unrecorded in
principle. The accurate and narrower claim: nothing in `stats/` reads stdout, and this sweep's own
banked cells carry none of the readings because the schema had no columns for them.

The original wording also cited run logs in a temporary directory, which is not in this checkout
and cannot be checked by a later reader. Replaced with the in-checkout file that makes the point
harder.

### 6. A printed line instructed the reader to make the borrowing the record retracted. APPLIED.

`reportChipCells` ended "compare them against this harness's season-scale detection threshold
directly". A threshold belongs to a comparison, and these two have none of their own — which is
what the retraction of the borrowed figure beside the chip-timing result says. The unit warning is
kept (these are one gameweek's points: do not divide by weeks, do not multiply by 38); the
instruction to borrow is gone, with the change marked in place.

### 7. Estimand: the oracle reading is a maximum over the played window. APPLIED to `stats/README.md`.

38 candidate weeks entering at GW1, 13 at GW26, while the median is close to window-invariant. So
`oracle − median` rises with the window by construction and a pooled mean over the grid mixes six
estimands; `oracle − threshold` is distorted too, in an ambiguous direction, since a longer window
both raises the maximum and makes the bar likelier to clear. The record already carries this on
this exact quantity and says to read the start-point column. It also means the six cells of one
season re-read nested windows of the same football, so a cell-level standard error over the grid
is invalid rather than merely optimistic — cluster on season. `start_gw` and `weeks` are both
banked, so nothing is owed in the schema.

### 8. Precision on the prose. ALL APPLIED.

- "blank for every sweep that is not the chip-week oracle" is a property of the **arm**: the
  baseline arm of the oracle's own sweep banks blanks, which is what the new test asserts.
- The `_gw` columns are gameweek numbers, not points, so the never-divide rule reads oddly over
  them. Split the sentence.
- The cell key omits `variant`: the four-tuple selects two rows in this sweep, one blank by design.
- "a different and **larger** quantity" is not forced. `oracle − median` exceeds
  `oracle − threshold` exactly when the threshold reading exceeds the median, and the fallback can
  land below it. Different question; ordering unmeasured.
- The median is not the expected value of a randomly chosen week — that is the mean, and it sits
  higher on a right-skewed series, which this one is precisely because the argmax is chasing one
  big week.
- "A row carrying them always carries `decision:chipweek`, which is the check" — true by
  construction, but nothing asserts the implication, so it is what a reader should filter on
  rather than a guarantee. Reworded.
- The argmax takes the **first** week on a tie, so `*_oracle_gw` is biased early by construction.
  Recorded beside the column.
- Ordinary sweeps plan no chips, so the pre-existing chip pair is `0` everywhere in them —
  otherwise `bench_boost_pts = 0` reads as "played and returned nothing".
- The fixture sentence in `stats/README.md` said the schema "has grown four times since" — five,
  counting this change, and it would need editing at every future block. Rewritten to quote only
  the fixture's own width, which cannot move.
- `chipReadingsOf` cites `ChipWeek`'s reason for keeping the median and threshold off the engine
  struct. That separation is genuinely weaker in the file, where all three sit on the oracled
  arm's row and only the column names keep them apart. Said out loud.

## What was declined

**Blanking the block on a POLICY-incomparable season.** The gains are computed on the transfer
path, and `runPolicySweep` refuses to report a pooled POLICY mean for a season whose transfer path
is not a sample of the same process. The hazard is real: a reader forming this within-row contrast
does not go through `sweep_inference.R`, so its row-dropping does not protect them. But blanking
would be inconsistent with how the schema treats the metric it is arguing from — `policy_points`
**is** banked for exactly those seasons, and the refusal is about reporting rather than recording.
The schema's own rule elsewhere is that a cell emits its row and the reader conditions. The season
is on every row. Documented rather than blanked.

**Strengthening the argmax mediator from "at least 2 distinct weeks" to a concentration measure**,
and checking the triple-captain argmax as well as the bench boost's. Correct that the current
guard is weak. Out of scope for a plumbing item, and the reason it can wait is the change itself:
the chosen week is now a banked column, so concentration is computable off the file rather than
only inside the diagnostic.

**Annotating CLAUDE.md.** Proposed wording is below and the underlying finding is real, but a
record edit is a separate act from plumbing and the second half of it deserves its own item rather
than riding in this commit.

## Handed on rather than fixed here

**The chip-timing bullet's figures have no banked cells.** CLAUDE.md quotes the scoring-chip
timing result at 36 cells. `grep -rl "decision:chipweek" stats/snapshots/` returns exactly one
run — `2026-08-12-4d61058`, whose `oraclechip.csv` is 48 rows: two arms over 24 cells, four
seasons by six starts. So neither the 36-cell invariance nor the levels beside it can be
re-derived from this checkout. This is the standing rule about a constant having been swept not
meaning its cells were banked, and it is a larger gap than the schema one this change closes.

The annotation proposed for that bullet, asserting only facts about the checkout:

> ⚠️ Plumbing only, nothing re-measured. The mechanism behind "no threshold of its own" is that
> the three readings per chip were printed per cell and written to no machine-readable file, so no
> SE could be formed from anything committed. Ten per-cell columns now exist
> (`bench_boost_oracle_gw` and its siblings); that makes a threshold **obtainable and does not
> supply one** — no sweep has run under the schema and `stats/sweep_inference.R` does not read the
> columns. ⚠️ And this bullet's own figures are unbanked: the only banked chip-week-oracle run is
> `stats/snapshots/2026-08-12-4d61058/cells/oraclechip.csv`, 24 cells over four seasons, whose
> header predates the block.

## What could not be checked on this harness

- **Whether the readings are worth what the record says they are.** Nothing here re-measures
  anything; a run under this schema is what would answer it, on the per-cell values now banked.
- **The wiring end to end.** `runPolicySweep`'s call is exercised only by a DIAG sweep against the
  live archive, so the tests cover the derivation and the file format and the call site is
  verified by reading. The derivation's arithmetic — median not truncated, threshold falling back
  to the final week rather than zero, bars banked as run — is pinned without the API.
- **Whether the skew in these columns breaks the inference machinery's assumptions.** Both
  contrasts are maximum-minus-something and right-skewed at the cell level, and averaging six
  nested correlated cells into a season mean does not remove that; CR2's normality and the wild
  bootstrap's symmetry assumption are both weaker here than on this record's usual arms. Noted for
  whoever quotes a figure; the per-cell values are what make it checkable.

## Notes, no fix owed

- `placeChips` returns a non-nil `*ChipOracle` with zeroed weeks for an empty season, so a
  zero-gameweek cell would bank the block with every reading zero. Unreachable from
  `runPolicySweep` — `Simulate` errors rather than returning a zero-week season, and that path
  ends in `asInfeasible`, which clears the block.
- `floatOrBlank` never returns a blank despite its name. Correct for this block, where a measured
  median of zero must be written as `0`, and pre-existing.
