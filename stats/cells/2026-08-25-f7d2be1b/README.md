# ANCHORED, six-season extended grid, at the shipped config

`TestDiagAnchoredChips`, 36 cells per arm, five arms (control at fixed offsets;
anchored at full sight; anchored at 2, 4 and 6 gameweeks of sight). POLICY.

## Why these are banked and the earlier ones were not

The record carried a standing gap — no anchored-chip cells were banked anywhere,
so nothing that used one could be re-derived, only re-measured. These close it
for this comparison.

They also replace a contaminated run. Every chip sweep taken earlier on
2026-08-25 was measured against a tree carrying an uncommitted change to
`MinutesWeight` (1.25 -> 1.0) and to `minutesExponent`, while its sidecar stamped
a commit that shipped 1.25. The sidecar recorded `dirty=true`, which is precisely
the flag meaning it is NOT a complete record of what was measured: it enumerates
config and cannot see a changed function body. **This run's sidecar records
`dirty=false`.** It is the first chip measurement in that sequence where the
provenance is complete.

⚠️ **CORRECTED 2026-08-25 — that is TRUE OF `anchored.csv` AND FALSE OF
`decomposition.csv`**, which is where three of the four per-chip figures come from:

| block | commit | `dirty` |
|---|---|---|
| #1 wildcard | `a0f20a01` | false |
| #2 bench boost | `3b8bf1ab` | **true** |
| #3 free hit | `3b8bf1ab` | **true** |
| #4 triple captain | `3b8bf1ab` | **true** |

`anchored.csv`, `wildcardvalue.csv` and every other banked directory here are
clean; `decomposition.csv` is the only mixed one.

⚠️ **The mitigation is real and is not a clearance.** `a0f20a01..3b8bf1ab` is a
single commit touching only `internal/backtest/captainweekskill_diag_test.go` — a
different test file, off the scored path — so the uncommitted delta during blocks
2-4 was very probably that file mid-edit. **But `dirty=true` means precisely that
this cannot be verified from the record**, which is the principle the paragraph
above states. Treat **+5.6, +14.5 and −0.75** as measured on an unverifiable tree.

✅ **RESOLVED 2026-08-25 by re-run.** `TestDiagAnchoredChipDecomposition` was re-run
on a verified-clean checkout at `3b8bf1ab` (all four blocks `dirty=false`) and
compared cell-for-cell against this file: **288 of 288 identical, zero POLICY or
HOLD differences, max delta 0.** So **+5.6, +14.5 and −0.75 stand** — the
uncommitted delta touched nothing the scored path reads. Cells at
`stats/cells/2026-08-25-decomp-rerun/`.

⚠️ **That is "confirmed by re-run", NOT "clean".** This sidecar still records
`dirty=true` and always will. What the flag costs is a re-run instead of a lookup —
and this one was one `git gc` from impossible, since `3b8bf1ab` is orphaned by PR
#83's squash.

⚠️ **How the false claim survived a check.** It was verified with
`awk '$3=="dirty"{print $4; exit}'`, which stops at the first row — block #1, the
only clean one. **A provenance check that reads one row and generalises to the file
is not a check.** Use `awk -F, '$3=="dirty"{print $4}' … | sort -u`.


## Read them with the right estimand

⚠️ `Rscript stats/sweep_inference.R --scale=per_path`, NOT the default. A chip is
an **event count**: the cell total is the whole effect, and dividing by weeks
manufactures a per-gameweek rate that does not exist. The default `per_gw` scale
inflates these arms by roughly 1.7x, and that convention error has already
produced one retracted figure in this project's history — and then produced
another on 2026-08-25 before review caught it.

⚠️ `--scale=per_path` writes `stats/out/inference-per_path.csv`, a DIFFERENT file
from the default's `stats/out/inference.csv`. Reading the wrong one silently
returns the previous run's numbers.

## What they say

At 4 gameweeks of sight — the shortest lookahead the test's own bar accepts as
strategy rather than hindsight — anchoring reads **+20.6 points a season-path,
CR2 t 3.63 against a threshold of 14.5**, season-clustered at df 5. Positive in
six of six seasons (+0 to +36), and resolving in all six leave-one-season-out
subsets (weakest t 2.86). Full sight reads +24.0; 6gw +20.6; **2gw does not
resolve** (+12.9, t 1.89, threshold 17.5).

⚠️ These are NOT comparable with any figure banked before 2026-08-25: the shipped
`MinutesWeight` moved 1.25 -> 1.0 in the same branch, which reprices every
midfielder in every cell. An estimator swap reads as a data change.

---

## Also banked here: the per-chip decomposition and the wildcard-value arm

Same run conditions — shipped config, six-season extended grid, POLICY, read with
`--scale=per_path`. ⚠️ **NOT a clean tree**: three of the four blocks in
`decomposition.csv` record `dirty=true`. See the correction above.

`decomposition.csv` — `TestDiagAnchoredChipDecomposition`, four two-arm blocks,
one chip isolated per block at 4 gameweeks of sight:

| chip alone | pts/season-path | CR2 t | threshold | |
|---|---:|---:|---:|---|
| free hit | +14.5 | 2.44 | 15.3 | **does not resolve** |
| bench boost | +5.6 | 7.74 | 1.9 | resolves |
| wildcard | +6.4 | 0.61 | 27.0 | does not resolve |
| triple captain | −0.8 | −0.98 | 2.0 | does not resolve |

⚠️ **Only bench boost resolves individually, and it is the smallest of the three
that contribute.** Free hit is the largest single component and misses its own
threshold (14.5 against 15.3, t 2.44 against t_crit 2.571). The three bundled
chips sum to +19.4 against the bundled arm's +20.6, so the effect is additive —
but "free hit carries it" is a statement about a point estimate, not a resolved
one, and must not be quoted as measured.

`wildcardvalue.csv` — `TestDiagWildcardValue`, one wildcard at entry+8 against no
chip at all: **−7.6 a season-path, t −1.07, threshold 18.2.** A tie. Four of six
seasons negative, two clearly positive, LOSO 0 of 6. The standing claim that this
policy has nothing for a wildcard to undo reproduces.

---

## ⚠️ These cells PREDATE `5b970338` (PR #82) and are not a baseline for anything after it

Added 2026-08-25, after the fact. These were produced inside PR #83 (`74d7fe1f`).
**PR #82 merged AFTER #83** — the order on `origin/main` is `74d7fe1f` (#83) then
`5b970338` (#82) — and #82 rewrites `internal/analysis/blend.go`, `dpseed.go` and
`funding.go`, which is squad construction on the scored path.

**So a new arm measured on today's `main` cannot be differenced against these.**
The gap would carry #82's squad-feasibility change as well as whatever the new
arm varies, and attributing the whole of it to the new arm is the
estimator-swap-reads-as-a-data-change error arriving from the other side.

**How this was caught, because the tell generalises**: an arm varying only the
xGC source for 2020-21/2021-22/2022-23-GW15 moved **2024-25 and 2025-26** as
well — seasons the source cannot touch, and whose priors it cannot touch either.
**A deterministic replay cannot move a season whose inputs did not change.** When
it does, something outside the declared variable moved.

**What they are still good for**: they remain the record of what PR #83 measured
— complete for `anchored.csv` and `wildcardvalue.csv`, and `dirty=true` for three
of `decomposition.csv`'s four blocks — and every figure in the 2026-08-25 chip notes re-derives
from them exactly. **Re-deriving a published figure: yes. Serving as the control
arm for a new comparison: no — run the control at the same commit.**
