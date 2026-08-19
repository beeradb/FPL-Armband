# Review: the net-basis hit charge and the visible arithmetic

**What was reviewed**: commits `3151c92..b2d1c0d` (net-basis correction + UI arithmetic): the
user's rulings of 2026-08-18 — "you are paying for this in points, so why are we not
subtracting the charge in our measurement?" and "make sure it is clearly shown in the UI and
subtracts from the gameweek score" — applied as: the holding criterion now subtracts the 4
from the SUM (bar ≥ 0 on both criteria; the first version's bar form classified identically
but made the means read 4 points high and mirrored a mechanically 4-point-stricter "horizon
net ≥ 4" bar onto an already-net figure); the replay HTML week header now renders the
arithmetic ("49 − 4 hit = 45") and the gameweek tab carries a hit badge; the finding, the
AGENTS.md line, the pre-registration doc comment, and the research-store note were corrected
and marked in place. No re-run: the transform is deterministic from the banked sidecar.

**Reviewers that ran**:

- **fpl-stats-review**: verified the classification equivalence exactly (0 of 398 hit rows
  differ between the two bar forms), the column semantics in code (`hit_net` net via
  `Verdict.Net()`, `hold_net` gross), the corrected H' gaps (−1/−2/0/−7 at n 73/45/39/30)
  against the banked sidecar, the exact −4 shift of every mean/median with every
  classification invariant unchanged, the R transform applied once on the hits frame only
  with no third convention (the wildcard section's bar-on-gross form is the same test), and
  the UI arithmetic at every reachable state (no misleading case; a −8 week unreachable at
  shipped MaxHits 1). Findings, all applied: the pre-registration doc comment re-registered
  the sum form with the equivalence to the first-registered bar form and the ⚠️ marking the
  H' mirror error; the finding's Settles paragraph reworded from "clear +4" to "clear their
  charge".
- **fpl-findings-audit**: five findings, all applied: the Settles bar-phrase leftover
  (fixed); the research-store note's gross figures marked in place on the next pair cut
  (done, merged); the H' pairs/gaps rounding noted in the finding (the gaps compute on
  unrounded shares, so pairs and gaps round independently); the replay test's assertion
  strengthened to pin the fixture's exact rendered arithmetic ("49 − 4 hit = 45"); the
  resident-index budget passes with ~286 bytes of headroom (noted, no action).

**Skipped with the triage reason**: fpl-code-review — the triage table has no row for
`internal/present`; the only code change is a template helper + rendering, covered by both
reviewers' scope and pinned by the strengthened replay test. fpl-security-review — nothing
on its surface.

**What was declined**: nothing.

**What could not be checked on this harness**: nothing new — the transform needs no replay
and every figure was re-derived from the banked files by the reviewers.

**The verdict recorded**: 78-79% of hits clear their charge in the hold (mean +31 to +41
net of the −4) on all three machines; H' reads −1/−2/0/−7 — no shape; MinGainHit 3.0 stands
and nothing ships; the page now shows the gross − hit = net arithmetic.
