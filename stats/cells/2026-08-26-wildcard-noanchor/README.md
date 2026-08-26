# The first-half wildcard, with no double to anchor to

`TestDiagWildcardDriftTrigger`, six arms, six-season extended grid, 36 cells per
arm, POLICY, clean tree (`dirty=false`). Read with `--scale=per_path`.

The question: **when do you play a wildcard that has no double to anchor to?** A
wildcard aimed at a double is a calendar decision and is measured elsewhere. One
with no target is a decision about the SQUAD, and the first half is where that is
true — two doubling gameweeks in GW1-19 across six seasons against forty after.

Control plays no wildcard trigger at all. All arms are confined to GW1-19 and
share `analysis.ChipBarAt`'s decay, so they differ in **what they read**.

## ⚠️ Nothing resolves, and the ladder is not monotone

| arm | mean | CR2 SE | t | threshold | seasons+ | fired | median GW |
|---|---:|---:|---:|---:|---|---|---|
| repair cost (shipped) | −4.94 | 12.09 | −0.41 | 31.1 | 4/6 | 24/36 | 9.5 |
| drift > 1.0 | +7.81 | 10.40 | 0.75 | 26.7 | 3/6 | 23/36 | 11 |
| drift > 2.0 | +0.11 | 5.63 | 0.02 | 14.5 | 3/6 | 18/36 | 11 |
| drift > 3.0 | +5.56 | 5.87 | 0.95 | 15.1 | 4/6 | 14/36 | 14 |
| drift > 5.0 | +1.56 | 1.37 | 1.14 | **3.5** | 2/6 | 6/36 | 18.5 |

+7.8, +0.1, +5.6, +1.6 is scatter, not a plateau with a cliff. **On this record's
own standard there is no knob here to find**, and no arm clears its threshold.

⚠️ The bottom arm is the sharp one: firing in 6 of 36 cells collapses its SE to
1.37 and its threshold to **3.5**, so it *could* resolve an effect the size a
wildcard should have. It reads **+1.56**.

## ⚠️ The mechanism finding, which is the useful part

Restricted to the cells where each rule actually fires:

| rule | hits | POLICY | fired |
|---|---:|---:|---|
| **repair cost (shipped)** | **+0.58** | **−7.4** | 24 |
| drift > 1.0 | −0.26 | +12.2 | 23 |
| drift > 2.0 | −0.44 | +0.2 | 18 |
| drift > 3.0 | **−1.00** | **+14.3** | 14 |
| drift > 5.0 | −0.83 | +9.3 | 6 |

**The shipped rule INCREASES hits.** A wildcard repairs the squad for free, so a
rule that fires it and leaves the policy taking *more* hits afterwards has fired
on the wrong squad. Every drift rule moves the other way.

The mechanism is `changesBetween`: a raw count over all fifteen, in which a £4.0m
bench swap scores like a lost captain. The shipped rule fires on a squad whose
"three changes" are cheap swaps nobody would pay for, burns the chip, and the
fresh squad still needs repairing.

⚠️ **This is a MECHANISM reading, not a significance one.** "Among cells that
fired" is a SELECTED subset and a DIFFERENT one per arm — drift>3.0's 14 cells
are not the shipped rule's 24 — so the point columns are not comparable across
rows. **What is robust is the SIGN on hits**: shipped positive, all four drift
arms negative.

⚠️ It also explains why the full-grid means look like noise: the effect lives in
the 17-65% of cells where a rule fires and is diluted by the rest.

## What this does NOT show

**It does not show drift beats repair cost.** No arm resolves, and the ladder has
no shape.

**The high-bar arm fires late** — median GW18.5, at the deadline — so part of what
it measures is "spend it before it expires", not "spend it when the squad is bad".

**`xiDriftOf` is fixture-blind.** `Score` at the shipped horizon is a five-week
average and `FixtureLoadInScore` is true only at horizon 1, so the drift these
arms read cannot see a double or a blank. `xiDriftSeries` fixes that and **no arm
here uses it yet**.

**Nothing prices the value of waiting for information** — injuries revealed,
rotations observed. `ChipBarAt`'s decay is a generic stand-in, and the replay is
as blind to that value as the rules are.
