# The first-half wildcard, every difference attributable

`TestDiagWildcardAttribution`, six arms in ONE process at ONE commit
(`d853a905`), `dirty=false`, six-season extended grid, 36 cells per arm, POLICY,
`--scale=per_path`. Confined to GW1-19, where there is no double to anchor to and
the rule is a condition on the squad.

Two earlier sweeps landed at different commits, so their most interesting
comparison could not be made — `sweep_inference.R`'s code-state guard refuses to
difference them. **Every arm here is one process apart, so the steps are
arithmetic.**

⚠️ **Each arm sets `WildcardReservation` EXPLICITLY.** `sweepConfig` does not map
`config.OptionValue` into `SimConfig` and `config.json` has no `option_value`
block, so an arm that omits it runs at a bar of **zero** and fires on anything
above zero. That is what invalidated the earlier "shipped rule" arm.

## Result — nothing resolves, and the decomposition is clean

| arm | mean | CR2 SE | t | threshold | fired | med GW |
|---|---:|---:|---:|---:|---|---|
| cost, RAW, bar 12 — **the shipped rule** | −8.08 | 10.90 | −0.74 | 28.0 | 23/36 | 9 |
| cost, XI-only, bar 12 | −5.83 | 10.94 | −0.53 | 28.1 | 14/36 | 8.5 |
| cost, XI-only, bar 10 | −0.33 | 10.43 | −0.03 | 26.8 | 17/36 | 11 |
| **drift > 3.0** | **+5.56** | 5.87 | 0.95 | 15.1 | 14/36 | 14 |
| drift > 5.0 | +1.56 | 1.37 | 1.14 | **3.5** | 6/36 | 18.5 |

**Each step isolates one change:**

| step | from → to | worth |
|---|---|---:|
| the INPUT — raw count → starters only | −8.08 → −5.83 | **+2.25** |
| the BAR — 12 → 10 | −5.83 → −0.33 | **+5.50** |
| the RULE — repair cost → XI drift | −0.33 → +5.56 | **+5.89** |
| | **total** | **+13.64** |

⚠️ **Nothing resolves.** Thresholds of 15-28 against effects under 9. These are
signs and a decomposition, **not** measured effects, and the total is a sum of
three unresolved steps.

⚠️ **Argmax.** Bar 10 and drift 3.0 were each chosen as the best rung of an
earlier ladder, so both steps are biased upward.

## Mechanism: hits taken

| arm | hits, all cells | hits, fired only | POLICY, fired only |
|---|---:|---:|---:|
| cost, RAW, bar 12 (shipped) | −0.03 | −0.04 | −12.7 |
| cost, XI-only, bar 12 | −0.11 | −0.29 | −15.0 |
| cost, XI-only, bar 10 | −0.22 | −0.47 | −0.7 |
| **drift > 3.0** | −0.39 | **−1.00** | +14.3 |
| drift > 5.0 | −0.14 | −0.83 | +9.3 |

**Every arm reduces hits, and the better rules reduce them more** — drift > 3.0
saves a full hit in each cell it fires.

⚠️ **The claim that the shipped rule INCREASES hits does NOT reproduce.** It read
+0.58 in `2026-08-26-wildcard-noanchor`, where that arm had a **zero bar**. At the
shipped bar of 12 it reads −0.03. That earlier bank is corrected in place.

## What this does NOT show

**That the wildcard trigger is worth having.** The shipped configuration is
−8.08 against no trigger at all, and the best arm is +5.56 against a threshold of
15.1. Nothing here clears its own bar.

**Any arm sees the fixture run.** Every drift arm reads `xiDriftOf`, priced on the
shipped horizon where `FixtureLoadInScore` is false, so it cannot tell a double
from a blank. `xiDriftSeries` fixes that and **is not wired to a trigger** — a
fixture-aware arm is the next one, kept out of this sweep so the change does not
ride in with the attribution.

**Anything prices the value of waiting** — injuries revealed, rotations observed.
The replay is as blind to it as the rules are.
