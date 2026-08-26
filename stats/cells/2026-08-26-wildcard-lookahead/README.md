# Does pricing the wildcard across a RUN of gameweeks beat reading one?

**No — the two readings rank weeks the same way, and neither beats not having the
rule.**

`TestDiagWildcardLookaheadTrigger`, eight arms in ONE process at ONE commit
(`2d852bdb`), `dirty=false`, six-season extended grid, 36 cells per arm, POLICY,
`--scale=per_path`. Confined to GW1-19, where there is no double to anchor to and
the rule is a condition on the squad. Every arm sets its bar explicitly.

Paired against **a control that plays no wildcard trigger at all**, so each arm
reads against not having the rule rather than against the other rules.

## Result — nothing resolves, and the best arm is a third of its own threshold

| arm | mean | CR2 SE | t | threshold | fired | med GW | hits/cell |
|---|---:|---:|---:|---:|---|---|---:|
| *control: no trigger* | — | — | — | — | 0/36 | — | 1.94 |
| cost, raw count, bar 12 — **the shipped rule** | **−3.53** | 3.66 | −0.97 | 9.4 | 22/36 | 9 | **2.06** |
| single-week drift > 3.0 (horizon-5) | +3.50 | 4.36 | 0.80 | 11.2 | 15/36 | 15 | 1.69 |
| lookahead value > 40 (horizon-1) | +1.67 | 4.31 | 0.39 | 11.1 | 14/36 | 14 | 1.69 |
| lookahead value > 55 | +6.11 | 7.32 | 0.84 | 18.8 | 7/36 | 18 | 1.53 |
| lookahead value > 70 | +1.42 | 1.42 | 1.00 | 3.6 | **4/36** | 18 | 1.86 |
| lookahead value > 85 | +1.42 | 1.42 | 1.00 | 3.6 | **4/36** | 18 | 1.86 |
| lookahead value > 100 | +1.33 | 1.33 | 1.00 | 3.4 | **4/36** | 18.5 | 1.86 |

`t_crit` 2.571 at df 5. **No arm reaches half its threshold.**

## What it says

**1. The two readings are the same ranking in different units.** Compare the only
two arms that fire at a comparable rate — drift > 3.0 at 15/36 and lookahead > 40
at 14/36. They pick **adjacent median gameweeks — 15 and 14** — take the **same
hits per cell (1.69)**, and their effects differ by less than half a standard
error.

⚠️ **This said "the same median gameweek (15)" until the counts got a committed
generator.** They are one week apart, not identical. The upper-median slip that
produced the wrong number is described under Reproducing; the argument is
unchanged by one gameweek, but "the same" was a stronger claim than the data
made and it was the sentence the whole finding turned on. The
two readings correlate **0.884** on the bracketing cells
(`TestDiagWildcardLookaheadValue`), and this is what that correlation looks like
in a sweep.

⚠️ **This does NOT say fixture awareness is worthless.** It says that on this
decision, at this bar, the horizon-1 per-gameweek reading and the horizon-5
average put the same weeks in the same order. A reading that changed the ranking
could still exist; this one does not.

**2. The shipped rule is negative and takes MORE hits than not having it** —
−3.53 a season-path, 2.06 hits per cell against the control's 1.94. A wildcard
repairs the squad for free, so a rule that fires well should leave the policy
taking FEWER hits. This one fires earliest of all (median GW 9) and does the
opposite. It reproduces `2026-08-26-wildcard-attribution/`'s −8.08 at a different
bar and configuration: same sign, same story, neither resolving.

**3. ⚠️ The three highest bars are INERT, not good.** Bars 70, 85 and 100 fire in
**4 of 36 cells** and read +1.42, +1.42, +1.33 with standard errors of 1.4 — the
small, tidy numbers of an arm that is mostly just the control. Bars 70 and 85 are
identical because no cell carries a reading between them. **An arm that never
fires is inert, not neutral**, and reads exactly like a rule that was tried and
found harmless. `p_wild` says `pinned` for all three, which is the same fact
arriving from the bootstrap: one season moves.

## What it does NOT say

⚠️ **This is not the peak rule the work set out to build, and that rule cannot be
built on `wildcardValueOverNext`** — its value is non-increasing in k by
construction, so `PeakAt` is identically zero and a peak gate would refuse
nothing. Confirmed at 0 of 20,000 random non-negative series and 0 of 36 real
cells. See `TestWildcardValueOverNextPricesTheLookahead`.

⚠️ **The ladder was bracketed from measured readings** — min 24.89, median 63.54,
max 110.14 over 36 cells at entry+5 — not by analogy with the two bars beside it,
which are in different units. Even so, three of five rungs landed in the inert
tail, so the bracketing was necessary and still not sufficient: the live rule
reads every eligible week with a varying allowance, and that distribution is
wider than the one-point-per-cell bracket suggested.

## Reproducing

```bash
DIAG=1 FPL_CELLS=stats/cells/2026-08-26-wildcard-lookahead/lookahead.csv \
  go test ./internal/backtest -run '^TestDiagWildcardLookaheadTrigger$' \
  -count=1 -v -timeout 90m
```

⚠️ `-count=1` is not optional: a cells file is a side effect the Go test cache
cannot see, and a cached run prints a full table and writes nothing.
⚠️ `FPL_CELLS` APPENDS — delete the file, never overwrite it.

## ⚠️ The medians in this table were WRONG until 2026-08-26

The effects and standard errors have always come from `stats/sweep_inference.R`,
which is committed. **The fire counts, median gameweeks and hits per cell did
not** — they were derived once with an ad-hoc script that was never committed, so
nothing could reproduce them and nothing could check them.

Both failures duly happened. The script took the **upper** element for an
even-length list instead of the median, so three of seven medians were one rung
too high: the shipped rule read 10 rather than **9**, lookahead > 40 read 15
rather than **14**, and lookahead > 100 read 19 rather than **18.5**. The second
of those was load-bearing — it was the "same median gameweek" the headline claim
rested on.

`fires.R` is now committed beside this README and regenerates all three columns
through `stats/cells_common.R`. **A number the README leans on with no generator
is not a measurement**, which the directory next door had just been corrected for
in the same session.

Read the fire count before reading any null in this table.
