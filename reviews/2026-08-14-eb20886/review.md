# Banking `BlendRateK`, and finding out it reverses a table nobody had re-checked

Covers `e3c50b4..eb20886` — the pre-registration, the `EXP=BLEND` sweep and its cells, the
write-ups, and the corrections two reviews produced.

## What was run

`EXP=BLEND scripts/replay -run TestDiagRejudge` — arms `BlendRateK` 8 (ships) / 12 / 16 / 24 on the
six-season grid at entry gameweeks 1/6/11/16/21/26. **144 cells, 10m49s, peak RSS 111 MB.**
`sweep_inference.R` reproduces Go's paired means with **max abs diff 0**.

It closes the gap the schedule screen named and could not reach: the block had existed since it was
written and had never been run to the archive.

**Nothing resolves.** `HOLD` reads −11.6 / +11.6 / +12.6 a season for 12/16/24, non-monotone, Holm
1.000. **8 ships unchanged.** The pre-registered *schedule* direction held at t 1.28, p 0.257, which
is the whole result.

## Reviewers dispatched, and why

The change touches `stats/*.R` (the screen's curated set), `docs/` and `CLAUDE.md`, and it is a
points claim about a scoring constant. The triage table's union is **fpl-stats-review** and
**fpl-findings-audit**, run concurrently on committed state.

**fpl-code-review was not owed** and this was a judgement call: the only code change was one line
added to a curated list, plus comment edits. Recorded as a decision so the next pass does not
re-ask. In hindsight it would have found nothing the other two did not — both independently found
the headline below.

**Not owed:** security (no credential, cache, agent or config path), run-review (no live run wrote
config), season-maintenance (no hand-maintained list).

## The invariant, first

Adding `blend.csv` to the screen's `--committed` set must not move any other ladder. Checked before
dispatching: **all 28 pre-existing arm rows are bit-identical**, every numeric column max-abs-change
0; only the Holm family grew, 28 → 31, and every value stays 1.000.

## Findings applied

### 1. The run reverses a recorded table, and I appended mine 1,600 lines below it — BOTH reviewERS, independently

`docs/notes/constants-and-sweeps.md` already carried a 24-cell table of **the same three arms,
against the same baseline, on the same metric**, under "Two attempts to fix the mid-season
over-confidence, both refuted": monotone decline, `k=12` at t −2.18 and `k=24` at t −2.27. This run
reads −0.306 / **+0.306** / **+0.331**. I never mentioned it.

That is a direct breach of the standing rule that *a later correction is marked in place with the
commit that caused it*, and it is the failure the rule exists for: a reader landing on the old
section gets "monotone decline, two arms clearing |t| = 2".

**Verified before applying**, because the obvious explanation would have been the wider grid:
restricted to the recorded table's own four seasons the new cells read **−0.445 / +0.302 / +0.124**,
so it is not grid width. The recorded **regime inversion reverses too** — pooled over arms, early
−0.05, middle −0.93, late **+1.32**, against the recorded +0.936 / −0.611 / −1.783.

Applied: the old table is marked in place with the retraction; the new section points back at it;
and the consequence is recorded — **`k=8`'s "two independent methods on different data — the
strongest support any constant here has" is now one method**, because the second was that replay.

⚠️ **No cause is written.** No Go file changed in this range, nobody has bisected, and the old table
has no banked cells to re-check. It can be marked stale and not explained.

### 2. My GW1 mechanism was false, four times over

I wrote, in the pre-registration and three downstream documents, that "at a GW1 build `n90 = 0`, so
`k` cannot touch the opening fifteen". Both halves are wrong:

- `internal/backtest/replay.go:82` sets `el.Minutes, el.Starts = q.Minutes, q.Starts`, so pre-season
  the element carries **last season's** minutes and `n90` is ~25-30 for an ever-present — which is
  CLAUDE.md's own standing rule about FPL's aggregates carrying last season's totals until GW1.
- `k` cannot reach the opening fifteen because `blendFor` returns from its `played == 0` branch
  (`internal/analysis/blend.go:297`) **before** `rk` is read at `:359`.

Right conclusion, and the in-season leverage argument the prediction rested on is unaffected — but
GW1 is a **separate code path, not the limit of the same curve**. Verified in the source before
applying; marked in place in the pre-registration, which is otherwise not edited after a run.

### 3. Things I withdrew about my own statistics

- **"At half its own MDE" is not a second fact.** `swing / threshold` is *identically* `t / t_crit`
  — span, range and the 38 all cancel — so 115.2 against 231.3 says exactly what p = 0.257 says. I
  had it in a heading, a bullet, a TODO body and a commit subject: four impressions of one
  non-result.
- **"MDE" was the wrong word**, in the note, in FINDINGS and in the screen's own column header. 231
  is the p = 0.05 detection threshold; the 80%-power MDE on `variance_components.R`'s convention is
  **314**. The script now prints `thr05` and `mde80` as separate columns.
- **"The largest of the seven ladders" is an argmax over seven on a non-neutral scale.** The swing
  multiplies by each ladder's own setting range, and BLEND's 8→24 is the widest in the bank — the
  same arm design I call a defect two paragraphs later. On the scale-free ratio it is **second**,
  0.498 against `FIXW`'s 0.516.
- **The three caveats are one fact.** The five largest GW26 arm-cells are 2021-22 and 2023-24 —
  exactly the two carrier seasons — and the largest is also the max-|d| cell. Presenting them as
  "three caveats, any one sufficient" reads as three independent problems; it is one shape counted
  three times, which is worse. And I stopped at "dropping either halves it" when **dropping both
  reverses the sign** (−0.0015, t −0.65), and 67% of the statistic is the GW26 column alone.
- **The discrete-flip diagnosis named one channel where there are two**, and I invoked the
  triple-captain precedent to consider suppressing the p. That precedent does not apply: there only
  23 of 36 cells *placed the chip*, whereas here every cell ran the intervention and the zeros are
  measured "changed nothing" outcomes. **Keep the p.**
- **The late column's "1.67× over-length"** is measured against a null nobody should expect — a
  squad flip is a persistent offset, not an average of independent weeks — with a denominator that
  is quiet for a mechanism reason. The substantive point (per-column SD 0.14 → 4.47, the statistic
  rests on GW26) survives; the quantification does not.

### 4. The post-hoc argument was wrong even though its conclusion survives

I argued the shape separates the calendar and evidence readings because "a GW11 entrant contains all
of GW16-28 and so should gain most". **Containment is not the criterion** — GW1, GW6 and GW16
contain it too. The quantity that varies is the band's *share* of the scored horizon:
0.34 / 0.39 / 0.46 / **0.57** / 0.44 / **0.23**. So the calendar reading predicts **GW16 largest and
GW26 smallest**, and the observed GW26 maximum refutes it for a *better* reason than I gave.

I also cited "consistent across all three arms" as corroboration. It is not: the arms share the
`k=8` baseline cell, whose single draw carries the GW11 spike — a point my own script's comments
already make about the per-arm contrasts. Restated on the **baseline-free ladder slope**, which
cancels the common baseline by construction and does reproduce the ordinal claim (+0.010, −0.011,
−0.086, −0.020, +0.076, **+0.210**) — while being **non-monotone, fitting neither reading**.

### 5. Smaller corrections applied

- The scope qualifier **"with banked `HOLD` cells"** restored to the schedule bullet; I had
  over-generalised it to every scoring constant on evidence covering nine sweeps.
- Stale counts: 28 → 31 arm contrasts, "five of six ladders trip it" → six of seven, the two "no
  banked cells" clauses, and the note heading that said the candidate "has no cells".
- **Two orphaned TODO paragraphs** left over from the old BLEND item now hung under "decompose
  within a season" and told its runner to pre-register the block comment — a prediction this run
  had just answered. Re-scoped.
- The **sweep block's own comment** now records that its prediction was tested and half failed, so
  the next runner does not re-register it.
- **Prediction 1 was registered and half failed** (`k=12` came back negative) and only FINDINGS said
  so; now in the note and TODO too.
- `sensitivity.R` **banked**, so every table in FINDINGS reproduces from the snapshot rather than
  from one session's scrollback. It sources `cells_common.R`, so it cannot drift from the screen.
- The data state is now named, including that the provenance records `dirty: true` — benign, since
  no Go file changed in the range, and said rather than left unexplained precisely because finding 1
  turns on which code produced a table.

## Declined

- **Writing a cause for the reversal.** Both reviews wanted it marked; neither could attribute it,
  and nor can I without a bisect. Recorded as unattributed, which is the honest state.
- **Suppressing the `POLICY` column.** It is not the arm for a scoring constant, but both estimator
  ends are now quoted per the standing rule rather than the friendlier one.
- **Re-scoping the Holm family after seeing results.** Same reason as last time: choosing a family
  after the fact is the move this record objects to.

## One reviewer figure did not survive recomputation

fpl-findings-audit reported that `POLICY` is dismissed on the flattering estimator, giving
start-fixed t of **−2.17** and **−2.32** (p 0.082, 0.068) against CR2's −0.69 and −0.87. The actual
values in `inference.csv` are **−0.710, −0.624, −1.566** (p 0.484, 0.538, 0.130) — nothing near −2.3.
The recommendation (quote both ends) was right and is applied; the figures were not, and the
conclusion it drew from them — that "not evidence of anything" was written against the friendly end
— does not hold.

fpl-stats-review's 80%-power figure was also off: it used `qt(0.90, 5)` where
`stats/variance_components.R`'s convention is `qt(0.80, df)`, giving 364 where the repo's own
formula gives **314**. Applied at 314.

## What could not be checked on this harness

- **Whether `BlendRateK` wants to be a schedule.** t 1.28 against a p = 0.05 threshold of 231 and an
  80%-power MDE of 314 — *unresolved*, and specifically not evidence that it is flat.
- **Which change caused the reversal** in finding 1. Needs a bisect; the old table has no cells.
- **The calendar reading of "helps most in the middle regime".** Entry columns cannot test it; it
  needs a within-season decomposition this harness does not produce. Queued.
- **Whether `k` below 8 is better.** The arms start at the shipped value, which is the design defect
  flagged before the run. Queued.
