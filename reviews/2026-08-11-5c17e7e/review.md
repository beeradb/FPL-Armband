# Review record — the batch-1 measurements

**Commit range reviewed:** `68b8af9..HEAD` on `batch1-restacked` — the 96 replay cells, the two
measurement scripts, and the write-up at `stats/snapshots/2026-08-11-0104d9d/FINDINGS.md`. Previous
record: [`2026-08-11-db0716c`](../2026-08-11-db0716c/review.md).

## Reviewers dispatched

Triage: research record → **`fpl-findings-audit`**; statistical claims → **`fpl-stats-review`**.
Both ran, concurrently, read-only. `fpl-code-review` was **not** owed: the branch's only Go file
(`cmd/scratchcal/main.go`, a throwaway a dying subagent committed) was removed before dispatch, so
the branch is evidence and analysis with no source change. `fpl-security-review`, `fpl-run-review`,
`fpl-season-maintenance` — nothing in scope.

**Both reviews found substantive errors, and between them they overturned the headline of one of the
two findings.** The gate is not running as theatre.

## The headline: my own denominator produced the result I was most pleased with

The write-up said FPL's doubt percentages are optimistic and that **squaring the flag** is very
nearly the correction — a striking fit across three levels. **That fit is an artefact of the
baseline I chose**, and I verified the refutation myself before applying it.

A player is flagged *because* he has been out, so his most recent **unflagged** week — the thing my
baseline averaged — is systematically older for a flagged player. Reproduced independently: mean
staleness runs 1.04 gameweeks for unflagged players to 6.13 at the 0% flag. And the placebo is
decisive — among players with **no flag at all**, those whose baseline is 6+ weeks old realise only
**0.658** of it, against 0.992 at one week. A third of the apparent effect is my denominator
drifting.

The deeper version is worse and is the transferable lesson. `availabilityFactor` multiplies `Score`,
whose `ExpectedMinutes` is recency-weighted and counts absent weeks **as zeros** — so the model has
already marked a doubtful player down before the multiplier is applied. Calibrating against his
pre-doubt healthy level charges the same absence twice, over-correcting by **92% at the 25% flag**.

Corrected by two independent routes that agree, the exponent is **≈1.6**, not 2.0.

**This is the second instance of a pattern already in the record** — *a constant fitted against a
proxy for its input is fitted to the proxy's noise too* — and *check what a multiplier multiplies
before calibrating it* has now failed twice.

## What survives, and it is the substantial half

- **The direction is established beyond argument**: t = −13 to −24 against face value under every
  clustering tried.
- **A 75% flag is worth about 0.55 to 0.60**, on all three denominators — far below 0.75 either way.
- The functional form is **suggestive at ≈1.6**, not established.

The statistics review also ran the attacks I could not and **none of them bit**: clustering by
player, player-season or season; mean-of-ratios against ratio-of-means; baseline window 4 to 38,
floor 30 to 60, median for mean; points instead of minutes; and capture staleness, which if anything
flatters the pooled figure *upward*. Recorded so the next pass does not repeat them.

## Applied, from both reviews

1. **The correction cannot be replayed at all.** `PointInTime` never populates
   `chance_of_playing_next_round` and `statusAt` never emits `d`, so `availabilityFactor` is 1.0 or
   0.0 in every replayed cell and a `flag` vs `flag²` sweep returns byte-identical seasons — the
   intervention failing to run, not a null. **Unmeasurable on this harness as wired.** My stated
   disqualifying condition could not have fired.
2. The square refuted, with the staleness table, the placebo, and the corrected exponent.
3. The channel error: I measured **minutes**, the multiplier acts on **Score**.
4. The 55-minute floor cannot do the job I credited it with — it screens blended *historic* minutes,
   the 85-100% figure belongs to players who already missed a week (18% for those still playing),
   and it filters the opening squad only. The reachable channel is the **transfer search**, which
   ranks with no floor — the argmax channel, which is the dangerous one, not a safe one.
5. Replication split by level: tight at 75% (3.7% spread), absent at 25% (factor of 2.75, monotone
   decline), so the forward-looking 25% figure is nearer 0.04-0.05 than the pooled 0.085 — *below*
   the square, opposite to the staleness correction, and the two partly cancel.
6. "About a fifth" hedged: zero in 18 of 24 cells, **positive in four of the six that move**,
   t = −0.48, degenerate under season clustering. Exact arithmetic on this grid, not a transferable
   share.
7. **The number nobody reported**: the backfill is worth **+138 points a season** on 2022-23's cells
   (t = +2.36) while the paired comparison it perturbs moves 0.4 — the "a shared baseline shift
   cannot move a paired comparison" rule demonstrated across a 300× level shift. Worth more than the
   fifth.
8. Cell terminology (6 of 24 paired cells, not 12 of 48 rows); the three-season invariance reframed
   as a pipeline check the code predicts, conditional on shipped prior settings; `FPL_NO_XG_AGGREGATE`
   exists to separate the two repairs and was not run; a provenance `dirty`-flag mismatch recorded
   rather than explained away; headline n corrected to 51,598; exposure reframed per squad (~one
   flagged player a week in a fifteen); the untested `d → 0.5` branch; the `EventsBehind` staleness
   channel; the `blankRunFactor` overlap, which would double-discount; and the keeper figure at the
   50% flag withdrawn (25 observations, 14 players).
9. `archive-and-data.md`'s "unmeasurable" verdict superseded **in half**: the archive half stands,
   the full stop does not, and what remains unmeasurable is the *value* rather than the calibration.

## Declined, or deferred, and why

- **Shipping any correction to `availabilityFactor`. Declined**, more firmly than before: the form is
  refuted, the right exponent is ≈1.6 on a denominator nobody has fitted properly, and the replay
  cannot value it. Three reasons, any one sufficient.
- **Deleting the original table. Declined** — the corrections are expressed relative to it, and the
  marking convention keeps superseded text.
- **Re-running the calibration against the model's own `ExpectedMinutes`.** This is the **highest-value
  follow-up** and it is deferred rather than done: it needs a Go diagnostic calling into
  `internal/analysis` rather than Python over cached JSON, and it would replace the whole exponent
  argument with a curve fitted on the right scale. Seconds of runtime, hours of work, no sweep.
- **Wiring the captures into the replay.** Deferred, and noted as **two** interventions that must be
  measured separately — giving the replay per-gameweek flags at all is a data change on the scale of
  the xG backfill, and the curve is a second arm on top of it. It belongs on `POLICY`, which is the
  noisy metric, so the effect may be smaller than the threshold.
- **The 25%/50% downward drift.** Nothing on this archive resolves whether it is trend or noise. The
  project's own forward capture is the instrument, and it answers in a couple of seasons.

## What could not be checked

Only the drift above is genuinely **unmeasurable here**. Everything else deferred has a named,
bounded piece of work behind it.

## A note on process

The statistics review reported that the worktree was switched under it mid-review by another
process, and it worked from `git show` rather than the working tree as a result. Both reviews were
dispatched against a branch that a third session could move. Worth avoiding next time by giving a
reviewer its own worktree.
