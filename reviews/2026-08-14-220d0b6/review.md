# Two chip sets in one type, and two regressions of the same class I was fixing

Covers `99c20db..220d0b6` — the stale stage-3 item, the free-hit error propagation,
`analysis.ChipSchedule`, and the review pass over all of it.

## What was actually built, and what turned out to already exist

The task was "do the stage 3 leftovers — free hit and wildcard". **Both had shipped
three days earlier at `27740ba`**, with `playWildcard`, `freeHitSquad` and five
regression tests, while the item still read "neither is modelled". It is checked off
with pointers rather than rebuilt. Reading the code before scheduling the item is what
caught it, and that is the only reliable defence against this class — see the count
correction below for why a *count* is not.

Verifying it found a real defect: the free hit's call site **discarded**
`freeHitSquad`'s error where the wildcard four lines above propagated it. The larger
piece is `analysis.ChipSchedule`, holding both chip sets with eight named slots, which
closes the queued item that `ChipPlan` could not express two sets.

## Reviewers dispatched, and why

The change touches `internal/analysis` (scoring), `internal/backtest` (harness),
`internal/agent` and config persistence, and the record. The triage table's union is
**all four of fpl-code-review, fpl-stats-review, fpl-findings-audit and
fpl-security-review**, and all four ran concurrently on the committed state.

**fpl-run-review and fpl-season-maintenance were not owed.** No live run wrote config,
and no hand-maintained season list is touched.

The invariant question was asked first, as the skill demands. The answer I gave then
was wrong, and the statistics review is why — see "The confinement check I claimed I
had" below.

## Two findings arrived with stale premises, and were not applied twice

The reviewers ran concurrently with fixes landing in the working tree. **fpl-security-review
finding 1 and fpl-findings-audit finding 9** both said `ChipSchedule` has no
`MarshalJSON`; **fpl-code-review findings 4 and 5** said the mixed JSON form is accepted
and that `UnmarshalJSON` skips the range check. All four were true of `6b83b08` and
already fixed by the time the reports landed. Recorded here so the next pass does not
re-raise them, and as a note on method: dispatching against a commit while working on
top of it produces exactly this, and the cost is small compared with serialising the
reviews.

## Findings applied

### Two regressions I introduced, both the class I was fixing

Both are a set-correctness check applied to a plan that carries no set information.

| finding | what it did |
|---|---|
| **`ValidateChipPlan` checked a set-1 slot only against the set-1 window** | A flat legacy plan loads wholly as set 1, so `bench_boost_gameweek: 34` — legal, and the form `docs/configuration.md` tells a user to write — reported as outside its GW1-19 window. A **false** error, and a regression against the old behaviour of reading the current set. `containingSet` now distinguishes "impossible week" from "legal week, wrong set" and names the slot to move it to |
| **The expiry warning suppressed only windows already *past*** | Every second-set window stops at GW38, so from GW1 all four second-set chips reported as expiring. **Four permanent lines on the shipped config where the old code gave none**, printed by `fplagent brief` and replayed to the agent in `get_chip_plan`'s issues on every call — under a comment claiming to prevent exactly that. It now reports only the window the season is currently *in*, which reproduces the old behaviour and extends correctly past GW20 |

Pinned by `TestASecondHalfChipInAFlatPlanIsNotCalledOutOfWindow` and
`TestAFullyPlannedCurrentSetIsClean`. Both fail against the pre-fix code.

### The silent no-op inside the fix for silent no-ops

`Plays`, `Next` and `Weeks` return `bool`/`int`/`[]int`, so they **cannot** report a
slot name that does not resolve — they fall back to the nothing-planned answer. Those
three are exactly what the four behaviour fixes call, with bare string literals. The
type's own doc comment claimed "an error everywhere it is accepted" and was wrong about
its own package.

Mitigated by removing the literals: `analysis.SlotWildcard` and three siblings are
exported constants, every internal caller uses them, and `backtest`'s private names
alias them rather than re-spelling. A typo now fails to compile. **A caller passing a
computed string is still unprotected**, and the record says so rather than claiming the
hole is closed.

Separately, those readers **parsed a set suffix and discarded it**, so `Next("wc2", …)`
read as "the second wildcard" and meant "either wildcard" — while `Set` and `Get`
honoured it. Now honoured throughout; `TestASuffixedSlotAsksAboutThatSetAlone` pins it.

### Config persistence

| finding | what it would have done |
|---|---|
| **No `MarshalJSON`** | The first roster override the agent persists rewrites every existing `config.json` into the two-set form — an unexplained diff on a tracked file, and a one-way door, since anything still typed `ChipPlan` reads that object as **all zeros with no error**. The flat form is now written whenever there is no second set, so nothing churns and the break reaches only plans that genuinely use a second set |
| **`config.Save` truncated in place** | Pre-existing, but this change would have made every install exercise the window once. A crash between truncate and write leaves a zero-length `config.json`, taking the roster overrides with it. Writes a sibling and renames |
| **`UnmarshalJSON` skipped the range check, zeroed on `null`, and dropped flat siblings in a mixed object** | Validated, no-ops, and refuses respectively |

`TestSavingDoesNotRewriteTheCheckedInChipPlan` round-trips the repository's **real**
`config.json` rather than a fixture, so it notices the day the real one changes shape.

### The fourth copy of the chip table, which had already drifted

`describeChipPlan` kept its own four-name list ordering wildcard, bench boost, triple
captain, free hit — against every other renderer's play order. The replay page therefore
listed a plan differently from `fplagent chips`. It renders through
`ChipSchedule.Entries` now, and `TestDescribeChipPlanNamesEveryChipItPlays` moved with
it. This is the drift the shared type exists to prevent, surviving inside the commit
that introduced the type.

Also: `briefChips` reported a chip planned **outside** the current window as
"unplanned", losing the user's own plan from the column whose job is to show it. It now
says "GW34 (outside this window)".

### The agent payload

`get_chip_plan` carries three vocabularies — slot keys in `plan`, FPL's names in
`windows`, display labels in `issues` — and left the model to infer that `bb1` and
`bboost` are the same chip. The `note` gained a legend. One sentence, paid once per
call, against a wrong inference about which chip is planned.

## Record corrections applied

The statistics review found the record wrong in three ways, all now corrected in place.

- **"A GW22 squad no longer truncates at a wildcard already played" states the inverse
  of the defect.** The old code's `if gw <= nextGW { continue }` forbade exactly that.
  The real failure is **under**-truncation: a single field holding a played first-set
  wildcard makes a live second-set one invisible. Verified against `99c20db`. The code
  comment had it right and the record did not.
- **Three of the four "silent wrong answers" had no reachable pre-commit plan**, because
  a two-set plan was inexpressible. They are correctness properties of a newly-expressible
  capability. Only the `ValidateChipPlan` one had live exposure — and the symptom quoted
  for it was the wrong message: "not available in the current half of the season" fires
  only when a chip has no unexpired window, which never happens against a live bootstrap.
- **The `data/captures/2025-26/GW01/…` path does not exist**; it is
  `GW01-2025-08-15T1102Z/…`. Pre-existing text, but "check which *file* a number came
  from" makes the exact path load-bearing.

The findings audit corrected the stale-item **count**: two drafts said "seventh" and
"ninth", the body's own arithmetic gives at least ten, and the title said six. The item
now carries a **rule instead of a count**, because the numbers have been written four
different ways and the count is not what anyone needs.

`docs/notes/chips.md` gained three markers it was owed: the two-set recommendation is
closed, the `TestDiagChipAnticipation` figures are marked superseded in place beside the
table (they were invalidated by an earlier `EffectiveHorizon` correction and the note
carried no marker while the code named the file), and free-hit figures produced in the
three days the swallowed error was live carry a caveat.

## The confinement check I claimed I had

**"The full suite passes unchanged, which is the confinement check" is false, and this
is the most useful finding of the pass.**

Every chip test compares two arms **inside one binary**, which cannot see a change
between commits. The accuracy snapshot moves only `stamp.commit` — but it was taken with
**no cells file**, and none of the twelve model diagnostics constructs a chip plan, so
`Engine.Chips` is the zero value in all of them and the 557 figures *could not* have
moved. That is this record's own rule one layer up: a byte-identical result under an
intervention that could not run is not a tie.

**What replaced it is a proof, and it is free.** At shipped config `Chips` and `Chips2`
are empty and `AnticipateChips` is false, so `cfg.plays(slotFreeHit, gw)` is false in
all 38 weeks and the free-hit branch is dead; `e.Chips` is assigned only behind that
switch; and the lookups reduce to the old field comparisons at an empty schedule. The
argument is now in TODO.md, with an instruction not to cite the suite.

## Declined, and why

- **Committing a migrated `config.json`** (fpl-security-review's proposed fix for the
  rewrite). A `MarshalJSON` that keeps the flat form is strictly better: nothing churns,
  and the downgrade break reaches only plans an older reader could not represent anyway.
- **Adding anything to CLAUDE.md.** It is 65,749 bytes against a 65,536 budget and
  `TestEveryNoteIsIndexed` is failing on it *now*, byte-identical to `origin/main`.
  Nothing here earns a place ahead of what is already over. The statistics review's
  candidate standing rule — "a passing suite is a within-version consistency check, not
  a cross-version confinement check" — is worth having and belongs there only once the
  file is back under budget.
- **A test seam to force `freeHitSquad` to fail inside `Simulate`.** No `SimConfig` field
  starves that call, and adding one purely to test it is a diagnostic carrying its own
  copy of the thing it checks. Left as an open follow-up instead, stated plainly.

## What could not be checked, and what is owed

- **The free-hit fix has no regression test on the join.** `freeHitSquad` is
  byte-identical across the fix and the new direct test calls it without going through
  `Simulate`, so **both new tests are green against the swallowed error**. Each half is
  pinned; the wiring between them is not, and reverting the six-line call-site change
  breaks nothing. This is a fix without a regression test, which "things that have
  already bitten" does not allow, and it is recorded as owed rather than glossed.
- **The three-day exposure window is unverified.** Any free-hit figure produced
  2026-08-10 to 2026-08-13 could have read a non-firing chip as a zero. Believed nil —
  that build cannot realistically fail — but settling it means re-running the
  free-hit-placing arms, where a failure is now a hard error.
- **The live-agent half has no tests at all.** Four of the five one-set behaviour changes
  were found by review, because nothing exercises `fplagent chips`, `briefChips` or the
  `chip_plan` tool. Golden tests on those three outputs are what would have caught them.
- **`ValidateChipPlan` and `ChipSetsFor` are now two answers to "how many sets does this
  season grant"** — one from how many windows the bootstrap lists, one from the season
  string. `wc2` on a 2019-20 plan validates cleanly through the first and is refused by
  the second. That is the signature failure shape on a knob nobody has run; raised by the
  findings audit, out of its remit, and left open deliberately rather than fixed
  unmeasured.
- **Nothing here was measured.** No sweep ran, no constant moved, and no figure in the
  record is restored by this work.
