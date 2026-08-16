# Review: the points-vs-rank record edits

**Commit range reviewed:** `07afdae..e4077bb` — that is `c8372cf` (the wiring rule, the triage
rule, the budget comment, the `TODO.md` reclassifications) plus `e4077bb`, which is the change this
review is about.

## What changed and why

The recurring question — *points decide rank, so is maximising expected points not the same as
maximising rank, and what would a rank-gain architecture look like?* — was put to the statistics
reviewer. Its answer was written into `CLAUDE.md`'s closed-lines bullet and into a new dated
section of `docs/rank-objective-handoff.md`. The findings audit then reviewed **that write-up**,
which is the pass that produced most of what is recorded below.

**No sweep was run at any point.** Everything here is reading, plus arithmetic on anchors already
in the record.

## Reviewers

| reviewer | ran? | why |
|---|---|---|
| **fpl-stats-review** | yes, twice | once to answer the question; once, still open, for sign-off on replacement wording for the bias-direction sentence |
| **fpl-findings-audit** | yes | the triage row for `CLAUDE.md` and `docs/` — the record only |
| fpl-code-review | no | no Go changed. `go build ./...` and `go vet ./...` clean |
| fpl-security-review | no | nothing touching `internal/agent`, `internal/fpl`, config persistence or the cache |
| fpl-run-review | no | no live run, no config written |
| fpl-season-maintenance | no | none of the four hand-maintained lists touched |

**What quantity must this change not move**, asked before dispatching anyone, per the skill's first
section: `CLAUDE.md`'s size against its 56 KB budget, and every cross-link resolving.
`go test ./internal/snapshot/` covers both and is green apart from the gate this record closes.
The file is **56,271 B against 57,344**.

## Findings, ranked by how misleading the state was

1. **The power argument was generalised beyond its measurand.** The draft appended "the binding
   reason is power, not 'it changes the picks'" to the do-not-build bullet. But the power arithmetic
   is about `P(top 10k)`, a rare binary event; the bullet covers a rank-maximising policy in
   general, whose measurand is percentile — continuous, monotone in points, ordinary power. It also
   demoted the recorded reason in favour of a different objection. **Applied:** the flag now scopes
   power to the threshold target and reinstates the recorded reason.
2. **`z = +1 to +2.3` had no run behind it** and contradicted the record's own span of −66 to +564
   points across 24 cells (`z` = −0.45 to +3.84 at the `σ_F` = 147 anchor). Worse, a `z` in the
   `μ_D/σ_D` sense is **not derivable from this record at all**, because `σ_D` needs the joint
   covariance matrix the same section lists as unbuilt. **Applied:** replaced with the recorded
   span, plus an explicit warning that every `z` in these documents is margin over `σ_F` and ignores
   our own variance. The per-cell count above and below the median is **not recorded**, and is now
   marked as such rather than asserted.
3. **One quantity, two figures, written the same day** — "1-5%" in `CLAUDE.md` against "0.1-5%" in
   the note, with the resident file carrying the narrower version. **Applied:** both read 0.1 to 5%,
   and the expected count is now a bracket (0.005 to 0.25 across the grid) rather than "near 0.2".
4. **The `CLAUDE.md` bullet carried edit narrative and a derivation**, both of which
   `internal/snapshot/notes_test.go` lists as never resident at any size, and it consumed roughly
   half the file's remaining headroom. **Applied:** trimmed to the two verdicts plus the link.
5. **Restated robustness figures lost their provenance.** They are recorded in the harness note as
   exploratory Python rather than the vetted R path — direction recorded, coefficients indicative —
   and a section headed "What it confirms" restating them bare reads as corroboration.
   **Applied:** flag appended, noting item 3 of "What is still left" is still open.
6. **"The `Φ(μ_D/σ_D)` form and the S-curve are the same fact" is false.** The S-curve is the payoff
   in a deterministic score; the variance term exists only once our own total is treated as random
   against a concave `Φ`. Calling them identical licenses treating an imported result as already
   established here. **Applied.**
7. **The disciplines paragraph asserted imported results with no label**, in a document whose other
   imported result is labelled. **Applied:** the paragraph now opens by saying it is a reading list
   with verdicts on relevance, not a result.
8. **`(c)`'s "same argmax" is right on the aggregate definition and will be "refuted" by the next
   reader**, because the per-player form `Σ μ_p(1 − EO_p)` is not a constant subtraction and does
   reorder. **Applied** as an inserted qualifier.
9. **`z_T ≈ 3.1` sits exactly on the outer edge** of the range over which the record checks the
   field against a Gaussian. **Applied.**
10. **`docs/README.md` advertised a 48 KB budget** against a 56 KB test constant. **Applied.**

## The bias-direction correction: signed off with amendments, and applied

Held back from the first commit and sent to `fpl-stats-review`, which returned **sign off with
amendments**. The calculus was confirmed — `d ln s/dσ_F = (a² − σ_F²)/σ_F³`, turning at
`|z_F| = 1`, with both regimes verified numerically at the record's own scale. Eight amendments
narrowed it, and **one is structural enough that the correction now claims something different
from what was drafted**:

- **No figure in this record was produced by the biased procedure.** The field mean comes from
  ownership marginals, which are linear and therefore independence-free, and `σ_F` = 147 is an
  **external anchor** from FPL's published percentile structure. The independent-sampling bias
  governs a distribution reconstructed by *sampling squads*, which nobody here has run. So the
  argument is **prospective**, and the recorded `z`'s inherit the anchor's weakness and the
  ±100-point field-mean bracket instead. Handling an uncertain `σ_F` is what the 80-to-220
  sensitivity box already does; a signed-bias argument is not the tool for it.
- **It is a derivative.** A finite bias classifies cleanly only when the true and reconstructed
  spreads fall on the same side of `|a|`; inside the box a mid-margin cell goes either way. Stated
  as "the local sensitivity turns sign at `|z_F| = 1`", never as "cells above `|z_F| = 1` are
  overstated".
- **The audit's "already contradicts its own bullet" was one step too strong.** The prose varies
  `z` across cells at fixed `σ_F`; the bullet varies `σ_F` at fixed cell. It carries the ingredient
  and implies the correction. Recorded as such.
- **The direction claim is cross-sectional within a gameweek.** The same reconstruction assumes
  independence across gameweeks, which errs the other way — ≤15.5 within-gameweek bounds the season
  spread at ≈96, *below* the 147 anchor.
- **The measurand split was not exhaustive.** A fourth was missing, and it is the one where the
  bias points at a conclusion this record draws: an overstated `σ_F` **understates** `P(top 10k)`,
  so the power argument in the handoff is conservative in its own direction. Added.
- **`σ_D` is unbuilt, not unmeasurable** — the archive supports it; nobody has built it. The two
  `z`'s are now named apart throughout: `z_F` (margin over `σ_F`) is what every recorded figure is
  in, `z_D` (`μ_D/σ_D`) is what the exchange-rate clause needs.
- **"Most cells are above the median" is removed.** The per-cell split is not recorded, and `σ_F`
  ∈ [80, 220] rescales every `z` by 0.67x-1.84x. Replaced with "both regimes are occupied".
- **A "never its sign" clause was dropped as drafted** — within a cell no `σ_F` changes a sign, but
  a *verdict* averaged over cells is exactly what the flip table shows moving. This section has
  already had two summary sentences wrong about their own table.

**Applied in both places** — `docs/notes/harness-and-inference.md` and the handoff's bullet 3 and
"What NOT to do" bullet 4 — in commit `5df2eb0`.

## What was declined, and why

- **Putting any of the bias correction in `CLAUDE.md`.** The reviewer's recommendation, and it is
  right: the closed-line bullet carries the S-curve argument and no bias claim, the do-not-build is
  unaffected, and a conditional mechanism claim in the always-loaded file gets quoted without its
  conditions — the exact failure being corrected.
- **Building any part of the rank architecture.** The five pieces are recorded, not started. The
  covariance matrix is the tractable half of an architecture whose payoff cannot be validated here,
  which is the worst combination — it looks like progress the whole way.

## Verified rather than accepted

The audit reported `selected` present in 2016-17, 2019-20 and 2021-22 beyond the three seasons the
entry-count series names. Checked directly: **it is in the header of all ten archived seasons,
2016-17 through 2025-26** — stronger than reported. ⚠️ A plain `grep -l selected` returns only
**seven of ten**, because the pre-2019-20 files are ISO-8859 rather than UTF-8 and grep skips them
as binary. The column is at position 41, quoted in the older files. This is the sixth arrival of
"the archive does not have X" being wrong and the first where the cause was **file encoding**
rather than the claim — recorded in the harness note beside the entry-count series.

## What could not be checked on this harness

- **`σ_F`, the field's season-total spread.** It anchors every exchange rate quoted in the new
  section and the 60-specification sensitivity box that defends the evaluation closure, and the
  record sources it to a *recalled* quantile ratio. It is **not in the archive**. It is also not
  fetchable today: the live Overall league (id 314) was probed during this review and returns zero
  rows in pre-season, and `bootstrap-static` serves `average_entry_score` as 0 for all 38 unstarted
  gameweeks. It becomes measurable **forward**, once 2026/27 runs, by reading points-at-rank from
  the standings pages — 50 entries per page, so top-10k is page 200. **Unmeasured and reachable,
  not unmeasurable.**
- **Whether a rank-maximising policy beats a points-maximising one.** **Unmeasurable here**, on
  power: a tail-probability payoff of order 0.1 to 5% against roughly five independent season draws,
  for an expected count of 0.005 to 0.25 events across the whole grid. Widening the archive does not
  reach it — seasons add linearly against a requirement quadratic in the inverse effect size, and
  2013-14 is the floor.
- **The per-cell count above and below the median**, which the replacement wording in finding 2
  needs to say "most cells". One pass over the same cells file would settle it; not a sweep.
