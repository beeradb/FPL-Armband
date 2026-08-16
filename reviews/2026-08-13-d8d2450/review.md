# Branch close-out — `4d61058..d8d2450`

No new review dispatched. The substantive record is
[`reviews/2026-08-13-cae6941/review.md`](../2026-08-13-cae6941/review.md) (three reviewers over
`4d61058..0a7a8ff`), continued in
[`2026-08-13-a6626ff`](../2026-08-13-a6626ff/review.md). This closes the branch: everything the
three reviewers raised is now either applied, declined with a reason, or run.

## The two runs made to close it

**The two-arm `FPL_NO_STARTS_REPAIR` check.** Byte-identical in all 36 cells on all ten populated
outcome columns, confirming a prediction two reviewers reached independently by reading the code.
At shipped config no scoring path reads `Starts`, so the harvest cannot invalidate a recorded
figure and no reproduction recipe needs a starts data state.

**The fourth cluster on `scoringPairNames`.** Review refuted "its price can never be measured
here"; the run then refuted the hope behind it. The cluster is real — 24 cells move over four
seasons, 18 inert — but **2019-20 reads +204 a season against three negative seasons**, so the
pooled mean flips from −34 to +25, the clustered *t* is +0.42 on df 3, and the threshold triples to
192. **More clusters do not help when the added one is an outlier.** The repair's price is
unresolved on every grid that can reach it, and the −34 was carried by three seasons that agree
rather than by an effect the design can see.

## Everything else applied since `a6626ff`

| finding | disposition |
|---|---|
| "No *t* is quoted because none can be" | Wrong. Figures now quoted with SEs and labelled *unmeasurable on this grid* — three season means identically zero pins the clustered *t* at 1.00 |
| "The ordering of the top two is stable in all four corners" | Withdrawn — one observation counted four times. Restricted to the six responding cells it inverts |
| "The strongest end-to-end check available here" | Paired-difference invariance, not pipeline invariance: one cell moved +41 in both arms underneath it |
| The vice ledger's "four fifths" | A chain, not a fraction; `c76c0d8` alone is 129% of the net. Both backfill terms marked single-season bookkeeping |
| "P1 holds on three independent sets of arms" | Two sets, and not independent — `xgoff` **is** `bothoff` by construction |
| "Reproduces the 6-of-24 result" | Covers the expected-goals contrast; the other two are new results |
| The `FIXW` hedge, dropped in compression | Restored: margins of 1-8 a season, inside the noise |
| The six-season vice figures | Flagged pre-repair; the current value is unmeasured |
| 2022-23's xG gap in the present tense | Past tense, and *why* all six cells now move |
| The `implied` xG/xA row | Relabelled: a direct contrast between two corners, conditioned on xGC being off in both |
| The three uncovered 2021-22 rows | **Fixed** by the post-cache ordering — see below |

## The partial-coverage gap closed itself

Moving the repair after the cache fixed the reviewer's finding 5 as a side effect: the
reconstruction now runs first on the full club-gameweek and the repair overwrites only what the
harvest reached, so a missed player keeps an *inferred* start with the flag true rather than a
confident zero. After load, 2019-20 / 2020-21 / 2022-23 each total exactly **8,360** with nothing
inferred and no conflicts; 2021-22 totals **8,359 with 2 inferred**.

The single residual start is honest: Weghorst's Burnley GW23 cell was dropped as ambiguous by the
per-match guard, and `reconstructStarts` declines double club-gameweeks by design. A real start is
unattributed rather than guessed at — both mechanisms behaving as designed where they meet.

## The byte budget was not raised

`CLAUDE.md` went over twice while applying these. The constant's own comment says to raise it only
to restore a hedge, never for evidence, and to audit the index if a third restoration binds. The
audit said the index had started carrying its notes' numbers again — the fourth-cluster figures and
the drift chain. Those moved to `stats/snapshots/2026-08-13-aa95f75/` and the verdicts stayed. It
fits at 54 KB.

## Declined, and still open — deliberately

- **`fplagent snapshot` is not re-run.** `TestSnapshotCoversTheCurrentCode` passes, so it is not
  blocking; refreshing the accuracy record is a separate exercise with its own eight diagnostics.
- **The bulk of `reviews/2026-08-13-backfill-runs/review.md` beyond the conclusion-changing set.**
  The remainder is wording and emphasis, recorded for whoever next touches those sections.
- **The `MinutesWeight` tie-break.** Restricting the contrast to ever-presents, where the
  reconstruction's own error is 3.9% rather than 29.7%, would break the non-independence. Cheap,
  unrun, and named in the record.

## What this branch cannot claim

No shipped constant moved and no score changed. The starts harvest is byte-identical at shipped
config *by design of the model*, not by luck. What changed is that four and a half seasons carry
recorded starting elevens instead of a rank guess with a 3:1 role bias, the sweep evidence base
survives a clean checkout, and four recorded verdicts are labelled data-state-dependent rather
than settled.
