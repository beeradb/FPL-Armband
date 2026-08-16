# The scoring-path conversion scale, sized

## What was reviewed

`integrate-the-engine-scale-exposure`, merging `size-the-scoring-path-conversion-scale-exposure`
(`845cf8c`, 8 commits) onto `main`, plus the `CLAUDE.md` verdict written on top. Two new test files;
**no shipped code changed**. The source branch carries its own record with the full account.

## Why this ran

`CLAUDE.md` said this scale's exposure "has NEVER been sized". It is the second of two conversion
scales built through the same `CalibrationRatio` — and unlike its sibling it is read by `baseXP90`,
which **is** `Score`. That claim is now false, which is why the entry was rewritten rather than
appended to.

## The result

**Bounded. No repair, no arm.** The shift has an exact closed form, `−100·A_b/A` with xA cancelling,
asserted on 605 cells: **median 0.00% over all 605 fitted outfield cutoffs**, non-zero in 206,
largest **−12.50%**. Reach into `Score` is a largest per-player move of **0.185 xP/90** on an assist
term worth 11-14% of a midfielder's `BaseXP90`.

**The deepest point is not the size.** The counterfactual **conditions on the outcome** — it selects
on `Assists > 0` — so this is a **decomposition, not a bias**, and it could not have been a repair
whatever the number came out at. The term exists to price exactly the assists xA does not count.

Three things recorded because a reader would otherwise get them backwards:

- **"Bounded by construction" is two claims and only one is deductive.** The `{1.0} ∪ [0.5, 3.0]`
  range is code; that the floor binds where exposure is large is empirical, with a counterexample
  in the branch's own data. The clamp binds in **0 of 936** cells.
- **`FPL_NO_XG_REPAIR=1` inverts the reading** — ~40× the exposure with a *narrower* shift, which is
  the strongest evidence the floor is the binding mechanism.
- **The population must be quoted.** `Simulate` rebuilds this engine every gameweek, so entry
  deadlines govern the opening fifteen alone.

## Reviewers, and what they caught

Three ran on the source branch. **All three found real defects after the agent had called it done**,
and the repo's own tripwires then caught two more that all three had read past.

1. **Two headline numbers were artifacts** — the thin-sample floor firing on the counterfactual, not
   football. −62.96% → −9.09% and −57.32% → −23.68%. ⚠️ **I had relayed both to the user as
   measurements.**
2. **The keeper verdict rested on a guard with no power.** `CalibrationRatio` is constant at 1.0
   below the floor, so on a floored cell the guard compared `1 == 1` — and floored cells are *every*
   cell the keeper claim is about. Demonstrated by zeroing every keeper's totals and watching the
   suite stay green.
3. **The headline was computed over the wrong population** — entry deadlines rather than every
   cutoff the replay stands at. This was the most consequential of the three.
4. **A measurement declared unmeasurable was not.** The translation into `Score` was called
   impossible because `assistPoints` is unexported; an engine over a copy of the bootstrap isolates
   it exactly, and the constant was **recovered from the model's own output as 3.000000** rather
   than written down.
5. Then `TestTheMiddleValueHasOneImplementation` caught an inline median and
   `TestPrintedGridLabelsAreDerived` a hardcoded "Six seasons".

⚠️ **That last pair is the argument for invariants over reviewers, arriving on schedule.** Three
review passes read past both; two free tests caught them on the next run.

## Applied here

- The `CLAUDE.md` entry rewritten from "never sized" to the verdict, carrying the closed form, the
  population caveat, the two-claims split on "bounded by construction", and the
  decomposition-not-a-bias point.
- Nothing else. **No shipped code changed on this branch**, and I added none.

## Declined

- **Folding `conversionfit_diag_test.go`'s duplicate floored-cell check into the new helpers.** The
  source branch flagged it as redundant and left it, because folding it in means changing another
  measurement's assertions from a branch meant only to count. Agreed — and recorded so it is not
  rediscovered as novel.
- **Re-deriving my own grid-label fix.** I had patched `TestPrintedGridLabelsAreDerived` locally
  before noticing the agent's final tip already fixed it. Discarded mine and took theirs, rather
  than carrying a duplicate.

## Owed and not done

- **The intercept diagnostic** — seconds, no replay, and it tests the one mechanism this points at.
- **A 25-40% canary** that would have to resolve before any points arm could be justified.

Both filed on the research side rather than left in a report.
