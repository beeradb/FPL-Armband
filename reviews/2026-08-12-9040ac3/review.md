# Review record — decoding the BPS schedule, and the tackled channel

**Commits reviewed:** `b752a7d..9040ac3` on `worktree-bps-tackled-channel`, off `main`. One
commit of new work, `9040ac3`, plus the corrections this record covers. Named for the commit
that carries it, which is the convention `TestReviewCoversTheCurrentCode` enforces. Previous
record: [`2026-08-12-1a0f3f4`](../2026-08-12-1a0f3f4/review.md).

**What the change is.** Two diagnostics and one finding, no scoring change.

`TestDiagBPSSchedule` decodes the pre-2024-25 Bonus Points System schedule from the archive's
own component counts — least squares over ~33 floor-transformed features, then a hard
requirement that the integer schedule reproduce recorded `bps` exactly. `TestDiagBPSTackled`
uses the decoded coefficient to price the one 2026/27 rule change that 2025-26 cannot see.

The finding is not the one the work was aimed at. It was built to fill in MID and FWD; what it
actually moved was the **keeper** result, in the opposite direction to the recorded one.

## Process note: the statistics reviewer saw the PLAN, before any code

New standing process, set by the user this session: **for a model or points change the
statistics reviewer reviews the plan first**, and the code and claims reviewers then work off
that plan and its test cases. This is the first application and it paid for itself immediately.

`fpl-stats-review` did not speculate about the design — it reimplemented the measurement in
Python in under ten minutes. That:

- **refuted the design's biggest stated risk.** The plan's own worry was that the tackled
  penalty might not have existed, or existed at a different coefficient, under the 2015-16
  schedule — which would have made these seasons the wrong regime. It was exactly −1 in all
  three, proven by the reconstruction;
- **overturned the expected headline** before a line of Go was written, surfacing the keeper
  result the plan had no hypothesis about;
- **supplied a guard the plan had backwards.** The plan said "a result showing midfielders
  losing bonus is a bug". Wrong: keepers *must* lose, and so must a low-tackled defender. The
  correct guard is monotonicity in tackled/90 within each position, and that is what shipped;
- **caught a specification trap by hitting it.** Coding a *sibling* feature wrong — `saves` as
  `floor(saves/3)` rather than raw-at-2 — returns `tackled ≈ −1.05` at an 88%-exact fit that
  looks healthy. That is why the acceptance gate is "residual identically zero" and not "a good
  fit"; the plan's phrase "exact or near-exact" was deleted on its instruction.

Its figures were treated as a **cross-check target, not a substitute**. The Go implementation
reproduces them independently and exactly.

## Before the reviewers: what must this change NOT move?

Invariants first, per the skill's instruction that they have caught more here than reviewers
and are free.

| invariant | outcome |
|---|---|
| the integer schedule reproduces recorded `bps` on every played row | passes — **31,402 of 31,402, max deviation 0**, per season |
| every solved coefficient is an integer | passes — max `\|coef − round(coef)\|` ≈ 1.1e-13, against a 1e-6 gate |
| `award()` reproduces the recorded bonus from recorded BPS in each old season | passes — 23,679 / 22,467 / 21,790, no era-specific tie branch needed |
| the coefficient the arm applies equals the coefficient the decode recovers | **now pinned** — it was hardcoded; see defect 3 |
| adding `tackled` to the shared `bpsTotals` is inert in `TestDiagBPSRuleChange` | passes — re-ran it: GK +15.4% at rate 0.00, DEF −6.4%, MID +1.3%, FWD +1.9%, all matching the recorded figures |
| both diagnostics are run-to-run deterministic | passes — two `-count=1` runs byte-identical apart from elapsed time |
| a *nonzero* collinear feature is a hard failure, not a silent zero | passes — verified by injecting a duplicate column and a constant column; both hit the named `t.Fatalf` |
| `go build`, `go vet`, full `go test ./...` | clean |

## What the reviewers found

Two agents on the finished code — `fpl-code-review` and `fpl-findings-audit`, the pair the
triage table owes for `internal/backtest` plus `docs`/`CLAUDE.md`. Both verified by *running*
the diagnostics rather than reading the diff. Sixteen findings, seven of them real code
defects. All applied.

### Code defects, all fixed

| # | defect | why it mattered |
|---|---|---|
| 1 | the `tackled` column's presence check was **conditioned on `tackled` being present** | Every other column got a hard error; the one the whole arm is about got a soft branch. Reviewer simulated the archive renaming it: the test **passes**, printing every position at `+0.0%`, terciles at `0.00` with named edge players, the monotonicity guard passing vacuously on `0 < 0`. A complete-looking table asserting the rule change does nothing — the exact failure the file header claimed to guard. Now keyed on `tackledSeasons` membership, never on presence |
| 2 | the GK tercile block was **a season split wearing a tackled/90 label** | Keepers are tackled 0.00/90, so both tercile boundaries fall inside the tied block and the split is decided by the element tie-break — which, the key being season-major, orders them by season. The printed gradient was the three per-season GK shifts re-bucketed. Worse, the monotonicity guard was then asserting something about *season ordering*: had 2018-19 been the most negative rather than 2016-17, the test would have failed with "this is a bug rather than a result" on nothing at all. Degenerate cuts are now detected and skipped |
| 3 | the arm **hardcoded** `+ c.Tackled` instead of reading the decoded coefficient | One quantity, two implementations — this record's signature failure, arriving inside a diagnostic. A re-decode returning anything but −1 would move the coefficient table and leave every figure in the arm unchanged. Now `const tackledBPS`, asserted against the decode |
| 4 | a fetch failure **silently dropped a season** from the transferability table | The 2025-26 row is the entire measured basis for "the modern boundary is looser". A timeout would have left three old seasons reading as a complete comparison, with no log line. Now a hard failure |
| 5 | `%%` passed as a `%10s` *argument*, so it printed literally | cosmetic |
| 6 | `bpsSchedule()`'s doc claimed pooling three seasons identifies `goal_gk`; `solveBPS`'s says it cannot be measured at all | The second is what runs — **no goalkeeper scored in any of the three seasons**. Contradictory comments in a file whose selling point is that it is checkable |
| 7 | a future archive gaining a `tackled` column for 2025-26 would have demanded ~27 columns it lacks | fails loudly rather than silently; removed by the fix for 1 |

I introduced and removed one further defect before review: row swaps in the Gaussian
elimination were being used to permute **variable** indices. Row swaps reorder equations, not
variables — after full Gauss-Jordan the pivot for column *j* sits in row *j*. Caught by
inspection, confirmed correct by the reviewer.

### Claims defects, all fixed

The audit found the prose overstated in nine places while confirming every *figure*
reproduces. The three that mattered:

1. **"The joint five-change figure is unmeasurable on any archived season, permanently" was
   false**, and it foreclosed the arm that would settle the headline. These three seasons carry
   `clearances_blocks_interceptions`, `saves`, `penalties_saved` **and** `tackled`, so the two
   changes that actually move positions can be applied jointly and re-ranked on one population
   at big-chance rate 0.00. Non-additivity is the reason to *run* that arm, not a reason it
   cannot be run. Corrected to: the **full five-change** figure is unmeasurable (no season
   carries both the 2025-26 saves baseline and a `tackled` column), and the joint
   CBI-plus-tackled arm is **unmeasured, not unmeasurable**.
2. **"MID and FWD: RESOLVED, and it is above the bar" was arithmetically wrong against the
   section's own pre-registered bar.** That bar is 10%; MID reads +5.7-6.2% and FWD +6.6-7.9%,
   both *below* it. Only the within-position spreads (21.8 and 20.1 points) clear it — the DEF
   pattern. "Resolved" is also this record's harness word for clearing a detection threshold,
   which a complete enumeration with no standard error does not do. Reworded to "no longer
   blind, and the shape is the DEF shape".
3. **"+15.4% is NOT a floor" was too strong in one direction and too weak in another.** It
   remains a floor *in the swept saves rate*, which is all it ever claimed; what it is not is a
   bound on the **net** change. And the two arms are on different seasons under different
   schedules — the keeper being the position whose BPS baseline moved most between them — so
   they must not be netted. The pair is now labelled **unsigned, not cancelling**, with the
   direction (which a keeper being tackled ~never guarantees in any era) carried and the size
   explicitly not.

Also applied: "DEF confirmed and roughly doubled" performed the addition the same section
forbids and asserted an unmeasured cross-tabulation — the low-tackled-third-is-centre-backs
claim is supported by six names and is now labelled a hypothesis, with the CBI/90-against-
tackled/90 correlation flagged as computable here and unrun. Monotonicity is no longer cited as
evidence, since the diagnostic treats a violation as a bug and the direction is arithmetic. The
mis-coded-saves counterfactual is now given as ≈ −1.05 / ≈ 0.57 / 88% and marked as a
development arm not reproducible from the shipped test — the reviewer re-derived it at −1.0517
/ 0.571 / 88.5%, close enough for the argument and not for the decimals. "Pre-2024-25" replaced
an ambiguous "the old value" on the penalty-save claim, and the regime span is now stated as
established for three seasons and an announcement-reading for the other six. Three superseded
claims elsewhere in the record are annotated in place rather than left clean.

### What was declined

**Nothing was declined.** Every finding from both reviews was verified against the source or a
re-run and applied. That is unusual here and worth flagging as a possible sign the briefs were
well-scoped rather than that the work was clean — the code review found seven real defects in
a file I had already run successfully.

### Verified before applying

Per the skill's warning that a report is a set of proposals: the two structural claims were
checked directly rather than taken on trust. The GK-tercile-is-a-season-split claim was
confirmed against the printed edge names (2016-17 keepers in "lowest", 2018-19 in "highest")
and by the shifts matching the per-season figures. The renamed-column claim was confirmed by
reading the branch condition, which did gate `need` on `col["tackled"]`.

## What could not be checked on this harness

- **The 2025-26 tackled rate.** The column stops after 2018-19. This is the larger of the two
  transferability caveats and it is *unmeasurable*, not merely unmeasured.
- **The 2025-26 and 2026/27 penalty-save coefficients.** The decode settles pre-2024-25 = 15
  and nothing later; the component columns are gone.
- **The regime span for 2015-16 and 2019-20 through 2023-24.** Same cause.
- **The effect on points of any bonus correction.** Every season the replay scores predates the
  new rules, so there is no cell that could score it. This one is genuinely permanent.
- **The joint CBI-plus-tackled arm** is measurable here and was not run — see above. It is the
  arm that would sign the keeper result.

## Nothing ships changed

`BonusWeight` stays 1.0, `BonusPriorWeight` stays 0.5. All three reasons the note gives for
declining a per-position bonus multiplier survive untouched, and the keeper finding
**strengthens** the decline rather than weakening it: the one position whose multiplier had a
confident sign no longer has one.
