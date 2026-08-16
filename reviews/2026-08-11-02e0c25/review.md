# Review record — widening the default replay grid from four seasons to six

**Commit range reviewed:** `e555c88..0b994d5` — `6acc5ad` (pre-registration, written before any
cell ran), `2a2d976` (verdict, analysis script, evidence), `0b994d5` (the flip and the doc
corrections it forced). Previous record:
[`2026-08-11-6d65e04`](../2026-08-11-6d65e04/review.md).

**Named for `02e0c25`**, the commit that carries the corrections this review produced. The review
itself read the three commits above; the guard resolves a record's directory suffix as a revision,
so a descriptive name is invisible to it and the name has to be a hash.

## Reviewers dispatched, and why

Per the triage table, a change touching `internal/backtest` and `stats/*.R` owes
**`fpl-stats-review`** and **`fpl-findings-audit`**. Both ran, concurrently, read-only.

Skipped with reason: `fpl-code-review` (no `internal/analysis` change — the diff is the grid
selector, its guard test, evidence files and prose); `fpl-security-review` (nothing touches
credentials, the cache, config persistence or the agent loop); `fpl-run-review` (no live run);
`fpl-season-maintenance` (the hand-maintained lists are untouched).

**The gate is not theatre here.** Neither reviewer returned "looks fine". Between them they
refuted the change's central mechanism sentence, found a scoring term silently disabled in a
third of the new grid, and found the guard test I had just written to be tautological.

## What was verified independently before anything was applied

I do not take a reviewer's report as the finding — this project has had a reviewer misattribute a
movement to a commit that provably caused none. Recomputed from the raw cells in
`stats/snapshots/2026-08-11-6acc5ad/`:

- season-clustered SE **0.1027** (four) and **0.0860** (six), reproducing `VERDICT.txt` exactly;
- `HOLD` threshold **12.4 → 8.4**; **`POLICY` 14.4 → 16.1**, confirming the direction reversal;
- all fifteen four-season subsets **8.2 to 16.0**, median 12.8; all six five-season subsets
  **8.4 to 11.0**; every five-season subset beats the median four-season one;
- `expected_goals_conceded` **exactly zero** in 2021-22 and beginning at **GW16** in 2022-23;
- `defcon` non-zero in **2025-26 only**;
- the clean-sheet and concede terms gated on `XGC90 > 0` at `metrics.go:1885` and `:1892`;
- the strengthened guard test **mutation-tested**: with `extendedPairNames` silently returning
  four pairs it now fails, where the previous version passed.

## Findings, ranked by how misleading the state was

1. **The mechanism sentence was false.** "The extra noise lands in the within-season spread, which
   the clustered SE divides away" — it does not. Within-season noise enters as the sampling error
   of each season mean: **37%** of the four-season clustered variance and **57%** of the
   six-season one. The borrowed offset costs ~**1 point a season of threshold (~12%)**, a lower
   bound. This was the sentence the change most rested on.
2. **Two of the six played seasons have no clean-sheet term at all.** Worth 26-45% of a defender's
   or keeper's points; any arm acting through that channel is inert in **12 of 36 cells**.
3. **The result is `HOLD`-only and `POLICY` moved the wrong way** (14.4 → 16.1), while the text
   applied a `HOLD`-derived ratio to a pooled median mixing both metrics.
4. **Neither dilution guard could have fired.** P1 passes under *total* dilution (0.2735 inside
   [0.0834, 0.7369]); P4 was separately powerless. "Effect grew *and* SE fell" is one observation,
   not two — both are functions of the same six season means.
5. **"Nothing recorded is invalidated" conflated grid-invariance with time-invariance.** Both
   sides of the nesting check are post-backfill, and the backfill touches two of the four shipped
   pairs.
6. **A third reconstruction nobody had named:** `starts` is absent before 2022-23 and inferred by
   rank (~8,200 / 7,400 / 7,000 rows), biased 3:1 toward making fringe players look nailed — and
   the positive control measures blanking, exactly that population.
7. **P2 is near-vacuous:** three of four near-null arms are byte-identical in 32-36 of 36 cells;
   one arm is zero in every cell on both grids. Two of P3's three ratios come from arms whose
   entire signal is in one season, which pins `t` at exactly ±1.00 by construction.
8. **I wrote a number that exists nowhere in the evidence** — "0.571 to 0.607" was a selection
   across two metrics. The three `HOLD` ratios are 0.677, 0.561, 0.571.
9. **The guard test was tautological**, comparing `sweepPairNames()` against `extendedPairNames()`
   — the function under test — so the regression it exists to catch would have passed.
10. **The strongest argument for the change was never made:** the subset shape (item above),
    which is monotone across grid width and is the standard this project demands over an argmax.

## What was applied

- CLAUDE.md's canonical block rewritten: the shape argument replaces the point estimate as the
  support; the `POLICY` reversal recorded; the false "divides away" mechanism marked refuted and
  priced; the nesting claim narrowed to grid-invariance with the `FPL_NO_XG_REPAIR=1` requirement
  stated; the four limitations listed; the dilution guards' powerlessness recorded; the
  unfollowable "11-to-34 band" instruction replaced with a mechanical rule *and* the note that
  `mde.csv` is gitignored so it cannot yet be run; the 0.607 error corrected in place.
- `sweepPairNames`' and `extendedPairNames`' comments corrected to match, including the
  clean-sheet inertness with its file:line evidence and the `starts` reconstruction.
- The guard test pinned to a literal in both directions, `"scoring"` pinned for the first time,
  and the whole thing mutation-tested.
- A dated postscript on the pre-registration — **appended, not edited**, since a pre-registration
  edited after its result is no longer one.

## What was declined, or deferred, and why

- **Relabelling the recorded "resolves 12.7 on `POLICY`" as a `HOLD` figure. Declined.** I had
  proposed this; the audit showed the metrics *swap order* between estimators, which a mislabel
  cannot produce. They are method-of-moments and CR2 with different df. Applying my "fix" would
  have corrupted a correct label. Which estimator is canonical is **undecided and recorded as
  such** rather than settled by fiat here.
- **Reverting the flip. Declined.** Both reviewers judge it defensible on `HOLD`, and the subset
  shape is stronger evidence than the headline ratio it replaces. The correct response was to
  narrow the claim, not withdraw the change.
- **The 26 hardcoded "24 cells" / "four seasons" labels in diagnostic `Printf`s. Deferred**, with
  a TODO entry. It is a real instance of this project's signature failure — a label that no longer
  describes what it names — but it is a mechanical change across seven-plus files, and the fix
  worth having is to derive the label from `len(sweepPairNames())*len(sweepStarts())` and widen
  `TestTheGridIsDeclaredOnce` to catch prose as well as literals. That is its own change.
- **Count drift in `docs/replay.md`, `stats/README.md`, `docs/accuracy.md`, `README.md`,
  `docs/architecture.md` and `internal/snapshot/render.go`. Deferred**, with a TODO entry.
  `docs/replay.md` and `render.go` are the two that matter, because a reference document
  disagreeing with the code is wrong by this project's own rule, and `render.go` prints
  "four seasons, all it ever will" into the generated snapshot the record trusts *over* its prose.
- **Re-deriving any constant on the new grid. Out of scope**, and now explicitly blocked: the
  mechanical re-derivation list needs `mde.csv`, which is gitignored and absent.

## What could not be checked on this harness

Nothing here is **unmeasurable**; everything open has a run that would answer it. Listed in
ascending cost, all **unmeasured**:

- whether the xG backfill moved the shipped four's cells — 96 cells, `FPL_SWEEP_SEASONS=default`
  with `FPL_NO_XG_REPAIR=1` against unset. **This is the highest-value follow-up**, because it is
  what "nothing recorded is invalidated" actually needs;
- whether `viceCaptainFallback` fires on the `POLICY` path in 2022-23, where the arm moves `HOLD`
  in all six cells while `policy_points` is identical to the point in all six — one instrumented
  run, no sweep;
- whether the widening helps or hurts an arm with a genuinely season-dependent effect — ~240
  cells, and the question the `POLICY` reversal raises;
- what the borrowed offset costs applied to a season with native xG — ~96 cells.

Two attributions remain **suspected, not verified**: that the backfill caused the vice-captain
figure's 11.5% drift, and that 2021-22's large control effect is Omicron rather than the
reconstructed `starts`.

## Evidence hygiene noted for next time

The archived evidence is missing both sidecars the harness writes for every sweep — the
provenance file (commit, dirty flag, constants digest, environment) and the means file, which
exists so R's arithmetic is checked against Go's. Their absence is why several questions above are
arguments rather than lookups. Any future snapshot should carry both.
