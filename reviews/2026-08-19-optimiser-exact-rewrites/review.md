# Review — the optimiser's exact rewrites

**Range:** `ded4762..4a4c9c4` on `make-the-serve-page-snappy`, which is the whole branch.
Reviewed in one sitting on 2026-08-19; `d8d2897` and `4aa6b2c` had been written in an earlier
sitting and never formally reviewed, so they are in scope.

## What was reviewed

A performance branch that claims to change no answer. `Optimize` goes from 3.77s to about 1.08s
on the live pool across six rewrites of the objective's inner loop — index sorts over parallel
scalar keys, a prefix-fold formation scan, a single-pass `materialise` with an `int` membership
scan, a bulk-copy `replaceInto`, a hoisted map lookup in `split`, and the extraction of the
armband's running top-two into one `promote`. Two diagnostic env switches are kept and
registered. Nothing here changes a scoring constant.

The reason this needed reviewing at all is the argmax: `Optimize`'s picks feed the replay and
the transfer choices, and one ULP flips a footballer.

## Reviewers

| reviewer | why |
|---|---|
| `fpl-code-review` | `internal/analysis` — scoring; and the triage's "a refactor asserting identical output" row, pointed at the differential comparison |
| `fpl-stats-review` | `internal/analysis` — scoring |
| `fpl-findings-audit` | `internal/analysis` — scoring |

None skipped: the `internal/analysis` row dispatches all three, and the union with the
identical-output row adds nothing. `fpl-security-review` was not owed — no `internal/agent`,
`internal/fpl` or config-persistence change. `fpl-docs-*` were not owed — `docs/` is untouched.

All three were dispatched concurrently, read-only, over the same range, and each was told what
was already known wrong and where the author thought the work was weakest.

## Findings, ranked by how misleading the state was

**1. `d8d2897`'s "answer-exact by construction" was false, and both instruments were blind to
it.** `bestFormation`'s prefix records add four per-position partials where the code they
replaced ran one accumulator across all eleven. That is a re-association, not a preserved
addition order, and floating-point addition is not associative. The claim appeared in a code
comment and, worse, was re-asserted in a guard sanction written this sitting.

Verified independently rather than taken on report. Two groupings compared bit-for-bit over 200k
trials: they differ on 38.9% of elevens with continuous scores, max gap 4.3e-14, and on **0%**
where every score is an exact multiple of 0.25 — because sums of eleven exact quarters are
exactly representable. Every score `optimizerdiff_test.go` generated was such a quarter, so the
differential comparison was green *because it could not run*.

Pushed further than either reviewer took it, to find whether it can change an answer. Both
callers discard `bestFormation`'s `best` and recompute over the sorted eleven, so the only
channel is the `total > best` formation comparison, which can bite only where two formations land
within a ULP. Measured against the frozen reference: 0 of 4000 on continuous scores, 0 of 4000 on
quarters, and **103 of 4000 (2.6%) on decimal tenths** — the one corpus that is both non-dyadic
and coarse enough for two formations to tie exactly. The divergence is a tie-break direction, and
it is the same defect `TestOptimizerDiffHarnessHasTeeth` injects deliberately as "ties go to the
later formation", arriving by another route.

**2. The guard sanction was the wrong remedy, and the right one was free.** `0a33a9f` raised
`TestTheCopiedExpressionsHaveOneImplementation`'s sanction for `squad.go` from 2 to 5 and argued
the copies were justified. `fpl-code-review` refuted this by implementing the alternative: the
running top-two is pure comparison and assignment with no arithmetic in it, so it extracts
without moving a bit. Reproduced here — all four copies now call `promote`, the differential
comparison stays bit-exact, the benchmarks are unchanged, and the scan's match count on
`squad.go` goes 5 to 0, so the entry is deleted rather than raised.

The sanction's stated argument — addition order, and not materialising an eleven — was about the
*sum*, and answered a question the guard does not ask. Both reviewers that looked at it reached
the same place from different directions.

**3. Two guards were red across `d8d2897` and `c9ba54a`, and the handoff named only one.**
`TestEnvSwitchListIsComplete` (two unregistered switches) and
`TestTheCopiedExpressionsHaveOneImplementation`. The second is the sharper miss: the sanction
count is *positional*, so at 2 the guard was excusing `foldPair`'s two lines and reporting
`xiValueShrunk`, the canonical implementation, as the offender.

**4. The banked snapshot proved less than it read as.** `4aa6b2c` is byte-identical to its
predecessor except `stamp.commit`, and 547 of its 555 figures are per-player
prediction-versus-actual that never construct a squad — equal by construction. Only the 8
`model.transfer_error` rows carry replay information, and they predated `c2d3321` entirely.

**5. The performance figures were quoted more precisely than they were measured.** The live-pool
stage timings are n=1 with no spread, and `git diff d8d2897 c9ba54a -- internal/analysis` is
empty — so the two figures the branch quotes as before-and-after across sittings (1.67s and
1570ms) measure *byte-identical code* 6.4% apart. The stage also spans `AssemblyBudget`,
`applyRoster` and `ApplyChipPlan`, so it is not `Optimize`. On the Go benchmarks the quoted
"after" of 2684 ns/op sat below an independent six-sample distribution (min 2743): a best-sample
figure, not a median.

**6. Coverage gaps in the differential comparison**, none of which turned out to hide a defect:
`replaceInto`'s rewritten bulk-copy path is not covered at all (the comparison *uses* `replace`
to build its cases, so a defect changes both sides equally); every arm built a fresh
`xiScratch`, while `polish` holds one across a whole search; `boost=true` never reaches
`materialise`; and the duplicate-id case the code documents is pinned by nothing.

**7. The rejected `blankRate` memo was recorded more strongly than measured.** "Answer-exact" was
an argument from construction the comparison could not have refuted, and 3.8% at n=1 is inside
that benchmark's own spread.

## What was applied

- `promote` extracted; all four copies folded; the `squad.go` sanction **deleted**, with a
  comment recording that the copies folded and that the next such entry should check whether it
  folds first (finding 2).
- The re-association comment rewritten to say what is true, with the measured rates and the bound
  (finding 1).
- `optimizerdiff_test.go` gains a **continuous-score arm**, 3400 cases, and a comment recording
  why a decimal grid must not be added — it would fail today, and as a mystery rather than as a
  re-opened decision. The teeth test still rejects all four injected defects (finding 1).
- A **shared-scratch arm**, running every case through one `xiScratch` as `polish` does, and
  checking `pick` and `bench` orders straight off it (finding 6).
- `pickIDs`' doc no longer claims to hold the eleven's ids after the final permute reorders
  `pick` and not it.
- Both env switches registered in the fingerprint guard's skip map; the instrumentation **kept**
  rather than stripped, on the user's ruling, since the optimiser gets optimised repeatedly
  (finding 3).
- Snapshot regenerated at `f146ced`, 554 figures, one run, `model.present` true (finding 4).
- `AGENTS.md`: the `blankRate` closed line worded "measured no faster" rather than "a slowdown"
  (finding 7); the dyadic blind spot under *Things that have already bitten*; and the
  exact-equality rule narrowed so it cannot be quoted to forbid the check it permits.
- A **36-cell replay A/B**, `c9ba54a` against `4a4c9c4`'s code, `TestDiagVarianceDecomposition`,
  six seasons × six entry points: **all 72 cells exactly equal** on `policy_points`,
  `hold_points`, `moves`, `hits` and every layer column. This is the evidence that replaces
  "answer-exact by construction".

## What was declined, and why

- **Reverting the prefix fold to exact sequential summation.** Raised as the alternative to
  finding 1 and put to the user, who ruled to keep the fold. The two formations differ by less
  than a ULP so neither is more correct; the fold is what made the formation scan constant-time;
  and the replay A/B is exactly equal. The cost of the decision is that "answer-exact by
  construction" is withdrawn for `d8d2897` and replaced by *answer-preserving on continuous
  scores, evidenced by the replay*.
- **A points sweep to validate the rewrites.** `fpl-stats-review` argued, and this record agrees,
  that a 36-cell points contrast would return a paired difference against a ~39 points-a-season
  threshold — a *weaker* statement than the equality check, and a null from it would be a tie.
  The A/B above was run instead.
- **Adding a decimal-grid arm to the differential corpus.** It fails today by construction of the
  accepted decision above. Recorded in the grid list rather than added.
- **Amending `c2d3321`'s and `d8d2897`'s commit messages** to scope their "byte-identical"
  claims. They are pushed history; the correction lives here, in `AGENTS.md`, and in the code
  comments, and this record is where a reader is sent.
- **An `AGENTS.md` entry for the speedups themselves.** They change no constant, no ordering and
  no verdict, and that file is not a changelog.

## What could not be checked on this harness

- **`boost=true` through `materialise` remains uncovered.** The A/B's chip columns are
  unpopulated, so the bench-boost arm is untested by it, and no test reaches `materialise` under
  the chip. **Unmeasured, not measured-null.**
- **`squad_hash` is empty in all 72 A/B cells**, so it contributed nothing to the equality
  comparison despite being the column that would most directly express "the same squad".
- **Whether the 2.6% tenths divergence has any live analogue** cannot be settled here. It
  requires real `Score` values to tie exactly at the formation level, which continuous arithmetic
  makes vanishingly unlikely but which nothing in this repository forbids.
- **The live-pool stage timings carry no spread** and n=1 is not enough to quote them at four
  significant figures. The Go benchmarks are the figures to quote; the medians are
  `BenchmarkObjectiveScratch` 2682 ns/op and `BenchmarkPolish/pool600` 347 ms over five samples.
- **`bit-exact` is established on `linux/arm64`, go1.26.5.** CI runs the same comparison on
  amd64, so both are in fact covered, but the qualifier belongs in the sentence.
