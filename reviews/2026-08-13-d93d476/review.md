# The second chip set, and two seams that a confident comment sat on top of

Covers `6031ef2..d93d476` — the page fixes (hits, chips, the season transfer list),
the two-set chip capability, and the review pass over both.

## Reviewers dispatched, and why

The change touches `internal/backtest`, so the triage table gives **fpl-stats-review**
and **fpl-findings-audit**. **fpl-code-review** was added because the specific failure
class here is the silent no-op — a chip plan that parses, validates and is never read
returns a season byte-identical to one where the setting was absent.

**fpl-security-review, fpl-run-review and fpl-season-maintenance were not owed.** No
credential path, no config written by a live run, no hand-maintained season list.

The invariant question was asked first, as the skill demands. **What must this change
NOT move: any figure recorded at one chip set.** That is now pinned by
`TestAnEmptySecondSetChangesNothing` and, more usefully, by
`TestASecondSetChipActuallyPlays`, which runs `Simulate` twice.

## Findings applied

Two reviewers independently found the same two defects. Both sat under comments I had
written asserting the opposite.

| finding | what it would have done |
|---|---|
| **`ValidateChipSets` was called only from `cmd/fplagent`** | Every sweep and diagnostic builds a `SimConfig` literal in `internal/backtest`. A diagnostic could set `Chips2` on 2022-23 and score eight chips in a season that granted four — no error, ordinary-looking total |
| **The comment at the scoring switch claimed that gate made it safe** | False on every path but one. Two chips in one gameweek would drop the second silently |
| **`anticipate` read only the first set** | `AnticipateChips` measures the first half and returns a plausible season; a second-set-only plan makes it a total no-op while still stamping the variant |
| **`TestBothChipSetsAreReadByEveryConsumer` tested no consumer** | It exercised the helpers. It would not have caught the line above — and it certified coverage it did not have, which is worse than no test |
| **Free-hit weeks rendered the squad that sat out under a "Rebuilt fifteen" banner** | `Week.Squad` is the permanent fifteen and `Week.XI` the borrowed one, so the page showed players whose points did not count, filed almost entirely as substitutes, under a banner announcing a rebuild |
| **`FPL_CHIP_PLAN=config` was a hard error on 2025-26** | `config.json` holds one flat plan; any real plan has a chip after GW19, which the validator then refuses as a first-set chip |

The gate moved into `Simulate`, keyed on `cur.Name`, beside `DefconScoredIn` — which is
enforced in that package for exactly this reason. That makes the switch comment true
rather than deleting it.

**Three existing chip tests changed, and that is the guard working.** They placed a
first-set chip at GW20 on 2025-26, which FPL forbids. They now use the second set.

## Record corrections applied

- **15 of 189 doubling club-gameweeks is the six-season total**, and 11 of the 15 are
  one COVID-rescheduled 2020-21 round. I had stated it as a per-season fact; the
  collinearity is five seasons of six.
- **The record asked for two sets as a fidelity fix for the LIVE agent.**
  `analysis.ChipPlan` and `config.Chips` are unchanged, so that item is untouched —
  what landed is the replay half the same paragraph called "not a measurement design".
  Corrected in `docs/notes/chips.md` and `TODO.md` rather than left implying closure.
- **`bench_boost_pts` keeps the last week with `BenchBoost` set**, so a two-set arm
  would report the second boost and drop the first. Every recorded bench-boost figure —
  9.4 against 16.7, the paired +7.28 — is a **one-set** figure.
- **`AxisChipWeek`'s oracle takes a single argmax per season**, so on 2025-26 it is now
  a one-set bound on a two-set season. Nothing is wrong today; a ceiling quoted from it
  there is not the ceiling the rule allows.
- **`FPL_CHIP_PLAN` is read only by `cmd`**, so a sweep run with it set gets a changed
  fingerprint over byte-identical cells — the byte-identical null inverted.

## The measurement question, recorded as unmeasurable

Not unresolved. 2025-26 is the only legal season, giving **6 cells** against this
record's floor of 24; the season-clustered SE is degenerate on one season, the
`DefConCleanCoupling` case where t is pinned at 1.00 by construction; and entries from
GW21 cannot play the first set at all, so treatment events vary per cell.

⚠️ **I accepted a reviewer finding that was wrong, and the user caught it.** The review
said picking chip weeks from a season's doubles is hindsight, I wrote that into three
files, and it contradicts this package's own `sightedWeeks` doc: *the constraint a short
lag imposes is not that you cannot find a double, it is that you cannot tell a double
from the biggest double of the season.* Reschedulings are rumoured for months and
confirmed weeks ahead — which is exactly why `sightLags` is `{2, 4, 6}` and not `{0}`.

**And the narrower version I retreated to was also wrong.** I said what full sight buys
is knowing which double is *biggest*. The manager reports knowing that in advance in
**3 years of 3** — the cup and European calendar makes it predictable long before the
fixture is formally moved. `sightedWeeks` asserts the opposite as fact and never
measured it, so the premise the whole lag ladder rests on is an unsupported assumption
contradicted by the only direct evidence available.

**Nothing measured moves; which row is the answer does.** `fullSight` is not the arm
that cheats — it is the arm a real manager plays, so its **−5.4 a season-path, clustered
t −0.66** is the estimate of what calendar anchoring is worth. The lag arms bound
**pessimism**, pricing a manager with less information than anyone has. Recorded at
`sightedWeeks`, in chips.md and in CLAUDE.md.

**The lesson is the one the review-gate skill states and I did not apply**: a reviewer's
report is a set of proposals, several will be wrong, and this one asserted something the
code it was reviewing already contradicted. I verified its two code findings and did not
verify this one.

## Declined

**The three totals are reported to the user with the contrast named, not as a ladder.**
The reviewer's position — that 2126 / 2232 / 2249 invites a false subtraction, that a
single `POLICY` cell has an SD near 73 points, and that most of both increments is two
wildcard rebuilds the harness provably cannot value — is correct and is stated to the
user alongside the numbers. I did not suppress them, because the user asked what the
season scored and the honest answer is a number with its limits attached rather than a
refusal. **The middle arm is the one I stopped quoting**: under the old rules a manager
also had a second wildcard, so "one set" is a game nobody played.

**`ChipPlanner` was not taught the reset.** It returns one plan and no arm can build a
two-set one, so no sweep can reach the capability. Since `Simulate` now validates, a
planner that placed a chip after the reset on 2025-26 errors rather than playing it.
Widening it is a measurement design, and the record says not to build one.

**`ChipOracle` was not widened** to two boosts and two triple captains. It reads
`res.Weeks` rather than the plan, so nothing is currently wrong; the restriction is
recorded where the oracle is documented instead.

## What could not be checked

**Whether the second set is worth anything.** Nothing was swept, and the section above
says why nothing can be.

**The snapshot's MDE half is absent** (`harness.mde.present` true → false). It is built
from a transient cells file, and regenerating it from a *different* sweep's committed
cells would move 24/139 to unrelated numbers and read as a real change. Recorded as
absent rather than manufactured.
