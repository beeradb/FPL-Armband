# Continuation of `reviews/2026-08-13-cae6941` — applying the rest, and one run

No new review was dispatched. **The substantive record is
[`reviews/2026-08-13-cae6941/review.md`](../2026-08-13-cae6941/review.md)** — three reviewers over
`4d61058..0a7a8ff`, ten conclusion changes applied, four declines with reasons. This file exists
because work continued after that record was taken, all of it either applying findings that record
already made or running a check it explicitly listed as owed, and the staleness guard is keyed on
the newest record rather than on whether the newest work was novel.

**Nothing here was reviewed by an agent.** That is the honest scope of this file: it is an
application log, not a second opinion.

## The one measurement

**The two-arm `FPL_NO_STARTS_REPAIR` check, which the previous record declined to run and listed
as owed.** Two processes on the six-season grid, `TestDiagBaseline`, 36 cells each, both exit 0
with the declared arm count. **Byte-identical in all 36 cells across all ten populated outcome
columns.**

It confirms a prediction two reviewers reached independently *by reading the code* rather than by
measuring: `reliabilityFrom` weights `startShare` by exactly zero, and `appearanceOdds` reads it
only in the `!unifiedAppearance` branch, which does not ship. So at shipped config no scoring path
reads `Starts`.

Three things follow. The harvest **cannot invalidate a recorded figure**, so no reproduction recipe
needs a starts data state. The check is **structural, not statistical** — no threshold, and it
cannot be a false negative from a noisy grid. And the harvest's value lies entirely outside the
replay's scoring path: the lineups and minutes oracles, which had been taking rank-reconstructed
starts as ground truth; the diagnostics; the agent-facing field; and the start/substitute/unused
multinomial that `reconstructStarts`' boundary two forbids.

Cells, provenance sidecars and the pairing script are in
`stats/snapshots/2026-08-13-aa95f75/`.

## Applied since the previous record

| finding | what changed |
|---|---|
| `fingerprint.go` described a squad-decision channel weighted zero twice over | Rewritten to state the expected byte-identity, and why the switch is still fingerprinted — it becomes a scoring switch under `FPL_RELIABILITY_SPLIT` or `FPL_NO_UNIFIED_APPEARANCE` |
| "The weekly fill carries essentially all of the effect" | **Withdrawn** in both places rather than softened, per the reviewer: SE ≈ 56 points a season at \|t\| = 0.07, and its six live cells cancel rather than agree |
| "The arms this frees still belong on `FPL_SWEEP_SEASONS=default`" | Corrected to the six-season grid — two thirds of the freed cells are off the four |
| "12 of 36 cells" in a block kept for its reasoning | Marked with the measured 18 rather than edited, since that block is preserved for its argument |

## Corrected in my own record

**The six-season aggregate arm was listed as owed and had already been run**, at `1a08d43`. Its
cells are `runC-6s-aggoff` and `runC-6s-norepair`. It reads **−17.5 points a season** on `HOLD`
over 18 reachable cells at season-clustered *t* = −1.21 on df 2, against the −3.7 that Run A's
single reachable season gave. Still unresolved, but with a season axis where it previously had
none.

## Still owed, unchanged from the previous record

- **`fplagent snapshot`** — the accuracy record predates the starts data.
- **The fourth cluster on `scoringPairNames`** — two arms; takes the expected-goals-conceded
  question from df 2 to df 3.
- **The bulk of `reviews/2026-08-13-backfill-runs/review.md`** — captured, verified by a second
  reviewer, applied only where a conclusion changed.
- **The three uncovered 2021-22 rows** — needs `reconstructStarts` to understand partial coverage.
