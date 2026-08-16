# The tier-1 batch integration

## What was reviewed

`integrate-the-tier-1-batch`, merging **nine branches** onto `origin/main` at `425be4b`. Each
branch was reviewed on its own branch and carries its own record under `reviews/`; this record
covers the **integration**, which is a different object.

| branch | what it carries |
|---|---|
| `xpoints-season-rules-and-unknown-position-guard` | xPoints errors on an unpriced position; per-season rules pinned |
| `fail-loudly-on-an-unreadable-fixture-scale` | `envScale` deleted; both fixture scales read the strict parser |
| `measure-blank-run-bias-by-position` | the position-cut falsifier; comment-only in `blend.go` |
| `xgc-transport-tercile-cancellation` | the tercile column and its transport contrast |
| `defensive-fixture-coefficient-hindsight-gate` | the hindsight gate and its pre-registration |
| `refit-the-defensive-fixture-coefficient-at-the-cutoff` | the point-in-time refit |
| `record-the-team-strength-revision-leak` | the archive finding and `stats/team_strength_revisions.py` |
| `correct-the-defensive-ladder-sign-inversion` | the sign correction in a closed line |
| `record-that-the-difficulty-column-is-end-stamped` | the end-stamping entry and the canary-sizing rule |

## The invariant came first, and it is what scoped this review

`review-gate`'s opening instruction is to ask what quantity the change must **not** move, and to
prefer an invariant to a reviewer. For an integration of already-reviewed branches the claim is:

> **the merge introduces no non-test change of its own.**

**Verified.** `git show --cc` over the one merge commit that conflicted returns no non-test file;
the sole conflict was in `internal/snapshot/fingerprint_test.go`, and the only other integration
edit is in `internal/backtest/antiresidualgate_test.go`. Both are `_test.go`.

That is why `fpl-code-review` and `fpl-stats-review` were **not** re-dispatched: the code they would
review is byte-identical to code they already reviewed on the nine branches, and re-running them
would be the theatre the skill warns against. Recorded as a decision, not an omission.

## Reviewers

| reviewer | ran | why |
|---|---|---|
| **fpl-findings-audit** | ✅ | `CLAUDE.md` moved, and three independently-written record edits landed together on top of ~100 lines `origin/main` added underneath them. This is the integration's real risk and nothing else would catch it |
| fpl-code-review | ❌ skipped | the integration delta is test-only (invariant above); each branch carries its own code review |
| fpl-stats-review | ❌ skipped | no new measurement was run here; each measurement branch carries its own statistics review. It reviewed the `def` arm's **plan** before it ran, and refused the literal design |
| fpl-security-review | ❌ skipped | nothing touches `internal/agent`, `internal/fpl` or config persistence |
| fpl-run-review, fpl-season-maintenance, fpl-docs-review | ❌ skipped | no live run wrote config; the four season lists are untouched; `docs/` changed only via a merged branch that carried its own record |

## Findings

Ranked by how misleading the current state would be if left.

### Applied

1. **A real integration bug, caught by the gate rather than by a reviewer.** `origin/main` added
   `TestTheAntiResidualGateIsTheResidualGateWithOneSignFlipped` in the six commits this batch was
   behind. Its fixture calls `calibrateConversion()` alone; the xPoints season-rules pin makes
   `XPointsResidual` **refuse** a zero-valued `XPointsRules` rather than price every channel at
   nothing. Each side is correct alone — together they panic. `resolveInstrumentInputs()` exists so
   the two halves cannot be done separately, and the fixture predated it while its comment already
   claimed to do "as `repaired()` does". Fixed there. **This is the argument for merging early
   rather than accumulating branches.**
2. **`teams.csv` is suspected end-stamped, and unlike `def` that channel is LIVE on the scoring
   path.** `season.go` treats the strength block as point-in-time-safe because *"played and points
   are zero — so it is a pre-season snapshot"*. **I verified that inference is invalid**: FPL's
   payload carries `played: 0, points: 0` in the **GW38** capture too, and Arsenal's 2023-24
   strength moves 4 → 5 within the season. Recorded in `CLAUDE.md` with the split stated —
   ⚠️ **the claim that `teams.csv` equals the last capture is reported, not reproduced**, because
   `teams.csv` is not in the checkout. The size is unmeasured and no magnitude is quoted.
3. **"This is the one place a new feature can silently train on matches that had not been played"
   was left claiming exclusivity** while the new entry says "second place". Narrowed to *results*,
   with a pointer.
4. **The archive-section entry read as a retraction of `b2 = 1.5688`.** The *scoring-model* copy
   carried the "unmeasurable, not refuted" guard and the archive copy carried none — and this file
   is read by section. Guard added to both, and the headline softened to name the mechanism step.
5. **The sign correction falsified the gloss it sits inside.** "Never won any, at any setting
   tried" was consistent with "costs 20" and is contradicted by "gains 20" in the same sentence —
   two of four alternative scales sit above shipped on the raw table. Narrowed to "never won
   **measurably**", with the two settings named.
6. **"The correction strengthens it" was one word too strong.** 20 points inside an uncomputed
   noise band is a tie in the other direction, not a result. Now "does not weaken it… does not
   strengthen it either".
7. **The corrected ladder was quoted without its cell count or data state.** Added: **3 cells**,
   absolute totals, no threshold ever computed, and run at a commit predating the
   defcon-visibility change worth −95 on one of its three seasons. So "no shape" is *unmeasured on
   the current grid*, not a measured null.
8. **A stale ordinal in the merge resolution.** Both conflicting branches registered an output-path
   switch; the surviving comment said "a fourth output path" when it had become the fifth. A
   mechanical resolution would have kept it.

### Declined, with the reason

- **Re-dispatching code and statistics review over the merged tree.** The invariant above shows the
  code is unchanged from what they reviewed. Declining is recorded so the next pass does not
  re-ask.
- **Re-measuring the defensive ladder on the current grid** (audit finding 5's implication). It is
  a run, not a record edit, and it belongs to whoever picks up the fixture line — which this batch
  closes as instrument-limited. Recorded in the queue rather than done here.
- **Sizing the `teams.csv` leak.** A run, and the audit agrees it is not a record edit. The entry
  deliberately quotes no magnitude.
- **Adding `±50` to the trap list** and **naming the pre-registration as the Spearman figures'
  generator**. Both are good and neither is load-bearing for anything shipped; left for the next
  record pass rather than growing this commit further.

## The second merge of `origin/main`, and the convention it forced

`origin/main` moved again mid-gate (3 commits), which is condition 7 doing its job. Merging it in
brought `a293551`, **"State the correction convention where a writer meets it"**, which is directly
about the five corrections in this batch:

> **A correction here REPLACES the claim; it does not narrate the replacement.** … Marking a
> withdrawal in place is correct where the evidence lives and wrong here, and the two being
> opposites is deliberate.

**Every correction in this batch was written the wrong way** — as a dated "⚠️ Corrected 2026-08-16:
this read X until then" annotation, which is the evidence-store convention. All five were rewritten
to state what is true now and drop the narration:

- the two sign-inversion sites,
- the ±50 defence (now a compact statement that ±50 is a **half-width** derived from a 105-point
  gap, rather than the story of a challenge that was made and rejected),
- the `def` end-stamping entry,
- the "0 of 380" withdrawal.

⚠️ **The convention's own caveat was honoured**: *"a deleted figure still owes its referent"*. The
recorded ladder totals, the 3-cell count, the data state and the "cannot be re-derived, only
re-measured" note all survive the cut, because without them the withdrawn reading is the only
available inference.

**This is the clearest argument in the batch for merging early.** The convention landed on `main`
while these branches were in flight; had they merged a day later, five annotations in the house's
wrong style would have landed first and been cited as precedent.

## A rejection I made and the audit upheld

A reviewer earlier claimed *"Both span less than the noise"* contradicts its source, because the
defensive column spans **61** against a stated **±50**. I judged that wrong and wrote the rejection
into the record. The audit verified it independently: **±50 is a half-width**, derived in the source
by halving an observed 105-point gap, and the same source says in terms that both ladders are
"inside the ±50 noise" about a column it has just said spans 61. 61 < 105. The reviewer compared a
span against a half-width. **No retraction owed** — and the note stays, because that misreading has
now been made once and is the natural one.

## What could not be checked on this harness

- **Whether `teams.csv` equals the last capture.** Not in the checkout. Reported, not reproduced.
- **What the `teams.csv` channel is worth in points.** Unmeasured, and it needs a run.
- **Whether FPL's *live* difficulty moved in-season.** The captures carry team strength but **no
  fixtures payload**, so the strength→difficulty step is a mechanism argument and is labelled as
  one in both places it appears. Adding fixtures to the weekly capture answers it by construction.
- **The three record-edit branches' figures.** They are quoted from banked runs, not re-derived
  here; the ladder totals in particular *cannot* be re-derived, only re-measured, and the entry now
  says so.

## Gate

Build, vet and `gofmt` clean. Full suite passes. `TestTheResidentIndexStaysSmall`,
`TestEnvSwitchListIsComplete` pass. 0 behind `origin/main` at merge time. Leak check clean in all
three channels — diff, commit messages, and branch name.
