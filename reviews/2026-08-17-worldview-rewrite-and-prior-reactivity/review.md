# Review: the worldview-rewrite observer and the prior-reactivity 2x2

## What was reviewed

The `FreshChurn` extension of the repair-cost observer (RepairWeek fields,
the repairSquad/changesBetween split of repairChanges, the prevFresh
threading in Simulate) and the two pre-registered diagnostics it carries
(worldview_diag_test.go, priorreactivity_diag_test.go). The designs were
reviewed BEFORE any cell ran, per this project's rule that the statistics
review gates the plan.

## Reviewers

- **fpl-stats-review** — ran on both designs before the run. Seven findings;
  dispositions below.
- **fpl-code-review** — ran on the committed plumbing. All seven checks
  clean: repairChanges is exactly repairSquad + changesBetween (trigger
  semantics unchanged); FreshChurnOK gates the only consumer; prevFresh is
  per-cell and RecordRepairCost-only; the quarter-split guards hold on this
  grid's series lengths; the reading-rule code matches the registered rule;
  the 2x2's leversOn installs all four fields and the live-cell criterion
  matches the doc comment; censusOf's unplayed-round handling is inherited
  correctly. One documentation note applied (the FreshChurn set-difference
  direction is symmetric for two fifteens; the comment now says so).
- Skipped: **fpl-findings-audit** (no AGENTS.md or vault-facing text moved
  in this change — its triage row is the record surfaces), **fpl-security-
  review** (no client/agent/config surface touched).

## Findings and dispositions (stats review)

1. **Applied**: the middle-two-quarters window alone could not see the
   information-poorest regime the OVERREACT mechanism names — added the
   first-quarter mean and the OVERREACT-FIRST-QUARTER reading, and the
   caveat that STABLE refutes a mid-season rewrite only.
2. **Applied (as a registered limitation)**: the ON corner moves levers
   together — if the interaction resolves, no sub-attribution is licensed;
   the finding would be "the heavier prior under the joint configuration".
3. **Applied**: `WeeklyXI = true` inside leversOn (the package's own
   recorded trap: a doubles/blanks arm that leaves it false switches off the
   fielding half of its mechanism); the OFF arms stay at the shipped false,
   and the meaning of the B main effect is registered accordingly.
4. **Applied**: the stable cutoff is asserted, so the category counts are
   printed at stable cuts 2, 1 and 3 — if the dominant category shifts, the
   range is the result.
5. **Applied**: moderator floor registered — an entry-point half with fewer
   than 4 live cells of 6 is reported as insufficient data, not a finding.
6. **Applied (as a registered limitation)**: anchoredPlan is full sight, so
   the B main effect is an upper bound on what the levers buy.
7. **No code change**: the availability confound in FreshChurn survives as
   stated — the column is the sum of model reactivity, budget drift and
   availability turnover, the budget channel is bounded by printing it, and
   the finding must carry that OVERREACT also has an availability
   explanation the design cannot separate.

Nothing was declined.

## What could not be checked on this harness

Whether the interaction is big enough to clear its own threshold — that is
what the run is for. The rough arithmetic says the within-cell pairing buys
the contrast a factor of two in SE; the registered expectation is a
direction, not a resolution.

## Postscript — the runs, 2026-08-17

Both diagnostics ran to completion on the committed design (worldview 44m10s,
exit 0; 2x2 5m01s, exit 0, 48 of 48 cells). Results: REWRITE in 8 of 8
worldview cells at all three registered cutoffs; the levers contrast resolves
(+73.0 a season against its own threshold of 36.2, Holm 0.0105), the prior
contrast does not (−20.4 against 107.7), and the interaction points the
predicted way without resolving (+19.4 against 67.0). User-driven
decomposition sharpened the levers verdict — chip WEEK payouts are ≈16 a
season of the ≈46 raw; the rest is the free hit, anticipation and fielding —
and that correction is recorded in the finding and in the vault notes beside
this record. Per the user's direction the results are written to the vault
notes (recency-and-priors, chips), not AGENTS.md. The accuracy snapshot is
regenerated for the moved backtest tree. Nothing in this postscript changes
the review dispositions above.
