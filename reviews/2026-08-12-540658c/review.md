# Review record — regenerating the accuracy snapshot

**Commit reviewed:** the snapshot regeneration on `flag-refit`, adding
`stats/snapshots/2026-08-12-705897c/`. Previous record:
[`2026-08-12-0e17af0`](../2026-08-12-0e17af0/review.md).

## Reviewers dispatched: none, and why

The three watched files are `constants.csv`, `figures.csv` and `snapshot.md` — a **generated
artefact**, produced by the documented two-command procedure and diffed against its predecessor.
There is no judgement in it to review: the diagnostics that produce it were reviewed where they
live, and the renderer is unchanged. A recorded "not applicable", in the form the gate asks for, so
the next pass does not re-ask.

The change that *prompted* the regeneration — `backtest.EngineAt` — was reviewed at
[`2026-08-12-0e17af0`](../2026-08-12-0e17af0/review.md) by `fpl-code-review`.

## What the diff shows, and the correction it forces

The commit message said `EngineAt` "adds no computation, so the model figures should be unchanged
and the snapshot is here to prove that". **The first half is right and the second half is wrong**,
and the diff is the evidence.

`EngineAt` is called by **`cmd/flagfit` and nothing else** — verified by grep across the tree. It
touches no scoring path and no diagnostic, so it cannot move a model figure.

But the snapshot moved a great deal: **741 differing lines and 57 more rows** than its predecessor
at `fe2ab99`. That is not `EngineAt`. `fe2ab99` is an ancestor of this branch, and the span between
them contains **the six-season grid flip**. The diagnostics behind the model half — calibration
drift among them — call `sweepPairNames()`, so widening the default from four seasons to six
changed the population every one of them reports on.

## ⚠️ The finding: the grid flip should have invalidated the snapshot and could not

`TestSnapshotCoversTheCurrentCode` scans watched files and **excludes anything ending `_test.go`**
(`staleness_test.go:144`). `sweepPairNames()` lives in `internal/backtest/harness_test.go`. So the
default replay grid — the single setting that determines which football every diagnostic measures —
sits in a file the staleness guard is structurally unable to see.

The consequence is that the accuracy snapshot on `main` has been describing a **four-season model**
since the default became six, with nothing saying so, and this regeneration is the first snapshot
taken on the new grid. Nobody noticed because the guard that exists to notice cannot.

The exclusion is not wrong in general — its stated purpose is that "adding a diagnostic does not
trip this" — but it assumes test files do not change what the model computes, and in this repository
one of them decides it. Two candidate fixes, neither applied here:

- watch `harness_test.go` specifically, as a named exception to the test-file exclusion; or
- move the grid declaration out of a `_test.go` file, which is the deeper fix and a larger change.

**Recorded rather than fixed**, because it is a guard change affecting what every future commit must
regenerate, and it belongs in its own reviewed diff rather than appended to a snapshot commit.

## What was applied

The regeneration itself, by the documented procedure — the model half into a scratch CSV, then the
renderer with `-model` pointed at the same path, which the guard's own note warns is the step people
get wrong.

`constants.csv` is **byte-identical** to its predecessor: 0 differing lines. No constant moved, which
is the invariant worth having, and it confirms this is a population change rather than a scoring one.

## What was declined

- **Dispatching reviewers on a generated artefact.** Nothing to judge.
- **Fixing the guard gap.** Recorded above; its own change.
- **Re-deriving anything from the new figures.** They are the first six-season readings of those
  diagnostics and they supersede the four-season ones *as six-season figures*, not as corrections —
  the same distinction the canonical block already draws for the grid change generally.

## What could not be checked

Whether any individual model figure moved *because of the grid* rather than for another reason in
the span is **unmeasured**; separating that needs a regeneration at the flip's parent commit, which
is cheap (about seventeen seconds for the model half) and was not run.
