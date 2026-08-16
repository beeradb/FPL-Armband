# The xPoints instrument prices xG and xA through a per-position conversion scale

Branch `xpoints-position-scale`, off `origin/main` at `2ebd2ae`. Reviewed before the commit, per
the standing rule that a review record rides with the change it reviews.

## What was reviewed

`XPointsResidual` differenced realised goals and assists against **raw** xG and xA. A raw xG is not
an FPL goal and a raw xA is emphatically not an FPL assist — FPL pays an assist for a won penalty, a
parried shot and a deflected pass, and a forward's assists/xA runs **2.13 / 2.08 / 2.11** across the
three native seasons, definitional rather than noise. Differencing against an unscaled expectation
therefore measured conversion luck *plus a fixed per-position offset*.

⚠️ **The motivation is NOT "the engine applies `scaleFor(pos)`, so mirror it."** That argument is
the one a reader reaches for and it does not survive: in the replay the engine's own scale is
neutral on DEF-goals and GKP through the opening block and on FWD-assists to GW10-20, goes live for
MID/FWD goals from GW2-3, and at GW1 is the *prior* season's ratio via `PreSeasonWith`. There is no
stable target to mirror. The fix stands on the definitional argument above.

Why it mattered: `perfectGateXPoints` sums `in − out` **across** positions, so the record's rule
that a position-shared bias is not an ordering error does not protect it. The defender-minus-forward
gap is 0.37-0.40 per appearance.

## Reviewers

| reviewer | why |
|---|---|
| **fpl-stats-review** (on the PLAN, before any code) | the change produces numbers; the standing rule is to review the plan |
| **fpl-stats-review** (fresh instance, on the implementation) | to check the plan's rulings were applied rather than softened |
| **fpl-code-review** | touches `internal/analysis` scoring and `internal/backtest` |
| **fpl-findings-audit** | edits `CLAUDE.md` and a long code-comment record |
| **private-store-audit** | the queue item's closure and the supersession marks |

Skipped: **fpl-security-review** — no network, no credential, no config persistence, and the one new
field is `json:"-"` so nothing new reaches the disk cache. **fpl-docs-accuracy** — `docs/` untouched.
**fpl-run-review**, **fpl-season-maintenance** — not applicable.

## Findings, ranked by how misleading the state was

### 1. The fit mismatched its numerator and denominator wherever xG coverage is partial (CONFIRMED, fixed)

The first implementation summed goals and xG over every row with minutes. A row the archive records
no xG for contributed its goals to the numerator and nothing to the denominator.

At shipped config this is exactly harmless — **zero** goals sit on a zero-xG row in any season or
position. Under **`FPL_NO_XG_REPAIR=1`**, which `CLAUDE.md` makes a *required* switch for
reproducing anything recorded before 2026-08-10, 2022-23 records native xG only from GW16 and the
fifteen blind gameweeks keep their goals:

	2022-23, repair off    fitted    coverage-matched    goals on zero-xG rows
	  DEF                  1.0002        0.6974                  33
	  MID                  1.5394        0.9703                 217
	  FWD                  1.4189        0.9211                 120

Nothing errored, no scale was zero, the panic guard never fired and the in-sample identity still
held season-wide. Reproduced independently before fixing.

**Applied**: `underlyingCoverage()` gates accumulation per (gameweek, channel) on whether the season
records the column at all. ⚠️ **Deliberately NOT a per-row `XG > 0` test** — 8 to 27 assists a
season sit on genuinely-zero-xA rows at shipped config (a won penalty really does carry ~0 xA), and
gating those would move the shipped forward assists scale by 5-14%. Same shape as `DefconScoredIn`.

### 2. Three claims stated more strongly than the evidence (applied)

- **"Paired within-cell differences are unaffected"** — false, and the most dangerous line in the
  change. The correction is proportional to each player's own underlying, not a per-cell constant.
  `+85.4` *is* a paired within-cell difference and is in the superseded list. Reworded: paired
  differences stay a comparison of **one metric**; they are **not numerically unchanged**.
- **"moves the criterion by 1.2-1.9 points"** — 2-3x too large, and in the direction the change
  argues for. It reused the residual head start and the original head start as if either were the
  delta. Corrected to **≈0.7 over five gameweeks** (0.59 / 0.93 / 0.63 by season). The follow-on
  "same order as many packages' distance from the threshold" rested on the inflated figure and on a
  distribution nobody has produced; withdrawn.
- **"exactly zero by arithmetic"** — true only where neither the thin-sample floor nor the
  `[0.5, 3.0]` clamp binds. Scoped to DEF/MID/FWD, and the keeper exception is now asserted by test
  rather than described.

### 3. `FPL_CS_XGC_FACTOR` named as the fix for the clean-sheet half — wrong mechanism (applied)

That knob is an *engine* scoring term, read at `teamstrength.go` and three sites in `metrics.go`.
**`xpoints.go` never reads it**, so no setting of it moves this residual. Replaced with the named
mechanism a hundred lines below: the Jensen gap, measured at 1.27 on two independent providers.

### 4. A duplicated scoring constant that re-created a bug the record had already caught (applied)

A test helper returned 6 for a keeper's goal against `goalPoints[1]` of 10 — byte-for-byte the
`GOAL[1]` divergence `TestTheXPointsScriptsShareTheScoringTable` exists to have caught once. It
survived for the same reason as the original: no keeper in the fixture. Deleted; the identity is
invariant to the multiplier, so the local copy guarded nothing.

### 5. Two tests could not fail on what they were named for (applied)

Both mutation-proved by the code reviewer. The fixture gave all ten players of a position identical
rows, so "every player carries the same scale" was true under a per-player fit too; and every row's
residual was individually zero, so cross-row cancellation was never exercised. The fixture now
carries two profiles per position whose aggregate is the target and whose rows are ±1.0.

### 6. Dead identifier, and a supersession notice missing from the primary site (applied)

`TestTheConversionScaleIsArmInvariant` did not exist; two comments cited it as the guard for the
load-bearing safety argument. Repointed to `TestTheConversionScaleIsSeasonGlobal`.
`internal/backtest/gatexpoints_diag_test.go` — the file a reader lands on when they grep the
identifier — carried every superseded figure unmarked. It now leads with the notice.

## What supersedes what

**Superseded**: the gate arm's `+2.2461` pts/gw on realised UNDERLYING (×38 = 85.35 — the record
spells it both 85.3 and 85.4, so quote the pts/gw), the `0.645` recovered fraction and its Fieller
CI, `AxisTransferGateResidual`, all three `policy_xpoints` figures, the −94.8 and +43.6 contrasts,
and the xppilot `hold_xpoints`/`policy_xpoints` SE figures — **whose verdict is untouched**, resting
on *removing variance removes signal*, which a rescaling does not reach.

⚠️ **NOT superseded: the points arm's `+132`.** It calls `pointsOver` and never reads the
instrument. It is the **confinement check** for the re-run and must return byte-identical.

## What was applied, and what was declined

**Applied**: all of findings 1-6 above, plus the `stats/xpoints_guard.py` omission from the Python
docstring enumeration (it is the fifth caller), the season-scoping of "zero keeper goals", the
per-appearance assumption on the five-week head start, and the LOSO question recorded as open.

**Declined, with reasons:**

- **Re-running `AxisTransferGateXPoints` before this lands.** Declined: the fix is a correctness
  change and ships on that. The re-run is named as owed, the confinement check is named, and until
  it lands the surviving reading is labelled a hypothesis rather than a measurement. Landing the fix
  first also means the re-run measures the instrument we intend to keep.
- **A leave-one-season-out scale.** Declined for this change and **recorded as open**. It would
  answer whether the ±0.023/appearance of season-common shock is worth paying to remove
  0.040/appearance of bias — a real question the change creates, but a different one from the
  correctness defect being fixed.
- **Restating the mechanism in `CLAUDE.md`.** Declined and actively *reverted*: a first version
  added 1.9 KB of which two bullets restated what `xpoints.go`, `season.go` and
  `stats/xpoints_common.py` now carry at length. That is a mirror paid for out of the resident
  budget. Cut to the supersession notice and its qualifiers, ~890 bytes.

## What could not be checked on this harness

- **Whether the change improves anything.** It is a correctness fix; no points claim is made and
  none is supported. `CLAUDE.md`'s "correcting a measured bias has lost points five times" applies.
- **The ordering pin.** `TestTheConversionScaleFollowsTheXGRepair` verifies the stored scale matches
  the season's post-repair rows, but **cannot** catch the ordering itself on a synthetic fixture:
  the xG repair fills from a harvest keyed on real player codes, so it moves no minutes-bearing row
  and the arm contrast skips. The test says so out loud and the comments that cite it now say so
  too. Pinning the ordering wants a real archived season through `Load`.
- **The size of the remaining bias.** The clean-sheet channel carries roughly two thirds of the
  cross-position gap and is untouched here.

## A finding from outside the review, recorded because it is larger than this change

The rule this change worked *around* — "a bias shared by every player in a position is not an
ordering error, because the optimiser consumes an ordering and FPL forces five defenders regardless"
— **does not hold**, and the user raised it rather than any reviewer.

`Optimize` is a knapsack against one shared budget, not an ordering consumer. Checked directly:
running it at `f=1.0` against `f=1.173` over five budgets moved the decision in **3 of 5**
landscapes and the **formation in 2 of 5** — at £95m, correcting the defenders' over-prediction took
a defender *out of the fielded eleven* (4-4-2 → 3-5-2). The squad quota fixes five defenders; it
does not fix how many play, and only the eleven scores (`internal/analysis/squad.go:52-54`).

⚠️ **This shows the decision MOVES, not that correcting improves it** — a different claim, and this
record has "a measured bias does not imply a correction exists". What it retires is the
*justification* for shipping the clean-sheet over-prediction uncorrected, which now rests on nothing
rather than on a sound argument. Deciding it needs a replay. **Not folded into this change**: it
touches a standing rule and a live scoring decision, and wants its own review.

## Redaction note — 2026-08-15

One cell of the reviewer table above was edited after this record was filed. The reviewer's name
identified a private store this repository may not name; it now reads **private-store-audit**. The
row is otherwise unchanged and no finding was touched — that reviewer ran, and on the scope stated.

This edits a dated attestation, which the standing exemption for already-committed disclosures was
meant to prevent. The user ruled that exemption a grandfather clause over an enumerated set only,
and this file was found afterwards, so it is cleaned at the acknowledged cost of amending a dated
record. The note exists so the amendment is itself attested rather than silent.
