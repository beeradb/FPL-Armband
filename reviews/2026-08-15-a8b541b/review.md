# The xPoints pilot: built, reviewed, run, and closed on its own evidence

Covers `556cabb..26d7505` — the accumulated-xPoints accumulator, its review fixes, the pilot run,
and the closure.

**Dispatched, not first-party**: `fpl-feature` (Opus) built it, `fpl-code-review` reviewed the
build, `fpl-stats-review` (Fable) made the close/continue recommendation. Three agents, three
distinct jobs, none of them reviewing their own work.

## The build, and what review found in it

One machinery, two accumulators: `weekPointsWithChip` became a projection of `weekScoreWithChip`,
which carries both totals through the same XI loop, armband fallback, chips and autosubs. Sixteen
call sites untouched; the snapshot moved by the commit stamp alone, twice.

The builder ran **eleven mutations against its own tests and strengthened three that survived** —
including a mirror check that was vacuous at a cell where the model captains one player all season,
and a hit-cost check unfalsifiable at shipped settings because the probe cell took no hits. It also
found a hole nobody briefed: with the new columns outside `cellMetricColumns`, a transfer-gate
oracle could have moved `hold_xpoints` while its points twin was pinned as an invariance.

Code review then found three more, all applied at `2532041`: a **half-pair cells file passed both R
contract checks silently** and printed a false "ok" for arithmetic that compared zero rows; the
control toggled a package global without `defer`; and its pre-registered band was printed rather
than asserted. The half-pair hole predated this branch on the captaincy rungs — the generalised
check closes it for all four pairs.

## The run

`TestDiagXPointsPilot`, 36 cells, MINHL ladder plus vice-captain control, both metrics from one
run. Cells, means and provenance banked at `stats/snapshots/2026-08-15-xppilot/`; the end-to-end
check reproduces **all 24 of Go's means to 8.9e-16**, new columns included.

**The gate passed**: per-arm SE ratios 0.71-0.82 naive, ~25% of sd removed. ✅ **Amendment 7 (the
user's) was load-bearing** — the vice control read **1.000** in the same run, so the gate measured
on the control alone would have closed the programme on a wrong number.

## Why it closes anyway, and why the first reason given was wrong

⚠️ **The SEs and the means fell together on `HOLD`.** The ladder's one real feature, the
half-life-2 cliff, reads **t 3.13 on points and 2.54 on xPoints**; |t| fell on five of six HOLD
contrasts. An SE-ratio gate passed a criterion the power evidence fails — **the lesson for any
successor protocol is to pre-register mean preservation beside it.**

⚠️ **"A flat constant cannot show a shape" was my closure ground and it is refuted.** The ladder
has a plateau-with-cliff — the first entry in this record's own shape taxonomy — and displayed it
on both metrics. The defect was in amendment 2's "≥4 consecutive monotone rungs", which cannot
represent that shape. **Fixing the criterion makes the verdict worse**, since the shape resolved
better on realised points. Declaring the pilot uninformative would have discarded the run's most
damaging evidence.

✅ **What survives is on the metric the scope excluded**: on `POLICY`, SE cuts of 30-60% with means
preserved (flat-recency t −2.21 → −3.22, ratio 1.06; `hl8−hl2` t 1.53 → 3.69). The instrument
sharpens the metric amendment 3 ruled out of the bundle and fails on the one it was built for.
`policy_xpoints` is kept as a second POLICY-side instrument — read beside points, never instead.

## Gates

`go build`, `go vet`, `go test ./...` clean at this commit. `CLAUDE.md` 72,047 of 73,728. Snapshot
regenerated at `556cabb` with figures moving by the commit stamp alone and `constants.csv`
byte-identical — two accumulators, one machinery, the points one provably untouched.

**Nothing shipped changed.**
