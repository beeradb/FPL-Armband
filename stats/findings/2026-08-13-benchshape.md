# The flat-versus-shaped bench contradiction: both records retracted, the tie confirmed

**Run 2026-08-13**, `TestDiagBenchShape`, four arms x 36 cells on the six-season grid, `HOLD`.
Cells in `cells/benchshape.csv`. Baseline is **flat 1/1/1/1**.

⚠️ **Provenance.** The cells stamp `commit = 68b6380` with `dirty = true`, so the only real anchor
is `constants_digest = 67455aaa8835`. A first version of this file claimed a clean `9317f70`; for
an exercise whose whole thesis is that data states are not recoverable, that had to be fixed.

⚠️ **This file was rewritten after statistics review, which refuted its original headline.** The
first version concluded that the derived bench weights' shipping justification was void. That was
wrong, and wrong in an instructive direction — it read a null as refuting a tie. Both readings are
kept below, because the error is more useful than the result.

## What was asked

Two places in the repository gave incompatible answers to "does shaping the bench beat a flat
multiplier":

| source | flat against shaped | held-out 2022-23 |
|---|---|---|
| `docs/notes/optimiser-and-squad.md` | flat **77 behind** all three shaped variants | a **tie**, 1666 v 1666 |
| `internal/analysis/squad.go` | flat **51 worse** on the tuned three | flat **79 better** |

## 1. Both records are retracted, and the simplest reason is the strongest

**Each rests on four cells** — one entry point, four seasons — which is inside `CLAUDE.md`'s own
rule that any verdict reached at twelve cells or fewer is unverified and must be re-judged. That
alone voids them and cannot be argued with. The data-state argument below is true and is the
*second* reason, not the first.

**They are two runs, not two readings of one table.** The `squad.go` block exists to justify
2.4/1.0/0.4/0.2 over 1.9/1.2/0.6/0.3 and scores that pair at "+30 mean, +27 on 2022-23"; the notes
table scores the same pair at +16 and **−44**. Dated `e299f57` (2026-08-06) and `337f83d`
(2026-08-07), both before the xG/xGC backfills — which move 6 of the 24 four-season cells, all six
of them 2022-23 — and before the appearance unification the notes table already flagged.

**And the decisive check is a reproduction on their own design, which needs no argument about data
states at all.** Both recorded tables are **`POLICY` season totals at a GW1 entry on four seasons**
— their magnitudes match `policy_points` in these cells, not `hold_points`. Restricted to exactly
that design:

| arm | POLICY, four seasons, GW1 | HOLD, same cells |
|---|---:|---:|
| **flat 1/1/1/1** | **8927** | 6858 |
| fixed 2.4/1.0/0.4/0.2 | 8750 | 7092 |
| fixed 1.9/1.2/0.6/0.3 | 8750 | 7092 |
| derived from the eleven | 8713 | 7012 |

Flat is **177 to 214 ahead** on the metric the old records used — matching neither the notes table
(−77) nor `squad.go` (+28) — and **154 to 234 behind** on `HOLD` over the identical cells. ⚠️ **The
sign of "does shaping the bench pay" depends on which metric you read**, and both old records were
`POLICY` while this exercise's headline was `HOLD`. Neither record reproduces on its own terms.

## 2. The tie is confirmed, not broken — the original conclusion here was wrong

**HOLD, paired against flat, 36 cells.** Thresholds are per arm from
`stats/variance_components.R`, as `t_crit(df) x SE x 38`:

| arm | pts/gw | a season | SE CR2 | t CR2 | p=0.05 threshold | 80%-power MDE |
|---|---:|---:|---:|---:|---:|---:|
| fixed 2.4/1.0/0.4/0.2 | +0.133 | +5.1 | 0.177 | 0.75 | **17** | 24 |
| fixed 1.9/1.2/0.6/0.3 | +0.058 | +2.2 | 0.144 | 0.40 | **14** | 19 |
| derived from the eleven (ships) | −0.180 | −6.8 | 0.408 | −0.44 | **34-40** | 54 |

⚠️ **The threshold arithmetic in the first version was wrong**: it used `2 x SE x 38`, but at the
CR2 df of 5 the critical value is **2.571**, so every threshold was understated by 29%. It moves no
conclusion and moves it in the safe direction.

⚠️ **Quote the range across estimators, not an end.** `F_seas` has about 30% power on a 6x6 grid,
so its p of 0.52-0.77 here is a failure to detect rather than a demonstration of homogeneity.
Note also that clustering makes the SE *smaller* here (0.250 → 0.177), because the start-point main
effect is 33% of cell variance — so the "clustered is conservative" reflex is false on this sweep.

**The conclusion: a tie.** Four arms spanning about 12 points a season against per-arm thresholds
of 17 to 40 is the textbook description of a tie, now measured at 36 cells instead of 4. **What is
refuted is the recorded magnitude** — flat is not 77 behind anything — and a magnitude is the only
thing a null can refute.

### ⚠️ The error worth keeping: a null is a tie, not the refutation of one

The first version of this file argued that because the point estimates no longer coincided, the tie
the derived weights ship on was gone, and therefore the `ViceCaptainWeight` precedent — *at a tie,
prefer the objective that says what the game actually pays* — no longer reached the choice.

That inverts the meaning of a null. A tie is the claim that two settings cannot be separated; a
non-resolving comparison is the observation that they cannot be separated. The null **confirms**
the tie. `CLAUDE.md` already carries the rule that was violated: *"'unresolved' is the expected
reading for a real effect of that size and is not evidence against one."* Read backwards, that rule
retires a mechanism argument on noise.

**The `ViceCaptainWeight` precedent still reaches. The derived weights keep their justification.**

## 3. The derived arm's negative sign is one cell, and the arm is confounded anyway

**One cell of 36 supplies 94% of it.** `(2023-24, GW26)` reads **−10.538 pts/gw** on
derived-minus-shipped-tuple. Drop it and that contrast is −0.020 pts/gw; on derived-minus-flat the
same cell **flips the sign**, −0.180 becoming **+0.120 (+4.6 a season)**. This is
`sweep_inference.R`'s own printed warning landing as written.

**And "positive in only 9 of 36" was not the sign-consistency statement it was quoted as.** The
split is **9 positive, 15 negative, 12 zero**; a sign test over the 24 cells that moved reads
p = 0.307. The two tuple arms are byte-identical in 26 of 36 cells, so the two contrasts against
them are not independent readings either.

⚠️ **The larger problem: the derived arm is not a shape-only arm.** `normaliseBenchSlots` forces the
three fixed tuples to sum to exactly 4, so flat / shipped / predecessor are a clean shape contrast.
The derived path does not renormalise per squad — deliberately, since that is the signal it exists
to carry — so its four weights sum to whatever a real eleven's blank rates imply: about **2.24**
behind a very sound eleven and **6.48** behind a fragile one, against 4.00 at the reference. So the
derived arm varies **shape and effective `BenchWeight` together**, on a knob whose documented
structure is a plateau to 0.13 and a cliff after it. Since the replay builds near-nailed elevens by
construction, the likely direction is a *quieter* bench than every other arm.

**So this run measures shape for three arms and shape-plus-level for the fourth**, which is the arm
the original conclusion was about. Its figure is unusable as evidence about shape until a
renormalised derived arm is run.

## 4. A start-point structure is real; the schedule reading is not

**Established for the two tuple arms**: `F_start(5,25)` = **3.93 (p 0.009)** and **4.66 (p 0.004)**.
Start point is a design factor, so that F is not an argmax. It is 1.63 (p 0.189) for the derived
arm, so this is not a property of "the bench weights" as a class.

**Not a horizon artefact.** Residual variance scales as `weeks^-0.38` against the assumed −1, and
**GW1 is the noisiest column, not the quietest** — so the GW1 effect is measured against the
largest spread in the grid and still reads t = +3.14.

⚠️ **But it is one column, not three regimes.** Per entry point, shipped tuple: **GW1 +2.167**,
then −0.081, −0.702, −0.478, −0.148, +0.038. The first version's "early +1.1, middle −0.7, late
−0.05" averaged GW1 with GW6 and hid that GW6 belongs with the middle. That is **not** the
`BonusWeight` signature, which was two opposite trends across consecutive columns.

**The GW1 column is the interesting object and is not yet quotable**: unanimous across six seasons
for both tuples (naive sign-test p 0.031), but selected after seeing six columns, so ~0.19 adjusted.
It is queued in `TODO.md` as a pre-registration, with the mechanism it should be written against:
at a GW1 entry the fifteen is built with no in-season minutes, so blanks are both more frequent and
worse predicted, and `HOLD` never repairs them.

## 5. Withdrawn: the 2022-23 sign claim

The first version claimed `squad.go`'s sign on held-out 2022-23 reproduced. **Withdrawn.** Its own
mean is −0.372 pts/gw at **t = −0.71** over six cells, driven by one of them; three of six seasons
are negative for that arm and 2022-23 is not the most negative. **And 2022-23 is not held out in
this design** — all six seasons are in the sweep, so there is no out-of-sample status to reproduce.
Awarding "the file that was closer to right" on that is the exact failure the exercise was called to
fix.

## Pre-registrations, scored

- **P1 — FAILED as stated**, and the failure is uninformative: it asked for all three shaped arms
  to beat flat, and the arm that did not is the confounded one whose sign rests on a single cell.
- **P2 — held.** Expected unresolved; Holm p = 1.000 throughout.
- **P3 — held**, on the corrected reading. The arms tie.
- **P4, the mediator — held.** All three arms are marked `HOLD MOVED`, and the cells confirm it:
  HOLD differs from baseline in 32/36, 29/36 and 35/36 cells. The null is a measurement, not an
  inert arm.

## What is owed

1. **A renormalised derived arm** — derived shape, forced to sum 4 — which with derived-as-shipped
   forms the 2x2 the crossing rule asks for. One extra arm, ~25% more runtime.
2. **A free check first**: print `sum(benchSlotWeightsFor(xi))` averaged over the elevens priced in
   each cell. If it is ≈4 the confound is small; if it is 2.5 or 6, the derived arm's figure is a
   `BenchWeight` result wearing a shape label.
3. **The GW1 pre-registration**, before that column is quoted anywhere.
