# The clean-sheet regressor refit, and the 2x2 it voided

Branch `clean-sheet-calibration-refit`, off `origin/main` at `6021401`.

## What was reviewed

A 2x2 over the clean-sheet term was commissioned. Building it required a new knob
(`cleanSheetScale` / `FPL_CS_SCALE`, the flat half beside the existing exponent half), two setters
so the design runs in one process, and the collapse of four copies of the clean-sheet exponent onto
a single `cleanSheetProb`. A plan review then blocked the run, a refit followed, the refit voided
the 2x2's constants, and the record was corrected in nine places. Two disclosed-but-unsized biases
were then sized, and finally the `XGC90 × def` arm ran — which refuted the factor a second time and
relocated the excess onto the defensive fixture ladder.

The change therefore spans: `internal/analysis` (the knob, the collapse, three corrected comments,
two exported calibration accessors), `internal/snapshot` (fingerprint registration, the renderer's
clean-sheet note, the resident-index budget), `internal/backtest` (two new diagnostics, one
corrected header), `stats/cs_calibration.R` (a retracted header assertion, and the two-channel
decomposition), `CLAUDE.md`, `docs/accuracy.md`, `docs/replay.md`,
`.claude/agents/fpl-stats-review.md`, and the banked snapshot.

⚠️ **This record was written mid-change and has been amended four times since.** Everything below is
marked with what it returned rather than only with what was owed — a record that lists obligations
and never their outcomes trains its reader to stop reading the list.

## Which reviewers ran, and which were skipped

| reviewer | ran | why |
|---|---|---|
| **fpl-stats-review** | **three times** — on the PLAN before the run, on the RESULTS after, and on the later `XGC90 × def` arm | the triage owes it for `internal/analysis` and `internal/backtest`. **Every one of the three overturned something**: the plan pass voided the constants before the cells were spent, the results pass killed a "correctly calibrated" headline, and the third refuted a "cannot separate" conclusion by finding the separation in the already-banked data |
| **fpl-code-review** | yes | `internal/analysis` is scoring code, and the collapse asserts byte-identical output |
| **fpl-findings-audit** | yes | the change edits `CLAUDE.md` and `docs/` and supersedes a recorded figure |
| **private-store-audit** | yes, three times | on each edit to the research record; not part of this repo's triage, recorded for completeness |
| fpl-security-review | **skipped** | nothing touches `internal/agent`, `internal/fpl`, config persistence or the cache. No new I/O beyond two diagnostic output paths |
| fpl-run-review | **skipped** | no live run, no config written |
| fpl-season-maintenance | **skipped** | none of the four hand-maintained lists is touched |
| **fpl-docs-review** | yes | the single documentation reviewer, which supersedes `fpl-docs-accuracy` and the private-store brief. It found the site my grep had missed (`docs/accuracy.md`'s "so it cannot change which ones you buy"), a wrong classification, and the only copy of the evidence never corrected |
| ~~fpl-docs-accuracy~~ | **superseded** | folded into `fpl-docs-review`, which can route between the two stores where a single-store reviewer could not |

## Invariants first, per the gate's own rule

Two were written **before** any reviewer was dispatched, and one of them is what actually carries
the refactor:

- `TestCleanSheetProbIsBitIdenticalToTheExpressionItReplaced` — 20,000 seeded draws, bit-exact.
- `TestEachCleanSheetCallSiteMatchesTheExpressionItReplaced` — pins each **call site** against a
  hand-written copy of the pre-collapse expression at a non-neutral `def`, plus a vacuity guard.
- `TestBothCleanSheetKnobsReachTheProbability` — liveness, and strictly stronger than the sweep's
  canary because it names the consumer instead of inferring it from a movement.

## Findings, ranked by how misleading the current state was

### 1. The plan was invalid, and the plan review caught it before the cells were spent

Both fitted constants (`f = 1.1731`, `flat = 0.9046`) were estimated against **realised
single-match xGC**, while the model scores `XGC90` — blended, shrunk, point-in-time. Different
variances, convex exponent, so the two disagree by construction.

**Applied:** built `TestDiagCleanSheetRegressor` and refit. On the model's own regressor,
predicted/actual is **1.052 native (n 1566), 1.004 pooled (n 2974)** against a recorded **1.281**,
rejected at |t| 6.7 and 9.5. The 2x2 was already in flight and so carries the voided constants; it
is reported as what it is — a bound on the harm of a mis-fitted correction — and not as an answer to
the clean-sheet question.

⚠️ This is the review-the-plan rule paying for itself: the code was correct throughout and no review
of the finished diff would have found it.

### 2. My own headline overstated the result, twice, and both were caught by review

Written: *"the clean-sheet term is correctly calibrated, and both one-parameter families are
accepted."* **Withdrawn.** Two LRTs failing to reject is not acceptance; the native interval is
[0.90, 1.20] and admits a fifth of the recorded bias. Second: *"it refutes b = 1.1731"* is **false
on the headline stratum** (t −1.19) and true only on the pooled one — which the same document
disowns. A stratum cannot be disowned for the null and borrowed for the rejection.

**Applied:** the findings record now claims only a refutation of a **magnitude**, and every one-line
restatement of it in `sweep.go`, `model.go` and `docs/accuracy.md` carries the population caveat,
the interval and the df.

### 3. The differential test did not test what its doc comment claimed

The first version restated `cleanSheetProb`'s own one-line body and never called a single call site.
The code reviewer showed that transposing `def` and `cf` at a site is **not** bit-neutral (35% of
random draws differ) and that nothing in the suite would catch it — every neutral-path check runs at
`def = 1`, where the transposition is exact.

**Applied:** added the call-site test above. The weak test is kept, explicitly labelled as a floor.

### 4. `FPL_CS_SCALE=0` silently ran at 1.0

`envDefaultAbove` rejects `v <= 0`. For a *scale*, 0 is a meaningful arm, so the fingerprint would
have stamped 0 on a run scored entirely at the shipped default — the byte-identical-null trap
wearing a provenance stamp.

**Applied:** a strict reader that accepts 0 and panics on anything unreadable, on the
`parseSweepStarts` precedent.

### 5. `.claude/agents/fpl-stats-review.md` carried a retracted claim as live review guidance

It asserted *"the clean sheet is over-predicted by a quarter and correcting it loses"*. The second
half rested solely on the ladder retracted on provenance 2026-08-14, and the only banked run of the
family has all three corrective arms weakly **positive**.

**Applied:** rewritten. This is the highest-leverage edit in the change — that agent gates every
future plan that produces a number.

### 6. The ~30% figure was corrected at four consumers and not at its source

**Applied:** `cleansheet_calibration_test.go`'s header now states that what it scores is not what
the model scores, so a reader regenerating the figure does not re-derive it clean.

### 7. P4 and P5 of the pre-registration were undischarged

The interaction was never reported despite an explicit commitment to report it "whatever it does".

**Applied and it produced something.** The **factorial** flat main effect has SE 0.0604 against the
simple effect's 0.1593, t **2.46** against a `t_crit` of 2.571 — the tightest estimate in the run,
and still short. ⚠️ **P4's own diagnostic prediction FAILED**: the interaction's SE came back
*larger* than the main effects', where the pre-registration said smaller and said larger would mean
the pairing is wrong. Recorded as failed and unexplained rather than explained away.

### 8. Smaller, all applied

The `cleanSheetScale` doc block had not received the correction its sibling twenty lines above did;
`FPL_CSREG_ROWS` was in no registration list (caught by `TestEnvSwitchListIsComplete`, which is
exactly its job); `docs/replay.md` documented half a 2x2; `docs/accuracy.md`'s heading and its 1.25
vs 1.281 denominators; `xpoints.go`'s dependent citation of a clause being qualified; the findings
record's SE column was labelled against the wrong null and mixed two clustering axes unnamed; and
its claim that the representative is "usually the ever-present keeper" is **wrong** — 765 GKP
against 801 DEF on the headline stratum.

## What was declined, and why

- **Amending `182141f`'s commit message**, which says "Both restrictions ACCEPTED". The commit is
  unpushed and amendable, but this record's convention is to mark a correction in place rather than
  rewrite history, and the wrong belief is part of the record. **Named here instead**, which is the
  in-place mark: *"ACCEPTED" in `182141f` is withdrawn — at a clustered SE of 0.152 that is
  non-separation, not acceptance.*
- **Re-running the 2x2 at the refitted constants** (`f` 0.992, flat 0.939). The canary shows the
  family sits ~4x below detection and a factor arm of 0.992 is a no-op. Declining is the finding.
- **Deleting the standing rule** "a bias shared by every player in a position is not an ordering
  error". It is contested from three directions but nothing has replayed against it, and *a measured
  bias does not imply a correction exists* carries most of its load. **Qualified, not withdrawn.**
- **A second home for the queue's snapshot counts** (proposed by the private-store audit).
  A two-SHA bookkeeping fact given a second home is the shape of a failure this project has already
  paid for.
- ~~**Sizing the 90-minute selection and the `cf` omission.**~~ ✅ **DONE 2026-08-15, after this
  record was first written.** Both are now numbers rather than caveats. The guard drops **14.2%** of
  club-gameweeks — ⚠️ **not the 22% this record's own audit estimated and which I repeated into four
  files without counting**, which is the failure this project keeps recording, committed inside the
  change that was correcting somebody else's figure. The dropped set is genuinely worse defensively
  (0.1992 against 0.2636), and removing the selection takes the pooled ratio 1.0051 → 1.0305;
  applying the omitted coupling takes it to 1.0112. **Together they move the pooled over-prediction
  from ~0.4% to roughly 3.7%** (composed, not jointly measured) — the refutation of 1.281 is
  untouched, and "calibrated to within half a percent" is dead. ⚠️ **This first read "roughly 3%" in
  five files** — the selection figure alone under the word "together", an error running the
  flattering way, caught by the documentation review. ⚠️ A mechanism claim I made for the coupling's direction was also wrong and is
  corrected in place at the diagnostic.

## What could not be checked on this harness

- **Whether a correct clean-sheet correction would pay.** The canary bounds the whole family at ~4x
  below detection on 36 cells, so this is **unmeasurable here**, not unresolved.
- ~~**The `XGC90 × def` path**~~ ✅ **RAN 2026-08-15, after this record was first written, and it
  answered more than expected.** The single-regressor fit on the product read `b = 1.2022` and
  looked unresolved — but `ladder` is `1 + s(base−1)`, so the exponent is `f·x + f·s·x·(def−1)`
  exactly, and the two channels separate in the banked data at **cor 0.001**: the clean-sheet factor
  is **null (1.0476, t 0.30)** while the defensive fixture ladder reads **1.5688, t 2.53 native /
  3.30 pooled**, above 1 in 6 of 6 seasons. **The factor is dead on this path too**, and the excess
  relocates onto `FPL_DEF_FIXTURE_SCALE`'s defensive half — already measured as points-null across a
  fourfold width change. ⚠️ **My first write-up of this arm said the design "cannot separate them"
  and was refuted by the third statistics review** — the same failure shape as the two above, a
  hedge standing in for an analysis that was available. ⚠️ Post-hoc; native season-clustering does
  not clear; `def` is ungated by the cutoff.
- **The comparison leg.** The realised-xGC dump behind `a = 0.1003, b = 1.1731` is banked nowhere in
  this repository, so that half of the contrast cannot be re-derived here.
- **Whether the exemption's other sites cost anything.** Four sites let a position-wide bias stand;
  two are fixed here, `blend.go`'s level (0.75 → ~0.70 undone) and the GKP conversion carve-out are
  not. None has been replayed.
