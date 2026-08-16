# Review record — integrating three branches for merge

**Commit reviewed:** `497c20d`, the integration of `worktree-prior-blend-experiment`,
`wayback-backfill` and `six-season-grid` onto `origin/main` (`e555c88`). Previous records:
[`2026-08-11-02e0c25`](../2026-08-11-02e0c25/review.md) and
[`2026-08-11-a39ba9e`](../2026-08-11-a39ba9e/review.md), both merged in here.

**Why this record exists.** `TestReviewCoversTheCurrentCode` failed on the integration, correctly.
The guard compares the tree against **one** named commit and has no notion of a merge parent, so
when two branches each arrive carrying their own record, whichever is newest fails to cover the
other's files. This is the same situation as [`2026-08-11-6d65e04`](../2026-08-11-6d65e04/review.md)
and takes the same form: a recorded "already covered" for what is covered, and a visible decision
for what is not.

No re-review of reviewed work. Four watched files changed since the newest record; each was traced
to its source commit with `git log -- <path>` rather than assumed.

## Group 1 — three files arriving from `wayback-backfill`, already reviewed

`docs/README.md`, `docs/architecture.md`, `docs/backfill.md`. All three trace to a single commit:

```
git log --format='%h %s' e555c88..HEAD -- docs/backfill.md docs/README.md docs/architecture.md
  ->  a39ba9e  Recover six seasons of point-in-time team news from the Internet Archive
```

`a39ba9e` carries [`2026-08-11-a39ba9e`](../2026-08-11-a39ba9e/review.md), recording three reviews
that found and fixed real defects — an index fetched over plaintext HTTP while naming the URL every
payload fetch then goes to, a documented field purpose that was false, and a headline honesty test
walking one capture per gameweek rather than all of them. Nothing in this integration modifies those
files further.

## Group 2 — `CLAUDE.md`, which has two contributors, one reviewed and one not

```
git log --format='%h %s' e555c88..HEAD -- CLAUDE.md
  ->  02e0c25  Narrow the six-season claim to what review could support
  ->  0b994d5  Widen the default replay grid from four seasons to six
  ->  62f3d37  Queue the prior-blend experiment, and withdraw "not measurable"
```

`0b994d5` and `02e0c25` are the grid work and are covered by
[`2026-08-11-02e0c25`](../2026-08-11-02e0c25/review.md) — two reviewers, `fpl-stats-review` and
`fpl-findings-audit`, whose findings are applied there.

**`62f3d37` is not separately reviewed, and that is a decision rather than an oversight.** By the
triage table a `CLAUDE.md`-only change owes `fpl-findings-audit`. It was not dispatched. The reasons,
so the next pass does not re-ask:

- **What it asserts is a count, and the count is reproducible.** It withdraws "the benefit of
  `prior_half_life` is not measurable on the archive" on the ground that the verdict was reached
  when the replay had two seasons and the archive now reaches seven, where the case occurs **235
  times**. That was computed directly from the archive (`players_raw.csv` for 2018-19 to 2020-21,
  the cached season files thereafter) and the per-season breakdown — 23 / 39 / 40 / 40 / 47 / 46 —
  is re-derivable by anyone in a few minutes. It is not an inference from a replay.
- **It changes no code and no constant.** `prior_half_life` remains `0`. The change is a withdrawal
  of a *limitation claim* plus a queued experiment, and the experiment is written with its
  disqualifying condition stated in advance.
- **Its own weakest number is marked as such in place.** The −36% error reduction is flagged as an
  upper bound in both `CLAUDE.md` and `TODO.md`, with both reasons given: the baseline is a strawman
  because `shrinkToLeague` already pulls a thin season toward league rates, and the population is
  selected on the thin season being short so its rate regresses upward mechanically.

**Residual risk, stated plainly:** nobody independent has checked that the 235 cases are the
population `prior_half_life` actually fires on, as opposed to the population the counting script
defined. If that is wrong the queued experiment is mis-specified — but the experiment has not been
run, so nothing downstream depends on it yet. A findings audit before that experiment starts would
close it cheaply.

## The merge itself

Clean. All three branches merged with no conflicts, verified before the fact with a scratch
integration rather than by inspection. The full suite passes on the integrated tree: 13 of 13
packages, `go build`, `go vet` and `gofmt` clean.

Two behavioural changes arrive together and do not interact. The grid widening touches only
`internal/backtest` test-harness selection and the record; the availability backfill adds three new
packages and is forbidden by `TestTheScoringPathCannotSeeRecoveredTeamNews` from being imported by
`internal/analysis`, `internal/backtest` or `internal/agent`. Verified that guard is not vacuous.

## What could not be checked

Everything carried forward from the two constituent records, unchanged — in particular the
highest-value open item, whether the expected-goals backfill moved the shipped four seasons' own
cells (96 cells, `FPL_SWEEP_SEASONS=default` with `FPL_NO_XG_REPAIR=1`). Queued in `TODO.md`.
