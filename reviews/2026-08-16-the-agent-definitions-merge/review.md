# The agent-definitions merge

## What was reviewed

The merge of `worktree-move-the-agent-definitions` into `main`. **No conflict, and no new prose** —
this record exists because the merged tree is covered by neither side's key, not because anything
was authored.

## Why the gate fired

`internal/analysis`, `internal/backtest`, `stats` and `docs` moved. ⚠️ **None of those is a tree
this branch touched** — the branch changed `.claude/`, `internal/snapshot/reviewgate_test.go` and
its own review record. They moved because **`main` advanced underneath the branch** while it was in
flight, carrying another session's separately-reviewed work.

So the union is new even though neither side is: my key covered my branch's index, theirs covered
theirs, and nothing had digested both together. **The gate is right to fire and there is nothing to
re-review** — git merged both sides cleanly and no line was hand-resolved.

⚠️ **This is the second time today this has happened on a `main` that is moving under concurrent
sessions**, and it is worth naming as a pattern rather than treating as an incident: on a busy
`main`, every merge of a reviewed branch produces a tree that owes a re-key, and the re-key is
bookkeeping unless a conflict was resolved by hand. **When a conflict IS hand-resolved, the
resolution is new content and does owe a review** — that was the earlier case today
(`reviews/2026-08-16-the-replay-doc-merge/`), and this is not.

## Which reviewers ran, and which were skipped

| reviewer | why |
|---|---|
| none | **A clean union of two already-reviewed sets, with no hand-resolved hunk.** Recorded as a deliberate skip. The branch's own review is `reviews/2026-08-16-move-the-agent-definitions/`; the incoming side arrived with its own |

## What was applied / declined

Nothing on either count. There was no finding — only a re-key.

## What could not be checked on this harness

Whether the incoming side's review was adequate. **This record makes no claim about it** — it
asserts only that this merge added nothing to review.

## Verification

`go build ./...`, `go vet ./...`, `gofmt -l ./internal ./cmd` (empty) and the full `go test ./...`
pass on the merged tree. `git ls-tree -r --name-only HEAD -- .claude` returns hooks, settings and
the two skills only — the eight agent definitions are gone from the tracked tree, which was the
point of the branch.
