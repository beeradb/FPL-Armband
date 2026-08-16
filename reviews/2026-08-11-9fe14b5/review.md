# Review record — the merge of `origin/main` into the rank-objective work

**Commit reviewed:** `9fe14b5`, the merge of `origin/main` (`fa2633b`) into
`rank-objective-handoff-finish` (`7b4e2be`). Merge base `149615c`.

**This record exists because `TestReviewCoversTheCurrentCode` failed on the merge, correctly.**
Its complaint was that 20 watched files had moved since `2026-08-11-316ae4f`, the previous newest
record. That is true, and the 20 fall into two groups, neither of which owes a fresh reviewer
dispatch. This is a **recorded "not applicable"**, in the form the `review-gate` skill asks for, so
the next pass does not re-ask.

## Group 1 — code and docs arriving from `origin/main`, already reviewed there

`internal/analysis/{appearance,metrics,sweep,teamform}.go`, `internal/backtest/{simulate,teamform}.go`,
`docs/expected-points-review.md`, `docs/notes/scoring-model.md`,
`stats/snapshots/2026-08-10-{774a2b4,a14ab00}/*`.

These are the club-form line, `774a2b4..fa2633b`. **They are not modified by this branch** — the
merge takes them unchanged from `origin/main`, and they carry their own review records, which this
merge also brings in:

- `reviews/2026-08-10-be65af1`
- `reviews/2026-08-10-0d7d31d`
- `reviews/2026-08-10-149615c`

The guard cannot see this, and should not be expected to: it compares the working tree against one
named commit and has no notion of a merge parent. So the correct response is to record why the
change is covered, not to re-dispatch reviewers over another branch's already-reviewed work.

**Verified rather than assumed:** `git merge-tree` reported the merge clean before it was run, and
`git diff origin/main HEAD` over those paths is empty — this branch contributes no change to any of
them.

## Group 2 — this branch's own files, changed *after* the previous record was written

`CLAUDE.md`, `docs/README.md`, `docs/notes/harness-and-inference.md`,
`docs/rank-objective-handoff.md`, `stats/README.md`, `stats/rank_robustness.R`.

These moved in `4c8b38a` and `7b4e2be`, which are **the applied output of the review recorded at
`2026-08-11-316ae4f`** — the retraction of the linearised scope block, the refutation of the
fat-tail follow-up, the Spearman-as-identity correction, the dangling-citation fix, and the
correction to that record's own opening claim. The findings, what was applied and what was declined
are all in that record; this one does not restate them.

There is an unavoidable ordering artifact here worth naming, because it will recur: **a review
record is named after the commit it was taken at, but applying its findings necessarily produces
later commits**, which the guard then reads as uncovered. The resolution used here — a follow-on
record naming the final commit and pointing back — is cheaper than the alternative of never
applying a finding in the same branch it was raised in.

## Which reviewers ran

**None, for this commit.** Triage under the `review-gate` table:

| the change touches | dispatch | decision |
|---|---|---|
| `internal/analysis`, `internal/backtest` | code + stats + findings | **skipped** — arriving unchanged from `origin/main`, covered by the three 2026-08-10 records |
| `CLAUDE.md`, `docs/`, `stats/` | findings-audit | **skipped** — these are the applied findings of `2026-08-11-316ae4f`, whose audit produced them |
| a merge asserting no semantic change | invariants over reviewers | **the tests are the check**, per the skill's opening section |

## What was checked instead, since invariants beat reviewers here

- `go build ./...` — clean.
- `go vet ./...` — clean.
- `go test ./...` — all packages pass, `internal/report` has no test files. Notably this includes
  `internal/backtest`, which exercises the merged scoring path, and `TestSnapshotCoversTheCurrentCode`.
- `Rscript stats/rank_robustness.R` on both cells files — still runs, output unchanged apart from the
  corrected footer text.
- `git merge-tree` before merging — clean, no conflicted paths.

## What was declined

**Re-reviewing the club-form code on this branch.** It was reviewed on `main` at the commits that
introduced it, by reviewers with that change's context. Re-running them here would produce a second
opinion on someone else's finished work while adding a record that claims coverage this branch's
author did not earn.

## What could not be checked on this harness

Unchanged from `2026-08-11-316ae4f`: the field's tail shape beyond ~3.1 standard deviations, the
exponent governing how the field's spread grows with cell length, the captaincy and bench
corrections (unmeasured rather than unmeasurable — `average_entry_score`, live season only), and
three of the eleven arms whose `EXP=HITS` harness is not on this branch.
