# Review: the chip 2x2, the per-cell chip column, the free-hit horizon, and the wildcard bound

**Commit range reviewed:** `a6682de..HEAD` (7 commits at dispatch: `626584d`, `2a8e23e`, `47e0940`,
`37fbfa6`, `e447e4d`, `d0d65ec`, `8de7d56`). Fixes applied at `5edf13c`, snapshot at `9c15067`, and the
remainder at `5d9846c` — which this record is named for, since the gate diffs from the commit in the
directory name and the applied pass has to be inside it.

**What changed.** One non-test source file — `internal/analysis/chips.go`, where `EffectiveHorizon`
stopped truncating the squad's horizon at a free hit. Everything else was test infrastructure (four
new per-cell chip columns in the cells CSV), two new diagnostics (a 2x2 for the chip preparation
channels, a one-arm bound for the wildcard horizon question), and a large amount of new material in
`docs/notes/chips.md`, `docs/notes/transfer-policy.md` and `CLAUDE.md`.

## Reviewers run, and the triage

| reviewer | why |
|---|---|
| **fpl-code-review** | `internal/analysis` (scoring) and `internal/backtest` (harness) both moved |
| **fpl-stats-review** | three new points claims, one of which closed a line without a sweep |
| **fpl-findings-audit** | three documents gained claims; `CLAUDE.md` was also trimmed for size |

Skipped with reasons: **fpl-security-review** — nothing in `internal/agent`, `internal/fpl` or config
persistence; **fpl-run-review** — no live run wrote config; **fpl-season-maintenance** — none of the
four hand-maintained lists is touched.

**Invariants first, before any reviewer was dispatched.** The quantity this range must not move is
`HOLD`, and it did not: 47105 in all four arms of both sweeps, on all three captaincy rungs. A fresh
accuracy snapshot moves nothing but the commit stamp. `gofmt`, build, vet and both heavy packages
pass. Those checks are free and they held — and then the reviewers found a wrong charge, a false
premise, a restriction trap and four lost qualifiers, none of which any invariant could have caught.

## Findings, ranked by how misleading the state was

### 1. The bound charged a confidence threshold as a realised cost, and it changed the verdict — APPLIED

`TestDiagPreWildcardReturn` charged `FreeCost` (2.0) per free transfer against realised archive
points. That constant is a *confidence threshold*, not points surrendered: refusing a free move
recovers the points difference and nothing else. Counting it inflated `refusable` — which is
*defined* as what a rule could recover — by 2 per negative move, worth about 11 a season-path across
the 185 moves.

Both reviewers found it independently and both predicted the direction. Corrected and re-run: the
ceiling moves from **−18.5 to −14.0** and the total from **+5.8 to +15.8**.

**That reverses a methodological failure of mine.** The first run missed its pre-registered "close
under 15" and I closed the line anyway on the sign of the total — promoting a contextual expectation
to a decision rule after the primary criterion failed, which is the argmax failure applied to an
analysis rather than to a constant. The stats reviewer called that moving the goalposts and was
right. With the charge corrected the criterion is met on its own quantity, so the improvised closure
is withdrawn rather than defended.

### 2. "Truncation can only refuse moves" was false, and the bound rested on it — APPLIED

`gate.value()` is `Gain*Horizon + Money − HitCost*h − FreeCost*(n−h)`. Shrinking the horizon shrinks
the gain term against fixed charges, so it shifts the argmax toward fewer and cheaper packages — a
funded pair becomes a single, a hit becomes a free move — and `bestPair`'s `Alternative` is
`solo.Gain*horizon − freeCost`, so a shorter horizon *lowers* the pair's bar. Truncation substitutes
as well as refusing, and the replacement need not be a leg of what it displaced. A refusal also
diverges squad, bank and free count for every later week.

So the interval bounds the direct channel with the path held fixed; it is not a containment
guarantee for a sweep. The line is now closed on **mechanism** instead, using a figure already on the
record: a ceiling of order 14 cannot be measured against the transfer path's own **303 points of
spread with `HOLD` byte-identical**. That is a better argument than the one it replaces and it was
available the whole time.

### 3. My correction to the 2x2's consistency was itself the restriction trap — APPLIED

I reported the interaction as "19 of 23 negative — 83%", having excluded 13 exactly-zero cells as
ones where the intervention could not run. Checked cell by cell, **only one of the 13** has all four
arms equal. In the other twelve both interventions ran and the interaction measured exactly zero,
because one channel was inert and the other's effect was identical either way — the sweep's
**strongest evidence for additivity**, dropped from its own denominator.

That is the opposite of the triple-captain null, where the chip was never placed. Same shape,
different fact, and I applied the earlier lesson without checking that it transferred. The zeros also
cluster — 6 of 6 in 2023-24, 5 of 6 in 2022-23 — so the restricted set was three and a half seasons
and "83% negative" and "spread across seasons" were one conditioning counted twice. Now reported as
**19 negative, 13 zero, 4 positive of 36**.

### 4. The free-hit fix moves a recorded figure, and its own comment said it did not — APPLIED

I wrote "no recorded figure moves" because `AnticipateChips` is off by default. But
`TestDiagChipAnticipation` sets it **and plans a free hit**, so four decision weeks per cell that ran
on a truncated horizon now run on the full one — more than half that arm's mechanism, since its
wildcard only ever supplied three. The coherent arm's +2.5 a season, the −17 mismatch and the
201→223/201→128 transfer counts will not reproduce. Now recorded as superseded rather than stale.

The same commit also left the retired rule alive in a second place: `simulate.go`'s `AnticipateChips`
comment still said "a wildcard *or free hit* … That is `EffectiveHorizon`". Repairing one of two
copies of a disagreement is how the disagreement got there. Fixed, along with `docs/workflow.md` and
`docs/configuration.md`, both of which described the retired behaviour to users.

### 5. Superseded and mis-scaled figures throughout the write-up — APPLIED

- **The season line is one season.** By season it reads −3.5, −1.7, +5.0, +3.1, +18.7, **+58.2** —
  2025-26 alone is **73%** of the +13.3 mean, and it is the season that differs in kind. The
  previously published by-season row was the superseded run's. The chip week is not like this
  (+16.3/+1.5/+10.7/−0.2/+6.2/+9.2), which is the reason to reason from it.
- **"≈93% of the effect is the chip week"** is neither figure: 95.7% on the `per_gw` estimand, 80.6%
  on `per_path`. Quote the ratio with its estimand.
- **`per_path` now disagrees with `per_gw` × 38 by 32%** (+9.0 against +13.3) where the superseded
  run agreed to 18%. The sensitivity grew with the effect.
- **"Two thirds redundant" was inverted** — additivity predicts +13.1 and both returns +8.9, so about
  a *third* is lost and two thirds is what survives.
- **The entry-clustered t of +8.70 is a collapse, not corroboration.** That axis carries 0% of the
  contrast's variance, so the clustered SE falls below the independence assumption — the exact case
  `se_cr2_start`'s own comment retracts. It no longer sits unqualified beside +2.91.
- **`TODO.md` still carried the whole superseded run as current**, in the highest-traffic place.

### 6. Smaller applied items

- **The predecessor-header ladder was wrong and `chipWeekCols` was decorative.** I declared the
  constant with a paragraph of justification and never referenced it, so the regression test
  synthesised a header no build ever wrote while claiming it was the predecessor. It passed anyway,
  for a reason unrelated to what it documents.
- **The sequence diagnostic's printed banner contradicted the doc comment above it**, still telling
  operators that HOLD moving is expected — which would have waved through a genuine confinement
  failure.
- **`BenchBoostActive` tested a calendar window against a scored one.** A free hit inside the horizon
  now makes the two differ by exactly the skipped weeks, so a boost could fall inside the scored
  window and be reported as outside. Unreachable before the free-hit fix; reachable after it.
- **The interaction-cost pre-registration was wrong and is corrected in place.** I predicted an
  interaction costs about twice a main effect's SE. Measured here it is *cheaper* — 0.216 against
  0.599 season-clustered — because a difference of differences within one cell cancels the path
  divergence a single difference carries. The textbook rule is an independent-samples rule.
- **The by-season row of the bound divided by the whole start grid** rather than by cells that placed
  a wildcard, understating 2022-23 threefold.
- **The autosub bias is now counted rather than caveated**: 12% of shadow moves sold a player who
  then recorded no minutes, where `pointsOver` scores him zero and an autosub would have covered him.
- **`CLAUDE.md`'s budget is raised to 58 KB**, which is what the guard's own comment instructs on a
  second binding. Four qualifiers had been compressed away to fit: a unit on "+20.8", "as a
  measurement" on the anchoring null, "a decision null rather than a wiring null, witnessed three
  ways", and two whole results that had no resident entry at all and would have been rebuilt.

## Declined, with reasons

- **Building one multiplicity family across the whole range.** The stats reviewer notes ~10 testable
  contrasts here with four p-values in 0.03-0.08, and recommends reporting the count rather than
  adjusting across it, since the sweeps are different estimands measured sequentially under different
  pre-registrations. Adopted as stated — the count is reported, no cross-sweep adjustment is made.
- **The 2022-23 H1 blank count.** The audit reproduced 28 against the note's 8; the difference is
  exactly GW7, the round nobody played after the September 2022 postponements. Excluding a cancelled
  round from "blanks a chip could be aimed at" is right, so the exclusion is footnoted rather than
  the figure changed. ⚠️ **This record asserted that footnote before it existed.** It was written
  during the applied pass and added afterwards, at `5d9846c`, along with three other items this
  section had treated as done: the section heading that still read "owed a second run" after the
  second run landed, a surviving `+9.7 / 13-17` cross-reference, and the Holm asymmetry between the
  two metrics of one sweep. A review record that describes work not yet done is the same failure
  class as a comment describing code that no longer exists — noted here rather than quietly
  corrected.
- **Restricting the interaction to the 32 wildcard-placing cells.** Named in the note as a design
  caution rather than applied, because it is the same restriction this review is correcting in
  finding 3. The 4 structural zeros are identified by season and entry point instead.

## Applied after this record was first committed

Four items above were written as done and were not, plus two the record did not name. All landed at
`5d9846c`: the GW7 blanks footnote; the stale section heading and its four cross-references; the
`+9.7 / 13-17` pair in the verdict passage; the Holm family-of-two figure for the season line
(0.442); `harness_test.go`'s pre-repair clean-sheet block, marked rather than deleted since a
declined finding was built on it; and a `TODO.md` item for two-set `ChipPlan` support, whose only
home had been a section headed "refused" — the refusal is of the measurement design, not of the
capability, and 2026-27 runs the rule.

**The cells are now in the repository**, at `stats/cells/2026-08-13-chipprep3.csv` and
`2026-08-13-chipseq2.csv` with both provenance sidecars, which is what the provenance paragraph
asked for and did not get the first time.

## What could not be checked on this harness

- **Whether the corrected bound's conclusion survives a sweep.** It closes on mechanism, and the
  mechanism argument is about the instrument rather than the effect, so no run tests it.
- **The size of the free-hit fix on `AnticipateChips`.** Its recorded figures are superseded; nobody
  has re-run that arm, and it is off by default so nothing shipped depends on it.
- **The 2025-26 concentration.** 73% of the season line in one season is either the effect being real
  and season-specific or a single noisy column, and 6 seasons cannot separate those.
- **The chip-week endpoint's forking path.** `bench_boost_pts` was promoted to primary after the
  arm-level gap was already known to be favourable. That is not covered by any adjustment made here,
  and it is one more reason the verdict is `suggestive`.
