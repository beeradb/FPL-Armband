# Review — A2, the cells-reader consolidation

**Commit range**: `5986103..HEAD` on `worktree-cells-reader-consolidation`, branched from `main`
and a fast-forward onto it.

**What the range does.** Consolidates every R reader of the replay's cells CSV onto
`stats/cells_common.R`, fixing a silent cross-block pairing defect, and extends the guard so the
next copy cannot appear.

## The invariant came first, and it was the wrong one

The acceptance test this item proposed — run every script on committed cells before and after,
require identical output — was run and passes. ⚠️ **It has no power over what the consolidation
fixed.** Measured across the bank before starting: 72 files, 7,320 rows, 82 blocks, **0 infeasible
rows**, **0 empty character fields**, `is_baseline` character in **all 72**. Every path the readers
differed on is dark there.

That is the same trap the defcon prior paid for one item earlier. So the branch carries two checks
answering two questions: the before/after as a **regression** check, and
`stats/cells_reader_selftest.R` — a synthetic fixture over the four dark paths — as the one with
power over the fix.

## Reviewers dispatched

| reviewer | why | outcome |
|---|---|---|
| **fpl-stats-review** | ran on the **plan**, before any code | refuted both of the item's claimed defects; found the real one |
| **fpl-code-review** | R inference layer + a guard change | 9 findings, **1 a regression I introduced that printed as a pass** |
| **fpl-findings-audit** | TODO.md and heavy in-source documentation | 11 findings, **1 a break in a banked reproduction script** |
| fpl-security-review | **skipped** — no Go production code, no credential or config path | — |
| fpl-run-review | **skipped** — no live run | — |
| fpl-season-maintenance | **skipped** — the four lists are untouched | — |

## Findings, ranked

### 1. I introduced a defect worse than the one I fixed — CONFIRMED, fixed

`grid_width.R`'s nesting check merges `pos6` against `pos4` — **two separate files from two
separate runs**, whose `run_id` is `unix-seconds-pid` and therefore never matches. Widening `cell`
to embed `(run_id, sweep)` made that join permanently empty, and an empty join takes the
`nrow(bad) == 0` branch and prints **"byte-identical"**.

The failure printing as the pass, on the script's only documented invocation. Worse than the
original defect, which only bit on a misuse nobody had committed. Fixed by joining on `label` —
the display key the migration separated out precisely for this — plus a hard failure on an empty
merge. Verified on synthetic non-nested inputs: **48 cells compared, 48 disagree, NOT NESTED**,
where before the fix it reported 0 compared and byte-identical.

**The lesson is narrower than "widening a key is risky": a key that embeds provenance is right for
a join WITHIN a file and wrong for one BETWEEN files, and this family contains both.**

### 2. A2 broke two banked reproduction scripts — CONFIRMED, fixed

`min_cells` with no default is right; its cost is that every *existing* caller must be found.
`stats/snapshots/2026-08-14-blend/sensitivity.R` called `diffs_for` positionally and **aborted**.
A script whose stated purpose is reproducibility, unable to run.

### 3. There were nine readers, not seven — CONFIRMED, fixed

The guard globbed `stats/*.R` **non-recursively**, so two readers under
`stats/snapshots/*/sensitivity.R` were structurally invisible — one sourcing `cells_common.R` then
reading raw with the narrow key, one reading raw with no coercion at all. **A2's own "every count
is a floor" landing on A2.** The glob now walks `stats/` recursively; both migrated with identical
output.

### 4. "Three different experiments" was wrong — CONFIRMED, corrected

I claimed the banked `MINHL` file's three blocks are three different experiments. Measured:
`MINHL#1` has **zero arms** — a lone baseline, not an experiment — and its baseline is
**byte-identical** to `MINHL#2`'s across the same 24 cells. Only `MINHL#3` differs. **The whole
0.1434 shift comes from one block.**

Two consequences now recorded: the shift is **constant within a block**, so the ladder's ordering
and every arm-to-arm gap survived while every level, SE and t moved; and a file whose blocks
**share** a baseline shows n tripling with **nothing moving**, which is why this went unnoticed.

### 5. My retraction over-retracted — CONFIRMED, corrected

I wrote that `entry_density.R`'s coercion was fine. Two of the three spellings, yes — but **`1`/`0`
does break it**: R types it integer, and `1 %in% c("TRUE","true")` is `FALSE`, giving that script
zero baselines with no error. My own fixture proves it.

### 6. Guard gaps — CONFIRMED, fixed

The `shared` name list omitted `read_cells`/`read_cells_all`/`read_sidecar` — so a re-added local
reader would pass **both** scans and reinstate the exact defect A2 removed. The regexp covered only
`read.csv`, missing `read.table`, `read.delim`, `read_csv` and `fread`.

### 7. Smaller, all fixed

`read_cells_all` silently intersected columns where `rbind` used to abort loudly — it now says what
it dropped. `grid_width.R` passed `quiet = TRUE`, making its own comment about reporting swallowed
arms false. `variance_components.R`'s selftest re-narrowed the key it had just read — the
`grid_width` defect in miniature. `shape_inference.R`'s invariance listing printed the wide key at
exactly the moment someone most needs to read it. `stats/README.md` never mentioned
`cells_common.R` at all.

## Declined, with reasons

- **Filing the `grid_width` defect under CLAUDE.md's "Things that have already bitten."** That
  section is shipped bugs with a regression test each. This is a reachable misuse of a developer
  tool that no banked figure went through, and filing it there would make it quotable as a
  contamination event. Recorded in A2 and in the guard instead — and the *decision* is recorded, so
  it is not re-proposed.
- **Running the selftest from Go.** `go build`/`vet`/`test` must pass with no R installed. The
  guard asserts the file exists; that is weaker than running it and is the most that is available.
- **Adding the "banked cells have no power" finding to CLAUDE.md as a new rule.** It is the third
  instance of a rule already resident ("a byte-identical season under an intervention is not a
  tie"). The worked case lives in A2 and the selftest header.

## What could not be checked on this harness

- **Whether `grid_width.R` was ever run on a multi-block file.** Its inputs are not banked, so "no
  banked figure moved" is an **argument** from its calling convention, not a measurement — stated
  that way in A2 so it cannot harden. For BLEND and BLENDLO it *is* measured: both single-block.
- **`grid_width.R` end to end on real data**, for the same reason. The nesting fix was verified on
  synthetic fixtures instead.
