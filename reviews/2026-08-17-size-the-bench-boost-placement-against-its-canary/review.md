# Sizing bench-boost placement against its own canary

Reviewed: `origin/development..HEAD` on `size-the-bench-boost-placement-against-its-canary`, three
commits — `b44088d` (pre-registration and the arms), `e579f30` (the inference script), `f96f0d3`
(the measurement, the record and the banked cells).

## What was reviewed

A two-block replay measurement of bench-boost **placement**, gated on a pre-registered canary. The
canary is exact rather than approximate: a placement rule's pick is one element of the per-week gain
slice the chip-week oracle's argmax maximises over, so `rule_i ≤ ceiling_i` cellwise and a ceiling
that cannot be separated from the comparison's own dispersion closes the family.

The design rests on a bench boost being **path-invariant** on this code, which the diagnostic
executes rather than asserts — including an exact integer identity,
`Δpolicy_points == bench_boost_pts`.

No production Go changed. The non-test surface is `AGENTS.md`, `stats/bench_boost_placement.R`, two
`stats/findings/` entries and the banked cells under
`stats/snapshots/2026-08-17-bench-boost-placement/`.

## Reviewers

| reviewer | ran | why |
|---|---|---|
| **fpl-stats-review** | yes | the branch produces two measured figures and a gate criterion |
| **fpl-code-review** | yes | `internal/backtest` (harness) and a new `stats/*.R` reader |
| **fpl-findings-audit** | yes | `AGENTS.md` is edited and the run bears on three of its existing chip bullets |
| fpl-docs-review | not applicable | no change to `README.md` or `docs/` |
| fpl-security-review | not applicable | no change to `internal/fpl`, `internal/agent`, credential handling or config persistence |
| fpl-season-maintenance | not applicable | none of the four hand-maintained lists is touched |
| fpl-run-review | not applicable | no live run; nothing wrote config |

All three reproduced the headline figures independently from the banked cells before judging them,
and all three agreed nothing was numerically wrong. **Every finding below is about what the numbers
were allowed to mean, or about a latent trap for the next run.** I re-derived each reviewer figure
myself before acting on it — two of the corrections are to errors of mine, and one reviewer count
(oracle weeks at GW ≥ 30) did not reproduce and was not used.

## Must-fix findings, and their outcome

### 1. The contrast confounds PLACEMENT with LATENESS — RECORDED, arm NOT run

The state rule plays the chip at mean **GW 27.97** against the control's **19.50** and the oracle's
**27.75**. That is designed in rather than discovered: `ChipBarAt` decays the bar to the chip's
expiry, so a rule finding nothing early fires late by construction. **The contrast therefore cannot
attribute +5.778 to reading state.**

Verified independently. The control column is a six-point fixed-week ladder — GW7/12/17/22/27/32 at
3.17 / 4.33 / 3.67 / 4.17 / 5.00 / 1.83 — with no trend, so "later is generically better" is
unsupported; but those points are confounded with entry point and squad, so it does not clear the
rule either.

**Applied**: the timing mediator is now printed by the Go diagnostic and by the R script, and the
confound leads the caveats in both `AGENTS.md` and the finding.

**Declined**: running the calendar-anchored-double arm the reviewer proposed, which is the
comparator that would settle it. It is ~11 minutes and it is the right next step — but an arm chosen
*after* seeing the numbers is exactly the family growth the pre-registration exists to prevent, and
bolting it on would convert a clean two-stage design into a three-arm search. Recorded as unrun and
owed, in both the finding and `AGENTS.md`.

### 2. "The control is a poor policy" was backwards — FIXED

I had apologised for the control being weak. Measured against the banked `bench_boost_median_pts`,
the median week beats the fixed offset by **+0.583**, season-clustered SE 0.533, t 1.09 against a
threshold of 1.37 — **does not resolve**. The offset is statistically indistinguishable from a
randomly chosen week, which is what a placement control should be. The real content is a fact about
the chip: a bench boost on an arbitrary week is worth ~4 points because it forfeits the autosubs.

### 3. Both levels are non-negative BY CONSTRUCTION — FIXED

`weekScoreWithChip` skips the autosub step under a bench boost and the substitutes are a subset of
the bench players who played, so `BenchBoostGain ≥ 0` identically (min 0 over all 144 banked gain
values). The finding quoted the rule's +9.47 level bare, three sentences after disowning the
ceiling's t for exactly that reason. Both levels now carry the mechanical label, and the sentence
that matters is added: **nothing here weighs the chip against its own opportunity cost.**

### 4. The path-invariance check is a confinement, and one third of it was vacuous — FIXED

For the ceiling block's oracle arm the identity is `0 == 0` — `AxisChipWeek` plays no chip and
`mustNotMoveForAxis` already declares the invariance — so it is powerless there. It has content in
**25 of 36** cells for the control and **33 of 36** for the rule; the liveness half is the chip's
week moving in 34 of 36. `verifyPathInvariance` now returns and prints the non-vacuous count, and
both records lead with the denominators.

Also fixed: `verifyPathInvariance` silently skipped a cell whose baseline arm was missing while the
report tables required only arms 1 and 2, so such a cell would have entered a reported difference
unchecked. It is now counted and is an error. Inert on this run (0 infeasible rows).

### 5. The invariance is conditional on four switches — RECORDED

`wildcardBuildsForBoost` (on by default), `PrepareBenchBoost`, `trig.anyPlays` and
`TaperFreeTransferValue` each provide a channel by which a bench boost reaches the transfer path.
All four are off here and the three executed checks would catch a violation as a `t.Error` — but the
header, the pre-registration and the `AGENTS.md` bullet all said "reaches `weekScoreWithChip` and
nothing else" unqualified, which is the sentence a future arm crossing placement with a wildcard
would quote as licence. Now qualified in all three.

### 6. The R script counted an unfired rule as a moved week — FIXED

`moved <- sum(reading_gw != bench_boost_gw)` counts a cell where the rule never fired (`gw == 0`) as
*moved*, and its `diff = 0 − ctl_pts` enters the mean as a real negative indistinguishable from a
badly-placed chip. Inert here (fired 36 of 36), but the R script is the sanctioned reader for a
re-run off banked cells, where the console is gone. `placed` and `moved` are now separate counts,
and a shortfall in `placed` prints its own warning. The Go side already counted `fired` separately —
the weakness was in the pre-registration's definition of liveness, and the implementation was better
than the document.

### 7. The recovered fraction had no interval and no committed derivation — FIXED

`read_cells` keys on `(run_id, sweep, season, start_gw)`, so the two blocks cannot be joined by the
shared key, and 0.358 was computed by hand. The script now takes `--ceiling=<path>`, checks the
shared control arm is byte-identical (36 of 36) rather than assuming it, and prints
**0.358, Fieller 95% [0.198, 0.518]** plus the by-entry span **0.049 to 0.704**. The season-clustered
moments it needs for the covariance are checked against `se_cr` and fail loudly on disagreement,
rather than being a second implementation of the estimator.

### 8. The gate used the wrong arm's dispersion — RECORDED as a standing rule

The canary was gated at 2.06 (its own SE 0.803) where the rule's threshold turned out to be 2.65
(SE 1.032) — 22% looser, in the direction that flatters the instrument. It did not bite at a sixfold
margin, but the pre-registration predicted a *marginal* gate, and on a marginal one this decides the
run. Added to *Standing rules*: **a canary is judged against the SE of the arm it gates, so gating on
its own SE makes the check necessary and not sufficient.**

### 9. Two independent bar-16 rules, disagreeing by 8.4 — RECORDED

`firstClearing` (behind `*_threshold_pts`) takes the first week whose **realised** gain clears 16;
`BenchBoostTrigger` scores a **projection**. Levels against no chip **17.889** and **9.472**,
agreeing in 6 of 36 cells. And the bar is two literals — `chipBarBenchBoost` and
`config.DefaultBenchBoostBar` — with no reference between them and no equality test. This branch is
the first to compare a column derived from one against an arm driven by the other, so the exposure is
now live. Both recorded; the equality test is left as a separate cheap change.

**Declined for now**: adding that test here. It touches a constant outside this branch's scope and is
better as its own commit with its own reasoning.

### 10. "bar 16" mis-describes the tested policy — FIXED

`bb_trig_bar` at firing spans **3.79 to 18.14**. The object tested is a decaying option reservation
*based at* 16, and its decay (`DefaultOptionHalfLife` 8) is asserted and unswept on the same footing
as the bar — and is what produces the lateness in finding 1. Both records now say so, and the
half-life joins the named conditions.

### 11. `WeeklyXI = false` sets the SHAPE, not the level — FIXED

The bench is what `pickXI` left out, so at `WeeklyXI` false it is chosen on the five-week horizon —
the fielding half of a doubles mechanism is off, while 16 of 36 oracle picks are GW ≥ 33. The paired
contrast survives because both arms share the bench; the gain profile the oracle and the rule
optimise over does not transport. Restated in both records.

### 12. The ceiling is a mixture of six argmax problems — FIXED

It ranges over 38 weeks at GW1 and 13 at GW26, falling monotonically 18.83 → 13.50, and picks the
**entry week** in 2 of 36 cells, which `gw > start` forbids the rule. Both now printed by the
diagnostic and stated in both records.

### 13. The size-budget comment reasoned from a difference and got it backwards — FIXED

It said "the previous raise left 197 bytes free". `AGENTS.md` is **136,244 bytes at
`origin/development`**, so the 136 KB ceiling left **3,020** free; the 197 was *this entry's own*
margin, meaning the entry fitted and the first raise was unnecessary. That is a direct breach of the
same comment's own ⚠️ *"quote a SIZE and the commit it was measured at, never a difference"*.
Corrected in place, and the budget is now 144 KB because the reviewed entry genuinely does not fit
under 140.

### 14. Estimator mislabel and a dropped standing caveat — FIXED

`AGENTS.md` said "start-fixed" where `stats/bench_boost_placement.R` clusters on `start_gw`; the
pre-registration and the finding both said "entry-point-clustered". Corrected. `S_eff` now carries
its floor, and the LOSO line carries the standing caveat that sign stability across subsets sharing
five of six seasons is arithmetic rather than evidence.

### 15. Two pre-registered predictions failed and nothing recorded it — FIXED

The pre-registration predicted a marginal gate (it cleared eightfold) and that the rule would not
resolve (t 5.60). It held the same path-invariance mechanism the finding now uses to explain the
resolution, so what was miscalibrated is the **assumed per-cell dispersion of a one-week chip gain**,
not the mechanism. Recorded in the finding as the run's most calibrating output.

### 16. The run closes a neighbouring bullet's open item — APPLIED

`AGENTS.md` said of the chip-timing levels "nothing has been re-measured under the banked schema; a
re-sweep is owed". `bbceiling.csv` banks all four readings for both scoring chips, 36 cells, current
schema. Recomputed: timing **+6.39** and threshold rule **+30.15**, summed over the two chips,
against the recorded +8.3 and +21.9 — **moving in opposite directions**. Verified myself off the
cells. Recorded beside the old pair rather than replacing it: different grid, different data state,
and both remain ≥ 0 by construction so neither carries a t.

## Smaller items applied

- `bbArm.lastGW` was dead; deleted. `firstGW` was dead and now has a reader (the entry-week count).
- `TestTheBenchBoostControlPlanPlacesOnlyTheBoost` cited an overrun walk-back branch the shipped grid
  never reaches (`start+6 ≤ 32`). A `start = 33` case now exercises it.
- The `AGENTS.md` bullet's first use of `consult` and `pickXI` is glossed.
- The provenance sidecar stamps only `FPL_SWEEP_SEASONS` from the environment, so "`FPL_MAGNITUDE`
  unset, repairs on" is the operator's record rather than a stamped fact. Said so in the finding.

## Declined, and why

- **The anchored-double arm** — finding 1. The right next run; wrong to bolt onto a pre-registered
  two-stage design after seeing the numbers.
- **An equality test pinning `chipBarBenchBoost` to `config.DefaultBenchBoostBar`** — finding 9. Real,
  cheap, and out of scope for a branch that changes no production code.
- **Dropping the +9.47 level from `AGENTS.md` entirely**, as the statistics reviewer preferred. Kept,
  because deleting it would leave the "not a case for turning the lever on" claim without its
  referent — the file's own rule that a deleted figure still owes what made it meaningful. It now
  carries the mechanical label and the opportunity-cost sentence instead.
- **Rewording the bullet's opening clause away from "resolves"**, as proposed. Partly taken: the
  opening now reads "a bench-boost PLACEMENT contrast is measurable", which is the accurate claim,
  and the timing confound is the first caveat rather than the fourth.

## What could not be checked on this harness

- **Whether the rule reads state or merely fires late.** Unresolved by construction here; needs the
  anchored arm. Not "unmeasurable" — the comparator exists and is one 11-minute block away.
- **The size of the entry-week inflation in the ceiling's denominator.** The per-week gain vector is
  not banked, so it can be re-measured but not re-derived.
- **Whether the floor argument's direction is right.** A flat bench plausibly flattens the gain
  profile, but the autosub credit a stronger bench would earn runs the other way and neither is
  sized. Labelled a mechanism argument in both records.
- **The `WeeklyXI = true` corner.** Untested rather than equivalent.

## Suite

`go build ./... && go vet ./... && go test ./...` is green except
`TestReviewCoversTheCurrentCode`, which this record answers.
