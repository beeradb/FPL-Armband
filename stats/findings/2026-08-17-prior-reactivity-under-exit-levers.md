# Prior reactivity under the exit levers: the 2×2 result

The re-judgement the user's hypothesis asks for: does a heavier prior — slower
reaction to new data — read differently once the machine has something to hold
a transfer for? Pre-registration lives in the doc comment of
`TestDiagPriorReactivityUnderExitLevers`, committed at `bca297a` before the
first cell ran. Banked cells at `/tmp/priorx.csv` → bank under
`stats/snapshots/2026-08-17-prior-reactivity-2x2/cells/` with this finding.

## What ran

Registered 2×2: factor A prior reactivity (`BlendRateK` 8 shipped vs 24),
factor B exit levers OFF (shipped) vs ON, where ON is the **override mode**
the user directed — a chip plan SET by the analysis layer (`anchoredPlan`,
full sight on the calendar's doubles and blanks), `AnticipateChips` so the
transfer decision knows the plan, `BankLookahead`, and `WeeklyXI` true so the
chip weeks field on the imminent gameweek (the package's recorded trap: an
arm about doubles and blanks that leaves it false switches off the fielding
half of its own mechanism). OFF arms stay at the shipped false.

Grid: extended six seasons (2020-21 … 2025-26) × entry GW1/GW16 = 12 cells
per arm, 48 cells, `POLICY`. 5m01s, peak RSS 116 MB, exit 0, 48 of 48 cells
feasible. Contrasts computed by `stats/priorx_contrasts.R` (duplicated
estimators from `sweep_inference.R`).

## The three registered contrasts (POLICY, paired per cell ×38)

| contrast | a season | CR2 SE | t (df 5) | p | threshold | Holm | start-fixed t (p) | wild p (S_eff 6) |
|---|---:|---:|---:|---:|---:|---:|---:|---:|
| A: k effect (mean over levers) | **−20.4** | 41.9 | −0.49 | 0.647 | 107.7 | 0.981 | −1.40 (0.221) | 0.642 |
| B: levers effect (mean over k) | **+73.0** | 14.1 | **+5.18** | **0.0035** | 36.2 | **0.0105** | +8.00 (0.0005) | 0.0078 |
| AxB: interaction | **+19.4** | 26.1 | +0.74 | 0.491 | 67.0 | 0.981 | +0.79 (0.467) | 0.472 |

**B resolves.** The override-mode configuration — chips placed on the
calendar rather than chosen by an optimiser, anticipated by the transfer
decision, and fielded properly — is worth **+73.0 a season against its own
threshold of 36.2**: double the threshold on the season-clustered estimator,
clear on the start-fixed rival (t 8.00), wild cluster bootstrap p 0.0078 at
S_eff 6 (floor 0.000129), Holm 0.0105 over the three registered contrasts.
The xPoints instrument corroborates (+2.05 pts/gw ≈ +78 a season). This is
the first chip-side lever in this record to resolve positive.

**A does not resolve.** The heavier prior, averaged over both
configurations, reads −20.4 a season against a threshold of 107.7 — the
season heterogeneity on the k contrast is enormous, and the flat prior's
record stands: nothing here moves it.

**AxB does not resolve, and the direction is as predicted.** The
pre-registered interaction (heavier prior loses *less* once there is
something to hold) is +19.4, positive as predicted, and about 3.5× below its
threshold of 67.0. The simple effects tell the same story at point-estimate
size: k24−k8 reads **−30.1 with levers off and −10.7 with levers on** — the
penalty for a heavier prior shrinks by about two-thirds once the machine has
exits, but neither simple effect resolves (thresholds 120.3 and 104.8).
Recorded so the direction is not re-discovered as new.

## Liveness and mediators

- The plan is a THREE-chip plan: bench boost on the biggest double, free hit
  on the biggest blank, triple captain on the second-biggest double. **The
  wildcard is structurally absent** — `matchedChips` intersects with
  `controlWeeks`, which by design never places a wildcard (holding it at a
  common week once made the full-sight arm the only one boosting a
  wildcard-rebuilt squad). The B effect therefore says nothing about
  wildcards, and the record's standing position — the replay cannot value
  one — is untouched.
- Bench boost played in **12 of 12** ON cells, free hit planned in **12 of
  12** (the cells CSV exports no free-hit play column, so it is verified by
  plan + the ordinary chip mechanism rather than counted), triple captain in
  **8 of 12** — the 2024-25 and 2025-26 plans hold only **two chips** (no
  triple captain matched). Each chip plays at most once per cell; each
  SEASON enters the grid twice (GW1 and GW16 entries), so firings count
  twice per season.
- **Decomposition of the raw ON−OFF totals at k=8** (registered contrast
  aside): +550 points over 12 cells, of which the chip WEEK payouts
  (bench_boost_pts + triple_captain_pts) are **+188 (≈15.7 a season)** and
  the remaining **+362 (≈30.2 a season)** is everything else — the free
  hit, the transfers made anticipating the plan, and `WeeklyXI` fielding the
  imminent eleven all season. The cells with the largest effects have almost
  no chip payout: 2024-25 GW1 is +108 with payouts of 3, and both 2025-26
  cells are +45/+84 with payouts of 2. **The resolution is mostly the
  machine around the chips, not the chips' own scoring weeks** — the
  registered no-sub-attribution limitation, now quantified.
- **`banked_weeks` is zero in all 48 cells** — banking never executed,
  exactly as the record predicts at shipped `MaxHits` (the inert 2→3+
  boundary). Banking contributes nothing to B.
- The OFF corner fires nothing, as shipped.
- `HOLD` moves only on the k arms (+0.011 pts/gw for k24 ≈ 0.4 a season, the
  prior moving the weekly re-pick) and is byte-identical on the lever arms —
  the confinement the design expects.

## The registered moderator split

All **12 of 12 cells are live**: every season holds doubles and blanks from
both entries, and the plan's first chip lands at GW8 or later in every cell,
never immediately after entry. The inert half has **zero cells**, which the
pre-registered floor rule (below 4 of 6 → insufficient data) reads as
**insufficient data on the inert side**, not a finding. The prediction "the
effect concentrates in the live half" is vacuously where it lives —
everything is in the live half on this grid, and the split buys nothing
here. A grid with a genuinely chip-free window (a mid-season entry past the
last double, say) would be needed to give the split teeth.

## Registered limitations, as carried into the result

- **Compound corner, no sub-attribution.** The B effect is the joint
  configuration; with banking never executing it is effectively chips +
  anticipation + WeeklyXI fielding, but the design does not separate those
  three. In particular, no arm isolates `WeeklyXI` true without chips.
- **Full sight is an upper bound.** `anchoredPlan` knows every double and
  blank from entry; the anchored-chips record prices imperfect sight at 2-6
  gameweeks of lag. A real manager's figure is lower by that cost.
- **The k effect inside the ON corner is measured under `WeeklyXI` true**;
  the cross-config difference in k effects carries that alongside the
  levers, as part of the compound the ask named.

## What this settles, and what it does not

**Settles**: a chip plan SET on the calendar and executed with anticipation
and correct fielding is worth roughly 73 points a season against its own
threshold on this grid — the override mode the user directed is the right
shape, and it resolves where every state-trigger and timing attempt in this
record failed. The user's core interaction (slower reactions worth more once
there is something to hold) points the right way but is 3.5× below
resolution.

**Does not settle**: what inside the corner buys the 73; whether lagged
sight preserves it; whether a wildcard belongs in the plan (none was
matched); and the prior question itself, which remains where the record left
it — flat at k=8, unresolved, direction upward on this evidence as before.
