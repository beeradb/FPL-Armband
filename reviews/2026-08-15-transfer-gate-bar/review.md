# Settling whether the 0.89 transfer-gate bar is grid-dependent

**What was reviewed.** The working tree of `transfer-gate-bar-and-anti-residual`, branched at
`63eff6f` and rebased onto `origin/main` — an arithmetic settlement, off banked cells with **no
sweep**, of whether `sig_season/perfect` = 0.89 is a property of the four-season comparison it was
computed on or a bar that transports. Seven files: `CLAUDE.md`,
`stats/snapshots/2026-08-15-gatescaled/FINDINGS.md`, comment-only edits to
`internal/analysis/xpoints.go`, `internal/backtest/gate.go` and
`internal/backtest/gatexpoints_diag_test.go`, a comment block added to
`internal/snapshot/retracted_test.go`, and this record.

**No behaviour changed, and nothing measured.** Every Go edit is inside a comment. No sweep ran, no
banked figure was altered, and the only new number in the change — the 51.3% season share in
`FINDINGS.md` — is arithmetic over `stats/snapshots/2026-08-15-gatescaled/cells/gatescaled.csv`,
re-derived in this session rather than carried across.

## Reviewers

| reviewer | when | why |
|---|---|---|
| **fpl-stats-review** | on the settlement, before this session | it produces a number and re-grounds a closed line; nine findings, four blocking, all applied on the branch |
| **the documentation reviewer** | round 1 on the branch's prose, round 2 on the finished diff | `CLAUDE.md` and a banked `FINDINGS.md` were both rewritten |
| **fpl-findings-audit** | round 1 on the branch's claims, round 2 on the finished diff | a closed line's *reason* was replaced, which is exactly this reviewer's remit |

⚠️ **Named by function, not by registry entry.** The documentation reviewer's registry name is not
one of the eight agents tracked in `.claude/agents/`, and a review record naming it would put a
fact about an off-repo tool into the repository. The forward rule was set by
`reviews/2026-08-15-gate-rerun/review.md` after the same leak was caught there.

**Two rounds, and the second was not a formality.** Round 1's findings were applied before this
session; round 2 ran over the *finished* diff and returned **nine further findings**, three of them
correcting claims round 1's fixes had introduced. Findings 11-16 below are round 2's, and one of
them contradicts the brief this session was given — see finding 11.

**Skipped, with reasons.** **fpl-code-review** — no behaviour changed: every Go edit is a comment
or a comment block, verified mechanically across the whole staged Go diff, so there is no
executable diff for it to review; and the one thing it would have flagged, the budget constant, was
dropped rather than merged. **fpl-security-review** — no `internal/agent`, `internal/fpl`,
config-persistence or cache change. **fpl-run-review** — no live run. **fpl-season-maintenance** —
the four hand-maintained lists are untouched.

## Findings, ranked by how misleading the state was

### 1. The re-grounding over-read its own new ground — BLOCKING, applied

The branch had already replaced the withdrawn 94/106 ratio with four rows from the swept gate
family, and left them reading as four equivalent supports. They are not equivalent, and read
properly they support a **narrower** claim than the one they replaced:

| row | what it actually is |
|---|---|
| `min_gain` byte-identical at or below 0.4 | **an invariance** — 12 cells and again at 36; a fact about the code, since the charge clause already demands `charge/horizon` = 0.4. The strongest row by a distance |
| the floor at horizon 8, −15.8 against 34 | **a tie.** Fails to reject; one season carries 68% |
| the horizon arm, −8.4 against 21.7 | **a tie.** Fails to reject |
| 0.7/0.95/1.30 monotone harmful | **24 cells on four seasons, no threshold recorded** — the grid `CLAUDE.md`'s own *"sweep transfers on six too"* calls wrong for a transfer constant, four of whose eleven refuting arms were `min_gain` itself |

The source bullet for the two middle rows is headed *"crossing them resolved nothing"*, so under
*a null is a tie* they support the closure **only by failing to refute it**.

**Applied.** The closure is now written as **"stop sweeping the transfer gate: nothing swept in this
family has resolved"**, which is narrower than the withdrawn *"nothing in this family can resolve"*,
and it names which row is the invariance and which two are ties. In `CLAUDE.md`, in the
`perfectGate` doc comment in `internal/backtest/gate.go`, and in the research record.

### 2. The consequence had to reach the resident file — BLOCKING, applied

The correction's *direction* had been recorded in a test comment and in the record but not in the
file every session loads. It matters, because it runs **against** the closure it supports: 94 is the
**perfect** arm's threshold, and a perfect gate replaces the squad far harder than a `min_gain`
nudge — which by `CLAUDE.md`'s own *footprint predicts a paired SE only through path divergence* is
what drives a paired SE. Charging a hypothetical constant 94 charges it too much.

**Applied.** `CLAUDE.md` now says in its own voice that **gate constants are MORE resolvable than
the withdrawn reason claimed, not less**: the family measures 11-34 a season and the two arms with
thresholds of their own carry **34 and 21.7**, not 94 — *at the edge of this instrument*, not *out
of reach of it*. The closure is **re-grounded, not reopened**, on "an oracle is not a constant".

### 3. The reconstructed-xG caveat was wrong, and its defence cannot reach the case — applied

`FINDINGS.md` said the two joining seasons "carry the underlying arm's two largest gains and 27% of
its six-season level". **27% is the DROP**, 84.7 → 61.9, when both are removed — not the share.

⚠️ **And leave-one-season-out cannot discharge this caveat.** Every LOSO subset is a *five*-season
subset and so retains at least one of the two joining seasons; the leave-**two**-out case is the
four-season row itself, which does not clear. The six LOSO subsets establish that no *single* season
carries the arm and **nothing whatever about the pair**. The caveat is therefore **open**.

**The share figure was re-derived rather than accepted.** The review offered "~51%" without a
generator, and a figure with no generator is what this project's *one quantity, two implementations*
rule is about. Re-derived here from `stats/snapshots/2026-08-15-gatescaled/cells/gatescaled.csv`:
**51.3%**, on the equal-weighted mean of season means, which on this balanced design is the same
estimator as the pooled cell mean — both give 2.2294 pts/gw, matching the banked figure. Per-season
levels ×38: 2020-21 +101.7, 2021-22 +158.9, 2022-23 +46.2, 2023-24 +26.5, 2024-25 +40.2, 2025-26
+134.8. The 51.3% and the 27% are one fact: leaving 48.7% of the sum over 4 seasons instead of 6 is
0.487 × 6/4 = 0.731 of the level. Both are now stated in `FINDINGS.md` with the estimator named.

### 4. Two Go comments attributed a movement to a cause the snapshot says is unidentified — applied

`internal/analysis/xpoints.go` read "0.696 on those same four seasons **post-repair**" and
`internal/backtest/gate.go` "**at a clean data state**". `FINDINGS.md` says the opposite: the run
factor is **not** a data-state effect alone — `gate.go` did not exist at the older leg, and
`oracle.go` and `season.go` both moved by hundreds of lines, so code and data moved together.

**Applied.** Both now read: *"0.696 on those same four seasons out of the later bank (`82fc8e0`,
clean) — a different run, not a data-state effect; the channels are not separated."*

### 5. "Untestable in both directions" misused a reserved word — applied

The Fieller test at θ₀ = 0.414 **ran**, gave t **+2.051** against `t_crit` 2.571 at df 5, and failed
to reject. `S_eff` is 6/6, so nothing is floor-bound. That is **unresolved, not unmeasurable** — and
the distinction is load-bearing here, because an unresolved question is still owed an answer if the
instrument improves. Corrected in `FINDINGS.md`.

### 6. A section heading asserted the reading its own body withdrew — applied

`FINDINGS.md` §"The fraction rejects 0.89 — and that, not the 50% straddle, is what it decides"
survived a rewrite that withdrew exactly that reading. Retitled to **"The fraction is 0.64 with an
upper limit of 0.813 — an information statement, not a bar"**, with a one-line in-place retraction
marker beneath quoting the old heading. In-place markers are correct in that file: it is a dated
banked artefact, and the verdict-only rule below applies to `CLAUDE.md` alone.

### 7. Citations by line number, in two stores, already stale — applied

`FINDINGS.md` cited `gatexpoints_diag_test.go:433` and the research record cited `:432` — the call
and the string on the following line, so one arm already had two "correct" citations. **This
session's own comment edits then moved the call from 432 to 441**, falsifying both. Now cited by
identifier and variant label — `Oracles{Decision: AxisTransferGate}`, `"perfect acceptance"` in
`transfergateoracle_test.go` and `"perfect on POINTS"` in `gatexpoints_diag_test.go`. Same treatment
for the other leg, which was cited by line, was correct at the time, and is equally fragile — it is
now named as `oracleVariant(Oracles{Decision: AxisTransferGate}, "perfect acceptance", nil)`.

### 8. "Arm is not the confound" was bare where the body qualified it — applied, and strengthened

Retitled to **"Arm is probably not the confound — verified for the newer leg, inferred for the older."**
The "twelve minutes"/"seven minutes" duplication is collapsed into one sentence naming all three
anchors: `0102d0d` 17:55:29, the run 18:00:33 (from its `run_id`), `f9591b1` 18:07:17 — so the run
provably falls between the commit it stamps and the commit that first contains the arm.

**Strengthened beyond what was asked**, and verified here: **both** predicates the arm runs on,
`perfectGate` **and** `pointsOver`, are byte-identical from `f9591b1` to HEAD — not just
`perfectGate`. So the acceptance rule *and* the realised-points window it is judged over have both
been fixed since, which is a better inference than the one that was written.

### 9. Smaller, all verified — applied

- `FINDINGS.md` said the threshold rises "57.8 → 78.4" four lines below its own **57.7**. Now 57.7.
- The table header says "df 5.0 throughout"; the four-season rows added beneath it are **df 3**,
  `t_crit` 3.182. A marker now says so, because a threshold from one is not comparable with the other.
- "monotone harmful **above** 0.4" over-generalises three swept values — now "at the three values
  swept above it, 0.7/0.95/1.30", in both stores.
- `gatexpoints_diag_test.go` cited "the four-season 2026-08-10 bank", which is not a directory that
  exists. Now `stats/snapshots/2026-08-12-4d61058/cells/oraclegate.csv` (commit `0102d0d`, **dirty**;
  the directory is dated by when the cells were banked, not when they ran).

### 10. A deliberate absence from the retraction guard, now recorded — applied

No entry was added to `retractedFigures` in `internal/snapshot/retracted_test.go` for 0.89 / 94 /
106, and **that is correct**: 0.89 is still quoted legitimately — in the surviving Fieller rejection
and in the sentence that does the retracting — so an entry would fire on the correction itself; and
94 and 106 collide with unrelated live quantities, "+106 `HOLD`" being the doubles contamination
event, with no `context` word able to separate them. That file's convention is to **record**
deliberate absences so the next auditor does not re-derive them, so a comment block in its existing
style now says this, beside the one already there for the three bench-shape figures.

## Round 2 — findings against the finished diff, three of them against round 1's own fixes

### 11. "not a data-state effect" is contradicted by the banked cells — applied, AGAINST THE BRIEF

The brief for this session prescribed the exact wording *"a different run, not a data-state effect;
the channels are not separated"* for the three Go sites, on the ground that `FINDINGS.md` says the
run factor is not a data-state effect. **It does not say that; it says "not a data-state effect
*alone*", and the missing word is load-bearing.** Round 2 reproduced the cell-level diff over the 24
shared cells: only **2022-23 (6 of 6)** and **2025-26 (1 baseline, 3 arm)** differ at all, and those
are precisely where the documented 2026-08-12 archive repairs land — 2023-24 and 2024-25 are
identical to the point. So the code churn has **no observable effect on the twelve cells where it
could have acted alone**, and the data state is the only channel with a positive signature.

**Applied, deviating from the prescribed wording**: all three Go sites now read "not a data-state
effect **ALONE**; the channels are not separated", matching `FINDINGS.md`'s own correct form. The
brief's intent — stop attributing the movement to an identified cause — is preserved; its literal
wording would have installed the opposite over-claim in three files at once.

### 12. "the two whose xG is reconstructed" is false, and correcting it STRENGTHENS the caveat — applied

`stats/gate_recovered_fraction.py`, run on this bank, prints **three** backfilled seasons, and
`xgRepairs` in `internal/backtest/xgrepair.go` lists `2022-23: {FirstGW: 1, LastGW: 15}`. So
2022-23 is reconstructed for GW1-15 **and** reads 2021-22 as its prior season. The four-season row
is therefore **not a reconstruction-free control**, which means the caveat is open for a stronger
reason than was written: **no subset of this archive holds the reconstruction out**, and the split
is collinear with era. Corrected in `FINDINGS.md`, with the script named.

### 13. "the two largest gains" is false — largest and third — applied

Per-season underlying levels ×38: 2021-22 **+158.9**, 2025-26 **+134.8**, 2020-21 **+101.7**,
2022-23 +46.2, 2024-25 +40.2, 2023-24 +26.5. The joining pair is **1st and 3rd**; 2025-26 is second
and was already inside the four. The same document already had this right fifteen lines below for
the perfect-gate arm, so this was the corrected sentence's uncorrected twin. Now stated as
"largest and third-largest" with all three figures.

### 14. The verdict-only cut orphaned 0.89 and 94 in `CLAUDE.md` — applied

Deleting the derivation left the resident file rejecting **0.89** without ever saying what 0.89
*is*, and quoting **94** once, unglossed, in a gate context — so the only inference available to a
reader was the withdrawn one. That is a live re-inflation route created by the cut itself. 0.89 is
now glossed in place as *"the four-season `sig_season/perfect` ratio, named here only as the value
tested"*, and 94 is named as **the perfect arm's threshold** with the reason it is not a
constant's. The non-rejections of 0.50 and 0.414 are stated beside the rejections, so the bullet no
longer lists only the results that point one way.

### 15. The new bullet was a second copy of four figures thirteen lines above — applied

−15.8/34, −8.4/21.7 and 0.7/0.95/1.30 all appear in the bullet immediately preceding, and the
12-and-36-cell byte-identity a third time in the weekly-transfer section. Three copies of one
quantity in a budgeted file is the drift this record has a rule about. The bullet now names the
rows by **role** — one invariance, two ties, one gap — and cites the bullet above for the figures.
This is why `CLAUDE.md` ends up smaller than round 1 left it.

### 16. Two smaller corrections that round 1 introduced, plus one it inherited — applied

- **"nothing swept in this family has resolved" over-read the ladder row.** Its threshold was never
  computed and its cells were never banked, while its bottom rung is −0.866 pts/gw = −32.9 a
  season, squarely at family scale. So that row's status is **unknown, not non-resolving**. Now
  "nothing swept in this family is **recorded as** having resolved", with the row named as a **gap
  rather than a null** in all three stores.
- **`xpoints.go` still called the 0.89 rejection "the load-bearing half"** while `FINDINGS.md`
  withdrew exactly that phrase and `CLAUDE.md` reframed it. Three readings of one withdrawn claim
  in three files is the drift the verdict-only decision exists to prevent. Rewritten to lead with
  the information statement.
- **`gate.go`'s dismissal of its own three FLOOR caveats was orphaned.** They were dismissed
  because the conclusion "rests on the ratio rather than on the level" — and the ratio is now gone.
  They are still harmless, for a better reason: the re-grounded closure never reads that arm's
  level. Said so in place.
- **The LOSO limitation is stronger than written.** The leave-two-out case drops two *clusters* as
  well as two seasons, so its threshold rises ×1.24 on `t_crit` alone; it could not have discharged
  the caveat even had it cleared. Added.
- **"four of whose eleven refuting arms were `min_gain`"** is a misreading of the record's own
  10-of-11. It is four of the **ten refuting** arms, out of eleven. Fixed.

### 17. Rebased onto the concurrent compaction, which had rewritten the same two bullets

`origin/main` moved three times during this session; the last move merged the branch compacting
`CLAUDE.md` on the verdict-only convention, cutting it from 105,655 B to 79,144 B. **That branch had
rewritten both bullets this change touches, from the pre-settlement state** — its *"stop sweeping"*
still carried 94/106/89% with the suspension attached, and its fraction bullet still said *"the
0.89 bar is itself grid-dependent and unverified… this suspends the 89% bar until someone
re-derives it on six"*.

Resolved by taking **`main`'s compacted wording as the base** and replacing only the two suspensions
with the settlement, in `main`'s style. Two consequences worth recording:

- **The duplication finding resolved itself.** `main` had already merged the floor and horizon arms
  into a single `min_gain` bullet carrying all four rows, so the re-grounded closure now cites that
  bullet instead of restating anything — which is what finding 15 asked for, arrived at from the
  other direction.
- **Nothing of `main`'s compaction was reverted.** The rebase discarded this branch's copies of
  five neighbouring bullets that `main` had already rewritten more tightly, rather than
  reinstating them.

### 18. A snapshot was regenerated, and it records WHEN not WHAT

`stats/snapshots/2026-08-15-b2a9334` is banked with this change. The staleness gate fired because
`internal/analysis` and `internal/backtest` moved — **by filename, not by executability**, which
that gate's own comment calls out: a comment-only edit moves the digest. So this snapshot records
when it was taken and **not** that the model behaved differently; nothing in this change can move a
figure, and the diff against `2026-08-15-ecebfcc` should be read that way.

Generated per the recipe, with its three recorded traps avoided: `-count=1` (without it a second
invocation returns `ok (cached)`, writes nothing and exits 0, banking `model.present,false`); a
**session-unique** CSV path passed to *both* halves, since the renderer reads a `-model` flag and
not the environment variable, and this record's own warning is that a shared `/tmp/model.csv` on a
shared machine silently reproduces another run's figures; and `TestDiagTransferError` included.
Verified after the run rather than assumed: **`model.present,true`, 555 model rows, all 8
`model.transfer_error.*` rows present.**

## Note on the two stores, and an axis amended

`reviews/2026-08-15-gate-rerun/review.md` fixed the axis for a banked `FINDINGS.md` as **run
provenance, with the verdict and the reasoning kept out**, on the ground that a conclusion held in
two places drifts. **This change amends that axis, deliberately, and it should not be read as
ignorance of it.** The verdict-only decision removes the resident file's ability to hold a
retraction narrative at all, so the history has to land somewhere a fresh checkout can reach or it
is lost. The dated banked artefact is the only such place. The amended axis is therefore: **the
banked artefact takes the history of the run it belongs to** — including a closed line's
re-grounding where that run is what re-grounded it — while the resident file keeps the verdict. The
drift risk this raises is real and is answered by deletion rather than duplication: what
`FINDINGS.md` now carries verbatim is precisely what `CLAUDE.md` no longer carries at all.

## What was declined, and why

- **The audit's "~51%" share figure, as offered.** Declined as written and **re-derived** instead —
  see finding 3. A share figure has an estimator, and one arriving without a generator is the
  failure mode this project has a rule about. The re-derivation agrees, so the figure ships; had it
  not been reproducible, `FINDINGS.md` would have carried the 27% drop and the LOSO limitation only.
- **Raising the `CLAUDE.md` size budget.** The branch carried `const budget = 101 * 1024` in
  `internal/snapshot/notes_test.go`. `main` had **already raised the same constant to 106 KB**
  concurrently, and 101 KB is *below* `main`'s own `CLAUDE.md` of 105,655 B, so keeping it would
  have failed the test outright. The hunk was **dropped entire**, not merged. No new raise section
  was written either, because after the verdict-only cut there is no raise: `CLAUDE.md` is
  **80,428 B** on this branch, under the **108,544 B** budget. The branch's headroom sentence
  ("leaving 997 B, the ~1 KB every prior step left") is **deleted rather than corrected** — quoting
  a *difference* between two sizes is the form `main`'s own budget comment retracts by name, and
  round 2 caught this record reproducing it while declining it. ⚠️ **This bullet then had to be
  corrected four times**, as `origin/main` moved three times during the session and the last move
  merged the concurrent compaction that cut the file by a quarter. That is the argument for the
  rule rather than an exception to it: a size is a fact about one tree, a headroom is a fact about
  two, and one of the two was being rewritten by somebody else the whole time.
- **Annotating the withdrawn claims in `CLAUDE.md` rather than deleting them.** By user decision
  `CLAUDE.md` now carries **the verdict only** — no retraction narrative, no before/after — and a
  concurrent branch is compacting the file on that convention. So the 94/106/89% derivation and the
  interim "⚠️ UNVERIFIED … suspended" marker are **deleted** from the resident file. They are not
  lost: `stats/snapshots/2026-08-15-gatescaled/FINDINGS.md` now reproduces both verbatim and carries
  the whole derivation — the three banked bars with their shas, the `t_crit` 3.182 → 2.571 = ×0.81
  arithmetic, the ×0.59 / ×0.79 traversal, the cancellation algebra and the LOSO t's.
- **Carrying the ×0.59 / ×0.79 multipliers into `CLAUDE.md`.** Kept out entirely. The 2×2's fourth
  corner needs a dirty tree replayed, so they are path-ordered simple effects of one traversal and
  quoting them as channel magnitudes would be the *simple-effect null* mistake wearing a
  decomposition's clothes. They stay in the banked artefact, labelled as such.
- **Reopening the closed line.** Declined on mechanism. The correction lowers the bar a constant
  faces, but **an oracle is not a constant** — a perfect gate is an information bound, not a member
  of the family — so nothing here licenses another sweep.
- **Round 2's objection to "this family measures 11-34 a season."** The point is fair — 11-34 is
  `CLAUDE.md`'s **global** range for nearly every constant, not a gate-family measurement, and this
  record has retracted one borrowed range ("42-59") of exactly that shape. Declined here because
  the phrasing is **inherited, not introduced**: `gate.go` and `gatexpoints_diag_test.go` have both
  carried "the constants in this family measure 11-34 a season" since `f9591b1`, so narrowing it in
  the resident file alone would leave three sites disagreeing, which is the failure this change is
  otherwise fixing. It is a real defect and it wants its own change, touching all the sites at once.
- **Round 2's objection to using 57.7 rather than 57.8.** The underlying arm's threshold recomputes
  to **57.772**, so 57.8 is the better *rounding* and 57.7 is the banked table's truncation. Kept at
  57.7 anyway, and this record does not call 57.8 an error: the defect being fixed was that one
  document said 57.8 four lines below its own table saying 57.7. Internal consistency with the
  banked figure is the tie-breaker, and altering the table is forbidden — no banked figure may move.

## What could not be checked here

- **The four-season leg is not reproducible from any commit.** Its bank stamps `0102d0d` with
  `dirty true`, and neither `AxisTransferGate` nor `transfergateoracle_test.go` exists in that tree.
  The 0.886 bar therefore comes from a working tree that was never committed and is gone. That is
  the strongest single reason it cannot arbitrate a bar, and it is also the reason the arm claim for
  that leg is **inferred** — from `perfectGate` and `pointsOver` being byte-identical since the
  adjacent commit — rather than verified.
- **The 2×2's fourth corner** — six seasons at the older data state — is **unmeasurable**, not
  merely unmeasured, for the same reason. So grid and run cannot be separated into main effects and
  no interaction exists to estimate.
- **The reconstructed-xG exposure is open.** Removing 2020-21 and 2021-22 drops the underlying arm
  27%, they are 51.3% of the level, and no subset of this archive can hold the pair out and still
  clear — the leave-two-out case *is* the four-season row. Answering it needs an instrument this
  grid does not have.
- **Nothing here was measured.** No sweep ran; this settles which figure may be quoted, and produces
  no new evidence about football.
