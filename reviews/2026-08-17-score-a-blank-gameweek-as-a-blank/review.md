# Scoring a blank gameweek as a blank

Reviewed: `d8a3eb9..HEAD` on `score-a-blank-gameweek-as-a-blank`, four commits, against the
merge of `development` that is this branch's base.

## What was reviewed

A correctness fix on the scoring path, plus the three defects it exposed and the one it created.

`fixtureLoadFor` — matches per gameweek, the multiplier the imminent-week eleven is picked with —
anchored its window on the club's next **fixture** rather than the next **gameweek**. If the club
blanked the imminent round, its first remaining fixture was a week later and the window slid
forward with it, so the blank disappeared. At horizon 1, where `FixtureLoadInScore()` applies the
term to `Score`, the window was `[first, first]` and therefore held a fixture by construction: the
load was **≥ 1 always**, and the "playing not at all scored as though it had" half of the term's
own comment described a case that had never once executed.

Checked against the archive's true fixture count over every club-gameweek of the six-season grid,
the old anchor missed **170 blanks and zero doubles** in 4,540 comparisons. It now misses none of
either.

Four consequences, each with its own guard:

1. **The window honours the skip set.** `engineAt` isolates one gameweek by skipping every round
   before it; reading `byTeamUpcoming` raw ignored that, so a club with an imminent double had
   every player doubled in *all* projected weeks.
2. **The free hit excludes clubs that do not play.** Zeroing the score is not enough — swept over
   the six seasons' blank rounds at four budgets, score-zero alone still left a blanking player in
   the fifteen in **17 builds of 160**, because a builder with four bench slots is indifferent
   between two footballers worth nothing and takes the cheapest.
3. **`fixturesPerGameweek` is deleted.** Its only callers were its own test's assertions while the
   live quantity had no test at all, and the two disagreed on precisely the leading blank.
4. **The denominator is the gameweeks the window found**, so an end-of-season horizon-5 window over
   two remaining rounds reads 1.0 rather than 0.4. At horizon 1 it is always 1.

## Reviewers

| reviewer | ran | why |
|---|---|---|
| **fpl-code-review** | yes | `internal/analysis` (scoring) and `internal/backtest` (harness). Found four defects, all confirmed before acting; one is severe and was *created* by this fix |
| **fpl-stats-review** | yes | the branch quotes a points figure, and the triage lists it for both trees |
| **fpl-findings-audit** | yes | `AGENTS.md` was edited, and the triage lists it for that and for `internal/analysis` |
| **fpl-docs-review** | yes | `docs/model.md` changed, and the record written for this change needed routing between the two stores. Found the two defects at the head of the list below, one of them a false claim in *this file* |
| fpl-security-review | not applicable | no change to `internal/fpl`, `internal/agent` or config persistence |
| fpl-season-maintenance | not applicable | none of the four hand-maintained lists is touched |
| fpl-run-review | not applicable | no live run wrote config |

Invariants were written before the reviewers were dispatched, per this gate's first section: the
archive probe (an exact comparison against 4,540 club-gameweeks), the doubles-must-not-break
assertions, and the free-hit selection check. The archive probe is what found the defect in the
first place, and it caught more than the reviewers did.

## Findings applied

Ranked by how misleading the state would have been.

**0. This file claimed a correction that had not been made.** *(docs review)* The entry under
*Findings declined* said the false thread-safety justification on `Engine.upcomingGWs` had been
"removed instead" — and the tree still carried the strongest form of it, newly added by this
branch: "built once by buildFixtureIndex and read-only thereafter, so it needs no lock."
`ApplyChipPlan` calls `buildFixtureIndex` a second time, from a tool handler the runner fans out
concurrently. **Applied**: the comment now names the writer, the concurrent reader and that it is
unfixed, and the declined entry says what was actually done. Worse than the race itself, because a
dated tracked artefact asserting a lock is unnecessary is what makes the next reader skip one.

**0b. `docs/model.md` said the term does not scale a fifteen built from scratch**, which the free
hit has contradicted since this branch added `ExcludeIDs` to that very builder — and
`docs/replay.md` already said so, so two reference documents disagreed in the section a reader
consults for exactly this. **Applied.**

**1. A wildcard was being built on one week's blanks — and only because this fix works.**
*(code review)* `WeekViews` built both rebuilt squads on `engineAt(gw)`, a horizon-1 engine where
`FixtureLoadInScore()` is true. Once `fixtureLoadFor` could express a blank, every blanking club's
score was zero *before* `Optimize` ranked on it, so a wildcard planned for a heavy blank (2023-24
GW29 blanked twelve clubs of twenty) returned a permanent fifteen drawn only from the clubs that
happened to play that week — a free-hit squad presented as a wildcard, and **kept**. The comment
justifying the split, "a wildcard is deliberately left alone", was true of the pool and false of
the objective. Reproduced on the synthetic calendar: the blanking club takes **0 of 15** places at
horizon 1 and its full cap of **3** at horizon 5. **Applied**: a new `engineAtHorizon(gw, horizon)`;
wildcards build at the caller's horizon, the free hit keeps horizon 1 and the pool exclusion,
because one round *is* its whole horizon. `TestAWildcardIsNotBuiltOnOneWeeksBlanks` pins it and
fails on the previous builder.

**2. `> 0` stopped meaning "was this computed", and my own comment had the reachability
backwards.** *(code review)* The previous commit recorded the `xiValueForTransfer` guard as
unreachable at horizon 5 and reachable at 1. Both halves were wrong: at horizon 1 `loadInScore` is
true and the guard short-circuits before reading the load at all, while at horizon **2 to 4** —
where `EffectiveHorizon` puts the transfer engine before a planned wildcard — it fires, and the
archive holds blank runs of two and three consecutive rounds. So a footballer who cannot appear was
valued at full score in the search that decides who to buy. **Applied**: an unexported
`PlayerMetrics.loadSet`, which is the fact the guard was reaching for.
`TestATotalBlankIsWorthNothingToTheTransferSearch` pins it. The two in-package tests that build
`PlayerMetrics` literals now set `loadSet` and say why — without it they would have passed on a
disabled multiplier, which is the failure mode that made this worth a field rather than a comment.

**3. The ordering `fixtureLoadFor` depends on was safe and untested.** *(code review)*
`buildFixtureIndex` sorts before appending, which is what makes both the early `break` and the
`upcomingGWs` dedupe correct; nothing exercised it, because the synthetic calendar is already
ascending. An unsorted index fails twice over and silently — the window truncates early *and* the
denominator inflates. **Applied**: `TestTheFixtureIndexIsInGameweekOrder` shuffles the fixture list
before rebuilding.

**4. The probe's header table was stale and disagreed with the code comment.** *(code review,
findings audit — both, independently)* The header quoted the original five-season audit with
2022-23 at **42**, directly above a probe that counts a wholly-postponed round separately and
reports **22**. Both are right about different populations and neither said so. **Applied**:
replaced with the six-season table this probe produces, with the reconciliation named and the
comparison count stated as `agree + blank` rather than as the agree column.

**5. `docs/model.md` described the old denominator** and mentioned neither the anchor nor the skip
set. **Applied.**

**6. `AGENTS.md` overstated three things.** *(findings audit)*
- The `+33` is now attributed to the **anchor** rather than to the blank count: the old horizon-1
  window was `[first, first]` and could hold only fixtures, so that contrast priced doubles *by
  construction*, whatever the blank count had been. The count says how often the defect bit. The
  shipped term's value is now stated as **unmeasured**, which the code comment said and the record
  did not.
- "Every banked `HOLD` cell survives" was unconditional and is false for the fixture-load family's
  own arms — the `LOADTR` and `BANK` blocks set `SetFixtureLoadWeeklyOnly(false)`, where the term
  is live on `Score` at horizon 5. Now stated as conditional.
- "12 cells" collides with this file's own glossary, which defines it as three seasons by four
  entry points; mine is six by two, the opposite shape, and the difference is why the start-fixed
  estimator is thin. The grid is now named, and "77 points" carries its metric and the warning that
  cells span 38 to 13 gameweeks.

**7. Three unmarked collisions with existing bullets.** *(findings audit)* The fixtures section's
blanket "every attempt to lean harder loses" is about fixture **difficulty**, not fixture
**count**, and nothing said so; "rotating for blanks and doubles pays" was measured with the
imminent blank invisible, so it is established on doubles and unverified on blanks; and the chips
section's "what those want was already wired" is about the *preparation credit*, not the free hit's
own builder. All three marked in place.

**8. The three shipped bugs are recorded in *Things that have already bitten***, which is that
section's stated membership. Budget raised 112 → 116 KB with each claim that needed the room named
in the test comment, per that constant's own rule.

## Findings declined

**The statistics review proposed re-running the pre arm from a named commit.** Not declined —
**done**, and it mattered. My first "pre" run came from a dirty tree, so its sidecar named the
*post-fix* commit for both arms. Both arms were re-run from clean checkouts (`d8a3eb9` detached and
branch HEAD), and every figure reproduced to the digit.

**The code review suggested a lock for `Engine.upcomingGWs`.** Declined; the race is **recorded on
the field, unfixed**. `ApplyChipPlan` rebuilds the fixture index from a tool handler the runner
fans out concurrently, and the reviewer reproduced a data race under `-race` — but the race is
**pre-existing** on `byTeamUpcoming` and on `e.Weights.Horizon`, both of which `ApplyChipPlan` also
writes unguarded. Fixing it properly means guarding the whole fixture index and the weights, which
is a different subsystem from the one under review and wants its own measurement of the read-path
cost (`fixtureLoadFor` runs once per player per scoring pass).

⚠️ **This entry said "the false justification removed instead" before the documentation review, and
that was false when written** — the tree still carried the strongest form of the assertion ("built
once … so it needs no lock"), newly added by this branch, while a dated tracked artefact said it
had been corrected. That is worse than the race: it is the line that makes the next reader skip a
lock they need. Now corrected in `metrics.go`, which names the writer, the concurrent reader, and
that it is unfixed.

**A pointer or second field to separate "unset" from "zero" in the three *reporting* predicates**
— `playerRow.Load` (`omitempty`), `noteFixtureLoad` and `present.corrections` — declined. It is the
same conflation `loadSet` fixes on the scoring path, but crossing three packages for a case that
goes silent only at a configured horizon of 1, and where `WeekViews` (horizon 1 by construction)
already names its blanks through `Blanks` and `Opponents`. Recorded on `FixtureLoadIsNotable`.

**Pricing a horizon-1 double as `f(d1)+f(d2)` rather than `2·f(d1)`.** Declined and recorded on
`Metrics`. `TeamFixtures` counts fixtures while `fixtureLoadFor` counts gameweeks, so the second
leg arrives as a multiplier and both matches are charged at the first one's difficulty. Bounded by
the ladders' spread (attack 1.30–0.72, defence 0.70–1.40), so at most about a quarter of the
fixture-sensitive part of one match, and exactly zero when the legs share a difficulty. It is a
magnitude error inside a week the model already knows is a double, not the blank-shaped one.

## What could not be checked on this harness

**The blank half of the term has never been priced, and this branch does not price it.** The
`+35.9` figure below is the *anchor fix*, which contains at least two live channels — the horizon-1
`Score` multiply under `WeeklyXI`, and `xiValueForTransfer` on the horizon-5 transfer engine, where
both the anchor and the denominator changed. It is not "scoring blanks as blanks is worth X".

**The free-hit and skip-set halves are unexercised by the measurement.** `sweepConfig` sets no
chips, so `freeHitSquad` and `ElementsWithoutFixtures` never executed and the skip set was empty in
all 24 runs. Those two stand on their tests, not on the sweep.

**The points arm does not resolve on the estimator its own variance components call for.** Six
seasons × two entry points, both arms from clean commits, paired within cell: `policy_points`
**+35.9 a season**, season-clustered t 2.98 — but start-fixed **t 1.40**, naive 1.88, mixed model
1.88 (p 0.087), with the season component estimated *negative* (MoM −2.111, `F_season` p 0.938).
`sweep_inference.R`'s own guidance is to prefer start-fixed where `%seas` is near zero, and it is
zero here. Clustering is doing the work and doing it in the optimistic direction — CR2 SE 0.318
against a naive 0.502, because the two cells within each season are negatively correlated.
`policy_xpoints` *does* resolve and on both estimators (+50.9, t_seas 3.94, t_fixd 3.90, 6 of 6
season means positive, wild p 0.0105), and is quoted beside points rather than instead.
`concentration_screen.R` does not flag either (96% survives dropping the three largest cells).
Both inherit the replay's final fixture list, so both carry the same hindsight caveat as the `+33`.

**`HOLD` cannot see this fix at all.** `HoldCaptaincyWeekly` builds every engine at `cfg.Weights`
and never sets `Horizon = 1`, so `FixtureLoadInScore()` is false on that path. The byte-identity in
12 of 12 cells is a **confinement — a code fact** — not an empirical null, and the metric this
project uses for scoring changes structurally cannot judge this one.

**The review fixes themselves are byte-identical on the replay in 12 of 12 cells**, re-measured
after they landed. That too is a confinement rather than a liveness check: `WeekViews` is not on
the sweep path, and the `loadSet` guard differs only where a club blanks a whole horizon-5 window,
which no archived club does.

**The fix ships on correctness. No points gain is claimed.**
