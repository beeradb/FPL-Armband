# What the exposed returns do to the conversion fit

## What was reviewed

Branch `measure-the-blank-xa-leak-into-the-conversion-fit`, off `460d3ce`. A two-phase brief:
**phase 1** measure how far `Season.conversionFit`'s fitted per-position-season conversion scale
moves if "exposed returns" leave the fit — a realised goal or assist on a row whose season records
that channel and whose own `XG`/`XA` is zero; **phase 2** implement the exclusion, only if phase 1
warranted it.

**Phase 1 ran. Phase 2 was declined.** Nothing shipped changed. Three files:

- `internal/backtest/season.go` — `calibrateConversion` split so the fit is `conversionFit(exposedReturns)`,
  with the shipped path passing `countExposedReturns`. Verified bit-for-bit identical: 33 fitted
  scales over all 8 cached seasons at 17 significant digits, before and after, `diff` clean.
- `internal/backtest/conversionfit_diag_test.go` — new DIAG diagnostic, ~2 s, no cells replayed.
- `internal/backtest/conversionscale_test.go` — one new pin, `TestTheShippedFitCountsTheExposedReturns`.

No points claim is made anywhere and no detection threshold applies: nothing was replayed.

## Reviewers

Triage says a change touching `internal/backtest` owes **fpl-stats-review** and
**fpl-findings-audit**; a refactor asserting byte-identical output owes **fpl-code-review**, pointed
at the differential check rather than the diff. All three ran, plus the reviewer that covers
writes to the research store.

Skipped with reason: **fpl-security-review** (no client, agent or config-persistence surface),
**fpl-run-review** (no live run), **fpl-season-maintenance** (none of the four hand-maintained lists),
**fpl-docs-accuracy** (no `docs/` change — though see *declined*, item 3).

## The invariant that beat the reviewers

Per the gate's own "invariants beat reviewers" rule, the refactor's claim was tested by mutation
rather than by reading. Flipping `calibrateConversion` to `dropExposedReturns` and running the
package failed **exactly one** test — `TestTheConversionScaleFollowsTheXGRepair` — whose message
blames the xG repair rather than the arm.

⚠️ **`TestTheFittedScaleZeroesThePositionMeanAttackingResidual` did not catch it, and it is the test
that looks as though it must.** `aCalibratableSeason` gives every outfield profile a non-zero xA and
the keeper xA 0.05, so the shared fixture **contains no exposed row at all** and both arms are
byte-identical on it. That is "a byte-identical result is not a tie" occurring inside a test fixture.
`TestTheShippedFitCountsTheExposedReturns` closes it, with its vacuity guard first, and was confirmed
to fail under the flip with a message naming the real cause. **That pin was owed before this branch
and did not exist.**

The two diagnostic assertions were mutation-checked the same way: loosening both tolerances produced
36 failures = 18 fitted cells × 2, so neither is vacuous.

## Findings applied

### From fpl-code-review

1. **The diagnostic mutated a process-global, contractually read-only `*Season`.** `harness_test.go`
   says a diagnostic that edits a cached season "would silently change what every later test in the
   process measures". The restore was exact, but one `t.Fatalf` from leaving every later test on the
   dropped scale. **Removed entirely** rather than made exception-safe: the alternative scale now
   rides on a per-player struct copy and the season is never written. `applyConversion` was reverted
   with it, shrinking the production diff.
2. **The header imported the sibling's pre-repair 868/924/327 counts.** Those are its *ungated*
   population; inside the fit the goal channel is 0 in **both** data states, forced by
   `underlyingCoverage`. Importing them told the reader to expect a movement this table can never
   produce. Removed, with the reason recorded.
3. **The in-sample-identity claim was false pre-repair**, where the cause is missing coverage rather
   than the keeper floor. The assertion is now gated on `fullyCovered` as well, and the prose names
   both conditions.
4. **The identity was asserted on the assist channel only** while `moved` covers both; **and** it was
   read off four printed decimals, which permits 0.19 points of un-cancelled residual over 3,745
   defender appearances. Now asserted on the signed **total**, over both channels.
5. **`%moved` used total residual as its denominator**, which the clean-sheet channel dominates for a
   defender and which does not exist for a forward. Switched to the attacking-only residual.
6. `posLabel` was a sixth copy of the position table and disagreed with a sibling on element_type 5.
   Replaced with the existing `posShort`. Unused tally fields dropped.

### From fpl-stats-review and the research-store reviewer

7. ⚠️ **A fabricated retraction, and the most serious finding of the review.** The note recorded
   "the residual-mass share is 0.90-2.06%, **not ~0.3%**", calling the 0.3% a brief's invention "not
   produced by any run". **Both halves were false.** They are different quantities, both correct:
   0.3% is the *differential* exposure (282 pts) over the *six-season* mass (~82,300); 0.90-2.06% is
   the *whole* exposed population against *that season's* mass. They reconcile — `degen_pts` sums to
   1107 and 1107/82,324 = 1.34%. I reproduced the arithmetic before withdrawing the retraction.
8. **`132-264 points` was wrong; the floor is 129.** `%degen` and `degen_pts` have their minima on
   **different rows**, so the two ranges are not co-indexed. Fixed in the Go comment before commit,
   with the co-indexing trap recorded.
9. **The keeper claim was contradicted by the table above it** — GKP 2024-25 at +0.0369 is smaller
   than 2020-21 FWD at 0.0483. Now "seventeen of the eighteen".
10. **"Independent evidence for the instrument-mismatch reading" was wrong on both words.** The
    `zero_share` numerator *is* the Fisher OR's own exposed population, and the ordering follows
    mechanically from publication conventions, so it would appear under either hypothesis. Withdrawn;
    what survives is that the confound is demonstrated and unquantified.
11. **`13.5-26.6%` named a denominator the arithmetic does not use** (the excess over 1, not the
    ratio), quoted the extreme cell as "three quarters" (it is 73-87%), and called the scale "~2.1"
    when it runs 1.64-2.13. All corrected. **And the inference after it — "so three quarters comes
    from rows that DO carry xA and under-predict" — is forbidden by this branch's own finding 4**:
    the 0.01-0.05 band is the same phenomenon across a rounding boundary, so 13.5-26.6% is a
    **floor**, and the attribution is untested.
12. **`reach` collided** with the sibling diagnostic's crosswalk `reach`. Renamed `zero_share`.
13. **Function misattribution.** I wrote that `bestSwap` enforces same-position; it does not — it
    delegates to `analysis.RankSwaps`. Caught by verifying my own claim rather than by the reviewer
    who agreed with it. Now cites `RankSwaps`, `RankPairs`, `diffSquads` and the structural
    `squadQuota` argument.
14. **Scope limit recorded, unprompted by me:** the display-threshold argument does **not** reach
    `XGC > 0`. At 60+ minutes — the only rows FPL pays a clean sheet on — rows with `xgc == 0`
    number 2/0/6 a season, so quantization does not bite there.

### The finding that outranks the task

15. ⚠️ **There are two conversion scales and the other one is live on `Score`.**
    `Engine.calibrateExpectedStats` builds `e.xScale[pos]` through the same `CalibrationRatio` over
    ungated season-to-date element totals, and `scaleFor` is read by `baseXP90`
    (`metrics.go:1985-1986`) and `fixtureSensitiveAt` (`:2382-2383`), both multiplying `XA90` by
    `sc.Assists`. Verified in source. **The exposed-return construction exists on the scoring path,
    and its exposure has never been sized.** The population differs — the engine fits per-element
    season aggregates, so an exposed element is a player whose whole season xA is zero. Recorded in
    the diagnostic's header and in the research store; **not measured here**, and no number from this
    branch may be carried to it.

## Declined

1. **Phase 2 itself.** Not warranted, on two independent grounds. (a) Fitting on a subset and scoring
   on everything breaks the in-sample identity `xpoints.go` rests the instrument's reading on —
   measured at 0.000000 shipped in all 18 fitted cells and +0.0088 to +0.0483 per appearance under
   the drop arm in all 18. (b) The population is selected by a **two-decimal display threshold**;
   `zero_share` is 15.0-17.4% in FPL-fed seasons, so the repair acts on a minority of its own
   phenomenon whether or not the mechanism story holds. The record already argued against it at
   `underlyingCoverage`, and this run reproduces its anchor (2.077 → 1.932) exactly.
2. **Merging the paired research-store branch for the change that landed at `460d3ce`**, which is
   correctly identified as what would make this session's queue-item edit legal, and which holds a
   competing fill of the same section. Outside what was asked; recorded in that item's own `## State`
   with an instruction not to resolve either side with "ours".
3. **Editing `CLAUDE.md`, `internal/analysis/xpoints.go` and `docs/model.md`.** The findings audit
   proposed specific replacements for five stale or false claims in them — the 0.3% at `CLAUDE.md:953`,
   the Fisher OR's standing, "Goals: closed" resting on the ungated population, the cross-position
   premise at `xpoints.go:179` and `:333`, and "because they win most of the penalties" in
   `docs/model.md` and `metrics.go`. **Editing `CLAUDE.md` was forbidden by the brief**, and the
   others are a separate change to shipped comments rather than part of a measurement. All are
   reported upward and queued.
   ⚠️ **This leaves a known cost in the tree**: the diagnostic prints the correction to
   `xpoints.go:179` while that file still asserts the false version, so the repo now holds one claim
   twice with the wrong copy in the file people read. Neither duplication scan keys on prose.

## What could not be checked

- **Any points consequence.** Nothing was replayed. `Player.Conversion` is read at exactly one
  non-test site, so this fit cannot move `hold_points` or `policy_points` at shipped config — which
  also means a change here **would** supersede every banked `hold_xpoints`/`policy_xpoints` figure and
  the 0.6402 recovered fraction. None of that is a threshold question.
- **Two variants of the repair remain open**, neither built nor measured: dropping from the fit *and*
  the scoring, which restores the identity exactly at the cost of `xPointsOver` no longer being a
  window total; and gating the assist channel on `XA > 0`. The crux of the second — `XGC == 0` is
  missing data while `XA == 0` on a won penalty is a correct zero — is untested.
- **The engine-side `xScale` exposure** (finding 15). Sizing it is another archive count of the same
  shape, not a sweep.
- **The band edge 0.05 is asserted**, five times the archive's quantum. The argument needs the bucket
  populated, not the edge anywhere in particular.
- **Whether the pins are proofs.** They are tripwires. `TestTheShippedFitCountsTheExposedReturns`
  pins the arm on one fixture with one exposed row; the identity assertions hold only where
  `CalibrationRatio` returned the plain ratio, and a clamped cell would fail them rather than be
  exempted.
