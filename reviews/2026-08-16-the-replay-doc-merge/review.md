# The replay-doc merge resolution

## What was reviewed

The conflict resolution in `docs/replay.md` when
`worktree-apply-the-replay-doc-findings` merged into `main` at `c584f3f`.

**This record exists because the merge itself is unreviewed content**, not because anything was
re-keyed after a rebase. Two independently reviewed changes touched the same list item, so the
merged tree is covered by neither side's key. The gate firing here was correct.

## The conflict, and how it was resolved

One hunk, in the guard-rails list. Both sides edited the same two bullets, each fixing a different
defect:

| side | what it changed |
|---|---|
| `main` (another session) | the peak-RSS bullet: **145 MB → the 89-142 MB band**, matching the corrected table above it |
| this branch | the exit-status bullet: **"says the word OOM"** → the message the wrapper actually prints, plus the warning to grep `SIGNAL` |

**Resolved as the union: both corrections kept.** Neither overwrites the other — they are adjacent
bullets that git could not auto-merge because they abut. The stale text on *both* sides is gone:
`grep` for `145 MB figure` and for `says the word OOM` each return nothing.

⚠️ **The `145 MB` on this branch's side was itself stale**, superseded by the other session's
correction while this branch was in flight. Taking `main`'s value was not a preference — it is the
later measurement, and carrying this branch's copy forward would have reintroduced a figure the
other session had already retired.

## Which reviewers ran, and which were skipped

| reviewer | why |
|---|---|
| none dispatched | **A union of two already-reviewed hunks, with no new prose.** `fpl-findings-audit` reviewed this branch's side (`reviews/2026-08-16-the-replay-doc-findings/`) and `main`'s side arrived with its own record. The resolution introduces **no sentence that neither reviewer saw** — it selects between two reviewed alternatives and keeps one of each. Recorded as a deliberate skip rather than an omission |

⚠️ **This is the one judgement in this record that could be wrong.** If a future reader finds the
resolution said something neither side said, this skip is where it got through.

## What was applied

Both corrections, verbatim from their respective sides. No third wording was invented.

## What was declined, and why

Nothing was declined. There was no finding here — only a choice between two texts, and the
choice was forced by which figure is later.

## What could not be checked on this harness

Nothing. Whether the stale strings survive the merge is a `grep`, and it was run.

## Verification

`grep -n "145 MB figure\|says the word OOM" docs/replay.md` returns nothing. Zero conflict markers
remain. `go build ./...`, `go vet ./...`, `gofmt -l ./internal ./cmd` (empty) and the full
`go test ./...` all pass on the merged tree.
