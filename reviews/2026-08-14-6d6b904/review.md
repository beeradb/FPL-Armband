# Review record — the minutes-floor reconciliation

**Commit range**: `5ba8765..HEAD`.
**Date**: 2026-08-14.

⚠️ **Extended after the original record was written.** `ffac3fb` answered the question this work left
open — does the optimiser return its own argmax — and drew a **third** review. Its findings are in
"Round two" at the end; the first published conclusion from that commit was **retracted within the
hour**, so read that section before quoting anything about the search.

## What the range does

Closes TODO.md's PRIORITY item "reconcile the minutes floor". Re-runs block J
(`TestDiagRejudge`) as paired differences on the six-season grid, adds a static probe
(`TestDiagFloorPopulation`), corrects a unit error that had propagated into four files, and extracts
`cutByExpectedMinutes` / `CutByExpectedMinutesFloor` in `internal/analysis/squad.go` so one
predicate serves both `Optimize` and the probe.

Primary artefact: `stats/snapshots/2026-08-14-minfloor/FINDINGS.md`.

## Reviewers

| reviewer | ran | why |
|---|---|---|
| **fpl-stats-review** | yes, twice | triage row "internal/backtest or stats/*.R". Ran **once on the plan before the sweep** and once on the results |
| **fpl-findings-audit** | yes | triage rows "internal/backtest" and "CLAUDE.md, docs/" |
| fpl-code-review | **skipped** | the only non-test change is the `squad.go` extraction, which is covered by a differential check stronger than a diff reading: 36 probe rows byte-identical before and after, plus the full `internal/analysis` suite. The triage table's own note says to point a reviewer at the differential test rather than the diff; the test exists and passed, so there was nothing left to ask |
| fpl-security-review | skipped | no `internal/agent`, `internal/fpl` or config-persistence change |
| fpl-run-review | skipped | no live run, nothing written to config |
| fpl-season-maintenance | skipped | none of the four hand-maintained lists touched |

**Invariant written before dispatching anyone**, per the skill's first section: *the probe and the
replay must agree on the flip count*. They do, 2 of 36 in both, same cells. Asking that question is
what found the `squad.go` duplication below, before any reviewer read the diff.

## Findings, ranked by how misleading the state was

### 1. The recorded "112 HOLD points" was a 12-cell total — APPLIED

Found by stats-review **on the plan, before the sweep ran**. `194c654` used `starts := []int{1,11,21}`
over four pairs = 336 cell-gameweeks, so 112 is **0.333 pts/gw = 12.7 a season** and the "122-point
spread" is 13.8. Verified independently by `git show 194c654`. The contradiction being investigated
was an eighth its published size. Corrected in `CLAUDE.md`, `docs/notes/transfer-policy.md`,
`docs/notes/scoring-model.md`, `TODO.md` and `internal/backtest/transferpolicy_test.go`.

### 2. On points the comparison is UNMEASURABLE, not unresolved — APPLIED, and it changed the verdict

Found by stats-review on the results. With `S` season clusters of which `k` are non-zero, CR2 on
balanced cells equals the between-season SE and **|t| ≤ √(k(S−1)/(S−k))** — 1.000 at k=1, 1.581 at
k=2. Against `t_crit(5) = 2.571`, **rejection is arithmetically impossible below four non-zero
seasons.**

Verified two ways before applying: the closed form, and direct construction. The empirical
confirmation is decisive — the `floor=45` arm has k=1 and measured **exactly −1.000**.

Three retractions followed, all of them of things this same commit had written: the pre-registered
3-season bar was too lax (the bar is 4); the 9.7/8.2 threshold must not be quoted, being the MDE for
a *uniform* shift that a squad-flipping mediator cannot produce; and the CI is uninformative by
construction, since effect and SE co-scale. Generalised into
`docs/notes/harness-and-inference.md` as a new section, because it applies to every discrete-mediator
contrast rather than to this constant.

### 3. "The ladder is not monotone" was FALSE — APPLIED

Found by stats-review. On this run the four arms read −0.125 → −0.024 → 0 → +0.430: **monotone
increasing**, on all three HOLD instruments. The non-monotone ladder is the *recorded* one. This
mattered directionally — monotonicity is one of three structures this project accepts as
establishing a constant, so read as structure it argues for **raising** the floor. The verdict
(decline `floor=65`) was right; the stated reason was a false fact, and is replaced by the
fragility argument: two GW26 cells contribute +0.585 of the +0.430 mean, and dropping them takes the
arm to −6.2 a season.

### 4. "90% is a search artefact" mixed the units this commit exists to separate — APPLIED

Found by **both** reviewers independently. 102/113 is a ratio of raw totals; the −0.125 pts/gw it was
attached to is built from per-gameweek means, and the cell banks 28 gameweeks, so the share is
**81%**. Verified by recomputation. This is the §1 error committed inside the write-up correcting §1.
Now stated as 81% with the totals share named, and the note and CLAUDE.md carry the
denominator-free form instead: dropping that cell takes the arm from −4.7 to −0.9 a season.

### 5. 229.7 is a whole-field count, not a pool-removal count — APPLIED

Found by findings-audit. The probe counts over `e.AllMetrics()`; `Optimize` reaches the
expected-minutes cut only after `clearsMinutesFloor` and the availability screen, so the pool-removal
count is strictly smaller. The audit also **refuted my own hedge**: I had offered grid width as a
candidate explanation of the gap against the recorded 96-126, and the probe's own four overlapping
seasons read 171-283, which excludes grid. Relabelled everywhere as whole-field; the post-screen
recount is queued.

### 6. A superseded claim left standing in the file that withdraws it — APPLIED

Found by findings-audit. `transfer-policy.md` still asserted the pool-*nesting* generalisation in
bold, 80 lines above the subsection withdrawing it, and the section lead still told readers not to
quote a reconciliation now sitting below it. Both fixed; the withdrawn sentence is struck rather than
deleted.

### 7. The `194c654` config hazard — APPLIED, with the reviewer's reason corrected

stats-review raised a third candidate cause I had not considered: that run read a hardcoded macOS
path and discarded the error, and `config.Load` returns `Default()` alongside an error. Verified the
mechanism exists (line 125 at that commit). **But the reviewer closed it on a wrong ground** —
"`config.json` had no `review` key". The field is tagged `review_policy` and the file carries it. I
closed it on the correct ground instead: every numeric scoring weight in `config.json@194c654`
equals `DefaultWeights()@194c654`, and `Review` reaches only the transfer path while this is a HOLD
result. Recorded that way, with the bad reason named.

### 8. Smaller, all applied

- Line-number citations: `simulate.go:996-1001` → the `minExp` switch in `openingSquad`;
  `squad.go:456-460` → `analysis.CutByExpectedMinutesFloor`; `metrics.go:470` → `DefaultRestPlayers`'
  own comment (470 is inside the AFCON data literal, and three files agreed on the wrong line).
  Where possible the fix names the consumer instead of a line, which is what the standing rule asks.
- The reach snapshot's "setting `MinExpectedMinutes` to 0" prose footnoted — 0 means the shipped 55.
- Owed instrument work moved out of the checked-off item into an open one, with two additions from
  the reviews: recount post-screen, and print `XIValue` for both squads so the 2021-22@11 cell says
  *which* run missed the argmax.
- Block J's own source comment corrected — it was a fifth uncorrected instance of the unit error.
- "Run independently" softened: the probe and the replay share `Optimize`, the engine and the
  archive.
- `floor=65`'s moved cells are **five up and three down**, not four and four as the review said.

## Found by the invariant, not by a reviewer

`TestDiagFloorPopulation`'s first version hardcoded `SettledMinutes < floor && Price > 4.5` — a
second copy of `squad.go`'s pool filter, which CLAUDE.md names as the worst place for that failure
because a diagnostic is what everything else is checked against. Caught by asking the gate's opening
question. Fixed by extracting the predicate; the probe re-ran **byte-identical on all 36 rows**.

## Declined

- **Restore the `FPL_WEIGHT=prior_half_life` parenthetical to CLAUDE.md** (stats-review §4).
  Declined: the two reviewers disagree, and findings-audit verified the destination —
  `harness-and-inference.md:30-45` carries both fields, the non-consumer, the `cmd/priorblend`
  condition and the wrong-tree scan, in more detail than the parenthetical did, and says it was moved
  there on 2026-08-13. The surviving rule still ends "Worked case in [harness-and-inference]", so
  nothing dangles. The byte budget also forces the issue: the file sits at 65516 of 65536.
- **Expose `Optimize`'s pool so the probe can count post-screen directly.** Deferred, not refused.
  The pool loop is a single contiguous block entangled with locks, exclusions and price overrides;
  extracting it is a real refactor and well outside a reconciliation task. Queued instead.
- **Two commits so history witnesses the pre-registration preceding the run** (stats-review). Partly
  declined: the registration is in the same commit as the results, so it is **self-attested** and is
  now labelled as such rather than presented as externally witnessed. Adopting the two-commit rule
  going forward is worth doing and is not retrofittable here.

## Round two — `4f0c3f9..3bb17cc`, the argmax question

`ffac3fb` added `Engine.ObjectiveFor` and used it to ask whether the optimiser returns its own
argmax when the pool is widened. **fpl-code-review** ran on it (triage: `internal/analysis`, and the
diff is small with a load-bearing conclusion). fpl-stats-review was **skipped** — the output is a
count with no estimator, no threshold and no p, so there was no inference to review.

### 1. "0 of 36" retracted; the informative denominator is 1 — APPLIED

The reviewer's central finding, verified independently before applying. **34 of 36 cells have
`differs = 0`**, which means the two squads are the same *set* — `Optimize` keys `selected` by id, so
fifteen matching distinct ids is set equality — so their objectives are equal **by construction**. A
35th has `entered = 1` and the guard correctly excludes it. **One cell discriminates.**

This is the record's signature failure — a cell where the intervention could not run is not a tie —
and the probe's own summary applies that rule correctly to `moved` two paragraphs above the line that
broke it. Restated everywhere as **1 of 1 discriminating build**, and **fixed in the probe** rather
than only in prose: it now prints `canTest`, quotes that denominator, and says outright that the run
establishes nothing if `canTest` is zero. Otherwise the next run re-makes the claim.

The reviewer's framing of what the counter measures is right and is now recorded: it tests **search
monotonicity under pool widening**, not the shipped search's optimality. A regression tomorrow would
print a clean result in every structural-zero cell.

**The other direction survives untouched and needs no denominator**: a strictly larger pool returning
a worse optimum convicts the search on any cell where it happens.

### 2. Two causal over-readings — APPLIED

"The −102 is the floorless arm's search failure" is an attribution a 0.13% objective difference
cannot support, and the reviewer noted my own two files disagreed about it. The sound claim is
weaker and sufficient: the −102 is **not evidence about what the floor is worth**, the squads being
objective-equivalent to one part in eight hundred. And §6(b)'s "the optimiser maximises `XIValue`"
was wrong and unmarked — `XIValue` is the *transfer* search's function and ignores the bench. Both
corrected in place.

### 3. Three duplications of the class the review was hired to check — APPLIED

- **`ObjectiveFor` promised "every parameter from req" and dropped `StartIDs`.** With forced starters
  it would score a squad on the free best eleven — reliably higher than the eleven the search
  climbed, reading as the search leaving points on the table in every cell. Now resolves it via a
  shared `mustStartSet`.
- **The `BenchWeight` fallback was implemented twice**, inside the function whose doc comment cites
  that exact failure as its reason to exist. Extracted to `Engine.resolveBenchWeight`.
- **The `0 → 55, < 0 → 0` floor switch was a third copy** — and the one deciding which arm is
  "shipped". If the default moved to 60 the probe would keep building the 55 arm and read as a
  perfect reproduction. Now `SimConfig.resolvedMinExpectedMinutes`, called by `openingSquad` and the
  probe. `unified.go` keeps its own because it reads a different config field.

All three verified bit-identical: the probe's two moving cells reprint −0.0799 and +0.0044 unchanged.

### Confirmed sound, no change needed

The reviewer traced and confirmed all four mechanics the conclusion rests on: `ObjectiveFor` is the
function the search climbs (`dpseed.go:419`/`:469` and the seed ranking all reach `objectiveWith`);
the cold and warm scratch paths provably agree, since `benchslots.go` reinitialises `dist` and zeroes
its accumulator; the two `Optimize` calls differ **only** in `MinExpectedMinutes`, so the floored
pool is an exact subset and legality is a property of the squad rather than the run; and residual
non-determinism is ruled out by ID-ordered iteration plus `TestSeedOrderIsDeterministic`. It also
confirmed that computing `removed` over `e.AllMetrics()` cannot widen `entered`, since `entered` only
tests players already in the returned squad.

### Declined

- **Nothing.** Every finding in round two was applied. Recorded explicitly because a review with no
  declines is unusual here and the reason is specific: all seven were about the *claim* or about
  duplication, and none proposed a change to what the program computes.

## What could not be checked on this harness

- **Whether the 2026-08-08 figure was correct when it was made.** The data state and five days of
  code both moved. Restoring the data switches does not restore the code state, so no available arm
  separates them. Recorded as a failed reproduction on the current tree, explicitly not as a claim
  about the original run.
- **Whether the floor is worth anything at settings or configurations other than those tested.**
  Every null here is a simple-effect null: shipped bench shape, no chips, bespoke search.
- **The points value of the floor, at any effect size.** This is the §2 finding: with 2 of 6 seasons
  moving, the instrument has zero power rather than low power. Unmeasurable on this harness, which is
  a statement about the instrument — the constant still has a right answer.
