# The scheduled floor: live only where it can act, positive point estimates, unresolved

The schedule arm pre-registered in the gate-floor 2×2's doc comment: `{FreeCost 1.0, MinGain
0.2}` through GW8 — the midpoint of the user's "6-10 games" — the shipped `{2.0, 0.4}` after,
under the ON corner, against the ON baseline re-run in the same process.
`TestDiagScheduledFloor`, pre-registration committed at `e3ba914` before the first cell.
Banked cells at `stats/snapshots/2026-08-18-scheduled-floor/cells/`, provenance `e3ba914`,
`dirty=false`. Grid: extended six seasons × six entry points, 72 cells, `POLICY`, 8m05s,
exit 0.

## The license, restated from the 2×2's canary

The level 2×2 read S = −2.5 a season against 32.3 (nothing resolves) with entry-point columns
pointing the user's predicted way (+22.7/+26.5 early, −8.4/−41.4 late). Its pre-registered
canary discriminator fired: `floor_flips_gt28` live in 6 of 6 seasons, so the late half is
LIVE and the level null does not bind the schedule — the committed conditional licenses this
arm even though the committed license (S resolving) did not fire. Both readings are stated
rather than conflated.

## The result

**POLICY, schedule − baseline: +0.047 pts/gw, +1.8 a season** — CR2 SE 0.059, t 0.80, p 0.460,
threshold ≈5.8; start-fixed SE 0.069, t 0.68; wild p 0.429, S_eff 6. The second instrument
reads −0.064 pts/gw (`policy_xpoints`) — mixed at point-estimate size. **HOLD is
byte-identical**, the code fact. Nothing resolves.

## Where the arm can act — and where it cannot, by construction

The schedule window is GW ≤ 8, so **only GW1 and GW6 entries ever open it**: the 24 cells
entered at GW11+ are byte-identical by construction, and the pooled +1.8 dilutes the live
third by 3×. The liveness confirms the split exactly:

- **flips: 6 of 6 seasons with early flips; 0 of 6 late** — the pre-registered floor passes,
  and the restore is a confinement, checked rather than assumed (after GW8 the arm's gate IS
  the shipped gate and the counterfactual re-prices against the shipped constants).
- **moves differ in 7 of the 12 early-exposure cells** (GW1: 4, GW6: 3) and 0 of the other 24.

The honest figure is the live-subset one: **+6.7 a season at GW1 entry, +4.0 at GW6** — the
user's predicted direction as point estimates, at six cells each, nowhere near a threshold
(the GW1 column's own CR2 threshold is roughly 32). ⚠️ The sign claim has **no cell-level
majority**: 2 of 6 cells are positive at each live entry, the column means are carried by two
large cells, and no size was ever predicted — only a direction. A pooled figure over all 36
cells would be a comparison that is three-quarters confinement.

## What this settles, and what it does not

**Settles**: the schedule is wired, live where its window opens (flips 6/6, moves differ in
the early columns), and the pooled contrast reads +1.8 against its own threshold of ≈5.8 —
consistent with zero, nothing resolves. The live columns read +6.7/+4.0 as point estimates
(2 of 6 cells positive per entry), a sign with no cell-level majority. The knob
(`review_policy.early_floor`) ships, off by default, with the zero value as its backfill. The
shipped floor stays `{2.0, 0.4}`.

**Does not settle**: whether the schedule buys points — 12 live cells cannot, and the record
says an unresolved positive is not a licence to ship one. The early-entry grid is the binding
constraint: only entries at or before GW8 exercise the window, and the six-season grid
supplies two such columns.
