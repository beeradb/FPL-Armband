# The clean sheet's calibration has two mechanisms, and neither replaces the other

Branch `clean-sheet-mechanism-reconciliation`, off `origin/main` at `07bbf1a`.

## What was reviewed

A survey commissioned to find other instances of the failure class the clean-sheet work had just
exposed — **a calibration evaluated at a regressor the scoring path does not consume**. The survey's
sharpest finding was about the clean-sheet write-up itself, merged hours earlier: its *levels* are
right and its *mechanism* was wrong. Settling the mechanism then took three passes, and the account
in this record is the one that stands.

Reviewers:

- **fpl-stats-review**, run on the fable model, briefed on the mechanism rather than the symptom (the
  bias only bites through nonlinearity, so `E[f(X)] = f(E[X])` clears every linear term).
- **fpl-stats-review** and **fpl-findings-audit** again at the merge gate, over `stats/*.R` and the
  record. The stats pass verified every figure in this record against
  `stats/snapshots/2026-08-15-clean-sheet-2x2/FINDINGS.md`; the audit swept the tracked tree for
  claims the correction had not reached, and **found two source files still asserting withdrawn ones**
  — see *What was applied*.

## The finding

The sibling diagnostic's 1.281 has **two mechanisms, and neither replaces the other**. The account
that ships is `stats/snapshots/2026-08-15-clean-sheet-2x2/FINDINGS.md`; this is its summary.

**Cross-match convexity explains the gap BETWEEN the two regressors.** `exp()` is convex, so the
aggregate `E[exp(−x)]/exp(−x̄)` is larger the more dispersed `x` is — it needs no proxy, only that
`x` be more dispersed at the same mean. Realised match xGC (sd **0.848**) is far more dispersed than
`XGC90` (0.204-0.301), and on season-matched populations the mechanism predicts the observed ratio to
**0.3%** (1.2759 against 1.2799). ⚠️ **Quote that exact ratio, never `exp(σ²/2)` at "σ ≈ 0.70"**: σ
there is the sd of the *deviation* rather than of `x`, and the approximation is 8% high at the
realised dispersion though excellent at `XGC90`'s (1.0385 against 1.0386).

**A shot-level Jensen gap explains why `exp(−x)` over-predicts on realised `x` at all** —
`exp(−Σxᵢ)` against `Π(1−xᵢ) ≈ exp(−c·Σxᵢ)` with `c > 1` — named in `internal/analysis/xpoints.go`
and `stats/xg_provider_scale.py`.

⚠️ **Its size is not established here.** Solving `mean(exp(−c·x)) = observed rate` over the 2870
sibling rows returns **c = 1.2830**, but that fit is **exactly identified** — one free parameter
matched to one moment — so it reproduces that moment by construction and returns the same value for
every rival mechanism reproducing it. Its clustered interval is ≈**[1.19, 1.39]** (SE ≈ 0.050), a
second estimate of the same parameter sits in the same output at **1.2556**, and the pure-scale
family that gives `c` its meaning is **rejected on those rows by that same script** (LRT p 0.00075).
`xg_provider_scale.py`'s **1.27** is a different season on a different feed — 2015/16, on 175 of 380
shared fixtures, while FPL uses Opta which that script says it does not identify — against **1.3291**
season-matched. The wedge is real and the right sign and order; read the order, not the constant.

## Why it matters, given no level changed

The near-calibration is a **cancellation of two opposing wedges**, not a structural property of using
a smoothed input: the realised rate lands within 2.6% of `exp(−x̄)`, which is what a smooth-regressor
model computes, because the two wedges net to a product of **1.026** on the sibling realised-xGC rows
where both are visible.

⚠️ **Quote that product; never the wedges.** `c` is fitted so `E[exp(−c·x)]` equals the observed
rate, so the decomposition **telescopes** — the parts agree with the product by construction, and
their sizes are an artefact of where the identity was cut. The same rows split elsewhere give
**+32.7%/−22.6%** against **−33.8%/+55.1%**, for an identical product.

⚠️ **The fragility is in the MEAN, not the dispersion.** The whole dispersion channel is **1.0410**,
so annihilating `XGC90`'s dispersion moves calibration **4.0%** and its recorded season range spans
**2.5%**. Calibration goes as `exp((c−1)·x̄)`, so a **10% shift in the mean moves it 4.1%** — more
than removing all of the dispersion. The reconstructed-xGC seasons therefore enter by the **level**
channel. ⚠️ Arithmetic off the banked rows rather than a measurement: it assumes `c` is
population-independent, which nobody has checked.

`CLAUDE.md` closes the points question on this term as unmeasurable, by a canary, and forbids a
re-run at the refitted constants. Any *future* arm on it must be designed off this mechanism, and off
the level channel rather than the dispersion one.

## What was applied

- The mechanism paragraph rewritten to the two-mechanism account in `FINDINGS.md`, `CLAUDE.md`,
  `internal/analysis/sweep.go`, `internal/backtest/cleansheet_calibration_test.go` and
  `stats/cs_calibration.R`.
- **A `c_implied` block added to `stats/cs_calibration.R`**, so the discriminator is reproducible
  rather than living in a review: it prints `c` for whatever dump it is given — **~1.28 identifies a
  realised-match-xGC regressor, ~1.00 the model's smoothed one**. Verified on both banked dumps
  (1.2830 / 1.0030). It fails rather than notes when there is no root, prints its exactly-identified
  caveat with every value, and gates the regressor gloss on dumps where it means something.
- **The sibling's realised-xGC dump banked** (`cs_sibling_realised_xgc_rows.csv`, 2870 rows), which
  was an outstanding item and is what made the re-derivation possible.
- The sentence *"the near-zero is two opposing biases roughly cancelling"* re-issued as **"the two
  wedges"**. It maps badly onto the two *selection* biases, which both run the same way, and
  correctly onto the two wedges. **Say "the two wedges", never "two opposing biases".**

## What was declined, and why

- **Changing any level.** 1.281 is still refuted as a model property; 1.052 / 1.004 are still direct
  measurements at the consumed regressor. Only the explanation moved.
- **Building the floor-div calibration** the survey flagged as an unbuilt trap (`E[floor(X/2)]` for
  the concede deduction, `E[floor(X/3)]` for saves). It has no calibration today, which is currently
  a virtue — no mis-paired figure exists. Recorded as a trap to sign rather than a gap to fill.
- **Acting on the defcon dose shift.** The replay consumes a `DefCon90` blended toward a zero prior,
  ~0.70× realised, so the coupling operates below its calibrated reference point. `CLAUDE.md` already
  records the substance and the coupling ships on mechanism, so nothing decision-bearing rests on it.
- **A guard for the class.** Scoped and costed rather than built: an AST scan flagging a nonlinear
  transform applied to an archive-row field would have caught this, the template exists in
  `cmd/priorblend/gatecallers_test.go`, and it is roughly a day including the allowlist. Left as a
  decision because it catches a *mismatched regressor* and not a mismatched *population*, and the
  `XGC` / `XGC90` naming distinction it relies on is luck rather than convention.
- **Extending `TestRetractedFiguresAreNotQuotedAsCurrent`** with entries for `1.2830` and `33.8`.
  This is the right mechanism and it is **owed**: the correction of this line reached five files, two
  of which — `internal/analysis/sweep.go` and `internal/backtest/cleansheet_calibration_test.go` —
  were missed by the commit that made it and went on stating withdrawn claims as current fact in a
  comment justifying shipped behaviour. A guard would have caught both for free. Declined **here**
  only because tuning its `context`/`unless` word lists is a change with its own failure modes, and
  this branch is at the merge gate. ⚠️ Note the guard's reach is narrower than it looks: `stats/*.md`
  is a **non-recursive** glob, so `stats/snapshots/*/FINDINGS.md` is out of scope, `.R` files are not
  scanned at all, and neither is `reviews/`. Queued in the research store.

## What could not be checked on this harness

- **Whether the two wedges cancel on any population but this one.** They are measured on the 2870
  sibling rows; the cancellation is a fact about that distribution of match xGC, not a theorem.
- **`modelrepro_test.go`'s use of the same realised regressor.** Likely deliberate — it pins
  determinism rather than measuring the model — but its output must not be read as a model property,
  and it carries no `Fixtures != 1` guard. Not investigated here.
