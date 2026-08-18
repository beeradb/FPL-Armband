# Review: the chip-plan and free-hit fixes

**What was reviewed:** `FullAnchoredPlan` (both sets, all four chips, use-it-or-lose-it
fallbacks), the free-hit borrowed-fifteen recording and page, and the replay command's
`FPL_CHIP_PLAN=anchored` wiring. Commit range `origin/development..HEAD` (two commits) on
branch `fix-the-chip-plan-both-sets-and-the-free-hit-pool`.

**Reviewers:**

- **fpl-code-review — ran.** Verified the planner's rules, the free-hit page plumbing, the
  env wiring, and the `minAnchorClubs` move; swept `FullAnchoredPlan` +
  `ValidateChipSets` over all six seasons at 18 entry points and found the three edge
  findings below.
- **fpl-stats-review — skipped, by triage.** No measurement ran and no figure is quoted:
  the planner is calendar arithmetic, and the free-hit change is presentation.
- **fpl-findings-audit — skipped, by triage.** No `AGENTS.md`/`CLAUDE.md` edit.

**Findings, ranked by severity:**

1. Late entries into a two-set season validated and printed a full plan the replay played
   almost none of (chips below the entry are never reached, silently).
2. The planner emitted self-colliding plans at window edges — including grid entry GW16 —
   and the command aborted on them.
3. Fallback weeks could land at or below the entry point, and `ValidateChipSets` cannot see
   it.
4. Minor: one dead branch in the free-hit caveat's fallback text.

**What was applied:** 1-3, by construction rather than by patching the symptoms: the
set-2 window is now entry-aware, `claim` searches the nearest free week (down then up) and
reports a full window, fallbacks clamp into the window, and the command **refuses** an
anchored plan that cannot place every granted chip — GW16 and GW34 entries now fail loudly
with an actionable message instead of playing a phantom plan. New tests pin the full
two-set plan and the borrowed-fifteen recording. 4: left as written — the fallback branch is
defensive text for any future caller that sets `FreeHit` without a recorded squad, and is
documented as such.

**What was declined:** nothing else. The reviewer's note that hits move by path divergence
stands as recorded behaviour — the hit-gain-bar and hits-verdict measurements are queued as
the direct follow-up.

**What could not be checked on this harness:** the live 2026-27 plan derivation (the season
is not archived; the shipped config's plan remains hand-set), and the hits question itself,
which is a measurement rather than a fix.
