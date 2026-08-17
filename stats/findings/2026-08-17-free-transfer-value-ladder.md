# The flat `free_transfer_value` ladder

**Nothing resolves, the ladder has no shape, and the shape question is confounded at the
baseline by arithmetic that was not noticed before the run.** Those are three separate findings
and the third is the one worth keeping.

Pre-registered in
[`2026-08-17-free-transfer-value-ladder-PREREGISTRATION.md`](2026-08-17-free-transfer-value-ladder-PREREGISTRATION.md),
committed at `1dc5d1d` before the first cell ran.

## What ran

`EXP=FREEVALUE FPL_SWEEP_SEASONS=extended scripts/replay -run TestDiagFreeTransferValue`,
5 arms × 36 cells on the six-season extended grid (2020-21 … 2025-26), entry gameweeks
1/6/11/16/21/26, `WeeklyXI` false, `BankUpTo` 5, all archive repairs on, `FPL_MAGNITUDE` unset,
no chips, no chip preparation, no banking lookahead, no oracles, the option-value taper off,
`FPL_BUDGET_WEIGHT` unset — which matters, because the kink identity below holds only while the
money term is zero.
12m33s, 125 MB peak RSS, exit 0, 180 of 180 cells feasible.

Metric `POLICY`, because this constant is *about* transfers. `policy_xpoints` beside it, never
instead of it.

Cells, means and provenance banked at
`stats/snapshots/2026-08-17-free-transfer-value-ladder/cells/`, provenance stamping commit
`c4de5ce` and **`dirty=false`**.

✅ **It ran twice and reproduced.** A first pass at `1dc5d1d` was discarded for banking because
its provenance stamped a dirty tree — the harness that produced the numbers was not yet
committed, so those cells could not have been tied to a commit. The banked re-run against the
committed tree is **byte-identical to it on `variant`, `season`, `start_gw`, `policy_points`,
`hold_points`, `moves` and `hits` across all 180 cells**, and every table below is unchanged
between them. That is a same-machine reproduction, so it says nothing about the recorded
cross-architecture caveat.

## The ladder

Paired per-cell difference against the shipped 2.0, on `per_gw × 38` as pre-registered.

| `free_transfer_value` | pts/gw | **a season** | CR2 t (df 5) | start-fixed t (df 25) | clustered threshold | start-fixed threshold | wild p | Holm p (on the CR2 p) |
|---|---|---|---|---|---|---|---|---|
| 1.0 | +0.231 | **+8.8** | 0.90 | 1.16 | 25 | 16 | 0.3784 | 0.9849 |
| 1.5 | +0.172 | **+6.5** | 1.08 | 0.90 | 16 | 15 | 0.3131 | 0.9849 |
| **2.0 (ships)** | — | *baseline* | — | — | — | — | — | — |
| 3.0 | −0.606 | **−23.0** | −1.75 | −2.25 | 34 | 21 | 0.1665 | 0.5649 |
| 4.0 | −0.275 | **−10.5** | −0.84 | −0.90 | 32 | 24 | 0.4275 | 0.9849 |

Thresholds are `t_crit(df) × SE × 38` computed per contrast by `stats/variance_components.R`,
`t_crit` 2.571 clustered and 2.0595 start-fixed; df 25 is `(S-1)(G-1)`, not an assumption.
Reported as a range because clustering is not uniformly conservative here. **Holm is applied to
the clustered p, not to the bootstrap p**, which sits beside it in the table and is easy to read
it as correcting: Holm on the CR2 p reproduces the printed 0.5649/0.9849 exactly, while Holm on
the bootstrap p would give 0.666 and then 0.939 for the other three. Checked here rather than
taken on trust.

The wild cluster bootstrap is Webb 6-point weights on the season, null imposed, enumerated
exactly. **`S_eff` is 6 on all four arms, so the floor is 6/6⁶ = 0.000129** — every arm is
**measurable**, and these nulls are ties rather than unmeasurable comparisons. That is worth
stating, because most nulls in this record are not.

`policy_xpoints`, the second `POLICY`-side instrument, reads −0.001 / +0.102 / −0.585 / −0.336
pts/gw for 1.0 / 1.5 / 3.0 / 4.0. It agrees in sign on three of four, and **the exception is the
arm with the largest positive points estimate**: 1.0's realised +8.8 a season is ≈0.0 on the
expected-points instrument, so the cheapest rung's apparent gain is not corroborated. The top two
also swap order. Point estimates only — no thresholds were computed for these — and it rescues
nothing.

## Nothing resolves

Every arm sits below its own clustered threshold, and Holm over the four contrasts leaves
nothing under 0.56. **A result below the detection threshold of its own comparison is not a
result, and that is what this is.**

One arm clears one estimator and it should not be read as a finding. `free_value=3.0` at −23.0 a
season exceeds its **start-fixed** threshold of 21 (t −2.25 against `t_crit(25)` 2.0595) while
failing its clustered one of 34 (t −1.75). The start-fixed estimator is valid only where the
season variance component is zero; for that arm `F_seas` is 1.66 at p 0.180 — the arm's own row, not the pooled one — and its
season share of variance is 10.0%, and `variance_components.R` says in terms that failing to reject `F_seas` is
**not** licence to read the start-fixed column — that test has about 30% power on this design. It
would not survive Holm on either estimator. Read it as the largest arm in a null ladder.

The season means behind it are wild: **+9.1 / −46.9 / +15.2 / −65.2 / −11.6 / −38.8** across
2020-21 … 2025-26 — negative in 4 of 6. That heterogeneity *is* the clustered standard error.

## The shape: there is none

Pre-registration made shape the primary result. The ladder is **not monotone**: 3.0 (−23.0) is
worse than 4.0 (−10.5), so the descent reverses at the far end. It is not a plateau with a cliff
either — the two low rungs are small and positive, the two high rungs are large and negative and
disordered between themselves.

The `4.0 − 3.0` between-arm contrast reads +12.6 a season at CR2 t 3.46, wild 0.0390. **This is
not quoted as a result.** It is one of six post-hoc between-arm contrasts, its p is raw, and
picking the reversal that happens to be largest is the argmax this record warns about on every
page. The honest reading is the absence: the ladder has no order to test.

⚠️ **No order was pre-registered for the points column**, so `stats/shape_inference.R` — which
refuses a trend test unless the order is supplied on the command line, precisely because noticing
an order and then testing it voids the p — was not run and may not be. Any shape claim
here is descriptive. The pre-registration committed
a direction for *moves* and for the 4.0 rung, and those are honoured below; it did not commit a
full order over the points column, and that is a defect in the pre-registration rather than
something to repair after the fact. The script's **peak distribution** needs no predicted order
and would strengthen the absence claim without a post-hoc p; it has not been run.

## The confound: the shipped value sits exactly on a kink

**This is the durable finding, and it is arithmetic rather than an estimate.** The identity is
already recorded in this repository from the other side — AGENTS.md's `min_gain` bullet says the
charge clause "already demands `charge/horizon` = 0.4", and `internal/config/policy.go` writes
the gate expression out in full. **What was not noticed before the run is the consequence for a
ladder spanning 2.0**: it splits into two mechanisms at its own baseline.

A free single swap is accepted iff `Gain ≥ MinGain` **and** `Gain·H + Money − FreeCost ≥ 0`
(`transferProposal.value` with one move and no hit, against an `Alternative` of zero). `Money` is
a real term and is zero only because `budgetWeight` ships at 0; the full expression also carries a
`Surcharge`, set only by the unified search, which does not ship. `H` is
`effectiveHorizon(sc.decisionHorizon(), gw)` — `decisionHorizon()` returns `DecisionHorizon` when
set and `Weights.Horizon` otherwise, and **this run took the latter**, `sweepConfig` never setting
the former. It shortens at the end of a season, which is the whole reason for the GW35 exception
two paragraphs down. The effective bar is therefore

```
max(MinGain, FreeCost / H)
```

and at the shipped decision horizon of 5 with `MinGain` 0.4, the shipped charge of **2.0 gives
exactly 0.4**. The two constants meet on the nose.

Three consequences, all of which change how this ladder may be read:

- **Below 2.0 the single-swap _bar_ cannot move** — 1.0 and 1.5 both push `FreeCost/H` under a
  floor `min_gain` is already holding. So every difference the low rungs produce enters through
  the funded pair, whose condition reduces to `gain·H + money − soloGain·H > FreeCost`, and
  reaches the singles loop only as path divergence. Decisions inside that loop do still differ
  once the path has diverged, which is what the per-cell counts below show.
- **Above 2.0 the singles bar does move** — 0.6 at 3.0, 0.8 at 4.0 — so the high rungs act
  through both channels. **The ladder varies one mechanism below its baseline and two above it**,
  which is why "monotone across all five rungs" was never going to be one shape.
- **An interior optimum at 2.0 would be confounded with `MinGain × DecisionHorizon`**, not a
  property of the charge.

The one exception is the end of a season, where `effectiveHorizon` shortens. ⚠️ **It starts at
GW35 for both low rungs, not GW36 and GW37** — a first version of this section had it wrong. The
binding condition is when a rung's bar differs from **shipped's**, not when the rung's own bar
clears `min_gain`, and shipped's own bar `2/H` rises above 0.4 the moment `H < 5`, which is GW35.
Four end-of-season weeks per cell, not two and three.

This is the exact mirror of the recorded `min_gain` result — *inert at or below 0.4, because the
charge clause already demands `charge/horizon` = 0.4* — read from the other side of the same
equation. It is now pinned by `TestTheFreeTransferChargeIsInertOnSinglesBelowTheKink`, which walks a gain
grid at the shipped horizon and requires byte-identical decisions. Three hardenings came out of
review, and each closes a way the guard could have passed for the wrong reason: it **refuses to
run** under the three switches that falsify the identity (`budgetWeight`, the option-value taper,
the unified search); it carries a **positive control**, failing if the gain grid does not straddle
the bar, since a constant answer would satisfy the inertness loop vacuously; and its liveness half
**pins the GW35 boundary** rather than asserting that one exists somewhere. That last assertion
immediately caught a real gap — at `H = 1` the bars run to 2.0 and the grid stopped at 1.2, so it
could not separate 1.5 from shipped and reported a disconnection that was a grid too narrow.
`TestTheSinglesProposalCarriesNoAlternativeOrStrictFlag` is the source half: the identity holds
only because `decide` builds the singles proposal with `Alternative` zero and `Strict` false, and
the test hard-codes both into its own literal rather than calling `decide`.

The per-cell counts corroborate it, for one arm rather than both. **`free_value=1.5` is diluted
by inert cells** — it changes `moves` in only **20 of 36** and `policy_points` in **23 of 36**,
against 35 and 36 for `free_value=4.0` — so its null is partly confinement rather than wholly a
tie. **`free_value=1.0` is not**: it moves `moves` in 32 of 36, more than `free_value=3.0` does,
the pair channel being further from the baseline.

⚠️ **A first version of this section claimed the census measured the split. It does not, and the
correction reverses the reading in both halves.** `useHit` is `free == 0` in `decide`, so a hit is
taken only after the week's free transfers are spent — which makes **every hit week a multi-move
week by construction**. The multi-move column is therefore the funded pair, the multi-free-move
week and the hit channel added together. `MaxHitsPerWeek` ships at 1, so `hits` counts hit *weeks*
and the subtraction is exact:

| `free_transfer_value` | moves | multi-move weeks | hits | residual |
|---|---|---|---|---|
| 1.0 | 1012 | 296 | 118 | **178** |
| 1.5 | 977 | 282 | 105 | **177** |
| **2.0** | **950** | **272** | **98** | **174** |
| 3.0 | 884 | 274 | 78 | **196** |
| 4.0 | 752 | 235 | 41 | **194** |

Netting the hit channel out, the residual is **flat below the baseline (178 / 177 / 174) and rises
above it (196 / 194)** — the opposite of the naive reading on both sides. Below the baseline about
83% of the movement is the hit channel (+20 of +24 at 1.0). At 3.0 the multi-move column's
"unchanged" 274 against 272 conceals hits falling 98 → 78 against a residual rising 174 → 196.

**So the census confirms that both channels move and attributes nothing.** The two-mechanism
reading of this ladder rests on the arithmetic above and not on this table. That is a weaker
finding than the one first written here, and it is the one the data supports.

## Liveness and confinement

**The constant arrives.** Pooled moves fall monotonically across the ladder —
**1012 / 977 / 950 / 884 / 752** for 1.0 / 1.5 / 2.0 / 3.0 / 4.0 — and per cell, against the same
cell's baseline:

| arm | `moves` differ | `hits` differ | `policy_points` differ | `hold_points` | `hold_xpoints` | `squad_hash` |
|---|---|---|---|---|---|---|
| 1.0 | 32/36 | 17/36 | 31/36 | **0/36** | **0/36** | **0/36** |
| 1.5 | 20/36 | 12/36 | 23/36 | **0/36** | **0/36** | **0/36** |
| 3.0 | 30/36 | 28/36 | 34/36 | **0/36** | **0/36** | **0/36** |
| 4.0 | 35/36 | 27/36 | 36/36 | **0/36** | **0/36** | **0/36** |

The `HOLD` columns and `squad_hash` are verified **cell by cell**, not off the printed arm total,
which can hide compensating movement. All four `HOLD` rungs — full, armband-pinned, nobody
doubled, and xPoints — are byte-identical in 144 of 144 cells **on each of the four columns**,
576 cell-column comparisons in all.

⚠️ **`squad_hash` is the OPENING fifteen**, built before the first weekly decision, so its 0/36
is forced by construction and is weaker evidence than the `HOLD` columns beside it. It says
nothing about the squad the arm went on to hold, which necessarily diverges in every cell where
`moves` differs.

⚠️ **That confinement is a code fact, not a result.** `FreeCost` is read only inside the weekly
transfer decision and `HOLD` makes no transfers, so re-running it can only fail. The check with
power is the liveness column beside it, and it is what licenses reading the points nulls as ties
rather than as a comparison that never ran.

## Churn: the recorded claim survives

The record says the charge is *"a volume brake, not an anti-churn device"* — raising it cuts
moves and round-trips, but the proportion that are round-trips barely shifts.

| `free_transfer_value` | moves | round-trips | trips/move |
|---|---|---|---|
| 1.0 | 1012 | 248 | 0.2451 |
| 1.5 | 977 | 222 | 0.2272 |
| **2.0** | **950** | **212** | **0.2232** |
| 3.0 | 884 | 191 | 0.2161 |
| 4.0 | 752 | 143 | 0.1902 |

From shipped to 4.0, **volume falls 21% and the round-trip share falls 15%**; across the whole
ladder, 26% against 22%. The share does move, and it moves in the direction the brake would
predict — but far less than the volume, which is what the recorded claim says. **Corroborated,
counts only, no threshold claimed.**

## The recorded four-point figure: direction yes, magnitude no

The record says charging the full four-point hit value dropped transfers from **73 to 39** and
scored *below* charging nothing.

- **The sign reproduces.** 4.0 is negative against shipped (−10.5 a season) and negative against
  the cheapest rung (`4.0 − 1.0` = −19.2 a season — a difference of point estimates, with no SE
  and no threshold computed for it, so it is not divided by anything either).
- **The magnitude does not.** 4.0 cuts moves by **21%** against shipped here, where 73 → 39 is a
  47% cut. Different era, different grid, different metric convention — that figure is an
  absolute total from single-path GW1 replays predating the doubles, selling-price and
  zero-penalty fixes, and it carries no threshold.
- **The comparator is absent.** "Charging nothing" is `free_transfer_value` 0.0, which is **not a
  rung on this ladder**. So the recorded comparison — 4.0 against 0.0 — has not been re-run, and
  this ladder does not settle it.

## Concentration

`stats/concentration_screen.R --metric=policy_per_gw`. No arm is CONCENTRATED: 0.59 to 1.46 of
each mean survives dropping its three largest cells. Two arms are flagged **MOST CELLS
DISAGREE** — `free_value=1.0` at 19/36 cells sharing the mean's sign and `free_value=4.0` at
19/36. `free_value=3.0` reads 22/36. So these are diffuse nulls rather than a few cells wearing
a mean, which is the better kind of null to have.

Leave-one-season-out: **no arm changes sign** on any of the six subsets, ranges +5.0 to +15.8
(1.0), +3.2 to +10.3 (1.5), −30.7 to −14.6 (3.0), −18.9 to −3.2 (4.0). ⚠️ **That stability is
arithmetic, not evidence** — each subset shares five of six seasons.

## What this does and does not license

- **`free_transfer_value` ships unchanged at 2.0.** No new default is proposed. Nothing resolved,
  and the ladder has no shape to argue from.
- **The level is now measured rather than untested**, which is what the run was for. The
  four-arm span is about 32 points a season against per-arm thresholds of 15 to 34, so the
  constant is somewhere in the band this harness cannot see into — the expected reading for an
  effect of that size, and not evidence that the charge does nothing.
- **The taper's attribution is improved but not discharged.** Two gaps remain. The taper is
  **mean-preserving by construction over `[1, Expiry]`** — `OptionValueAt` divides by
  `analysis.MeanOptionDecay`, and `internal/analysis/optionvalue.go` states the window and the
  residual in terms. So the level confound this run was commissioned against is smaller than the
  commission assumed. But the normalisation is exact only over the option's whole window: a cell
  entering at GW26 still carries a residual level shift, and the congestion half is not
  mean-preserving at all. **A taper arm should therefore be
  read per entry point against this ladder's per-entry-point columns**, which the banked cells
  carry.
- **A 0.0 rung is owed and was not run.** It is the taper's own expiry endpoint — the factor is
  exactly 0 at expiry — and it is the comparator in the one recorded figure this run re-tests.
  The ladder as pre-registered brackets 2.0 but stops at 1.0, so it reproduces one end of that
  recorded direction and not the other. One arm, 36 cells, ~3 minutes.
- **`AGENTS.md` had to change, and not because a figure was won.** It was carrying the claim that
  this constant *"has never been varied in any banked sweep"* — in five tracked places, counting
  `docs/configuration.md` and three Go comments — and this run falsifies it. Per the file's own
  convention a correction **replaces** the claim, so the sentence is gone and the ladder's figures,
  thresholds and null verdict stand in its place, with an explicit note that **0.0 is not a rung**
  so the recorded four-against-nothing comparison is still un-re-run. The kink arithmetic goes in
  too, as a clause on the existing `min_gain` bullet rather than a second bullet: it is one
  identity, and giving it two entries is the failure mode this file names most often.

## Caveats that bound every number above

- **`WeeklyXI` is false**, the `runPolicySweep` default and what the sibling `min_gain` ladder
  ran at. The *previous* version of this diagnostic ran at `true`, so this run is not directly
  comparable with the figure it re-tests. **Hypothesis, not measured:** fielding the horizon
  eleven rather than the imminent one dilutes the realised payoff of blank-and-double rotations,
  which are the best moves the policy makes and exactly the class the charge gates — which would
  understate what a transfer is worth and so flatter the high rungs.
- **`BankUpTo` is pinned at 5** in all six seasons, where 2020-21 … 2023-24 allowed 2. Every arm
  shares it, so it is not arm-level bias — but the charge is *the* lever governing spend-now
  against accrue, so the high rungs exercise a saving capacity four of six seasons did not have.
- **Two citations in the pre-registration were stronger than their sources** and are corrected
  here. The `min_gain` ladder swept above 0.4 carries **no recorded threshold and its cells were
  never banked**; the 21.7 and 34 belong to the horizon arm and the horizon-8 floor arm, which
  the record says explicitly must not be composed into one ladder. And the `POLICY` median
  threshold of 70 is a **four-season** figure over 23 comparisons; the six-season equivalent is
  unrecorded, and the record expects it lower — widening helped 10 of 11 transfer arms at a median
  SE ratio of 0.62 — but that is an ordering, not a figure. **This comparison's own thresholds are the only ones that bind, and they
  are 15 to 34.**
- **One reason given in the pre-registration is wrong, though its conclusion is not.** Unset
  `FPL_SWEEP_SEASONS` already returns the extended grid, so pinning it explicitly guards against
  an operator environment carrying `scoring` or `default` rather than against the default itself.
  A pre-registration is a dated artefact, so it is left as written and corrected here.
- **Run on arm64.** The points columns are integers and `squad_hash` is a digest, so both should
  reproduce on amd64 unless a decision flips; `policy_xpoints` and `hold_xpoints` are banked at
  full float64 and are not expected to.
