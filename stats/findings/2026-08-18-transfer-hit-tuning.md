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
banks' cells differ only in run_id). All per-hit numbers here are from the corrected sidecar.

## Why the knob is MinGainHit — the kink that killed the first design

The original plan swept a per-gameweek gain bar on the hit branch. The plan review showed
that bar cannot bind: a hit is accepted iff Gain·H − 4 − FreeCost·(n−h) beats the
alternative by MinGainHit, so the branch's implied per-gameweek bar is (MinGainHit + 4)/H =
**7/H = 1.4 pts/gw** at the shipped horizon — 3.5× the free single's 0.4. Rungs at 0.2-0.6
sat below the existing bar: a null by construction. **The knob that binds is MinGainHit
(3.0, net across the horizon, never swept until this run).**

## What ran

Six arms, extended six seasons × six entry points, 36 cells per arm, 216 cells, `POLICY`,
16m04s, exit 0, 216 of 216 feasible, peak RSS 119 MB: the MinGainHit ladder 3 (shipped, flat
machine) / 4 / 5 / 6; `mgh3` on the **floored machine** (the shipped target — early floor
{1.0, 0.2} through GW8 + the override-mode corner); and **no hits (wait)** (`MaxHits` 0). The
verdict sidecar carries one row per **package** — a funded pair is one package, its legs
summed, its hit charged once — with `out_played` for the availability adjustment, `hit` for
which packages paid the −4 (the registered rates are per HIT), and the in-player ids for the
wait-match.

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
read **+0.003 / −0.024 / +0.032** (t 0.07 / −0.41 / 0.42, over the 25 / 24 / 21 cells where
both arms took a hit with an available sold player, 4 / 7 / 5 negative): rung 5 filters a
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
this size. ⚠️ The Holm family in the inference output is **5** (every alternative against
the baseline, sweep-wide) where the pre-registration registered **3** (the rungs only); no
p moves under either — the three rungs' Holm p are all 1.0, and the floored-machine arm's
0.0219 under family 5 is 0.0044 as its own single contrast.

## The verdicts — the user's question, answered

**The shipped arm's 98 hit packages, against the gate's OWN bar:**

| quantity | reading |
|---|---|
| net < 0 | **20.4%** |
| net < 3 (the gate's own bar) | **24.5%** |
| net < 3, availability-adjusted (sold player appeared) | **29.7%** (n 74) |
| mean / median hit_net | **+15.7 / +14.0**, spread [−61, +76] |

A calibrated gate gives ~50% by truncation at net < 3, so the measured 24.5% is BELOW the
null — **the gate is not mistuned in the feared direction. Three-quarters of hit packages
clear the gate's own bar ex post.** *"There's no way that pays off"* is measured false: the
mean hit package returns +15.7 after its −4. ⚠️ Fidelity note: five of the 98 rows are
same-week packages with `n_moves > 2` (a funded pair plus free singles merged in one row,
because the sidecar groups by gameweek); two of the five are below the bar, so excluding
them would move the headline 24.5% → 23.7% — inside any noise, and both counts stay well
below the 50% null.

**The wait-counterfactual.** Season level: no-hits reads **−10.0 a season** (CR2 SE 16.0,
t −0.63, p 0.558) — waiting is not better, at point-estimate size, unresolved. Per hit
(descriptive, matched to the no-hits arm's later free purchase of the same in-player —
gw+1..gw+4, the earliest later purchase — 48% matched): **workedOut (≥ +4 vs waiting) in
51% of matched hits** (24 of 47 — a coin flip at this n, and the pairs are clustered by cell
so the effective n is smaller); mean hitNet +19.5 vs mean waitedNet +13.8 — hits beat
waiting by +5.7 on average after paying the −4 (descriptive, no SE quoted). The two
readings agree through the mean: the user's +4 hurdle is not cleared by a majority of
hits, but the wins are bigger than the losses and the policy level is better for taking
them.

The floored machine: **+99.2 a season** (CR2 SE 20.1, t 4.92, p 0.0044, wild 0.0070) — the
machine contrast, consistent with the option-decay run's +97.4. Its own hit loss rate reads
26.8% (net < 3, n 82) — the shipped machine's hits behave like the flat machine's.

⚠️ **The wildcard-week-after split is vacuous in this measurement**: the measured corner's
planner plays no wildcard (the anchored measurement's own design — the confound the
wildcard-rebuilt squad installs). The user's wildcard-week-after observation belongs to the
user-facing replay, whose full plan DOES play wildcards; the split remains a registered
column with no data here.

⚠️ **The user's GW2 observation is a reporting convention, not evidence.** The funding
legs' gains are deliberately zeroed (none stands alone), so the +0.00 on the funding leg is
not a fact about the hit — the verdict table is.

## What this settles, and what it does not

**Settles**: the hit branch is tuned about right — 24.5% (29.7% adjusted) below its own bar
against a 50% truncation null, mean package +15.7; raising MinGainHit has no suggestive case
(no shape, no resolution, rate deltas within their own noise, season means inconsistent);
waiting instead of hitting costs ~10 a season at point-estimate size. **MinGainHit 3.0
stays, and the "there's no way that pays off" hypothesis is refuted at the measured size.**

**Does not settle**: whether a per-hit gain bar of a different shape (gross bars above 1.4
at H=5) would differ — the plan review showed the tested family cannot bind; the
wildcard-week-after hit behaviour (vacuous here); and the floored machine's hits under the
full user-facing plan (the measurement corner plays no wildcard).
