# The BUNDLED anchoring arm, controlled: reconstruction against measured xGC

`TestDiagAnchoredChips`, five arms (control at fixed offsets; anchored at full
sight and at 2/6/4 gameweeks of sight), six-season extended grid, 36 cells per
arm, POLICY.

Both runs at commit `91cedb29`, both `dirty=false`, differing in exactly one
sidecar field: `FPL_XGC_EXTERNAL_DIR` (absent in `reconstruction.csv`, set in
`measured-xgc.csv`).

⚠️ Read with `Rscript stats/sweep_inference.R --scale=per_path`.

## The scope check passes, so this is the source and not a confound

Maximum absolute per-cell POLICY difference over all 180 matched cells:

| 2020-21 | 2021-22 | 2022-23 | 2023-24 | 2024-25 | 2025-26 |
|---:|---:|---:|---:|---:|---:|
| 127 | 146 | 120 | **0** | **0** | **0** |

Only the three declared seasons move. **A deterministic replay cannot move a
season whose inputs did not change**; that they do not is what makes the
difference below attributable.

## ⚠️ The headline result does not resolve on the measured source

| arm | reconstruction | measured xGC |
|---|---|---|
| full sight *(hindsight ceiling)* | +30.9, SE 6.25, **t 4.95**, thr 16.1, 6/6 — resolves | +31.4, SE 9.80, **t 3.20**, thr 25.2, 5/6 — resolves |
| **4 gameweeks of sight** *(the strategy bar)* | **+27.0, SE 5.06, t 5.33, thr 13.0, 6/6 — RESOLVES** | **+26.4, SE 11.41, t 2.32, thr 29.3, 5/6 — DOES NOT** |
| 6 gameweeks of sight | +27.0, SE 5.06, t 5.34, 6/6 — resolves | +26.8, SE 11.67, t 2.30, 5/6 — does not |
| 2 gameweeks of sight | +21.1, t 2.42, thr 22.4 — does not | +19.6, t 1.57, thr 32.0 — does not |

**The effect estimate is stable and the precision is not.** 4gw reads +27.0
against +26.4 — a difference of 0.6 points — while the standard error more than
doubles, 5.06 to 11.41. Per-season at 4gw:

| arm | 20-21 | 21-22 | 22-23 | 23-24 | 24-25 | 25-26 | sd |
|---|---:|---:|---:|---:|---:|---:|---:|
| reconstruction | +37.8 | +23.7 | +6.2 | +22.2 | +32.8 | +39.2 | 12.4 |
| measured xGC | **−22.8** | **+61.8** | **+25.3** | +22.2 | +32.8 | +39.2 | **27.9** |

2020-21 and 2021-22 swing hard in opposite directions. That, and nothing else,
is what moves the SE.

## ⚠️ What this does and does not license

**It does not say either source is right.** Two readings fit:

- The reconstruction **borrows strength**. It derives xGC from opponents' xG
  inside the same archive, so it is mechanically tied to the same rows the squad
  is scored on, and may understate genuine season-to-season variation.
- The measured source **injects variance** those seasons do not really have.

Nothing here separates them, and
`2026-08-24-fotmob-is-opta-...` establishes only that the source is "not worse
than what ships", never more accurate.

**What it does say is narrower and firmer: a finding whose resolution depends on
which of two defensible xGC sources you use is not a resolved finding.** The
anchoring effect's SIZE is reproducible across sources at ~+27 a season-path. Its
significance at realistic sight is not.

⚠️ **This matters more now that the measured source is the local default.** Under
that default, the project's headline chip finding does not clear its own
threshold at 4 or 6 gameweeks of sight, and only the hindsight-ceiling arm — the
one the test's own bar excludes as strategy — survives.
