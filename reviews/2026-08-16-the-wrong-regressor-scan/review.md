# The wrong-regressor scan, and the reproducibility pin it exempts

Reviewed: the working tree of `guard-the-wrong-regressor-class` against `425be4b`. Three changes,
none of which moves a replayed point.

- **`internal/backtest/regressorscan_test.go`** (new) — `TestNonlinearTransformsScoreTheModelsOwnRegressor`,
  a `go/ast` source scan for a nonlinear transform (`math.Exp`, `math.Floor`, `poissonFloorDiv`)
  applied to a field of an archive row where the scoring path applies it to a field of
  `analysis.PlayerMetrics`; and `TestTheRegressorScanFindsWhatItClaims`, its positive control.
- **`internal/backtest/modelrepro_test.go`** — doc comment only. What its `exp(-g.XGC)` is and is not,
  what the pin actually reaches, and why it carries no `Fixtures != 1` guard.
- **`CLAUDE.md`** — one verdict bullet in *Standing rules*.

**Nothing here is a measurement.** No arm, no cells, no standard error, so no detection threshold
applies and none of the five contamination events bears. A green run after widening a source scan
proves that the widening ran, and nothing about the class being closed.

## Reviewers

| reviewer | why |
|---|---|
| **fpl-code-review** | the change is Go in `internal/backtest`, and its whole value is whether the AST walk decides correctly |
| **fpl-stats-review** | the disposition on `modelrepro_test.go` rests on a counting argument used to justify a deliberate divergence from a sibling diagnostic |
| **fpl-findings-audit** | `CLAUDE.md` gained a resident line |

Skipped, with reasons: **fpl-security-review** — nothing touches `internal/fpl`, `internal/agent` or
config persistence. **fpl-run-review** — no live run, nothing written to config. **fpl-season-maintenance**
— none of the four hand-maintained lists is touched. **fpl-docs-accuracy** — `docs/` unchanged.

## Findings applied

**1. The scan had no positive control, and its other assertions go vacuous by design.** Every
remaining assertion is a *count* against the debt list. Counts pin `seen` to exactly the sanctioned
number in both directions while the list is non-empty — but the list is meant to shrink, and an empty
one would leave a walk returning nothing for every argument passing green. Added
`TestTheRegressorScanFindsWhatItClaims`: ten synthetic sources parsed from strings, sharing one
implementation of the walk (`scanFileForHits`) with the scan itself. Half the cases are negative.

**2. FALSE POSITIVE — a selector's field name was resolved through the local-variable table.**
`SelectorExpr.Sel` is an `*ast.Ident` and the walk descended into it, so a local `xgc := g.XGC`
anywhere in the same function made every `b.xgc` read as archive-rooted. That is
`cleansheet_calibration_test.go`'s bucket accumulator verbatim — the line the header *promises*
escapes — and the cheapest fix a reader would reach for is raising that file's sanction, which would
mask a genuine third offender. Fixed: only the receiver is followed. Pinned by the
"a field spelled like an archive-rooted local" case, **verified to fail against the unfixed walk**.

**3. FALSE POSITIVE — a closure parameter shadowing an archive-rooted local inherited the outer
definition.** Parameters, named results and receivers were not counted, so `func(x float64)` kept an
outer `x := g.XGC`. The doc's *"shadowing is self-protecting"* was true of `:=` only. Fixed by
counting them (not defining them). Pinned by the "closure parameter shadowing" case, **verified to
fail against the unfixed walk**.

**4. The doc claimed the scan cannot over-report, and it can.** `bpsRow.Saves` is a local struct
spelled like an archive field; its sanction exists because of a name collision and can never be
retired by fixing anything. The header now says so, and the debt list's shrink discipline carries that
one exception explicitly.

**5. Reach widened.** Was `internal/backtest/*_test.go`. Now `internal/backtest/*.go` and
`cmd/priorblend/*.go` — non-tests because a reduction extracted into a helper would otherwise leave
the scan behind, and `cmd/priorblend` because it imports this package and builds `backtest.Player`
values directly. Neither carries a matching transform outside the debt list today, so the widening is
free and buys only future coverage.

**6. `numericFields`' doc justified the numeric filter with an example the test's own output
refutes** — `Player.Type` is `int` and is already in the guarded set. Corrected, and the consequence
stated: `Type`, `Team` and `Code` are categorical ints and a transform on one would produce a message
about regressors, which is confusing rather than wrong.

**7. `modelrepro_test.go`'s disposition was right in its conclusion and wrong in its argument.**
Three corrections, all in the header now:

- **The pin reaches less than the header claimed.** The dedup key is `[2]int{p.Team, gw}` and `gw` is
  the inner map's own key, so within one player every key is distinct and `range p.GWs` cannot change
  which rows are selected — only the float addition order. The bit-exact half cannot fire on repeats
  of one ordering; what it pins is that the outer loop is `sortedPlayerIDs`, **in this file's copy**.
  Being a copy, it cannot see the diagnostic regress at all.
- **The population argument counted the wrong channel.** xGC disagreement feeds only the `1e-4`
  tolerance. The bit-exact assertion needs disagreement on the clean sheet: 10 / 18 / 30 cells over
  2023-24 / 2024-25 / 2025-26, of which 6 / 14 / 26 are single-fixture. The conclusion holds — the
  guard would leave rows the order can still move — on a margin 3-9x smaller than first written.
- **The stated mechanism was wrong for a quarter to two-fifths of the cells.** `Player.Team` is the
  end-of-season club from `players_raw.csv`, so a mid-season transferee's earlier gameweeks are keyed
  to the club he finished at. Of the single-fixture cells disagreeing on the clean sheet, the rows
  also disagree on `GoalsConceded` in 6 of 6, 14 of 14 and 26 of 26 — they were not in the same match.

  All figures above were re-derived here rather than taken from the review, and reproduce it exactly.

**8. The `CLAUDE.md` bullet moved from *Things that have already bitten* to *Standing rules*,** beside
"One quantity, two implementations", which is the exact precedent for its class: a rule that was
written down and shipped anyway, named source scans, and a stated limit on what they reach. The
"bitten" section's preamble is *shipped bugs, each now covered by a regression test*, and this is a
measurement defect covered by a source scan. The claim was also scoped — convexity owns the *gap
between the two regressors*, not the realised-regressor level, which the file records as having a
second and unsized mechanism — and `PlayerMetrics` gained its first-use gloss.

## Declined

- **Refactoring the package's other source scans onto a shared walker.** They answer different
  questions and share only "parse and Inspect"; a shared walker would be a third implementation to
  keep honest, and the standing remedy it would invoke is about a duplicated *quantity*, of which
  there is none here. Both the code review and this record agree.
- **Extracting the clean-sheet team-match reduction into one helper called by both the diagnostic and
  the reproducibility pin, and asserting across a permuted player order.** This is the correct fix for
  finding 7's first bullet and it is **owed work, not done here**: it changes what a test asserts,
  which is a larger act than a disposition, and it wants its own review. The header says so in terms
  rather than leaving the reader to infer it.
- **Sizing the end-of-season-team keying defect in `TestDiagCleanSheetPoisson`.** A figure was offered
  (0.4-2.8% of team-gameweeks mis-selected) and is **not re-derived here, so not recorded anywhere as
  established**. Its arm reaches `docs/accuracy.md`, so it is worth doing — and it is precisely the
  class this scan's own header says it cannot catch: a matched regressor with a mismatched population.
- **A resident line about that keying.** One measurement, unreviewed. It has not earned one.
- **Editing the `## Conventions` bullet in `CLAUDE.md` and the paragraph in
  `internal/snapshot/notes_test.go`.** ⚠️ **Neither is this branch's work.** Both were already present
  in the worktree before this branch's first edit, they are one change with each other, and they are
  **excluded from this commit**. The audit's finding on them — that two of the bullet's five clauses
  narrate the bullet's own provenance, which is the thing the bullet forbids — is recorded here for
  whoever owns them. Editing another session's in-flight text would be worse than leaving it.

## What could not be checked on this harness

- **Whether the guard would have caught the defect that motivated it.** It would have caught the
  *shape* — that is what the positive control demonstrates — but the defect reached five surfaces
  through a diagnostic that was, and remains, a legitimate realised-xGC arm. What failed was quoting
  its size without naming its regressor, and no source scan reaches a quotation.
- **Whether the reproducibility pin has ever been the thing that caught a regression.** It is
  `DIAG`-gated, so it does not run in the default suite, and nothing records when it last ran.
- **The three unguarded shared field names as a risk.** `Minutes`, `Goals`, `Assists`, `Bonus`,
  `Starts`, `ID` and `TotalPoints` are archive fields whose names `PlayerMetrics` also carries, so a
  nonlinear transform of a realised one is invisible. There is no site today; there is also no
  mechanism that would make one visible, which is why the limit is in the header rather than a TODO.

## Verification

`go build ./... && go vet ./... && go test ./...` green. Both new controls were verified to **fail**
against a deliberately reverted walk and pass against the shipped one; the debt list's
shrink-direction was verified by raising a sanction count. `git merge-base --is-ancestor origin/main HEAD`
exits 0.
