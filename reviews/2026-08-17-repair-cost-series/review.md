# Review: the repair-cost series observer and its pre-registered run

## What was reviewed

The `RecordRepairCost` observer (`SimConfig` switch, `SimResult.RepairSeries`, `wallet.clone`,
the `repairCost` split into `repairChanges` + `repairCostOf`, the frozen-arm snapshots in
`Simulate`), its two pinning tests, the pre-registered diagnostic
`TestDiagRepairCostSeries`, and the run of that diagnostic on four seasons × entry GW1/GW16
(8 cells, 236 observed gameweeks, exit 0, zero "no reading" weeks). Finding at
`stats/findings/2026-08-17-repair-cost-series.md`. Commit range: the uncommitted build as of
2026-08-17 morning, reviewed before commit, then `3b2278e` and its finding commit.

## Reviewers

- **fpl-stats-review** — ran, on the DESIGN before the run (this project's rule: the statistics
  reviewer gates the plan). Findings, all applied before commit: (1) the frozen-gross gap is a
  BOUND on the selling-rule confound, not a clean subtraction — the two pricings differ in
  budget size, and a negative gap would mean the extra budget upgraded away from the frozen
  squad; the comments now say so; (2) BOTH is the expected reading by construction — any
  standing gap plus any drift composes into a floor plus a decay — so the reading rule reports
  the two sizes and never the category; (3) injury and absence accumulation is a third mechanism
  with the same signature as a standing gap, carried as a residual caveat in the header, the
  finding, and the vault note.
- **fpl-code-review** — ran. Clean across the seven recorded failure classes: OK flags honoured
  at every consumer; `repairCost` is exactly `repairCostOf(repairChanges(...))` with the
  trigger's `OptimizeRequest` byte-identical to the pre-split version (behaviour-identical for
  the shipped-off trigger); `wallet.clone` deep-copies the only mutable field; the frozen
  snapshots are taken where the opening state exists and before the first decision; the
  `frozenFree` accrual matches `decide`'s rule with the join pinned by the confinement test;
  the confinement test compares the FULL path and is not vacuous (liveness half requires a
  non-empty series with OK readings); the "three Optimize calls a gameweek" cost claim is
  exact.
- Skipped: **fpl-security-review** (no client/agent/config-persistence surface touched),
  **fpl-season-maintenance** (no season lists touched), **fpl-run-review** (no live run wrote
  config). No replay other than the one under review was running; it occupied slot 1 of 3.

## Findings and dispositions

1. Stats, applied: bound-not-subtraction wording on the frozen-gross gap (comments in
   `simulate.go` and the diagnostic header).
2. Stats, applied: reading-rule stanza — BOTH is the expected reading; the verdict is the two
   sizes.
3. Stats, applied: injury-accumulation caveat in the diagnostic header, the finding, and the
   vault note.
4. Run observation, reported not explained: 2025-26 GW16 is anomalous (both arms rise,
   low head-third). Carried as a caveat; one cell, no mechanism claimed.

Nothing was declined.

## What could not be checked on this harness

The non-churn residual cannot be decomposed into standing gap versus injury accumulation on
this design — the frozen fifteen's deterioration and the argmax's ownership-unconstraint share
a signature. The series measures held-versus-fresh distance, not week-to-week movement of the
fresh optimum itself; that observable is unmeasured in every configuration and is queued.
