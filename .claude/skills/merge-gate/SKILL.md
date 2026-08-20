---
name: merge-gate
description: The conditions that must hold before a branch may be merged to main, and the commands that check each one. Use before merging, and whenever you are about to claim a branch is ready. Binds on every agent that can merge, including the default agent and any feature-building subagent.
---

# The merge gate

A branch is ready when **every** condition holds. Not most of them.

| # | condition | check |
|---|---|---|
| 1 | build, vet and the whole suite pass **on the commit being merged**, with a run on record | `go build ./... && go vet ./... && go test ./...`, then the CI run — see below |
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

## Condition 1

Run the whole suite, on the commit you are merging. **Do not narrow it** — not by package, not by
`-run`, not by a scoped wrapper. That was tried on 2026-08-19 and refused: Go's test cache already
re-runs the packages a change reaches — the changed package, everything importing it, and
anything whose tests *open* a changed file **within the module**, which is `go help test`'s own
boundary — and it tracks the cross-package source scans an import graph cannot see, so any hand-derived scope skips precisely the guards this project pins
its shipped bugs with. ⚠️ **If the suite seems to re-run everything every time, check `df -h /`
before you believe it** — Go silently declines to cache a result it cannot write.

⚠️ **Run it `-count=1` HERE, though nowhere else.** The cache keys on files a test *opens*, and
`internal/snapshot`'s guards read git through a subprocess — so a `HEAD` move they would fail on
is invisible to the cache and conditions 3 and 4 come back green on a stale result. Measured: with
five modified files and a new `reviews/` directory uncommitted, the package passed, because it
digests the **committed** tree.

⚠️ **And run it AFTER committing, not before.** Conditions 3 and 4 digest git `HEAD`. A suite run
on an uncommitted working tree evaluates them against whatever `HEAD` still points at — which,
before the first commit on a branch, is `origin/main`. That run is green and **vacuous** for both.
The order is: commit → `reviewkey` → commit the record → run the suite `-count=1`.

Two witnesses, and only the second is checkable by someone who was not there:

- the local `go test ./...`, at the head commit, *after* the last `origin/main` merge — condition
  7 re-opens this one;
- the CI run keyed to that commit. `.github/workflows/ci.yml` runs the whole suite on every
  pushed branch and every pull request:

      git push && gh run list --workflow CI --commit "$(git rev-parse HEAD)" \
        --json databaseId,status,conclusion

  **Read `conclusion`, not the run id.** `cancel-in-progress` is on, so a second push cancels the
  first run, and a cancelled run's `status` is `completed`, exactly like a passing one's — only `conclusion`
  separates them — quoting an id alone
  reinstates "somebody said the gates passed" one level up.

  **It must read `success` — with one exception, which is not a waiver.** While
  `internal/webui`'s goldens are red on `main` for the machine-dependent reason recorded below, a
  run that is red on `TestLayout` **alone** and green everywhere else satisfies this condition
  only if the review record names the run id **and** the failing subtests. **Any other red, and
  any missing run at all, blocks the merge.** ⚠️ Without this exception the condition is
  unsatisfiable rather than strict: no branch cut from a red `main` can produce a green run, so a
  rule demanding one would be ignored on the first day and dead by the second.

  **Then name the run id in the review record.** That is what makes this condition a fact rather
  than a claim: a reader can pull the run and read its log. A commit with no run has no second
  witness — say so, rather than letting the absence read as a pass.

⚠️ **A red CI run is read, not waived.** It runs on a different machine and sees what one local
run cannot. Recorded 2026-08-19, because it is this gate's own failure arriving from the other
side: `main` merged four times over a red CI — runs 32293310361, 32307352631, 32313698406,
32319637183, every one failing `internal/webui`'s `TestLayout` subtests on channel deltas of at
most 2 out of 255, which the same goldens passed locally. Whether that is still open on the day
you read this is a question for `gh run list`, not for this file. If a run is red for something
your branch did not introduce, name the failing test and the run id in the review record and file
it. "It was already red" is not a check.

⚠️ Two traps in the check itself. A **short** SHA returns `[]` rather than an error, which reads
exactly like "no run exists" — pass the full one, as `git rev-parse HEAD` does. And the run is
keyed to the **branch head**, not to the merge commit, which does not exist yet; condition 7 makes
the trees identical but does not make that run a witness to what lands on `main`. Read the `main`
run afterwards too — it is the one that went unread four times above.

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

⚠️ **Do not rely on a `pre-receive` hook — there is not one on the push path.** This sentence used
to say the hook was fail-closed and unbypassable, and that it merely rejected late. Checked
2026-08-19: `git remote -v` shows one remote, the hosted public GitHub repository, and a hosted
remote runs no such hook. The mechanical backstop is CI's leak-scan step, which now runs on push
as well as on pull requests — it had never executed at all before that, because this repository
has never opened a pull request. **Run the three greps anyway.** ⚠️ CI scans **added lines and added paths** in the pushed
range — `scripts/leakscan` reads `git diff -p -U0`, and **never reads the ref name at all**. So it
catches a branch name only once something has already written it into a path (a
`reviews/<date>-<branch>/` directory) or into a line (`snapshot.md`'s branch row). **A branch
pushed with neither is unscanned on that channel**, which is exactly the channel the third grep
covers. It also cannot correct a commit message that is already pushed.

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
