# The two-regime chip strategy, both data sources

`TestDiagTwoRegimeChips`, three arms, six-season extended grid, 36 cells per arm,
POLICY, `--scale=per_path`. Both runs at `5e8300a7`, both `dirty=false`,
differing in exactly one sidecar field (`FPL_XGC_EXTERNAL_DIR`).

⚠️ **"legacy" is the RECONSTRUCTION** — xGC inferred from opponents' expected
goals already in FPL's archive, which is what every public clone reads.
"measured-xgc" is the external per-match source.

## The rule

The user's own practice, 2026-08-25: *"Bundle the wildcard, free hit, and bench
boost in the second half of the year. Always. Triple captain is an afterthought.
Best available gameweek after those 3 are played. First half is more open. There
are no blanks and doubles so you mostly just wildcard if your team is bad."*

⚠️ **Every season is replayed under TODAY'S two-set rules**, including five that
granted one set. Authorised by the user — the points, minutes and fixtures are
real and only the allowance is counterfactual.

## Result — it resolves, and on BOTH sources

| arm | legacy | measured xGC |
|---|---|---|
| **two-regime (both sets spent)** | **+38.1, SE 8.46, t 4.51, thr 21.7, 6/6 — RESOLVES** | **+38.4, SE 8.66, t 4.43, thr 22.3, 6/6 — RESOLVES** |
| second set only, first set wasted | +26.2, SE 10.74, t 2.44, thr 27.6, 5/6 — no | +22.8, SE 10.91, t 2.09, thr 28.0, 5/6 — no |

Per season, two-regime:

| | 20-21 | 21-22 | 22-23 | 23-24 | 24-25 | 25-26 |
|---|---:|---:|---:|---:|---:|---:|
| legacy | +43.3 | +29.7 | +39.7 | +66.3 | +3.7 | +46.0 |
| measured | +48.7 | +37.2 | +28.5 | +66.3 | +3.7 | +46.0 |

⚠️ The last three columns are identical because the measured source covers only
2020-21, 2021-22 and 2022-23 GW1-15 — the scope check, passing again.

## ⚠️ Two things that make this different from every other chip result here

**It agrees across data sources.** +38.1 against +38.4, t 4.51 against 4.43, SE
8.46 against 8.66. The bundled anchoring arm and the free-hit arm both had their
SE roughly DOUBLE under the measured source; this does not. Spending eight chips
across both halves spreads the effect over more of the season, so a season-
specific xGC difference has less leverage on the total.

⚠️ **How much of the effect is the first set does NOT resolve.** The gap against
the otherwise-identical first-set-wasted arm reads **+11.9 legacy (SE 6.27, t 1.91,
threshold 16.1)** and **+15.6 measured (SE 7.01, t 2.22, threshold 18.0)**, positive
in **15 of 36 cells** on each. Both are below their own thresholds and neither is
positive in a majority of cells. **It is a point estimate.**

The arithmetic argument — a set unplayed by the GW19 deadline is lost, so an arm
that concentrates its plan in one half discards four chips — is a *separate*
argument and is not evidence for the size. ⚠️ The first version of this README
stated +11.9/+15.6 as measured fact with no threshold beside it, in a directory
whose whole point is that a result below its own threshold is not a result.

## ⚠️ What this does NOT show

**It is not comparable with `TestDiagAnchoredChips`'s +27.0** (the post-`5b970338`
reading; the pre-#82 +20.6 is superseded). Different control
(both sets, four chips each half, against one set of three), different chip count,
different set rules. Do not difference them.

**The first half is not the user's rule.** "Wildcard if your team is bad" is a
condition on the squad; here the first-set wildcard takes the earliest free week
and the other three fill behind it by offset. `xiDriftOf` is the signal that rule
needs and the planner seam cannot see it. **A positive result here is not evidence
for the reactive rule**, only for spending the set at all.

**2024-25 is nearly flat** (+3.7 on both sources) and a GW1 entry reads −3.7 on
legacy against +28.2 measured. The mean is not evenly earned.

**New, unreviewed code**, and the second-set bundle reuses `sightedWeeks` at a
later start rather than a placement rule designed for a bundle.
