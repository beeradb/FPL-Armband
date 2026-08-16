# Review — the club-level error's timescale, and two staleness guards left red

**Commit range reviewed:** `a14ab00..be65af1`. Supersedes `2026-08-10-149615c`, which was
**misnamed** — see below, because the naming is the reason `main` went red.

## Why the previous record did not hold

`reviews/2026-08-10-149615c/` was named after the *base* commit rather than the commit it reviewed.
`TestReviewCoversTheCurrentCode` diffs `<recorded sha>..HEAD`, so the record went stale the instant
the work it described was committed on top of it, and `main` failed the guard from the moment of the
merge. The full suite passed before merging only because `HEAD` was still the base commit, so neither
staleness guard had anything to compare against.

**The convention that works, and it is not obvious:** commit the code first, then write the record
named after *that* commit, then commit the record alone. `reviews/` is not itself a watched path, so
the record's own commit does not re-trigger the guard. Any record written before the change it
covers is stale on arrival.

The same trap took `TestSnapshotCoversTheCurrentCode` down with it, for the same reason and with a
different fix — the snapshot is regenerated in `4297d6b`.

## Reviewers dispatched

**None.** Recorded as a decision rather than an omission.

The triage table sends `internal/backtest` and `docs`/`CLAUDE.md` changes to **fpl-stats-review** and
**fpl-findings-audit**. Both ran against `a14ab00` two commits ago, on this exact diagnostic and this
exact claim, and this range is their output: the stats review's central criticism was that the Gap 3
verdict rested on a single lag and was quoted without season clustering, and the entire content of
`be65af1` is measuring the other lag and clustering the result. Re-dispatching them to review their
own finding is the theatre the skill's closing section warns against.

What that leaves uncovered is stated plainly: **nobody independent has checked the timescale
measurement.** It is the weaker half of this record and the first thing a later pass should attack.
The specific things to attack are listed under "not checked" below.

## Findings

### 1. The Gap 3 verdict was too broad. APPLIED — verdict narrowed

`a14ab00` recorded the team-goals anchor as "closed on mechanism" on the strength of a persistence
correlation of −0.232 across seasons. That correctly kills a **static** per-club offset, since an
offset fitted on history can only remove what repeats. It was then generalised to the idea, and the
generalisation does not follow: one lag was measured, and a negative correlation is the signature of
something *reverting* rather than of nothing being there.

Re-measured at a shorter clock, a 50/50 blend of the model with the club's own preceding output
predicts its next stretch **13.1% better** pooled, with an **interior** optimum — both ends worse
than the middle, which is the shape this project demands before believing a constant.

**And it does not resolve:** +22.3% / −13.7% / +29.4% / −4.2% by season, mean 8.5%, **t = 0.82 at
df 3**. Recorded as unresolved and better-supported-than-the-static-form, which is a weaker claim
than either "closed" or "found something".

### 2. The obvious version of the test is invalid, and the diagnostic says so. APPLIED

Correlating a club's error in one block against the next **cannot work** while the model is held
fixed: both ratios share a numerator, so a common term is correlated with itself and the answer is
positive regardless of the football. Asking the question as out-of-sample prediction avoids it. This
is written into the diagnostic rather than left as a thing the next person rediscovers.

### 3. `fplagent snapshot` silently reads a stale file. APPLIED in `4297d6b`

The renderer takes a `-model` **flag** defaulting to `/tmp/model.csv` and does not read
`FPL_MODEL_CSV`, which is what the diagnostics write to. The documented recovery steps only work
because both happen to default to the same path. Pointing the diagnostics elsewhere and running the
renderer bare builds the snapshot from whatever is already at `/tmp/model.csv` — on a shared machine,
another job's file.

It happened here: the first regenerated snapshot **reproduced the previous one's figures to four
decimals**, which is the most convincing possible wrong answer, because "nothing moved" is what a
reader expects. Discarded and rebuilt with an explicit path; the instructions now pass `-model` and
explain why.

### 4. The snapshot's sixty-minute section was measuring the retired curve. APPLIED in `4297d6b`

Now records the shipped `playsSixty` — error **−0.043** at 20-30 mean minutes against **+0.050** at
60-70, where the proxy was negative until the top band — with the proxy emitted alongside under a
name that says what it is. This closes the caveat the previous record left open.

## Declined

- **Re-running the two reviewers on this range.** Reasoned above. Recorded as the known gap in this
  record rather than papered over.
- **Building the blend behind a flag and replaying it.** It is the right next experiment and it is
  not this pass: at t = 0.82 the prediction result does not justify replay time yet, and the standing
  trap means the replay would be the arbiter regardless of how good the prediction figure got.
- **Tuning the blend weight or the split point.** `shareMid` is the midpoint of the held-out window
  and 0.50 is the middle of the swept weights. Both were chosen before the numbers and neither is
  tuned, deliberately — picking the split that gave the best answer is an argmax over the thing being
  measured.

## What could not be checked on this harness

- **Whether the blend earns points.** Everything here is club-level expected-goals prediction. The
  recorded case of 2% better prediction costing about 49 points a season applies directly, and only
  the replay arbitrates.
- **Whether the short-clock structure is real or is two seasons.** Four seasons give three degrees of
  freedom and no amount of club-level data changes that. 2022-23, one of the two seasons carrying it,
  has fifteen of its nineteen pre-cutoff gameweeks supplied by the Understat backfill and scored
  against FPL's own expected goals. `FPL_NO_XG_REPAIR` would size that; not run.
- **Whether the +0.334 fixture correlation is separable from the rest.** The model already scales per
  fixture, and every attempt in this project to strengthen that response has lost points. Treat it as
  corroboration of the diagnosis, not as a lever.
