# The cells-and-findings merge

## What was reviewed

The merge of `worktree-relocate-the-banked-cells-the-r-screens-read` into `main`. **Clean merge, no
conflict, no new prose** — this record exists because the merged tree is covered by neither side's
key, not because anything was authored.

## Why the gate fired

`internal/backtest`, `internal/snapshot`, `stats` and `docs` moved. **`main` advanced underneath the
branch** while it was in flight, carrying other sessions' separately-reviewed work. The union is new
even though neither side is.

⚠️ **Third time today on this `main`.** The pattern is now established and worth stating rather than
re-diagnosing: on a `main` moving under concurrent sessions, every merge of a reviewed branch owes a
re-key, and that re-key is **bookkeeping** unless a hunk was hand-resolved. When one is, the
resolution is new content and does owe a review — that was `reviews/2026-08-16-the-replay-doc-merge/`.
This is not.

## Which reviewers ran, and which were skipped

| reviewer | why |
|---|---|
| none | a clean union of two already-reviewed sets, no hand-resolved hunk. The branch's own review is `reviews/2026-08-16-relocate-the-banked-cells/` |

## What was applied / declined

Nothing on either count. A re-key only.

## What could not be checked

Whether the incoming side's review was adequate. This record asserts only that the merge itself
added nothing to review.

## Verification

`go test ./...` passes on the merged tree.
