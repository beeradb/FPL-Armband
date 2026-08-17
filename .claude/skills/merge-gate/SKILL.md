---
name: merge-gate
description: The conditions that must hold before a branch may be merged to main, and the commands that check each one. Use before merging, and whenever you are about to claim a branch is ready. Binds on every agent that can merge, including the default agent and any feature-building subagent.
---

# The merge gate

A branch is ready when **every** condition holds. Not most of them.

| # | condition | check |
|---|---|---|
| 1 | build, vet and the whole suite pass | `go build ./... && go vet ./... && go test ./...` |
| 2 | source is formatted | `gofmt -l ./internal ./cmd` is empty |
| 3 | a review record exists and covers current content | `TestReviewCoversTheCurrentCode` |
| 4 | the accuracy snapshot covers current content | `TestSnapshotCoversTheCurrentCode` — see below |
| 5 | the resident record is within budget | `TestTheResidentIndexStaysSmall` |
| 6 | every `FPL_*` switch is registered | `TestEnvSwitchListIsComplete` |
| 7 | **0 behind** `origin/main` | `git rev-list --left-right --count origin/main...HEAD` |
| 8 | working tree clean, nothing unpushed | `git status --short`, `git log origin/<branch>..HEAD` |
| 9 | no leak, in three channels | below |
| 10 | the reviewers the triage owes have run | `review-gate`'s table |
| 11 | every finding applied **or declined with a reason recorded** | the review record |
| 12 | the paired branch for this work, where one exists, merges in the same sitting | one unit of work |

3 through 6 are tests and fail loudly. 7 through 12 are the ones that get skipped.

## Condition 7

Re-check **immediately before merging**, not when you start. If `origin/main` has moved, merge it
in and re-run the whole gate — resolving a conflict re-opens 1 through 6.

## Condition 4

`.github/workflows/snapshot.yml` regenerates on push to `main` and publishes the result as a
release asset — it does not commit one. A failure to publish shows only as a red workflow on
`main`.

**The committed series is still what this test reads, so today condition 4 is satisfied by
regenerating and committing a snapshot.** `FPL_SNAPSHOTS_EXTERNAL=1` skips the local check **only**
once no keyed snapshot is committed at all; that is not this repository's state, and the flag does
nothing until it is.

Deciding a change was inert requires computing the figures, so that route is not a free
alternative to regenerating. If you rely on CI instead, say so in the review record — **a skip is
not a pass**, and condition 4 is then satisfied by CI or by nothing.

## Condition 9 — three channels

```
git diff origin/main..HEAD | grep -inE '^\+.*(<the withheld name>|<its path>)'
git log origin/main..HEAD --format='%B' | grep -inE '<the withheld name>'
git branch --show-current
```

A branch name reaches record directory names and merge commit subjects on its own, with nobody
typing it. **Name a branch for what it does in the repo.** A commit message cannot be corrected
after it is pushed, so check that channel before committing rather than before merging.

The `pre-receive` hook is fail-closed and unbypassable, but it rejects after you have built the
history you are trying to publish. Check first.

## Never

- Force-push, or merge with `--allow-unrelated-histories`.
- Delete a paid-for line to fit a budget. Raise the budget in the same commit and name the claim
  that needed the room.
- Re-key a review record because history was rewritten. If the gate still fires, real content
  moved — read what `key.csv` says moved and review it.

## What this gate does not establish

That the change is right, that the reviewers were right, or that nothing is owed. Every condition
here is a process fact. A branch can pass every line and still leave work owed; record that
rather than blocking the merge on it.
