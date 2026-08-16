# Review record — merging `origin/main` into the replay-concurrency work

**Commit reviewed:** `6d65e04`, the merge of `origin/main` (`d962341`) into
`worktree-replay-concurrency` (`865cc1b`), plus the two corrections that merge exposed.
Merge base `c4faf9a`. Previous record: [`2026-08-11-89618ec`](../2026-08-11-89618ec/review.md).

**Why this record exists.** `TestReviewCoversTheCurrentCode` failed on the merge, correctly,
naming **65 watched files**. Those fall into two groups, and only the second owes anything. This
is a recorded "not applicable" for the first group, in the form the `review-gate` skill asks for,
so the next pass does not re-ask.

## Group 1 — 61 files arriving from `origin/main`, already reviewed there

`internal/analysis/*`, `internal/backtest/*`, `docs/notes/*`, `docs/expected-points-review.md`,
`stats/*` and the snapshot trees, spanning the 63 commits `c4faf9a..d962341`.

**This branch does not modify any of them.** Verified rather than assumed:

```
git diff --name-only origin/main..HEAD -- <all watched paths>
  ->  CLAUDE.md, docs/notes/harness-and-inference.md, docs/replay.md, stats/README.md
```

Four files, and nothing else. Everything else in the 65 is `origin/main`'s own work, arriving
unchanged, and it carries its own records — `2026-08-11-{530b7a4,9de80a7,a6f582a,b1af338,f05b391}`
among them, all merged in here. The guard compares the tree against one named commit and has no
notion of a merge parent, so the correct response is to record why the change is covered rather
than re-review another branch's reviewed work.

The merge itself was clean: `git merge-tree` reported no conflicts before it was run, and the
merge produced none.

## Group 2 — this branch's four files

`CLAUDE.md` and `docs/notes/harness-and-inference.md` were reviewed at
[`2026-08-11-c54917d`](../2026-08-11-c54917d/review.md) and are unchanged since, apart from the
before/after figure recorded at `89618ec`.

`docs/replay.md` and `stats/README.md` are **new in this commit, and are a finding from the merge
rather than a change carried into it.** After merging, grep found the retracted rule — "run one at
a time; this machine does not tolerate parallel replays" — stated in two further places that the
original change had missed, both as a property of the machine rather than of the toolchain.

Left alone, the record would have asserted the retraction in two files and the retracted claim in
two others. That is precisely the failure `CLAUDE.md` documents repeatedly and the reason its
house style demands a verdict rather than a title: a claim living in several copies reverts in the
copies nobody edited. **Applied:** both now carry the measurement (1031 MB driver against 145 MB
sweep; 1031 against 97 for the same block measured both ways) and point at `scripts/replay`.

Note what was *not* changed in them: both retain the warning that a killed run is a silently
partial result, and `docs/replay.md` retains the provenance-sidecar reasoning. The kill got rarer;
its consequence did not change.

## What must not move

Unchanged from the previous record, and still free to check:

```
git diff --name-only origin/main..HEAD -- '*.go'   ->   0 files
```

**This branch contributes no Go source at all**, so it cannot move a scoring constant, a harness
path or an inference figure. `go build ./... && go vet ./... && go test ./...` is green on the
merged tree — the review gate was the only failure, and this record is its answer.

## Reviewers dispatched

**None, and as before this is a substitution rather than a skip.** Triage puts a `docs/` + `stats/`
+ `CLAUDE.md` change in the **fpl-findings-audit** row; that agent was not dispatched because this
session operates under an instruction not to spawn subagents unless asked, so the audit was done
directly over the same scope. The remaining six are skipped on triage: this branch touches no
`internal/` tree, no config persistence, no season list and no live run output.

## What could not be checked here

**Whether `origin/main`'s 63 commits interact with the wrapper.** They were reviewed on their own
branch against their own claims, and nothing in them mentions sweep concurrency, but no combined
re-measurement was run — the per-process peak table was measured before the merge. The wrapper
prints peak RSS on every run, so a drift shows up in the next sweep anyone runs rather than
needing a scheduled re-check.
