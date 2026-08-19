# Review: the holding-window criterion for hits

**What was reviewed**: branch `judge-hits-over-the-holding-window`, commits
`f2a5477..421e6c6`: the pre-registration and implementation of the holding-window criterion
(the user's ruling on the horizon criterion: judge a hit by +4 net before the in-players are
sold or a wildcard lands, accounting for free hits, bench boost and captaincy, split forced
vs preference), the `Week.Contrib` per-player scoring attribution it rests on, the new
full-plan arm, the R verdict script, the rewritten finding, the re-banked 252-cell snapshot,
the AGENTS.md verdict line, and the accuracy snapshot regeneration.

**Reviewers that ran**:

- **fpl-stats-review — the plan review** (before any cell ran): ten findings, all applied
  before the run: the sidecar row was per-gameweek not per-package (a free single + hit
  single in one week merged — the free leg flattered the hit's net) → package-boundary
  splitting on `Gain != 0`; the forced/preference classifier credited the transfer week →
  appearance now measured after the transfer week; (H') re-registered as the hold−horizon
  clearance GAP (a monotone share rise would be near-forced through the mechanical
  correlation); the per-cell loss-rate pairing dropped zero-hit cells → NA-preserving with
  compared-cell counts; clause (a)'s noise named (naive paired t, printed); the confinement
  given a mechanism (join the banks, diff integer columns, read before any verdict); the
  cut-liveness claim corrected (the floored machine plays BB/FH/TC — only the wildcard cut
  is absent); `hold_weeks` printed; `TestHoldingNetWindowEnds` pins the window arithmetic;
  the vice-fallback asymmetry and the FH-week captain flag documented.
- **fpl-stats-review — the output review**: verified the confinement (216 pre-existing
  cells byte-identical on every column), every headline figure from the banked files, the
  full-plan contrast (+97.6 a season, t 3.85, p 0.0120, wild 0.0111, Holm 0.0600), the
  horizon supersession framing, `holdingNet` against the pre-registration, and that
  `Week.Contrib` cannot alias across the four weekly scoring calls. Findings, all applied:
  the wildcard-after figure was 73% (22 of 30), now produced by the committed R; the
  workedOut wait-match **retired** (superseded by the user's criterion, computed by no
  committed script, its match-order tie-breaking kept it from reproducing across banks —
  the season-level −10.0 carries the wait reading); the holding means labelled **gross of
  the −4** (the +4 bar is the charge); the Holm footnote updated to family 6 / 0.0263; the
  ladder thresholds corrected to 16.2/13.4/18.6 from the MDE file; "two to three times"
  corrected to "roughly double"; the stale fidelity note replaced (one `n_moves > 2` row
  remains in 4,785).
- **fpl-findings-audit** (the writing): twelve findings, all applied or superseded by the
  output review's: stale pre-fix figures marked in place in the paired research-store note (Verdict block,
  first-State deltas and median, the queued (H') definition, the does-not-show item); n's
  added to the horizon table and the free-package shares; the Holm-family contradiction and
  the "FALLS" overclaim fixed; the first-bank/second-bank referent collision fixed; the
  budget check passed (AGENTS.md 80,538 of 81,920 bytes).

**Skipped with the triage reason**: fpl-code-review — the triage table assigns
`internal/backtest` and `stats/*.R` to fpl-stats-review + fpl-findings-audit (both ran);
the `Week.Contrib` change is recording-only, pinned by `TestContribSumsToGross` and
empirically by the byte-identical confinement. fpl-security-review — no authenticated
surface or credential path touched.

**What was declined**: nothing. Every finding was applied or resolved.

**What could not be checked on this harness**: the workedOut wait-match's exact figures
(retired for non-reproducibility — the resolution was recompute-or-delete, and the
pre-registration had already superseded it); the out-side raw convention's undercount where
a sold player was the previous week's captain (flagged per package, 6-14 per arm, rare).

**The verdict recorded**: 78-79% of hits clear +4 in the hold (mean +35 to +45 gross of
the −4, holds ~10 gameweeks) on all three machines; the forced/preference split does not
separate; H' shows no suggestive shape; MinGainHit 3.0 stands and nothing ships; the
full-plan machine's +97.6 over flat is suggestive, not Holm-clearing.
