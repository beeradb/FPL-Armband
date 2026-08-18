# Review: the gate-floor 2×2 and the scheduled floor

**What was reviewed:** the "react faster early" measurement — the gate-floor 2×2
(`TestDiagGateFloorUnderExitLevers`), the floor-flip counter in `decide`, the scheduled-floor
arm (`TestDiagScheduledFloor`) with the new `review_policy.early_floor` knob, both findings,
and the banked cells. Commit range `origin/development..ded1f8d` on branch
`floor-2x2-charge-x-min-gain`.

**Reviewers:**

- **fpl-stats-review — ran twice.** On the PLAN, before any cell ran: corrected the kink
  arithmetic per horizon regime (the GW35 boundary), reattributed the mains' channels (pair +
  banking + hits, not pair-only), replaced the unsound canary with the conditional, demanded
  the flip counter and the Holm-over-4 family (S = A + B), and re-derived the column roles for
  early-season exposure. On the OUTPUT: recomputed every figure against the banked cells (all
  reproduce), verified the counterfactual and the schedule code, and reported the five
  findings below.
- **fpl-findings-audit — skipped, by triage.** No `AGENTS.md`/`CLAUDE.md` edit.
- **fpl-code-review — skipped, by triage.** The code is a measured gate change and its
  counter; no `Score` path is touched, and the zero-value byte-identity is pinned by
  `TestTheOptionValueLeversAreOffByDefault` (extended by this branch) plus the {2.0,0.2}
  corner's measured invariance.

**Findings, ranked by how misleading the current state was:**

1. "Positive at the predicted size" overread +6.7/+4.0 (no size was ever predicted; 2 of 6
   cells positive per live entry) and juxtaposed a six-cell column mean against the pooled
   36-cell threshold.
2. "The level is measured as ≈ zero" rounded a ±32 bound into a verdict.
3. The schedule finding's "start-fixed SE 0.064, t 0.74" was the naive pooled SE, not the
   start-fixed estimator (0.069, t 0.68) — the error ran in the flattering direction.
4. The schedule/taper comment asserted "schedule first, curve second" while the code
   multiplied the UNSCHEDULED base — latent, no banked figure contaminated.
5. "As the kink arithmetic requires" over-attributed the {2.0,0.2} pair channel's zero.

**What was applied:** all five, verified before editing. 1: title and Settles reworded — the
sign claim now carries "no cell-level majority", and the live columns are quoted as point
estimates with their own ~32 threshold. 2: "consistent with zero, bounded within ±32,
unresolved" in both findings. 3: the start-fixed figures corrected. 4: the taper now
multiplies the local (possibly scheduled) base, making the comment true — one line, and a
zero-value pin added to `TestTheOptionValueLeversAreOffByDefault`. 5: the attribution scoped
to the singles gate, the pair channel's zero marked measured.

**What was declined:** nothing. The reviewer's CLAUDE.md suggestion is noted for the record's
next pass; the results live in the research record's notes and the findings per the user's
standing direction.

**What could not be checked on this harness:** the schedule's sign at cell level (2 of 6
positive per live entry — no majority, and 12 live cells cannot); the shape question for any
later switch point (only GW8 was pre-registered); and the schedule + taper composition beyond
the code fix above (no arm has ever run both).

**Recorded after the review:** the accuracy snapshot was regenerated and committed
(`2026-08-18-fd1280a`, mechanical, driven by `TestSnapshotCoversTheCurrentCode` — the branch
moved the scored path), and the re-key above covers it.
