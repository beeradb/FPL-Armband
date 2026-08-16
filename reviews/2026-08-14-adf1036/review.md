# The floor-population count, and three items that were never queued

Covers `adf1036`.

⚠️ **First-party.** The three dispatched reviewers ran against `a70f7af`; this commit applies work
they asked for plus one measurement, and is small enough that re-dispatching would review a
reviewed branch. **The branch's substantive review is `reviews/2026-08-14-a70f7af/review.md`** and
this record does not replace it.

## The measurement

`DIAG=1 TestDiagFloorPopulation`, six-season grid, output banked at
`stats/snapshots/2026-08-14-minfloor/floorpopulation-2026-08-14.txt`.

**229.7 removed a build whole-field; 114.7 of those still in the pool when the floor ran.** The
recorded 96-126 is the pool figure and 114.7 sits mid-range.

**This settles an open contradiction by dissolving it rather than deciding it.** The record carried
the two numbers as disagreeing and said neither was safe alone. They were measuring different
populations: the whole-field figure counts everyone failing the floor predicate over `AllMetrics()`,
including players `Optimize`'s total-minutes floor and availability screen had already removed. The
`pool` column added earlier today is the difference.

⚠️ **What it does not settle.** The floor's *value* is untouched — the mediator is still 2 of 36
cells, and the argmax check is still **0 of 1 discriminating builds**, a denominator of one. Nothing
here argues for or against 55.

## The three queued items, and why they were missing

⚠️ **`reviews/2026-08-14-a70f7af/review.md` says the wild cluster bootstrap was "queued". It was
not.** I wrote that it was declined-and-queued and then queued nothing. This commit adds it, and the
TODO entry says so in its own text so the correction is visible where someone would look.

That matters more than the omission: the bootstrap is the item most likely to **retract existing
findings**. The cells are spike-and-slab (pooled excess kurtosis +3.46) so the CLT is slow on 36 of
them, and the worked case reads CR2 t 3.94 while bootstrapping to **p = 0.0625**.

The other two are smaller and both structural:

- **The shared-quantities guard cannot see an inlined copy.** It matches `name <- function(`, so a
  script pasting a body rather than redefining is invisible. `concentration_screen.R` did that with
  `as_flag`, and the guard passed. Fixed at the call site by sourcing `cells_common.R`; ⚠️ **the
  guard is unchanged and would miss the next one.** The cheaper fix queued is a positive check that
  every `stats/*.R` reading a cells file sources the common file.
- **Four different minimum-cell thresholds** (2, 4, 2, and now a named 6), all dropping arms
  silently — the defect already recorded against `grid_width.R`.

## Gates

`go build`, `go vet`, `go test ./...` clean. `CLAUDE.md` 69,467 of 69,632 — the reconciliation is
shorter than the "unreconciled" text it replaces. No snapshot needed: no watched source file moved,
and the R and TODO changes are not on the scoring path.
