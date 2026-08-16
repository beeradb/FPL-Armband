# Review record — chip anticipation, the anchored plan, and the reveal lag

**Commit range reviewed:** `b1af338..a6f582a` on `worktree-oracle-harness`. Follows the record at
`2026-08-11-b1af338`, which covered the oracle harness and catalogue.

Adds `SimConfig.AnticipateChips`, `AnticipateGate` and `ChipPlanner`; the fixture-calendar census;
the anchored chip plan and its reveal-lag sweep; and one fix to shipped scoring code.

## Reviewers dispatched

| reviewer | why | outcome |
|---|---|---|
| **fpl-stats-review** | `internal/backtest` changed and four new measured claims | 7 findings, 2 of which retracted a headline |

**Skipped, with reasons.** `fpl-findings-audit` — the record edits here were made in response to the
statistical review and re-checked against it line by line; a second audit was owed but the session
hit an API limit, and it is the outstanding debt on this range rather than a decision that none was
needed. `fpl-code-review` — no refactor asserting byte-identical output in this range; the one code
defect was found by the statistical reviewer and verified against source before fixing.
`fpl-security-review`, `fpl-run-review`, `fpl-season-maintenance` — nothing in their scope changed.

## Invariants, which again did more than the reviewers

- **`HOLD` byte-identical across every chip arm.** Recorded explicitly as a *construction*
  guarantee rather than as evidence: `anticipate` reaches one site on the transfer engine and
  chips do not reach the held metric, so `HOLD` could not have moved. It is a leak check.
- **Full sight reproduces the anchored plan.** `laggedPlan(38)` is asserted equal to
  `anchoredPlan` on the bench-boost week *before* the sweep runs, so the five arms are one rule at
  five sights rather than five different rules.
- **Matched chip sets.** The control is derived from the anchored plan, so a chip the anchor could
  not place is dropped from both arms and the comparison stays about placement.
- **`TestFixtureLoadIsAppliedOnce`**, mutation-tested: fails at 50.08 against 30.08 without the
  guard.

## Findings applied

1. **"Refuted" retracted.** `AnticipateChips` does not implement the option-value hypothesis — it
   shortens the fixture-averaging window before a chip. `decide` never prices a future unwind in
   *either* arm, so a cost absent from both cannot be removed from one. The hypothesis is
   **unmeasurable on a myopic policy**, not refuted.
2. **`XIValue` squared the fixture load at horizon 1**, in exactly the weeks the chip work acts
   hardest — the transfer engine sits at horizon 1 before a chip, where `Score` already carries
   the multiplier. The comment asserting the collision impossible was true when written and
   falsified by `AnticipateChips`. Fixed by carrying `loadInScore` on the value rather than
   re-deriving the condition in the second consumer.
3. **The mismatch was the whole effect.** Adding `AnticipateGate` so the gate shortens in step
   moves POLICY from −17 a season (CR2 t −2.34) to **+2.5 (t 0.18)**, with transfer counts moving
   in opposite directions (201→223 mismatched, 201→128 coherent).
4. **The borrowed threshold was 4–6× too high.** These arms need 24 and 45 a season, not the
   "107–139" taken from the oracle arms' figures — a section that says they do not order.
   `FPL_CELLS` is now set on every chip run so each has its own standard error.
5. **Club concentration is censored, not flat** (2.70→2.80 against a hard cap of 3.00, a third of
   available headroom). `topThreeClubs` added as the uncensored statistic.
6. **The sign count needed clustering.** 21 of 24 cells became 3 of 4 seasons, and the mediator is
   near-mechanical anyway — an implementation check, not evidence about football.
7. **The `(HINDSIGHT)` label on the anchored arm was wrong**, corrected on the user's evidence
   rather than the archive's: the pool of candidate doublers is knowable weeks ahead from the
   public fixture list. Relabelled `(full sight)`, with the three-way split of what is knowable
   when recorded in `docs/notes/chips.md`.

## Declined, and why

- **A blanket liveness rule for every oracle.** It would fail `AxisChipWeek`, whose total
  invariance is the point. `MustMove` is a declared tier instead.
- **Reading the anchored result as established.** Four arms agree at +8 to +12 a season, but the
  Holm family is four and the full-sight arm's adjusted p is 0.387. Recorded as consistent with a
  mechanism and unresolved, which is the early-wildcard footing.

## Could not be checked on this harness

- **Whether squad *preparation* for a bench boost pays.** `XIValue` takes no bench weight, so the
  transfer search cannot price it at any level of knowledge. Queued in `TODO.md` with its design.
- **The judgement-layer residual.** "A double around GW34, probably these clubs, certain in a
  fortnight" is a web-search task; the replay has no judgement layer, which is the same reason it
  cannot measure perfect team news.
- **A second findings audit of this range**, as above.
