# Review — re-statusing the expected-points review, and screening its two live gaps

> ⚠️ **This record is MISNAMED and is kept for its findings, not as the current record.** It is named
> after the commit the work started from rather than the commit it reviews, so
> `TestReviewCoversTheCurrentCode` — which diffs `<recorded sha>..HEAD` — went red the moment the work
> it describes was committed on top of it. See `reviews/2026-08-10-be65af1/` for the convention that
> works and for the correction to this pass's Gap 3 verdict, which was too broad.

**Commit range reviewed:** `149615c..HEAD` on `worktree-xp-review-gaps`, plus the untracked
`internal/backtest/teamshare_test.go`. This is the first review record in the repository, so
`TestReviewCoversTheCurrentCode` was skipping rather than failing before it.

**The change in one line:** no shipped scoring value moves. Everything here is a diagnostic, an
escape hatch, or the record.

## Reviewers dispatched

| reviewer | why | outcome |
|---|---|---|
| **fpl-stats-review** | the pass produced two measurements and the triage table sends `internal/backtest` changes here | **overturned the headline.** See finding 1 |
| **fpl-code-review** | `internal/analysis` changed, and a refactor claiming no behavioural change is exactly what it is for | confirmed the invariant, found three real defects. Findings 4-6 |
| **fpl-findings-audit** | `CLAUDE.md` and `docs/` are most of the diff | found a stale imported table and several over-statements. Findings 2, 7-10 |

**Skipped, with reasons.** `fpl-security-review` — nothing touches `internal/agent`, `internal/fpl`,
config persistence or the cache; the one `internal/snapshot` change is a name added to a list and a
prose string. `fpl-run-review` — no live run, nothing written to config. `fpl-season-maintenance` —
the four hand-maintained lists are untouched.

**Invariants written before dispatching**, per the skill's first section, since they are free and
run every time where a reviewer does not:

- `TestTheShippedFitIsWhatRuns` — the live curves equal the shipped constants when nothing is set,
  checked as values *and* as four evaluated points so a transposition fails.
- `TestTheAppearanceFitReachesEveryConsumer` — all five call sites move under an override, including
  `PlayerMetrics.Score` end to end.
- `TestTheAppearanceFitOverrideIsAllOrNothing` — a partial or unparseable fit falls back entirely.
- `TestTheDiagnosticBaselineMatchesTheShippedFit` — added *after* review, closing finding 6.

## Findings, ranked by how misleading the state was

### 1. The Gap 3 headline was wrong. APPLIED — conclusion reversed

Recorded as "the gate fires, the item stays live" on a between-club spread of 0.162. Three
corrections, all verified independently before applying:

- **12% of the pooled variance is a between-*season* level shift**, not a between-club one — a
  league-wide `calibrateExpectedStats` miss that no per-club anchor addresses. Reproduced the printed
  0.222 exactly from the per-season table, so the decomposition is arithmetic rather than opinion.
- **Season-clustered it does not resolve.** Per-season excess 0.269 / 0.047 / 0.276 / 0.000 — two of
  four seasons at or below their own noise floor — mean 0.148, SE 0.072, **t = 2.04 at df 3** against
  the 3.182 four clusters demand.
- **The tail had no null.** 14 of 80 beyond ±30% is against **2.8 expected** from the denominator's
  own sampling noise, not against zero.

And the measurement that decides it was missing entirely: a static between-club offset **must persist
for the same club** to be removable by an anchor fitted on history. Built it — 51 consecutive-season
pairs — and the correlation is **−0.232**. Gap 3 now closes **on mechanism**.

⚠️ **The pre-committed threshold was unreachable in the drop direction**, because the raw sd it was
written against contains a floor of ~0.137 before the model is wrong about anything. Recorded as a
lesson about gates rather than quietly restated: pre-registering against the wrong quantity buys
nothing.

### 2. The category table imported stale figures. APPLIED

The "ours" row (1.674 / 1.553 / 1.695 / 5.803 / 2.918) came from `docs/notes/harness-and-inference.md`
and predates several scoring changes. Both *baseline* rows matched the snapshot exactly, which is
what made it hard to see. Verified against `stats/snapshots/2026-08-10-27740ba/figures.csv` and
against a fresh run: the model row is 1.876 / 1.643 / 1.574 / 5.597 / 2.887.

**A conclusion died with it.** "We win the low-return categories and lose the tail by 3.3%, the same
shape as the commercial leader" is false on current figures — the model is 0.4% *ahead* of the
five-game baseline on Haulers. Withdrawn rather than restated. The note's table wants the same fix
and has not had it.

### 3. Gap 7's verdict overstated. APPLIED

"REFUTED" and "worse than shipped" were too strong: the measurement is −4.1% on P(appears) and
**+0.1%** on P(60+). Narrowed to "does not transfer to the axis the model scores on".

Two things the reviewers added that strengthen the finding rather than weaken it. The losing arm was
**flattered** — its constants are in-sample over all four seasons while the honest arm is held out
per fold — so −4.1% is conservative. And the real evidence is **parameter recovery**, not the
percentages: refitting against `ExpectedMinutes` returns the shipped constants to within 2% from a
different population and a different loss, which is the `k=8` two-methods pattern.

The one open question — whether the +1.3% on P(60+) survives isolation — was **closed by running it**
rather than left: pinning the two identity constants at shipped reproduces the four-constant arm to
three decimals (bias +0.0282 against +0.0291), so the sixty-minute slope carries the whole effect and
it is a bias-for-variance trade.

### 4. `TestDiagTeamGoalShare` compared different player populations. APPLIED

The modelled numerator ran over `boot.Elements` (registered by GW19); the realised denominator ran
over every player in the season. A January arrival's expected goals landed in the denominator with
nothing above it — a mean 4.6% of a club's realised window and up to 30% for one club-season, and
**invisible to the split-half control**, since both halves are realised data containing the same
arrivals. Denominator now restricted to the registered set, and the excluded share is printed per
season (2.8% to 5.8%). Every Gap 3 figure in this record is post-fix.

### 5. A second copy of the sixty-minute curve, in the file that found the first. APPLIED

`TestDiagPlaysAtAll` inlined the logistic **without either exact bound**, diverging by −0.061 at
ninety minutes, and printed `0.0423` where the shipped model returns exactly 0 for a player with no
minutes — reporting a failure of the property `research_targets` depends on that the model does not
have. Now reads `analysis.PlaysSixty`; the line prints `0.0000`. It had also seeded the table quoted
in `appearanceFactor`'s comment, which is refreshed with the movement explained.

### 6. Arm restore path could silently mix states. APPLIED

The two new benchmark arms restored to *shipped* rather than to what was live, so running the whole
benchmark with `FPL_APPEARANCE_FIT` set would have left later arms scoring against a baseline the
earlier ones did not use — no error, deltas carrying two changes. Now captures and restores the live
fit. `AppearanceFit()` existed for exactly this and was unused.

### 7. Diagnostic baseline unbound. APPLIED

`shippedSixty` / `shippedAtAll` in `playsatall_diag_test.go` are a *deliberate* frozen mirror — they
cannot call the package, because an arm may have overridden it — but nothing tied them to the
constants they mirror. `TestTheDiagnosticBaselineMatchesTheShippedFit` now checks the constants and
seven evaluated points.

### 8. `internal/snapshot/model.go` said "uniformly negative". APPLIED

The superseded proxy's error also changes sign, near eighty minutes rather than near fifty. Corrected
to say the two disagree about *where* the model over-credits. The dated boundary was also useless —
"before 2026-08-10" excludes the only snapshot in the tree, which is same-day and *is* the proxy — so
it now names the snapshot.

### 9. The conclusion drawn from the wrong curve survives, and CLAUDE.md under-claimed it. APPLIED

The snapshot's reading — that the error mis-ranks a part-timer against an ever-present — is true of
the shipped curve too: the 20-30 band minus the 80-91 band is **−0.038 under the proxy and −0.040
under the shipped curve**. Recorded, with the reason it is still the wrong instrument: the two
diverge across the middle of the range, which is where every refit argument lives.

### 10. Smaller record fixes. APPLIED

"the gap is 16%" described a spread as a level; "rules out the cheaper repair" over-read an sd of 45
on 990 (it bounds the minutes channel at ~4.6%, it does not rule it out); the 39-points figure was
quoted as a harness property rather than a per-comparison median; `docs/notes/scoring-model.md` still
carried the superseded inference unmarked and now carries a ⚠️ in place.

## Declined

- **An entry in `retracted_test.go` for 2.0% / 3.9%.** Every existing entry is a figure that *did not
  reproduce*; these reproduce exactly on a defined axis. An entry would assert something false, and
  `2.0` collides with `free_transfer_value` — the "fires on legitimate prose, gets deleted, then
  guards nothing" failure that file's own comment records. The withdrawn thing is an *inference*, and
  the guard has no vocabulary for inferences. Both the audit and I reached this independently; the
  right instrument was finding 10, marking it in the note.
- **Running `stats/prediction_inference.R` over the new arms.** Correct in principle and would make
  "trade" an inference rather than a classification of point estimates. Not done: it does not change
  the verdict (nothing is being taken to the replay), and the limitation is now stated in the text
  instead. Named as unmeasured, not unmeasurable.
- **Regenerating the accuracy snapshot.** Its `sixty_minute_threshold` rows are now known to be the
  superseded proxy. Regenerating runs the full diagnostic suite and belongs to its own pass; the
  section's reading text carries the warning in the meantime.
- **Splitting `winRow`'s two x-axes into one population with one fit protocol.** The clean 2×2 the
  stats review describes would isolate the axis better than the current table, whose first row is
  three seasons and LOSO while rows 2-3 are four seasons. Real, and it does not change the direction —
  the excluded rows have zero mean minutes, where every column scores identically. Recorded as the
  way to do it properly if the number is ever quoted more precisely.
- **Fixing `docs/model.md`'s 0.34 against CLAUDE.md's 0.45, and 28% against 29%.** Both pre-existing
  and outside this range. Recorded here so the next pass does not re-find them.

## What could not be checked on this harness

- **Whether either appearance refit would earn points.** The benchmark measures prediction error, and
  this project has a recorded case of 2% better out-of-sample error costing about 49 points a season.
  Only the replay arbitrates, and nothing here was taken to it — deliberately, since both arms
  classify as bias-for-variance trades.
- **Whether an anchor that adapts *within* a season would help.** The persistence result closes the
  static form Gap 3 proposes. A within-season normaliser is a different object and is untested.
- **2022-23's contribution to Gap 3.** 15 of its 19 pre-cutoff gameweeks are Understat backfill scored
  against FPL's own expected goals. It is one of the two seasons carrying the effect. `FPL_NO_XG_REPAIR`
  would size it; not run, and named in the document.
