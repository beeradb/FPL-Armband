# The xGC proration's cancellation, re-scored on the input the chain actually runs on

Branch `xgc-transport-tercile-cancellation`, off `origin/main` at `cf4379a`. Commits `754d4c5`,
`826e7a8`, and the corrections that followed the code review.

## What was reviewed

`xgcrepair.go` recorded that the xGC minutes-proration's two errors — over-crediting a withdrawn
starter, under-crediting a late substitute — "largely cancel at player-season level", measured as
season `XGC90` reconstructed over actual running **0.983-1.014 across substitution terciles**, within
±1.7%. That figure is **FPL-fed**. The chain only ever runs on Understat-fed seasons, and the 2026-08-13
transport run measured dispersion and ordering rather than the tercile structure — so the claim
covered a population its evidence did not reach, and the file said so.

The change adds a proration-exposure tercile column to `scoreXGCArm`, does the inference in
`stats/xgc_tercile_transport.R`, and banks both under `stats/cells/xgc-transport/`. **No replay**: the
arm-B inputs were already banked. Diagnostic only — `xgcScale` untouched at 1.0, the reconstruction
untouched, no shipped code path changed.

Reviewers, per the triage row for `internal/backtest` and `stats/*.R`:

- **fpl-stats-review** — the recentring, the estimator, the cluster choice, the strength of the verdict.
- **fpl-code-review** — dispatched beyond the triage row because the column is new code with a cut,
  a CSV contract and a liveness guard in it. This was the pass that found the load-bearing defect.
- **fpl-findings-audit** — the record the block writes. It returned eleven findings, all of which
  reproduced when I checked them, and it is the pass that caught the two *arguments* that were wrong
  rather than the numbers.

## The invariant, written before the reviewers were read

The skill's first instruction is to ask what the change must **not** move. Two things:

- **The ever-present control.** `everN`, `everRatio` and `everMAE` are pinned to the recorded 1.0088
  at 3.9% by the positive control already in the test. After splitting the exposure predicate away
  from the ever-present one, all four seasons' ever-present columns are **byte-identical** to the
  pre-change baseline (2914/5184/5172/5261 rows; 1.0110/4.6, 1.0100/3.2, 1.0094/3.0, 1.0054/5.2), as
  are `ratio`, `corr`, `MAE` and `spearman`. Confinement holds.
- **That the two predicates stay distinct.** `TestTheProrationExposureCutIsNotTheEverPresentCut` is
  new and counts the player-gameweeks where they disagree: **213 / 117 / 48 / 54**. It is a *liveness*
  check — it fails at zero — because zero would mean the tercile figures were measured on a
  distinction that does not exist. This is the test that would have caught the defect below for free.

## The finding

**Two results, and neither is "the cancellation transports".**

**The FPL-fed band reproduces.** Under a named cut (share of scored minutes not prorated exactly) and
a named estimator (ratio of totals over per-player season `XGC90`), arm A spans **0.9857 to 1.0100**
on the three full seasons — inside 0.983-1.014, max deviation from 1 of 1.43%, so within ±1.7% too.
Mean-of-ratios spans 0.9897 to 1.0129, also inside. 2022-23 sits outside at 1.0231 and is the
GW16-38 season the claim's own "all three seasons" excludes.

**The transport contrast does not resolve.** Paired UST−FPL on recentred ratios, season-clustered on
four seasons, `thresh` = t_crit(3) × SE: low −0.0036 (0.0052), mid +0.0010 (0.0092), high +0.0025
(0.0093), signed high−low **+0.0061 (0.0116)**, spread +0.0043 (0.0166). Holm ≥ 0.579 on all five,
and nothing clears on the three full seasons either.

⚠️ **It is a tie that LEANS.** The tercile cut is not the sharpest form. The continuous version — OLS
of `log(rec90_ust/rec90_fpl)` on exposure, club-clustered within season then season-clustered across —
reads slopes +0.0256 / +0.0232 / +0.0008 / −0.0017, pooled **+0.0120, SE 0.0072, t 1.66**, positive in
three of four seasons. Times the observed high−low exposure gap of 0.386 it implies +0.0046 against
the tercile contrast's +0.0061, so the two forms agree in sign and size. The point estimate runs
toward the transported arm over-crediting high-exposure players by about half a percentage point, and
this instrument cannot resolve an effect that size.

**Power.** The 80%-power MDE on signed high−low is **0.0151**, against a recorded band **0.031** wide.
So the comparison could have seen the cancellation break outright; it could not have seen it degrade
by half. `thresh` (p = 0.05) and `MDE80` are printed as separate columns throughout and are not
interchanged anywhere.

**Two liveness checks, because the obvious one cannot fire against this reading.** Raw movement is
570 of 570 player-seasons (494 by more than 1%) — but a pure per-season **scalar** rescale would pass
that at 100% while forcing every recentred contrast to zero *by construction*, since the recentring
divides by the whole-population ratio and a scalar cancels. The check with power is the within-season
CV of `rec90_ust/rec90_fpl`, zero under a rescale: **4.61 / 4.33 / 4.36 / 4.76 percent**.

## What was applied

Every figure below was **re-derived here before being applied**, not taken from a report.

From **fpl-code-review** (the load-bearing one):

- **The exposure cut was substantially a double-gameweek cut.** The first version reused the
  ever-present predicate (`Minutes == 90 && n == 1`) for both `everN` and the exposure share, so a
  player who went 90+90 in a double was booked as fully exposed even though the proration hands him
  his club's whole two-match xGA **exactly**. Corrected to `Minutes == 90*n`. Verified: it moves 22%
  of player-seasons between terciles, and 2022-23's median exposure falls from **0.256 — the highest
  of the four — to 0.124**, in line with 0.120-0.148 elsewhere.
- **That defect had manufactured a finding, now retracted.** The previous commit built a labelled
  hypothesis on 2022-23 being "enriched in double-gameweek non-exactness". It was an artefact of the
  predicate and is gone; the season is still held apart, for its n and its level only.
- **It had also manufactured the opposite verdict.** On the contaminated cut arm A spanned
  0.9915-1.0237 and the recorded band was declared **withdrawn**. On the corrected cut it spans
  0.9857-1.0100 and the band **reproduces**. The withdrawal is itself withdrawn, marked in place.
- R's season labels were literals (`"four seasons"`) over a directory-derived season set, so a
  partial input printed "four seasons" over three. Now derived.
- R re-implemented the bucket estimators with nothing comparing them to Go's. Go now writes an
  `xgc-tercile-<season>-buckets.csv` sidecar and R `stop`s on a mismatch — agreement **5.78e-10** over
  24 cells.
- The CSV writer deferred and discarded `Flush` and `Close` errors, so a full disk produced a
  truncated file and a passing test. Both checked now.
- `panic` → `t.Fatalf` in `xgcTercileLiveness` (a panic aborts the binary, so no sibling test runs).
- CSV precision 6 dp → 9 dp; at `XGC90` ≈ 1.3 the file was quantising at ~8e-7 relative, three orders
  above the 1e-9 liveness threshold R applies to it.
- The tie-handling comment claimed the low boundary can fall inside the run of exposure-zero players.
  Counted: 3/6/17/14 zeros against boundaries at 40/51/47/50, so it does not. Reworded to the measured
  counts; the element-id tiebreak is kept because `sort.Slice` is unstable.

From **fpl-stats-review**:

- **"and it is not close" withdrawn**, replaced by the continuous-slope reading above. The largest
  paired |t| is 1.59 and the sharpest form is t 1.66 — a tie that leans, not a comfortable one.
- **The recentring's zero-sum constraint stated and asserted.** The three recentred ratios satisfy an
  act90-weighted mean of exactly 1 (holds to 2.2e-16), so the buckets are **two** degrees of freedom,
  Holm over six rows is conservative rather than wrong, and signed high−low is the statistic to read.
- **The scalar-rescale liveness added** (above). This was the sharpest catch on the statistics side:
  the guard as shipped could not fire against the null it was guarding.
- "mean-of-ratios runs 0.2-1.2% higher, the usual direction" corrected to **−0.08% to +1.10%**, with
  the real argument: the gap **increases with exposure in 7 of 8 season-arms**, so that estimator
  *manufactures* tercile structure. That is why ratio-of-totals is the headline.
- The paired-spread defence was wrong. The UST arm's bootstrap SEs run 1.5-2× the FPL arm's, so under
  a null of identical structure `E[spread_UST]` exceeds `E[spread_FPL]`: the statistic is biased
  **toward** finding a transport failure and does not find one. "The bias is common to both arms" is
  withdrawn.
- The whole-population ratios used as the recentring divisor are rate-space over the 900+ minute
  keeper-and-defender population, while the pooled figures recorded above are level-space over all
  appearances — 0.8pp apart on 2022-23. "The same finding" softened to "the same level, on a narrower
  population and a different estimator".
- **The bounded-stake sentence added**, which is what closes the line: the whole ±1.7% band is worth
  ~0.015 pts/90 through the clean sheet, the MDE is about the size of the entire phenomenon, so even
  total failure is two orders below replay resolution. **Do not re-run this.**
- The post-hoc high-bucket reading kept but given its three further disqualifications: non-monotone
  (a step at the top bucket, not the gradient a substitution bias must produce), nearly the arithmetic
  complement of the other two under the zero-sum constraint, and an SE from three numbers on df 2.
  Its four-season counterpart is −0.0031, t −0.60.

From **fpl-findings-audit**, all eleven verified here before applying:

- **The bounded-stake argument was wrong three ways, and it is the paragraph that closes the line.**
  The "about 0.015 pts/90" it leaned on is **unsourced** — it entered in prose with no producing run —
  and it contradicts this same file's own conversion 190 lines up, where a 1% move at `XGC90` 1.35 is
  0.014 pts/90, making the ±1.7% band ~0.023 and the 3.1% width ~0.042. Verified:
  `4·(e^-1.35 − e^-1.3635) = 0.0139`. Worse, that figure was computed for a **uniform** scaling, which
  this record holds is *not* an ordering error, while the tercile effect is within-position — the case
  it holds *is* one. And "two orders below anything the replay resolves" set a per-player pts/90
  displacement against a per-season squad threshold. Replaced with the **canary** bound the record's
  own rule names: halving every clean sheet costs −21.6 against a threshold of 28, and a 1.5-3% move
  in `XGC90` is a 2-4% move in clean-sheet probability — under a fiftieth of it. Conclusion survives,
  derivation did not.
- **"nothing clears on the three full seasons either" is false.** `UST−FPL recentred low` reads −0.0020
  against a thresh of **0.0013**, |t| 6.57, raw p 0.0224. Holm 0.1121 so it carries nothing, but the
  sentence was wrong and it runs the same sign as the four-season low.
- **The half-band arithmetic was inverted.** Half of 0.031 is 0.0155, which is **above** the MDE of
  0.0151 — so a half-band degradation sits marginally *inside* 80% power. I had asserted the opposite.
- **"The tercile cut is not the sharpest form" no longer holds.** True at the contaminated cut (1.94
  against 1.59); now t 1.66 against **1.68**, effect/threshold 0.524 against 0.526. The second form
  buys corroboration, not power. Also flagged: "positive in three of four seasons" counts a +0.0008.
- **Season-set switching.** The band was quoted on three seasons and the lean on four, each time on
  the set that made it quietest. Both sets now quoted for both, including the FPL-fed signed high−low
  of −0.0122 (t −3.31) on three seasons — the largest signed structure in the run, and the statistic
  my own degrees-of-freedom note nominates as *the* one to read, which I had never quoted.
- **The FPL-fed gradient runs the wrong way for the mechanism**, which is the audit's most valuable
  catch. The recorded mechanism over-credits withdrawn starters, so the high-exposure bucket should
  sit **above** 1; it sits below the low bucket in 3 of 4 seasons (raw 2023-24 descends 1.0051 /
  0.9987 / 0.9857). Now recorded, with the two readings it cannot separate, and with the explicit
  instruction to read "the band reproduces" as a width reproducing rather than the cancellation claim
  being confirmed on its own mechanism.
- **A withdrawn justification survived verbatim in the R script** — "the bias is common to both arms
  and differences it", five lines above the code it justifies, while `xgcrepair.go` records it as
  withdrawn. Marked in place. ⚠️ Worth escalating: `internal/snapshot/retracted_test.go` does not scan
  `stats/*.R` at all, so no guard could have found it and my own grep caught figures but not prose.
- **A contaminated count survived, sourced to the banked rows.** The exposure-zero counts read
  3 / 6 / 17 / 14; recomputed off the banked CSVs they are **25 / 20 / 23 / 23**. The conclusion
  (no boundary falls inside a zero run) is unchanged — 25 < 40, 20 < 51, 23 < 47, 23 < 50.
- **"1.5-2×" for the SE ratio is 0.88 to 2.35**, median 1.69, with four of twelve buckets below 1.5
  and one below 1. The directional argument needs only the expectation and survives; the range did not.
- **18-71% and 22% are not reproducible from a checkout** — both need the contaminated cut and only
  the corrected exposure is banked. Now labelled as the correction's account of itself, with the
  checkable substitute named.
- **The cancellation bullet named the cut two ways in adjacent sentences.** One clause added, because
  that bullet is 220 lines before the block that explains the distinction.

## What was declined, and why

- **Fixing the lean.** The task scope, and the right call regardless: acting on a +0.0120 slope that
  does not clear its own threshold would be correcting a measured bias, which this record says has
  lost points five times. It is recorded as a direction, not a correction.
- **Renaming the cut to "substitution exposure" to match the claim's wording.** It is proration
  exposure and the file now says so in three places. The claim's wording is what is imprecise; the cut
  is right for the mechanism, since the proration's two errors are what the cancellation is about.
- **Re-deriving `everN` on the corrected predicate.** It is the pinned positive control for the whole
  transport finding and its recorded value is 1.0088. Two questions, two predicates, kept apart — and
  the new invariant test now fails if anyone re-merges them.
- **A run-id or timestamp column in the tercile CSV.** fpl-code-review is right that `list.files` over
  a fixed `/tmp` path can pool two builds under one label, and the sidecar count check only catches
  the mismatched case. Declined here because the banked copies under `stats/cells/xgc-transport/` are
  the artefact of record and they reproduce; noted as a real gap rather than fixed at the gate.
- **Adding entries to `TestRetractedFiguresAreNotQuotedAsCurrent`.** The two withdrawn claims
  (0.9915-1.0237, and "the band is withdrawn") never existed outside this branch, so nothing on
  `main` can quote them. Grepped the three source files for all fourteen contaminated-cut figures:
  the only survivor is the deliberate account of what the wrong predicate produced.
- **Editing `CLAUDE.md`.** Out of scope by instruction, and the verdict belongs beside the code it is
  about. ⚠️ The audit is right that this leaves the "tie that leans / do not re-run" verdict living
  only in a Go doc comment while `CLAUDE.md`'s xGC bullet still carries only the 2026-08-13 transport
  verdict. **Owed to whoever next edits the resident record**, not fixable here.
- **Extending `retracted_test.go` to scan `stats/*.R`.** This branch is the first instance of a
  withdrawn claim landing in an inference script, and the guard's surfaces are `CLAUDE.md`,
  `README.md`, `.claude/*.md`, Go sources and `docs/*.md` + `stats/*.md` — `.R` is not among them.
  Marked in place in the script instead. Declined here because widening the guard has its own failure
  modes (its `context`/`unless` word lists) and this branch is at the gate. **Queued.**
- **Deciding which of the two readings explains the wrong-way gradient.** It needs a minutes-band
  decomposition of exposure inside the 900+ population. That is a new measurement, and the canary
  bound says the whole question is worth under a point a season.

## What could not be checked on this harness

- **Whether the recorded 0.983-1.014 was measured on this cut and this estimator.** It has no
  producing test in the repository. The reproduction is "the same population to a good approximation",
  and the band is now cited with a cut and an estimator so the next reader does not have to guess.
- **Whether the lean is real.** t 1.66 on four season clusters. Six seasons cannot deliver the SE cut
  this would need, and the bounded-stake argument above says it would not be worth it if they could.
- **The points consequence.** Nothing here touches replayed points, and the record already closes
  that question: the reconstruction's price does not resolve on any grid, and 45% of its −34 is
  captaincy. The tercile structure is a fidelity statistic, not a points one.
