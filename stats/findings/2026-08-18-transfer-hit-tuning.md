# Transfer-hit tuning: the hits mostly pay, MinGainHit 3.0 stands, and the wait-verdict answers the question

The measurement the hit branch's no-gain-bar asymmetry was preserved for. The user,
2026-08-18, watching the 2025-26 replay take two transfers in GW2, GW5 and GW33: *"we take
so many hits. there's no way that pays off... measure which hits 'worked out'... if we lose
too many bets it's a sign we're not tuned well."*

Pre-registered in the doc comment of `TestDiagHitTuning` (`internal/backtest/hittuning_diag_test.go`),
committed at `a07f3f4` before the first cell ran, after a plan review that replaced the
original design. Banked cells at `stats/snapshots/2026-08-18-transfer-hit-tuning/cells/`,
sidecar at `hittune-hits.csv` beside them.

⚠️ **The headline was recomputed after an output review caught the first bank as a mixed
population.** The sidecar computed the `hit` flag and discarded it, so the first bank's "641
packages, 34.3%" folded the free transfers (gated at `min_gain` 0.4) in with the hits (gated at
`MinGainHit` 3.0). The flag was written at `a0877c2` and the 216-cell sweep re-run at that
commit (the banked provenance pins it; the paired cells statistics were unchanged — the two
runs' cells differ only in run_id). All per-hit numbers here are from the corrected sidecar.

## Why the knob is MinGainHit — the kink that killed the first design

The original plan swept a per-gameweek gain bar on the hit branch. The plan review showed
that bar cannot bind: a hit is accepted iff Gain·H − 4 − FreeCost·(n−h) beats the
alternative by MinGainHit, so the branch's implied per-gameweek bar is (MinGainHit + 4)/H =
**7/H = 1.4 pts/gw** at the shipped horizon — 3.5× the free single's 0.4. Rungs at 0.2-0.6
sat below the existing bar: a null by construction. **The knob that binds is MinGainHit
(3.0, net across the horizon, never swept until this run).**

## What ran

Seven arms, extended six seasons × six entry points, 36 cells per arm, 252 cells, `POLICY`,
23m56s, exit 0, 252 of 252 feasible, peak RSS 119 MB: the MinGainHit ladder 3 (shipped, flat
machine) / 4 / 5 / 6; `mgh3` on the **floored machine** (the shipped target — early floor
{1.0, 0.2} through GW8 + the override-mode corner); **no hits (wait)** (`MaxHits` 0); and
`mgh3` on the **full plan** (the floored machine on `FullAnchoredPlan` — both chip sets, all
four chips, the machine the user watches). The verdict sidecar carries one row per **package**
— a funded pair is one package, its legs summed, its hit charged once; a free single accepted
in the same gameweek as a hit single is its own row — with `out_played` for the availability
adjustment, `hit` for which packages paid the −4, and the holding-window columns (`hold_net`,
`hold_out_played`, `hold_weeks`, `out_was_captain`) from the pre-registered criterion. The
six pre-existing arms' 216 cells are **byte-identical to the first bank on every column**
(the pre-registered confinement — `Contrib` is recording-only), so every ladder figure above
stands; the horizon headline moved only through the package-unit fix.

## The ladder: nothing resolves, no shape, and nothing ships

| rung | a season | CR2 SE | t (df 5) | p | threshold | wild p |
|---|---:|---:|---:|---:|---:|---:|
| 4 | +2.0 | 6.3 | +0.31 | 0.767 | 16.3 | 0.722 |
| 5 | +6.0 | 5.2 | +1.14 | 0.306 | 13.5 | 0.364 |
| 6 | +2.1 | 7.3 | +0.29 | 0.785 | 18.8 | 0.755 |

Positive point estimates at all three rungs, no shape (5 the peak), nothing resolves.
`HOLD` byte-identical in 36 of 36 cells (the code fact). Season means are non-negative in
4/6, 3/6 and 4/6 seasons — **no rung clears the shipping rule's consistency clause.**

**The pre-registered shipping rule — (a) loss-rate below shipped beyond the paired noise,
(b) points ≥ 0 and ≥ 5 of 6 season means, (c) HOLD byte-identical, (d) hits not converted
to frees — clears on no rung.** On (a), the availability-adjusted per-cell loss-rate deltas
read **+0.016 / −0.010 / +0.045** (naive paired t 0.37 / −0.18 / 0.65, over the 26 / 25 / 22
cells where both arms took a hit with an available sold player — 10 / 11 / 14 cells dropped
for zero adjusted hits, 4 / 7 / 5 negative): rung 5 filters a
few losing hits directionally, rungs 4 and 6 run the other way — the survivors of a higher
bar are bigger bets with more variance — and none moves the rate beyond its own paired
noise. On (d), the hit reductions read 1.08 / 1.36 /
1.69 hits per cell against `moves − hits` rises of +0.08 / +0.03 / +0.11 — the bar refused,
it did not convert — though (b) alone already fails the rule on every rung (4/6, 3/6, 4/6
season means non-negative). **MinGainHit 3.0 stands; nothing ships.**
⚠️ The points arm was pre-registered as a veto only: the whole hit program is ~2.7 hits per
cell on average (max 8; ~11 points a season at 4 points a hit) against this comparison's own
thresholds of **13.5-18.8** —
unmeasurable by design, and the grid-wide ~26-39 figure does not apply to a contrast of
this size. ⚠️ The Holm family in the inference output is **6** with the full-plan arm
(every alternative against the baseline, sweep-wide; it was 5 at the first bank) where the
pre-registration registered **3** (the rungs only); no p moves under either — the three
rungs' Holm p are all 1.0, and the floored-machine arm's
0.0263 under family 6 is 0.0044 as its own single contrast.

## The verdicts — the user's question, answered twice

**The flat mgh3 machine's hit packages, against the gate's OWN bar (horizon criterion):**

| quantity | reading |
|---|---|
| net < 0 | **19.4%** (n 98) |
| net < 3 (the gate's own bar) | **23.5%** (n 98) |
| net < 3, availability-adjusted (sold player appeared) | **26.9%** (n 78) |
| mean / median hit_net | **+14.1 / +12.5**, spread [−61, +76] |

A calibrated gate gives ~50% by truncation at net < 3, so the measured 23.5% is BELOW the
null — **the gate is not mistuned in the feared direction. Three-quarters of hit packages
clear the gate's own bar ex post.** *"There's no way that pays off"* is measured false: the
mean hit package returns +14.1 after its −4. ⚠️ These numbers supersede the pre-fix bank's
24.5%/29.7%/+15.7 (the second bank's reading; the first bank read 34.3% on 641 packages):
the sidecar's row unit was fixed to one PACKAGE per row (a free single
accepted in the same gameweek as a hit single is now its own row, so a free leg's net no
longer folds into a hit's), which is what the pre-registration had always claimed the unit
was. ⚠️ The package-unit split eliminated the same-week merged rows: exactly one `n_moves > 2`
row remains in 4,785 sidecar rows (a full-plan free package), and none in the 98 mgh3 hit
rows, so the first bank's fidelity-note exclusion is now vacuous.

**The wait-counterfactual.** Season level: no-hits reads **−10.0 a season** (CR2 SE 16.0,
t −0.63, p 0.558) — waiting is not better, at point-estimate size, unresolved. Per hit
(descriptive, matched to the no-hits arm's later free purchase of the same in-player —
gw+1..gw+4, the earliest later purchase — 47% matched): **workedOut (≥ +4 vs waiting) in
54% of matched hits** (25 of 46 — a coin flip at this n, and the pairs are clustered by cell
so the effective n is smaller); mean hitNet +18.1 vs mean waitedNet +13.8 — hits beat
waiting by +4.3 on average after paying the −4 (descriptive, no SE quoted). The two
readings agree through the mean: the user's +4 hurdle is not cleared by a majority of
hits, but the wins are bigger than the losses and the policy level is better for taking
them.

## The holding-window criterion — the user's ruling, and it vindicates the hits

2026-08-18, the user on the horizon criterion: *"maybe we're judging on the wrong horizon.
We should only judge if it was +4 net points before they were either transferred out, or we
wildcarded. Anything less than +4 is not worth it for a hit."* Then: *"account for free hits
and bench boost too. Captaincy too."* Then: *"separate hits due to injury (the replaced
player stops playing) vs due to preference."*

The criterion as built (pre-registered at `d41b486`, plan-reviewed before the first cell):
per leg, from the transfer week until the earlier of the in-player's sale, a wildcard, or
the season's end, the incoming player's recorded **squad contribution** (`Week.Contrib` —
autosubs, the armband's copies, bench-boost bench and free-hit weeks all inside the same
scoring pass) minus the sold player's raw points; a hit worked iff the package's net is
**≥ +4**; forced (the replaced player stopped appearing after the transfer week) split from
preference (he kept playing).

| arm | hits | clear +4 in the hold | mean | median | preference clear | forced clear |
|---|---:|---:|---:|---:|---:|---:|
| mgh3 flat (no chips) | 98 | **79%** | +43.4 | +23.0 | 78% (n 73) | 80% (n 25) |
| floored machine (BB+FH+TC) | 82 | **78%** | +44.9 | +34.0 | 79% (n 57) | 76% (n 25) |
| full plan (all four chips) | 73 | **78%** | +35.2 | +17.0 | 75% (n 57) | 88% (n 16) |

Holds run mean 8.7-10.2 gameweeks (median 7-8, max ~30) — two to three times the five-week
horizon the old criterion judged on, which is exactly why it understated the hits. **On the
user's own criterion, roughly four in five hits pay for themselves, at a mean +35 to +45
after the −4, on every machine measured — and the forced/preference split does not separate
them** (75-88% clear in both populations; the sold player who stops appearing is what the
horizon criterion's availability adjustment already isolates). The `out_was_captain` flag
fires on 6-14 packages per arm — a sold captain is rare, so the out-side raw convention
understates by little. Free transfers read 64-73% non-negative on the same criterion
(n 608/598/591; the full plan's 73%, mean +12.2, the best).

**The registered rung pattern (H') — no suggestive shape.** On the preference population,
the holding-minus-horizon clearance gap by rung reads **+5 / +2 / +8 / +7** percentage
points (78-73, 73-71, 74-67, 67-60) at n 73 / 45 / 39 / 30 — no monotone widening, so no
evidence the bar buys horizon-net quality that does not survive the hold. The clearance
share declines overall with a blip at rung 5 on the holding criterion (78→73→74→67; the
horizon series is monotone, 73→71→67→60):
higher bars take fewer, bigger bets, and no rung clears the pre-registered shipping rule.
**MinGainHit 3.0 stands; nothing ships.**

⚠️ **The wildcard-week-after split is now live and is all free.** In the full-plan arm — the
only one that plays wildcards — 30 packages land the week after a wildcard, **0 of them
hits**: the post-wildcard churn the user flagged is free-transfer adjustment, and 73% of it
(22 of 30) is non-negative on the holding criterion (mean +17). The "two transfers the week
after every wildcard" observation is confirmed as real and mostly pays.

The full-plan machine resolves at **+97.6 a season** against the flat machine (2.569 pts/gw,
CR2 SE 0.667, t 3.85, p 0.0120, wild 0.0111, Holm 0.0600 under the sweep's family of 6) —
consistent with the floored machine's +99.2 and the option-decay run's +97.4; the user-facing
machine's edge over the flat one is suggestive rather than Holm-clearing.

⚠️ **The user's GW2 observation is a reporting convention, not evidence.** The funding
legs' gains are deliberately zeroed (none stands alone), so the +0.00 on the funding leg is
not a fact about the hit — the verdict tables are.

## What this settles, and what it does not

**Settles**: the hit branch is tuned about right — 23.5% (26.9% adjusted) below its own bar
against a 50% truncation null on the horizon criterion, and **78-79% of hits clear +4 on the
holding criterion** (mean +35 to +45 after the −4, holds of ~10 gameweeks) on all three
machines, the full user-facing plan included; the forced/preference split does not separate
(75-88% clear in both); raising MinGainHit has no suggestive case (no shape, no resolution,
rate deltas within their own noise, no widening hold−horizon gap, season means inconsistent);
waiting instead of hitting costs ~10 a season at point-estimate size. **MinGainHit 3.0
stays, and "there's no way that pays off" is refuted on both criteria at the measured
size.** The horizon criterion understated the hits — the holding criterion is the user's and
is the one to quote going forward; both are reported here.

**Does not settle**: whether a per-hit gain bar of a different shape (gross bars above 1.4
at H=5) would differ — the plan review showed the tested family cannot bind; and the
wildcard-week-after hit behaviour — it reads 0 hits in the full-plan arm, which is a count,
not a mechanism (the post-wildcard churn is free and pays).
