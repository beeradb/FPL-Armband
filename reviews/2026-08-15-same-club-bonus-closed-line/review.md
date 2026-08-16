# Review — the same-club / "talisman" closed line

**Branch** `worktree-closed-line-same-club-bonus`, off `82fc8e0`. Working tree vs HEAD; nothing was
committed before review, so the reviewers read the staged diff rather than a commit range.

## What was reviewed

A proposal arrived in conversation in two forms and neither ships. **Form A**: penalise a squad for
holding two players from the same club, because bonus is awarded from a fixed 3/2/1 pool per match,
so teammates compete and their expected bonus is not additive. **Form B**, offered after A was
refused: not every club has a talisman, but some do, and finding those players makes or breaks a
season.

The change records both verdicts in `CLAUDE.md` under *Closed lines*, and carries four riders that
the review process itself produced:

1. **`CLAUDE.md`** — a new closed-line entry (Form A refuted on **arithmetic**; Form B
   adjacent-to-closed), plus an amendment to the existing *"Do not remove the bonus term for being
   circular"* bullet, plus an in-place correction to the `shrinkToLeague`/`LeagueShrinkK` bullet.
2. **`internal/analysis/metrics.go`** — doc-comment only. The `BonusWeight` field said "Default 1";
   it has shipped at 1.5 since `10da7a2` made the weight a schedule. Also corrects the
   `LeagueShrinkK` unanimity claim in the same file.
3. **`internal/backtest/leagueshrink_test.go`** — same unanimity correction in the package doc.
4. **`internal/snapshot/notes_test.go`** — raises the `CLAUDE.md` size budget 80 KB → 86 KB, naming
   the claim that needed the room, per that constant's own instruction to raise rather than compress.
5. **`.claude/agents/*.md`** — adds read-only MCP tool access to six reviewer agents' `tools:`
   allowlists, at the user's explicit instruction. Not a scoring or harness change.

## Reviewers

| reviewer | ran | why |
|---|---|---|
| **fpl-findings-audit** | yes | triage: `CLAUDE.md` / the record. Two rounds — it reviewed the entry, and its repairs were themselves re-checked. |
| **fpl-stats-review** | yes | three rounds. Rounds 1-2 adjudicated the *proposal* before any text existed (the plan, not the output — the standing rule). Round 3 reviewed the statistical claims in the final prose before they entered the resident record. |
| **fpl-code-review** | yes | triage: the diff touches `internal/analysis`. Pointed at the doc comment's truth and the budget arithmetic, not at scoring behaviour, since no behaviour changed. |
| **fpl-security-review** | no | nothing touches `internal/agent`, `internal/fpl`, config persistence or the cache. |
| **fpl-run-review** | no | no live run, no config written. |
| **fpl-docs-accuracy** | no | `docs/` unchanged, and it was independently confirmed correct at 1.5/0.5 already. |
| **fpl-season-maintenance** | no | the four hand-maintained lists are untouched. |

**Self-review was not used at any point.** Every finding below came from a reviewer, and the
verification of each came from reading the code.

## Findings, ranked by how misleading the pre-review state was

1. **The `LeagueShrinkK` unanimity claim was false, and resident in four places.** Recorded as "beats
   the shared 8 in **all four seasons** — real structure" (`CLAUDE.md`), "in every season
   individually" (`metrics.go`), "in every one of four seasons individually"
   (`leagueshrink_test.go`). **Refuted by the MAE ranges printed beside it**: K=2 spans 0.0774-0.0811
   against K=8's 0.0761-0.0839, and lower is better — so K=8's best season beats K=2's best, and one
   season must dissent. It is 2022-23, the season repaired the day after the measurement. This was
   pre-existing, not introduced here; the change propagated it before review caught it.
2. **The first draft had `MaxPerClub` backwards *and* offered it as support.** It claimed the cap
   "binds first, so the knob would likely return a byte-identical null". The cap bounds exposure at
   three and *binds* — `repairClubs` exists because the DP seed cannot express it — so squads reach
   the cap and such a knob would have targets. Separately, a *predicted* byte-identical null reads in
   this record as "the intervention could not run", which refutes nothing. Offering it as evidence
   was a third misused citation inside an entry whose subject is two misused citations.
3. **The first correction to the `BonusWeight` comment installed a fresh misreading while fixing
   one.** It said the sweep table peaking at 1.0 is "the retired flat weight, **NOT this field**".
   The table *is* a sweep of this field — run before `BonusPriorWeight` existed, applied flat — and
   that regime is still reachable today as `bonus_prior_weight: -1`. A reader told "1.0 is not this
   field" goes hunting for an identifier that never existed.
4. **The df hedge cited a bar the arm does not carry.** "|t| 1.94 clears neither 2.571 at a CR2 df of
   5" — but 2.571 is what *six* seasons give, and this is a four-season, 24-cell arm whose two-sided
   bar is **3.182** at df 3. Naming the looser bar did no work. Worse, t −1.94 is **not a CR2 t at
   all**: the retired inline estimator printed a naive and an uncorrected season-mean t side by side,
   and the record does not say which was transcribed — which is the difference between a one-sided
   pass and a fail.
5. **"Ships on mechanism" over-credited the hypothesis** and would have invited the next reader to
   attack the mechanism and thereby move a shipped constant. Nothing measured the variance channel.
   The burden framing is correct and is what the evidence actually supports.
6. **Unmeasured sign claim.** The draft asserted teammates' bonus is negatively **covariant**. The
   pool is rivalrous within a match, but a dominant performance puts two of the same club in the top
   three, so the sign is genuinely not obvious — and linearity of expectation holds under *any*
   dependence, so the refutation never needed it. A later reader could have falsified the sign with
   one archive query and it would have looked like the whole entry falling.
7. **`xiValueShrunk`'s gloss named the wrong consumer** — it is the transfer and replay path, not the
   squad objective, in a file whose signature failure is one quantity with two implementations.
8. **The circular-bonus bullet's "peaks at exactly 1.0" was unstamped.** It is three GW1 cells on
   `POLICY`, on totals, an argmax over five values, from the 2026-08-05 data state — inside the
   zero-penalty window and before the doubles, substitution and selling-price fixes. And 1.0 loses to
   both 0.5 and 1.25 in 2023-24.
9. **"Default 1.5" is approached, never reached.** With `blend_rate_k` 8 an ever-present sits at
   ~1.33 after 38 full matches, so the applied range is 0.5 to ~1.33.

## What was applied

All nine. Items 1, 2, 3 and 4 are corrections of statements that were false as written; 5-9 are
qualifiers restoring scope that the drafts had dropped. The unanimity correction (1) is marked in
place at all four sites with the date and the refuting arithmetic, rather than silently reworded.
The `LeagueShrinkK` bullet additionally now carries the `HOLD` flat null (+0.0095/gw, t 0.03), which
was resident in `metrics.go` but absent from the record.

## What was declined, and why

- **A fully derived detection threshold beside the 67-point bonus-removal cost.** fpl-stats-review
  computed a season-clustered SE of 33.9, t 1.99 against a df-2 bar of 4.303, concluding the
  contrast's own threshold is ~146 a season and that *no* arm in that table clears its bar. That is
  probably right and it is the correct instinct — a number without a threshold is not a result. It is
  declined **for this commit only**, because it is reviewer-derived arithmetic over a table that is
  no longer in the checkout, and this record's own rule is that a reviewer's report is a proposal
  until verified. Applying an unverified derivation to defend a rule about unverified numbers would
  be self-defeating. What *was* applied is the part verifiable from the surviving output: the metric,
  the data state, the non-unanimity, and that 66% of the 67 is 2024-25 — the season the zero-penalty
  bug was worth 113. **Whoever verifies the SE should add it.**
- **Tightening the entry to fit the old budget.** The budget constant's own comment forbids deleting
  qualifiers to fit, and every byte added after round one *was* a qualifier. Raised instead.
- **Re-siting the `LeagueShrinkK` hedge entirely into the same-club bullet.** It is mirrored: the
  correction is marked at the `shrinkToLeague` bullet where the claim lives, and the same-club bullet
  points at it for the estimator and the bar, rather than restating the derivation twice.
- **A monotonicity test on `bonusEvidence`.** fpl-code-review noted the new comment leans on
  evidence rising with minutes, which `threshold_test.go` asserts only as `0 <= v <= 1`. A real gap
  and a one-line fix, but it is a test addition outside this change's scope; recorded here so it is
  not lost.

## What could not be checked on this harness

- **Nothing in this change was measured, and nothing should be.** Form A is refused on arithmetic,
  which needs no cells; Form B is refused on a mechanism plus one precedent, and its own falsifier is
  archive-side. No sweep ran.
- **Which of the retired estimator's two t's produced −1.94 is not recoverable.** The cells were
  never banked — there is no `LeagueShrinkK` arm under `stats/snapshots/*/cells/` — so it cannot be
  re-analysed without re-running. The entry now says so rather than naming an estimator.
- **`TestReviewCoversTheCurrentCode` was already failing at HEAD**, on `stats` and `docs`, from
  unrelated in-flight work. Confirmed by construction rather than by assertion: the gate digests
  `HEAD` via `git ls-tree`, never the working tree, so no uncommitted change can trip it. This
  record re-keys it.
- **`TestSnapshotCoversTheCurrentCode` digests `internal/analysis` by blob content, comments
  included**, so the doc-comment edit here made the accuracy snapshot stale on commit. Predicted by
  fpl-code-review before it happened; precedent `758efab`, same class.

## Addendum — the snapshot, and why this key was written twice

`stats/snapshots/2026-08-15-497b1b2` regenerated the model half and both documented traps were
**checked rather than assumed**: the CSV went to a job-private path and that same path was passed to
the renderer with `-model`, so it is not a rebuild from whatever sat at `/tmp/model.csv`; it carries
exactly one `run_id` (`1786815496-1422237`); and all eight `model.transfer_error.*` rows are present.
**560 figure rows, unchanged.** No figure moved, because no behaviour moved — this snapshot records
*when* it was taken, not that the model changed, which is what the guard's own text says the series
means.

⚠️ **`key.csv` was then rewritten, and this is not the rebase antipattern the review-gate skill
warns about.** Committing the snapshot moved `stats`, a watched tree, so the first key no longer
covered HEAD. Real content moved; the re-key covers it. It converges rather than looping because
`reviews/` is not itself watched, so writing the key does not move the digest it records.
