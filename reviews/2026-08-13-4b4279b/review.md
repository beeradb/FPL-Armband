# The xGC transport test, reviewed properly

Covers `fbd01db..4b4279b` on `xgc-transport-test` — the seven commits that build, run and
write up the transport test, the fixes applied at `15c6caa`, and the snapshot at `4b4279b`,
which is the commit this record is named for.

The record is named for the snapshot rather than for the fixes because the snapshot moves a
watched path (`stats/`) and would otherwise leave the review gate red at the tip. Same
two-step the previous record used: record the changes, then name the record for the commit
it actually covers.

## This record exists because the previous one asked for it

`reviews/2026-08-13-8c1ba70/review.md` is a **self-review**. The authoring session could
not dispatch agents, recorded that plainly rather than glossing it, and wrote: *"Whoever
picks this up next should dispatch `fpl-code-review` and `fpl-findings-audit` properly on
this range."* That has now happened, and `fpl-stats-review` was dispatched too — its
mitigation ("this session's own brief **is** `fpl-stats-review`") was real but still left
the same session grading a measurement it had designed.

**The self-review's four findings all survive.** Nothing it reported was wrong. But it
missed the defect that matters most, which is what an independent pass buys and is
recorded here rather than smoothed over.

## Triage

| reviewer | ran? | why |
|---|---|---|
| **fpl-code-review** | **yes** | ~300 lines of new Go diagnostic and ~90 of new Python that nothing else checks. A bug in it manufactures the finding |
| **fpl-stats-review** | **yes** | a new measurement and a verdict word attached to it |
| **fpl-findings-audit** | **yes** | `docs/notes/archive-and-data.md`, `TODO.md`, and the record edits |
| fpl-security-review | no | no agent layer, no client, no config persistence, no credential path |
| fpl-run-review | no | no live run; nothing written to `config.json` |
| fpl-season-maintenance | no | none of the four hand-maintained lists touched |

All three ran concurrently, read-only, briefed with the commit range, the known-wrong list,
and an explicit instruction not to treat recorded nulls as settled.

## Invariants first, per the skill — and they were checked, not asserted

- **Every non-test Go change is a comment.** `internal/backtest/xgcrepair.go` is the only
  non-test Go file in the range and after the fixes; `const xgcScale = 1.0` is untouched.
  Verified by filtering the diff, both before and after this record's own changes.
- **The model half of the snapshot is byte-identical.** 555 `model.*` rows identical
  between the `b2a3d9c` and `a4632d1` stamps — which is what a comment-only change must
  produce, and it is the invariant the branch owed.
- **The recipe defect the previous record reports as fixed really is fixed.** All 8
  `model.transfer_error.*` rows are present in the repaired snapshot.
- **The published table reproduces exactly.** The arm-B inputs were regenerated from the
  Understat cache and the diagnostic re-run: every figure identical, which is also the
  differential check that swapping the inlined Pearson for `corr()` is behaviour-preserving.
- Full suite green, `internal/backtest` 103.9s.

## Findings

Ranked by how misleading the state was.

**1. The liveness guard could not fire in the direction that threatens the result.**
CONFIRMED, fixed. It asserted the two arms differ, to catch arm B collapsing onto arm A and
reporting a *perfect* transport. That collapse was **already unreachable** —
`readTransportXG` fatals on a missing file and again on `len(out) == 0`. The reachable
failure is *partial* delivery, and `withTransportXG` gives an unmapped player **zero**, so a
thin crosswalk raises arm B's error and produces exactly the published headline. Every
surviving delivery failure was invisible to the only guard aimed at it.

The disposition on the self-review's finding 4 ("compares two floats for exact equality",
acknowledged, not fixed) is therefore too generous: a *tolerance* would have bought nothing,
because the guard was pointed the wrong way, not merely brittle. `TestDiagXGCTransport` now
asserts transported xG **mass** against FPL's own over the identical scored rows — measured
0.9287 / 1.0021 / 1.0451 / 1.0216, matching `armB/armA` to four decimals.

**2. The accuracy verdict declined a standard error that was free.** CONFIRMED, fixed. The
self-review's finding 3 acknowledged the gap and declined it, arguing a fourfold change at
"n = 5,184 cells" over four seasons is not plausibly noise. Both halves of that defence are
wrong, and the conclusion is right anyway:

- **5,184 is 2023-24's ever-present count alone.** The diagnostic's `ever-n` column reads
  2,914 / 5,184 / 5,172 / 5,261, summing to the 18,531 already recorded in `xgcrepair.go`.
  A single season's n was quoted in a pooled argument.
- **The cell axis is wrong regardless.** `reconstructedXGC` returns
  `xga[club,gw] × minutes/(90·n)`, so for an ever-present in a single-fixture gameweek both
  the reconstruction and the truth *are* the club figure — every such row in a club-match
  carries the identical relative error, ~7.2 cells per independent unit.
- **The honest statistic costs four subtractions.** Deltas 12.8 / 12.8 / 14.8 / 15.0,
  mean 13.85, season-clustered SE 0.608, **t 22.79 on df 3, p 0.0002**, every
  leave-one-season-out subset under p 0.003. `established` now rests on a t.

**3. Five places still advertised the superseded fidelity, three of them upstream.**
CONFIRMED, fixed. The self-review's finding 1 says the qualifier was applied "in all three
places the result is recorded" — true, and verified. But the *superseded figure* sits in six
places, and a reader meets three of them long before reaching the correction: the
validating subsection of `archive-and-data.md` (270 lines above its own retraction), three
anchors in `xgcrepair.go`'s doc comment, and **`CLAUDE.md`, which this range never touched**
and which instructed every session to quote 1.0088. All now marked in place.

**4. The level was checked against the wrong baseline.** CONFIRMED, fixed. Arm A carries
the chain's own bias (0.9962-1.0020), so the quantity the offset predicts is **arm B over
arm A**, not arm B against 1.0. Done properly the residual is ≤0.033% in all four seasons;
the absolute baseline leaves **−0.370% on 2023-24** — the season the text calls the clean
case — so a reader checking the arithmetic would find a discrepancy that does not exist.
Fixing it strengthens the claim and doubles as the coverage proof finding 1 asked for.

**5. The whole 2022-23 row is scored on GW16-38, not just its ordering population.**
CONFIRMED, fixed. `scoreXGCArm` skips every row where `g.XGC <= 0 || g.XGCReconstructed`,
so *every* column of that row is the short window. The self-review's finding 2 scoped this
to the ordering population, which implies the MAE columns are comparable across seasons when
they are not. Its **deltas** remain legitimate replicates — the contrast is paired within
season — and that distinction is load-bearing, since the replication argument is about
deltas.

**6. Two estimators share one name.** CONFIRMED, documented. `measure_ratios` harvests only
900+-minute players; `transport()`'s `own` is a ratio of totals over the full population. So
recomputing "+7.7%" from `borrowed_ratio`'s recorded 1.031/1.091/1.128/1.104 gives +7.4%,
and neither figure is wrong. This reconciles a discrepancy the findings audit flagged and
could not resolve — the code review found the cause independently.

**7. The headline statistic is not the pre-registered one.** CONFIRMED, documented.
Spearman was pre-registered and came back `suggestive`; ever-present MAE got the strong
grade. This is *not* statistic-shopping — ever-present MAE is the pre-existing validation's
own statistic, recorded before this test existed — but the asymmetry should be met with an
explanation rather than surprise.

**8. Smaller code findings.** `scoreXGCArm` inlined a second Pearson formula beside a
package-local `corr()` (fixed, guarded on `n > 0` since `corr` divides by it); `spearman`'s
`ok` was discarded, collapsing "no ordering exists" onto 0.0000 (fixed to NaN).

**9. The run was not reproducible from the repository.** CONFIRMED, fixed. `stats/out/` is
gitignored, so both arm-B inputs and the console table were absent — the previous record
faults MINHL for exactly this (its finding 6) without applying it to its own run. The four
transport CSVs and both console outputs are now committed under `stats/cells/xgc-transport/`.

## A reviewer finding that did not survive verification

The skill warns not to let a reviewer's report become the finding, and one did not.
`fpl-stats-review` reported the ordering leave-one-out subsets as
"0.066 / 0.076 / 0.102 / 0.018" and concluded **"the season whose removal costs most is
2022-23, the one whose population is selected differently"** — a tidy story, since that is
also the differently-scoped season. Recomputed, it is backwards: dropping 2022-23 gives
p 0.066, the *mildest* of the three failures, while dropping **2024-25** gives 0.102, the
worst. The published text says 2024-25. The reviewer's own numbers were right; the sentence
it drew from them was not.

## What was applied

Findings 1-9, in `CLAUDE.md`, `docs/notes/archive-and-data.md`, `TODO.md`,
`internal/backtest/xgcrepair.go` and `internal/backtest/xgctransport_test.go`, plus the
committed inputs. The published figures did not move; what moved is what defends them.

## What was declined, and why

- **Fitting a level correction from the transport residual.** Declined again, but the
  *reason* is re-ordered. "Invisible to an argmax" is the weaker leg and is not strictly
  true — the clean sheet is `4·exp(−cs·XGC90)`, so a shared multiplicative level preserves
  defenders' order among themselves while changing the spread the optimiser trades against
  price. The decisive argument is in the numbers: the four ratios average **0.9989 with no
  consistent sign, two above 1 and two below**. There is no level to correct, and what
  little there is is noise in a borrowed constant that is unknowable by construction for the
  seasons needing the repair.
- **Re-running the 18-cell `FPL_NO_XGC_REPAIR` sweep.** Nothing here changes what the replay
  computes, so the recorded reading is not stale. It is still a draw.
- **Re-writing the substitution-correction refutation.** It is over-determined — the same
  discriminator is withdrawn twice in `TODO.md` on disjoint grounds — so the two items were
  cross-linked rather than merged. Both survive.
- **Restating the accuracy grade as `suggestive`.** The findings audit proposed
  "established in direction and order of magnitude, unquantified in precision". Once the t
  is computed that wording is unnecessary: `established` is carried by t 22.79 on df 3.

## What could not be checked on this harness

- **Whether the transport error explains the recorded `FPL_NO_XGC_REPAIR` result.** It does
  not, and nothing in the range claims it does. The restraint is correct in all four places.
  One thing was added: the premise is now measured **and measured small** — arm B still
  orders defenders at Spearman 0.87-0.95 — and the −34 now carries the sign-flips caveat the
  same file states 180 lines earlier.
- **Whether the substitution-tercile cancellation survives the input change.**
  `xgcrepair.go` records that the proration's two errors "largely cancel at player-season
  level", measured FPL-fed. The transport run measures dispersion and ordering, not the
  tercile structure, so this is a claim whose evidence now covers a population the claim
  does not. Queued in `TODO.md`; one extra column in `scoreXGCArm` settles it, no replay.
- **Whether the era gap matters.** The test measures provider disagreement on 2022-23 to
  2025-26 and the repair runs on 2018-19 to 2022-23 GW1-15. Older crosswalks are thinner, so
  the direction is conservative, but "there is no fifth season" covers only the missing truth
  column, not the era gap.
- **Whether the ordering loss is material to squad choice.** xGC feeds 26-45% of a
  defender's score, so the loss reaching `Score` is smaller than the 0.019-0.052 measured on
  xGC90 — by how much is unmeasured.

## The snapshot, taken after the fixes

`stats/snapshots/2026-08-13-08b9eef` was taken because the fixes touched
`internal/backtest/xgcrepair.go` and the staleness guard is owed one even when every change
in that file is a comment. That is the point of taking it: **all 555 `model.*` rows are
byte-identical to `a4632d1`'s**, which records the branch's central invariant rather than
asserting it, and all eight `model.transfer_error.*` rows are present — the repaired recipe,
not the short one the previous record found.

The harness half remains absent for the reason `a4632d1` gives: those figures come from a
per-cell sweep CSV that was never committed. Not regressed here and not fixed here. ⚠️ Note
the asymmetry now on the record: the *transport* run's inputs are committed under
`stats/cells/xgc-transport/` while MINHL's are not, so the convention exists, is followed
once, and is still unfollowed for the sweep series.

## ⚠️ Handover: CLAUDE.md has three bytes of headroom

`TestEveryNoteIsIndexed` caps `CLAUDE.md` at 64 KB and it now sits at **65,533 of 65,536
bytes**. It was already at 65,317 before this work, so the xGC correction was compressed to
fit and the detail lives in the note. **The next documented finding will fail the build**,
and the failure will look like a problem with that finding rather than with the budget.
Whoever hits it should either displace a superseded verdict or raise the cap deliberately —
not trim the new finding until it fits, which is how a summary file becomes a set of
assertions with no evidence behind any of them.
