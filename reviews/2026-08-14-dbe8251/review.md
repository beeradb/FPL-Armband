# The sweep self-verification columns and the `BlendRateK` low-side run

Covers `5a039c1` (the cells-schema block and the floor-population count), `5a683b0` (the
pre-registration) and `dbe8251` (the run and its write-up).

⚠️ **No reviewer agents were dispatched on this branch, and that is a deviation from the standing
habit.** The session's operating instructions forbade dispatching subagents, so this record is a
first-party review. It is written as such rather than presented as an independent one — a review
record that overstates its own independence is worse than a short one. **Anything below is my own
reading of my own diff, and the branch is owed an independent pass before it is treated as
reviewed.** What partly offsets it is that the two defects found here were both caught by *running*
the code rather than by reading it, which is the check this repo says outperforms reviewers.

## The first question: what must this change NOT move?

The change adds columns to a CSV, extracts two predicates, and adds a diagnostic column. **Nothing
about scoring may move.** That is testable and was tested three ways:

1. **The accuracy snapshot.** Regenerated at `5a683b0` and diffed against `2026-08-14-1b7a27b`:
   **only the stamp moves** — commit, branch, timestamp, model path. No model figure changed. This
   is the direct check on the `internal/analysis/squad.go` extraction.
2. **The replay itself.** The four arms `BLENDLO` shares with the banked `BLEND` run come back
   **byte-identical** — 144 cells, `hold_points` and `policy_points`, **0 differences**, across
   `4a56c75` → `5a683b0`. A schema change that had perturbed the engine could not do that.
3. **`go build`, `go vet`, `go test ./...`** all clean.

The `squad.go` edit is the only production-path change on the branch. `Optimize` now calls
`reachesExpectedMinutesCut` instead of inlining the two screens, and `resolvedFodderPrice` /
`scaledMinMinutes` replace three hand-copies. Behaviour-preserving by construction — same
predicates, same order, same short-circuits — and confirmed by (1) and (2).

## Two defects found, both by running rather than reading

**1. `schedule_screen.R` fell silently back to label parsing and printed a false provenance line.**
The declared settings were plumbed into the `blocks[[b]]` list in its *unusable* branch and not in
its *main* one, so `bi$settings` was NULL for every real block. `ladder_of` then took the
pre-column path and printed *"this bank predates the `setting` column"* — a false statement about a
file that carried the column, emitted by the guard written to make provenance honest.

Found by running the screen on the new bank and reading the warning. Fixed twice over: the missing
`settings = settings`, and `ladder_of`'s argument is now **required with no default**, so a call
site that forgets is an R error. Absence must be a property of the file — which reads as all-NA —
never of the caller.

⚠️ **This is the branch's own lesson turned on itself.** The whole point of the `setting` column is
that a provenance claim should come from the data rather than from something a caller remembered to
do, and the first implementation reproduced exactly that failure one layer up.

**2. `fodderPrice` and the scaled `minMinutes` were re-derived a third time.** Adding the exported
`ReachesExpectedMinutesCut` copied both resolutions out of `Optimize`, beside the copy already in
`CutByExpectedMinutesFloor`. Caught on a read of my own diff before commit; both are now single
helpers. This is the package's signature failure arriving by its usual route — not a rewrite, just
one more caller needing the same number.

## Schema: the part most likely to break something later

The cells file is append-only and its header is the compatibility contract, so column *position* is
load-bearing. The new block sits **before** the oracle pair, which is the only position that keeps
`TestOracleColumnsAreLastAndCounted` true and keeps the stale-header test's predecessor synthesis
meaningful.

- `armCols = 3` and `TestTheArmBlockIsBeforeTheOracleBlockAndCounted` pin position and length. The
  new test asserts the columns *between* the chip block and the oracle pair, which is the gap
  neither existing end-anchored test covered.
- `TestAppendingUnderAWrongSchemaIsRefused` gained the new stripping level. ⚠️ Its existing comment
  already warned that a mislabelled entry there still passes, because every synthesised header
  differs from the current one; the labels are now correct but the test still cannot detect a wrong
  one. Unchanged risk, not a new one.
- Blank rules are asserted per column rather than assumed: no declared setting is a **gap**, a
  `setting` of 0 is a **rung**, the floor is never blank, and `squad_hash` is cleared by
  `asInfeasible` because it is a measurement while the other two describe the arm.

## Judgement calls a reviewer should second-guess

- **The label parse was kept.** One correct path would say delete it. It survives because every
  committed bank predates the column, including the one the screen's recorded 7-ladder result was
  measured on, and deleting it would make that result irreproducible from its own bank. Verified
  the committed banks still run and `BLEND#1` reproduces to the digit. A reviewer may reasonably
  prefer deletion plus a re-run of the bank.
- **`entered` stays keyed on the whole-field removed set** in `TestDiagFloorPopulation`, not on the
  new `pool` set. The smaller set would make `entered` smaller and let *more* cells claim they can
  convict the shipped search; the superset is conservative for a guard. Stated in the source.
- **`BLENDLO` re-runs four arms that were already banked.** Deliberate — R keys on
  `(run_id, sweep)` so two runs cannot be pooled — and it bought the byte-identity check above. It
  costs six minutes.
- **`BLEND2`'s setting getter panics** if the two anchors disagree. A panic inside a sweep is
  aggressive; it is there because that arm's whole premise is that both move together, and a slope
  reported over a setting half the cells did not have is the failure the column exists to prevent.

## Claims checked against the numbers

- "Nothing resolves" — Holm 1.000 on all five arms, largest |t| 0.76, against per-arm thresholds of
  27-48 a season from `variance_components.R`. ✔
- "The low side is flat" — stated as point estimates with the thresholds beside them, and the
  write-up explicitly refuses the stronger reading ("not *the low side is worth nothing*"). ✔
- "Not corroboration" — the byte-identity check is what licenses this, and it is quoted with its
  denominator (144 cells). ✔
- The `k=16 minus k=12` contrast is reported as **not a result** despite t 3.94, on the positive-cell
  count (17 of 36) and the three-cell concentration. ⚠️ Worth a second opinion: the leave-one-
  season-out table (LOSO — re-compute the mean six times, each time dropping one season, and see
  whether the answer survives) is stable, so a reader applying that check alone would reach the opposite
  conclusion. That is why it is queued in TODO.md as a general lesson rather than left in the run.

## Gates

`go build`, `go vet`, `go test ./...` clean. Accuracy snapshot regenerated. ⚠️ **CLAUDE.md is at
exactly 69,632 bytes of a 69,632-byte budget — zero headroom**, so the next branch to touch it must
cut first. Paid for here by compressing the two `BlendRateK` bullets, whose detail is now in
`docs/notes/constants-and-sweeps.md`.
