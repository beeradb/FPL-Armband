# Review record — merging origin/main into the oracle branch

**Commit range reviewed:** `a6f582a..9de80a7`. This is the integration merge (`fe2ab99`) plus its
snapshot, not new research. The two research ranges before it have their own records at
`2026-08-11-b1af338` and `2026-08-11-a6f582a`.

## Reviewers dispatched

**None, and the triage reason is that a merge owes a different check than a change does.** The
review-gate table maps reviewers onto what a change *claims*; this commit claims nothing new. It
asserts that two lines of work compose. The right instrument for that is invariance, and the gate's
own first section says so: ask what quantity must not move, and test it.

What was checked instead, all mechanical:

- **No model figure moved.** The regenerated snapshot diffs against `2026-08-11-a84cbb7` and the
  only row that changed is `stamp.commit`. Both sides' new behaviour is off by default —
  `TeamForm` ships disabled on main's side, and this branch's oracles and `AnticipateChips` /
  `AnticipateGate` / `ChipPlanner` are all off — so a moved figure would have meant a resolution
  error rather than a finding.
- **The full suite is green**, including both staleness guards and every invariance test the two
  lines added.
- **Four conflict resolutions verified by grep after the fact**, listed below.

## The conflicts, and how each was decided

**`simulate.go` — kept both.** Main added `Engine.TeamForm` at five engine sites; this branch had
replaced the `newRecentIndexWith` calls with `cfg.recentIndex` so the minutes oracle has a single
seam. Neither supersedes the other, so every site now sets both, with `cfg.anticipate` where this
branch had it.

**`harness_test.go` — took main's wholesale, and it is the better fix.** Both lines independently
repaired the same hardcoded `/Users/bbowman/...` config path. Ours resolved it via an env override;
main's resolves against the repository root *and* makes absence an error. That second half is the
one that matters: `config.Load` writes defaults and returns nil for a missing file, so on any Mac
that is not the author's, every DIAG figure would have come from `config.Default()` with a clean
exit. Ours only appeared to work because `/Users` does not exist on this box — the fatal fired by
luck of the platform. Main also fixed `CacheDir` resolving against the package directory rather
than the repository.

Resolved by hand rather than with `--theirs`, because this branch's oracle seeding lives in the
same file and had not conflicted. Verified afterwards that `OraclesFromEnv()` survives.

**`modelrepro_test.go`, `chipsequence_test.go` — took main's** `loadConfig(t)`, the same change.

## What was verified to survive, and why each mattered

- **`XIValue`'s `loadInScore` guard.** Main does not have it and still carries the comment claiming
  the double-count is impossible. Losing it in the merge would have silently restored a bug that is
  unreachable at shipped defaults — so nothing would have failed.
- **`AnticipateGate` and `ChipPlanner`**, which carry the chip findings.
- **No `configPath()` call sites left on the old signature.**

## Could not be checked here

- **Whether main's chip caveat applies to this branch's chip figures.** Main's rewritten
  `docs/notes/chips.md` warns that its chip numbers are owed a re-run because `Optimize` was
  non-deterministic until the `seedFor` map-iteration fix, and because the pool contained players
  who had not yet joined. The determinism fix (`9e5e1d1`) **is** in this branch's base, so the
  anchored-chip and reveal-lag figures are unaffected by that half. The pool-contamination half is
  untraced and is the open question before citing +10.8 alongside main's figures.
- **A findings audit of either research range**, still outstanding from the previous record.

## The merged documents are the thing to read

Both lines edited `CLAUDE.md`, `docs/expected-points-review.md`,
`docs/notes/harness-and-inference.md` and `docs/notes/scoring-model.md`, and git auto-merged all
four. Auto-merge is textual, so a paragraph from one side can now sit beside a paragraph from the
other that contradicts it — which is exactly how a retracted figure gets re-asserted here. Those
four files are where a human read is worth most before this reaches `main`.
