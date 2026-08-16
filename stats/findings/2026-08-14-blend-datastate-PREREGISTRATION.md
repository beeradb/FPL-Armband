# Does the documented data state explain the `BlendRateK` reversal?

**Written before the run**, on the merged tree (`blend-schedule-cells` after `origin/main` at
`9bba522`). Results go in `FINDINGS.md` beside this file; this document is not edited afterwards
except to mark, in place, anything it got wrong.

## Why this run exists, and what it corrects about the last one

Banking `BlendRateK`'s cells (`stats/snapshots/2026-08-14-blend/`) reversed a recorded 24-cell table
in `docs/notes/constants-and-sweeps.md`: two of three arms flip sign, and the recorded regime
inversion reverses. I marked it stale with **"cause unattributed"**, having checked only that grid
width does not explain it.

⚠️ **That was one check short of the obvious one.** CLAUDE.md carries an explicit recipe:
*"Reproducing anything recorded before 2026-08-10 needs two switches: `FPL_SWEEP_SEASONS=default`
**and** `FPL_NO_XG_REPAIR=1`, which disables the xGC reconstruction too."* The reversed table entered
`constants-and-sweeps.md` at `fcb969c` on **2026-08-10**, in the split that created the file, so the
measurement predates the boundary and the recipe applies to it. **The banked run used neither
switch.** "Cause unattributed" was therefore premature: there is a documented candidate cause and
nobody had tried it.

## The run

```
EXP=BLEND FPL_SWEEP_SEASONS=default FPL_NO_XG_REPAIR=1 \
  FPL_CELLS=stats/snapshots/2026-08-14-blend-datastate/cells/blend-old.csv \
  scripts/replay -run TestDiagRejudge -v -timeout 3h
```

Four arms — `BlendRateK` 8 (ships), 12, 16, 24 — on the **four-season** grid at the six entry
gameweeks. **96 cells.** `scripts/replay` sets `DIAG=1`.

## The three states, two of which are already in hand

| state | k=12 | k=16 | k=24 |
|---|---:|---:|---:|
| **recorded**, pre-2026-08-10, 24 cells | −0.632 | −0.740 | −1.509 |
| **current** data state, restricted to the same four seasons | −0.445 | +0.302 | +0.124 |
| **old** data state — repairs off, four seasons — *this run* | ? | ? | ? |

`HOLD` pts/gw, paired against `BlendRateK=8`.

## The decision rule, fixed in advance

- **If this run lands near the recorded row** — the backfills explain the reversal. It becomes an
  ordinary data-state finding, already covered by "name the data state or do not quote a recorded
  level", and the retraction in the note gets its cause. Nothing further is owed.
- **If this run lands near the current row** — the backfills do **not** explain it, and the cause is
  a harness change between that table and today. That makes `BlendRateK` the **eighth** constant to
  lose its documented structure to a re-sweep, and it bears on every figure from that era rather
  than on this one constant. It would then be worth a bisect, which is tractable: the sweep is
  ~10 minutes.
- **If it lands between the two** — both contribute, and neither is dismissible.

## My prior, stated so the run is informative rather than confirmatory

**I expect it will NOT reproduce the recorded row**, and the indirect evidence already in hand is
why: among the four shipped seasons the backfills move **only 2022-23 cells**, and dropping 2022-23
entirely from the banked run still leaves `k=16` and `k=24` **positive**. So the reversal survives
removing the only season the switches can reach.

⚠️ **That is suggestive and not decisive**, which is exactly why the run is worth doing: dropping a
season changes an estimate for reasons other than the intervention, and a 6-of-24-cell data change
can move a mean without dominating it. Registering the prior means a reproduction would be a genuine
surprise rather than something read back into the noise.

## What this run cannot do

- **It cannot identify *which* harness change**, only whether the data state is sufficient. A
  negative result buys a bisect, not an answer.
- **It cannot rule out an interaction** between data state and harness change — the two are not
  crossed here, and this is a two-corner comparison, not a 2×2.
- **It cannot reconstruct the recorded table's full config.** That table has no banked cells, so
  beyond the switches, its settings are known only from prose. One confound is excluded by
  mechanism rather than by evidence: `min_gain` was at 0.7 for part of that era and is 0.4 now, but
  it is confined to `decide()`, which `HOLD` never calls, so it cannot touch the column being
  compared.

## What will be read

`sweep_inference.R` on the new cells for the per-arm paired differences and CR2, then a direct
three-way comparison against the table above. **The comparison is of point estimates against a
recorded row that carries no cells**, so this is an attribution question, not a significance one —
no p-value decides it, and none should be quoted as if it did.
