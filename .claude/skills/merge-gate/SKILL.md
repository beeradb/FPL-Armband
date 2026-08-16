---
name: merge-gate
description: The conditions that must hold before a branch may be merged to main, and the commands that check each one. Use before merging, and whenever you are about to claim a branch is ready. Binds on every agent that can merge, including the default agent and any feature-building subagent.
---

# The merge gate

`review-gate` establishes that a review happened. **This establishes that everything else did.**

It exists because "ready to merge" was, until it was written down, a judgement each session made its
own way — which means it could not be checked, could not be enforced, and did not survive into a
session that worked differently. Every condition below is mechanical, and **every one of them caught
something real** rather than being written from first principles.

⚠️ **Merging is the one irreversible step.** A wrong commit on a branch is a commit; a wrong commit
on `main` is shared history, and this project's own record is that the remedies are a rewrite of
published refs or an accepted permanent disclosure.

## The gate

Run all of it. A branch is ready when **every** line holds — not most of them.

| # | condition | check |
|---|---|---|
| 1 | build, vet and the whole suite pass | `go build ./... && go vet ./... && go test ./...` |
| 2 | source is formatted | `gofmt -l ./internal ./cmd` is empty |
| 3 | a review record exists and **covers current content** | `TestReviewCoversTheCurrentCode` |
| 4 | the accuracy snapshot covers current content | `TestSnapshotCoversTheCurrentCode` — ⚠️ **being handed to CI, see below** |
| 5 | the resident record is within budget | `TestTheResidentIndexStaysSmall` |
| 6 | every `FPL_*` switch is registered | `TestEnvSwitchListIsComplete` |
| 7 | **0 behind** `origin/main` | `git rev-list --left-right --count origin/main...HEAD` |
| 8 | working tree clean, nothing unpushed | `git status --short`, `git log origin/<branch>..HEAD` |
| 9 | no leak, in **three** channels | see below |
| 10 | the reviewers the triage owes have run | `review-gate`'s table |
| 11 | every finding is applied **or declined with a reason recorded** | the review record's own sections |
| 12 | the paired research-store branch merges too | *merge one, merge the other* |

3 through 6 are tests, so they fail loudly. **7 through 12 are the ones a session skips**, and each
has cost something.

## ⚠️ Condition 4 is being handed to CI, and that is a deliberate weakening

**Do not read this as "the gate is going away".** It is the only thing tying a figure in the record
to the code that produced it, and deleting it would trade away provenance — the thing this whole
line of work exists to keep — in order to fix churn, which is the thing it exists to reduce.

**What is wrong is not the gate but what it compares.** Its digest is over source blobs, so a
comment-only edit trips it and a person pays: eight diagnostics, a render, a committed directory.
**Measured 2026-08-16 across four regenerations in one session, every one forced by a change with no
behavioural content** — the diagnostics take ~90 seconds, and one of the four moved *no figure at
all* except the commit stamp.

So `.github/workflows/snapshot.yml` regenerates and publishes on merge to `main`, and the cost stops
landing on a human. ⚠️ **Note what that does NOT save**: deciding whether a change was inert requires
computing the figures, so the 90 seconds is irreducible and merely moves off your machine.

**Three consequences to know before relying on it:**

- ⚠️ **The provenance arrives one step LATER than it does today.** CI regenerates *after* the merge,
  so a branch is green before its snapshot exists. That is a real weakening and it is the price of
  the churn fix.
- ⚠️ **A failure to publish is quieter than a failing local test.** The workflow going red on `main`
  is the only signal, and nobody is forced to look at it the way a red suite forces you.
- **Once the series leaves the repository**, `NewestKey` finds nothing locally and
  `FPL_SNAPSHOTS_EXTERNAL=1` makes the local gate skip. That flag stops being a placeholder and
  starts describing the actual arrangement — but **a skip is not a pass**, and condition 4 is then
  satisfied by CI or by nothing.

## The three leak channels, because one grep does not cover them

The `pre-receive` hook on the bare repository is the real gate — fail-closed, unbypassable, and it
sees every ref from every machine. **Do not treat it as your check.** It rejects a push *after* you
have built the history you are trying to publish, and a rejection mid-push on a rewritten or
long-lived branch is a bad place to debug from. Check first:

```
git diff origin/main..HEAD | grep -inE '^\+.*(<the store's name>|<its path>)'
git log origin/main..HEAD --format='%B' | grep -inE '<the store's name>'
git branch --show-current          # the ref name reaches snapshot.md and reviews/<date>-<branch>/
```

⚠️ **The branch-name channel is not theoretical and prose review does not catch it** — a branch name
has reached the default branch on its own, carried into a generated header by tooling, with nobody
writing the word. **Name a branch for what it does in the repo.**

⚠️ **A commit message cannot be corrected after it is pushed.** Of the three, it is the one to check
before committing rather than before merging.

## Condition 7 deserves its own note

`0 behind` is not pedantry. This repository has many concurrent sessions, and `main` moved **five
times in one afternoon** during the session that produced this skill. A branch that was level an
hour ago is not level now, and merging a stale branch is how another session's work gets reverted in
a diff nobody reads. **Re-check immediately before merging, not when you start.**

⚠️ And if `origin/main` has moved, **merge it in and re-run the whole gate**. The merge can conflict,
and resolving a conflict is a change like any other — it re-opens 1 through 6.

## Condition 11 is the one that decays

A record that lists what was *owed* and never what it *returned* trains its reader to stop reading
the list. **Mark each finding with its outcome**, including the ones you declined and why — a
declined finding that is not recorded gets re-raised every pass, which is its own tax.

## What this gate does NOT establish

- **That the change is right.** Every condition here is a process fact. A green suite on a
  well-reviewed branch measuring the wrong quantity is still measuring the wrong quantity — this
  project has shipped exactly that, more than once, with the code correct throughout.
- **That the reviewers were right.** A review is a set of proposals, several of which will be wrong.
  Verify before applying, and say in the record that you did.
- **That nothing is owed.** A branch can pass every line here and still leave real work undone; that
  belongs in the queue, not in a merge decision.

## After merging

**Merge the paired research-store branch too, in the same sitting.** They are one unit of work. The
failure this prevents is specific and has happened: the repository half of a change merged, its
research-store twin did not, and shipped `main` pointed readers at a note still carrying the claim
the change had just refuted — for hours, with nothing to notice.
