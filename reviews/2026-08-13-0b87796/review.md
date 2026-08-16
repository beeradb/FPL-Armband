# Review: the bench-shape run, whose headline I had backwards

**Commit range reviewed:** `68b6380..bf2da71`. Fixes applied at `c3f6548`, accuracy snapshot at
`0b87796`, which this record is named for because it is the tip the record covers.

**What changed.** A new diagnostic `TestDiagBenchShape` and its four-arm, 36-cell run; a new
exported `SetBenchSlots` with a shared `normaliseBenchSlots`; two guard tests; a findings write-up
at `stats/snapshots/2026-08-13-benchshape/`; and corrections to `internal/analysis/squad.go`,
`docs/notes/optimiser-and-squad.md` and `TODO.md`.

## Reviewers run, and the triage

| reviewer | why |
|---|---|
| **fpl-stats-review** | a new points claim that retired a shipped constant's justification |
| **fpl-code-review** | a new exported mutator on a scoring path, and a refactor of a shipped parser |

Skipped: **fpl-findings-audit** — the claims under audit are this run's own and the statistics
reviewer checked every one against the cells. **fpl-security-review**, **fpl-run-review**,
**fpl-season-maintenance** — nothing in scope.

**Invariants first, and they were the weakest part of this change.** `go build`, `go vet`,
`internal/analysis` and `internal/snapshot` were green throughout, and the harness's own
`HOLD MOVED` markers fired on all three arms. **None of that caught anything**, and one of the two
tests written specifically to pin this change turned out to pin nothing. The accuracy snapshot,
regenerated at `0b87796`, is the invariant that did work: every model figure unchanged, nothing
dropped, only the commit stamp moving.

## Findings, ranked

### 1. The headline conclusion was refuted, and it was an inversion — RETRACTED

The write-up concluded that the tie the derived bench weights ship on was not reproduced, so their
`ViceCaptainWeight` justification was void. **That inverts what a null means.** A tie is the claim
that two settings cannot be separated; a non-resolving comparison is the observation that they
cannot be separated. The null *confirms* the tie, at 36 cells rather than the four the old table
had.

`CLAUDE.md` already carried the rule violated — *"'unresolved' is the expected reading for a real
effect of that size and is not evidence against one"* — and I read it backwards, which retires a
mechanism argument on noise. A new standing rule is added for the mirror image, because the
existing wording only covers one direction.

**What a null can refute is a recorded magnitude**, and that part survives: flat is not 77 behind
anything.

### 2. The separating estimate is one cell, and I quoted its sign split wrongly — RETRACTED

`(2023-24, GW26)` reads −10.538 pts/gw on derived-minus-shipped-tuple and supplies **94%** of that
contrast's mean. Dropping it takes the contrast to −0.020, and flips derived-minus-flat from −0.180
to **+0.120 (+4.6 a season)**. Verified independently against the committed cells before applying.

I also quoted "positive in only 9 of 36" as sign consistency. The split is **9 positive, 15
negative, 12 zero**; the sign test over the 24 cells that moved is p = 0.307. `sweep_inference.R`
prints the warning this ignores — *"a mean that rests on a handful of cells is a one-cell result
whatever its t says"*.

### 3. The arm the conclusion was about is confounded — APPLIED, and it is the best finding here

`normaliseBenchSlots` forces the three fixed tuples to sum to exactly 4, so flat / shipped /
predecessor are a clean shape contrast. **The derived path does not renormalise**, deliberately —
`benchslots.go` says a per-squad normalisation "would throw away exactly the signal this exists to
capture". So the derived arm's weights sum to whatever a real eleven's blank rates imply, about
2.24 behind a sound eleven against 6.48 behind a fragile one.

**The derived arm therefore varies shape and effective `BenchWeight` together**, on a knob whose
recorded structure is a plateau to 0.13 and a cliff after it. The run measures shape for three arms
and shape-plus-level for the fourth. A renormalised derived arm is owed, and with derived-as-shipped
it is the 2x2 the crossing rule actually names.

### 4. Threshold arithmetic understated by 29% — APPLIED

Built as `2 × SE × 38`; `variance_components.R` builds it as `t_crit(df) × SE × 38`, and at the CR2
df of 5 that is **2.571**. Thresholds are 17 / 14 / 40 at p = 0.05 and 24 / 19 / 54 at 80% power,
not 13.5 / 11 / 31. It moves no conclusion and moves it in the safe direction. A second standing
rule records the arithmetic, plus two things I would otherwise have got wrong again: `F_seas` has
~30% power on a 6×6 grid so a large p there is a failure to detect rather than homogeneity, and
clustering made the SE *smaller* here, so the "clustered is conservative" reflex is false on this
sweep.

### 5. The 2022-23 claim — WITHDRAWN

I claimed `squad.go`'s sign on held-out 2022-23 reproduced. Its own mean is −0.372 pts/gw at
**t = −0.71** over six cells, driven by one of them, and three of six seasons are negative for that
arm. **And 2022-23 is not held out in this design** — all six seasons are in the sweep, so there was
no out-of-sample status to reproduce. Awarding "the file that was closer to right" on that is the
failure the exercise was called to fix, committed inside the fix.

### 6. The schedule was one column — NARROWED

`F_start(5,25)` is real for the two tuples, 3.93 (p 0.009) and 4.66 (p 0.004), and start point is a
design factor so it is not an argmax. But the effect is **entirely the GW1 entry**, +2.167 pts/gw
with 6 of 6 seasons positive, against five flat columns. My "early +1.1, middle −0.7, late −0.05"
averaged GW1 with GW6 and concealed that GW6 belongs with the middle — so it is **not** the
`BonusWeight` signature, which was two opposite trends across consecutive columns. Post-hoc over six
columns, so queued in `TODO.md` as a pre-registration with its mechanism rather than quoted.

Not a horizon artefact, which was worth checking: residual variance scales as `weeks^-0.38` against
the assumed −1, and GW1 is the *noisiest* column, so the effect is measured against the largest
spread in the grid.

### 7. What survives, on better ground than I gave it — APPLIED

Both old records are retracted. **The strongest reason is not the data state but that each rests on
four cells**, inside the standing rule that a verdict at twelve or fewer is unverified. And the
decisive check needs no argument about data at all: both are `POLICY` totals at a GW1 entry, and on
exactly that design the current archive puts flat **177-214 ahead**, matching neither record, while
the same cells on `HOLD` put flat **154-234 behind**. Verified independently before applying. The
sign of the whole question depends on which metric is read, and both old records were `POLICY`
while the re-measurement's headline was `HOLD`.

### 8. Code review: four defects in the machinery — ALL APPLIED

- **The guard test tested nothing.** `TestTheFixedTupleIsInertWhileSlotsAreDerived` called
  `benchSlotWeightsFor`, which cannot read the package tuple by construction, so it passed for
  reasons unrelated to the branch it claimed to pin — proven by a mutation making the tuple
  authoritative always, which left it green. Rewritten through `benchValue` with both halves and a
  positive control; **the mutation was re-run against the new test and it now fails**, as it should.
- **`SetBenchSlots` shared the arithmetic but not the input contract.** The env path rejects a
  negative component and returns the default; the setter installed it. And on refusal the setter
  left the *previous arm's* tuple, so a `{0,0,0}` zero control — the obvious arm for this very
  sweep — would have silently replayed its predecessor and reported a byte-identical null. Validation
  moved into `normaliseBenchSlots`; a refused tuple now installs the shipped default.
- **The diagnostic's restore assumed the default and was not deferred.** Under
  `FPL_FIXED_BENCH_SLOTS=1` it would have flipped every later diagnostic in the process onto derived
  slots while the fingerprint still stamped the environment as fixed. `BenchSlotState` added; restore
  is now a `defer` capturing prior state.
- **The setters were invisible to the constants fingerprint.** `envSwitches` exists so a run at a
  non-default tuple cannot be called comparable with the default, and it reads the environment — so
  three non-default arms carried the shipped digest. The setters now keep their env vars in step.

### 9. Smaller applied items

- `FINDINGS.md` claimed the tree was `9317f70`; the cells stamp `68b6380` with `dirty = true`. On an
  exercise whose thesis is that data states are not recoverable, that had to be fixed — the only
  real anchor is `constants_digest = 67455aaa8835`.
- The accuracy snapshot was first rendered without `-cells` and then without
  `TestDiagTransferError`, silently dropping the harness half and then four model rows into "no
  longer measured". Both caught by reading the diff rather than the exit status.

## Declined, with reasons

- **Running the renormalised derived arm now.** Correct and queued rather than done: it is a
  replay of the same order as the run being corrected, and the user has not asked for it. Recorded
  in `TODO.md` with the free `sum(benchSlotWeightsFor(xi))` check that should precede it.
- **Recording the GW1 finding as a result.** It is post-hoc over six columns; queued as a
  pre-registration instead.

## What could not be checked on this harness

- **Whether the derived arm's confound is large.** It needs the free diagnostic above; until then
  "the derived weights are worst" and "the derived arm ran a quieter bench" are indistinguishable.
- **The GW1 column.** The cells that generated the hypothesis cannot test it.
- **Nothing about which bench shape is right.** Four arms, one tie, and that is the whole result.
