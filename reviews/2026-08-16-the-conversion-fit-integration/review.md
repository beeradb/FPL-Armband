# The conversion-fit measurement, integrated — and a scoring-path scale nobody had sized

## What was reviewed

`integrate-the-conversion-fit-measurement`, merging `measure-the-blank-xa-leak-into-the-conversion-fit`
(`07eb42b`) onto `main`, plus the `CLAUDE.md` entries written on top. The source branch carries its
own record at `reviews/2026-08-16-the-exposed-return-scale-shift/`, including its own reviewers'
findings and three self-corrections.

## The task was structured measure-first, and the measurement refused the fix

This was dispatched as **phase 1 measure, phase 2 fix only if warranted**. Phase 1 ran; **phase 2
was declined**, and both reasons are better than the reasoning that produced the brief — which was
mine.

**The scale shift, which is the number nobody had.** An exposed row adds a realised return to
`CalibrationRatio`'s numerator and nothing to its denominator, inflating the fitted scale, and a
higher scale subtracts more from every row that *does* carry underlying. Measured: assists shift
**−2.7% to −14.0%** across the 18 fitted DEF/MID/FWD cells, always downward, moving **0.97-4.25%**
of each fitted position-season's attacking residual. Goals inert in both data states — and that is
**forced by the existing coverage gate rather than measured**, which is the sort of distinction this
record exists to keep.

**Why the fix was refused:**

1. **It breaks the in-sample identity.** Signed attacking residual is exactly `0.000000` shipped in
   all 18 fitted cells, and +0.0088 to +0.0483 per appearance under the drop arm in **all 18**. The
   proposed repair destroys the property the instrument is built on.
2. **`XA == 0` is a two-decimal DISPLAY threshold, not a real zero.** It captures **15.0-17.4%** of
   near-zero-expectation assists in FPL-fed seasons (46.4-52.1% Understat-fed), so a repair defined
   on `== 0` acts on a **minority of its own phenomenon**. ⚠️ **This objection is fatal to the
   SHAPE of the fix, not merely to its size** — and I did not check it before proposing it.

## The finding that outranks the task, verified here rather than taken on report

**There are two conversion scales built the same way, and the other is live on `Score`.** I had
stated — to the user and in the brief — that this was instrumentation and could not move replayed
points. That holds for `Player.Conversion` only, read at exactly one non-test site (`xPointsOf`).

Verified against source in this worktree:

- `Engine.calibrateExpectedStats` (`internal/analysis/metrics.go:1258`) builds `e.xScale` at
  `:1274-1279` through the **same `CalibrationRatio`** (`:1287`), same `minCalibrationSample` 20.0
  floor, same `[0.5, 3.0]` clamp.
- `e.scaleFor(pos)` (`:1296`) has three non-test call sites: `:1551` (reporting), **`baseXP90` at
  `:1984-1986`**, and **`fixtureSensitiveAt` at `:2380-2383`**.
- `baseXP90` **is** `Score`. I read the call site and confirmed
  `xp += m.XA90 * sc.Assists * assistPoints`.
- `calibrateExpectedStats` applies **no coverage gate and no exposure gate** — grepped and confirmed
  zero gating terms in its body.

⚠️ **Its exposure has never been sized, and no figure from the instrument side transports.** The
engine fits over per-element **season-to-date aggregates**, so an exposed element there is a player
whose *whole season* xA is zero — rarer than a zero-xA gameweek row, and measured by nothing in the
tree. Sizing it is another archive count of the same shape, not a sweep.

## Applied

1. The scale-shift measurement, with both refusal grounds, into the archive section.
2. The two-scales correction into the xPoints entry — including that my "cannot move replayed
   points" was true of one scale and not the other.
3. **"Goals: closed" narrowed.** It rests on the population the existing coverage gate admits; it is
   not a statement that the goals channel cannot be exposed. Flagged by the source branch as one of
   five stale or false claims it had queued for me.

## Declined

- **Sizing the `e.xScale` exposure now.** It is a different population and a different count, and
  folding it into an integration commit would hide it. It is the single most valuable follow-up in
  this batch and belongs in the queue with its own brief.
- **Editing `xpoints.go:179`'s cross-position premise.** The source branch declined to edit a
  shipped comment as part of a measurement, which is right, and named the cost: the repo holds that
  claim twice with the wrong copy in the file people read. Recorded rather than absorbed.

## The source branch's own self-corrections, recorded rather than dropped

Its report to me contained three claims it later withdrew, and the first is the one worth keeping
visible:

- **It fabricated a retraction.** It recorded the ~0.3% mass share as "wrong" and "not produced by
  any run". The figure is **correct** — a different quantity (differential exposure over six-season
  mass), reconciling at 1107/82,324 = 1.34%. Recording a correct figure as withdrawn is worse than
  leaving it alone, because a retraction is the one edit nobody re-checks.
- A keeper claim contradicted its own table (+0.0369 < 0.0483).
- "Independent evidence for the instrument mismatch" was wrong on both words — the statistic
  re-describes the Fisher odds ratio's own numerator.

## What could not be checked

- **The `e.xScale` exposure size.** Unmeasured, and no existing figure bounds it.
- **Whether the two-decimal display threshold has consequences elsewhere.** `XA == 0` is used as a
  proxy for "no underlying" in more than one place; only the conversion fit was examined.
- **No points claim anywhere in this batch.** Nothing was replayed.
